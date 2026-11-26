// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package archivos

import (
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"git.sytue.eu/sytue/one/apps/nucleo/local/biblioteca"
)

// # El sistema de ficheros de la raíz (DR-12)
//
// Archivos trabaja con RUTAS: el identificador de cada elemento es su ruta codificada, el
// índice guarda rutas y la papelera y las versiones cuelgan de `<raíz>/.sytue`. Lo que
// hace con ellas pasa por [Sistema], no por `os`: así la misma lógica sirve para una
// carpeta del disco ([DelDisco], lo de siempre) y para una carpeta que la persona elige
// con el selector del sistema en Android, que no es una ruta sino un árbol de documentos
// (`content://…`, Storage Access Framework; ver [Arbol]).
//
// Cada método es el de `os` (o `filepath`) del mismo nombre, con su misma semántica: un
// error de «no existe» cumple `errors.Is(err, fs.ErrNotExist)`, uno de «ya existe»
// `errors.Is(err, fs.ErrExist)`, y Rename sustituye un fichero que ya esté en el destino.

// Sistema es lo que Archivos hace con el sistema de ficheros de su raíz.
type Sistema interface {
	Stat(ruta string) (fs.FileInfo, error)
	Lstat(ruta string) (fs.FileInfo, error)
	ReadDir(ruta string) ([]fs.DirEntry, error)
	ReadFile(ruta string) ([]byte, error)
	// Open abre un fichero para leerlo.
	Open(ruta string) (Fichero, error)
	Mkdir(ruta string, permisos fs.FileMode) error
	MkdirAll(ruta string, permisos fs.FileMode) error
	// CreateTemp crea un fichero nuevo en dir con un nombre que sigue el patrón de
	// os.CreateTemp, abierto para escribir.
	CreateTemp(dir, patron string) (Temporal, error)
	// Rename mueve o renombra; un fichero que ya esté en el destino se sustituye.
	Rename(de, a string) error
	Remove(ruta string) error
	RemoveAll(ruta string) error
	Chmod(ruta string, permisos fs.FileMode) error
	// WalkDir es filepath.WalkDir: en orden léxico, con fs.SkipDir y fs.SkipAll.
	WalkDir(raiz string, fn fs.WalkDirFunc) error
	// EscribirAtomico deja datos en ruta sin que se vea nunca a medias (temporal y
	// renombrado), creando la carpeta si falta.
	EscribirAtomico(ruta string, datos []byte) error
	// Espacio es el total y lo libre del almacenamiento donde está ruta.
	Espacio(ruta string) (total, libre int64, err error)
}

// Fichero es un fichero abierto para leer.
type Fichero interface {
	io.ReadCloser
	Stat() (fs.FileInfo, error)
}

// Temporal es un fichero recién creado, abierto para escribir.
type Temporal interface {
	io.WriteCloser
	// Name es su ruta.
	Name() string
	Sync() error
}

// DelDisco es el sistema de ficheros del equipo: `os` tal cual. Es el de siempre, el que
// usa Archivos si Opciones.Sistema va vacío.
var DelDisco Sistema = disco{}

type disco struct{}

func (disco) Stat(r string) (fs.FileInfo, error)      { return os.Stat(r) }
func (disco) Lstat(r string) (fs.FileInfo, error)     { return os.Lstat(r) }
func (disco) ReadDir(r string) ([]fs.DirEntry, error) { return os.ReadDir(r) }
func (disco) ReadFile(r string) ([]byte, error)       { return os.ReadFile(r) }

func (disco) Open(r string) (Fichero, error) {
	f, err := os.Open(r)
	if err != nil {
		return nil, err
	}
	return f, nil
}

func (disco) Mkdir(r string, p fs.FileMode) error    { return os.Mkdir(r, p) }
func (disco) MkdirAll(r string, p fs.FileMode) error { return os.MkdirAll(r, p) }

func (disco) CreateTemp(dir, patron string) (Temporal, error) {
	f, err := os.CreateTemp(dir, patron)
	if err != nil {
		return nil, err
	}
	return f, nil
}

func (disco) Rename(de, a string) error                 { return os.Rename(de, a) } //nolint:gosec // G703: rutas de Archivos.ruta, ya absolutas, limpias y dentro de lo accesible
func (disco) Remove(r string) error                     { return os.Remove(r) }
func (disco) RemoveAll(r string) error                  { return os.RemoveAll(r) }
func (disco) Chmod(r string, p fs.FileMode) error       { return os.Chmod(r, p) }
func (disco) WalkDir(r string, fn fs.WalkDirFunc) error { return filepath.WalkDir(r, fn) }
func (disco) EscribirAtomico(r string, datos []byte) error {
	return biblioteca.EscribirAtomico(r, datos)
}
func (disco) Espacio(r string) (total, libre int64, err error) { return espacioDisco(r) }

// copiarAtomico copia el fichero de (en el sistema origen) a a2 (en el destino), por un
// temporal de la carpeta de destino y un renombrado: nunca se ve una copia a medias.
func copiarAtomico(origen Sistema, de string, destino Sistema, a2 string) error {
	in, err := origen.Open(de)
	if err != nil {
		return err
	}
	defer in.Close()
	tmp, err := destino.CreateTemp(filepath.Dir(a2), ".version.*.tmp")
	if err != nil {
		return err
	}
	defer func() { _ = destino.Remove(tmp.Name()) }() // no hace nada tras el renombrado
	_, err = io.Copy(tmp, in)
	if err == nil {
		err = tmp.Sync()
	}
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return err
	}
	return destino.Rename(tmp.Name(), a2)
}

// escribirAtomicoEn es biblioteca.EscribirAtomico en el sistema s: la carpeta si falta, un
// temporal a su lado, volcado y cerrado, y el renombrado.
func escribirAtomicoEn(s Sistema, ruta string, datos []byte) error {
	dir := filepath.Dir(ruta)
	if err := s.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	f, err := s.CreateTemp(dir, "."+filepath.Base(ruta)+".*.tmp")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer func() { _ = s.Remove(tmp) }() // no hace nada tras el renombrado
	if _, err := f.Write(datos); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return s.Rename(tmp, ruta)
}
