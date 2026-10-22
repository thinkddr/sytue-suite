// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package chathttp

import (
	"encoding/json"
	"io"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/portabilidad"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
)

func TestExportPaginaLosMensajesPropios(t *testing.T) {
	inicio := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	f := &fakeService{teams: []domain.Team{{ID: uuid.New()}}, channels: []domain.Channel{{Name: "general"}}}
	// Más de una página, para que el cursor tenga que avanzar.
	for i := range paginaExport + 3 {
		f.ownMessages = append(f.ownMessages, domain.Message{ID: uuid.New(), Body: "hola", CreatedAt: inicio.Add(time.Duration(i) * time.Second)})
	}
	rec := httptest.NewRecorder()
	newHandlers(f).Export(rec, req("GET", "/api/v1/export?format=json", "", claims(), nil))

	partes := map[string][]byte{}
	for parte, err := range portabilidad.Leer(rec.Result()) {
		if err != nil {
			t.Fatal(err)
		}
		partes[parte.Ruta], _ = io.ReadAll(parte)
	}
	var mensajes []domain.Message
	if err := json.Unmarshal(partes["messages.json"], &mensajes); err != nil || len(mensajes) != paginaExport+3 {
		t.Fatalf("mensajes: %d (%v)", len(mensajes), err)
	}
	var canales []domain.Channel
	if err := json.Unmarshal(partes["channels.json"], &canales); err != nil || len(canales) != 1 {
		t.Fatalf("canales: %v %v", canales, err)
	}
}
