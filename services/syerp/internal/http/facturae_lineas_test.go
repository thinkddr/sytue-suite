// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package syerphttp

import (
	"encoding/xml"
	"strings"
	"testing"

	domain "git.sytue.eu/sytue/one/apps/nucleo/erp/dominio"
	"git.sytue.eu/sytue/one/services/syerp/internal/store"
)

func TestFacturaeIVARealDIR3YPagada(t *testing.T) {
	org := store.Organizacion{Nombre: "Acme SL", NIF: "B12345678", Direccion: "Gran Vía 1", CP: "28013", Ciudad: "Madrid", Pais: "ES"}
	cliente := store.Registro{ID: "cliente", Etiqueta: "Ayuntamiento", Data: map[string]any{"nif": "P2807900B", "direccion": "Plaza 1", "cp": "28001", "ciudad": "Madrid", "pais": "ES", "tipo": "organizacion", "dir3_contable": "L01280796", "dir3_gestor": "L01280796", "dir3_tramitadora": "LA0000001"}}
	reg := store.Registro{Ref: "FV-1", Estado: "pagado", Data: map[string]any{"fecha": "2026-10-03", "base": int64(20000), "iva": int64(3100)}}
	lineas := []domain.LineaFactura{{Descripcion: "Servicio", Cantidad: "1", Precio: 12100, Tipo: 21, Importe: 12100}, {Descripcion: "Producto", Cantidad: "2", Precio: 5500, Tipo: 10, Importe: 11000}}
	doc, err := facturaeDe(org, reg, cliente, lineas...)
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Factura.Impuestos) != 2 || len(doc.Factura.Lineas) != 2 || doc.Factura.Pendiente != "0.00" {
		t.Fatalf("detalle=%+v", doc.Factura)
	}
	cuerpo, err := xml.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{"<TaxRate>21.00</TaxRate>", "<TaxRate>10.00</TaxRate>", "<TotalOutstandingAmount><TotalAmount>0.00</TotalAmount>", "<CentreCode>LA0000001</CentreCode><RoleTypeCode>03</RoleTypeCode>"} {
		if !strings.Contains(string(cuerpo), s) {
			t.Errorf("falta %s", s)
		}
	}
	if strings.Contains(string(cuerpo), "<AdministrativeCentres></AdministrativeCentres>") {
		t.Fatal("centros vacíos del emisor invalidan el XSD")
	}
	delete(cliente.Data, "dir3_gestor")
	if _, err := facturaeDe(org, reg, cliente, lineas...); err == nil {
		t.Fatal("DIR3 parcial admitido")
	}
}
