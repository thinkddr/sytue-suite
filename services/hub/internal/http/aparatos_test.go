// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package hubhttp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
)

var tokenAparato = strings.Repeat("cd02", 8)

type registroFalso struct {
	registrados []string // subject/bundle/token
	olvidos     []string // subject/token ("" = todos)
}

func (f *registroFalso) RegistrarAparato(_ context.Context, subject, bundle, token string) error {
	f.registrados = append(f.registrados, subject+"/"+bundle+"/"+token)
	return nil
}

func (f *registroFalso) OlvidarAparatos(_ context.Context, subject, token string) (int64, error) {
	f.olvidos = append(f.olvidos, subject+"/"+token)
	return 1, nil
}

type despertadorFalso struct{ cambios []string }

func (d *despertadorFalso) Cambio(_ context.Context, tenant, subject, producto string) {
	d.cambios = append(d.cambios, tenant+"/"+subject+"/"+producto)
}

func TestRegistrarAparato(t *testing.T) {
	reg := &registroFalso{}
	h := &Handlers{aparatos: reg}
	mandar := func(claims *authn.Claims, cuerpo string) int {
		rec := httptest.NewRecorder()
		h.RegistrarAparato(rec, request(t, http.MethodPost, "/api/v1/me/push/apns", claims, cuerpo))
		return rec.Code
	}
	if c := mandar(userClaims(), `{"bundle":"eu.sytue.calendar","token":" `+tokenAparato+` "}`); c != http.StatusNoContent {
		t.Fatalf("registrar: HTTP %d, quiero 204", c)
	}
	if len(reg.registrados) != 1 || reg.registrados[0] != "u/eu.sytue.calendar/"+tokenAparato {
		t.Fatalf("registrados = %v", reg.registrados)
	}
	for _, cuerpo := range []string{
		`{"bundle":"com.ejemplo.otra","token":"` + tokenAparato + `"}`,
		`{"bundle":"eu.sytue.calendar","token":"no-es-hex"}`,
		`{"bundle":"eu.sytue.calendar"}`,
		`{"bundle":"eu.sytue.calendar","token":"` + tokenAparato + `","subject":"otro"}`,
	} {
		if c := mandar(userClaims(), cuerpo); c != http.StatusBadRequest {
			t.Errorf("%s: HTTP %d, quiero 400", cuerpo, c)
		}
	}
	// Quien actúa en nombre de otra persona no hace que SU aparato se despierte con
	// los cambios de ella.
	delegado := userClaims()
	delegado.Delegado = true
	if c := mandar(delegado, `{"bundle":"eu.sytue.mail","token":"`+tokenAparato+`"}`); c != http.StatusForbidden {
		t.Fatalf("delegado: HTTP %d, quiero 403", c)
	}
	if len(reg.registrados) != 1 {
		t.Fatalf("registrados = %v: nada más que el primero", reg.registrados)
	}
}

// Sin cuerpo, olvidar borra TODOS los aparatos de la persona: es lo que promete el ADR
// 0039 y lo que pide la app al salir. Con {token}, solo ese. Visto en rojo exigiendo
// el token.
func TestOlvidarAparatosSinCuerpoLosBorraTodos(t *testing.T) {
	reg := &registroFalso{}
	h := &Handlers{aparatos: reg}
	mandar := func(cuerpo string) int {
		rec := httptest.NewRecorder()
		h.OlvidarAparatos(rec, request(t, http.MethodPost, "/api/v1/me/push/apns/forget", userClaims(), cuerpo))
		return rec.Code
	}
	for _, cuerpo := range []string{"", "  ", "{}"} {
		if c := mandar(cuerpo); c != http.StatusNoContent {
			t.Fatalf("olvidar %q: HTTP %d, quiero 204", cuerpo, c)
		}
	}
	if c := mandar(`{"token":"` + tokenAparato + `"}`); c != http.StatusNoContent {
		t.Fatalf("olvidar uno: HTTP %d", c)
	}
	quiere := []string{"u/", "u/", "u/", "u/" + tokenAparato}
	if strings.Join(reg.olvidos, ",") != strings.Join(quiere, ",") {
		t.Fatalf("olvidos = %v, quiero %v", reg.olvidos, quiere)
	}
	for _, cuerpo := range []string{`{"token":"no-es-hex"}`, `{"tokens":[]}`, `[`} {
		if c := mandar(cuerpo); c != http.StatusBadRequest {
			t.Errorf("%s: HTTP %d, quiero 400", cuerpo, c)
		}
	}
	// Quien actúa en nombre de otra persona no le borra los aparatos: sin cuerpo serían
	// todos. Visto en rojo quitando la comprobación.
	delegado := userClaims()
	delegado.Delegado = true
	for _, cuerpo := range []string{"", `{"token":"` + tokenAparato + `"}`} {
		rec := httptest.NewRecorder()
		h.OlvidarAparatos(rec, request(t, http.MethodPost, "/api/v1/me/push/apns/forget", delegado, cuerpo))
		if rec.Code != http.StatusForbidden {
			t.Fatalf("delegado %q: HTTP %d, quiero 403", cuerpo, rec.Code)
		}
	}
	if len(reg.olvidos) != len(quiere) {
		t.Fatalf("olvidos = %v: el delegado no olvida nada", reg.olvidos)
	}
}

// La señal de cambios despierta, además, a los aparatos de iOS: la recibe la réplica
// que atiende la petición, con la organización, el sujeto y el producto de la señal.
// Un evento que no es `datos` no despierta. Visto en rojo quitando la llamada.
func TestLaSenalDeCambiosDespierta(t *testing.T) {
	d := &despertadorFalso{}
	h := newHandlers(nil, &fakeNotifier{}, nil, nil, nil)
	h.despertar = d
	for _, cuerpo := range []string{
		`{"tenant":"acme","subject":"ana","event":"datos","data":{"producto":"calendar"}}`,
		`{"tenant":"acme","subject":"ana","event":"avatar","data":{"version":"3"}}`,
		`{"tenant":"acme","subject":"ana","event":"datos","data":{"producto":"inventado"}}`,
	} {
		h.CreateInternalEvent(httptest.NewRecorder(), request(t, http.MethodPost, "/internal/events", nil, cuerpo))
	}
	if len(d.cambios) != 1 || d.cambios[0] != "acme/ana/calendar" {
		t.Fatalf("cambios = %v, quiero solo acme/ana/calendar", d.cambios)
	}
}
