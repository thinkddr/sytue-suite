// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package noteshttp

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/services/notes/internal/domain"
)

// decodificarPapelera pasa la respuesta por JSON, que es lo que ve el cliente: así
// el test cubre también qué campos viajan y cuáles no.
func decodificarPapelera(t *testing.T, r map[string]any) (dias int, notas []map[string]any, crudo string) {
	t.Helper()
	b, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	var out struct {
		Notes         []map[string]any `json:"notes"`
		RetentionDays int              `json:"retention_days"`
	}
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	return out.RetentionDays, out.Notes, string(b)
}

func TestLaPapeleraDiceCuandoSeVaciaCadaNota(t *testing.T) {
	hoy := time.Date(2026, 11, 12, 10, 0, 0, 0, time.UTC)
	notas := []domain.TrashedNote{
		{Note: domain.Note{ID: uuid.New(), Title: "billetes"}, DeletedAt: hoy, NotebookTitle: "Viaje"},
		{Note: domain.Note{ID: uuid.New(), Title: "contrato"}, DeletedAt: hoy, NotebookTitle: "Legal", Held: true},
	}
	dias, out, crudo := decodificarPapelera(t, respuestaPapelera(notas, 30*24*time.Hour))
	if dias != 30 || len(out) != 2 {
		t.Fatalf("retention_days = %d, notas = %d", dias, len(out))
	}
	if out[0]["notebook_title"] != "Viaje" || out[0]["title"] != "billetes" || out[0]["deleted_at"] == nil {
		t.Fatalf("la fila no lleva cuaderno, título o fecha de borrado: %v", out[0])
	}
	if out[0]["purge_at"] != hoy.Add(30*24*time.Hour).Format(time.RFC3339) {
		t.Fatalf("purge_at = %v, quería deleted_at + 30 días", out[0]["purge_at"])
	}
	// La retenida no caduca, y la respuesta no dice POR QUÉ: un hold no se anuncia.
	if _, ok := out[1]["purge_at"]; ok {
		t.Fatalf("una nota bajo retención legal no debe traer purge_at: %v", out[1])
	}
	if strings.Contains(crudo, "held") {
		t.Fatalf("la respuesta delata el hold: %s", crudo)
	}
}

func TestLaPapeleraSinPurgaNoPoneFecha(t *testing.T) {
	notas := []domain.TrashedNote{{Note: domain.Note{ID: uuid.New()}, DeletedAt: time.Now()}}
	dias, out, _ := decodificarPapelera(t, respuestaPapelera(notas, 0))
	if dias != 0 {
		t.Fatalf("retention_days = %d con la purga apagada", dias)
	}
	if _, ok := out[0]["purge_at"]; ok {
		t.Fatalf("con TRASH_RETENTION=0 nada caduca: %v", out[0])
	}
	// Vacía, la lista es [] y no null: el cliente la recorre sin comprobar.
	_, vacia, crudo := decodificarPapelera(t, respuestaPapelera(nil, time.Hour))
	if vacia == nil || !strings.Contains(crudo, `"notes":[]`) {
		t.Fatalf("papelera vacía = %s", crudo)
	}
	// Un plazo que no es de días enteros se redondea hacia arriba.
	if d, _, _ := decodificarPapelera(t, respuestaPapelera(nil, 36*time.Hour)); d != 2 {
		t.Fatalf("36 h son %d días, quería 2", d)
	}
}
