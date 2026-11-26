// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package mediafoundation descodifica vídeo con Media Foundation, el marco de Windows
// (PLAN-EDICION-LOCAL §6.10, CA-08): el lector de origen (`IMFSourceReader`, de
// `MFCreateSourceReaderFromURL`) da la imagen en I420 (o NV12, que se pasa a I420
// aquí) y el sonido en PCM, cada muestra con su instante; buscar es
// `SetCurrentPosition`. `video/sistema` lo envuelve como `video.Sistema` y lo registra
// en Windows; el Reproductor de Cast y el visor de Photos no cambian.
//
// Go puro, sin cgo: las llamadas COM van por `nucleo/sistema/com`, así que se compila
// en cruzado (`GOOS=windows CGO_ENABLED=0 go vet`), cosa que `video` no puede (enlaza
// `llamada/camara`, que es cgo). Por eso no importa `video`: entrega sus propios
// [Muestra] e [Info], con la misma forma.
//
// Defensivo: si falta la DLL (Windows N sin el paquete multimedia), el fichero no se
// abre o no hay descodificador para su formato (HEVC sin la extensión de la tienda,
// por ejemplo), Abrir devuelve un error y la app sigue con lo de siempre (ffmpeg, o el
// reproductor del sistema).
//
// Lo que no es COM (pasar NV12 a I420, encontrar los planos en el búfer, el PCM a mono
// de 48 kHz, los instantes y qué flujo va antes) está en este fichero, sin etiqueta de
// sistema, y se prueba en cualquiera.
//
// SIN PROBAR en un Windows de verdad (se escribió en un equipo sin Windows). Qué mirar
// la primera vez, con Cast y con el visor de Photos:
//
//  1. Que arranca y no se cae: los índices de los métodos COM y los GUID salen de las
//     cabeceras de mingw-w64; un índice mal es un fallo de acceso, no un error.
//  2. MP4 H.264 + AAC de 1080p (se descodifica en 1088 filas): sin franja verde abajo
//     ni colores cambiados (I420 o NV12, ver distribuir); y uno de tamaño impar.
//  3. MOV de un iPhone (HEVC: necesita la extensión HEVC de la tienda; sin ella, que
//     caiga a lo de siempre sin colgarse) y un vídeo vertical (rotación: no se aplica).
//  4. El sonido a su hora con el vídeo, en estéreo 44,1 kHz y en 5.1: si el sistema no
//     remuestrea a 48 kHz mono, lo hace remuestreo.
//  5. Buscar hacia delante y hacia atrás en un vídeo largo: que sea rápido y la imagen
//     no salte a un fotograma clave anterior.
//  6. Un MKV, un AVI y un WMV; un vídeo sin sonido y un audio sin vídeo.
//  7. Windows N sin el paquete multimedia: Disponible da false y Cast dice lo de ffmpeg.
//  8. Abrir y cerrar muchos vídeos seguidos (la galería de Photos): ni hilos ni memoria
//     que crezcan (cada Lector tiene su hilo y su MFStartup/MFShutdown).
//  9. El póster sin ffmpeg (Cast y Photos): primero sale de aquí; si no, la miniatura
//     del Explorador (`local/videoteca`), con sus colores bien (BGRA) y sin transparencia.
package mediafoundation

import (
	"encoding/binary"
	"errors"
	"image"
	"math"
	"time"
)

// Muestreo es el del PCM que se entrega: mono a 48 kHz, el de la tarjeta del
// Reproductor.
const Muestreo = 48000

// Muestra es un fotograma (Imagen, en I420) o un trozo de audio (PCM mono a 48 kHz),
// con su instante. Es `video.Muestra`.
type Muestra struct {
	Tiempo time.Duration
	Imagen image.Image
	PCM    []int16
}

// Info es lo que se sabe del fichero al abrirlo. Es `video.InfoSistema`.
type Info struct {
	Duracion    time.Duration
	Ancho, Alto int
	ConVideo    bool
	ConAudio    bool
}

// ErrNoDisponible: en este sistema no hay Media Foundation.
var ErrNoDisponible = errors.New("mediafoundation: no está en este sistema")

// ---- Instantes ----

