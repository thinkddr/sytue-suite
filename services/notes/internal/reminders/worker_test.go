// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package reminders

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/services/notes/internal/store"
)

// El aviso lleva a la nota dentro de su cuaderno y dice cuál es; sin título (una nota
// cifrada o sin nombre) va con el tipo que no lo pide, y no lleva ninguno.
func TestAvisoDeRecordatorio(t *testing.T) {
	v := store.RecordatorioVencido{
		Tenant: "acme", OwnerSub: "ada", NoteID: uuid.New(), NotebookID: uuid.New(),
		Title: "Llamar al fontanero", DueAt: time.Date(2026, 10, 1, 7, 0, 0, 0, time.UTC),
	}
	n := Aviso("https://notes.sytue.eu", v)
	if n.Type != TipoAviso || n.Params["title"] != v.Title || n.Subject != "ada" || n.Tenant != "acme" ||
		n.Params["start"] != "2026-10-01T07:00:00Z" || !strings.Contains(n.Title, v.Title) {
		t.Errorf("aviso con título: %+v", n)
	}
	if !strings.HasPrefix(n.URL, "https://notes.sytue.eu/notes/acme/?") ||
		!strings.Contains(n.URL, "note="+v.NoteID.String()) || !strings.Contains(n.URL, "notebook="+v.NotebookID.String()) {
		t.Errorf("el enlace no abre la nota: %q", n.URL)
	}
	// Solo por compartida: el cuaderno no lo ve, así que se abre en «Compartido conmigo».
	v.SoloCompartida = true
	if n := Aviso("https://notes.sytue.eu", v); !strings.Contains(n.URL, "shared="+v.NoteID.String()) ||
		strings.Contains(n.URL, v.NotebookID.String()) {
		t.Errorf("el enlace de una nota solo compartida: %q", n.URL)
	}
	v.SoloCompartida = false
	v.Title = ""
	n = Aviso("https://notes.sytue.eu", v)
	if n.Type != TipoAvisoSinNom || n.Params["title"] != "" || strings.Contains(n.Title, "fontanero") {
		t.Errorf("aviso sin título: %+v", n)
	}
}
