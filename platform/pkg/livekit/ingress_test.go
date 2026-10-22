// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Checks del cliente de ingesta RTMP y de los participantes. Ejecuta:
// `go test ./platform/pkg/livekit/...`.
//
// Como en egress_test.go: no se comprueba que LiveKit acepte la llamada, sino
// que lo que sale por el cable es lo que LiveKit espera (servicio Twirp, nombres
// de campo, enumeraciones por su nombre) y que lo que vuelve se lee, incluidos
// los enteros que protojson manda entre comillas.
package livekit

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// servidorTwirp devuelve un cliente contra un LiveKit de mentira que captura la
// ruta, el cuerpo y la autorización de la última llamada y contesta `respuesta`.
func servidorTwirp(t *testing.T, respuesta string) (*Client, *struct {
	Ruta   string
	Cuerpo map[string]any
	Grant  VideoGrant
},
) {
	t.Helper()
	capt := &struct {
		Ruta   string
		Cuerpo map[string]any
		Grant  VideoGrant
	}{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capt.Ruta = r.URL.Path
		capt.Cuerpo = map[string]any{}
		_ = json.NewDecoder(r.Body).Decode(&capt.Cuerpo)
		if tok, ok := cutBearer(r.Header.Get("Authorization")); ok {
			capt.Grant = decodeJWTClaims(t, tok).Video
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(respuesta))
	}))
	t.Cleanup(srv.Close)
	return NewClient(srv.URL, NewSigner("clave", "secreto")), capt
}

func cutBearer(h string) (string, bool) {
	const p = "Bearer "
	if len(h) > len(p) && h[:len(p)] == p {
		return h[len(p):], true
	}
	return "", false
}

func TestCreateIngressMandaRTMPConVP8YOpus(t *testing.T) {
	c, capt := servidorTwirp(t, `{"ingress_id":"IN_1","stream_key":"sk-abc","url":"rtmp://ingest/x",`+
		`"input_type":"RTMP_INPUT","room_name":"cast-acme-1","participant_identity":"emisor-ana",`+
		`"state":{"status":"ENDPOINT_INACTIVE","started_at":"0"}}`)
	info, err := c.CreateIngress(context.Background(), IngressRequest{
		Name: "acme/ana", RoomName: "cast-acme-1", ParticipantIdentity: "emisor-ana", ParticipantName: "Ana",
	})
	if err != nil {
		t.Fatalf("CreateIngress: %v", err)
	}
	if capt.Ruta != "/twirp/livekit.Ingress/CreateIngress" {
		t.Errorf("ruta = %q: el ingress es OTRO servicio Twirp", capt.Ruta)
	}
	if !capt.Grant.IngressAdmin {
		t.Error("el token de servidor no lleva ingressAdmin: LiveKit contestaría 401")
	}
	if capt.Cuerpo["input_type"] != EntradaRTMP || capt.Cuerpo["enable_transcoding"] != true {
		t.Errorf("entrada/transcodificación: %v", capt.Cuerpo)
	}
	if capt.Cuerpo["room_name"] != "cast-acme-1" || capt.Cuerpo["participant_identity"] != "emisor-ana" || capt.Cuerpo["participant_name"] != "Ana" {
		t.Errorf("room/identidad: %v", capt.Cuerpo)
	}
	video, _ := capt.Cuerpo["video"].(map[string]any)
	opciones, _ := video["options"].(map[string]any)
	if opciones["video_codec"] != CodecVideoVP8 {
		t.Errorf("video_codec = %v, la app nativa solo decodifica VP8", opciones["video_codec"])
	}
	capas, _ := opciones["layers"].([]any)
	if len(capas) != 2 {
		t.Fatalf("capas = %d, quería 720p y 360p", len(capas))
	}
	alta, _ := capas[0].(map[string]any)
	if alta["quality"] != "HIGH" || alta["height"] != float64(720) || alta["bitrate"] != float64(2_500_000) {
		t.Errorf("capa alta: %v", alta)
	}
	baja, _ := capas[1].(map[string]any)
	if baja["quality"] != "LOW" || baja["height"] != float64(360) || baja["bitrate"] != float64(800_000) {
		t.Errorf("capa baja: %v", baja)
	}
	audio, _ := capt.Cuerpo["audio"].(map[string]any)
	opcionesAudio, _ := audio["options"].(map[string]any)
	if opcionesAudio["audio_codec"] != CodecAudioOpus {
		t.Errorf("audio_codec = %v", opcionesAudio["audio_codec"])
	}
	// La respuesta: la clave de emisión es lo que se le enseña a la persona.
	if info.IngressID != "IN_1" || info.StreamKey != "sk-abc" || info.URL != "rtmp://ingest/x" || info.RoomName != "cast-acme-1" {
		t.Errorf("respuesta mal leída: %+v", info)
	}
	if info.Emitiendo() {
		t.Error("un ingress ENDPOINT_INACTIVE no está emitiendo")
	}
}

