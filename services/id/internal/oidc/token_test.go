// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package oidc

import (
	"testing"
	"time"
)

func TestAccessAudience(t *testing.T) {
	cases := []struct {
		resource string
		client   string
		want     []string
	}{
		// Resource + client distintos: ambos presentes (recurso primero).
		{"https://id.sytue.eu", "sytue-one", []string{"https://id.sytue.eu", "sytue-one"}},
		// client == resource: no se duplica.
		{"https://id.sytue.eu", "https://id.sytue.eu", []string{"https://id.sytue.eu"}},
		// client vacío (p. ej. no aplica): solo el recurso.
		{"https://id.sytue.eu", "", []string{"https://id.sytue.eu"}},
	}
	for _, c := range cases {
		got := accessAudience(c.resource, c.client)
		if len(got) != len(c.want) {
			t.Fatalf("accessAudience(%q,%q) = %v, quería %v", c.resource, c.client, got, c.want)
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Fatalf("accessAudience(%q,%q)[%d] = %q, quería %q", c.resource, c.client, i, got[i], c.want[i])
			}
		}
	}
}

// TestIssueAccessToken_IncludesResourceAudience verifica que el access token
// emitido lleva el identificador del resource server en `aud`, de modo que el
// resource server pueda exigir su audiencia (F6, RFC 9068 §4).
func TestIssueAccessToken_IncludesResourceAudience(t *testing.T) {
	priv, set := testKeys(t)
	km := &KeyManager{}
	e := keyEntry{kid: "test-kid", private: priv}
	km.keys = []keyEntry{e}
	km.active = &km.keys[0]

	const issuer = "https://id.test"
	const resource = "https://id.test"
	iss := NewIssuer(km, issuer, resource, time.Minute, time.Hour)

	raw, err := iss.IssueAccessToken(TokenClaims{
		Subject:  "user-1",
		Audience: "sytue-one",
		Scope:    "openid",
	}, time.Now())
	if err != nil {
		t.Fatalf("IssueAccessToken: %v", err)
	}

	// El resource server exige su audiencia: debe validar.
	tok, err := Verify(raw, issuer, resource, set)
	if err != nil {
		t.Fatalf("Verify con la audiencia del recurso falló: %v", err)
	}
	aud := tok.Audience()
	if !contains(aud, resource) || !contains(aud, "sytue-one") {
		t.Fatalf("aud = %v; debe incluir el recurso y el client", aud)
	}
}

// TestIssueAccessToken_Groups verifica que el claim `groups` se emite con los
// NOMBRES de grupo (la forma que Sytue Drive matchea contra shares.subject) y que
// se OMITE cuando el sujeto no tiene grupos (no emitir claims vacíos).
func TestIssueAccessToken_Groups(t *testing.T) {
	priv, set := testKeys(t)
	km := &KeyManager{keys: []keyEntry{{kid: "test-kid", private: priv}}}
	km.active = &km.keys[0]
	const issuer = "https://id.test"
	iss := NewIssuer(km, issuer, issuer, time.Minute, time.Hour)

	// Con grupos: el claim aparece como []string de nombres.
	raw, err := iss.IssueAccessToken(TokenClaims{
		Subject: "u1", Audience: "sytue-one", Scope: "openid",
		Groups: []string{"eng", "finanzas"},
	}, time.Now())
	if err != nil {
		t.Fatalf("IssueAccessToken: %v", err)
	}
	tok, err := Verify(raw, issuer, issuer, set)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	got, ok := tok.Get("groups")
	if !ok {
		t.Fatal("falta el claim groups")
	}
	names := toStrings(got)
	if len(names) != 2 || names[0] != "eng" || names[1] != "finanzas" {
		t.Fatalf("groups = %v; quería [eng finanzas]", names)
	}

	// Sin grupos: el claim NO debe estar presente.
	raw2, err := iss.IssueAccessToken(TokenClaims{
		Subject: "u1", Audience: "sytue-one", Scope: "openid",
	}, time.Now())
	if err != nil {
		t.Fatalf("IssueAccessToken(sin grupos): %v", err)
	}
	tok2, err := Verify(raw2, issuer, issuer, set)
	if err != nil {
		t.Fatalf("Verify(2): %v", err)
	}
	if _, ok := tok2.Get("groups"); ok {
		t.Fatal("el claim groups no debe emitirse cuando está vacío")
	}
}

