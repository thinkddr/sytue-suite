// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package livekit

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Client es un cliente mínimo de la API de servidor de LiveKit (RoomService),
// que LiveKit expone como endpoints Twirp en /twirp/livekit.RoomService/<Method>
// con cuerpo y respuesta JSON. Autentica cada llamada con un JWT de servidor
// minteado por el Signer. No usa el SDK de LiveKit (clean-room).
type Client struct {
	baseURL string
	signer  *Signer
	http    *http.Client
}

// APIError conserva el código Twirp para distinguir, por ejemplo, un recurso
// caducado (not_found) de una ruta inválida (bad_route) con el mismo HTTP 404.
type APIError struct {
	StatusCode int
	Code       string
	method     string
	body       string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("livekit: %s respondió %d: %s", e.method, e.StatusCode, e.body)
}

func (e *APIError) Is(target error) bool {
	return target == ErrParticipantNotFound && e.StatusCode == http.StatusNotFound
}

// NewClient construye el cliente apuntando al host de la API de LiveKit
// (p. ej. http://livekit:7880). El signer firma los tokens de servidor.
func NewClient(host string, signer *Signer) *Client {
	return &Client{
		baseURL: strings.TrimRight(host, "/"),
		signer:  signer,
		http:    &http.Client{Timeout: 10 * time.Second},
	}
}

// Room es la representación JSON de una room devuelta por LiveKit.
type Room struct {
	SID             string `json:"sid"`
	Name            string `json:"name"`
	EmptyTimeout    uint32 `json:"emptyTimeout"`
	MaxParticipants uint32 `json:"maxParticipants"`
	NumParticipants uint32 `json:"numParticipants"`
	CreationTime    int64  `json:"creationTime"`
	Metadata        string `json:"metadata"`
}

// CreateRoomRequest son los parámetros de creación de una room en LiveKit.
type CreateRoomRequest struct {
	Name            string `json:"name"`
	EmptyTimeout    uint32 `json:"emptyTimeout,omitempty"`
	MaxParticipants uint32 `json:"maxParticipants,omitempty"`
	Metadata        string `json:"metadata,omitempty"`
}

// CreateRoom crea (o devuelve la existente) una room en LiveKit. Idempotente:
// LiveKit devuelve la room si el nombre ya existe.
func (c *Client) CreateRoom(ctx context.Context, req CreateRoomRequest) (Room, error) {
	var out Room
	err := c.call(ctx, "CreateRoom", VideoGrant{RoomCreate: true, RoomList: true}, req, &out)
	return out, err
}

// ListRooms lista las rooms vivas; si names está vacío, lista todas.
func (c *Client) ListRooms(ctx context.Context, names ...string) ([]Room, error) {
	body := map[string]any{}
	if len(names) > 0 {
		body["names"] = names
	}
	var out struct {
		Rooms []Room `json:"rooms"`
	}
	if err := c.call(ctx, "ListRooms", VideoGrant{RoomList: true}, body, &out); err != nil {
		return nil, err
	}
	return out.Rooms, nil
}

// DeleteRoom cierra una room en LiveKit (desconecta a todos los participantes).
func (c *Client) DeleteRoom(ctx context.Context, name string) error {
	return c.call(ctx, "DeleteRoom", VideoGrant{RoomCreate: true},
		map[string]string{"room": name}, &struct{}{})
}

// RemoveParticipant expulsa a un participante de la room (moderación).
func (c *Client) RemoveParticipant(ctx context.Context, room, identity string) error {
	return c.call(ctx, "RemoveParticipant",
		VideoGrant{Room: room, RoomAdmin: true},
		map[string]string{"room": room, "identity": identity}, &struct{}{})
}

// MutePublishedTrack silencia (o reabre) una pista concreta de un participante.
//
// Silenciar NO es expulsar, y la diferencia importa: un micrófono abierto en una
// sala llena se corta sin echar a nadie de la reunión. `muted=false` devuelve el
// permiso, pero el SFU no puede reabrir el micrófono de nadie por su cuenta —el
// navegador exige un gesto del propio usuario—, así que en la práctica reabrir
// solo levanta el bloqueo.
func (c *Client) MutePublishedTrack(ctx context.Context, room, identity, trackSID string, muted bool) error {
	return c.call(ctx, "MutePublishedTrack",
		VideoGrant{Room: room, RoomAdmin: true},
		map[string]any{
			"room":      room,
			"identity":  identity,
			"track_sid": trackSID,
			"muted":     muted,
		}, &struct{}{})
}

// ParticipantInfo es lo que devuelve LiveKit de un participante, con sus pistas
// publicadas. El SID de la pista es lo que hace falta para silenciarla.
type ParticipantInfo struct {
	Metadata string `json:"metadata"`
	SID      string `json:"sid"`
	Identity string `json:"identity"`
	Name     string `json:"name"`
	// State: JOINING | JOINED | ACTIVE | DISCONNECTED.
	State string `json:"state"`
	// Kind: STANDARD | INGRESS | EGRESS | SIP | AGENT. Con él se distingue a
	// quien mira de quien emite (INGRESS) y de quien graba (EGRESS): los dos
	// últimos no cuentan como espectadores.
	Kind   string      `json:"kind"`
	Tracks []TrackInfo `json:"tracks"`
}

