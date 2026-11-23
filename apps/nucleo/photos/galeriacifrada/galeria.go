// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package galeriacifrada es la galería cifrada de Sytue Photos (ADR 0086): el
// índice de las fotos que viven cifradas en «Fotos» del Drive de la bóveda, escrito
// y leído SOLO en el dispositivo.
//
// # Dónde vive y por qué ahí
//
// El índice son ficheros del propio Drive de la bóveda, dentro de «Fotos»:
//
//	Fotos/
//	  IMG_0001.jpg                      el original (cifrado, como todo el Drive)
//	  .sytue-galeria/indice/<id>.json   una entrada por foto (id del fichero de Drive)
//	  .sytue-galeria/miniaturas/<id>.jpg la miniatura, hecha en el dispositivo
//	  .sytue-galeria/albumes/<id>.json  un registro por álbum
//
// No son registros sueltos de otro tipo en la bóveda de `drive`: el catálogo de
// Drive (web y nativo) y su copia de Matrix a la bóveda verifican lo que hay, y un
// tipo ajeno ahí es un riesgo sin necesidad. Tampoco es una bóveda `photos` nueva:
// esa es la de los registros de Photos de la ADR 0086, que aún no está decidida, y
// crearla haría creer al nativo que ya guarda registros. Como ficheros del Drive, el
// índice va cifrado con la misma clave y el mismo formato que la foto, el servidor
// solo ve bloques opacos y Drive lo trata como cualquier otro fichero (versiones
// incluidas: escribir una entrada otra vez crea una versión nueva, no borra la de
// antes).
//
// # Reglas
//
//   - Una entrada por foto, no un índice entero: dos dispositivos que tocan fotos
//     distintas no se pisan. Cada cambio relee la entrada antes de escribirla y aplica
//     solo lo suyo (favorita, álbumes), así que repetirlo no cambia nada.
//   - El índice nunca es la única copia de nada: si falta una entrada, o no se lee, la
//     foto sale igual con lo que dice Drive (nombre y fecha de subida) y se puede
//     volver a indexar desde el original ([Galeria.Reconstruir]).
//   - Nada se borra desde aquí: ni fotos, ni entradas, ni miniaturas.
package galeriacifrada

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image/jpeg"
	"io"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/miniatura"
	"git.sytue.eu/sytue/one/apps/nucleo/photos/exif"
	"git.sytue.eu/sytue/one/apps/nucleo/photos/model"
	"git.sytue.eu/sytue/one/apps/nucleo/photos/render"
)

// Nombres y topes del formato. Los mismos que la web
// (`apps/frontend/apps/photos/src/client/galeriaCifrada.ts`).
const (
	CarpetaFotos      = "Fotos"
	CarpetaGaleria    = ".sytue-galeria"
	CarpetaIndice     = "indice"
	CarpetaMiniaturas = "miniaturas"
	CarpetaAlbumes    = "albumes"

	Version   = 1
	TipoFoto  = "sytue.photos.cifrada.foto"
	TipoAlbum = "sytue.photos.cifrada.album"

	// LadoMiniatura es el de las miniaturas de Drive y de la galería del servidor.
	LadoMiniatura = 512
	// MaxEntrada es lo más que se lee de una entrada o un álbum.
	MaxEntrada = 64 << 10
	// MaxMiniatura es lo más que se lee de una miniatura.
	MaxMiniatura = 8 << 20
	// MaxAlbumesPorFoto acota la lista de álbumes de una entrada.
	MaxAlbumesPorFoto = 1000
	// MaxTitulo es lo más largo que puede ser el título de un álbum.
	MaxTitulo = 200
	// lectores es cuántas entradas se descifran a la vez al leer el índice.
	lectores = 4
)

// Errores que distingue quien llama.
var (
	// ErrEntrada: una entrada o un álbum que no siguen el formato.
	ErrEntrada = errors.New("galeriacifrada: entrada no válida")
	// ErrSinFoto: la foto no está en «Fotos».
	ErrSinFoto = errors.New("galeriacifrada: la foto no está en «Fotos»")
	// ErrSinAlbum: el álbum no está en el índice.
	ErrSinAlbum = errors.New("galeriacifrada: el álbum no está en el índice")
)

