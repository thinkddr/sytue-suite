// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package traer

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/carrete"
	sqlfotos "git.sytue.eu/sytue/one/apps/nucleo/local/fototeca"
	privatePhotos "git.sytue.eu/sytue/one/apps/nucleo/photos"
)

// # Photos
//
// La entrega de Photos son dos tablas (`photos/photos.<formato>` y
// `photos/albums.<formato>`, en JSON, CSV o XML: tabla.go) con lo que Photos sabe de cada
// foto y no está en el fichero —si es favorita, su descripción y sus etiquetas, en qué
// álbumes está— y cuándo se hizo y cuándo se subió. Los BYTES no van ahí: en Sytue el
// original de una foto es un fichero de Drive, y llega en `drive/files/` (casi siempre en
// `Fotos/`). La tabla no dice cuál: se casa por el nombre y el tamaño, prefiriendo el de
// `Fotos/`.
//
// Cada original se deja en Drive, en la carpeta de lo traído, como el resto de Drive
// (una sola vez: memoria.go), y Photos lo ENLAZA desde allí (`ImportarExterno`), igual
// que enlaza las fotos de una carpeta del equipo (FO-09): la fototeca guarda la ruta, la
// huella y las miniaturas, y el fichero no se copia dos veces. Es lo mismo que en la
// edición conectada, donde el original de Photos es un fichero de Drive. Si luego falta o
// cambia, Photos lo dice sin romper la galería, como con cualquier enlazada. Entra igual:
//
//   - una foto en lo que la fototeca de este proceso mide: JPEG, PNG, GIF, WebP y, si
//     quien llama ha registrado sus decodificadores (el Hub, con `local/imagenes`), AVIF
//     y BMP, como Photos;
//   - un VÍDEO, con su duración, sus lados y un fotograma de póster que mide quien llama
//     ([Destino.Video], lo mismo que usa Photos); sin eso, con los lados de la tabla y un
//     póster neutro;
//   - una HEIC no se puede enlazar (no hay decodificador en Go: `local/imagenes`): como
//     en Photos, entra el JPEG que saca el sistema ([Destino.HEIC]), COPIADO a la
//     fototeca, y la HEIC original se queda en Drive. Se reconoce otra vez por su huella
//     (memoria.go apunta la del JPEG). Sin conversor, no entra y se dice.
//
// Lo que la fototeca no sabe abrir en ESTE equipo (un formato que no mide, una HEIC sin
// conversor) se queda en Drive como un fichero más, y el resumen lo cuenta aparte
// ([Parte.SinAbrir]).
//
// Una foto que Photos ya tiene (por la huella de sus bytes, también si está en su
// papelera) no se vuelve a traer ni a copiar: así, traer otra vez la misma copia no
// repite ninguna. Si la que tiene es una enlazada cuyo fichero ya no está, se busca (la
// persona pudo mover la carpeta) o se deja otra vez en Drive, y se recoloca allí.
//
// De cada foto nueva se ponen su favorita, su descripción y sus etiquetas, su fecha de
// captura si el fichero no la dice y la de SUBIDA (con la que Sytue ponía en la línea de
// tiempo lo que no tiene fecha de captura); y los álbumes que tengan alguna foto nueva se
// crean (o, si ya están, se les añaden las nuevas). Un álbum se reconoce por su
// identificador en Sytue, que se guarda como su origen en Photos: traerlo otra vez no
// duplica uno renombrado ni junta dos con el mismo título; uno que la persona borró no
// vuelve. Solo una tabla sin identificadores se casa por el título. Lo que la fototeca
// saca del propio fichero (los lados, la cámara, el lugar) no se toma de la tabla.

// plazoDeMedir es lo más que se espera a medir un vídeo o a convertir una HEIC con lo
// del sistema (lo que espera Photos): uno que se cuelga no para el resto.
const plazoDeMedir = 2 * time.Minute

