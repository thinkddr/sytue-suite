// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build !linux

package documentos

import "context"

func abrir(context.Context) error { return ErrNoDisponible }