// Almacen es lo que la galería usa del Drive de la bóveda. *api.Drive con sus datos
// privados lo cumple.
type Almacen interface {
	Root(ctx context.Context) (api.Folder, error)
	Children(ctx context.Context, folder, cursor string) (api.Children, error)
	CreateFolder(ctx context.Context, parent, name string) (api.Folder, error)
	Upload(ctx context.Context, folder, name, contentType string, input io.Reader, size int64) (api.File, error)
	Download(ctx context.Context, id string) (io.ReadCloser, error)
}

// Entrada es lo que el índice sabe de una foto.
type Entrada struct {
	V    int    `json:"v"`
	Tipo string `json:"tipo"`
	// Foto es el id del fichero en «Fotos» del Drive de la bóveda.
	Foto string `json:"foto"`
	// Suma es la del contenido que se indexó: si la foto cambia (una edición), la
	// entrada se rehace (miniatura y medidas), conservando favorita y álbumes.
	Suma   string `json:"suma"`
	Nombre string `json:"nombre"`
	// Tomada es la fecha de captura del EXIF o, sin ella (Exif false), la de subida.
	Tomada      time.Time `json:"tomada"`
	Exif        bool      `json:"exif"`
	Ancho       int       `json:"ancho"`
	Alto        int       `json:"alto"`
	Favorita    bool      `json:"favorita"`
	Albumes     []string  `json:"albumes"`
	Miniatura   string    `json:"miniatura"`
	Actualizada time.Time `json:"actualizada"`
}

// Album es un álbum de la galería cifrada. Sus fotos son las entradas que lo nombran.
type Album struct {
	V           int       `json:"v"`
	Tipo        string    `json:"tipo"`
	ID          string    `json:"id"`
	Titulo      string    `json:"titulo"`
	Creado      time.Time `json:"creado"`
	Actualizado time.Time `json:"actualizado"`
}

func idValido(id string) bool {
	if id == "" || len(id) > 128 {
		return false
	}
	for _, c := range id {
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9', c == '-', c == '_':
		default:
			return false
		}
	}
	return true
}

// Validar comprueba el formato de una entrada.
func (e Entrada) Validar() error {
	switch {
	case e.V != Version, e.Tipo != TipoFoto, !idValido(e.Foto),
		e.Miniatura != "" && !idValido(e.Miniatura),
		len(e.Nombre) > 255, len(e.Suma) > 128,
		e.Ancho < 0, e.Alto < 0, e.Ancho > 1<<20, e.Alto > 1<<20,
		len(e.Albumes) > MaxAlbumesPorFoto:
		return ErrEntrada
	}
	for _, a := range e.Albumes {
		if !idValido(a) {
			return ErrEntrada
		}
	}
	return nil
}

// Validar comprueba el formato de un álbum.
func (a Album) Validar() error {
	t := strings.TrimSpace(a.Titulo)
	if a.V != Version || a.Tipo != TipoAlbum || !idValido(a.ID) || t == "" || len([]rune(t)) > MaxTitulo {
		return ErrEntrada
	}
	return nil
}

// DecodificarEntrada lee y valida una entrada.
func DecodificarEntrada(datos []byte) (Entrada, error) {
	var e Entrada
	if len(datos) > MaxEntrada || json.Unmarshal(datos, &e) != nil {
		return Entrada{}, ErrEntrada
	}
	if err := e.Validar(); err != nil {
		return Entrada{}, err
	}
	e.Albumes = sinRepetir(e.Albumes)
	return e, nil
}

// DecodificarAlbum lee y valida un álbum.
func DecodificarAlbum(datos []byte) (Album, error) {
	var a Album
	if len(datos) > MaxEntrada || json.Unmarshal(datos, &a) != nil {
		return Album{}, ErrEntrada
	}
	if err := a.Validar(); err != nil {
		return Album{}, err
	}
	a.Titulo = strings.TrimSpace(a.Titulo)
	return a, nil
}

func sinRepetir(ids []string) []string {
	out := make([]string, 0, len(ids))
	visto := map[string]bool{}
	for _, id := range ids {
		if !visto[id] {
			visto[id] = true
			out = append(out, id)
		}
	}
	slices.Sort(out)
	return out
}

