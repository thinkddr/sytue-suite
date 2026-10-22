// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

// Tests de integración de la identidad SALIENTE por SAML (Sytue ID como IdP) contra un
// PostgreSQL real (pgtest) y un SP SIMULADO (el lado SP de crewjam EMBEBIDO en el test).
// Bucle criptográfico cerrado, cero red: el SP genera el AuthnRequest → httptest contra
// nuestro IdP → sp.ParseResponse valida firma/Audience/Recipient/InResponseTo. Cubre:
//   - SP-initiated con sesión ⇒ Assertion firmada válida;
//   - IdP-initiated (1-click) ⇒ Assertion válida (AllowIDPInitiated);
//   - SP deshabilitado ⇒ 403 (sin emisión);
//   - sesión de OTRA org ⇒ 403 (sin fuga cross-org, DENY-BY-DEFAULT);
//   - AuthnRequest malformado ⇒ 400.
package http

import (
	"encoding/xml"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	csaml "github.com/crewjam/saml"

	"git.sytue.eu/sytue/one/services/id/internal/domain"
)

const (
	itSPEntity = "https://app.example/saml/metadata"
	itSPACS    = "https://app.example/saml/acs"
	itEmailFmt = "urn:oasis:names:tc:SAML:1.1:nameid-format:emailAddress"
)

// session crea una sesión SSO viva y devuelve su cookie (valor = token en claro).
func (h *fedHarness) session(t *testing.T, userID, orgID string) *http.Cookie {
	t.Helper()
	raw, err := domain.GenerateToken(32)
	if err != nil {
		t.Fatalf("token: %v", err)
	}
	now := time.Now()
	if _, err := h.st.InsertSession(h.ctx, nil, domain.Session{
		TokenHash: domain.HashToken(raw), UserID: userID, OrgID: orgID,
		AuthTime: now, AMR: []string{"pwd"}, UserAgent: "test", ExpiresAt: now.Add(time.Hour),
	}); err != nil {
		t.Fatalf("InsertSession: %v", err)
	}
	return &http.Cookie{Name: sessionCookieName, Value: raw}
}

// seedApp crea una org y un SP SAML de terceros registrado en ella.
func (h *fedHarness) seedApp(t *testing.T, slug string, enabled, idpInitiated bool) (orgID, spID string) {
	t.Helper()
	org, err := h.st.CreateOrg(h.ctx, nil, domain.Organization{Slug: slug, Name: strings.ToUpper(slug)})
	if err != nil {
		t.Fatalf("CreateOrg: %v", err)
	}
	sp, err := h.st.CreateSAMLServiceProvider(h.ctx, nil, domain.SAMLServiceProvider{
		OrgID: org.ID, Name: "App de prueba", EntityID: itSPEntity, ACSURL: itSPACS,
		NameIDFormat: itEmailFmt, IDPInitiated: idpInitiated, Enabled: enabled,
	})
	if err != nil {
		t.Fatalf("CreateSAMLServiceProvider: %v", err)
	}
	return org.ID, sp.ID
}

// userSession crea un usuario, lo hace miembro de la org y devuelve su cookie de sesión.
func (h *fedHarness) userSession(t *testing.T, email, orgID string) *http.Cookie {
	t.Helper()
	u, err := h.st.CreateUser(h.ctx, nil, domain.User{Email: email, EmailVerified: true, Name: "Grace Hopper", Status: domain.StatusActive})
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if err := h.st.UpsertMembership(h.ctx, nil, domain.Membership{OrgID: orgID, UserID: u.ID, Role: domain.RoleMember}); err != nil {
		t.Fatalf("UpsertMembership: %v", err)
	}
	return h.session(t, u.ID, orgID)
}

// trustingClientSP construye el SP crewjam que confía en la metadata (cert) del IdP de
// la org, para validar de verdad la Response emitida.
func (h *fedHarness) trustingClientSP(t *testing.T, orgID string) *csaml.ServiceProvider {
	t.Helper()
	rec := httptest.NewRecorder()
	h.mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/saml/idp/metadata?org="+orgID, http.NoBody))
	if rec.Code != http.StatusOK {
		t.Fatalf("metadata IdP código=%d body=%s", rec.Code, rec.Body.String())
	}
	var idpED csaml.EntityDescriptor
	if err := xml.Unmarshal(rec.Body.Bytes(), &idpED); err != nil {
		t.Fatalf("unmarshal metadata IdP: %v", err)
	}
	acs, _ := url.Parse(itSPACS)
	meta, _ := url.Parse(itSPEntity)
	return &csaml.ServiceProvider{
		EntityID: itSPEntity, AcsURL: *acs, MetadataURL: *meta,
		IDPMetadata: &idpED, AllowIDPInitiated: true,
	}
}

