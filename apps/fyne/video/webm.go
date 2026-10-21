// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package video reproduce en Go el WebM VP9/Opus que el transcodificador de Sytue Cast
// deja para las apps nativas (decisión 4 del dueño, ADR 0061): un demuxer de Matroska
// propio y mínimo (este fichero), govpx para el VP9, go-opus para el audio y la
// tarjeta de `llamada` para sacarlo por el altavoz (reproductor.go).
//
// # Por qué un demuxer propio
//
// Del contenedor solo hace falta lo que escribe ffmpeg con `-f webm`: cabecera EBML,
// Segment, Info, Tracks, Cues y Clusters con SimpleBlock o BlockGroup. Una biblioteca
// entera de Matroska son miles de líneas para leer eso, y ninguna de las que hay lee
// POR RANGOS sobre un io.ReadSeeker, que es lo que deja buscar en un vídeo de una hora
// sin bajarlo entero (`api.CastMedia`).
//
// Lo que NO lee: lacing en SimpleBlock de vídeo (nadie lo escribe; el de audio sí se
// lee), cifrado (ContentEncodings), pistas de subtítulos, capítulos ni adjuntos. Todo
// eso se salta por su tamaño.
package video

import (
	"bufio"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"sort"
	"time"
)

// Los identificadores de elemento que se leen (Matroska, RFC 9559), con su marca.
const (
	idEBML          = 0x1A45DFA3
	idDocType       = 0x4282
	idSegment       = 0x18538067
	idSeekHead      = 0x114D9B74
	idSeek          = 0x4DBB
	idSeekID        = 0x53AB
	idSeekPosition  = 0x53AC
	idInfo          = 0x1549A966
	idTimecodeScale = 0x2AD7B1
	idDuration      = 0x4489
	idTracks        = 0x1654AE6B
	idTrackEntry    = 0xAE
	idTrackNumber   = 0xD7
	idTrackType     = 0x83
	idCodecID       = 0x86
	idCodecPrivate  = 0x63A2
	idVideo         = 0xE0
	idPixelWidth    = 0xB0
	idPixelHeight   = 0xBA
	idAudio         = 0xE1
	idSampling      = 0xB5
	idChannels      = 0x9F
	idCues          = 0x1C53BB6B
	idCuePoint      = 0xBB
	idCueTime       = 0xB3
	idCueTrackPos   = 0xB7
	idCueTrack      = 0xF7
	idCueClusterPos = 0xF1
	idCluster       = 0x1F43B675
	idTimecode      = 0xE7
	idSimpleBlock   = 0xA3
	idBlockGroup    = 0xA0
	idBlock         = 0xA1
	idReference     = 0xFB
)

// Los dos tipos de pista que se reproducen.
const (
	TipoVideo = 1
	TipoAudio = 2
)

// Los códecs que se descodifican.
const (
	CodecVP9  = "V_VP9"
	CodecOpus = "A_OPUS"
)

// tamanoDesconocido es el tamaño «todo unos» de un elemento vivo (un directo grabado
// sin cerrar): se lee hasta que empieza el siguiente del mismo nivel.
const tamanoDesconocido = -1

// maxElemento acota lo que se lee en memoria de un solo elemento: un bloque de vídeo de
// 720p es de decenas de KB; más de 32 MiB es un fichero roto o malicioso.
const maxElemento = 32 << 20

// ErrNoEsWebM es un fichero que no empieza por la cabecera EBML de un WebM/Matroska.
var ErrNoEsWebM = errors.New("video: no es un WebM")

// Pista es una pista del fichero.
type Pista struct {
	Numero uint64
	Tipo   int
	Codec  string
	// Privado es el CodecPrivate: en Opus, la cabecera OpusHead (canales, pre-skip).
	Privado []byte
	// Ancho y Alto, en vídeo.
	Ancho, Alto int
	// Muestreo y Canales, en audio.
	Muestreo float64
	Canales  int
}

// Paquete es un fotograma comprimido de una pista, con su instante.
type Paquete struct {
	Pista  uint64
	Tiempo time.Duration
	Clave  bool
	Datos  []byte
}

// puntoCue es un punto de búsqueda: en `tiempo` empieza un Cluster en `pos` (relativa al
// principio de los datos del Segment).
type puntoCue struct {
	tiempo time.Duration
	pista  uint64
	pos    int64
}

