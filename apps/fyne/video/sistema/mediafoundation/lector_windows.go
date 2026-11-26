// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build windows

package mediafoundation

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	"git.sytue.eu/sytue/one/apps/nucleo/sistema/com"
)

// Las funciones de Media Foundation, de System32. En Windows N sin el paquete
// multimedia no están: Disponible da false y no se registra nada.
var (
	mfplat      = windows.NewLazySystemDLL("mfplat.dll")
	mfreadwrite = windows.NewLazySystemDLL("mfreadwrite.dll")
	ole32       = windows.NewLazySystemDLL("ole32.dll")

	fMFStartup            = com.NuevaFuncion(mfplat, "MFStartup")
	fMFShutdown           = com.NuevaFuncion(mfplat, "MFShutdown")
	fMFCreateAttributes   = com.NuevaFuncion(mfplat, "MFCreateAttributes")
	fMFCreateMediaType    = com.NuevaFuncion(mfplat, "MFCreateMediaType")
	fMFCreateSourceReader = com.NuevaFuncion(mfreadwrite, "MFCreateSourceReaderFromURL")
	fPropVariantClear     = com.NuevaFuncion(ole32, "PropVariantClear")
)

// MF_VERSION (MF_SDK_VERSION 2 << 16 | MF_API_VERSION 0x70) y MFSTARTUP_LITE: sin
// sockets (Sytue Office no abre conexiones que la persona no haya pedido).
const (
	mfVersion     = 0x00020070
	mfStartupLite = 0x1
)

// Índices especiales de flujo del lector (mfreadwrite.h).
const (
	flujoTodos       = 0xfffffffe // MF_SOURCE_READER_ALL_STREAMS
	flujoPrimerAudio = 0xfffffffd // MF_SOURCE_READER_FIRST_AUDIO_STREAM
	flujoPrimerVideo = 0xfffffffc // MF_SOURCE_READER_FIRST_VIDEO_STREAM
	fuenteDeMedios   = 0xffffffff // MF_SOURCE_READER_MEDIASOURCE
)

// Banderas de ReadSample (MF_SOURCE_READERF_*).
const (
	lecturaError        = 0x1
	lecturaFin          = 0x2
	lecturaTipoCambiado = 0x20
)

// MFMEDIASOURCE_CAN_SEEK y los tipos de PROPVARIANT que se usan.
const (
	puedeBuscar = 0x2
	vtUI4       = 19
	vtI8        = 20
	vtUI8       = 21
)

// Métodos de las interfaces, por su orden en las cabeceras (mfreadwrite.h,
// mfobjects.h). 0, 1 y 2 son los de IUnknown.
const (
	// IMFSourceReader.
	srSetStreamSelection       = 4
	srGetCurrentMediaType      = 6
	srSetCurrentMediaType      = 7
	srSetCurrentPosition       = 8
	srReadSample               = 9
	srGetPresentationAttribute = 12
	// IMFAttributes (y IMFMediaType e IMFSample, que la heredan).
	atGetUINT32 = 7
	atGetUINT64 = 8
	atGetGUID   = 10
	atGetBlob   = 15
	atSetUINT32 = 21
	atSetGUID   = 24
	// IMFSample.
	smGetSampleDuration         = 37
	smConvertToContiguousBuffer = 41
	// IMFMediaBuffer.
	mbLock   = 3
	mbUnlock = 4
)

// guid escribe un GUID como en las cabeceras.
func guid(d1 uint32, d2, d3 uint16, d4 ...byte) windows.GUID {
	g := windows.GUID{Data1: d1, Data2: d2, Data3: d3}
	copy(g.Data4[:], d4)
	return g
}

// formatoBase es un subtipo de las cabeceras: el código del formato (un FOURCC o un
// WAVE_FORMAT) y el resto fijo.
func formatoBase(codigo uint32) windows.GUID {
	return guid(codigo, 0x0000, 0x0010, 0x80, 0x00, 0x00, 0xaa, 0x00, 0x38, 0x9b, 0x71)
}

