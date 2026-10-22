// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build windows

// gocritic lee este fichero ya pasado por cgo y da avisos de otro código («dupImport»
// de `C` y `unsafe`): se silencia en el fichero, y el resto de linters sí lo miran.
//
//nolint:gocritic // cgo: los avisos no son de este fichero, ver arriba
package altavoz

/*
#cgo LDFLAGS: -lole32 -loleaut32 -lmsdmo -ldmoguids -lwmcodecdspuuid -luuid -lpropsys
#define COBJMACROS
#include <windows.h>
#include <initguid.h>
#include <mediaobj.h>
#include <dmo.h>
#include <propsys.h>
#include <uuids.h>
#include <wmcodecdsp.h>
#include <stdint.h>

extern void sytueVozEntradaWin(int16_t *pcm, int n);

// Las propiedades del DSP de voz (wmcodecdsp.h no las trae en MinGW).
DEFINE_PROPERTYKEY(SYTUE_AEC_SYSTEM_MODE, 0x6f52c567, 0x0360, 0x4bd2, 0x96, 0x17, 0xcc, 0xbf, 0x14, 0x21, 0xc9, 0x39, 2);
DEFINE_PROPERTYKEY(SYTUE_AEC_SOURCE_MODE, 0x6f52c567, 0x0360, 0x4bd2, 0x96, 0x17, 0xcc, 0xbf, 0x14, 0x21, 0xc9, 0x39, 3);

// Un IMediaBuffer mínimo sobre un búfer fijo: el DSP escribe ahí su salida.
typedef struct {
	IMediaBufferVtbl *lpVtbl;
	BYTE datos[16000];
	DWORD largo;
} Buf;

static HRESULT STDMETHODCALLTYPE bQI(IMediaBuffer *b, REFIID r, void **o) {
	if (IsEqualIID(r, &IID_IUnknown) || IsEqualIID(r, &IID_IMediaBuffer)) { *o = b; return S_OK; }
	*o = NULL;
	return E_NOINTERFACE;
}
static ULONG STDMETHODCALLTYPE bAdd(IMediaBuffer *b) { return 1; }
static ULONG STDMETHODCALLTYPE bRel(IMediaBuffer *b) { return 1; }
static HRESULT STDMETHODCALLTYPE bSet(IMediaBuffer *b, DWORD n) { ((Buf *)b)->largo = n; return S_OK; }
static HRESULT STDMETHODCALLTYPE bMax(IMediaBuffer *b, DWORD *n) { *n = sizeof(((Buf *)b)->datos); return S_OK; }
static HRESULT STDMETHODCALLTYPE bGet(IMediaBuffer *b, BYTE **p, DWORD *n) {
	if (p) *p = ((Buf *)b)->datos;
	if (n) *n = ((Buf *)b)->largo;
	return S_OK;
}
static IMediaBufferVtbl bVtbl = {bQI, bAdd, bRel, bSet, bMax, bGet};

static IMediaObject *dmo;
static HANDLE hilo;
static volatile LONG parar;

// leerVoz saca del DSP, cada 10 ms, el micrófono ya sin eco a 16 kHz y lo sube a 48 kHz
// (el DSP no da más de 22,05 kHz; se interpola lineal ×3, que para voz basta).
static DWORD WINAPI leerVoz(LPVOID p) {
	CoInitializeEx(NULL, COINIT_MULTITHREADED);
	Buf buf = {&bVtbl, {0}, 0};
	int16_t sube[16000 * 3];
	int16_t previa = 0;
	while (!parar) {
		DMO_OUTPUT_DATA_BUFFER o = {0};
		o.pBuffer = (IMediaBuffer *)&buf;
		DWORD estado;
		do {
			buf.largo = 0;
			o.dwStatus = 0;
			if (FAILED(IMediaObject_ProcessOutput(dmo, 0, 1, &o, &estado))) break;
			int n = (int)(buf.largo / 2);
			int16_t *m = (int16_t *)buf.datos;
			for (int i = 0; i < n; i++) {
				int a = previa, b = m[i];
				sube[3 * i] = (int16_t)((2 * a + b) / 3);
				sube[3 * i + 1] = (int16_t)((a + 2 * b) / 3);
				sube[3 * i + 2] = (int16_t)b;
				previa = m[i];
			}
			if (n > 0) sytueVozEntradaWin(sube, 3 * n);
		} while (o.dwStatus & DMO_OUTPUT_DATA_BUFFERF_INCOMPLETE);
		Sleep(10);
	}
	CoUninitialize();
	return 0;
}

// sytueVozAbrirWin arranca el DSP de captura de voz de Windows (el de Skype y Teams) en
// modo fuente: él abre el micro y el altavoz por omisión y resta del micro lo que suena.
static HRESULT sytueVozAbrirWin(void) {
	CoInitializeEx(NULL, COINIT_MULTITHREADED);
	HRESULT hr = CoCreateInstance(&CLSID_CWMAudioAEC, NULL, CLSCTX_INPROC_SERVER, &IID_IMediaObject, (void **)&dmo);
	if (FAILED(hr)) return hr;
	IPropertyStore *ps = NULL;
	if (SUCCEEDED(IMediaObject_QueryInterface(dmo, &IID_IPropertyStore, (void **)&ps))) {
		PROPVARIANT v;
		PropVariantInit(&v);
		v.vt = VT_I4;
		v.lVal = 0; // SINGLE_CHANNEL_AEC
		IPropertyStore_SetValue(ps, &SYTUE_AEC_SYSTEM_MODE, &v);
		v.vt = VT_BOOL;
		v.boolVal = VARIANT_TRUE;
		IPropertyStore_SetValue(ps, &SYTUE_AEC_SOURCE_MODE, &v);
		IPropertyStore_Release(ps);
	}
	DMO_MEDIA_TYPE t;
	if (FAILED(hr = MoInitMediaType(&t, sizeof(WAVEFORMATEX)))) goto mal;
	t.majortype = MEDIATYPE_Audio;
	t.subtype = MEDIASUBTYPE_PCM;
	t.formattype = FORMAT_WaveFormatEx;
	t.bFixedSizeSamples = TRUE;
	t.lSampleSize = 2;
	WAVEFORMATEX *f = (WAVEFORMATEX *)t.pbFormat;
	f->wFormatTag = WAVE_FORMAT_PCM;
	f->nChannels = 1;
	f->nSamplesPerSec = 16000;
	f->nAvgBytesPerSec = 32000;
	f->nBlockAlign = 2;
	f->wBitsPerSample = 16;
	f->cbSize = 0;
	hr = IMediaObject_SetOutputType(dmo, 0, &t, 0);
	MoFreeMediaType(&t);
	if (FAILED(hr)) goto mal;
	if (FAILED(hr = IMediaObject_AllocateStreamingResources(dmo))) goto mal;
	parar = 0;
	hilo = CreateThread(NULL, 0, leerVoz, NULL, 0, NULL);
	if (hilo == NULL) { hr = E_FAIL; goto mal; }
	return S_OK;
mal:
	IMediaObject_Release(dmo);
	dmo = NULL;
	return hr;
}

static void sytueVozCerrarWin(void) {
	if (hilo) {
		InterlockedExchange(&parar, 1);
		WaitForSingleObject(hilo, 2000);
		CloseHandle(hilo);
		hilo = NULL;
	}
	if (dmo) {
		IMediaObject_FreeStreamingResources(dmo);
		IMediaObject_Release(dmo);
		dmo = NULL;
	}
}
*/
import "C"