// deCien pasa un instante de Media Foundation (en unidades de 100 ns) a Duration.
func deCien(t int64) time.Duration {
	if t > math.MaxInt64/100 {
		return time.Duration(math.MaxInt64)
	}
	if t < math.MinInt64/100 {
		return time.Duration(math.MinInt64)
	}
	return time.Duration(t * 100)
}

// aCien pasa una Duration a unidades de 100 ns.
func aCien(d time.Duration) int64 { return int64(d / 100) }

// instanteImagen es el de un fotograma: nunca antes de cero (algunos MP4 empiezan un
// poco antes por su lista de edición, y el primero tiene que poder enseñarse).
func instanteImagen(t int64) time.Duration { return max(deCien(t), 0) }

// pasado dice si una muestra que empieza en t y dura dur acaba antes del objetivo de
// la última búsqueda: tras SetCurrentPosition el lector empieza en el fotograma clave
// anterior, y lo que queda antes del punto no se enseña ni suena (el Reproductor lo
// descarta), así que ni se convierte. Sin duración conocida no se da por pasada.
func pasado(t, dur, objetivo time.Duration) bool {
	return objetivo > 0 && dur > 0 && t+dur <= objetivo
}

// ---- Qué flujo va antes ----

// flujo es uno de los dos que se leen.
type flujo int

const (
	ninguno flujo = iota
	deVideo
	deAudio
)

// pendiente es lo que hay leído y sin entregar de un flujo.
type pendiente struct {
	hay bool
	t   time.Duration
}

// elegir dice qué muestra se entrega ahora: la que va antes en el tiempo; a la par, la
// imagen (como el GStreamer de Linux). ninguno es que no queda nada.
func elegir(v, a pendiente) flujo {
	switch {
	case v.hay && (!a.hay || v.t <= a.t):
		return deVideo
	case a.hay:
		return deAudio
	}
	return ninguno
}

// ---- La imagen ----

// formatoImagen es cómo vienen los planos de un fotograma.
type formatoImagen int

const (
	// I420 (o IYUV, que es lo mismo): Y, luego U y luego V, cada croma a la mitad.
	formatoI420 formatoImagen = iota + 1
	// NV12: Y, luego U y V intercalados en un solo plano a la mitad de alto.
	formatoNV12
)

// planos es dónde está cada cosa de un fotograma en el búfer contiguo que da Media
// Foundation: el paso (bytes por fila de luma) y las filas del búfer pueden ser más que
// lo que se ve (un H.264 de 1080 filas se descodifica en 1088), y lo que se ve es un
// rectángulo (la apertura) que empieza en (x, y).
type planos struct {
	formato     formatoImagen
	paso, filas int
	x, y        int
	ancho, alto int
}

// tamano es cuántos bytes ocupa un fotograma de paso × filas en el formato.
func tamano(f formatoImagen, paso, filas int) int {
	cf := (filas + 1) / 2
	if f == formatoNV12 {
		return paso*filas + paso*cf
	}
	return paso*filas + 2*((paso+1)/2)*cf
}

// medida es lo que dice el tipo de salida de un flujo de vídeo.
type medida struct {
	formato formatoImagen
	// anchoCod × altoCod es MF_MT_FRAME_SIZE: el tamaño del búfer descodificado.
	anchoCod, altoCod int
	// paso es MF_MT_DEFAULT_STRIDE (0 si no lo dice).
	paso int
	// La apertura que se ve (MF_MT_MINIMUM_DISPLAY_APERTURE), o todo si no la hay.
	x, y, ancho, alto int
}

// visible es el tamaño que se enseña.
func (m medida) visible() (int, int) {
	if m.ancho > 0 && m.alto > 0 {
		return m.ancho, m.alto
	}
	return m.anchoCod, m.altoCod
}

// errPlanos: el búfer no casa con ningún reparto de planos razonable.
var errPlanos = errors.New("mediafoundation: el fotograma no tiene el tamaño que dice su tipo")

