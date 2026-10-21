// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package saml implementa el lado Service Provider (SP) de Sytue ID para
// federación SAML 2.0: una empresa con Microsoft EntraID, ADFS u Okta federa su
// login corporativo por SAML contra Sytue. Sytue ID emite el AuthnRequest, publica
// su metadata de SP y consume la Response en el ACS.
//
// Clean-room sobre SAML 2.0 (OASIS). La validación XML-DSig (firma de la Response/
// Assertion), las Conditions (NotBefore/NotOnOrAfter), la AudienceRestriction, el
// Recipient del SubjectConfirmation y el InResponseTo la delega ÍNTEGRAMENTE en
// github.com/crewjam/saml (MIT, la lib SP de Go estándar, sobre russellhaering/
// goxmldsig): NO se rueda cripto XML a mano. Esta capa solo construye el SP a partir
// de la config del provider, extrae/mapea atributos y aísla a crewjam del resto del
// servicio (todo el uso de la librería vive aquí). El parseo XML usa encoding/xml de
// la stdlib, que NO expande entidades externas (XXE-safe por defecto), y crewjam
// valida el well-formedness con xml-roundtrip-validator antes de firmar.
package saml

import (
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"encoding/xml"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	csaml "github.com/crewjam/saml"

	"git.sytue.eu/sytue/one/services/id/internal/domain"
)

// Errores públicos (envueltos; no filtran detalle del IdP hacia el navegador).
var (
	ErrConfig    = errors.New("saml: configuración del proveedor inválida")
	ErrMetadata  = errors.New("saml: metadata del IdP ilegible")
	ErrAssertion = errors.New("saml: assertion inválida")
)

// certWS colapsa espacios/saltos dentro de un certificado base64.
var certWS = regexp.MustCompile(`\s+`)

// SP envuelve un crewjam ServiceProvider para un provider concreto. Toda la
// validación criptográfica de la Response la ejecuta crewjam (ParseResponse);
// esta capa no toca la firma ni las condiciones.
type SP struct {
	inner *csaml.ServiceProvider
}

// BuildSP construye el SP de un provider SAML: EntityID/audience del SP, ACS y
// metadata URLs propias, y el IDPMetadata (EntityID + SSO URL + cert de firma) del
// que crewjam saca la clave para validar la firma y la identidad del emisor.
//
// AllowIDPInitiated=false: solo se admiten flujos SP-initiated, de modo que la
// Response DEBE traer un InResponseTo que casemos contra un AuthnRequest emitido
// (single-use, anti-replay). Sin clave de SP: los AuthnRequest van SIN firmar
// (interoperable con EntraID/Okta/ADFS por defecto); lo crítico —validar la firma
// de la assertion ENTRANTE— no depende de ello.
func BuildSP(p domain.IdentityProvider, acsURL, metadataURL string) (*SP, error) {
	acs, err := url.Parse(acsURL)
	if err != nil {
		return nil, fmt.Errorf("%w: ACS URL", ErrConfig)
	}
	meta, err := url.Parse(metadataURL)
	if err != nil {
		return nil, fmt.Errorf("%w: metadata URL", ErrConfig)
	}
	entityID := strings.TrimSpace(p.SPEntityID)
	if entityID == "" {
		entityID = metadataURL // por defecto la metadata URL (estable, derivada del id)
	}
	idp, err := idpDescriptor(p)
	if err != nil {
		return nil, err
	}
	return &SP{inner: &csaml.ServiceProvider{
		EntityID:          entityID,
		AcsURL:            *acs,
		MetadataURL:       *meta,
		IDPMetadata:       idp,
		AuthnNameIDFormat: csaml.NameIDFormat(strings.TrimSpace(p.NameIDFormat)),
		AllowIDPInitiated: false,
	}}, nil
}

