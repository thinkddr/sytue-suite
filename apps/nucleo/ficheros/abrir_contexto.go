// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ficheros

import "context"

// Abrir entrega un fichero al visor asociado por la persona.
func Abrir(ruta string) error { return AbrirContexto(context.Background(), ruta) }

// AbrirContexto permite cancelar antes de entregar el fichero al sistema. Una vez
// abierto, el visor conserva su propia vida, independiente de esta operación.
func AbrirContexto(ctx context.Context, ruta string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return abrirConContexto(ctx, ruta)
}
