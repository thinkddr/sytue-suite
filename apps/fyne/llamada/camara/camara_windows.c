// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build windows

// La cámara de Windows: Media Foundation, con un IMFSourceReader que convierte a NV12
// (MF_SOURCE_READER_ENABLE_VIDEO_PROCESSING) y un hilo que lee en síncrono y deja el
// último fotograma en I420. Go lo lee a su ritmo, como en los demás sistemas.

#define COBJMACROS
#include <windows.h>
#include <mfapi.h>
#include <mfidl.h>
#include <mfreadwrite.h>
#include <mferror.h>
#include <stdint.h>
#include <stdlib.h>
#include <string.h>
#include "camara_windows.h"

static CRITICAL_SECTION mu;
static INIT_ONCE unaVez = INIT_ONCE_STATIC_INIT;
static uint8_t *ultimo;
static int ultimoCap, ultimoAncho, ultimoAlto;
static long ultimaSec;
static IMFSourceReader *lector;
static HANDLE hilo;
static volatile LONG parar;
static UINT32 anchoMF, altoMF;

static BOOL CALLBACK iniciar(PINIT_ONCE o, PVOID p, PVOID *c) {
	InitializeCriticalSection(&mu);
	return TRUE;
}

static DWORD WINAPI leerSiempre(LPVOID p) {
	CoInitializeEx(NULL, COINIT_MULTITHREADED);
	while (!parar) {
		DWORD flujo, banderas;
		LONGLONG t;
		IMFSample *m = NULL;
		if (FAILED(IMFSourceReader_ReadSample(lector, MF_SOURCE_READER_FIRST_VIDEO_STREAM, 0, &flujo, &banderas, &t, &m))) break;
		if (banderas & MF_SOURCE_READERF_ENDOFSTREAM) break;
		if (m == NULL) continue;
		IMFMediaBuffer *b = NULL;
		if (SUCCEEDED(IMFSample_ConvertToContiguousBuffer(m, &b))) {
			BYTE *d; DWORD n;
			if (SUCCEEDED(IMFMediaBuffer_Lock(b, &d, NULL, &n))) {
				int w = (int)anchoMF & ~1, h = (int)altoMF & ~1, tam = w * h * 3 / 2;
				if ((DWORD)((int)anchoMF * (int)altoMF * 3 / 2) <= n) {
					EnterCriticalSection(&mu);
					if (ultimoCap < tam) { free(ultimo); ultimo = malloc(tam); ultimoCap = ultimo ? tam : 0; }
					if (ultimo) {
						for (int f = 0; f < h; f++) memcpy(ultimo + f * w, d + f * anchoMF, w);
						const uint8_t *uv = d + anchoMF * altoMF;
						uint8_t *u = ultimo + w * h, *v = u + (w / 2) * (h / 2);
						for (int f = 0; f < h / 2; f++)
							for (int x = 0; x < w / 2; x++) {
								u[f * (w / 2) + x] = uv[f * anchoMF + 2 * x];
								v[f * (w / 2) + x] = uv[f * anchoMF + 2 * x + 1];
							}
						ultimoAncho = w; ultimoAlto = h; ultimaSec++;
					}
					LeaveCriticalSection(&mu);
				}
				IMFMediaBuffer_Unlock(b);
			}
			IMFMediaBuffer_Release(b);
		}
		IMFSample_Release(m);
	}
	CoUninitialize();
	return 0;
}

static IMFActivate **enumerar(UINT32 *n) {
	IMFAttributes *a = NULL;
	IMFActivate **ds = NULL;
	*n = 0;
	if (FAILED(MFCreateAttributes(&a, 1))) return NULL;
	IMFAttributes_SetGUID(a, &MF_DEVSOURCE_ATTRIBUTE_SOURCE_TYPE, &MF_DEVSOURCE_ATTRIBUTE_SOURCE_TYPE_VIDCAP_GUID);
	if (FAILED(MFEnumDeviceSources(a, &ds, n))) ds = NULL;
	IMFAttributes_Release(a);
	return ds;
}

