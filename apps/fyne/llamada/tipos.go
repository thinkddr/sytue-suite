// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package llamada es la videollamada NATIVA de Sytue Rooms contra el LiveKit de la
// suite (ADR 0046, enmendado el 2026-09-25): el mismo SFU y el mismo token que la web
// —`POST /rooms/{id}/join` del servicio, `api.Rooms.JoinMeeting`—, sin webview.
//
// Todo es Go puro salvo la tarjeta de sonido:
//
//   - Transporte: `livekit/server-sdk-go/v2` sobre pion (señalización, ICE, DTLS, SRTP).
//   - Voz: Opus con `tphakala/go-opus` (codificador CELT y descodificador completo).
//   - Vídeo: VP8 con `thesyncim/govpx` (descodificador e interfotogramas).
//   - Micrófono y altavoz: el `Dispositivo` que se le pase; el del sistema es
//     `llamada/altavoz` (miniaudio, cgo), aparte para que esto se pruebe sin tarjeta.
//
// Los medios van al SFU propio (`livekit_url` del servicio), no a terceros.
package llamada

import (
	"errors"
	"image"
	"time"

	"git.sytue.eu/sytue/one/apps/fyne/llamada/camara"
)

// Lo que comparten la llamada de verdad (llamada.go) y el hueco de js/wasm
// (llamada_js.go): los tipos que la interfaz lee sin saber en qué destino está.

// Muestreo y tramo de la voz: 48 kHz mono en tramos de 20 ms, lo que WebRTC usa con Opus.
const (
	Muestreo = 48000
	Tramo    = Muestreo / 50 // 960 muestras = 20 ms
)

// Dispositivo es la tarjeta de sonido. Abrir arranca a la vez la captura y la
// reproducción: `capturado` recibe cada tramo del micrófono y `reproducir` rellena cada
// tramo del altavoz. Los dos se llaman desde la rutina de audio y no deben bloquear.
type Dispositivo interface {
	Abrir(capturado, reproducir func(pcm []int16)) (cerrar func(), err error)
}

// Fuente distingue la cámara de la pantalla compartida.
type Fuente uint8

// Las dos fuentes de vídeo.
const (
	Camara Fuente = iota
	Pantalla
)

// Participante es alguien en la llamada, uno mismo incluido.
type Participante struct {
	Identidad, Nombre string
	Yo                bool
	Hablando          bool
	MicroCerrado      bool
	ManoAlzada        bool
	// Video y Compartida son el último fotograma de su cámara y de su pantalla; nil sin
	// vídeo.
	Video, Compartida image.Image
}

// Opciones de la llamada.
// CifradoMedios contains only the ephemeral SDK passphrase and its authenticated
// lease deadline. Rooms obtains it over verified Olm for this exact device.
type CifradoMedios struct {
	Clave  string
	Caduca time.Time
}

type Opciones struct {
	// Rooms always requires an authenticated per-device media key. Other services
	// using this transport (Cast broadcasts) keep their separate explicit contract.
	RequiereCifrado bool
	Cifrado         *CifradoMedios

	// Micro es si se entra hablando. La web publica SOLO lo que se eligió en la
	// antesala; aquí igual: con el micro cerrado no se publica pista de audio.
	Micro bool
	// Audio es la tarjeta de sonido; nil, solo se conecta (ni se oye ni se habla).
	Audio Dispositivo
	// AlCambiar avisa de que cambió quién está, quién habla o el estado; AlFotograma, de
	// que llegó un fotograma. Se llaman desde otras rutinas: quien pinta los reenvía.
	AlCambiar, AlFotograma func()
	// Camara y Pantalla abren la cámara y la pantalla del sistema (`camara.Abrir`,
	// `camara.AbrirPantalla`); nil, la llamada no las ofrece.
	Camara, Pantalla func() (camara.Fuente, error)
	// AlSenal avisa de una mano o una reacción, propia o ajena (para pintar la reacción
	// flotando unos segundos); se llama desde otra rutina.
	AlSenal func(identidad string, s Senal)
	// AlSalir avisa de que la llamada terminó sin colgar: el SFU la cerró, echaron a
	// quien mira o se perdió la conexión.
	AlSalir func(motivo string)
}

// ErrSinSala es entrar sin URL o sin token.
var ErrSinSala = errors.New("llamada: sin URL o sin token del SFU")

// ErrSinLlamadaNativa es pedir la llamada nativa donde no la hay (js/wasm, que solo se
// compila para comprobar tipos): quien la pide abre la de la web.
var ErrSinLlamadaNativa = errors.New("llamada: la llamada nativa no existe en web; se abre la de la web")

var ErrSinClaveMedios = errors.New("llamada: verified media encryption key is missing or expired")
