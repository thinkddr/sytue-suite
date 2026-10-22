// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Este fichero implementa el lado IDENTITY PROVIDER (IdP) de Sytue ID: emitir
// Assertions SAML 2.0 FIRMADAS hacia SP de terceros (SSO saliente / 1-click). Es el
// espejo de saml.go (el lado SP): AÍSLA github.com/crewjam/saml aquí — todo el uso de
// la librería del lado IdP vive en este fichero; la capa HTTP nunca toca crewjam.
//
// La firma XML-DSig (a nivel de ELEMENTO Assertion y de Response) la ejecuta crewjam
// sobre goxmldsig; NO se rueda cripto XML a mano. Cada tenant es su propio IdP con su
// propia clave de firma (per-org, sellada en reposo): esta capa solo construye la
// Response a partir de la config del SP y la identidad del sujeto de la sesión. El
// parseo del AuthnRequest/LogoutRequest entrante usa encoding/xml de la stdlib
// (XXE-safe: no expande entidades externas) más el decodificador de crewjam
// (base64+inflate con guarda anti zip-bomb) para el transporte.
package saml

import (
	"bytes"
	"compress/flate"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/pem"
	"encoding/xml"
	"fmt"
	"math/big"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/beevik/etree"
	csaml "github.com/crewjam/saml"
	dsig "github.com/russellhaering/goxmldsig"

	"git.sytue.eu/sytue/one/services/id/internal/domain"
)

// Formatos de NameID soportados (OASIS SAML 2.0). ” ⇒ emailAddress por defecto.
const (
	NameIDEmail       = "urn:oasis:names:tc:SAML:1.1:nameid-format:emailAddress"
	NameIDPersistent  = "urn:oasis:names:tc:SAML:2.0:nameid-format:persistent"
	NameIDTransient   = "urn:oasis:names:tc:SAML:2.0:nameid-format:transient"
	NameIDUnspecified = "urn:oasis:names:tc:SAML:1.1:nameid-format:unspecified"
)

// idpKeyValidity es la vida del certificado autofirmado del IdP (largo: la clave se
// conserva por-org y se rota aparte; no debe caducar en el ciclo normal).
const idpKeyValidity = 10 * 365 * 24 * time.Hour

// GenerateIDPKeyPEM genera el par de firma del IdP de una organización: una clave RSA
// 2048 y un certificado X.509 autofirmado (subject = commonName). Devuelve la clave en
// PEM PKCS#1 (que el store SELLA en reposo) y el certificado en PEM (público). Solo se
// invoca la 1ª vez que una org emite/expone metadata (generación perezosa).
func GenerateIDPKeyPEM(commonName string) (privPEM, certPEM string, err error) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return "", "", fmt.Errorf("saml idp: generar clave: %w", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		Subject:      pkix.Name{CommonName: commonName},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(idpKeyValidity),
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return "", "", fmt.Errorf("saml idp: certificado: %w", err)
	}
	privPEM = string(pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}))
	certPEM = string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
	return privPEM, certPEM, nil
}

// IDP envuelve un crewjam IdentityProvider ya cargado con la clave+cert de UNA
// organización y sus URLs (entityID/SSO/SLO). Toda la firma la ejecuta crewjam.
type IDP struct {
	inner *csaml.IdentityProvider
}

// NewIDP construye el IdP de una org a partir de su clave+cert (PEM, ya descifrados)
// y sus URLs públicas. entityID es el EntityID/Issuer del IdP (== metadata URL, único
// por org); ssoURL/sloURL son los endpoints propios. Firma con RSA-SHA256.
func NewIDP(privPEM, certPEM, entityID, ssoURL, sloURL string) (*IDP, error) {
	block, _ := pem.Decode([]byte(privPEM))
	if block == nil {
		return nil, fmt.Errorf("%w: clave IdP ilegible", ErrConfig)
	}
	key, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("%w: clave IdP inválida", ErrConfig)
	}
	cblock, _ := pem.Decode([]byte(certPEM))
	if cblock == nil {
		return nil, fmt.Errorf("%w: certificado IdP ilegible", ErrConfig)
	}
	cert, err := x509.ParseCertificate(cblock.Bytes)
	if err != nil {
		return nil, fmt.Errorf("%w: certificado IdP inválido", ErrConfig)
	}
	meta, err := url.Parse(entityID)
	if err != nil {
		return nil, fmt.Errorf("%w: entityID IdP", ErrConfig)
	}
	sso, err := url.Parse(ssoURL)
	if err != nil {
		return nil, fmt.Errorf("%w: SSO URL IdP", ErrConfig)
	}
	slo, _ := url.Parse(sloURL)
	if slo == nil {
		slo = &url.URL{}
	}
	return &IDP{inner: &csaml.IdentityProvider{
		Key:             key,
		Certificate:     cert,
		MetadataURL:     *meta,
		SSOURL:          *sso,
		LogoutURL:       *slo,
		SignatureMethod: dsig.RSASHA256SignatureMethod,
	}}, nil
}

