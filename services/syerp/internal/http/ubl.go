// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package syerphttp

import (
	"encoding/xml"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/syerp/internal/store"
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
// una línea. Las facturas de este ERP no llevan líneas propias (base, IVA y
// total), así que la línea es la base entera.

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

// UBL — GET /api/v1/{modelo}/{id}/ubl — la factura como XML UBL 2.1.
// Solo para los modelos que cobran (facturas de venta): lo que se emite.
func (h *Handlers) UBL(w http.ResponseWriter, r *http.Request) {
	m := h.modeloDe(w, r)
	if m == nil {
		return
	}
	if m.Contable == nil || !m.Contable.Cobro {
		httpx.Fail(w, r, errNoContable.WithMessage("Only issued invoices can be exported as UBL."))
		return
	}
	reg, err := h.store.Obtener(r.Context(), m.Clave, chi.URLParam(r, "id"))
	if err != nil {
		httpx.Fail(w, r, errorDeLectura(err))
		return
	}
	if err := h.efacturaAdmitida(r.Context(), reg); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	org, err := h.store.Organizacion(r.Context())
	if errors.Is(err, store.ErrNoEncontrado) {
		httpx.Fail(w, r, errSinOrganizacion)
		return
	}
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var cliente store.Registro
	if id, _ := reg.Data[m.Contable.CampoTercero].(string); id != "" {
		if cliente, err = h.store.Obtener(r.Context(), "contacto", id); err != nil && !errors.Is(err, store.ErrNoEncontrado) {
			httpx.Fail(w, r, err)
			return
		}
	}

	base, _ := centimos(reg.Data[m.Contable.CampoBase])
	iva, _ := centimos(reg.Data[m.Contable.CampoIVA])
	total := base + iva
	pct := "0.00"
	if base != 0 {
		pct = fmt.Sprintf("%.2f", float64(iva)*100/float64(base))
	}
	moneda := "EUR"
	dinero := func(c int64) ublMoney { return ublMoney{moneda, importe(c)} }
	texto := func(k string) string { s, _ := cliente.Data[k].(string); return s }

	f := ublFactura{
		CAC:           "urn:oasis:names:specification:ubl:schema:xsd:CommonAggregateComponents-2",
		CBC:           "urn:oasis:names:specification:ubl:schema:xsd:CommonBasicComponents-2",
		Customization: "urn:cen.eu:en16931:2017#compliant#urn:fdc:peppol.eu:2017:poacc:billing:3.0",
		Profile:       "urn:fdc:peppol.eu:2017:poacc:billing:01:1.0",
		ID:            reg.Ref,
		Fecha:         cadena(reg.Data["fecha"]),
		Vencimiento:   cadena(reg.Data["vencimiento"]),
		Tipo:          "380",
		Nota:          cadena(reg.Data["notas"]),
		Moneda:        moneda,
		Emisor: ublParty{
			Nombre: org.Nombre, Calle: org.Direccion, Ciudad: org.Ciudad, CP: org.CP, Pais: org.Pais,
			NIF: org.NIF, Esquema: "VAT", Registral: org.Nombre,
		},
		Receptor: ublParty{
			Nombre: cliente.Etiqueta, Calle: texto("direccion"), Ciudad: texto("ciudad"), CP: texto("cp"),
			Pais: codigoPais(texto("pais"), org.Pais), NIF: texto("nif"), Esquema: "VAT", Registral: cliente.Etiqueta,
		},
		IVATotal: dinero(iva), IVABase: dinero(base), IVACuota: dinero(iva),
		IVACat: categoriaIVA(iva), IVAPct: pct, IVAEsquema: "VAT",
		LineaTotal: dinero(base), SinImpuesto: dinero(base), ConImpuesto: dinero(total), APagar: dinero(total),
		LineaID: "1", LineaImporte: dinero(base), LineaNombre: reg.Ref,
		LineaIVACat: categoriaIVA(iva), LineaIVAPct: pct, LineaIVAEsq: "VAT", LineaPrecio: dinero(base),
	}
	f.LineaCantidad.Unidad, f.LineaCantidad.Valor = "C62", "1"
	if f.ID == "" {
		f.ID = reg.ID
	}
	// La URL del QR tributario, en la nota: Peppol admite una sola.
	_, urlQR, err := h.registrosDeFactura(r.Context(), reg)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if urlQR != "" {
		f.Nota = strings.TrimSpace(f.Nota + "\nVERI*FACTU. QR tributario: " + urlQR)
	}

	cuerpo, err := xml.MarshalIndent(f, "", "  ")
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.xml"`, strings.ReplaceAll(f.ID, "/", "-")))
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(xml.Header))
	_, _ = w.Write(cuerpo)
}

func cadena(v any) string {
	s, _ := v.(string)
	return s
}

// categoriaIVA: S = tipo estándar; Z = tipo cero cuando no hay cuota.
func categoriaIVA(iva int64) string {
	if iva == 0 {
		return "Z"
	}
	return "S"
}

// codigoPais deja pasar un ISO de dos letras y, si el contacto trae el país
// escrito de otra forma, cae al del emisor: un código inventado invalida el
// fichero entero en el receptor.
func codigoPais(s, respaldo string) string {
	s = strings.ToUpper(strings.TrimSpace(s))
	if len(s) == 2 {
		return s
	}
	return respaldo
}
