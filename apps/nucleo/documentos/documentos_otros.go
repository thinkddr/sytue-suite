// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build !android && !ios

package documentos

import "context"

func servir(context.Context, Fuente, Textos) error { return ErrNoDisponible }
