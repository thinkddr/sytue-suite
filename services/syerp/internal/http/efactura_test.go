// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package syerphttp

import (
	"encoding/xml"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/services/syerp/internal/store"
)

const ublRecibida = `<?xml version="1.0" encoding="UTF-8"?>
<Invoice xmlns="urn:oasis:names:specification:ubl:schema:xsd:Invoice-2"
  xmlns:cac="urn:oasis:names:specification:ubl:schema:xsd:CommonAggregateComponents-2"
  xmlns:cbc="urn:oasis:names:specification:ubl:schema:xsd:CommonBasicComponents-2">
  <cbc:ID>F-2026-0042</cbc:ID>
  <cbc:IssueDate>2026-03-02</cbc:IssueDate>
  <cbc:DueDate>2026-04-01</cbc:DueDate>
  <cbc:Note>Servicios de marzo</cbc:Note>
  <cbc:DocumentCurrencyCode>EUR</cbc:DocumentCurrencyCode>
  <cac:AccountingSupplierParty><cac:Party>
    <cac:PartyName><cbc:Name>Levante</cbc:Name></cac:PartyName>
    <cac:PostalAddress><cbc:StreetName>Calle Colón 5</cbc:StreetName><cbc:CityName>Valencia</cbc:CityName>
      <cbc:PostalZone>46004</cbc:PostalZone><cac:Country><cbc:IdentificationCode>ES</cbc:IdentificationCode></cac:Country></cac:PostalAddress>
    <cac:PartyTaxScheme><cbc:CompanyID>ESB98765432</cbc:CompanyID><cac:TaxScheme><cbc:ID>VAT</cbc:ID></cac:TaxScheme></cac:PartyTaxScheme>
    <cac:PartyLegalEntity><cbc:RegistrationName>Proveedora Levante SL</cbc:RegistrationName></cac:PartyLegalEntity>
    <cac:Contact><cbc:ElectronicMail>facturas@levante.es</cbc:ElectronicMail></cac:Contact>
  </cac:Party></cac:AccountingSupplierParty>
  <cac:AccountingCustomerParty><cac:Party>
    <cac:PartyTaxScheme><cbc:CompanyID>ESB12345678</cbc:CompanyID><cac:TaxScheme><cbc:ID>VAT</cbc:ID></cac:TaxScheme></cac:PartyTaxScheme>
    <cac:PartyLegalEntity><cbc:RegistrationName>Acme Ibérica SL</cbc:RegistrationName></cac:PartyLegalEntity>
  </cac:Party></cac:AccountingCustomerParty>
  <cac:TaxTotal><cbc:TaxAmount currencyID="EUR">259.26</cbc:TaxAmount>
    <cac:TaxSubtotal><cbc:TaxableAmount currencyID="EUR">1234.56</cbc:TaxableAmount><cbc:TaxAmount currencyID="EUR">259.26</cbc:TaxAmount></cac:TaxSubtotal>
  </cac:TaxTotal>
  <cac:LegalMonetaryTotal>
    <cbc:TaxExclusiveAmount currencyID="EUR">1234.56</cbc:TaxExclusiveAmount>
    <cbc:TaxInclusiveAmount currencyID="EUR">1493.82</cbc:TaxInclusiveAmount>
    <cbc:PayableAmount currencyID="EUR">1493.82</cbc:PayableAmount>
  </cac:LegalMonetaryTotal>
</Invoice>`

