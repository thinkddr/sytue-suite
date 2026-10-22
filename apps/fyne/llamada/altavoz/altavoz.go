// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build !js

// Package altavoz es la tarjeta de sonido del sistema para `llamada`: micrófono y
// altavoz a la vez con miniaudio (`gen2brain/malgo`, cgo sin bibliotecas que enlazar
// salvo `-ldl` en Linux). Cubre los cinco sistemas: CoreAudio en macOS e iOS, WASAPI en
// Windows, PulseAudio/ALSA en Linux y AAudio/OpenSL ES en Android.
//
// Va aparte de `llamada` para que la llamada se pruebe sin tarjeta (con el doble de los
// guardianes) y sin compilar miniaudio.
//
// # El eco
//
// Sin cancelación de eco, quien usa altavoces se oye a sí mismo en los demás. Se usa la
// del SISTEMA, que es la que ya está afinada para su hardware:
//
//   - macOS e iOS: la unidad de audio VoiceProcessingIO (altavoz_darwin.go), la de
//     FaceTime.
//   - Windows: el DSP de captura de voz de Windows (altavoz_windows.go).
//   - Android: el preajuste de entrada «comunicación de voz» de AAudio.
//   - Linux: miniaudio sin eco; PulseAudio y PipeWire lo quitan si el usuario carga su
//     módulo `echo-cancel`. El APM de libwebrtc, que es lo que usa el navegador, sigue en
//     el roadmap para cuando el del sistema no baste.
package altavoz

import (
	"encoding/binary"
	"encoding/hex"
	"sync"
	"unsafe"

	"github.com/gen2brain/malgo"

	"git.sytue.eu/sytue/one/apps/fyne/llamada"
)

// Sistema es la tarjeta del sistema. Micro y Altavoz son los dispositivos elegidos
// (Dispositivos); vacíos, los de por omisión.
type Sistema struct {
	Micro, Altavoz string
}

var _ llamada.Dispositivo = Sistema{}

// Abrir abre el micrófono y el altavoz por omisión en un solo dispositivo dúplex, a 48
// kHz mono en tramos de 20 ms. Un micrófono sin permiso hace fallar esto, y la llamada
// sigue sin audio (ver llamada.Entrar).
//
// Sin `capturado` (el reproductor de vídeo de Cast) se abre SOLO el altavoz, como
// reproducción de medios: el dúplex encendía el micrófono —el punto verde de Android y el
// naranja de macOS— para ver un vídeo, y sonaba por el canal de llamada y no por el del
// volumen multimedia (visto en el emulador, 2026-09-28).
func (s Sistema) Abrir(capturado, reproducir func([]int16)) (func(), error) {
	if capturado == nil {
		return abrirMiniaudio(nil, reproducir, s, malgo.Playback)
	}
	if s.Micro != "" || s.Altavoz != "" {
		// ponytail: con un dispositivo elegido va miniaudio, que sabe abrir cualquiera; la
		// unidad de voz de macOS y el DSP de Windows son los de por omisión. Se pierde la
		// cancelación de eco del sistema, que con unos auriculares elegidos no hace falta.
		return abrirMiniaudio(capturado, reproducir, s, malgo.Duplex)
	}
	return abrirSistema(capturado, reproducir)
}

// Dispositivo es un micrófono o un altavoz que se puede elegir.
type Dispositivo struct {
	ID, Nombre string
}

// Dispositivos son los micrófonos y los altavoces que ve miniaudio.
func Dispositivos() (micros, altavoces []Dispositivo) {
	ctx, err := malgo.InitContext(nil, malgo.ContextConfig{}, nil)
	if err != nil {
		return nil, nil
	}
	defer func() { _ = ctx.Uninit(); ctx.Free() }()
	lista := func(t malgo.DeviceType) []Dispositivo {
		ds, err := ctx.Devices(t)
		if err != nil {
			return nil
		}
		out := make([]Dispositivo, 0, len(ds))
		for _, d := range ds {
			out = append(out, Dispositivo{ID: hex.EncodeToString(d.ID[:]), Nombre: d.Name()})
		}
		return out
	}
	return lista(malgo.Capture), lista(malgo.Playback)
}

