// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package videoteca es la biblioteca de vídeos de Sytue Cast Office: los vídeos son
// FICHEROS de una carpeta (`<Datos>/Vídeos`) más los que la persona abre del disco y se
// quedan apuntados donde están, con un índice JSON legible al lado
// (apps/fyne/PLAN-EDICION-LOCAL.md §6.10, D-26).
//
// Cumple `api.CastData`, `api.CastMediaData` y `api.CastExploreData`, así que las
// pantallas de Cast de la edición conectada (Inicio, Explorar, Ver) se montan encima tal
// cual (D-04). «Publicar» es solo un estado del índice: lo publicado sale en la portada
// «Explorar» de esta app en este equipo; no se sirve nada a la red.
//
// # Qué se reproduce
//
// El reproductor de la app lee WebM/Matroska con VP9 y Opus. Lo demás (MP4, MOV, MKV con
// H.264, AVI, HEVC…) se convierte, con el ffmpeg que haya en el equipo (paquete
// `conversor`), a un WebM de visionado en la caché, que se puede borrar sin perder nada.
// Mientras se convierte el vídeo está «procesando». Sin ffmpeg, si la app tiene un
// descodificador del sistema que lo lee (Opciones.LeeElSistema: GStreamer en Linux),
// se ve tal cual; si no, «fallido» con ErrNecesitaFFmpeg, y la app ofrece el reproductor
// del sistema.
//
// # Pósters y colecciones
//
// El póster por defecto es un fotograma: con ffmpeg, o con el descodificador de la app
// (Opciones.Fotograma: el primero de un WebM, o el del sistema) y, si no, la miniatura
// que da el sistema (MiniaturaDelSistema). Las colecciones son un campo de cada vídeo;
// Colecciones las lista y Channel con PrefijoColeccion enseña una.
package videoteca

import (
	"cmp"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/buscar"
	"git.sytue.eu/sytue/one/apps/nucleo/local/conversor"
	"git.sytue.eu/sytue/one/apps/nucleo/local/espacio"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

// Extensiones son las de los vídeos que se recogen de la carpeta y se ofrecen al abrir.
var Extensiones = []string{"webm", "mkv", "mp4", "m4v", "mov", "avi", "mpg", "mpeg", "ogv", "3gp", "ts", "mts", "m2ts", "wmv", "flv"}

// ErrNecesitaFFmpeg: el vídeo no es WebM VP9/Opus y no hay ffmpeg para convertirlo.
var ErrNecesitaFFmpeg = errors.New("videoteca: para ver este formato en la app hace falta ffmpeg")

// ErrNoEsta: no hay ningún vídeo con ese id.
var ErrNoEsta = errors.New("videoteca: ese vídeo no está en la videoteca")

// ErrSinFichero: el vídeo sigue en el índice pero su fichero ya no está donde estaba.
var ErrSinFichero = errors.New("videoteca: el fichero del vídeo ya no está")

// EsVideo dice si un nombre de fichero tiene extensión de vídeo.
func EsVideo(nombre string) bool {
	ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(nombre)), ".")
	return ext != "" && slices.Contains(Extensiones, ext)
}

// Entrada es un vídeo del índice. Se guarda en claro en `videoteca.json`.
type Entrada struct {
	ID          string `json:"id"`
	Ruta        string `json:"ruta"`
	Titulo      string `json:"titulo"`
	Descripcion string `json:"descripcion,omitempty"`
	// Publicado: sale en «Explorar». Si no, es un borrador.
	Publicado   bool       `json:"publicado,omitempty"`
	PublicadoEn *time.Time `json:"publicado_en,omitempty"`
	// Coleccion ordena la portada (los «canales» de la edición conectada).
	Coleccion string `json:"coleccion,omitempty"`
	// Copiado: el fichero es una copia dentro de la carpeta de la videoteca. Solo esos se
	// pueden borrar del disco al quitarlos; los apuntados no se tocan nunca.
	Copiado     bool      `json:"copiado,omitempty"`
	Creado      time.Time `json:"creado"`
	Actualizado time.Time `json:"actualizado"`
	// Huella del fichero cuando se midió: si cambia, se vuelve a medir y a convertir.
	Tamano  int64 `json:"tamano"`
	ModNano int64 `json:"mod"`
	// Lo medido.
	Medido     bool   `json:"medido,omitempty"`
	DuracionMS int64  `json:"duracion_ms,omitempty"`
	Ancho      int    `json:"ancho,omitempty"`
	Alto       int    `json:"alto,omitempty"`
	Contenedor string `json:"contenedor,omitempty"`
	Video      string `json:"video,omitempty"`
	Audio      string `json:"audio,omitempty"`
	Nativo     bool   `json:"nativo,omitempty"`
	// Poster es el nombre del fichero en la carpeta de pósters; vacío, ninguno.
	Poster  string `json:"poster,omitempty"`
	PosterN int    `json:"poster_n,omitempty"`
}