// Las tres sintaxis tienen que dar la misma factura: número, fechas, importes
// en céntimos exactos, y emisor y receptor con su NIF.
func TestLeerFacturasElectronicas(t *testing.T) {
	abono := `<CreditNote xmlns="urn:oasis:names:specification:ubl:schema:xsd:CreditNote-2"
	  xmlns:cac="urn:oasis:names:specification:ubl:schema:xsd:CommonAggregateComponents-2"
	  xmlns:cbc="urn:oasis:names:specification:ubl:schema:xsd:CommonBasicComponents-2">
	  <cbc:ID>AB-7</cbc:ID><cbc:IssueDate>2026-03-09</cbc:IssueDate><cbc:DocumentCurrencyCode>EUR</cbc:DocumentCurrencyCode>
	  <cac:TaxTotal><cbc:TaxAmount currencyID="EUR">21.00</cbc:TaxAmount></cac:TaxTotal>
	  <cac:LegalMonetaryTotal><cbc:TaxExclusiveAmount currencyID="EUR">100.00</cbc:TaxExclusiveAmount></cac:LegalMonetaryTotal>
	</CreditNote>`
	cii := `<rsm:CrossIndustryInvoice xmlns:rsm="urn:un:unece:uncefact:data:standard:CrossIndustryInvoice:100"
	  xmlns:ram="urn:un:unece:uncefact:data:standard:ReusableAggregateBusinessInformationEntity:100"
	  xmlns:udt="urn:un:unece:uncefact:data:standard:UnqualifiedDataType:100">
	  <rsm:ExchangedDocument><ram:ID>RE-1001</ram:ID><ram:TypeCode>380</ram:TypeCode>
	    <ram:IssueDateTime><udt:DateTimeString format="102">20260315</udt:DateTimeString></ram:IssueDateTime></rsm:ExchangedDocument>
	  <rsm:SupplyChainTradeTransaction>
	    <ram:ApplicableHeaderTradeAgreement>
	      <ram:SellerTradeParty><ram:Name>Muster GmbH</ram:Name>
	        <ram:PostalTradeAddress><ram:PostcodeCode>10115</ram:PostcodeCode><ram:LineOne>Hauptstraße 1</ram:LineOne>
	          <ram:CityName>Berlin</ram:CityName><ram:CountryID>DE</ram:CountryID></ram:PostalTradeAddress>
	        <ram:SpecifiedTaxRegistration><ram:ID schemeID="VA">DE123456789</ram:ID></ram:SpecifiedTaxRegistration></ram:SellerTradeParty>
	      <ram:BuyerTradeParty><ram:Name>Acme Ibérica SL</ram:Name></ram:BuyerTradeParty>
	    </ram:ApplicableHeaderTradeAgreement>
	    <ram:ApplicableHeaderTradeSettlement>
	      <ram:InvoiceCurrencyCode>EUR</ram:InvoiceCurrencyCode>
	      <ram:SpecifiedTradePaymentTerms><ram:DueDateDateTime><udt:DateTimeString format="102">20260414</udt:DateTimeString></ram:DueDateDateTime></ram:SpecifiedTradePaymentTerms>
	      <ram:SpecifiedTradeSettlementHeaderMonetarySummation>
	        <ram:TaxBasisTotalAmount>500.00</ram:TaxBasisTotalAmount>
	        <ram:TaxTotalAmount currencyID="EUR">95.00</ram:TaxTotalAmount>
	        <ram:GrandTotalAmount>595.00</ram:GrandTotalAmount>
	      </ram:SpecifiedTradeSettlementHeaderMonetarySummation>
	    </ram:ApplicableHeaderTradeSettlement>
	  </rsm:SupplyChainTradeTransaction>
	</rsm:CrossIndustryInvoice>`
	// Facturae en ISO-8859-1, como la guardan muchos programas españoles, con
	// un autónomo de emisor y dos facturas en el mismo lote.
	facturae := latin1(`<?xml version="1.0" encoding="ISO-8859-1"?>
	<fe:Facturae xmlns:fe="http://www.facturae.gob.es/formato/Versiones/Facturaev3_2_2.xml">
	  <Parties>
	    <SellerParty><TaxIdentification><PersonTypeCode>F</PersonTypeCode><ResidenceTypeCode>R</ResidenceTypeCode>
	      <TaxIdentificationNumber>12345678Z</TaxIdentificationNumber></TaxIdentification>
	      <Individual><Name>Ana</Name><FirstSurname>Muñoz</FirstSurname><SecondSurname>Peña</SecondSurname>
	        <AddressInSpain><Address>Plaza Mayor 3</Address><PostCode>37002</PostCode><Town>Salamanca</Town>
	          <Province>Salamanca</Province><CountryCode>ESP</CountryCode></AddressInSpain></Individual></SellerParty>
	    <BuyerParty><TaxIdentification><PersonTypeCode>J</PersonTypeCode><ResidenceTypeCode>R</ResidenceTypeCode>
	      <TaxIdentificationNumber>B12345678</TaxIdentificationNumber></TaxIdentification>
	      <LegalEntity><CorporateName>Acme Ibérica SL</CorporateName></LegalEntity></BuyerParty>
	  </Parties>
	  <Invoices>
	    <Invoice><InvoiceHeader><InvoiceNumber>0007</InvoiceNumber><InvoiceSeriesCode>A</InvoiceSeriesCode></InvoiceHeader>
	      <InvoiceIssueData><IssueDate>2026-02-01</IssueDate><InvoiceCurrencyCode>EUR</InvoiceCurrencyCode></InvoiceIssueData>
	      <InvoiceTotals><TotalGrossAmountBeforeTaxes>300.00</TotalGrossAmountBeforeTaxes><TotalTaxOutputs>63.00</TotalTaxOutputs></InvoiceTotals></Invoice>
	    <Invoice><InvoiceHeader><InvoiceNumber>0008</InvoiceNumber><InvoiceSeriesCode>A</InvoiceSeriesCode></InvoiceHeader>
	      <InvoiceIssueData><IssueDate>2026-02-15</IssueDate><InvoiceCurrencyCode>EUR</InvoiceCurrencyCode></InvoiceIssueData>
	      <InvoiceTotals><TotalGrossAmountBeforeTaxes>-50.00</TotalGrossAmountBeforeTaxes><TotalTaxOutputs>-10.50</TotalTaxOutputs></InvoiceTotals></Invoice>
	  </Invoices>
	</fe:Facturae>`)

	casos := []struct {
		nombre string
		xml    []byte
		quiero []facturaRecibida
	}{
		{"UBL", []byte(ublRecibida), []facturaRecibida{{
			Numero: "F-2026-0042", Fecha: "2026-03-02", Vencimiento: "2026-04-01", Moneda: "EUR", Nota: "Servicios de marzo",
			Base: 123456, IVA: 25926, Total: 149382,
			Emisor: parteFactura{
				Nombre: "Proveedora Levante SL", NIF: "ESB98765432", Direccion: "Calle Colón 5", Ciudad: "Valencia",
				CP: "46004", Pais: "ES", Correo: "facturas@levante.es",
			},
			Receptor: parteFactura{Nombre: "Acme Ibérica SL", NIF: "ESB12345678"},
		}}},
		{"UBL abono", []byte(abono), []facturaRecibida{{
			Numero: "AB-7", Fecha: "2026-03-09", Moneda: "EUR", Base: -10000, IVA: -2100, Total: -12100,
		}}},
		{"CII", []byte(cii), []facturaRecibida{{
			Numero: "RE-1001", Fecha: "2026-03-15", Vencimiento: "2026-04-14", Moneda: "EUR",
			Base: 50000, IVA: 9500, Total: 59500,
			Emisor: parteFactura{
				Nombre: "Muster GmbH", NIF: "DE123456789", Direccion: "Hauptstraße 1", Ciudad: "Berlin", CP: "10115", Pais: "DE",
			},
			Receptor: parteFactura{Nombre: "Acme Ibérica SL"},
		}}},
		{"Facturae", facturae, []facturaRecibida{
			{
				Numero: "A/0007", Fecha: "2026-02-01", Moneda: "EUR", Base: 30000, IVA: 6300, Total: 36300,
				Emisor: parteFactura{
					Nombre: "Ana Muñoz Peña", NIF: "12345678Z", Direccion: "Plaza Mayor 3", Ciudad: "Salamanca",
					CP: "37002", Pais: "ES", Persona: true,
				},
				Receptor: parteFactura{Nombre: "Acme Ibérica SL", NIF: "B12345678"},
			},
			{
				Numero: "A/0008", Fecha: "2026-02-15", Moneda: "EUR", Base: -5000, IVA: -1050, Total: -6050,
				Emisor: parteFactura{
					Nombre: "Ana Muñoz Peña", NIF: "12345678Z", Direccion: "Plaza Mayor 3", Ciudad: "Salamanca",
					CP: "37002", Pais: "ES", Persona: true,
				},
				Receptor: parteFactura{Nombre: "Acme Ibérica SL", NIF: "B12345678"},
			},
		}},
	}
	for _, c := range casos {
		got, err := leerFacturas(c.xml)
		if err != nil {
			t.Fatalf("%s: %v", c.nombre, err)
		}
		if len(got) != len(c.quiero) {
			t.Fatalf("%s: %d facturas, quiero %d", c.nombre, len(got), len(c.quiero))
		}
		for i := range got {
			if got[i] != c.quiero[i] {
				t.Errorf("%s[%d]:\n got %+v\nwant %+v", c.nombre, i, got[i], c.quiero[i])
			}
		}
	}

	if _, err := leerFacturas([]byte(`<Order><ID>1</ID></Order>`)); err == nil {
		t.Error("un pedido no es una factura")
	}
}

