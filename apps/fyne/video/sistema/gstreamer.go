// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build !js && !android && !ios && !windows

// Package sistema son los descodificadores del sistema del reproductor de vídeo
// (PLAN-EDICION-LOCAL §6.10, CA-08): cumplen `video.Sistema` y se registran con
// `video.RegistrarSistema`. Solo los usa Sytue Office; la edición conectada no los
// registra.
//
// Hecho: GStreamer POR PROCESO, sin enlazar sus bibliotecas (sin cgo): si están
// `gst-launch-1.0` y `gst-discoverer-1.0` (Linux; también el instalador de GStreamer
// para macOS), el discoverer dice duración, tamaño y cadencia, y dos
// `gst-launch-1.0` descodifican uno la imagen a I420 y otro el sonido a PCM mono de
// 48 kHz, cada uno a su tubería (fdsink fd=3). Límites de este camino: el instante de
// cada fotograma sale de la cadencia (un vídeo de cadencia variable se desfasa), y
// buscar es volver a lanzar y descartar hasta el punto (gst-launch no sabe empezar en
// un instante), así que saltar lejos en un vídeo largo tarda.
//
// También hechos, con la misma interfaz y registrados igual: MediaCodec en Android
// (`MediaExtractor` + `MediaCodec`, por el puente JNI de la app: mediacodec_android.go) y
// Media Foundation en Windows (`mediafoundation_windows.go`, con el lector de origen en Go
// puro en `video/sistema/mediafoundation`), los dos sin probar aún en un dispositivo.
// GStreamer por proceso no vale en Windows: `exec.Cmd.ExtraFiles` (la tubería fd=3) no
// existe allí, así que este fichero no entra. Por hacer (ver `video/sistema.go`):
// AVFoundation en macOS e iOS (`AVAssetReader`, puente en Objective-C).
package sistema

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"git.sytue.eu/sytue/one/apps/fyne/video"
)

// Muestreo es el del PCM que se pide (el de la tarjeta del reproductor).
const Muestreo = 48000

// trozoAudio son las muestras de cada trozo de audio que se entrega (20 ms).
const trozoAudio = Muestreo / 50

// Programas son las rutas de las herramientas de GStreamer.
type Programas struct {
	Lanzar, Discoverer string
}

// BuscarGStreamer encuentra gst-launch-1.0 y gst-discoverer-1.0 en el PATH.
func BuscarGStreamer() (Programas, error) {
	l, err := exec.LookPath("gst-launch-1.0")
	if err != nil {
		return Programas{}, fmt.Errorf("sistema: no está gst-launch-1.0: %w", err)
	}
	d, err := exec.LookPath("gst-discoverer-1.0")
	if err != nil {
		return Programas{}, fmt.Errorf("sistema: no está gst-discoverer-1.0: %w", err)
	}
	return Programas{Lanzar: l, Discoverer: d}, nil
}

// Registrar registra GStreamer en el reproductor si está instalado, y dice si lo está.
func Registrar() bool {
	p, err := BuscarGStreamer()
	if err != nil {
		return false
	}
	video.RegistrarSistema(func(ruta string) (video.Sistema, error) { return p.Abrir(ruta) })
	return true
}

// Datos es lo que el discoverer dice de un fichero.
type Datos struct {
	Duracion    time.Duration
	Ancho, Alto int
	Cadencia    float64 // fotogramas por segundo
	Video       bool
	Audio       bool
}

// Descubrir pregunta a gst-discoverer-1.0 por el fichero.
func (p Programas) Descubrir(ruta string) (Datos, error) {
	ctx, fin := context.WithTimeout(context.Background(), 30*time.Second)
	defer fin()
	out, err := exec.CommandContext(ctx, p.Discoverer, "-v", ruta).Output() //nolint:gosec // G204: el programa es el de GStreamer y la ruta va como argumento
	if err != nil && len(out) == 0 {
		return Datos{}, fmt.Errorf("sistema: gst-discoverer-1.0: %w", err)
	}
	return leerDescubrimiento(out)
}

// leerDescubrimiento lee la salida de `gst-discoverer-1.0 -v`.
func leerDescubrimiento(out []byte) (Datos, error) {
	var d Datos
	sc := bufio.NewScanner(bytes.NewReader(out))
	sc.Buffer(make([]byte, 64<<10), 1<<20)
	for sc.Scan() {
		l := strings.TrimSpace(sc.Text())
		switch {
		case strings.HasPrefix(l, "Duration:"):
			d.Duracion = duracionGst(strings.TrimSpace(strings.TrimPrefix(l, "Duration:")))
		case strings.HasPrefix(l, "video #"):
			d.Video = true
		case strings.HasPrefix(l, "audio #"):
			d.Audio = true
		case strings.HasPrefix(l, "Width:") && d.Ancho == 0:
			d.Ancho, _ = strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(l, "Width:")))
		case strings.HasPrefix(l, "Height:") && d.Alto == 0:
			d.Alto, _ = strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(l, "Height:")))
		case strings.HasPrefix(l, "Frame rate:") && d.Cadencia == 0:
			n, m, _ := strings.Cut(strings.TrimSpace(strings.TrimPrefix(l, "Frame rate:")), "/")
			a, _ := strconv.ParseFloat(n, 64)
			b, err := strconv.ParseFloat(m, 64)
			if err == nil && b > 0 {
				d.Cadencia = a / b
			}
		}
	}
	if !d.Video && !d.Audio {
		return d, errors.New("sistema: GStreamer no reconoce el fichero")
	}
	if d.Video && (d.Ancho <= 0 || d.Alto <= 0) {
		return d, errors.New("sistema: GStreamer no dice el tamaño del vídeo")
	}
	if d.Cadencia <= 0 || d.Cadencia > 300 {
		d.Cadencia = 25
	}
	return d, nil
}

