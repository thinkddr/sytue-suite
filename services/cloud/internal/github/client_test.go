// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package github

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

// testKeyPEM genera una clave RSA de test y la devuelve en PEM PKCS#1 (el formato
// que descarga GitHub), junto con la clave para verificar firmas.
func testKeyPEM(t *testing.T) (string, *rsa.PrivateKey) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generar clave: %v", err)
	}
	pemBytes := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(key),
	})
	return string(pemBytes), key
}

// testClient construye un Client apuntando a baseURL (httptest) con reloj fijo.
func testClient(t *testing.T, baseURL string) (*Client, *rsa.PrivateKey) {
	t.Helper()
	pemStr, key := testKeyPEM(t)
	c, err := NewClient(424242, pemStr)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	c.baseURL = baseURL
	c.now = func() time.Time { return time.Unix(1_700_000_000, 0) }
	return c, key
}

// verifyJWT valida la firma RS256 y devuelve los claims decodificados.
func verifyJWT(t *testing.T, token string, pub *rsa.PublicKey) map[string]any {
	t.Helper()
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		t.Fatalf("JWT malformado: %d segmentos", len(parts))
	}
	// Cabecera: alg RS256.
	hdrRaw, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		t.Fatalf("decodificar header: %v", err)
	}
	var hdr map[string]string
	if err := json.Unmarshal(hdrRaw, &hdr); err != nil {
		t.Fatalf("header JSON: %v", err)
	}
	if hdr["alg"] != "RS256" || hdr["typ"] != "JWT" {
		t.Fatalf("header inesperado: %+v", hdr)
	}
	// Firma sobre header.payload.
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		t.Fatalf("decodificar firma: %v", err)
	}
	digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if err := rsa.VerifyPKCS1v15(pub, crypto.SHA256, digest[:], sig); err != nil {
		t.Fatalf("firma RS256 inválida: %v", err)
	}
	claimsRaw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatalf("decodificar claims: %v", err)
	}
	var claims map[string]any
	if err := json.Unmarshal(claimsRaw, &claims); err != nil {
		t.Fatalf("claims JSON: %v", err)
	}
	return claims
}

func TestAppJWTClaimsAndExpiry(t *testing.T) {
	c, key := testClient(t, defaultBaseURL)
	token, err := c.appJWT()
	if err != nil {
		t.Fatalf("appJWT: %v", err)
	}
	claims := verifyJWT(t, token, &key.PublicKey)

	// iss = App ID (numérico).
	if iss, _ := claims["iss"].(float64); int64(iss) != 424242 {
		t.Fatalf("iss=%v, se esperaba 424242", claims["iss"])
	}
	iat, _ := claims["iat"].(float64)
	exp, _ := claims["exp"].(float64)
	now := c.now().Unix()
	if int64(iat) > now {
		t.Fatalf("iat en el futuro: iat=%d now=%d", int64(iat), now)
	}
	// GitHub rechaza exp > iat + 10min. Comprobamos el margen holgado.
	if d := int64(exp) - int64(iat); d <= 0 || d > 600 {
		t.Fatalf("vida del JWT fuera de rango (0,600]: %d s", d)
	}
}

func TestNewClientConfiguration(t *testing.T) {
	// No configurada: (nil, nil) → integración deshabilitada.
	if c, err := NewClient(0, ""); c != nil || err != nil {
		t.Fatalf("sin configurar debe ser (nil,nil): c=%v err=%v", c, err)
	}
	// Solo uno de los dos: error.
	if _, err := NewClient(1, ""); err == nil {
		t.Fatal("App ID sin clave debe fallar")
	}
	pemStr, _ := testKeyPEM(t)
	if _, err := NewClient(0, pemStr); err == nil {
		t.Fatal("clave sin App ID debe fallar")
	}
	// Clave basura: error.
	if _, err := NewClient(1, "no-es-pem"); err == nil {
		t.Fatal("clave no-PEM debe fallar")
	}
}