import (
	"sync"
	"unsafe"

	"github.com/gen2brain/malgo"
)

// La tarjeta de Windows: el micrófono por el DSP de captura de voz (CLSID_CWMAudioAEC),
// que quita el eco de lo que suena por el altavoz por omisión, y el altavoz por
// miniaudio. Si el DSP no arranca, miniaudio en los dos sentidos, sin eco.
//
// ponytail: SIN PROBAR en un Windows de verdad; compila y enlaza con MinGW en el frontal.
// Lo que falta verlo es que el DSP arranque en un equipo con su micro, y medir el eco
// como hace eco_darwin_test.go en macOS.

var (
	winMu     sync.Mutex
	winTrozos *trozos
)

//export sytueVozEntradaWin
func sytueVozEntradaWin(pcm *C.int16_t, n C.int) {
	winMu.Lock()
	t := winTrozos
	winMu.Unlock()
	if t != nil && n > 0 {
		t.capturar(unsafe.Slice((*int16)(unsafe.Pointer(pcm)), int(n)))
	}
}

func abrirSistema(capturado, reproducir func([]int16)) (func(), error) {
	winMu.Lock()
	winTrozos = &trozos{capturado: capturado, reproducir: reproducir}
	winMu.Unlock()
	if C.sytueVozAbrirWin() != 0 {
		winMu.Lock()
		winTrozos = nil
		winMu.Unlock()
		return abrirMiniaudio(capturado, reproducir, Sistema{}, malgo.Duplex)
	}
	cerrarAltavoz, err := abrirMiniaudio(nil, reproducir, Sistema{}, malgo.Playback)
	if err != nil {
		C.sytueVozCerrarWin()
		return nil, err
	}
	return func() {
		cerrarAltavoz()
		C.sytueVozCerrarWin()
		winMu.Lock()
		winTrozos = nil
		winMu.Unlock()
	}, nil
}