// Los GUID que se usan (mfapi.h, mfidl.h, mfreadwrite.h).
var (
	guidNulo = windows.GUID{} // GUID_NULL: posiciones en unidades de 100 ns

	mtMajorType       = guid(0x48eba18e, 0xf8c9, 0x4687, 0xbf, 0x11, 0x0a, 0x74, 0xc9, 0xf9, 0x6a, 0x8f)
	mtSubtype         = guid(0xf7e34c9a, 0x42e8, 0x4714, 0xb7, 0x4b, 0xcb, 0x29, 0xd7, 0x2c, 0x35, 0xe5)
	mtFrameSize       = guid(0x1652c33d, 0xd6b2, 0x4012, 0xb8, 0x34, 0x72, 0x03, 0x08, 0x49, 0xa3, 0x7d)
	mtDefaultStride   = guid(0x644b4e48, 0x1e02, 0x4516, 0xb0, 0xeb, 0xc0, 0x1c, 0xa9, 0xd4, 0x9a, 0xc6)
	mtMinimumAperture = guid(0xd7388766, 0x18fe, 0x48c6, 0xa1, 0x77, 0xee, 0x89, 0x48, 0x67, 0xc8, 0xc4)
	mtAudioCanales    = guid(0x37e48bf5, 0x645e, 0x4c5b, 0x89, 0xde, 0xad, 0xa9, 0xe2, 0x9b, 0x69, 0x6a)
	mtAudioMuestreo   = guid(0x5faeeae7, 0x0290, 0x4c31, 0x9e, 0x8a, 0xc5, 0x34, 0xf6, 0x8d, 0x9d, 0xba)
	mtAudioBytesSeg   = guid(0x1aab75c8, 0xcfef, 0x451c, 0xab, 0x95, 0xac, 0x03, 0x4b, 0x8e, 0x17, 0x31)
	mtAudioBloque     = guid(0x322de230, 0x9eeb, 0x43bd, 0xab, 0x7a, 0xff, 0x41, 0x22, 0x51, 0x54, 0x1d)
	mtAudioBits       = guid(0xf2deb57f, 0x40fa, 0x4764, 0xaa, 0x33, 0xed, 0x4f, 0x2d, 0x1f, 0xf6, 0x69)

	tipoVideo = formatoBase(0x73646976) // MFMediaType_Video ('vids')
	tipoAudio = formatoBase(0x73647561) // MFMediaType_Audio ('auds')

	videoI420 = formatoBase(0x30323449) // MFVideoFormat_I420 ('I420')
	videoIYUV = formatoBase(0x56555949) // MFVideoFormat_IYUV ('IYUV'), el mismo reparto
	videoNV12 = formatoBase(0x3231564e) // MFVideoFormat_NV12 ('NV12')
	audioPCM  = formatoBase(0x00000001) // MFAudioFormat_PCM (WAVE_FORMAT_PCM)
	audioReal = formatoBase(0x00000003) // MFAudioFormat_Float (WAVE_FORMAT_IEEE_FLOAT)

	pdDuracion = guid(0x6c990d33, 0xbb8e, 0x477a, 0x85, 0x98, 0x0d, 0x5d, 0x96, 0xfc, 0xd8, 0x8a) // MF_PD_DURATION
	srCaracter = guid(0x6d23f5c8, 0xc5d7, 0x4a9b, 0x99, 0x71, 0x5d, 0x11, 0xf8, 0xbc, 0xa8, 0x80) // MF_SOURCE_READER_MEDIASOURCE_CHARACTERISTICS
	srAvanzado = guid(0x0f81da2c, 0xb537, 0x4672, 0xa8, 0xb2, 0xa6, 0x81, 0xb1, 0x73, 0x07, 0xa3) // MF_SOURCE_READER_ENABLE_ADVANCED_VIDEO_PROCESSING
)

