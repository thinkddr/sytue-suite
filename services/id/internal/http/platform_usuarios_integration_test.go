// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

// Propietarios y cuentas desde la consola de PLATAFORMA, contra PostgreSQL real.
//
// Lo que se vigila no es el código de respuesta sino lo que queda en la base:
// un 409 después de haber suspendido ya a la única propietaria pasaría una
// comprobación de status y habría dejado la organización sin nadie.
package http

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/services/id/internal/domain"
	"git.sytue.eu/sytue/one/services/id/internal/oidc"
	"git.sytue.eu/sytue/one/services/id/internal/store"
)

// operadorElevado deja a un admin global con su elevación viva y devuelve su id.
func operadorElevado(t *testing.T, l *liveServer, email string) string {
	t.Helper()
	op := pimActor(t, l, email)
	if rec := l.pimReq(t, http.MethodPost, "/api/v1/platform/activate", op, []string{"pwd", "mfa"},
		`{"justification":"soporte"}`); rec.Code != http.StatusOK {
		t.Fatalf("elevar: código=%d body=%s", rec.Code, rec.Body)
	}
	return op
}

// errorCode lee el `code` del sobre de error de httpx.
func errorCode(t *testing.T, body []byte) (code, message string) {
	t.Helper()
	var env struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	_ = json.Unmarshal(body, &env)
	return env.Error.Code, env.Error.Message
}

func (l *liveServer) orgCliente(t *testing.T, slug string) domain.Organization {
	t.Helper()
	org, err := l.st.CreateOrg(l.ctx, nil, domain.Organization{Slug: slug, Name: strings.ToUpper(slug)})
	if err != nil {
		t.Fatalf("CreateOrg(%s): %v", slug, err)
	}
	return org
}

func (l *liveServer) rol(t *testing.T, orgID, userID string) domain.Role {
	t.Helper()
	m, err := l.st.GetMembership(l.ctx, orgID, userID)
	if err != nil {
		t.Fatalf("GetMembership: %v", err)
	}
	return m.Role
}

