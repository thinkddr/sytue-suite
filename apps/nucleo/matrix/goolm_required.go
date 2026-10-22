// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build !goolm

package matrix

// Native builds must select the maintained Go implementation, never libolm C.
// This deliberately fails early rather than silently linking a retired engine.
var _ = nativeMatrixRequiresBuildTagGoolm
