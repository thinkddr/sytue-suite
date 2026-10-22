// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

// Tests de integración de los dos flujos de autenticación nuevos contra un
// PostgreSQL real (pgtest, build-tag `integration`): la ceremonia de LOGIN por
// passkey (WebAuthn assertion, begin/finish) de extremo a extremo con una firma
// ES256 real, y la recuperación de contraseña (respuesta uniforme del forgot +
// fijado con token de un solo uso, caducado y reusado). Reusa el harness
// newLiveServer de security_integration_test.go.
package http

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/services/id/internal/domain"
)

// --- helpers de petición sin token (formularios / JSON públicos) ---

func (l *liveServer) postJSON(t *testing.T, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	b, _ := json.Marshal(body)
	r := httptest.NewRequest(http.MethodPost, path, strings.NewReader(string(b)))
	r.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	l.mux.ServeHTTP(rr, r)
	return rr
}

func (l *liveServer) postForm(t *testing.T, path string, form url.Values) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(http.MethodPost, path, strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	l.mux.ServeHTTP(rr, r)
	return rr
}

func (l *liveServer) get(t *testing.T, path string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(http.MethodGet, path, http.NoBody)
	rr := httptest.NewRecorder()
	l.mux.ServeHTTP(rr, r)
	return rr
}

// --- constructores CBOR/COSE mínimos para la assertion (RFC 8949/9052) ---

func cborUintH(k uint64) []byte {
	switch {
	case k < 24:
		return []byte{byte(k)}
	case k < 256:
		return []byte{0x18, byte(k)}
	default:
		b := []byte{0x19, 0, 0}
		binary.BigEndian.PutUint16(b[1:], uint16(k))
		return b
	}
}

func cborNegH(v int64) []byte {
	h := cborUintH(uint64(-1 - v))
	h[0] |= 0x20
	return h
}

func cborBytesH(b []byte) []byte {
	h := cborUintH(uint64(len(b)))
	h[0] |= 0x40
	return append(h, b...)
}

func coseKeyES256(pub *ecdsa.PublicKey) []byte {
	x := pub.X.FillBytes(make([]byte, 32))
	y := pub.Y.FillBytes(make([]byte, 32))
	var m []byte
	m = append(m, 0xA5)
	m = append(m, cborUintH(1)...)
	m = append(m, cborUintH(2)...) // kty EC2
	m = append(m, cborUintH(3)...)
	m = append(m, cborNegH(-7)...) // alg ES256
	m = append(m, cborNegH(-1)...)
	m = append(m, cborUintH(1)...) // crv P-256
	m = append(m, cborNegH(-2)...)
	m = append(m, cborBytesH(x)...)
	m = append(m, cborNegH(-3)...)
	m = append(m, cborBytesH(y)...)
	return m
}

func buildAuthData(rpID string, flags byte, count uint32) []byte {
	h := sha256.Sum256([]byte(rpID))
	b := make([]byte, 37)
	copy(b[0:32], h[:])
	b[32] = flags
	binary.BigEndian.PutUint32(b[33:37], count)
	return b
}

func signAssertion(t *testing.T, priv *ecdsa.PrivateKey, ad, cdj []byte) []byte {
	t.Helper()
	cdHash := sha256.Sum256(cdj)
	digest := sha256.Sum256(append(append([]byte{}, ad...), cdHash[:]...))
	sig, err := ecdsa.SignASN1(rand.Reader, priv, digest[:])
	if err != nil {
		t.Fatalf("SignASN1: %v", err)
	}
	return sig
}

func sessionCookieSet(rr *httptest.ResponseRecorder) bool {
	for _, c := range rr.Result().Cookies() {
		if c.Name == sessionCookieName && c.Value != "" {
			return true
		}
	}
	return false
}

