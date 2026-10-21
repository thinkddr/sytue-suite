// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build js

package auth

import "errors"

// En el navegador NO hay almacén de sesión, y es a propósito: allí el token no baja a la
// página. Lo guarda el BFF tras una cookie httpOnly y lo pone él en cada petición que
// pasa por su proxy, así que ningún script lo alcanza (ver el comentario del paquete).
//
// Esto existe para que el paquete compile en `GOOS=js`, que es como se type-chequean los
// `cmd/` de Fyne sin bibliotecas gráficas —la única forma barata de ver lo que si no
// nadie ve hasta el CI de release—. Devolver un error y no un token vacío es la
// diferencia entre «no hay sesión» y «aquí no se guardan sesiones».
var errSinAlmacen = errors.New("auth: en el navegador la sesión no se guarda en el cliente")

func cargar() (Tokens, error) { return Tokens{}, errSinAlmacen }

func guardar(Tokens) error { return errSinAlmacen }

func leerSecreto(string) ([]byte, error) { return nil, errSinAlmacen }

func guardarSecreto(string, []byte) error { return errSinAlmacen }

func borrarSecreto(string) error { return errSinAlmacen }

func cargarCuenta(string) (Tokens, error) { return Tokens{}, errSinAlmacen }

func guardarCuenta(string, Tokens) error { return errSinAlmacen }
