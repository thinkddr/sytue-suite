// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

// Tests de integración de la administración de usuarios de una organización desde
// Console (paridad Sytue ID) contra un PostgreSQL real (pgtest, build-tag
// `integration`): invitación por email (token válido activa la cuenta + membresía;
// caducado/reusado rechazan), autorización (no-admin → 403; aislamiento cross-org
// → 404), guard del último admin (409), cambio de rol, quitar miembro (revoca
// sesiones) y reset por email disparado por admin. Reusa el harness newLiveServer.
package http

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/services/id/internal/domain"
)

// captureMailer captura los enlaces (reseteo/invitación) en vez de enviarlos: el
// token en claro solo viaja en el email (en BD vive su hash), así que capturarlo
// permite seguir el flujo de extremo a extremo.
type captureMailer struct {
	resetLink, inviteLink, inviteOrg, inviteTo string
	verifyLink, verifyTo                       string
	// Avisos de cambio de alcance de plataforma: a quién se mandó y qué decía.
	// `mu` porque el aviso sale en segundo plano y el test lo lee desde el suyo.
	mu             sync.Mutex
	scopeTo        []string
	scopeSujeto    string
	scopeActor     string
	scopeMotivo    string
	scopeConcedido bool
}

func (m *captureMailer) SendPasswordReset(_ context.Context, _, link string) error {
	m.resetLink = link
	return nil
}

func (m *captureMailer) SendInvitation(_ context.Context, to, org, link string) error {
	m.inviteTo, m.inviteOrg, m.inviteLink = to, org, link
	return nil
}

func (m *captureMailer) SendEmailVerification(_ context.Context, to, link string) error {
	m.verifyTo, m.verifyLink = to, link
	return nil
}

// Los avisos de alcance de plataforma se capturan para poder comprobar A QUIÉN
// se avisa, que es la mitad del valor: al afectado y al resto de admin globales.
func (m *captureMailer) SendPlatformScopeChange(_ context.Context, to, sujeto, actor, motivo string, concedido bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.scopeTo = append(m.scopeTo, to)
	m.scopeSujeto, m.scopeActor, m.scopeMotivo, m.scopeConcedido = sujeto, actor, motivo, concedido
	return nil
}

func tokenFromLink(t *testing.T, link string) string {
	t.Helper()
	u, err := url.Parse(link)
	if err != nil {
		t.Fatalf("parse link %q: %v", link, err)
	}
	tok := u.Query().Get("token")
	if tok == "" {
		t.Fatalf("enlace sin token: %q", link)
	}
	return tok
}

// seedOrgAdmin crea una org con un admin (con membresía) y devuelve (org, admin).
func (l *liveServer) seedOrgAdmin(t *testing.T, slug string) (domain.Organization, domain.User) {
	t.Helper()
	org, err := l.st.CreateOrg(l.ctx, nil, domain.Organization{Slug: slug, Name: strings.ToUpper(slug)})
	if err != nil {
		t.Fatalf("CreateOrg(%q): %v", slug, err)
	}
	admin := l.mkUser(t, "admin@"+slug+".test", "admin-pass-123")
	if err := l.st.UpsertMembership(l.ctx, nil, domain.Membership{
		OrgID: org.ID, UserID: admin.ID, Role: domain.RoleAdmin,
	}); err != nil {
		t.Fatalf("UpsertMembership admin: %v", err)
	}
	return org, admin
}

func (l *liveServer) addMember(t *testing.T, org domain.Organization, email string, role domain.Role) domain.User {
	t.Helper()
	u := l.mkUser(t, email, "member-pass-123")
	if err := l.st.UpsertMembership(l.ctx, nil, domain.Membership{OrgID: org.ID, UserID: u.ID, Role: role}); err != nil {
		t.Fatalf("UpsertMembership(%q): %v", email, err)
	}
	return u
}

func (l *liveServer) liveSession(t *testing.T, org domain.Organization, userID, tokenHash string) {
	t.Helper()
	now := time.Now()
	if _, err := l.st.InsertSession(l.ctx, nil, domain.Session{
		TokenHash: domain.HashToken(tokenHash), UserID: userID, OrgID: org.ID,
		AuthTime: now, AMR: []string{"pwd"}, UserAgent: "test", ExpiresAt: now.Add(time.Hour),
	}); err != nil {
		t.Fatalf("InsertSession: %v", err)
	}
}

// --- A) Invitar / aceptar ---

