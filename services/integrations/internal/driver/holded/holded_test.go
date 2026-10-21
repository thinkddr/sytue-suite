// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package holded

import (
	"context"
	"encoding/json"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/ssrfhttp"
	"git.sytue.eu/sytue/one/services/integrations/internal/connector"
	"git.sytue.eu/sytue/one/services/integrations/internal/policy"
)

func runtime(t *testing.T) connector.Runtime {
	t.Helper()
	t.Cleanup(ssrfhttp.SetIPPolicyForTest(func(net.IP) bool { return false }))
	return policy.BuildRuntime(policy.RuntimeParams{OrgID: "org-1", InstanceID: "inst-1", Log: slog.Default()})
}

// La ruta, la cabecera `key`, la paginación y las dos traducciones (fecha Unix y
// dirección anidada) son lo que decide si una importación trae datos o nada: el
// test afirma cada una contra la petición real, no solo el resultado.
func TestQueryPaginaAplanaYTraduceFechas(t *testing.T) {
	var paginas []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/invoicing/v1/documents/invoice" {
			t.Errorf("ruta: %s", r.URL.Path)
		}
		if r.Header.Get("key") != "clave-holded" || r.Header.Get("Authorization") != "" {
			t.Errorf("auth: key=%q Authorization=%q", r.Header.Get("key"), r.Header.Get("Authorization"))
		}
		paginas = append(paginas, r.URL.Query().Get("page"))
		switch r.URL.Query().Get("page") {
		case "1":
			_, _ = w.Write([]byte(`[{"id":"f1","date":1767225600,"dueDate":1769904000,"subtotal":1234.56,"tax":259.26,"total":1493.82,"status":1,"billAddress":{"city":"Valencia"}},
				{"id":"f2","date":1767312000,"subtotal":10,"tax":2.1,"total":12.1,"status":0}]`))
		default:
			_, _ = w.Write([]byte(`[]`))
		}
	}))
	defer srv.Close()

	inst := connector.Instance{Config: map[string]string{ConfigBaseURL: srv.URL + "/api/invoicing/v1"}, Secrets: map[string]string{"api_key": "clave-holded"}}
	spec, _ := json.Marshal(consulta{Resource: "documents/invoice", Fields: []string{"id", "date", "dueDate", "total", "status", "billAddress.city"}})
	tabla, err := New().Query(context.Background(), inst, runtime(t), spec)
	if err != nil {
		t.Fatal(err)
	}
	if len(paginas) != 2 || paginas[0] != "1" || paginas[1] != "2" {
		t.Fatalf("paginación: %v", paginas)
	}
	col := map[string]int{}
	for i, c := range tabla.Columns {
		col[c.Name] = i
	}
	if len(tabla.Rows) != 2 {
		t.Fatalf("filas: %d", len(tabla.Rows))
	}
	f := tabla.Rows[0]
	if f[col["date"]] != "2026-01-01" || f[col["dueDate"]] != "2026-02-01" || f[col["status"]] != "1" || f[col["billAddress.city"]] != "Valencia" {
		t.Fatalf("traducciones: %v", f)
	}
	if n, ok := f[col["total"]].(json.Number); !ok || n.String() != "1493.82" {
		t.Fatalf("el importe no debe pasar por float: %#v", f[col["total"]])
	}
}

func TestQueryRechazaRecursosFueraDeLaLista(t *testing.T) {
	spec, _ := json.Marshal(consulta{Resource: "../../admin"})
	if _, err := New().Query(context.Background(), connector.Instance{Secrets: map[string]string{"api_key": "k"}}, connector.Runtime{}, spec); err == nil {
		t.Fatal("un recurso fuera de la lista no se pide")
	}
}

func TestLaColeccionQueNoPaginaNoDaVueltas(t *testing.T) {
	n := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n++
		_, _ = w.Write([]byte(`[{"id":"p1","name":"Café"}]`))
	}))
	defer srv.Close()
	inst := connector.Instance{Config: map[string]string{ConfigBaseURL: srv.URL}, Secrets: map[string]string{"api_key": "k"}}
	// Por `dataset`, que es como pregunta Sheets.
	spec := json.RawMessage(`{"dataset":"products"}`)
	tabla, err := New().Query(context.Background(), inst, runtime(t), spec)
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 || len(tabla.Rows) != 1 {
		t.Fatalf("peticiones=%d filas=%d", n, len(tabla.Rows))
	}
}
