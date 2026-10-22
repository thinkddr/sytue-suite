// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build !js

package video

import (
	"bytes"
	"errors"
	"image"
	"io"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/thesyncim/govpx"

	"git.sytue.eu/sytue/one/apps/fyne/llamada"
)

// La fixture es un WebM de 2 s como los que deja el transcodificador, a escala: VP9 a
// 320×180 y 15 fps con un fotograma clave por segundo, y Opus estéreo a 48 kHz con un
// tono de 440 Hz. La mitad izquierda es azul (#1e6fd9) y la derecha vino (#9f1239).
// Se generó UNA vez, en Linux, con:
//
//	ffmpeg -f lavfi -i "color=c=0x9f1239:s=320x180:r=15:d=2,drawbox=x=0:y=0:w=160:h=180:color=0x1e6fd9:t=fill" \
//	  -f lavfi -i "sine=frequency=440:sample_rate=48000:duration=2" \
//	  -c:v libvpx-vp9 -b:v 200k -g 15 -deadline good -cpu-used 4 \
//	  -c:a libopus -b:a 48k -ac 2 -cues_to_front 1 -shortest dos-segundos.webm
//
// ffprobe dice de ella: 30 paquetes de vídeo (claves en 0 y 1 s) y 101 de audio.
const fixture = "testdata/dos-segundos.webm"

func abrirFixture(t *testing.T) *bytes.Reader {
	t.Helper()
	b, err := os.ReadFile(fixture)
	if err != nil {
		t.Fatal(err)
	}
	if len(b) > 200<<10 {
		t.Fatalf("la fixture pesa %d bytes: el tope es 200 KB", len(b))
	}
	return bytes.NewReader(b)
}

// TestElDemuxerLeeLaFixture: pistas, duración, puntos de búsqueda y cada bloque con su
// instante, contra lo que dice ffprobe.
func TestElDemuxerLeeLaFixture(t *testing.T) {
	w, err := Abrir(abrirFixture(t))
	if err != nil {
		t.Fatal(err)
	}
	v, a := w.Pista(TipoVideo), w.Pista(TipoAudio)
	if v == nil || v.Codec != CodecVP9 || v.Ancho != 320 || v.Alto != 180 {
		t.Fatalf("pista de vídeo %+v", v)
	}
	if a == nil || a.Codec != CodecOpus || a.Canales != 2 || a.Muestreo != 48000 || preskipDe(a.Privado) != 312 {
		t.Fatalf("pista de audio %+v (pre-skip %d)", a, preskipDe(a.Privado))
	}
	if w.Duracion < 1990*time.Millisecond || w.Duracion > 2020*time.Millisecond {
		t.Errorf("duración %v y ffprobe dice 2,008 s", w.Duracion)
	}
	if got := w.PuntosDeBusqueda(); len(got) != 2 || got[0] != 0 || got[1] != time.Second {
		t.Errorf("puntos de búsqueda %v; el fichero tiene una clave por segundo", got)
	}
	var nv, na, claves int
	var ultimoV time.Duration = -1
	for {
		p, err := w.Siguiente()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		switch p.Pista {
		case v.Numero:
			if p.Tiempo <= ultimoV {
				t.Errorf("vídeo fuera de orden: %v tras %v", p.Tiempo, ultimoV)
			}
			// 15 fps: el fotograma n está en n × 66,67 ms, redondeado al ms.
			if quiero := time.Duration(nv) * time.Second / 15; p.Tiempo-quiero > time.Millisecond || quiero-p.Tiempo > time.Millisecond {
				t.Errorf("el fotograma %d está en %v y debería en %v", nv, p.Tiempo, quiero)
			}
			ultimoV = p.Tiempo
			nv++
			if p.Clave {
				claves++
			}
		case a.Numero:
			na++
		}
	}
	if nv != 30 || na != 101 || claves != 2 {
		t.Errorf("%d de vídeo, %d de audio y %d claves; ffprobe dice 30, 101 y 2", nv, na, claves)
	}
}

