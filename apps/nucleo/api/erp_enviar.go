// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import (
	"context"
	"io"
	"net/url"
)

// La factura de cliente en PDF y enviada por correo con el PDF adjunto: las rutas
// `/{modelo}/{id}/pdf` y `/{modelo}/{id}/enviar` de services/syerp
// (`internal/http/enviar_factura.go`), que la edición local sirve igual en proceso
// (`apps/nucleo/local/erp`). Sin la fachada privada: con ella, el cliente falla cerrado
// como en el resto de rutas que no tiene.

// PDFFactura devuelve la factura en PDF —el mismo impreso que Imprimir, escrito con
// apps/nucleo/pdfdoc— y el nombre con que la nombra el servicio
// (`GET /api/v1/{modelo}/{id}/pdf`).
//
// Quien llama cierra el lector.
func (e *ERP) PDFFactura(ctx context.Context, modelo, id string) (io.ReadCloser, string, error) {
	return e.documentoConNombre(ctx, modelo, id, "/pdf", "factura"+extPDF)
}

// extPDF va aparte: «factura.pdf» escrito de una vez tiene forma de clave de catálogo.
const extPDF = ".pdf"

// ERPCuentaCorreo es una cuenta desde la que sale la factura cuando el servicio deja
// elegirla: en la edición local, cada cuenta de correo de Sytue Mail de este equipo.
type ERPCuentaCorreo struct {
	ID        string `json:"id"`
	Direccion string `json:"direccion"`
	Nombre    string `json:"nombre,omitempty"`
}

// ERPEnvioFactura es con qué arranca el diálogo de enviar
// (`GET /api/v1/{modelo}/{id}/enviar`): la dirección de facturación del cliente (o la
// general), si la factura se puede enviar ya y si hay con qué.
type ERPEnvioFactura struct {
	Para string `json:"para"`
	// Enviable: ni borrador ni anulada.
	Enviable bool `json:"enviable"`
	// Disponible: hay servidor de correo (en la suite) o alguna cuenta (en local).
	Disponible bool `json:"disponible"`
	// PorCuenta: la factura sale por una cuenta de correo de quien envía, una de
	// Cuentas, y no por el buzón del servicio. El servicio de la suite no lo dice.
	PorCuenta bool              `json:"por_cuenta,omitempty"`
	Cuentas   []ERPCuentaCorreo `json:"cuentas,omitempty"`
	// Asunto y Mensaje son los que propone el servicio (en local, con el número, el
	// importe y el vencimiento). Vacíos, la pantalla pone los de su catálogo.
	Asunto  string `json:"asunto,omitempty"`
	Mensaje string `json:"mensaje,omitempty"`
}

// EnvioFactura pide con qué arranca el diálogo de enviar la factura.
func (e *ERP) EnvioFactura(ctx context.Context, modelo, id string) (ERPEnvioFactura, error) {
	return JSON[ERPEnvioFactura](ctx, e.c, e.pedir("/"+url.PathEscape(modelo)+"/"+url.PathEscape(id)+"/enviar", nil))
}

// ERPCorreoFactura es el correo de la factura: a UNA dirección, con el asunto y el texto
// que haya dejado quien envía. Cuenta es la de ERPEnvioFactura.Cuentas por la que sale;
// vacía, la primera (y el servicio de la suite la ignora).
type ERPCorreoFactura struct {
	Para    string `json:"para"`
	Asunto  string `json:"asunto"`
	Mensaje string `json:"mensaje"`
	Cuenta  string `json:"cuenta,omitempty"`
}

// EnviarCorreoFactura manda la factura en PDF por correo, por la cuenta elegida, y la
// deja apuntada en su historial (`POST /api/v1/{modelo}/{id}/enviar`). Un 502
// `correo_no_enviado` es que el servidor de correo no la aceptó; si la respuesta se
// pierde, puede haber salido. El recordatorio de pago es EnviarFactura (erp_cartera.go).
func (e *ERP) EnviarCorreoFactura(ctx context.Context, modelo, id string, c ERPCorreoFactura) error {
	_, err := postJSON[struct {
		Enviada bool `json:"enviada"`
	}](ctx, e, "/"+url.PathEscape(modelo)+"/"+url.PathEscape(id)+"/enviar", c)
	return err
}