func TestCreateIngressRechazaLoIncompleto(t *testing.T) {
	c, _ := servidorTwirp(t, `{"ingress_id":"IN_2","stream_key":"sk"}`)
	if _, err := c.CreateIngress(context.Background(), IngressRequest{RoomName: "r"}); err == nil {
		t.Error("sin identidad del emisor debería fallar antes de llamar")
	}
	if _, err := c.CreateIngress(context.Background(), IngressRequest{ParticipantIdentity: "e"}); err == nil {
		t.Error("sin room debería fallar antes de llamar")
	}
}

func TestCreateIngressSinClaveEsUnFallo(t *testing.T) {
	// Un 200 sin stream_key dejaría un ingress creado que nadie puede usar ni
	// enseñar: mejor error que guardar una clave vacía.
	c, _ := servidorTwirp(t, `{"ingress_id":"IN_3"}`)
	if _, err := c.CreateIngress(context.Background(), IngressRequest{RoomName: "r", ParticipantIdentity: "e"}); err == nil {
		t.Error("una respuesta sin stream_key debería fallar")
	}
}

func TestUpdateIngressCambiaLaRoom(t *testing.T) {
	c, capt := servidorTwirp(t, `{"ingress_id":"IN_1","stream_key":"sk","room_name":"cast-acme-2"}`)
	info, err := c.UpdateIngress(context.Background(), "IN_1", "cast-acme-2", "")
	if err != nil {
		t.Fatalf("UpdateIngress: %v", err)
	}
	if capt.Ruta != "/twirp/livekit.Ingress/UpdateIngress" {
		t.Errorf("ruta = %q", capt.Ruta)
	}
	if capt.Cuerpo["ingress_id"] != "IN_1" || capt.Cuerpo["room_name"] != "cast-acme-2" {
		t.Errorf("cuerpo: %v", capt.Cuerpo)
	}
	if _, hay := capt.Cuerpo["participant_name"]; hay {
		t.Error("se mandó participant_name vacío: LiveKit lo tomaría como «borra el nombre»")
	}
	if info.RoomName != "cast-acme-2" {
		t.Errorf("room de vuelta = %q", info.RoomName)
	}
	if _, err := c.UpdateIngress(context.Background(), "", "r", ""); err == nil {
		t.Error("sin identificador debería fallar antes de llamar")
	}
}

func TestDeleteIngress(t *testing.T) {
	c, capt := servidorTwirp(t, `{"ingress_id":"IN_1"}`)
	if err := c.DeleteIngress(context.Background(), "IN_1"); err != nil {
		t.Fatalf("DeleteIngress: %v", err)
	}
	if capt.Ruta != "/twirp/livekit.Ingress/DeleteIngress" || capt.Cuerpo["ingress_id"] != "IN_1" {
		t.Errorf("ruta/cuerpo: %s %v", capt.Ruta, capt.Cuerpo)
	}
	if err := c.DeleteIngress(context.Background(), " "); err == nil {
		t.Error("sin identificador debería fallar antes de llamar")
	}
}

func TestListIngressFiltraYLeeElEstado(t *testing.T) {
	// protojson manda los int64 entre comillas: `started_at` llega como cadena.
	c, capt := servidorTwirp(t, `{"items":[{"ingress_id":"IN_1","stream_key":"sk","room_name":"cast-acme-1",`+
		`"state":{"status":"ENDPOINT_PUBLISHING","started_at":"1785238899"}}]}`)
	items, err := c.ListIngress(context.Background(), FiltroIngress{RoomName: "cast-acme-1"})
	if err != nil {
		t.Fatalf("ListIngress: %v", err)
	}
	if capt.Ruta != "/twirp/livekit.Ingress/ListIngress" || capt.Cuerpo["room_name"] != "cast-acme-1" {
		t.Errorf("ruta/filtro: %s %v", capt.Ruta, capt.Cuerpo)
	}
	if _, hay := capt.Cuerpo["ingress_id"]; hay {
		t.Error("se mandó ingress_id vacío como filtro")
	}
	if len(items) != 1 || !items[0].Emitiendo() || items[0].State.StartedAt != 1785238899 {
		t.Fatalf("estado mal leído: %+v", items)
	}
	// Sin filtro, el cuerpo va vacío: lista todos.
	if _, err := c.ListIngress(context.Background(), FiltroIngress{}); err != nil {
		t.Fatal(err)
	}
	if len(capt.Cuerpo) != 0 {
		t.Errorf("sin filtro se mandó %v", capt.Cuerpo)
	}
}

