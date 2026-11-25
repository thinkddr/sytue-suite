// SPDX-License-Identifier: Apache-2.0
package chathttp

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
)

// clientCoreAssets se declara en client_core_embed.go (desarrollo: el directorio puede
// estar sin generar y la ruta contesta 503) o en client_core_embed_imagen.go (-tags
// imagen, lo que va a la imagen: sin el núcleo generado el binario no compila).
var compiledClientModules = httpx.NewCompilerAssets(clientCoreAssets, map[string]string{"core": "client_core_assets", "vault": "client_core_assets/vault", "office": "client_core_assets/office", "mail": "client_core_assets/mail", "pdf": "client_core_assets/pdf"})

func ClientCore(w http.ResponseWriter, r *http.Request) {
	compiledClientModules.Serve(w, r, chi.URLParam(r, "asset"))
}