// propVariant es un PROPVARIANT (24 bytes en 64 bits; en 32 sobra, que no estorba):
// aquí solo se usan sus valores enteros.
type propVariant struct {
	vt    uint16
	_     [3]uint16
	valor int64
	_     uintptr
}

// mfVideoArea es MFVideoArea: dos MFOffset {fract uint16; value int16} y un SIZE.
type mfVideoArea struct {
	fraccionX uint16
	x         int16
	fraccionY uint16
	y         int16
	ancho     int32
	alto      int32
}

var (
	disponibleUna sync.Once
	disponibleSi  bool
)

// Disponible dice si este Windows tiene Media Foundation (mfplat y mfreadwrite, con
// sus funciones). No arranca nada.
func Disponible() bool {
	disponibleUna.Do(func() {
		disponibleSi = fMFStartup.Esta() && fMFShutdown.Esta() && fMFCreateAttributes.Esta() &&
			fMFCreateMediaType.Esta() && fMFCreateSourceReader.Esta() && fPropVariantClear.Esta()
	})
	return disponibleSi
}

// Lector es un fichero abierto con el lector de origen de Media Foundation. Todo lo
// COM va por su hilo propio (MTA). Se puede usar desde cualquier rutina: las llamadas
// se ordenan con su cerrojo.
type Lector struct {
	mu   sync.Mutex
	hilo *com.Hilo
	sr   *com.Objeto // IMFSourceReader
	info Info

	iniciado     bool // MFStartup hecho: al cerrar, MFShutdown
	video, audio bool
	medida       medida
	sonido       formatoSonido
	rem          remuestreo
	buscable     bool

	pendV, pendA *Muestra
	finV, finA   bool
	objetivo     time.Duration
	cerrado      bool
}

// Abrir abre un fichero del disco. Un error es que Media Foundation no está, no abre el
// fichero o no sabe descodificar ni su imagen ni su sonido.
func Abrir(ruta string) (*Lector, error) {
	if !Disponible() {
		return nil, ErrNoDisponible
	}
	abs, err := filepath.Abs(ruta)
	if err != nil {
		return nil, err
	}
	// Solo ficheros de verdad: nada de direcciones, que MFCreateSourceReaderFromURL
	// aceptaría y con ellas saldría a la red.
	fi, err := os.Stat(abs)
	if err != nil {
		return nil, err
	}
	if !fi.Mode().IsRegular() {
		return nil, fmt.Errorf("mediafoundation: %s no es un fichero", abs)
	}
	nombre, err := windows.UTF16PtrFromString(abs)
	if err != nil {
		return nil, err
	}
	hilo, err := com.NuevoHilo(com.Multihilo)
	if err != nil {
		return nil, err
	}
	l := &Lector{hilo: hilo}
	if err := hilo.Hacer(func() error { return l.abrir(nombre) }); err != nil {
		_ = hilo.Hacer(func() error { l.soltar(); return nil })
		hilo.Cerrar()
		return nil, err
	}
	return l, nil
}