// TestIssueAccessToken_Impersonation cubre la FORMA del token de un canje RFC
// 8693: `sub` del usuario impersonado, `act.sub` del client que lo obtuvo y una
// vida corta. Y el invariante que sostiene todo lo demás: el token resultante se
// reconoce como DELEGADO (ActorOf), que es lo que impide re-canjearlo.
func TestIssueAccessToken_Impersonation(t *testing.T) {
	priv, set := testKeys(t)
	km := &KeyManager{keys: []keyEntry{{kid: "test-kid", private: priv}}}
	km.active = &km.keys[0]
	const issuer = "https://id.test"
	iss := NewIssuer(km, issuer, issuer, 15*time.Minute, time.Hour)

	// Truncado al segundo: `exp` viaja como unix en segundos (RFC 7519 §2), así
	// que sin esto la comparación perdería la fracción por redondeo.
	now := time.Now().Truncate(time.Second)
	raw, err := iss.IssueAccessToken(TokenClaims{
		Subject: "user-1", Audience: "acme-migrator", Scope: "openid drive.write",
		Actor: "acme-migrator", TTL: 5 * time.Minute,
	}, now)
	if err != nil {
		t.Fatalf("IssueAccessToken: %v", err)
	}
	tok, err := Verify(raw, issuer, issuer, set)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}

	// El token ES del usuario: los servicios de la suite escribirán en SU cuenta.
	if tok.Subject() != "user-1" {
		t.Errorf("sub=%q, quiero el usuario impersonado", tok.Subject())
	}
	// Y dice quién lo obtuvo (RFC 8693 §4.1).
	actor, delegated := ActorOf(tok)
	if !delegated || actor != "acme-migrator" {
		t.Errorf("act = (%q,%v), quiero el client que actuó", actor, delegated)
	}
	// TTL corto: 5 minutos, no los 15 configurados.
	if got := tok.Expiration().Sub(now).Round(time.Second); got != 5*time.Minute {
		t.Errorf("vida del token = %v, quiero 5m", got)
	}

	// Un token normal NO lleva `act`: solo así se distingue del delegado.
	plain, err := iss.IssueAccessToken(TokenClaims{
		Subject: "user-1", Audience: "sytue-console", Scope: "openid",
	}, now)
	if err != nil {
		t.Fatalf("IssueAccessToken(normal): %v", err)
	}
	ptok, err := Verify(plain, issuer, issuer, set)
	if err != nil {
		t.Fatalf("Verify(normal): %v", err)
	}
	if _, delegated := ActorOf(ptok); delegated {
		t.Error("un token de emisión normal no debe llevar `act`")
	}
	if got := ptok.Expiration().Sub(now).Round(time.Second); got != 15*time.Minute {
		t.Errorf("vida del token normal = %v, quiero los 15m configurados", got)
	}
}

// TestEffectiveAccessTokenTTL: el TTL pedido solo puede ACORTAR. Pedir más no
// alarga la vida de un token (ni por error de configuración ni a propósito).
func TestEffectiveAccessTokenTTL(t *testing.T) {
	iss := NewIssuer(nil, "https://id.test", "", 15*time.Minute, time.Hour)
	cases := []struct{ want, got time.Duration }{
		{5 * time.Minute, 5 * time.Minute},   // acorta
		{time.Hour, 15 * time.Minute},        // pedir más NO alarga
		{0, 15 * time.Minute},                // sin petición: el configurado
		{-time.Minute, 15 * time.Minute},     // negativo: el configurado
		{15 * time.Minute, 15 * time.Minute}, // igual: el configurado
	}
	for _, tc := range cases {
		if got := iss.EffectiveAccessTokenTTL(tc.want); got != tc.got {
			t.Errorf("EffectiveAccessTokenTTL(%v)=%v, quiero %v", tc.want, got, tc.got)
		}
	}
}

