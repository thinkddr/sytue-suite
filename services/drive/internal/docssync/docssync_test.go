// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package docssync

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/services/drive/internal/store"
)

type colaFalsa struct {
	pendientes []store.PendienteDocs
	quitados   []store.PendienteDocs
	aplazados  []store.PendienteDocs
}

func (c *colaFalsa) AplazarPendientesDocs(_ context.Context, f []store.PendienteDocs) ([]store.PendienteDocs, error) {
	c.aplazados = append(c.aplazados, f...)
	return nil, nil
}

func (c *colaFalsa) PendientesDocs(context.Context, int) ([]store.PendienteDocs, error) {
	return c.pendientes, nil
}

func (c *colaFalsa) QuitarPendientesDocs(_ context.Context, e []store.PendienteDocs) error {
	c.quitados = append(c.quitados, e...)
	return nil
}

// TestVuelta: una petición por organización, con el secreto; lo que Docs rechaza
// se queda en la cola, aplazado, para una vuelta siguiente.
func TestVuelta(t *testing.T) {
	var recibido []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var c struct {
			Tenant string `json:"tenant"`
			Files  []struct {
				ID, State string
			} `json:"files"`
		}
		_ = json.NewDecoder(r.Body).Decode(&c)
		if r.URL.Path != RutaDocs || r.Header.Get("Authorization") != "Bearer s3cr3t" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		recibido = append(recibido, c.Tenant)
		if c.Tenant == "caida" {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	cola := &colaFalsa{pendientes: []store.PendienteDocs{
		{Tenant: "acme", FileID: uuid.New(), Estado: store.EstadoPapelera},
		{Tenant: "caida", FileID: uuid.New(), Estado: store.EstadoVivo},
		{Tenant: "acme", FileID: uuid.New(), Estado: store.EstadoPurgado},
	}}
	w := Nuevo(cola, srv.URL+"/", "s3cr3t", 0)
	if err := w.Vuelta(context.Background()); err == nil {
		t.Fatal("un fallo de Docs tiene que decirse")
	}
	if len(recibido) != 2 || recibido[0] != "acme" || recibido[1] != "caida" {
		t.Fatalf("peticiones = %v, quiero una por organización", recibido)
	}
	if len(cola.quitados) != 2 || cola.quitados[0].Tenant != "acme" || cola.quitados[1].Tenant != "acme" {
		t.Fatalf("quitados = %+v; lo de «caida» tiene que quedarse", cola.quitados)
	}
	if len(cola.aplazados) != 1 || cola.aplazados[0].Tenant != "caida" {
		t.Fatalf("aplazados = %+v; lo de «caida» se aplaza para no tapar a nadie", cola.aplazados)
	}
	if Nuevo(cola, " ", "s", 0) != nil || Nuevo(cola, srv.URL, "", 0) != nil {
		t.Fatal("sin URL de Docs o sin secreto no hay worker")
	}
}
