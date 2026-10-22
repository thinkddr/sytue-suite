// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
)

// ERPProcesoObligacion es el avance del proceso fiscal compartido con todas las
// personas que pueden consultar las obligaciones de la organización.
type ERPProcesoObligacion struct {
	Fase       int    `json:"fase"`
	Referencia string `json:"referencia"`
}

// ProcesoObligacion lee el estado persistido; abrir una vista no completa pasos.
func (e *ERP) ProcesoObligacion(ctx context.Context, clave, periodo string) (ERPProcesoObligacion, error) {
	return JSON[ERPProcesoObligacion](ctx, e.c, e.pedir("/obligaciones/proceso", url.Values{"clave": {clave}, "periodo": {periodo}}))
}

// AvanzarProcesoObligacion pide al servicio comprobar y guardar el siguiente paso.
// La respuesta puede haber avanzado por otra persona; siempre se usa la del servidor.
func (e *ERP) AvanzarProcesoObligacion(ctx context.Context, clave, periodo string, fase int, referencia string) (ERPProcesoObligacion, error) {
	cuerpo, err := json.Marshal(ERPProcesoObligacion{Fase: fase, Referencia: referencia})
	if err != nil {
		return ERPProcesoObligacion{}, err
	}
	r := e.pedir("/obligaciones/proceso", url.Values{"clave": {clave}, "periodo": {periodo}})
	r.Method, r.Body = http.MethodPost, cuerpo
	r.Header.Set("Content-Type", "application/json")
	return JSON[ERPProcesoObligacion](ctx, e.c, r)
}

// FacturasPendientesObligacion pagina los pendientes del período. El servicio
// limita el resultado a los modelos de facturación que la sesión puede leer.
func (e *ERP) FacturasPendientesObligacion(ctx context.Context, clave, periodo, cursor string, limite int) (ERPPagina, error) {
	q := url.Values{"clave": {clave}, "periodo": {periodo}}
	if cursor != "" {
		q.Set("cursor", cursor)
	}
	if limite > 0 {
		q.Set("limite", strconv.Itoa(limite))
	}
	return JSON[ERPPagina](ctx, e.c, e.pedir("/obligaciones/proceso/pendientes", q))
}
