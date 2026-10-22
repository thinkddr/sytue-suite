// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import (
	"context"
	"net/http"
)

// # Etiquetas de sensibilidad
//
// La TAXONOMÍA la define un administrador en Sytue Admin y vive en Sytue ID; Drive la
// lee con el mismo token (`GET /sensitivity-labels`) y solo la APLICA a un fichero. El
// cliente manda el id de la etiqueta y nada más: la política —si bloquea el enlace
// público— la resuelve el servicio contra la taxonomía, así que no se puede falsificar.

// EtiquetaDrive es una etiqueta de la taxonomía de la organización.
type EtiquetaDrive struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Rank int    `json:"rank"`
	// BlockExternalShare: con ella puesta, el servicio rechaza cualquier enlace público.
	BlockExternalShare bool `json:"block_external_share"`
}

// SensitivityLabels es la taxonomía de la organización de quien pregunta.
func (d *Drive) SensitivityLabels(ctx context.Context) ([]EtiquetaDrive, error) {
	r, err := JSON[struct {
		Labels []EtiquetaDrive `json:"labels"`
	}](ctx, d.c, Request{Path: "/sensitivity-labels"})
	return r.Labels, err
}

// SetLabel clasifica un fichero con una etiqueta de la taxonomía; `etiqueta` vacía le
// quita la que tenga. El servicio rechaza (400) un id que no sea de ESTA organización.
func (d *Drive) SetLabel(ctx context.Context, fichero, etiqueta string) error {
	ruta := elemento(DriveFile, fichero, "/label")
	if etiqueta == "" {
		return d.mandar(ctx, http.MethodDelete, ruta, nil)
	}
	return d.mandar(ctx, http.MethodPut, ruta, map[string]string{"label_id": etiqueta})
}
