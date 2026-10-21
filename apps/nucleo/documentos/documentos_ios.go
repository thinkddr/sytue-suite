// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build ios

package documentos

import "context"

func servir(ctx context.Context, f Fuente, t Textos) error {
	return ServirIOS(ctx, f, t)
}
