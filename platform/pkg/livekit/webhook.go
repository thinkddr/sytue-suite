// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package livekit

import (
	"encoding/json"
	"fmt"
	"strconv"
)

// Int64 es un entero de 64 bits que acepta TANTO número como cadena en el JSON.
//
// No es una manía: LiveKit serializa sus mensajes con protojson, y el estándar de
// protobuf JSON manda los int64/uint64 **entre comillas** para no perder precisión en
// lenguajes cuyo número nativo es un float64. Declarar estos campos como `int64` a secas
// hacía que TODO webhook se rechazara con «cannot unmarshal string into int64» — y como
// el SFU no tenía configurado a quién avisar, nunca llegó uno que lo destapara.
type Int64 int64

// UnmarshalJSON acepta 1785238899 y "1785238899" por igual. Un valor ilegible se
// resuelve como 0 en vez de tumbar el evento entero: quien lo consume ya trata el 0 como
// «sin dato» (ver eventTime), y perder una marca de tiempo no justifica descartar un
// aviso de que una reunión terminó.
func (n *Int64) UnmarshalJSON(b []byte) error {
	s := string(b)
	if s == "null" {
		return nil
	}
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		s = s[1 : len(s)-1]
	}
	if s == "" {
		return nil
	}
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		//nolint:nilerr // deliberado: una marca de tiempo ilegible se queda en 0 (que
		// quien la consume ya trata como «sin dato») en vez de descartar el aviso
		// entero de que una reunión ha terminado.
		return nil
	}
	*n = Int64(v)
	return nil
}

// WebhookEvent es el sobre de un webhook de LiveKit (formato JSON público). Solo
// modelamos los campos que Sytue Rooms consume; el resto se ignora.
type WebhookEvent struct {
	// Event es el tipo: room_started | room_finished | participant_joined |
	// participant_left | track_published | track_unpublished | egress_started |
	// egress_updated | egress_ended | ingress_started | ingress_ended.
	Event       string         `json:"event"`
	Room        *WebhookRoom   `json:"room,omitempty"`
	Participant *WebhookPart   `json:"participant,omitempty"`
	EgressInfo  *WebhookEgress `json:"egressInfo,omitempty"`
	// IngressInfo va en los eventos ingress_*: qué puerta RTMP empezó o dejó de
	// emitir y en qué room publica.
	IngressInfo *WebhookIngress `json:"ingressInfo,omitempty"`
	// Track va en los eventos track_*: la pista que se publicó o retiró.
	Track     *WebhookTrack `json:"track,omitempty"`
	ID        string        `json:"id"`
	CreatedAt Int64         `json:"createdAt"`
}

// WebhookIngress es la sección ingressInfo de un webhook (nombres protojson, en
// camelCase, a diferencia de las respuestas Twirp de IngressInfo).
type WebhookIngress struct {
	IngressID           string               `json:"ingressId"`
	Name                string               `json:"name"`
	RoomName            string               `json:"roomName"`
	ParticipantIdentity string               `json:"participantIdentity"`
	ParticipantName     string               `json:"participantName"`
	State               *WebhookIngressState `json:"state,omitempty"`
}

// WebhookIngressState es el estado que acompaña a un evento ingress_*.
type WebhookIngressState struct {
	Status    string `json:"status"`
	Error     string `json:"error,omitempty"`
	RoomID    string `json:"roomId,omitempty"`
	StartedAt Int64  `json:"startedAt,omitempty"`
	EndedAt   Int64  `json:"endedAt,omitempty"`
}

// WebhookTrack es la pista de un evento track_*.
type WebhookTrack struct {
	SID    string `json:"sid"`
	Type   string `json:"type"` // AUDIO | VIDEO
	Source string `json:"source"`
	Muted  bool   `json:"muted"`
}

// WebhookRoom es la sección room de un webhook.
type WebhookRoom struct {
	SID             string `json:"sid"`
	Name            string `json:"name"`
	NumParticipants uint32 `json:"numParticipants"`
	Metadata        string `json:"metadata"`
	CreationTime    Int64  `json:"creationTime"`
}