// WebM es un fichero abierto. No es seguro para varias rutinas.
type WebM struct {
	r *lector
	// Pistas son todas las del fichero.
	Pistas []Pista
	// Duracion es la que dice Info; cero si no la dice (un directo sin cerrar).
	Duracion time.Duration

	escala        uint64 // TimecodeScale: nanosegundos por unidad
	inicioSegment int64  // dónde empiezan los datos del Segment
	primerCluster int64
	cues          []puntoCue

	tiempoCluster uint64
	pendientes    []Paquete
}

// Abrir lee las cabeceras hasta el primer Cluster: pistas, duración y puntos de
// búsqueda. Si los Cues están al final (ffmpeg sin `-cues_to_front`), salta a leerlos
// por el SeekHead y vuelve.
func Abrir(r io.ReadSeeker) (*WebM, error) {
	w := &WebM{r: nuevoLector(r), escala: 1_000_000}
	id, tam, err := w.r.cabecera()
	if err != nil || id != idEBML {
		return nil, ErrNoEsWebM
	}
	if err := w.leerEBML(tam); err != nil {
		return nil, err
	}
	id, tam, err = w.r.cabecera()
	if err != nil {
		return nil, fmt.Errorf("video: sin Segment: %w", err)
	}
	if id != idSegment {
		return nil, fmt.Errorf("video: se esperaba el Segment y hay %#x", id)
	}
	_ = tam
	w.inicioSegment = w.r.pos
	var posCues int64 = -1
	for {
		inicio := w.r.pos
		id, tam, err := w.r.cabecera()
		if err != nil {
			return nil, fmt.Errorf("video: el fichero acaba antes del primer Cluster: %w", err)
		}
		switch id {
		case idInfo:
			err = w.leerInfo(tam)
		case idTracks:
			err = w.leerTracks(tam)
		case idCues:
			err = w.leerCues(tam)
		case idSeekHead:
			var p int64
			p, err = w.leerSeekHead(tam)
			if p >= 0 {
				posCues = p
			}
		case idCluster:
			w.primerCluster = inicio
			if len(w.cues) == 0 && posCues >= 0 {
				w.cuesDelFinal(posCues)
			}
			if err := w.r.ir(inicio); err != nil {
				return nil, err
			}
			if len(w.Pistas) == 0 {
				return nil, errors.New("video: el fichero no declara pistas")
			}
			return w, nil
		default:
			err = w.r.saltar(tam)
		}
		if err != nil {
			return nil, err
		}
	}
}

// cuesDelFinal lee los Cues que ffmpeg deja detrás de los Clusters. Si no se puede
// (un fichero cortado), se busca a lo lento: desde el primer Cluster.
func (w *WebM) cuesDelFinal(pos int64) {
	if err := w.r.ir(w.inicioSegment + pos); err != nil {
		return
	}
	id, tam, err := w.r.cabecera()
	if err != nil || id != idCues {
		return
	}
	_ = w.leerCues(tam)
}

// Pista devuelve la primera pista de ese tipo, o nil.
func (w *WebM) Pista(tipo int) *Pista {
	for i := range w.Pistas {
		if w.Pistas[i].Tipo == tipo {
			return &w.Pistas[i]
		}
	}
	return nil
}

// PuntosDeBusqueda son los instantes que dicen los Cues, en orden.
func (w *WebM) PuntosDeBusqueda() []time.Duration {
	out := make([]time.Duration, len(w.cues))
	for i, c := range w.cues {
		out[i] = c.tiempo
	}
	return out
}

// Siguiente devuelve el paquete siguiente en el orden del fichero, que es el de
// presentación salvo las cosas del B-frame que VP9 no tiene. io.EOF al acabar.
func (w *WebM) Siguiente() (Paquete, error) {
	for len(w.pendientes) == 0 {
		id, tam, err := w.r.cabecera()
		if err != nil {
			return Paquete{}, err
		}
		switch id {
		case idCluster:
			// Se entra: sus hijos son lo que se lee a continuación.
			w.tiempoCluster = 0
		case idTimecode:
			if w.tiempoCluster, err = w.r.uint(tam); err != nil {
				return Paquete{}, err
			}
		case idSimpleBlock:
			datos, err := w.r.bytes(tam)
			if err != nil {
				return Paquete{}, err
			}
			if err := w.bloque(datos, true, false); err != nil {
				return Paquete{}, err
			}
		case idBlockGroup:
			if err := w.grupo(tam); err != nil {
				return Paquete{}, err
			}
		default:
			// Void, CRC-32, Cues o Tags detrás de los Clusters, un Segment encadenado…
			if tam == tamanoDesconocido {
				return Paquete{}, fmt.Errorf("video: elemento %#x de tamaño desconocido", id)
			}
			if err := w.r.saltar(tam); err != nil {
				return Paquete{}, err
			}
		}
	}
	p := w.pendientes[0]
	w.pendientes = w.pendientes[1:]
	return p, nil
}