// TestElPrimerFotogramaSeDescodifica: el VP9 del primer bloque sale a 320×180 con el
// azul a la izquierda y el vino a la derecha.
func TestElPrimerFotogramaSeDescodifica(t *testing.T) {
	w, err := Abrir(abrirFixture(t))
	if err != nil {
		t.Fatal(err)
	}
	img := primerFotograma(t, w)
	if img.Bounds().Dx() != 320 || img.Bounds().Dy() != 180 {
		t.Fatalf("el fotograma mide %v", img.Bounds())
	}
	cerca(t, "izquierda", img.At(40, 90), 0x1e, 0x6f, 0xd9)
	cerca(t, "derecha", img.At(280, 90), 0x9f, 0x12, 0x39)
}

func primerFotograma(t *testing.T, w *WebM) image.Image {
	t.Helper()
	dec, err := govpx.NewVP9Decoder(govpx.VP9DecoderOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = dec.Close() }()
	video := w.Pista(TipoVideo).Numero
	for {
		p, err := w.Siguiente()
		if err != nil {
			t.Fatal(err)
		}
		if p.Pista != video {
			continue
		}
		if err := dec.Decode(p.Datos); err != nil {
			t.Fatal(err)
		}
		f, ok := dec.NextFrame()
		if !ok {
			t.Fatal("el primer bloque no da fotograma")
		}
		return llamada.ImagenI420(f)
	}
}

// cerca compara un píxel con su color de origen con la tolerancia de VP9 a 200 kbps y
// del paso de YUV a RGB (redondeos y rango limitado).
func cerca(t *testing.T, donde string, c interface{ RGBA() (r, g, b, a uint32) }, r, g, b uint8) {
	t.Helper()
	cr, cg, cb, _ := c.RGBA()
	d := func(x uint32, y uint8) int {
		v := int(x>>8) - int(y)
		if v < 0 {
			return -v
		}
		return v
	}
	if d(cr, r) > 12 || d(cg, g) > 12 || d(cb, b) > 12 {
		t.Errorf("el píxel de la %s es (%d,%d,%d) y la fuente es (%d,%d,%d)", donde, cr>>8, cg>>8, cb>>8, r, g, b)
	}
}

// TestBuscarVaAlPuntoAnterior: pedir 1,5 s deja el fichero en el Cluster de 1 s, y lo
// siguiente que sale es su fotograma clave.
func TestBuscarVaAlPuntoAnterior(t *testing.T) {
	w, err := Abrir(abrirFixture(t))
	if err != nil {
		t.Fatal(err)
	}
	// Se lee algo antes, para que buscar tenga que volver atrás de verdad.
	for range 20 {
		if _, err := w.Siguiente(); err != nil {
			t.Fatal(err)
		}
	}
	desde, err := w.Buscar(1500 * time.Millisecond)
	if err != nil || desde != time.Second {
		t.Fatalf("Buscar(1,5 s) = %v, %v; el punto anterior es 1 s", desde, err)
	}
	video := w.Pista(TipoVideo).Numero
	for {
		p, err := w.Siguiente()
		if err != nil {
			t.Fatal(err)
		}
		if p.Pista == video {
			if !p.Clave || p.Tiempo != time.Second {
				t.Errorf("tras buscar sale %v (clave %v)", p.Tiempo, p.Clave)
			}
			break
		}
	}
	if desde, _ := w.Buscar(200 * time.Millisecond); desde != 0 {
		t.Errorf("Buscar(0,2 s) = %v", desde)
	}
}

