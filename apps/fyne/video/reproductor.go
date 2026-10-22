// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build !js

package video

import (
	"encoding/binary"
	"errors"
	"image"
	"io"
	"sync"
	"time"

	"github.com/thesyncim/govpx"
	"github.com/tphakala/go-opus/opus"

	"git.sytue.eu/sytue/one/apps/fyne/llamada"
)

// # Cómo va el reloj
//
// El reloj maestro es el AUDIO: lo que ya ha sonado por la tarjeta dice en qué
// instante está el vídeo, y cada fotograma se enseña cuando le llega su hora —si llega
// tarde se descodifica igual (VP9 lo necesita para el siguiente) pero no se pinta; si
// llega pronto, se espera—. Es lo que hace cualquier reproductor: el oído nota un corte
// de 20 ms y el ojo no nota un fotograma de menos.
//
// Sin pista de audio o sin tarjeta, el reloj es el de pared desde que se pulsó
// reproducir.
//
// Tres rutinas:
//
//   - la que LEE (bucleLectura): saca paquetes del WebM, descodifica el Opus a PCM y
//     encola el VP9 comprimido; se para cuando lleva `Adelanto` leído por delante del
//     reloj, para no bajar el fichero entero;
//   - la que PINTA (bucleVideo): descodifica el VP9 cuando le toca y entrega la imagen;
//   - la de la TARJETA (la del Dispositivo): pide PCM cada 20 ms y así avanza el reloj.

// Adelanto es cuánto se lee por delante de lo que suena.
const Adelanto = 1500 * time.Millisecond

// Reproductor reproduce un WebM VP9/Opus. El cero no vale: se crea con Nuevo.
type Reproductor struct {
	w   *WebM
	src io.ReadSeeker
	// dispositivo es la tarjeta; nil, sin sonido y con el reloj de pared.
	dispositivo llamada.Dispositivo

	// AlFotograma recibe cada imagen que toca enseñar y su instante. Se llama desde otra
	// rutina: quien pinta la lleva al hilo de la interfaz.
	AlFotograma func(img image.Image, t time.Duration)
	// AlCambiar avisa de que ha cambiado el estado (pausa, fin, error). Otra rutina.
	AlCambiar func()

	video, audio *Pista
	vdec         *govpx.VP9Decoder
	adec         *opus.Decoder
	canalesAudio int
	preskip      int

	lectura sync.Mutex // el WebM: lo usan la rutina que lee y Buscar
	// soltada: Cerrar ya ha devuelto y la fuente puede cerrarse; nadie la vuelve a leer.
	// Va con `lectura`, no con `mu`.
	soltada bool
	mu      sync.Mutex
	hay     *sync.Cond

	gen         int // sube con cada búsqueda: lo leído antes es de otra posición
	base        time.Duration
	muestras    int64     // de audio que han sonado desde base
	pared       time.Time // cuándo arrancó el reloj de pared (desde base)
	pausado     bool
	cerrado     bool
	finLectura  bool
	fin         bool
	err         error
	leidoHasta  time.Duration
	descartar   time.Duration // lo anterior a esto no se enseña ni suena (tras buscar)
	pcm         []int16       // mono 48 kHz, lo que falta por sonar
	cola        []Paquete     // vídeo comprimido, en orden
	volumen     float64
	silencio    bool
	cerrarAudio func()
	// sinTarjeta: la tarjeta no se pudo abrir; el reloj es el de pared y el audio no se
	// descodifica (se acumularía sin que nadie lo gaste).
	sinTarjeta   bool
	ultimo       time.Duration
	hayFotograma bool
}

// Nuevo abre el WebM y prepara los descodificadores. `dispositivo` puede ser nil. Empieza
// en pausa: nada suena ni se pinta hasta Reproducir, salvo el primer fotograma, que se
// entrega en cuanto está (es el póster de verdad).
func Nuevo(src io.ReadSeeker, dispositivo llamada.Dispositivo) (*Reproductor, error) {
	w, err := Abrir(src)
	if err != nil {
		return nil, err
	}
	r := &Reproductor{w: w, src: src, dispositivo: dispositivo, pausado: true, volumen: 1}
	r.hay = sync.NewCond(&r.mu)
	if v := w.Pista(TipoVideo); v != nil && v.Codec == CodecVP9 {
		r.video = v
		if r.vdec, err = govpx.NewVP9Decoder(govpx.VP9DecoderOptions{Threads: 2}); err != nil {
			return nil, err
		}
	}
	if a := w.Pista(TipoAudio); a != nil && a.Codec == CodecOpus {
		r.audio = a
		r.canalesAudio = min(max(a.Canales, 1), 2)
		r.preskip = preskipDe(a.Privado)
		if r.adec, err = opus.NewDecoder(llamada.Muestreo, r.canalesAudio); err != nil {
			return nil, err
		}
	}
	if r.video == nil && r.audio == nil {
		return nil, errors.New("video: el fichero no trae VP9 ni Opus")
	}
	return r, nil
}

