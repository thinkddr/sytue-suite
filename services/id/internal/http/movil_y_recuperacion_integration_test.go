// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

// El móvil verificado por SMS y el correo de recuperación (ADR 0041) contra un
// PostgreSQL real: la cuenta no nace sin el código; el código se equivoca, caduca
// y se reenvía con sus límites; un móvil con cuenta solo se delata a quien
// demuestra que es suyo; y se entra con los cuatro identificadores y la misma
// contraseña.
package http

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"hash/crc32"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/altcha"
	"git.sytue.eu/sytue/one/services/id/internal/domain"
	"git.sytue.eu/sytue/one/services/id/internal/i18n"
)

// smsDePrueba guarda el último SMS enviado a cada número; el código sale de ahí.
type smsDePrueba struct {
	mu    sync.Mutex
	texto map[string]string
	fallo error
}

func (f *smsDePrueba) Send(_ context.Context, to, text string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.fallo != nil {
		return f.fallo
	}
	if f.texto == nil {
		f.texto = map[string]string{}
	}
	f.texto[to] = text
	return nil
}

var reCodigoSMS = regexp.MustCompile(`#([0-9]{6})$`)

// codigo es el código del último SMS a ese número (va en la línea de WebOTP).
func (f *smsDePrueba) codigo(t *testing.T, to string) string {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	m := reCodigoSMS.FindStringSubmatch(f.texto[to])
	if m == nil {
		t.Fatalf("no llegó ningún SMS con código a %s: %q", to, f.texto[to])
	}
	return m[1]
}

func (l *liveServer) smsDePrueba() *smsDePrueba {
	if f, ok := l.srv.sms.(*smsDePrueba); ok {
		return f
	}
	f := &smsDePrueba{}
	l.srv.sms = f
	return f
}

// telefonoDePrueba da a cada @usuario un móvil propio y estable.
func telefonoDePrueba(username string) string {
	return fmt.Sprintf("+34%09d", 600000000+crc32.ChecksumIEEE([]byte(username))%99999999)
}

// sinLimites vacía los limitadores: estos tests hacen desde una sola IP más
// peticiones seguidas de las que deja el login, y lo que se mide aquí no es eso.
func (l *liveServer) sinLimites(t *testing.T) {
	t.Helper()
	if _, err := l.st.DB().Exec(l.ctx, `DELETE FROM platform_rate_limits`); err != nil {
		t.Fatal(err)
	}
}

var reAltaID = regexp.MustCompile(`name="id" value="([0-9a-f-]{36})"`)

// altaID saca el alta pendiente de la pantalla del código.
func altaID(t *testing.T, rr *httptest.ResponseRecorder) string {
	t.Helper()
	m := reAltaID.FindStringSubmatch(rr.Body.String())
	if m == nil {
		t.Fatalf("la pantalla del código no trae el alta (status=%d): %s", rr.Code, rr.Body)
	}
	return m[1]
}

// alta hace el alta entera —formulario, SMS y código— y devuelve la respuesta
// del último paso. Si el formulario no llega a mandar el SMS, devuelve la suya.
func (l *liveServer) alta(t *testing.T, campos url.Values) *httptest.ResponseRecorder {
	t.Helper()
	sms := l.smsDePrueba()
	if campos.Get("phone") == "" {
		campos.Set("phone", telefonoDePrueba(domain.NormalizeUsername(campos.Get("username"))))
	}
	rr := l.formulario(t, "/signup", campos)
	if rr.Code != http.StatusOK {
		return rr
	}
	phone, _ := domain.NormalizePhone(campos.Get("phone"))
	return l.formulario(t, "/signup/code", url.Values{
		"id": {altaID(t, rr)}, "code": {sms.codigo(t, phone)}, "return_to": {campos.Get("return_to")},
	})
}

