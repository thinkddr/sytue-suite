// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package conversor convierte y sondea vídeos de Sytue Cast Office con el `ffmpeg` que la
// persona YA tenga instalado en su equipo (apps/fyne/PLAN-EDICION-LOCAL.md §6.10, D-27).
//
// # Por qué un ejecutable de fuera
//
// La suite no empaqueta ffmpeg: su licencia (GPL o LGPL) queda fuera de lo que admite
// `docs/CONVENTIONS.md` §12. Tampoco se enlaza: aquí solo se BUSCA el ejecutable (en el
// PATH o en la ruta que la persona haya puesto en la app) y se lanza como un proceso
// aparte, igual que se abriría con el reproductor del sistema. Sin ffmpeg, la app lo dice
// y ofrece el reproductor del sistema; en el teléfono no hay procesos que lanzar.
//
// El paquete es Go puro y no sabe nada de la interfaz: arma los argumentos, lee el
// progreso de `-progress pipe:1` y el JSON de `ffprobe`, y escribe a un temporal que solo
// se renombra al destino cuando ffmpeg ha terminado bien (R-NO-CORROMPE). Nunca escribe
// encima del fichero de entrada.
package conversor

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ErrSinFFmpeg: no hay un ffmpeg que lanzar (no está instalado, la ruta no vale o es un
// teléfono, donde no se lanzan procesos).
var ErrSinFFmpeg = errors.New("conversor: ffmpeg no está instalado en este equipo")

// ErrMismoFichero: la salida es la misma que la entrada; nunca se escribe encima.
var ErrMismoFichero = errors.New("conversor: la salida no puede ser el mismo fichero que la entrada")

// Herramientas son los ejecutables encontrados. FFprobe puede faltar (algunas
// instalaciones solo traen ffmpeg): entonces no se detectan códecs.
type Herramientas struct {
	FFmpeg  string
	FFprobe string
	// Version es la primera línea de `ffmpeg -version` («ffmpeg version 6.1.1 …»).
	Version string
}

// buscarEnPath es exec.LookPath; es una variable para las pruebas.
var buscarEnPath = exec.LookPath

// Buscar encuentra ffmpeg: primero en `configurada` (la ruta del ejecutable o la carpeta
// que lo contiene, como la ponga la persona), si no en el PATH. ffprobe se busca al lado
// de ffmpeg y, si no, en el PATH.
func Buscar(configurada string) (Herramientas, error) {
	if runtime.GOOS == "android" || runtime.GOOS == "ios" {
		return Herramientas{}, ErrSinFFmpeg
	}
	h := Herramientas{}
	if c := strings.TrimSpace(configurada); c != "" {
		ff, err := ejecutableEn(c, "ffmpeg")
		if err != nil {
			return Herramientas{}, fmt.Errorf("%w: %s", ErrSinFFmpeg, c)
		}
		h.FFmpeg = ff
	} else {
		ff, err := buscarEnPath(nombreEjecutable("ffmpeg"))
		if err != nil {
			return Herramientas{}, ErrSinFFmpeg
		}
		h.FFmpeg = ff
	}
	if fp, err := ejecutableEn(filepath.Dir(h.FFmpeg), "ffprobe"); err == nil {
		h.FFprobe = fp
	} else if fp, err := buscarEnPath(nombreEjecutable("ffprobe")); err == nil {
		h.FFprobe = fp
	}
	return h, nil
}

func nombreEjecutable(base string) string {
	if runtime.GOOS == "windows" {
		return base + ".exe"
	}
	return base
}

// ejecutableEn acepta la ruta del ejecutable o la carpeta en la que está.
func ejecutableEn(ruta, base string) (string, error) {
	fi, err := os.Stat(ruta)
	if err != nil {
		return "", err
	}
	if fi.IsDir() {
		ruta = filepath.Join(ruta, nombreEjecutable(base))
		if fi, err = os.Stat(ruta); err != nil {
			return "", err
		}
	}
	if !fi.Mode().IsRegular() || (runtime.GOOS != "windows" && fi.Mode().Perm()&0o111 == 0) {
		return "", fmt.Errorf("conversor: %s no es un ejecutable", ruta)
	}
	return ruta, nil
}

// Hay dice si se encontró ffmpeg.
func (h Herramientas) Hay() bool { return h.FFmpeg != "" }

// LeerVersion pide la versión a ffmpeg y la deja en h.Version. Un ejecutable que no
// responde como ffmpeg es ErrSinFFmpeg.
func (h *Herramientas) LeerVersion(ctx context.Context) error {
	if !h.Hay() {
		return ErrSinFFmpeg
	}
	out, err := comando(ctx, h.FFmpeg, "-hide_banner", "-version").Output()
	if err != nil {
		return fmt.Errorf("%w: %w", ErrSinFFmpeg, err)
	}
	linea, _, _ := strings.Cut(string(out), "\n")
	if !strings.HasPrefix(linea, "ffmpeg") {
		return fmt.Errorf("%w: %s no parece ffmpeg", ErrSinFFmpeg, h.FFmpeg)
	}
	h.Version = strings.TrimSpace(linea)
	return nil
}

// Codificadores son los codificadores que trae este ffmpeg (`-encoders`), por nombre.
// Una compilación mínima puede no traer libvpx o libx264: la app no ofrece entonces el
// formato que no se puede hacer.
func (h Herramientas) Codificadores(ctx context.Context) (map[string]bool, error) {
	if !h.Hay() {
		return nil, ErrSinFFmpeg
	}
	out, err := comando(ctx, h.FFmpeg, "-hide_banner", "-encoders").Output()
	if err != nil {
		return nil, err
	}
	return LeerCodificadores(out), nil
}

// LeerCodificadores lee la tabla de `ffmpeg -encoders`: « V....D libx264  …».
func LeerCodificadores(salida []byte) map[string]bool {
	cods := map[string]bool{}
	tabla := false
	for _, linea := range strings.Split(string(salida), "\n") {
		campos := strings.Fields(linea)
		if len(campos) > 0 && strings.HasPrefix(campos[0], "---") {
			tabla = true
			continue
		}
		if !tabla || len(campos) < 2 {
			continue
		}
		if len(campos[0]) == 6 && strings.ContainsAny(campos[0][:1], "VAS") {
			cods[campos[1]] = true
		}
	}
	return cods
}

// ---- Formatos ----

// Formato es un destino de «Convertir a…».
type Formato string

// Los formatos a los que se convierte. Visionado es el WebM que la app sabe reproducir
// (VP9 + Opus, como el que deja el transcodificador de la edición conectada), hecho
// deprisa para la caché; WebM es el mismo con calidad de archivo.
const (
	Visionado Formato = "visionado"
	WebM      Formato = "webm"
	MP4       Formato = "mp4"
	MKV       Formato = "mkv"
	MOV       Formato = "mov"
	GIF       Formato = "gif"
	MP3       Formato = "mp3"
	Opus      Formato = "opus"
)

// Destino describe un formato para la interfaz.
type Destino struct {
	Formato   Formato
	Extension string // con el punto: ".mp4"
	// Descripcion es técnica y no se traduce: «MP4 (H.264 + AAC)».
	Descripcion string
	SoloAudio   bool
	// Necesita son los codificadores de ffmpeg sin los que no se puede.
	Necesita []string
}

// Destinos son los de «Convertir a…», en el orden en que se ofrecen.
func Destinos() []Destino {
	return []Destino{
		{Formato: MP4, Extension: ".mp4", Descripcion: "MP4 (H.264 + AAC)", Necesita: []string{"libx264", "aac"}},
		{Formato: WebM, Extension: ".webm", Descripcion: "WebM (VP9 + Opus)", Necesita: []string{"libvpx-vp9", "libopus"}},
		{Formato: MKV, Extension: ".mkv", Descripcion: "Matroska (MKV, sin recodificar)"},
		{Formato: MOV, Extension: ".mov", Descripcion: "QuickTime (MOV, H.264 + AAC)", Necesita: []string{"libx264", "aac"}},
		{Formato: GIF, Extension: ".gif", Descripcion: "GIF animado (sin sonido)", Necesita: []string{"gif"}},
		{Formato: MP3, Extension: ".mp3", Descripcion: "MP3 (solo el audio)", SoloAudio: true, Necesita: []string{"libmp3lame"}},
		{Formato: Opus, Extension: ".opus", Descripcion: "Opus (solo el audio)", SoloAudio: true, Necesita: []string{"libopus"}},
	}
}

// DestinoDe devuelve el destino de un formato (Visionado incluido).
func DestinoDe(f Formato) (Destino, bool) {
	if f == Visionado {
		return Destino{Formato: Visionado, Extension: ".webm", Descripcion: "WebM (VP9 + Opus)", Necesita: []string{"libvpx-vp9", "libopus"}}, true
	}
	for _, d := range Destinos() {
		if d.Formato == f {
			return d, true
		}
	}
	return Destino{}, false
}

// Posible dice si este ffmpeg puede hacer el destino. cods nil: no se sabe, se da por
// posible.
func (d Destino) Posible(cods map[string]bool) bool {
	if cods == nil {
		return true
	}
	for _, c := range d.Necesita {
		if !cods[c] {
			return false
		}
	}
	return true
}

// Argumentos arma la orden de ffmpeg para convertir `entrada` a `salida` en el formato
// f. Siempre: sin preguntar (`-y` sobre el temporal, que es nuevo), sin leer la entrada
// estándar, con el progreso por la salida estándar y solo los errores por la de error.
// Solo se toman el primer vídeo y el primer audio (`?`: si no hay, no falla); los
// subtítulos y los datos se dejan fuera.
func Argumentos(entrada, salida string, f Formato) ([]string, error) {
	base := []string{"-hide_banner", "-nostdin", "-y", "-loglevel", "error", "-progress", "pipe:1", "-nostats", "-i", entrada}
	var resto []string
	switch f {
	case Visionado:
		// Deprisa (realtime + cpu-used 8): es para verlo ya en la app, no para guardar.
		// 4:2:0 de 8 bits y audio a 48 kHz y como mucho estéreo, que es lo que lee el
		// reproductor; los Cues delante, para buscar sin leer hasta el final.
		resto = []string{
			"-map", "0:v:0?", "-map", "0:a:0?", "-sn", "-dn",
			"-c:v", "libvpx-vp9", "-deadline", "realtime", "-cpu-used", "8", "-row-mt", "1",
			"-b:v", "0", "-crf", "33", "-pix_fmt", "yuv420p",
			"-c:a", "libopus", "-b:a", "128k", "-ar", "48000", "-ac", "2",
			"-cues_to_front", "1", "-f", "webm",
		}
	case WebM:
		resto = []string{
			"-map", "0:v:0?", "-map", "0:a:0?", "-sn", "-dn",
			"-c:v", "libvpx-vp9", "-deadline", "good", "-cpu-used", "4", "-row-mt", "1",
			"-b:v", "0", "-crf", "31", "-pix_fmt", "yuv420p",
			"-c:a", "libopus", "-b:a", "128k", "-ar", "48000",
			"-cues_to_front", "1", "-f", "webm",
		}
	case MP4, MOV:
		muxer := "mp4"
		if f == MOV {
			muxer = "mov"
		}
		resto = []string{
			"-map", "0:v:0?", "-map", "0:a:0?", "-sn", "-dn",
			"-c:v", "libx264", "-preset", "medium", "-crf", "23", "-pix_fmt", "yuv420p",
			"-c:a", "aac", "-b:a", "160k", "-movflags", "+faststart", "-f", muxer,
		}
	case MKV:
		// Matroska admite casi cualquier códec: se cambia el contenedor sin recodificar,
		// que es rápido y no pierde calidad.
		resto = []string{"-map", "0:v?", "-map", "0:a?", "-sn", "-dn", "-c", "copy", "-f", "matroska"}
	case GIF:
		resto = []string{
			"-an", "-sn", "-dn",
			"-vf", "fps=10,scale='min(480,iw)':-2:flags=lanczos,split[a][b];[a]palettegen[p];[b][p]paletteuse",
			"-loop", "0", "-f", "gif",
		}
	case MP3:
		resto = []string{"-vn", "-sn", "-dn", "-map", "0:a:0", "-c:a", "libmp3lame", "-q:a", "2", "-f", "mp3"}
	case Opus:
		resto = []string{"-vn", "-sn", "-dn", "-map", "0:a:0", "-c:a", "libopus", "-b:a", "128k", "-f", "ogg"}
	default:
		return nil, fmt.Errorf("conversor: formato desconocido %q", f)
	}
	return slices.Concat(base, resto, []string{salida}), nil
}

// ArgumentosFotograma saca un fotograma en JPEG (el póster): en `en`, a 640 de ancho como
// mucho.
func ArgumentosFotograma(entrada, salida string, en time.Duration) []string {
	return []string{
		"-hide_banner", "-nostdin", "-y", "-loglevel", "error",
		"-ss", strconv.FormatFloat(en.Seconds(), 'f', 3, 64), "-i", entrada,
		"-frames:v", "1", "-vf", "scale='min(640,iw)':-2", "-q:v", "3", "-f", "image2", "-c:v", "mjpeg", salida,
	}
}

// ---- Progreso ----

// Progreso es lo que lleva una conversión.
type Progreso struct {
	// Hecho es el instante del vídeo por el que va la salida.
	Hecho time.Duration
	// Total es la duración de la entrada; cero si no se sabe.
	Total time.Duration
	// Velocidad es la de ffmpeg respecto al tiempo real («2.5x»); vacía si no la da.
	Velocidad string
	// Fin: ffmpeg ha dicho `progress=end`.
	Fin bool
}

// Fraccion es de 0 a 1; sin total conocido, 0 hasta el final.
func (p Progreso) Fraccion() float64 {
	if p.Fin {
		return 1
	}
	if p.Total <= 0 {
		return 0
	}
	return min(max(float64(p.Hecho)/float64(p.Total), 0), 0.999)
}

// LeerProgreso lee los bloques `clave=valor` de `-progress` hasta que se acaba r, y avisa
// a f al final de cada bloque (la línea `progress=continue|end`). `out_time_ms` está en
// MICROsegundos, igual que `out_time_us` (un error histórico de ffmpeg que se mantiene);
// se prefiere `out_time_us` y, si no, `out_time` («00:01:02.500000»).
func LeerProgreso(r io.Reader, total time.Duration, f func(Progreso)) error {
	p := Progreso{Total: total}
	s := bufio.NewScanner(r)
	s.Buffer(make([]byte, 0, 4096), 1<<20)
	tieneUS := false
	for s.Scan() {
		clave, valor, ok := strings.Cut(strings.TrimSpace(s.Text()), "=")
		if !ok {
			continue
		}
		valor = strings.TrimSpace(valor)
		switch clave {
		case "out_time_us", "out_time_ms":
			if us, err := strconv.ParseInt(valor, 10, 64); err == nil && us >= 0 {
				p.Hecho, tieneUS = time.Duration(us)*time.Microsecond, true
			}
		case "out_time":
			if d, ok := LeerInstante(valor); ok && !tieneUS {
				p.Hecho = d
			}
		case "speed":
			if valor != "N/A" {
				p.Velocidad = valor
			}
		case "progress":
			p.Fin = valor == "end"
			if f != nil {
				f(p)
			}
			tieneUS = false
		}
	}
	return s.Err()
}

// LeerInstante lee «HH:MM:SS.micro» (el `out_time` de ffmpeg); acepta signo menos
// delante, que ffmpeg pone al principio de algunos ficheros, y lo deja en cero.
func LeerInstante(s string) (time.Duration, bool) {
	s = strings.TrimSpace(s)
	negativo := strings.HasPrefix(s, "-")
	s = strings.TrimPrefix(s, "-")
	partes := strings.Split(s, ":")
	if len(partes) != 3 {
		return 0, false
	}
	h, err1 := strconv.Atoi(partes[0])
	m, err2 := strconv.Atoi(partes[1])
	seg, err3 := strconv.ParseFloat(partes[2], 64)
	if err1 != nil || err2 != nil || err3 != nil {
		return 0, false
	}
	if negativo {
		return 0, true
	}
	return time.Duration(h)*time.Hour + time.Duration(m)*time.Minute + time.Duration(seg*float64(time.Second)), true
}

// ---- Sondeo ----

// Info es lo que se sabe de un fichero: contenedor, códecs, duración y tamaño de imagen.
type Info struct {
	// Contenedor es el `format_name` de ffprobe («matroska,webm», «mov,mp4,m4a,3gp,…»).
	Contenedor string
	Duracion   time.Duration
	// Video y Audio son los `codec_name` de la primera pista de cada tipo; vacíos si no
	// hay («vp9», «h264», «hevc», «opus», «aac»…).
	Video, Audio string
	Ancho, Alto  int
	// PixFmt es el formato de píxel del vídeo («yuv420p»).
	PixFmt string
}

// Nativo dice si el reproductor propio de la app lo lee tal cual: WebM o Matroska con
// VP9 (4:2:0 de 8 bits) y Opus, o solo uno de los dos.
func (i Info) Nativo() bool {
	if !strings.Contains(i.Contenedor, "matroska") && !strings.Contains(i.Contenedor, "webm") {
		return false
	}
	if i.Video == "" && i.Audio == "" {
		return false
	}
	if i.Video != "" && ((i.Video != "vp9" && i.Video != "vp8") || (i.PixFmt != "" && i.PixFmt != "yuv420p")) {
		return false
	}
	return i.Audio == "" || i.Audio == "opus"
}

// Sondear pide a ffprobe la descripción del fichero.
func (h Herramientas) Sondear(ctx context.Context, ruta string) (Info, error) {
	if h.FFprobe == "" {
		return Info{}, fmt.Errorf("%w (falta ffprobe)", ErrSinFFmpeg)
	}
	var errbuf bytes.Buffer
	cmd := comando(ctx, h.FFprobe, "-v", "error", "-print_format", "json", "-show_format", "-show_streams", ruta)
	cmd.Stderr = &errbuf
	out, err := cmd.Output()
	if err != nil {
		return Info{}, fmt.Errorf("conversor: ffprobe no puede leer %s: %s", filepath.Base(ruta), primeraLinea(errbuf.String(), err))
	}
	return LeerSondeo(out)
}

// LeerSondeo lee el JSON de `ffprobe -print_format json -show_format -show_streams`.
func LeerSondeo(datos []byte) (Info, error) {
	var s struct {
		Streams []struct {
			Tipo   string `json:"codec_type"`
			Codec  string `json:"codec_name"`
			Ancho  int    `json:"width"`
			Alto   int    `json:"height"`
			PixFmt string `json:"pix_fmt"`
			Dur    string `json:"duration"`
			Disp   struct {
				Portada int `json:"attached_pic"`
			} `json:"disposition"`
		} `json:"streams"`
		Format struct {
			Nombre string `json:"format_name"`
			Dur    string `json:"duration"`
		} `json:"format"`
	}
	if err := json.Unmarshal(datos, &s); err != nil {
		return Info{}, fmt.Errorf("conversor: sondeo ilegible: %w", err)
	}
	i := Info{Contenedor: s.Format.Nombre, Duracion: segundos(s.Format.Dur)}
	for _, st := range s.Streams {
		switch {
		case st.Tipo == "video" && i.Video == "" && st.Disp.Portada == 0:
			i.Video, i.Ancho, i.Alto, i.PixFmt = st.Codec, st.Ancho, st.Alto, st.PixFmt
		case st.Tipo == "audio" && i.Audio == "":
			i.Audio = st.Codec
		}
		if i.Duracion == 0 {
			i.Duracion = segundos(st.Dur)
		}
	}
	if i.Contenedor == "" {
		return Info{}, errors.New("conversor: el fichero no es un vídeo que ffprobe entienda")
	}
	return i, nil
}

func segundos(s string) time.Duration {
	f, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil || f < 0 {
		return 0
	}
	return time.Duration(f * float64(time.Second))
}

// ---- Convertir ----

// Convertir pasa `entrada` a `salida` en el formato f y va avisando del progreso
// (`total` es la duración de la entrada, para la fracción; cero si no se sabe). Escribe
// a un temporal al lado de la salida y solo lo renombra cuando ffmpeg acaba bien: una
// conversión cancelada o fallida no deja nada a medias ni toca un fichero que ya
// existiera con ese nombre.
func (h Herramientas) Convertir(ctx context.Context, entrada, salida string, f Formato, total time.Duration, alProgreso func(Progreso)) error {
	if !h.Hay() {
		return ErrSinFFmpeg
	}
	if mismoFichero(entrada, salida) {
		return ErrMismoFichero
	}
	d, ok := DestinoDe(f)
	if !ok {
		return fmt.Errorf("conversor: formato desconocido %q", f)
	}
	if err := os.MkdirAll(filepath.Dir(salida), 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(salida), "."+strings.TrimSuffix(filepath.Base(salida), filepath.Ext(salida))+".*.parcial"+d.Extension)
	if err != nil {
		return err
	}
	temporal := tmp.Name()
	_ = tmp.Close()
	listo := false
	defer func() {
		if !listo {
			_ = os.Remove(temporal)
		}
	}()
	args, err := Argumentos(entrada, temporal, f)
	if err != nil {
		return err
	}
	cmd := comando(ctx, h.FFmpeg, args...)
	errores := &cola{max: 4096}
	cmd.Stderr = errores
	salidaProg, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("conversor: no se pudo lanzar ffmpeg: %w", err)
	}
	_ = LeerProgreso(salidaProg, total, alProgreso)
	if err := cmd.Wait(); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("conversor: ffmpeg no pudo convertir %s: %s", filepath.Base(entrada), primeraLinea(errores.String(), err))
	}
	if fi, err := os.Stat(temporal); err != nil || fi.Size() == 0 {
		return fmt.Errorf("conversor: ffmpeg no escribió nada para %s", filepath.Base(entrada))
	}
	if err := os.Rename(temporal, salida); err != nil {
		return err
	}
	listo = true
	return nil
}

