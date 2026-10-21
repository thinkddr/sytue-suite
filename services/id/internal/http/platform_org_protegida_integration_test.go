// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

// La organización que OPERA la plataforma no se suspende ni se elimina, contra
// PostgreSQL real.
//
// No es una preferencia de producto: sus miembros son los administradores
// globales, y `createSession` deniega el login a quien pertenece a una
// organización que no está activa. Suspenderla deja fuera a TODO el mundo,
// incluidos los que tendrían que reactivarla, y no hay una consola por encima de
// esta desde la que arreglarlo. Un clic, y la plataforma se queda sin nadie que
// pueda entrar.
//
// Se prueba contra la base porque lo que importa no es el 403: es que el estado
// NO cambie. Un handler que responde 403 después de haber escrito ya el estado
// pasaría una comprobación de código de respuesta y habría dejado la plataforma
// bloqueada igualmente.
package http

import (
	"net/http"
	"testing"

	"git.sytue.eu/sytue/one/services/id/internal/domain"
)

func TestPlatformOrg_LaDeLaPlataformaNoSeSuspendeNiSeElimina(t *testing.T) {
	l := newLiveServer(t)
	conMFA := []string{"pwd", "mfa"}
	solo := pimActor(t, l, "unico@id.test")
	if rec := l.pimReq(t, http.MethodPost, "/api/v1/platform/activate", solo, conMFA,
		`{"justification":"pruebas"}`); rec.Code != http.StatusOK {
		t.Fatalf("elevar: código=%d body=%s", rec.Code, rec.Body)
	}

	// La del despliegue: el slug que lleva la configuración (por defecto
	// `corporate`, el mismo que `orgHost.canonical` del chart).
	propia, err := l.st.CreateOrg(l.ctx, nil,
		domain.Organization{Slug: l.srv.cfg.PlatformOrgSlug, Name: "Sytue"})
	if err != nil {
		t.Fatalf("CreateOrg(%s): %v", l.srv.cfg.PlatformOrgSlug, err)
	}
	ruta := "/api/v1/platform/orgs/" + propia.ID + "/status"

	// Con motivo escrito y con un solo administrador global —o sea, por la vía
	// que SÍ cambia el estado, sin segunda firma de por medio.
	for _, cuerpo := range []string{
		`{"status":"suspended","reason":"por probar"}`,
		`{"status":"deleted","reason":"por probar"}`,
	} {
		rec := l.pimReq(t, http.MethodPost, ruta, solo, conMFA, cuerpo)
		if rec.Code != http.StatusForbidden {
			t.Errorf("%s: código=%d, quiero 403 (body=%s)", cuerpo, rec.Code, rec.Body)
		}
		o, err := l.st.GetOrgByID(l.ctx, propia.ID)
		if err != nil {
			t.Fatalf("releer la organización: %v", err)
		}
		if o.Status != domain.StatusActive {
			t.Fatalf("%s: la organización de la plataforma quedó %q — nadie podría entrar ya",
				cuerpo, o.Status)
		}
	}

	// Y una organización CLIENTE sí se elimina: la verja es para una sola, no un
	// candado sobre la función entera.
	cliente, err := l.st.CreateOrg(l.ctx, nil, domain.Organization{Slug: "cliente", Name: "Cliente SL"})
	if err != nil {
		t.Fatalf("CreateOrg(cliente): %v", err)
	}
	rec := l.pimReq(t, http.MethodPost, "/api/v1/platform/orgs/"+cliente.ID+"/status", solo, conMFA,
		`{"status":"deleted","reason":"organización de pruebas"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("eliminar una organización cliente: código=%d body=%s", rec.Code, rec.Body)
	}
	o, err := l.st.GetOrgByID(l.ctx, cliente.ID)
	if err != nil {
		t.Fatalf("releer el cliente: %v", err)
	}
	if o.Status != domain.StatusDeleted {
		t.Fatalf("la organización cliente quedó %q; quería deleted", o.Status)
	}
}