// idpDescriptor materializa el EntityDescriptor del IdP a partir de los campos
// almacenados (entityID + SSO URL + cert PEM). Tolera SSO URL o cert vacíos (p. ej.
// para el endpoint de metadata de un provider aún sin configurar): el flujo real
// exige Configured() y crewjam rechaza una Response sin cert de firma.
func idpDescriptor(p domain.IdentityProvider) (*csaml.EntityDescriptor, error) {
	var idpSSO csaml.IDPSSODescriptor
	if loc := strings.TrimSpace(p.IDPSSOURL); loc != "" {
		idpSSO.SingleSignOnServices = []csaml.Endpoint{
			{Binding: csaml.HTTPRedirectBinding, Location: loc},
			{Binding: csaml.HTTPPostBinding, Location: loc},
		}
	}
	if pemStr := strings.TrimSpace(p.IDPCertificate); pemStr != "" {
		der, err := certDER(pemStr)
		if err != nil {
			return nil, err
		}
		idpSSO.KeyDescriptors = []csaml.KeyDescriptor{{
			Use: "signing",
			KeyInfo: csaml.KeyInfo{
				X509Data: csaml.X509Data{
					X509Certificates: []csaml.X509Certificate{{Data: der}},
				},
			},
		}}
	}
	return &csaml.EntityDescriptor{
		EntityID:          strings.TrimSpace(p.IDPEntityID),
		IDPSSODescriptors: []csaml.IDPSSODescriptor{idpSSO},
	}, nil
}

// MetadataXML devuelve la metadata del SP en XML (entityID, ACS URL) para que el
// administrador la registre en su IdP. No incluye secretos.
func (s *SP) MetadataXML() ([]byte, error) {
	out, err := xml.MarshalIndent(s.inner.Metadata(), "", "  ")
	if err != nil {
		return nil, err
	}
	return append([]byte(xml.Header), out...), nil
}

// MakeRedirect genera un AuthnRequest y lo prepara para HTTP-Redirect: devuelve el
// ID del request (a persistir para el anti-replay del InResponseTo) y la URL del IdP
// con el SAMLRequest desinflado+base64. RelayState viaja como parámetro.
func (s *SP) MakeRedirect(relayState string) (requestID, redirectURL string, err error) {
	loc := s.inner.GetSSOBindingLocation(csaml.HTTPRedirectBinding)
	if loc == "" {
		return "", "", fmt.Errorf("%w: sin endpoint SSO", ErrConfig)
	}
	req, err := s.inner.MakeAuthenticationRequest(loc, csaml.HTTPRedirectBinding, csaml.HTTPPostBinding)
	if err != nil {
		return "", "", err
	}
	u, err := req.Redirect(relayState, s.inner)
	if err != nil {
		return "", "", err
	}
	return req.ID, u.String(), nil
}

// MakePost genera un AuthnRequest para HTTP-POST: devuelve el ID del request y un
// formulario HTML autoenviable hacia el IdP (con SAMLRequest + RelayState).
func (s *SP) MakePost(relayState string) (requestID string, form []byte, err error) {
	loc := s.inner.GetSSOBindingLocation(csaml.HTTPPostBinding)
	if loc == "" {
		return "", nil, fmt.Errorf("%w: sin endpoint SSO", ErrConfig)
	}
	req, err := s.inner.MakeAuthenticationRequest(loc, csaml.HTTPPostBinding, csaml.HTTPPostBinding)
	if err != nil {
		return "", nil, err
	}
	return req.ID, req.Post(relayState), nil
}

// Authenticate valida la SAML Response del ACS con crewjam (firma XML-DSig contra el
// cert del IdP, Conditions NotBefore/NotOnOrAfter, AudienceRestriction==SPEntityID,
// Recipient==ACS, e InResponseTo ∈ requestIDs) y extrae los atributos mapeados. El
// llamante DEBE haber ejecutado r.ParseForm() antes (crewjam lee req.PostForm).
func (s *SP) Authenticate(r *http.Request, requestIDs []string, p domain.IdentityProvider) (domain.FederatedClaims, error) {
	assertion, err := s.inner.ParseResponse(r, requestIDs)
	if err != nil {
		// crewjam devuelve *InvalidResponseError con Error() estático: no filtra detalle.
		return domain.FederatedClaims{}, fmt.Errorf("%w: %w", ErrAssertion, err)
	}
	return extractClaims(p, assertion), nil
}

