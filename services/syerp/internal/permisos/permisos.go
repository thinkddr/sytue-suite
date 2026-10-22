// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package permisos decide qué puede hacer cada persona en cada módulo del ERP.
//
// ES EL MODELO DE ODOO, recortado a lo que sirve a una pyme: en cada aplicación se
// elige un nivel y ya. Cuatro niveles —sin acceso, lectura, usuario y
// administrador— en vez de grupos combinables: con treinta módulos, una matriz de
// grupos es justo la pantalla que nadie termina de rellenar.
//
// El dueño y los administradores de la organización lo son también de todo el
// ERP, por su rol: no se les puede quitar desde aquí, igual que en Odoo el
// administrador del sistema no se queda fuera de una app.
package permisos

import "slices"

// Niveles, de menos a más.
const (
	Ninguno       = "ninguno"
	Lectura       = "lectura"
	Usuario       = "usuario"
	Administrador = "administrador"
)

// Niveles en orden: cada uno incluye lo del anterior.
var Niveles = []string{Ninguno, Lectura, Usuario, Administrador}

// Obligaciones es el módulo que no sale del catálogo: el calendario de
// obligaciones, que existe en cuanto la organización hace el asistente y no se
// puede apagar.
const Obligaciones = "obligaciones"

// sensibles son los módulos con datos que no toda la organización debe ver:
// sueldos, contabilidad, facturación y lo que se presenta a la administración.
// Empiezan cerrados y se abren a quien toque; el resto empieza abierto, que es lo
// que espera quien comparte el ERP con su equipo.
var sensibles = map[string]bool{
	"contabilidad": true, "facturacion": true, "nominas": true, "rrhh": true, "ausencias": true,
	"evaluaciones": true, "reclutamiento": true, "consolidacion": true, "gastos": true, Obligaciones: true,
}

// PorDefecto es el nivel de quien no tiene uno puesto en el módulo.
func PorDefecto(modulo string) string {
	if sensibles[modulo] {
		return Ninguno
	}
	return Usuario
}

// Valido dice si un texto es un nivel.
func Valido(nivel string) bool { return slices.Contains(Niveles, nivel) }

// Alcanza dice si `tiene` llega a `necesita`.
func Alcanza(tiene, necesita string) bool {
	return slices.Index(Niveles, tiene) >= slices.Index(Niveles, necesita)
}

// EsAdministradorDeLaOrganizacion: owner o admin de la organización.
func EsAdministradorDeLaOrganizacion(roles []string) bool {
	return slices.Contains(roles, "owner") || slices.Contains(roles, "admin")
}

// Nivel de una persona en un módulo: administrador si lo es de la organización;
// si no, el suyo puesto o el del módulo por defecto.
func Nivel(roles []string, puestos map[string]string, modulo string) string {
	if EsAdministradorDeLaOrganizacion(roles) {
		return Administrador
	}
	if n, ok := puestos[modulo]; ok && Valido(n) {
		return n
	}
	return PorDefecto(modulo)
}

// --- Segregación de funciones: los VERBOS ------------------------------------
//
// Un nivel es una escalera y por eso no puede separar funciones: quien sube a
// «usuario» para crear una factura sube también para asentarla. Crear, aprobar,
// pagar, conciliar, auditar y administrar NO se implican entre sí, así que van
// aparte, como un conjunto y no como un número.
//
// Se deducen del nivel mientras nadie diga otra cosa (VerbosPorDefecto), y una
// lista puesta a mano —la que guarda `permisos_verbo`— los sustituye ENTEROS
// para esa persona en ese módulo, incluida la administración de la organización.

const (
	Crear       = "crear"
	Aprobar     = "aprobar"
	Asentar     = "asentar"
	Pagar       = "pagar"
	Conciliar   = "conciliar"
	Auditar     = "auditar"
	Administrar = "administrar"
)

// Verbos son todos los gestos que se pueden conceder o retirar por separado.
var Verbos = []string{Crear, Aprobar, Asentar, Pagar, Conciliar, Auditar, Administrar}

// VerboValido dice si un texto es un verbo del catálogo.
func VerboValido(v string) bool { return slices.Contains(Verbos, v) }

// nivelQueImplica es el nivel a partir del cual un verbo se da por concedido
// cuando NADIE lo ha puesto a mano. Reproduce lo que el ERP ya hacía antes de
// que existieran los verbos, para que desplegarlos no cambie nada por su cuenta:
// la separación de funciones es un gesto que alguien tiene que dar.
var nivelQueImplica = map[string]string{
	Auditar:     Lectura,
	Crear:       Usuario,
	Asentar:     Usuario,
	Conciliar:   Usuario,
	Aprobar:     Administrador,
	Pagar:       Administrador,
	Administrar: Administrador,
}

// VerbosPorDefecto son los verbos que concede un nivel por sí solo.
func VerbosPorDefecto(nivel string) []string {
	out := make([]string, 0, len(Verbos))
	for _, v := range Verbos {
		if Alcanza(nivel, nivelQueImplica[v]) {
			out = append(out, v)
		}
	}
	return out
}

// VerbosDe resuelve los verbos EFECTIVOS de una persona en un módulo: la lista
// que le hayan puesto, si la hay, y si no los que su nivel implica.
//
// Que una lista puesta mande también sobre owner/admin es la decisión que hace
// que esto sirva de algo: una separación de funciones de la que el
// administrador se escapa por su rol no separa nada.
func VerbosDe(roles []string, puestos map[string]string, listas map[string][]string, modulo string) []string {
	if lista, ok := listas[modulo]; ok {
		return lista
	}
	return VerbosPorDefecto(Nivel(roles, puestos, modulo))
}

// PuedeVerbo dice si esa persona puede ese gesto en ese módulo.
func PuedeVerbo(roles []string, puestos map[string]string, listas map[string][]string, modulo, verbo string) bool {
	return slices.Contains(VerbosDe(roles, puestos, listas, modulo), verbo)
}
