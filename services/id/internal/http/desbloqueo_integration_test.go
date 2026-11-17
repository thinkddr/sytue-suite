// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package http

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"html"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/recoverywrap"
	"git.sytue.eu/sytue/one/services/id/internal/domain"
)

// desbloqueoPeticion hace una petición del navegador al IdP desde su propio origen.
func (l *liveServer) desbloqueoPeticion(t *testing.T, method, path string, sess *http.Cookie, body any) *httptest.ResponseRecorder {
	t.Helper()
	raw, _ := json.Marshal(body)
	req := httptest.NewRequest(method, path, bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	if sess != nil {
		req.AddCookie(sess)
	}
	rec := httptest.NewRecorder()
	l.mux.ServeHTTP(rec, req)
	return rec
}

func (l *liveServer) salDe(t *testing.T, email string) recoverywrap.KDF {
	t.Helper()
	rec := l.desbloqueoPeticion(t, http.MethodPost, "/login/unlock/salt", nil, map[string]string{"email": email})
	if rec.Code != http.StatusOK {
		t.Fatalf("sal de %s: %d %s", email, rec.Code, rec.Body)
	}
	var out struct {
		KDF recoverywrap.KDF `json:"kdf"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	return out.KDF
}

// sobreDe cierra en Go lo que en producción cierra el navegador.
func sobreDe(t *testing.T, kind, userID, cred string, kdf recoverywrap.KDF, password string, prf, key []byte) recoverywrap.Envelope {
	t.Helper()
	var kek []byte
	var err error
	if kind == recoverywrap.KindPassword {
		kek, err = recoverywrap.EnvelopePasswordKEK(password, kdf)
	} else {
		kek, err = recoverywrap.EnvelopePasskeyKEK(prf)
	}
	if err != nil {
		t.Fatal(err)
	}
	e, err := recoverywrap.SealEnvelope(kek, kind, userID, cred, kdf, key, nil)
	if err != nil {
		t.Fatal(err)
	}
	return e
}

// La sal sale antes de comprobar la contraseña, así que no puede decir si la
// cuenta existe: la de una dirección sin cuenta es igual de estable y tiene la
// misma forma. Con sobre, es la de su sobre.
func TestLaSalDelDesbloqueoNoDiceSiLaCuentaExiste(t *testing.T) {
	l := newLiveServer(t)
	ana := l.mkUser(t, "ana@desbloqueo.test", "contraseña-de-ana-1")
	k1, k2 := l.salDe(t, ana.Email), l.salDe(t, ana.Email)
	n1, n2 := l.salDe(t, "nadie@desbloqueo.test"), l.salDe(t, "nadie@desbloqueo.test")
	if !bytes.Equal(k1.Salt, k2.Salt) || !bytes.Equal(n1.Salt, n2.Salt) || bytes.Equal(k1.Salt, n1.Salt) {
		t.Fatalf("sales: ana %x/%x, nadie %x/%x", k1.Salt, k2.Salt, n1.Salt, n2.Salt)
	}
	if k1.Alg != n1.Alg || k1.Memory != n1.Memory || len(k1.Salt) != 16 || len(n1.Salt) != 16 {
		t.Fatalf("formas distintas: %+v / %+v", k1, n1)
	}
	// Sin el mismo origen no hay sal.
	req := httptest.NewRequest(http.MethodPost, "/login/unlock/salt", strings.NewReader(`{"email":"ana@desbloqueo.test"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Sec-Fetch-Site", "cross-site")
	rec := httptest.NewRecorder()
	l.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("sal desde otro sitio: %d", rec.Code)
	}
}

// Los sobres solo los guarda su persona, desde este origen, y todos con la misma
// huella; «empezar de cero» es lo único que la cambia. Lo guardado no lleva la
// clave de desbloqueo.
func TestLosSobresSoloLosGuardaSuPersonaYConUnaSolaHuella(t *testing.T) {
	l := newLiveServer(t)
	org, err := l.st.CreateOrg(l.ctx, nil, domain.Organization{Slug: "acme", Name: "Acme"})
	if err != nil {
		t.Fatal(err)
	}
	sess := l.memberSession(t, "ana@acme.example", org.ID)
	stored, err := l.st.GetSessionByToken(l.ctx, domain.HashToken(sess.Value))
	if err != nil {
		t.Fatal(err)
	}
	userID := stored.UserID
	kdf := l.salDe(t, "ana@acme.example")
	key, _ := recoverywrap.NewUnlockKey()
	sobre := sobreDe(t, recoverywrap.KindPassword, userID, "", kdf, "contraseña", nil, key)

	if rec := l.desbloqueoPeticion(t, http.MethodPut, "/e2e/envelopes", nil, map[string]any{"envelope": sobre}); rec.Code != http.StatusUnauthorized {
		t.Fatalf("sin sesión: %d", rec.Code)
	}
	// Un sobre cerrado en el navegador para OTRA persona (su AAD) no sustituye al suyo.
	if rec := l.desbloqueoPeticion(t, http.MethodPut, "/e2e/envelopes", sess, map[string]any{"envelope": sobre, "sub": "otra-persona"}); rec.Code != http.StatusForbidden {
		t.Fatalf("sobre de otra persona: %d", rec.Code)
	}
	if rec := l.desbloqueoPeticion(t, http.MethodPut, "/e2e/envelopes", sess, map[string]any{"envelope": sobre, "sub": userID}); rec.Code != http.StatusNoContent {
		t.Fatalf("guardar: %d %s", rec.Code, rec.Body)
	}
	if got := l.salDe(t, "ana@acme.example"); !bytes.Equal(got.Salt, kdf.Salt) {
		t.Fatal("la sal cambió al guardar el sobre")
	}
	rows, err := l.st.UnlockEnvelopes(l.ctx, userID)
	if err != nil || len(rows) != 1 {
		t.Fatalf("filas: %d, %v", len(rows), err)
	}
	if bytes.Contains(rows[0].Envelope, []byte(base64.StdEncoding.EncodeToString(key))) || bytes.Contains(rows[0].Envelope, []byte("contraseña")) {
		t.Fatalf("el sobre guardado lleva un secreto: %s", rows[0].Envelope)
	}

	// Una passkey que no es suya no tiene sobre.
	ajena := base64.RawURLEncoding.EncodeToString([]byte("credencial-ajena"))
	pk := sobreDe(t, recoverywrap.KindPasskey, userID, ajena, recoverywrap.KDF{Alg: recoverywrap.AlgPasskeyPRF, PRFSalt: recoverywrap.PasskeyPRFInput}, "", bytes.Repeat([]byte{7}, 32), key)
	if rec := l.desbloqueoPeticion(t, http.MethodPut, "/e2e/envelopes", sess, map[string]any{"envelope": pk}); rec.Code != http.StatusBadRequest {
		t.Fatalf("passkey ajena: %d", rec.Code)
	}
	// La suya, sí; y al borrarla se va su sobre.
	if err := l.st.InsertCredential(l.ctx, nil, domain.WebAuthnCredential{UserID: userID, CredID: []byte("credencial-de-ana"), PublicKey: []byte{1}, Name: "Portátil"}); err != nil {
		t.Fatal(err)
	}
	suya := base64.RawURLEncoding.EncodeToString([]byte("credencial-de-ana"))
	pk = sobreDe(t, recoverywrap.KindPasskey, userID, suya, recoverywrap.KDF{Alg: recoverywrap.AlgPasskeyPRF, PRFSalt: recoverywrap.PasskeyPRFInput}, "", bytes.Repeat([]byte{7}, 32), key)
	if rec := l.desbloqueoPeticion(t, http.MethodPut, "/e2e/envelopes", sess, map[string]any{"envelope": pk}); rec.Code != http.StatusNoContent {
		t.Fatalf("passkey suya: %d %s", rec.Code, rec.Body)
	}

	// Otra clave de desbloqueo (otra huella) no entra sin empezar de cero.
	otra, _ := recoverywrap.NewUnlockKey()
	nuevo := sobreDe(t, recoverywrap.KindPassword, userID, "", kdf, "contraseña", nil, otra)
	if rec := l.desbloqueoPeticion(t, http.MethodPut, "/e2e/envelopes", sess, map[string]any{"envelope": nuevo}); rec.Code != http.StatusConflict {
		t.Fatalf("otra huella: %d", rec.Code)
	}
	if rec := l.desbloqueoPeticion(t, http.MethodPut, "/e2e/envelopes", sess, map[string]any{"envelope": nuevo, "reset": true}); rec.Code != http.StatusNoContent {
		t.Fatalf("empezar de cero: %d", rec.Code)
	}
	if rows, _ := l.st.UnlockEnvelopes(l.ctx, userID); len(rows) != 1 || rows[0].Kind != recoverywrap.KindPassword {
		t.Fatalf("tras empezar de cero quedan %d sobres", len(rows))
	}
	if rec := l.desbloqueoPeticion(t, http.MethodPut, "/e2e/envelopes", sess, map[string]any{"envelope": pk}); rec.Code != http.StatusConflict {
		t.Fatalf("el sobre de la clave vieja vuelve a entrar: %d", rec.Code)
	}
	pk = sobreDe(t, recoverywrap.KindPasskey, userID, suya, recoverywrap.KDF{Alg: recoverywrap.AlgPasskeyPRF, PRFSalt: recoverywrap.PasskeyPRFInput}, "", bytes.Repeat([]byte{7}, 32), otra)
	if rec := l.desbloqueoPeticion(t, http.MethodPut, "/e2e/envelopes", sess, map[string]any{"envelope": pk}); rec.Code != http.StatusNoContent {
		t.Fatalf("passkey con la clave nueva: %d", rec.Code)
	}
	creds, _ := l.st.ListCredentialsForUser(l.ctx, userID)
	if borrada, err := l.st.DeleteCredentialForUser(l.ctx, nil, userID, creds[0].ID); err != nil || !borrada {
		t.Fatalf("borrar passkey: %v %v", borrada, err)
	}
	if rows, _ := l.st.UnlockEnvelopes(l.ctx, userID); len(rows) != 1 {
		t.Fatalf("la passkey borrada dejó su sobre: %d sobres", len(rows))
	}

	// Desde otro sitio, nada.
	req := httptest.NewRequest(http.MethodPut, "/e2e/envelopes", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Sec-Fetch-Site", "same-site")
	req.RemoteAddr = "198.51.100.7:1234" // otro cubo del limitador: aquí se prueba el origen
	req.AddCookie(sess)
	rec := httptest.NewRecorder()
	l.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("desde un subdominio de la suite: %d", rec.Code)
	}
}

var datosDelRelevo = regexp.MustCompile(`<script type="application/json" id="e2e-relay">(.*?)</script>`)

// La autorización acaba en el relevo SOLO si la app lo pide; los datos del relevo
// son públicos (sobres, huella, sal, la vuelta con el código) y la app nativa
// recibe su clave pública de vuelta para la entrega.
func TestLaAutorizacionPintaElRelevoSoloSiSeLoPiden(t *testing.T) {
	l := newLiveServer(t)
	org, err := l.st.CreateOrg(l.ctx, nil, domain.Organization{Slug: "acme", Name: "Acme"})
	if err != nil {
		t.Fatal(err)
	}
	sess := l.memberSession(t, "ana@acme.example", org.ID)
	stored, _ := l.st.GetSessionByToken(l.ctx, domain.HashToken(sess.Value))
	kdf := l.salDe(t, "ana@acme.example")
	key, _ := recoverywrap.NewUnlockKey()
	sobre := sobreDe(t, recoverywrap.KindPassword, stored.UserID, "", kdf, "contraseña", nil, key)
	if rec := l.desbloqueoPeticion(t, http.MethodPut, "/e2e/envelopes", sess, map[string]any{"envelope": sobre}); rec.Code != http.StatusNoContent {
		t.Fatalf("guardar: %d", rec.Code)
	}
	web := func(extra url.Values) *httptest.ResponseRecorder {
		sum := sha256.Sum256([]byte("verificador-de-prueba-con-longitud-suficiente"))
		q := url.Values{
			"client_id": {"sytue-contacts"}, "redirect_uri": {"https://contacts.sytue.eu/contacts/auth/callback"},
			"response_type": {"code"}, "scope": {"openid"}, "state": {"st"},
			"code_challenge": {base64.RawURLEncoding.EncodeToString(sum[:])}, "code_challenge_method": {"S256"},
		}
		for k, v := range extra {
			q[k] = v
		}
		req := httptest.NewRequest(http.MethodGet, "/oauth2/authorize?"+q.Encode(), http.NoBody)
		req.AddCookie(sess)
		rec := httptest.NewRecorder()
		l.mux.ServeHTTP(rec, req)
		return rec
	}
	if rec := web(nil); rec.Code != http.StatusFound {
		t.Fatalf("sin pedirlo: %d; quiero la redirección de siempre", rec.Code)
	}
	rec := web(url.Values{"sytue_unlock": {"1"}})
	m := datosDelRelevo.FindStringSubmatch(rec.Body.String())
	if rec.Code != http.StatusOK || m == nil {
		t.Fatalf("relevo: %d", rec.Code)
	}
	var datos relevoDatos
	if err := json.Unmarshal([]byte(m[1]), &datos); err != nil {
		t.Fatal(err)
	}
	next, _ := url.Parse(datos.Next)
	if datos.Sub != stored.UserID || next.Host != "contacts.sytue.eu" || next.Query().Get("code") == "" || next.Query().Get("state") != "st" ||
		len(datos.Envelopes) != 1 || datos.AppKey != "" || datos.Salt != base64.StdEncoding.EncodeToString(kdf.Salt) {
		t.Fatalf("datos del relevo: %+v", datos)
	}
	hint, _ := recoverywrap.Hint(key)
	if datos.Hint != base64.StdEncoding.EncodeToString(hint) || strings.Contains(m[1], base64.StdEncoding.EncodeToString(key)) {
		t.Fatalf("huella %q / lleva la clave", datos.Hint)
	}
	if !strings.Contains(rec.Header().Get("Content-Security-Policy"), "frame-ancestors 'none'") {
		t.Fatal("el relevo se deja incrustar")
	}

	// La app nativa: tras el consentimiento de su primera vez, el relevo con su clave.
	priv, _ := recoverywrap.NewDeliveryKey()
	pub := recoverywrap.DeliveryPublic(priv)
	escritorio := nativa{client: "sytue-desktop", vuelta: "http://127.0.0.1:8921/auth/callback", secreto: "secreto-del-portatil-de-ana-32-bytes!"}
	sum := sha256.Sum256([]byte("verificador-nativo-con-longitud-suficiente"))
	q := url.Values{
		"client_id": {escritorio.client}, "redirect_uri": {escritorio.vuelta}, "response_type": {"code"},
		"scope": {"openid offline_access"}, "state": {"st"}, paramDispositivo: {escritorio.hash()},
		"code_challenge": {base64.RawURLEncoding.EncodeToString(sum[:])}, "code_challenge_method": {"S256"},
		"sytue_unlock_key": {pub},
	}
	rec, _ = l.pedirAutorizacion(t, escritorio, sess, "/oauth2/authorize?"+q.Encode())
	enlace := enlacePermitir.FindStringSubmatch(rec.Body.String())
	if enlace == nil {
		t.Fatalf("sin consentimiento: %d", rec.Code)
	}
	rec, _ = l.pedirAutorizacion(t, escritorio, sess, html.UnescapeString(enlace[1]))
	m = datosDelRelevo.FindStringSubmatch(rec.Body.String())
	if rec.Code != http.StatusOK || m == nil {
		t.Fatalf("relevo nativo: %d %s", rec.Code, rec.Header().Get("Location"))
	}
	if err := json.Unmarshal([]byte(m[1]), &datos); err != nil || datos.AppKey != pub || !strings.HasPrefix(datos.Next, escritorio.vuelta+"?") {
		t.Fatalf("relevo nativo: %+v (%v)", datos, err)
	}
	// Una clave pública rota no pinta el relevo: la redirección de siempre.
	q.Set("sytue_unlock_key", "no-es-una-clave")
	rec, _ = l.pedirAutorizacion(t, escritorio, sess, "/oauth2/authorize?"+q.Encode())
	if rec.Code == http.StatusOK && datosDelRelevo.MatchString(rec.Body.String()) {
		t.Fatal("una clave rota pinta el relevo")
	}
}

// `prompt=login` manda a identificarse aunque haya sesión, y la vuelta no lo
// repite (o el inicio de sesión no acabaría nunca).
func TestPromptLoginObligaAEntrarSinBucle(t *testing.T) {
	l := newLiveServer(t)
	org, err := l.st.CreateOrg(l.ctx, nil, domain.Organization{Slug: "acme", Name: "Acme"})
	if err != nil {
		t.Fatal(err)
	}
	sess := l.memberSession(t, "ana@acme.example", org.ID)
	sum := sha256.Sum256([]byte("verificador-de-prueba-con-longitud-suficiente"))
	q := url.Values{
		"client_id": {"sytue-contacts"}, "redirect_uri": {"https://contacts.sytue.eu/contacts/auth/callback"},
		"response_type": {"code"}, "scope": {"openid"}, "state": {"st"}, "prompt": {"login"},
		"code_challenge": {base64.RawURLEncoding.EncodeToString(sum[:])}, "code_challenge_method": {"S256"},
	}
	req := httptest.NewRequest(http.MethodGet, "/oauth2/authorize?"+q.Encode(), http.NoBody)
	req.AddCookie(sess)
	rec := httptest.NewRecorder()
	l.mux.ServeHTTP(rec, req)
	loc, _ := url.Parse(rec.Header().Get("Location"))
	if rec.Code != http.StatusFound || loc.Path != "/login/" {
		t.Fatalf("prompt=login con sesión: %d %s", rec.Code, loc)
	}
	vuelta := loc.Query().Get("return_to")
	if strings.Contains(vuelta, "prompt") || !strings.Contains(vuelta, "client_id=sytue-contacts") {
		t.Fatalf("vuelta %q", vuelta)
	}
	req = httptest.NewRequest(http.MethodGet, vuelta, http.NoBody)
	req.AddCookie(sess)
	rec = httptest.NewRecorder()
	l.mux.ServeHTTP(rec, req)
	if loc, _ := url.Parse(rec.Header().Get("Location")); rec.Code != http.StatusFound || loc.Host != "contacts.sytue.eu" {
		t.Fatalf("la vuelta vuelve a pedir el inicio de sesión: %d %s", rec.Code, rec.Header().Get("Location"))
	}
}

// El sobre de la contraseña nueva viaja con el cambio (ADR 0085) y Sytue ID lo
// guarda SOLO si el cambio sale bien: con la contraseña actual equivocada, el sobre
// vigente se queda como estaba.
func TestElSobreDeLaContrasenaNuevaLlegaConElCambio(t *testing.T) {
	l := newLiveServer(t)
	org, err := l.st.CreateOrg(l.ctx, nil, domain.Organization{Slug: "acme", Name: "Acme"})
	if err != nil {
		t.Fatal(err)
	}
	const antigua, nueva = "Antigua-contrasena-1", "Nueva-contrasena-segura-2"
	u := l.mkUser(t, "bea@acme.example", antigua)
	if err := l.st.UpsertMembership(l.ctx, nil, domain.Membership{OrgID: org.ID, UserID: u.ID, Role: domain.RoleMember}); err != nil {
		t.Fatal(err)
	}
	raw, _ := domain.GenerateToken(32)
	now := time.Now()
	if _, err := l.st.InsertSession(l.ctx, nil, domain.Session{
		TokenHash: domain.HashToken(raw), UserID: u.ID, OrgID: org.ID,
		AuthTime: now, AMR: []string{"pwd"}, UserAgent: "test", ExpiresAt: now.Add(time.Hour),
	}); err != nil {
		t.Fatal(err)
	}
	sess := &http.Cookie{Name: sessionCookieName, Value: raw}
	kdf := l.salDe(t, "bea@acme.example")
	key, _ := recoverywrap.NewUnlockKey()
	if rec := l.desbloqueoPeticion(t, http.MethodPut, "/e2e/envelopes", sess, map[string]any{"envelope": sobreDe(t, recoverywrap.KindPassword, u.ID, "", kdf, antigua, nil, key)}); rec.Code != http.StatusNoContent {
		t.Fatalf("sobre inicial: %d", rec.Code)
	}
	nuevo, _ := json.Marshal(sobreDe(t, recoverywrap.KindPassword, u.ID, "", kdf, nueva, nil, key))
	cambiar := func(actual string) {
		form := url.Values{"current_password": {actual}, "new_password": {nueva}, "confirm_password": {nueva}, "unlock_envelope": {string(nuevo)}}
		req := httptest.NewRequest(http.MethodPost, "/account/password", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Sec-Fetch-Site", "same-origin")
		req.AddCookie(sess)
		l.mux.ServeHTTP(httptest.NewRecorder(), req)
	}
	abre := func(password string) bool {
		rows, err := l.st.UnlockEnvelopes(l.ctx, u.ID)
		if err != nil || len(rows) != 1 {
			t.Fatalf("sobres: %d %v", len(rows), err)
		}
		var e recoverywrap.Envelope
		if err := json.Unmarshal(rows[0].Envelope, &e); err != nil {
			t.Fatal(err)
		}
		kek, err := recoverywrap.EnvelopePasswordKEK(password, e.KDF)
		if err != nil {
			t.Fatal(err)
		}
		_, err = recoverywrap.OpenEnvelope(kek, u.ID, e)
		return err == nil
	}
	cambiar("no-es-la-actual")
	if !abre(antigua) || abre(nueva) {
		t.Fatal("un cambio que no sale no puede tocar el sobre")
	}
	cambiar(antigua)
	if !abre(nueva) || abre(antigua) {
		t.Fatal("tras el cambio, el sobre debe abrir con la contraseña nueva")
	}
}

// Subir sobres tiene su propio límite por cliente (ADR 0085): da para el uso real
// (un sobre por inicio de sesión, uno por passkey, empezar de cero) sin gastar el
// presupuesto anti-fuerza-bruta del inicio de sesión, y sigue cortando a quien con
// una sesión machaque PUT /e2e/envelopes.
func TestLosSobresTienenSuPropioLimiteYCortanElAbuso(t *testing.T) {
	l := newLiveServer(t)
	org, err := l.st.CreateOrg(l.ctx, nil, domain.Organization{Slug: "acme", Name: "Acme"})
	if err != nil {
		t.Fatal(err)
	}
	sess := l.memberSession(t, "ana@acme.example", org.ID)
	stored, err := l.st.GetSessionByToken(l.ctx, domain.HashToken(sess.Value))
	if err != nil {
		t.Fatal(err)
	}
	kdf := l.salDe(t, "ana@acme.example")
	key, _ := recoverywrap.NewUnlockKey()
	sobre := map[string]any{"envelope": sobreDe(t, recoverywrap.KindPassword, stored.UserID, "", kdf, "contraseña", nil, key)}

	// El uso real de un rato: más que la ráfaga del inicio de sesión (10), sin 429.
	for i := range 12 {
		if rec := l.desbloqueoPeticion(t, http.MethodPut, "/e2e/envelopes", sess, sobre); rec.Code != http.StatusNoContent {
			t.Fatalf("sobre %d: %d", i+1, rec.Code)
		}
	}
	// Y el inicio de sesión de ese mismo cliente no lo ha pagado.
	l.salDe(t, "ana@acme.example")

	// Machacarlo, sí se corta.
	cortado := false
	for range 40 {
		if rec := l.desbloqueoPeticion(t, http.MethodPut, "/e2e/envelopes", sess, sobre); rec.Code == http.StatusTooManyRequests {
			cortado = true
			break
		}
	}
	if !cortado {
		t.Fatal("cuarenta sobres seguidos de una sesión no se cortan con 429")
	}
}
