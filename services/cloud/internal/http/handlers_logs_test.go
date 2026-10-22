// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package cloudhttp

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/services/cloud/internal/domain"
	"git.sytue.eu/sytue/one/services/cloud/internal/service"
)

// Buscar logs solo sirve si el texto y el rango ACOTAN la consulta en el
// servidor: filtrar en el cliente sobre «lo último que quepa en la página»
// responde a la pregunta equivocada — quien busca en un log busca el error de
// anoche, no el de hace treinta segundos.
func TestSearchLogsEmpujaLosFiltrosAlServicio(t *testing.T) {
	f := &fakeSvc{}
	rr := httptest.NewRecorder()
	newHandlers(f).SearchLogs(rr, req(http.MethodGet,
		"/projects/x/logs?q=timeout&since=2026-07-01&until=2026-07-10&cursor=tok&limit=25", validUUID, ""))

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, quiero 200: %s", rr.Code, rr.Body.String())
	}
	if f.logProject != uuid.MustParse(validUUID) {
		t.Errorf("proyecto = %s, quiero %s", f.logProject, validUUID)
	}
	if f.logFilter == nil || f.logFilter.Query != "timeout" {
		t.Fatalf("filtro = %+v, quiero q=timeout", f.logFilter)
	}
	if f.logCursor != "tok" || f.logLimit != 25 {
		t.Errorf("cursor/limit = %q/%d, quiero tok/25", f.logCursor, f.logLimit)
	}
	if f.logFilter.Since == nil || !f.logFilter.Since.Equal(time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("since = %v, quiero 2026-07-01", f.logFilter.Since)
	}
	// «hasta el 10» INCLUYE el día 10 (el filtro es ts < Until), igual que en la
	// auditoría: si no, se pierde el último día del rango sin avisar.
	if f.logFilter.Until == nil || !f.logFilter.Until.Equal(time.Date(2026, 7, 11, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("until = %v, quiero 2026-07-11 (día final inclusivo)", f.logFilter.Until)
	}
}

func TestSearchLogsDevuelveLineasYCursor(t *testing.T) {
	f := &fakeSvc{logPage: service.LogPage{
		Lines: []domain.LogLine{{
			ID: uuid.MustParse(validUUID), DeploymentID: uuid.MustParse(validUUID),
			TS: time.Date(2026, 7, 1, 10, 0, 0, 0, time.UTC), Text: "ERROR timeout",
		}},
		NextCursor: "siguiente",
	}}
	rr := httptest.NewRecorder()
	newHandlers(f).SearchLogs(rr, req(http.MethodGet, "/projects/x/logs", validUUID, ""))

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, quiero 200", rr.Code)
	}
	body := decodeEnvelope(t, rr)
	got, _ := body["lines"].([]any)
	if len(got) != 1 {
		t.Fatalf("devueltas %d líneas, quiero 1: %s", len(got), rr.Body.String())
	}
	if body["next_cursor"] != "siguiente" {
		t.Errorf("next_cursor = %v, quiero 'siguiente'", body["next_cursor"])
	}
	// Sin líneas, `lines` tiene que ser [] y no null: un cliente que itere sobre
	// null revienta.
	rr = httptest.NewRecorder()
	newHandlers(&fakeSvc{}).SearchLogs(rr, req(http.MethodGet, "/projects/x/logs", validUUID, ""))
	if !strings.Contains(rr.Body.String(), `"lines":[]`) {
		t.Errorf("sin resultados el cuerpo es %s; quiero lines:[]", rr.Body.String())
	}
	if _, ok := decodeEnvelope(t, rr)["next_cursor"]; ok {
		t.Error("sin más páginas no debe haber next_cursor")
	}
}

func TestSearchLogsRangoInvalidoEs400(t *testing.T) {
	for _, q := range []string{
		"/projects/x/logs?since=anoche",
		"/projects/x/logs?until=2026-13-99",
		"/projects/x/logs?since=2026-07-10&until=2026-07-01",
		"/projects/x/logs?q=" + strings.Repeat("x", 201),
	} {
		rr := httptest.NewRecorder()
		newHandlers(&fakeSvc{}).SearchLogs(rr, req(http.MethodGet, q, validUUID, ""))
		if rr.Code != http.StatusBadRequest {
			t.Errorf("%s → status %d, quiero 400", q, rr.Code)
		}
	}
}
