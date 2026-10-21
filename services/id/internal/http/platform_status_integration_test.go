// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

// El MOTIVO de suspender o reactivar una organización, contra PostgreSQL real.
//
// «Toda escritura exige motivo» es un invariante escrito de la administración de
// plataforma (docs/security.md §4.9), y suspender es su ejemplo literal: deja
// fuera a una empresa entera. Pero el único sitio donde se exigía era el
// navegador —un `window.prompt()` del panel—, así que cualquier llamada directa
// dejaba en el registro «suspended: », que no responde nada seis meses después.
//
// Se prueba contra la base y no con un test de unidad porque lo que importa no
// es el 400: es que NADA quede escrito cuando falta el motivo. Un handler que
// rechaza pero ya ha creado la solicitud de segunda firma, o ya ha cambiado el
// estado, pasaría una comprobación de código de respuesta y seguiría roto.
package http

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/services/id/internal/domain"
)

// TestPlatformEstado_SinMotivoNoSeEscribeNada usa UN solo admin global: así no
// hay solicitud de segunda firma de por medio y el 400 se mide contra la vía que
// de verdad cambia el estado.
func TestPlatformEstado_SinMotivoNoSeEscribeNada(t *testing.T) {
	l := newLiveServer(t)
	conMFA := []string{"pwd", "mfa"}
	solo := pimActor(t, l, "unico@id.test")
	if rec := l.pimReq(t, http.MethodPost, "/api/v1/platform/activate", solo, conMFA,
		`{"justification":"pruebas"}`); rec.Code != http.StatusOK {
		t.Fatalf("elevar: código=%d body=%s", rec.Code, rec.Body)
	}

	org, err := l.st.CreateOrg(l.ctx, nil, domain.Organization{Slug: "motivo", Name: "Con motivo"})
	if err != nil {
		t.Fatalf("CreateOrg: %v", err)
	}
	ruta := "/api/v1/platform/orgs/" + org.ID + "/status"

	// Sin campo, y con un motivo que son solo espacios: escribir tres espacios no
	// es dar una razón, y sin recortar el registro queda igual de mudo.
	for _, cuerpo := range []string{
		`{"status":"suspended"}`,
		`{"status":"suspended","reason":"   "}`,
		`{"status":"suspended","reason":"\t\n"}`,
	} {
		rec := l.pimReq(t, http.MethodPost, ruta, solo, conMFA, cuerpo)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s: código=%d, quiero 400 (body=%s)", cuerpo, rec.Code, rec.Body)
		}
		if o, _ := l.st.GetPlatformOrg(l.ctx, org.ID); o.Status != domain.StatusActive {
			t.Fatalf("%s: la organización quedó %q — se suspendió sin motivo", cuerpo, o.Status)
		}
	}

	// Reactivar lo exige igual. No es simetría por gusto: la pregunta que se hace
	// después de un corte de servicio es por qué volvió, y quién lo decidió.
	if _, err := l.st.SetOrgStatus(l.ctx, nil, org.ID, domain.StatusSuspended); err != nil {
		t.Fatalf("SetOrgStatus: %v", err)
	}
	if rec := l.pimReq(t, http.MethodPost, ruta, solo, conMFA, `{"status":"active"}`); rec.Code != http.StatusBadRequest {
		t.Errorf("reactivar sin motivo: código=%d, quiero 400 (body=%s)", rec.Code, rec.Body)
	}
	if o, _ := l.st.GetPlatformOrg(l.ctx, org.ID); o.Status != domain.StatusSuspended {
		t.Fatalf("la organización quedó %q — se reactivó sin motivo", o.Status)
	}

	// Y con motivo sí, con el motivo EN EL REGISTRO: es lo único que hace que la
	// fila sirva para algo cuando alguien la lea dentro de seis meses.
	if rec := l.pimReq(t, http.MethodPost, ruta, solo, conMFA,
		`{"status":"active","reason":"  pagó la factura de marzo  "}`); rec.Code != http.StatusOK {
		t.Fatalf("reactivar con motivo: código=%d body=%s", rec.Code, rec.Body)
	}
	if o, _ := l.st.GetPlatformOrg(l.ctx, org.ID); o.Status != domain.StatusActive {
		t.Fatalf("la organización sigue %q tras reactivarla con motivo", o.Status)
	}
	rec := l.pimReq(t, http.MethodGet, "/api/v1/platform/audit?action=platform.org.", solo, conMFA, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("registro: código=%d body=%s", rec.Code, rec.Body)
	}
	var registro struct {
		Items []struct {
			Action string `json:"action"`
			Target string `json:"target"`
		} `json:"items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &registro); err != nil {
		t.Fatalf("registro ilegible: %v", err)
	}
	// Una sola fila: los cuatro intentos sin motivo no dejaron rastro de cambio.
	if len(registro.Items) != 1 {
		t.Fatalf("%d filas platform.org.*; quiero 1 (los rechazos no cambian estado): %+v", len(registro.Items), registro.Items)
	}
	// Recortado, no en crudo: el motivo se guarda como se va a leer.
	if quiero := "active: pagó la factura de marzo"; registro.Items[0].Target != quiero {
		t.Errorf("el registro dice %q, quiero %q", registro.Items[0].Target, quiero)
	}
}

// Con DOS admins el estado no lo cambia la petición sino la segunda firma, y ahí
// el motivo importa aún más: es lo único que el otro administrador va a leer para
// decidir. Una solicitud muda no se crea.
func TestPlatformEstado_SinMotivoNoSePideSegundaFirma(t *testing.T) {
	l := newLiveServer(t)
	conMFA := []string{"pwd", "mfa"}
	pide, aprueba := dosAdmins(t, l)

	org, err := l.st.CreateOrg(l.ctx, nil, domain.Organization{Slug: "muda", Name: "Muda"})
	if err != nil {
		t.Fatalf("CreateOrg: %v", err)
	}
	ruta := "/api/v1/platform/orgs/" + org.ID + "/status"

	rec := l.pimReq(t, http.MethodPost, ruta, pide, conMFA, `{"status":"suspended","reason":" "}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("código=%d, quiero 400 (body=%s)", rec.Code, rec.Body)
	}
	// El 409 de segunda firma y el 400 de motivo que falta se arreglan en sitios
	// distintos, así que el cuerpo no puede confundirlos.
	if strings.Contains(rec.Body.String(), "approval_required") {
		t.Errorf("el rechazo se disfraza de «esperando firma»: %s", rec.Body)
	}

	rec = l.pimReq(t, http.MethodGet, "/api/v1/platform/approvals", aprueba, conMFA, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("listado: código=%d body=%s", rec.Code, rec.Body)
	}
	var lista struct {
		Items []map[string]any `json:"items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &lista); err != nil {
		t.Fatalf("listado ilegible: %v", err)
	}
	if len(lista.Items) != 0 {
		t.Errorf("se creó una solicitud sin motivo: %+v", lista.Items)
	}
}
