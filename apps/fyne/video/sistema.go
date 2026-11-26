// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build !js

package video

import (
	"errors"
	"image"
	"io"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"git.sytue.eu/sytue/one/apps/fyne/llamada"
)

// # Descodificadores del sistema (PLAN-EDICION-LOCAL §6.10, CA-08)
//
// El reproductor propio lee WebM (VP9/VP8 + Opus) en Go. Lo demás (MP4, MOV, H.264,
// HEVC, AAC…, y el WebM cuyo vídeo Go no saca: ver más abajo) lo descodifica, si lo hay,
// el sistema: un [Sistema] entrega fotogramas y PCM YA descodificados, con su instante,
// y el MISMO Reproductor los pone a su hora con el reloj del audio (visor.go no
// cambia). Cada sistema se registra con
// [RegistrarSistema]; la edición conectada no registra ninguno, así que para ella nada
// cambia: lo que no es WebM sigue siendo un error.
//
// Hecho: Linux con GStreamer por proceso (`video/sistema`, `gst-launch-1.0` a dos
// tuberías de fotogramas I420 y PCM crudos), Android con `MediaExtractor` + `MediaCodec`
// por el puente JNI (`video/sistema/mediacodec_android.go`) y Windows con el lector de origen
// de Media Foundation (`IMFSourceReader` con `MFVideoFormat_I420` o NV12 y
// `MFAudioFormat_PCM`, en Go puro sin cgo: `video/sistema/mediafoundation`). Android y
// Windows, sin probar aún en un dispositivo. Por hacer, con la misma interfaz: AVFoundation
// (`AVAssetReader` con `kCVPixelFormatType_420YpCbCr8Planar` y PCM `kAudioFormatLinearPCM`
// a 48 kHz) en macOS e iOS.

// Muestra es lo que da un descodificador del sistema: un fotograma (Imagen) o un trozo
// de audio (PCM mono a 48 kHz), con su instante.
type Muestra struct {
	Tiempo time.Duration
	Imagen image.Image
	PCM    []int16
}

// InfoSistema es lo que se sabe del fichero al abrirlo.
type InfoSistema struct {
	Duracion    time.Duration
	Ancho, Alto int
	ConVideo    bool
	ConAudio    bool
}

// Sistema es un descodificador del sistema abierto sobre un fichero. No hace falta que
// sea seguro para varias rutinas: el Reproductor lo usa de una en una.
type Sistema interface {
	Info() InfoSistema
	// Siguiente es la muestra siguiente en orden de tiempo; io.EOF al acabar.
	Siguiente() (Muestra, error)
	// Buscar salta a t (o justo antes) y devuelve dónde quedó.
	Buscar(t time.Duration) (time.Duration, error)
	Close() error
}

// AbrirSistema abre un fichero con un descodificador del sistema; un error es que no
// puede (no está instalado, no sabe el formato).
type AbrirSistema func(ruta string) (Sistema, error)

var (
	muSistemas sync.Mutex
	sistemas   []AbrirSistema
)

// RegistrarSistema añade un descodificador del sistema. Los registrados se prueban en
// orden con lo que no es WebM y con el WebM cuyo vídeo Go no saca.
func RegistrarSistema(abrir AbrirSistema) {
	muSistemas.Lock()
	defer muSistemas.Unlock()
	sistemas = append(sistemas, abrir)
}

// HaySistema dice si hay algún descodificador del sistema registrado.
func HaySistema() bool {
	muSistemas.Lock()
	defer muSistemas.Unlock()
	return len(sistemas) > 0
}

// AbrirConSistema prueba los descodificadores registrados con la ruta.
func AbrirConSistema(ruta string) (Sistema, error) {
	muSistemas.Lock()
	ss := append([]AbrirSistema(nil), sistemas...)
	muSistemas.Unlock()
	err := errors.New("video: no hay descodificador del sistema")
	for _, abrir := range ss {
		s, e := abrir(ruta)
		if e == nil {
			return s, nil
		}
		err = e
	}
	return nil, err
}

// rutaDe es la ruta de src si es un fichero con nombre (un *os.File, o lo que lo envuelva
// y diga su Name); "" si no lo es.
func rutaDe(src io.ReadSeeker) string {
	if f, ok := src.(interface{ Name() string }); ok {
		return f.Name()
	}
	return ""
}

// abrirConSistema abre src con el sistema si es un fichero con nombre.
func abrirConSistema(src io.ReadSeeker) (Sistema, bool) {
	ruta := rutaDe(src)
	if ruta == "" || !HaySistema() {
		return nil, false
	}
	s, err := AbrirConSistema(ruta)
	return s, err == nil
}

