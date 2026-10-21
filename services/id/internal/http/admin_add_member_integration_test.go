// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

// Multi-pertenencia: añadir un usuario YA EXISTENTE a una segunda organización
// (POST /api/v1/orgs/{slug}/members) contra un PostgreSQL real. La persona que
// trabaja en dos organizaciones del mismo despliegue es el caso, y la verja es
// lo que se prueba aquí:
//
//   - El claim `roles` del token habla SOLO de la org activa, así que ser admin
//     en A no autoriza en B: la pertenencia se re-verifica por-org contra la BD
//     (anti-IDOR cross-tenant). Sin membresía owner/admin en la org de destino ⇒ 403.
//   - Un admin NO puede conceder owner: solo un owner promueve a owner
//     (anti auto-escalada). Un admin pidiendo `owner` ⇒ 403.
//
// Ambos 403 llevan mensaje propio y la Console lo propaga tal cual, así que el
// test comprueba también que el mensaje distingue un caso del otro: si los dos
// dijeran «prohibido», el admin no sabría si le falta el rol o la organización.
package http

import (
	"net/http"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/services/id/internal/domain"
)

// mensajeError saca el texto del sobre canónico de httpx, que va ANIDADO:
// {"error":{"code","message"},"request_id"}. Es el mismo desanidado que hace el
// BFF de la Console para propagar el motivo, así que si el sobre cambiara de
// forma este test y la Console se enterarían a la vez.
func mensajeError(out map[string]any) string {
	env, _ := out["error"].(map[string]any)
	msg, _ := env["message"].(string)
	return msg
}

// TestOrgMembers_AddExistingUserToSecondOrg — el camino feliz y sus rechazos de
// entrada. Marta ya existe en `acme`; el owner de `globex` la añade allí como
// member SIN crear una cuenta nueva: la misma identidad en dos organizaciones.
func TestOrgMembers_AddExistingUserToSecondOrg(t *testing.T) {
	l := newLiveServer(t)
	orgA, _ := l.seedOrgAdmin(t, "acme")
	orgB, _ := l.seedOrgAdmin(t, "globex")

	marta := l.addMember(t, orgA, "marta@acme.test", domain.RoleMember)
	// Owner de la org de DESTINO (seedOrgAdmin deja un admin; aquí hace falta owner
	// para poder comprobar después que sí puede conceder owner).
	owner := l.addMember(t, orgB, "owner@globex.test", domain.RoleOwner)
	tok := l.token(t, owner.ID, orgB.Slug, []string{"owner"})

	rr, out := l.do(t, http.MethodPost, "/api/v1/orgs/globex/members", tok,
		map[string]string{"email": marta.Email, "role": "member"})
	if rr.Code != http.StatusOK {
		t.Fatalf("añadir a la segunda org: status=%d cuerpo=%s", rr.Code, rr.Body)
	}
	if out["org"] != "globex" || out["user_id"] != marta.ID || out["role"] != "member" {
		t.Fatalf("respuesta inesperada: %v", out)
	}

	// La membresía es REAL y no reemplaza a la primera: sigue en las dos orgs.
	if mem, err := l.st.GetMembership(l.ctx, orgB.ID, marta.ID); err != nil || mem.Role != domain.RoleMember {
		t.Fatalf("membresía en globex: %+v (%v)", mem, err)
	}
	if _, err := l.st.GetMembership(l.ctx, orgA.ID, marta.ID); err != nil {
		t.Fatalf("la membresía original en acme debe seguir viva: %v", err)
	}

	// Rol fuera del catálogo ⇒ 400 (la Console ofrece un Select, pero el gate es este).
	rr, _ = l.do(t, http.MethodPost, "/api/v1/orgs/globex/members", tok,
		map[string]string{"email": marta.Email, "role": "superadmin"})
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("rol inválido: status=%d, quiero 400", rr.Code)
	}

	// Nadie con ese correo ⇒ 404: este endpoint AÑADE a quien ya existe, no da de
	// alta (para eso está la invitación), y la Console tiene que poder decirlo.
	rr, _ = l.do(t, http.MethodPost, "/api/v1/orgs/globex/members", tok,
		map[string]string{"email": "nadie@ninguna.test", "role": "member"})
	if rr.Code != http.StatusNotFound {
		t.Fatalf("correo desconocido: status=%d, quiero 404", rr.Code)
	}

	// Un owner SÍ puede conceder owner (contraparte del caso de abajo).
	rr, _ = l.do(t, http.MethodPost, "/api/v1/orgs/globex/members", tok,
		map[string]string{"email": marta.Email, "role": "owner"})
	if rr.Code != http.StatusOK {
		t.Fatalf("owner concediendo owner: status=%d cuerpo=%s", rr.Code, rr.Body)
	}
}