// abrir, en el hilo: arranca Media Foundation, crea el lector, elige los flujos y lee
// la primera muestra de cada uno. Un sonido que no da ninguna se quita (para que el
// reloj del Reproductor no espere lo que no llega) y el vídeo sigue mudo; una imagen
// que no da ningún fotograma es un error, para que la app caiga a lo de siempre en vez
// de poner solo el sonido.
func (l *Lector) abrir(ruta *uint16) error {
	r, err := fMFStartup.Llamar(mfVersion, mfStartupLite)
	if err == nil {
		err = com.HR(r, "MFStartup")
	}
	if err != nil {
		return err
	}
	l.iniciado = true
	if l.sr, err = crearLector(ruta); err != nil {
		return err
	}
	_ = l.sr.Llamar(srSetStreamSelection, flujoTodos, 0)
	_ = l.sr.Llamar(srSetStreamSelection, flujoPrimerVideo, 1) // sin vídeo, falla y da igual
	_ = l.sr.Llamar(srSetStreamSelection, flujoPrimerAudio, 1)
	if l.video = l.ponerVideo(); !l.video {
		_ = l.sr.Llamar(srSetStreamSelection, flujoPrimerVideo, 0)
	}
	if l.audio = l.ponerAudio(); !l.audio {
		_ = l.sr.Llamar(srSetStreamSelection, flujoPrimerAudio, 0)
	}
	l.leerPresentacion()
	l.finV, l.finA = !l.video, !l.audio
	if l.video {
		if l.pendV, l.finV = l.leerMuestra(true); l.pendV == nil {
			return errors.New("mediafoundation: el vídeo no dio ningún fotograma")
		}
	}
	if l.audio {
		if l.pendA, l.finA = l.leerMuestra(false); l.pendA == nil {
			l.audio, l.finA = false, true
			_ = l.sr.Llamar(srSetStreamSelection, flujoPrimerAudio, 0)
		}
	}
	if !l.video && !l.audio {
		return errors.New("mediafoundation: no hay en este equipo con qué descodificar este fichero")
	}
	ancho, alto := 0, 0
	if l.video {
		ancho, alto = l.medida.visible()
	}
	l.info = Info{Duracion: l.info.Duracion, Ancho: ancho, Alto: alto, ConVideo: l.video, ConAudio: l.audio}
	return nil
}

// crearLector es MFCreateSourceReaderFromURL con el procesado de vídeo avanzado (que
// convierte a I420 o NV12 lo que el descodificador dé en otro formato); si no lo
// admite (Windows 7), sin él.
func crearLector(ruta *uint16) (*com.Objeto, error) {
	var at *com.Objeto
	if r, err := fMFCreateAttributes.Llamar(uintptr(unsafe.Pointer(&at)), 1); err != nil || com.HR(r, "") != nil {
		at = nil
	}
	defer at.Soltar()
	if at != nil {
		_ = at.Llamar(atSetUINT32, uintptr(unsafe.Pointer(&srAvanzado)), 1)
	}
	var sr *com.Objeto
	r, err := fMFCreateSourceReader.Llamar(uintptr(unsafe.Pointer(ruta)), uintptr(unsafe.Pointer(at)), uintptr(unsafe.Pointer(&sr)))
	if err != nil {
		return nil, err
	}
	err = com.HR(r, "MFCreateSourceReaderFromURL")
	if err != nil && at != nil {
		sr = nil
		r, _ = fMFCreateSourceReader.Llamar(uintptr(unsafe.Pointer(ruta)), 0, uintptr(unsafe.Pointer(&sr)))
		err = com.HR(r, "MFCreateSourceReaderFromURL")
	}
	if err != nil {
		return nil, err
	}
	if sr == nil {
		return nil, errors.New("mediafoundation: el sistema no dio lector")
	}
	return sr, nil
}

// nuevoTipo crea un IMFMediaType con su tipo mayor y su subtipo.
func nuevoTipo(mayor, sub *windows.GUID) (*com.Objeto, error) {
	var t *com.Objeto
	r, err := fMFCreateMediaType.Llamar(uintptr(unsafe.Pointer(&t)))
	if err == nil {
		err = com.HR(r, "MFCreateMediaType")
	}
	if err == nil && t == nil {
		err = errors.New("mediafoundation: el sistema no dio tipo")
	}
	if err != nil {
		return nil, err
	}
	if err = ponerGUID(t, &mtMajorType, mayor); err == nil {
		err = ponerGUID(t, &mtSubtype, sub)
	}
	if err != nil {
		t.Soltar()
		return nil, err
	}
	return t, nil
}

func ponerGUID(o *com.Objeto, clave, valor *windows.GUID) error {
	return com.HR(o.Llamar(atSetGUID, uintptr(unsafe.Pointer(clave)), uintptr(unsafe.Pointer(valor))), "SetGUID")
}

