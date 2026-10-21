// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ficheros

import "context"

// Fotografiar abre la cámara del sistema para UNA foto y devuelve la ruta donde quedó,
// en la caché privada de la app: el `capture="environment"` de la web. Cancelar no es un
// error: devuelve "", nil. Solo en Android; en el resto, ErrNoDisponible, y quien llama
// ofrece elegir fotos ya hechas. BLOQUEA mientras la cámara está abierta.
func Fotografiar(ctx context.Context) (string, error) { return fotografiar(ctx) }
