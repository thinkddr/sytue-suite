// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import (
	"context"
	"net/http"
)

// # Aviso de escrituras
//
// El widget nativo de una app se vuelve a publicar tras cada cambio (ADR 0078,
// enmienda del 2026-11-06). Las pantallas no saben qué peticiones cambian datos y no
// deberían tener que acordarse de avisar en cada botón: lo sabe el cliente HTTP. Un
// 2xx de POST, PUT, PATCH o DELETE del cliente del PROPIO producto avisa; el resto de
// clientes (presencia, avisos, preferencias, otros productos) no llevan este envoltorio.
//
// No avisan: las consultas por POST (Request.Lectura), lo que no cambia el widget
// (Request.SinAviso: el autoguardado de un borrador, la subida de un adjunto) y lo
// que corre con SinAviso(ctx), que es como lee la propia Fuente del widget.

type claveSinAviso struct{}

// SinAviso devuelve un contexto cuyas peticiones no cuentan como escrituras.
func SinAviso(ctx context.Context) context.Context {
	return context.WithValue(ctx, claveSinAviso{}, true)
}

func sinAvisoEn(ctx context.Context) bool {
	quitado, _ := ctx.Value(claveSinAviso{}).(bool)
	return quitado
}

// ConAvisoDeEscritura devuelve una copia de `base` (nil: el cliente con Transporte)
// que llama a `avisar` tras cada escritura con éxito. `avisar` corre en la rutina de
// la petición: tiene que volver enseguida.
func ConAvisoDeEscritura(base *http.Client, avisar func()) *http.Client {
	if base == nil {
		base = &http.Client{Transport: Transporte}
	}
	copia := *base
	transporte := copia.Transport
	if transporte == nil {
		transporte = http.DefaultTransport
	}
	copia.Transport = avisoDeEscritura{base: transporte, avisar: avisar}
	return &copia
}

type avisoDeEscritura struct {
	base   http.RoundTripper
	avisar func()
}

func (a avisoDeEscritura) RoundTrip(r *http.Request) (*http.Response, error) {
	resp, err := a.base.RoundTrip(r)
	if err == nil && resp.StatusCode >= 200 && resp.StatusCode < 300 && esEscritura(r.Method) &&
		!sinAvisoEn(r.Context()) && a.avisar != nil {
		a.avisar()
	}
	return resp, err
}

func esEscritura(metodo string) bool {
	switch metodo {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	}
	return false
}