// idDe es el DeviceID de miniaudio de un ID de Dispositivos; nil, el de por omisión.
func idDe(s string) unsafe.Pointer {
	b, err := hex.DecodeString(s)
	var id malgo.DeviceID
	if err != nil || s == "" || len(b) != len(id) {
		return nil
	}
	copy(id[:], b)
	return id.Pointer()
}

// abrirMiniaudio es la tarjeta de miniaudio, la de todos los sistemas; macOS pone antes
// la de procesado de voz (altavoz_darwin.go), que quita el eco.
func abrirMiniaudio(capturado, reproducir func([]int16), s Sistema, tipo malgo.DeviceType) (func(), error) {
	cfgCtx, cfg := configMiniaudio(s, tipo)
	ctx, err := malgo.InitContext(nil, cfgCtx, nil)
	if err != nil {
		return nil, err
	}

	var mu sync.Mutex
	entrada, salida := make([]int16, 0, llamada.Tramo), make([]int16, llamada.Tramo)
	dev, err := malgo.InitDevice(ctx.Context, cfg, malgo.DeviceCallbacks{
		Data: func(out, in []byte, marcos uint32) {
			mu.Lock()
			defer mu.Unlock()
			entrada = aMuestras(entrada[:0], in)
			if len(entrada) > 0 && capturado != nil {
				capturado(entrada)
			}
			if n := int(marcos); cap(salida) < n {
				salida = make([]int16, n)
			}
			s := salida[:marcos]
			reproducir(s)
			for i, v := range s {
				binary.LittleEndian.PutUint16(out[2*i:], uint16(v)) //nolint:gosec // G115: el bit de signo viaja tal cual
			}
		},
	})
	if err != nil {
		_ = ctx.Uninit()
		ctx.Free()
		return nil, err
	}
	if err := dev.Start(); err != nil {
		dev.Uninit()
		_ = ctx.Uninit()
		ctx.Free()
		return nil, err
	}
	return func() {
		dev.Uninit()
		_ = ctx.Uninit()
		ctx.Free()
	}, nil
}

// configMiniaudio es la configuración del contexto y del dispositivo: la de una llamada
// (voz, grabar y reproducir) o, solo con altavoz, la de reproducir un vídeo.
func configMiniaudio(s Sistema, tipo malgo.DeviceType) (malgo.ContextConfig, malgo.DeviceConfig) {
	sesion := malgo.IOSSessionCategoryPlayAndRecord
	cfg := malgo.DefaultDeviceConfig(tipo)
	cfg.Capture.DeviceID, cfg.Playback.DeviceID = idDe(s.Micro), idDe(s.Altavoz)
	cfg.SampleRate = llamada.Muestreo
	cfg.PeriodSizeInFrames = llamada.Tramo
	cfg.Capture.Format, cfg.Capture.Channels = malgo.FormatS16, 1
	cfg.Playback.Format, cfg.Playback.Channels = malgo.FormatS16, 1
	if tipo == malgo.Playback {
		sesion = malgo.IOSSessionCategoryPlayback
		cfg.AAudio.Usage, cfg.AAudio.ContentType = malgo.AAudioUsageMedia, malgo.AAudioContentTypeMovie
	} else {
		cfg.AAudio.InputPreset = malgo.AAudioInputPresetVoiceCommunication
		cfg.AAudio.Usage = malgo.AAudioUsageVoiceCommunication
	}
	return malgo.ContextConfig{CoreAudio: malgo.CoreAudioConfig{SessionCategory: sesion}}, cfg
}

// aMuestras lee PCM de 16 bits little-endian, que es lo que miniaudio entrega con
// FormatS16 en todas las plataformas que sostenemos.
func aMuestras(dst []int16, b []byte) []int16 {
	for i := 0; i+1 < len(b); i += 2 {
		dst = append(dst, int16(binary.LittleEndian.Uint16(b[i:]))) //nolint:gosec // G115: ídem
	}
	return dst
}
