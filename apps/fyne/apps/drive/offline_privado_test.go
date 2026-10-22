// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package drive

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/offline"
)

// TestLaCacheNoGuardaContenidoDeEquipoNiElSecretoDelNAS (FYS-05): con las rutas de
// Drive, ni el contenido de un fichero de una unidad de equipo ni el estado del NAS
// (que trae el secreto del túnel) quedan en el disco; un listado sí.
func TestLaCacheNoGuardaContenidoDeEquipoNiElSecretoDelNAS(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"command":"newt --secret S3CR3T"}`))
	}))
	defer srv.Close()
	almacen, err := offline.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	tr := &offline.Transport{Store: almacen, Owner: func() string { return "ana@acme" }, Product: "drive", Private: SinGuardarSinConexion}
	pedir := func(base http.RoundTripper, ruta string) (int, error) {
		req, _ := http.NewRequest(http.MethodGet, srv.URL+ruta, http.NoBody)
		c := &http.Client{Transport: base}
		resp, err := c.Do(req)
		if err != nil {
			return 0, err
		}
		defer func() { _ = resp.Body.Close() }()
		_, _ = io.Copy(io.Discard, resp.Body)
		return resp.StatusCode, nil
	}
	rutas := []string{"/api/v1/team-drives/t1/files/f1/content", "/api/v1/storage/external/nas/status", "/api/v1/folders/raiz"}
	for _, r := range rutas {
		if _, err := pedir(tr, r); err != nil {
			t.Fatal(err)
		}
	}
	srv.Close() // sin red: solo contesta lo guardado
	for i, r := range rutas {
		_, err := pedir(tr, r)
		guardada := err == nil
		if quiero := i == 2; guardada != quiero {
			t.Errorf("%s: guardada=%v, quiero %v", r, guardada, quiero)
		}
	}
}