// Clases de participante que devuelve LiveKit.
const (
	ParticipanteEstandar = "STANDARD"
	ParticipanteIngress  = "INGRESS"
	ParticipanteEgress   = "EGRESS"
)

// ListParticipants lista quién está en la room ahora mismo, con sus pistas. Es
// lo que usa el sondeo de respaldo para contar espectadores sin webhooks.
func (c *Client) ListParticipants(ctx context.Context, room string) ([]ParticipantInfo, error) {
	if strings.TrimSpace(room) == "" {
		return nil, fmt.Errorf("livekit: listar participantes exige la room")
	}
	var out struct {
		Participants []ParticipantInfo `json:"participants"`
	}
	err := c.call(ctx, "ListParticipants", VideoGrant{Room: room, RoomAdmin: true},
		map[string]string{"room": room}, &out)
	if err != nil {
		return nil, err
	}
	return out.Participants, nil
}

// TrackInfo es una pista publicada por un participante.
//
// Con nombre y no anónima dentro de ParticipantInfo: quien quiera construir un
// participante en un test tenía que repetir la estructura entera con sus etiquetas
// JSON, seis líneas por cada caso, y eso desanima a escribir el caso.
type TrackInfo struct {
	SID    string `json:"sid"`
	Type   string `json:"type"` // AUDIO | VIDEO
	Muted  bool   `json:"muted"`
	Source string `json:"source"`
}

// SetCanPublish retira (o devuelve) el permiso de publicar de un participante.
//
// Es lo que falta cuando silenciar no basta: cortar la pista deja al otro lado
// con el botón de micrófono, y quien está molestando lo vuelve a pulsar. Esto le
// quita el permiso en el SFU, así que ya no puede aunque quiera.
//
// Suscribir y el canal de datos se mantienen: sigue viendo, oyendo y pudiendo
// levantar la mano. Retirar la palabra no es echar a nadie de la sala.
func (c *Client) SetCanPublish(ctx context.Context, room, identity string, canPublish bool) error {
	return c.call(ctx, "UpdateParticipant",
		VideoGrant{Room: room, RoomAdmin: true},
		map[string]any{
			"room":     room,
			"identity": identity,
			"permission": map[string]any{
				"can_publish":      canPublish,
				"can_subscribe":    true,
				"can_publish_data": true,
			},
		}, &struct{}{})
}

// SetMediaPermissions changes media and data grants together. Encrypted calls
// keep both disabled until the current device has installed its epoch key.
func (c *Client) SetMediaPermissions(ctx context.Context, room, identity string, publish, data bool) error {
	return c.call(ctx, "UpdateParticipant", VideoGrant{Room: room, RoomAdmin: true}, map[string]any{
		"room": room, "identity": identity, "permission": map[string]any{"can_publish": publish, "can_subscribe": true, "can_publish_data": data},
	}, &struct{}{})
}

var ErrParticipantNotFound = errors.New("livekit: participant not found")

// GetParticipant trae un participante con sus pistas. Hace falta para silenciar:
// la API de LiveKit pide el SID de la pista, no «el micrófono».
func (c *Client) GetParticipant(ctx context.Context, room, identity string) (ParticipantInfo, error) {
	var out ParticipantInfo
	err := c.call(ctx, "GetParticipant",
		VideoGrant{Room: room, RoomAdmin: true},
		map[string]string{"room": room, "identity": identity}, &out)
	return out, err
}

// call ejecuta una llamada Twirp/JSON autenticada contra el RoomService.
func (c *Client) call(ctx context.Context, method string, grant VideoGrant, in, out any) error {
	return c.callService(ctx, "livekit.RoomService", method, grant, in, out)
}

// callService es lo mismo pero diciendo a QUÉ servicio Twirp se llama. La
// grabación no vive en `livekit.RoomService` sino en `livekit.Egress`, y hasta
// ahora la ruta estaba clavada: no había forma de hablar con el otro.
func (c *Client) callService(ctx context.Context, servicio, method string, grant VideoGrant, in, out any) error {
	token, err := c.signer.serverToken(grant)
	if err != nil {
		return err
	}
	payload, err := json.Marshal(in)
	if err != nil {
		return fmt.Errorf("livekit: serializar %s: %w", method, err)
	}
	url := c.baseURL + "/twirp/" + servicio + "/" + method
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("livekit: construir petición %s: %w", method, err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+token)

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return fmt.Errorf("livekit: llamada %s: %w", method, err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		var fallo struct {
			Code string `json:"code"`
		}
		_ = json.Unmarshal(respBody, &fallo)
		return &APIError{StatusCode: resp.StatusCode, Code: fallo.Code, method: method, body: string(respBody)}
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(respBody, out); err != nil {
		return fmt.Errorf("livekit: decodificar respuesta %s: %w", method, err)
	}
	return nil
}