// Foto es una foto de «Fotos» con lo que el índice sabe de ella.
type Foto struct {
	Fichero api.File
	Entrada Entrada
	// ConEntrada: hay una entrada válida de esta foto. Sin ella, Entrada sale de lo que
	// dice Drive (nombre y fecha de subida) y la foto se puede reconstruir.
	ConEntrada bool
	// Pendiente: sin entrada, o la entrada es de otro contenido (la foto se editó), o
	// le falta la miniatura. [Galeria.Reconstruir] la pone al día.
	Pendiente bool
}

// Indice es la galería cifrada leída.
type Indice struct {
	// Fotos, de la más reciente a la más antigua por fecha de captura.
	Fotos []Foto
	// Albumes, por título.
	Albumes []Album
}

// Galeria es la galería cifrada sobre un Drive de la bóveda. Se puede usar desde
// varias goroutines.
type Galeria struct {
	a     Almacen
	ahora func() time.Time
	nuevo func() string

	mu       sync.Mutex
	carpetas map[string]string
}

// Nueva crea la galería sobre `a`. `nuevoID` da ids de álbum (uuid); nil usa uno
// aleatorio del paquete.
func Nueva(a Almacen, nuevoID func() string) *Galeria {
	if nuevoID == nil {
		nuevoID = idAleatorio
	}
	return &Galeria{a: a, ahora: time.Now, nuevo: nuevoID, carpetas: map[string]string{}}
}

// todos los hijos de una carpeta, página a página.
func (g *Galeria) hijos(ctx context.Context, carpeta string) ([]api.Folder, []api.File, error) {
	var carpetas []api.Folder
	var ficheros []api.File
	vistas := map[string]bool{}
	for cursor := ""; ; {
		h, err := g.a.Children(ctx, carpeta, cursor)
		if err != nil {
			return nil, nil, err
		}
		for _, c := range h.Folders {
			if !vistas[c.ID] {
				vistas[c.ID] = true
				carpetas = append(carpetas, c)
			}
		}
		ficheros = append(ficheros, h.Files...)
		if cursor = h.NextCursor; cursor == "" {
			return carpetas, ficheros, nil
		}
	}
}

// hija es la carpeta `nombre` dentro de `padre`: si hay varias con el mismo nombre
// (dos dispositivos que la crearon a la vez), la de id menor, siempre la misma.
func (g *Galeria) hija(ctx context.Context, padre, nombre string, crear bool) (string, error) {
	carpetas, _, err := g.hijos(ctx, padre)
	if err != nil {
		return "", err
	}
	var ids []string
	for _, c := range carpetas {
		if c.Name == nombre {
			ids = append(ids, c.ID)
		}
	}
	if len(ids) > 0 {
		slices.Sort(ids)
		return ids[0], nil
	}
	if !crear {
		return "", nil
	}
	c, err := g.a.CreateFolder(ctx, padre, nombre)
	if err != nil {
		return "", err
	}
	return c.ID, nil
}

// carpeta resuelve (y recuerda) una de las carpetas de la galería: "fotos",
// "galeria" o una de sus subcarpetas. Sin `crear`, "" si no está.
func (g *Galeria) carpeta(ctx context.Context, cual string, crear bool) (string, error) {
	g.mu.Lock()
	id := g.carpetas[cual]
	g.mu.Unlock()
	if id != "" {
		return id, nil
	}
	var padre, nombre string
	switch cual {
	case "fotos":
		raiz, err := g.a.Root(ctx)
		if err != nil {
			return "", err
		}
		padre, nombre = raiz.ID, CarpetaFotos
	case "galeria":
		p, err := g.carpeta(ctx, "fotos", crear)
		if err != nil || p == "" {
			return "", err
		}
		padre, nombre = p, CarpetaGaleria
	default:
		p, err := g.carpeta(ctx, "galeria", crear)
		if err != nil || p == "" {
			return "", err
		}
		padre, nombre = p, cual
	}
	id, err := g.hija(ctx, padre, nombre, crear)
	if err != nil || id == "" {
		return "", err
	}
	g.mu.Lock()
	g.carpetas[cual] = id
	g.mu.Unlock()
	return id, nil
}

func (g *Galeria) leerFichero(ctx context.Context, id string, tope int64) ([]byte, error) {
	r, err := g.a.Download(ctx, id)
	if err != nil {
		return nil, err
	}
	defer func() { _ = r.Close() }()
	datos, err := io.ReadAll(io.LimitReader(r, tope+1))
	if err != nil {
		return nil, err
	}
	if int64(len(datos)) > tope {
		clear(datos)
		return nil, ErrEntrada
	}
	return datos, nil
}

