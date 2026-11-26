// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package erp

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/erp/efactura"
)

func leerTodo(t *testing.T, rc io.ReadCloser, err error) string {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
	defer rc.Close()
	b, err := io.ReadAll(rc)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// La factura que sale: Facturae, UBL, el impreso y el PDF, con la generación de
// apps/nucleo/erp/efactura; y la que entra, con su lector.
func TestFacturasElectronicasEImpreso(t *testing.T) {
	ctx := context.Background()
	e, _ := abrir(t, t.TempDir())
	if _, err := e.GuardarOrganizacion(ctx, api.ERPOrganizacion{
		Nombre: "Acme Ibérica SL", NIF: "B12345678", Direccion: "Gran Vía 1",
		CP: "28013", Ciudad: "Madrid", Pais: "ES", Forma: "sl",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.CambiarModulo(ctx, "facturacion", true); err != nil {
		t.Fatal(err)
	}
	cli, err := e.Crear(ctx, "contacto", map[string]any{
		"nombre": "Client Français SARL", "tipo": "organizacion", "nif": "12345678901",
		"direccion": "1 rue de Rivoli", "cp": "75001", "ciudad": "Paris", "pais": "FR",
	})
	if err != nil {
		t.Fatal(err)
	}
	fac, err := e.Crear(ctx, "factura_venta", map[string]any{"cliente": cli.ID, "fecha": "2026-05-04", "base": 123456, "iva": 25926, "total": 149382})
	if err != nil {
		t.Fatal(err)
	}

	cuerpo, nombre, err := e.Facturae(ctx, "factura_venta", fac.ID)
	x := leerTodo(t, cuerpo, err)
	if nombre != "facturae-FV-2026-00001.xml" {
		t.Errorf("nombre: %q", nombre)
	}
	for _, quiero := range []string{
		`<TaxIdentificationNumber>B12345678</TaxIdentificationNumber>`, `<Province>Madrid</Province>`,
		`<TaxIdentificationNumber>FR12345678901</TaxIdentificationNumber>`, `<InvoiceTotal>1493.82</InvoiceTotal>`,
	} {
		if !strings.Contains(x, quiero) {
			t.Errorf("Facturae: falta %s", quiero)
		}
	}
	if strings.Contains(x, "VERI*FACTU") {
		t.Error("sin VERI*FACTU no hay QR tributario")
	}
	// Lo que sale vuelve a leerse igual.
	vuelta, err := efactura.Leer([]byte(x))
	if err != nil || len(vuelta) != 1 || vuelta[0].Base != 123456 || vuelta[0].Emisor.NIF != "B12345678" {
		t.Fatalf("releer: %v %+v", err, vuelta)
	}

	cuerpo, nombre, err = e.UBL(ctx, "factura_venta", fac.ID)
	if u := leerTodo(t, cuerpo, err); !strings.Contains(u, "<cbc:ID>FV-2026-00001</cbc:ID>") || nombre != "FV-2026-00001.xml" {
		t.Errorf("UBL: %q\n%s", nombre, u)
	}

	cuerpo, err = e.Imprimir(ctx, "factura_venta", fac.ID)
	h := leerTodo(t, cuerpo, err)
	for _, quiero := range []string{"Acme Ibérica SL", "Client Français SARL", "1.234,56 €", "1.493,82 €", "04/05/2026"} {
		if !strings.Contains(h, quiero) {
			t.Errorf("impreso: falta %q", quiero)
		}
	}

	if _, _, err := e.Facturae(ctx, "factura_compra", fac.ID); estado(err) != http.StatusBadRequest {
		t.Errorf("una recibida no sale como Facturae: %v", err)
	}
}

// El PDF, por la ruta que api.ERP no tiene como método.
func TestPDFDeFactura(t *testing.T) {
	ctx := context.Background()
	cat, obl, err := Cargar()
	if err != nil {
		t.Fatal(err)
	}
	alm, err := Abrir(t.TempDir(), cat)
	if err != nil {
		t.Fatal(err)
	}
	s := NuevoServidor(cat, obl, alm)
	e := api.NewERP(Base, nil, s.Cliente(), "es")
	if _, err := e.GuardarOrganizacion(ctx, api.ERPOrganizacion{Nombre: "Acme", NIF: "B12345678", Pais: "ES", Forma: "sl"}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.CambiarModulo(ctx, "facturacion", true); err != nil {
		t.Fatal(err)
	}
	cli, err := e.Crear(ctx, "contacto", map[string]any{"nombre": "Cliente", "tipo": "persona"})
	if err != nil {
		t.Fatal(err)
	}
	fac, err := e.Crear(ctx, "factura_venta", map[string]any{"cliente": cli.ID, "fecha": "2026-05-04", "base": 1000, "iva": 210, "total": 1210})
	if err != nil {
		t.Fatal(err)
	}
	resp, err := s.Cliente().Get(Base + "/api/v1/factura_venta/" + fac.ID + "/pdf")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK || !bytes.HasPrefix(b, []byte("%PDF-")) || !strings.Contains(resp.Header.Get("Content-Disposition"), "FV-2026-00001.pdf") {
		t.Fatalf("PDF: %d %q %q", resp.StatusCode, resp.Header.Get("Content-Disposition"), b[:min(len(b), 80)])
	}
}

const ublDeProveedor = `<?xml version="1.0" encoding="UTF-8"?>
<Invoice xmlns="urn:oasis:names:specification:ubl:schema:xsd:Invoice-2"
  xmlns:cac="urn:oasis:names:specification:ubl:schema:xsd:CommonAggregateComponents-2"
  xmlns:cbc="urn:oasis:names:specification:ubl:schema:xsd:CommonBasicComponents-2">
  <cbc:ID>F-2026-0042</cbc:ID>
  <cbc:IssueDate>2026-03-02</cbc:IssueDate>
  <cbc:DocumentCurrencyCode>EUR</cbc:DocumentCurrencyCode>
  <cac:AccountingSupplierParty><cac:Party>
    <cac:PartyName><cbc:Name>Levante</cbc:Name></cac:PartyName>
    <cac:PostalAddress><cbc:StreetName>Calle Colón 5</cbc:StreetName><cbc:CityName>Valencia</cbc:CityName>
      <cbc:PostalZone>46004</cbc:PostalZone><cac:Country><cbc:IdentificationCode>ES</cbc:IdentificationCode></cac:Country></cac:PostalAddress>
    <cac:PartyTaxScheme><cbc:CompanyID>ESB98765432</cbc:CompanyID><cac:TaxScheme><cbc:ID>VAT</cbc:ID></cac:TaxScheme></cac:PartyTaxScheme>
    <cac:PartyLegalEntity><cbc:RegistrationName>Proveedora Levante SL</cbc:RegistrationName></cac:PartyLegalEntity>
  </cac:Party></cac:AccountingSupplierParty>
  <cac:AccountingCustomerParty><cac:Party>
    <cac:PartyName><cbc:Name>Acme</cbc:Name></cac:PartyName>
    <cac:PartyTaxScheme><cbc:CompanyID>ESB12345678</cbc:CompanyID><cac:TaxScheme><cbc:ID>VAT</cbc:ID></cac:TaxScheme></cac:PartyTaxScheme>
  </cac:Party></cac:AccountingCustomerParty>
  <cac:TaxTotal><cbc:TaxAmount currencyID="EUR">21.00</cbc:TaxAmount></cac:TaxTotal>
  <cac:LegalMonetaryTotal><cbc:TaxExclusiveAmount currencyID="EUR">100.00</cbc:TaxExclusiveAmount></cac:LegalMonetaryTotal>
</Invoice>`

func TestLeerEImportarFacturaRecibida(t *testing.T) {
	ctx := context.Background()
	e, _ := abrir(t, t.TempDir())
	if _, err := e.GuardarOrganizacion(ctx, api.ERPOrganizacion{Nombre: "Acme", NIF: "B12345678", Pais: "ES", Forma: "sl"}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.CambiarModulo(ctx, "compras", true); err != nil {
		t.Fatal(err)
	}
	// El asistente prepara la ficha sin crear nada: el proveedor, como contacto propuesto.
	rel, err := e.LeerFacturaXML(ctx, []byte(ublDeProveedor))
	if err != nil {
		t.Fatal(err)
	}
	if rel.NumeroOrigen != "F-2026-0042" || rel.Contacto["nombre"] != "Proveedora Levante SL" || rel.Data["externo_id"] != "B98765432/F-2026-0042" {
		t.Fatalf("asistente: %+v", rel)
	}
	if pag, err := e.ListarCon(ctx, "contacto", api.ERPFiltro{}); err != nil || len(pag.Registros) != 0 {
		t.Fatalf("leer no crea contactos: %v %d", err, len(pag.Registros))
	}
	// Importar el mismo XML dos veces: crea y luego actualiza; el proveedor, una vez.
	res, err := e.Importar(ctx, "factura_compra", []byte(ublDeProveedor), true)
	if err != nil || res.Creados != 1 || len(res.Fallos) != 0 {
		t.Fatalf("importar: %v %+v", err, res)
	}
	res, err = e.Importar(ctx, "factura_compra", []byte(ublDeProveedor), true)
	if err != nil || res.Actualizados != 1 {
		t.Fatalf("reimportar: %v %+v", err, res)
	}
	if pag, err := e.ListarCon(ctx, "contacto", api.ERPFiltro{}); err != nil || len(pag.Registros) != 1 {
		t.Fatalf("un solo proveedor: %v %d", err, len(pag.Registros))
	}
	// Y ahora el asistente la reconoce como ya entrada.
	if rel, err := e.LeerFacturaXML(ctx, []byte(ublDeProveedor)); err != nil || rel.Existente == nil || rel.Contacto != nil {
		t.Fatalf("ya entrada: %v %+v", err, rel)
	}
	// A nombre de otra organización, no.
	otra := strings.Replace(ublDeProveedor, "ESB12345678", "ESB11111111", 1)
	if _, err := e.LeerFacturaXML(ctx, []byte(otra)); estado(err) != http.StatusUnprocessableEntity {
		t.Fatalf("a nombre de otra: %v", err)
	}
}
