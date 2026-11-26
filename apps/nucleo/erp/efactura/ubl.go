// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package efactura

import (
	"encoding/xml"
	"fmt"
	"strings"
)

// UBL 2.1 (EN 16931, perfil Peppol BIS Billing 3.0): la factura electrónica que
// entienden SAP, Dynamics, Sage, Odoo, Holded y cualquier plataforma Peppol.
// Es la vía de salida hacia OTROS ERP —y hacia la facturación electrónica
// obligatoria entre empresas que la Unión está imponiendo— sin un conector
// por destino.
//
// Se escribe con `encoding/xml` y nombres con prefijo en la etiqueta: Go emite
// `<cbc:ID>` tal cual y los espacios de nombres se declaran en la raíz. Es el
// subconjunto MÍNIMO válido: emisor, receptor, totales, un desglose de IVA y
// una línea, que es la base entera.

type ublMoney struct {
	Moneda string `xml:"currencyID,attr"`
	Valor  string `xml:",chardata"`
}

type ublParty struct {
	Nombre    string `xml:"cac:PartyName>cbc:Name"`
	Calle     string `xml:"cac:PostalAddress>cbc:StreetName,omitempty"`
	Ciudad    string `xml:"cac:PostalAddress>cbc:CityName,omitempty"`
	CP        string `xml:"cac:PostalAddress>cbc:PostalZone,omitempty"`
	Pais      string `xml:"cac:PostalAddress>cac:Country>cbc:IdentificationCode"`
	NIF       string `xml:"cac:PartyTaxScheme>cbc:CompanyID,omitempty"`
	Esquema   string `xml:"cac:PartyTaxScheme>cac:TaxScheme>cbc:ID"`
	Registral string `xml:"cac:PartyLegalEntity>cbc:RegistrationName"`
}

type ublFactura struct {
	XMLName xml.Name `xml:"urn:oasis:names:specification:ubl:schema:xsd:Invoice-2 Invoice"`
	CAC     string   `xml:"xmlns:cac,attr"`
	CBC     string   `xml:"xmlns:cbc,attr"`

	Customization string   `xml:"cbc:CustomizationID"`
	Profile       string   `xml:"cbc:ProfileID"`
	ID            string   `xml:"cbc:ID"`
	Fecha         string   `xml:"cbc:IssueDate"`
	Vencimiento   string   `xml:"cbc:DueDate,omitempty"`
	Tipo          string   `xml:"cbc:InvoiceTypeCode"`
	Nota          string   `xml:"cbc:Note,omitempty"`
	Moneda        string   `xml:"cbc:DocumentCurrencyCode"`
	Emisor        ublParty `xml:"cac:AccountingSupplierParty>cac:Party"`
	Receptor      ublParty `xml:"cac:AccountingCustomerParty>cac:Party"`

	IVATotal    ublMoney `xml:"cac:TaxTotal>cbc:TaxAmount"`
	IVABase     ublMoney `xml:"cac:TaxTotal>cac:TaxSubtotal>cbc:TaxableAmount"`
	IVACuota    ublMoney `xml:"cac:TaxTotal>cac:TaxSubtotal>cbc:TaxAmount"`
	IVACat      string   `xml:"cac:TaxTotal>cac:TaxSubtotal>cac:TaxCategory>cbc:ID"`
	IVAPct      string   `xml:"cac:TaxTotal>cac:TaxSubtotal>cac:TaxCategory>cbc:Percent"`
	IVAEsquema  string   `xml:"cac:TaxTotal>cac:TaxSubtotal>cac:TaxCategory>cac:TaxScheme>cbc:ID"`
	LineaTotal  ublMoney `xml:"cac:LegalMonetaryTotal>cbc:LineExtensionAmount"`
	SinImpuesto ublMoney `xml:"cac:LegalMonetaryTotal>cbc:TaxExclusiveAmount"`
	ConImpuesto ublMoney `xml:"cac:LegalMonetaryTotal>cbc:TaxInclusiveAmount"`
	APagar      ublMoney `xml:"cac:LegalMonetaryTotal>cbc:PayableAmount"`

	LineaID       string `xml:"cac:InvoiceLine>cbc:ID"`
	LineaCantidad struct {
		Unidad string `xml:"unitCode,attr"`
		Valor  string `xml:",chardata"`
	} `xml:"cac:InvoiceLine>cbc:InvoicedQuantity"`
	LineaImporte ublMoney `xml:"cac:InvoiceLine>cbc:LineExtensionAmount"`
	LineaNombre  string   `xml:"cac:InvoiceLine>cac:Item>cbc:Name"`
	LineaIVACat  string   `xml:"cac:InvoiceLine>cac:Item>cac:ClassifiedTaxCategory>cbc:ID"`
	LineaIVAPct  string   `xml:"cac:InvoiceLine>cac:Item>cac:ClassifiedTaxCategory>cbc:Percent"`
	LineaIVAEsq  string   `xml:"cac:InvoiceLine>cac:Item>cac:ClassifiedTaxCategory>cac:TaxScheme>cbc:ID"`
	LineaPrecio  ublMoney `xml:"cac:InvoiceLine>cac:Price>cbc:PriceAmount"`
}