// Opciones dicen dónde vive la videoteca y con qué se mide y se convierte.
type Opciones struct {
	// Carpeta es la de los vídeos (`<Datos>/Vídeos`).
	Carpeta string
	// Datos es donde va el índice y los pósters (`<Datos>/Datos/cast`).
	Datos string
	// Cache es donde van los WebM de visionado (`<caché>/cast`).
	Cache string
	// Autor es el nombre que se enseña como dueño (el del perfil).
	Autor string
	// Canal es el nombre de la portada de este equipo («En este equipo»).
	Canal string
	// Herramientas da el ffmpeg en uso; se pregunta cada vez, porque la persona lo puede
	// configurar con la app abierta. nil: conversor.Buscar("").
	Herramientas func() (conversor.Herramientas, error)
	// Sondear mide un fichero. nil: con ffprobe. La app pone delante su lector de WebM,
	// que dice lo que SU reproductor sabe leer sin depender de ffprobe.
	Sondear func(ctx context.Context, ruta string) (conversor.Info, error)
	// AlCambiar avisa (desde cualquier rutina) cuando cambia un vídeo: termina una
	// conversión, llega un póster.
	AlCambiar func(id string)
	// LeeElSistema dice si el reproductor de la app abre ese fichero tal cual con un
	// descodificador del sistema (CA-08, GStreamer en Linux): sin ffmpeg, entonces se
	// ve sin convertir. Se pregunta con la videoteca bloqueada: que lo recuerde. nil:
	// solo lo nativo.
	LeeElSistema func(ruta string) bool
	// Fotograma saca, sin ffmpeg, el fotograma del instante en como JPEG: el
	// descodificador de la app (el primero de un WebM, CA-06) o el del sistema. nil o
	// con error, se prueba la miniatura del sistema (MiniaturaDelSistema).
	Fotograma func(ctx context.Context, ruta string, en time.Duration) ([]byte, error)
}

// Videoteca es la biblioteca abierta. Es segura para varias rutinas.
type Videoteca struct {
	o Opciones

	mu       sync.Mutex
	entradas map[string]*Entrada
	trabajos map[string]*trabajo
	cola     chan string
	posters  map[string]bool
	// medidos son los que ya se intentaron medir en esta sesión (aunque fallara).
	medidos map[string]bool
}

type trabajo struct {
	progreso conversor.Progreso
	fallo    error
	cancelar context.CancelFunc
	enCurso  bool
}

// CarpetaEn es la carpeta de los vídeos de Cast dentro de la carpeta de datos de la
// edición (`<Datos>/Vídeos`): la que abre Cast y donde otra app de la edición deja lo que
// le da para que lo tenga (Photos, «Abrir en Cast» con un vídeo copiado a su fototeca).
// Lo que aparece en ella lo recoge la videoteca como suyo.
func CarpetaEn(datos string) string { return filepath.Join(datos, "Vídeos") }

// Abrir abre (o crea) la videoteca y recoge los vídeos nuevos de la carpeta.
func Abrir(o Opciones) (*Videoteca, error) {
	if o.Carpeta == "" || o.Datos == "" || o.Cache == "" {
		return nil, errors.New("videoteca: faltan carpetas")
	}
	for _, d := range []string{o.Carpeta, o.Datos, o.Cache} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			return nil, err
		}
	}
	v := &Videoteca{o: o, entradas: map[string]*Entrada{}, trabajos: map[string]*trabajo{}, cola: make(chan string, 256), posters: map[string]bool{}, medidos: map[string]bool{}}
	var lista []*Entrada
	if err := espacio.LeerJSON(v.rutaIndice(), &lista); err != nil {
		return nil, fmt.Errorf("videoteca: índice ilegible: %w", err)
	}
	for _, e := range lista {
		if e != nil && e.ID != "" {
			v.entradas[e.ID] = e
		}
	}
	if err := v.Recoger(); err != nil {
		return nil, err
	}
	go v.convertidor()
	// Los pósters que falten (lo recogido de la carpeta), de uno en uno y sin prisa.
	go func() {
		for _, e := range v.Lista() {
			if e.Poster == "" {
				v.posterPorDefecto(e.ID)
			}
		}
	}()
	return v, nil
}

func (v *Videoteca) rutaIndice() string { return filepath.Join(v.o.Datos, "videoteca.json") }
func (v *Videoteca) dirPosters() string { return filepath.Join(v.o.Datos, "posters") }

// Carpeta es la de los vídeos.
func (v *Videoteca) Carpeta() string { return v.o.Carpeta }

// guardar escribe el índice; con v.mu tomado.
func (v *Videoteca) guardar() error {
	lista := make([]*Entrada, 0, len(v.entradas))
	for _, e := range v.entradas {
		lista = append(lista, e)
	}
	slices.SortFunc(lista, func(a, b *Entrada) int { return cmp.Compare(a.Creado.UnixNano(), b.Creado.UnixNano()) })
	return espacio.EscribirJSON(v.rutaIndice(), lista)
}

func (v *Videoteca) herramientas() (conversor.Herramientas, error) {
	if v.o.Herramientas != nil {
		return v.o.Herramientas()
	}
	return conversor.Buscar("")
}

