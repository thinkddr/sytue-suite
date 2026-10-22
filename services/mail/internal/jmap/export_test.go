// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package jmap

import (
	"encoding/json"
	"io"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/portabilidad"
	"git.sytue.eu/sytue/one/services/mail/internal/domain"
)

func TestExportEntregaCadaEMLEnSuCarpeta(t *testing.T) {
	clientes := uuid.New()
	acme := domain.Mailbox{ID: uuid.New(), Name: "Acme", ParentID: &clientes}
	st := &fakeStore{
		mailboxes: []domain.Mailbox{{ID: clientes, Name: "Clientes"}, acme},
		messages: []domain.Message{{
			ID: uuid.MustParse("0b1c2d3e-0000-4000-8000-000000000000"), Subject: "Pedido 7/2026",
			ReceivedAt: time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC), BlobKey: "k",
		}},
	}
	rec := httptest.NewRecorder()
	newAPI(st, &fakeBlobs{getBody: "Subject: Pedido\r\n\r\nhola"}, nil).exportData(rec, req("GET", "/api/v1/export?format=json", "", true, nil))

	partes := map[string]string{}
	for parte, err := range portabilidad.Leer(rec.Result()) {
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(parte)
		partes[parte.Ruta] = string(b)
	}
	// La barra del asunto no abre una carpeta nueva; la del buzón padre, sí.
	eml := "messages/Clientes/Acme/2026-09-01 100000 Pedido 7-2026 (0b1c2d3e).eml"
	if partes[eml] != "Subject: Pedido\r\n\r\nhola" {
		t.Fatalf("falta %q en %v", eml, partes)
	}
	var filas []mensajeExportado
	if err := json.Unmarshal([]byte(partes["messages.json"]), &filas); err != nil || len(filas) != 2 {
		t.Fatalf("tabla (el fake da el mismo mensaje en cada buzón): %v %v", filas, err)
	}
	if filas[1].Path != eml || filas[1].Subject != "Pedido 7/2026" {
		t.Fatalf("fila: %+v", filas[1])
	}
	for _, tabla := range []string{"mailboxes.json", "labels.json", "filters.json"} {
		if _, ok := partes[tabla]; !ok {
			t.Errorf("falta %s", tabla)
		}
	}
}