// grupo lee un BlockGroup: el Block y, si trae ReferenceBlock, que NO es clave.
func (w *WebM) grupo(tam int64) error {
	fin := w.r.pos + tam
	var datos []byte
	referencia := false
	for w.r.pos < fin {
		id, t, err := w.r.cabecera()
		if err != nil {
			return err
		}
		switch id {
		case idBlock:
			if datos, err = w.r.bytes(t); err != nil {
				return err
			}
		case idReference:
			referencia = true
			err = w.r.saltar(t)
		default:
			err = w.r.saltar(t)
		}
		if err != nil {
			return err
		}
	}
	if datos == nil {
		return nil
	}
	return w.bloque(datos, false, !referencia)
}

// bloque parte un (Simple)Block en sus fotogramas: número de pista, tiempo relativo al
// Cluster, banderas y, con lacing, varios fotogramas seguidos.
func (w *WebM) bloque(b []byte, simple, claveDeGrupo bool) error {
	pista, n := vint(b, false)
	if n == 0 || len(b) < n+3 {
		return errors.New("video: bloque corto")
	}
	rel := int16(binary.BigEndian.Uint16(b[n:])) //nolint:gosec // G115: el tiempo relativo es con signo en el formato
	banderas := b[n+2]
	resto := b[n+3:]
	clave := claveDeGrupo
	if simple {
		clave = banderas&0x80 != 0
	}
	base := int64(w.tiempoCluster) + int64(rel)     //nolint:gosec // G115: un Timecode cabe de sobra en int64
	tiempo := time.Duration(base * int64(w.escala)) //nolint:gosec // G115: TimecodeScale es de nanosegundos
	marcos, err := deslazar(resto, (banderas>>1)&0x03)
	if err != nil {
		return err
	}
	for _, m := range marcos {
		w.pendientes = append(w.pendientes, Paquete{Pista: pista, Tiempo: tiempo, Clave: clave, Datos: m})
	}
	return nil
}

// deslazar separa los fotogramas de un bloque con lacing: 0 ninguno, 1 Xiph, 2 fijo, 3
// EBML. Los tiempos de los que no son el primero no vienen en el bloque; se dejan en el
// del bloque, que es lo que hace falta para el audio (va seguido).
func deslazar(b []byte, lacing byte) ([][]byte, error) {
	if lacing == 0 {
		return [][]byte{b}, nil
	}
	if len(b) < 1 {
		return nil, errors.New("video: lacing sin recuento")
	}
	cuantos := int(b[0]) + 1
	b = b[1:]
	var tamanos []int
	var err error
	switch lacing {
	case 1:
		tamanos, b, err = tamanosXiph(b, cuantos)
	case 3:
		tamanos, b, err = tamanosEBML(b, cuantos)
	default: // 2, fijo: todos iguales
		if len(b)%cuantos != 0 {
			return nil, errors.New("video: lacing fijo que no reparte")
		}
		tamanos = make([]int, cuantos)
		for i := range tamanos {
			tamanos[i] = len(b) / cuantos
		}
	}
	if err != nil {
		return nil, err
	}
	out := make([][]byte, cuantos)
	for i, t := range tamanos {
		out[i], b = b[:t], b[t:]
	}
	return out, nil
}

// tamanosXiph: cada tamaño es una suma de bytes 255 terminada en uno menor; el último
// es lo que sobra.
func tamanosXiph(b []byte, cuantos int) ([]int, []byte, error) {
	tamanos := make([]int, cuantos)
	for i := range cuantos - 1 {
		for {
			if len(b) == 0 {
				return nil, nil, errors.New("video: lacing Xiph cortado")
			}
			v := int(b[0])
			b = b[1:]
			tamanos[i] += v
			if v != 255 {
				break
			}
		}
	}
	return cerrarTamanos(tamanos, b)
}

