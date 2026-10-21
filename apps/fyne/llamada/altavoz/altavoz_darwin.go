// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build darwin

// gocritic lee este fichero ya pasado por cgo y da avisos de otro código («dupImport»
// de `C` y `unsafe`): se silencia en el fichero, y el resto de linters sí lo miran.
//
//nolint:gocritic // cgo: los avisos no son de este fichero, ver arriba
package altavoz

/*
#cgo LDFLAGS: -framework AudioToolbox -framework CoreAudio
#include <AudioToolbox/AudioToolbox.h>
#include <stdlib.h>

extern void sytueVozEntrada(int16_t *pcm, int n);
extern void sytueVozSalida(int16_t *pcm, int n);

static AudioUnit sytueAU;
static int16_t *sytueBuf;
static UInt32 sytueBufCap;

static OSStatus sytueEntrada(void *ref, AudioUnitRenderActionFlags *flags, const AudioTimeStamp *ts,
		UInt32 bus, UInt32 n, AudioBufferList *nada) {
	if (n > sytueBufCap) return -1;
	AudioBufferList abl;
	abl.mNumberBuffers = 1;
	abl.mBuffers[0].mNumberChannels = 1;
	abl.mBuffers[0].mDataByteSize = n * 2;
	abl.mBuffers[0].mData = sytueBuf;
	OSStatus r = AudioUnitRender(sytueAU, flags, ts, 1, n, &abl);
	if (r == noErr) sytueVozEntrada(sytueBuf, (int)n);
	return r;
}

static OSStatus sytueSalida(void *ref, AudioUnitRenderActionFlags *flags, const AudioTimeStamp *ts,
		UInt32 bus, UInt32 n, AudioBufferList *io) {
	sytueVozSalida((int16_t *)io->mBuffers[0].mData, (int)(io->mBuffers[0].mDataByteSize / 2));
	return noErr;
}

// sytueVozAbrir arranca la VoiceProcessingIO a 48 kHz mono de 16 bits en los dos
// sentidos: la misma unidad reproduce y graba, y por eso sabe qué restar del micro.
static OSStatus sytueVozAbrir(void) {
	AudioComponentDescription d = {kAudioUnitType_Output, kAudioUnitSubType_VoiceProcessingIO,
		kAudioUnitManufacturer_Apple, 0, 0};
	AudioComponent c = AudioComponentFindNext(NULL, &d);
	if (c == NULL) return -1;
	OSStatus r = AudioComponentInstanceNew(c, &sytueAU);
	if (r != noErr) return r;
	UInt32 uno = 1;
	AudioUnitSetProperty(sytueAU, kAudioOutputUnitProperty_EnableIO, kAudioUnitScope_Input, 1, &uno, sizeof uno);
	AudioUnitSetProperty(sytueAU, kAudioOutputUnitProperty_EnableIO, kAudioUnitScope_Output, 0, &uno, sizeof uno);
	AudioStreamBasicDescription f = {0};
	f.mSampleRate = 48000;
	f.mFormatID = kAudioFormatLinearPCM;
	f.mFormatFlags = kAudioFormatFlagIsSignedInteger | kAudioFormatFlagIsPacked;
	f.mBytesPerPacket = 2;
	f.mFramesPerPacket = 1;
	f.mBytesPerFrame = 2;
	f.mChannelsPerFrame = 1;
	f.mBitsPerChannel = 16;
	if ((r = AudioUnitSetProperty(sytueAU, kAudioUnitProperty_StreamFormat, kAudioUnitScope_Output, 1, &f, sizeof f)) != noErr) goto mal;
	if ((r = AudioUnitSetProperty(sytueAU, kAudioUnitProperty_StreamFormat, kAudioUnitScope_Input, 0, &f, sizeof f)) != noErr) goto mal;
	AURenderCallbackStruct en = {sytueEntrada, NULL}, sa = {sytueSalida, NULL};
	if ((r = AudioUnitSetProperty(sytueAU, kAudioOutputUnitProperty_SetInputCallback, kAudioUnitScope_Global, 1, &en, sizeof en)) != noErr) goto mal;
	if ((r = AudioUnitSetProperty(sytueAU, kAudioUnitProperty_SetRenderCallback, kAudioUnitScope_Input, 0, &sa, sizeof sa)) != noErr) goto mal;
	sytueBufCap = 8192;
	sytueBuf = malloc(sytueBufCap * 2);
	if ((r = AudioUnitInitialize(sytueAU)) != noErr) goto mal;
	if ((r = AudioOutputUnitStart(sytueAU)) != noErr) goto mal;
	return noErr;
mal:
	AudioComponentInstanceDispose(sytueAU);
	sytueAU = NULL;
	return r;
}

static void sytueVozCerrar(void) {
	if (sytueAU == NULL) return;
	AudioOutputUnitStop(sytueAU);
	AudioUnitUninitialize(sytueAU);
	AudioComponentInstanceDispose(sytueAU);
	sytueAU = NULL;
	free(sytueBuf);
	sytueBuf = NULL;
}
*/
import "C"

import (
	"fmt"
	"sync"
	"unsafe"

	"github.com/gen2brain/malgo"
)

// La tarjeta de macOS e iOS: VoiceProcessingIO, la unidad de audio de FaceTime, con su
// cancelación de eco, su supresión de ruido y su control de ganancia. Si no arranca (un
// equipo sin micrófono), se cae a miniaudio sin eco.

var (
	vozMu     sync.Mutex
	vozTrozos *trozos
)

//export sytueVozEntrada
func sytueVozEntrada(pcm *C.int16_t, n C.int) {
	vozMu.Lock()
	t := vozTrozos
	vozMu.Unlock()
	if t != nil && n > 0 {
		t.capturar(unsafe.Slice((*int16)(unsafe.Pointer(pcm)), int(n)))
	}
}

//export sytueVozSalida
func sytueVozSalida(pcm *C.int16_t, n C.int) {
	out := unsafe.Slice((*int16)(unsafe.Pointer(pcm)), int(n))
	vozMu.Lock()
	t := vozTrozos
	vozMu.Unlock()
	if t == nil {
		clear(out)
		return
	}
	t.rellenar(out)
}

func abrirSistema(capturado, reproducir func([]int16)) (func(), error) {
	if cerrar, err := abrirVoz(capturado, reproducir); err == nil {
		return cerrar, nil
	}
	return abrirMiniaudio(capturado, reproducir, Sistema{}, malgo.Duplex)
}

// abrirVoz arranca la VoiceProcessingIO. Una a la vez, como la llamada.
func abrirVoz(capturado, reproducir func([]int16)) (func(), error) {
	vozMu.Lock()
	vozTrozos = &trozos{capturado: capturado, reproducir: reproducir}
	vozMu.Unlock()
	if r := C.sytueVozAbrir(); r != 0 {
		vozMu.Lock()
		vozTrozos = nil
		vozMu.Unlock()
		return nil, fmt.Errorf("altavoz: VoiceProcessingIO no arranca (OSStatus %d)", int32(r))
	}
	return func() {
		C.sytueVozCerrar()
		vozMu.Lock()
		vozTrozos = nil
		vozMu.Unlock()
	}, nil
}
