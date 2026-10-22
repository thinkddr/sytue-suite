// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build !js

package api

import (
	"net/http"

	"github.com/coder/websocket"
)

// opcionesWS: fuera del navegador el WebSocket sale por el mismo cliente HTTP que todo
// lo demás y lleva el token en la cabecera.
func opcionesWS(h *http.Client, token string) *websocket.DialOptions {
	o := &websocket.DialOptions{HTTPClient: h}
	if token != "" {
		o.HTTPHeader = http.Header{"Authorization": {"Bearer " + token}}
	}
	return o
}
