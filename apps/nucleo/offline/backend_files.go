// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build !js

package offline

import (
	"crypto/rand"
	"errors"
	"io/fs"
	"os"
	"path"
)

// ponytail: un fichero por valor, no bbolt como pedía el encargo. bbolt bloquea su
// fichero para UN proceso, y la suite son varios binarios (Mail, Drive, el paraguas…)
// abiertos a la vez sobre los mismos datos: el segundo se quedaría esperando al
// arrancar. Techo: miles de valores por espacio, que es leer un directorio. Si se
// queda corto, SQLite en modo WAL, que sí admite varios procesos.
//
// Todo pasa por os.Root: aunque las claves ya son hashes, es el sistema —y no una
// comprobación nuestra— quien garantiza que nada se abre fuera del directorio.
type ficheros struct{ raiz *os.Root }

// Open abre (y crea) el almacén bajo dir: lo normal, os.UserCacheDir()/sytue/offline.
func Open(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	raiz, err := os.OpenRoot(dir)
	if err != nil {
		return nil, err
	}
	return &Store{b: ficheros{raiz}}, nil
}

func (f ficheros) get(clave string) ([]byte, error) { return f.raiz.ReadFile(clave + ".json") }

// put escribe a un temporal y lo renombra: un corte de luz deja el valor viejo o el
// nuevo, nunca medio.
func (f ficheros) put(clave string, datos []byte) error {
	if err := f.raiz.MkdirAll(path.Dir(clave), 0o700); err != nil {
		return err
	}
	tmp := clave + ".tmp-" + rand.Text()
	if err := f.raiz.WriteFile(tmp, datos, 0o600); err != nil {
		_ = f.raiz.Remove(tmp)
		return err
	}
	if err := f.raiz.Rename(tmp, clave+".json"); err != nil {
		_ = f.raiz.Remove(tmp)
		return err
	}
	return nil
}

func (f ficheros) del(clave string) error {
	if err := f.raiz.Remove(clave + ".json"); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}

func (f ficheros) list(prefijo string) ([][]byte, error) {
	entradas, err := fs.ReadDir(f.raiz.FS(), prefijo)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out [][]byte
	for _, e := range entradas {
		if e.IsDir() || path.Ext(e.Name()) != ".json" {
			continue
		}
		datos, err := f.raiz.ReadFile(path.Join(prefijo, e.Name()))
		if errors.Is(err, fs.ErrNotExist) {
			continue // otro proceso lo ha borrado entre el ReadDir y ahora
		}
		if err != nil {
			return nil, err
		}
		out = append(out, datos)
	}
	return out, nil
}

func (f ficheros) contar(espacio string) (int, error) {
	duenos, err := fs.ReadDir(f.raiz.FS(), ".")
	if err != nil {
		return 0, err
	}
	n := 0
	for _, d := range duenos {
		if !d.IsDir() {
			continue
		}
		entradas, err := fs.ReadDir(f.raiz.FS(), path.Join(d.Name(), espacio))
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return n, err
		}
		for _, e := range entradas {
			if !e.IsDir() && path.Ext(e.Name()) == ".json" {
				n++
			}
		}
	}
	return n, nil
}

func (f ficheros) clear() error {
	entradas, err := fs.ReadDir(f.raiz.FS(), ".")
	if err != nil {
		return err
	}
	for _, e := range entradas {
		if e.Name() == ".http-cache-lock" {
			continue // stable inode, also while another app is waiting for the lock
		}
		if err := f.raiz.RemoveAll(e.Name()); err != nil {
			return err
		}
	}
	return nil
}
