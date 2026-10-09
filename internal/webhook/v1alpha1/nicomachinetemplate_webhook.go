// SPDX-FileCopyrightText: Copyright (c) 2026 NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

// Package v1alpha1 holds the admission webhooks of the v1alpha1 API.
package v1alpha1

import (
	"context"
	"fmt"
	"reflect"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"sigs.k8s.io/cluster-api/util/topology"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	infrav1 "github.com/dsx-ai-factory/cluster-api-provider-nico/api/v1alpha1"
)

// +kubebuilder:webhook:path=/validate-infrastructure-cluster-x-k8s-io-v1alpha1-nicomachinetemplate,mutating=false,failurePolicy=fail,sideEffects=None,groups=infrastructure.cluster.x-k8s.io,resources=nicomachinetemplates,verbs=update,versions=v1alpha1,name=validation.nicomachinetemplate.infrastructure.cluster.x-k8s.io,admissionReviewVersions=v1

// NicoMachineTemplateValidator keeps NicoMachineTemplate.spec.template.spec immutable.
//
// The one exception is the dry-run request the Cluster API topology controller sends to find out whether a
// template changed before it rotates the MachineDeployment or KubeadmControlPlane to a new template. That
// request is a server-side apply with DryRun set and the topology.cluster.x-k8s.io/dry-run annotation on the
// object, and it never persists anything. A CEL rule cannot make this exception because it sees neither the
// dry-run flag nor the annotation; without it, a ClusterClass cannot change any field of its NICo workers.
type NicoMachineTemplateValidator struct{}

var _ admission.Validator[*infrav1.NicoMachineTemplate] = &NicoMachineTemplateValidator{}

// SetupNicoMachineTemplateWebhookWithManager registers the NicoMachineTemplate validating webhook.
func SetupNicoMachineTemplateWebhookWithManager(mgr ctrl.Manager) error {
	return builder.WebhookManagedBy(mgr, &infrav1.NicoMachineTemplate{}).
		WithValidator(&NicoMachineTemplateValidator{}).
		Complete()
}

// ValidateCreate accepts every template: the CRD schema validates the spec.
func (*NicoMachineTemplateValidator) ValidateCreate(context.Context, *infrav1.NicoMachineTemplate) (admission.Warnings, error) {
	return nil, nil
}

// ValidateUpdate rejects any change to spec.template.spec, unless the request is a topology dry-run.
func (*NicoMachineTemplateValidator) ValidateUpdate(
	ctx context.Context, oldTemplate, newTemplate *infrav1.NicoMachineTemplate,
) (admission.Warnings, error) {
	req, err := admission.RequestFromContext(ctx)
	if err != nil {
		return nil, apierrors.NewBadRequest(fmt.Sprintf("expected an admission request in the context: %v", err))
	}
	if topology.IsDryRunRequest(req, newTemplate) {
		return nil, nil
	}
	if reflect.DeepEqual(newTemplate.Spec.Template.Spec, oldTemplate.Spec.Template.Spec) {
		return nil, nil
	}

	return nil, apierrors.NewInvalid(
		infrav1.GroupVersion.WithKind("NicoMachineTemplate").GroupKind(),
		newTemplate.Name,
		field.ErrorList{field.Forbidden(field.NewPath("spec", "template", "spec"), "spec.template.spec is immutable")},
	)
}

// ValidateDelete accepts every deletion.
func (*NicoMachineTemplateValidator) ValidateDelete(context.Context, *infrav1.NicoMachineTemplate) (admission.Warnings, error) {
	return nil, nil
}