// fotoDeSytue es una fila de `photos/photos.<formato>`.
type fotoDeSytue struct {
	id, nombre  string
	bytes       int64
	favorita    bool
	descripcion string
	etiquetas   []string
	fila        fila
}

// clase es lo que es un elemento de Photos para traerlo.
type clase int

const (
	claseFoto clase = iota
	claseVideo
	claseHEIC
)

// claseDe dice qué es un elemento de Photos, por el tipo que dice la tabla o, si no
// dice, por su nombre; y, si es un vídeo, su tipo.
func claseDe(nombre, tipo string) (clase, string) {
	tipo = strings.ToLower(strings.TrimSpace(strings.Split(tipo, ";")[0]))
	porNombre := carrete.TipoDe(nombre)
	switch {
	case carrete.EsVideo(tipo):
		return claseVideo, tipo
	case carrete.EsVideo(porNombre):
		return claseVideo, porNombre
	case esHEIC(tipo) || esHEIC(porNombre):
		return claseHEIC, ""
	}
	return claseFoto, ""
}

// esHEIC dice si un tipo es HEIC o HEIF (también en secuencia).
func esHEIC(tipo string) bool {
	switch tipo {
	case "image/heic", "image/heif", "image/heic-sequence", "image/heif-sequence":
		return true
	}
	return false
}

// tablaDe busca, entre es, la tabla `<producto>/<nombre>.<formato>`.
func tablaDe(es []entrada, producto, nombre string) (entrada, bool) {
	for _, e := range es {
		base := strings.TrimPrefix(e.ruta, producto+"/")
		ext := path.Ext(base)
		if strings.TrimSuffix(base, ext) == nombre && slices.Contains([]string{".json", ".csv", ".xml"}, strings.ToLower(ext)) {
			return e, true
		}
	}
	return entrada{}, false
}

// leerFilas lee la tabla e.
func leerFilas(e entrada) ([]fila, error) {
	datos, err := e.leer()
	if err != nil {
		return nil, err
	}
	return leerTabla(datos, path.Ext(e.ruta))
}

// casada es una fila de la tabla con su original en la copia.
type casada struct {
	foto fotoDeSytue
	e    entrada
}

// fototeca es Photos abierta mientras se traen sus fotos, con lo que se lleva hecho.
type fototeca struct {
	repo      *sqlfotos.Fototeca
	conocidas map[string]bool   // las huellas que ya tiene (también en la papelera)
	locales   map[string]string // id en Sytue → id aquí
	nuevas    map[string]bool   // ids de aquí que entran ahora
	p         *Parte
	errs      []error
	// perdidas: algo de la tabla no ha entrado, así que las tablas se dejan en Drive.
	perdidas bool
}