// tamanosEBML: el primero como vint y los demás como diferencia con signo con el
// anterior, desplazada a la mitad del rango de su longitud.
func tamanosEBML(b []byte, cuantos int) ([]int, []byte, error) {
	tamanos := make([]int, cuantos)
	for i := range cuantos - 1 {
		crudo, n := vint(b, false)
		if n == 0 {
			return nil, nil, errors.New("video: lacing EBML cortado")
		}
		b = b[n:]
		if i == 0 {
			tamanos[0] = int(crudo) //nolint:gosec // G115: acotado por el tamaño del bloque en cerrarTamanos
			continue
		}
		dif := int64(crudo) - (int64(1)<<(7*n-1) - 1) //nolint:gosec // G115: n ≤ 8
		tamanos[i] = tamanos[i-1] + int(dif)
	}
	return cerrarTamanos(tamanos, b)
}

// cerrarTamanos comprueba que los tamaños caben y pone el del último, que es el resto.
func cerrarTamanos(tamanos []int, b []byte) ([]int, []byte, error) {
	suma := 0
	for _, t := range tamanos[:len(tamanos)-1] {
		if t < 0 {
			return nil, nil, errors.New("video: tamaño de lacing negativo")
		}
		suma += t
	}
	if suma > len(b) {
		return nil, nil, errors.New("video: lacing más largo que el bloque")
	}
	tamanos[len(tamanos)-1] = len(b) - suma
	return tamanos, b, nil
}

// Buscar deja el fichero en el último punto de búsqueda que no pasa de `t` (un Cluster
// que empieza en un fotograma clave de vídeo) y devuelve su instante. Lo que quede
// entre ese punto y `t` lo descarta quien reproduce. Sin Cues, vuelve al principio.
func (w *WebM) Buscar(t time.Duration) (time.Duration, error) {
	w.pendientes = nil
	w.tiempoCluster = 0
	video := w.Pista(TipoVideo)
	mejor := -1
	for i, c := range w.cues {
		if video != nil && c.pista != video.Numero {
			continue
		}
		if c.tiempo <= t {
			mejor = i
		}
	}
	if mejor < 0 {
		return 0, w.r.ir(w.primerCluster)
	}
	return w.cues[mejor].tiempo, w.r.ir(w.inicioSegment + w.cues[mejor].pos)
}

// ---- Las secciones de cabecera ----

func (w *WebM) leerEBML(tam int64) error {
	fin := w.r.pos + tam
	for w.r.pos < fin {
		id, t, err := w.r.cabecera()
		if err != nil {
			return err
		}
		if id == idDocType {
			tipo, err := w.r.bytes(t)
			if err != nil {
				return err
			}
			if s := string(tipo); s != "webm" && s != "matroska" {
				return fmt.Errorf("%w: DocType %q", ErrNoEsWebM, s)
			}
			continue
		}
		if err := w.r.saltar(t); err != nil {
			return err
		}
	}
	return nil
}

func (w *WebM) leerInfo(tam int64) error {
	var duracion float64
	err := w.r.hijos(tam, func(id uint32, t int64) error {
		var err error
		switch id {
		case idTimecodeScale:
			w.escala, err = w.r.uint(t)
		case idDuration:
			duracion, err = w.r.float(t)
		default:
			err = w.r.saltar(t)
		}
		return err
	})
	if w.escala == 0 {
		w.escala = 1_000_000
	}
	w.Duracion = time.Duration(duracion * float64(w.escala))
	return err
}