// MetadataXML devuelve la metadata del IdP (EntityID, cert de firma, SSO/SLO URLs)
// para que el administrador del SP de terceros configure su lado. Pública, sin secretos.
func (idp *IDP) MetadataXML() ([]byte, error) {
	out, err := xml.MarshalIndent(idp.inner.Metadata(), "", "  ")
	if err != nil {
		return nil, err
	}
	return append([]byte(xml.Header), out...), nil
}

// AuthnRequestInfo son los campos extraídos de un AuthnRequest entrante (SP-initiated).
type AuthnRequestInfo struct {
	SPEntityID string // Issuer del AuthnRequest == EntityID del SP (para casar el registro)
	RequestID  string // ID del AuthnRequest (InResponseTo de la Response, anti-replay del SP)
	ACSURL     string // ACS solicitado (se valida contra el registrado)
	RelayState string
	Raw        []byte // XML crudo del AuthnRequest (para reanudar el flujo tras login)
}

// DecodeAuthnRequest decodifica un AuthnRequest entrante (binding HTTP-Redirect en GET
// —base64+inflate— o HTTP-POST —base64—) y extrae Issuer/ID/ACS/RelayState. NO exige
// la clave del IdP (el sujeto/org se resuelven de la SESIÓN, no del request). Rechaza
// versión != 2.0 o Issuer vacío. XXE-safe (encoding/xml de la stdlib).
func DecodeAuthnRequest(r *http.Request) (AuthnRequestInfo, error) {
	// crewjam.NewIdpAuthnRequest solo decodifica el transporte (base64/inflate con
	// guarda anti zip-bomb) y no deref-erencia la clave: un IdP vacío basta aquí.
	req, err := csaml.NewIdpAuthnRequest(&csaml.IdentityProvider{}, r)
	if err != nil {
		return AuthnRequestInfo{}, fmt.Errorf("%w: AuthnRequest ilegible", ErrAssertion)
	}
	var ar csaml.AuthnRequest
	//nolint:gosec // G709: encoding/xml sobre un buffer ya acotado por crewjam (anti zip-bomb); no hay deserialización de tipos arbitrarios.
	if err := xml.Unmarshal(req.RequestBuffer, &ar); err != nil {
		return AuthnRequestInfo{}, fmt.Errorf("%w: AuthnRequest malformado", ErrAssertion)
	}
	if ar.Version != "2.0" || ar.Issuer == nil || strings.TrimSpace(ar.Issuer.Value) == "" {
		return AuthnRequestInfo{}, fmt.Errorf("%w: AuthnRequest inválido", ErrAssertion)
	}
	return AuthnRequestInfo{
		SPEntityID: strings.TrimSpace(ar.Issuer.Value),
		RequestID:  ar.ID,
		ACSURL:     ar.AssertionConsumerServiceURL,
		RelayState: req.RelayState,
		Raw:        req.RequestBuffer,
	}, nil
}

// DeflateBase64 comprime (raw DEFLATE) y codifica en base64 un XML para transportarlo
// por el binding HTTP-Redirect (SAMLRequest). Se usa para REANUDAR un flujo SP-initiated
// tras el login: el AuthnRequest (POST o GET) se re-presenta como un GET uniforme.
func DeflateBase64(raw []byte) (string, error) {
	var buf bytes.Buffer
	w, err := flate.NewWriter(&buf, flate.BestCompression)
	if err != nil {
		return "", err
	}
	if _, err := w.Write(raw); err != nil {
		return "", err
	}
	if err := w.Close(); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(buf.Bytes()), nil
}

// UserIdentity es el sujeto autenticado (de la sesión SSO) cuyo NameID y atributos se
// vierten en la Assertion.
type UserIdentity struct {
	StableID string // UUID del usuario (NameID persistent / fallback)
	Email    string
	Name     string
	Groups   []string
}

