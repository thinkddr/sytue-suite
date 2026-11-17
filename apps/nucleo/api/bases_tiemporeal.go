// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
)

// Coedición en tiempo real de Bases, contrato v1 (services/bases/internal/http/
// tiemporeal.go): el flujo SSE `GET /bases/{id}/events` dice QUÉ ha cambiado, sin datos,
// y el latido `PUT /bases/{id}/presence` dice a los demás que esta ventana sigue ahí y en
// qué celda. Es el `seguir` y el `heartbeat` de la web (lib/tiemporeal.ts y
// packages/api-clients/src/bases).

// BasesChange es un cambio del flujo. El registro se vuelve a pedir con GetRecord, que
// autoriza la lectura.
type BasesChange struct {
	// Action es `record.create|update|link|delete`, `field.*`, `table.*`, `view.*`,
	// `comment.*`, `base.update` o `access`.
	Action   string `json:"action"`
	TableID  string `json:"table_id,omitempty"`
	RecordID string `json:"record_id,omitempty"`
	FieldID  string `json:"field_id,omitempty"`
	Actor    string `json:"actor,omitempty"`
	// OpID es la operación de celda que lo causó: quien la lanzó reconoce su eco.
	OpID string `json:"op_id,omitempty"`
}

// BasesPresence es una ventana (o pestaña) con la base abierta y, si lo dice, su celda.
type BasesPresence struct {
	Conn     string `json:"conn"`
	Sub      string `json:"sub"`
	Name     string `json:"name,omitempty"`
	TableID  string `json:"table_id,omitempty"`
	RecordID string `json:"record_id,omitempty"`
	FieldID  string `json:"field_id,omitempty"`
}

// BasesLatido es lo que manda el latido: quién es lo pone el servicio con el token.
type BasesLatido struct {
	Conn     string `json:"conn"`
	TableID  string `json:"table_id,omitempty"`
	RecordID string `json:"record_id,omitempty"`
	FieldID  string `json:"field_id,omitempty"`
}

// Los eventos del flujo.
const (
	// BasesHola llega al abrir (también al reconectar): el flujo está vivo.
	BasesHola = "hello"
	// BasesCambio trae un BasesChange.
	BasesCambio = "change"
	// BasesPresencia trae la lista de quienes están.
	BasesPresencia = "presence"
	// BasesResync: se quedó demasiado atrás y hay que recargarlo todo.
	BasesResync = "resync"
	// BasesAdios: el servicio cierra; Motivo dice por qué.
	BasesAdios = "bye"
	// BasesSinAcceso es el motivo del adiós cuando se ha retirado el acceso: no se reintenta.
	BasesSinAcceso = "access"
)

// BasesEvento es un evento del flujo, ya leído.
type BasesEvento struct {
	Tipo      string
	Cambio    BasesChange
	Presentes []BasesPresence
	// Motivo del adiós: `access`, `token` o `error`.
	Motivo string
}

// Events abre el flujo de cambios de una base y llama a `alEvento` con cada evento que
// entiende, en orden. `conn` es la ventana (8-64 de [A-Za-z0-9_-]) y `desde` el cursor
// por el que reanudar (el `Last-Event-ID`; vacío, desde ahora). `alCursor`, si no es nil,
// recibe cada cursor nuevo, también el de los bloques de solo `id:` con que el servicio
// lo deja al día mientras no pasa nada: es el que hay que pasar como `desde` al
// reconectar para que el servicio cuente lo que se perdió.
//
// No vuelve hasta que el flujo se corta o muere `ctx`; para mantenerlo vivo, Escuchar.
// Sin acceso a la base el servicio contesta 404 antes de abrirlo (un *Error).
func (b *Bases) Events(ctx context.Context, base, conn, desde string, alCursor func(string), alEvento func(BasesEvento)) error {
	q := url.Values{"v": {strconv.Itoa(BasesRealtimeVersion)}}
	if conn != "" {
		q.Set("conn", conn)
	}
	cab := http.Header{"Accept": {"text/event-stream"}}
	if desde != "" {
		cab.Set("Last-Event-ID", desde)
	}
	resp, err := b.c.Do(ctx, Request{Path: "/bases/" + url.PathEscape(base) + "/events", Query: q, Header: cab})
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	ultimo := desde
	return leerSSEConID(resp.Body, func(id, evento, datos string) {
		if id != "" && id != ultimo {
			ultimo = id
			if alCursor != nil {
				alCursor(id)
			}
		}
		e := BasesEvento{Tipo: evento}
		switch evento {
		case BasesHola, BasesResync:
		case BasesCambio:
			if json.Unmarshal([]byte(datos), &e.Cambio) != nil || e.Cambio.Action == "" {
				return
			}
		case BasesPresencia:
			if json.Unmarshal([]byte(datos), &e.Presentes) != nil {
				return
			}
			if e.Presentes == nil {
				e.Presentes = []BasesPresence{}
			}
		case BasesAdios:
			var d struct {
				Reason string `json:"reason"`
			}
			_ = json.Unmarshal([]byte(datos), &d)
			e.Motivo = d.Reason
		default:
			return // un bloque de solo `id:`, o un evento de otra versión
		}
		alEvento(e)
	})
}

// Heartbeat es el latido de presencia: que los demás sepan que esta ventana sigue en la
// base, y en qué tabla y celda. Va SinAviso: no cambia nada de lo que enseña el widget,
// y cada diez segundos lo volvería a publicar.
func (b *Bases) Heartbeat(ctx context.Context, base string, l BasesLatido) error {
	datos, err := json.Marshal(l)
	if err != nil {
		return err
	}
	resp, err := b.c.Do(ctx, Request{
		Method: http.MethodPut, Path: "/bases/" + url.PathEscape(base) + "/presence", Body: datos,
		Header: http.Header{"Content-Type": {"application/json"}}, SinAviso: true,
	})
	if err != nil {
		return err
	}
	return resp.Body.Close()
}