// fotos lleva a Photos las fotos de la copia, con sus originales de drive. Devuelve lo de
// photos que hay que copiar tal cual y lo de drive que queda por repartir.
//
//nolint:contextcheck // el contexto va en r.ctx; abrir y cerrar el almacén y el repositorio de Photos no lo piden, y se cierran también si se canceló
func (r *repartidor) fotos(es, drive []entrada) (restoFotos, restoDrive []entrada, err error) {
	tablaFotos, ok := tablaDe(es, "photos", "photos")
	if !ok {
		return es, drive, nil
	}
	filas, err := leerFilas(tablaFotos)
	if err != nil {
		return es, drive, err
	}
	var albumes []fila
	if t, ok := tablaDe(es, "photos", "albums"); ok {
		if albumes, err = leerFilas(t); err != nil {
			return es, drive, err
		}
	}

	ft := &fototeca{locales: map[string]string{}, nuevas: map[string]bool{}, p: r.parte("photos")}
	casadas, usado := casarConOriginales(filas, drive, ft)
	for i, e := range drive {
		if !usado[i] {
			restoDrive = append(restoDrive, e)
		}
	}
	defer func() {
		// Las tablas ya están dentro; se copian solo si algo se quedó fuera, para que no se
		// pierda lo que decían de ello.
		for _, e := range es {
			if (e.ruta != tablaFotos.ruta && !esTablaDeAlbumes(e)) || ft.perdidas {
				restoFotos = append(restoFotos, e)
			}
		}
	}()
	if len(casadas) == 0 {
		return nil, restoDrive, nil
	}
	// Si Photos no se puede abrir, sus originales van a Drive como los demás ficheros.
	sinPhotos := func(err error) ([]entrada, []entrada, error) {
		ft.perdidas = true
		for _, c := range casadas {
			restoDrive = append(restoDrive, c.e)
		}
		return nil, restoDrive, err
	}

	// La fototeca de SQLite de Photos (`local/fototeca`): si aún estaba en el almacén de
	// antes, abrirla la migra. Mide lo mismo que Photos al importar (lo que este proceso
	// sepa leer: AVIF y BMP, si quien llama ha registrado sus decodificadores).
	repo, err := sqlfotos.Abrir(r.ctx, filepath.Join(r.d.Datos, "Datos", "photos"), r.d.Sujeto, sqlfotos.Opciones{Formatos: []string{"webp", "avif", "bmp"}})
	if err != nil {
		return sinPhotos(err)
	}
	defer repo.Close() //nolint:contextcheck // cerrar no depende de ctx: se cierra también si ya se canceló
	if ft.conocidas, err = repo.HuellasConocidas(r.ctx); err != nil {
		return sinPhotos(err)
	}
	ft.repo = repo

	aDrive, err := r.llevarCasadas(ft, casadas)
	restoDrive = append(restoDrive, aDrive...)
	if err != nil {
		return nil, restoDrive, err
	}
	if err := r.albumes(repo, albumes, ft.locales, ft.nuevas); err != nil {
		ft.perdidas = true
		ft.errs = append(ft.errs, err)
	}
	return nil, restoDrive, errors.Join(ft.errs...)
}

// casarConOriginales empareja cada foto de la tabla con su original de drive, por su
// nombre y su tamaño. Una sin original en la copia cuenta como fallida en ft: solo queda
// en la tabla. usado dice qué entradas de drive han quedado emparejadas.
func casarConOriginales(filas []fila, drive []entrada, ft *fototeca) (casadas []casada, usado []bool) {
	// Los originales candidatos, por su nombre y su tamaño.
	type clave struct {
		nombre string
		bytes  int64
	}
	candidatos := map[clave][]int{}
	for i, e := range drive {
		if strings.HasPrefix(e.ruta, "drive/files/") {
			k := clave{path.Base(e.ruta), int64(e.f.UncompressedSize64)} //nolint:gosec // lo mide el ZIP
			candidatos[k] = append(candidatos[k], i)
		}
	}
	usado = make([]bool, len(drive))
	for _, f := range filas {
		foto := fotoDeSytue{
			id: f.texto("id"), nombre: f.texto("name"), bytes: f.entero("size_bytes"),
			favorita: f.si("favorite"), descripcion: f.texto("description"), etiquetas: f.lista("tags"), fila: f,
		}
		var libres []int
		for _, i := range candidatos[clave{foto.nombre, foto.bytes}] {
			if !usado[i] {
				libres = append(libres, i)
			}
		}
		if len(libres) == 0 {
			ft.p.Fallidos++ // sin su original en la copia: solo queda en la tabla
			ft.perdidas = true
			continue
		}
		// El de `Fotos/` (la carpeta de los originales de Photos) y, si no, el primero.
		elegido := libres[0]
		for _, i := range libres {
			if strings.HasPrefix(drive[i].ruta, "drive/files/Fotos/") {
				elegido = i
				break
			}
		}
		usado[elegido] = true
		casadas = append(casadas, casada{foto, drive[elegido]})
	}
	return casadas, usado
}

