// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/apps/fyne/apps/photos"
	"git.sytue.eu/sytue/one/apps/fyne/local/imagenes"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/carrete"
	"git.sytue.eu/sytue/one/apps/nucleo/local/fototeca"
	privatePhotos "git.sytue.eu/sytue/one/apps/nucleo/photos"
)

// # Importar a la fototeca (FO-04, FO-09)
//
// Lo que en la conectada es la copia de seguridad a Drive (`apps/photos/copia.go`) aquí
// es IMPORTAR a la fototeca del equipo, con el MISMO motor (`photos.Copia`): cada
// elemento una sola vez, reconocido por el SHA-256 de su contenido, de lo más antiguo a
// lo más reciente, y tres fallos seguidos paran la ronda. Cambian las dos puntas:
//
//   - el DESTINO es el repositorio local ([destinoLocal]): «subir» es `ImportarFoto`, sin
//     servidor, y lo que ya está en la fototeca (también en su papelera) no se vuelve a
//     traer;
//   - el ORIGEN es la galería del sistema en el móvil (el carrete) o, en el escritorio,
//     una carpeta elegida o las carpetas vigiladas ([galeriaCarpetas]), filtrado a lo que
//     la fototeca sabe enseñar ([soloImportables]).
//
// De una carpeta del escritorio (la elegida o una vigilada) la foto se ENLAZA en vez de
// copiarse (FO-09, `ImportarExterno`): el original se queda donde está y la fototeca
// guarda sus derivadas; si la persona lo prefiere, «Copiar a la fototeca» las copia como
// antes. Los VÍDEOS de esas carpetas entran siempre enlazados, con su duración y un
// fotograma de póster (video.go). Lo que no tiene una ruta estable se COPIA: lo
// convertido (una HEIC pasada a JPEG en un temporal) y lo del carrete del teléfono, fotos
// y vídeos (estos con su póster y su duración, copiar.go).

// destinoLocal es la fototeca del equipo como destino del motor de la copia.
type destinoLocal struct {
	repo *fototeca.Fototeca
	cli  *api.Photos
	// carpetas son las del origen cuando es de carpetas: lo que está dentro de ellas se
	// enlaza (salvo copiar); nil en el carrete, donde siempre se copia.
	carpetas func() []string
	copiar   bool
	// medir saca la duración, los lados y un fotograma de un vídeo (video.go).
	medir func(ctx context.Context, ruta string) videoMedido
	// abiertos es lo que dijo el carrete de los vídeos que tiene abiertos: su duración y
	// sus lados por si no se pueden medir del fichero. nil fuera del carrete.
	abiertos *elementosAbiertos

	mu        sync.Mutex
	conocidas map[string]bool
}

func nuevoDestinoLocal(repo *fototeca.Fototeca, cli *api.Photos) *destinoLocal {
	return &destinoLocal{repo: repo, cli: cli}
}

// enlazable dice si ruta es un fichero de las carpetas del origen (y no un temporal de
// una conversión), que es lo que se puede enlazar.
func (d *destinoLocal) enlazable(ruta string) bool {
	if d.carpetas == nil {
		return false
	}
	return dentroDe(d.carpetas(), ruta)
}

// dentroDe dice si ruta está en alguna de las carpetas.
func dentroDe(carpetas []string, ruta string) bool {
	ruta = filepath.Clean(ruta)
	for _, base := range carpetas {
		if rel, err := filepath.Rel(base, ruta); err == nil && filepath.IsLocal(rel) {
			return true
		}
	}
	return false
}

// Contenido son las huellas de lo que ya está en la fototeca. Los nombres no cuentan:
// en la fototeca dos fotos pueden llamarse igual.
func (d *destinoLocal) Contenido(ctx context.Context) (map[string]bool, map[string]bool, error) {
	hs, err := d.repo.HuellasConocidas(ctx)
	if err != nil {
		return nil, nil, err
	}
	d.mu.Lock()
	d.conocidas = hs
	d.mu.Unlock()
	return hs, map[string]bool{}, nil
}

