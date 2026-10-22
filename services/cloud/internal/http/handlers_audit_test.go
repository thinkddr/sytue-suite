// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package cloudhttp

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/services/cloud/internal/store"
)

// El visor de auditoría solo sirve si los filtros que escribe el admin ACOTAN la
// consulta en el servidor. Filtrar en el cliente sobre "los últimos 50" —que es lo
// que hacía la pantalla— responde a la pregunta equivocada: quien audita busca algo
// de hace tres semanas, no en la última página.
func TestListAuditEmpujaLosFiltrosAlServicio(t *testing.T) {
	f := &fakeSvc{}
	h := newHandlers(f)
	rr := httptest.NewRecorder()
	h.ListAudit(rr, req(http.MethodGet,
		"/audit?actor=ana&action=token.&since=2026-07-01&until=2026-07-10&cursor=tok&limit=25", "", ""))

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, quiero 200: %s", rr.Code, rr.Body.String())
	}
	if f.auditFilter == nil {
		t.Fatal("el servicio no recibió filtro")
	}
	if f.auditFilter.Actor != "ana" || f.auditFilter.Action != "token." {
		t.Errorf("filtro = %+v, quiero actor=ana action=token.", *f.auditFilter)
	}
	if f.auditCursor != "tok" || f.auditLimit != 25 {
		t.Errorf("cursor/limit = %q/%d, quiero tok/25", f.auditCursor, f.auditLimit)
	}
	if f.auditFilter.Since == nil || !f.auditFilter.Since.Equal(time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("since = %v, quiero 2026-07-01", f.auditFilter.Since)
	}
	// «hasta el 10» tiene que INCLUIR el día 10: el filtro es occurred_at < Until,
	// así que una fecha suelta avanza al día siguiente. Sin esto se pierde el último
	// día del rango sin que nadie lo note.
	if f.auditFilter.Until == nil || !f.auditFilter.Until.Equal(time.Date(2026, 7, 11, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("until = %v, quiero 2026-07-11 (día final inclusivo)", f.auditFilter.Until)
	}
}

func TestListAuditDevuelveEventosYCursor(t *testing.T) {
	f := &fakeSvc{
		auditEvents: []store.AuditRecord{{
			ID:         uuid.MustParse(validUUID),
			ActorSub:   "ana@acme.eu",
			Action:     "token.create",
			Resource:   "ci",
			Detail:     json.RawMessage(`{"scope":"deploy"}`),
			OccurredAt: time.Date(2026, 7, 20, 10, 0, 0, 0, time.UTC),
		}},
		nextCursor: "sig",
	}
	rr := httptest.NewRecorder()
	newHandlers(f).ListAudit(rr, req(http.MethodGet, "/audit", "", ""))

	env := decodeEnvelope(t, rr)
	if got := env["next_cursor"]; got != "sig" {
		t.Errorf("next_cursor = %v, quiero sig", got)
	}
	events, _ := env["events"].([]any)
	if len(events) != 1 {
		t.Fatalf("%d eventos, quiero 1: %s", len(events), rr.Body.String())
	}
	e, _ := events[0].(map[string]any)
	// El dashboard lee estas claves; renombrarlas rompe la pantalla en silencio.
	for k, want := range map[string]any{
		"actor_sub": "ana@acme.eu", "action": "token.create", "resource": "ci",
	} {
		if e[k] != want {
			t.Errorf("%s = %v, quiero %v", k, e[k], want)
		}
	}
	// detail viaja como OBJETO (jsonb verbatim), no como cadena escapada.
	d, ok := e["detail"].(map[string]any)
	if !ok || d["scope"] != "deploy" {
		t.Errorf("detail = %#v, quiero un objeto con scope=deploy", e["detail"])
	}
}

func TestListAuditSinEventosDevuelveListaVacia(t *testing.T) {
	// null en vez de [] obliga a cada cliente a defenderse; y sin next_cursor la UI
	// sabe que no hay más páginas.
	rr := httptest.NewRecorder()
	newHandlers(&fakeSvc{}).ListAudit(rr, req(http.MethodGet, "/audit", "", ""))
	if !strings.Contains(rr.Body.String(), `"events":[]`) {
		t.Errorf("sin eventos quiero events:[], salió %s", rr.Body.String())
	}
	if _, ok := decodeEnvelope(t, rr)["next_cursor"]; ok {
		t.Error("sin más páginas no debe haber next_cursor")
	}
}

func TestListAuditRangoInvalidoEs400(t *testing.T) {
	// Un rango vacío o una fecha ilegible devolviendo "todo" es peor que un error:
	// el admin creería que en ese periodo no pasó nada.
	for _, q := range []string{
		"/audit?since=ayer",
		"/audit?until=2026-13-99",
		"/audit?since=2026-07-10&until=2026-07-01",
		"/audit?actor=" + strings.Repeat("x", 201),
	} {
		rr := httptest.NewRecorder()
		newHandlers(&fakeSvc{}).ListAudit(rr, req(http.MethodGet, q, "", ""))
		if rr.Code != http.StatusBadRequest {
			t.Errorf("%s → status %d, quiero 400", q, rr.Code)
		}
	}
}

func TestListAuditExportaCSVConElMismoFiltro(t *testing.T) {
	f := &fakeSvc{auditCSV: "ts,actor,action,target,detail\n"}
	rr := httptest.NewRecorder()
	newHandlers(f).ListAudit(rr, req(http.MethodGet, "/audit?format=CSV&action=billing.", "", ""))

	if !f.auditExported {
		t.Fatal("?format=csv debe exportar, no listar")
	}
	if f.auditFilter == nil || f.auditFilter.Action != "billing." {
		t.Errorf("el export ignoró el filtro: %+v", f.auditFilter)
	}
	if ct := rr.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/csv") {
		t.Errorf("content-type = %q, quiero text/csv", ct)
	}
	// Sin Content-Disposition el navegador PINTA el CSV en vez de descargarlo.
	if cd := rr.Header().Get("Content-Disposition"); !strings.HasPrefix(cd, "attachment;") {
		t.Errorf("content-disposition = %q, quiero una descarga", cd)
	}
	if rr.Body.String() != f.auditCSV {
		t.Errorf("cuerpo = %q, quiero el CSV del servicio", rr.Body.String())
	}
}