// porNombre: el fichero `<id><ext>` de cada id; con dos del mismo nombre, el más
// reciente.
func porNombre(ficheros []api.File, ext string) map[string]api.File {
	out := map[string]api.File{}
	for _, f := range ficheros {
		id, ok := strings.CutSuffix(f.Name, ext)
		if !ok || !idValido(id) {
			continue
		}
		if ya, hay := out[id]; !hay || f.UpdatedAt.After(ya.UpdatedAt) {
			out[id] = f
		}
	}
	return out
}

// EsFoto dice si un fichero de «Fotos» es una imagen de la galería.
func EsFoto(f api.File) bool {
	return f.DeletedAt == nil && strings.HasPrefix(strings.ToLower(f.ContentType), "image/")
}

// Leer lee la galería cifrada entera. Una entrada que no se lee o no vale no la
// para: esa foto sale con lo que dice Drive y queda pendiente.
func (g *Galeria) Leer(ctx context.Context) (Indice, error) {
	fotos, err := g.carpeta(ctx, "fotos", false)
	if err != nil || fotos == "" {
		return Indice{}, err
	}
	_, ficheros, err := g.hijos(ctx, fotos)
	if err != nil {
		return Indice{}, err
	}
	entradas, err := g.leerTodas(ctx, CarpetaIndice, ".json")
	if err != nil {
		return Indice{}, err
	}
	crudos, err := g.leerTodas(ctx, CarpetaAlbumes, ".json")
	if err != nil {
		return Indice{}, err
	}
	var indice Indice
	for _, f := range ficheros {
		if !EsFoto(f) || !idValido(f.ID) {
			continue
		}
		indice.Fotos = append(indice.Fotos, juntar(f, entradas[f.ID]))
	}
	slices.SortFunc(indice.Fotos, func(a, b Foto) int {
		if c := b.Entrada.Tomada.Compare(a.Entrada.Tomada); c != 0 {
			return c
		}
		return strings.Compare(a.Fichero.ID, b.Fichero.ID)
	})
	for id, datos := range crudos {
		a, err := DecodificarAlbum(datos)
		if err == nil && a.ID == id {
			indice.Albumes = append(indice.Albumes, a)
		}
	}
	slices.SortFunc(indice.Albumes, func(a, b Album) int {
		if c := strings.Compare(strings.ToLower(a.Titulo), strings.ToLower(b.Titulo)); c != 0 {
			return c
		}
		return strings.Compare(a.ID, b.ID)
	})
	return indice, nil
}

// juntar decide qué se enseña de una foto: su entrada si vale y es suya; si no, lo
// que dice Drive.
func juntar(f api.File, datos []byte) Foto {
	foto := Foto{Fichero: f}
	if e, err := DecodificarEntrada(datos); datos != nil && err == nil && e.Foto == f.ID {
		foto.Entrada, foto.ConEntrada = e, true
		foto.Pendiente = e.Miniatura == "" || f.Checksum != "" && e.Suma != f.Checksum
		return foto
	}
	foto.Entrada = entradaDeDrive(f)
	foto.Pendiente = true
	return foto
}

func entradaDeDrive(f api.File) Entrada {
	fecha := f.CreatedAt
	if fecha.IsZero() {
		fecha = f.UpdatedAt
	}
	return Entrada{V: Version, Tipo: TipoFoto, Foto: f.ID, Nombre: f.Name, Tomada: fecha, Albumes: []string{}}
}

// leerTodas descifra los ficheros `<id><ext>` de una subcarpeta, unos pocos a la vez.
// Los que no se leen se quedan fuera (sin parar el resto); un fallo al listar, no.
func (g *Galeria) leerTodas(ctx context.Context, sub, ext string) (map[string][]byte, error) {
	carpeta, err := g.carpeta(ctx, sub, false)
	if err != nil || carpeta == "" {
		return map[string][]byte{}, err
	}
	_, ficheros, err := g.hijos(ctx, carpeta)
	if err != nil {
		return nil, err
	}
	nombres := porNombre(ficheros, ext)
	var (
		mu  sync.Mutex
		wg  sync.WaitGroup
		out = map[string][]byte{}
		sem = make(chan struct{}, lectores)
	)
	for id, f := range nombres {
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			datos, err := g.leerFichero(ctx, f.ID, MaxEntrada)
			if err != nil {
				return
			}
			mu.Lock()
			out[id] = datos
			mu.Unlock()
		}()
	}
	wg.Wait()
	return out, ctx.Err()
}