// llevarCasadas mete en Photos cada foto con su original. Lo que no se deja leer cuenta
// como fallido y se devuelve, para que vaya a Drive como los demás ficheros.
func (r *repartidor) llevarCasadas(ft *fototeca, casadas []casada) (aDrive []entrada, err error) {
	for _, c := range casadas {
		if err := r.ctx.Err(); err != nil {
			return aDrive, err
		}
		suma, err := huellaDeEntrada(c.e)
		if err != nil {
			ft.p.Fallidos++
			ft.perdidas = true
			aDrive = append(aDrive, c.e)
			continue
		}
		rel := path.Join("Drive", strings.TrimPrefix(c.e.ruta, "drive/files/"))
		que, tipo := claseDe(c.foto.nombre, c.foto.fila.texto("content_type"))
		switch {
		case que == claseHEIC:
			r.heic(ft, c, suma, rel)
		case ft.conocidas[suma]:
			r.yaEstaba(ft, c, suma, suma, rel)
		default:
			r.enlazar(ft, c, suma, rel, que == claseVideo, tipo)
		}
	}
	return aDrive, nil
}

// yaEstaba cuenta la foto c, cuyo original (huella original) ya tiene Photos (como foto
// de huella enPhotos: la misma, o el JPEG de una HEIC), y la apunta para sus álbumes. Si
// está enlazada y su fichero falta, lo busca en Drive (la persona pudo mover la carpeta)
// o lo deja otra vez, y Photos lo recoloca.
func (r *repartidor) yaEstaba(ft *fototeca, c casada, original, enPhotos, rel string) {
	ft.p.Repetidos++
	ya, viva, err := ft.repo.FotoPorHuella(r.ctx, enPhotos)
	switch {
	case err != nil:
		ft.errs = append(ft.errs, err)
	case !viva:
		// La persona la tiró a la papelera de Photos: no vuelve, como no vuelve a entrar
		// del carrete ni de una carpeta vigilada (FO-04).
	default:
		ft.locales[c.foto.id] = ya.ID
		if ya.Externo != "" && ya.EstadoExterno != "" {
			donde, err := r.original(c.e, rel, original)
			if err == nil {
				_, err = ft.repo.Recolocar(r.ctx, []string{donde})
			}
			ft.errs = append(ft.errs, err)
		}
	}
}

// enlazar deja en Drive el original de la foto o del vídeo c (si no está ya de otra
// vuelta) y Photos lo enlaza desde allí, con sus fechas, su favorita, su descripción y
// sus etiquetas. Si Photos no lo sabe abrir, se queda en Drive.
func (r *repartidor) enlazar(ft *fototeca, c casada, suma, rel string, video bool, tipo string) {
	donde, err := r.original(c.e, rel, suma)
	if err != nil {
		ft.p.Fallidos++
		ft.perdidas = true
		ft.errs = append(ft.errs, err)
		return
	}
	o := privatePhotos.OrigenExterno{
		Ruta: donde, Nombre: c.foto.nombre,
		TomadoEn: c.foto.fila.hora("taken_at"), SubidoEn: c.foto.fila.hora("uploaded_at"),
	}
	if video {
		var v Video
		if r.d.Video != nil {
			ctx, fin := context.WithTimeout(r.ctx, plazoDeMedir)
			v = r.d.Video(ctx, donde)
			fin()
		}
		o.Tipo, o.Poster, o.Duracion, o.Ancho, o.Alto = tipo, v.Poster, v.Duracion, v.Ancho, v.Alto
		if o.Ancho <= 0 || o.Alto <= 0 {
			o.Ancho, o.Alto = int(c.foto.fila.entero("width")), int(c.foto.fila.entero("height"))
		}
	}
	foto, err := ft.repo.ImportarExterno(r.ctx, uuid.NewString(), o)
	if err != nil {
		if r.ctx.Err() != nil {
			ft.errs = append(ft.errs, r.ctx.Err())
			return
		}
		// No es algo que la fototeca sepa enseñar en este equipo: se queda en Drive, donde
		// ya está.
		ft.p.SinAbrir++
		ft.perdidas = true
		return
	}
	ft.conocidas[suma] = true
	r.nueva(ft, c, foto)
}

