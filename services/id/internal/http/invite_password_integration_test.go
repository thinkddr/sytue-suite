// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

// Alta directa con contraseña, contra PostgreSQL real.
//
// Es la salida cuando el correo no llega: sin ella, un alta depende de un enlace
// que puede no salir nunca y deja a alguien esperando mientras quien la hizo
// cree que ya está dentro. Lo que se prueba es que sigue siendo un alta DE
// VERDAD —cuenta utilizable, membresía y buzón— y que no se salta ni la política
// de contraseñas ni el techo de quién puede repartir el rol owner: una vía
// alternativa que relaja los guardas no es una salida, es un agujero.
package http

import (
	"net/http"
	"testing"

	"git.sytue.eu/sytue/one/services/id/internal/domain"
)

func TestAltaDirectaConPassword(t *testing.T) {
	l := newLiveServer(t)
	org, admin := l.seedOrgAdmin(t, "acme")
	adminTok := l.token(t, admin.ID, org.Slug, []string{"admin"})

	// 1. Alta con contraseña: crea la cuenta y NO dice que haya invitado.
	rr, body := l.do(t, http.MethodPost, "/api/v1/admin/invitations", adminTok, map[string]any{
		"email": "directa@acme.test", "role": "member", "password": "Contraseña-larga-2026",
	})
	if rr.Code != http.StatusCreated {
		t.Fatalf("alta directa: código=%d body=%v", rr.Code, body)
	}
	if body["invited"] != false {
		t.Errorf("invited=%v: la consola diría «te hemos enviado un correo» sin haberlo enviado", body["invited"])
	}

	// 2. La cuenta EXISTE, es miembro y puede entrar con esa contraseña.
	u, err := l.st.GetUserByEmail(l.ctx, "directa@acme.test")
	if err != nil {
		t.Fatalf("el usuario no se creó: %v", err)
	}
	if u.Status != domain.StatusActive {
		t.Errorf("estado %q: la cuenta nace inutilizable", u.Status)
	}
	if u.EmailVerified {
		t.Error("email_verified=true sin haber demostrado control del buzón: un admin de org podría fabricarse correo verificado global (como en SCIM #679)")
	}
	if l.srv.soloCuentasPersonales(l.ctx, u.ID) {
		t.Error("soloCuentasPersonales=true siendo miembro de una organización: el login le pediría verificar el correo")
	}
	m, err := l.st.GetMembership(l.ctx, org.ID, u.ID)
	if err != nil || m.Role != domain.RoleMember {
		t.Fatalf("membresía = %q (%v)", m.Role, err)
	}
	completo, err := l.st.GetUserByID(l.ctx, u.ID)
	if err != nil {
		t.Fatalf("GetUserByID: %v", err)
	}
	ok, verr := domain.VerifyPassword("Contraseña-larga-2026", completo.PasswordHash)
	if verr != nil || !ok {
		t.Errorf("la contraseña fijada no sirve para entrar (%v): el alta no vale para nada", verr)
	}

	// 3. NO se salta la política de contraseñas.
	rr, _ = l.do(t, http.MethodPost, "/api/v1/admin/invitations", adminTok, map[string]any{
		"email": "debil@acme.test", "role": "member", "password": "123",
	})
	if rr.Code != http.StatusBadRequest {
		t.Errorf("contraseña débil: código=%d, quiero 400", rr.Code)
	}

	// 4. Ni el techo del rol owner: un admin no lo reparte por esta vía tampoco.
	rr, _ = l.do(t, http.MethodPost, "/api/v1/admin/invitations", adminTok, map[string]any{
		"email": "jefe@acme.test", "role": "owner", "password": "Contraseña-larga-2026",
	})
	if rr.Code != http.StatusForbidden {
		t.Errorf("un admin repartió owner por el alta directa: código=%d, quiero 403", rr.Code)
	}

	// 5. Y sin contraseña sigue siendo una invitación normal (no se rompió el
	//    camino recomendado, que es el que no hace viajar ningún secreto).
	rr, body = l.do(t, http.MethodPost, "/api/v1/admin/invitations", adminTok, map[string]any{
		"email": "normal@acme.test", "role": "member",
	})
	if rr.Code != http.StatusCreated && rr.Code != http.StatusServiceUnavailable {
		t.Errorf("invitación normal: código=%d body=%v", rr.Code, body)
	}

	// 6. Una cuenta global que YA existe no se puede pisar con alta directa con contraseña:
	//    hacerlo permitiría a cualquier admin de organización cambiarle la contraseña a
	//    una cuenta existente sin demostrar control del correo.
	adminAntes, err := l.st.GetUserByID(l.ctx, admin.ID)
	if err != nil {
		t.Fatalf("GetUserByID admin: %v", err)
	}
	rr, _ = l.do(t, http.MethodPost, "/api/v1/admin/invitations", adminTok, map[string]any{
		"email": admin.Email, "role": "member", "password": "Otra-contraseña-2026",
	})
	if rr.Code != http.StatusConflict {
		t.Errorf("alta con contraseña sobre cuenta existente: código=%d, quiero 409", rr.Code)
	}
	adminDespues, err := l.st.GetUserByID(l.ctx, admin.ID)
	if err != nil {
		t.Fatalf("GetUserByID admin después: %v", err)
	}
	if adminDespues.PasswordHash != adminAntes.PasswordHash {
		t.Error("el alta directa con contraseña sobrescribió el hash de una cuenta ya existente")
	}

	// 7. Cambiar el correo de una cuenta verificada deja email_verified=false
	//    hasta que confirme el nuevo buzón.
	adminCambiado, err := l.st.UpdateUser(l.ctx, nil, domain.User{
		ID: admin.ID, Email: "nuevo-admin@acme.test", Name: admin.Name, Status: domain.StatusActive,
	})
	if err != nil {
		t.Fatalf("UpdateUser admin: %v", err)
	}
	if adminCambiado.EmailVerified {
		t.Error("cambiar el correo dejó email_verified=true con un buzón nuevo sin verificar")
	}
}