// extractSAMLResponse saca el valor de SAMLResponse del formulario POST autoenviable.
// La Response es base64 estándar (sin &<>"'), intacta tras html.EscapeString.
func extractSAMLResponse(t *testing.T, body string) string {
	t.Helper()
	const marker = `name="SAMLResponse" value="`
	i := strings.Index(body, marker)
	if i < 0 {
		t.Fatalf("no hay SAMLResponse en el formulario: %s", body)
	}
	rest := body[i+len(marker):]
	j := strings.IndexByte(rest, '"')
	if j < 0 {
		t.Fatalf("SAMLResponse sin cierre")
	}
	return rest[:j]
}

// parseAtSP alimenta la Response emitida al SP (POST /acs) y valida firma/condiciones.
func parseAtSP(t *testing.T, sp *csaml.ServiceProvider, samlResp string, requestIDs []string) (*csaml.Assertion, error) {
	t.Helper()
	form := url.Values{"SAMLResponse": {samlResp}}
	r := httptest.NewRequest(http.MethodPost, itSPACS, strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if err := r.ParseForm(); err != nil {
		t.Fatalf("ParseForm: %v", err)
	}
	return sp.ParseResponse(r, requestIDs)
}

func TestSAMLIdP_SPInitiated(t *testing.T) {
	h := newFedHarness(t)
	orgID, _ := h.seedApp(t, "acme", true, false)
	cookie := h.userSession(t, "grace@acme.example", orgID)
	sp := h.trustingClientSP(t, orgID)

	// El SP genera el AuthnRequest (redirect binding) y lo envía a nuestro SSO.
	loc := sp.GetSSOBindingLocation(csaml.HTTPRedirectBinding)
	authReq, err := sp.MakeAuthenticationRequest(loc, csaml.HTTPRedirectBinding, csaml.HTTPPostBinding)
	if err != nil {
		t.Fatalf("MakeAuthenticationRequest: %v", err)
	}
	redirectURL, err := authReq.Redirect("relay-1", sp)
	if err != nil {
		t.Fatalf("Redirect: %v", err)
	}
	req := httptest.NewRequest(http.MethodGet, "/saml/idp/sso?"+redirectURL.RawQuery, http.NoBody)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	h.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("SSO código=%d body=%s", rec.Code, rec.Body.String())
	}

	assertion, err := parseAtSP(t, sp, extractSAMLResponse(t, rec.Body.String()), []string{authReq.ID})
	if err != nil {
		t.Fatalf("el SP debería validar la Response: %v", err)
	}
	if assertion.Subject == nil || assertion.Subject.NameID == nil || assertion.Subject.NameID.Value != "grace@acme.example" {
		t.Fatalf("NameID inesperado: %+v", assertion.Subject)
	}
}

func TestSAMLIdP_IDPInitiated(t *testing.T) {
	h := newFedHarness(t)
	orgID, spID := h.seedApp(t, "acme", true, true)
	cookie := h.userSession(t, "grace@acme.example", orgID)
	sp := h.trustingClientSP(t, orgID)

	req := httptest.NewRequest(http.MethodGet, "/saml/idp/initiate/"+spID, http.NoBody)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	h.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("initiate código=%d body=%s", rec.Code, rec.Body.String())
	}
	// InResponseTo vacío (unsolicited): el SP lo admite con "" entre los posibles.
	if _, err := parseAtSP(t, sp, extractSAMLResponse(t, rec.Body.String()), []string{""}); err != nil {
		t.Fatalf("el SP debería validar la Response IdP-initiated: %v", err)
	}
}

