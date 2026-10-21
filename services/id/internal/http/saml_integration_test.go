// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

// Tests de integración del login federado SAML (Sytue ID como SP) contra un
// PostgreSQL real (pgtest) y un IdP SIMULADO con firma XML-DSig real (el lado IdP de
// crewjam). Ejercen el ACS de punta a punta por HTTP:
//   - Response válida ⇒ sesión SSO + aprovisionamiento JIT en la org del provider;
//   - segunda vez ⇒ casa por federated_identities (mismo usuario, sin alta);
//   - REPLAY de la misma Response ⇒ rechazo (estado single-use consumido);
//   - casa por email (verificado por la firma) de un miembro existente de la org;
//   - usuario de OTRA org con el mismo email ⇒ rechazo (aislamiento tenant);
//   - firma inválida (otra clave) ⇒ rechazo;
//   - enrutado por dominio ⇒ /login/sso lleva a /saml/{id}/login.
package http

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/pem"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/beevik/etree"
	csaml "github.com/crewjam/saml"
	dsig "github.com/russellhaering/goxmldsig"

	"git.sytue.eu/sytue/one/services/id/internal/domain"
)

func samlKeyCert(t *testing.T) (*rsa.PrivateKey, *x509.Certificate, string) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("rsa: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "idp"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(24 * time.Hour),
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("cert: %v", err)
	}
	cert, _ := x509.ParseCertificate(der)
	return key, cert, string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
}

func mintSAMLResponse(t *testing.T, key *rsa.PrivateKey, cert *x509.Certificate,
	idpEntityID, audience, acsURL, requestID string, now time.Time, subject, email string, groups []string,
) string {
	t.Helper()
	metaURL, _ := url.Parse(idpEntityID)
	ssoURL, _ := url.Parse("https://idp.acme/sso")
	idp := &csaml.IdentityProvider{
		Key: key, Certificate: cert, MetadataURL: *metaURL, SSOURL: *ssoURL,
		SignatureMethod: dsig.RSASHA256SignatureMethod,
	}
	spMeta := &csaml.EntityDescriptor{EntityID: audience, SPSSODescriptors: []csaml.SPSSODescriptor{{}}}
	req := &csaml.IdpAuthnRequest{
		Now: now, IDP: idp,
		HTTPRequest:             httptest.NewRequest(http.MethodPost, "https://idp.acme/sso", nil),
		Request:                 csaml.AuthnRequest{ID: requestID, IssueInstant: now, Version: "2.0"},
		ServiceProviderMetadata: spMeta,
		SPSSODescriptor:         &spMeta.SPSSODescriptors[0],
		ACSEndpoint:             &csaml.IndexedEndpoint{Binding: csaml.HTTPPostBinding, Location: acsURL},
	}
	attrs := []csaml.Attribute{{
		Name: "email", Values: []csaml.AttributeValue{{Type: "xs:string", Value: email}},
	}, {Name: "name", Values: []csaml.AttributeValue{{Type: "xs:string", Value: "Grace Hopper"}}}}
	if len(groups) > 0 {
		gv := make([]csaml.AttributeValue, 0, len(groups))
		for _, g := range groups {
			gv = append(gv, csaml.AttributeValue{Type: "xs:string", Value: g})
		}
		attrs = append(attrs, csaml.Attribute{Name: "groups", Values: gv})
	}
	if err := (csaml.DefaultAssertionMaker{}).MakeAssertion(req, &csaml.Session{
		NameID: subject, NameIDFormat: "urn:oasis:names:tc:SAML:1.1:nameid-format:emailAddress",
		CustomAttributes: attrs,
	}); err != nil {
		t.Fatalf("MakeAssertion: %v", err)
	}
	if err := req.MakeResponse(); err != nil {
		t.Fatalf("MakeResponse: %v", err)
	}
	doc := etree.NewDocument()
	doc.SetRoot(req.ResponseEl)
	raw, _ := doc.WriteToBytes()
	return base64.StdEncoding.EncodeToString(raw)
}

func (h *fedHarness) seedSAMLProvider(t *testing.T, cert *x509.Certificate, slug, idpEntityID string, domains []string) (string, domain.IdentityProvider) {
	t.Helper()
	org, err := h.st.CreateOrg(h.ctx, nil, domain.Organization{Slug: slug, Name: strings.ToUpper(slug)})
	if err != nil {
		t.Fatalf("CreateOrg: %v", err)
	}
	certPEM := string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Raw}))
	p, err := h.st.CreateIdentityProvider(h.ctx, nil, domain.IdentityProvider{
		OrgID: org.ID, Type: "saml", Name: "Corp SAML", Slug: "corp-saml",
		IDPEntityID: idpEntityID, IDPSSOURL: "https://idp.acme/sso", IDPCertificate: certPEM,
		SAMLBinding: "redirect", AllowedEmailDomains: domains, Enabled: true,
	})
	if err != nil {
		t.Fatalf("CreateIdentityProvider(saml): %v", err)
	}
	return org.ID, p
}

