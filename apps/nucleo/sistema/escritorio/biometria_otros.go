// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build !darwin && !windows && !android

package escritorio

import "context"

func biometria() (string, bool) { return "", false }

func verificar(context.Context, string) error { return ErrNoDisponible }
