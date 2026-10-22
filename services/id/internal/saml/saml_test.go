// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Tests herméticos (sin BD) del SP SAML: metadata, AuthnRequest, parseo de metadata
// del IdP, validación de config y —lo crítico— la matriz de validación de la
// Response con crewjam. Las Response FIRMADAS se acuñan con el propio lado IdP de
// crewjam (DefaultAssertionMaker + MakeResponse), de modo que la firma es real y la
// validación del SP se ejerce de verdad: válida ⇒ claims; firma inválida / audiencia
// mal / expirada / InResponseTo desconocido / recipient mal ⇒ RECHAZO.
package saml

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/pem"
	"encoding/xml"
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

const (
	tIDPEntityID = "https://idp.example.com/metadata"
	tSPEntityID  = "https://id.test/saml/pid/metadata"
	tACSURL      = "https://id.test/saml/pid/acs"
	tSSOURL      = "https://idp.example.com/sso"
	tRequestID   = "id-req-0123456789"
	tEmailFormat = "urn:oasis:names:tc:SAML:1.1:nameid-format:emailAddress"
)

// genKeyCert genera una clave RSA + certificado autofirmado (el par de firma del
// IdP simulado) y devuelve además el certificado en PEM.
func genKeyCert(t *testing.T) (*rsa.PrivateKey, *x509.Certificate, string) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("rsa: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "test-idp"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("cert: %v", err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("parse cert: %v", err)
	}
	return key, cert, string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
}

func testAttrs(email string, groups []string) []csaml.Attribute {
	const uri = "urn:oasis:names:tc:SAML:2.0:attrname-format:uri"
	attrs := make([]csaml.Attribute, 0, 3) // email + name + groups
	attrs = append(attrs, csaml.Attribute{
		Name: "email", NameFormat: uri,
		Values: []csaml.AttributeValue{{Type: "xs:string", Value: email}},
	}, csaml.Attribute{
		Name: "name", NameFormat: uri,
		Values: []csaml.AttributeValue{{Type: "xs:string", Value: "Ada Lovelace"}},
	})
	gv := make([]csaml.AttributeValue, 0, len(groups))
	for _, g := range groups {
		gv = append(gv, csaml.AttributeValue{Type: "xs:string", Value: g})
	}
	attrs = append(attrs, csaml.Attribute{Name: "groups", NameFormat: uri, Values: gv})
	return attrs
}

// mintResponse acuña una SAML Response FIRMADA (Response + Assertion) con el lado IdP
// de crewjam, tal y como la emitiría un IdP real, y la devuelve en base64 (lista para
// el campo SAMLResponse del POST al ACS).
func mintResponse(t *testing.T, key *rsa.PrivateKey, cert *x509.Certificate,
	idpEntityID, spEntityID, acsURL, requestID string, now time.Time,
	nameID string, attrs []csaml.Attribute,
) string {
	t.Helper()
	metaURL, _ := url.Parse(idpEntityID)
	ssoURL, _ := url.Parse(tSSOURL)
	idp := &csaml.IdentityProvider{
		Key: key, Certificate: cert, MetadataURL: *metaURL, SSOURL: *ssoURL,
		SignatureMethod: dsig.RSASHA256SignatureMethod,
	}
	spMeta := &csaml.EntityDescriptor{
		EntityID:         spEntityID,
		SPSSODescriptors: []csaml.SPSSODescriptor{{}},
	}
	req := &csaml.IdpAuthnRequest{
		Now: now, IDP: idp,
		HTTPRequest:             httptest.NewRequest(http.MethodPost, tSSOURL, http.NoBody),
		Request:                 csaml.AuthnRequest{ID: requestID, IssueInstant: now, Version: "2.0"},
		ServiceProviderMetadata: spMeta,
		SPSSODescriptor:         &spMeta.SPSSODescriptors[0],
		ACSEndpoint:             &csaml.IndexedEndpoint{Binding: csaml.HTTPPostBinding, Location: acsURL},
	}
	if err := (csaml.DefaultAssertionMaker{}).MakeAssertion(req, &csaml.Session{
		NameID: nameID, NameIDFormat: tEmailFormat, CustomAttributes: attrs,
	}); err != nil {
		t.Fatalf("MakeAssertion: %v", err)
	}
	if err := req.MakeResponse(); err != nil {
		t.Fatalf("MakeResponse: %v", err)
	}
	doc := etree.NewDocument()
	doc.SetRoot(req.ResponseEl)
	raw, err := doc.WriteToBytes()
	if err != nil {
		t.Fatalf("WriteToBytes: %v", err)
	}
	return base64.StdEncoding.EncodeToString(raw)
}