// # Cuando Go no saca el vídeo de un WebM
//
// govpx descodifica VP8 y el perfil 0 de VP9 (4:2:0 a 8 bits), que es lo que deja casi
// todo; pero un WebM puede traer VP9 de otro perfil —el 1, 4:4:4, es lo que saca libvpx de
// una fuente RGB como el `mandelbrot` de ffmpeg; el 2, de 10 bits— o AV1, y entonces el
// reproductor de Go lee el fichero sin sacar ni un fotograma. Con un descodificador del
// sistema registrado (Sytue Office) y el WebM en un fichero, antes de quedarse con Go se
// comprueba que saca el primer fotograma, en un plazo (PlazoPrimerFotograma); si no lo
// saca —falla o tarda—, el vídeo va por el sistema, como lo que no es WebM. Sin sistema
// (la edición conectada no registra ninguno) no se comprueba nada y todo es como siempre.

// PlazoPrimerFotograma es lo más que se espera al primer fotograma de un WebM en Go antes
// de pasarlo al descodificador del sistema. Medido (2026-11-25, 4 núcleos): la prueba
// tarda 0,03-0,05 s con un VP9 1080p y 0,11-0,12 s con uno 4K, y un perfil 1 falla en
// menos de 2 ms; es lo que se añade al abrir un WebM donde hay sistema.
var PlazoPrimerFotograma = 4 * time.Second

// maxPaquetesPrueba son los paquetes de vídeo que se prueban, como mucho, buscando el
// primer fotograma: un WebM cortado puede empezar sin fotograma clave.
const maxPaquetesPrueba = 60

// sinFotogramaEnGo dice si el WebM w de src, con vídeo, NO va a dar imagen en Go: su
// códec no es VP8 ni VP9, o no saca el primer fotograma en PlazoPrimerFotograma. Solo se
// mira si src es un fichero y hay un descodificador del sistema al que pasarlo; la
// prueba lee el fichero aparte, así que no mueve src.
func sinFotogramaEnGo(w *WebM, src io.ReadSeeker) bool {
	v := w.Pista(TipoVideo)
	ruta := rutaDe(src)
	if v == nil || ruta == "" || !HaySistema() {
		return false
	}
	if v.Codec != CodecVP9 && v.Codec != CodecVP8 {
		return true
	}
	return !primerFotogramaEnGo(ruta, PlazoPrimerFotograma)
}

// primerFotogramaEnGo dice si govpx saca un fotograma del vídeo del WebM de ruta antes de
// plazo. Si se pasa, la prueba se deja (al acabar el paquete en curso) y cuenta como que no.
func primerFotogramaEnGo(ruta string, plazo time.Duration) bool {
	hecho := make(chan bool, 1)
	var parar atomic.Bool
	go func() { hecho <- probarEnGo(ruta, &parar) }()
	t := time.NewTimer(plazo)
	defer t.Stop()
	select {
	case ok := <-hecho:
		return ok
	case <-t.C:
		parar.Store(true)
		return false
	}
}

func probarEnGo(ruta string, parar *atomic.Bool) bool {
	f, err := os.Open(ruta) //nolint:gosec // G304: el mismo fichero que se va a reproducir
	if err != nil {
		return false
	}
	defer func() { _ = f.Close() }()
	w, err := Abrir(f)
	if err != nil {
		return false
	}
	v := w.Pista(TipoVideo)
	if v == nil {
		return false
	}
	dec, err := nuevoDescodificador(v.Codec)
	if err != nil {
		return false
	}
	defer func() { _ = dec.Close() }()
	for n := 0; n < maxPaquetesPrueba && !parar.Load(); {
		p, err := w.Siguiente()
		if err != nil {
			return false
		}
		if p.Pista != v.Numero {
			continue
		}
		n++
		if dec.Decode(p.Datos) != nil {
			continue
		}
		if _, ok := dec.NextFrame(); ok {
			return true
		}
	}
	return false
}

// Pistas de mentira para lo que trae un descodificador del sistema.
const (
	pistaVideoSistema = 1
	pistaAudioSistema = 2
)

// fuenteSistema pasa las muestras del sistema a paquetes ya descodificados.
type fuenteSistema struct{ s Sistema }

func (f fuenteSistema) Siguiente() (Paquete, error) {
	m, err := f.s.Siguiente()
	if err != nil {
		return Paquete{}, err
	}
	if m.Imagen != nil {
		return Paquete{Pista: pistaVideoSistema, Tiempo: m.Tiempo, Clave: true, imagen: m.Imagen}, nil
	}
	return Paquete{Pista: pistaAudioSistema, Tiempo: m.Tiempo, pcm: m.PCM}, nil
}

func (f fuenteSistema) Buscar(t time.Duration) (time.Duration, error) { return f.s.Buscar(t) }