// distribuir encuentra los planos de un fotograma en un búfer de `largo` bytes. Prueba
// primero lo que dice el tipo (paso y filas), luego lo mínimo (el ancho y las filas que
// se ven) y los pasos y filas alineados (a 16, 32 y 64) que usan los descodificadores;
// vale el primero que llena el búfer justo. Si ninguno lo llena justo, vale el que dice el tipo
// mientras quepa. Nunca da un reparto que se salga del búfer.
func distribuir(largo int, m medida) (planos, error) {
	ancho, alto := m.visible()
	if ancho <= 0 || alto <= 0 || ancho > 16384 || alto > 16384 || largo <= 0 {
		return planos{}, errPlanos
	}
	x, y := 0, 0
	if m.ancho > 0 && m.alto > 0 {
		// La apertura empieza en una posición par (el croma va de dos en dos).
		x, y = m.x&^1, m.y&^1
	}
	minAncho, minAlto := x+ancho, y+alto
	pasos := []int{abs(m.paso), m.anchoCod, minAncho, alinear(minAncho, 16), alinear(minAncho, 32), alinear(minAncho, 64)}
	filas := []int{m.altoCod, minAlto, alinear(minAlto, 16), alinear(minAlto, 32)}
	con := func(p, f int) planos {
		return planos{formato: m.formato, paso: p, filas: f, x: x, y: y, ancho: ancho, alto: alto}
	}
	// Las filas por fuera: las que dice el tipo son más de fiar que su paso (un
	// descodificador que alinea el paso y no lo dice es lo más común).
	for _, f := range filas {
		for _, p := range pasos {
			if pl := con(p, f); pl.cabe() && tamano(m.formato, p, f) == largo {
				return pl, nil
			}
		}
	}
	// Sobra búfer (relleno al final): lo que dice el tipo, si cabe.
	p := abs(m.paso)
	if p == 0 {
		p = max(m.anchoCod, minAncho)
	}
	if pl := con(p, max(m.altoCod, minAlto)); pl.cabe() && tamano(m.formato, pl.paso, pl.filas) <= largo {
		return pl, nil
	}
	return planos{}, errPlanos
}

