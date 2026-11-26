// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build !android && !(darwin && cgo)

package carrete

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"git.sytue.eu/sytue/one/apps/nucleo/ficheros"
)

// # La galería del escritorio son carpetas
//
// En Windows y Linux no hay una fototeca con API: la app Fotos de Windows, GNOME Fotos,
// Shotwell o digiKam leen la carpeta de imágenes del perfil (`XDG_PICTURES_DIR`) o, en
// Windows, la BIBLIOTECA Imágenes, que son varias carpetas: la del perfil (que OneDrive
// puede llevarse a la suya), el «Álbum de cámara», «Imágenes guardadas» y las que la
// persona añada de otros discos (carpeta_windows.go). Así que la galería son esas
// carpetas, recorridas enteras, y guardar es dejar el fichero en la subcarpeta «Sytue
// Photos» de la de imágenes del perfil, que es la que las dos apps Fotos enseñan.
//
// No hace falta permiso: son las carpetas de quien corre el programa. Cada elemento se
// identifica por su ruta absoluta: con varias raíces, una relativa no diría de cuál.
//
// Este fichero también es el de macOS e iOS SIN cgo (un binario de pruebas): en macOS
// se queda con ~/Pictures, y en iOS no hay carpeta y no está disponible.

// raiz es la carpeta de imágenes del perfil, donde se guarda; raices, todas las que
// forman la galería. Las pruebas las cambian.
var (
	raiz   = ficheros.CarpetaImagenes
	raices = carpetasGaleria
)

func disponible() bool {
	d, err := raiz()
	return err == nil && d != ""
}

func estadoPermiso() Permiso { return Concedido }

func pedirPermiso(context.Context) (Permiso, error) { return Concedido, nil }

// carpetasLimpias quita las repetidas y las que están dentro de otra (se recorrerían dos
// veces), conservando el orden.
func carpetasLimpias(l []string) []string {
	var limpias []string
	for _, c := range l {
		if c == "" {
			continue
		}
		c = filepath.Clean(c)
		dentro := false
		for i, o := range limpias {
			switch {
			case dentroDe(c, o):
				dentro = true
			case dentroDe(o, c):
				limpias[i] = c // la nueva contiene a una anterior: se queda la de fuera
				dentro = true
			}
		}
		if !dentro {
			limpias = append(limpias, c)
		}
	}
	vistas := map[string]bool{}
	return slices.DeleteFunc(limpias, func(c string) bool {
		ya := vistas[c]
		vistas[c] = true
		return ya
	})
}

// dentroDe dice si `ruta` es `carpeta` o está dentro de ella.
func dentroDe(ruta, carpeta string) bool {
	rel, err := filepath.Rel(carpeta, ruta)
	return err == nil && (rel == "." || filepath.IsLocal(rel))
}

func listar(ctx context.Context) ([]Elemento, error) {
	todas, err := raices()
	if err != nil {
		return nil, err
	}
	propio := ""
	if r, err := raiz(); err == nil && r != "" {
		propio = filepath.Join(r, AlbumPropio)
	}
	var l []Elemento
	for _, base := range carpetasLimpias(todas) {
		if err := recorrer(ctx, base, propio, &l); err != nil {
			return nil, err
		}
	}
	return l, nil
}

// recorrer añade a `l` las fotos y los vídeos de una carpeta, sin las ocultas ni la de
// Sytue (`propio`).
func recorrer(ctx context.Context, base, propio string, l *[]Elemento) error {
	return filepath.WalkDir(base, func(ruta string, d fs.DirEntry, err error) error {
		if err != nil {
			// Una subcarpeta sin permiso no tumba la lista; la raíz que no existe, tampoco
			// (una cuenta recién creada sin «Imágenes», un disco desconectado): es una
			// galería vacía.
			if d != nil && d.IsDir() && ruta != base {
				return fs.SkipDir
			}
			return nil
		}
		if e := ctx.Err(); e != nil {
			return e
		}
		rel, _ := filepath.Rel(base, ruta)
		if d.IsDir() {
			if ruta == base {
				return nil
			}
			if strings.HasPrefix(d.Name(), ".") || ruta == propio ||
				strings.Count(rel, string(filepath.Separator)) >= ProfundidadMaxima {
				return fs.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() || strings.HasPrefix(d.Name(), ".") {
			return nil
		}
		tipo := TipoDe(d.Name())
		if tipo == "" {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil //nolint:nilerr // un fichero que desaparece mientras se recorre no es un fallo
		}
		*l = append(*l, Elemento{
			ID: filepath.ToSlash(ruta), Nombre: d.Name(), Tipo: tipo,
			Tomada: info.ModTime(), Tamano: info.Size(), Video: EsVideo(tipo),
		})
		return nil
	})
}

// abrir no copia nada: el original ya es un fichero. La ruta tiene que estar dentro de
// alguna carpeta de la galería, por si el ID viniera de otro sitio.
func abrir(_ context.Context, e Elemento) (string, func(), error) {
	nada := func() {}
	todas, err := raices()
	if err != nil {
		return "", nada, err
	}
	ruta := filepath.Clean(filepath.FromSlash(e.ID))
	if !filepath.IsAbs(ruta) || !slices.ContainsFunc(todas, func(c string) bool { return c != "" && dentroDe(ruta, c) }) {
		return "", nada, fs.ErrNotExist
	}
	if _, err := os.Stat(ruta); err != nil {
		return "", nada, err
	}
	return ruta, nada, nil
}

// convertir no existe en el escritorio: el decodificador del sistema (WIC en Windows,
// libheif en Linux) no siempre está, y no hace falta. Quien llama sube el original, que
// es la copia de seguridad igual, y syphotos decodifica el HEIC él mismo (`miniatura`,
// HEVC en Go puro, desde el 2026-11-25).
func convertir(context.Context, string) (string, func(), error) {
	return "", func() {}, ErrNoDisponible
}

func guardar(_ context.Context, ruta, nombre string) (string, error) {
	base, err := raiz()
	if err != nil {
		return "", err
	}
	f, err := os.Open(ruta) //nolint:gosec // la copia local que acaba de escribir quien llama
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()
	destino, err := ficheros.Guardar(filepath.Join(base, AlbumPropio), nombre, f)
	if err != nil {
		return "", err
	}
	return filepath.ToSlash(destino), nil
}
