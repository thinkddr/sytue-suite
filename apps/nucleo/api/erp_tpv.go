// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import (
	"context"
	"encoding/json"
	"maps"
	"net/http"
	"net/url"
	"strings"
)

// # El punto de venta (`PuntoVenta.tsx`)
//
// Las formas son las de `services/syerp/internal/store/tpv.go` (LineaTPV, CobroTPV,
// VentaTPV). El cobro NO carga una tarjeta: registra un pago que ya hizo el terminal
// bancario, y el servicio confirma factura, asiento, cobro y sesión en una sola
// transacción.

// ERPModeloSesionTPV es la clave del modelo de las sesiones de caja.
const ERPModeloSesionTPV = "sesion_tpv"

// Los dos medios de cobro que admite el servicio.
const (
	ERPMedioEfectivo = "efectivo"
	ERPMedioTarjeta  = "tarjeta"
)

// ERPLineaTPV es una línea de la cesta. `Precio` es el precio con IVA que se VIO, en
// céntimos: si el catálogo cambió entretanto, el servicio rechaza el cobro y pide revisar
// la cesta en vez de cobrar otra cosa.
type ERPLineaTPV struct {
	Producto string `json:"producto"`
	Cantidad int64  `json:"cantidad"`
	Precio   int64  `json:"precio"`
	TipoIVA  string `json:"tipo_iva"`
}

// ERPCobroTPV es un cobro. `Operacion` es un UUID que pone quien cobra y es lo que hace
// el reintento seguro: el servicio reconoce la misma operación y devuelve la factura que
// ya emitió en vez de emitir otra. Por eso un reintento manda EXACTAMENTE el mismo cuerpo
// —el servicio compara también su huella—.
type ERPCobroTPV struct {
	Operacion string        `json:"operacion"`
	Sesion    string        `json:"sesion"`
	Cliente   string        `json:"cliente"`
	Medio     string        `json:"medio"`
	Recibido  int64         `json:"recibido"`
	Lineas    []ERPLineaTPV `json:"lineas"`
}

// ERPVentaTPV es lo que devuelve un cobro: la factura emitida y el cambio en céntimos.
type ERPVentaTPV struct {
	Factura ERPRegistro `json:"factura"`
	Cambio  int64       `json:"cambio"`
}

// CobrarTPV registra un cobro (`POST /api/v1/tpv/cobrar`). No reintenta solo: tras un
// error de red no se sabe si la venta se registró, y es la pantalla la que vuelve a
// llamar con el MISMO cobro.
func (e *ERP) CobrarTPV(ctx context.Context, c ERPCobroTPV) (ERPVentaTPV, error) {
	datos, err := json.Marshal(c)
	if err != nil {
		return ERPVentaTPV{}, err
	}
	r := e.pedir("/tpv/cobrar", nil)
	r.Method, r.Body = http.MethodPost, datos
	r.Header.Set("Content-Type", "application/json")
	return JSON[ERPVentaTPV](ctx, e.c, r)
}

// ERPPresupuestoTPV es «Crear presupuesto» desde la cesta: el cliente y las líneas tal
// como se vieron. `Operacion` hace el reintento seguro igual que en el cobro: la misma
// operación devuelve el presupuesto que ya se creó.
type ERPPresupuestoTPV struct {
	Operacion string        `json:"operacion"`
	Cliente   string        `json:"cliente"`
	Lineas    []ERPLineaTPV `json:"lineas"`
}

// PresupuestoTPV crea un presupuesto con la cesta (`POST /api/v1/tpv/presupuesto`) y lo
// devuelve, para abrirlo. No necesita caja abierta ni cobra nada.
func (e *ERP) PresupuestoTPV(ctx context.Context, p ERPPresupuestoTPV) (ERPRegistro, error) {
	return e.escribir(ctx, http.MethodPost, "/tpv/presupuesto", p)
}

// MaxImagenesProductos es cuántos productos admite una consulta de imágenes: el tope
// del servicio (`maxImagenesProductos`).
const MaxImagenesProductos = 200

// ImagenesProductos dice qué adjunto es la imagen de cada producto que la tiene
// (`GET /api/v1/productos/imagenes?registros=…`). Solo identificadores: los bytes se
// bajan con [ERP.Adjunto], por la descarga autenticada de siempre. Parte la consulta en
// tandas de [MaxImagenesProductos].
func (e *ERP) ImagenesProductos(ctx context.Context, ids []string) (map[string]string, error) {
	out := map[string]string{}
	for i := 0; i < len(ids); i += MaxImagenesProductos {
		tanda := ids[i:min(i+MaxImagenesProductos, len(ids))]
		r, err := JSON[struct {
			Imagenes map[string]string `json:"imagenes"`
		}](ctx, e.c, e.pedir("/productos/imagenes", url.Values{"registros": {strings.Join(tanda, ",")}}))
		if err != nil {
			return nil, err
		}
		maps.Copy(out, r.Imagenes)
	}
	return out, nil
}