func TestAlta_SinCodigoNoHayCuentaYElCodigoTieneLimites(t *testing.T) {
	l := newLiveServer(t)
	sms := l.smsDePrueba()
	const pw = "una-contraseña-larga-2026"
	const movil = "+34 611 222 333"

	rr := l.formulario(t, "/signup", url.Values{"username": {"elena"}, "password": {pw}, "phone": {movil}})
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), "333") {
		t.Fatalf("formulario: status=%d cuerpo=%s", rr.Code, rr.Body)
	}
	id := altaID(t, rr)
	if _, err := l.st.GetUserByUsername(l.ctx, "elena"); err == nil {
		t.Fatal("la cuenta no puede existir antes de confirmar el código")
	}
	primero := sms.codigo(t, "+34611222333")
	if !strings.Contains(sms.texto["+34611222333"], "@"+strings.TrimPrefix(itIssuer, "https://")) {
		t.Fatalf("el SMS debe llevar la línea de WebOTP ligada al dominio: %q", sms.texto["+34611222333"])
	}

	// Un código mal escrito lo dice y deja seguir.
	otro := "000000"
	if primero == otro {
		otro = "111111"
	}
	rr = l.formulario(t, "/signup/code", url.Values{"id": {id}, "code": {otro}})
	if rr.Code != http.StatusBadRequest || !strings.Contains(rr.Body.String(), i18n.T("es", "signup.code_wrong")) {
		t.Fatalf("código incorrecto: status=%d", rr.Code)
	}

	// Reenviar antes de un minuto no manda otro SMS.
	rr = l.formulario(t, "/signup/code", url.Values{"id": {id}, "resend": {"1"}})
	if rr.Code != http.StatusTooManyRequests || sms.codigo(t, "+34611222333") != primero {
		t.Fatalf("reenvío inmediato: status=%d", rr.Code)
	}
	// Pasado el minuto sí, y el código anterior deja de valer.
	if _, err := l.st.DB().Exec(l.ctx, `UPDATE signup_phone_codes SET sent_at = now() - interval '2 minutes' WHERE id = $1`, id); err != nil {
		t.Fatal(err)
	}
	for sms.codigo(t, "+34611222333") == primero {
		rr = l.formulario(t, "/signup/code", url.Values{"id": {id}, "resend": {"1"}})
		if rr.Code != http.StatusOK {
			t.Fatalf("reenvío pasado el minuto: status=%d cuerpo=%s", rr.Code, rr.Body)
		}
		if _, err := l.st.DB().Exec(l.ctx, `UPDATE signup_phone_codes SET sent_at = now() - interval '2 minutes' WHERE id = $1`, id); err != nil {
			t.Fatal(err)
		}
	}
	if rr := l.formulario(t, "/signup/code", url.Values{"id": {id}, "code": {primero}}); rr.Code != http.StatusBadRequest {
		t.Fatalf("el código reemplazado no puede crear la cuenta: status=%d", rr.Code)
	}

	// El bueno crea la cuenta con su móvil y entra.
	rr = l.formulario(t, "/signup/code", url.Values{"id": {id}, "code": {sms.codigo(t, "+34611222333")}})
	if rr.Code != http.StatusOK || cookieNamed(rr.Result().Cookies(), sessionCookieName) == nil {
		t.Fatalf("código correcto: status=%d cuerpo=%s", rr.Code, rr.Body)
	}
	u, err := l.st.GetUserByUsername(l.ctx, "elena")
	if err != nil || u.Phone != "+34611222333" {
		t.Fatalf("cuenta sin su móvil: %+v (%v)", u, err)
	}
	// Y el alta ya no sirve para nada más.
	if rr := l.formulario(t, "/signup/code", url.Values{"id": {id}, "code": {"123456"}}); rr.Code != http.StatusGone {
		t.Fatalf("alta ya usada: status=%d", rr.Code)
	}

	// Cinco fallos seguidos tiran el alta entera.
	l.sinLimites(t)
	rr = l.formulario(t, "/signup", url.Values{"username": {"fallos"}, "password": {pw}, "phone": {"+34 611 000 999"}})
	id = altaID(t, rr)
	bueno := sms.codigo(t, "+34611000999")
	malo := "000000"
	if bueno == malo {
		malo = "111111"
	}
	for i := 1; i <= domain.SignupCodeMaxAttempts; i++ {
		rr = l.formulario(t, "/signup/code", url.Values{"id": {id}, "code": {malo}})
	}
	if rr.Code != http.StatusGone {
		t.Fatalf("tras %d fallos el alta debe caducar: status=%d", domain.SignupCodeMaxAttempts, rr.Code)
	}
	if rr := l.formulario(t, "/signup/code", url.Values{"id": {id}, "code": {bueno}}); rr.Code != http.StatusGone {
		t.Fatalf("ni el código bueno vale tras agotar los intentos: status=%d", rr.Code)
	}
}

