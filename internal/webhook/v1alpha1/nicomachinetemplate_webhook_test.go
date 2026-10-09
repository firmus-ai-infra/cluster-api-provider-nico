// SPDX-FileCopyrightText: Copyright (c) 2026 NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	infrav1 "github.com/dsx-ai-factory/cluster-api-provider-nico/api/v1alpha1"
)

const immutableMessage = "spec.template.spec is immutable"

func newTemplate(name string) *infrav1.NicoMachineTemplate {
	return &infrav1.NicoMachineTemplate{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
		Spec: infrav1.NicoMachineTemplateSpec{
			Template: infrav1.NicoMachineTemplateResource{
				Spec: infrav1.NicoMachineSpec{
					VPCID:          "vpc-1",
					InstanceTypeID: "instance-type-1",
					Interfaces:     []infrav1.NicoMachineInterface{{SubnetID: "subnet-1"}},
				},
			},
		},
	}
}

var _ = Describe("NicoMachineTemplate validating webhook", func() {
	var (
		ctx      context.Context
		template *infrav1.NicoMachineTemplate
	)

	BeforeEach(func() {
		ctx = context.Background()
		template = newTemplate("template-" + randomSuffix())
		Expect(k8sClient.Create(ctx, template)).To(Succeed())
	})

	AfterEach(func() {
		Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, template))).To(Succeed())
	})

	It("rejects a change to spec.template.spec", func() {
		template.Spec.Template.Spec.InstanceTypeID = "instance-type-2"
		err := k8sClient.Update(ctx, template)
		Expect(err).To(MatchError(ContainSubstring(immutableMessage)))
	})

	It("allows metadata changes", func() {
		template.Labels = map[string]string{"owner": "test"}
		Expect(k8sClient.Update(ctx, template)).To(Succeed())
	})

	It("allows the topology controller's dry-run and persists nothing", func() {
		// The topology controller probes with a server-side apply dry-run carrying the dry-run annotation.
		probe := topologyProbe(template, "instance-type-2")
		Expect(k8sClient.Apply(ctx, probe, client.ForceOwnership, client.FieldOwner("capi-topology"),
			client.DryRunAll)).To(Succeed())

		stored := &infrav1.NicoMachineTemplate{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: template.Name, Namespace: template.Namespace}, stored)).To(Succeed())
		Expect(stored.Spec.Template.Spec.InstanceTypeID).To(Equal("instance-type-1"))
		Expect(stored.Annotations).NotTo(HaveKey(clusterv1.TopologyDryRunAnnotation))
	})

	It("still rejects a real update that only carries the dry-run annotation", func() {
		probe := topologyProbe(template, "instance-type-2")
		err := k8sClient.Apply(ctx, probe, client.ForceOwnership, client.FieldOwner("capi-topology"))
		Expect(err).To(MatchError(ContainSubstring(immutableMessage)))
	})

	It("still rejects a dry-run without the topology annotation", func() {
		template.Spec.Template.Spec.InstanceTypeID = "instance-type-2"
		err := k8sClient.Update(ctx, template, client.DryRunAll)
		Expect(err).To(MatchError(ContainSubstring(immutableMessage)))
	})
})

// topologyProbe builds the apply configuration the topology controller sends: the full desired template with
// the dry-run annotation, from an unstructured object so server-side apply only owns the listed fields.
func topologyProbe(template *infrav1.NicoMachineTemplate, instanceTypeID string) runtime.ApplyConfiguration {
	probe := &unstructured.Unstructured{}
	probe.SetGroupVersionKind(infrav1.GroupVersion.WithKind("NicoMachineTemplate"))
	probe.SetName(template.Name)
	probe.SetNamespace(template.Namespace)
	probe.SetAnnotations(map[string]string{clusterv1.TopologyDryRunAnnotation: ""})
	Expect(unstructured.SetNestedField(probe.Object, "vpc-1", "spec", "template", "spec", "vpcID")).To(Succeed())
	Expect(unstructured.SetNestedField(probe.Object, instanceTypeID, "spec", "template", "spec", "instanceTypeID")).To(Succeed())
	Expect(unstructured.SetNestedSlice(probe.Object, []any{map[string]any{"subnetID": "subnet-1"}},
		"spec", "template", "spec", "interfaces")).To(Succeed())

	return client.ApplyConfigurationFromUnstructured(probe)
}
