// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package chathttp

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/operador"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
	"git.sytue.eu/sytue/one/services/rooms/internal/store"
)

// fakeModeracion apunta con qué identidad llega cada llamada al servicio.
type fakeModeracion struct {
	reporter, blocker, unblocker, moderador string
	orgAdmin                                bool
}

func (f *fakeModeracion) Report(_ context.Context, rec store.NewReport) (uuid.UUID, error) {
	f.reporter = rec.Reporter
	return uuid.New(), nil
}

func (f *fakeModeracion) ListReports(_ context.Context, actor string, orgAdmin, _ bool) ([]domain.Report, error) {
	f.moderador, f.orgAdmin = actor, orgAdmin
	return nil, nil
}

func (f *fakeModeracion) ResolveReport(_ context.Context, _ uuid.UUID, actor string, orgAdmin bool, _ string) (domain.Report, error) {
	f.moderador, f.orgAdmin = actor, orgAdmin
	return domain.Report{}, nil
}

func (f *fakeModeracion) BlockMember(_ context.Context, _ uuid.UUID, actor, _ string) (domain.Block, error) {
	f.blocker = actor
	return domain.Block{}, nil
}

func (f *fakeModeracion) ListBlocks(_ context.Context, actor string) ([]domain.Block, error) {
	f.blocker = actor
	return nil, nil
}

func (f *fakeModeracion) Unblock(_ context.Context, actor, _ string) error {
	f.unblocker = actor
	return nil
}

// Nadie denuncia, bloquea ni desbloquea EN NOMBRE DE OTRO: el actor es el del
// token, venga lo que venga en el cuerpo o en la ruta. El `sub` de la ruta es
// siempre el OTRO (a quién), nunca quién lo hace.
func TestModeracion_ElActorEsElDelToken(t *testing.T) {
	f := &fakeModeracion{}
	h := NewModeracionHandlers(f)
	id := uuid.NewString()
	cuerpo := `{"reason":"spam"}`

	// Un cuerpo que intenta decir quién denuncia ni siquiera entra: 400.
	rec := httptest.NewRecorder()
	h.ReportMessage(rec, req(http.MethodPost, "/", `{"reason":"spam","reporter":"victima"}`, claims(), messageID(id)))
	if rec.Code != http.StatusBadRequest || f.reporter != "" {
		t.Fatalf("cuerpo con «reporter»: code=%d reporter=%q, quiero 400 y nada registrado", rec.Code, f.reporter)
	}

	h.ReportMessage(httptest.NewRecorder(), req(http.MethodPost, "/", cuerpo, claims(), messageID(id)))
	if f.reporter != "user-1" {
		t.Fatalf("denuncia de mensaje a nombre de %q, quiero el del token", f.reporter)
	}
	f.reporter = ""
	h.ReportMember(httptest.NewRecorder(), req(http.MethodPost, "/", cuerpo, claims(),
		map[string]string{"channelID": id, "sub": "victima"}))
	if f.reporter != "user-1" {
		t.Fatalf("denuncia de persona a nombre de %q, quiero el del token", f.reporter)
	}
	h.Block(httptest.NewRecorder(), req(http.MethodPut, "/", "", claims(),
		map[string]string{"channelID": id, "sub": "victima"}))
	if f.blocker != "user-1" {
		t.Fatalf("bloqueo a nombre de %q, quiero el del token", f.blocker)
	}
	h.Unblock(httptest.NewRecorder(), req(http.MethodDelete, "/", "", claims(), map[string]string{"sub": "victima"}))
	if f.unblocker != "user-1" {
		t.Fatalf("desbloqueo a nombre de %q, quiero el del token", f.unblocker)
	}
}

// La cola de toda la organización es de sus administradores. El rol sale de los
// claims verificados y de ningún otro sitio: ni de la query ni del cuerpo.
func TestModeracion_AdminDeOrganizacionSoloPorRolDelToken(t *testing.T) {
	f := &fakeModeracion{}
	h := NewModeracionHandlers(f)

	h.ListReports(httptest.NewRecorder(), req(http.MethodGet, "/?admin=1&role=owner", "", claims(), nil))
	if f.orgAdmin || f.moderador != "user-1" {
		t.Fatalf("un miembro sin rol entra como admin de organización (orgAdmin=%v, actor=%q)", f.orgAdmin, f.moderador)
	}
	f.moderador = ""
	h.ResolveReport(httptest.NewRecorder(), req(http.MethodPost, "/", `{"action":"dismissed"}`,
		claims(), map[string]string{"reportID": uuid.NewString()}))
	if f.orgAdmin || f.moderador != "user-1" {
		t.Fatal("un miembro sin rol resuelve como admin de organización")
	}

	c := claims()
	c.Roles = []string{"admin"}
	h.ListReports(httptest.NewRecorder(), req(http.MethodGet, "/", "", c, nil))
	if !f.orgAdmin {
		t.Fatal("un admin de la organización no entra como tal")
	}
}

