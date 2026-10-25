// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package documentos

import "context"

// Abrir muestra la raíz ya conectada en el gestor de archivos del sistema.
// No entrega credenciales a la línea de órdenes ni inicia configuración manual.
func Abrir(ctx context.Context) error { return abrir(ctx) }