// TestOrgMembers_AddMemberAuthz — la verja, que es lo que no se puede relajar:
// ser admin en otra organización no autoriza aquí, y un admin no se fabrica un
// owner. Los dos rechazos son 403 CON mensaje distinto.
func TestOrgMembers_AddMemberAuthz(t *testing.T) {
	l := newLiveServer(t)
	orgA, adminA := l.seedOrgAdmin(t, "acme")
	orgB, adminB := l.seedOrgAdmin(t, "globex")
	marta := l.addMember(t, orgA, "marta@acme.test", domain.RoleMember)

	// 1) Admin de A, SIN membresía en B, con un token que dice `admin` (su rol en A):
	// el claim no basta, la pertenencia se mira en la BD org por org.
	ajeno := l.token(t, adminA.ID, orgA.Slug, []string{"admin"})
	rr, out := l.do(t, http.MethodPost, "/api/v1/orgs/globex/members", ajeno,
		map[string]string{"email": marta.Email, "role": "member"})
	if rr.Code != http.StatusForbidden {
		t.Fatalf("admin de otra org: status=%d cuerpo=%s, quiero 403", rr.Code, rr.Body)
	}
	// Lo que importa es que los DOS rechazos se distingan, no cómo estén
	// redactados: este test se rompió el día que el mensaje pasó al inglés, y
	// atarlo otra vez a una frase concreta lo volvería a romper con la siguiente
	// traducción. La comparación va al final, cuando ya se tienen los dos.
	msgCrossOrg := mensajeError(out)
	if msgCrossOrg == "" {
		t.Fatal("el 403 cross-org debe explicar por qué, no venir con el cuerpo vacío")
	}
	// Y no ha escrito nada.
	if _, err := l.st.GetMembership(l.ctx, orgB.ID, marta.ID); err == nil {
		t.Fatal("un rechazo no puede dejar la membresía creada")
	}

	// 2) Admin DE B pidiendo el rol owner: autorizado en la org, pero no para eso.
	tokB := l.token(t, adminB.ID, orgB.Slug, []string{"admin"})
	rr, out = l.do(t, http.MethodPost, "/api/v1/orgs/globex/members", tokB,
		map[string]string{"email": marta.Email, "role": "owner"})
	if rr.Code != http.StatusForbidden {
		t.Fatalf("admin concediendo owner: status=%d cuerpo=%s, quiero 403", rr.Code, rr.Body)
	}
	// «owner» sí se puede exigir: es el identificador del rol, no prosa, y no
	// cambia al traducir.
	msgOwner := mensajeError(out)
	if !strings.Contains(msgOwner, "owner") {
		t.Fatalf("el 403 de auto-escalada debe nombrar el rol owner: %q", msgOwner)
	}
	if msgCrossOrg == msgOwner {
		t.Fatalf("los dos 403 tienen que distinguirse; ambos dicen %q", msgOwner)
	}
	if _, err := l.st.GetMembership(l.ctx, orgB.ID, marta.ID); err == nil {
		t.Fatal("el rechazo de auto-escalada no puede dejar la membresía creada")
	}

	// 3) El mismo admin de B con un rol que sí puede conceder: 200. Confirma que
	// lo que bloquea el caso 2 es el ROL pedido, no la org ni el solicitante.
	rr, _ = l.do(t, http.MethodPost, "/api/v1/orgs/globex/members", tokB,
		map[string]string{"email": marta.Email, "role": "admin"})
	if rr.Code != http.StatusOK {
		t.Fatalf("admin concediendo admin: status=%d cuerpo=%s", rr.Code, rr.Body)
	}
}
