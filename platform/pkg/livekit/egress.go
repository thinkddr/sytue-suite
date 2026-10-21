// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Grabación de reuniones sobre la API de Egress de LiveKit.
//
// Vive en su propio fichero y no en client.go porque es OTRO servicio Twirp
// (`livekit.Egress`, no `livekit.RoomService`) y porque el que graba no es el
// SFU: es un proceso aparte —el contenedor de egress— que se une a la sala como
// un participante más, compone el vídeo y sube el fichero. Eso tiene dos
// consecuencias que se notan en producción: si ese contenedor no está desplegado
// las llamadas de aquí fallan aunque el SFU esté perfecto, y el destino del
// fichero lo escribe ÉL, no nosotros, así que las credenciales del bucket viajan
// en la petición.
//
// El servicio no espera a que termine: `StartRoomComposite` devuelve en cuanto
// el egress arranca, y el resultado (tamaño, duración, fichero final) llega
// después por webhook, que es lo que ya alimenta `recordings_meta`.
package livekit

import (
	"context"
	"fmt"
	"strings"
)

// DestinoS3 es dónde deja el egress el fichero grabado.
//
// Las credenciales van en la petición porque quien escribe en el bucket es el
// contenedor de egress, no este servicio. Conviene que sean de un usuario capaz
// SOLO de escribir en el prefijo de grabaciones: un egress comprometido no puede
// devolver lo que no puede leer.
type DestinoS3 struct {
	AccessKey string `json:"access_key"`
	Secret    string `json:"secret"`
	Region    string `json:"region,omitempty"`
	Bucket    string `json:"bucket"`
	Endpoint  string `json:"endpoint,omitempty"`
	// ForcePathStyle: Scaleway admite las dos formas, pero con `virtual-host` el
	// nombre del bucket va en el DNS y cualquier bucket con un punto en el nombre
	// rompe la validación del certificado. Por ruta es lo predecible.
	ForcePathStyle bool `json:"force_path_style,omitempty"`
}

// EgressInfo es lo que LiveKit devuelve al arrancar o parar una grabación.
type EgressInfo struct {
	EgressID string `json:"egress_id"`
	RoomID   string `json:"room_id"`
	RoomName string `json:"room_name"`
	// Status: EGRESS_STARTING | EGRESS_ACTIVE | EGRESS_ENDING | EGRESS_COMPLETE |
	// EGRESS_FAILED | EGRESS_ABORTED | EGRESS_LIMIT_REACHED.
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
	// SegmentResults es la salida HLS del API Twirp (snake_case); los webhooks
	// llevan los mismos datos con nombres camelCase en WebhookEgressSegments.
	SegmentResults []EgressSegments `json:"segment_results,omitempty"`
}

// EgressSegments es la grabación segmentada que devuelve ListEgress.
type EgressSegments struct {
	PlaylistName     string `json:"playlist_name"`
	LivePlaylistName string `json:"live_playlist_name"`
	PlaylistLocation string `json:"playlist_location"`
	Duration         Int64  `json:"duration"`
	Size             Int64  `json:"size"`
	SegmentCount     Int64  `json:"segment_count"`
}

// ListEgress consulta una grabación por id, incluidas las que ya terminaron. El
// filtro evita paginar el historial global y no exige que su room siga viva.
func (c *Client) ListEgress(ctx context.Context, egressID string) ([]EgressInfo, error) {
	if strings.TrimSpace(egressID) == "" {
		return nil, fmt.Errorf("livekit: consultar una grabación exige su identificador")
	}
	var out struct {
		Items []EgressInfo `json:"items"`
	}
	err := c.callService(ctx, "livekit.Egress", "ListEgress", VideoGrant{RoomRecord: true},
		map[string]string{"egress_id": egressID}, &out)
	return out.Items, err
}

