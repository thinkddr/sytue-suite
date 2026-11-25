// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

// El registro de solicitudes RGPD y la supresión física, contra PostgreSQL real.
//
// Lo que importa no es que se guarde una fila: es que el plazo que se le enseña a
// quien administra sea el de la ley, que lo cerrado no se reescriba, que la
// supresión borre de verdad —y sin ventana de recuperación— y que NO alcance lo
// que no es de la organización: una cuenta que también usa otra organización, o
// la de alguien de fuera.
package http

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	"git.sytue.eu/sytue/one/services/id/internal/domain"
	"git.sytue.eu/sytue/one/services/id/internal/oidc"
	"git.sytue.eu/sytue/one/services/id/internal/store"
)

// tokenSinReautenticar son los claims de quien entró hace horas y solo con
// contraseña: no pasan requireStepUp.
func tokenSinReautenticar(sub, tenant string) oidc.TokenClaims {
	return oidc.TokenClaims{
		Subject: sub, Audience: "sytue-console", Scope: "openid", Tenant: tenant, Roles: []string{"admin"},
		AuthTime: time.Now().Add(-6 * time.Hour).UTC(), AMR: []string{"pwd"},
	}
}

func TestSolicitudesRGPDRegistroYPlazos(t *testing.T) {
	l := newLiveServer(t)
	org, admin := l.seedOrgAdmin(t, "acme")
	tok := l.token(t, admin.ID, org.Slug, []string{"admin"})
	miembro := l.addMember(t, org, "rosa@acme.test", domain.RoleMember)

	// Por correo de una cuenta de la organización: se enlaza sola, y el plazo es un
	// mes desde la RECEPCIÓN, no desde que se registra.
	recibida := time.Now().AddDate(0, 0, -10).UTC().Format("2006-01-02")
	rr, body := l.do(t, http.MethodPost, "/api/v1/admin/privacy/requests", tok, map[string]any{
		"subject_email": "ROSA@acme.test", "kind": "access", "channel": "email", "received_at": recibida,
		"identity_verified": true, "identity_method": "respuesta desde su correo corporativo",
	})
	if rr.Code != http.StatusCreated {
		t.Fatalf("alta: %d %v", rr.Code, body)
	}
	sol := body["request"].(map[string]any)
	if sol["subject_user_id"] != miembro.ID || sol["subject_email"] != "rosa@acme.test" || sol["status"] != "received" {
		t.Fatalf("solicitud: %v", sol)
	}
	rec, _ := time.Parse(time.RFC3339Nano, sol["received_at"].(string))
	vence, _ := time.Parse(time.RFC3339Nano, sol["due_at"].(string))
	if !vence.Equal(rec.AddDate(0, 1, 0)) {
		t.Fatalf("plazo %v desde %v: no es un mes", vence, rec)
	}
	id := sol["id"].(string)

	// Alguien de fuera (un cliente): solo su correo, sin cuenta.
	rr, body = l.do(t, http.MethodPost, "/api/v1/admin/privacy/requests", tok, map[string]any{
		"subject_email": "cliente@fuera.test", "subject_name": "Cliente", "kind": "erasure", "channel": "letter",
	})
	if rr.Code != http.StatusCreated || body["request"].(map[string]any)["subject_user_id"] != nil {
		t.Fatalf("solicitud de alguien de fuera: %d %v", rr.Code, body)
	}
	deFuera := body["request"].(map[string]any)["id"].(string)

	// Validación: tipo, canal, fecha futura o de hace más de un año.
	for _, malo := range []map[string]any{
		{"subject_email": "x@y.test", "kind": "olvido", "channel": "email"},
		{"subject_email": "x@y.test", "kind": "access", "channel": "paloma"},
		{"subject_email": "no-es-un-correo", "kind": "access", "channel": "email"},
		{"subject_email": "x@y.test", "kind": "access", "channel": "email", "received_at": time.Now().AddDate(0, 0, 3).Format("2006-01-02")},
		{"subject_email": "x@y.test", "kind": "access", "channel": "email", "received_at": time.Now().AddDate(-2, 0, 0).Format("2006-01-02")},
	} {
		if rr, _ := l.do(t, http.MethodPost, "/api/v1/admin/privacy/requests", tok, malo); rr.Code != http.StatusBadRequest {
			t.Errorf("alta inválida %v: %d", malo, rr.Code)
		}
	}

	// La prórroga: una vez, con motivo, dos meses más.
	if rr, _ := l.do(t, http.MethodPost, "/api/v1/admin/privacy/requests/"+id+"/extend", tok, map[string]any{"reason": ""}); rr.Code != http.StatusBadRequest {
		t.Fatalf("prórroga sin motivo: %d", rr.Code)
	}
	rr, body = l.do(t, http.MethodPost, "/api/v1/admin/privacy/requests/"+id+"/extend", tok, map[string]any{"reason": "Diez años de correo que revisar"})
	if rr.Code != http.StatusOK {
		t.Fatalf("prórroga: %d %v", rr.Code, body)
	}
	prorrogada, _ := time.Parse(time.RFC3339Nano, body["request"].(map[string]any)["due_at"].(string))
	if !prorrogada.Equal(vence.AddDate(0, 2, 0)) {
		t.Fatalf("prórroga hasta %v; quiero %v", prorrogada, vence.AddDate(0, 2, 0))
	}
	if rr, _ := l.do(t, http.MethodPost, "/api/v1/admin/privacy/requests/"+id+"/extend", tok, map[string]any{"reason": "otra vez"}); rr.Code != http.StatusConflict {
		t.Fatalf("segunda prórroga: %d", rr.Code)
	}

	// Cerrar exige resolución; cerrada, ya no cambia.
	if rr, _ := l.do(t, http.MethodPatch, "/api/v1/admin/privacy/requests/"+id, tok, map[string]any{"status": "completed"}); rr.Code != http.StatusBadRequest {
		t.Fatalf("cierre sin resolución: %d", rr.Code)
	}
	rr, body = l.do(t, http.MethodPatch, "/api/v1/admin/privacy/requests/"+id, tok, map[string]any{
		"status": "completed", "resolution": "Copia entregada el día 12 por enlace cifrado.",
	})
	if rr.Code != http.StatusOK || body["request"].(map[string]any)["closed_at"] == nil {
		t.Fatalf("cierre: %d %v", rr.Code, body)
	}
	if rr, _ := l.do(t, http.MethodPatch, "/api/v1/admin/privacy/requests/"+id, tok, map[string]any{"notes": "retoque"}); rr.Code != http.StatusConflict {
		t.Fatalf("editar una cerrada: %d", rr.Code)
	}

	// La historia, en orden y solo de añadir.
	rr, body = l.do(t, http.MethodGet, "/api/v1/admin/privacy/requests/"+id, tok, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("detalle: %d", rr.Code)
	}
	eventos, _ := body["events"].([]any)
	acciones := make([]string, 0, len(eventos))
	for _, e := range eventos {
		acciones = append(acciones, e.(map[string]any)["action"].(string))
	}
	if strings.Join(acciones, ",") != "created,extended,updated" {
		t.Fatalf("historia: %v", acciones)
	}
	if _, err := l.st.DB().Exec(l.ctx, `UPDATE dsr_request_events SET action = 'otra' WHERE request_id = $1`, id); err == nil {
		t.Fatal("la historia de una solicitud se pudo reescribir")
	}
	if body["erasure"].(map[string]any)["reason"] != "not_erasure" {
		t.Fatalf("una solicitud de acceso no ofrece supresión: %v", body["erasure"])
	}

	// El listado: filtros y recuento de vencidas.
	if _, err := l.st.DB().Exec(l.ctx, `UPDATE dsr_requests SET due_at = now() - interval '1 day' WHERE id = $1`, deFuera); err != nil {
		t.Fatal(err)
	}
	rr, body = l.do(t, http.MethodGet, "/api/v1/admin/privacy/requests?status=open", tok, nil)
	if rr.Code != http.StatusOK || len(body["requests"].([]any)) != 1 {
		t.Fatalf("abiertas: %d %v", rr.Code, body)
	}
	if s := body["summary"].(map[string]any); s["open"] != float64(1) || s["overdue"] != float64(1) {
		t.Fatalf("recuento: %v", s)
	}
	if rr, _ := l.do(t, http.MethodGet, "/api/v1/admin/privacy/requests?status=raro", tok, nil); rr.Code != http.StatusBadRequest {
		t.Fatalf("filtro desconocido: %d", rr.Code)
	}

	// Frontera de organización: lo de otra no se ve ni se toca, y un miembro raso
	// no administra el registro.
	otra, otroAdmin := l.seedOrgAdmin(t, "otra")
	otroTok := l.token(t, otroAdmin.ID, otra.Slug, []string{"admin"})
	if rr, _ := l.do(t, http.MethodGet, "/api/v1/admin/privacy/requests/"+id, otroTok, nil); rr.Code != http.StatusNotFound {
		t.Fatalf("detalle desde otra organización: %d", rr.Code)
	}
	if rr, body := l.do(t, http.MethodGet, "/api/v1/admin/privacy/requests", otroTok, nil); rr.Code != http.StatusOK || len(body["requests"].([]any)) != 0 {
		t.Fatalf("listado desde otra organización: %d %v", rr.Code, body)
	}
	if rr, _ := l.do(t, http.MethodPost, "/api/v1/admin/privacy/requests", otroTok, map[string]any{
		"user_id": miembro.ID, "kind": "erasure", "channel": "email",
	}); rr.Code != http.StatusNotFound {
		t.Fatalf("enlazar una cuenta de otra organización: %d", rr.Code)
	}
	miembroTok := l.token(t, miembro.ID, org.Slug, []string{"member"})
	if rr, _ := l.do(t, http.MethodGet, "/api/v1/admin/privacy/requests", miembroTok, nil); rr.Code != http.StatusForbidden {
		t.Fatalf("un miembro raso leyó el registro: %d", rr.Code)
	}

	// Retención: lo cerrado hace más de tres años se va con su historia.
	if _, err := l.st.DB().Exec(l.ctx, `UPDATE dsr_requests SET closed_at = now() - interval '4 years' WHERE id = $1`, id); err != nil {
		t.Fatal(err)
	}
	if n, err := l.st.PurgarSolicitudesRGPDVencidas(l.ctx); err != nil || n != 1 {
		t.Fatalf("retención: %d %v", n, err)
	}
}

