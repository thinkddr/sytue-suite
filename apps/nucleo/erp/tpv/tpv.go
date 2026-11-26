// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package tpv son las reglas del punto de venta, sin almacén: la forma del cobro, lo
// que se admite en una cesta, el importe en efectivo, el territorio del IVA y la
// línea de factura que sale de un producto. services/syerp (Postgres) y el ERP de Sytue
// Office (SQLite) las aplican igual; cada uno escribe la factura, su asiento, el
// cobro y la sesión de caja en su transacción.
//
// El cobro NO carga una tarjeta: registra un pago que ya hizo el terminal bancario.
package tpv

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"slices"
	"strconv"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/apps/nucleo/erp/dominio"
	"git.sytue.eu/sytue/one/apps/nucleo/validation"
)

// Linea es una línea de la cesta. Precio es el precio con IVA que se VIO: si el
// catálogo cambió entretanto, se pide revisar la cesta en vez de cobrar otra cosa.
type Linea struct {
	Producto string `json:"producto"`
	Cantidad int64  `json:"cantidad"`
	Precio   int64  `json:"precio"`
	TipoIVA  string `json:"tipo_iva"`
}

// Cobro es un cobro. Operacion es un UUID que pone quien cobra y hace el reintento
// seguro: la misma operación con la misma cesta devuelve la factura ya emitida.
type Cobro struct {
	Operacion string  `json:"operacion"`
	Sesion    string  `json:"sesion"`
	Cliente   string  `json:"cliente"`
	Medio     string  `json:"medio"`
	Recibido  int64   `json:"recibido"`
	Lineas    []Linea `json:"lineas"`
}

// Los medios de cobro.
const (
	Efectivo = "efectivo"
	Tarjeta  = "tarjeta"
)

// TopeSimplificada es lo más que se cobra sin identificar al cliente (factura
// simplificada), en céntimos: el tope general, sin presumir una actividad minorista.
const TopeSimplificada = 40000

// TopeEfectivo es desde dónde no se cobra en efectivo (Ley 7/2012, art. 7: el
// límite incluye exactamente 1.000 €).
const TopeEfectivo = 100000

// ExternoPublico es el `externo_id` del contacto genérico de las ventas sin cliente.
const ExternoPublico = "tpv:publico"

// Validar comprueba la forma del cobro antes de abrir la escritura.
func (c Cobro) Validar() error {
	if uuid.Validate(c.Operacion) != nil || uuid.Validate(c.Sesion) != nil || (c.Cliente != "" && uuid.Validate(c.Cliente) != nil) || (c.Medio != Efectivo && c.Medio != Tarjeta) || c.Recibido < 0 || c.Recibido > 1000000000 || len(c.Lineas) < 1 || len(c.Lineas) > 100 {
		return fmt.Errorf("cobro de TPV inválido: %w", dominio.ErrDatos)
	}
	vistos := map[string]bool{}
	for _, l := range c.Lineas {
		if uuid.Validate(l.Producto) != nil || vistos[l.Producto] || l.Cantidad < 1 || l.Cantidad > 1000 || l.Precio < 0 || l.Precio > 1000000 || !slices.Contains([]string{"21", "10", "5", "4", "0"}, l.TipoIVA) {
			return fmt.Errorf("línea de TPV inválida: %w", dominio.ErrDatos)
		}
		vistos[l.Producto] = true
	}
	return nil
}

// Huella es la del cuerpo del cobro: un reintento con la misma operación tiene que
// traer exactamente la misma cesta.
func (c Cobro) Huella() (string, error) {
	crudo, err := json.Marshal(c)
	if err != nil {
		return "", err
	}
	suma := sha256.Sum256(crudo)
	return hex.EncodeToString(suma[:]), nil
}

// ExigirTerritorio: este TPV usa el IVA de España peninsular y Baleares (no IGIC ni
// IPSI).
func ExigirTerritorio(pais, cp string) error {
	if pais != "ES" || (len(cp) >= 2 && slices.Contains([]string{"35", "38", "51", "52"}, cp[:2])) {
		return fmt.Errorf("este TPV utiliza IVA de España peninsular y Baleares: %w", dominio.ErrDatos)
	}
	return nil
}

// ValidarImporte comprueba el total de la venta contra lo recibido y el tope del
// efectivo. No se presume la excepción de particulares no residentes: el TPV no
// recoge todavía la documentación necesaria para justificarla.
func ValidarImporte(c Cobro, total int64) error {
	if total <= 0 || total > 1000000000 || (c.Medio == Efectivo && c.Recibido < total) {
		return fmt.Errorf("el importe recibido no cubre la venta: %w", dominio.ErrDatos)
	}
	if c.Medio == Efectivo && total >= TopeEfectivo {
		return fmt.Errorf("una venta de 1.000 € o más no se puede cobrar en efectivo; utiliza tarjeta: %w", dominio.ErrDatos)
	}
	return nil
}

