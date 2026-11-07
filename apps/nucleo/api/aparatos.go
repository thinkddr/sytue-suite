// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import (
	"context"
	"encoding/json"
	"net/http"
)

// # Los aparatos de iOS a los que despertar (ADR 0039, enmienda del 2026-11-06)
//
// El Hub guarda el registro único de tokens APNs de las quince apps y les manda un aviso
// VACÍO cuando cambia algo de su producto, para que repinten su widget con la app
// cerrada. La app solo se registra con el interruptor de la persona encendido
// (apps/fyne/sesion/despertar.go).

// RegistrarAparatoApple registra el token APNs de la app `bundle` en este aparato
// (`POST /me/push/apns`).
func (h *Hub) RegistrarAparatoApple(ctx context.Context, bundle, token string) error {
	return h.aparatos(ctx, "/me/push/apns", map[string]string{"bundle": bundle, "token": token})
}

// OlvidarAparatoApple borra ese token (`POST /me/push/apns/forget {token}`); vacío, TODOS
// los aparatos de la persona en su organización.
func (h *Hub) OlvidarAparatoApple(ctx context.Context, token string) error {
	cuerpo := map[string]string{}
	if token != "" {
		cuerpo["token"] = token
	}
	return h.aparatos(ctx, "/me/push/apns/forget", cuerpo)
}

func (h *Hub) aparatos(ctx context.Context, ruta string, cuerpo map[string]string) error {
	b, err := json.Marshal(cuerpo)
	if err != nil {
		return err
	}
	// SinAviso: registrarse u olvidarse no cambia lo que enseña ningún widget.
	resp, err := h.c.Do(ctx, Request{
		Method: http.MethodPost, Path: ruta, Body: b, SinAviso: true,
		Header: http.Header{"Content-Type": {"application/json"}},
	})
	if err != nil {
		return err
	}
	return resp.Body.Close()
}
