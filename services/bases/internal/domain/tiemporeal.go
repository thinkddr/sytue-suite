// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// VersionTiempoReal versiona el CONTRATO del flujo de cambios y de las operaciones de
// celda, no los datos. Un cliente que pida otra versión recibe un error y no un flujo
// que no sabe leer.
const VersionTiempoReal = 1

// Límites del tiempo real.
const (
	// MaxCambiosPorTanda es cuántos cambios se entregan de una vez; quien se ha quedado
	// más atrás recibe `resync` y vuelve a cargar.
	MaxCambiosPorTanda = 500
	// MaxPresentes acota las pestañas presentes que se guardan y se enseñan por base.
	MaxPresentes = 100
	// VidaPresencia es cuánto vale un latido de presencia sin renovarse.
	VidaPresencia = 30 * time.Second
)

// Cursor es la posición en el registro de cambios de una base: la transacción que
// escribió el cambio y su id. El orden (xact, id) es el de entrega.
type Cursor struct {
	Xact int64
	ID   int64
}

// String lo escribe como va en el `id` de cada evento y en `Last-Event-ID`.
func (c Cursor) String() string {
	return strconv.FormatInt(c.Xact, 10) + "-" + strconv.FormatInt(c.ID, 10)
}

// LeerCursor lee un cursor recibido del cliente: dos enteros no negativos.
func LeerCursor(s string) (Cursor, error) {
	x, i, ok := strings.Cut(s, "-")
	xact, err1 := strconv.ParseInt(x, 10, 64)
	id, err2 := strconv.ParseInt(i, 10, 64)
	if !ok || err1 != nil || err2 != nil || xact < 0 || id < 0 || len(s) > 40 {
		return Cursor{}, fmt.Errorf("%w: cursor de cambios", ErrInvalido)
	}
	return Cursor{Xact: xact, ID: id}, nil
}

// Cambio es un evento del flujo: QUÉ ha cambiado y dónde, sin los datos. El cliente
// vuelve a pedir lo que ha cambiado por la API de siempre, que autoriza la lectura; por
// el flujo no viaja ningún valor.
type Cambio struct {
	Cursor   string `json:"-"`
	Action   string `json:"action"`
	TableID  string `json:"table_id,omitempty"`
	RecordID string `json:"record_id,omitempty"`
	FieldID  string `json:"field_id,omitempty"`
	// Actor es quien lo hizo; OpID, la operación de celda que lo causó (el cliente
	// reconoce el eco de las suyas). Ausentes en los cambios de acceso.
	Actor string `json:"actor,omitempty"`
	OpID  string `json:"op_id,omitempty"`
}

// Presente es una pestaña con la base abierta, y la celda en la que está.
type Presente struct {
	Conn     string `json:"conn"`
	Sub      string `json:"sub"`
	Name     string `json:"name,omitempty"`
	TableID  string `json:"table_id,omitempty"`
	RecordID string `json:"record_id,omitempty"`
	FieldID  string `json:"field_id,omitempty"`
}

var conexion = regexp.MustCompile(`^[A-Za-z0-9_-]{8,64}$`)

// ConexionValida dice si es un identificador de pestaña admisible.
func ConexionValida(s string) bool { return conexion.MatchString(s) }

// accionesDelFlujo es lo que viaja por el flujo: lo que cambia lo que otra persona
// tiene en pantalla. Lo demás de la traza (exportar, leer por un conector…) no.
var accionesDelFlujo = map[string]bool{
	"record.create": true, "record.update": true, "record.link": true, "record.delete": true,
	"field.create": true, "field.rename": true, "field.delete": true,
	"table.create": true, "table.rename": true, "table.delete": true, "table.import": true,
	"view.create": true, "view.delete": true,
	"comment.create": true, "comment.delete": true,
	"base.update": true,
}

// accionesDeAcceso cambian QUIÉN puede: por el flujo solo se dice que el acceso ha
// cambiado («access»), sin quién ni a quién, que es cosa del propietario.
var accionesDeAcceso = map[string]bool{
	"permission.grant": true, "permission.revoke": true, "base.transfer": true,
}

// CambioParaElFlujo decide cómo sale por el flujo una entrada de la traza para alguien
// con rol `rolTabla` sobre la tabla de la entrada (vacío = no la puede leer) y `rolBase`
// sobre la base. ok=false: no se entrega.
func CambioParaElFlujo(accion, tabla, registro, campo, actor, op string, rolBase, rolTabla Rol) (Cambio, bool) {
	if accion == "record.create.form" { // un envío de un formulario público: un alta como otra
		accion, actor = "record.create", ""
	}
	switch {
	case accionesDeAcceso[accion]:
		return Cambio{Action: "access"}, true
	case !accionesDelFlujo[accion]:
		return Cambio{}, false
	case tabla == "":
		// De la base entera (renombrarla): a quien tiene rol en la base.
		if !Permite(rolBase, AccLeer) {
			return Cambio{}, false
		}
	case !Permite(rolTabla, AccLeer):
		return Cambio{}, false
	}
	return Cambio{Action: accion, TableID: tabla, RecordID: registro, FieldID: campo, Actor: actor, OpID: op}, true
}