// Cambio es lo que se devuelve en efectivo; con tarjeta, nada.
func Cambio(c Cobro, total int64) int64 {
	if c.Medio == Efectivo {
		return c.Recibido - total
	}
	return 0
}

// TipoFactura es el de la factura que se emite: completa (F1) con un cliente que
// tiene NIF válido, simplificada (F2) sin cliente y hasta TopeSimplificada.
func TipoFactura(cliente, nifCliente string, total int64) (string, error) {
	if cliente != "" {
		if !validation.NIF(nifCliente) {
			return "", fmt.Errorf("el cliente necesita un NIF válido: %w", dominio.ErrDatos)
		}
		return "F1", nil
	}
	if total > TopeSimplificada {
		return "", fmt.Errorf("por encima de 400 € identifica al cliente para emitir factura completa: %w", dominio.ErrDatos)
	}
	return "F2", nil
}

// LineaDe es la línea de factura de una línea de la cesta con el producto tal como
// está ahora (su etiqueta, su estado y sus datos). Rechaza la cesta si el precio o
// el IVA cambiaron, si el producto está en pausa o si está exento.
func LineaDe(l Linea, etiqueta, estado string, producto map[string]any) (dominio.LineaFactura, error) {
	precio, err := dominio.Centimos(producto, "precio_con_iva")
	tipoProducto, _ := producto["tipo_iva"].(string)
	if err != nil || producto["precio_con_iva"] == nil || precio != l.Precio || tipoProducto != l.TipoIVA || estado == "pausada" {
		return dominio.LineaFactura{}, fmt.Errorf("revisa el precio y el IVA de %s: %w", etiqueta, dominio.ErrDatos)
	}
	tipo, err := strconv.ParseInt(l.TipoIVA, 10, 64)
	if err != nil {
		return dominio.LineaFactura{}, err
	}
	if tipo == 0 {
		return dominio.LineaFactura{}, fmt.Errorf("el TPV solo vende productos sujetos a IVA; las exenciones se facturan desde facturación: %w", dominio.ErrDatos)
	}
	return dominio.LineaFactura{Descripcion: etiqueta, Cantidad: strconv.FormatInt(l.Cantidad, 10), Precio: precio, Tipo: tipo, Importe: precio * l.Cantidad}, nil
}

// Totales son base, IVA y total de las líneas, por desglose de grupo.
func Totales(lineas []dominio.LineaFactura) (base, iva, total int64) {
	for _, g := range dominio.Desglose(lineas) {
		base += g.Base
		iva += g.Cuota
	}
	return base, iva, base + iva
}

// DatosFactura son los campos de la factura de venta que emite el cobro.
func DatosFactura(c Cobro, cliente, tipo, fecha, huella string, base, iva int64) map[string]any {
	return map[string]any{
		"cliente": cliente, "fecha": fecha, "base": base, "iva": iva, "total": base + iva,
		"tipo_factura": tipo, "regimen_iva": "iva_general", "tpv_operacion": c.Operacion, "sesion": c.Sesion,
		"medio_pago": c.Medio, "tpv_huella": huella,
	}
}

// DatosLinea son los campos de la `linea_factura` de una línea.
func DatosLinea(factura string, l dominio.LineaFactura) map[string]any {
	return map[string]any{
		"factura": factura, "descripcion": l.Descripcion, "cantidad": l.Cantidad,
		"precio_con_iva": l.Precio, "tipo_iva": strconv.FormatInt(l.Tipo, 10),
	}
}

// Caja son la cuenta y el diario del cobro: 570 caja en efectivo, 572 bancos con tarjeta.
func Caja(medio string) (cuenta, diario string) {
	if medio == Efectivo {
		return "570000", "caja"
	}
	return "572000", "bancos"
}

// ParaPresupuesto prepara la cesta como presupuesto: no pide caja abierta ni importe
// recibido, sí un cliente.
func (c Cobro) ParaPresupuesto() (Cobro, error) {
	if c.Cliente == "" {
		return c, fmt.Errorf("elige el cliente del presupuesto: %w", dominio.ErrDatos)
	}
	c.Sesion, c.Medio, c.Recibido = c.Operacion, Tarjeta, 0
	return c, c.Validar()
}
