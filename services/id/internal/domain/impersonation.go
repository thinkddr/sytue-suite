// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"errors"
	"strings"
	"time"
)

// GrantTokenExchange es el grant_type del canje de tokens (RFC 8693 §2.1).
//
//nolint:gosec // G101: identificador del estándar, no una credencial.
const GrantTokenExchange = "urn:ietf:params:oauth:grant-type:token-exchange"

// ImpersonationTokenTTL es la vida FIJA y corta de un token de impersonación.
// No es configurable a propósito: un token que actúa como otra persona no debe
// poder alargarse por despliegue. Tampoco se emite refresh con él (ver
// impersonationDeniedScopes), así que el techo real de la delegación son estos
// minutos: quien la quiera sostener, vuelve a canjear y vuelve a auditarse.
const ImpersonationTokenTTL = 5 * time.Minute

// Errores del canje. Se mantienen SEPARADOS porque se traducen a errores OAuth
// distintos: el primero habla del client (unauthorized_client), el segundo del
// sujeto pedido (invalid_target).
var (
	// ErrImpersonationForbidden: el client NO es delegable.
	ErrImpersonationForbidden = errors.New("domain: el client no puede actuar en nombre de un usuario")
	// ErrImpersonationTarget: el sujeto pedido no es impersonable por ese client
	// (no existe, no es de su organización, no está activo o es owner/admin).
	ErrImpersonationTarget = errors.New("domain: sujeto no impersonable")
)

// AuthorizeImpersonationClient es la puerta DENY-BY-DEFAULT del canje: ¿este
// client puede pedir un token en nombre de un usuario? Se comprueba ANTES de
// mirar nada de la petición, para que un client no delegable no pueda usar este
// endpoint ni como oráculo de qué tokens o qué usuarios existen.
//
// Las CUATRO condiciones son acumulativas:
//   - may_impersonate marcado a mano por el operador (mig 0025);
//   - el grant declarado en grant_types (un admin puede desactivarlo desde la
//     Console sin tocar la columna protegida);
//   - client CONFIDENCIAL: uno público no prueba quién es, solo dice su id;
//   - acotado a una organización: sin ella no hay "misma organización" que exigir.
func AuthorizeImpersonationClient(c Client) error {
	if !c.MayImpersonate || !c.AllowsGrant(GrantTokenExchange) || c.IsPublic() || c.OrgID == "" {
		return ErrImpersonationForbidden
	}
	return nil
}

// AuthorizeImpersonationTarget decide si `c` puede actuar como `target`. role es
// el rol del objetivo EN LA ORGANIZACIÓN DEL CLIENT; cadena vacía ⇒ no es
// miembro (o no existe: el llamante NO debe distinguir ambos casos).
//
// Un usuario owner/admin solo es impersonable si el client lo declara
// explícitamente (may_impersonate_admins): actuar como administrador es actuar
// con todos los permisos de la organización, y eso se concede aparte.
func AuthorizeImpersonationTarget(c Client, target User, role Role) error {
	if !target.Active() || !role.Valid() {
		return ErrImpersonationTarget
	}
	if (role == RoleOwner || role == RoleAdmin) && !c.MayImpersonateAdmins {
		return ErrImpersonationTarget
	}
	return nil
}

// impersonationDeniedScopes NUNCA viajan en un token de impersonación, aunque el
// client los tenga concedidos:
//   - offline_access daría un refresh token, y con él delegación indefinida y sin
//     nueva traza: la impersonación caduca en minutos, punto;
//   - los scopes de MÁQUINA de la pasarela son capacidades del servicio, no del
//     usuario — dárselos al token del usuario sería exceder lo que él tendría.
var impersonationDeniedScopes = map[string]struct{}{
	"offline_access":    {},
	"id.directory.read": {},
	"id.apps.manage":    {},
}

// impersonationAdminScopes solo se conceden si el usuario impersonado es de
// verdad owner/admin: el techo del token es lo que el propio usuario tendría.
var impersonationAdminScopes = map[string]struct{}{"admin": {}}

// ImpersonationScope calcula el scope EFECTIVO del token canjeado: la
// INTERSECCIÓN de lo pedido con lo que el client tiene concedido, menos lo que
// nunca se delega, menos lo que el usuario objetivo no podría tener. Nunca
// añade: un `scope` vacío hereda el del client (ya acotado), no "todo".
//
// Deduplica y preserva el orden de la petición (salida estable para la traza).
func ImpersonationScope(requested string, c Client, role Role) string {
	want := strings.Fields(requested)
	if len(want) == 0 {
		want = c.Scopes
	}
	privileged := role == RoleOwner || role == RoleAdmin
	out := make([]string, 0, len(want))
	seen := make(map[string]struct{}, len(want))
	for _, sc := range want {
		if _, dup := seen[sc]; dup {
			continue
		}
		if !c.AllowsScope(sc) {
			continue
		}
		if _, denied := impersonationDeniedScopes[sc]; denied {
			continue
		}
		if _, adminOnly := impersonationAdminScopes[sc]; adminOnly && !privileged {
			continue
		}
		seen[sc] = struct{}{}
		out = append(out, sc)
	}
	return strings.Join(out, " ")
}