func (v *Videoteca) avisar(id string) {
	if v.o.AlCambiar != nil {
		v.o.AlCambiar(id)
	}
}

// Recoger añade al índice los vídeos de la carpeta que aún no están (como borradores).
// Lo que se copió a mano a la carpeta aparece así en la videoteca.
func (v *Videoteca) Recoger() error {
	v.mu.Lock()
	defer v.mu.Unlock()
	conocidos := map[string]bool{}
	for _, e := range v.entradas {
		conocidos[limpia(e.Ruta)] = true
	}
	nuevos := false
	err := filepath.WalkDir(v.o.Carpeta, func(ruta string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil //nolint:nilerr // lo ilegible se salta, no para la recogida
		}
		if strings.HasPrefix(d.Name(), ".") && ruta != v.o.Carpeta {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() || !d.Type().IsRegular() || !EsVideo(d.Name()) || conocidos[limpia(ruta)] {
			return nil
		}
		e, err := v.nuevaEntrada(ruta, true)
		if err == nil {
			v.entradas[e.ID] = e
			nuevos = true
		}
		return nil
	})
	if err != nil {
		return err
	}
	if nuevos {
		return v.guardar()
	}
	return nil
}

func limpia(ruta string) string {
	if abs, err := filepath.Abs(ruta); err == nil {
		return filepath.Clean(abs)
	}
	return filepath.Clean(ruta)
}

func (v *Videoteca) dentroDeLaCarpeta(ruta string) bool {
	rel, err := filepath.Rel(limpia(v.o.Carpeta), limpia(ruta))
	return err == nil && rel != "." && !strings.HasPrefix(rel, "..")
}

func nuevoID() string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

// nuevaEntrada da de alta un fichero sin medir.
func (v *Videoteca) nuevaEntrada(ruta string, copiado bool) (*Entrada, error) {
	fi, err := os.Stat(ruta)
	if err != nil {
		return nil, err
	}
	if !fi.Mode().IsRegular() {
		return nil, fmt.Errorf("videoteca: %s no es un fichero", ruta)
	}
	ahora := time.Now().UTC()
	return &Entrada{
		ID: nuevoID(), Ruta: limpia(ruta), Titulo: TituloDe(filepath.Base(ruta)), Copiado: copiado,
		Creado: ahora, Actualizado: ahora, Tamano: fi.Size(), ModNano: fi.ModTime().UnixNano(),
	}, nil
}

// TituloDe es el título que nace de un nombre de fichero: sin la extensión y sin los
// guiones bajos (como el de subir de la edición conectada).
func TituloDe(nombre string) string {
	sin := strings.TrimSuffix(nombre, filepath.Ext(nombre))
	sin = strings.Join(strings.Fields(strings.NewReplacer("_", " ", ".", " ").Replace(sin)), " ")
	if sin == "" {
		return nombre
	}
	return sin
}

// ---- Altas, cambios y bajas ----

// Abrir pone en la videoteca un fichero del disco SIN copiarlo (se queda donde está) y
// devuelve su entrada. Si ya estaba, la que había.
func (v *Videoteca) Apuntar(ctx context.Context, ruta string) (Entrada, error) {
	return v.alta(ctx, ruta, false)
}

// Importar COPIA el fichero a la carpeta de la videoteca y lo da de alta. La copia va a
// un temporal que se renombra al acabar: nunca queda un vídeo a medias.
func (v *Videoteca) Importar(ctx context.Context, ruta string) (Entrada, error) {
	return v.alta(ctx, ruta, true)
}

func (v *Videoteca) alta(ctx context.Context, ruta string, copiar bool) (Entrada, error) {
	ruta = limpia(ruta)
	v.mu.Lock()
	for _, e := range v.entradas {
		if limpia(e.Ruta) == ruta {
			x := *e
			v.mu.Unlock()
			return x, nil
		}
	}
	v.mu.Unlock()
	destino := ruta
	if copiar && !v.dentroDeLaCarpeta(ruta) {
		var err error
		if destino, err = copiarA(ctx, ruta, v.o.Carpeta); err != nil {
			return Entrada{}, err
		}
	}
	e, err := v.nuevaEntrada(destino, copiar || v.dentroDeLaCarpeta(destino))
	if err != nil {
		return Entrada{}, err
	}
	v.medir(ctx, e)
	v.mu.Lock()
	v.entradas[e.ID] = e
	err = v.guardar()
	x := *e
	v.mu.Unlock()
	if err != nil {
		return Entrada{}, err
	}
	go v.posterPorDefecto(x.ID) //nolint:gosec,contextcheck // G118: el póster va aparte y con su propio plazo; el alta ya ha terminado
	return x, nil
}

