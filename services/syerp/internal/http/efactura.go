// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package syerphttp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"git.sytue.eu/sytue/one/apps/nucleo/erp/catalogo"
	domain "git.sytue.eu/sytue/one/apps/nucleo/erp/dominio"
	"git.sytue.eu/sytue/one/apps/nucleo/erp/efactura"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/syerp/internal/store"
)

// Facturas electrónicas RECIBIDAS. Es la otra mitad de la salida UBL: lo que
// emiten SAP, Dynamics, Sage, Odoo, Holded, a3 o cualquier plataforma Peppol
// entra aquí como factura de compra, sin un conector por origen.
//
// Se leen las tres sintaxis que circulan en Europa:
//   - UBL 2.1 (Invoice y CreditNote): Peppol BIS 3, XRechnung UBL, la de FACe B2B;
//   - UN/CEFACT CII: Factur-X, ZUGFeRD y XRechnung CII (el XML, no el PDF);
//   - Facturae 3.2.x: la española, con varias facturas por fichero.
//
// Solo se toma la cabecera: emisor, receptor, fechas y totales. Las facturas de
// este ERP no llevan líneas (base, IVA y total), así que leerlas no aportaría
// nada que se pudiera guardar.
//
// ponytail: la retención (IRPF) no se guarda; el total es base + impuestos, que
// es lo que cuadra con el asiento. Si hace falta, un campo `retencion` en el
// catálogo y leer TaxesWithheld / AllowanceTotalAmount.

var errFacturaIlegible = httpx.NewError(http.StatusUnprocessableEntity, "factura_ilegible",
	"The file is not a UBL, CII or Facturae electronic invoice.")

// Las facturas recibidas se leen con apps/nucleo/erp/efactura, el mismo lector del
// ERP de Sytue Office.
type (
	facturaRecibida = efactura.Recibida
	parteFactura    = efactura.ParteRecibida
)

func leerFacturas(cuerpo []byte) ([]facturaRecibida, error) { return efactura.Leer(cuerpo) }

// importarFacturas — la rama XML de POST /{modelo}/importar. Devuelve el mismo
// resultado que el CSV, con una «fila» por factura del fichero.
func (h *Handlers) importarFacturas(w http.ResponseWriter, r *http.Request, m *catalogo.Modelo, cuerpo []byte) {
	if !m.RecibeFacturas() {
		httpx.Fail(w, r, errFacturaIlegible.WithMessage("Electronic invoices can only be imported as received (purchase) invoices."))
		return
	}
	facturas, err := leerFacturas(cuerpo)
	if err != nil {
		httpx.Fail(w, r, errFacturaIlegible.WithMessage(err.Error()))
		return
	}
	ctx := r.Context()
	org, err := h.store.Organizacion(ctx)
	hayOrg := err == nil
	if err != nil && !errors.Is(err, store.ErrNoEncontrado) {
		httpx.Fail(w, r, err)
		return
	}
	contacto, ok := h.cat.Modelo("contacto")
	if !ok {
		httpx.Fail(w, r, errModeloDesconocido)
		return
	}

	res := struct {
		Filas        int           `json:"filas"`
		Creados      int           `json:"creados"`
		Actualizados int           `json:"actualizados"`
		Ignoradas    []string      `json:"ignoradas"`
		Fallos       []falloDeFila `json:"fallos"`
	}{Ignoradas: []string{}, Fallos: []falloDeFila{}}

	for i, f := range facturas {
		n := i + 1
		res.Filas++
		if res.Filas > maxImportFilas {
			res.Fallos = append(res.Fallos, falloDeFila{n, fmt.Sprintf("tope de %d facturas por importación", maxImportFilas)})
			break
		}
		nifOrg := ""
		if hayOrg {
			nifOrg = org.NIF
		}
		if err := f.Comprobar(nifOrg); err != nil {
			res.Fallos = append(res.Fallos, falloDeFila{n, f.Numero + ": " + err.Error()})
			continue
		}
		tercero, err := h.terceroDe(ctx, contacto, f.Emisor, sub(r))
		if err != nil {
			res.Fallos = append(res.Fallos, falloDeFila{n, f.Numero + ": " + err.Error()})
			continue
		}
		// El número lo pone el proveedor y solo es único por proveedor: con su NIF
		// delante, importar dos veces el mismo fichero actualiza en vez de duplicar.
		externo := f.ClaveExterna()
		datos := map[string]any{
			m.Contable.CampoTercero: tercero, "fecha": f.Fecha, "base": f.Base, "iva": f.IVA,
			"total": f.Total, "externo_id": externo,
		}
		if f.Vencimiento != "" {
			datos["vencimiento"] = f.Vencimiento
		}
		if f.Nota != "" {
			datos["notas"] = f.Nota
		}
		crudo, err := json.Marshal(datos)
		if err != nil {
			httpx.Fail(w, r, err)
			return
		}
		data, etiqueta, err := domain.Validar(m, crudo)
		if err != nil {
			res.Fallos = append(res.Fallos, falloDeFila{n, f.Numero + ": " + err.Error()})
			continue
		}
		creado, err := h.store.UpsertPorExterno(ctx, m, externo, data, etiqueta, "", sub(r))
		if err != nil {
			res.Fallos = append(res.Fallos, falloDeFila{n, f.Numero + ": " + err.Error()})
			continue
		}
		if creado {
			res.Creados++
		} else {
			res.Actualizados++
		}
	}
	httpx.JSON(w, r, http.StatusOK, res)
}

// terceroDe encuentra al emisor entre los contactos —por NIF y, si la factura no
// lo trae, por nombre— o lo da de alta con lo que dice la factura.
func (h *Handlers) terceroDe(ctx context.Context, contacto *catalogo.Modelo, p parteFactura, sub string) (string, error) {
	var id string
	var err error
	if p.NIF != "" {
		id, err = h.store.BuscarPorNIF(ctx, contacto.Clave, p.NIF)
	} else {
		id, err = h.store.BuscarPorEtiqueta(ctx, contacto.Clave, p.Nombre)
	}
	if !errors.Is(err, store.ErrNoEncontrado) {
		return id, err
	}
	datos := p.DatosContacto()
	crudo, err := json.Marshal(datos)
	if err != nil {
		return "", err
	}
	data, etiqueta, err := domain.Validar(contacto, crudo)
	if err != nil {
		return "", fmt.Errorf("emisor %s: %w", p.Nombre, err)
	}
	reg, err := h.store.Crear(ctx, contacto, data, etiqueta, sub)
	return reg.ID, err
}

// esXML distingue una factura electrónica de un CSV por su primer carácter.
func esXML(cuerpo []byte) bool { return efactura.EsXML(cuerpo) }