func TestInvitation_InviteAcceptFlow(t *testing.T) {
	l := newLiveServer(t)
	mc := &captureMailer{}
	l.srv.mailer = mc
	org, admin := l.seedOrgAdmin(t, "acme")
	adminTok := l.token(t, admin.ID, org.Slug, []string{"admin"})

	// Invitar por email → 201 y correo capturado (con el enlace de aceptación).
	rr, _ := l.do(t, http.MethodPost, "/api/v1/admin/invitations", adminTok,
		map[string]string{"email": "Newbie@acme.test", "role": "member"})
	if rr.Code != http.StatusCreated {
		t.Fatalf("invitar: status=%d cuerpo=%s", rr.Code, rr.Body)
	}
	if mc.inviteLink == "" || mc.inviteTo != "newbie@acme.test" {
		t.Fatalf("invitación no enviada/normalizada: %+v", mc)
	}
	token := tokenFromLink(t, mc.inviteLink)

	// GET del formulario con token válido → 200; token manipulado → 400. Se altera
	// el PRIMER carácter (bits plenamente significativos de la parte aleatoria), que
	// cambia el mensaje del HMAC y por tanto invalida la firma de forma determinista.
	if rr := l.get(t, "/accept-invitation?token="+url.QueryEscape(token)); rr.Code != http.StatusOK {
		t.Fatalf("GET accept válido: status=%d", rr.Code)
	}
	tampered := "A" + token[1:]
	if token[0] == 'A' {
		tampered = "B" + token[1:]
	}
	if rr := l.get(t, "/accept-invitation?token="+url.QueryEscape(tampered)); rr.Code != http.StatusBadRequest {
		t.Fatalf("GET accept manipulado: status=%d, quiero 400", rr.Code)
	}

	// Aceptar → 200; cuenta creada con la contraseña elegida + membresía member.
	rr = l.postForm(t, "/accept-invitation", url.Values{
		"token": {token}, "new_password": {"welcome-secret-1"}, "confirm_password": {"welcome-secret-1"},
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("aceptar válido: status=%d cuerpo=%s", rr.Code, rr.Body)
	}
	u, err := l.st.GetUserByEmail(l.ctx, "newbie@acme.test")
	if err != nil {
		t.Fatalf("el invitado debería existir: %v", err)
	}
	if ok, _ := domain.VerifyPassword("welcome-secret-1", u.PasswordHash); !ok {
		t.Fatal("la contraseña elegida no se fijó")
	}
	if mem, err := l.st.GetMembership(l.ctx, org.ID, u.ID); err != nil || mem.Role != domain.RoleMember {
		t.Fatalf("membresía = %+v (%v), quiero member", mem, err)
	}

	// Reuso del MISMO token (ya aceptado) → 400 (un solo uso).
	rr = l.postForm(t, "/accept-invitation", url.Values{
		"token": {token}, "new_password": {"another-secret-9"}, "confirm_password": {"another-secret-9"},
	})
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("reuso del token: status=%d, quiero 400", rr.Code)
	}
}

func TestInvitation_ExpiredRejected(t *testing.T) {
	l := newLiveServer(t)
	org, _ := l.seedOrgAdmin(t, "acme")
	// Token FIRMADO válido (clave del servidor) pero fila caducada.
	token, hash, err := domain.NewResetToken(l.srv.resetKey)
	if err != nil {
		t.Fatal(err)
	}
	if err := l.st.InTx(l.ctx, func(q pg.Querier) error {
		return l.st.InsertInvitation(l.ctx, q, hash, org.ID, "late@acme.test", "member", "", "", "", time.Now().Add(-time.Minute))
	}); err != nil {
		t.Fatalf("InsertInvitation: %v", err)
	}
	rr := l.postForm(t, "/accept-invitation", url.Values{
		"token": {token}, "new_password": {"whatever-secret-1"}, "confirm_password": {"whatever-secret-1"},
	})
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("invitación caducada: status=%d, quiero 400", rr.Code)
	}
	if _, err := l.st.GetUserByEmail(l.ctx, "late@acme.test"); err == nil {
		t.Fatal("una invitación caducada no debe crear la cuenta")
	}
}

