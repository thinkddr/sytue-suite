// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build js

package sesion

import (
	"context"
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

// ponerSoloLectura: en `js` no hay variables de entorno con las que pedirlo.
func ponerSoloLectura() {}

// Almacen en `js` no existe: el almacén sin conexión vive en IndexedDB, del lado del
// navegador. Quien llama ya trata el error como «sin almacén» (cmd/notes).
func Almacen() (*offline.Store, error) {
	return nil, errors.New("sesion: sin almacén local en js")
}

func AlmacenConContexto(ctx context.Context) (*offline.Store, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return Almacen()
}

func contarAlmacen(...string) (int, error) { return 0, nil }

func (s *Suite) vaciarAlmacenCon(ctx context.Context) error { return ctx.Err() }
func directorioAlmacen() (string, error)                    { return "", offline.ErrEncryptedStore }

func directorioAlmacenActivo(context.Context) (string, error) { return "", offline.ErrEncryptedStore }

func contarAlmacenCon(ctx context.Context, _ ...string) (int, error) { return 0, ctx.Err() }
