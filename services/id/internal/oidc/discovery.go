// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package oidc

import "git.sytue.eu/sytue/one/services/id/internal/domain"

// DiscoveryDocument es el documento de metadatos del proveedor OpenID
// (OIDC Discovery 1.0 / RFC 8414). Se sirve en /.well-known/openid-configuration.
type DiscoveryDocument struct {
	Issuer                                    string   `json:"issuer"`
	AuthorizationEndpoint                     string   `json:"authorization_endpoint"`
	TokenEndpoint                             string   `json:"token_endpoint"`
	IntrospectionEndpoint                     string   `json:"introspection_endpoint"`
	RevocationEndpoint                        string   `json:"revocation_endpoint"`
	UserinfoEndpoint                          string   `json:"userinfo_endpoint"`
	JWKSURI                                   string   `json:"jwks_uri"`
	EndSessionEndpoint                        string   `json:"end_session_endpoint"`
	RegistrationEndpoint                      string   `json:"registration_endpoint,omitempty"`
	ScopesSupported                           []string `json:"scopes_supported"`
	ResponseTypesSupported                    []string `json:"response_types_supported"`
	ResponseModesSupported                    []string `json:"response_modes_supported"`
	GrantTypesSupported                       []string `json:"grant_types_supported"`
	SubjectTypesSupported                     []string `json:"subject_types_supported"`
	IDTokenSigningAlgValuesSupported          []string `json:"id_token_signing_alg_values_supported"`
	TokenEndpointAuthMethodsSupported         []string `json:"token_endpoint_auth_methods_supported"`
	IntrospectionEndpointAuthMethodsSupported []string `json:"introspection_endpoint_auth_methods_supported"`
	RevocationEndpointAuthMethodsSupported    []string `json:"revocation_endpoint_auth_methods_supported"`
	CodeChallengeMethodsSupported             []string `json:"code_challenge_methods_supported"`
	ClaimsSupported                           []string `json:"claims_supported"`
	// Declaraciones NEGATIVAS explícitas: varias librerías cliente (Java/.NET) las
	// leen para decidir si pueden mandar `request`/`request_uri`/`claims`. Omitirlas
	// deja a la librería adivinando; anunciarlas como true sería mentir.
	RequestParameterSupported    bool `json:"request_parameter_supported"`
	RequestURIParameterSupported bool `json:"request_uri_parameter_supported"`
	ClaimsParameterSupported     bool `json:"claims_parameter_supported"`
}

// BuildDiscovery construye el documento de discovery a partir del issuer base.
// `issuer` no debe terminar en barra.
func BuildDiscovery(issuer string) DiscoveryDocument {
	return DiscoveryDocument{
		Issuer:                issuer,
		AuthorizationEndpoint: issuer + "/oauth2/authorize",
		TokenEndpoint:         issuer + "/oauth2/token",
		IntrospectionEndpoint: issuer + "/oauth2/introspect",
		RevocationEndpoint:    issuer + "/oauth2/revoke",
		UserinfoEndpoint:      issuer + "/oauth2/userinfo",
		JWKSURI:               issuer + "/.well-known/jwks.json",
		EndSessionEndpoint:    issuer + "/oauth2/logout",
		// Registro dinámico de clients (RFC 7591): registro por-org con token de
		// acceso inicial. El endpoint valida el token y crea el client org-scoped.
		RegistrationEndpoint: issuer + "/oauth2/register",
		ScopesSupported: []string{
			"openid", "profile", "email", "offline_access",
			"admin", "hub.read", "cloud.deploy", "cloud.read",
		},
		ResponseTypesSupported: []string{"code"},
		// Solo `query`: el authorization endpoint devuelve el código en la query y no
		// interpreta `response_mode`. Anunciar `fragment` haría que un cliente que lo
		// eligiera se quedara esperando un fragmento que nunca llega.
		ResponseModesSupported: []string{"query"},
		// token-exchange (RFC 8693) se anuncia porque ESTÁ implementado en
		// /oauth2/token. Anunciarlo no lo concede: el canje exige además que el
		// client esté marcado como delegable (mig 0025), así que para la inmensa
		// mayoría de los clients la respuesta será unauthorized_client.
		GrantTypesSupported: []string{
			"authorization_code", "refresh_token", "client_credentials",
			domain.GrantTokenExchange,
		},
		SubjectTypesSupported:            []string{"public"},
		IDTokenSigningAlgValuesSupported: []string{"RS256"},
		TokenEndpointAuthMethodsSupported: []string{
			"client_secret_basic", "client_secret_post", "none",
		},
		// Introspección: SOLO clients confidenciales (RFC 7662 §2.1 exige proteger
		// el endpoint). Un client público se identifica con un client_id que viaja
		// dentro del binario de la app, así que anunciar `none` aquí sería anunciar
		// que cualquiera puede preguntar por un token. handleIntrospect lo rechaza.
		IntrospectionEndpointAuthMethodsSupported: []string{
			"client_secret_basic", "client_secret_post",
		},
		// Revocación: `none` SE QUEDA, y es deliberado. Un client público (app
		// móvil, SPA) tiene que poder revocar SU refresh token al cerrar sesión
		// (RFC 7009); exigirle un secreto que por definición no tiene dejaría vivos
		// justo los tokens que el usuario acaba de pedir que se anulen. La verja de
		// ese endpoint no es el secreto sino la posesión del token y el cotejo del
		// client dueño.
		RevocationEndpointAuthMethodsSupported: []string{
			"client_secret_basic", "client_secret_post", "none",
		},
		CodeChallengeMethodsSupported: []string{"S256"},
		ClaimsSupported: []string{
			"sub", "iss", "aud", "exp", "iat", "auth_time", "nonce", "amr",
			"email", "email_verified", "name", "given_name", "family_name",
			"preferred_username", "locale", "picture", "tenant", "org_logo", "org_color", "org_forma", "roles", "perms", "groups", "scope",
		},
	}
}