func TestWebAuthnLogin_EndToEnd(t *testing.T) {
	l := newLiveServer(t)
	u := l.mkUser(t, "passkey@id.test", "")

	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	credID := []byte("credential-id-0001")
	if err := l.st.InsertCredential(l.ctx, nil, domain.WebAuthnCredential{
		UserID: u.ID, CredID: credID, PublicKey: coseKeyES256(&priv.PublicKey),
		SignCount: 0, Transports: []string{}, Name: "Passkey de prueba",
	}); err != nil {
		t.Fatalf("InsertCredential: %v", err)
	}

	b64 := base64.RawURLEncoding.EncodeToString

	// begin (usuario-primero): devuelve un reto y allowCredentials con nuestra passkey.
	rr := l.postJSON(t, "/login/webauthn/begin", map[string]any{"email": "passkey@id.test"})
	if rr.Code != http.StatusOK {
		t.Fatalf("begin: status=%d cuerpo=%s", rr.Code, rr.Body)
	}
	var opts struct {
		Challenge        string              `json:"challenge"`
		RPID             string              `json:"rpId"`
		AllowCredentials []map[string]string `json:"allowCredentials"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &opts); err != nil {
		t.Fatal(err)
	}
	if len(opts.AllowCredentials) != 1 || opts.AllowCredentials[0]["id"] != b64(credID) {
		t.Fatalf("allowCredentials = %v, quiero la passkey registrada", opts.AllowCredentials)
	}

	// Construir la assertion firmada (UP+UV, count=1) sobre el reto emitido.
	ad := buildAuthData(l.srv.cfg.RPID, 0x05, 1)
	cdj := []byte(`{"type":"webauthn.get","challenge":"` + opts.Challenge + `","origin":"` + itIssuer + `"}`)
	sig := signAssertion(t, priv, ad, cdj)
	finishBody := map[string]any{
		"id": b64(credID), "rawId": b64(credID), "type": "public-key",
		"response": map[string]any{
			"authenticatorData": b64(ad), "clientDataJSON": b64(cdj),
			"signature": b64(sig), "userHandle": b64([]byte(u.ID)),
		},
		"return_to": "/account",
	}

	rr = l.postJSON(t, "/login/webauthn/finish", finishBody)
	if rr.Code != http.StatusOK {
		t.Fatalf("finish: status=%d cuerpo=%s", rr.Code, rr.Body)
	}
	if !sessionCookieSet(rr) {
		t.Fatal("finish: no se estableció la cookie de sesión SSO")
	}
	var fin struct {
		Redirect string `json:"redirect"`
	}
	_ = json.Unmarshal(rr.Body.Bytes(), &fin)
	if fin.Redirect != "/account" {
		t.Fatalf("redirect = %q, quiero /account", fin.Redirect)
	}
	// signCount persistido (anti-clonación): 0 → 1.
	cred, err := l.st.GetCredentialByID(l.ctx, credID)
	if err != nil || cred.SignCount != 1 {
		t.Fatalf("signCount tras login = %d (err=%v), quiero 1", cred.SignCount, err)
	}

	// Replay del MISMO reto (ya consumido) → 401.
	rr = l.postJSON(t, "/login/webauthn/finish", finishBody)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("replay del reto: status=%d, quiero 401", rr.Code)
	}

	// Anti-clonación de extremo a extremo: nuevo reto, pero signCount que NO avanza
	// (1, igual al almacenado) → rechazo.
	rr = l.postJSON(t, "/login/webauthn/begin", map[string]any{"email": "passkey@id.test"})
	_ = json.Unmarshal(rr.Body.Bytes(), &opts)
	ad2 := buildAuthData(l.srv.cfg.RPID, 0x05, 1) // count 1 == almacenado
	cdj2 := []byte(`{"type":"webauthn.get","challenge":"` + opts.Challenge + `","origin":"` + itIssuer + `"}`)
	sig2 := signAssertion(t, priv, ad2, cdj2)
	rr = l.postJSON(t, "/login/webauthn/finish", map[string]any{
		"id": b64(credID), "rawId": b64(credID), "type": "public-key",
		"response": map[string]any{
			"authenticatorData": b64(ad2), "clientDataJSON": b64(cdj2),
			"signature": b64(sig2), "userHandle": b64([]byte(u.ID)),
		},
	})
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("signCount estancado: status=%d, quiero 401", rr.Code)
	}
}

func TestForgotPassword_UniformResponse(t *testing.T) {
	l := newLiveServer(t)
	l.mkUser(t, "exists@id.test", "some-password-123")

	rrHit := l.postForm(t, "/forgot-password", url.Values{"email": {"exists@id.test"}})
	rrMiss := l.postForm(t, "/forgot-password", url.Values{"email": {"nobody@id.test"}})

	// Respuesta idéntica exista o no la cuenta (no enumera).
	if rrHit.Code != http.StatusSeeOther || rrMiss.Code != http.StatusSeeOther {
		t.Fatalf("status: hit=%d miss=%d, quiero 303 ambos", rrHit.Code, rrMiss.Code)
	}
	if loc := rrHit.Header().Get("Location"); loc != rrMiss.Header().Get("Location") || loc != "/forgot-password?sent=1" {
		t.Fatalf("Location no uniforme: hit=%q miss=%q", loc, rrMiss.Header().Get("Location"))
	}

	// El envío es asíncrono, así que hay que esperar al efecto: SOLO la cuenta
	// existente obtiene un token (no enumeración también en los datos).
	//
	// Se SONDEA en vez de dormir un rato fijo. Con `time.Sleep(300ms)` el test
	// pasaba en un portátil ocioso y caía en el CI —runner compartido, más lento—
	// contando 0 tokens: un rojo que no señalaba ningún defecto del producto y
	// que solo se reproducía cuando la máquina iba cargada. El tope de 5 s no
	// esconde nada: si el token no llega nunca, el test sigue fallando, y con el
	// mismo mensaje.
	var n int
	plazo := time.Now().Add(5 * time.Second)
	for {
		if err := l.st.DB().QueryRow(l.ctx, `SELECT count(*) FROM password_reset_tokens`).Scan(&n); err != nil {
			t.Fatalf("contar tokens: %v", err)
		}
		if n >= 1 || time.Now().After(plazo) {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if n != 1 {
		t.Fatalf("tokens de reseteo = %d, quiero 1 (solo la cuenta existente)", n)
	}
}

func TestResetPassword_ValidExpiredReused(t *testing.T) {
	l := newLiveServer(t)
	u := l.mkUser(t, "reset@id.test", "old-password-123")

	// Sesión viva para comprobar que el reseteo la revoca.
	now := time.Now()
	if _, err := l.st.InsertSession(l.ctx, nil, domain.Session{
		TokenHash: domain.HashToken("live-sess"), UserID: u.ID, AuthTime: now,
		AMR: []string{"pwd"}, UserAgent: "test", ExpiresAt: now.Add(time.Hour),
	}); err != nil {
		t.Fatalf("InsertSession: %v", err)
	}

	// Emitir un token válido (misma clave HMAC que el servidor) y persistir su hash.
	token, hash, err := domain.NewResetToken(l.srv.resetKey)
	if err != nil {
		t.Fatal(err)
	}
	if err := l.st.InTx(l.ctx, func(q pg.Querier) error {
		return l.st.InsertResetToken(l.ctx, q, hash, u.ID, now.Add(30*time.Minute))
	}); err != nil {
		t.Fatalf("InsertResetToken: %v", err)
	}

	// GET del formulario con token válido → 200.
	if rr := l.get(t, "/reset-password?token="+url.QueryEscape(token)); rr.Code != http.StatusOK {
		t.Fatalf("GET reset con token válido: status=%d", rr.Code)
	}
	// GET con token manipulado → 400. Se AÑADE un carácter en vez de sustituir el
	// último por una «X»: si el token acababa en X, el «manipulado» era idéntico al
	// válido y el test fallaba ~1 de cada 64 ejecuciones. Un test que pasa el 98 % de
	// las veces es peor que uno que falla: enseña a ignorar el rojo.
	if rr := l.get(t, "/reset-password?token="+url.QueryEscape(token+"X")); rr.Code != http.StatusBadRequest {
		t.Fatalf("GET reset con token manipulado: status=%d, quiero 400", rr.Code)
	}

	// POST: fijar la nueva contraseña → 200, contraseña cambiada, sesión revocada.
	rr := l.postForm(t, "/reset-password", url.Values{
		"token": {token}, "new_password": {"brand-new-secret-1"}, "confirm_password": {"brand-new-secret-1"},
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("POST reset válido: status=%d cuerpo=%s", rr.Code, rr.Body)
	}
	u2, _ := l.st.GetUserByID(l.ctx, u.ID)
	if ok, _ := domain.VerifyPassword("brand-new-secret-1", u2.PasswordHash); !ok {
		t.Fatal("la contraseña no se actualizó al hash nuevo")
	}
	if ss, _ := l.st.GetSessionByToken(l.ctx, domain.HashToken("live-sess")); ss.Valid(time.Now()) {
		t.Fatal("la sesión viva debería haberse revocado tras el reseteo")
	}

	// Reuso del MISMO token (ya consumido) → 400 (un solo uso).
	rr = l.postForm(t, "/reset-password", url.Values{
		"token": {token}, "new_password": {"another-secret-9"}, "confirm_password": {"another-secret-9"},
	})
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("POST reset reusado: status=%d, quiero 400", rr.Code)
	}

	// Token CADUCADO → 400.
	expToken, expHash, _ := domain.NewResetToken(l.srv.resetKey)
	if err := l.st.InTx(l.ctx, func(q pg.Querier) error {
		return l.st.InsertResetToken(l.ctx, q, expHash, u.ID, now.Add(-time.Minute))
	}); err != nil {
		t.Fatal(err)
	}
	rr = l.postForm(t, "/reset-password", url.Values{
		"token": {expToken}, "new_password": {"third-secret-77"}, "confirm_password": {"third-secret-77"},
	})
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("POST reset caducado: status=%d, quiero 400", rr.Code)
	}
}
