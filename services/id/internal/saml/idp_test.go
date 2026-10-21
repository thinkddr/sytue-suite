// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Tests herméticos (sin BD, sin red) del lado IdP: Sytue ID EMITE una Response SAML
// firmada y el lado SP de crewjam la VALIDA — bucle criptográfico cerrado. La firma
// es real (a nivel de elemento Assertion + Response), de modo que la validación se
// ejerce de verdad: cert de confianza ⇒ claims; cert ajeno (firma mal) ⇒ RECHAZO.
package saml

import (
	"encoding/xml"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	csaml "github.com/crewjam/saml"

	"git.sytue.eu/sytue/one/services/id/internal/domain"
)

const (
	tiIDPEntity = "https://id.test/saml/idp/metadata?org=o1"
	tiIDPSSO    = "https://id.test/saml/idp/sso"
	tiIDPSLO    = "https://id.test/saml/idp/slo"
	tiSPEntity  = "https://sp.example/metadata"
	tiSPACS     = "https://sp.example/acs"
)

func mustURL(t *testing.T, s string) url.URL {
	t.Helper()
	u, err := url.Parse(s)
	if err != nil {
		t.Fatalf("url %q: %v", s, err)
	}
	return *u
}

// buildTestIDP genera una clave IdP nueva y construye el IdP con las URLs de prueba.
func buildTestIDP(t *testing.T) *IDP {
	t.Helper()
	priv, cert, err := GenerateIDPKeyPEM("sytue-idp-test")
	if err != nil {
		t.Fatalf("GenerateIdPKeyPEM: %v", err)
	}
	idp, err := NewIDP(priv, cert, tiIDPEntity, tiIDPSSO, tiIDPSLO)
	if err != nil {
		t.Fatalf("NewIdP: %v", err)
	}
	return idp
}

// trustingSP construye un SP crewjam que confía en la metadata (cert de firma) del IdP
// dado, para VALIDAR de verdad la Response emitida.
func trustingSP(t *testing.T, idp *IDP) *csaml.ServiceProvider {
	t.Helper()
	metaBytes, err := idp.MetadataXML()
	if err != nil {
		t.Fatalf("MetadataXML: %v", err)
	}
	var idpED csaml.EntityDescriptor
	if err := xml.Unmarshal(metaBytes, &idpED); err != nil {
		t.Fatalf("unmarshal metadata: %v", err)
	}
	acs := mustURL(t, tiSPACS)
	meta := mustURL(t, tiSPEntity)
	return &csaml.ServiceProvider{
		EntityID:          tiSPEntity,
		AcsURL:            acs,
		MetadataURL:       meta,
		IDPMetadata:       &idpED,
		AllowIDPInitiated: true,
	}
}