// extractClaims saca el NameID (sujeto externo) y mapea email/name/groups desde los
// atributos de la assertion. EmailVerified=true: una assertion FIRMADA por el IdP de
// la organización es la verificación del correo (equivale a email_verified=true en
// OIDC). El anti account-takeover NO se relaja por ello: resolveFederatedUser sigue
// acotando el enlace/JIT a miembros de la org del provider (ver federated.go).
func extractClaims(p domain.IdentityProvider, a *csaml.Assertion) domain.FederatedClaims {
	fc := domain.FederatedClaims{EmailVerified: true}
	if a.Subject != nil && a.Subject.NameID != nil {
		fc.Subject = strings.TrimSpace(a.Subject.NameID.Value)
	}
	attrs := collectAttributes(a)
	fc.Email = domain.NormalizeEmail(firstAttr(attrs, candidates(p, "email", emailAliases)))
	fc.Name = firstAttr(attrs, candidates(p, "name", nameAliases))
	fc.Groups = allAttr(attrs, candidates(p, "groups", groupAliases))
	// Sin atributo email pero NameID con formato emailAddress ⇒ úsalo como correo.
	if fc.Email == "" && a.Subject != nil && a.Subject.NameID != nil &&
		strings.Contains(fc.Subject, "@") &&
		(a.Subject.NameID.Format == "" || strings.Contains(a.Subject.NameID.Format, "emailAddress")) {
		fc.Email = domain.NormalizeEmail(fc.Subject)
	}
	return fc
}

// collectAttributes indexa los valores de la assertion por Name y por FriendlyName.
func collectAttributes(a *csaml.Assertion) map[string][]string {
	m := map[string][]string{}
	for _, st := range a.AttributeStatements {
		for _, attr := range st.Attributes {
			var vals []string
			for _, v := range attr.Values {
				if s := strings.TrimSpace(v.Value); s != "" {
					vals = append(vals, s)
				}
			}
			if len(vals) == 0 {
				continue
			}
			if attr.Name != "" {
				m[attr.Name] = append(m[attr.Name], vals...)
			}
			if attr.FriendlyName != "" && attr.FriendlyName != attr.Name {
				m[attr.FriendlyName] = append(m[attr.FriendlyName], vals...)
			}
		}
	}
	return m
}

// Aliases habituales de atributo por IdP (EntraID/ADFS/Okta) para cada campo lógico,
// usados cuando el admin no mapeó explícitamente. Si mapeó, se usa SOLO su nombre.
var (
	emailAliases = []string{
		"email", "mail", "emailAddress", "Email", "E-Mail",
		"http://schemas.xmlsoap.org/ws/2005/05/identity/claims/emailaddress",
		"urn:oid:0.9.2342.19200300.100.1.3",
	}
	nameAliases = []string{
		"name", "displayName", "displayname", "cn",
		"http://schemas.xmlsoap.org/ws/2005/05/identity/claims/name",
		"http://schemas.xmlsoap.org/ws/2005/05/identity/claims/displayname",
		"urn:oid:2.16.840.1.113730.3.1.241",
	}
	groupAliases = []string{
		"groups", "Groups", "memberOf",
		"http://schemas.xmlsoap.org/claims/Group",
		"http://schemas.microsoft.com/ws/2008/06/identity/claims/groups",
	}
)

// candidates devuelve los nombres de atributo a probar para un campo lógico: el
// mapeo explícito del admin (y solo ese) o la lista de aliases habituales.
func candidates(p domain.IdentityProvider, field string, aliases []string) []string {
	if v, ok := p.Mapping[field]; ok {
		if v = strings.TrimSpace(v); v != "" {
			return []string{v}
		}
	}
	return aliases
}

func firstAttr(m map[string][]string, keys []string) string {
	for _, k := range keys {
		if vs := m[k]; len(vs) > 0 {
			return vs[0]
		}
	}
	return ""
}

func allAttr(m map[string][]string, keys []string) []string {
	for _, k := range keys {
		if vs := m[k]; len(vs) > 0 {
			return vs
		}
	}
	return nil
}