func TestInstallationTokenExchange(t *testing.T) {
	var gotAuth, gotUA, gotPath, gotMethod string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotUA = r.Header.Get("User-Agent")
		gotPath = r.URL.Path
		gotMethod = r.Method
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"token":"ghs_test123","expires_at":"2030-01-01T00:00:00Z"}`))
	}))
	defer srv.Close()

	c, key := testClient(t, srv.URL)
	token, exp, err := c.InstallationToken(context.Background(), 123)
	if err != nil {
		t.Fatalf("InstallationToken: %v", err)
	}
	if token != "ghs_test123" {
		t.Fatalf("token=%q", token)
	}
	if exp.Year() != 2030 {
		t.Fatalf("expires_at no parseado: %v", exp)
	}
	if gotMethod != http.MethodPost || gotPath != "/app/installations/123/access_tokens" {
		t.Fatalf("método/ruta inesperados: %s %s", gotMethod, gotPath)
	}
	if gotUA != userAgent {
		t.Fatalf("User-Agent=%q (obligatorio en GitHub)", gotUA)
	}
	// El intercambio va autenticado con el JWT de app (Bearer), y ese JWT verifica.
	jwt, ok := strings.CutPrefix(gotAuth, "Bearer ")
	if !ok {
		t.Fatalf("Authorization no es Bearer: %q", gotAuth)
	}
	verifyJWT(t, jwt, &key.PublicKey) // falla el test si la firma no valida
}

func TestInstallationTokenRejectsBadID(t *testing.T) {
	c, _ := testClient(t, defaultBaseURL)
	if _, _, err := c.InstallationToken(context.Background(), 0); err == nil {
		t.Fatal("installation id <= 0 debe fallar sin llamar a GitHub")
	}
}

// ghCommentServer simula la API de comentarios de PR, contando POST/PATCH y
// devolviendo un id incremental en cada creación.
type ghCommentServer struct {
	posts   int
	patches int
	nextID  int64
	// patchStatus permite forzar 404 en PATCH (comentario borrado).
	patchStatus int
}

func (g *ghCommentServer) handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/comments"):
			if auth := r.Header.Get("Authorization"); auth != "token ghs_x" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			g.posts++
			g.nextID++
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"id":` + strconv.FormatInt(g.nextID, 10) + `}`))
		case r.Method == http.MethodPatch:
			g.patches++
			st := g.patchStatus
			if st == 0 {
				st = http.StatusOK
			}
			w.WriteHeader(st)
			if st < 300 {
				_, _ = w.Write([]byte(`{"id":100}`))
			}
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
}

func TestUpsertPRCommentCreatesThenEdits(t *testing.T) {
	g := &ghCommentServer{}
	srv := httptest.NewServer(g.handler())
	defer srv.Close()
	c, _ := testClient(t, srv.URL)
	ctx := context.Background()

	// Primera vez (sin id previo): crea → id 1.
	id, err := c.UpsertPRComment(ctx, "ghs_x", "octo", "repo", 7, 0, "body")
	if err != nil {
		t.Fatalf("crear: %v", err)
	}
	if id != 1 || g.posts != 1 || g.patches != 0 {
		t.Fatalf("primera vez: id=%d posts=%d patches=%d", id, g.posts, g.patches)
	}

	// Re-deploy del mismo PR (con id previo): EDITA, no duplica.
	id2, err := c.UpsertPRComment(ctx, "ghs_x", "octo", "repo", 7, id, "body v2")
	if err != nil {
		t.Fatalf("editar: %v", err)
	}
	if id2 != id {
		t.Fatalf("editar debe devolver el mismo id: %d != %d", id2, id)
	}
	if g.posts != 1 {
		t.Fatalf("no debe crear un segundo comentario: posts=%d", g.posts)
	}
	if g.patches != 1 {
		t.Fatalf("debe editar el comentario existente: patches=%d", g.patches)
	}
}

func TestUpsertPRCommentRecreatesIfGone(t *testing.T) {
	g := &ghCommentServer{patchStatus: http.StatusNotFound}
	srv := httptest.NewServer(g.handler())
	defer srv.Close()
	c, _ := testClient(t, srv.URL)

	// Con id previo pero el comentario fue borrado (PATCH 404): recrea vía POST.
	id, err := c.UpsertPRComment(context.Background(), "ghs_x", "octo", "repo", 7, 55, "body")
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if g.patches != 1 || g.posts != 1 {
		t.Fatalf("debe intentar PATCH y caer a POST: patches=%d posts=%d", g.patches, g.posts)
	}
	if id != 1 {
		t.Fatalf("id recreado=%d, se esperaba 1", id)
	}
}