// Un móvil que ya tiene cuenta no se delata en el formulario: se manda el SMS
// igual y solo quien teclea el código se entera. Y no se crea otra cuenta.
func TestAlta_UnMovilConCuentaSoloSeDiceTrasElCodigo(t *testing.T) {
	l := newLiveServer(t)
	const pw = "una-contraseña-larga-2026"
	if rr := l.alta(t, url.Values{"username": {"hugo"}, "password": {pw}, "phone": {"+34 622 333 444"}}); rr.Code != http.StatusOK {
		t.Fatalf("primera alta: %d %s", rr.Code, rr.Body)
	}
	rr := l.formulario(t, "/signup", url.Values{"username": {"hugo2"}, "password": {pw}, "phone": {"0034622333444"}})
	if rr.Code != http.StatusOK {
		t.Fatalf("el formulario no puede decir que el móvil tiene cuenta: status=%d", rr.Code)
	}
	rr = l.formulario(t, "/signup/code", url.Values{"id": {altaID(t, rr)}, "code": {l.smsDePrueba().codigo(t, "+34622333444")}})
	if rr.Code != http.StatusConflict || !strings.Contains(rr.Body.String(), i18n.T("es", "signup.phone_has_account")) {
		t.Fatalf("tras el código sí se dice: status=%d", rr.Code)
	}
	if _, err := l.st.GetUserByUsername(l.ctx, "hugo2"); err == nil {
		t.Fatal("con el móvil cogido no puede nacer otra cuenta")
	}
}

func TestAlta_SinSMSNoHayAltaYUnFalloNoDejaNada(t *testing.T) {
	l := newLiveServer(t)
	sms := l.smsDePrueba()
	sms.fallo = errors.New("sweego sms: HTTP 402: Not enough credits")
	rr := l.formulario(t, "/signup", url.Values{"username": {"iris"}, "password": {"una-contraseña-larga-2026"}, "phone": {"+34 633 444 555"}})
	if rr.Code != http.StatusBadGateway || !strings.Contains(rr.Body.String(), i18n.T("es", "signup.sms_failed")) {
		t.Fatalf("SMS que no sale: status=%d", rr.Code)
	}
	var n int
	if err := l.st.DB().QueryRow(l.ctx, `SELECT count(*) FROM signup_phone_codes`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("un alta cuyo SMS no salió no puede quedarse pendiente: %d (%v)", n, err)
	}
	// Formato de móvil sin prefijo de país.
	sms.fallo = nil
	rr = l.formulario(t, "/signup", url.Values{"username": {"iris"}, "password": {"una-contraseña-larga-2026"}, "phone": {"633 444 555"}})
	if rr.Code != http.StatusBadRequest || !strings.Contains(rr.Body.String(), i18n.T("es", "signup.phone_invalid")) {
		t.Fatalf("móvil sin prefijo: status=%d", rr.Code)
	}

	l.srv.sms = nil
	if rr := l.get(t, "/signup"); rr.Code != http.StatusNotFound {
		t.Fatalf("sin SMS la página de alta no se ofrece: status=%d", rr.Code)
	}
	if rr := l.formulario(t, "/signup", url.Values{"username": {"iris"}}); rr.Code != http.StatusNotFound {
		t.Fatalf("sin SMS no se da de alta: status=%d", rr.Code)
	}
}

// correoDePrueba anota a quién va cada enlace de reseteo.
type correoDePrueba struct {
	captureMailer
	resetTo []string
}

