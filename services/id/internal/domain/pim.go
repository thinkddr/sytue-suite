// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"sort"
	"time"
)

// PIM (Privileged Identity Management, mig 0020): elevación de privilegios
// just-in-time sobre los roles personalizados (mig 0019). Un usuario ELEGIBLE para un
// rol lo ACTIVA por una ventana acotada; solo entonces sus permisos entran en el claim
// `perms`. Estas funciones son la DECISIÓN pura (sin BD ni reloj real): reciben el
// «ahora» por parámetro para ser deterministas y testeables offline.

// Estados de una activación (role_activations.status). Solo ActivationActive concede
// permisos, y únicamente mientras now < expires_at.
const (
	ActivationPending = "pending" // solicitada, espera aprobación
	ActivationActive  = "active"  // concede sus perms hasta expires_at
	ActivationRevoked = "revoked" // desactivada por el usuario o un admin
	ActivationDenied  = "denied"  // solicitud rechazada por el aprobador
	ActivationExpired = "expired" // caducada (etiqueta de reporte; la expiración es perezosa)
)

// PIMActivation es una activación evaluable para el cálculo de perms efectivos: los
// permisos que concede (rol personalizado) o su condición de built-in, más su estado y
// fin de ventana. Es la unidad que EffectivePerms filtra por «ahora».
type PIMActivation struct {
	Perms     []string  // permisos del rol personalizado; vacío para built-in
	Builtin   bool      // true ⇒ elevación a un rol built-in (concede el set delegable)
	ExpiresAt time.Time // fin de la ventana; concede solo si now < ExpiresAt
	Status    string    // solo ActivationActive concede
}

// grantablePerms devuelve el catálogo DELEGABLE (AllPermissions menos roles.manage):
// lo que concede una elevación PIM a un rol built-in (admin/owner). Excluye roles.manage
// (reservado a owner/admin vía requireRole; anti auto-escalada), igual que un rol
// personalizado nunca puede concederlo. Orden estable (AllPermissions).
func grantablePerms() []string {
	out := make([]string, 0, len(AllPermissions))
	for _, p := range AllPermissions {
		if _, ok := permGrantable[p]; ok {
			out = append(out, string(p))
		}
	}
	return out
}

// EffectivePerms calcula los permisos EFECTIVOS de un usuario en una org: la unión
// (distinct, ordenada) de
//
//   - `direct`: permisos de sus roles personalizados ASIGNADOS de forma permanente
//     (user_custom_roles). Se conceden SIEMPRE (retrocompat con mig 0019; PIM es una
//     capa ADICIONAL, no los sustituye).
//   - las activaciones que están ACTIVAS en `now` (status active y now < expires): un
//     rol personalizado aporta sus perms; un rol built-in aporta el set delegable.
//
// Una elegibilidad SIN activación no aparece aquí (no concede). Una activación
// pending/revoked/denied, o active pero caducada (now >= expires), TAMPOCO — de ahí que
// «expirado ⇒ los perms desaparecen» sin barrido: se reevalúa contra `now` en cada
// emisión de token. Alimenta el claim `perms`; vacío ⇒ nil (el claim se omite —
// owner/admin no lo necesitan: son superusuarios por el claim `roles`).
func EffectivePerms(direct []string, acts []PIMActivation, now time.Time) []string {
	set := make(map[string]struct{}, len(direct)+len(AllPermissions))
	for _, p := range direct {
		set[p] = struct{}{}
	}
	for _, a := range acts {
		if a.Status != ActivationActive || !now.Before(a.ExpiresAt) {
			continue
		}
		if a.Builtin {
			for _, p := range grantablePerms() {
				set[p] = struct{}{}
			}
			continue
		}
		for _, p := range a.Perms {
			set[p] = struct{}{}
		}
	}
	if len(set) == 0 {
		return nil
	}
	out := make([]string, 0, len(set))
	for p := range set {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

// ValidActivationDuration valida la duración (minutos) pedida para una activación contra
// el máximo de la elegibilidad: positiva y no mayor que el máximo. Pedir más del máximo
// es un error EXPLÍCITO (400), no un recorte silencioso (paridad Entra: la política de
// la elegibilidad manda sobre la ventana).
func ValidActivationDuration(requested, maxMinutes int) bool {
	return requested > 0 && maxMinutes > 0 && requested <= maxMinutes
}

// ElevatableBuiltin indica si un rol built-in es objetivo válido de elevación PIM: solo
// owner/admin (los superusuarios). Elevarse a member/billing no tendría sentido (una
// elevación built-in concede el set delegable completo). Un role_id de rol personalizado
// sigue el otro camino (elegibilidad por role_id).
func ElevatableBuiltin(role string) bool {
	return role == string(RoleOwner) || role == string(RoleAdmin)
}

// EffectiveStatus resuelve el estado MOSTRADO de una activación en `now`: una fila
// 'active' cuya ventana ya pasó (now >= expires) se reporta ActivationExpired — la
// expiración es perezosa y el status en BD puede ir por detrás. El resto se muestra tal
// cual. Espeja, para la UI/reporte, la misma decisión que EffectivePerms toma sobre
// conceder o no los permisos.
func EffectiveStatus(status string, expiresAt, now time.Time) string {
	if status == ActivationActive && !now.Before(expiresAt) {
		return ActivationExpired
	}
	return status
}
