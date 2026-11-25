// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// La cartera del ERP (`services/syerp/internal/http/cartera.go`): la antigüedad de lo
// pendiente, el estado de cuenta de un contacto, los productos bajo su stock mínimo y
// el libro mayor de una cuenta; y lo que la lista y la ficha piden además desde la ola
// del 2026-11-25: los recuentos con su búsqueda y su estado, exportar en el orden de la
// lista y el recordatorio de pago. Los mismos cables que `apps/frontend/apps/erp`.

// ERPTramoAntiguedad es un tramo de la antigüedad: «al_dia», «1_30», «31_60», «61_90» o
// «mas_90», con lo que suma (céntimos) y cuántos documentos.
type ERPTramoAntiguedad struct {
	Tramo      string `json:"tramo"`
	Centimos   int64  `json:"centimos"`
	Documentos int    `json:"documentos"`
}

// ERPAntiguedad es lo pendiente de cobro o de pago en cinco tramos de días vencido.
type ERPAntiguedad struct {
	Modelo  string               `json:"modelo"`
	Hoy     string               `json:"hoy"`
	Tramos  []ERPTramoAntiguedad `json:"tramos"`
	Total   int64                `json:"total"`
	Vencido int64                `json:"vencido"`
}

// Antiguedad pide la antigüedad de `tipo` («cobros» o «pagos») a `hoy` (AAAA-MM-DD, el
// día del dispositivo).
func (e *ERP) Antiguedad(ctx context.Context, tipo, hoy string) (ERPAntiguedad, error) {
	return JSON[ERPAntiguedad](ctx, e.c, e.pedir("/cobros/antiguedad", url.Values{"tipo": {tipo}, "hoy": {hoy}}))
}

// ERPResumenCuenta es un lado del estado de cuenta, en céntimos.
type ERPResumenCuenta struct {
	Facturado int64 `json:"facturado"`
	Pagado    int64 `json:"pagado"`
	Pendiente int64 `json:"pendiente"`
	Vencido   int64 `json:"vencido"`
}

// ERPDocumentoCuenta es una factura del estado de cuenta.
type ERPDocumentoCuenta struct {
	ID          string `json:"id"`
	Modelo      string `json:"modelo"`
	Ref         string `json:"ref"`
	Fecha       string `json:"fecha"`
	Vencimiento string `json:"vencimiento,omitempty"`
	Estado      string `json:"estado"`
	Total       int64  `json:"total"`
	DiasVencida int    `json:"dias_vencida"`
}

// ERPEstadoCuenta es la relación económica con un contacto.
type ERPEstadoCuenta struct {
	Ventas     ERPResumenCuenta     `json:"ventas"`
	Compras    ERPResumenCuenta     `json:"compras"`
	Documentos []ERPDocumentoCuenta `json:"documentos"`
	Recortado  bool                 `json:"recortado"`
}

// EstadoDeCuenta pide el estado de cuenta de un contacto a `hoy`.
func (e *ERP) EstadoDeCuenta(ctx context.Context, contacto, hoy string) (ERPEstadoCuenta, error) {
	return JSON[ERPEstadoCuenta](ctx, e.c, e.pedir("/contacto/"+url.PathEscape(contacto)+"/estado-cuenta", url.Values{"hoy": {hoy}}))
}

// ERPProductoBajoMinimo es un producto con menos existencias que su mínimo; las
// cantidades, como cadena decimal.
type ERPProductoBajoMinimo struct {
	ID        string `json:"id"`
	Ref       string `json:"ref,omitempty"`
	Nombre    string `json:"nombre"`
	Minimo    string `json:"minimo"`
	Stock     string `json:"stock"`
	Unidad    string `json:"unidad,omitempty"`
	Faltan    string `json:"faltan"`
	Almacenes int    `json:"almacenes"`
}

// BajoMinimo pide los productos por debajo de su stock mínimo, los más apurados antes.
func (e *ERP) BajoMinimo(ctx context.Context) ([]ERPProductoBajoMinimo, error) {
	r, err := JSON[struct {
		Productos []ERPProductoBajoMinimo `json:"productos"`
	}](ctx, e.c, e.pedir("/inventario/bajo-minimo", nil))
	return r.Productos, err
}

// ERPApunteMayor es una línea del mayor con el saldo que deja.
type ERPApunteMayor struct {
	AsientoID string `json:"asiento_id"`
	Ref       string `json:"ref"`
	Fecha     string `json:"fecha"`
	Concepto  string `json:"concepto"`
	Debe      int64  `json:"debe"`
	Haber     int64  `json:"haber"`
	Saldo     int64  `json:"saldo"`
}

