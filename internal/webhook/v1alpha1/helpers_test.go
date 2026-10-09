// SPDX-FileCopyrightText: Copyright (c) 2026 NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import "k8s.io/apimachinery/pkg/util/rand"

func randomSuffix() string {
	return rand.String(6)
}
