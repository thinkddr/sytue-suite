// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build !linux || android

package documentos

import "context"

func prepararApertura(context.Context, Identidad, string) (*Apertura, error) {
	return nil, ErrNoDisponible
}
