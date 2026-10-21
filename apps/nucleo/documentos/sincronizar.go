// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package documentos

import (
	"context"
	"encoding/json"
	"io/fs"
	"mime"
	"os"
	"path/filepath"
	"strings"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

const (
	NombreManifest = ".sytue-drive.json"
)

type FicheroSinc struct {
	ID        string `json:"id"`
	Tamano    int64  `json:"tamano"`
	ModUnixNs int64  `json:"mod_unix_ns"`
	RemotoMs  int64  `json:"remoto_ms"`
}

type EstadoSinc struct {
	Carpetas map[string]string      `json:"carpetas"`
	Ficheros map[string]FicheroSinc `json:"ficheros"`
}

func nuevoEstadoSinc() *EstadoSinc {
	return &EstadoSinc{
		Carpetas: make(map[string]string),
		Ficheros: make(map[string]FicheroSinc),
	}
}

func cargarEstado(dir string) *EstadoSinc {
	b, err := os.ReadFile(filepath.Join(dir, NombreManifest))
	if err != nil {
		return nuevoEstadoSinc()
	}
	var e EstadoSinc
	if err := json.Unmarshal(b, &e); err != nil {
		return nuevoEstadoSinc()
	}
	if e.Carpetas == nil {
		e.Carpetas = make(map[string]string)
	}
	if e.Ficheros == nil {
		e.Ficheros = make(map[string]FicheroSinc)
	}
	return &e
}

func guardarEstado(dir string, e *EstadoSinc) error {
	b, err := json.MarshalIndent(e, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".manifest-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	if _, err := tmp.Write(b); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), filepath.Join(dir, NombreManifest))
}

func detectarBorradosLocales(ctx context.Context, f Fuente, dir string, estado *EstadoSinc) error {
	for rel, reg := range estado.Ficheros {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		ruta := filepath.Join(dir, rel)
		if _, err := os.Stat(ruta); os.IsNotExist(err) {
			_ = f.Delete(ctx, api.DriveFile, reg.ID)
			delete(estado.Ficheros, rel)
		}
	}
	for rel, id := range estado.Carpetas {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		ruta := filepath.Join(dir, rel)
		if _, err := os.Stat(ruta); os.IsNotExist(err) {
			_ = f.Delete(ctx, api.DriveFolder, id)
			delete(estado.Carpetas, rel)
		}
	}
	return nil
}

func resolverPadreID(relDir, raizID string, carpetas map[string]string) string {
	if relDir == "." || relDir == "" {
		return raizID
	}
	if id, ok := carpetas[relDir]; ok {
		return id
	}
	return raizID
}

func procesarEntradaLocal(ctx context.Context, f Fuente, dir, raizID, ruta string, d fs.DirEntry, estado *EstadoSinc) error {
	nombre := d.Name()
	if strings.HasPrefix(nombre, ".") || nombre == "Inbox" {
		if d.IsDir() {
			return filepath.SkipDir
		}
		return nil
	}
	rel, errRel := filepath.Rel(dir, ruta)
	if errRel != nil {
		return nil //nolint:nilerr // si la ruta no es relativa se ignora
	}
	relSlash := filepath.ToSlash(rel)

	if d.IsDir() {
		if _, ok := estado.Carpetas[relSlash]; !ok {
			relPadre := filepath.ToSlash(filepath.Dir(rel))
			padreID := resolverPadreID(relPadre, raizID, estado.Carpetas)
			c, err := f.CreateFolder(ctx, padreID, nombre)
			if err == nil {
				estado.Carpetas[relSlash] = c.ID
			}
		}
		return nil
	}

	fi, errInfo := d.Info()
	if errInfo != nil || !fi.Mode().IsRegular() {
		return nil //nolint:nilerr // entradas irregulares se ignoran
	}

	if reg, ok := estado.Ficheros[relSlash]; ok {
		if fi.ModTime().UnixNano() != reg.ModUnixNs || fi.Size() != reg.Tamano {
			subido, err := subir(ctx, f, reg.ID, ruta)
			if err == nil {
				estado.Ficheros[relSlash] = FicheroSinc{
					ID:        subido.ID,
					Tamano:    fi.Size(),
					ModUnixNs: fi.ModTime().UnixNano(),
					RemotoMs:  subido.UpdatedAt.UnixMilli(),
				}
			}
		}
		return nil
	}

	relPadre := filepath.ToSlash(filepath.Dir(rel))
	padreID := resolverPadreID(relPadre, raizID, estado.Carpetas)
	tipo := mime.TypeByExtension(filepath.Ext(nombre))
	if tipo == "" {
		tipo = "application/octet-stream"
	}
	fl, errOpen := os.Open(ruta) //nolint:gosec // ruta dentro de dir
	if errOpen == nil {
		subido, errSub := f.Upload(ctx, padreID, nombre, tipo, fl, fi.Size())
		_ = fl.Close()
		if errSub == nil {
			estado.Ficheros[relSlash] = FicheroSinc{
				ID:        subido.ID,
				Tamano:    fi.Size(),
				ModUnixNs: fi.ModTime().UnixNano(),
				RemotoMs:  subido.UpdatedAt.UnixMilli(),
			}
		}
	}
	return nil
}

func subirCambiosLocales(ctx context.Context, f Fuente, dir, raizID string, estado *EstadoSinc) error {
	return filepath.WalkDir(dir, func(ruta string, d fs.DirEntry, err error) error {
		if err != nil || ctx.Err() != nil {
			return ctx.Err()
		}
		if ruta == dir {
			return nil
		}
		return procesarEntradaLocal(ctx, f, dir, raizID, ruta, d, estado)
	})
}

type trabajoDescarga struct {
	carpetaID string
	relDir    string
	nivel     int
}

func listarHijos(ctx context.Context, f Fuente, carpetaID string) api.Children {
	var hijos api.Children
	for cursor := ""; ; {
		h, err := f.Children(ctx, carpetaID, cursor)
		if err != nil {
			break
		}
		hijos.Folders = append(hijos.Folders, h.Folders...)
		hijos.Files = append(hijos.Files, h.Files...)
		if cursor = h.NextCursor; cursor == "" || len(hijos.Folders)+len(hijos.Files) >= maxFilas {
			break
		}
	}
	return hijos
}

func sincronizarFicheroRemoto(ctx context.Context, f Fuente, dir string, fic api.File, relFic string, estado *EstadoSinc) {
	rutaFic := filepath.Join(dir, filepath.FromSlash(relFic))
	necesitaBajar := false
	reg, ok := estado.Ficheros[relFic]
	if !ok || reg.ID != fic.ID {
		necesitaBajar = true
	} else if fic.UpdatedAt.UnixMilli() > 0 && fic.UpdatedAt.UnixMilli() != reg.RemotoMs {
		necesitaBajar = true
	} else if _, err := os.Stat(rutaFic); os.IsNotExist(err) {
		necesitaBajar = true
	}

	if necesitaBajar {
		if err := bajar(ctx, f, fic.ID, rutaFic); err == nil {
			if info, errStat := os.Stat(rutaFic); errStat == nil {
				estado.Ficheros[relFic] = FicheroSinc{
					ID:        fic.ID,
					Tamano:    info.Size(),
					ModUnixNs: info.ModTime().UnixNano(),
					RemotoMs:  fic.UpdatedAt.UnixMilli(),
				}
			}
		}
	}
}

func descargarRemoto(ctx context.Context, f Fuente, dir, raizID string, estado *EstadoSinc) (map[string]bool, map[string]bool, error) {
	vistosFicheros := make(map[string]bool)
	vistosCarpetas := make(map[string]bool)
	cola := []trabajoDescarga{{carpetaID: raizID, relDir: "", nivel: 0}}

	for len(cola) > 0 {
		if ctx.Err() != nil {
			return nil, nil, ctx.Err()
		}
		actual := cola[0]
		cola = cola[1:]

		hijos := listarHijos(ctx, f, actual.carpetaID)
		for _, sub := range hijos.Folders {
			relSub := sub.Name
			if actual.relDir != "" {
				relSub = actual.relDir + "/" + sub.Name
			}
			vistosCarpetas[relSub] = true
			estado.Carpetas[relSub] = sub.ID
			_ = os.MkdirAll(filepath.Join(dir, filepath.FromSlash(relSub)), 0o750)
			if actual.nivel < 8 {
				cola = append(cola, trabajoDescarga{carpetaID: sub.ID, relDir: relSub, nivel: actual.nivel + 1})
			}
		}

		for _, fic := range hijos.Files {
			relFic := fic.Name
			if actual.relDir != "" {
				relFic = actual.relDir + "/" + fic.Name
			}
			vistosFicheros[relFic] = true
			sincronizarFicheroRemoto(ctx, f, dir, fic, relFic, estado)
		}
	}
	return vistosFicheros, vistosCarpetas, nil
}

func limpiarBorradosRemotos(dir string, estado *EstadoSinc, vistosFicheros, vistosCarpetas map[string]bool) {
	for rel := range estado.Ficheros {
		if !vistosFicheros[rel] {
			_ = os.Remove(filepath.Join(dir, filepath.FromSlash(rel)))
			delete(estado.Ficheros, rel)
		}
	}
	for rel := range estado.Carpetas {
		if !vistosCarpetas[rel] {
			_ = os.RemoveAll(filepath.Join(dir, filepath.FromSlash(rel)))
			delete(estado.Carpetas, rel)
		}
	}
}

// Sincronizar sincroniza bidireccionalmente la carpeta de Sytue Drive con `dir`
// (en iOS, `~/Documents`, visible en la app «Archivos» bajo «En mi iPhone / En mi iPad»).
func Sincronizar(ctx context.Context, f Fuente, dir string) error {
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return err
	}
	raiz, err := f.Root(ctx)
	if err != nil {
		return err
	}

	estado := cargarEstado(dir)

	if err := detectarBorradosLocales(ctx, f, dir, estado); err != nil {
		return err
	}

	if err := subirCambiosLocales(ctx, f, dir, raiz.ID, estado); err != nil {
		return err
	}

	vistosFicheros, vistosCarpetas, err := descargarRemoto(ctx, f, dir, raiz.ID, estado)
	if err != nil {
		return err
	}

	limpiarBorradosRemotos(dir, estado, vistosFicheros, vistosCarpetas)

	return guardarEstado(dir, estado)
}

// ServirIOS atiende la sincronización periódica de la carpeta ~/Documents de iOS.
func ServirIOS(ctx context.Context, f Fuente, _ Textos) error {
	casa, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	dir := filepath.Join(casa, "Documents")
	_ = Sincronizar(ctx, f, dir)

	reloj := time.NewTicker(10 * time.Second)
	defer reloj.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-reloj.C:
			_ = Sincronizar(ctx, f, dir)
		}
	}
}
