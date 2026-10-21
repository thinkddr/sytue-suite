// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package cloud expone el contrato OpenAPI del servicio como dato embebido.
//
// Vive en la RAÍZ del servicio porque `go:embed` no puede subir de directorio
// («..» no está permitido) y la convención del repo —docs/CONVENTIONS.md §
// estructura— sitúa `openapi.yaml` justo aquí, junto a `cmd/` e `internal/`.
// Mover el fichero para poder embeberlo habría roto esa convención en el único
// servicio que sirve su contrato; un paquete de tres líneas cuesta menos.
package cloud

import _ "embed"

// OpenAPISpec es el contrato OpenAPI 3.1 de la API pública, embebido en el binario
// para que no pueda desincronizarse del despliegue: se sirve la spec del commit que
// está corriendo, no un fichero suelto que alguien olvide copiar a la imagen.
//
//go:embed openapi.yaml
var OpenAPISpec []byte
