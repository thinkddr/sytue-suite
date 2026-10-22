// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package domain es el modelo de Sytue Bases: bases, tablas, campos tipados,
// registros, enlaces, vistas y permisos, sin base de datos ni HTTP. Toda la
// validación de lo que escribe un cliente vive aquí y se repite SIEMPRE en el
// servidor: lo que valida la interfaz es comodidad, no seguridad.
package domain

import (
	"encoding/json"
	"errors"
	"strings"
	"time"
	"unicode/utf8"
)

// Errores de dominio. La capa HTTP los traduce a estados.
var (
	ErrNoExiste     = errors.New("bases: no existe")                   // 404, también cuando no hay acceso: no se revela la existencia
	ErrSinPermiso   = errors.New("bases: permiso insuficiente")        // 403
	ErrInvalido     = errors.New("bases: entrada no válida")           // 422
	ErrReferenciado = errors.New("bases: lo referencia otro registro") // 409
	ErrLimite       = errors.New("bases: límite alcanzado")            // 409
	ErrConflicto    = errors.New("bases: conflicto de versión")        // 409
)

// Límites. Cada uno protege una consulta o un cuerpo de tamaño no acotado.
const (
	MaxBasesPorOrg       = 200
	MaxTablasPorBase     = 50
	MaxCamposPorTabla    = 100
	MaxRegistrosPorTabla = 1000000 // lo lleva base_tables.record_count (migración 0005)
	MaxVistasPorTabla    = 50
	MaxPermisosPorBase   = 200
	MaxOpciones          = 100
	MaxLongOpcion        = 80
	MaxLongNombre        = 120
	MaxLongTexto         = 10000
	MaxEnlaces           = 100
	MaxFiltros           = 10
	MaxOrden             = 3
	MaxPagina            = 500
	PaginaPorDefecto     = 100
	MaxCursor            = 4096  // bytes del cursor de página, ya codificado
	MaxClaveOrden        = 128   // caracteres de un texto que cuentan para ordenar
	MaxTamRegistro       = 60000 // bytes de data serializado; el CHECK de la tabla es 65536
)

// Tipo de campo.
type Tipo string

// Tipos de campo admitidos. El tipo de un campo no cambia tras crearlo: los datos
// ya escritos dejarían de cumplirlo.
const (
	TipoTexto     Tipo = "text"
	TipoNumero    Tipo = "number"
	TipoFecha     Tipo = "date"
	TipoCasilla   Tipo = "checkbox"
	TipoSeleccion Tipo = "select"
	TipoMultiple  Tipo = "multiselect"
	TipoRelacion  Tipo = "link"
)

// Valido indica si es un tipo conocido.
func (t Tipo) Valido() bool {
	switch t {
	case TipoTexto, TipoNumero, TipoFecha, TipoCasilla, TipoSeleccion, TipoMultiple, TipoRelacion, TipoFormula, TipoAdjunto:
		return true
	}
	return false
}

// Base es una base de datos de una organización.
type Base struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	OwnerSub    string `json:"owner_sub"`
	Role        Rol    `json:"role"` // rol efectivo de quien pregunta
	// HostOrg, si viene, es la organización que aloja una base compartida con quien
	// pregunta desde fuera. Vacío en las bases de su propia organización.
	HostOrg   string    `json:"host_org,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Tabla de una base.
type Tabla struct {
	ID       string `json:"id"`
	BaseID   string `json:"base_id"`
	Name     string `json:"name"`
	Position int    `json:"position"`
	Role     Rol    `json:"role,omitempty"`
}

// Campo de una tabla.
type Campo struct {
	ID          string   `json:"id"`
	TableID     string   `json:"table_id"`
	Name        string   `json:"name"`
	Kind        Tipo     `json:"kind"`
	Choices     []string `json:"choices,omitempty"`
	LinkTableID string   `json:"link_table_id,omitempty"`
	// Formula y Result solo en los campos de fórmula: la expresión (referencias por id,
	// ver formula.go) y el tipo de su resultado (number o text).
	Formula  string `json:"formula,omitempty"`
	Result   Tipo   `json:"result,omitempty"`
	Position int    `json:"position"`
}

// Registro de una tabla. Data lleva los campos que no son enlaces; Links, los
// enlaces por campo.
type Registro struct {
	ID      string                     `json:"id"`
	TableID string                     `json:"table_id"`
	Data    map[string]json.RawMessage `json:"data"`
	Links   map[string][]string        `json:"links"`
	// Attachments son los ficheros de cada campo de adjuntos, por campo.
	Attachments map[string][]Adjunto `json:"attachments,omitempty"`
	Rev         int64                `json:"rev"`
	CreatedBy   string               `json:"created_by"`
	UpdatedBy   string               `json:"updated_by"`
	CreatedAt   time.Time            `json:"created_at"`
	UpdatedAt   time.Time            `json:"updated_at"`
}

// Permiso es una entrada de la ACL de una base (o de una de sus tablas).
type Permiso struct {
	ID            string `json:"id"`
	TableID       string `json:"table_id,omitempty"`
	PrincipalKind string `json:"principal_kind"`
	PrincipalID   string `json:"principal_id"`
	// PrincipalLabel es cómo se enseña una persona en la lista (su nombre o su correo,
	// del directorio al concederlo). Vacío en grupos y en toda la organización.
	PrincipalLabel string    `json:"principal_label,omitempty"`
	Role           Rol       `json:"role"`
	CreatedBy      string    `json:"created_by"`
	CreatedAt      time.Time `json:"created_at"`
}

// Entrada de la auditoría.
type Entrada struct {
	ID       int64           `json:"id"`
	TableID  string          `json:"table_id,omitempty"`
	RecordID string          `json:"record_id,omitempty"`
	FieldID  string          `json:"field_id,omitempty"`
	Actor    string          `json:"actor"`
	Action   string          `json:"action"`
	Before   json.RawMessage `json:"before,omitempty"`
	After    json.RawMessage `json:"after,omitempty"`
	At       time.Time       `json:"at"`
}

// Actor es quien hace la petición: su sub, su organización y sus grupos, de los claims.
type Actor struct {
	Sub    string
	Tenant string
	// Groups son los nombres de sus grupos en la organización (claim `groups` del token).
	Groups []string
	// Externo: viene de OTRA organización y Tenant es la ANFITRIONA de la base que toca
	// (la resuelve el servicio, nunca el cliente). Allí no es nada más que su `sub`.
	Externo bool
}

// LimpiarNombre recorta y valida un nombre visible (1..120 runas, sin control).
func LimpiarNombre(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" || utf8.RuneCountInString(s) > MaxLongNombre || !utf8.ValidString(s) || tieneControl(s) {
		return "", ErrInvalido
	}
	return s, nil
}

func tieneControl(s string) bool {
	for _, r := range s {
		if r < 0x20 || r == 0x7f {
			return true
		}
	}
	return false
}
