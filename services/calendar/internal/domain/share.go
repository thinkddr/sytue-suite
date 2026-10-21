// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Compartir calendarios con ACL: un dueño concede a otro usuario del tenant
// acceso de lectura o edición a uno de sus calendarios. Este fichero define el
// modelo y las reglas de negocio independientes de transporte/almacenamiento.
//
// Autocontenido (sin directorio email→usuario): el share se guarda por la
// IDENTIDAD del destinatario (su correo verificado y/o su sub) y se hace cumplir
// al acceder casando contra los claims del JWT ya verificado. El correo se
// normaliza a minúsculas para casar de forma estable con el claim `email`.
package domain

import (
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Errores de dominio específicos de la compartición. (Los generales viven en
// domain.go; la escritura de un reader reutiliza ErrReadOnly de subscription.go.)
var (
	// ErrInvalidShare: datos de compartición inválidos (correo/permiso). Mapea a 400.
	ErrInvalidShare = errors.New("calendar: datos de compartición inválidos")
	// ErrShareSelf: intento de compartir un calendario con uno mismo. Mapea a 400.
	ErrShareSelf = errors.New("calendar: no puedes compartir un calendario contigo mismo")
)

// SharePermission es el nivel de acceso concedido al compartir un calendario.
type SharePermission string

const (
	// PermReader concede VER los eventos del calendario (solo lectura).
	PermReader SharePermission = "reader"
	// PermWriter concede ver Y editar los eventos del calendario.
	PermWriter SharePermission = "writer"
	// PermManage es la DELEGACIÓN de agenda: como writer (ver + crear/editar/borrar
	// eventos en el calendario), pero marca la relación como «el asistente gestiona
	// la agenda del dueño». Se presenta como delegación (no como un share normal) y
	// es el gancho para atribuir «en nombre de» (SENT-BY iTIP) en el futuro.
	PermManage SharePermission = "manage"
)

// ValidSharePermission valida un nivel de permiso de compartición.
func ValidSharePermission(p SharePermission) bool {
	return p == PermReader || p == PermWriter || p == PermManage
}

// CanWrite indica si un permiso concede ESCRITURA (crear/editar/borrar eventos):
// writer y manage (delegación) sí; reader no. Es la regla única que consulta la
// puerta de escritura de calendarios compartidos y delegados.
func CanWrite(p SharePermission) bool {
	return p == PermWriter || p == PermManage
}

// CalendarShare es una concesión de acceso a un calendario a un destinatario
// identificado por su correo (verificado del JWT al acceder) y/o su sub.
type CalendarShare struct {
	ID           uuid.UUID       `json:"id"`
	Tenant       string          `json:"-"`
	CalendarID   uuid.UUID       `json:"calendar_id"`
	GranteeEmail string          `json:"grantee_email,omitempty"`
	GranteeSub   string          `json:"grantee_sub,omitempty"`
	Permission   SharePermission `json:"permission"`
	CreatedBy    string          `json:"created_by"`
	CreatedAt    time.Time       `json:"created_at"`
	UpdatedAt    time.Time       `json:"updated_at"`
}

// NormalizeEmail recorta y baja a minúsculas un correo para casarlo de forma
// estable (case-insensitive) con el claim `email` del JWT y con la columna
// grantee_email (guardada ya normalizada).
//
// NO hace lo mismo que el `NormalizeEmail` de Sytue ID, y es a propósito: aquel
// se queda con el primer campo para tragarse el doble relleno de los gestores de
// contraseñas, porque en un formulario de acceso adivinar no cuesta nada —si
// adivina mal, el acceso falla y se reintenta—. Aquí decide con QUIÉN se comparte
// un calendario, así que un valor con espacios lo rechaza `ValidEmail` en vez de
// quedarse con una de las dos direcciones. Unificarlos sería un error.
func NormalizeEmail(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

// ValidEmail es una validación mínima de correo en el borde de confianza (no
// pretende RFC 5322 completo): no vacío, sin espacios ni control, exactamente una
// '@' interior y un punto en el dominio, longitud acotada. Basta para no guardar
// basura como destinatario de un share; el casado real es contra el claim del JWT.
func ValidEmail(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" || len(s) > 320 {
		return false
	}
	if strings.ContainsAny(s, " \t\r\n\x00") {
		return false
	}
	at := strings.IndexByte(s, '@')
	if at <= 0 || at != strings.LastIndexByte(s, '@') || at == len(s)-1 {
		return false
	}
	return strings.IndexByte(s[at+1:], '.') > 0
}