// ResponseForm son los campos del binding HTTP-POST hacia el ACS del SP. La capa HTTP
// los renderiza en un formulario autoenviable con una CSP acotada (controla el script).
type ResponseForm struct {
	ACSURL       string
	SAMLResponse string // base64 de la Response firmada
	RelayState   string
}

// MakeResponse emite una SAML Response FIRMADA (Response + Assertion, firma a nivel de
// elemento) para un SP registrado. inResponseTo es el ID del AuthnRequest (SP-initiated)
// o ” (IdP-initiated, unsolicited). El NameID sigue sp.NameIDFormat y los atributos
// email/name/groups se nombran según sp.AttrMapping. La firma la ejecuta crewjam.
//
// r aporta el RemoteAddr para el SubjectLocality de la Assertion (crewjam lo exige).
func (idp *IDP) MakeResponse(r *http.Request, sp domain.SAMLServiceProvider, inResponseTo, relayState string, u UserIdentity) (ResponseForm, error) {
	if !sp.Configured() {
		return ResponseForm{}, fmt.Errorf("%w: SP sin EntityID/ACS", ErrConfig)
	}
	nameID, nameFmt := resolveNameID(sp.NameIDFormat, u)
	spMeta := &csaml.EntityDescriptor{
		EntityID:         sp.EntityID,
		SPSSODescriptors: []csaml.SPSSODescriptor{{}},
	}
	req := &csaml.IdpAuthnRequest{
		Now:                     time.Now(),
		IDP:                     idp.inner,
		HTTPRequest:             r,
		RelayState:              relayState,
		Request:                 csaml.AuthnRequest{ID: inResponseTo, IssueInstant: time.Now(), Version: "2.0"},
		ServiceProviderMetadata: spMeta,
		SPSSODescriptor:         &spMeta.SPSSODescriptors[0],
		ACSEndpoint:             &csaml.IndexedEndpoint{Binding: csaml.HTTPPostBinding, Location: sp.ACSURL},
	}
	assertMaker := csaml.DefaultAssertionMaker{}
	if err := assertMaker.MakeAssertion(req, &csaml.Session{
		NameID:           nameID,
		NameIDFormat:     nameFmt,
		CustomAttributes: attributes(sp, u),
	}); err != nil {
		return ResponseForm{}, fmt.Errorf("%w: %w", ErrAssertion, err)
	}
	// PostBinding llama a MakeResponse (firma Assertion y Response) y devuelve el form.
	form, err := req.PostBinding()
	if err != nil {
		return ResponseForm{}, fmt.Errorf("%w: %w", ErrAssertion, err)
	}
	return ResponseForm{ACSURL: form.URL, SAMLResponse: form.SAMLResponse, RelayState: form.RelayState}, nil
}

// resolveNameID elige el valor y formato del NameID según el formato solicitado por el
// SP y la identidad del sujeto.
//
// ponytail: 'persistent' usa el UUID global del usuario (estable y único), no un
// pairwise por-SP. Suficiente para SSO; si un SP exige no-correlación entre SP,
// derivar HMAC(userID, spEntityID).
func resolveNameID(format string, u UserIdentity) (value, outFormat string) {
	switch strings.TrimSpace(format) {
	case NameIDPersistent:
		return u.StableID, NameIDPersistent
	case NameIDTransient:
		if tok, err := domain.GenerateToken(16); err == nil {
			return tok, NameIDTransient
		}
		return u.StableID, NameIDTransient
	case NameIDEmail, "", NameIDUnspecified:
		if u.Email != "" {
			return u.Email, NameIDEmail
		}
		return u.StableID, NameIDUnspecified
	default:
		// Formato desconocido: emite el email con ese formato (interoperable).
		if u.Email != "" {
			return u.Email, format
		}
		return u.StableID, format
	}
}

// attributes construye los AttributeStatement de la Assertion (email/name/groups) con
// los nombres del mapeo del SP y NameFormat URI.
func attributes(sp domain.SAMLServiceProvider, u UserIdentity) []csaml.Attribute {
	const uri = "urn:oasis:names:tc:SAML:2.0:attrname-format:uri"
	out := make([]csaml.Attribute, 0, 3)
	add := func(field, val string) {
		if val == "" {
			return
		}
		out = append(out, csaml.Attribute{
			Name: sp.AttrName(field), NameFormat: uri,
			Values: []csaml.AttributeValue{{Type: "xs:string", Value: val}},
		})
	}
	add("email", u.Email)
	add("name", u.Name)
	if len(u.Groups) > 0 {
		gv := make([]csaml.AttributeValue, 0, len(u.Groups))
		for _, g := range u.Groups {
			gv = append(gv, csaml.AttributeValue{Type: "xs:string", Value: g})
		}
		out = append(out, csaml.Attribute{Name: sp.AttrName("groups"), NameFormat: uri, Values: gv})
	}
	return out
}