int sytueCamAbrir(const wchar_t *id, int ancho, int alto, int fps) {
	InitOnceExecuteOnce(&unaVez, iniciar, NULL, NULL);
	sytueCamCerrar();
	CoInitializeEx(NULL, COINIT_MULTITHREADED);
	if (FAILED(MFStartup(MF_VERSION, MFSTARTUP_NOSOCKET))) return -1;
	UINT32 n;
	IMFActivate **ds = enumerar(&n);
	if (ds == NULL || n == 0) return -1;
	IMFActivate *elegido = ds[0];
	for (UINT32 i = 0; id != NULL && id[0] != 0 && i < n; i++) {
		WCHAR *enlace = NULL; UINT32 largo;
		if (SUCCEEDED(IMFActivate_GetAllocatedString(ds[i], &MF_DEVSOURCE_ATTRIBUTE_SOURCE_TYPE_VIDCAP_SYMBOLIC_LINK, &enlace, &largo))) {
			if (wcscmp(enlace, id) == 0) elegido = ds[i];
			CoTaskMemFree(enlace);
		}
	}
	IMFMediaSource *fuente = NULL;
	HRESULT hr = IMFActivate_ActivateObject(elegido, &IID_IMFMediaSource, (void **)&fuente);
	for (UINT32 i = 0; i < n; i++) IMFActivate_Release(ds[i]);
	CoTaskMemFree(ds);
	// E_ACCESSDENIED: Configuración > Privacidad > Cámara la tiene apagada para las apps.
	if (hr == E_ACCESSDENIED) return -2;
	if (FAILED(hr)) return -1;
	IMFAttributes *a = NULL;
	MFCreateAttributes(&a, 1);
	IMFAttributes_SetUINT32(a, &MF_SOURCE_READER_ENABLE_VIDEO_PROCESSING, TRUE);
	hr = MFCreateSourceReaderFromMediaSource(fuente, a, &lector);
	IMFAttributes_Release(a);
	IMFMediaSource_Release(fuente);
	if (FAILED(hr)) return -1;
	IMFMediaType *t = NULL;
	MFCreateMediaType(&t);
	IMFMediaType_SetGUID(t, &MF_MT_MAJOR_TYPE, &MFMediaType_Video);
	IMFMediaType_SetGUID(t, &MF_MT_SUBTYPE, &MFVideoFormat_NV12);
	// MFSetAttributeSize y MFSetAttributeRatio son en línea de C++: en C es el UINT64
	// con el primero en la mitad alta.
	IMFMediaType_SetUINT64(t, &MF_MT_FRAME_SIZE, ((UINT64)(UINT32)ancho << 32) | (UINT32)alto);
	if (fps > 0) IMFMediaType_SetUINT64(t, &MF_MT_FRAME_RATE, ((UINT64)(UINT32)fps << 32) | 1);
	hr = IMFSourceReader_SetCurrentMediaType(lector, MF_SOURCE_READER_FIRST_VIDEO_STREAM, NULL, t);
	IMFMediaType_Release(t);
	if (FAILED(hr)) {
		// Sin ese tamaño: NV12 al que tenga la cámara.
		MFCreateMediaType(&t);
		IMFMediaType_SetGUID(t, &MF_MT_MAJOR_TYPE, &MFMediaType_Video);
		IMFMediaType_SetGUID(t, &MF_MT_SUBTYPE, &MFVideoFormat_NV12);
		hr = IMFSourceReader_SetCurrentMediaType(lector, MF_SOURCE_READER_FIRST_VIDEO_STREAM, NULL, t);
		IMFMediaType_Release(t);
		if (FAILED(hr)) { IMFSourceReader_Release(lector); lector = NULL; return -1; }
	}
	IMFMediaType *actual = NULL;
	if (SUCCEEDED(IMFSourceReader_GetCurrentMediaType(lector, MF_SOURCE_READER_FIRST_VIDEO_STREAM, &actual))) {
		UINT64 tam = 0;
		IMFMediaType_GetUINT64(actual, &MF_MT_FRAME_SIZE, &tam);
		anchoMF = (UINT32)(tam >> 32);
		altoMF = (UINT32)tam;
		IMFMediaType_Release(actual);
	}
	parar = 0;
	hilo = CreateThread(NULL, 0, leerSiempre, NULL, 0, NULL);
	return hilo ? 0 : -1;
}

int sytueCamLeer(uint8_t *dst, int cap, int *ancho, int *alto, long *secuencia) {
	InitOnceExecuteOnce(&unaVez, iniciar, NULL, NULL);
	int r = 0;
	EnterCriticalSection(&mu);
	*ancho = ultimoAncho; *alto = ultimoAlto;
	int n = ultimoAncho * ultimoAlto * 3 / 2;
	if (ultimo && ultimaSec != *secuencia && n > 0 && n <= cap) {
		memcpy(dst, ultimo, n);
		*secuencia = ultimaSec;
		r = 1;
	}
	LeaveCriticalSection(&mu);
	return r;
}

void sytueCamCerrar(void) {
	InitOnceExecuteOnce(&unaVez, iniciar, NULL, NULL);
	if (hilo) {
		InterlockedExchange(&parar, 1);
		WaitForSingleObject(hilo, 2000);
		CloseHandle(hilo);
		hilo = NULL;
	}
	if (lector) { IMFSourceReader_Release(lector); lector = NULL; }
	EnterCriticalSection(&mu);
	ultimoAncho = ultimoAlto = 0;
	LeaveCriticalSection(&mu);
}

int sytueCamLista(wchar_t *dst, int cap) {
	CoInitializeEx(NULL, COINIT_MULTITHREADED);
	if (FAILED(MFStartup(MF_VERSION, MFSTARTUP_NOSOCKET))) return 0;
	UINT32 n;
	IMFActivate **ds = enumerar(&n);
	int usado = 0;
	dst[0] = 0;
	for (UINT32 i = 0; ds && i < n; i++) {
		WCHAR *enlace = NULL, *nombre = NULL; UINT32 l1, l2;
		IMFActivate_GetAllocatedString(ds[i], &MF_DEVSOURCE_ATTRIBUTE_SOURCE_TYPE_VIDCAP_SYMBOLIC_LINK, &enlace, &l1);
		IMFActivate_GetAllocatedString(ds[i], &MF_DEVSOURCE_ATTRIBUTE_FRIENDLY_NAME, &nombre, &l2);
		if (enlace && nombre && usado + (int)(l1 + l2) + 3 < cap) {
			usado += swprintf(dst + usado, cap - usado, L"%ls\t%ls\n", enlace, nombre);
		}
		CoTaskMemFree(enlace);
		CoTaskMemFree(nombre);
		IMFActivate_Release(ds[i]);
	}
	CoTaskMemFree(ds);
	return usado;
}
