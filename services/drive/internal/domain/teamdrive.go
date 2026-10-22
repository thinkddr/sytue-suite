// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

// ErrLastManager indica que la operación dejaría la unidad de equipo SIN ningún
// gestor (quitar o degradar al último manager): se rechaza para que la unidad no
// quede sin quien gestione membresía/ajustes. La capa HTTP lo traduce a 409.
var ErrLastManager = errors.New("drive: la unidad quedaría sin gestor")

// TeamRole es el rol de un miembro dentro de una unidad de equipo. Orden de
// privilegio creciente: viewer < editor < manager.
//   - viewer:  leer / descargar / navegar.
//   - editor:  además crear / editar / subir / borrar.
//   - manager: además gestionar miembros y ajustes de la unidad.
type TeamRole string

const (
	TeamViewer  TeamRole = "viewer"
	TeamEditor  TeamRole = "editor"
	TeamManager TeamRole = "manager"
)

// ValidTeamRole valida un rol de unidad de equipo.
func ValidTeamRole(r TeamRole) bool {
	return r == TeamViewer || r == TeamEditor || r == TeamManager
}

// rank ordena los roles por privilegio (mayor = más permisos; 0 = inválido).
func (r TeamRole) rank() int {
	switch r {
	case TeamViewer:
		return 1
	case TeamEditor:
		return 2
	case TeamManager:
		return 3
	default:
		return 0
	}
}

// AtLeast indica si el rol concede al menos el privilegio de `need` (deny-by-default:
// un rol inválido/desconocido no concede nada).
func (r TeamRole) AtLeast(need TeamRole) bool {
	return r.rank() > 0 && r.rank() >= need.rank()
}

// CanWrite indica permiso de escritura (editor o superior).
func (r TeamRole) CanWrite() bool { return r.AtLeast(TeamEditor) }

// CanManage indica permiso de gestión de miembros/ajustes (solo manager).
func (r TeamRole) CanManage() bool { return r.AtLeast(TeamManager) }

// TeamDrive es una unidad de equipo: propiedad de la ORG (tenant), no de un
// usuario. Su carpeta RAÍZ (RootFolderID, nunca serializada) posee todo el
// contenido, de modo que al salir un miembro sus ficheros PERMANECEN (continuidad).
type TeamDrive struct {
	ID           uuid.UUID `json:"id"`
	Tenant       string    `json:"-"`
	Name         string    `json:"name"`
	RootFolderID uuid.UUID `json:"-"`
	CreatedBy    string    `json:"created_by,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
	// ArchivedAt marca la unidad como archivada (organizativa; no borra contenido
	// ni revoca acceso). Nil = activa. Solo un manager la conmuta.
	ArchivedAt *time.Time `json:"archived_at,omitempty"`
}

// TeamMember es la pertenencia de un usuario a una unidad de equipo, con su rol.
type TeamMember struct {
	DriveID   uuid.UUID `json:"-"`
	Tenant    string    `json:"-"`
	Subject   string    `json:"subject"`
	Role      TeamRole  `json:"role"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
