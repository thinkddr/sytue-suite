// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package saml

import (
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/beevik/etree"
	csaml "github.com/crewjam/saml"
	dsig "github.com/russellhaering/goxmldsig"
)

// mintResponseMutada firma, como un IdP de verdad, una assertion que antes de firmar
// pasa por antes (la estructura) y despues (el XML): así se prueba lo que crewjam
// acepta con la firma en regla.
func mintResponseMutada(t *testing.T, key *rsa.PrivateKey, cert *x509.Certificate,
	antes func(*csaml.Assertion), despues func(*etree.Element),
) string {
	t.Helper()
	now := time.Now()
	metaURL, _ := url.Parse(tIDPEntityID)
	ssoURL, _ := url.Parse(tSSOURL)
	idp := &csaml.IdentityProvider{
		Key: key, Certificate: cert, MetadataURL: *metaURL, SSOURL: *ssoURL,
		SignatureMethod: dsig.RSASHA256SignatureMethod,
	}
	spMeta := &csaml.EntityDescriptor{EntityID: tSPEntityID, SPSSODescriptors: []csaml.SPSSODescriptor{{}}}
	req := &csaml.IdpAuthnRequest{
		Now: now, IDP: idp,
		HTTPRequest:             httptest.NewRequest(http.MethodPost, tSSOURL, http.NoBody),
		Request:                 csaml.AuthnRequest{ID: tRequestID, IssueInstant: now, Version: "2.0"},
		ServiceProviderMetadata: spMeta,
		SPSSODescriptor:         &spMeta.SPSSODescriptors[0],
		ACSEndpoint:             &csaml.IndexedEndpoint{Binding: csaml.HTTPPostBinding, Location: tACSURL},
	}
	if err := (csaml.DefaultAssertionMaker{}).MakeAssertion(req, &csaml.Session{
		NameID: "ada@acme.example", NameIDFormat: tEmailFormat,
		CustomAttributes: testAttrs("ada@acme.example", nil),
	}); err != nil {
		t.Fatalf("MakeAssertion: %v", err)
	}
	antes(req.Assertion)
	el := req.Assertion.Element()
	despues(el)
	ctx := dsig.NewDefaultSigningContext(dsig.TLSCertKeyStore(tls.Certificate{
		Certificate: [][]byte{cert.Raw}, PrivateKey: key, Leaf: cert,
	}))
	if err := ctx.SetSignatureMethod(dsig.RSASHA256SignatureMethod); err != nil {
		t.Fatal(err)
	}
	firmada, err := ctx.SignEnveloped(el)
	if err != nil {
		t.Fatalf("SignEnveloped: %v", err)
	}
	req.AssertionEl = firmada
	if err := req.MakeResponse(); err != nil {
		t.Fatalf("MakeResponse: %v", err)
	}
	doc := etree.NewDocument()
	doc.SetRoot(req.ResponseEl)
	raw, err := doc.WriteToBytes()
	if err != nil {
		t.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString(raw)
}

func subjectDe(el *etree.Element) *etree.Element {
	for _, c := range el.ChildElements() {
		if c.Tag == "Subject" {
			return c
		}
	}
	return nil
}

// GO-2026-6909: sin <saml:NameID> y con un NameID de OTRO espacio de nombres dentro
// del Subject, encoding/xml rellena Subject.NameID con el ajeno y crewjam lo da por
// bueno. Una assertion así, aunque venga firmada, no identifica a nadie.
func TestAuthenticate_NameIDDeOtroEspacioSeRechaza(t *testing.T) {
	key, cert, pemStr := genKeyCert(t)
	prov := samlProvider(pemStr)
	sp, _ := BuildSP(prov, tACSURL, tSPEntityID)
	resp := mintResponseMutada(t, key, cert, func(a *csaml.Assertion) { a.Subject.NameID = nil }, func(el *etree.Element) {
		ajeno := etree.NewElement("x:NameID")
		ajeno.CreateAttr("xmlns:x", "urn:otro")
		ajeno.SetText("jefa@acme.example")
		subjectDe(el).InsertChildAt(0, ajeno)
	})
	claims, err := sp.Authenticate(acsPost(t, resp), []string{tRequestID}, prov)
	if err == nil {
		t.Fatalf("un NameID de otro espacio de nombres debería RECHAZARSE; entró como %q", claims.Subject)
	}
}

// La misma assertion sin tocar sigue entrando: la comprobación no rompe el caso bueno.
func TestAuthenticate_NameIDDelEspacioSAMLEntra(t *testing.T) {
	key, cert, pemStr := genKeyCert(t)
	prov := samlProvider(pemStr)
	sp, _ := BuildSP(prov, tACSURL, tSPEntityID)
	resp := mintResponseMutada(t, key, cert, func(*csaml.Assertion) {}, func(*etree.Element) {})
	claims, err := sp.Authenticate(acsPost(t, resp), []string{tRequestID}, prov)
	if err != nil {
		t.Fatalf("assertion válida rechazada: %v", err)
	}
	if claims.Subject != "ada@acme.example" {
		t.Fatalf("sujeto %q", claims.Subject)
	}
}

// Sin <saml:Subject> crewjam desreferencia un nil y entra en pánico: el ACS tiene
// que contestar con un error, no tirar la petición.
func TestAuthenticate_SinSubjectNoEntraEnPanico(t *testing.T) {
	key, cert, pemStr := genKeyCert(t)
	prov := samlProvider(pemStr)
	sp, _ := BuildSP(prov, tACSURL, tSPEntityID)
	resp := mintResponseMutada(t, key, cert, func(*csaml.Assertion) {}, func(el *etree.Element) {
		if s := subjectDe(el); s != nil {
			el.RemoveChild(s)
		}
	})
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("pánico: %v", r)
		}
	}()
	if _, err := sp.Authenticate(acsPost(t, resp), []string{tRequestID}, prov); err == nil {
		t.Fatal("una assertion sin Subject debería RECHAZARSE")
	}
}

// Sin <saml:Issuer> en la assertion también se rechaza sin entrar en pánico.
func TestAuthenticate_SinIssuerNoEntraEnPanico(t *testing.T) {
	key, cert, pemStr := genKeyCert(t)
	prov := samlProvider(pemStr)
	sp, _ := BuildSP(prov, tACSURL, tSPEntityID)
	resp := mintResponseMutada(t, key, cert, func(*csaml.Assertion) {}, func(el *etree.Element) {
		for _, c := range el.ChildElements() {
			if c.Tag == "Issuer" {
				el.RemoveChild(c)
			}
		}
	})
	if _, err := sp.Authenticate(acsPost(t, resp), []string{tRequestID}, prov); err == nil {
		t.Fatal("una assertion sin Issuer debería RECHAZARSE")
	}
}
