// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package drivehttp

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/services/drive/internal/config"
)

// TestDavEnrutaLosVerbosDeWebDAV: chi solo conoce los métodos HTTP estándar, y sin
// httpx.EnableWebDAVMethods un PROPFIND a /dav se quedaba en el router con 405
// (`route=desconocida`), antes de llegar a la autenticación y a WebDAV. Así estuvo
// Drive en el Finder y en Archivos (File Provider) y en todo cliente WebDAV: el log
// de producción del 2026-11-06 son solo PROPFIND → 405. Sin credenciales, cada verbo
// tiene que llegar a /dav y salir con el 401 que pide la contraseña de aplicación.
func TestDavEnrutaLosVerbosDeWebDAV(t *testing.T) {
	llego := false
	r := NewRouter(Deps{
		Cfg:          config.Config{PublicURL: "https://drive.sytue.eu"},
		AppPasswords: authn.NewAppPasswords("http://127.0.0.1:1", "secreto-de-prueba"),
		WebDAV: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			llego = true
			w.WriteHeader(http.StatusMultiStatus)
		}),
	})
	for _, metodo := range []string{"PROPFIND", "PROPPATCH", "MKCOL", "COPY", "MOVE", "LOCK", "UNLOCK"} {
		for _, ruta := range []string{"/dav/", "/dav/carpeta/fichero.txt"} {
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, httptest.NewRequest(metodo, ruta, http.NoBody))
			if rec.Code != http.StatusUnauthorized {
				t.Errorf("%s %s → %d; quiero 401 de la contraseña de aplicación (405 = verbo sin registrar en chi)", metodo, ruta, rec.Code)
			}
			if rec.Header().Get("WWW-Authenticate") == "" {
				t.Errorf("%s %s sin WWW-Authenticate: no llegó a /dav", metodo, ruta)
			}
		}
	}
	if llego {
		t.Error("una petición sin credenciales llegó a WebDAV")
	}
}