// StartRoomComposite arranca la grabación COMPUESTA de una sala: un solo fichero
// con el mosaico de participantes, que es lo que espera quien luego mira el acta.
//
// `layout` es la plantilla del compositor («grid» o «speaker»); vacío deja la de
// LiveKit. `ruta` es la clave dentro del bucket, sin barra inicial.
func (c *Client) StartRoomComposite(ctx context.Context, sala, layout, ruta string, dest DestinoS3) (EgressInfo, error) {
	if strings.TrimSpace(sala) == "" || strings.TrimSpace(ruta) == "" {
		return EgressInfo{}, fmt.Errorf("livekit: grabar exige sala y ruta de destino")
	}
	if dest.Bucket == "" || dest.AccessKey == "" || dest.Secret == "" {
		return EgressInfo{}, fmt.Errorf("livekit: el destino de la grabación está incompleto")
	}
	peticion := map[string]any{
		"room_name": sala,
		// `file_outputs` (lista) y no el `file` suelto, que LiveKit dejó obsoleto y
		// que en las versiones nuevas se ignora en silencio: la grabación arrancaba
		// y no se guardaba en ninguna parte.
		"file_outputs": []map[string]any{{
			"filepath": ruta,
			"s3":       dest,
		}},
	}
	if layout != "" {
		peticion["layout"] = layout
	}
	var out EgressInfo
	err := c.callService(ctx, "livekit.Egress", "StartRoomCompositeEgress",
		// RoomRecord, no RoomAdmin: son permisos distintos y el de administrar la
		// sala NO autoriza a grabarla.
		VideoGrant{Room: sala, RoomRecord: true}, peticion, &out)
	if err != nil {
		return EgressInfo{}, err
	}
	if out.EgressID == "" {
		return EgressInfo{}, fmt.Errorf("livekit: el egress arrancó sin identificador; sin él no se puede parar")
	}
	return out, nil
}

// StartRoomAudioSegments arranca un egress de SOLO AUDIO troceado, que es lo
// que se transcribe después.
//
// POR QUÉ NO SE TRANSCRIBE LA GRABACIÓN NORMAL. La compuesta es vídeo: una
// reunión de una hora son cientos de megas que ni caben en una petición al
// modelo ni tienen por qué viajar para sacar el texto. En solo audio la misma
// hora baja a decenas de megas.
//
// Y POR QUÉ TROCEADO. Aunque el audio quepa en disco, el transcriptor lee el
// fichero entero en memoria y el modelo tiene su propio tope. Trocear a mano
// exigiría entender el contenedor —y recodificar—, mientras que LiveKit ya sabe
// escribir segmentos de la duración que se le pida. El troceado sale gratis y lo
// hace quien ya tiene el audio abierto.
//
// `prefijo` es la carpeta dentro del bucket; los segmentos caen ahí numerados,
// que es lo que permite recomponer el orden después. Sin orden, la transcripción
// de una reunión no vale para nada.
func (c *Client) StartRoomAudioSegments(ctx context.Context, sala, prefijo string, segundos int, dest DestinoS3) (EgressInfo, error) {
	if strings.TrimSpace(sala) == "" || strings.TrimSpace(prefijo) == "" {
		return EgressInfo{}, fmt.Errorf("livekit: los segmentos de audio exigen sala y prefijo")
	}
	if dest.Bucket == "" || dest.AccessKey == "" || dest.Secret == "" {
		return EgressInfo{}, fmt.Errorf("livekit: el destino de la grabación está incompleto")
	}
	// Cinco minutos por defecto: con trozos de un minuto una reunión larga pasa
	// del tope de cien y multiplica las llamadas al modelo; con media hora,
	// vuelve el problema del tamaño que esto viene a resolver.
	if segundos <= 0 {
		segundos = 300
	}
	peticion := map[string]any{
		"room_name": sala,
		// Sin vídeo: es la mitad del motivo de que esto exista.
		"audio_only": true,
		"segment_outputs": []map[string]any{{
			"filename_prefix":  strings.TrimSuffix(prefijo, "/") + "/seg",
			"segment_duration": segundos,
			"s3":               dest,
		}},
	}
	var out EgressInfo
	err := c.callService(ctx, "livekit.Egress", "StartRoomCompositeEgress",
		VideoGrant{Room: sala, RoomRecord: true}, peticion, &out)
	if err != nil {
		return EgressInfo{}, err
	}
	if out.EgressID == "" {
		return EgressInfo{}, fmt.Errorf("livekit: el egress de audio arrancó sin identificador; sin él no se puede parar")
	}
	return out, nil
}