// Subir importa el fichero. Si su contenido ya está (importado a mano desde la última
// lectura), no lo duplica.
func (d *destinoLocal) Subir(ctx context.Context, ruta, nombre, tipo string) (photos.Subido, error) {
	f, err := os.Open(ruta) //nolint:gosec // lo deja el origen de la importación
	if err != nil {
		return photos.Subido{}, err
	}
	defer func() { _ = f.Close() }()
	h := sha256.New()
	tam, err := io.Copy(h, f)
	if err != nil {
		return photos.Subido{}, err
	}
	suma := hex.EncodeToString(h.Sum(nil))
	d.mu.Lock()
	if d.conocidas == nil {
		d.mu.Unlock()
		if _, _, err := d.Contenido(ctx); err != nil {
			return photos.Subido{}, err
		}
		d.mu.Lock()
	}
	ya := d.conocidas[suma]
	d.mu.Unlock()
	if ya {
		if foto, ok, err := d.repo.FotoPorHuella(ctx, suma); err == nil && ok {
			return photos.Subido{ID: foto.ID, Nombre: foto.Name}, nil
		}
		return photos.Subido{Nombre: nombre}, nil
	}
	var foto api.Photo
	video := carrete.EsVideo(tipo)
	switch {
	case d.enlazable(ruta) && (video || !d.copiar):
		foto, err = d.enlazar(ctx, ruta, tipo, video)
	case video:
		// Sin ruta estable (el carrete del teléfono): se copia, con su póster (FO-09).
		foto, err = copiarVideo(ctx, d.repo, d.medir, ruta, nombre, tipo, d.abiertos.de(ruta))
	default:
		if _, err := f.Seek(0, io.SeekStart); err != nil {
			return photos.Subido{}, err
		}
		c, fin := context.WithTimeout(ctx, photos.PlazoSubida(tam))
		defer fin()
		foto, err = d.cli.ImportarFoto(c, uuid.NewString(), nombre, tipo, f, tam)
	}
	if err != nil {
		return photos.Subido{}, err
	}
	d.mu.Lock()
	d.conocidas[suma] = true
	d.mu.Unlock()
	return photos.Subido{ID: foto.ID, Nombre: foto.Name}, nil
}

// enlazar deja en la fototeca el fichero de ruta SIN copiarlo (FO-09), con el nombre
// del fichero: es ese fichero.
func (d *destinoLocal) enlazar(ctx context.Context, ruta, tipo string, video bool) (api.Photo, error) {
	abs, err := filepath.Abs(ruta)
	if err != nil {
		return api.Photo{}, err
	}
	o := privatePhotos.OrigenExterno{Ruta: abs, Tipo: tipo}
	if video {
		if d.medir != nil {
			m := d.medir(ctx, abs)
			o.Poster, o.Duracion, o.Ancho, o.Alto = m.poster, m.duracion, m.ancho, m.alto
		}
		if info, err := os.Stat(abs); err == nil {
			cuando := info.ModTime()
			o.TomadoEn = &cuando
		}
	} else {
		o.Tipo = "" // la mide su cabecera
	}
	return d.repo.ImportarExterno(ctx, uuid.NewString(), o)
}

// Indexar no hace nada: ImportarFoto ya la deja en la línea de tiempo.
func (*destinoLocal) Indexar(context.Context, string) error { return nil }

// importable dice si la fototeca sabe enseñar un tipo: los que mide en el dispositivo
// (JPEG, PNG, GIF, WebP, AVIF, BMP) y HEIC si este equipo sabe convertirlo.
func importable(tipo string, heic bool) bool {
	switch tipo {
	case "image/jpeg", "image/png", "image/gif", "image/webp", "image/avif", imagenes.TipoBMP:
		return true
	}
	return heic && imagenes.EsHEIC(tipo)
}

