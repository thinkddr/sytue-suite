// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

// Tests de integración de la SUSPENSIÓN DE ORGANIZACIÓN contra PostgreSQL real: una org
// suspendida no admite inicios de sesión, y volver a activarla los restaura.
//
// Hallazgo de la caza del patrón «autorización apoyada en la fila equivocada»:
// `organizations.status` se escribía (UpdateOrgSettings) y NO se leía en NINGUNA verja de
// autorización —solo al serializar respuestas—, así que el botón de cortar un tenant
// (impago, incidente, cuenta comprometida) era puramente cosmético: sus miembros seguían
// entrando y recibiendo tokens con el claim `tenant` de esa org, del que toda la suite
// deriva su aislamiento. Y no hay otra palanca: no existe borrado de organización.
//
// La verja vive en createSession, el mismo punto anti-bypass donde se aplica el Acceso
// Condicional (por eso este fichero reusa el harness de access_integration_test.go).
package http

import (
	"net/http"
	"testing"

	"git.sytue.eu/sytue/one/services/id/internal/domain"
)

// TestOrgSuspendedBlocksLoginAndReactivationRestores: suspender corta el login; reactivar
// lo devuelve. Lo segundo importa tanto como lo primero: la verja se puso en createSession
// y NO en las lecturas del store precisamente para no autobloquear la reactivación.
func TestOrgSuspendedBlocksLoginAndReactivationRestores(t *testing.T) {
	l := newLiveServer(t)
	org, err := l.st.CreateOrg(l.ctx, nil, domain.Organization{Slug: "susp", Name: "Suspendible"})
	if err != nil {
		t.Fatalf("CreateOrg: %v", err)
	}
	caOrgUser(t, l, org, "ada@susp.test")

	// Con la org ACTIVA el login funciona (línea base del test).
	if code, sess := caLogin(t, l, "ada@susp.test"); !sess || code != http.StatusFound {
		t.Fatalf("org activa: code=%d session=%v, quiero 302 con sesión", code, sess)
	}

	// SUSPENDER la organización.
	if _, err := l.st.UpdateOrgSettings(l.ctx, nil, org.ID, org.Name, domain.StatusSuspended); err != nil {
		t.Fatalf("UpdateOrgSettings(suspended): %v", err)
	}
	if code, sess := caLogin(t, l, "ada@susp.test"); sess || code == http.StatusFound {
		t.Fatalf("org SUSPENDIDA debe cortar el login: code=%d session=%v", code, sess)
	}

	// La org suspendida sigue siendo LEGIBLE por id: es lo que permite reactivarla.
	got, err := l.st.GetOrgByID(l.ctx, org.ID)
	if err != nil {
		t.Fatalf("una org suspendida debe seguir siendo legible para poder reactivarla: %v", err)
	}
	if got.Status != domain.StatusSuspended {
		t.Fatalf("estado inesperado: %q", got.Status)
	}

	// REACTIVAR ⇒ el login vuelve. Si alguien «arreglara» esto filtrando por estado en
	// GetOrgByID/GetOrgBySlug, este test lo cazaría: sería un autobloqueo permanente.
	if _, err := l.st.UpdateOrgSettings(l.ctx, nil, org.ID, org.Name, domain.StatusActive); err != nil {
		t.Fatalf("UpdateOrgSettings(active): %v", err)
	}
	if code, sess := caLogin(t, l, "ada@susp.test"); !sess || code != http.StatusFound {
		t.Fatalf("tras reactivar, el login debe volver: code=%d session=%v", code, sess)
	}
}