// toStrings normaliza el valor del claim (jwx lo entrega como []any) a []string.
func toStrings(v any) []string {
	switch t := v.(type) {
	case []string:
		return t
	case []any:
		out := make([]string, 0, len(t))
		for _, e := range t {
			if s, ok := e.(string); ok {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

func contains(ss []string, v string) bool {
	for _, s := range ss {
		if s == v {
			return true
		}
	}
	return false
}

// La audiencia de las APIs de Google va DETRÁS del client: quien busca el client en
// el `aud` toma la primera entrada que no es el resource server.
func TestAccessAudienceConExtras(t *testing.T) {
	got := accessAudience("https://id.example", "app-1", "sytue-googleapis", "app-1", "")
	want := []string{"https://id.example", "app-1", "sytue-googleapis"}
	if len(got) != len(want) {
		t.Fatalf("accessAudience = %v, quiero %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("accessAudience = %v, quiero %v", got, want)
		}
	}
}

// TestIssueAccessToken_AuthTime vigila que `auth_time` viaje en el ACCESS token
// y no solo en el ID Token.
//
// POR QUÉ ES UN GATE Y NO UN DETALLE. El step-up de las acciones sensibles
// (`requireStepUp`, veinte rutas de administración) exige DOS cosas: segundo
// factor y haberse identificado hace poco. Las dos las lee del Bearer, que es un
// access token. Con `auth_time` solo en el ID Token, `AuthTime` llegaba siempre
// a cero, «hace poco» era siempre falso y esas veinte rutas quedaban cerradas
// para todo el mundo —incluido un owner con MFA recién identificado—.
//
// El fallo llegó a `main` y a cinco tags antes de verse, porque el CI de
// integración no corre en pull request: apareció como doce tests contestando
// `insufficient_user_authentication`, no como una consola inservible.
func TestIssueAccessToken_AuthTime(t *testing.T) {
	priv, set := testKeys(t)
	km := &KeyManager{}
	km.keys = []keyEntry{{kid: "test-kid", private: priv}}
	km.active = &km.keys[0]

	const issuer = "https://id.test"
	iss := NewIssuer(km, issuer, issuer, time.Minute, time.Hour)

	cuando := time.Now().UTC().Add(-90 * time.Second).Truncate(time.Second)
	raw, err := iss.IssueAccessToken(TokenClaims{
		Subject: "user-1", Audience: "sytue-console", Scope: "openid",
		AuthTime: cuando, AMR: []string{"pwd", "otp"},
	}, time.Now())
	if err != nil {
		t.Fatalf("IssueAccessToken: %v", err)
	}
	tok, err := Verify(raw, issuer, issuer, set)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	v, ok := tok.Get("auth_time")
	if !ok {
		t.Fatal("el access token no lleva `auth_time`: el step-up no puede saber cuándo se identificó la persona, así que rechazará SIEMPRE")
	}
	// Se emite como segundos Unix, igual que en el ID Token.
	var seg int64
	switch n := v.(type) {
	case float64:
		seg = int64(n)
	case int64:
		seg = n
	default:
		t.Fatalf("`auth_time` = %T, se esperaba un número", v)
	}
	if seg != cuando.Unix() {
		t.Fatalf("`auth_time` = %d, quiero %d", seg, cuando.Unix())
	}

	// Y sin AuthTime NO se inventa uno: un token de client_credentials no tiene
	// persona detrás, y poner ahí la hora de emisión abriría el step-up a quien
	// no se ha identificado nunca.
	raw2, err := iss.IssueAccessToken(TokenClaims{
		Subject: "svc", Audience: "sytue-console", Scope: "openid",
	}, time.Now())
	if err != nil {
		t.Fatalf("IssueAccessToken sin AuthTime: %v", err)
	}
	tok2, err := Verify(raw2, issuer, issuer, set)
	if err != nil {
		t.Fatalf("Verify sin AuthTime: %v", err)
	}
	if _, ok := tok2.Get("auth_time"); ok {
		t.Fatal("sin AuthTime no debe emitirse el claim")
	}
}

// El claim `mailboxes` (ADR 0053) va con el scope `email` y NUNCA en un token
// delegado: quien actúa en nombre de otro no tiene por qué saber en qué otras
// organizaciones está.
func TestIssueAccessToken_MailboxesNoEnUnTokenDelegado(t *testing.T) {
	priv, set := testKeys(t)
	km := &KeyManager{keys: []keyEntry{{kid: "test-kid", private: priv}}}
	km.active = &km.keys[0]
	const issuer = "https://id.test"
	iss := NewIssuer(km, issuer, issuer, time.Minute, time.Hour)
	emitir := func(c TokenClaims) bool {
		t.Helper()
		raw, err := iss.IssueAccessToken(c, time.Now())
		if err != nil {
			t.Fatal(err)
		}
		tok, err := Verify(raw, issuer, issuer, set)
		if err != nil {
			t.Fatal(err)
		}
		_, ok := tok.Get("mailboxes")
		return ok
	}
	base := TokenClaims{Subject: "u1", Audience: "sytue-one", Scope: "openid email", Mailboxes: []string{"eva@acme.test"}}
	if !emitir(base) {
		t.Fatal("con scope email y sin act, falta el claim mailboxes")
	}
	delegado := base
	delegado.Actor = "app-x"
	if emitir(delegado) {
		t.Fatal("un token delegado lleva mailboxes")
	}
	sinEmail := base
	sinEmail.Scope = "openid"
	if emitir(sinEmail) {
		t.Fatal("sin scope email lleva mailboxes")
	}
}
