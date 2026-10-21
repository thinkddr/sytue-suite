// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build !js

package altavoz

import (
	"testing"

	"github.com/gen2brain/malgo"
)

// TestVerUnVideoNoAbreElMicrofono: solo con altavoz (el reproductor de Cast) el
// dispositivo es de reproducción, suena por el canal de medios y la sesión de iOS no
// pide grabar; una llamada sigue siendo voz en dúplex. VISTO EN ROJO con la configuración
// de llamada para los dos: el vídeo encendía el punto verde del micrófono en Android.
func TestVerUnVideoNoAbreElMicrofono(t *testing.T) {
	ctx, cfg := configMiniaudio(Sistema{}, malgo.Playback)
	if cfg.DeviceType != malgo.Playback || cfg.AAudio.Usage != malgo.AAudioUsageMedia ||
		ctx.CoreAudio.SessionCategory != malgo.IOSSessionCategoryPlayback {
		t.Errorf("el vídeo abre %v con uso %v y sesión %v", cfg.DeviceType, cfg.AAudio.Usage, ctx.CoreAudio.SessionCategory)
	}
	ctx, cfg = configMiniaudio(Sistema{}, malgo.Duplex)
	if cfg.AAudio.Usage != malgo.AAudioUsageVoiceCommunication || ctx.CoreAudio.SessionCategory != malgo.IOSSessionCategoryPlayAndRecord {
		t.Errorf("la llamada ya no es de voz: uso %v, sesión %v", cfg.AAudio.Usage, ctx.CoreAudio.SessionCategory)
	}
}