// TestLosCuesDelFinalSeEncuentran: sin `-cues_to_front`, ffmpeg deja los Cues detrás de
// los Clusters y los apunta en el SeekHead. `cues-al-final.webm` es la misma fixture
// remuxada así (`ffmpeg -i dos-segundos.webm -c copy cues-al-final.webm`).
func TestLosCuesDelFinalSeEncuentran(t *testing.T) {
	b, err := os.ReadFile("testdata/cues-al-final.webm")
	if err != nil {
		t.Fatal(err)
	}
	w, err := Abrir(bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	// El remux corre los tiempos 7 ms (el retardo del Opus): los dos puntos siguen a un
	// segundo uno del otro.
	got := w.PuntosDeBusqueda()
	if len(got) != 2 || got[1]-got[0] != time.Second {
		t.Fatalf("puntos de búsqueda %v", got)
	}
	// Y tras ir a buscarlos, la lectura sigue en el primer Cluster.
	if p, err := w.Siguiente(); err != nil || p.Tiempo > got[0] {
		t.Errorf("tras leer los Cues del final, lo primero es %v, %v", p.Tiempo, err)
	}
	if desde, err := w.Buscar(1200 * time.Millisecond); err != nil || desde != got[1] {
		t.Errorf("Buscar = %v, %v", desde, err)
	}
}

// TestUnFicheroQueNoEsWebMSeRechaza: ni otro formato ni uno cortado.
func TestUnFicheroQueNoEsWebMSeRechaza(t *testing.T) {
	if _, err := Abrir(bytes.NewReader([]byte("RIFF....WAVEfmt "))); !errors.Is(err, ErrNoEsWebM) {
		t.Errorf("un WAV da %v", err)
	}
	b, _ := os.ReadFile(fixture)
	if _, err := Abrir(bytes.NewReader(b[:200])); err == nil {
		t.Error("un WebM cortado antes del primer Cluster se abre sin error")
	}
}

// TestElLacingSeParte: los tres tipos de lacing, con tres fotogramas de 3, 300 y 5 bytes.
func TestElLacingSeParte(t *testing.T) {
	a, b, c := bytes.Repeat([]byte{1}, 3), bytes.Repeat([]byte{2}, 300), bytes.Repeat([]byte{3}, 5)
	cuerpo := append(append(append([]byte{}, a...), b...), c...)
	xiph := append([]byte{2, 3, 255, 45}, cuerpo...)
	// EBML: 3 como vint de un byte (0x83) y 297 de diferencia, desplazada 8191 en dos bytes.
	dif := 297 + 8191
	ebml := append([]byte{2, 0x83, 0x40 | byte(dif>>8), byte(dif)}, cuerpo...)
	for nombre, caso := range map[string]struct {
		datos  []byte
		lacing byte
	}{"Xiph": {xiph, 1}, "EBML": {ebml, 3}} {
		out, err := deslazar(caso.datos, caso.lacing)
		if err != nil || len(out) != 3 || len(out[0]) != 3 || len(out[1]) != 300 || len(out[2]) != 5 || out[2][0] != 3 {
			t.Errorf("%s: %d trozos, %v", nombre, len(out), err)
		}
	}
	fijo, err := deslazar(append([]byte{2}, bytes.Repeat([]byte{7}, 9)...), 2)
	if err != nil || len(fijo) != 3 || len(fijo[1]) != 3 {
		t.Errorf("fijo: %v, %v", fijo, err)
	}
	if _, err := deslazar([]byte{2, 255, 255}, 1); err == nil {
		t.Error("un lacing Xiph cortado no da error")
	}
}

// tarjetaFalsa hace de altavoz: pide un tramo de 20 ms cada vez que se le dice, sin
// tiempo real, y guarda lo que sonaría.
type tarjetaFalsa struct {
	mu      sync.Mutex
	pedir   func([]int16)
	sonado  []int16
	abierta bool
}

func (f *tarjetaFalsa) Abrir(_, reproducir func([]int16)) (func(), error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.pedir, f.abierta = reproducir, true
	return func() { f.mu.Lock(); f.abierta = false; f.mu.Unlock() }, nil
}

func (f *tarjetaFalsa) tramo() {
	f.mu.Lock()
	pedir := f.pedir
	f.mu.Unlock()
	if pedir == nil {
		return
	}
	out := make([]int16, llamada.Tramo)
	pedir(out)
	f.mu.Lock()
	f.sonado = append(f.sonado, out...)
	f.mu.Unlock()
}

// TestElRelojEsElAudio: con tarjeta, el vídeo avanza al ritmo al que la tarjeta pide
// sonido, y lo que suena es el tono (no silencio). Sin pedir, el reloj no se mueve.
func TestElRelojEsElAudio(t *testing.T) {
	tarjeta := &tarjetaFalsa{}
	r, err := Nuevo(abrirFixture(t), tarjeta)
	if err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	var vistos []time.Duration
	r.AlFotograma = func(_ image.Image, t time.Duration) { mu.Lock(); vistos = append(vistos, t); mu.Unlock() }
	r.Arrancar()
	defer r.Cerrar()
	esperar(t, func() bool { mu.Lock(); defer mu.Unlock(); return len(vistos) == 1 }, "el primer fotograma en pausa")
	r.Reproducir()
	if !tarjeta.abierta {
		t.Fatal("reproducir no abre la tarjeta")
	}
	time.Sleep(100 * time.Millisecond)
	if p := r.Posicion(); p != 0 {
		t.Errorf("sin que la tarjeta pida, el reloj está en %v", p)
	}
	// Medio segundo de audio, tramo a tramo.
	for range 25 {
		esperar(t, func() bool { r.mu.Lock(); defer r.mu.Unlock(); return len(r.pcm) >= llamada.Tramo }, "PCM que sonar")
		tarjeta.tramo()
	}
	if p := r.Posicion(); p != 500*time.Millisecond {
		t.Errorf("tras 25 tramos el reloj está en %v y deberían ser 500 ms", p)
	}
	esperar(t, func() bool { mu.Lock(); defer mu.Unlock(); return vistos[len(vistos)-1] >= 400*time.Millisecond }, "fotogramas hasta los 400 ms")
	mu.Lock()
	ultimo := vistos[len(vistos)-1]
	mu.Unlock()
	if ultimo > 500*time.Millisecond {
		t.Errorf("se ha enseñado un fotograma de %v con el reloj en 500 ms", ultimo)
	}
	pico := 0
	for _, m := range tarjeta.sonado[llamada.Tramo*5:] {
		pico = max(pico, int(m), -int(m))
	}
	if pico < 1000 {
		t.Errorf("el tono suena con un pico de %d: el Opus no se está descodificando", pico)
	}
	r.Silenciar(true)
	tarjeta.tramo()
	for _, m := range tarjeta.sonado[len(tarjeta.sonado)-llamada.Tramo:] {
		if m != 0 {
			t.Fatal("silenciado y sigue sonando")
		}
	}
}

// TestSinTarjetaVaConElRelojDePared: sin audio llega al final en unos dos segundos, con
// los fotogramas en orden, y se queda en pausa al acabar. Buscar hacia atrás lo
// devuelve a ese punto.
func TestSinTarjetaVaConElRelojDePared(t *testing.T) {
	r, err := Nuevo(abrirFixture(t), nil)
	if err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	var vistos []time.Duration
	r.AlFotograma = func(_ image.Image, t time.Duration) { mu.Lock(); vistos = append(vistos, t); mu.Unlock() }
	r.Arrancar()
	defer r.Cerrar()
	inicio := time.Now()
	r.Reproducir()
	esperar(t, r.Terminado, "el final")
	if d := time.Since(inicio); d < 1800*time.Millisecond {
		t.Errorf("ha acabado en %v: va más rápido que el reloj", d)
	}
	if !r.Pausado() {
		t.Error("al acabar no se queda en pausa")
	}
	mu.Lock()
	for i := 1; i < len(vistos); i++ {
		if vistos[i] <= vistos[i-1] {
			t.Errorf("fotogramas fuera de orden: %v tras %v", vistos[i], vistos[i-1])
		}
	}
	if len(vistos) < 20 {
		t.Errorf("solo se han enseñado %d de 30 fotogramas", len(vistos))
	}
	vistos = nil
	mu.Unlock()

	r.Buscar(1500 * time.Millisecond)
	esperar(t, func() bool { mu.Lock(); defer mu.Unlock(); return len(vistos) > 0 }, "el fotograma tras buscar")
	mu.Lock()
	if vistos[0] < 1500*time.Millisecond-70*time.Millisecond || vistos[0] > 1600*time.Millisecond {
		t.Errorf("tras buscar 1,5 s se enseña %v", vistos[0])
	}
	mu.Unlock()
	if p := r.Posicion(); p != 1500*time.Millisecond {
		t.Errorf("tras buscar en pausa, la posición es %v", p)
	}
}

func esperar(t *testing.T, f func() bool, que string) {
	t.Helper()
	limite := time.Now().Add(10 * time.Second)
	for !f() {
		if time.Now().After(limite) {
			t.Fatalf("no llega %s", que)
		}
		time.Sleep(5 * time.Millisecond)
	}
}
