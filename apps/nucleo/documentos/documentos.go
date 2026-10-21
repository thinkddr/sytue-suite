// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package documentos pone Sytue Drive en el sistema: en «Archivos» de Android y en el
// selector de cualquier app (Storage Access Framework), en el gestor de ficheros de
// Linux, y en la carpeta sincronizada de iOS.
//
// En Android y en Linux quien habla con Drive es el SISTEMA, no la app: el proveedor
// `eu.sytue.nativo.Documentos` de la APK y GVfs hablan WebDAV con Basic y una
// contraseña de aplicación del equipo ([Asegurar], credencial.go), así que funcionan
// con la app cerrada. La app solo crea esa credencial al entrar y la retira al salir.
package documentos

import (
	"context"
	"errors"
	"io"
	"mime"
	"os"
	"path/filepath"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// Fuente es lo que hace falta de Drive; la da `*api.Drive`, y el guardián, un doble.
type Fuente interface {
	Root(ctx context.Context) (api.Folder, error)
	Children(ctx context.Context, carpeta, cursor string) (api.Children, error)
	File(ctx context.Context, id string) (api.File, error)
	Folder(ctx context.Context, id string) (api.Folder, error)
	Download(ctx context.Context, fichero string) (io.ReadCloser, error)
	Upload(ctx context.Context, carpeta, nombre, tipo string, cuerpo io.Reader, tamano int64) (api.File, error)
	CreateFolder(ctx context.Context, padre, nombre string) (api.Folder, error)
	Rename(ctx context.Context, k api.DriveKind, id, nombre string) error
	Delete(ctx context.Context, k api.DriveKind, id string) error
	Search(ctx context.Context, texto string) (api.Children, error)
}

var _ Fuente = (*api.Drive)(nil)

// Textos son los que el proveedor enseña por su cuenta: ya traducidos.
type Textos struct {
	Titulo string // el de la raíz: el nombre de la app
	SinApp string // qué hacer para ver los ficheros cuando no hay credencial
}

// ErrNoDisponible es que este sistema no tiene proveedor de documentos.
var ErrNoDisponible = errors.New("documentos: este sistema no tiene proveedor de documentos")

// maxFilas es el tope de una carpeta al recorrerla.
// ponytail: la página del servicio se sigue hasta aquí; una carpeta más grande sale
// cortada. Se sube el día que alguien tenga una.
const maxFilas = 5000

// bajar deja el contenido en `ruta`, de la carpeta sincronizada: escribe al lado y
// renombra, para que nadie lea un fichero a medias.
func bajar(ctx context.Context, f Fuente, id, ruta string) error {
	if ruta == "" {
		return errors.New("documentos: sin ruta")
	}
	cuerpo, err := f.Download(ctx, id)
	if err != nil {
		return err
	}
	defer func() { _ = cuerpo.Close() }()
	tmp, err := os.CreateTemp(filepath.Dir(ruta), ".bajando-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	if _, err := io.Copy(tmp, cuerpo); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), ruta)
}

// subir manda `ruta` como versión nueva del fichero: el servicio archiva la anterior al
// subir con el mismo nombre a la misma carpeta.
func subir(ctx context.Context, f Fuente, id, ruta string) (api.File, error) {
	x, err := f.File(ctx, id)
	if err != nil {
		return api.File{}, err
	}
	c, err := os.Open(ruta) //nolint:gosec // G304: la ruta es de la carpeta sincronizada
	if err != nil {
		return api.File{}, err
	}
	defer func() { _ = c.Close() }()
	info, err := c.Stat()
	if err != nil {
		return api.File{}, err
	}
	tipo := x.ContentType
	if tipo == "" {
		tipo = mime.TypeByExtension(filepath.Ext(x.Name))
	}
	return f.Upload(ctx, x.FolderID, x.Name, tipo, c, info.Size())
}

// Servir atiende lo que el sistema necesita de la app mientras `ctx` viva: en iOS, la
// carpeta sincronizada; en Android, los textos del proveedor. En el resto,
// ErrNoDisponible.
func Servir(ctx context.Context, f Fuente, t Textos) error { return servir(ctx, f, t) }
