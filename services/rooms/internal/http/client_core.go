// SPDX-License-Identifier: Apache-2.0
package chathttp

import (
	"embed"
	"net/http"

	"github.com/go-chi/chi/v5"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
)

//go:embed client_core_assets
var clientCoreAssets embed.FS
var compiledClientModules = httpx.NewCompilerAssets(clientCoreAssets, map[string]string{"core": "client_core_assets", "office": "client_core_assets/office", "mail": "client_core_assets/mail"})

func ClientCore(w http.ResponseWriter, r *http.Request) {
	compiledClientModules.Serve(w, r, chi.URLParam(r, "asset"))
}