func ponerUINT32(o *com.Objeto, clave *windows.GUID, valor uint32) error {
	return com.HR(o.Llamar(atSetUINT32, uintptr(unsafe.Pointer(clave)), uintptr(valor)), "SetUINT32")
}

func leerUINT32(o *com.Objeto, clave *windows.GUID) (uint32, bool) {
	var v uint32
	return v, com.HR(o.Llamar(atGetUINT32, uintptr(unsafe.Pointer(clave)), uintptr(unsafe.Pointer(&v))), "") == nil
}

func leerUINT64(o *com.Objeto, clave *windows.GUID) (uint64, bool) {
	var v uint64
	return v, com.HR(o.Llamar(atGetUINT64, uintptr(unsafe.Pointer(clave)), uintptr(unsafe.Pointer(&v))), "") == nil
}

// leerGUID da el GUID de la clave, o el nulo si no está.
func leerGUID(o *com.Objeto, clave *windows.GUID) windows.GUID {
	var v windows.GUID
	if com.HR(o.Llamar(atGetGUID, uintptr(unsafe.Pointer(clave)), uintptr(unsafe.Pointer(&v))), "") != nil {
		return windows.GUID{}
	}
	return v
}

// tipoActual es GetCurrentMediaType del flujo; quien lo pide lo suelta.
func (l *Lector) tipoActual(flujo uint32) (*com.Objeto, error) {
	var t *com.Objeto
	if err := com.HR(l.sr.Llamar(srGetCurrentMediaType, uintptr(flujo), uintptr(unsafe.Pointer(&t))), "GetCurrentMediaType"); err != nil {
		return nil, err
	}
	if t == nil {
		return nil, errors.New("mediafoundation: sin tipo")
	}
	return t, nil
}

// ponerTipo es SetCurrentMediaType con un tipo nuevo que prepara `rellenar`.
func (l *Lector) ponerTipo(flujo uint32, mayor, sub *windows.GUID, rellenar func(*com.Objeto) error) error {
	t, err := nuevoTipo(mayor, sub)
	if err != nil {
		return err
	}
	defer t.Soltar()
	if rellenar != nil {
		if err := rellenar(t); err != nil {
			return err
		}
	}
	return com.HR(l.sr.Llamar(srSetCurrentMediaType, uintptr(flujo), 0, uintptr(unsafe.Pointer(t))), "SetCurrentMediaType")
}

// ponerVideo pide la imagen en I420 (o IYUV, igual) y, si no, en NV12.
func (l *Lector) ponerVideo() bool {
	for _, sub := range []*windows.GUID{&videoI420, &videoIYUV, &videoNV12} {
		if l.ponerTipo(flujoPrimerVideo, &tipoVideo, sub, nil) == nil && l.leerTipoVideo() == nil {
			return true
		}
	}
	return false
}

// leerTipoVideo lee del tipo de salida el formato, el tamaño, el paso y la apertura.
func (l *Lector) leerTipoVideo() error {
	t, err := l.tipoActual(flujoPrimerVideo)
	if err != nil {
		return err
	}
	defer t.Soltar()
	var m medida
	switch leerGUID(t, &mtSubtype) {
	case videoI420, videoIYUV:
		m.formato = formatoI420
	case videoNV12:
		m.formato = formatoNV12
	default:
		return errors.New("mediafoundation: la imagen no viene en I420 ni NV12")
	}
	if tam, ok := leerUINT64(t, &mtFrameSize); ok {
		m.anchoCod, m.altoCod = int(tam>>32), int(uint32(tam)) //nolint:gosec // G115: MF_MT_FRAME_SIZE son dos UINT32
	}
	if paso, ok := leerUINT32(t, &mtDefaultStride); ok {
		m.paso = int(int32(paso)) //nolint:gosec // G115: el paso es un LONG guardado como UINT32 (negativo: de abajo arriba)
	}
	var area mfVideoArea
	var n uint32
	if com.HR(t.Llamar(atGetBlob, uintptr(unsafe.Pointer(&mtMinimumAperture)), uintptr(unsafe.Pointer(&area)), unsafe.Sizeof(area), uintptr(unsafe.Pointer(&n))), "") == nil &&
		n == uint32(unsafe.Sizeof(area)) && area.ancho > 0 && area.alto > 0 && area.x >= 0 && area.y >= 0 {
		m.x, m.y, m.ancho, m.alto = int(area.x), int(area.y), int(area.ancho), int(area.alto)
	}
	if a, b := m.visible(); a <= 0 || b <= 0 || a > 16384 || b > 16384 {
		return errors.New("mediafoundation: el vídeo no dice su tamaño")
	}
	l.medida = m
	return nil
}

