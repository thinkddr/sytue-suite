// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package auth

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"slices"
	"strings"
)

// Reclamos es lo que una pantalla necesita saber de quién ha entrado para decidir QUÉ
// PINTA: la organización activa, su clase y su plan, y los roles y permisos. Son los
// claims del access token de Sytue ID (`services/id/internal/oidc/token.go`), los mismos
// que la web lee en su BFF (`userFromClaims` y `permsFromAccessToken`).
//
// Se leen SIN verificar la firma, igual que el `sub`: el token llega por TLS del propio
// IdP y esto solo decide si se ofrece una puerta. Quien autoriza es el servicio, que sí
// verifica —una tesela de Sytue Admin pintada de más da un 403, no un agujero—.
type Reclamos struct {
	Sub    string `json:"sub"`
	Tenant string `json:"tenant"`
	// Email va en el token si se concedió el scope `email`, que las apps nativas piden.
	Email string `json:"email"`
	// Name es el nombre visible, con el scope `profile` (que también piden).
	Name  string   `json:"name"`
	Roles []string `json:"roles"`
	Perms []string `json:"perms"`
	// OrgKind es `org` o `personal`; vacío en tokens anteriores a la distinción.
	OrgKind string `json:"org_kind"`
	// Plan es `personal_free`, `pro` o `enterprise`.
	Plan string `json:"plan"`
}

// ReclamosDe lee los claims de un JWT. Un token que no lo es devuelve el cero, que en
// todas las reglas de abajo es «no»: lo que falta nunca abre puertas.
func ReclamosDe(token string) Reclamos {
	partes := strings.Split(token, ".")
	if len(partes) != 3 {
		return Reclamos{}
	}
	crudo, err := base64.RawURLEncoding.DecodeString(partes[1])
	if err != nil {
		return Reclamos{}
	}
	var r Reclamos
	_ = json.Unmarshal(crudo, &r)
	return r
}

// Reclamos son los del access token vigente.
func (s *Session) Reclamos(ctx context.Context) (Reclamos, error) {
	token, err := s.AccessToken(ctx)
	if err != nil {
		return Reclamos{}, err
	}
	return ReclamosDe(token), nil
}

// EsCuentaPersonal dice, SIN red, si la sesión es de una cuenta personal: lo lee del
// access token que hay guardado, aunque esté caducado (la clase de cuenta no cambia al
// renovarlo). Sin sesión, no. Es para lo que se pinta en el acto, como el menú de cuenta.
func (s *Session) EsCuentaPersonal() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return ReclamosDe(s.tokens.Access).EsCuentaPersonal()
}

// EsCuentaPersonal es el `esCuentaPersonal` de `@sytue/auth`: la organización es la
// propia persona. Se mira la clase, el plan y el prefijo del tenant, por ese orden,
// porque los tokens anteriores a `org_kind` solo traen lo segundo o lo tercero.
func (r Reclamos) EsCuentaPersonal() bool {
	return r.OrgKind == "personal" || strings.HasPrefix(r.Plan, "personal_") ||
		strings.HasPrefix(r.Tenant, "p-") || strings.HasPrefix(r.Tenant, "personal-")
}

// PuedeAdministrar es el `puedeAdministrar` de `@sytue/ui` MÁS el `sinAdminPorPlan`
// de `@sytue/auth`, que es la condición con la que el portal decide si pinta la tesela
// de Sytue Admin: owner, admin o cualquier permiso delegado —los administradores
// delegados entran sin ser `admin`—, y no una cuenta personal sin plan de pago.
//
// ponytail: la web cuenta solo los permisos que conoce (`KNOWN` en `perms.ts`) y aquí
// cuenta todos. Solo difieren si Sytue ID emite un permiso que la web aún no sabe
// nombrar; si pasa, se trae aquí esa lista.
func (r Reclamos) PuedeAdministrar() bool {
	admin := slices.Contains(r.Roles, "owner") || slices.Contains(r.Roles, "admin") || len(r.Perms) > 0
	dePago := r.Plan == "pro" || r.Plan == "enterprise"
	return admin && (!r.EsCuentaPersonal() || dePago)
}
