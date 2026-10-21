// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build js

package sesion

import (
	"errors"
	"net/http"

	"git.sytue.eu/sytue/one/apps/nucleo/offline"
)

// Conexion en `js` no lleva caché propia: ahí `net/http` es `fetch()` y la caché la
// hace el service worker (ver `offline/transport.go`). Existe solo para que el
// type-check de todas las plataformas siga pasando.
func (s *Suite) Conexion(string, ...string) (*offline.Monitor, *http.Client) {
	return &offline.Monitor{}, nil
}

// vaciarAlmacen: en `js` no hay almacén propio que vaciar.
func (s *Suite) vaciarAlmacen() {}

// ponerSoloLectura: en `js` no hay variables de entorno con las que pedirlo.
func ponerSoloLectura() {}

// Almacen en `js` no existe: el almacén sin conexión vive en IndexedDB, del lado del
// navegador. Quien llama ya trata el error como «sin almacén» (cmd/notes).
func Almacen() (*offline.Store, error) {
	return nil, errors.New("sesion: sin almacén local en js")
}
