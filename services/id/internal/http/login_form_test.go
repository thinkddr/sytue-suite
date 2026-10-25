// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package http

import (
	"html"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// forgotPasswordHTML renderiza el formulario de recuperación tal cual se sirve
// (vive inline en su handler, que no toca ni base de datos ni configuración).
func forgotPasswordHTML(t *testing.T) string {
	t.Helper()
	rec := httptest.NewRecorder()
	(&Server{}).handleForgotPasswordForm(rec, httptest.NewRequest(http.MethodGet, "/forgot-password", http.NoBody))
	return rec.Body.String()
}

// Señales de autofill del campo de identidad. Con `type=email` (perfil de
// direcciones) Y `autocomplete=username` (credencial guardada), algunos gestores
// rellenan DOS veces sin limpiar y el campo acaba con el correo repetido; peor
// aún, la validación nativa de `type=email` rechaza ese valor y BLOQUEA el envío,
// así que «Continuar» no hace nada y no hay forma de entrar.
//
// Los tres formularios que piden identidad tienen que dar las MISMAS señales: si
// uno diverge, el fallo reaparece solo en él y cuesta el doble de encontrar.
func TestCampoDeIdentidadSinSenalesAmbiguas(t *testing.T) {
	formularios := map[string]string{
		"acceso":       loginFormHTML("es", "/", "", "", true),
		"recuperación": forgotPasswordHTML(t),
		"organización": ssoFormHTML("es", "/", "", ""),
	}
	for nombre, html := range formularios {
		if strings.Contains(html, `type="email"`) {
			t.Errorf("%s: el campo sigue siendo type=email (bloquea el envío si el gestor duplica)", nombre)
		}
		for _, attr := range []string{
			`type="text"`, `inputmode="email"`, `name="email"`,
			`autocomplete="username"`, `required`,
		} {
			if !strings.Contains(html, attr) {
				t.Errorf("%s: falta %s en el campo de identidad", nombre, attr)
			}
		}
		// Un solo campo de identidad por página: dos le dan al gestor dos destinos
		// donde escribir, que es la otra forma de acabar con el valor repetido.
		if n := strings.Count(html, `id="email"`); n != 1 {
			t.Errorf("%s: %d campos con id=email, quiero exactamente 1", nombre, n)
		}
	}
}

// El paso 2 del acceso tiene que ofrecer la PASSKEY, no solo el código.
//
// `mfa_enabled` se pone a true tanto al confirmar un TOTP como al registrar una
// passkey, pero este formulario solo tenía el campo del código: quien había
// elegido passkey y no tenía TOTP quedaba atrapado —se le pedía un código que no
// existe, fallaba siempre, y la única salida era «volver a iniciar sesión», que
// devuelve al mismo sitio—. Un segundo factor que no se puede superar es un
// lockout, no una medida de seguridad.
func TestPasoDosOfreceLaPasskey(t *testing.T) {
	s := &Server{}
	rec := httptest.NewRecorder()
	s.handleLoginForm(rec, httptest.NewRequest(http.MethodGet, "/login/?step_up=1", http.NoBody))

	cuerpo := rec.Body.String()
	if !strings.Contains(cuerpo, `id="passkey-btn"`) {
		t.Error("el paso 2 no ofrece la passkey: quien solo tiene passkey no puede pasar de aquí")
	}
	// Y el script que la activa, o el botón se queda oculto para siempre.
	if !strings.Contains(cuerpo, "navigator.credentials") {
		t.Error("falta el script de la ceremonia: el botón nunca se muestra")
	}
	// El código TOTP sigue estando: esto añade una vía, no sustituye la otra.
	if !strings.Contains(cuerpo, `name="code"`) {
		t.Error("desapareció el campo del código")
	}
}

// El desafío tiene que ser una página utilizable incluso sin JavaScript y
// conservar el destino que pidió la reautenticación.
func TestDesafioMFASirveHTMLConFormularioYViaje(t *testing.T) {
	const viaje = "/oauth2/authorize?client_id=sytue-console&scope=openid&max_age=300"
	rec := httptest.NewRecorder()
	(&Server{}).handleLoginForm(rec, httptest.NewRequest(http.MethodGet,
		"/login/?"+url.Values{"step_up": {"1"}, "return_to": {viaje}}.Encode(), http.NoBody))
	if rec.Code != http.StatusOK || !strings.HasPrefix(rec.Header().Get("Content-Type"), "text/html") {
		t.Fatalf("desafío: status=%d Content-Type=%q", rec.Code, rec.Header().Get("Content-Type"))
	}
	for _, parte := range []string{
		`<form method="post" action="/login/mfa">`,
		`name="code" autocomplete="one-time-code"`,
		`<button type="submit">`,
		`id="passkey-btn"`,
		`<input type="hidden" name="return_to" value="` + html.EscapeString(viaje) + `">`,
		`href="/login/?return_to=` + url.QueryEscape(viaje) + `"`,
	} {
		if !strings.Contains(rec.Body.String(), parte) {
			t.Errorf("el desafío MFA no contiene %s", parte)
		}
	}
}

// El viaje (return_to) tiene que sobrevivir al desvío por «olvidé mi contraseña».
//
// El enlace se pintaba SIN return_to mientras el de SSO, justo debajo, y el paso
// de MFA sí lo conservaban: quien entraba desde one.sytue.eu y pasaba por aquí
// terminaba en la cuenta de Sytue ID y tenía que rehacer el OAuth entero. Se
// recorre la cadena completa dentro de la misma pestaña —login → recuperación →
// PRG de «enlace enviado» → vuelta al login—, porque basta con que UN eslabón lo
// suelte para perder el viaje.
func TestOlvidoDeContrasenaConservaElViaje(t *testing.T) {
	const viaje = "/oauth2/authorize?client_id=one&scope=openid"
	// Escapado como aparece en un href/atributo (& → &amp;) y en una query.
	esperado := "return_to=" + url.QueryEscape(viaje)

	// 1. El login manda el viaje al enlace de olvido.
	rec := httptest.NewRecorder()
	(&Server{}).handleLoginForm(rec,
		httptest.NewRequest(http.MethodGet, "/login/?"+url.Values{"return_to": {viaje}}.Encode(), http.NoBody))
	if !strings.Contains(rec.Body.String(), "/forgot-password?"+esperado) {
		t.Errorf("el enlace de olvido pierde el viaje; quiero /forgot-password?%s", esperado)
	}

	// 2. La página de recuperación lo conserva en el POST y en la vuelta al login.
	rec = httptest.NewRecorder()
	(&Server{}).handleForgotPasswordForm(rec,
		httptest.NewRequest(http.MethodGet, "/forgot-password?"+url.Values{"return_to": {viaje}}.Encode(), http.NoBody))
	cuerpo := rec.Body.String()
	if !strings.Contains(cuerpo, `<input type="hidden" name="return_to" value="`+html.EscapeString(viaje)+`">`) {
		t.Error("el formulario de recuperación no lleva el viaje en su campo oculto")
	}
	if !strings.Contains(cuerpo, "/login/?"+esperado) {
		t.Errorf("la vuelta al login pierde el viaje; quiero /login/?%s", esperado)
	}

	// 3. El PRG del envío lo arrastra a ?sent=1 (email vacío: no dispara el correo
	//    ni toca el store, y la respuesta uniforme es la misma).
	rec = httptest.NewRecorder()
	post := httptest.NewRequest(http.MethodPost, "/forgot-password",
		strings.NewReader(url.Values{"email": {""}, "return_to": {viaje}}.Encode()))
	post.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	(&Server{}).handleForgotPasswordSubmit(rec, post)
	loc := rec.Header().Get("Location")
	if !strings.Contains(loc, "sent=1") || !strings.Contains(loc, esperado) {
		t.Errorf("el PRG pierde el viaje: Location = %q", loc)
	}

	// 4. Y la página de «enlace enviado» ofrece la vuelta al viaje, no al login pelado.
	rec = httptest.NewRecorder()
	(&Server{}).handleForgotPasswordForm(rec, httptest.NewRequest(http.MethodGet, loc, http.NoBody))
	if !strings.Contains(rec.Body.String(), "/login/?"+esperado) {
		t.Error("«enlace enviado» pierde el viaje en su vuelta al login")
	}

	// Sin viaje no se cuelga un ?return_to= vacío de ninguna de las dos páginas.
	for nombre, pagina := range map[string]string{
		"acceso":       loginFormHTML("es", "", "", "", false),
		"recuperación": forgotPasswordHTML(t),
	} {
		if strings.Contains(pagina, "return_to=") {
			t.Errorf("%s: sin viaje que conservar, el href no debe llevar return_to", nombre)
		}
	}
}

// TestElFalloDeLaPasskeySaleEnElIdiomaDeLaPagina: el aviso de que la passkey no ha
// valido es el traducido que la página deja en el botón (`data-err`), no un texto fijo
// en español. Es la pantalla por la que entran también las apps nativas (RFC 8252: el
// navegador del sistema). VISTO EN ROJO con el literal de antes.
func TestElFalloDeLaPasskeySaleEnElIdiomaDeLaPagina(t *testing.T) {
	if !strings.Contains(loginScript, "fail(b.dataset.err)") || strings.Contains(loginScript, "No se pudo") {
		t.Error("el script del login por passkey no usa el aviso traducido del botón")
	}
	if !strings.Contains(passkeyBlockHTML("de"), `data-err="Anmeldung mit Passkey fehlgeschlagen.`) {
		t.Error("el botón de passkey no lleva el aviso en el idioma de la página")
	}
}