// ponerAudio pide el sonido en PCM mono de 16 bits a 48 kHz; si el sistema no sabe
// convertir a eso, en PCM de 16 bits como venga y, si no, en coma flotante (lo que
// falte se convierte aquí).
func (l *Lector) ponerAudio() bool {
	intentos := []struct {
		sub      *windows.GUID
		rellenar func(*com.Objeto) error
	}{
		{&audioPCM, func(t *com.Objeto) error {
			return errors.Join(ponerUINT32(t, &mtAudioBits, 16), ponerUINT32(t, &mtAudioCanales, 1),
				ponerUINT32(t, &mtAudioMuestreo, Muestreo), ponerUINT32(t, &mtAudioBloque, 2),
				ponerUINT32(t, &mtAudioBytesSeg, 2*Muestreo))
		}},
		{&audioPCM, func(t *com.Objeto) error { return ponerUINT32(t, &mtAudioBits, 16) }},
		{&audioReal, nil},
	}
	for _, i := range intentos {
		if l.ponerTipo(flujoPrimerAudio, &tipoAudio, i.sub, i.rellenar) == nil && l.leerTipoAudio() == nil {
			return true
		}
	}
	return false
}

// leerTipoAudio lee del tipo de salida el formato del PCM.
func (l *Lector) leerTipoAudio() error {
	t, err := l.tipoActual(flujoPrimerAudio)
	if err != nil {
		return err
	}
	defer t.Soltar()
	var f formatoSonido
	switch leerGUID(t, &mtSubtype) {
	case audioPCM:
	case audioReal:
		f.flotante = true
	default:
		return errors.New("mediafoundation: el sonido no viene en PCM")
	}
	c, _ := leerUINT32(t, &mtAudioCanales)
	m, _ := leerUINT32(t, &mtAudioMuestreo)
	b, _ := leerUINT32(t, &mtAudioBits)
	f.canales, f.muestreo, f.bits = int(c), int(m), int(b)
	if !f.valido() {
		return fmt.Errorf("mediafoundation: PCM que no se sabe leer (%d canales, %d Hz, %d bits)", c, m, b)
	}
	if l.sonido.muestreo != f.muestreo {
		l.rem = remuestreo{de: f.muestreo, a: Muestreo}
	}
	l.sonido = f
	return nil
}

// leerPresentacion lee la duración y si se puede buscar.
func (l *Lector) leerPresentacion() {
	var pv propVariant
	if com.HR(l.sr.Llamar(srGetPresentationAttribute, fuenteDeMedios, uintptr(unsafe.Pointer(&pdDuracion)), uintptr(unsafe.Pointer(&pv))), "") == nil {
		if pv.vt == vtUI8 && pv.valor > 0 {
			l.info.Duracion = deCien(pv.valor)
		}
		_, _ = fPropVariantClear.Llamar(uintptr(unsafe.Pointer(&pv)))
	}
	pv = propVariant{}
	if com.HR(l.sr.Llamar(srGetPresentationAttribute, fuenteDeMedios, uintptr(unsafe.Pointer(&srCaracter)), uintptr(unsafe.Pointer(&pv))), "") == nil {
		l.buscable = pv.vt == vtUI4 && uint32(pv.valor)&puedeBuscar != 0 //nolint:gosec // G115: VT_UI4
		_, _ = fPropVariantClear.Llamar(uintptr(unsafe.Pointer(&pv)))
	}
}

