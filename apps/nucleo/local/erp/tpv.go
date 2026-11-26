// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package erp

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/erp/catalogo"
	"git.sytue.eu/sytue/one/apps/nucleo/erp/dominio"
	"git.sytue.eu/sytue/one/apps/nucleo/erp/tpv"
)

// El punto de venta: las reglas son las de apps/nucleo/erp/tpv, las mismas del
// servidor; aquí la factura, sus líneas, su asiento, el del cobro y la sesión de caja
// se escriben en una sola transacción de SQLite. Sin VERI*FACTU (D-29).

// VentaTPV es lo que devuelve un cobro: la factura emitida y el cambio.
type VentaTPV struct {
	Factura api.ERPRegistro `json:"factura"`
	Cambio  int64           `json:"cambio"`
}

func (s *Servidor) rutasTPV(h func(string, http.HandlerFunc)) {
	h("POST /api/v1/tpv/cobrar", s.cobrarTPV)
	h("POST /api/v1/tpv/presupuesto", s.presupuestoTPV)
}

func (s *Servidor) cobrarTPV(w http.ResponseWriter, r *http.Request) {
	var c tpv.Cobro
	if err := leer(r, &c); err != nil {
		falloDe(w, err)
		return
	}
	v, err := s.alm.CobrarTPV(r.Context(), c)
	if err != nil {
		falloDe(w, err)
		return
	}
	responder(w, http.StatusOK, v)
}

func (s *Servidor) presupuestoTPV(w http.ResponseWriter, r *http.Request) {
	var c tpv.Cobro
	if err := leer(r, &c); err != nil {
		falloDe(w, err)
		return
	}
	p, err := s.alm.PresupuestoTPV(r.Context(), c)
	if err != nil {
		falloDe(w, err)
		return
	}
	responder(w, http.StatusOK, p)
}

// ErrModulosApagados: el TPV escribe en módulos que no están encendidos.
var ErrModulosApagados = fmt.Errorf("enciende los módulos del TPV: %w", dominio.ErrDatos)