// copiarA copia ruta dentro de carpeta con un nombre libre.
func copiarA(ctx context.Context, ruta, carpeta string) (string, error) {
	origen, err := os.Open(ruta)
	if err != nil {
		return "", err
	}
	defer func() { _ = origen.Close() }()
	base := filepath.Base(ruta)
	ext := filepath.Ext(base)
	nombre := strings.TrimSuffix(base, ext)
	destino := filepath.Join(carpeta, base)
	for i := 2; ; i++ {
		if _, err := os.Lstat(destino); errors.Is(err, os.ErrNotExist) {
			break
		}
		destino = filepath.Join(carpeta, fmt.Sprintf("%s (%d)%s", nombre, i, ext))
	}
	tmp, err := os.CreateTemp(carpeta, ".importando-*"+ext)
	if err != nil {
		return "", err
	}
	_, err = io.Copy(tmp, lectorConContexto{ctx: ctx, r: origen})
	if errC := tmp.Close(); err == nil {
		err = errC
	}
	if err == nil {
		err = os.Rename(tmp.Name(), destino)
	}
	if err != nil {
		_ = os.Remove(tmp.Name())
		return "", err
	}
	return destino, nil
}

type lectorConContexto struct {
	ctx context.Context //nolint:containedctx // vive lo que dura la copia
	r   io.Reader
}

func (l lectorConContexto) Read(p []byte) (int, error) {
	if err := l.ctx.Err(); err != nil {
		return 0, err
	}
	return l.r.Read(p)
}

// medir rellena lo medido de e (sin el candado: e aún no es de nadie, o es una copia).
func (v *Videoteca) medir(ctx context.Context, e *Entrada) {
	sondear := v.o.Sondear
	if sondear == nil {
		sondear = func(ctx context.Context, ruta string) (conversor.Info, error) {
			h, err := v.herramientas()
			if err != nil {
				return conversor.Info{}, err
			}
			return h.Sondear(ctx, ruta)
		}
	}
	c, fin := context.WithTimeout(ctx, 30*time.Second)
	defer fin()
	info, err := sondear(c, e.Ruta)
	if err != nil {
		// Sin ffprobe, un MP4/MOV se mide en Go (CA-05): así la videoteca enseña su
		// duración y su tamaño, aunque para verlo haga falta convertirlo.
		info, err = conversor.SondearMP4(e.Ruta)
	}
	if err != nil {
		// Sin poder medirlo: un WebM se intenta abrir tal cual; lo demás necesita ffmpeg.
		e.Medido = false
		e.Nativo = strings.EqualFold(filepath.Ext(e.Ruta), ".webm")
		return
	}
	e.Medido, e.Nativo = true, info.Nativo()
	e.DuracionMS, e.Ancho, e.Alto = info.Duracion.Milliseconds(), info.Ancho, info.Alto
	e.Contenedor, e.Video, e.Audio = info.Contenedor, info.Video, info.Audio
}

// Cambios son los campos que se cambian; nil no se toca.
type Cambios struct {
	Titulo, Descripcion, Coleccion *string
	Publicado                      *bool
}

// Actualizar cambia lo que venga en c.
func (v *Videoteca) Actualizar(id string, c Cambios) (Entrada, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	e, ok := v.entradas[id]
	if !ok {
		return Entrada{}, ErrNoEsta
	}
	if c.Titulo != nil {
		t := strings.TrimSpace(*c.Titulo)
		if t == "" {
			return Entrada{}, errors.New("videoteca: el título no puede quedar vacío")
		}
		e.Titulo = t
	}
	if c.Descripcion != nil {
		e.Descripcion = strings.TrimSpace(*c.Descripcion)
	}
	if c.Coleccion != nil {
		e.Coleccion = strings.TrimSpace(*c.Coleccion)
	}
	if c.Publicado != nil && *c.Publicado != e.Publicado {
		e.Publicado = *c.Publicado
		if e.Publicado {
			ahora := time.Now().UTC()
			e.PublicadoEn = &ahora
		} else {
			e.PublicadoEn = nil
		}
	}
	e.Actualizado = time.Now().UTC()
	return *e, v.guardar()
}