// NuevoDeSistema crea un Reproductor sobre un descodificador del sistema. El
// Reproductor se queda con él y lo cierra en Cerrar.
func NuevoDeSistema(s Sistema, dispositivo llamada.Dispositivo) (*Reproductor, error) {
	in := s.Info()
	if !in.ConVideo && !in.ConAudio {
		_ = s.Close()
		return nil, errors.New("video: el descodificador del sistema no da ni imagen ni sonido")
	}
	r := &Reproductor{fuente: fuenteSistema{s}, duracion: in.Duracion, sistema: s, dispositivo: dispositivo, pausado: true, volumen: 1, velocidad: 1}
	r.hay = sync.NewCond(&r.mu)
	if in.ConVideo {
		r.video = &Pista{Numero: pistaVideoSistema, Tipo: TipoVideo, Ancho: in.Ancho, Alto: in.Alto}
	}
	if in.ConAudio {
		r.audio = &Pista{Numero: pistaAudioSistema, Tipo: TipoAudio, Muestreo: llamada.Muestreo, Canales: 1}
		r.canalesAudio = 1
	}
	return r, nil
}

// fuentePaquetes es lo que lee el Reproductor: el WebM o un descodificador del sistema.
type fuentePaquetes interface {
	Siguiente() (Paquete, error)
	Buscar(t time.Duration) (time.Duration, error)
}

// UltimoFotograma es el fotograma que se está enseñando (nil si aún no hay) y su
// instante: el de «Usar el fotograma de ahora».
func (r *Reproductor) UltimoFotograma() (image.Image, time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.imagen, r.ultimo
}

// MaxPaquetesFotograma es cuántos paquetes se leen como mucho buscando un fotograma.
const MaxPaquetesFotograma = 5000

// Fotograma descodifica el fotograma del instante en (el último que empieza antes o
// en él) sin reproductor ni tarjeta: el póster de un WebM sin ffmpeg (CA-06). Lo que no
// es WebM, y el WebM cuyo vídeo Go no saca (otro perfil de VP9, AV1), va por los
// descodificadores del sistema registrados, si los hay.
func Fotograma(src io.ReadSeeker, en time.Duration) (image.Image, error) {
	w, err := Abrir(src)
	if err != nil {
		return fotogramaPorSistema(src, en, err)
	}
	img, err := fotogramaEnGo(w, en)
	if err != nil {
		return fotogramaPorSistema(src, en, err)
	}
	return img, nil
}

// fotogramaPorSistema es Fotograma con el descodificador del sistema; sin él (o si no
// abre el fichero), el error de Go, motivo.
func fotogramaPorSistema(src io.ReadSeeker, en time.Duration, motivo error) (image.Image, error) {
	s, ok := abrirConSistema(src)
	if !ok {
		return nil, motivo
	}
	defer s.Close()
	return fotogramaDeSistema(s, en)
}

// fotogramaEnGo es Fotograma con govpx, sobre el WebM ya abierto.
func fotogramaEnGo(w *WebM, en time.Duration) (image.Image, error) {
	v := w.Pista(TipoVideo)
	if v == nil || (v.Codec != CodecVP9 && v.Codec != CodecVP8) {
		return nil, errors.New("video: el fichero no trae VP9 ni VP8")
	}
	dec, err := nuevoDescodificador(v.Codec)
	if err != nil {
		return nil, err
	}
	defer dec.Close()
	if en > 0 {
		if _, err := w.Buscar(en); err != nil {
			return nil, err
		}
	}
	var ultima image.Image
	for range MaxPaquetesFotograma {
		p, err := w.Siguiente()
		if err != nil {
			break
		}
		if p.Pista != v.Numero || dec.Decode(p.Datos) != nil {
			continue
		}
		f, ok := dec.NextFrame()
		if !ok {
			continue
		}
		if ultima != nil && p.Tiempo > en {
			break
		}
		ultima = llamada.ImagenI420(f)
		if p.Tiempo >= en {
			break
		}
	}
	if ultima == nil {
		return nil, errors.New("video: no se pudo descodificar ningún fotograma")
	}
	return ultima, nil
}

func fotogramaDeSistema(s Sistema, en time.Duration) (image.Image, error) {
	if en > 0 {
		if _, err := s.Buscar(en); err != nil {
			return nil, err
		}
	}
	var ultima image.Image
	for range MaxPaquetesFotograma {
		m, err := s.Siguiente()
		if err != nil {
			break
		}
		if m.Imagen == nil {
			continue
		}
		if ultima != nil && m.Tiempo > en {
			break
		}
		ultima = m.Imagen
		if m.Tiempo >= en {
			break
		}
	}
	if ultima == nil {
		return nil, errors.New("video: el sistema no dio ningún fotograma")
	}
	return ultima, nil
}
