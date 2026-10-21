// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
)

// IA es el cliente de Sytue AI (services/ai, `/api/v1`). La web lo llama desde su BFF;
// una app nativa, en directo y con el token de la persona, por el ingress que el
// servicio abre para ellas (`ai.<apex>`, `deploy/helm/charts/ai`). El token de
// escritorio y el de móvil entran en su lista de audiencias (`FirstPartyAudiences`).
type IA struct{ c *Client }

// NewIA crea el cliente sobre la raíz del servicio (ServiceURL(apex, "ai")).
func NewIA(servicio string, s Session, h *http.Client) *IA {
	return &IA{c: New(servicio+"/api/v1", s, h)}
}

// PeticionIA es una petición a `/assist`. Lo que escribe una persona va SIEMPRE en
// Contexto y nunca en Instruccion: el contexto es dato y la instrucción es orden, y
// mezclarlos deja que un texto escrito a mano reescriba las reglas de la de arriba.
type PeticionIA struct {
	Accion, Tipo, Instruccion, Contexto string
	// Esfuerzo es none | low | medium | high: en visión, razonar menos se traga la
	// mitad de lo que hay en la foto; en texto, `none` acierta igual y cuesta menos.
	Esfuerzo string
	// Modo es el modelo: `interactivo` (rápido, el de por defecto) o `calidad` (acierta
	// más y tarda del orden de quince segundos; el servicio le da 120). Vacío no se manda.
	Modo string
	// Imagen es una foto para las tareas de visión, en JPEG (ImagenTipo por defecto).
	Imagen     []byte
	ImagenTipo string
	// Idioma es la lengua de la sesión: va en `Accept-Language`, que es lo que el
	// servicio usa para contestar en ella.
	Idioma string
}

// Asistir manda la consulta y devuelve el texto del modelo.
func (a *IA) Asistir(ctx context.Context, q PeticionIA) (string, error) {
	cuerpo := map[string]any{
		"action": q.Accion, "kind": q.Tipo, "instruction": q.Instruccion,
		"context": q.Contexto, "esfuerzo": q.Esfuerzo,
	}
	if q.Modo != "" {
		cuerpo["modo"] = q.Modo
	}
	if len(q.Imagen) > 0 {
		tipo := q.ImagenTipo
		if tipo == "" {
			tipo = "image/jpeg"
		}
		// `imagen_tipo` y no `imagenTipo`: con el nombre viejo el servicio lo ignora y la
		// foto viaja etiquetada como PNG siendo JPEG.
		cuerpo["imagen"], cuerpo["imagen_tipo"] = base64.StdEncoding.EncodeToString(q.Imagen), tipo
	}
	datos, err := json.Marshal(cuerpo)
	if err != nil {
		return "", err
	}
	h := http.Header{"Content-Type": {"application/json"}}
	if q.Idioma != "" {
		h.Set("Accept-Language", q.Idioma)
	}
	r, err := JSON[struct {
		Text string `json:"text"`
	}](ctx, a.c, Request{Method: http.MethodPost, Path: "/assist", Body: datos, Header: h})
	return r.Text, err
}

// IANoConfigurada dice si el fallo es que el servicio no tiene modelo (503
// `ai_not_configured`): la web lo dice con otro texto que el de «ha fallado».
func IANoConfigurada(err error) bool {
	var e *Error
	return errors.As(err, &e) && e.Status == http.StatusServiceUnavailable && e.Code() == "ai_not_configured"
}