// tipoDeFoto es el tipo de un elemento de la galería o de una carpeta: el que dice el
// sistema o, si no dice, el de su extensión (`carrete.TipoDe`, que no nombra el BMP: la
// galería de la conectada no lo enseña). Un BMP es siempre `image/bmp`, que es como lo
// mide la fototeca por su cabecera, aunque el sistema lo llame `image/x-ms-bmp`.
func tipoDeFoto(nombre, tipo string) string {
	if tipo == "" {
		tipo = carrete.TipoDe(nombre)
	}
	if imagenes.EsBMP(tipo) || tipo == "" && strings.EqualFold(filepath.Ext(nombre), ".bmp") {
		return imagenes.TipoBMP
	}
	return tipo
}

// videoImportable dice si un vídeo entra en la galería (FO-09): los contenedores que
// reconocen el conversor y el reproductor (MP4, MOV, M4V, WebM, MKV y los demás de
// `carrete.TipoDe`). Reproducirlo aquí depende de lo que haya en el equipo (video.go);
// si no, se abre en Cast o en el reproductor del sistema.
func videoImportable(tipo string) bool { return carrete.EsVideo(tipo) }

// soloImportables es una galería del sistema sin lo que la fototeca no sabe enseñar
// (RAW; HEIC donde no hay con qué convertirlo; los vídeos, si no se piden): así no se
// apuntan como hechos y se traerán el día que sepa. Y convierte con el sistema lo HEIC,
// pero deja WebP, AVIF y BMP tal cual, que la fototeca local sí mide (la conectada los pasaba
// a JPEG).
type soloImportables struct {
	photos.GaleriaSistema
	heic bool
	// videos deja pasar los vídeos (FO-09): los de las carpetas del escritorio y los del
	// carrete del teléfono.
	videos bool
	// abiertos, si no es nil, apunta lo que dijo la galería de cada vídeo que abre.
	abiertos *elementosAbiertos
}

// Abrir es el de la galería; de un vídeo apunta además lo que dijo el sistema (su
// duración y sus lados) mientras está abierto, para quien lo copie.
func (g soloImportables) Abrir(ctx context.Context, e carrete.Elemento) (string, func(), error) {
	ruta, limpiar, err := g.GaleriaSistema.Abrir(ctx, e)
	if err != nil || g.abiertos == nil || !e.Video {
		return ruta, limpiar, err
	}
	g.abiertos.apuntar(ruta, e)
	return ruta, func() { g.abiertos.olvidar(ruta); limpiar() }, nil
}

// Listar es el de la galería, filtrado.
func (g soloImportables) Listar(ctx context.Context) ([]carrete.Elemento, error) {
	l, err := g.GaleriaSistema.Listar(ctx)
	if err != nil {
		return nil, err
	}
	out := l[:0]
	for _, e := range l {
		tipo := tipoDeFoto(e.Nombre, e.Tipo)
		if imagenes.EsBMP(e.Tipo) {
			e.Tipo = tipo // el que mide la fototeca
		}
		if !e.Video && importable(tipo, g.heic) || g.videos && (e.Video || carrete.EsVideo(tipo)) && videoImportable(tipo) {
			out = append(out, e)
		}
	}
	return out, nil
}

// Convertir solo pasa a JPEG lo HEIC.
func (g soloImportables) Convertir(ctx context.Context, ruta string) (string, func(), error) {
	if !imagenes.EsHEIC(ruta) && !imagenes.EsHEIC(carrete.TipoDe(ruta)) {
		return "", func() {}, carrete.ErrNoDisponible
	}
	return imagenes.Convertir(ctx, ruta)
}

// galeriaCarpetas es una «galería del sistema» hecha de carpetas del equipo: la que se
// elige para importar una vez y las vigiladas. No hace falta permiso ni hay datos móviles.
type galeriaCarpetas struct {
	carpetas func() []string
}