// ParseMetadataXML extrae de la metadata XML de un IdP los tres datos que Sytue ID
// necesita para federar por SAML: EntityID, SingleSignOnService URL (prefiere
// HTTP-Redirect) y certificado de firma (PEM). Camino de conveniencia del admin:
// pegar la metadata en vez de teclear los tres campos. XXE-safe (encoding/xml).
func ParseMetadataXML(data []byte) (entityID, ssoURL, certPEM string, err error) {
	ed, err := parseEntityDescriptor(data)
	if err != nil {
		return "", "", "", err
	}
	entityID = strings.TrimSpace(ed.EntityID)
	for _, idp := range ed.IDPSSODescriptors {
		for _, ep := range idp.SingleSignOnServices {
			if ep.Binding == csaml.HTTPRedirectBinding {
				ssoURL = ep.Location
			}
		}
		if ssoURL == "" && len(idp.SingleSignOnServices) > 0 {
			ssoURL = idp.SingleSignOnServices[0].Location
		}
		for _, kd := range idp.KeyDescriptors {
			if kd.Use != "" && kd.Use != "signing" {
				continue
			}
			for _, c := range kd.KeyInfo.X509Data.X509Certificates {
				if raw := certWS.ReplaceAllString(c.Data, ""); raw != "" {
					certPEM = derToPEM(raw)
					break
				}
			}
			if certPEM != "" {
				break
			}
		}
	}
	if entityID == "" || ssoURL == "" || certPEM == "" {
		return "", "", "", fmt.Errorf("%w: falta entityID, SSO URL o certificado de firma", ErrMetadata)
	}
	return entityID, strings.TrimSpace(ssoURL), certPEM, nil
}

// parseEntityDescriptor admite una EntityDescriptor suelta o un bundle
// EntitiesDescriptor (toma la primera entidad con IDPSSODescriptor).
func parseEntityDescriptor(data []byte) (*csaml.EntityDescriptor, error) {
	var ed csaml.EntityDescriptor
	if err := xml.Unmarshal(data, &ed); err == nil && len(ed.IDPSSODescriptors) > 0 {
		return &ed, nil
	}
	var eds csaml.EntitiesDescriptor
	if err := xml.Unmarshal(data, &eds); err != nil {
		return nil, fmt.Errorf("%w: XML ilegible", ErrMetadata)
	}
	for i := range eds.EntityDescriptors {
		if len(eds.EntityDescriptors[i].IDPSSODescriptors) > 0 {
			return &eds.EntityDescriptors[i], nil
		}
	}
	return nil, fmt.Errorf("%w: sin IDPSSODescriptor", ErrMetadata)
}

// ValidateConfig comprueba la config SAML de un provider antes de guardarla/activarla:
// EntityID presente, SSO URL https y certificado de firma que parsea. No hace red.
func ValidateConfig(p domain.IdentityProvider) error {
	if strings.TrimSpace(p.IDPEntityID) == "" {
		return fmt.Errorf("%w: falta el EntityID del IdP", ErrConfig)
	}
	u, perr := url.Parse(strings.TrimSpace(p.IDPSSOURL))
	if perr != nil || u.Scheme != "https" || u.Host == "" {
		return fmt.Errorf("%w: la URL de SSO del IdP debe ser https", ErrConfig)
	}
	if _, err := certDER(strings.TrimSpace(p.IDPCertificate)); err != nil {
		return err
	}
	return nil
}

// certDER decodifica un certificado (PEM o base64 DER pelado), valida que parsea
// como x509 y devuelve su DER en base64 (la forma que crewjam espera).
func certDER(cert string) (string, error) {
	if block, _ := pem.Decode([]byte(cert)); block != nil {
		if _, err := x509.ParseCertificate(block.Bytes); err != nil {
			return "", fmt.Errorf("%w: certificado ilegible", ErrConfig)
		}
		return base64.StdEncoding.EncodeToString(block.Bytes), nil
	}
	der, err := base64.StdEncoding.DecodeString(certWS.ReplaceAllString(cert, ""))
	if err != nil {
		return "", fmt.Errorf("%w: certificado no es PEM ni base64", ErrConfig)
	}
	if _, err := x509.ParseCertificate(der); err != nil {
		return "", fmt.Errorf("%w: certificado ilegible", ErrConfig)
	}
	return base64.StdEncoding.EncodeToString(der), nil
}

// derToPEM envuelve un DER en base64 como certificado PEM.
func derToPEM(b64der string) string {
	der, err := base64.StdEncoding.DecodeString(b64der)
	if err != nil {
		return ""
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
}