// El bloqueo responde 403 con código propio y un texto que no dice quién bloqueó.
func TestErrBlockedEs403ConCodigo(t *testing.T) {
	rec := httptest.NewRecorder()
	f := &fakeService{messageErr: domain.ErrBlocked}
	newHandlers(f).PostMessage(rec, req(http.MethodPost, "/", `{"body":"hola"}`, claims(), channelID(uuid.NewString())))
	e, _ := decode(t, rec)["error"].(map[string]any)
	if rec.Code != http.StatusForbidden || e["code"] != "blocked" {
		t.Fatalf("code=%d body=%s, quiero 403 «blocked»", rec.Code, rec.Body.String())
	}
}

// fakeOperador es la cola del operador vista desde el handler.
type fakeOperador struct {
	org, actor string
	resuelta   bool
}

func (f *fakeOperador) ListReportsOperador(_ context.Context, actor string, _ bool) ([]domain.Report, error) {
	f.actor = actor
	return []domain.Report{{Tenant: "acme"}}, nil
}

func (f *fakeOperador) OrgDeDenuncia(_ context.Context, id uuid.UUID) (string, error) {
	if id == uuid.Nil {
		return "", domain.ErrNotFound
	}
	return "acme", nil
}

func (f *fakeOperador) ResolveReportOperador(_ context.Context, org string, _ uuid.UUID, actor, _ string) (domain.Report, error) {
	f.org, f.actor, f.resuelta = org, actor, true
	return domain.Report{}, nil
}

// La cola del operador: solo con un token de la consola de plataforma y el sí de
// Sytue ID, que se entera de qué se hace y sobre qué organización. Quien
// administra una organización no entra, aunque traiga su rol; y quien no es
// operador no sabe si una denuncia existe.
func TestModeracion_ColaDelOperador(t *testing.T) {
	f := &fakeOperador{}
	var pedidos []operador.Acceso
	var respuesta error
	h := NewModeracionHandlers(&fakeModeracion{}).ConOperador(f, func(_ context.Context, _ string, a operador.Acceso) error {
		pedidos = append(pedidos, a)
		return respuesta
	})
	admin := claims()
	admin.Roles, admin.Audience = []string{"owner"}, []string{"sytue-chat"}
	consola := claims()
	consola.Audience = []string{"sytue-platform"}
	id := uuid.NewString()
	resolver := func(c *authn.Claims, rid string) int {
		rec := httptest.NewRecorder()
		h.ResolveReportOperador(rec, req(http.MethodPost, "/", `{"action":"dismissed"}`, c, map[string]string{"reportID": rid}))
		return rec.Code
	}
	listar := func(c *authn.Claims) int {
		rec := httptest.NewRecorder()
		h.ListReportsOperador(rec, req(http.MethodGet, "/", "", c, nil))
		return rec.Code
	}

	if listar(admin) != http.StatusForbidden || resolver(admin, id) != http.StatusForbidden || len(pedidos) != 0 {
		t.Fatalf("un admin de organización entra en la cola del operador (pedidos=%v)", pedidos)
	}
	respuesta = operador.ErrNoEsOperador
	if listar(consola) != http.StatusForbidden || resolver(consola, id) != http.StatusForbidden || f.actor != "" {
		t.Fatal("quien no es operador abre la cola")
	}
	if resolver(consola, uuid.Nil.String()) != http.StatusForbidden {
		t.Fatal("quien no es operador sabe que una denuncia no existe")
	}
	respuesta = errors.New("sin red")
	if listar(consola) != http.StatusServiceUnavailable {
		t.Fatal("sin respuesta de Sytue ID no es 503")
	}

	respuesta, pedidos = nil, nil
	if listar(consola) != http.StatusOK || f.actor != "user-1" || pedidos[0].Servicio != "rooms" || pedidos[0].Accion != "reports.list" {
		t.Fatalf("cola: actor=%q pedidos=%+v", f.actor, pedidos)
	}
	if resolver(consola, uuid.Nil.String()) != http.StatusNotFound {
		t.Fatal("operadora, denuncia que no existe")
	}
	pedidos = nil
	if resolver(consola, id) != http.StatusOK || !f.resuelta || f.org != "acme" || f.actor != "user-1" {
		t.Fatalf("resolver: %+v", f)
	}
	if p := pedidos[0]; p.Accion != "report.resolve" || p.Org != "acme" || p.Objetivo != id || p.Detalle["action"] != "dismissed" {
		t.Fatalf("lo que se dijo a Sytue ID: %+v", p)
	}
}