func (a *Almacen) exigirModulos(ctx context.Context, tx *sql.Tx, modulos ...string) error {
	for _, m := range modulos {
		var n int
		if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM modulo WHERE clave = ?`, m).Scan(&n); err != nil {
			return err
		}
		if n == 0 {
			return ErrModulosApagados
		}
	}
	return nil
}

func (a *Almacen) modelo(clave string) *catalogo.Modelo {
	m, _ := a.cat.Modelo(clave)
	return m
}

// lineasTPV leen cada producto de la cesta tal como está ahora.
func (a *Almacen) lineasTPV(ctx context.Context, tx *sql.Tx, entradas []tpv.Linea) ([]dominio.LineaFactura, error) {
	out := make([]dominio.LineaFactura, 0, len(entradas))
	for _, l := range entradas {
		p, err := obtener(ctx, tx, "producto", l.Producto)
		if err != nil {
			return nil, err
		}
		linea, err := tpv.LineaDe(l, p.Etiqueta, p.Estado, p.Data)
		if err != nil {
			return nil, err
		}
		out = append(out, linea)
	}
	return out, nil
}

// porOperacion es el documento de `modelo` que ya registró esa operación, si lo hay;
// con otra cesta (otra huella), es un error.
func (a *Almacen) porOperacion(ctx context.Context, tx *sql.Tx, modelo, operacion, huella string) (api.ERPRegistro, bool, error) {
	r, hay, err := a.buscarPor(ctx, tx, modelo, "tpv_operacion", operacion)
	if err != nil || !hay {
		return r, false, err
	}
	if h, _ := r.Data["tpv_huella"].(string); h != huella {
		return r, false, fmt.Errorf("la operación ya se usó para otra cesta: %w", dominio.ErrDatos)
	}
	return r, true, nil
}

// clienteTPV es el cliente de la factura y su tipo: el elegido (con NIF válido) o el
// contacto genérico del TPV para una simplificada.
func (a *Almacen) clienteTPV(ctx context.Context, tx *sql.Tx, cliente string, total int64) (string, string, error) {
	if cliente != "" {
		c, err := obtener(ctx, tx, "contacto", cliente)
		if err != nil {
			return "", "", err
		}
		nif, _ := c.Data["nif"].(string)
		tipo, err := tpv.TipoFactura(cliente, nif, total)
		return cliente, tipo, err
	}
	tipo, err := tpv.TipoFactura("", "", total)
	if err != nil {
		return "", "", err
	}
	if c, hay, err := a.buscarPor(ctx, tx, "contacto", "externo_id", tpv.ExternoPublico); err != nil || hay {
		return c.ID, tipo, err
	}
	c, err := a.insertarValidado(ctx, tx, a.modelo("contacto"), map[string]any{"nombre": "TPV", "tipo": "organizacion", "externo_id": tpv.ExternoPublico})
	return c.ID, tipo, err
}

// CobrarTPV registra un cobro ya hecho: factura con sus líneas, su asiento, el del cobro
// (caja o bancos), la factura pagada y la sesión acumulada. Un reintento con la misma
// operación y la misma cesta devuelve la venta ya registrada.
func (a *Almacen) CobrarTPV(ctx context.Context, c tpv.Cobro) (VentaTPV, error) {
	var out VentaTPV
	if err := c.Validar(); err != nil {
		return out, err
	}
	huella, err := c.Huella()
	if err != nil {
		return out, err
	}
	// Fuera de la transacción: la base tiene una sola conexión.
	org, err := a.Organizacion(ctx)
	if errors.Is(err, ErrNoEncontrado) {
		err = fmt.Errorf("configura antes el perfil de la organización: %w", dominio.ErrDatos)
	}
	if err != nil {
		return out, err
	}
	err = a.tx(ctx, func(tx *sql.Tx) error {
		var err error
		out, err = a.cobrarTPVEn(ctx, tx, c, org, huella)
		return err
	})
	return out, err
}

// cobrarTPVEn es CobrarTPV dentro de su transacción.
func (a *Almacen) cobrarTPVEn(ctx context.Context, tx *sql.Tx, c tpv.Cobro, org api.ERPOrganizacion, huella string) (VentaTPV, error) {
	previa, hay, err := a.porOperacion(ctx, tx, "factura_venta", c.Operacion, huella)
	if err != nil {
		return VentaTPV{}, err
	}
	if hay {
		total, err := dominio.Centimos(previa.Data, "total")
		return VentaTPV{Factura: previa, Cambio: tpv.Cambio(c, total)}, err
	}
	if err := a.exigirModulos(ctx, tx, "tpv", "productos", "contactos", "facturacion"); err != nil {
		return VentaTPV{}, err
	}
	if err := tpv.ExigirTerritorio(org.Pais, org.CP); err != nil {
		return VentaTPV{}, err
	}
	sesion, err := obtener(ctx, tx, api.ERPModeloSesionTPV, c.Sesion)
	if err != nil {
		return VentaTPV{}, err
	}
	if sesion.Estado != "abierto" {
		return VentaTPV{}, fmt.Errorf("la caja está cerrada: %w", dominio.ErrDatos)
	}
	lineas, err := a.lineasTPV(ctx, tx, c.Lineas)
	if err != nil {
		return VentaTPV{}, err
	}
	base, iva, total := tpv.Totales(lineas)
	if err := tpv.ValidarImporte(c, total); err != nil {
		return VentaTPV{}, err
	}
	cliente, tipo, err := a.clienteTPV(ctx, tx, c.Cliente, total)
	if err != nil {
		return VentaTPV{}, err
	}
	zona, err := time.LoadLocation("Europe/Madrid")
	if err != nil {
		zona = time.Local
	}
	ahora := a.ahora().In(zona)
	mf := a.modelo("factura_venta")
	factura, err := a.insertarValidado(ctx, tx, mf, tpv.DatosFactura(c, cliente, tipo, ahora.Format(time.DateOnly), huella, base, iva))
	if err != nil {
		return VentaTPV{}, err
	}
	for _, l := range lineas {
		if _, err := a.insertarValidado(ctx, tx, a.modelo("linea_factura"), tpv.DatosLinea(factura.ID, l)); err != nil {
			return VentaTPV{}, err
		}
	}
	if _, err := a.asentarEn(ctx, tx, mf, factura.ID); err != nil {
		return VentaTPV{}, err
	}
	cuenta, diario := tpv.Caja(c.Medio)
	fecha := time.Date(ahora.Year(), ahora.Month(), ahora.Day(), 0, 0, 0, 0, time.UTC)
	if _, err := a.escribirAsiento(ctx, tx, diario, fecha, "TPV "+factura.Ref, dominio.MonedaPorDefecto, "",
		[]Apunte{{Cuenta: cuenta, Debe: total}, {Cuenta: "430000", Haber: total, Tercero: cliente}}); err != nil {
		return VentaTPV{}, err
	}
	if err := a.ponerEstado(ctx, tx, "factura_venta", factura.ID, "pagado"); err != nil {
		return VentaTPV{}, err
	}
	if err := a.acumularEnSesion(ctx, tx, sesion, c.Medio, total); err != nil {
		return VentaTPV{}, err
	}
	if factura, err = obtener(ctx, tx, "factura_venta", factura.ID); err != nil {
		return VentaTPV{}, err
	}
	return VentaTPV{Factura: factura, Cambio: tpv.Cambio(c, total)}, nil
}

// acumularEnSesion suma el cobro a la sesión de caja: a lo cobrado con su medio y al
// total.
func (a *Almacen) acumularEnSesion(ctx context.Context, tx *sql.Tx, sesion api.ERPRegistro, medio string, total int64) error {
	for _, campo := range []string{medio, "total"} {
		previo, err := dominio.Centimos(sesion.Data, campo)
		if err != nil {
			return err
		}
		sesion.Data[campo] = previo + total
	}
	return a.guardarDatos(ctx, tx, sesion.Modelo, sesion.ID, sesion.Estado, sesion.Data)
}

// PresupuestoTPV guarda la cesta como presupuesto con sus líneas, sin cobrar ni
// emitir. La misma operación con la misma cesta devuelve el mismo presupuesto.
func (a *Almacen) PresupuestoTPV(ctx context.Context, c tpv.Cobro) (api.ERPRegistro, error) {
	var out api.ERPRegistro
	c, err := c.ParaPresupuesto()
	if err != nil {
		return out, err
	}
	huella, err := c.Huella()
	if err != nil {
		return out, err
	}
	err = a.tx(ctx, func(tx *sql.Tx) error {
		previo, hay, err := a.porOperacion(ctx, tx, "presupuesto", c.Operacion, huella)
		if err != nil || hay {
			out = previo
			return err
		}
		if err := a.exigirModulos(ctx, tx, "tpv", "ventas", "productos", "contactos"); err != nil {
			return err
		}
		lineas, err := a.lineasTPV(ctx, tx, c.Lineas)
		if err != nil {
			return err
		}
		base, iva, total := tpv.Totales(lineas)
		if total <= 0 || total > 1000000000 {
			return fmt.Errorf("el importe del presupuesto no es válido: %w", dominio.ErrDatos)
		}
		out, err = a.insertarValidado(ctx, tx, a.modelo("presupuesto"), map[string]any{
			"cliente": c.Cliente, "fecha": a.ahora().Format(time.DateOnly), "base": base, "iva": iva, "total": total,
			"tpv_operacion": c.Operacion, "tpv_huella": huella,
		})
		if err != nil {
			return err
		}
		for i, l := range lineas {
			if _, err := a.insertarValidado(ctx, tx, a.modelo("linea_presupuesto"), map[string]any{
				"presupuesto": out.ID, "producto": c.Lineas[i].Producto, "nombre": l.Descripcion,
				"cantidad": l.Cantidad, "precio_con_iva": l.Precio, "tipo_iva": fmt.Sprint(l.Tipo),
			}); err != nil {
				return err
			}
		}
		return nil
	})
	return out, err
}