// UBL escribe la factura como XML UBL 2.1 y devuelve el fichero y su nombre. Quien
// llama comprueba antes Admitida. receptor nil es una factura sin cliente.
func UBL(emisor Parte, receptor *Parte, f Factura) ([]byte, string, error) {
	var r Parte
	if receptor != nil {
		r = *receptor
	}
	base, iva := f.Base, f.IVA
	total := base + iva
	pct := "0.00"
	if base != 0 {
		pct = fmt.Sprintf("%.2f", float64(iva)*100/float64(base))
	}
	moneda := "EUR"
	dinero := func(c int64) ublMoney { return ublMoney{moneda, importe(c)} }

	u := ublFactura{
		CAC:           "urn:oasis:names:specification:ubl:schema:xsd:CommonAggregateComponents-2",
		CBC:           "urn:oasis:names:specification:ubl:schema:xsd:CommonBasicComponents-2",
		Customization: "urn:cen.eu:en16931:2017#compliant#urn:fdc:peppol.eu:2017:poacc:billing:3.0",
		Profile:       "urn:fdc:peppol.eu:2017:poacc:billing:01:1.0",
		ID:            f.numero(),
		Fecha:         f.Fecha,
		Vencimiento:   f.Vencimiento,
		Tipo:          "380",
		Nota:          f.Notas,
		Moneda:        moneda,
		Emisor: ublParty{
			Nombre: emisor.Nombre, Calle: emisor.Direccion, Ciudad: emisor.Ciudad, CP: emisor.CP, Pais: emisor.Pais,
			NIF: emisor.NIF, Esquema: "VAT", Registral: emisor.Nombre,
		},
		Receptor: ublParty{
			Nombre: r.Nombre, Calle: r.Direccion, Ciudad: r.Ciudad, CP: r.CP,
			Pais: CodigoPais(r.Pais, emisor.Pais), NIF: r.NIF, Esquema: "VAT", Registral: r.Nombre,
		},
		IVATotal: dinero(iva), IVABase: dinero(base), IVACuota: dinero(iva),
		IVACat: categoriaIVA(iva), IVAPct: pct, IVAEsquema: "VAT",
		LineaTotal: dinero(base), SinImpuesto: dinero(base), ConImpuesto: dinero(total), APagar: dinero(total),
		LineaID: "1", LineaImporte: dinero(base), LineaNombre: f.Numero,
		LineaIVACat: categoriaIVA(iva), LineaIVAPct: pct, LineaIVAEsq: "VAT", LineaPrecio: dinero(base),
	}
	u.LineaCantidad.Unidad, u.LineaCantidad.Valor = "C62", "1"
	// La URL del QR tributario, en la nota: Peppol admite una sola.
	if f.QR != "" {
		u.Nota = strings.TrimSpace(u.Nota + "\nVERI*FACTU. QR tributario: " + f.QR)
	}
	cuerpo, err := xml.MarshalIndent(u, "", "  ")
	if err != nil {
		return nil, "", err
	}
	return append([]byte(xml.Header), cuerpo...), strings.ReplaceAll(u.ID, "/", "-") + ".xml", nil
}

// categoriaIVA: S = tipo estándar; Z = tipo cero cuando no hay cuota.
func categoriaIVA(iva int64) string {
	if iva == 0 {
		return "Z"
	}
	return "S"
}
