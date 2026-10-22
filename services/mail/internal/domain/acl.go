// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"time"

	"github.com/google/uuid"
)

// Tipos de cuenta (accounts.kind): persona vs buzón compartido de organización.
// Un buzón compartido (soporte@, ventas@) es una `accounts` con kind=shared y un
// subject sintético; reúsa el modelo de cuenta/buzones/entrega íntegro.
const (
	KindUser   = "user"   // cuenta personal (1:1 con una identidad de Sytue ID)
	KindShared = "shared" // buzón compartido de la organización (sin persona dueña)
)

// Permission es el nivel de acceso de un grantee sobre una cuenta (buzón
// compartido o delegación). Escalera deny-by-default: read < send < manage; cada
// nivel incluye los inferiores. Sin fila de ACL vigente NO hay acceso.
type Permission string

const (
	PermRead   Permission = "read"   // ver el buzón y leer sus mensajes
	PermSend   Permission = "send"   // además, enviar-como / enviar-en-nombre-de
	PermManage Permission = "manage" // además, gestionar el ACL del buzón
)

// rank ordena los niveles (0 = desconocido/sin permiso).
func (p Permission) rank() int {
	switch p {
	case PermRead:
		return 1
	case PermSend:
		return 2
	case PermManage:
		return 3
	default:
		return 0
	}
}

// AtLeast indica si p concede al menos el nivel need (y es un permiso válido).
// Base de la comprobación de ACL: deny-by-default (un permiso desconocido nunca
// alcanza nada).
func (p Permission) AtLeast(need Permission) bool {
	return p.rank() > 0 && p.rank() >= need.rank()
}

// ValidPermission acepta solo los tres niveles conocidos.
func ValidPermission(p string) bool { return Permission(p).rank() > 0 }

// SharedMailbox es una cuenta (compartida o delegada) a la que el usuario tiene
// acceso por ACL, con su nivel de permiso. Alimenta la lista del sidebar y el
// selector «De» del redactor (enviar-como / enviar-en-nombre-de).
type SharedMailbox struct {
	AccountID   uuid.UUID  `json:"account_id"`
	Address     string     `json:"address"`
	DisplayName string     `json:"display_name,omitempty"`
	Kind        string     `json:"kind"`       // shared (compartido) | user (delegado)
	Permission  Permission `json:"permission"` // read | send | manage
	// EditRequested: en `read`, ya ha pedido acceso (la franja lo dice).
	EditRequested bool `json:"edit_requested,omitempty"`
}

// MailboxGrant es una entrada de ACL vista por quien gestiona el buzón: el
// grantee (por su dirección legible) y su permiso. El sub queda oculto.
type MailboxGrant struct {
	GranteeSubject string     `json:"-"`
	GranteeEmail   string     `json:"grantee"`
	Permission     Permission `json:"permission"`
	CreatedAt      time.Time  `json:"created_at"`
}

// GrantedIdentity resuelve una dirección «De» a la cuenta a la que pertenece y el
// permiso del usuario sobre ella. Base de la autorización server-side de
// enviar-como (kind=shared) / enviar-en-nombre-de (kind=user).
type GrantedIdentity struct {
	AccountID  uuid.UUID
	Address    string
	Kind       string
	Permission Permission
}