// preskipDe es el pre-skip de la cabecera OpusHead: las muestras del principio que el
// codificador mete de relleno y no se oyen.
func preskipDe(cabecera []byte) int {
	if len(cabecera) < 12 || string(cabecera[:8]) != "OpusHead" {
		return 0
	}
	return int(binary.LittleEndian.Uint16(cabecera[10:12]))
}

// Arrancar pone en marcha las rutinas. Se llama una vez.
func (r *Reproductor) Arrancar() {
	go r.bucleLectura()
	go r.bucleVideo()
}

// Duracion es la del fichero; cero si no la dice.
func (r *Reproductor) Duracion() time.Duration { return r.w.Duracion }

// Tamano es el del vídeo; cero sin vídeo.
func (r *Reproductor) Tamano() (ancho, alto int) {
	if r.video == nil {
		return 0, 0
	}
	return r.video.Ancho, r.video.Alto
}

// Posicion es el instante que se está enseñando.
func (r *Reproductor) Posicion() time.Duration {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.reloj()
}

// Pausado dice si está en pausa (también al acabar).
func (r *Reproductor) Pausado() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.pausado
}

// Terminado dice si ha llegado al final.
func (r *Reproductor) Terminado() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.fin
}

// Err es el error que paró la reproducción, si lo hubo.
func (r *Reproductor) Err() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.err
}

// reloj es el instante actual; con el cerrojo tomado.
func (r *Reproductor) reloj() time.Duration {
	if r.usaAudio() {
		return r.base + time.Duration(r.muestras)*time.Second/llamada.Muestreo
	}
	if r.pausado || r.pared.IsZero() {
		return r.base
	}
	return r.base + time.Since(r.pared)
}

func (r *Reproductor) usaAudio() bool { return r.audio != nil && r.cerrarAudio != nil }

// Reproducir quita la pausa. Al final, vuelve a empezar.
func (r *Reproductor) Reproducir() {
	r.mu.Lock()
	fin := r.fin
	r.mu.Unlock()
	if fin {
		r.Buscar(0)
	}
	r.mu.Lock()
	if !r.pausado || r.cerrado {
		r.mu.Unlock()
		return
	}
	r.pausado = false
	r.pared = time.Now()
	abrir := r.audio != nil && r.dispositivo != nil && r.cerrarAudio == nil
	r.mu.Unlock()
	if abrir {
		// La tarjeta se abre al reproducir y no al crear: abrirla es pedir permiso de
		// audio al sistema, y quien solo mira el póster no tiene por qué.
		cerrar, err := r.dispositivo.Abrir(nil, r.alimentar)
		r.mu.Lock()
		if err == nil {
			r.cerrarAudio = cerrar
			r.base, r.muestras = r.reloj(), 0
		} else {
			r.sinTarjeta = true
			r.pcm = r.pcm[:0]
		}
		r.mu.Unlock()
	}
	r.hay.Broadcast()
	r.avisar()
}

// Pausar para el reloj.
func (r *Reproductor) Pausar() {
	r.mu.Lock()
	if r.pausado {
		r.mu.Unlock()
		return
	}
	r.base = r.reloj()
	r.muestras = 0
	r.pausado = true
	r.mu.Unlock()
	r.avisar()
}

// Alternar es la barra espaciadora.
func (r *Reproductor) Alternar() {
	if r.Pausado() {
		r.Reproducir()
		return
	}
	r.Pausar()
}

