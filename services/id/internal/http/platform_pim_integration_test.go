// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

// PIM del alcance de PLATAFORMA (mig 0043) contra PostgreSQL real.
//
// Lo que se prueba es el cambio de contrato: estar en `platform_admins` ya NO
// autoriza por sí solo. Es la superficie que atraviesa todas las organizaciones,
// así que cada candado se comprueba por separado —y también que sigue siendo
// posible elevarse, que es la forma de romper esto sin que nadie se entere hasta
// que alguien lo necesita de madrugada.
package http

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/services/id/internal/oidc"
)

// pimActor deja un usuario en la allowlist y devuelve su id.
func pimActor(t *testing.T, l *liveServer, email string) string {
	t.Helper()
	u := l.mkUser(t, email, "")
	if err := l.st.UpsertPlatformAdmin(l.ctx, nil, u.ID, u.ID, "prueba"); err != nil {
		t.Fatalf("UpsertPlatformAdmin: %v", err)
	}
	return u.ID
}

// tokenAMR emite un access token con los métodos de autenticación dados. El
// `token` del harness no los pone, y aquí el `amr` es justo lo que se prueba.
// tokenAMR emite un token con los métodos de autenticación pedidos y con la
// identificación RECIÉN hecha: quien llama elige el `amr` porque es lo que
// prueban estos tests, pero `auth_time` no es parte de eso y sin él las rutas
// con step-up rechazan incluso a quien trae segundo factor.
func (l *liveServer) tokenAMR(t *testing.T, sub string, amr []string) string {
	t.Helper()
	raw, err := l.iss.IssueAccessToken(oidc.TokenClaims{
		Subject: sub, Audience: "sytue-console", Scope: "openid", AMR: amr,
		AuthTime: time.Now().UTC(),
	}, time.Now().UTC())
	if err != nil {
		t.Fatalf("IssueAccessToken: %v", err)
	}
	return raw
}

// pimReq llama a la API con un token de los claims dados.
func (l *liveServer) pimReq(t *testing.T, metodo, ruta, sub string, amr []string, cuerpo string) *httptest.ResponseRecorder {
	t.Helper()
	tok := l.tokenAMR(t, sub, amr)
	var body *strings.Reader
	if cuerpo == "" {
		body = strings.NewReader("")
	} else {
		body = strings.NewReader(cuerpo)
	}
	req := httptest.NewRequest(metodo, ruta, body)
	req.Header.Set("Authorization", "Bearer "+tok)
	if cuerpo != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	l.mux.ServeHTTP(rec, req)
	return rec
}

