// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Ingesta RTMP sobre la API de Ingress de LiveKit (servicio Twirp `livekit.Ingress`).
//
// Un ingress es la «puerta» por la que un codificador externo (OBS) entra en una
// room: LiveKit le da una URL rtmp:// y una clave de emisión, y cuando el
// codificador conecta, el servicio de ingress transcodifica la señal y la publica
// en la room como un participante más. Igual que el egress, es OTRO proceso —el
// contenedor `livekit/ingress`, ver deploy/cast-media— y si no está desplegado
// estas llamadas fallan aunque el SFU esté perfecto.
//
// Los nombres JSON son los de LiveKit (protobuf con nombres originales, que es
// lo que acepta y devuelve su servidor Twirp); las enumeraciones viajan por su
// nombre («RTMP_INPUT», «VP8»). Clean-room: solo la biblioteca estándar.
package livekit

import (
	"context"
	"fmt"
	"strings"
)

// Códigos y capas que Sytue Cast pide al ingress. VP8 y no H.264 a propósito: la
// app nativa decodifica VP8 (CAST-ENCARGO §1.4); Opus es el único audio de WebRTC.
const (
	CodecVideoVP8  = "VP8"
	CodecAudioOpus = "OPUS"
	// EntradaRTMP es el único tipo de entrada que se usa: OBS y compañía.
	EntradaRTMP = "RTMP_INPUT"
)

// CapaVideo es una capa de simulcast que el ingress publica en la room.
type CapaVideo struct {
	// Quality: LOW | MEDIUM | HIGH. Una sola capa va como HIGH.
	Quality string `json:"quality"`
	Width   uint32 `json:"width"`
	Height  uint32 `json:"height"`
	// Bitrate en bits por segundo.
	Bitrate uint32 `json:"bitrate"`
}

// CapasCast son las dos capas de un directo de Sytue Cast: 720p a 2,5 Mbit/s
// para quien tiene ancho de banda y 360p a 800 kbit/s para quien no.
func CapasCast() []CapaVideo {
	return []CapaVideo{
		{Quality: "HIGH", Width: 1280, Height: 720, Bitrate: 2_500_000},
		{Quality: "LOW", Width: 640, Height: 360, Bitrate: 800_000},
	}
}

// IngressRequest es lo que hace falta para crear un ingress RTMP.
type IngressRequest struct {
	// Name es una etiqueta para quien administre LiveKit; no la ve nadie más.
	Name string
	// RoomName es la room en la que publica. Se cambia después con UpdateIngress
	// cada vez que se programa un directo.
	RoomName string
	// ParticipantIdentity e ParticipantName son cómo aparece el emisor en la room.
	ParticipantIdentity string
	ParticipantName     string
	// Capas de vídeo; vacío = CapasCast.
	Capas []CapaVideo
}

// IngressInfo es lo que LiveKit devuelve de un ingress: la clave de emisión que
// se pega en OBS, la URL y en qué room y con qué identidad publica.
type IngressInfo struct {
	IngressID string `json:"ingress_id"`
	Name      string `json:"name"`
	// StreamKey es la clave que se pega en OBS. Se guarda tal cual: es la única
	// forma de volver a enseñársela a su dueño.
	StreamKey string `json:"stream_key"`
	// URL es la dirección rtmp:// tal como la conoce LiveKit (su `rtmp_base_url`).
	URL                 string        `json:"url"`
	InputType           string        `json:"input_type"`
	RoomName            string        `json:"room_name"`
	ParticipantIdentity string        `json:"participant_identity"`
	ParticipantName     string        `json:"participant_name"`
	EnableTranscoding   bool          `json:"enable_transcoding"`
	State               *IngressState `json:"state,omitempty"`
}

// IngressState es cómo está el ingress ahora mismo: si hay alguien emitiendo.
type IngressState struct {
	// Status: ENDPOINT_INACTIVE | ENDPOINT_BUFFERING | ENDPOINT_PUBLISHING |
	// ENDPOINT_ERROR | ENDPOINT_COMPLETE.
	Status    string `json:"status"`
	Error     string `json:"error,omitempty"`
	RoomID    string `json:"room_id,omitempty"`
	StartedAt Int64  `json:"started_at,omitempty"`
	EndedAt   Int64  `json:"ended_at,omitempty"`
}

// Estados de un ingress que consume Sytue Cast.
const (
	IngressInactivo   = "ENDPOINT_INACTIVE"
	IngressPublicando = "ENDPOINT_PUBLISHING"
	IngressBuffering  = "ENDPOINT_BUFFERING"
)

