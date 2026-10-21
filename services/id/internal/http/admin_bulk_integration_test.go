// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

// Acciones en lote, contra PostgreSQL real.
//
// La forma de romper esto no es que no funcione: es que funcione DEMASIADO. Un
// lote es la vía obvia para saltarse lo que la acción individual no deja hacer
// —tocar a un owner siendo admin, autobloquearse, dejar la organización sin
// administradores—, y también para que un fallo legítimo de uno se lleve por
// delante a los otros veintinueve. Eso es lo que se prueba.
package http

import (
	"net/http"
	"testing"

	"git.sytue.eu/sytue/one/services/id/internal/domain"
)

// resultadosPorID indexa la respuesta del lote para poder preguntarle por uno.
func resultadosPorID(t *testing.T, body map[string]any) map[string]map[string]any {
	t.Helper()
	lista, _ := body["results"].([]any)
	out := make(map[string]map[string]any, len(lista))
	for _, it := range lista {
		m, _ := it.(map[string]any)
		id, _ := m["id"].(string)
		out[id] = m
	}
	return out
}

func TestAdminBulk_SuspenderRespetaLosGuardas(t *testing.T) {
	l := newLiveServer(t)
	org, admin := l.seedOrgAdmin(t, "acme")
	adminTok := l.token(t, admin.ID, org.Slug, []string{"admin"})

	uno := l.addMember(t, org, "uno@acme.test", domain.RoleMember)
	dos := l.addMember(t, org, "dos@acme.test", domain.RoleMember)
	jefe := l.addMember(t, org, "jefe@acme.test", domain.RoleOwner)
	l.liveSession(t, org, uno.ID, "s-uno")
	otra, _ := l.st.CreateOrg(l.ctx, nil, domain.Organization{Slug: "otra", Name: "Otra"})
	ajeno := l.addMember(t, otra, "ajeno@otra.test", domain.RoleMember)

	// Un lote con de todo: dos válidos, un owner (un admin no puede), quien pide
	// (nadie se suspende a sí mismo), alguien de otra organización, y un id
	// repetido para comprobar que no se cuenta dos veces.
	rr, body := l.do(t, http.MethodPost, "/api/v1/admin/users/bulk", adminTok, map[string]any{
		"action":   "suspend",
		"user_ids": []string{uno.ID, dos.ID, jefe.ID, admin.ID, ajeno.ID, uno.ID},
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("lote: código=%d body=%v", rr.Code, body)
	}
	res := resultadosPorID(t, body)
	if len(res) != 5 {
		t.Errorf("%d resultados: el id repetido se procesó dos veces", len(res))
	}
	// Los dos legítimos se aplican PESE a que otros tres fallan: si abortara el
	// lote entero, la función no serviría para nada real.
	for _, u := range []domain.User{uno, dos} {
		if res[u.ID]["ok"] != true {
			t.Errorf("%s no se suspendió: %v", u.Email, res[u.ID])
		}
	}
	if res[jefe.ID]["code"] != "forbidden" {
		t.Errorf("un admin pudo suspender a un owner por lote: %v", res[jefe.ID])
	}
	if res[admin.ID]["code"] != "self" {
		t.Errorf("quien pide se pudo suspender a sí mismo: %v", res[admin.ID])
	}
	if res[ajeno.ID]["code"] != "not_found" {
		t.Errorf("el lote cruzó a otra organización: %v", res[ajeno.ID])
	}
	if n, _ := body["applied"].(float64); int(n) != 2 {
		t.Errorf("applied=%v, quiero 2", body["applied"])
	}

	// El estado es real, no solo la respuesta.
	u, err := l.st.GetUserByID(l.ctx, uno.ID)
	if err != nil || u.Status != domain.StatusSuspended {
		t.Fatalf("uno@ quedó en %q (%v)", u.Status, err)
	}
	jefeU, _ := l.st.GetUserByID(l.ctx, jefe.ID)
	if jefeU.Status != domain.StatusActive {
		t.Errorf("el owner acabó %q pese al 403 del lote", jefeU.Status)
	}
	// Suspender cierra las sesiones abiertas, y el lote lo dice por usuario.
	if n, _ := res[uno.ID]["sessions_revoked"].(float64); int(n) != 1 {
		t.Errorf("sesiones cerradas de uno@ = %v, quiero 1", res[uno.ID]["sessions_revoked"])
	}
}

func TestAdminBulk_ElRolNoDejaLaOrgSinAdministradores(t *testing.T) {
	l := newLiveServer(t)
	org, admin := l.seedOrgAdmin(t, "acme")
	// Quien pide es owner: puede tocar a otros owners y asignar owner.
	jefe := l.addMember(t, org, "jefe@acme.test", domain.RoleOwner)
	jefeTok := l.token(t, jefe.ID, org.Slug, []string{"owner"})
	miembro := l.addMember(t, org, "miembro@acme.test", domain.RoleMember)

	// Degradar a todo el mundo de golpe, incluido quien lo pide. El lote no puede
	// ser la vía para quedarse fuera de tu propia organización.
	rr, body := l.do(t, http.MethodPost, "/api/v1/admin/users/bulk", jefeTok, map[string]any{
		"action":   "set_role",
		"role":     "member",
		"user_ids": []string{jefe.ID, admin.ID, miembro.ID},
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("lote de rol: código=%d body=%v", rr.Code, body)
	}
	res := resultadosPorID(t, body)
	// A sí mismo, NUNCA (adminMayManage) — ni siquiera para degradarse. Es el
	// guarda que de verdad se puede alcanzar desde fuera: los otros dos
	// (último owner, último admin) son inalcanzables por HTTP, porque quien pide
	// ya cuenta como uno más y por eso nunca es el último; siguen ahí como
	// defensa en profundidad de `aplicarRol`.
	if res[jefe.ID]["code"] != "self" {
		t.Errorf("el owner se degradó a sí mismo por lote: %v", res[jefe.ID])
	}
	// Al admin sí se le puede degradar: queda el owner que lo pide, así que la
	// organización no se queda sin administradores.
	if res[admin.ID]["ok"] != true {
		t.Errorf("admin@: %v", res[admin.ID])
	}
	if res[miembro.ID]["ok"] != true {
		t.Errorf("miembro@: %v", res[miembro.ID])
	}
	// Y lo importante: sigue habiendo quien administre.
	n, err := l.st.CountOrgAdmins(l.ctx, nil, org.ID, "")
	if err != nil || n < 1 {
		t.Fatalf("la organización quedó con %d administradores (%v)", n, err)
	}
	// Degradar cierra las sesiones de quien pierde el rol: el acceso ya emitido
	// no puede sobrevivir a la degradación.
	m, _ := l.st.GetMembership(l.ctx, org.ID, admin.ID)
	if m.Role != domain.RoleMember {
		t.Errorf("el admin acabó con rol %q", m.Role)
	}
}

func TestAdminBulk_LimitesDeLaPeticion(t *testing.T) {
	l := newLiveServer(t)
	org, admin := l.seedOrgAdmin(t, "acme")
	adminTok := l.token(t, admin.ID, org.Slug, []string{"admin"})

	// Sin nadie seleccionado: 400, no un 200 que dice «hecho» sin haber hecho nada.
	rr, _ := l.do(t, http.MethodPost, "/api/v1/admin/users/bulk", adminTok, map[string]any{
		"action": "suspend", "user_ids": []string{},
	})
	if rr.Code != http.StatusBadRequest {
		t.Errorf("lote vacío: código=%d, quiero 400", rr.Code)
	}
	// Acción inventada.
	rr, _ = l.do(t, http.MethodPost, "/api/v1/admin/users/bulk", adminTok, map[string]any{
		"action": "borrar_todo", "user_ids": []string{admin.ID},
	})
	if rr.Code != http.StatusBadRequest {
		t.Errorf("acción inválida: código=%d, quiero 400", rr.Code)
	}
	// Tope: 201 ids.
	muchos := make([]string, 201)
	for i := range muchos {
		muchos[i] = admin.ID
	}
	rr, _ = l.do(t, http.MethodPost, "/api/v1/admin/users/bulk", adminTok, map[string]any{
		"action": "suspend", "user_ids": muchos,
	})
	if rr.Code != http.StatusBadRequest {
		t.Errorf("lote de 201: código=%d, quiero 400", rr.Code)
	}
	// Un admin no puede repartir el rol owner ni de uno en uno ni en lote.
	otro := l.addMember(t, org, "otro@acme.test", domain.RoleMember)
	rr, _ = l.do(t, http.MethodPost, "/api/v1/admin/users/bulk", adminTok, map[string]any{
		"action": "set_role", "role": "owner", "user_ids": []string{otro.ID},
	})
	if rr.Code != http.StatusForbidden {
		t.Errorf("un admin repartió owner por lote: código=%d, quiero 403", rr.Code)
	}
}