func TestInvitation_Authz(t *testing.T) {
	l := newLiveServer(t)
	org, admin := l.seedOrgAdmin(t, "acme")
	member := l.addMember(t, org, "member@acme.test", domain.RoleMember)

	// No-admin (rol member) → 403 (deny-by-default de requireRole).
	memberTok := l.token(t, member.ID, org.Slug, []string{"member"})
	rr, _ := l.do(t, http.MethodPost, "/api/v1/admin/invitations", memberTok,
		map[string]string{"email": "x@acme.test", "role": "member"})
	if rr.Code != http.StatusForbidden {
		t.Fatalf("no-admin invita: status=%d, quiero 403", rr.Code)
	}

	adminTok := l.token(t, admin.ID, org.Slug, []string{"admin"})
	// Un admin no puede invitar como owner → 403.
	rr, _ = l.do(t, http.MethodPost, "/api/v1/admin/invitations", adminTok,
		map[string]string{"email": "y@acme.test", "role": "owner"})
	if rr.Code != http.StatusForbidden {
		t.Fatalf("admin invita owner: status=%d, quiero 403", rr.Code)
	}
	// Email correcto pero ya es miembro → 409.
	rr, _ = l.do(t, http.MethodPost, "/api/v1/admin/invitations", adminTok,
		map[string]string{"email": "member@acme.test", "role": "member"})
	if rr.Code != http.StatusConflict {
		t.Fatalf("invitar a un miembro existente: status=%d, quiero 409", rr.Code)
	}
	// Email inválido → 400.
	rr, _ = l.do(t, http.MethodPost, "/api/v1/admin/invitations", adminTok,
		map[string]string{"email": "no-arroba", "role": "member"})
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("email inválido: status=%d, quiero 400", rr.Code)
	}
}

// --- B) Cambiar rol / quitar miembro ---

func TestMember_RoleChangeRevokesOnDemotion(t *testing.T) {
	l := newLiveServer(t)
	org, err := l.st.CreateOrg(l.ctx, nil, domain.Organization{Slug: "acme", Name: "Acme"})
	if err != nil {
		t.Fatal(err)
	}
	owner := l.addMember(t, org, "owner@acme.test", domain.RoleOwner)
	// Arranca admin (con owner presente, no es el último admin) para poder degradar.
	target := l.addMember(t, org, "target@acme.test", domain.RoleAdmin)
	l.liveSession(t, org, target.ID, "target-sess")
	ownerTok := l.token(t, owner.ID, org.Slug, []string{"owner"})
	path := "/api/v1/admin/users/" + target.ID + "/role"

	// Promoción member→admin no revoca (subir privilegio); probamos la degradación:
	rr, out := l.do(t, http.MethodPost, path, ownerTok, map[string]string{"role": "member"})
	if rr.Code != http.StatusOK {
		t.Fatalf("degradar: status=%d cuerpo=%s", rr.Code, rr.Body)
	}
	if n, _ := out["sessions_revoked"].(float64); n < 1 {
		t.Fatalf("una degradación debe revocar sesiones: %v", out["sessions_revoked"])
	}
	if mem, _ := l.st.GetMembership(l.ctx, org.ID, target.ID); mem.Role != domain.RoleMember {
		t.Fatalf("rol tras degradar = %q, quiero member", mem.Role)
	}
	if ss, _ := l.st.GetSessionByToken(l.ctx, domain.HashToken("target-sess")); ss.Valid(time.Now()) {
		t.Fatal("la sesión del degradado debería estar revocada")
	}

	// Rol inválido → 400.
	rr, _ = l.do(t, http.MethodPost, path, ownerTok, map[string]string{"role": "superuser"})
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("rol inválido: status=%d, quiero 400", rr.Code)
	}
}

func TestMember_RemoveRevokesSessions(t *testing.T) {
	l := newLiveServer(t)
	org, admin := l.seedOrgAdmin(t, "acme")
	member := l.addMember(t, org, "member@acme.test", domain.RoleMember)
	l.liveSession(t, org, member.ID, "m-sess")
	adminTok := l.token(t, admin.ID, org.Slug, []string{"admin"})

	rr, out := l.do(t, http.MethodDelete, "/api/v1/admin/users/"+member.ID+"/membership", adminTok, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("quitar miembro: status=%d cuerpo=%s", rr.Code, rr.Body)
	}
	if n, _ := out["sessions_revoked"].(float64); n < 1 {
		t.Fatalf("quitar debe revocar sesiones: %v", out["sessions_revoked"])
	}
	if _, err := l.st.GetMembership(l.ctx, org.ID, member.ID); err == nil {
		t.Fatal("la membresía debería haberse eliminado")
	}
	if ss, _ := l.st.GetSessionByToken(l.ctx, domain.HashToken("m-sess")); ss.Valid(time.Now()) {
		t.Fatal("la sesión del expulsado debería estar revocada")
	}
}