func TestListParticipantsDistingueClases(t *testing.T) {
	c, capt := servidorTwirp(t, `{"participants":[`+
		`{"sid":"PA_1","identity":"emisor-ana","kind":"INGRESS","state":"ACTIVE","tracks":[{"sid":"TR_1","type":"VIDEO"}]},`+
		`{"sid":"PA_2","identity":"zoe","kind":"STANDARD","state":"ACTIVE"},`+
		`{"sid":"PA_3","identity":"EG_x","kind":"EGRESS","state":"ACTIVE"}]}`)
	ps, err := c.ListParticipants(context.Background(), "cast-acme-1")
	if err != nil {
		t.Fatalf("ListParticipants: %v", err)
	}
	if capt.Ruta != "/twirp/livekit.RoomService/ListParticipants" || capt.Cuerpo["room"] != "cast-acme-1" {
		t.Errorf("ruta/cuerpo: %s %v", capt.Ruta, capt.Cuerpo)
	}
	if !capt.Grant.RoomAdmin || capt.Grant.Room != "cast-acme-1" {
		t.Errorf("grant del token de servidor: %+v", capt.Grant)
	}
	if len(ps) != 3 || ps[0].Kind != ParticipanteIngress || ps[1].Kind != ParticipanteEstandar || ps[2].Kind != ParticipanteEgress {
		t.Fatalf("clases mal leídas: %+v", ps)
	}
	if len(ps[0].Tracks) != 1 || ps[0].Tracks[0].Type != "VIDEO" {
		t.Errorf("pistas del emisor: %+v", ps[0].Tracks)
	}
	if _, err := c.ListParticipants(context.Background(), ""); err == nil {
		t.Error("sin room debería fallar antes de llamar")
	}
}

// --- Grabación del directo en segmentos HLS ---------------------------------

func TestParticipantSegmentsPideHLSPorIdentidad(t *testing.T) {
	c, capt := servidorTwirp(t, `{"egress_id":"EG_1","status":"EGRESS_STARTING"}`)
	dest := DestinoS3{AccessKey: "ak", Secret: "sk", Bucket: "cast-medios", Endpoint: "https://s3.fr-par.scw.cloud", ForcePathStyle: true}
	info, err := c.StartParticipantSegments(context.Background(), "cast-acme-1", "emisor-ana", "tenants/acme/v1/live/", 4, dest)
	if err != nil {
		t.Fatalf("StartParticipantSegments: %v", err)
	}
	if info.EgressID != "EG_1" {
		t.Errorf("egress_id = %q", info.EgressID)
	}
	if capt.Ruta != "/twirp/livekit.Egress/StartParticipantEgress" {
		t.Errorf("ruta = %q: por participante, no TrackComposite (las pistas aún no existen al primer aviso)", capt.Ruta)
	}
	if !capt.Grant.RoomRecord || capt.Grant.Room != "cast-acme-1" {
		t.Errorf("grant: %+v", capt.Grant)
	}
	if capt.Cuerpo["room_name"] != "cast-acme-1" || capt.Cuerpo["identity"] != "emisor-ana" {
		t.Errorf("room/identidad: %v", capt.Cuerpo)
	}
	salidas, _ := capt.Cuerpo["segment_outputs"].([]any)
	if len(salidas) != 1 {
		t.Fatalf("segment_outputs: %v", capt.Cuerpo)
	}
	seg, _ := salidas[0].(map[string]any)
	if seg["protocol"] != ProtocoloHLS || seg["segment_duration"] != float64(4) {
		t.Errorf("protocolo/duración: %v", seg)
	}
	// La barra final del prefijo no duplica: `tenants/acme/v1/live//seg` sería otra carpeta.
	if seg["filename_prefix"] != "tenants/acme/v1/live/seg" || seg["playlist_name"] != "tenants/acme/v1/live/index.m3u8" ||
		seg["live_playlist_name"] != "tenants/acme/v1/live/live.m3u8" {
		t.Errorf("rutas: %v", seg)
	}
	s3, _ := seg["s3"].(map[string]any)
	if s3["bucket"] != "cast-medios" || s3["force_path_style"] != true {
		t.Errorf("destino: %v", s3)
	}
	// Y NO va un fichero suelto: sería otra grabación.
	if _, hay := capt.Cuerpo["file_outputs"]; hay {
		t.Error("se pidió también un fichero")
	}
}

func TestParticipantSegmentsRechazaLoIncompleto(t *testing.T) {
	c, _ := servidorTwirp(t, `{"egress_id":"EG_1"}`)
	dest := DestinoS3{AccessKey: "ak", Secret: "sk", Bucket: "b"}
	casos := []struct {
		nombre, sala, identidad, prefijo string
		dest                             DestinoS3
	}{
		{"sin sala", "", "e", "p", dest},
		{"sin identidad", "s", "", "p", dest},
		{"sin prefijo", "s", "e", "", dest},
		{"destino sin credenciales", "s", "e", "p", DestinoS3{Bucket: "b"}},
	}
	for _, c2 := range casos {
		if _, err := c.StartParticipantSegments(context.Background(), c2.sala, c2.identidad, c2.prefijo, 4, c2.dest); err == nil {
			t.Errorf("%s: debería fallar antes de llamar a LiveKit", c2.nombre)
		}
	}
	// Sin identificador de vuelta, la grabación corre y nadie puede pararla.
	c2, _ := servidorTwirp(t, `{"status":"EGRESS_STARTING"}`)
	if _, err := c2.StartParticipantSegments(context.Background(), "s", "e", "p", 4, dest); err == nil {
		t.Error("una respuesta sin egress_id debería fallar")
	}
}