// Borrar quita el vídeo de la videoteca, con su póster y su visionado. conFichero borra
// además el fichero, pero SOLO si es una copia dentro de la carpeta de la videoteca: un
// fichero apuntado de fuera nunca se borra desde aquí.
func (v *Videoteca) Borrar(id string, conFichero bool) error {
	v.mu.Lock()
	e, ok := v.entradas[id]
	if !ok {
		v.mu.Unlock()
		return ErrNoEsta
	}
	x := *e
	if t := v.trabajos[id]; t != nil && t.cancelar != nil {
		t.cancelar()
	}
	delete(v.trabajos, id)
	delete(v.entradas, id)
	err := v.guardar()
	v.mu.Unlock()
	if err != nil {
		return err
	}
	v.borrarVisionados(id)
	if x.Poster != "" {
		_ = os.Remove(filepath.Join(v.dirPosters(), x.Poster))
	}
	if conFichero && x.Copiado && v.dentroDeLaCarpeta(x.Ruta) {
		if err := os.Remove(x.Ruta); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return nil
}

// PonerPoster pone una imagen propia (JPEG, PNG o WebP) como póster.
func (v *Videoteca) PonerPoster(id string, datos []byte, ext string) (Entrada, error) {
	ext = strings.ToLower(strings.TrimPrefix(ext, "."))
	switch ext {
	case "jpg", "jpeg", "png", "webp":
	default:
		return Entrada{}, fmt.Errorf("videoteca: el póster tiene que ser JPEG, PNG o WebP")
	}
	if len(datos) == 0 || len(datos) > 16<<20 {
		return Entrada{}, errors.New("videoteca: el póster está vacío o pasa de 16 MiB")
	}
	nombre := id + "." + ext
	if err := escribirAtomico(filepath.Join(v.dirPosters(), nombre), datos); err != nil {
		return Entrada{}, err
	}
	return v.anotarPoster(id, nombre)
}

func (v *Videoteca) anotarPoster(id, nombre string) (Entrada, error) {
	v.mu.Lock()
	e, ok := v.entradas[id]
	if !ok {
		v.mu.Unlock()
		return Entrada{}, ErrNoEsta
	}
	if e.Poster != "" && e.Poster != nombre {
		_ = os.Remove(filepath.Join(v.dirPosters(), e.Poster))
	}
	e.Poster, e.PosterN = nombre, e.PosterN+1
	e.Actualizado = time.Now().UTC()
	x := *e
	err := v.guardar()
	v.mu.Unlock()
	v.avisar(id)
	return x, err
}

// PosterDeFotograma pone de póster el fotograma del instante en: con ffmpeg si lo hay
// y, si no, con el descodificador de la app o el del sistema (Opciones.Fotograma).
func (v *Videoteca) PosterDeFotograma(ctx context.Context, id string, en time.Duration) (Entrada, error) {
	e, err := v.Entrada(id)
	if err != nil {
		return Entrada{}, err
	}
	nombre := id + ".jpg"
	h, err := v.herramientas()
	if err == nil {
		if err := h.Fotograma(ctx, e.Ruta, filepath.Join(v.dirPosters(), nombre), en); err != nil {
			return Entrada{}, err
		}
		return v.anotarPoster(id, nombre)
	}
	if v.o.Fotograma == nil {
		return Entrada{}, ErrNecesitaFFmpeg
	}
	jpg, err := v.o.Fotograma(ctx, e.Ruta, en)
	if err != nil {
		return Entrada{}, err
	}
	return v.PonerPoster(id, jpg, "jpg")
}

// PonerFotograma pone de póster una imagen ya descodificada (el fotograma que enseña
// el reproductor), en JPEG.
func (v *Videoteca) PonerFotograma(id string, jpg []byte) (Entrada, error) {
	return v.PonerPoster(id, jpg, "jpg")
}

// posterPorDefecto saca un póster al dar de alta un vídeo, si hay ffmpeg y no tiene.
func (v *Videoteca) posterPorDefecto(id string) {
	v.mu.Lock()
	e, ok := v.entradas[id]
	if !ok || e.Poster != "" || v.posters[id] {
		v.mu.Unlock()
		return
	}
	v.posters[id] = true
	dur := time.Duration(e.DuracionMS) * time.Millisecond
	ruta := e.Ruta
	v.mu.Unlock()
	en := min(time.Second, dur/2)
	ctx, fin := context.WithTimeout(context.Background(), time.Minute)
	defer fin()
	if _, err := v.PosterDeFotograma(ctx, id, en); err == nil {
		return
	}
	// Sin ffmpeg ni descodificador que lo lea: la miniatura que ya hizo el sistema.
	if img, ext, err := MiniaturaDelSistema(ctx, ruta); err == nil {
		_, _ = v.PonerPoster(id, img, ext)
	}
}

func escribirAtomico(ruta string, datos []byte) error {
	if err := os.MkdirAll(filepath.Dir(ruta), 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(ruta), "."+filepath.Base(ruta)+".*.tmp")
	if err != nil {
		return err
	}
	_, err = tmp.Write(datos)
	if errC := tmp.Close(); err == nil {
		err = errC
	}
	if err == nil {
		err = os.Rename(tmp.Name(), ruta)
	}
	if err != nil {
		_ = os.Remove(tmp.Name())
	}
	return err
}

// ---- Consultas ----

// Entrada devuelve una copia de la entrada de id.
func (v *Videoteca) Entrada(id string) (Entrada, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	e, ok := v.entradas[id]
	if !ok {
		return Entrada{}, ErrNoEsta
	}
	return *e, nil
}

// Lista son todas las entradas, la más reciente primero.
func (v *Videoteca) Lista() []Entrada {
	v.mu.Lock()
	defer v.mu.Unlock()
	out := make([]Entrada, 0, len(v.entradas))
	for _, e := range v.entradas {
		out = append(out, *e)
	}
	slices.SortFunc(out, func(a, b Entrada) int {
		return cmp.Or(b.Actualizado.Compare(a.Actualizado), cmp.Compare(a.ID, b.ID))
	})
	return out
}

// ---- Visionado: lo que lee el reproductor ----

// Estado es en qué punto está un vídeo para verse en la app.
type Estado struct {
	// Listo: se puede reproducir ya (Ruta es lo que se abre).
	Listo bool
	Ruta  string
	// Convirtiendo, con su progreso.
	Convirtiendo bool
	Progreso     conversor.Progreso
	// Fallo es por qué no se puede ver (ErrNecesitaFFmpeg, ErrSinFichero o lo que dijo
	// ffmpeg).
	Fallo error
}

func (v *Videoteca) rutaVisionado(e *Entrada) string {
	return filepath.Join(v.o.Cache, fmt.Sprintf("%s-%d-%d.webm", e.ID, e.Tamano, e.ModNano))
}

func (v *Videoteca) borrarVisionados(id string) {
	viejos, _ := filepath.Glob(filepath.Join(v.o.Cache, id+"-*.webm"))
	for _, f := range viejos {
		_ = os.Remove(f)
	}
}

// refrescarHuella vuelve a medir si el fichero ha cambiado desde la última vez; con v.mu
// tomado. Dice si el fichero existe.
//
//nolint:contextcheck // medir lleva su propio plazo y lo medido queda guardado para todos: no es de la consulta que lo destapa
func (v *Videoteca) refrescarHuella(e *Entrada) bool {
	fi, err := os.Stat(e.Ruta)
	if err != nil {
		return false
	}
	if fi.Size() != e.Tamano || fi.ModTime().UnixNano() != e.ModNano || (!e.Medido && !v.medidos[e.ID]) {
		v.medidos[e.ID] = true
		e.Tamano, e.ModNano = fi.Size(), fi.ModTime().UnixNano()
		x := *e
		v.mu.Unlock()
		v.medir(context.Background(), &x)
		v.mu.Lock()
		e.Medido, e.Nativo, e.DuracionMS, e.Ancho, e.Alto = x.Medido, x.Nativo, x.DuracionMS, x.Ancho, x.Alto
		e.Contenedor, e.Video, e.Audio = x.Contenedor, x.Video, x.Audio
		_ = v.guardar()
	}
	return true
}

// Visionado dice si el vídeo se puede ver ya y, si hace falta convertirlo y hay ffmpeg,
// lo pone en la cola (Preparar).
func (v *Videoteca) Visionado(id string) Estado {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.visionado(id, true)
}

// EstadoDe es Visionado sin poner nada en la cola.
func (v *Videoteca) EstadoDe(id string) Estado {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.visionado(id, false)
}

func (v *Videoteca) visionado(id string, preparar bool) Estado {
	e, ok := v.entradas[id]
	if !ok {
		return Estado{Fallo: ErrNoEsta}
	}
	if !v.refrescarHuella(e) {
		return Estado{Fallo: ErrSinFichero}
	}
	if e.Nativo {
		return Estado{Listo: true, Ruta: e.Ruta}
	}
	cache := v.rutaVisionado(e)
	if fi, err := os.Stat(cache); err == nil && fi.Size() > 0 {
		return Estado{Listo: true, Ruta: cache}
	}
	t := v.trabajos[id]
	if t != nil && t.enCurso {
		return Estado{Convirtiendo: true, Progreso: t.progreso}
	}
	if t != nil && t.fallo != nil {
		return Estado{Fallo: t.fallo}
	}
	if _, err := v.herramientas(); err != nil {
		// Sin ffmpeg, el descodificador del sistema, si lo lee, lo enseña tal cual (con
		// ffmpeg se prefiere convertir: el WebM de visionado busca al instante).
		if v.o.LeeElSistema != nil && v.o.LeeElSistema(e.Ruta) {
			return Estado{Listo: true, Ruta: e.Ruta}
		}
		return Estado{Fallo: ErrNecesitaFFmpeg}
	}
	if !preparar {
		return Estado{}
	}
	v.trabajos[id] = &trabajo{enCurso: true}
	select {
	case v.cola <- id:
	default:
		v.trabajos[id] = &trabajo{fallo: errors.New("videoteca: hay demasiadas conversiones en cola")}
		return Estado{Fallo: v.trabajos[id].fallo}
	}
	return Estado{Convirtiendo: true}
}

// Reintentar olvida el fallo de la última conversión (p. ej., tras instalar ffmpeg).
func (v *Videoteca) Reintentar(id string) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if t := v.trabajos[id]; t != nil && !t.enCurso {
		delete(v.trabajos, id)
	}
}