// maxLecturas es cuántas veces se llama a ReadSample como mucho por muestra entregada
// (huecos del flujo y lo que queda antes del punto buscado); maxFallos, cuántas muestras
// seguidas que no se pueden convertir se aguantan antes de dar el flujo por acabado (si
// no, un fotograma con un reparto que no se entiende descodificaría el vídeo entero).
const (
	maxLecturas = 20000
	maxFallos   = 8
)

// leerMuestra lee la siguiente muestra del flujo de vídeo o del de audio. nil con fin
// es que el flujo se acabó (o falló: lo que se vio, se vio).
func (l *Lector) leerMuestra(esVideo bool) (*Muestra, bool) {
	flujo := uint32(flujoPrimerAudio)
	if esVideo {
		flujo = flujoPrimerVideo
	}
	fallos := 0
	for range maxLecturas {
		var indice, banderas uint32
		var t int64
		var s *com.Objeto
		r := l.sr.Llamar(srReadSample, uintptr(flujo), 0, uintptr(unsafe.Pointer(&indice)),
			uintptr(unsafe.Pointer(&banderas)), uintptr(unsafe.Pointer(&t)), uintptr(unsafe.Pointer(&s)))
		if com.HR(r, "") != nil || banderas&lecturaError != 0 {
			s.Soltar()
			return nil, true
		}
		if banderas&lecturaTipoCambiado != 0 {
			var err error
			if esVideo {
				err = l.leerTipoVideo()
			} else {
				err = l.leerTipoAudio()
			}
			if err != nil {
				s.Soltar()
				return nil, true
			}
		}
		if s == nil {
			if banderas&lecturaFin != 0 {
				return nil, true
			}
			continue // un hueco (STREAMTICK): a por la siguiente
		}
		m, err := l.convertir(s, esVideo, t)
		s.Soltar()
		if err == nil && m != nil {
			return m, false
		}
		if err != nil && !errors.Is(err, errAntes) {
			fallos++
			if fallos >= maxFallos {
				return nil, true
			}
		}
		if banderas&lecturaFin != 0 {
			return nil, true
		}
		// Una muestra que no se pudo convertir o que queda antes del punto buscado: la
		// siguiente.
	}
	return nil, true
}

// errAntes: la muestra queda antes del punto buscado y no se entrega.
var errAntes = errors.New("mediafoundation: antes del punto buscado")

// convertir pasa una IMFSample a Muestra (errAntes si queda antes del punto buscado;
// nil sin error si es un trozo de audio que no da ninguna muestra a 48 kHz).
func (l *Lector) convertir(s *com.Objeto, esVideo bool, t int64) (*Muestra, error) {
	var dur int64
	if com.HR(s.Llamar(smGetSampleDuration, uintptr(unsafe.Pointer(&dur))), "") != nil {
		dur = 0
	}
	instante := deCien(t)
	if esVideo {
		instante = instanteImagen(t)
	}
	if pasado(instante, deCien(dur), l.objetivo) {
		return nil, errAntes
	}
	var m *Muestra
	err := bloquear(s, func(datos []byte) error {
		if esVideo {
			pl, err := distribuir(len(datos), l.medida)
			if err != nil {
				return err
			}
			img, err := aI420(datos, pl)
			if err != nil {
				return err
			}
			m = &Muestra{Tiempo: instante, Imagen: img}
			return nil
		}
		if pcm := l.rem.pasar(aMono(datos, l.sonido)); len(pcm) > 0 {
			m = &Muestra{Tiempo: instante, PCM: pcm}
		}
		return nil
	})
	return m, err
}