func (m *correoDePrueba) SendPasswordReset(ctx context.Context, to, link string) error {
	m.resetTo = append(m.resetTo, to)
	return m.captureMailer.SendPasswordReset(ctx, to, link)
}

// Se entra con los cuatro identificadores y la misma contraseña; el correo de
// recuperación pide la contraseña, no se fija hasta confirmarlo y recibe el
// enlace de «olvidé mi contraseña».
func TestLogin_CuatroIdentificadoresYCorreoDeRecuperacion(t *testing.T) {
	l := newLiveServer(t)
	mc := &correoDePrueba{}
	l.srv.mailer = mc
	const pw = "una-contraseña-larga-2026"
	if rr := l.alta(t, url.Values{"username": {"julia"}, "password": {pw}, "phone": {"+34 644 555 666"}}); rr.Code != http.StatusOK {
		t.Fatalf("alta: %d %s", rr.Code, rr.Body)
	}
	u, _ := l.st.GetUserByUsername(l.ctx, "julia")
	tok := l.token(t, u.ID, "", nil)

	// Sin la contraseña, o con una dirección de Sytue, no.
	if rr, _ := l.do(t, http.MethodPut, "/api/v1/me/recovery-email", tok, map[string]string{"email": "julia@example.org", "password": "mala"}); rr.Code != http.StatusUnauthorized {
		t.Fatalf("sin la contraseña: status=%d", rr.Code)
	}
	if rr, _ := l.do(t, http.MethodPut, "/api/v1/me/recovery-email", tok, map[string]string{"email": "otra@" + suiteDomain(itIssuer), "password": pw}); rr.Code != http.StatusBadRequest {
		t.Fatalf("dirección de Sytue: status=%d", rr.Code)
	}
	rr, out := l.do(t, http.MethodPut, "/api/v1/me/recovery-email", tok, map[string]string{"email": "Julia@Example.org", "password": pw})
	if rr.Code != http.StatusAccepted || out["pending"] != "julia@example.org" || mc.verifyTo != "julia@example.org" {
		t.Fatalf("pedir el correo de recuperación: status=%d %v %+v", rr.Code, out, mc)
	}
	if got, _ := l.st.GetUserByID(l.ctx, u.ID); got.RecoveryEmail != "" {
		t.Fatal("no se fija hasta confirmar el enlace")
	}
	if rr := l.formulario(t, "/verify-email", url.Values{"token": {tokenFromLink(t, mc.verifyLink)}}); rr.Code != http.StatusOK ||
		!strings.Contains(rr.Body.String(), "recuperación") {
		t.Fatalf("confirmar: status=%d cuerpo=%s", rr.Code, rr.Body)
	}
	if got, _ := l.st.GetUserByID(l.ctx, u.ID); got.RecoveryEmail != "julia@example.org" {
		t.Fatalf("correo de recuperación sin fijar: %+v", got)
	}

	// Un buzón de Sytue con otra dirección también es suyo.
	buzon := "julia.trabajo@" + suiteDomain(itIssuer)
	org, err := l.st.CreateOrg(l.ctx, nil, domain.Organization{Slug: "trabajo", Name: "Trabajo"})
	if err != nil {
		t.Fatal(err)
	}
	if err := l.st.ReserveMailbox(l.ctx, nil, org.ID, u.ID, buzon); err != nil {
		t.Fatal(err)
	}
	l.sinLimites(t)
	for _, ident := range []string{"+34 644 555 666", "0034644555666", "@julia", "julia", u.Email, buzon, "julia@example.org"} {
		if rr := l.formulario(t, "/login/", url.Values{"email": {ident}, "password": {pw}}); rr.Code != http.StatusFound {
			t.Fatalf("login con %q: status=%d", ident, rr.Code)
		}
	}
	if rr := l.formulario(t, "/login/", url.Values{"email": {"+34 644 555 666"}, "password": {"otra-contraseña-2026"}}); rr.Code != http.StatusUnauthorized {
		t.Fatalf("el móvil no cambia la contraseña: status=%d", rr.Code)
	}

	// El reseteo va a los dos correos.
	if err := l.srv.sendResetLink(l.ctx, mustUser(t, l, u.ID)); err != nil {
		t.Fatal(err)
	}
	if strings.Join(mc.resetTo, ",") != u.Email+",julia@example.org" {
		t.Fatalf("el reseteo debe ir al correo de acceso y al de recuperación: %v", mc.resetTo)
	}

	// La dirección que ya da acceso a otra cuenta no se puede quedar como
	// recuperación, y retirarla también pide la contraseña.
	l.sinLimites(t)
	l.mkUser(t, "otra@example.org", pw)
	if rr, _ := l.do(t, http.MethodPut, "/api/v1/me/recovery-email", tok, map[string]string{"email": "otra@example.org", "password": pw}); rr.Code != http.StatusAccepted {
		t.Fatalf("pedirla se puede (no delata a nadie): status=%d", rr.Code)
	}
	if rr := l.formulario(t, "/verify-email", url.Values{"token": {tokenFromLink(t, mc.verifyLink)}}); rr.Code != http.StatusConflict {
		t.Fatalf("confirmar una dirección de otra cuenta: status=%d", rr.Code)
	}
	if rr, out := l.do(t, http.MethodPut, "/api/v1/me/recovery-email", tok, map[string]string{"email": "", "password": pw}); rr.Code != http.StatusOK || out["recovery_email"] != "" {
		t.Fatalf("retirar: status=%d %v", rr.Code, out)
	}
	if rr := l.formulario(t, "/login/", url.Values{"email": {"julia@example.org"}, "password": {pw}}); rr.Code != http.StatusUnauthorized {
		t.Fatalf("retirado, ya no da acceso: status=%d", rr.Code)
	}
}

