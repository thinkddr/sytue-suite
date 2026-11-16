// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

// El canal en vivo de la coedición: `GET /documents/{id}/ws`, el protocolo de
// services/docs/internal/collab/protocol.go. Al abrir se manda `hello` con la última seq
// que se tiene, el servicio repite lo que falte y después cada lote llega en cuanto se
// secuencia. Es lo que en la web quita el sondeo de `collab.ts`.

// DocProtocolVersion es el contrato v1; el servidor acepta también clientes
// publicados antes de la negociación. No cambia el formato persistido del CRDT.
const DocProtocolVersion = 1

// DocMensaje es un mensaje del servicio por el canal: `welcome` (rol y seq), `role` (mi
// rol ha cambiado: el dueño concedió o quitó un permiso), `ops` (las
// secuenciadas, propias confirmadas o de otros), `presence` (quién está, o dónde tiene el
// cursor uno de ellos) o `error`.
type DocMensaje struct {
	Type    string             `json:"type"`
	Version int                `json:"version,omitempty"`
	Role    string             `json:"role,omitempty"`
	Seq     int64              `json:"seq,omitempty"`
	Ops     []DocOpSecuenciada `json:"ops,omitempty"`
	// Presence es el roster: los `sub` de quien está en la sala.
	Presence []DocPresencia `json:"presence,omitempty"`
	// Cursor es la difusión del cursor de UN participante, ya validada (docs_cursor.go):
	// nil si el mensaje no traía o no encajaba.
	Cursor  *DocPresencia `json:"cursor,omitempty"`
	Code    string        `json:"code,omitempty"`
	Message string        `json:"message,omitempty"`
}

// CanalDoc es el lado de escritura del canal abierto: lo que el cliente manda por la MISMA
// conexión por la que recibe, para que su cursor llegue a los demás.
type CanalDoc struct {
	conn *websocket.Conn
}

// Enviar publica la posición del cursor propio. Es presencia efímera: el servicio la
// difunde y no la guarda, así que un envío que falla no se reintenta (el siguiente
// movimiento manda la posición buena). Se puede llamar desde cualquier gorrutina.
func (c *CanalDoc) Enviar(ctx context.Context, cursor Cursor) error {
	if err := wsjson.Write(ctx, c.conn, map[string]any{"type": "cursor", "cursor": cursor}); err != nil {
		return fmt.Errorf("docs: canal en vivo: %w", err)
	}
	return nil
}

// Coeditar abre el canal del documento y entrega cada mensaje hasta que se corta o se
// cancela `ctx`; devuelve por qué. Quien llama reconecta, con la última seq que haya visto.
func (d *Docs) Coeditar(ctx context.Context, id string, desde int64, alLlegar func(DocMensaje)) error {
	return d.CoeditarConCursor(ctx, id, desde, nil, alLlegar)
}

// CoeditarConCursor es Coeditar dando además, en cuanto el canal está abierto, su lado de
// escritura (`alAbrir`), que deja de servir cuando esta llamada vuelve.
func (d *Docs) CoeditarConCursor(ctx context.Context, id string, desde int64, alAbrir func(*CanalDoc), alLlegar func(DocMensaje)) error {
	token := ""
	if d.c.session != nil {
		t, err := d.c.token(ctx, d.c.base)
		if err != nil {
			return err
		}
		token = t
	}
	conn, resp, err := websocket.Dial(ctx, d.c.WSURL("/documents/"+url.PathEscape(id)+"/ws", nil), opcionesWS(d.c.http, token))
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
	if err != nil {
		return fmt.Errorf("docs: canal en vivo: %w", err)
	}
	defer func() { _ = conn.CloseNow() }()
	conn.SetReadLimit(4 << 20) // un replay tras una desconexión larga trae muchas ops
	if err := wsjson.Write(ctx, conn, map[string]any{"type": "hello", "version": DocProtocolVersion, "last_seq": desde}); err != nil {
		return fmt.Errorf("docs: canal en vivo: %w", err)
	}
	if alAbrir != nil {
		alAbrir(&CanalDoc{conn: conn})
	}
	for {
		var crudo json.RawMessage
		if err := wsjson.Read(ctx, conn, &crudo); err != nil {
			return fmt.Errorf("docs: canal en vivo: %w", err)
		}
		var m DocMensaje
		if json.Unmarshal(crudo, &m) == nil {
			if (m.Version != 0 && m.Version != DocProtocolVersion) || m.Code == "protocol_version" {
				return fmt.Errorf("docs: unsupported collaboration protocol version: %d", m.Version)
			}
			m.Cursor = cursorDePar(m.Cursor)
			alLlegar(m)
		}
	}
}