// entradaActual es la entrada de `foto` y el fichero que la guarda, si la hay.
func (g *Galeria) entradaActual(ctx context.Context, foto string) (Entrada, bool, error) {
	carpeta, err := g.carpeta(ctx, CarpetaIndice, false)
	if err != nil || carpeta == "" {
		return Entrada{}, false, err
	}
	_, ficheros, err := g.hijos(ctx, carpeta)
	if err != nil {
		return Entrada{}, false, err
	}
	f, ok := porNombre(ficheros, ".json")[foto]
	if !ok {
		return Entrada{}, false, nil
	}
	datos, err := g.leerFichero(ctx, f.ID, MaxEntrada)
	if err != nil {
		return Entrada{}, false, err
	}
	e, err := DecodificarEntrada(datos)
	if err != nil || e.Foto != foto {
		// Una entrada rota no se usa, pero tampoco se pierde: la nueva es otra
		// versión del mismo fichero.
		return Entrada{}, false, nil //nolint:nilerr // ver arriba
	}
	return e, true, nil
}

func (g *Galeria) escribirEntrada(ctx context.Context, e Entrada) error {
	if err := e.Validar(); err != nil {
		return err
	}
	carpeta, err := g.carpeta(ctx, CarpetaIndice, true)
	if err != nil {
		return err
	}
	datos, err := json.Marshal(e)
	if err != nil {
		return err
	}
	_, err = g.a.Upload(ctx, carpeta, e.Foto+".json", "application/json", bytes.NewReader(datos), int64(len(datos)))
	return err
}

// Indexar apunta en el índice la foto `foto`, cuyo contenido es `datos`: su fecha de
// captura (EXIF), sus medidas ya orientadas y su miniatura cifrada. Conserva la
// favorita y los álbumes de la entrada anterior. Si la entrada ya es de este
// contenido y tiene miniatura, no escribe nada.
func (g *Galeria) Indexar(ctx context.Context, foto api.File, datos []byte) error {
	if !idValido(foto.ID) {
		return ErrSinFoto
	}
	previa, hay, err := g.entradaActual(ctx, foto.ID)
	if err != nil {
		return err
	}
	if hay && previa.Miniatura != "" && foto.Checksum != "" && previa.Suma == foto.Checksum {
		return nil
	}
	e := entradaDeDrive(foto)
	if hay {
		e.Favorita, e.Albumes = previa.Favorita, previa.Albumes
	}
	e.Suma = foto.Checksum
	meta, _ := exif.Leer(datos) // sin EXIF no es un fallo
	switch {
	case meta.TomadaEn != nil:
		e.Tomada, e.Exif = meta.TomadaEn.UTC(), true
	case hay && previa.Exif:
		// Una edición puede salir sin EXIF: la fecha de captura es la de antes.
		e.Tomada, e.Exif = previa.Tomada, true
	}
	miniaturaJPEG, ancho, alto, err := Miniatura(ctx, datos, meta.Orientacion)
	switch {
	case err == nil:
		e.Ancho, e.Alto = ancho, alto
		if e.Miniatura, err = g.subirMiniatura(ctx, foto.ID, miniaturaJPEG); err != nil {
			return err
		}
	case ctx.Err() != nil:
		return ctx.Err()
	case hay:
		// Un formato que aquí no se decodifica (HEIC): se queda lo que hubiera.
		e.Miniatura, e.Ancho, e.Alto = previa.Miniatura, previa.Ancho, previa.Alto
	}
	e.Actualizada = g.ahora().UTC()
	return g.escribirEntrada(ctx, e)
}

func (g *Galeria) subirMiniatura(ctx context.Context, foto string, datos []byte) (string, error) {
	defer clear(datos)
	carpeta, err := g.carpeta(ctx, CarpetaMiniaturas, true)
	if err != nil {
		return "", err
	}
	subida, err := g.a.Upload(ctx, carpeta, foto+".jpg", "image/jpeg", bytes.NewReader(datos), int64(len(datos)))
	if err != nil {
		return "", err
	}
	return subida.ID, nil
}