// bloquear junta los búferes de la muestra en uno, lo bloquea y da sus bytes a usar
// (que no se guardan: se copian dentro de usar).
func bloquear(s *com.Objeto, usar func([]byte) error) error {
	var b *com.Objeto
	if err := com.HR(s.Llamar(smConvertToContiguousBuffer, uintptr(unsafe.Pointer(&b))), "ConvertToContiguousBuffer"); err != nil {
		return err
	}
	if b == nil {
		return errors.New("mediafoundation: muestra sin búfer")
	}
	defer b.Soltar()
	var p *byte
	var n uint32
	if err := com.HR(b.Llamar(mbLock, uintptr(unsafe.Pointer(&p)), 0, uintptr(unsafe.Pointer(&n))), "Lock"); err != nil {
		return err
	}
	defer b.Llamar(mbUnlock)
	if p == nil || n == 0 {
		return errors.New("mediafoundation: búfer vacío")
	}
	return usar(unsafe.Slice(p, n))
}

// soltar, en el hilo: suelta el lector y apaga Media Foundation.
func (l *Lector) soltar() {
	l.pendV, l.pendA = nil, nil
	l.sr.Soltar()
	l.sr = nil
	if l.iniciado {
		_, _ = fMFShutdown.Llamar()
		l.iniciado = false
	}
}

// Info es lo que se supo al abrir.
func (l *Lector) Info() Info { return l.info }

// Siguiente es la muestra siguiente en orden de tiempo (imagen o sonido, la que vaya
// antes); io.EOF al acabar.
func (l *Lector) Siguiente() (Muestra, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.cerrado {
		return Muestra{}, io.EOF
	}
	var m *Muestra
	err := l.hilo.Hacer(func() error {
		if !l.finV && l.pendV == nil {
			l.pendV, l.finV = l.leerMuestra(true)
		}
		if !l.finA && l.pendA == nil {
			l.pendA, l.finA = l.leerMuestra(false)
		}
		switch elegir(pendienteDe(l.pendV), pendienteDe(l.pendA)) {
		case deVideo:
			m, l.pendV = l.pendV, nil
		case deAudio:
			m, l.pendA = l.pendA, nil
		case ninguno: // los dos acabados: io.EOF
		}
		return nil
	})
	if err != nil {
		return Muestra{}, err
	}
	if m == nil {
		return Muestra{}, io.EOF
	}
	return *m, nil
}

func pendienteDe(m *Muestra) pendiente {
	if m == nil {
		return pendiente{}
	}
	return pendiente{hay: true, t: m.Tiempo}
}

// Buscar salta a t: el lector se pone en el fotograma clave anterior y lo que quede
// antes de t no se entrega (ver pasado). Devuelve t.
func (l *Lector) Buscar(t time.Duration) (time.Duration, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.cerrado {
		return 0, errors.New("mediafoundation: el lector está cerrado")
	}
	if !l.buscable {
		return 0, errors.New("mediafoundation: este fichero no deja buscar")
	}
	if d := l.info.Duracion; d > 0 && t > d {
		t = d
	}
	t = max(t, 0)
	err := l.hilo.Hacer(func() error {
		pv := propVariant{vt: vtI8, valor: aCien(t)}
		if err := com.HR(l.sr.Llamar(srSetCurrentPosition, uintptr(unsafe.Pointer(&guidNulo)), uintptr(unsafe.Pointer(&pv))), "SetCurrentPosition"); err != nil {
			return err
		}
		l.pendV, l.pendA = nil, nil
		l.finV, l.finA = !l.video, !l.audio
		l.objetivo = t
		l.rem.reiniciar()
		return nil
	})
	return t, err
}

// Close suelta el lector, apaga Media Foundation y cierra el hilo.
func (l *Lector) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.cerrado {
		return nil
	}
	l.cerrado = true
	err := l.hilo.Hacer(func() error { l.soltar(); return nil })
	l.hilo.Cerrar()
	return err
}