// SegmentosHLS es lo que se pide para grabar un directo en trozos HLS.
const (
	// ProtocoloHLS es el único protocolo de segmentos que tiene LiveKit.
	ProtocoloHLS = "HLS_PROTOCOL"
	// PlaylistGrabada es el índice completo, que queda cerrado al terminar y es
	// lo que después reproduce la web como vídeo bajo demanda.
	PlaylistGrabada = "index.m3u8"
	// PlaylistEnVivo es la ventana deslizante que LiveKit reescribe mientras graba.
	PlaylistEnVivo = "live.m3u8"
	// PrefijoSegmento es el nombre base de cada trozo (`seg_00001.ts`).
	PrefijoSegmento = "seg"
)

// StartParticipantSegments graba las pistas de UN participante —el emisor de un
// directo— en segmentos HLS dentro de `prefijo` del bucket: `prefijo/seg_*.ts`,
// `prefijo/index.m3u8` (completo, al terminar) y `prefijo/live.m3u8`.
//
// POR QUÉ POR PARTICIPANTE Y NO «TRACK COMPOSITE». TrackComposite pide los SID
// de las pistas de audio y vídeo, y cuando el ingress avisa de que ha empezado
// esas pistas aún no existen: la grabación arrancaría vacía o habría que esperar
// a dos webhooks más. El egress de participante toma las pistas por IDENTIDAD,
// las que haya y las que lleguen, que es lo que hace falta al primer aviso.
//
// El egress se une a la room como un participante más (kind EGRESS): quien
// cuenta espectadores lo excluye. Devuelve en cuanto arranca; el resultado
// (playlist, duración, tamaño) llega por webhook `egress_ended`.
func (c *Client) StartParticipantSegments(ctx context.Context, sala, identidad, prefijo string, segundos int, dest DestinoS3) (EgressInfo, error) {
	if strings.TrimSpace(sala) == "" || strings.TrimSpace(identidad) == "" || strings.TrimSpace(prefijo) == "" {
		return EgressInfo{}, fmt.Errorf("livekit: grabar un directo exige sala, identidad del emisor y prefijo")
	}
	if dest.Bucket == "" || dest.AccessKey == "" || dest.Secret == "" {
		return EgressInfo{}, fmt.Errorf("livekit: el destino de la grabación está incompleto")
	}
	if segundos <= 0 {
		segundos = 4
	}
	prefijo = strings.TrimSuffix(prefijo, "/")
	peticion := map[string]any{
		"room_name": sala,
		"identity":  identidad,
		"segment_outputs": []map[string]any{{
			"protocol":           ProtocoloHLS,
			"filename_prefix":    prefijo + "/" + PrefijoSegmento,
			"playlist_name":      prefijo + "/" + PlaylistGrabada,
			"live_playlist_name": prefijo + "/" + PlaylistEnVivo,
			"segment_duration":   segundos,
			"s3":                 dest,
		}},
	}
	var out EgressInfo
	err := c.callService(ctx, "livekit.Egress", "StartParticipantEgress",
		VideoGrant{Room: sala, RoomRecord: true}, peticion, &out)
	if err != nil {
		return EgressInfo{}, err
	}
	if out.EgressID == "" {
		return EgressInfo{}, fmt.Errorf("livekit: el egress del directo arrancó sin identificador; sin él no se puede parar")
	}
	return out, nil
}

// StopEgress detiene una grabación en curso por su identificador.
//
// Que falle NO significa que se siga grabando: LiveKit responde con error si el
// egress ya había terminado por su cuenta —la sala se vació, el proceso murió—,
// y ese caso lo distingue quien llama.
func (c *Client) StopEgress(ctx context.Context, egressID string) (EgressInfo, error) {
	if strings.TrimSpace(egressID) == "" {
		return EgressInfo{}, fmt.Errorf("livekit: parar una grabación exige su identificador")
	}
	var out EgressInfo
	err := c.callService(ctx, "livekit.Egress", "StopEgress",
		VideoGrant{RoomRecord: true},
		map[string]string{"egress_id": egressID}, &out)
	return out, err
}
