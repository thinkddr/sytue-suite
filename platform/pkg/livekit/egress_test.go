// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Checks del cliente de grabación. Ejecuta: `go test ./services/rooms/...`.
//
// Lo que se comprueba aquí no es que la llamada "funcione" —eso lo dirá LiveKit—
// sino que lo que sale por el cable es lo que LiveKit espera, y que lo que NO
// puede pasar se corta antes de salir. Una grabación que arranca mal no falla
// ruidosamente: se une a la sala, avisa a todo el mundo de que se está grabando y
// después no guarda nada.
package livekit

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// servidorEgress devuelve un cliente apuntando a un LiveKit de mentira que
// captura la ruta y el cuerpo de la última llamada.
func servidorEgress(t *testing.T, respuesta string) (*Client, *struct {
	Ruta   string
	Cuerpo map[string]any
	Auth   string
},
) {
	t.Helper()
	capt := &struct {
		Ruta   string
		Cuerpo map[string]any
		Auth   string
	}{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capt.Ruta = r.URL.Path
		capt.Auth = r.Header.Get("Authorization")
		_ = json.NewDecoder(r.Body).Decode(&capt.Cuerpo)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(respuesta))
	}))
	t.Cleanup(srv.Close)
	return NewClient(srv.URL, NewSigner("clave", "secreto")), capt
}

func TestEgressVaAlServicioTwirpCorrecto(t *testing.T) {
	// El error que esto evita: `livekit.Egress` NO es `livekit.RoomService`. Con
	// la ruta clavada al segundo, la llamada devuelve 404 y la grabación no
	// arranca nunca —sin que el mensaje mencione la grabación por ningún lado—.
	c, capt := servidorEgress(t, `{"egress_id":"EG_1","status":"EGRESS_STARTING"}`)
	if _, err := c.StartRoomComposite(context.Background(), "sala-1", "grid", "acme/sala-1/1.mp4", DestinoS3{
		AccessKey: "ak", Secret: "sk", Bucket: "grabaciones",
	}); err != nil {
		t.Fatalf("StartRoomComposite: %v", err)
	}
	if capt.Ruta != "/twirp/livekit.Egress/StartRoomCompositeEgress" {
		t.Errorf("ruta = %q, quería la del servicio Egress", capt.Ruta)
	}
	if capt.Auth == "" || !strings.HasPrefix(capt.Auth, "Bearer ") {
		t.Error("la llamada salió sin token de servidor")
	}
}

func TestEgressPideFileOutputsYNoElFicheroSuelto(t *testing.T) {
	// `file` (singular) está obsoleto y las versiones nuevas lo IGNORAN en
	// silencio: la grabación arrancaría y no se guardaría en ninguna parte.
	c, capt := servidorEgress(t, `{"egress_id":"EG_2","status":"EGRESS_STARTING"}`)
	if _, err := c.StartRoomComposite(context.Background(), "sala-1", "", "acme/x.mp4", DestinoS3{
		AccessKey: "ak", Secret: "sk", Bucket: "grabaciones",
	}); err != nil {
		t.Fatalf("StartRoomComposite: %v", err)
	}
	salidas, ok := capt.Cuerpo["file_outputs"].([]any)
	if !ok || len(salidas) != 1 {
		t.Fatalf("no se mandó file_outputs: %v", capt.Cuerpo)
	}
	if _, hay := capt.Cuerpo["file"]; hay {
		t.Error("se mandó el `file` suelto y obsoleto")
	}
	salida := salidas[0].(map[string]any)
	if salida["filepath"] != "acme/x.mp4" {
		t.Errorf("filepath = %v", salida["filepath"])
	}
	if _, hay := salida["s3"]; !hay {
		t.Error("la salida no lleva destino S3: el egress no sabría dónde dejar el fichero")
	}
}

func TestEgressSinLayoutNoMandaElCampo(t *testing.T) {
	// Mandar `layout: ""` no es lo mismo que no mandarlo: el compositor lo toma
	// como una plantilla llamada «» y falla, en vez de usar la suya por defecto.
	c, capt := servidorEgress(t, `{"egress_id":"EG_3","status":"EGRESS_STARTING"}`)
	if _, err := c.StartRoomComposite(context.Background(), "s", "", "r.mp4", DestinoS3{
		AccessKey: "ak", Secret: "sk", Bucket: "b",
	}); err != nil {
		t.Fatalf("StartRoomComposite: %v", err)
	}
	if _, hay := capt.Cuerpo["layout"]; hay {
		t.Error("se mandó layout vacío en vez de omitirlo")
	}
}

func TestEgressRechazaLoQueNoPuedeGrabar(t *testing.T) {
	c, _ := servidorEgress(t, `{"egress_id":"EG_4"}`)
	dest := DestinoS3{AccessKey: "ak", Secret: "sk", Bucket: "b"}
	casos := []struct {
		nombre     string
		sala, ruta string
		destino    DestinoS3
	}{
		{"sin sala", "", "r.mp4", dest},
		{"sala en blanco", "   ", "r.mp4", dest},
		{"sin ruta", "s", "", dest},
		// Un destino a medias es el caso peligroso: el egress arrancaría, se
		// uniría a la sala avisando a todos de que se graba, y fallaría al subir.
		{"destino sin bucket", "s", "r.mp4", DestinoS3{AccessKey: "ak", Secret: "sk"}},
		{"destino sin credenciales", "s", "r.mp4", DestinoS3{Bucket: "b"}},
	}
	for _, c2 := range casos {
		t.Run(c2.nombre, func(t *testing.T) {
			if _, err := c.StartRoomComposite(context.Background(), c2.sala, "", c2.ruta, c2.destino); err == nil {
				t.Error("debería haberse rechazado ANTES de salir por el cable")
			}
		})
	}
}