func TestExchangeUserCode(t *testing.T) {
	var gotPath, gotCT, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotCT = r.Header.Get("Content-Type")
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"ghu_user123","token_type":"bearer"}`))
	}))
	defer srv.Close()

	c, _ := testClient(t, defaultBaseURL)
	c.oauthBaseURL = srv.URL
	tok, err := c.ExchangeUserCode(context.Background(), "Iv1.cid", "sec", "the-code")
	if err != nil {
		t.Fatalf("ExchangeUserCode: %v", err)
	}
	if tok != "ghu_user123" {
		t.Fatalf("token=%q", tok)
	}
	if gotPath != "/login/oauth/access_token" {
		t.Fatalf("path=%q", gotPath)
	}
	if !strings.HasPrefix(gotCT, "application/x-www-form-urlencoded") {
		t.Fatalf("content-type=%q", gotCT)
	}
	if !strings.Contains(gotBody, "code=the-code") || !strings.Contains(gotBody, "client_secret=sec") {
		t.Fatalf("body no lleva code/secret: %q", gotBody)
	}
}

func TestExchangeUserCodeErrorBody(t *testing.T) {
	// GitHub responde 200 con {error:...} ante un code inválido → debe fallar.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"error":"bad_verification_code"}`))
	}))
	defer srv.Close()
	c, _ := testClient(t, defaultBaseURL)
	c.oauthBaseURL = srv.URL
	if _, err := c.ExchangeUserCode(context.Background(), "cid", "sec", "x"); err == nil {
		t.Fatal("error en el cuerpo debe propagarse como fallo")
	}
}

func TestExchangeUserCodeValidatesInput(t *testing.T) {
	c, _ := testClient(t, defaultBaseURL)
	if _, err := c.ExchangeUserCode(context.Background(), "", "sec", "x"); err == nil {
		t.Fatal("sin client_id debe fallar sin llamar a GitHub")
	}
	if _, err := c.ExchangeUserCode(context.Background(), "cid", "sec", ""); err == nil {
		t.Fatal("sin code debe fallar sin llamar a GitHub")
	}
}

func TestUserInstallationsPaginates(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		if r.URL.Path != "/user/installations" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		// total_count=3, 2 por respuesta pero declarando 100 per_page: la parada
		// real es len(ids) >= total_count. Página 1 → [1,2]; página 2 → [3].
		switch r.URL.Query().Get("page") {
		case "1":
			_, _ = w.Write([]byte(`{"total_count":3,"installations":[{"id":1},{"id":2}]}`))
		case "2":
			_, _ = w.Write([]byte(`{"total_count":3,"installations":[{"id":3}]}`))
		default:
			_, _ = w.Write([]byte(`{"total_count":3,"installations":[]}`))
		}
	}))
	defer srv.Close()
	c, _ := testClient(t, srv.URL)
	ids, err := c.UserInstallations(context.Background(), "ghu_user123")
	if err != nil {
		t.Fatalf("UserInstallations: %v", err)
	}
	if len(ids) != 3 || ids[0] != 1 || ids[2] != 3 {
		t.Fatalf("ids=%v, se esperaba [1 2 3]", ids)
	}
	if gotAuth != "Bearer ghu_user123" {
		t.Fatalf("Authorization=%q (debe llevar el user token en Bearer)", gotAuth)
	}
}

func TestUserInstallationsRejectsEmptyToken(t *testing.T) {
	c, _ := testClient(t, defaultBaseURL)
	if _, err := c.UserInstallations(context.Background(), ""); err == nil {
		t.Fatal("token vacío debe fallar sin llamar a GitHub")
	}
}

func TestParseRepo(t *testing.T) {
	cases := []struct {
		in    string
		owner string
		repo  string
		ok    bool
	}{
		{"https://github.com/octo/hello.git", "octo", "hello", true},
		{"https://github.com/octo/hello", "octo", "hello", true},
		{"git@github.com:octo/hello.git", "octo", "hello", true},
		{"https://gitlab.com/octo/hello.git", "", "", false},
		{"https://github.com/octo", "", "", false},
		{"not a url ::::", "", "", false},
	}
	for _, tc := range cases {
		owner, repo, ok := ParseRepo(tc.in)
		if ok != tc.ok || owner != tc.owner || repo != tc.repo {
			t.Errorf("ParseRepo(%q)=(%q,%q,%v), se esperaba (%q,%q,%v)",
				tc.in, owner, repo, ok, tc.owner, tc.repo, tc.ok)
		}
	}
}
