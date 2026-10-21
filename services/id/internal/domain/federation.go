// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"strings"
	"time"
)

// IdentityProvider es un IdP OIDC upstream configurado por una organización para
// login federado (Sytue ID actúa de broker/RP). El ClientSecret viaja descifrado
// solo dentro del proceso (la capa store lo cifra/descifra en reposo con la KEK);
// nunca se serializa hacia el navegador.
type IdentityProvider struct {
	ID           string
	OrgID        string
	Type         string // 'oidc'
	Name         string
	Slug         string
	Issuer       string
	ClientID     string
	ClientSecret string // '' para client público (PKCE puro)
	Scopes       string // separados por espacio; por defecto "openid email profile"
	// Mapping son los nombres de claim del upstream para los atributos de Sytue
	// (email, name, groups). Vacío ⇒ se usan los nombres OIDC estándar.
	Mapping             map[string]string
	AllowedEmailDomains []string
	// Endpoints descubiertos (cacheados del discovery del issuer). OIDC.
	AuthorizationEndpoint string
	TokenEndpoint         string
	UserinfoEndpoint      string
	JWKSURI               string
	PKCE                  bool // el upstream soporta code_challenge S256
	// --- SAML 2.0 (Type=="saml"): Sytue ID actúa de SP. Vacíos en OIDC. ---
	IDPEntityID    string // EntityID del IdP == Issuer de la assertion (crewjam lo valida)
	IDPSSOURL      string // SingleSignOnService del IdP: destino del AuthnRequest
	IDPCertificate string // certificado de firma del IdP en PEM (PÚBLICO, no secreto)
	SPEntityID     string // EntityID/audience del SP; vacío ⇒ la metadata URL del provider
	SAMLBinding    string // 'redirect' | 'post' — binding del AuthnRequest hacia el IdP
	NameIDFormat   string // NameIDPolicy solicitado; vacío ⇒ defecto de la librería (transient)
	Enabled        bool
	CreatedBy      string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// IsSAML indica si el provider federa por SAML 2.0 (Sytue ID como SP).
func (p IdentityProvider) IsSAML() bool { return p.Type == "saml" }

// Configured indica si el provider puede iniciar un flujo. OIDC: endpoints
// resueltos por discovery. SAML: EntityID + SSO URL + certificado de firma del IdP.
func (p IdentityProvider) Configured() bool {
	if p.IsSAML() {
		return p.IDPEntityID != "" && p.IDPSSOURL != "" && p.IDPCertificate != ""
	}
	return p.AuthorizationEndpoint != "" && p.TokenEndpoint != "" && p.JWKSURI != ""
}

// ClaimName devuelve el nombre de claim del upstream para un atributo lógico
// (email|name|groups), cayendo al nombre OIDC estándar si no hay mapeo explícito.
func (p IdentityProvider) ClaimName(field string) string {
	if v, ok := p.Mapping[field]; ok {
		if v = strings.TrimSpace(v); v != "" {
			return v
		}
	}
	return field // los nombres lógicos coinciden con los claims estándar OIDC
}

// DomainAllowed indica si el email cae dentro de allowed_email_domains. Lista
// vacía ⇒ sin restricción (true). La comparación es case-insensitive sobre el
// dominio. Es una barrera anti-abuso: acota a qué correos puede aprovisionar o
// enlazar este provider.
func (p IdentityProvider) DomainAllowed(email string) bool {
	if len(p.AllowedEmailDomains) == 0 {
		return true
	}
	at := strings.LastIndexByte(email, '@')
	if at < 0 || at == len(email)-1 {
		return false
	}
	dom := strings.ToLower(email[at+1:])
	for _, d := range p.AllowedEmailDomains {
		if strings.ToLower(strings.TrimSpace(d)) == dom {
			return true
		}
	}
	return false
}

// FederatedClaims son los atributos extraídos y mapeados del id_token/userinfo
// del IdP upstream tras validar la firma, iss, aud, exp y nonce.
type FederatedClaims struct {
	Subject       string // `sub` del upstream (identidad externa estable)
	Email         string
	EmailVerified bool
	Name          string
	Groups        []string
}

// NormalizeEmail baja a minúsculas y recorta un email (misma normalización que el
// login por contraseña; la unicidad en BD es case-insensitive).
//
// Se queda con el PRIMER token si llegan varios. Un email no lleva espacios, y
// hay gestores de contraseñas que rellenan el mismo campo dos veces sin limpiar
// lo anterior: el formulario llega con «correo@dominio correo@dominio». Ese valor
// no casa con ninguna cuenta, así que el acceso fallaba con «email o contraseña
// incorrectos» y mandaba a buscar el problema a la contraseña, que estaba bien.
func NormalizeEmail(email string) string {
	if f := strings.Fields(email); len(f) > 0 {
		return strings.ToLower(f[0])
	}
	return ""
}