func TestPlatformPIM_LaAllowlistYaNoAutorizaSola(t *testing.T) {
	l := newLiveServer(t)
	sub := pimActor(t, l, "global-pim@id.test")
	conMFA := []string{"pwd", "mfa"}

	// 1. En la allowlist y con MFA, pero SIN elevación: la API de plataforma
	//    responde 403 con un código que dice qué hacer.
	rec := l.pimReq(t, http.MethodGet, "/api/v1/platform/usage", sub, conMFA, "")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("sin elevación: código=%d, quiero 403 (cuerpo=%s)", rec.Code, rec.Body)
	}
	if !strings.Contains(rec.Body.String(), "activation_required") {
		t.Errorf("el 403 no distingue «sin elevar» de «no eres admin»: %s", rec.Body)
	}

	// 2. Elevarse EXIGE motivo: sin él no hay registro que valga.
	rec = l.pimReq(t, http.MethodPost, "/api/v1/platform/activate", sub, conMFA, `{"justification":"  "}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("activación sin motivo: código=%d, quiero 400", rec.Code)
	}

	// 3. Con motivo, se eleva y la API se abre.
	rec = l.pimReq(t, http.MethodPost, "/api/v1/platform/activate", sub, conMFA,
		`{"justification":"migrar la org acme","duration_minutes":30}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("activación: código=%d body=%s", rec.Code, rec.Body)
	}
	var resp struct {
		Activation struct {
			EffectiveStatus  string `json:"effective_status"`
			Justification    string `json:"justification"`
			RemainingSeconds int    `json:"remaining_seconds"`
		} `json:"activation"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("respuesta ilegible: %v", err)
	}
	if resp.Activation.EffectiveStatus != "active" || resp.Activation.Justification != "migrar la org acme" {
		t.Errorf("activación devuelta: %+v", resp.Activation)
	}
	if resp.Activation.RemainingSeconds <= 0 {
		t.Errorf("remaining_seconds=%d: el cliente no puede saber cuánto le queda", resp.Activation.RemainingSeconds)
	}

	rec = l.pimReq(t, http.MethodGet, "/api/v1/platform/usage", sub, conMFA, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("elevado: código=%d body=%s", rec.Code, rec.Body)
	}

	// 4. Bajar la elevación vuelve a cerrar la puerta, sin esperar a que caduque.
	rec = l.pimReq(t, http.MethodPost, "/api/v1/platform/deactivate", sub, conMFA, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("desactivación: código=%d body=%s", rec.Code, rec.Body)
	}
	rec = l.pimReq(t, http.MethodGet, "/api/v1/platform/usage", sub, conMFA, "")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("tras desactivar: código=%d, quiero 403", rec.Code)
	}
}

func TestPlatformPIM_LaElevacionCaducaSola(t *testing.T) {
	l := newLiveServer(t)
	sub := pimActor(t, l, "caduca@id.test")
	now := time.Now()

	// Una elevación que venció hace un minuto. Se escribe en la base con la fecha
	// pasada porque lo que importa es que NADIE la marcó como caducada: el gate
	// tiene que verla vencida por sí mismo, sin barrido que la actualice.
	if _, err := l.st.CreatePlatformActivation(l.ctx, nil, sub, "ventana corta", 5, now.Add(-6*time.Minute)); err != nil {
		t.Fatalf("CreatePlatformActivation: %v", err)
	}
	vivo, err := l.st.HasLivePlatformActivation(l.ctx, sub, now)
	if err != nil {
		t.Fatalf("HasLivePlatformActivation: %v", err)
	}
	if vivo {
		t.Fatal("una elevación vencida sigue autorizando: la caducidad no se evalúa al leer")
	}
	rec := l.pimReq(t, http.MethodGet, "/api/v1/platform/usage", sub, []string{"pwd", "mfa"}, "")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("con la elevación vencida: código=%d, quiero 403", rec.Code)
	}
}

func TestPlatformPIM_NoSePuedeElevarSinAllowlistNiSinMFA(t *testing.T) {
	l := newLiveServer(t)

	// Un usuario cualquiera, con MFA, NO puede elevarse: la elegibilidad sigue
	// siendo la allowlist, y esta ruta está fuera del gate de plataforma.
	ajeno := l.mkUser(t, "ajeno@id.test", "")
	rec := l.pimReq(t, http.MethodPost, "/api/v1/platform/activate", ajeno.ID,
		[]string{"pwd", "mfa"}, `{"justification":"quiero entrar"}`)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("ajeno a la allowlist: código=%d, quiero 403", rec.Code)
	}

	// Y un admin de plataforma SIN segundo factor tampoco: elevarse es
	// exactamente el momento en el que el MFA importa.
	//
	// Quien lo contesta es el step-up, que va DELANTE del handler en esta ruta,
	// y lo hace con el 401 del RFC 9470 en lugar del 403 `mfa_required`. No es
	// un candado de menos sino el mismo candado antes y mejor explicado: el 403
	// es terminal —«no puedes»— y el 401 con `WWW-Authenticate` dice qué falta,
	// que es lo único que permite al cliente ofrecer «vuelve a identificarte».
	sub := pimActor(t, l, "sin-mfa@id.test")
	rec = l.pimReq(t, http.MethodPost, "/api/v1/platform/activate", sub,
		[]string{"pwd"}, `{"justification":"sin segundo factor"}`)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("sin MFA: código=%d, quiero 401 (cuerpo=%s)", rec.Code, rec.Body)
	}
	if !strings.Contains(rec.Body.String(), "insufficient_user_authentication") {
		t.Errorf("el 401 no dice que falta el segundo factor: %s", rec.Body)
	}
	if cab := rec.Header().Get("WWW-Authenticate"); !strings.Contains(cab, "insufficient_user_authentication") {
		t.Errorf("sin la cabecera del RFC 9470 el cliente no sabe qué pedir: %q", cab)
	}

	// El gate del handler sigue vivo y sigue distinguiendo los motivos: se llega
	// a él por las rutas de plataforma que NO son la elevación. Se comprueba
	// aquí porque, si algún día se le pone step-up a todas, este 403 se vuelve
	// inalcanzable sin que nada avise —y `verifyaccess` lo lee para decir «el
	// token no acredita segundo factor» en vez de «no estás en la allowlist».
	rec = l.pimReq(t, http.MethodGet, "/api/v1/platform/activation", sub, []string{"pwd"}, "")
	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "mfa_required") {
		t.Errorf("consultando la elevación sin MFA: código=%d cuerpo=%s, quiero 403 mfa_required", rec.Code, rec.Body)
	}
}

func TestPlatformPIM_ElevarseDosVecesNoDejaDosVivas(t *testing.T) {
	l := newLiveServer(t)
	sub := pimActor(t, l, "doble@id.test")
	now := time.Now()

	for _, motivo := range []string{"primera", "segunda"} {
		if _, err := l.st.CreatePlatformActivation(l.ctx, nil, sub, motivo, 30, now); err != nil {
			t.Fatalf("CreatePlatformActivation(%s): %v", motivo, err)
		}
	}
	lista, err := l.st.ListPlatformActivations(l.ctx, 10)
	if err != nil {
		t.Fatalf("ListPlatformActivations: %v", err)
	}
	activas := 0
	for _, a := range lista {
		if a.Status == "active" {
			activas++
		}
	}
	if activas != 1 {
		t.Errorf("%d elevaciones activas a la vez: el histórico diría que estuvo elevado dos veces al mismo tiempo", activas)
	}
	// Y la que queda viva es la última, con su motivo.
	actual, existe, err := l.st.CurrentPlatformActivation(l.ctx, sub, now)
	if err != nil || !existe {
		t.Fatalf("CurrentPlatformActivation: %v (existe=%v)", err, existe)
	}
	if actual.Justification != "segunda" {
		t.Errorf("motivo vigente = %q, quiero el de la última elevación", actual.Justification)
	}
}

// El bootstrap deja al primer admin en la allowlist; que pueda elevarse es lo que
// impide que un despliegue nuevo se quede sin nadie capaz de entrar.
func TestPlatformPIM_ElPrimerAdminPuedeElevarse(t *testing.T) {
	l := newLiveServer(t)
	u := l.mkUser(t, "bootstrap@id.test", "")
	if err := l.st.UpsertPlatformAdmin(l.ctx, nil, u.ID, "", "bootstrap del despliegue"); err != nil {
		t.Fatalf("UpsertPlatformAdmin: %v", err)
	}
	rec := l.pimReq(t, http.MethodPost, "/api/v1/platform/activate", u.ID,
		[]string{"pwd", "mfa"}, `{"justification":"primer arranque"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("el primer admin no pudo elevarse: código=%d body=%s", rec.Code, rec.Body)
	}
}