// Emitiendo dice si hay señal entrando ahora por este ingress.
func (i IngressInfo) Emitiendo() bool {
	return i.State != nil && (i.State.Status == IngressPublicando || i.State.Status == IngressBuffering)
}

// CreateIngress crea un ingress RTMP que publica en `RoomName` con la identidad
// dada, transcodificando a VP8/Opus en las capas pedidas. Devuelve la clave de
// emisión: LiveKit la genera y no la vuelve a enseñar por otro camino que
// ListIngress, así que quien llama la guarda.
func (c *Client) CreateIngress(ctx context.Context, req IngressRequest) (IngressInfo, error) {
	if strings.TrimSpace(req.RoomName) == "" || strings.TrimSpace(req.ParticipantIdentity) == "" {
		return IngressInfo{}, fmt.Errorf("livekit: un ingress exige room e identidad del emisor")
	}
	capas := req.Capas
	if len(capas) == 0 {
		capas = CapasCast()
	}
	peticion := map[string]any{
		"input_type":           EntradaRTMP,
		"name":                 req.Name,
		"room_name":            req.RoomName,
		"participant_identity": req.ParticipantIdentity,
		"participant_name":     req.ParticipantName,
		// Sin transcodificar, lo que sale de OBS (H.264) llegaría tal cual y la app
		// nativa no lo decodifica.
		"enable_transcoding": true,
		"video": map[string]any{
			"options": map[string]any{
				"video_codec": CodecVideoVP8,
				"layers":      capas,
			},
		},
		"audio": map[string]any{
			"options": map[string]any{
				"audio_codec": CodecAudioOpus,
			},
		},
	}
	var out IngressInfo
	if err := c.callService(ctx, "livekit.Ingress", "CreateIngress", VideoGrant{IngressAdmin: true}, peticion, &out); err != nil {
		return IngressInfo{}, err
	}
	if out.IngressID == "" || out.StreamKey == "" {
		return IngressInfo{}, fmt.Errorf("livekit: el ingress se creó sin identificador o sin clave de emisión")
	}
	return out, nil
}

// UpdateIngress apunta un ingress existente a otra room (y, si se da, cambia el
// nombre visible del emisor). LiveKit lo rechaza mientras el ingress está
// emitiendo: por eso se hace al PROGRAMAR un directo, no al empezarlo.
func (c *Client) UpdateIngress(ctx context.Context, ingressID, roomName, participantName string) (IngressInfo, error) {
	if strings.TrimSpace(ingressID) == "" || strings.TrimSpace(roomName) == "" {
		return IngressInfo{}, fmt.Errorf("livekit: actualizar un ingress exige su identificador y la room")
	}
	peticion := map[string]any{
		"ingress_id": ingressID,
		"room_name":  roomName,
	}
	if participantName != "" {
		peticion["participant_name"] = participantName
	}
	var out IngressInfo
	err := c.callService(ctx, "livekit.Ingress", "UpdateIngress", VideoGrant{IngressAdmin: true}, peticion, &out)
	return out, err
}

// DeleteIngress borra un ingress: su clave de emisión deja de valer al momento.
// Es lo que hace «regenerar la clave» junto con CreateIngress.
func (c *Client) DeleteIngress(ctx context.Context, ingressID string) error {
	if strings.TrimSpace(ingressID) == "" {
		return fmt.Errorf("livekit: borrar un ingress exige su identificador")
	}
	return c.callService(ctx, "livekit.Ingress", "DeleteIngress", VideoGrant{IngressAdmin: true},
		map[string]string{"ingress_id": ingressID}, &struct{}{})
}

// FiltroIngress acota ListIngress; vacío lista todos.
type FiltroIngress struct {
	RoomName  string
	IngressID string
}

// ListIngress lista los ingress con su estado: es lo que usa el sondeo de
// respaldo para saber quién está emitiendo aunque se pierda un webhook.
func (c *Client) ListIngress(ctx context.Context, f FiltroIngress) ([]IngressInfo, error) {
	peticion := map[string]any{}
	if f.RoomName != "" {
		peticion["room_name"] = f.RoomName
	}
	if f.IngressID != "" {
		peticion["ingress_id"] = f.IngressID
	}
	var out struct {
		Items []IngressInfo `json:"items"`
	}
	if err := c.callService(ctx, "livekit.Ingress", "ListIngress", VideoGrant{IngressAdmin: true}, peticion, &out); err != nil {
		return nil, err
	}
	return out.Items, nil
}
