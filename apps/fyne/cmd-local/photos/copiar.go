// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/apps/fyne/apps/photos"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/carrete"
	"git.sytue.eu/sytue/one/apps/nucleo/ficheros"
	"git.sytue.eu/sytue/one/apps/nucleo/local/fototeca"
	privatePhotos "git.sytue.eu/sytue/one/apps/nucleo/photos"
)

// # Vídeos COPIADOS a la fototeca (FO-09)
//
// De una carpeta del escritorio un vídeo se enlaza (importar.go). Por las otras puertas se
// COPIA, como una foto: el del carrete del teléfono (que llega a un temporal: allí no hay
// ruta estable a la que enlazar), el que se elige con «Añadir fotos» y el que se suelta
// en la ventana. Se mide igual que uno enlazado (medirVideo: duración, lados y póster) y
// lo que no se pueda medir lo pone lo que dijo el carrete (MediaStore, PhotoKit). Su
// original queda en la fototeca (`fototeca.Fototeca.ImportarVideo`).
//
// Para VERLO, el visor y Cast necesitan un fichero con su extensión, y la fototeca lo
// guarda por su huella y sin ella: copiasParaVer deja en la caché de la app un enlace a
// él (simbólico o duro; si no se puede, una copia).

// extensionesVideo son las de los vídeos que «Añadir fotos» deja elegir: las que reconoce
// `carrete.TipoDe`, que es lo que la galería sabe enseñar (video.go).
var extensionesVideo = []string{"mp4", "mov", "m4v", "3gp", "webm", "mkv", "avi"}

// extensionesParaAnadir son las de «Añadir fotos»: las imágenes de siempre y los vídeos.
func extensionesParaAnadir() []string {
	return append(append([]string{}, ficheros.ExtensionesImagen...), extensionesVideo...)
}

// tipoDeVideo es el MIME de un vídeo por su nombre; "" si no es un vídeo que la galería
// admita.
func tipoDeVideo(nombre string) string {
	if t := carrete.TipoDe(nombre); videoImportable(t) {
		return t
	}
	return ""
}

// copiarVideo mide el vídeo de ruta y lo copia a la fototeca con nombre. respaldo, si no
// es nil, es lo que dijo de él la galería del sistema (el carrete): su duración y sus
// lados valen para lo que no se pueda medir del fichero, y su fecha es la de la toma; sin
// él, la del fichero.
func copiarVideo(ctx context.Context, repo *fototeca.Fototeca, medir func(context.Context, string) videoMedido, ruta, nombre, tipo string, respaldo *carrete.Elemento) (api.Photo, error) {
	f, err := os.Open(ruta) //nolint:gosec // un vídeo que eligió la persona o que dejó el carrete
	if err != nil {
		return api.Photo{}, err
	}
	defer func() { _ = f.Close() }()
	info, err := f.Stat()
	if err != nil {
		return api.Photo{}, err
	}
	o := privatePhotos.OrigenVideo{Nombre: nombre, Tipo: tipo, Datos: f, Tamano: info.Size()}
	if medir != nil {
		m := medir(ctx, ruta)
		o.Poster, o.Duracion, o.Ancho, o.Alto = m.poster, m.duracion, m.ancho, m.alto
	}
	cuando := info.ModTime()
	if respaldo != nil {
		if o.Duracion == 0 {
			o.Duracion = respaldo.Duracion
		}
		if (o.Ancho == 0 || o.Alto == 0) && respaldo.Ancho > 0 && respaldo.Alto > 0 {
			o.Ancho, o.Alto = respaldo.Ancho, respaldo.Alto
		}
		if !respaldo.Tomada.IsZero() {
			cuando = respaldo.Tomada
		}
	}
	o.TomadoEn = &cuando
	c, fin := context.WithTimeout(ctx, photos.PlazoSubida(info.Size()))
	defer fin()
	return repo.ImportarVideo(c, uuid.NewString(), o)
}

// importarVideoElegido es App.ImportarVideo: el vídeo elegido con «Añadir fotos» o
// soltado en la ventana se copia a la fototeca, salvo que ya esté (por su huella).
func importarVideoElegido(ctx context.Context, repo *fototeca.Fototeca, ruta string) error {
	nombre := filepath.Base(ruta)
	tipo := tipoDeVideo(nombre)
	if tipo == "" {
		return errors.New("photos: " + nombre + " no es un vídeo que la galería sepa enseñar")
	}
	suma, err := huellaDeRuta(ruta)
	if err != nil {
		return err
	}
	if _, ya, err := repo.FotoPorHuella(ctx, suma); err != nil || ya {
		return err
	}
	_, err = copiarVideo(ctx, repo, medirVideo, ruta, nombre, tipo, nil)
	return err
}