func TestMember_LastAdminGuard(t *testing.T) {
	l := newLiveServer(t)
	org, err := l.st.CreateOrg(l.ctx, nil, domain.Organization{Slug: "acme", Name: "Acme"})
	if err != nil {
		t.Fatal(err)
	}
	sole := l.addMember(t, org, "sole@acme.test", domain.RoleAdmin) // único administrador
	// Llamante con claim admin de la org pero SIN membresía (divergencia claim/BD):
	// el guard del "último admin" (recuento en vivo) es la última línea de defensa.
	callerTok := l.token(t, "00000000-0000-0000-0000-0000000000aa", org.Slug, []string{"admin"})

	// Quitar al único admin → 409, sin tocar la membresía.
	rr, _ := l.do(t, http.MethodDelete, "/api/v1/admin/users/"+sole.ID+"/membership", callerTok, nil)
	if rr.Code != http.StatusConflict {
		t.Fatalf("quitar último admin: status=%d, quiero 409", rr.Code)
	}
	// Degradar al único admin → 409.
	rr, _ = l.do(t, http.MethodPost, "/api/v1/admin/users/"+sole.ID+"/role", callerTok, map[string]string{"role": "member"})
	if rr.Code != http.StatusConflict {
		t.Fatalf("degradar último admin: status=%d, quiero 409", rr.Code)
	}
	if mem, err := l.st.GetMembership(l.ctx, org.ID, sole.ID); err != nil || mem.Role != domain.RoleAdmin {
		t.Fatalf("el único admin no debe cambiar: %+v (%v)", mem, err)
	}
}

func TestMember_CrossOrgIsolation(t *testing.T) {
	l := newLiveServer(t)
	orgA, adminA := l.seedOrgAdmin(t, "acme")
	orgB, _ := l.seedOrgAdmin(t, "globex")
	userB := l.addMember(t, orgB, "only-b@globex.test", domain.RoleMember)

	adminATok := l.token(t, adminA.ID, orgA.Slug, []string{"admin"})
	// Admin de A intenta actuar sobre un usuario SOLO de B → 404 (no miembro de A).
	rr, _ := l.do(t, http.MethodPost, "/api/v1/admin/users/"+userB.ID+"/role", adminATok, map[string]string{"role": "admin"})
	if rr.Code != http.StatusNotFound {
		t.Fatalf("cross-org cambiar rol: status=%d, quiero 404", rr.Code)
	}
	rr, _ = l.do(t, http.MethodDelete, "/api/v1/admin/users/"+userB.ID+"/membership", adminATok, nil)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("cross-org quitar: status=%d, quiero 404", rr.Code)
	}
	// La membresía en B sigue intacta.
	if _, err := l.st.GetMembership(l.ctx, orgB.ID, userB.ID); err != nil {
		t.Fatalf("la membresía en B no debe tocarse: %v", err)
	}
}

// --- C) Reset por admin (enlace por email) ---

func TestAdminTriggerReset(t *testing.T) {
	l := newLiveServer(t)
	mc := &captureMailer{}
	l.srv.mailer = mc
	org, admin := l.seedOrgAdmin(t, "acme")
	member := l.addMember(t, org, "member@acme.test", domain.RoleMember)
	adminTok := l.token(t, admin.ID, org.Slug, []string{"admin"})
	path := "/api/v1/admin/users/" + member.ID + "/reset-password"

	// No-admin → 403.
	memberTok := l.token(t, member.ID, org.Slug, []string{"member"})
	if rr, _ := l.do(t, http.MethodPost, path, memberTok, nil); rr.Code != http.StatusForbidden {
		t.Fatalf("no-admin dispara reset: status=%d, quiero 403", rr.Code)
	}
	// Admin dispara → 200 y enlace de reseteo emitido + fila del token persistida.
	rr, out := l.do(t, http.MethodPost, path, adminTok, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("admin dispara reset: status=%d cuerpo=%s", rr.Code, rr.Body)
	}
	if out["sent"] != true || mc.resetLink == "" {
		t.Fatalf("no se emitió el enlace de reseteo: out=%v link=%q", out, mc.resetLink)
	}
	var n int
	if err := l.st.DB().QueryRow(l.ctx,
		`SELECT count(*) FROM password_reset_tokens WHERE user_id=$1`, member.ID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("tokens de reseteo del usuario = %d, quiero 1", n)
	}
}