// Buscar salta a `t`: al punto de búsqueda anterior y desde ahí descodifica sin
// enseñar hasta llegar.
func (r *Reproductor) Buscar(t time.Duration) {
	if d := r.w.Duracion; d > 0 && t > d {
		t = d
	}
	t = max(t, 0)
	r.lectura.Lock()
	if r.soltada {
		r.lectura.Unlock()
		return
	}
	_, err := r.w.Buscar(t)
	r.lectura.Unlock()
	r.mu.Lock()
	r.gen++
	r.base, r.muestras, r.descartar = t, 0, t
	r.pared = time.Now()
	r.pcm, r.cola = r.pcm[:0], r.cola[:0]
	r.leidoHasta, r.finLectura, r.fin, r.err = t, false, false, err
	r.hayFotograma = false
	if r.adec != nil {
		r.adec.Reset()
	}
	r.mu.Unlock()
	r.hay.Broadcast()
	r.avisar()
}

// Volumen pone el volumen, de 0 a 1.
func (r *Reproductor) Volumen(v float64) {
	r.mu.Lock()
	r.volumen = min(max(v, 0), 1)
	r.mu.Unlock()
}

// Silenciar quita o devuelve el sonido sin tocar el volumen (la tecla M).
func (r *Reproductor) Silenciar(si bool) {
	r.mu.Lock()
	r.silencio = si
	r.mu.Unlock()
}

// Silenciado dice si está sin sonido.
func (r *Reproductor) Silenciado() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.silencio
}

// Cerrar lo para todo y suelta la tarjeta. La fuente la cierra quien la abrió, y puede
// hacerlo en cuanto esto devuelve: espera a la lectura en curso y nadie vuelve a leer.
func (r *Reproductor) Cerrar() {
	r.mu.Lock()
	if r.cerrado {
		r.mu.Unlock()
		return
	}
	r.cerrado = true
	cerrar := r.cerrarAudio
	r.cerrarAudio = nil
	r.mu.Unlock()
	r.hay.Broadcast()
	r.lectura.Lock()
	r.soltada = true
	r.lectura.Unlock()
	if cerrar != nil {
		cerrar()
	}
	// El descodificador de vídeo lo cierra su rutina al salir: puede estar a medias.
}

func (r *Reproductor) avisar() {
	if r.AlCambiar != nil {
		r.AlCambiar()
	}
}

// alimentar es la rutina de la tarjeta: rellena `out` con lo que toca sonar y así
// avanza el reloj. En pausa, o sin datos, silencio (y el reloj no se mueve).
func (r *Reproductor) alimentar(out []int16) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.pausado || r.cerrado {
		clear(out)
		return
	}
	n := min(len(out), len(r.pcm))
	ganancia := r.volumen
	if r.silencio {
		ganancia = 0
	}
	for i := range n {
		out[i] = int16(float64(r.pcm[i]) * ganancia)
	}
	clear(out[n:])
	r.pcm = r.pcm[n:]
	r.muestras += int64(n)
	if n > 0 {
		r.hay.Broadcast()
	}
}

// bucleLectura saca paquetes del WebM mientras no vaya demasiado por delante.
func (r *Reproductor) bucleLectura() {
	pcm := make([]int16, llamada.Muestreo/10*2) // 120 ms en estéreo, el paquete de Opus más largo
	for {
		r.mu.Lock()
		for !r.cerrado && (r.finLectura || r.leidoHasta-r.reloj() > Adelanto && (!r.pausado || r.hayFotograma)) {
			r.hay.Wait()
		}
		if r.cerrado {
			r.mu.Unlock()
			return
		}
		gen := r.gen
		r.mu.Unlock()

		r.lectura.Lock()
		if r.soltada {
			r.lectura.Unlock()
			return
		}
		p, err := r.w.Siguiente()
		r.lectura.Unlock()

		r.mu.Lock()
		if gen != r.gen {
			r.mu.Unlock()
			continue
		}
		if err != nil {
			r.finLectura = true
			if !errors.Is(err, io.EOF) {
				r.err = err
			}
			r.mu.Unlock()
			r.hay.Broadcast()
			continue
		}
		r.leidoHasta = max(r.leidoHasta, p.Tiempo)
		switch {
		case r.video != nil && p.Pista == r.video.Numero:
			r.cola = append(r.cola, p)
		case r.audio != nil && p.Pista == r.audio.Numero && r.dispositivo != nil && !r.sinTarjeta:
			r.encolarAudio(p, pcm)
		}
		r.mu.Unlock()
		r.hay.Broadcast()
	}
}