// acsRequest arma el POST al ACS con la Response emitida (crewjam lee r.PostForm).
func acsRequest(t *testing.T, samlResponse string) *http.Request {
	t.Helper()
	form := url.Values{"SAMLResponse": {samlResponse}, "RelayState": {"relay-1"}}
	r := httptest.NewRequest(http.MethodPost, tiSPACS, strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if err := r.ParseForm(); err != nil {
		t.Fatalf("ParseForm: %v", err)
	}
	return r
}

func testSP() domain.SAMLServiceProvider {
	return domain.SAMLServiceProvider{
		EntityID: tiSPEntity, ACSURL: tiSPACS, NameIDFormat: NameIDEmail, Enabled: true,
	}
}

func testUser() UserIdentity {
	return UserIdentity{StableID: "user-uuid-1", Email: "ada@acme.example", Name: "Ada Lovelace", Groups: []string{"eng", "ops"}}
}

// TestIdP_SPInitiated_ValidatedBySP: emisión SP-initiated (InResponseTo) validada por
// el SP; comprueba NameID, atributos y el InResponseTo.
func TestIdP_SPInitiated_ValidatedBySP(t *testing.T) {
	idp := buildTestIDP(t)
	form, err := idp.MakeResponse(httptest.NewRequest(http.MethodGet, "/", http.NoBody),
		testSP(), "authn-req-123", "relay-1", testUser())
	if err != nil {
		t.Fatalf("MakeResponse: %v", err)
	}
	if form.ACSURL != tiSPACS || form.RelayState != "relay-1" {
		t.Fatalf("form ACS/RelayState inesperados: %+v", form)
	}

	sp := trustingSP(t, idp)
	assertion, err := sp.ParseResponse(acsRequest(t, form.SAMLResponse), []string{"authn-req-123"})
	if err != nil {
		t.Fatalf("el SP debería validar la Response firmada: %v", err)
	}
	if assertion.Subject == nil || assertion.Subject.NameID == nil || assertion.Subject.NameID.Value != "ada@acme.example" {
		t.Fatalf("NameID inesperado: %+v", assertion.Subject)
	}
	got := map[string][]string{}
	for _, st := range assertion.AttributeStatements {
		for _, a := range st.Attributes {
			for _, v := range a.Values {
				got[a.Name] = append(got[a.Name], v.Value)
			}
		}
	}
	if len(got["email"]) != 1 || got["email"][0] != "ada@acme.example" {
		t.Errorf("atributo email = %v", got["email"])
	}
	if strings.Join(got["groups"], ",") != "eng,ops" {
		t.Errorf("atributo groups = %v", got["groups"])
	}
	if len(got["name"]) == 0 || got["name"][0] != "Ada Lovelace" {
		t.Errorf("atributo name = %v", got["name"])
	}
}

// TestIdP_IDPInitiated_ValidatedBySP: emisión IdP-initiated (sin InResponseTo) validada
// por un SP con AllowIDPInitiated.
func TestIdP_IDPInitiated_ValidatedBySP(t *testing.T) {
	idp := buildTestIDP(t)
	form, err := idp.MakeResponse(httptest.NewRequest(http.MethodGet, "/", http.NoBody),
		testSP(), "", "portal", testUser())
	if err != nil {
		t.Fatalf("MakeResponse: %v", err)
	}
	sp := trustingSP(t, idp)
	// InResponseTo vacío ⇒ el SP lo admite con "" entre los posibles request-IDs.
	if _, err := sp.ParseResponse(acsRequest(t, form.SAMLResponse), []string{""}); err != nil {
		t.Fatalf("el SP debería validar la Response IdP-initiated: %v", err)
	}
}

// TestIdP_BadSignatureRejected: una Response emitida por el IdP A pero validada contra
// la metadata (cert) del IdP B ⇒ RECHAZO. Prueba que la firma es REAL (si se firma mal,
// el SP no la acepta).
func TestIdP_BadSignatureRejected(t *testing.T) {
	idpA := buildTestIDP(t) // firma con A…
	idpB := buildTestIDP(t) // …pero el SP confía en el cert de B.
	form, err := idpA.MakeResponse(httptest.NewRequest(http.MethodGet, "/", http.NoBody),
		testSP(), "authn-req-123", "relay-1", testUser())
	if err != nil {
		t.Fatalf("MakeResponse: %v", err)
	}
	sp := trustingSP(t, idpB)
	if _, err := sp.ParseResponse(acsRequest(t, form.SAMLResponse), []string{"authn-req-123"}); err == nil {
		t.Fatal("una Response firmada por otra clave debería RECHAZARSE")
	}
}

// TestIdP_PersistentNameID: el formato persistent usa el ID estable del usuario.
func TestIdP_PersistentNameID(t *testing.T) {
	idp := buildTestIDP(t)
	sp := testSP()
	sp.NameIDFormat = NameIDPersistent
	form, err := idp.MakeResponse(httptest.NewRequest(http.MethodGet, "/", http.NoBody),
		sp, "req-1", "", testUser())
	if err != nil {
		t.Fatalf("MakeResponse: %v", err)
	}
	spVal := trustingSP(t, idp)
	assertion, err := spVal.ParseResponse(acsRequest(t, form.SAMLResponse), []string{"req-1"})
	if err != nil {
		t.Fatalf("ParseResponse: %v", err)
	}
	if assertion.Subject.NameID.Value != "user-uuid-1" {
		t.Fatalf("persistent NameID = %q, quiero el StableID", assertion.Subject.NameID.Value)
	}
	if assertion.Subject.NameID.Format != NameIDPersistent {
		t.Errorf("formato NameID = %q", assertion.Subject.NameID.Format)
	}
}
