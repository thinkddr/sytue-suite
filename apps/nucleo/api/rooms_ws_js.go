// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build js

package api

import (
	"net/http"

	"github.com/coder/websocket"
)

// opcionesWS: en el navegador el WebSocket es el suyo, y no admite ni cabeceras ni
// cliente HTTP. Va al mismo origen, así que la cookie de sesión viaja sola.
func opcionesWS(*http.Client, string) *websocket.DialOptions { return nil }
