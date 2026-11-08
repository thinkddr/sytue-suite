// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package hubhttp

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// La señal de cambios (`datos`) entra por la puerta de estado solo con un producto de
// la lista cerrada, y sale con {"producto"} y NADA más: lo que el emisor añadiera iría
// por el flujo a todas las pantallas de la persona. Visto en rojo dejando pasar
// `req.Data` tal cual (salía el título) y sin `datos` en eventosDeEstado (400).
func TestLaSenalDeCambiosSoloLlevaElProducto(t *testing.T) {
	n := &fakeNotifier{}
	h := newHandlers(nil, n, nil, nil, nil)
	mandar := func(cuerpo string) int {
		rec := httptest.NewRecorder()
		h.CreateInternalEvent(rec, request(t, http.MethodPost, "/internal/events", nil, cuerpo))
		return rec.Code
	}
	if c := mandar(`{"tenant":"acme","subject":"ana","event":"datos","data":{"producto":"calendar","title":"Cena con Luis"}}`); c != http.StatusAccepted {
		t.Fatalf("datos de calendar: HTTP %d, quiero 202", c)
	}
	if n.evento != "datos" || n.datos != `{"producto":"calendar"}` {
		t.Fatalf("sale %s %s, quiero datos {\"producto\":\"calendar\"}", n.evento, n.datos)
	}
	for _, cuerpo := range []string{
		`{"tenant":"acme","subject":"ana","event":"datos","data":{"producto":"avatar"}}`,
		`{"tenant":"acme","subject":"ana","event":"datos","data":{}}`,
		`{"tenant":"acme","subject":"ana","event":"datos"}`,
		`{"tenant":"acme","subject":"ana","event":"datos","data":["mail"]}`,
	} {
		n.evento = ""
		if c := mandar(cuerpo); c != http.StatusBadRequest || n.evento != "" {
			t.Errorf("%s: HTTP %d (publicado %q), quiero 400 sin publicar", cuerpo, c, n.evento)
		}
	}
}