// convertidor hace las conversiones de la cola de una en una: el equipo es uno y así la
// primera acaba antes.
func (v *Videoteca) convertidor() {
	for id := range v.cola {
		v.convertir(id)
	}
}

func (v *Videoteca) convertir(id string) {
	v.mu.Lock()
	e, ok := v.entradas[id]
	t := v.trabajos[id]
	if !ok || t == nil {
		v.mu.Unlock()
		return
	}
	ctx, cancelar := context.WithCancel(context.Background())
	t.cancelar = cancelar
	entrada, salida, total := e.Ruta, v.rutaVisionado(e), time.Duration(e.DuracionMS)*time.Millisecond
	v.mu.Unlock()
	defer cancelar()
	h, err := v.herramientas()
	if err == nil {
		v.borrarVisionados(id)
		ultimo := time.Time{}
		err = h.Convertir(ctx, entrada, salida, conversor.Visionado, total, func(p conversor.Progreso) {
			v.mu.Lock()
			t.progreso = p
			v.mu.Unlock()
			if time.Since(ultimo) > time.Second {
				ultimo = time.Now()
				v.avisar(id)
			}
		})
	} else {
		err = ErrNecesitaFFmpeg
	}
	v.mu.Lock()
	t.enCurso, t.cancelar = false, nil
	if err != nil {
		t.fallo = err
	} else {
		delete(v.trabajos, id)
	}
	v.mu.Unlock()
	v.avisar(id)
	go v.posterPorDefecto(id)
}