func (w *WebM) leerTracks(tam int64) error {
	return w.r.hijos(tam, func(id uint32, t int64) error {
		if id != idTrackEntry {
			return w.r.saltar(t)
		}
		var p Pista
		err := w.r.hijos(t, func(id uint32, t int64) error {
			var err error
			switch id {
			case idTrackNumber:
				p.Numero, err = w.r.uint(t)
			case idTrackType:
				var v uint64
				v, err = w.r.uint(t)
				p.Tipo = int(v) //nolint:gosec // G115: el tipo de pista es de un byte
			case idCodecID:
				var b []byte
				b, err = w.r.bytes(t)
				p.Codec = string(b)
			case idCodecPrivate:
				p.Privado, err = w.r.bytes(t)
			case idVideo:
				err = w.r.hijos(t, func(id uint32, t int64) error {
					var v uint64
					var err error
					switch id {
					case idPixelWidth:
						v, err = w.r.uint(t)
						p.Ancho = int(v) //nolint:gosec // G115: dimensiones de un fotograma
					case idPixelHeight:
						v, err = w.r.uint(t)
						p.Alto = int(v) //nolint:gosec // G115: dimensiones de un fotograma
					default:
						err = w.r.saltar(t)
					}
					return err
				})
			case idAudio:
				err = w.r.hijos(t, func(id uint32, t int64) error {
					var err error
					switch id {
					case idSampling:
						p.Muestreo, err = w.r.float(t)
					case idChannels:
						var v uint64
						v, err = w.r.uint(t)
						p.Canales = int(v) //nolint:gosec // G115: canales de audio
					default:
						err = w.r.saltar(t)
					}
					return err
				})
			default:
				err = w.r.saltar(t)
			}
			return err
		})
		if err != nil {
			return err
		}
		w.Pistas = append(w.Pistas, p)
		return nil
	})
}

func (w *WebM) leerCues(tam int64) error {
	err := w.r.hijos(tam, func(id uint32, t int64) error {
		if id != idCuePoint {
			return w.r.saltar(t)
		}
		var tiempo uint64
		var posiciones []puntoCue
		err := w.r.hijos(t, func(id uint32, t int64) error {
			switch id {
			case idCueTime:
				var err error
				tiempo, err = w.r.uint(t)
				return err
			case idCueTrackPos:
				var c puntoCue
				err := w.r.hijos(t, func(id uint32, t int64) error {
					var err error
					var v uint64
					switch id {
					case idCueTrack:
						c.pista, err = w.r.uint(t)
					case idCueClusterPos:
						v, err = w.r.uint(t)
						c.pos = int64(v) //nolint:gosec // G115: una posición dentro del fichero
					default:
						err = w.r.saltar(t)
					}
					return err
				})
				posiciones = append(posiciones, c)
				return err
			}
			return w.r.saltar(t)
		})
		for _, c := range posiciones {
			c.tiempo = time.Duration(tiempo * w.escala) //nolint:gosec // G115: nanosegundos de un vídeo
			w.cues = append(w.cues, c)
		}
		return err
	})
	sort.SliceStable(w.cues, func(i, j int) bool { return w.cues[i].tiempo < w.cues[j].tiempo })
	return err
}

// leerSeekHead devuelve dónde están los Cues según el índice, o -1.
func (w *WebM) leerSeekHead(tam int64) (int64, error) {
	var pos int64 = -1
	err := w.r.hijos(tam, func(id uint32, t int64) error {
		if id != idSeek {
			return w.r.saltar(t)
		}
		var cual uint64
		var donde uint64
		err := w.r.hijos(t, func(id uint32, t int64) error {
			var err error
			switch id {
			case idSeekID:
				var b []byte
				b, err = w.r.bytes(t)
				for _, x := range b {
					cual = cual<<8 | uint64(x)
				}
			case idSeekPosition:
				donde, err = w.r.uint(t)
			default:
				err = w.r.saltar(t)
			}
			return err
		})
		if cual == idCues {
			pos = int64(donde) //nolint:gosec // G115: una posición dentro del fichero
		}
		return err
	})
	return pos, err
}

// ---- El lector de EBML ----

// lector lee elementos EBML llevando la cuenta de la posición, con un búfer que se tira
// al saltar (la fuente es por rangos y un salto es otra petición).
type lector struct {
	src io.ReadSeeker
	b   *bufio.Reader
	pos int64
}

func nuevoLector(r io.ReadSeeker) *lector {
	return &lector{src: r, b: bufio.NewReaderSize(r, 64<<10)}
}

// ir salta a una posición absoluta.
func (l *lector) ir(pos int64) error {
	if pos == l.pos {
		return nil
	}
	if _, err := l.src.Seek(pos, io.SeekStart); err != nil {
		return err
	}
	l.b.Reset(l.src)
	l.pos = pos
	return nil
}

// cabecera lee el ID y el tamaño del siguiente elemento.
func (l *lector) cabecera() (uint32, int64, error) {
	id, err := l.vintCrudo(4, true)
	if err != nil {
		return 0, 0, err
	}
	tam, err := l.vintCrudo(8, false)
	if err != nil {
		return 0, 0, noEOF(err)
	}
	return uint32(id), tam, nil //nolint:gosec // G115: un ID de EBML son 4 bytes como mucho
}