// ERPMayor es el libro mayor de una cuenta en un período.
type ERPMayor struct {
	Cuenta       string           `json:"cuenta"`
	SaldoInicial int64            `json:"saldo_inicial"`
	Apuntes      []ERPApunteMayor `json:"apuntes"`
	Debe         int64            `json:"debe"`
	Haber        int64            `json:"haber"`
	SaldoFinal   int64            `json:"saldo_final"`
	Recortado    bool             `json:"recortado"`
}

// Mayor pide el mayor de una cuenta entre `desde` y `hasta` (AAAA-MM-DD; vacíos, sin
// acotar).
func (e *ERP) Mayor(ctx context.Context, cuenta, desde, hasta string) (ERPMayor, error) {
	q := url.Values{"cuenta": {cuenta}}
	periodo(q, desde, hasta)
	return JSON[ERPMayor](ctx, e.c, e.pedir("/mayor", q))
}

// EstadisticasDe es Estadisticas con la búsqueda y el estado de la lista: el recuento y
// la suma de la barra cuentan lo que se ve. En la fuente privada del dispositivo vale el
// recuento de siempre, del modelo entero.
func (e *ERP) EstadisticasDe(ctx context.Context, modelo, q, estado string) (ERPEstadisticas, error) {
	if e.private != nil {
		return e.private.Estadisticas(ctx, modelo)
	}
	consulta := url.Values{}
	for clave, valor := range map[string]string{"q": q, "estado": estado} {
		if valor != "" {
			consulta.Set(clave, valor)
		}
	}
	return JSON[ERPEstadisticas](ctx, e.c, e.pedir("/"+url.PathEscape(modelo)+"/estadisticas", consulta))
}

// ExportarEnOrden es Exportar con el orden de la lista (`orden`, la clave del campo; `asc`
// su sentido). Sin orden, el de siempre.
func (e *ERP) ExportarEnOrden(ctx context.Context, modelo, q, estado string, ids []string, orden string, asc bool) (io.ReadCloser, string, error) {
	if e.private != nil || orden == "" || len(ids) > 0 {
		return e.Exportar(ctx, modelo, q, estado, ids)
	}
	consulta := url.Values{"orden": {orden}}
	if asc {
		consulta.Set("dir", "asc")
	}
	for clave, valor := range map[string]string{"q": q, "estado": estado} {
		if valor != "" {
			consulta.Set(clave, valor)
		}
	}
	resp, err := e.c.Do(ctx, e.pedir("/"+url.PathEscape(modelo)+"/export", consulta))
	if err != nil {
		return nil, "", err
	}
	return resp.Body, nombreAdjunto(resp.Header.Get("Content-Disposition"), modelo+".csv"), nil
}

// ERPDatosEnvio es lo que el diálogo de envío necesita saber antes de abrirse: la
// dirección de facturación del cliente, si la factura se puede enviar ya y si el
// despliegue tiene correo saliente.
type ERPDatosEnvio struct {
	Para       string `json:"para"`
	Enviable   bool   `json:"enviable"`
	Disponible bool   `json:"disponible"`
}

// DatosEnvio pide los datos del envío de una factura de cliente.
func (e *ERP) DatosEnvio(ctx context.Context, factura string) (ERPDatosEnvio, error) {
	return JSON[ERPDatosEnvio](ctx, e.c, e.pedir("/"+ERPFacturaVenta+"/"+url.PathEscape(factura)+"/enviar", nil))
}

// EnviarFactura manda la factura en PDF a una dirección. Con `recordatorio`, es un
// recordatorio de pago de una factura abierta y queda así en el historial.
func (e *ERP) EnviarFactura(ctx context.Context, factura, para, asunto, mensaje string, recordatorio bool) error {
	cuerpo := map[string]any{"para": strings.TrimSpace(para), "asunto": strings.TrimSpace(asunto), "mensaje": mensaje}
	if recordatorio {
		cuerpo["recordatorio"] = true
	}
	datos, err := json.Marshal(cuerpo)
	if err != nil {
		return err
	}
	r := e.pedir("/"+ERPFacturaVenta+"/"+url.PathEscape(factura)+"/enviar", nil)
	r.Method, r.Body = http.MethodPost, datos
	r.Header.Set("Content-Type", "application/json")
	resp, err := e.c.Do(ctx, r)
	if err != nil {
		return err
	}
	_ = resp.Body.Close()
	return nil
}