// Reconstruir vuelve a indexar una foto desde su original en la bóveda: lo que se
// hace con una entrada que falta o es de otro contenido.
func (g *Galeria) Reconstruir(ctx context.Context, foto api.File) error {
	if foto.SizeBytes > render.MaxOriginal {
		return miniatura.ErrMedioNoSoportado
	}
	datos, err := g.leerFichero(ctx, foto.ID, render.MaxOriginal)
	if err != nil {
		return err
	}
	defer clear(datos)
	return g.Indexar(ctx, foto, datos)
}

// Miniatura hace la miniatura de una foto, ya orientada: JPEG de 512 px de lado como
// mucho, y las medidas de la foto orientada.
func Miniatura(ctx context.Context, datos []byte, orientacion uint16) ([]byte, int, int, error) {
	release, err := miniatura.TurnoConContexto(ctx)
	if err != nil {
		return nil, 0, 0, err
	}
	defer release()
	img, err := render.Imagen(ctx, datos, model.OrientacionDe(int(orientacion)), model.RecetaVacia())
	if err != nil {
		return nil, 0, 0, err
	}
	b := img.Bounds()
	reducida := miniatura.Reducir(img, LadoMiniatura)
	var out bytes.Buffer
	if err := jpeg.Encode(&out, render.AplanarSobreBlanco(reducida), &jpeg.Options{Quality: miniatura.CalidadJPEG}); err != nil {
		return nil, 0, 0, fmt.Errorf("galeriacifrada: codificar miniatura: %w", err)
	}
	return out.Bytes(), b.Dx(), b.Dy(), nil
}

// LeerMiniatura descifra la miniatura de una entrada.
func (g *Galeria) LeerMiniatura(ctx context.Context, e Entrada) ([]byte, error) {
	if e.Miniatura == "" {
		return nil, ErrSinFoto
	}
	return g.leerFichero(ctx, e.Miniatura, MaxMiniatura)
}

// Actualizar cambia la entrada de `foto` con `cambio`, releyéndola justo antes: lo
// que haya cambiado otro dispositivo en otro campo se conserva. Sin entrada, parte
// de lo que dice Drive de la foto.
func (g *Galeria) Actualizar(ctx context.Context, foto api.File, cambio func(*Entrada)) (Entrada, error) {
	if !idValido(foto.ID) {
		return Entrada{}, ErrSinFoto
	}
	e, hay, err := g.entradaActual(ctx, foto.ID)
	if err != nil {
		return Entrada{}, err
	}
	if !hay {
		e = entradaDeDrive(foto)
	}
	antes, _ := json.Marshal(e)
	cambio(&e)
	e.Albumes = sinRepetir(e.Albumes)
	despues, _ := json.Marshal(e)
	if hay && bytes.Equal(antes, despues) {
		return e, nil // nada que escribir: repetir el cambio no crea versiones
	}
	e.Actualizada = g.ahora().UTC()
	return e, g.escribirEntrada(ctx, e)
}

// CrearAlbum guarda un álbum nuevo.
func (g *Galeria) CrearAlbum(ctx context.Context, titulo string) (Album, error) {
	ahora := g.ahora().UTC()
	a := Album{V: Version, Tipo: TipoAlbum, ID: g.nuevo(), Titulo: strings.TrimSpace(titulo), Creado: ahora, Actualizado: ahora}
	if err := a.Validar(); err != nil {
		return Album{}, err
	}
	carpeta, err := g.carpeta(ctx, CarpetaAlbumes, true)
	if err != nil {
		return Album{}, err
	}
	datos, err := json.Marshal(a)
	if err != nil {
		return Album{}, err
	}
	if _, err := g.a.Upload(ctx, carpeta, a.ID+".json", "application/json", bytes.NewReader(datos), int64(len(datos))); err != nil {
		return Album{}, err
	}
	return a, nil
}

// DelAlbum son las fotos del índice que están en el álbum `id`.
func (i Indice) DelAlbum(id string) []Foto {
	var out []Foto
	for _, f := range i.Fotos {
		if slices.Contains(f.Entrada.Albumes, id) {
			out = append(out, f)
		}
	}
	return out
}

// Favoritas son las fotos marcadas.
func (i Indice) Favoritas() []Foto {
	var out []Foto
	for _, f := range i.Fotos {
		if f.Entrada.Favorita {
			out = append(out, f)
		}
	}
	return out
}

func idAleatorio() string { return uuid.NewString() }