// heic lleva a Photos la foto HEIC c como el JPEG que saca el sistema, copiado a la
// fototeca (como Photos), y deja la HEIC original en Drive. Sin conversor, o si no se
// puede convertir, la HEIC se queda en Drive y se dice.
func (r *repartidor) heic(ft *fototeca, c casada, suma, rel string) {
	if jpeg := r.mem.Convertidas[suma]; jpeg != "" && ft.conocidas[jpeg] {
		r.yaEstaba(ft, c, suma, jpeg, rel)
		return
	}
	donde, err := r.original(c.e, rel, suma)
	if err != nil {
		ft.p.Fallidos++
		ft.perdidas = true
		ft.errs = append(ft.errs, err)
		return
	}
	sinAbrir := func() {
		ft.p.SinAbrir++
		ft.perdidas = true
	}
	if r.d.HEIC == nil {
		sinAbrir()
		ft.p.SinConversor++
		return
	}
	ctx, fin := context.WithTimeout(r.ctx, plazoDeMedir)
	ruta, limpiar, err := r.d.HEIC(ctx, donde)
	fin()
	if limpiar != nil {
		defer limpiar()
	}
	if err != nil {
		sinAbrir()
		return
	}
	jpeg, err := os.ReadFile(ruta) //nolint:gosec // G304: el JPEG que acaba de dejar el conversor
	if err != nil || len(jpeg) == 0 {
		sinAbrir()
		return
	}
	s := sha256.Sum256(jpeg)
	enPhotos := hex.EncodeToString(s[:])
	r.mem.Convertidas[suma] = enPhotos
	if ft.conocidas[enPhotos] {
		r.yaEstaba(ft, c, suma, enPhotos, rel)
		return
	}
	nombre := strings.TrimSuffix(c.foto.nombre, path.Ext(c.foto.nombre)) + ".jpg"
	foto, err := ft.repo.ImportarFoto(r.ctx, uuid.NewString(), nombre, "image/jpeg", bytes.NewReader(jpeg), int64(len(jpeg)))
	if err != nil {
		sinAbrir()
		return
	}
	// La fecha que diga la copia, si el conversor no ha guardado la de captura (ffmpeg no
	// copia los metadatos), y siempre la de subida.
	if err := ft.repo.PonerFechas(r.ctx, foto.ID, c.foto.fila.hora("taken_at"), c.foto.fila.hora("uploaded_at")); err != nil {
		ft.errs = append(ft.errs, err)
	}
	ft.conocidas[enPhotos] = true
	r.nueva(ft, c, foto)
}

// nueva apunta la foto que acaba de entrar y le pone su favorita, su descripción y sus
// etiquetas.
func (r *repartidor) nueva(ft *fototeca, c casada, foto api.Photo) {
	ft.locales[c.foto.id], ft.nuevas[foto.ID] = foto.ID, true
	ft.p.Importados++
	if c.foto.favorita {
		ft.errs = append(ft.errs, ft.repo.SetFavorite(r.ctx, foto.ID, true))
	}
	if c.foto.descripcion != "" || len(c.foto.etiquetas) > 0 {
		ft.errs = append(ft.errs, ft.repo.SetDescription(r.ctx, foto.ID, c.foto.descripcion, c.foto.etiquetas))
	}
}

// esTablaDeAlbumes dice si e es `photos/albums.<formato>`.
func esTablaDeAlbumes(e entrada) bool {
	t, ok := tablaDe([]entrada{e}, "photos", "albums")
	return ok && t.ruta == e.ruta
}

// origenDeAlbum es el origen en Photos del álbum de Sytue id.
func origenDeAlbum(id string) string { return "sytue:" + id }