// El ciclo COMPLETO desde la consola: invitar, VERLO en la lista y revocarlo.
//
// El listado es lo que faltaba: sin él, revocar exigía recordar de memoria el
// correo exacto de alguien invitado hace semanas, así que la consola no podía
// ofrecer el botón. Se comprueba también que la revocación deja el enlace muerto
// —que es de lo que sirve revocar— y que repetirla no falla, porque la pantalla
// reintenta y una segunda pulsación no puede parecer un error.
func TestInvitation_ListarYRevocar(t *testing.T) {
	l := newLiveServer(t)
	mc := &captureMailer{}
	l.srv.mailer = mc
	org, admin := l.seedOrgAdmin(t, "acme")
	adminTok := l.token(t, admin.ID, org.Slug, []string{"admin"})

	rr, _ := l.do(t, http.MethodPost, "/api/v1/admin/invitations", adminTok,
		map[string]string{"email": "pendiente@acme.test", "role": "member"})
	if rr.Code != http.StatusCreated {
		t.Fatalf("invitar: status=%d cuerpo=%s", rr.Code, rr.Body)
	}
	token := tokenFromLink(t, mc.inviteLink)

	// Aparece en el listado, con su rol y su caducidad.
	rr, _ = l.do(t, http.MethodGet, "/api/v1/admin/invitations", adminTok, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("listar: status=%d cuerpo=%s", rr.Code, rr.Body)
	}
	var lista struct {
		Items []struct {
			Email     string `json:"email"`
			Role      string `json:"role"`
			ExpiresAt string `json:"expires_at"`
		} `json:"items"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &lista); err != nil {
		t.Fatalf("listar: cuerpo ilegible: %v", err)
	}
	if len(lista.Items) != 1 || lista.Items[0].Email != "pendiente@acme.test" {
		t.Fatalf("listado = %+v, quiero la invitación pendiente", lista.Items)
	}
	if lista.Items[0].Role != "member" || lista.Items[0].ExpiresAt == "" {
		t.Fatalf("la fila no trae rol y caducidad: %+v", lista.Items[0])
	}

	// Un no-admin no ve el listado: mismo deny-by-default que el resto.
	member := l.addMember(t, org, "member@acme.test", domain.RoleMember)
	memberTok := l.token(t, member.ID, org.Slug, []string{"member"})
	if rr, _ := l.do(t, http.MethodGet, "/api/v1/admin/invitations", memberTok, nil); rr.Code != http.StatusForbidden {
		t.Fatalf("no-admin lista: status=%d, quiero 403", rr.Code)
	}

	// Revocar → revoked:true y el enlace deja de valer.
	rr, _ = l.do(t, http.MethodPost, "/api/v1/admin/invitations:revoke", adminTok,
		map[string]string{"email": "pendiente@acme.test"})
	if rr.Code != http.StatusOK {
		t.Fatalf("revocar: status=%d cuerpo=%s", rr.Code, rr.Body)
	}
	if !bytes.Contains(rr.Body.Bytes(), []byte(`"revoked":true`)) {
		t.Fatalf("revocar: cuerpo=%s, quiero revoked:true", rr.Body)
	}
	if rr := l.get(t, "/accept-invitation?token="+url.QueryEscape(token)); rr.Code == http.StatusOK {
		t.Fatal("el enlace revocado sigue abriendo el formulario")
	}

	// Y la lista se queda vacía.
	rr, _ = l.do(t, http.MethodGet, "/api/v1/admin/invitations", adminTok, nil)
	lista.Items = nil
	_ = json.Unmarshal(rr.Body.Bytes(), &lista)
	if len(lista.Items) != 0 {
		t.Fatalf("tras revocar el listado sigue con %d", len(lista.Items))
	}

	// Repetir la revocación NO es un error: idempotente, revoked:false.
	rr, _ = l.do(t, http.MethodPost, "/api/v1/admin/invitations:revoke", adminTok,
		map[string]string{"email": "pendiente@acme.test"})
	if rr.Code != http.StatusOK || !bytes.Contains(rr.Body.Bytes(), []byte(`"revoked":false`)) {
		t.Fatalf("revocar dos veces: status=%d cuerpo=%s", rr.Code, rr.Body)
	}
}