// WebhookPart es la sección participant de un webhook.
type WebhookPart struct {
	SID      string `json:"sid"`
	Identity string `json:"identity"`
	Name     string `json:"name"`
	JoinedAt Int64  `json:"joinedAt"`
	State    string `json:"state"`
	// Kind: STANDARD | INGRESS | EGRESS | … (ver ParticipanteEstandar). Vacío en
	// versiones viejas del servidor: se lee como STANDARD.
	Kind     string `json:"kind"`
	Metadata string `json:"metadata"`
}

// Espectador dice si este participante es alguien que MIRA: ni el ingress que
// trae la señal ni el egress que la graba cuentan.
func (p WebhookPart) Espectador() bool {
	return p.Kind == "" || p.Kind == ParticipanteEstandar
}

// WebhookEgress es la sección egressInfo (grabación) de un webhook.
type WebhookEgress struct {
	EgressID    string              `json:"egressId"`
	RoomName    string              `json:"roomName"`
	Status      string              `json:"status"`
	Error       string              `json:"error,omitempty"`
	File        *WebhookEgressFile  `json:"file,omitempty"`
	FileResults []WebhookEgressFile `json:"fileResults,omitempty"`
	// SegmentResults es lo que deja un egress de segmentos (HLS): dónde quedó la
	// playlist completa y cuánto duró.
	SegmentResults []WebhookEgressSegments `json:"segmentResults,omitempty"`
}

// WebhookEgressSegments describe la grabación troceada de un egress.
type WebhookEgressSegments struct {
	PlaylistName     string `json:"playlistName"`
	LivePlaylistName string `json:"livePlaylistName"`
	PlaylistLocation string `json:"playlistLocation"`
	// Duration en nanosegundos (un time.Duration de Go, como en FileInfo); Size en bytes.
	Duration     Int64 `json:"duration"`
	Size         Int64 `json:"size"`
	SegmentCount Int64 `json:"segmentCount"`
}

// WebhookEgressFile describe el fichero resultante de una grabación.
type WebhookEgressFile struct {
	Filename string `json:"filename"`
	Location string `json:"location"`
	Size     Int64  `json:"size"`
	Duration Int64  `json:"duration"`
}

// Tipos de evento de webhook que Sytue Rooms maneja.
const (
	EventRoomStarted       = "room_started"
	EventRoomFinished      = "room_finished"
	EventParticipantJoined = "participant_joined"
	EventParticipantLeft   = "participant_left"
	EventEgressStarted     = "egress_started"
	EventEgressUpdated     = "egress_updated"
	EventEgressEnded       = "egress_ended"
	// Los que consume Sytue Cast además de los de arriba.
	EventTrackPublished   = "track_published"
	EventTrackUnpublished = "track_unpublished"
	EventIngressStarted   = "ingress_started"
	EventIngressEnded     = "ingress_ended"
)

// ParseWebhook verifica la firma del JWT de la cabecera Authorization frente al
// cuerpo y, si es válida, decodifica el evento. Es el único punto de entrada de
// los webhooks: rechaza cualquier cuerpo no firmado por el api_secret.
func (s *Signer) ParseWebhook(authToken string, body []byte) (WebhookEvent, error) {
	if authToken == "" {
		return WebhookEvent{}, fmt.Errorf("livekit: webhook sin cabecera Authorization")
	}
	if err := s.verifyWebhook(authToken, body); err != nil {
		return WebhookEvent{}, err
	}
	var ev WebhookEvent
	if err := json.Unmarshal(body, &ev); err != nil {
		return WebhookEvent{}, fmt.Errorf("livekit: cuerpo de webhook inválido: %w", err)
	}
	if ev.Event == "" {
		return WebhookEvent{}, fmt.Errorf("livekit: webhook sin tipo de evento")
	}
	return ev, nil
}
