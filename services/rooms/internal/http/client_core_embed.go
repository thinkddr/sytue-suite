// SPDX-License-Identifier: Apache-2.0

//go:build !imagen

package chathttp

import "embed"

// En desarrollo el núcleo de cliente puede no estar generado (lo genera
// scripts/client-core-assets.sh): el binario compila igual y /api/v1/client-core/
// contesta 503, nunca un éxito vacío. La compilación que va a la imagen usa
// client_core_embed_imagen.go, que no lo tolera.
//
//go:embed client_core_assets
var clientCoreAssets embed.FS
