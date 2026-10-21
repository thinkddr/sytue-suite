// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package migrate

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/ssrfhttp"
	"git.sytue.eu/sytue/one/services/integrations/internal/policy"
)

// NEG-07: una URL que DEVUELVE el tercero (la descarga pre-autenticada de Graph, el
// blob crudo de un gist) se pide sin token, y eso tiene que seguir siendo verdad
// cuando la conexión guarda uno: el egress se lo añadía a toda petición sin
// Authorization, así que viajaba al host que dijera la respuesta.
func TestUnaURLDelTerceroNoLlevaElTokenDeLaConexion(t *testing.T) {
	t.Cleanup(ssrfhttp.SetIPPolicyForTest(func(net.IP) bool { return false }))
	var cabeceras []string
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		cabeceras = append(cabeceras, r.Header.Get("Authorization"))
	}))
	defer srv.Close()
	rt := policy.BuildRuntime(policy.RuntimeParams{
		OrgID: "org-1", InstanceID: "inst-1", Bearer: "token-guardado", Log: slog.Default(),
	})

	// La descarga que devuelve el tercero: sin token.
	cuerpo, err := openStream(context.Background(), rt, srv.URL+"/descarga", "")
	if err != nil {
		t.Fatal(err)
	}
	_ = cuerpo.Close()
	// Una llamada a la API con el token que el origen acuñó: lo lleva.
	cuerpo, err = openStream(context.Background(), rt, srv.URL+"/api", "token-del-origen")
	if err != nil {
		t.Fatal(err)
	}
	_ = cuerpo.Close()

	if len(cabeceras) != 2 || cabeceras[0] != "" || cabeceras[1] != "Bearer token-del-origen" {
		t.Fatalf("cabeceras = %q, quiero la primera vacía y la segunda con el token del origen", cabeceras)
	}
}