func TestSAMLIdP_NoSessionRedirectsToLogin(t *testing.T) {
	h := newFedHarness(t)
	orgID, _ := h.seedApp(t, "acme", true, false)
	sp := h.trustingClientSP(t, orgID)
	loc := sp.GetSSOBindingLocation(csaml.HTTPRedirectBinding)
	authReq, _ := sp.MakeAuthenticationRequest(loc, csaml.HTTPRedirectBinding, csaml.HTTPPostBinding)
	redirectURL, _ := authReq.Redirect("", sp)

	// Sin cookie de sesión: se rebota al login conservando el flujo (return_to).
	rec := httptest.NewRecorder()
	h.mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/saml/idp/sso?"+redirectURL.RawQuery, http.NoBody))
	if rec.Code != http.StatusFound {
		t.Fatalf("sin sesión debería redirigir (302), código=%d", rec.Code)
	}
	if loc := rec.Header().Get("Location"); !strings.HasPrefix(loc, "/login/") || !strings.Contains(loc, "return_to") {
		t.Fatalf("Location no lleva a login con return_to: %q", loc)
	}
}

func TestSAMLIdP_DisabledSPDenied(t *testing.T) {
	h := newFedHarness(t)
	orgID, _ := h.seedApp(t, "acme", false /*deshabilitado*/, false)
	cookie := h.userSession(t, "grace@acme.example", orgID)
	sp := h.trustingClientSP(t, orgID)
	loc := sp.GetSSOBindingLocation(csaml.HTTPRedirectBinding)
	authReq, _ := sp.MakeAuthenticationRequest(loc, csaml.HTTPRedirectBinding, csaml.HTTPPostBinding)
	redirectURL, _ := authReq.Redirect("", sp)

	req := httptest.NewRequest(http.MethodGet, "/saml/idp/sso?"+redirectURL.RawQuery, http.NoBody)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	h.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("SP deshabilitado debería denegar (403), código=%d", rec.Code)
	}
}

func TestSAMLIdP_CrossOrgDenied(t *testing.T) {
	h := newFedHarness(t)
	// El SP está registrado en ACME…
	orgA, _ := h.seedApp(t, "acme", true, false)
	sp := h.trustingClientSP(t, orgA)
	// …pero la sesión es de BETA (que NO tiene ese SP registrado).
	beta, err := h.st.CreateOrg(h.ctx, nil, domain.Organization{Slug: "beta", Name: "Beta"})
	if err != nil {
		t.Fatalf("CreateOrg beta: %v", err)
	}
	cookie := h.userSession(t, "eve@beta.example", beta.ID)

	loc := sp.GetSSOBindingLocation(csaml.HTTPRedirectBinding)
	authReq, _ := sp.MakeAuthenticationRequest(loc, csaml.HTTPRedirectBinding, csaml.HTTPPostBinding)
	redirectURL, _ := authReq.Redirect("", sp)
	req := httptest.NewRequest(http.MethodGet, "/saml/idp/sso?"+redirectURL.RawQuery, http.NoBody)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	h.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("un SP de ACME NO debe emitir para una sesión de BETA (código=%d)", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "SAMLResponse") {
		t.Fatal("no debería haberse emitido ninguna Assertion cross-org")
	}
}

func TestSAMLIdP_MalformedAuthnRequest(t *testing.T) {
	h := newFedHarness(t)
	orgID, _ := h.seedApp(t, "acme", true, false)
	cookie := h.userSession(t, "grace@acme.example", orgID)

	req := httptest.NewRequest(http.MethodGet, "/saml/idp/sso?SAMLRequest=not-a-valid-request", http.NoBody)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	h.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("AuthnRequest malformado debería dar 400, código=%d", rec.Code)
	}
}

func TestSAMLIdP_InitiateCrossOrgDenied(t *testing.T) {
	h := newFedHarness(t)
	orgA, spID := h.seedApp(t, "acme", true, true)
	_ = orgA
	beta, _ := h.st.CreateOrg(h.ctx, nil, domain.Organization{Slug: "beta", Name: "Beta"})
	cookie := h.userSession(t, "eve@beta.example", beta.ID)

	req := httptest.NewRequest(http.MethodGet, "/saml/idp/initiate/"+spID, http.NoBody)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	h.mux.ServeHTTP(rec, req)
	if rec.Code == http.StatusOK || strings.Contains(rec.Body.String(), "SAMLResponse") {
		t.Fatalf("IdP-initiated cross-org debería denegar (código=%d)", rec.Code)
	}
}
