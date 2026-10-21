// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import "slices"

// Rol es el nivel de acceso a una base, coherente con el modelo de Docs:
// propietario > editor > comentarista > lector.
type Rol string

// Roles. El propietario es el `owner_sub` de la base y no se concede por ACL.
const (
	RolPropietario  Rol = "owner"
	RolEditor       Rol = "editor"
	RolComentarista Rol = "commenter"
	RolLector       Rol = "viewer"
)

var rangoRol = map[Rol]int{RolLector: 1, RolComentarista: 2, RolEditor: 3, RolPropietario: 4}

// Valido indica si es un rol conocido.
func (r Rol) Valido() bool { return rangoRol[r] > 0 }

// Concedible indica si la ACL puede concederlo (el propietario no).
func (r Rol) Concedible() bool { return r.Valido() && r != RolPropietario }

// Accion es lo que se pide hacer; Permite es la ÚNICA tabla de decisión.
type Accion string

// Acciones protegidas.
const (
	AccLeer          Accion = "leer"         // registros, campos, vistas, exportar
	AccComentar      Accion = "comentar"     // comentar un registro (y borrar lo propio)
	AccEscribirReg   Accion = "escribir_reg" // crear, editar y borrar registros
	AccGestionVistas Accion = "gestionar_vistas"
	AccEsquema       Accion = "esquema"     // tablas, campos, importar CSV
	AccPermisos      Accion = "permisos"    // ver y cambiar la ACL
	AccAuditoria     Accion = "auditoria"   // leer la traza
	AccBorrarBase    Accion = "borrar_base" // renombrar y borrar la base
	AccTransferir    Accion = "transferir"  // dar la propiedad a otra persona de la organización
	AccConector      Accion = "conector"    // conceder y revocar conectores
	AccFormulario    Accion = "formulario"  // enlaces públicos de un formulario
	// AccRevisarScripts: aprobar o rechazar la versión de un script compartido y leer el
	// registro de ejecuciones. Es de quien ya ve y decide todo en la base: revisar un
	// script no le enseña nada que no pudiera leer.
	AccRevisarScripts Accion = "revisar_scripts"
)

var minimo = map[Accion]Rol{
	AccLeer:           RolLector,
	AccComentar:       RolComentarista,
	AccEscribirReg:    RolEditor,
	AccGestionVistas:  RolEditor,
	AccEsquema:        RolPropietario,
	AccPermisos:       RolPropietario,
	AccAuditoria:      RolPropietario,
	AccBorrarBase:     RolPropietario,
	AccTransferir:     RolPropietario,
	AccConector:       RolPropietario,
	AccFormulario:     RolPropietario,
	AccRevisarScripts: RolPropietario,
}

// Permite decide si el rol puede hacer la acción. Deny-by-default: rol vacío o
// desconocido, y acción desconocida, niegan.
func Permite(r Rol, a Accion) bool {
	need, ok := minimo[a]
	return ok && rangoRol[r] > 0 && rangoRol[r] >= rangoRol[need]
}

// Clases de principal de una entrada de ACL.
const (
	PrincipalUsuario      = "user"   // una persona de la organización, por su `sub`
	PrincipalGrupo        = "group"  // un grupo de Sytue ID de la organización, por su nombre
	PrincipalOrganizacion = "tenant" // toda la organización (solo la propia)
	// PrincipalExterno es una persona de OTRA organización, por su `sub` (ADR 0040): la
	// base se queda en la anfitriona y esta entrada es lo ÚNICO que esa persona es aquí.
	PrincipalExterno = "external"
)

// MaxGruposDelActor acota los grupos del token que entran en una consulta de ACL.
const MaxGruposDelActor = 200

// Entrada de ACL tal como la lee el resolutor.
type EntradaACL struct {
	TableID       string // "" = toda la base
	PrincipalKind string // user | group | tenant | external
	PrincipalID   string
	Role          Rol
}

// ResolverRol calcula el rol efectivo de un actor sobre una tabla de una base:
// el propietario es siempre propietario; si no, el MÁXIMO entre las entradas de la
// base y las de esa tabla que le aplican (a su nombre, a uno de sus grupos o a toda la
// organización). Sin ninguna entrada no hay acceso: ("", false). tabla == "" resuelve
// solo el nivel base. Una clase de principal desconocida no aplica (deny-by-default).
//
// Quien viene de otra organización (Actor.Externo) NUNCA es propietario y solo cuenta
// con las entradas `external` a su nombre: ni las de «toda la organización», ni las de
// un grupo, ni una `user` con su mismo `sub`.
func ResolverRol(ownerSub string, entradas []EntradaACL, a Actor, tabla string) (Rol, bool) {
	if !a.Externo && ownerSub != "" && ownerSub == a.Sub {
		return RolPropietario, true
	}
	var mejor Rol
	for _, e := range entradas {
		if !e.Role.Concedible() {
			continue
		}
		if e.TableID != "" && e.TableID != tabla {
			continue
		}
		if aplica(e, a) && rangoRol[e.Role] > rangoRol[mejor] {
			mejor = e.Role
		}
	}
	return mejor, mejor != ""
}

// aplica dice si la entrada es para este actor. Un identificador vacío no coincide con nada.
func aplica(e EntradaACL, a Actor) bool {
	if e.PrincipalID == "" {
		return false
	}
	if a.Externo || e.PrincipalKind == PrincipalExterno {
		return a.Externo && e.PrincipalKind == PrincipalExterno && e.PrincipalID == a.Sub
	}
	switch e.PrincipalKind {
	case PrincipalUsuario:
		return e.PrincipalID == a.Sub
	case PrincipalOrganizacion:
		return e.PrincipalID == a.Tenant
	case PrincipalGrupo:
		return slices.Contains(a.Groups, e.PrincipalID)
	}
	return false
}