// huellaDeRuta es el SHA-256 en hexadecimal del fichero de ruta.
func huellaDeRuta(ruta string) (string, error) {
	f, err := os.Open(ruta) //nolint:gosec // lo que se va a importar
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// elementosAbiertos apunta lo que dijo el carrete de cada vídeo que tiene abierto (por
// la ruta del temporal), para que el destino lo tenga al copiarlo: el motor de la copia
// solo le pasa la ruta.
type elementosAbiertos struct{ m sync.Map }

func (a *elementosAbiertos) apuntar(ruta string, e carrete.Elemento) { a.m.Store(ruta, e) }
func (a *elementosAbiertos) olvidar(ruta string)                     { a.m.Delete(ruta) }

// de es lo apuntado de ruta; nil si nada.
func (a *elementosAbiertos) de(ruta string) *carrete.Elemento {
	if a == nil {
		return nil
	}
	if v, ok := a.m.Load(ruta); ok {
		if e, ok := v.(carrete.Elemento); ok { // solo se guardan Elemento
			return &e
		}
	}
	return nil
}

// ----- Para verlo -----

// copiasParaVer da un fichero con el vídeo copiado a la fototeca, con su nombre y su
// extensión, en `<caché>/photos-videos/<id><ext>`: un enlace simbólico al original del
// la fototeca (donde no se puede, uno duro, y si tampoco, una copia). Lo de los vídeos que ya
// no están en la galería se quita al arrancar y al borrar para siempre (podar).
type copiasParaVer struct {
	dir     string
	repo    *fototeca.Fototeca
	enClaro func(sha string) (string, bool)
	mu      sync.Mutex
}

func nuevasCopiasParaVer(dir string, repo *fototeca.Fototeca, enClaro func(string) (string, bool)) *copiasParaVer {
	return &copiasParaVer{dir: dir, repo: repo, enClaro: enClaro}
}

// extensionDe es la extensión con la que se ve un vídeo: la de su nombre o, sin ella, la
// de su tipo.
func extensionDe(f api.Photo) string {
	if ext := strings.ToLower(filepath.Ext(f.Name)); ext != "" && tipoDeVideo("x"+ext) != "" {
		return ext
	}
	for _, e := range extensionesVideo {
		if carrete.TipoDe("x."+e) == f.ContentType {
			return "." + e
		}
	}
	return ".mp4"
}

// ruta es el fichero del vídeo f para verlo. La primera vez lo prepara: un enlace es
// instantáneo; una copia tarda lo que tarde en leerse.
func (c *copiasParaVer) ruta(ctx context.Context, f api.Photo) (string, error) {
	if f.ID == "" || filepath.Base(f.ID) != f.ID {
		return "", os.ErrInvalid
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	destino := filepath.Join(c.dir, f.ID+extensionDe(f))
	if info, err := os.Stat(destino); err == nil && info.Mode().IsRegular() && info.Size() == f.SizeBytes {
		return destino, nil
	}
	// Lo que hubiera (un enlace a un objeto que ya no está, una copia a medias) sobra.
	_ = os.Remove(destino)
	if err := os.MkdirAll(c.dir, 0o700); err != nil {
		return "", err
	}
	if huella, err := c.repo.HuellaOriginal(ctx, f.ID); err == nil && huella != "" && c.enClaro != nil {
		if origen, ok := c.enClaro(huella); ok {
			if os.Symlink(origen, destino) == nil || os.Link(origen, destino) == nil {
				return destino, nil
			}
		}
	}
	tmp, err := os.CreateTemp(c.dir, ".copia-*"+filepath.Ext(destino))
	if err != nil {
		return "", err
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	err = c.repo.Original(ctx, f.ID, tmp)
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return "", err
	}
	return destino, os.Rename(tmp.Name(), destino)
}

// podar quita los ficheros de los vídeos que ya no están en la galería (en la papelera o
// borrados para siempre): así un vídeo borrado no sigue ocupando sitio por su copia o su
// enlace duro.
func (c *copiasParaVer) podar(ctx context.Context) {
	c.mu.Lock()
	defer c.mu.Unlock()
	entradas, err := os.ReadDir(c.dir)
	if err != nil {
		return
	}
	for _, e := range entradas {
		nombre := e.Name()
		id := strings.TrimSuffix(nombre, filepath.Ext(nombre))
		if strings.HasPrefix(nombre, ".copia-") {
			_ = os.Remove(filepath.Join(c.dir, nombre))
			continue
		}
		if _, err := uuid.Parse(id); err == nil {
			if _, err := c.repo.Photo(ctx, id); err == nil {
				continue
			}
		}
		if err := os.Remove(filepath.Join(c.dir, nombre)); err != nil {
			slog.Debug("photos: no se pudo quitar de la caché", "fichero", nombre, "error", err)
		}
	}
}

// ----- Para Cast -----

// paraCast deja el vídeo copiado f en carpeta —la de los vídeos de Cast,
// `videoteca.CarpetaEn`— y dice dónde, para «Abrir en Cast»: Cast no lee la caché de
// Photos (y lo de la caché se poda), así que se le da un fichero SUYO, que su videoteca
// recoge como propio. Es un CLON del original de la fototeca donde el sistema de ficheros
// clona (APFS, Btrfs, XFS: no ocupa más hasta que uno cambie) o, si no, una copia. Nunca
// un enlace duro: la carpeta de Cast está a la vista de la persona y de sus programas, y
// uno que reescribiera el fichero en su sitio cambiaría también el original de Photos,
// que se guarda por su huella.
// Va con su nombre; si ya hay otro fichero que se llama así, «nombre (2)»… Si el vídeo ya
// está en la carpeta (el mismo fichero, o los mismos bytes), se da ese: pulsarlo otra vez
// no lo duplica.
func (c *copiasParaVer) paraCast(ctx context.Context, f api.Photo, carpeta string) (string, error) {
	if f.ID == "" || carpeta == "" {
		return "", os.ErrInvalid
	}
	huella, err := c.repo.HuellaOriginal(ctx, f.ID)
	if err != nil {
		return "", err
	}
	origen := ""
	if c.enClaro != nil && huella != "" {
		origen, _ = c.enClaro(huella)
	}
	if err := os.MkdirAll(carpeta, 0o700); err != nil {
		return "", err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	base, ext := nombreParaCast(f)
	for i := 1; i <= 1000; i++ {
		nombre := base + ext
		if i > 1 {
			nombre = fmt.Sprintf("%s (%d)%s", base, i, ext)
		}
		ruta := filepath.Join(carpeta, nombre)
		if _, err := os.Lstat(ruta); err == nil {
			if mismoVideo(ruta, f.SizeBytes, huella) {
				return ruta, nil
			}
			continue
		} else if !errors.Is(err, fs.ErrNotExist) {
			return "", err
		}
		if err := c.ponerParaCast(ctx, f.ID, origen, carpeta, ruta, ext); err != nil {
			return "", err
		}
		return ruta, nil
	}
	return "", fs.ErrExist
}

// ponerParaCast deja en ruta, que está libre, el vídeo id: un clon de origen (su
// original en claro) si se puede y, si no, una copia a un temporal oculto (la videoteca
// no recoge lo oculto) y de ahí a su nombre: Cast nunca ve un vídeo a medias.
func (c *copiasParaVer) ponerParaCast(ctx context.Context, id, origen, carpeta, ruta, ext string) error {
	if origen != "" {
		clon := nombreOculto(carpeta, ext)
		if clonarFichero(origen, clon) == nil {
			if err := os.Rename(clon, ruta); err != nil {
				_ = os.Remove(clon)
				return err
			}
			return nil
		}
	}
	tmp, err := os.CreateTemp(carpeta, ".photos-*"+ext)
	if err != nil {
		return err
	}
	err = c.repo.Original(ctx, id, tmp)
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Rename(tmp.Name(), ruta)
	}
	if err != nil {
		_ = os.Remove(tmp.Name())
		return err
	}
	return nil
}

// nombreParaCast es el nombre (sin y con extensión) con el que un vídeo copiado llega a
// Cast: el suyo, sin carpetas ni lo que ningún sistema de ficheros acepta, y con la
// extensión de su tipo si no la trae.
func nombreParaCast(f api.Photo) (base, ext string) {
	nombre := filepath.Base(strings.ReplaceAll(f.Name, "\\", "/"))
	nombre = strings.TrimLeft(strings.Map(func(r rune) rune {
		if r < 0x20 || strings.ContainsRune(`<>:"/\|?*`, r) {
			return '_'
		}
		return r
	}, nombre), ". ")
	ext = filepath.Ext(nombre)
	if tipoDeVideo("x"+ext) == "" {
		ext = extensionDe(f)
	} else {
		nombre = strings.TrimSuffix(nombre, ext)
	}
	if strings.TrimSpace(nombre) == "" {
		nombre = f.ID
	}
	return nombre, ext
}

// mismoVideo dice si el fichero de ruta ya es ese vídeo: del mismo tamaño y con la misma
// huella que el original.
func mismoVideo(ruta string, tamano int64, huella string) bool {
	info, err := os.Stat(ruta)
	if err != nil || !info.Mode().IsRegular() || info.Size() != tamano {
		return false
	}
	if huella == "" {
		return false
	}
	h, err := huellaDeRuta(ruta)
	return err == nil && h == huella
}

// podarEnFondo es podar sin bloquear a quien la llama.
func (c *copiasParaVer) podarEnFondo() {
	if c == nil {
		return
	}
	go func() {
		ctx, fin := context.WithTimeout(context.Background(), time.Minute)
		defer fin()
		c.podar(ctx)
	}()
}