// vintCrudo lee un entero de longitud variable: la marca de longitud se queda en el ID
// (`marca`) y se quita en los tamaños; un tamaño todo unos es «desconocido».
func (l *lector) vintCrudo(maxLargo int, marca bool) (int64, error) {
	primero, err := l.b.ReadByte()
	if err != nil {
		return 0, err
	}
	l.pos++
	largo := 1
	for m := byte(0x80); largo <= 8 && primero&m == 0; m >>= 1 {
		largo++
	}
	if largo > maxLargo {
		return 0, fmt.Errorf("video: entero de EBML de %d bytes en %d", largo, l.pos-1)
	}
	v := uint64(primero)
	if !marca {
		v &= uint64(0xFF >> largo)
	}
	todoUnos := v == uint64(0xFF>>largo)
	for range largo - 1 {
		c, err := l.b.ReadByte()
		if err != nil {
			return 0, noEOF(err)
		}
		l.pos++
		v = v<<8 | uint64(c)
		todoUnos = todoUnos && c == 0xFF
	}
	if !marca && todoUnos {
		return tamanoDesconocido, nil
	}
	if v > math.MaxInt64 {
		return 0, errors.New("video: entero de EBML fuera de rango")
	}
	return int64(v), nil
}

// hijos recorre los hijos de un elemento maestro de tamaño conocido.
func (l *lector) hijos(tam int64, f func(id uint32, t int64) error) error {
	if tam < 0 {
		return errors.New("video: cabecera de tamaño desconocido")
	}
	fin := l.pos + tam
	for l.pos < fin {
		id, t, err := l.cabecera()
		if err != nil {
			return noEOF(err)
		}
		if t < 0 || l.pos+t > fin {
			return fmt.Errorf("video: el elemento %#x se sale de su padre", id)
		}
		if err := f(id, t); err != nil {
			return err
		}
	}
	return nil
}

func (l *lector) bytes(tam int64) ([]byte, error) {
	if tam < 0 || tam > maxElemento {
		return nil, fmt.Errorf("video: elemento de %d bytes", tam)
	}
	b := make([]byte, tam)
	n, err := io.ReadFull(l.b, b)
	l.pos += int64(n)
	return b, noEOF(err)
}

func (l *lector) saltar(tam int64) error {
	if tam < 0 {
		return errors.New("video: no se puede saltar un elemento de tamaño desconocido")
	}
	if tam > int64(l.b.Buffered()) {
		return l.ir(l.pos + tam)
	}
	n, err := l.b.Discard(int(tam))
	l.pos += int64(n)
	return noEOF(err)
}

func (l *lector) uint(tam int64) (uint64, error) {
	if tam > 8 {
		return 0, errors.New("video: entero de más de 8 bytes")
	}
	b, err := l.bytes(tam)
	var v uint64
	for _, x := range b {
		v = v<<8 | uint64(x)
	}
	return v, err
}

func (l *lector) float(tam int64) (float64, error) {
	b, err := l.bytes(tam)
	if err != nil {
		return 0, err
	}
	switch len(b) {
	case 4:
		return float64(math.Float32frombits(binary.BigEndian.Uint32(b))), nil
	case 8:
		return math.Float64frombits(binary.BigEndian.Uint64(b)), nil
	case 0:
		return 0, nil
	}
	return 0, errors.New("video: coma flotante de tamaño raro")
}

// vint lee un entero de longitud variable de un trozo en memoria (el número de pista de
// un bloque, los tamaños del lacing EBML). Devuelve 0 bytes leídos si no cabe.
func vint(b []byte, marca bool) (uint64, int) {
	if len(b) == 0 {
		return 0, 0
	}
	largo := 1
	for m := byte(0x80); largo <= 8 && b[0]&m == 0; m >>= 1 {
		largo++
	}
	if largo > 8 || len(b) < largo {
		return 0, 0
	}
	v := uint64(b[0])
	if !marca {
		v &= uint64(0xFF >> largo)
	}
	for _, c := range b[1:largo] {
		v = v<<8 | uint64(c)
	}
	return v, largo
}

// noEOF: un EOF a media estructura es un fichero cortado, no el final.
func noEOF(err error) error {
	if errors.Is(err, io.EOF) {
		return io.ErrUnexpectedEOF
	}
	return err
}
