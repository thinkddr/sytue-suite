// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"strings"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
)

// CUENTA DE GOOGLE EMULADA. Una app escrita para Google pide permisos con los
// scopes de Google (`https://www.googleapis.com/auth/calendar`…). Sytue ID los
// entiende y los concede como a cualquier otro scope —solo si el administrador los
// dio a esa app al registrarla—, y los productos los exigen en sus rutas
// /compat/google. Google no interviene en nada: ni emite, ni ve, ni guarda.

// GoogleAPIsAudience es la audiencia extra de un access token que lleva scopes de
// Google: la que aceptan las rutas /compat/google de los productos, y SOLO ellas.
// Vive en la plataforma porque la emite Sytue ID y la comprueba cada producto.
const GoogleAPIsAudience = authn.GoogleAPIsAudience

const googleScopePrefix = "https://www.googleapis.com/auth/"

// GoogleScopes son los scopes de Google que Sytue sabe servir. Sin `drive.file` ni
// `drive.metadata.readonly`: Drive no sabe qué ficheros creó cada app ni separa leer
// metadatos de leer contenido, y conceder más de lo pedido no es emular. Los de directorio
// además exigen, al usarse, que quien inició sesión administre la organización
// (como en Google Workspace); eso lo comprueba la Directory API, no el token.
var GoogleScopes = map[string]struct{}{
	googleScopePrefix + "calendar":                              {},
	googleScopePrefix + "calendar.readonly":                     {},
	googleScopePrefix + "calendar.events":                       {},
	googleScopePrefix + "calendar.events.readonly":              {},
	googleScopePrefix + "drive":                                 {},
	googleScopePrefix + "drive.readonly":                        {},
	googleScopePrefix + "contacts":                              {},
	googleScopePrefix + "contacts.readonly":                     {},
	googleScopePrefix + "admin.directory.user.readonly":         {},
	googleScopePrefix + "admin.directory.group.readonly":        {},
	googleScopePrefix + "admin.directory.group.member.readonly": {},
}

// googleIdentityAlias traduce los scopes de identidad con nombre de Google a los
// de OpenID, que son los que el resto de Sytue ID ya sabe tratar.
var googleIdentityAlias = map[string]string{
	googleScopePrefix + "userinfo.email":   "email",
	googleScopePrefix + "userinfo.profile": "profile",
}

// NormalizeGoogleScopes traduce los alias de identidad de Google y quita
// duplicados, conservando el orden. Lo demás pasa tal cual: la validación contra
// lo permitido al client la hace el authorize de siempre.
func NormalizeGoogleScopes(scope string) string {
	var out []string
	visto := map[string]bool{}
	for _, sc := range strings.Fields(scope) {
		if alias, ok := googleIdentityAlias[sc]; ok {
			sc = alias
		}
		if !visto[sc] {
			visto[sc] = true
			out = append(out, sc)
		}
	}
	return strings.Join(out, " ")
}

// HasGoogleAPIScope dice si una lista de scopes lleva alguno de APIs de Google.
func HasGoogleAPIScope(scope string) bool {
	for _, sc := range strings.Fields(scope) {
		if _, ok := GoogleScopes[sc]; ok {
			return true
		}
	}
	return false
}