// Lo que no se puede guardar sin mentir en el libro no entra: otra divisa, una
// factura a nombre de otro. El NIF casa aunque uno lleve prefijo y guiones.
func TestComprobarFacturaRecibida(t *testing.T) {
	facturas, err := leerFacturas([]byte(ublRecibida))
	if err != nil {
		t.Fatal(err)
	}
	f := facturas[0]
	org := store.Organizacion{Nombre: "Acme Ibérica SL", NIF: "B-12345678"}
	if err := f.comprobar(org, true); err != nil {
		t.Fatalf("la factura es de la organización: %v", err)
	}
	if err := f.comprobar(store.Organizacion{NIF: "B87654321"}, true); err == nil || !strings.Contains(err.Error(), "no de esta organización") {
		t.Errorf("a nombre de otro: %v", err)
	}
	enDolares := f
	enDolares.Moneda = "USD"
	if err := enDolares.comprobar(org, true); err == nil {
		t.Error("una factura en dólares no puede entrar como euros")
	}
}

// Lo que sale como Facturae tiene que volver a leerse igual: es la prueba de que
// cada elemento está donde lo busca quien lo recibe. La validación contra el XSD
// oficial se hace aparte, en el piloto.
func TestFacturaeIdaYVuelta(t *testing.T) {
	org := store.Organizacion{Nombre: "Acme Ibérica SL", NIF: "ESB12345678", Direccion: "Gran Vía 1", CP: "28013", Ciudad: "Madrid", Pais: "ES"}
	reg := store.Registro{ID: "r1", Ref: "FV-2026-00007", Data: map[string]any{"fecha": "2026-05-04", "base": float64(123456), "iva": float64(25926)}}
	cliente := store.Registro{ID: "c1", Etiqueta: "Client Français SARL", Data: map[string]any{
		"nif": "12345678901", "direccion": "1 rue de Rivoli", "cp": "75001", "ciudad": "Paris", "pais": "FR", "tipo": "organizacion",
	}}
	doc, err := facturaeDe(org, false, reg, "base", "iva", cliente)
	if err != nil {
		t.Fatal(err)
	}
	cuerpo, err := xml.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	x := string(cuerpo)
	for _, quiero := range []string{
		`<fe:Facturae xmlns:fe="http://www.facturae.gob.es/formato/Versiones/Facturaev3_2_2.xml"><FileHeader><SchemaVersion>3.2.2</SchemaVersion>`,
		// El emisor español, sin prefijo y con la provincia sacada del CP.
		`<TaxIdentificationNumber>B12345678</TaxIdentificationNumber>`, `<Province>Madrid</Province>`,
		// El cliente de la Unión: residencia U, NIF-IVA con prefijo y país en alfa-3.
		`<ResidenceTypeCode>U</ResidenceTypeCode><TaxIdentificationNumber>FR12345678901</TaxIdentificationNumber>`,
		`<PostCodeAndTown>75001 Paris</PostCodeAndTown>`, `<CountryCode>FRA</CountryCode>`,
		`<TaxRate>21.00</TaxRate>`, `<InvoiceTotal>1493.82</InvoiceTotal>`,
	} {
		if !strings.Contains(x, quiero) {
			t.Errorf("falta %s en:\n%s", quiero, x)
		}
	}
	vuelta, err := leerFacturas(cuerpo)
	if err != nil || len(vuelta) != 1 {
		t.Fatalf("releer: %v", err)
	}
	if v := vuelta[0]; v.Numero != "FV-2026-00007" || v.Fecha != "2026-05-04" || v.Base != 123456 || v.IVA != 25926 ||
		v.Emisor.NIF != "B12345678" || v.Receptor.Pais != "FR" || v.Receptor.Nombre != "Client Français SARL" {
		t.Errorf("vuelta = %+v", v)
	}

	// Lo que Facturae no admite se dice antes de descargar, con nombre y apellidos.
	sinDireccion := cliente
	sinDireccion.Data = map[string]any{"nif": "FR12345678901", "pais": "FR"}
	if _, err := facturaeDe(org, false, reg, "base", "iva", sinDireccion); err == nil || !strings.Contains(err.Error(), "dirección de Client Français SARL") {
		t.Errorf("sin dirección: %v", err)
	}
	orgSinCP := org
	orgSinCP.CP = "280"
	if _, err := facturaeDe(orgSinCP, false, reg, "base", "iva", cliente); err == nil || !strings.Contains(err.Error(), "código postal") {
		t.Errorf("CP español inválido: %v", err)
	}
}

func latin1(s string) []byte {
	out := make([]byte, 0, len(s))
	for _, r := range s {
		out = append(out, byte(r))
	}
	return out
}
