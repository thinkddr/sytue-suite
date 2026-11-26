// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build !windows

package escritorio

// ejecutarToast: los toasts programados son de Windows.
func ejecutarToast(string) error { return ErrNoDisponible }