func TestEgressSinIdentificadorEsUnFallo(t *testing.T) {
	// Si LiveKit responde 200 pero sin egress_id, la grabación está corriendo y
	// NADIE puede pararla: se queda hasta que la sala muera. Mejor tratarlo como
	// error que devolver una cadena vacía que el llamante guardará tan tranquilo.
	c, _ := servidorEgress(t, `{"status":"EGRESS_STARTING"}`)
	if _, err := c.StartRoomComposite(context.Background(), "s", "", "r.mp4", DestinoS3{
		AccessKey: "ak", Secret: "sk", Bucket: "b",
	}); err == nil {
		t.Error("una respuesta sin egress_id debería fallar")
	}
}

func TestStopEgressExigeIdentificador(t *testing.T) {
	c, _ := servidorEgress(t, `{"egress_id":"EG_5","status":"EGRESS_ENDING"}`)
	if _, err := c.StopEgress(context.Background(), "  "); err == nil {
		t.Error("parar sin identificador debería fallar antes de llamar")
	}
	if _, err := c.StopEgress(context.Background(), "EG_5"); err != nil {
		t.Errorf("StopEgress: %v", err)
	}
}

// --- Audio troceado para transcribir -----------------------------------------

// LO QUE HACE ÚTIL A ESTE EGRESS son sus dos diferencias con el normal, y las dos
// se pueden perder en un refactor sin que nada falle: sin `audio_only` se
// grabaría vídeo otra vez —cientos de megas que no caben en una petición al
// modelo— y sin `segment_outputs` saldría un solo fichero que hay que trocear a
// mano, que era justo lo que se venía a evitar.
func TestAudioSegmentadoPideSoloAudioYEnTrozos(t *testing.T) {
	c, capt := servidorEgress(t, `{"egress_id":"EG_A","status":"EGRESS_STARTING"}`)
	if _, err := c.StartRoomAudioSegments(context.Background(), "sala-1", "acme/reunion", 300,
		DestinoS3{AccessKey: "ak", Secret: "sk", Bucket: "grabaciones"}); err != nil {
		t.Fatalf("StartRoomAudioSegments: %v", err)
	}
	if capt.Cuerpo["audio_only"] != true {
		t.Error("no se pidió solo audio: volvería a grabarse vídeo")
	}
	salidas, ok := capt.Cuerpo["segment_outputs"].([]any)
	if !ok || len(salidas) != 1 {
		t.Fatalf("no se mandó segment_outputs: %v", capt.Cuerpo)
	}
	seg := salidas[0].(map[string]any)
	if seg["filename_prefix"] != "acme/reunion/seg" {
		t.Errorf("prefijo = %v", seg["filename_prefix"])
	}
	if seg["segment_duration"] != float64(300) {
		t.Errorf("duración = %v", seg["segment_duration"])
	}
	if _, hay := seg["s3"]; !hay {
		t.Error("los segmentos no llevan destino: el egress no sabría dónde dejarlos")
	}
	// Y NO se manda `file_outputs`: la grabación de vídeo es otro egress.
	if _, hay := capt.Cuerpo["file_outputs"]; hay {
		t.Error("se pidió también un fichero de vídeo")
	}
}

func TestAudioSegmentadoPonePorDefectoCincoMinutos(t *testing.T) {
	c, capt := servidorEgress(t, `{"egress_id":"EG_B","status":"EGRESS_STARTING"}`)
	if _, err := c.StartRoomAudioSegments(context.Background(), "sala-1", "acme/r", 0,
		DestinoS3{AccessKey: "ak", Secret: "sk", Bucket: "b"}); err != nil {
		t.Fatalf("StartRoomAudioSegments: %v", err)
	}
	seg := capt.Cuerpo["segment_outputs"].([]any)[0].(map[string]any)
	if seg["segment_duration"] != float64(300) {
		t.Errorf("duración por defecto = %v, quería 300", seg["segment_duration"])
	}
}

func TestAudioSegmentadoRechazaLoIncompleto(t *testing.T) {
	c, _ := servidorEgress(t, `{"egress_id":"EG_C","status":"EGRESS_STARTING"}`)
	casos := []struct {
		nombre, sala, prefijo string
		dest                  DestinoS3
	}{
		{"sin sala", "", "p", DestinoS3{AccessKey: "ak", Secret: "sk", Bucket: "b"}},
		{"sin prefijo", "s", "", DestinoS3{AccessKey: "ak", Secret: "sk", Bucket: "b"}},
		{"sin bucket", "s", "p", DestinoS3{AccessKey: "ak", Secret: "sk"}},
	}
	for _, c2 := range casos {
		if _, err := c.StartRoomAudioSegments(context.Background(), c2.sala, c2.prefijo, 60, c2.dest); err == nil {
			t.Errorf("%s: debería fallar antes de llamar a LiveKit", c2.nombre)
		}
	}
}
