// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"strings"
	"time"
)

// SAMLServiceProvider es un SP SAML de terceros registrado por una organización:
// Sytue ID actúa de IdP y le emite Assertions firmadas (SSO saliente). Todo campo
// es PÚBLICO (ni claves ni secretos): CertPEM es el certificado del SP (opcional,
// para cifrar/verificar), no un secreto propio.
type SAMLServiceProvider struct {
	ID    string
	OrgID string
	Name  string
	// EntityID es la audiencia del SP e Issuer de su AuthnRequest. Casa el flujo
	// SP-initiated con este registro, acotado a la org de la sesión.
	EntityID string
	// ACSURL es el Assertion Consumer Service (HTTP-POST): destino de la Response.
	ACSURL string
	// CertPEM es el certificado del SP en PEM (opcional). PÚBLICO.
	CertPEM string
	// NameIDFormat es el formato del NameID a emitir; '' ⇒ emailAddress.
	NameIDFormat string
	// AttrMapping son los nombres de atributo SAML para email/name/groups; vacío ⇒
	// se usan los nombres lógicos (email/name/groups).
	AttrMapping       map[string]string
	IDPInitiated      bool
	RelayStateDefault string
	Enabled           bool
	CreatedBy         string
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

// Configured indica si el SP puede recibir una Assertion (audiencia + ACS presentes).
func (p SAMLServiceProvider) Configured() bool {
	return strings.TrimSpace(p.EntityID) != "" && strings.TrimSpace(p.ACSURL) != ""
}

// AttrName devuelve el nombre de atributo SAML para un campo lógico (email|name|
// groups), cayendo al nombre lógico si no hay mapeo explícito.
func (p SAMLServiceProvider) AttrName(field string) string {
	if v, ok := p.AttrMapping[field]; ok {
		if v = strings.TrimSpace(v); v != "" {
			return v
		}
	}
	return field
}