// albumes crea los álbumes que tienen alguna foto nueva, con todas las suyas que hay
// aquí; si ya está (de otra vuelta), le añade solo las nuevas. Uno sin fotos nuevas no
// se toca: traer otra vez la misma copia no cambia nada. Un álbum con identificador se
// reconoce por él (su origen en Photos), aunque se haya renombrado, y dos con el mismo
// título son dos; uno que la persona borró no vuelve. Sin identificador, por su título.
//
//nolint:gocognit // las dos maneras de reconocer un álbum, cada una con su forma de fallar (leer los álbumes corta; crear uno sigue con los demás), se leen mejor juntas
func (r *repartidor) albumes(repo *sqlfotos.Fototeca, filas []fila, locales map[string]string, nuevas map[string]bool) error {
	if len(filas) == 0 || len(nuevas) == 0 {
		return nil
	}
	porOrigen, err := repo.AlbumesPorOrigen(r.ctx)
	if err != nil {
		return err
	}
	var hay []api.Album // los de aquí, para casar por título; se leen si hace falta
	var errs []error
	for _, f := range filas {
		titulo, id := f.texto("title"), f.texto("id")
		todas, recien := fotosDeAlbum(f, locales, nuevas)
		if len(recien) == 0 || titulo == "" {
			continue
		}
		var album string
		poner := recien
		if id != "" {
			ya, ok := porOrigen[origenDeAlbum(id)]
			switch {
			case ok && ya.Borrado:
				continue
			case ok:
				album = ya.ID
			default:
				a, err := repo.CrearAlbumDe(r.ctx, titulo, origenDeAlbum(id))
				if err != nil {
					errs = append(errs, fmt.Errorf("álbum %q: %w", titulo, err))
					continue
				}
				porOrigen[origenDeAlbum(id)] = privatePhotos.AlbumDeOrigen{ID: a.ID}
				album, poner = a.ID, todas
			}
		} else {
			if hay == nil {
				if hay, err = repo.Albums(r.ctx); err != nil {
					return errors.Join(append(errs, err)...)
				}
			}
			if i := slices.IndexFunc(hay, func(a api.Album) bool { return a.Title == titulo }); i >= 0 {
				album = hay[i].ID
			} else {
				a, err := repo.CreateAlbum(r.ctx, titulo)
				if err != nil {
					errs = append(errs, fmt.Errorf("álbum %q: %w", titulo, err))
					continue
				}
				hay, album, poner = append(hay, a), a.ID, todas
			}
		}
		if _, err := repo.AddToAlbum(r.ctx, album, poner); err != nil {
			errs = append(errs, fmt.Errorf("álbum %q: %w", titulo, err))
		}
	}
	return errors.Join(errs...)
}

// fotosDeAlbum son las fotos de un álbum de la copia que ya están en Photos (todas, por
// su id local, sin repetir) y, de ellas, las que entran en esta vuelta (recien).
func fotosDeAlbum(f fila, locales map[string]string, nuevas map[string]bool) (todas, recien []string) {
	for _, foto := range f.lista("photos") {
		if l, ok := locales[strings.TrimSpace(foto)]; ok && !slices.Contains(todas, l) {
			todas = append(todas, l)
			if nuevas[l] {
				recien = append(recien, l)
			}
		}
	}
	return todas, recien
}

// huellaDeEntrada es el SHA-256 de los bytes de e, en hexadecimal.
func huellaDeEntrada(e entrada) (string, error) {
	if e.f.UncompressedSize64 > MaxFichero {
		return "", fmt.Errorf("traer: %s es demasiado grande", e.ruta)
	}
	rc, err := e.f.Open()
	if err != nil {
		return "", err
	}
	defer rc.Close()
	h := sha256.New()
	if _, err := io.Copy(h, io.LimitReader(rc, int64(e.f.UncompressedSize64)+1)); err != nil { //nolint:gosec // tope arriba
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