func (galeriaCarpetas) Disponible() bool         { return true }
func (galeriaCarpetas) Permiso() carrete.Permiso { return carrete.Concedido }
func (galeriaCarpetas) PedirPermiso(context.Context) (carrete.Permiso, error) {
	return carrete.Concedido, nil
}
func (galeriaCarpetas) RedMedida() bool { return false }
func (galeriaCarpetas) Guardar(context.Context, string, string) (string, error) {
	return "", carrete.ErrNoDisponible
}

func (galeriaCarpetas) Convertir(context.Context, string) (string, func(), error) {
	return "", func() {}, carrete.ErrNoDisponible
}

// profundidadCarpeta es hasta dónde se baja en una carpeta: la misma que la galería del
// escritorio (`carrete.ProfundidadMaxima`).
const profundidadCarpeta = carrete.ProfundidadMaxima

// Listar recorre las carpetas: fotos y vídeos, sin lo oculto. El ID es la ruta.
func (g galeriaCarpetas) Listar(ctx context.Context) ([]carrete.Elemento, error) {
	var out []carrete.Elemento
	vistos := map[string]bool{}
	for _, base := range g.carpetas() {
		err := filepath.WalkDir(base, func(ruta string, e fs.DirEntry, err error) error {
			el, ok, err := elementoEnCarpeta(ctx, base, ruta, e, err, vistos)
			if ok {
				vistos[ruta] = true
				out = append(out, el)
			}
			return err
		})
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}

// elementoEnCarpeta es el paso de WalkDir por ruta, dentro de base: la foto o el vídeo
// que hay ahí (ok), o lo que el recorrido tiene que hacer. Lo oculto, lo que no se puede
// leer, lo que pasa de profundidadCarpeta y lo ya visto se saltan.
func elementoEnCarpeta(ctx context.Context, base, ruta string, e fs.DirEntry, err error, vistos map[string]bool) (carrete.Elemento, bool, error) {
	if err != nil {
		if e != nil && e.IsDir() && ruta != base {
			return carrete.Elemento{}, false, fs.SkipDir
		}
		return carrete.Elemento{}, false, nil
	}
	if cerr := ctx.Err(); cerr != nil {
		return carrete.Elemento{}, false, cerr
	}
	if e.IsDir() {
		rel, _ := filepath.Rel(base, ruta)
		if ruta != base && (strings.HasPrefix(e.Name(), ".") || strings.Count(rel, string(filepath.Separator)) >= profundidadCarpeta) {
			return carrete.Elemento{}, false, fs.SkipDir
		}
		return carrete.Elemento{}, false, nil
	}
	if !e.Type().IsRegular() || strings.HasPrefix(e.Name(), ".") || vistos[ruta] {
		return carrete.Elemento{}, false, nil
	}
	tipo := tipoDeFoto(e.Name(), "")
	if tipo == "" {
		return carrete.Elemento{}, false, nil
	}
	info, err := e.Info()
	if err != nil {
		return carrete.Elemento{}, false, nil //nolint:nilerr // un fichero que desaparece mientras se recorre no es un fallo
	}
	return carrete.Elemento{
		ID: ruta, Nombre: e.Name(), Tipo: tipo, Tomada: info.ModTime(),
		Tamano: info.Size(), Video: carrete.EsVideo(tipo),
	}, true, nil
}

// Abrir no copia: el original ya es un fichero, y tiene que estar en una de las carpetas.
func (g galeriaCarpetas) Abrir(_ context.Context, e carrete.Elemento) (string, func(), error) {
	nada := func() {}
	ruta := filepath.Clean(e.ID)
	for _, base := range g.carpetas() {
		if rel, err := filepath.Rel(base, ruta); err == nil && filepath.IsLocal(rel) {
			if _, err := os.Stat(ruta); err != nil {
				return "", nada, err
			}
			return ruta, nada, nil
		}
	}
	return "", nada, fs.ErrNotExist
}
