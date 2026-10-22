// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package http

import (
	"encoding/json"
	"net/http"
	"testing"

	"git.sytue.eu/sytue/one/services/id/internal/domain"
	"git.sytue.eu/sytue/one/services/id/internal/store"
)

// La puerta de la cola de denuncias del operador: solo pasa quien está elevado,
// y cada paso queda en el registro de plataforma y en el de la organización
// afectada. Un fallo aquí deja leer contenido de clientes sin rastro.
func TestPlatformModeracion_SoloElevadoYConTraza(t *testing.T) {
	l := newLiveServer(t)
	conMFA := []string{"pwd", "mfa"}
	op := pimActor(t, l, "operadora@id.test")
	nadie := l.mkUser(t, "admin-de-org@id.test", "").ID
	org, err := l.st.CreateOrg(l.ctx, nil, domain.Organization{Slug: "acme-mod", Name: "Acme"})
	if err != nil {
		t.Fatal(err)
	}
	cuerpo := `{"service":"cast","action":"report.resolve","org":"acme-mod","target":"r-1","detail":{"action":"dismiss"}}`
	ruta := "/api/v1/platform/moderation/access"

	// Sin allowlist, o en ella pero sin elevarse: 403 y nada en el registro.
	for _, sub := range []string{nadie, op} {
		if rec := l.pimReq(t, http.MethodPost, ruta, sub, conMFA, cuerpo); rec.Code != http.StatusForbidden {
			t.Fatalf("%s sin elevar: %d %s", sub, rec.Code, rec.Body)
		}
	}
	if rec := l.pimReq(t, http.MethodPost, "/api/v1/platform/activate", op, conMFA,
		`{"justification":"revisar denuncias","duration_minutes":30}`); rec.Code != http.StatusOK {
		t.Fatalf("activación: %d %s", rec.Code, rec.Body)
	}
	// Elevada pero sin segundo factor en ESTE token: 403.
	if rec := l.pimReq(t, http.MethodPost, ruta, op, []string{"pwd"}, cuerpo); rec.Code != http.StatusForbidden {
		t.Fatalf("sin segundo factor: %d", rec.Code)
	}
	// Lo que no está en la lista cerrada, 400.
	for _, malo := range []string{
		`{"service":"drive","action":"reports.list"}`,
		`{"service":"cast","action":"reports.borrar"}`,
		`{"service":"cast","action":"reports.list","org":"NO VALE"}`,
	} {
		if rec := l.pimReq(t, http.MethodPost, ruta, op, conMFA, malo); rec.Code != http.StatusBadRequest {
			t.Fatalf("%s: %d", malo, rec.Code)
		}
	}
	if rec := l.pimReq(t, http.MethodPost, ruta, op, conMFA, cuerpo); rec.Code != http.StatusNoContent {
		t.Fatalf("elevada: %d %s", rec.Code, rec.Body)
	}

	// Una fila, con quién, qué, sobre qué y desde dónde; y la ve la organización.
	filas, _, err := l.st.ListAuditPage(l.ctx, org.ID, store.AuditFilter{Action: "platform.moderation."}, nil, 10)
	if err != nil || len(filas) != 1 {
		t.Fatalf("registro de la organización: %d filas, %v", len(filas), err)
	}
	f := filas[0]
	var det map[string]any
	_ = json.Unmarshal(f.Detail, &det)
	if f.Action != "platform.moderation.cast.report.resolve" || f.ActorID != op || f.Target != "r-1" ||
		det["action"] != "dismiss" || det["client"] != "sytue-console" {
		t.Fatalf("fila: %+v %v", f, det)
	}
	if todas, _, _ := l.st.ListPlatformAuditPage(l.ctx, store.AuditFilter{Action: "platform.moderation."}, nil, 10); len(todas) != 1 {
		t.Fatalf("registro de plataforma: %d filas", len(todas))
	}
}
