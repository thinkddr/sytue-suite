// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build js

package offline

import (
	"fmt"
	"io/fs"
	"strings"
	"syscall/js"
)

// ponytail: localStorage y no IndexedDB, que es lo que usa storage.ts. Es síncrono y
// cabe en cuarenta líneas; a cambio el navegador lo corta en ~5 MB por origen. Vale
// para borradores y preferencias. El día que la web en Gio guarde documentos enteros
// sin conexión, esto pasa a IndexedDB detrás de la misma interfaz.
const prefijoJS = "sytue-offline/"

type almacenLocal struct{}

// OpenBrowser abre el almacén del navegador.
func OpenBrowser() *Store { return &Store{b: almacenLocal{}} }

// seguro convierte las excepciones de JavaScript (cuota llena, almacenamiento
// bloqueado en modo privado) en errores de Go, que es como las ve syscall/js: pánicos.
func seguro(f func()) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("offline: localStorage: %v", r)
		}
	}()
	f()
	return nil
}

func ls() js.Value { return js.Global().Get("localStorage") }

func (almacenLocal) get(clave string) (datos []byte, err error) {
	err = seguro(func() {
		if v := ls().Call("getItem", prefijoJS+clave); !v.IsNull() {
			datos = []byte(v.String())
		}
	})
	if err == nil && datos == nil {
		err = fs.ErrNotExist
	}
	return datos, err
}

func (almacenLocal) put(clave string, datos []byte) error {
	return seguro(func() { ls().Call("setItem", prefijoJS+clave, string(datos)) })
}

func (almacenLocal) del(clave string) error {
	return seguro(func() { ls().Call("removeItem", prefijoJS+clave) })
}

func (almacenLocal) claves(prefijo string) (out []string, err error) {
	err = seguro(func() {
		for i := range ls().Get("length").Int() {
			if k := ls().Call("key", i).String(); strings.HasPrefix(k, prefijoJS+prefijo) {
				out = append(out, k)
			}
		}
	})
	return out, err
}

func (a almacenLocal) list(prefijo string) ([][]byte, error) {
	claves, err := a.claves(prefijo + "/")
	if err != nil {
		return nil, err
	}
	var out [][]byte
	for _, k := range claves {
		datos, err := a.get(strings.TrimPrefix(k, prefijoJS))
		if err != nil {
			return nil, err
		}
		out = append(out, datos)
	}
	return out, nil
}

func (a almacenLocal) contar(espacio string) (int, error) {
	claves, err := a.claves("")
	n := 0
	for _, k := range claves {
		if partes := strings.Split(strings.TrimPrefix(k, prefijoJS), "/"); len(partes) > 2 && partes[1] == espacio {
			n++
		}
	}
	return n, err
}

func (a almacenLocal) clear() error {
	claves, err := a.claves("")
	if err != nil {
		return err
	}
	return seguro(func() {
		for _, k := range claves {
			ls().Call("removeItem", k)
		}
	})
}
