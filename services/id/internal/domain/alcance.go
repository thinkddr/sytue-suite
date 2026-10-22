// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"sort"
	"time"
)

// ALCANCE EFECTIVO: qué puede hacer de verdad una persona, resuelto.
//
// Sytue Admin ya enseñaba las PIEZAS —el rol de la membresía, los roles
// personalizados asignados, las elegibilidades PIM, los grupos—, cada una en su
// pantalla. Sumarlas mentalmente es justo donde se equivoca quien administra:
// las tres fuentes se solapan, una de ellas CADUCA, y la respuesta a «¿puede
// esta persona exportar la auditoría?» dependía de acordarse de mirar las
// cuatro páginas.
//
// EffectivePerms (pim.go) ya calcula la unión, pero la aplana: devuelve la lista
// de permisos y pierde de dónde sale cada uno. Sin eso no se puede quitar nada
// —no se sabe qué rol tocar— ni distinguir un permiso permanente de uno que se
// apaga a las 14:35. Aquí se calcula lo mismo CONSERVANDO la procedencia.
//
// Es una DECISIÓN PURA: recibe el «ahora» y no toca ni base ni reloj, así que
// dice exactamente lo que dirá el siguiente token que se emita.

// Fuentes de un permiso, de la más estable a la más efímera.
const (
	FuenteRolIntegrado      = "rol_integrado"      // owner/admin: superusuario por el claim `roles`
	FuenteRolPersonalizado  = "rol_personalizado"  // asignación permanente (user_custom_roles)
	FuenteElevacionTemporal = "elevacion_temporal" // activación PIM viva, con hora de apagado
)

// Concesion es UN permiso con la razón por la que lo tiene.
type Concesion struct {
	Permiso string     `json:"permission"`
	Fuente  string     `json:"source"`
	Detalle string     `json:"detail,omitempty"` // nombre del rol, o el rol al que se elevó
	Hasta   *time.Time `json:"expires_at,omitempty"`
}

// ElevacionViva es una activación PIM tal como la mira este cálculo: qué concede
// y hasta cuándo. Es la misma información que PIMActivation más el nombre con el
// que enseñarla.
type ElevacionViva struct {
	Nombre    string
	Perms     []string
	Builtin   bool
	ExpiresAt time.Time
	Status    string
}

// AlcanceEfectivo resuelve todos los permisos de una persona en una
// organización, cada uno con su procedencia y, si caduca, con su hora.
//
// Reglas, y son las MISMAS que aplica el enforcement (HasPermission y
// EffectivePerms), no una aproximación para la pantalla:
//
//   - owner/admin son superusuarios: tienen el catálogo ENTERO por su rol, y por
//     eso aparecen con todos los permisos y fuente `rol_integrado`. Enseñar
//     «sin permisos» para un owner —que es lo que salía de mirar `perms`, vacío
//     a propósito en su token— es la respuesta más peligrosa posible.
//   - un rol personalizado asignado concede siempre;
//   - una activación PIM concede solo si está `active` y `now` va por delante de
//     su fin; una elevación a rol integrado concede el conjunto DELEGABLE, que
//     no incluye roles.manage.
//
// Cuando un permiso llega por varios caminos se queda el MÁS ESTABLE: saber que
// alguien lo tiene para siempre por su rol importa más que saber que además lo
// tiene por una elevación que se apaga esta tarde.
func AlcanceEfectivo(rolIntegrado string, roles []CustomRole, elevaciones []ElevacionViva, now time.Time) []Concesion {
	mejor := map[string]Concesion{}
	poner := func(c Concesion) {
		ya, hay := mejor[c.Permiso]
		if hay && rango(ya.Fuente) <= rango(c.Fuente) {
			return
		}
		mejor[c.Permiso] = c
	}

	if rolIntegrado == string(RoleOwner) || rolIntegrado == string(RoleAdmin) {
		for _, p := range AllPermissions {
			poner(Concesion{Permiso: string(p), Fuente: FuenteRolIntegrado, Detalle: rolIntegrado})
		}
	}
	for _, r := range roles {
		for _, p := range r.Permissions {
			poner(Concesion{Permiso: p, Fuente: FuenteRolPersonalizado, Detalle: r.Name})
		}
	}
	for _, e := range elevaciones {
		if e.Status != ActivationActive || !now.Before(e.ExpiresAt) {
			continue
		}
		hasta := e.ExpiresAt
		concede := e.Perms
		if e.Builtin {
			concede = grantablePerms()
		}
		for _, p := range concede {
			poner(Concesion{Permiso: p, Fuente: FuenteElevacionTemporal, Detalle: e.Nombre, Hasta: &hasta})
		}
	}

	out := make([]Concesion, 0, len(mejor))
	for _, c := range mejor {
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Permiso < out[j].Permiso })
	return out
}

// rango ordena las fuentes: cuanto menor, más estable.
func rango(fuente string) int {
	switch fuente {
	case FuenteRolIntegrado:
		return 0
	case FuenteRolPersonalizado:
		return 1
	default:
		return 2
	}
}