// ---- api.CastData ----

// Las rutas de los ficheros que se piden por OpenMedia: «local:<id>/ver» y
// «local:<id>/poster». No son direcciones de red: solo las entiende esta videoteca.
const prefijoRuta = "local:"

// PrivateStorageScope es el de esta instalación; solo sirve para que la fachada de Cast
// acepte la videoteca.
func (v *Videoteca) PrivateStorageScope() storage.Scope {
	return storage.Scope{Tenant: "local", Subject: Yo, Product: "cast"}
}

// Yo es el «sub» de quien usa la app: lo propio no se denuncia ni se bloquea.
const Yo = "local"

func (v *Videoteca) aVideo(e *Entrada, est Estado) api.CastVideo {
	x := api.CastVideo{
		ID: e.ID, Tenant: "local", Owner: api.CastOwner{Sub: Yo, Name: cmp.Or(v.o.Autor, Yo)}, Kind: "upload",
		Status: api.CastStatusReady, Visibility: api.CastUnlisted, Title: e.Titulo, Description: e.Descripcion,
		DurationMS: e.DuracionMS, Width: e.Ancho, Height: e.Alto, PublishedAt: e.PublicadoEn,
		CreatedAt: e.Creado, UpdatedAt: e.Actualizado,
	}
	if e.Publicado {
		x.Visibility = api.CastPublic
	}
	if e.Poster != "" {
		x.PosterURL = prefijoRuta + e.ID + "/poster?n=" + strconv.Itoa(e.PosterN)
	}
	switch {
	case est.Listo:
	case est.Convirtiendo:
		x.Status = api.CastStatusProcessing
	case est.Fallo != nil:
		x.Status, x.Failure = api.CastStatusFailed, est.Fallo.Error()
	default:
		// Sin convertir todavía: se convierte al abrirlo.
		x.Status = api.CastStatusProcessing
	}
	return x
}

// Videos es una página de la videoteca. scope «org» no tiene nada (no hay organización);
// status y q filtran; el cursor es la posición.
func (v *Videoteca) Videos(_ context.Context, scope, status, q, cursor string, limite int) (api.CastPage, error) {
	if scope == api.CastOrgScope {
		return api.CastPage{}, nil
	}
	return v.pagina(func(Entrada) bool { return true }, status, q, cursor, limite), nil
}

func (v *Videoteca) pagina(filtro func(Entrada) bool, status, q, cursor string, limite int) api.CastPage {
	q = buscar.Plegar(strings.TrimSpace(q))
	var todos []api.CastVideo
	for _, e := range v.Lista() {
		if !filtro(e) || (q != "" && !strings.Contains(buscar.Plegar(e.Titulo+" "+e.Descripcion+" "+filepath.Base(e.Ruta)), q)) {
			continue
		}
		x := v.aVideo(&e, v.EstadoDe(e.ID))
		if status != "" && x.Status != status {
			continue
		}
		todos = append(todos, x)
	}
	desde, _ := strconv.Atoi(cursor)
	desde = min(max(desde, 0), len(todos))
	if limite <= 0 {
		limite = 24
	}
	hasta := min(desde+limite, len(todos))
	p := api.CastPage{Videos: todos[desde:hasta]}
	if hasta < len(todos) {
		p.NextCursor = strconv.Itoa(hasta)
	}
	return p
}

// Video es la ficha. Abrirla pone a convertir lo que no se ve tal cual.
func (v *Videoteca) Video(_ context.Context, id string) (api.CastVideo, error) {
	est := v.Visionado(id)
	if errors.Is(est.Fallo, ErrNoEsta) {
		return api.CastVideo{}, &api.Error{Status: 404, Body: `{"message":"` + ErrNoEsta.Error() + `"}`}
	}
	e, err := v.Entrada(id)
	if err != nil {
		return api.CastVideo{}, &api.Error{Status: 404, Body: `{"message":"` + ErrNoEsta.Error() + `"}`}
	}
	return v.aVideo(&e, est), nil
}