// startSAML ejecuta GET /saml/{id}/login y devuelve la cookie de binding y el
// request-ID emitido (leído del auth_state por el hash del RelayState).
func (h *fedHarness) startSAML(t *testing.T, provID string) (*http.Cookie, string) {
	t.Helper()
	rec := httptest.NewRecorder()
	h.mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/saml/"+provID+"/login", http.NoBody))
	if rec.Code != http.StatusFound {
		t.Fatalf("/login código=%d body=%s", rec.Code, rec.Body.String())
	}
	var cookie *http.Cookie
	for _, c := range rec.Result().Cookies() {
		if c.Name == samlCookieName {
			cookie = c
		}
	}
	if cookie == nil {
		t.Fatal("/login no fijó la cookie de binding SAML")
	}
	var requestID string
	if err := h.st.DB().QueryRow(h.ctx,
		`SELECT nonce FROM federated_auth_states WHERE state_hash=$1`,
		domain.HashToken(cookie.Value)).Scan(&requestID); err != nil {
		t.Fatalf("leer request-id: %v", err)
	}
	return cookie, requestID
}

func (h *fedHarness) postACS(provID string, cookie *http.Cookie, samlResponse string) *httptest.ResponseRecorder {
	form := url.Values{"SAMLResponse": {samlResponse}, "RelayState": {cookie.Value}}
	req := httptest.NewRequest(http.MethodPost, "/saml/"+provID+"/acs", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	h.mux.ServeHTTP(rec, req)
	return rec
}

// samlLogin hace el flujo completo login→mint→acs con una Response VÁLIDA.
func (h *fedHarness) samlLogin(t *testing.T, prov domain.IdentityProvider, key *rsa.PrivateKey, cert *x509.Certificate, subject, email string) *httptest.ResponseRecorder {
	t.Helper()
	cookie, requestID := h.startSAML(t, prov.ID)
	acsURL := itIssuer + "/saml/" + prov.ID + "/acs"
	audience := itIssuer + "/saml/" + prov.ID + "/metadata"
	resp := mintSAMLResponse(t, key, cert, prov.IDPEntityID, audience, acsURL, requestID, time.Now(), subject, email, []string{"eng"})
	return h.postACS(prov.ID, cookie, resp)
}

func TestSAML_JITAndSession(t *testing.T) {
	h := newFedHarness(t)
	key, cert, _ := samlKeyCert(t)
	orgID, prov := h.seedSAMLProvider(t, cert, "acme", "https://idp.acme/metadata", []string{"acme.example"})

	rec := h.samlLogin(t, prov, key, cert, "grace@acme.example", "grace@acme.example")
	if rec.Code != http.StatusFound {
		t.Fatalf("ACS código=%d body=%s", rec.Code, rec.Body.String())
	}
	if !hasSessionCookie(rec) {
		t.Fatal("el ACS no abrió sesión SSO")
	}
	u, err := h.st.GetUserByEmail(h.ctx, "grace@acme.example")
	if err != nil {
		t.Fatalf("usuario no aprovisionado (JIT): %v", err)
	}
	if !u.EmailVerified {
		t.Error("el JIT SAML debe marcar el email verificado")
	}
	if _, err := h.st.GetMembership(h.ctx, orgID, u.ID); err != nil {
		t.Errorf("sin membresía en la org del provider: %v", err)
	}
	if _, err := h.st.GetFederatedUserID(h.ctx, prov.ID, "grace@acme.example"); err != nil {
		t.Errorf("sin enlace federated_identities: %v", err)
	}
}

func TestSAML_FindByFederatedIdentity(t *testing.T) {
	h := newFedHarness(t)
	key, cert, _ := samlKeyCert(t)
	_, prov := h.seedSAMLProvider(t, cert, "acme", "https://idp.acme/metadata", nil)

	if rec := h.samlLogin(t, prov, key, cert, "grace@acme.example", "grace@acme.example"); rec.Code != http.StatusFound {
		t.Fatalf("primer login: %d", rec.Code)
	}
	first, _ := h.st.GetUserByEmail(h.ctx, "grace@acme.example")
	// Segundo login del mismo sujeto: casa por federated_identities, sin alta nueva.
	if rec := h.samlLogin(t, prov, key, cert, "grace@acme.example", "grace@acme.example"); rec.Code != http.StatusFound {
		t.Fatalf("segundo login: %d", rec.Code)
	}
	uid, err := h.st.GetFederatedUserID(h.ctx, prov.ID, "grace@acme.example")
	if err != nil || uid != first.ID {
		t.Fatalf("no casó por federated_identity: uid=%s first=%s err=%v", uid, first.ID, err)
	}
}

func TestSAML_ReplayRejected(t *testing.T) {
	h := newFedHarness(t)
	key, cert, _ := samlKeyCert(t)
	_, prov := h.seedSAMLProvider(t, cert, "acme", "https://idp.acme/metadata", nil)

	cookie, requestID := h.startSAML(t, prov.ID)
	acsURL := itIssuer + "/saml/" + prov.ID + "/acs"
	audience := itIssuer + "/saml/" + prov.ID + "/metadata"
	resp := mintSAMLResponse(t, key, cert, prov.IDPEntityID, audience, acsURL, requestID, time.Now(), "grace@acme.example", "grace@acme.example", nil)

	if rec := h.postACS(prov.ID, cookie, resp); rec.Code != http.StatusFound {
		t.Fatalf("primer ACS debería aceptar: %d", rec.Code)
	}
	// Replay de la MISMA Response con la misma cookie: el estado ya se consumió.
	if rec := h.postACS(prov.ID, cookie, resp); rec.Code == http.StatusFound {
		t.Fatal("un replay de la Response NO debería abrir sesión (anti-replay)")
	}
}

func TestSAML_CrossOrgRejected(t *testing.T) {
	h := newFedHarness(t)
	key, cert, _ := samlKeyCert(t)
	// Usuario miembro de la org BETA.
	beta, _ := h.st.CreateOrg(h.ctx, nil, domain.Organization{Slug: "beta", Name: "Beta"})
	u, _ := h.st.CreateUser(h.ctx, nil, domain.User{Email: "victim@acme.example", EmailVerified: true, Status: domain.StatusActive})
	_ = h.st.UpsertMembership(h.ctx, nil, domain.Membership{OrgID: beta.ID, UserID: u.ID, Role: domain.RoleMember})

	// Provider SAML de ACME afirma el email del usuario de BETA (sujeto nuevo).
	_, prov := h.seedSAMLProvider(t, cert, "acme", "https://idp.acme/metadata", nil)
	rec := h.samlLogin(t, prov, key, cert, "ext-subject-x", "victim@acme.example")
	if rec.Code == http.StatusFound || hasSessionCookie(rec) {
		t.Fatalf("un provider de ACME NO debe autenticar a un miembro de BETA (código=%d)", rec.Code)
	}
	// El usuario sigue siendo solo de BETA (no se enlazó ni se le añadió a ACME).
	if _, err := h.st.GetFederatedUserID(h.ctx, prov.ID, "ext-subject-x"); err == nil {
		t.Error("no debería haberse creado enlace federado cross-org")
	}
}

func TestSAML_LinkByEmailMember(t *testing.T) {
	h := newFedHarness(t)
	key, cert, _ := samlKeyCert(t)
	orgID, prov := h.seedSAMLProvider(t, cert, "acme", "https://idp.acme/metadata", nil)
	// Usuario que YA es miembro de ACME (p. ej. creado por contraseña antes).
	u, _ := h.st.CreateUser(h.ctx, nil, domain.User{Email: "ada@acme.example", EmailVerified: true, Status: domain.StatusActive})
	_ = h.st.UpsertMembership(h.ctx, nil, domain.Membership{OrgID: orgID, UserID: u.ID, Role: domain.RoleMember})

	rec := h.samlLogin(t, prov, key, cert, "ada-ext-sub", "ada@acme.example")
	if rec.Code != http.StatusFound || !hasSessionCookie(rec) {
		t.Fatalf("un miembro existente debería enlazarse por email (código=%d body=%s)", rec.Code, rec.Body.String())
	}
	uid, err := h.st.GetFederatedUserID(h.ctx, prov.ID, "ada-ext-sub")
	if err != nil || uid != u.ID {
		t.Fatalf("no enlazó al usuario existente: uid=%s want=%s err=%v", uid, u.ID, err)
	}
}

func TestSAML_BadSignatureRejected(t *testing.T) {
	h := newFedHarness(t)
	key, cert, _ := samlKeyCert(t)           // provider confía en este cert…
	otherKey, otherCert, _ := samlKeyCert(t) // …pero la Response la firma OTRA clave.
	_ = key
	_ = cert
	_, prov := h.seedSAMLProvider(t, cert, "acme", "https://idp.acme/metadata", nil)

	cookie, requestID := h.startSAML(t, prov.ID)
	acsURL := itIssuer + "/saml/" + prov.ID + "/acs"
	audience := itIssuer + "/saml/" + prov.ID + "/metadata"
	resp := mintSAMLResponse(t, otherKey, otherCert, prov.IDPEntityID, audience, acsURL, requestID, time.Now(), "grace@acme.example", "grace@acme.example", nil)
	if rec := h.postACS(prov.ID, cookie, resp); rec.Code == http.StatusFound {
		t.Fatal("una Response firmada por otra clave debería RECHAZARSE")
	}
}

func TestSAML_DomainRouting(t *testing.T) {
	h := newFedHarness(t)
	_, cert, _ := samlKeyCert(t)
	_, prov := h.seedSAMLProvider(t, cert, "acme", "https://idp.acme/metadata", []string{"acme.example"})

	form := url.Values{"email": {"someone@acme.example"}}
	req := httptest.NewRequest(http.MethodPost, "/login/sso", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	h.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusFound {
		t.Fatalf("/login/sso código=%d", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "/saml/"+prov.ID+"/login" {
		t.Fatalf("el enrutado por dominio no llevó al flujo SAML: %q", loc)
	}
}