// cabe dice si lo que se ve (y su croma) está dentro de las filas y el paso.
func (pl planos) cabe() bool {
	if pl.paso <= 0 || pl.filas <= 0 || pl.ancho <= 0 || pl.alto <= 0 || pl.x < 0 || pl.y < 0 ||
		pl.x+pl.ancho > pl.paso || pl.y+pl.alto > pl.filas {
		return false
	}
	cw := (pl.ancho + 1) / 2
	switch pl.formato {
	case formatoNV12:
		return pl.x+2*cw <= pl.paso
	case formatoI420:
		return pl.x/2+cw <= (pl.paso+1)/2
	}
	return false
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

func alinear(n, a int) int { return (n + a - 1) / a * a }

// aI420 copia el fotograma del búfer a una imagen I420 nueva (de Go: el búfer de Media
// Foundation se suelta al volver). NV12 se separa en U y V aquí.
func aI420(datos []byte, pl planos) (*image.YCbCr, error) {
	if !pl.cabe() || len(datos) < tamano(pl.formato, pl.paso, pl.filas) {
		return nil, errPlanos
	}
	img := image.NewYCbCr(image.Rect(0, 0, pl.ancho, pl.alto), image.YCbCrSubsampleRatio420)
	for f := range pl.alto {
		o := (pl.y+f)*pl.paso + pl.x
		copy(img.Y[f*img.YStride:f*img.YStride+pl.ancho], datos[o:o+pl.ancho])
	}
	cw, ch := (pl.ancho+1)/2, (pl.alto+1)/2
	cx, cy := pl.x/2, pl.y/2
	inicioC := pl.paso * pl.filas
	switch pl.formato {
	case formatoNV12:
		for f := range ch {
			o := inicioC + (cy+f)*pl.paso + 2*cx
			cb := img.Cb[f*img.CStride : f*img.CStride+cw]
			cr := img.Cr[f*img.CStride : f*img.CStride+cw]
			uv := datos[o : o+2*cw]
			for i := range cw {
				cb[i], cr[i] = uv[2*i], uv[2*i+1]
			}
		}
	case formatoI420:
		pasoC, filasC := (pl.paso+1)/2, (pl.filas+1)/2
		inicioV := inicioC + pasoC*filasC
		for f := range ch {
			o := (cy+f)*pasoC + cx
			copy(img.Cb[f*img.CStride:f*img.CStride+cw], datos[inicioC+o:inicioC+o+cw])
			copy(img.Cr[f*img.CStride:f*img.CStride+cw], datos[inicioV+o:inicioV+o+cw])
		}
	}
	return img, nil
}

// ---- El sonido ----

// formatoSonido es el PCM que da el lector, que puede no ser el pedido (mono de 16 bits
// a 48 kHz) si el sistema no sabe convertir: entonces se convierte aquí.
type formatoSonido struct {
	flotante bool // float de 32 bits; si no, enteros de `bits`
	bits     int
	canales  int
	muestreo int
}

// valido dice si es un formato que se sabe pasar a mono de 16 bits.
func (f formatoSonido) valido() bool {
	if f.canales < 1 || f.canales > 16 || f.muestreo < 4000 || f.muestreo > 768000 {
		return false
	}
	if f.flotante {
		return f.bits == 32
	}
	return f.bits == 16 || f.bits == 24 || f.bits == 32
}

// aMono pasa el PCM intercalado a mono de 16 bits (la media de los canales). Lo que
// sobre al final (una muestra a medias) se ignora.
func aMono(datos []byte, f formatoSonido) []int16 {
	if !f.valido() {
		return nil
	}
	bm := f.bits / 8
	cuadro := bm * f.canales
	n := len(datos) / cuadro
	out := make([]int16, n)
	for i := range n {
		var suma float64
		for c := range f.canales {
			suma += muestraEn(datos[i*cuadro+c*bm:], f)
		}
		out[i] = aInt16(suma / float64(f.canales))
	}
	return out
}

// muestraEn lee una muestra y la deja entre -1 y 1.
func muestraEn(b []byte, f formatoSonido) float64 {
	switch {
	case f.flotante:
		return float64(math.Float32frombits(binary.LittleEndian.Uint32(b)))
	case f.bits == 16:
		return float64(int16(binary.LittleEndian.Uint16(b))) / 32768 //nolint:gosec // G115: PCM de 16 bits con signo
	case f.bits == 24:
		v := int32(uint32(b[0])<<8|uint32(b[1])<<16|uint32(b[2])<<24) >> 8 //nolint:gosec // G115: 24 bits con signo, extendidos
		return float64(v) / (1 << 23)
	default:
		return float64(int32(binary.LittleEndian.Uint32(b))) / (1 << 31) //nolint:gosec // G115: PCM de 32 bits con signo
	}
}

// aInt16 pasa una muestra entre -1 y 1 a 16 bits (las de 16 bits vuelven tal cual).
func aInt16(v float64) int16 { return redondear16(v * 32768) }

// redondear16 redondea y recorta a lo que cabe en 16 bits.
func redondear16(v float64) int16 {
	return int16(min(max(math.Round(v), -32768), 32767))
}

// remuestreo pasa de `de` Hz a `a` Hz por interpolación lineal, trozo a trozo y sin
// costuras: recuerda la última muestra y por dónde iba. Para la voz y la música de un
// vídeo en el altavoz basta; solo se usa si el sistema no remuestrea él.
type remuestreo struct {
	de, a  int
	pos    float64 // de la siguiente muestra de salida, en muestras de entrada; -1 es `previa`
	previa int16
	hay    bool
}

// reiniciar olvida lo anterior (tras buscar).
func (r *remuestreo) reiniciar() { r.pos, r.previa, r.hay = 0, 0, false }

// pasar remuestrea un trozo. Si los dos muestreos son iguales lo devuelve tal cual.
func (r *remuestreo) pasar(in []int16) []int16 {
	if r.de <= 0 || r.a <= 0 || r.de == r.a || len(in) == 0 {
		return in
	}
	paso := float64(r.de) / float64(r.a)
	out := make([]int16, 0, int(float64(len(in))/paso)+2)
	for {
		i := int(math.Floor(r.pos))
		if i+1 >= len(in) {
			break
		}
		a := r.previa
		if i >= 0 {
			a = in[i]
		}
		b := in[i+1]
		frac := r.pos - float64(i)
		out = append(out, redondear16(float64(a)+(float64(b)-float64(a))*frac))
		r.pos += paso
	}
	r.pos -= float64(len(in))
	r.previa, r.hay = in[len(in)-1], true
	return out
}
