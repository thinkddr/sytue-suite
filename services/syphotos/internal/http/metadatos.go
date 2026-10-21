// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package http

import (
	"context"
	"net/http"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/log"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
)

// VentanaMetadatos es cada cuánto, como mucho, se intenta completar el tipo y el
// tamaño de las fotos de una persona (service/metadatos.go). Una pasada que se paró
// con el token caducado vuelve a intentarlo en la siguiente visita pasada la hora;
// una que acabó no encuentra nada que hacer y cuesta una consulta.
const VentanaMetadatos = time.Hour

// completarMetadatos lanza, en segundo plano y con el token de quien abre su galería,
// la pasada que rellena el tipo y el tamaño de sus fotos indexadas sin ellos.
//
// Con la sesión de la persona porque leer de Drive la exige, y en segundo plano
// porque la línea de tiempo no puede esperar a cientos de llamadas a Drive.
func (h *Handlers) completarMetadatos(r *http.Request) {
	token, yo := bearerDe(r), sub(r)
	if h.ingesta == nil || token == "" || yo == "" {
		return
	}
	if !h.completando.Primera(tenancy.MustFrom(r.Context()).Slug + "|" + yo) {
		return
	}
	// Sin la cancelación de la petición —que acaba al contestar— pero con sus valores:
	// la organización va en el contexto y el store la necesita.
	ctx, cancelar := context.WithTimeout(context.WithoutCancel(r.Context()), 10*time.Minute)
	go func() {
		defer cancelar()
		c, err := h.ingesta.CompletarMetadatos(ctx, token, yo)
		if err != nil {
			log.From(ctx).Warn("no se completaron el tipo y el tamaño de las fotos",
				"arregladas", c.Arreglados, "sin_original", c.SinOriginal, "err", err)
			return
		}
		if c.Arreglados > 0 || c.SinOriginal > 0 {
			log.From(ctx).Info("tipo y tamaño de las fotos completados",
				"arregladas", c.Arreglados, "sin_original", c.SinOriginal)
		}
	}()
}
