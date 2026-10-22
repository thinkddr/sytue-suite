// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import (
	"context"
	"net/http"
	"net/url"
)

// # El banco como tareas (`Banco.tsx`)
//
// Cada movimiento del extracto es una tarea hasta que tiene su factura o su
// explicación. Las formas son las de `services/syerp/internal/store/banca.go`
// (LineaBanco, FacturaAbierta) y las de `internal/http/banca.go` (tareaBanco,
// resultadoN43).

// ERPMovimientoBanco es un movimiento del extracto. `Importe` va en céntimos: positivo
// si entra dinero, negativo si sale.
type ERPMovimientoBanco struct {
	ID            string `json:"id"`
	Fecha         string `json:"fecha"`
	Descripcion   string `json:"descripcion"`
	Importe       int64  `json:"importe"`
	FacturaID     string `json:"factura_id,omitempty"`
	FacturaModelo string `json:"factura_modelo,omitempty"`
	FacturaRef    string `json:"factura_ref,omitempty"`
	Motivo        string `json:"motivo,omitempty"`
	// La pista de Sytue AI: quién lo cobró y dónde conseguir su factura.
	Empresa       string `json:"empresa,omitempty"`
	Web           string `json:"web,omitempty"`
	URLFactura    string `json:"url_factura,omitempty"`
	EmailFacturas string `json:"email_facturas,omitempty"`
}

// ERPFacturaCandidata es una factura sin cobrar o sin pagar que podría ser la de un
// movimiento. `Segura`: el concepto del banco la nombra y no hay otra igual.
type ERPFacturaCandidata struct {
	ID      string `json:"id"`
	Modelo  string `json:"modelo"`
	Ref     string `json:"ref"`
	Estado  string `json:"estado"`
	Tercero string `json:"tercero"`
	Fecha   string `json:"fecha"`
	Total   int64  `json:"total"`
	Segura  bool   `json:"segura"`
}

// ERPTareaBanco es un movimiento pendiente con el motivo que parece y sus candidatas.
type ERPTareaBanco struct {
	ERPMovimientoBanco
	Sugerencia string                `json:"sugerencia,omitempty"`
	Candidatas []ERPFacturaCandidata `json:"candidatas"`
}

// ERPMotivoBanco es una explicación de un movimiento sin factura, con su rótulo.
type ERPMotivoBanco struct {
	Clave  string `json:"clave"`
	Rotulo string `json:"rotulo"`
}

// ERPBanco son las tareas del banco y los últimos movimientos justificados.
type ERPBanco struct {
	Pendientes      []ERPTareaBanco      `json:"pendientes"`
	TotalPendientes int                  `json:"total_pendientes"`
	Hechas          []ERPMovimientoBanco `json:"hechas"`
	Motivos         []ERPMotivoBanco     `json:"motivos"`
}

// Banco pide las tareas (`GET /api/v1/banco`).
func (e *ERP) Banco(ctx context.Context) (ERPBanco, error) {
	return JSON[ERPBanco](ctx, e.c, e.pedir("/banco", nil))
}

// ConciliarMovimiento casa un movimiento con una factura.
func (e *ERP) ConciliarMovimiento(ctx context.Context, id, factura string) error {
	return e.enviar(ctx, "/banco/"+url.PathEscape(id)+"/conciliar", map[string]string{"factura": factura})
}

// ExplicarMovimiento justifica un movimiento sin factura con un motivo.
func (e *ERP) ExplicarMovimiento(ctx context.Context, id, motivo string) error {
	return e.enviar(ctx, "/banco/"+url.PathEscape(id)+"/explicar", map[string]string{"motivo": motivo})
}

// ReabrirMovimiento deshace la justificación: vuelve a ser una tarea.
func (e *ERP) ReabrirMovimiento(ctx context.Context, id string) error {
	return e.enviar(ctx, "/banco/"+url.PathEscape(id)+"/reabrir", struct{}{})
}

// PistasBanco le pasa a Sytue AI los cargos sin pista y dice cuántos buscó.
func (e *ERP) PistasBanco(ctx context.Context) (int, error) {
	r := e.pedir("/banco/pistas", nil)
	r.Method, r.Body = http.MethodPost, []byte("{}")
	r.Header.Set("Content-Type", "application/json")
	res, err := JSON[struct {
		Buscadas int `json:"buscadas"`
	}](ctx, e.c, r)
	return res.Buscadas, err
}

// ERPResultadoExtracto es lo que pasó al subir un extracto Norma 43.
type ERPResultadoExtracto struct {
	Cuentas    []string `json:"cuentas"`
	Nuevos     int      `json:"nuevos"`
	Repetidos  int      `json:"repetidos"`
	Casados    int      `json:"casados"`
	Explicados int      `json:"explicados"`
	Pendientes int      `json:"pendientes"`
}

// ImportarExtracto sube un Norma 43 EN BRUTO (`POST /api/v1/linea_extracto/importar`):
// los bancos lo escriben en ISO-8859-1 y el servicio lo lee como tal; pasarlo por UTF-8
// rompería cada «Ñ».
func (e *ERP) ImportarExtracto(ctx context.Context, datos []byte) (ERPResultadoExtracto, error) {
	r := e.pedir("/linea_extracto/importar", nil)
	r.Method, r.Body = http.MethodPost, datos
	r.Header.Set("Content-Type", "text/plain")
	return JSON[ERPResultadoExtracto](ctx, e.c, r)
}