func samlProvider(idpCertPEM string) domain.IdentityProvider {
	return domain.IdentityProvider{
		Type: "saml", IDPEntityID: tIDPEntityID, IDPSSOURL: tSSOURL,
		IDPCertificate: idpCertPEM, SPEntityID: tSPEntityID,
	}
}

func acsPost(t *testing.T, samlResponse string) *http.Request {
	t.Helper()
	form := url.Values{"SAMLResponse": {samlResponse}, "RelayState": {"rs"}}
	r := httptest.NewRequest(http.MethodPost, tACSURL, strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if err := r.ParseForm(); err != nil {
		t.Fatalf("ParseForm: %v", err)
	}
	return r
}

// --- La matriz de validación de la Response ---

func TestAuthenticate_Valid(t *testing.T) {
	key, cert, pemStr := genKeyCert(t)
	prov := samlProvider(pemStr)
	sp, err := BuildSP(prov, tACSURL, tSPEntityID)
	if err != nil {
		t.Fatalf("BuildSP: %v", err)
	}
	resp := mintResponse(t, key, cert, tIDPEntityID, tSPEntityID, tACSURL, tRequestID,
		time.Now(), "ada@acme.example", testAttrs("ada@acme.example", []string{"eng", "ops"}))

	claims, err := sp.Authenticate(acsPost(t, resp), []string{tRequestID}, prov)
	if err != nil {
		t.Fatalf("Authenticate válida falló: %v", err)
	}
	if claims.Email != "ada@acme.example" {
		t.Errorf("email=%q", claims.Email)
	}
	if claims.Subject != "ada@acme.example" {
		t.Errorf("subject=%q", claims.Subject)
	}
	if !claims.EmailVerified {
		t.Error("una assertion firmada debe contar como email verificado")
	}
	if strings.Join(claims.Groups, ",") != "eng,ops" {
		t.Errorf("groups=%v", claims.Groups)
	}
	if claims.Name != "Ada Lovelace" {
		t.Errorf("name=%q", claims.Name)
	}
}

func TestAuthenticate_BadSignature(t *testing.T) {
	key, cert, _ := genKeyCert(t)   // firma con este par…
	_, _, otherPEM := genKeyCert(t) // …pero el provider confía en OTRO certificado.
	prov := samlProvider(otherPEM)
	sp, _ := BuildSP(prov, tACSURL, tSPEntityID)
	resp := mintResponse(t, key, cert, tIDPEntityID, tSPEntityID, tACSURL, tRequestID,
		time.Now(), "ada@acme.example", testAttrs("ada@acme.example", nil))

	if _, err := sp.Authenticate(acsPost(t, resp), []string{tRequestID}, prov); err == nil {
		t.Fatal("firma inválida (cert que no firmó) debería RECHAZARSE")
	}
}

func TestAuthenticate_WrongAudience(t *testing.T) {
	key, cert, pemStr := genKeyCert(t)
	prov := samlProvider(pemStr) // audiencia esperada = tSPEntityID
	sp, _ := BuildSP(prov, tACSURL, tSPEntityID)
	// Acuñada para OTRA audiencia.
	resp := mintResponse(t, key, cert, tIDPEntityID, "https://otro.sp/metadata", tACSURL, tRequestID,
		time.Now(), "ada@acme.example", testAttrs("ada@acme.example", nil))

	if _, err := sp.Authenticate(acsPost(t, resp), []string{tRequestID}, prov); err == nil {
		t.Fatal("audiencia distinta del SP debería RECHAZARSE")
	}
}

func TestAuthenticate_Expired(t *testing.T) {
	key, cert, pemStr := genKeyCert(t)
	prov := samlProvider(pemStr)
	sp, _ := BuildSP(prov, tACSURL, tSPEntityID)
	// Emitida hace 10 min: NotOnOrAfter (≈ +90s) quedó muy en el pasado.
	resp := mintResponse(t, key, cert, tIDPEntityID, tSPEntityID, tACSURL, tRequestID,
		time.Now().Add(-10*time.Minute), "ada@acme.example", testAttrs("ada@acme.example", nil))

	if _, err := sp.Authenticate(acsPost(t, resp), []string{tRequestID}, prov); err == nil {
		t.Fatal("assertion expirada debería RECHAZARSE")
	}
}

func TestAuthenticate_UnknownInResponseTo(t *testing.T) {
	key, cert, pemStr := genKeyCert(t)
	prov := samlProvider(pemStr)
	sp, _ := BuildSP(prov, tACSURL, tSPEntityID)
	resp := mintResponse(t, key, cert, tIDPEntityID, tSPEntityID, tACSURL, tRequestID,
		time.Now(), "ada@acme.example", testAttrs("ada@acme.example", nil))

	// El InResponseTo emitido (tRequestID) NO está entre los request-IDs esperados.
	if _, err := sp.Authenticate(acsPost(t, resp), []string{"id-otro-request"}, prov); err == nil {
		t.Fatal("InResponseTo desconocido debería RECHAZARSE (anti-replay)")
	}
}

func TestAuthenticate_WrongRecipient(t *testing.T) {
	key, cert, pemStr := genKeyCert(t)
	prov := samlProvider(pemStr)
	sp, _ := BuildSP(prov, tACSURL, tSPEntityID)
	// Recipient/Destination apuntan a OTRO ACS, distinto del SP.
	resp := mintResponse(t, key, cert, tIDPEntityID, tSPEntityID, "https://id.test/saml/otro/acs",
		tRequestID, time.Now(), "ada@acme.example", testAttrs("ada@acme.example", nil))

	if _, err := sp.Authenticate(acsPost(t, resp), []string{tRequestID}, prov); err == nil {
		t.Fatal("Recipient/Destination distinto del ACS debería RECHAZARSE")
	}
}

// --- metadata, AuthnRequest, parseo de metadata, validación de config ---

func TestBuildSP_Metadata(t *testing.T) {
	_, _, pemStr := genKeyCert(t)
	sp, err := BuildSP(samlProvider(pemStr), tACSURL, tSPEntityID)
	if err != nil {
		t.Fatalf("BuildSP: %v", err)
	}
	md, err := sp.MetadataXML()
	if err != nil {
		t.Fatalf("MetadataXML: %v", err)
	}
	s := string(md)
	if !strings.Contains(s, tSPEntityID) {
		t.Errorf("metadata sin entityID del SP:\n%s", s)
	}
	if !strings.Contains(s, tACSURL) {
		t.Errorf("metadata sin ACS URL:\n%s", s)
	}
}

func TestMakeRedirect(t *testing.T) {
	_, _, pemStr := genKeyCert(t)
	sp, _ := BuildSP(samlProvider(pemStr), tACSURL, tSPEntityID)
	reqID, redirect, err := sp.MakeRedirect("relay-1")
	if err != nil {
		t.Fatalf("MakeRedirect: %v", err)
	}
	if !strings.HasPrefix(reqID, "id-") {
		t.Errorf("request id inesperado: %q", reqID)
	}
	u, err := url.Parse(redirect)
	if err != nil || u.Host != "idp.example.com" {
		t.Fatalf("URL de redirección mal: %v (%q)", err, redirect)
	}
	if u.Query().Get("SAMLRequest") == "" {
		t.Error("la URL de redirección no lleva SAMLRequest")
	}
}

func TestParseMetadataXML(t *testing.T) {
	_, cert, _ := genKeyCert(t)
	ed := &csaml.EntityDescriptor{EntityID: tIDPEntityID}
	var idp csaml.IDPSSODescriptor
	idp.SingleSignOnServices = []csaml.Endpoint{{Binding: csaml.HTTPRedirectBinding, Location: tSSOURL}}
	idp.KeyDescriptors = []csaml.KeyDescriptor{{
		Use: "signing",
		KeyInfo: csaml.KeyInfo{X509Data: csaml.X509Data{
			X509Certificates: []csaml.X509Certificate{{Data: base64.StdEncoding.EncodeToString(cert.Raw)}},
		}},
	}}
	ed.IDPSSODescriptors = []csaml.IDPSSODescriptor{idp}
	raw, err := xml.Marshal(ed)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	eid, sso, certPEM, err := ParseMetadataXML(raw)
	if err != nil {
		t.Fatalf("ParseMetadataXML: %v", err)
	}
	if eid != tIDPEntityID || sso != tSSOURL {
		t.Errorf("eid=%q sso=%q", eid, sso)
	}
	if !strings.Contains(certPEM, "BEGIN CERTIFICATE") {
		t.Errorf("cert no es PEM: %q", certPEM)
	}
	// El PEM extraído debe reconstruir el mismo DER.
	if _, err := certDER(certPEM); err != nil {
		t.Errorf("cert extraído no valida: %v", err)
	}
}

func TestValidateConfig(t *testing.T) {
	_, _, pemStr := genKeyCert(t)
	if err := ValidateConfig(samlProvider(pemStr)); err != nil {
		t.Errorf("config válida rechazada: %v", err)
	}
	bad := samlProvider("no-es-un-certificado")
	if err := ValidateConfig(bad); err == nil {
		t.Error("cert inválido debería rechazarse")
	}
	httpSSO := samlProvider(pemStr)
	httpSSO.IDPSSOURL = "http://idp.example.com/sso" // no https
	if err := ValidateConfig(httpSSO); err == nil {
		t.Error("SSO URL http (no https) debería rechazarse")
	}
	noEntity := samlProvider(pemStr)
	noEntity.IDPEntityID = ""
	if err := ValidateConfig(noEntity); err == nil {
		t.Error("EntityID vacío debería rechazarse")
	}
}