func TestPlatformPropietario_AsignarNoDegradaYCrearInvita(t *testing.T) {
	l := newLiveServer(t)
	mc := &captureMailer{}
	l.srv.mailer = mc
	op := operadorElevado(t, l, "global@id.test")
	conMFA := []string{"pwd", "mfa"}
	org := l.orgCliente(t, "cliente")
	ana := l.addMember(t, org, "ana@cliente.test", domain.RoleOwner)
	carla := l.addMember(t, org, "carla@cliente.test", domain.RoleMember)
	berta := l.mkUser(t, "berta@otra.test", "berta-pass-123")
	base := "/api/v1/platform/orgs/" + org.ID

	// Sin motivo no se escribe nada.
	if rec := l.pimReq(t, http.MethodPost, base+"/owners", op, conMFA,
		`{"email":"berta@otra.test","reason":""}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("sin motivo: código=%d, quiero 400", rec.Code)
	}

	// Alguien de fuera: entra como propietaria. Y Ana SIGUE siéndolo: una
	// organización puede tener varias y asignar no degrada a nadie.
	if rec := l.pimReq(t, http.MethodPost, base+"/owners", op, conMFA,
		`{"email":"Berta@otra.test","reason":"el propietario se fue"}`); rec.Code != http.StatusOK {
		t.Fatalf("asignar a una externa: código=%d body=%s", rec.Code, rec.Body)
	}
	if r := l.rol(t, org.ID, berta.ID); r != domain.RoleOwner {
		t.Fatalf("Berta quedó %q; quiero owner", r)
	}
	if r := l.rol(t, org.ID, ana.ID); r != domain.RoleOwner {
		t.Fatalf("Ana quedó %q: asignar otra propietaria la degradó", r)
	}

	// Una miembro sube de rol.
	if rec := l.pimReq(t, http.MethodPost, base+"/owners", op, conMFA,
		`{"email":"carla@cliente.test","reason":"lo pide la dirección"}`); rec.Code != http.StatusOK {
		t.Fatalf("asignar a una miembro: código=%d body=%s", rec.Code, rec.Body)
	}
	if r := l.rol(t, org.ID, carla.ID); r != domain.RoleOwner {
		t.Fatalf("Carla quedó %q; quiero owner", r)
	}

	// Un correo sin cuenta no se asigna: se invita.
	rec := l.pimReq(t, http.MethodPost, base+"/owners", op, conMFA,
		`{"email":"nadie@cliente.test","reason":"x"}`)
	if code, _ := errorCode(t, rec.Body.Bytes()); rec.Code != http.StatusBadRequest || code != "no_such_user" {
		t.Fatalf("asignar sin cuenta: código=%d code=%q", rec.Code, code)
	}

	// Crear propietaria: la invitación de siempre, con rol owner y su nombre.
	rec = l.pimReq(t, http.MethodPost, base+"/owner-invitations", op, conMFA,
		`{"email":"dani@nuevo.test","name":"Dani Pérez","reason":"alta por contrato"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("invitar propietaria: código=%d body=%s", rec.Code, rec.Body)
	}
	if mc.inviteTo != "dani@nuevo.test" {
		t.Fatalf("la invitación fue a %q", mc.inviteTo)
	}
	clave := "dani-elige-la-suya-1"
	if rr := l.postForm(t, "/accept-invitation", url.Values{
		"token": {tokenFromLink(t, mc.inviteLink)}, "new_password": {clave}, "confirm_password": {clave},
	}); rr.Code != http.StatusOK {
		t.Fatalf("aceptar: código=%d body=%s", rr.Code, rr.Body)
	}
	dani, err := l.st.GetUserByEmail(l.ctx, "dani@nuevo.test")
	if err != nil {
		t.Fatalf("la cuenta nueva no existe: %v", err)
	}
	if dani.Name != "Dani Pérez" {
		t.Errorf("nombre de la cuenta = %q; quiero el de la invitación", dani.Name)
	}
	if r := l.rol(t, org.ID, dani.ID); r != domain.RoleOwner {
		t.Fatalf("Dani quedó %q; quiero owner", r)
	}

	// Con cuenta ya, «crear» no: se asigna.
	rec = l.pimReq(t, http.MethodPost, base+"/owner-invitations", op, conMFA,
		`{"email":"berta@otra.test","name":"Berta","reason":"x"}`)
	if code, _ := errorCode(t, rec.Body.Bytes()); rec.Code != http.StatusConflict || code != "account_exists" {
		t.Fatalf("crear con cuenta existente: código=%d code=%q", rec.Code, code)
	}

	// Una cuenta personal es de una persona: no admite otra propietaria.
	personal, err := l.st.CreateOrg(l.ctx, nil, domain.Organization{
		Slug: domain.PersonalSlug("eva@personal.test"), Name: "eva", Kind: domain.OrgKindPersonal,
	})
	if err != nil {
		t.Fatalf("CreateOrg personal: %v", err)
	}
	if rec := l.pimReq(t, http.MethodPost, "/api/v1/platform/orgs/"+personal.ID+"/owners", op, conMFA,
		`{"email":"berta@otra.test","reason":"x"}`); rec.Code != http.StatusConflict {
		t.Fatalf("propietaria en cuenta personal: código=%d, quiero 409", rec.Code)
	}
	if _, err := l.st.GetMembership(l.ctx, personal.ID, berta.ID); err == nil {
		t.Fatal("Berta entró en una cuenta personal ajena")
	}

	// Todo queda en el registro de plataforma, con la organización.
	page, _, err := l.st.ListPlatformAuditPage(l.ctx, store.AuditFilter{Action: "platform.org.owner_"}, nil, 50)
	if err != nil {
		t.Fatalf("ListPlatformAuditPage: %v", err)
	}
	if len(page) != 3 {
		t.Fatalf("%d filas de propietario en el registro; quiero 3 (dos asignadas, una invitada): %+v", len(page), page)
	}
	for _, a := range page {
		if a.OrgSlug != "cliente" || a.ActorID != op {
			t.Errorf("fila %s: org=%q actor=%q; quiero cliente y el operador", a.Action, a.OrgSlug, a.ActorID)
		}
	}
}

func TestPlatformCuentas_LaUnicaPropietariaNoSeBloqueaNiSeElimina(t *testing.T) {
	l := newLiveServer(t)
	op := operadorElevado(t, l, "global@id.test")
	conMFA := []string{"pwd", "mfa"}
	org := l.orgCliente(t, "cliente")
	sola := l.addMember(t, org, "sola@cliente.test", domain.RoleOwner)
	l.liveSession(t, org, sola.ID, "sesion-de-sola")
	ruta := "/api/v1/platform/orgs/" + org.ID + "/members/" + sola.ID

	for _, c := range []struct{ accion, cuerpo string }{
		{"block", `{"reason":"cuenta comprometida"}`},
		{"delete", `{"confirm_email":"sola@cliente.test","reason":"se fue"}`},
	} {
		rec := l.pimReq(t, http.MethodPost, ruta+"/"+c.accion, op, conMFA, c.cuerpo)
		code, msg := errorCode(t, rec.Body.Bytes())
		if rec.Code != http.StatusConflict || code != "sole_owner" || !strings.Contains(msg, "CLIENTE") {
			t.Fatalf("%s de la única propietaria: código=%d code=%q msg=%q; quiero 409 sole_owner nombrando la organización",
				c.accion, rec.Code, code, msg)
		}
		if st, _ := l.estado(t, sola.ID); st != string(domain.StatusActive) {
			t.Fatalf("%s: la única propietaria quedó %q — la organización se queda sin nadie", c.accion, st)
		}
		if !l.viva(t, "sesion-de-sola") {
			t.Fatalf("%s: se le cerró la sesión a la única propietaria aunque la acción se rechazó", c.accion)
		}
	}

	// Con otra propietaria activa, sí. Y bloquear corta el acceso ya emitido.
	l.addMember(t, org, "relevo@cliente.test", domain.RoleOwner)
	rec := l.pimReq(t, http.MethodPost, ruta+"/block", op, conMFA, `{"reason":"cuenta comprometida"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("bloquear con relevo: código=%d body=%s", rec.Code, rec.Body)
	}
	if st, _ := l.estado(t, sola.ID); st != string(domain.StatusSuspended) {
		t.Fatalf("bloqueada quedó %q; quiero suspended", st)
	}
	if l.viva(t, "sesion-de-sola") {
		t.Fatal("bloquear no cerró la sesión")
	}

	// Y se deshace.
	if rec := l.pimReq(t, http.MethodPost, ruta+"/unblock", op, conMFA, `{"reason":"falsa alarma"}`); rec.Code != http.StatusOK {
		t.Fatalf("desbloquear: código=%d body=%s", rec.Code, rec.Body)
	}
	if st, _ := l.estado(t, sola.ID); st != string(domain.StatusActive) {
		t.Fatalf("desbloqueada quedó %q", st)
	}

	// Eliminar pide repetir el correo; con él, es la baja recuperable de siempre.
	rec = l.pimReq(t, http.MethodPost, ruta+"/delete", op, conMFA, `{"confirm_email":"otra@cliente.test","reason":"se fue"}`)
	if code, _ := errorCode(t, rec.Body.Bytes()); rec.Code != http.StatusBadRequest || code != "confirm_mismatch" {
		t.Fatalf("eliminar sin confirmar: código=%d code=%q", rec.Code, code)
	}
	if st, _ := l.estado(t, sola.ID); st != string(domain.StatusActive) {
		t.Fatalf("una confirmación equivocada eliminó la cuenta: %q", st)
	}
	rec = l.pimReq(t, http.MethodPost, ruta+"/delete", op, conMFA, `{"confirm_email":"SOLA@cliente.test","reason":"se fue"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("eliminar: código=%d body=%s", rec.Code, rec.Body)
	}
	if st, propia := l.estado(t, sola.ID); st != string(domain.StatusDeleted) || propia {
		t.Fatalf("eliminada quedó %q (propia=%v); quiero deleted y dada de baja por el operador", st, propia)
	}
	// La organización la puede restaurar en su ventana: es la misma baja.
	cerradas, err := l.st.ClosedUsersInOrg(l.ctx, org.ID, time.Now().Add(-retencionBaja))
	if err != nil || len(cerradas) != 1 || cerradas[0].ID != sola.ID {
		t.Fatalf("la baja no aparece entre las restaurables de la organización: %+v (%v)", cerradas, err)
	}

	page, _, err := l.st.ListPlatformAuditPage(l.ctx, store.AuditFilter{Action: "platform.user."}, nil, 50)
	if err != nil {
		t.Fatalf("ListPlatformAuditPage: %v", err)
	}
	if len(page) != 3 {
		t.Fatalf("%d filas de cuentas en el registro; quiero 3 (bloquear, desbloquear, eliminar): %+v", len(page), page)
	}
	for _, a := range page {
		if a.OrgSlug != "cliente" || a.ActorID != op || !strings.Contains(a.Target, "sola@cliente.test") {
			t.Errorf("fila %s: org=%q actor=%q target=%q; quiero quién, a quién y dónde", a.Action, a.OrgSlug, a.ActorID, a.Target)
		}
	}
}

func TestPlatformCuentas_ContrasenaTemporalObligaACambiarla(t *testing.T) {
	l := newLiveServer(t)
	mc := &captureMailer{}
	l.srv.mailer = mc
	op := operadorElevado(t, l, "global@id.test")
	conMFA := []string{"pwd", "mfa"}
	org := l.orgCliente(t, "cliente")
	u := l.addMember(t, org, "luis@cliente.test", domain.RoleMember)
	l.liveSession(t, org, u.ID, "sesion-de-luis")
	ruta := "/api/v1/platform/orgs/" + org.ID + "/members/" + u.ID + "/password"

	rec := l.pimReq(t, http.MethodPost, ruta, op, conMFA, `{"mode":"temporary","reason":"no le llega el correo"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("temporal: código=%d body=%s", rec.Code, rec.Body)
	}
	var out struct {
		TemporaryPassword string `json:"temporary_password"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	if len(out.TemporaryPassword) < 16 {
		t.Fatalf("contraseña temporal %q demasiado corta", out.TemporaryPassword)
	}
	if l.viva(t, "sesion-de-luis") {
		t.Fatal("resetear no cerró las sesiones")
	}
	page, _, err := l.st.ListPlatformAuditPage(l.ctx, store.AuditFilter{Action: "platform.user.password_temporary"}, nil, 5)
	if err != nil || len(page) != 1 {
		t.Fatalf("registro: %+v (%v)", page, err)
	}
	if strings.Contains(page[0].Target, out.TemporaryPassword) || strings.Contains(string(page[0].Detail), out.TemporaryPassword) {
		t.Fatal("la contraseña temporal está en la auditoría")
	}

	// Con la temporal NO se abre sesión: se va al formulario de cambiarla.
	rr := l.postFormRR(t, "/login/", url.Values{"email": {"luis@cliente.test"}, "password": {out.TemporaryPassword}}, nil)
	if rr.Code != http.StatusSeeOther || !strings.Contains(rr.Header().Get("Location"), "/reset-password?token=") {
		t.Fatalf("login con la temporal: código=%d Location=%q; quiero el formulario de cambio", rr.Code, rr.Header().Get("Location"))
	}
	if findCookie(rr, sessionCookieName) != nil {
		t.Fatal("la contraseña temporal abrió sesión")
	}
	nueva := "luis-ya-tiene-la-suya-7"
	if rr := l.postForm(t, "/reset-password", url.Values{
		"token": {tokenFromLink(t, rr.Header().Get("Location"))}, "new_password": {nueva}, "confirm_password": {nueva},
	}); rr.Code != http.StatusOK {
		t.Fatalf("cambiarla: código=%d body=%s", rr.Code, rr.Body)
	}
	if got, _ := l.st.GetUserByID(l.ctx, u.ID); got.PasswordMustChange {
		t.Fatal("tras elegir la suya sigue marcada como temporal")
	}
	if rr := l.postFormRR(t, "/login/", url.Values{"email": {"luis@cliente.test"}, "password": {nueva}}, nil); findCookie(rr, sessionCookieName) == nil {
		t.Fatalf("con la suya no entra: código=%d Location=%q", rr.Code, rr.Header().Get("Location"))
	}

	// El enlace por correo: la vía de siempre, y también cierra sesiones.
	l.liveSession(t, org, u.ID, "otra-sesion-de-luis")
	if rec := l.pimReq(t, http.MethodPost, ruta, op, conMFA, `{"mode":"link","reason":"lo pide"}`); rec.Code != http.StatusOK {
		t.Fatalf("enlace: código=%d body=%s", rec.Code, rec.Body)
	}
	if mc.resetLink == "" {
		t.Fatal("no salió el enlace de restablecimiento")
	}
	if l.viva(t, "otra-sesion-de-luis") {
		t.Fatal("el enlace no cerró las sesiones")
	}
}

func TestPlatformCuentas_VerjasDeQuienSeToca(t *testing.T) {
	l := newLiveServer(t)
	op := operadorElevado(t, l, "global@id.test")
	conMFA := []string{"pwd", "mfa"}
	org := l.orgCliente(t, "cliente")
	l.addMember(t, org, "duena@cliente.test", domain.RoleOwner)
	u := l.addMember(t, org, "pepe@cliente.test", domain.RoleMember)
	base := "/api/v1/platform/orgs/" + org.ID + "/members/"

	// Otra administradora global, aunque sea miembro: su alcance se retira por su
	// camino, con su aviso, no bloqueando la cuenta.
	otra := pimActor(t, l, "otra-global@id.test")
	if err := l.st.UpsertMembership(l.ctx, nil, domain.Membership{OrgID: org.ID, UserID: otra, Role: domain.RoleMember}); err != nil {
		t.Fatalf("UpsertMembership: %v", err)
	}
	rec := l.pimReq(t, http.MethodPost, base+otra+"/block", op, conMFA, `{"reason":"x"}`)
	if code, _ := errorCode(t, rec.Body.Bytes()); rec.Code != http.StatusConflict || code != "platform_admin" {
		t.Fatalf("bloquear a otra admin global: código=%d code=%q", rec.Code, code)
	}

	// Alguien que no es de ESTA organización: 404, aunque exista.
	fuera := l.mkUser(t, "fuera@otra.test", "")
	if rec := l.pimReq(t, http.MethodPost, base+fuera.ID+"/block", op, conMFA, `{"reason":"x"}`); rec.Code != http.StatusNotFound {
		t.Fatalf("bloquear a alguien de fuera: código=%d, quiero 404", rec.Code)
	}

	// Sin identificación reciente, nada: es step-up, no solo la elevación.
	viejo, err := l.iss.IssueAccessToken(oidc.TokenClaims{
		Subject: op, Audience: "sytue-console", Scope: "openid", AMR: conMFA,
		AuthTime: time.Now().Add(-time.Hour).UTC(),
	}, time.Now().UTC())
	if err != nil {
		t.Fatalf("IssueAccessToken: %v", err)
	}
	rr, _ := l.do(t, http.MethodPost, base+u.ID+"/block", viejo, map[string]string{"reason": "x"})
	if code, _ := errorCode(t, rr.Body.Bytes()); rr.Code != http.StatusUnauthorized || code != "insufficient_user_authentication" {
		t.Fatalf("sin step-up: código=%d code=%q", rr.Code, code)
	}
	if st, _ := l.estado(t, u.ID); st != string(domain.StatusActive) {
		t.Fatalf("sin step-up la cuenta quedó %q", st)
	}

	// La lista de miembros es lectura y no lo pide.
	rr, out := l.do(t, http.MethodGet, "/api/v1/platform/orgs/"+org.ID+"/members", viejo, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("listar miembros: código=%d body=%s", rr.Code, rr.Body)
	}
	if items, _ := out["items"].([]any); len(items) != 3 {
		t.Fatalf("miembros = %d; quiero 3", len(items))
	}
}