// duracionGst lee «0:01:02.500000000».
func duracionGst(s string) time.Duration {
	partes := strings.Split(s, ":")
	if len(partes) != 3 {
		return 0
	}
	h, _ := strconv.Atoi(partes[0])
	m, _ := strconv.Atoi(partes[1])
	seg, _ := strconv.ParseFloat(partes[2], 64)
	return time.Duration(h)*time.Hour + time.Duration(m)*time.Minute + time.Duration(seg*float64(time.Second))
}

// Abrir abre ruta con GStreamer.
func (p Programas) Abrir(ruta string) (*Fichero, error) {
	if _, err := os.Stat(ruta); err != nil {
		return nil, err
	}
	d, err := p.Descubrir(ruta)
	if err != nil {
		return nil, err
	}
	f := &Fichero{p: p, ruta: ruta, d: d}
	if err := f.lanzar(0); err != nil {
		return nil, err
	}
	// Una pista que GStreamer ve pero no sabe descodificar (falta el plugin) acaba sin
	// dar nada: se quita, para que el reloj no espere a un audio que no llega.
	if f.d.Audio {
		if _, err := f.aud.datos.Peek(2); err != nil {
			f.d.Audio, f.finAud = false, true
			f.aud.cerrar()
			f.aud = nil
		}
	}
	if f.d.Video {
		if _, err := f.vid.datos.Peek(f.tamCuadro()); err != nil {
			f.d.Video, f.finVid = false, true
			f.vid.cerrar()
			f.vid = nil
		}
	}
	if !f.d.Video && !f.d.Audio {
		return nil, errors.New("sistema: GStreamer no sabe descodificar este fichero (¿faltan plugins?)")
	}
	return f, nil
}

// Fichero es un fichero abierto con GStreamer: cumple video.Sistema.
type Fichero struct {
	p    Programas
	ruta string
	d    Datos

	mu             sync.Mutex
	vid, aud       *proceso
	cuadro         int64 // fotogramas leídos desde el inicio
	muestras       int64 // de audio leídas desde el inicio
	finVid, finAud bool
}

var _ video.Sistema = (*Fichero)(nil)

// proceso es un gst-launch-1.0 con su tubería de datos.
type proceso struct {
	cmd   *exec.Cmd
	datos *bufio.Reader
	r     *os.File
}

func (p Programas) proceso(rama ...string) (*proceso, error) {
	r, w, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	args := append([]string{"-q"}, rama...)
	// Vive lo que el fichero abierto: lo para cerrar (Kill), no un contexto.
	cmd := exec.CommandContext(context.Background(), p.Lanzar, args...) //nolint:gosec // G204: el programa es el de GStreamer y la ruta va como argumento
	cmd.ExtraFiles = []*os.File{w}                                      // fd 3
	cmd.Stdout, cmd.Stderr = io.Discard, io.Discard
	if err := cmd.Start(); err != nil {
		_ = r.Close()
		_ = w.Close()
		return nil, err
	}
	_ = w.Close()
	return &proceso{cmd: cmd, datos: bufio.NewReaderSize(r, 1<<20), r: r}, nil
}

func (pr *proceso) cerrar() {
	if pr == nil {
		return
	}
	if pr.cmd.Process != nil {
		_ = pr.cmd.Process.Kill()
	}
	_ = pr.r.Close()
	_ = pr.cmd.Wait()
}