// Play da la ruta de lo que se reproduce: el original si el reproductor lo lee, o su
// visionado ya convertido.
func (v *Videoteca) Play(_ context.Context, id string) (api.CastPlay, error) {
	est := v.EstadoDe(id)
	if !est.Listo {
		if est.Fallo != nil {
			return api.CastPlay{}, est.Fallo
		}
		return api.CastPlay{}, errors.New("videoteca: el vídeo aún se está preparando")
	}
	e, err := v.Entrada(id)
	if err != nil {
		return api.CastPlay{}, err
	}
	p := api.CastPlay{Kind: "upload", WebMURL: prefijoRuta + id + "/ver", DurationMS: e.DuracionMS}
	if e.Poster != "" {
		p.PosterURL = prefijoRuta + id + "/poster?n=" + strconv.Itoa(e.PosterN)
	}
	return p, nil
}

// medio es un fichero abierto para el reproductor.
type medio struct {
	*os.File
	tam int64
}

func (m medio) Size() int64 { return m.tam }

// OpenMedia abre una ruta de Play o del póster.
func (v *Videoteca) OpenMedia(_ context.Context, ruta string) (api.CastPrivateMedia, error) {
	resto, ok := strings.CutPrefix(ruta, prefijoRuta)
	if !ok {
		return nil, api.ErrPrivateCastOperation
	}
	resto, _, _ = strings.Cut(resto, "?")
	id, que, _ := strings.Cut(resto, "/")
	var fichero string
	switch que {
	case "ver":
		est := v.EstadoDe(id)
		if !est.Listo {
			return nil, cmp.Or(est.Fallo, errors.New("videoteca: el vídeo aún se está preparando"))
		}
		fichero = est.Ruta
	case "poster":
		e, err := v.Entrada(id)
		if err != nil {
			return nil, err
		}
		if e.Poster == "" {
			return nil, os.ErrNotExist
		}
		fichero = filepath.Join(v.dirPosters(), e.Poster)
	default:
		return nil, api.ErrPrivateCastOperation
	}
	f, err := os.Open(fichero)
	if err != nil {
		return nil, err
	}
	fi, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, err
	}
	return medio{File: f, tam: fi.Size()}, nil
}

// ---- api.CastExploreData ----

// Explore es la portada de este equipo: lo publicado.
func (v *Videoteca) Explore(_ context.Context, cursor string, limite int) (api.CastExplore, error) {
	p := v.pagina(func(e Entrada) bool { return e.Publicado }, "", "", cursor, limite)
	return api.CastExplore{Videos: p.Videos, NextCursor: p.NextCursor}, nil
}

// PrefijoColeccion es el del «canal» de una colección: `AbrirCanal("coleccion:Viajes")`.
const PrefijoColeccion = "coleccion:"

// Channel es el «canal» de este equipo: lo publicado, como Explore; el de una colección
// (PrefijoColeccion + nombre) son todos sus vídeos, publicados o no.
func (v *Videoteca) Channel(_ context.Context, org, cursor string) (api.CastChannel, error) {
	if nombre, ok := strings.CutPrefix(org, PrefijoColeccion); ok {
		p := v.pagina(func(e Entrada) bool { return strings.EqualFold(e.Coleccion, nombre) }, "", "", cursor, 24)
		return api.CastChannel{Org: org, Name: nombre, Videos: p.Videos, NextCursor: p.NextCursor}, nil
	}
	p := v.pagina(func(e Entrada) bool { return e.Publicado }, "", "", cursor, 24)
	return api.CastChannel{Org: "local", Name: cmp.Or(v.o.Canal, v.o.Autor, Yo), Videos: p.Videos, NextCursor: p.NextCursor}, nil
}

// Coleccion es una colección con cuántos vídeos tiene y cuántos publicados.
type Coleccion struct {
	Nombre             string
	Videos, Publicados int
	Actualizada        time.Time
	Poster             string // el id del vídeo más reciente con póster, para la tarjeta
}

// Colecciones son las que hay (las escritas en algún vídeo), por nombre.
func (v *Videoteca) Colecciones() []Coleccion {
	por := map[string]*Coleccion{}
	for _, e := range v.Lista() {
		if e.Coleccion == "" {
			continue
		}
		k := strings.ToLower(e.Coleccion)
		c := por[k]
		if c == nil {
			c = &Coleccion{Nombre: e.Coleccion}
			por[k] = c
		}
		c.Videos++
		if e.Publicado {
			c.Publicados++
		}
		if e.Actualizado.After(c.Actualizada) {
			c.Actualizada = e.Actualizado
		}
		if c.Poster == "" && e.Poster != "" {
			c.Poster = e.ID
		}
	}
	out := make([]Coleccion, 0, len(por))
	for _, c := range por {
		out = append(out, *c)
	}
	slices.SortFunc(out, func(a, b Coleccion) int { return strings.Compare(strings.ToLower(a.Nombre), strings.ToLower(b.Nombre)) })
	return out
}

// Comprobaciones de compilación: lo que la fachada de Cast espera.
var (
	_ api.CastData        = (*Videoteca)(nil)
	_ api.CastMediaData   = (*Videoteca)(nil)
	_ api.CastExploreData = (*Videoteca)(nil)
)