// Fotograma deja en `salida` (JPEG) el fotograma del instante `en`.
func (h Herramientas) Fotograma(ctx context.Context, entrada, salida string, en time.Duration) error {
	if !h.Hay() {
		return ErrSinFFmpeg
	}
	if mismoFichero(entrada, salida) {
		return ErrMismoFichero
	}
	if err := os.MkdirAll(filepath.Dir(salida), 0o700); err != nil {
		return err
	}
	temporal := salida + ".parcial.jpg"
	defer func() { _ = os.Remove(temporal) }()
	errores := &cola{max: 2048}
	cmd := comando(ctx, h.FFmpeg, ArgumentosFotograma(entrada, temporal, en)...)
	cmd.Stderr = errores
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("conversor: no se pudo sacar el fotograma: %s", primeraLinea(errores.String(), err))
	}
	if fi, err := os.Stat(temporal); err != nil || fi.Size() == 0 {
		return errors.New("conversor: el vídeo no tiene fotograma en ese instante")
	}
	return os.Rename(temporal, salida)
}

func mismoFichero(a, b string) bool {
	aa, err1 := filepath.Abs(a)
	bb, err2 := filepath.Abs(b)
	if err1 == nil && err2 == nil && aa == bb {
		return true
	}
	fa, err1 := os.Stat(a)
	fb, err2 := os.Stat(b)
	return err1 == nil && err2 == nil && os.SameFile(fa, fb)
}

// comando es exec.CommandContext; en Windows, sin ventana de consola (comando_windows.go).
var comando = func(ctx context.Context, nombre string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, nombre, args...)
	ocultarVentana(cmd)
	return cmd
}

func primeraLinea(s string, err error) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return err.Error()
	}
	// La última línea con contenido: ffmpeg deja el motivo al final.
	lineas := strings.Split(s, "\n")
	return strings.TrimSpace(lineas[len(lineas)-1])
}

// cola guarda los últimos `max` bytes de lo que escribe ffmpeg por la salida de error.
type cola struct {
	mu  sync.Mutex
	b   []byte
	max int
}

func (c *cola) Write(p []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.b = append(c.b, p...)
	if len(c.b) > c.max {
		c.b = c.b[len(c.b)-c.max:]
	}
	return len(p), nil
}

func (c *cola) String() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return string(c.b)
}