func TestSupresionFisicaRGPD(t *testing.T) {
	l := newLiveServer(t)
	org, admin := l.seedOrgAdmin(t, "acme")
	tok := l.token(t, admin.ID, org.Slug, []string{"admin"})
	pablo := l.addMember(t, org, "pablo@acme.test", domain.RoleMember)
	l.liveSession(t, org, pablo.ID, "sesion-pablo")

	crear := func(body map[string]any) string {
		t.Helper()
		rr, out := l.do(t, http.MethodPost, "/api/v1/admin/privacy/requests", tok, body)
		if rr.Code != http.StatusCreated {
			t.Fatalf("alta: %d %v", rr.Code, out)
		}
		return out["request"].(map[string]any)["id"].(string)
	}
	id := crear(map[string]any{"user_id": pablo.ID, "kind": "erasure", "channel": "email"})

	// Confirmación por escrito y reautenticación reciente.
	if rr, _ := l.do(t, http.MethodPost, "/api/v1/admin/privacy/requests/"+id+"/erase", tok, map[string]any{"confirm": "otro@acme.test"}); rr.Code != http.StatusBadRequest {
		t.Fatalf("sin la confirmación correcta: %d", rr.Code)
	}
	viejo, err := l.iss.IssueAccessToken(tokenSinReautenticar(admin.ID, org.Slug), time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if rr, _ := l.do(t, http.MethodPost, "/api/v1/admin/privacy/requests/"+id+"/erase", viejo, map[string]any{"confirm": "pablo@acme.test"}); rr.Code != http.StatusUnauthorized && rr.Code != http.StatusForbidden {
		t.Fatalf("sin reautenticación: %d", rr.Code)
	}

	rr, body := l.do(t, http.MethodPost, "/api/v1/admin/privacy/requests/"+id+"/erase", tok, map[string]any{"confirm": "Pablo@acme.test"})
	if rr.Code != http.StatusOK {
		t.Fatalf("supresión: %d %v", rr.Code, body)
	}
	// En el acto: baja, sin sesiones, sin restauración posible.
	if st, _ := l.estado(t, pablo.ID); st != "deleted" {
		t.Fatalf("estado tras la supresión: %s", st)
	}
	if n := l.cuenta(t, `SELECT count(*) FROM sessions WHERE user_id = $1`, pablo.ID); n != 0 {
		t.Fatalf("quedan %d sesiones", n)
	}
	if rr, _ := l.do(t, http.MethodPost, "/api/v1/admin/users/"+pablo.ID+"/restore", tok, nil); rr.Code != http.StatusConflict {
		t.Fatalf("una supresión RGPD se pudo restaurar: %d", rr.Code)
	}
	// No se da por resuelta antes de que el barrido termine.
	if rr, _ := l.do(t, http.MethodPatch, "/api/v1/admin/privacy/requests/"+id, tok, map[string]any{
		"status": "completed", "resolution": "Cuenta suprimida.",
	}); rr.Code != http.StatusConflict {
		t.Fatalf("resolver una supresión en curso: %d", rr.Code)
	}

	// El barrido: sin esperar los 30 días, en cuanto caduca el último access token.
	if _, err := l.st.DB().Exec(l.ctx, `UPDATE users SET closed_at = now() - interval '1 hour' WHERE id = $1`, pablo.ID); err != nil {
		t.Fatal(err)
	}
	l.srv.basesDeServicio = nil
	pend, err := l.st.CuentasPropiasPendientes(l.ctx, time.Now(), time.Now().Add(-retencionBaja), 10)
	if err != nil || len(pend) != 1 || pend[0].Supresion != id {
		t.Fatalf("la supresión no entra en el barrido: %+v %v", pend, err)
	}
	if err := l.st.MarcarDatosPersonalesBorrados(l.ctx, pablo.ID); err != nil {
		t.Fatal(err)
	}
	if p, err := l.st.PurgarCuentaPropia(l.ctx, pablo.ID, time.Now().Add(-retencionBaja)); err != nil || p == nil {
		t.Fatalf("purga de la supresión: %v %v", p, err)
	}
	if err := l.st.CompletarSupresionRGPD(l.ctx, nil, id); err != nil {
		t.Fatal(err)
	}
	var email string
	if err := l.st.DB().QueryRow(l.ctx, `SELECT email FROM users WHERE id = $1`, pablo.ID).Scan(&email); err != nil || strings.Contains(email, "pablo") {
		t.Fatalf("ficha sin anonimizar: %q %v", email, err)
	}
	rr, body = l.do(t, http.MethodGet, "/api/v1/admin/privacy/requests/"+id, tok, nil)
	if rr.Code != http.StatusOK || body["request"].(map[string]any)["erasure_completed_at"] == nil ||
		body["erasure"].(map[string]any)["reason"] != "completed" {
		t.Fatalf("la solicitud no refleja la supresión terminada: %v", body)
	}
	// Ahora sí se puede contestar.
	if rr, _ := l.do(t, http.MethodPatch, "/api/v1/admin/privacy/requests/"+id, tok, map[string]any{
		"status": "completed", "resolution": "Cuenta suprimida.",
	}); rr.Code != http.StatusOK {
		t.Fatalf("resolver la supresión terminada: %d", rr.Code)
	}

	// Lo que no es de la organización no se suprime desde aquí: una cuenta que
	// también es de otra organización, y la propia cuenta de quien administra.
	ana := l.addMember(t, org, "ana@acme.test", domain.RoleMember)
	otra, err := l.st.CreateOrg(l.ctx, nil, domain.Organization{Slug: "otra", Name: "Otra"})
	if err != nil {
		t.Fatal(err)
	}
	if err := l.st.UpsertMembership(l.ctx, nil, domain.Membership{OrgID: otra.ID, UserID: ana.ID, Role: domain.RoleMember}); err != nil {
		t.Fatal(err)
	}
	idAna := crear(map[string]any{"user_id": ana.ID, "kind": "erasure", "channel": "email"})
	if rr, _ := l.do(t, http.MethodPost, "/api/v1/admin/privacy/requests/"+idAna+"/erase", tok, map[string]any{"confirm": "ana@acme.test"}); rr.Code != http.StatusConflict {
		t.Fatalf("suprimir una cuenta que también es de otra organización: %d", rr.Code)
	}
	if st, _ := l.estado(t, ana.ID); st != "active" {
		t.Fatalf("la supresión negada tocó la cuenta: %s", st)
	}
	idPropia := crear(map[string]any{"user_id": admin.ID, "kind": "erasure", "channel": "email"})
	if rr, _ := l.do(t, http.MethodPost, "/api/v1/admin/privacy/requests/"+idPropia+"/erase", tok, map[string]any{"confirm": admin.Email}); rr.Code != http.StatusForbidden {
		t.Fatalf("suprimir la propia cuenta desde la consola: %d", rr.Code)
	}
	// Ni una solicitud que no es de supresión, ni una de alguien sin cuenta.
	idAcceso := crear(map[string]any{"user_id": ana.ID, "kind": "access", "channel": "email"})
	if rr, _ := l.do(t, http.MethodPost, "/api/v1/admin/privacy/requests/"+idAcceso+"/erase", tok, map[string]any{"confirm": "ana@acme.test"}); rr.Code != http.StatusConflict {
		t.Fatalf("suprimir desde una solicitud de acceso: %d", rr.Code)
	}
}

// TestRegistroRGPDConfinadoPorRLS: el registro va con la RLS de 0127 (mig 0130), y
// la ruta lo confina a la organización activa. Con el ámbito de una organización, ni
// un SQL sin `org_id` ve las solicitudes de otra, ni su historia, ni su purga
// pendiente (mig 0129), ni deja escribir en ellas; sin ámbito —el barrido—, todo.
func TestRegistroRGPDConfinadoPorRLS(t *testing.T) {
	l := newLiveServer(t)
	orgA, adminA := l.seedOrgAdmin(t, "rgpd-a")
	orgB, adminB := l.seedOrgAdmin(t, "rgpd-b")
	tokA := l.token(t, adminA.ID, orgA.Slug, []string{"admin"})
	tokB := l.token(t, adminB.ID, orgB.Slug, []string{"admin"})
	alta := func(tok string) string {
		t.Helper()
		rr, out := l.do(t, http.MethodPost, "/api/v1/admin/privacy/requests", tok, map[string]any{
			"subject_email": "cliente@fuera.test", "kind": "access", "channel": "email",
		})
		if rr.Code != http.StatusCreated {
			t.Fatalf("alta: %d %v", rr.Code, out)
		}
		return out["request"].(map[string]any)["id"].(string)
	}
	idA, idB := alta(tokA), alta(tokB)
	if err := l.st.EncolarPurgaDeTenant(l.ctx, nil, orgB.ID, store.PurgaCuentaBorrada); err != nil {
		t.Fatal(err)
	}

	// Por la puerta: A no ve la de B, ni la cambia.
	if rr, _ := l.do(t, http.MethodGet, "/api/v1/admin/privacy/requests/"+idB, tokA, nil); rr.Code != http.StatusNotFound {
		t.Fatalf("A lee la solicitud de B: %d", rr.Code)
	}
	if rr, _ := l.do(t, http.MethodPost, "/api/v1/admin/privacy/requests/"+idB+"/extend", tokA, map[string]any{"reason": "complejidad"}); rr.Code != http.StatusNotFound {
		t.Fatalf("A prorroga la solicitud de B: %d", rr.Code)
	}
	rr, body := l.do(t, http.MethodGet, "/api/v1/admin/privacy/requests", tokA, nil)
	if lista, _ := body["requests"].([]any); rr.Code != http.StatusOK || len(lista) != 1 {
		t.Fatalf("listado de A: %d %v", rr.Code, body)
	}

	// Por la base, con SQL que no filtra: lo que para la política y no el método.
	contar := func(ctx context.Context, sql string, args ...any) int {
		t.Helper()
		var n int
		if err := l.st.DB().QueryRow(ctx, sql, args...).Scan(&n); err != nil {
			t.Fatalf("%s: %v", sql, err)
		}
		return n
	}
	conA := store.ConAmbitoDeOrganizacion(l.ctx, orgA.ID, orgA.Slug)
	for sql, quiero := range map[string]int{
		`SELECT count(*) FROM dsr_requests`:       1,
		`SELECT count(*) FROM dsr_request_events`: 1,
		`SELECT count(*) FROM tenant_purges`:      0,
	} {
		if n := contar(conA, sql); n != quiero {
			t.Errorf("con el ámbito de A, %s = %d; quiero %d", sql, n, quiero)
		}
	}
	if _, err := l.st.DB().Exec(conA, `INSERT INTO dsr_request_events (request_id, action) VALUES ($1, 'otra')`, idB); !esViolacionRLS(err) {
		t.Errorf("con el ámbito de A, escribir en la historia de B: %v; quiero 42501", err)
	}
	if n := contar(conA, `SELECT count(*) FROM dsr_request_events WHERE request_id = $1`, idA); n != 1 {
		t.Errorf("la historia propia: %d eventos", n)
	}
	// Sin ámbito —el barrido de purgas y el de retención— se ve todo.
	if n := contar(l.ctx, `SELECT count(*) FROM dsr_requests`); n != 2 {
		t.Errorf("sin ámbito: %d solicitudes, quiero 2", n)
	}
	if n := contar(l.ctx, `SELECT count(*) FROM tenant_purges`); n != 1 {
		t.Errorf("sin ámbito: %d purgas, quiero 1", n)
	}
}

func esViolacionRLS(err error) bool {
	var pe *pgconn.PgError
	return errors.As(err, &pe) && pe.Code == "42501"
}