func mustUser(t *testing.T, l *liveServer, id string) domain.User {
	t.Helper()
	u, err := l.st.GetUserByID(l.ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	return u
}

// El login con contraseña pide el captcha: sin reto resuelto no se mira la
// cuenta; con él, se entra. Y un alta que empezó en una app vuelve a esa app.
func TestLogin_ConCaptchaYAltaQueVuelveALaApp(t *testing.T) {
	l := newLiveServer(t)
	const pw = "una-contraseña-larga-2026"
	rr := l.alta(t, url.Values{"username": {"karla"}, "password": {pw}, "return_to": {"/oauth2/authorize?client_id=app"}})
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), `href="/oauth2/authorize?client_id=app"`) {
		t.Fatalf("el final del alta debe volver a la app que pidió entrar: %d %s", rr.Code, rr.Body)
	}

	l.srv.cfg.LoginCaptchaEnabled = true
	if pagina := l.get(t, "/login/"); !strings.Contains(pagina.Body.String(), `data-challenge="/login/captcha"`) {
		t.Fatal("el formulario de acceso debe llevar el captcha")
	}
	rr = l.formulario(t, "/login/", url.Values{"email": {"karla"}, "password": {pw}})
	if rr.Code != http.StatusUnauthorized || !strings.Contains(rr.Body.String(), i18n.T("es", "signup.captcha_failed")) {
		t.Fatalf("sin captcha no se entra: status=%d", rr.Code)
	}
	reto := l.get(t, "/login/captcha")
	var ch altcha.Challenge
	if err := json.NewDecoder(reto.Body).Decode(&ch); err != nil || reto.Code != http.StatusOK {
		t.Fatalf("reto del login: status=%d %v", reto.Code, err)
	}
	sol, err := altcha.SolveChallenge(ch)
	if err != nil {
		t.Fatal(err)
	}
	if rr := l.formulario(t, "/login/", url.Values{"email": {"karla"}, "password": {pw}, "altcha": {sol}}); rr.Code != http.StatusFound {
		t.Fatalf("con el captcha resuelto se entra: status=%d cuerpo=%s", rr.Code, rr.Body)
	}
	l.srv.cfg.LoginCaptchaEnabled = false
	if rr := l.get(t, "/login/captcha"); rr.Code != http.StatusNotFound {
		t.Fatalf("sin captcha en el login no hay reto: status=%d", rr.Code)
	}
}