// LogoutRequestInfo son los campos extraídos de un LogoutRequest entrante (SLO).
type LogoutRequestInfo struct {
	SPEntityID string
	RequestID  string
}

// DecodeLogoutRequest decodifica un LogoutRequest entrante (binding Redirect/POST) y
// extrae Issuer/ID. XXE-safe.
func DecodeLogoutRequest(r *http.Request) (LogoutRequestInfo, error) {
	// Reusa el decodificador de transporte de crewjam (lee SAMLRequest).
	req, err := csaml.NewIdpAuthnRequest(&csaml.IdentityProvider{}, r)
	if err != nil {
		return LogoutRequestInfo{}, fmt.Errorf("%w: LogoutRequest ilegible", ErrAssertion)
	}
	var lr csaml.LogoutRequest
	//nolint:gosec // G709: ídem, buffer acotado por crewjam y destino de tipo fijo.
	if err := xml.Unmarshal(req.RequestBuffer, &lr); err != nil {
		return LogoutRequestInfo{}, fmt.Errorf("%w: LogoutRequest malformado", ErrAssertion)
	}
	if lr.Issuer == nil || strings.TrimSpace(lr.Issuer.Value) == "" {
		return LogoutRequestInfo{}, fmt.Errorf("%w: LogoutRequest sin Issuer", ErrAssertion)
	}
	return LogoutRequestInfo{SPEntityID: strings.TrimSpace(lr.Issuer.Value), RequestID: lr.ID}, nil
}

// MakeLogoutResponse produce un LogoutResponse FIRMADO (Status=Success) hacia el SP.
// SLO mínimo: consume el LogoutRequest y responde. Se entrega por HTTP-POST al ACS del
// SP (el único endpoint del SP que persistimos).
//
// ponytail: sin slo_url almacenada, el LogoutResponse se postea al ACS del SP.
// Upgrade: añadir columna slo_url + binding dedicado si un SP lo requiere.
func (idp *IDP) MakeLogoutResponse(sp domain.SAMLServiceProvider, inResponseTo, relayState string) (ResponseForm, error) {
	resp := csaml.LogoutResponse{
		ID:           fmt.Sprintf("id-%s", randHex()),
		InResponseTo: inResponseTo,
		Version:      "2.0",
		IssueInstant: time.Now(),
		Destination:  sp.ACSURL,
		Issuer: &csaml.Issuer{
			Format: "urn:oasis:names:tc:SAML:2.0:nameid-format:entity",
			Value:  idp.inner.MetadataURL.String(),
		},
		Status: csaml.Status{StatusCode: csaml.StatusCode{Value: csaml.StatusSuccess}},
	}
	el, err := idp.sign(resp.Element())
	if err != nil {
		return ResponseForm{}, err
	}
	doc := etree.NewDocument()
	doc.SetRoot(el)
	raw, err := doc.WriteToBytes()
	if err != nil {
		return ResponseForm{}, err
	}
	return ResponseForm{
		ACSURL:       sp.ACSURL,
		SAMLResponse: base64.StdEncoding.EncodeToString(raw),
		RelayState:   relayState,
	}, nil
}

// sign firma (XML-DSig enveloped) un elemento con la clave+cert del IdP (RSA-SHA256).
func (idp *IDP) sign(el *etree.Element) (*etree.Element, error) {
	keyPair := tls.Certificate{
		Certificate: [][]byte{idp.inner.Certificate.Raw},
		PrivateKey:  idp.inner.Key,
		Leaf:        idp.inner.Certificate,
	}
	ctx := dsig.NewDefaultSigningContext(dsig.TLSCertKeyStore(keyPair))
	if err := ctx.SetSignatureMethod(dsig.RSASHA256SignatureMethod); err != nil {
		return nil, err
	}
	return ctx.SignEnveloped(el)
}

func randHex() string {
	b := make([]byte, 20)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%x", b)
}