// lanzar (re)arranca los procesos y descarta hasta t. Con f.mu tomado o antes de usar.
func (f *Fichero) lanzar(t time.Duration) error {
	f.vid.cerrar()
	f.aud.cerrar()
	f.vid, f.aud = nil, nil
	f.cuadro, f.muestras, f.finVid, f.finAud = 0, 0, !f.d.Video, !f.d.Audio
	fuente := []string{"filesrc", "location=" + f.ruta, "!", "decodebin", "!"}
	var err error
	if f.d.Video {
		rama := append(append([]string{}, fuente...), "queue", "!", "videoconvert", "!", "videoscale", "!",
			fmt.Sprintf("video/x-raw,format=I420,width=%d,height=%d", f.d.Ancho, f.d.Alto), "!", "fdsink", "fd=3", "sync=false")
		if f.vid, err = f.p.proceso(rama...); err != nil {
			return err
		}
	}
	if f.d.Audio {
		rama := append(append([]string{}, fuente...), "queue", "!", "audioconvert", "!", "audioresample", "!",
			fmt.Sprintf("audio/x-raw,format=S16LE,channels=1,rate=%d,layout=interleaved", Muestreo), "!", "fdsink", "fd=3", "sync=false")
		if f.aud, err = f.p.proceso(rama...); err != nil {
			f.vid.cerrar()
			return err
		}
	}
	if t <= 0 {
		return nil
	}
	// Descartar hasta t: los fotogramas enteros y el audio hasta su muestra.
	if f.d.Video {
		n := int64(t.Seconds() * f.d.Cadencia)
		tam := int64(f.tamCuadro())
		if _, err := io.CopyN(io.Discard, f.vid.datos, n*tam); err != nil {
			f.finVid = true
		}
		f.cuadro = n
	}
	if f.d.Audio {
		n := int64(t.Seconds() * Muestreo)
		if _, err := io.CopyN(io.Discard, f.aud.datos, 2*n); err != nil {
			f.finAud = true
		}
		f.muestras = n
	}
	return nil
}

func (f *Fichero) tamCuadro() int {
	cw, ch := (f.d.Ancho+1)/2, (f.d.Alto+1)/2
	return f.d.Ancho*f.d.Alto + 2*cw*ch
}

// Info es lo que dijo el discoverer.
func (f *Fichero) Info() video.InfoSistema {
	return video.InfoSistema{Duracion: f.d.Duracion, Ancho: f.d.Ancho, Alto: f.d.Alto, ConVideo: f.d.Video, ConAudio: f.d.Audio}
}

func (f *Fichero) tiempoCuadro() time.Duration {
	return time.Duration(float64(f.cuadro) / f.d.Cadencia * float64(time.Second))
}

func (f *Fichero) tiempoAudio() time.Duration {
	return time.Duration(f.muestras) * time.Second / Muestreo
}

// Siguiente entrega la imagen o el trozo de audio que va antes en el tiempo.
func (f *Fichero) Siguiente() (video.Muestra, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for {
		if f.finVid && f.finAud {
			return video.Muestra{}, io.EOF
		}
		if !f.finVid && (f.finAud || f.tiempoCuadro() <= f.tiempoAudio()) {
			m, err := f.leerCuadro()
			if err != nil {
				f.finVid = true
				continue
			}
			return m, nil
		}
		m, err := f.leerAudio()
		if err != nil {
			f.finAud = true
			continue
		}
		return m, nil
	}
}

func (f *Fichero) leerCuadro() (video.Muestra, error) {
	img := image.NewYCbCr(image.Rect(0, 0, f.d.Ancho, f.d.Alto), image.YCbCrSubsampleRatio420)
	cw, ch := (f.d.Ancho+1)/2, (f.d.Alto+1)/2
	// Las tres capas van seguidas y sin relleno (videoconvert a I420 de ese tamaño); la
	// imagen de Go tiene sus pasos, que con estos anchos son los mismos.
	if img.YStride != f.d.Ancho || img.CStride != cw {
		return video.Muestra{}, errors.New("sistema: pasos inesperados")
	}
	for _, plano := range [][]byte{img.Y[:f.d.Ancho*f.d.Alto], img.Cb[:cw*ch], img.Cr[:cw*ch]} {
		if _, err := io.ReadFull(f.vid.datos, plano); err != nil {
			return video.Muestra{}, err
		}
	}
	m := video.Muestra{Tiempo: f.tiempoCuadro(), Imagen: img}
	f.cuadro++
	return m, nil
}

func (f *Fichero) leerAudio() (video.Muestra, error) {
	raw := make([]byte, 2*trozoAudio)
	n, err := io.ReadFull(f.aud.datos, raw)
	if n == 0 {
		if err == nil {
			err = io.EOF
		}
		return video.Muestra{}, err
	}
	pcm := make([]int16, n/2)
	for i := range pcm {
		pcm[i] = int16(binary.LittleEndian.Uint16(raw[2*i:])) //nolint:gosec // G115: PCM de 16 bits con signo
	}
	m := video.Muestra{Tiempo: f.tiempoAudio(), PCM: pcm}
	f.muestras += int64(len(pcm))
	return m, nil
}

// Buscar vuelve a lanzar GStreamer y descarta hasta t.
func (f *Fichero) Buscar(t time.Duration) (time.Duration, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if d := f.d.Duracion; d > 0 && t > d {
		t = d
	}
	return t, f.lanzar(max(t, 0))
}

// Close mata los procesos.
func (f *Fichero) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.vid.cerrar()
	f.aud.cerrar()
	f.vid, f.aud = nil, nil
	f.finVid, f.finAud = true, true
	return nil
}