// encolarAudio descodifica un paquete de Opus y deja su PCM, en mono, detrás de lo que
// falta por sonar. Con el cerrojo tomado.
func (r *Reproductor) encolarAudio(p Paquete, pcm []int16) {
	n, err := r.adec.Decode(p.Datos, pcm)
	if err != nil {
		return
	}
	muestras := pcm[:n*r.canalesAudio]
	desde := 0
	if r.preskip > 0 {
		desde = min(r.preskip, n)
		r.preskip -= desde
	}
	if p.Tiempo < r.descartar {
		// Antes del punto buscado: no suena. El reloj arranca en `base`.
		return
	}
	for i := desde; i < n; i++ {
		if r.canalesAudio == 2 {
			r.pcm = append(r.pcm, int16((int32(muestras[2*i])+int32(muestras[2*i+1]))/2)) //nolint:gosec // G115: la media de dos int16 cabe en int16
		} else {
			r.pcm = append(r.pcm, muestras[i])
		}
	}
}

// bucleVideo descodifica y entrega cada fotograma a su hora.
func (r *Reproductor) bucleVideo() {
	for {
		p, gen, ensenar, seguir := r.siguienteFotograma()
		if !seguir {
			if r.vdec != nil {
				_ = r.vdec.Close()
			}
			return
		}
		if p == nil {
			continue
		}
		if err := r.vdec.Decode(p.Datos); err != nil {
			continue
		}
		f, ok := r.vdec.NextFrame()
		if !ok || !ensenar {
			continue
		}
		img := llamada.ImagenI420(f)
		r.mu.Lock()
		vale := gen == r.gen
		if vale {
			r.hayFotograma = true
			r.ultimo = p.Tiempo
		}
		r.mu.Unlock()
		if vale && r.AlFotograma != nil {
			r.AlFotograma(img, p.Tiempo)
		}
	}
}

// siguienteFotograma espera a que toque el siguiente paquete de vídeo y lo saca de la
// cola, diciendo si se enseña o solo se descodifica. nil con `seguir` es «vuelve a
// mirar»; `seguir` a false, que se ha cerrado.
func (r *Reproductor) siguienteFotograma() (p *Paquete, gen int, ensenar, seguir bool) {
	r.mu.Lock()
	for !r.cerrado && len(r.cola) == 0 && (!r.finLectura || r.fin) {
		r.hay.Wait()
	}
	if r.cerrado {
		r.mu.Unlock()
		return nil, 0, false, false
	}
	if len(r.cola) == 0 {
		// Leído entero y sin vídeo pendiente: se acaba cuando acaba el audio.
		if !r.usaAudio() || len(r.pcm) == 0 {
			r.acabar()
			r.mu.Unlock()
			r.avisar()
			return nil, 0, false, true
		}
		r.mu.Unlock()
		time.Sleep(20 * time.Millisecond)
		return nil, 0, false, true
	}
	ahora := r.reloj()
	sig := r.cola[0]
	// El primer fotograma tras abrir o buscar se enseña ya, en pausa también: es el que
	// queda quieto en pantalla.
	primero := !r.hayFotograma && sig.Tiempo >= r.descartar
	if sig.Tiempo > ahora && !primero {
		if r.pausado {
			// Quieto hasta que alguien quite la pausa o busque.
			r.hay.Wait()
			r.mu.Unlock()
			return nil, 0, false, true
		}
		espera := min(sig.Tiempo-ahora, 20*time.Millisecond)
		r.mu.Unlock()
		time.Sleep(espera)
		return nil, 0, false, true
	}
	r.cola = r.cola[1:]
	// Se enseña si es el último que ya toca: si el siguiente también llegó, este va tarde
	// y solo se descodifica.
	ensenar = sig.Tiempo >= r.descartar && (primero || len(r.cola) == 0 || r.cola[0].Tiempo > ahora)
	gen = r.gen
	r.mu.Unlock()
	r.hay.Broadcast()
	return &sig, gen, ensenar, true
}

// acabar deja el reproductor parado al final. Con el cerrojo tomado.
func (r *Reproductor) acabar() {
	r.fin = true
	if !r.pausado {
		r.base = r.reloj()
		r.muestras = 0
		r.pausado = true
	}
	if d := r.w.Duracion; d > 0 {
		r.base = d
	}
}
