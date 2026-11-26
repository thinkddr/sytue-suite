// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package efactura

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"math"
	"strings"

	"golang.org/x/net/html/charset"

	"git.sytue.eu/sytue/one/apps/nucleo/erp/dominio"
	"git.sytue.eu/sytue/one/apps/nucleo/erp/importar"
	"git.sytue.eu/sytue/one/apps/nucleo/validation"
)

// Facturas electrónicas RECIBIDAS. Es la otra mitad de la salida UBL: lo que
// emiten SAP, Dynamics, Sage, Odoo, Holded, a3 o cualquier plataforma Peppol
// entra como factura de compra, sin un conector por origen.
//
// Se leen las tres sintaxis que circulan en Europa:
//   - UBL 2.1 (Invoice y CreditNote): Peppol BIS 3, XRechnung UBL, la de FACe B2B;
//   - UN/CEFACT CII: Factur-X, ZUGFeRD y XRechnung CII (el XML, no el PDF);
//   - Facturae 3.2.x: la española, con varias facturas por fichero.
//
// Solo se toma la cabecera: emisor, receptor, fechas y totales.
//
// ponytail: la retención (IRPF) no se guarda; el total es base + impuestos, que
// es lo que cuadra con el asiento. Si hace falta, un campo `retencion` en el
// catálogo y leer TaxesWithheld / AllowanceTotalAmount.

// Recibida es lo que Sytue ERP necesita de una factura electrónica.
type Recibida struct {
	Numero      string
	Fecha       string // AAAA-MM-DD
	Vencimiento string
	Moneda      string
	Nota        string
	// Céntimos. Una rectificativa (CreditNote, CII 381) llega en negativo.
	Base, IVA, Total int64
	Emisor, Receptor ParteRecibida
}

type ParteRecibida struct {
	Nombre, NIF, Direccion, Ciudad, CP, Pais, Correo string
	// Persona: Facturae distingue persona física de jurídica; UBL y CII no.
	Persona bool
}

// Leer reconoce la sintaxis por el elemento raíz y lee sus facturas.
func Leer(cuerpo []byte) ([]Recibida, error) {
	raiz, err := elementoRaiz(cuerpo)
	if err != nil {
		return nil, err
	}
	switch raiz {
	case "Invoice", "CreditNote":
		var u ublLeida
		if err := decodificar(cuerpo, &u); err != nil {
			return nil, err
		}
		f, err := u.factura(raiz == "CreditNote")
		return []Recibida{f}, err
	case "CrossIndustryInvoice":
		var c ciiLeida
		if err := decodificar(cuerpo, &c); err != nil {
			return nil, err
		}
		f, err := c.factura()
		return []Recibida{f}, err
	case "Facturae":
		var fe facturaeLeida
		if err := decodificar(cuerpo, &fe); err != nil {
			return nil, err
		}
		return fe.facturas()
	}
	return nil, fmt.Errorf("<%s> no es una factura UBL, CII ni Facturae", raiz)
}

// decodificar admite ficheros en ISO-8859-1 o Windows-1252, que es como los
// guardan todavía muchos programas de facturación españoles. encoding/xml no
// resuelve entidades externas: un XML hostil no puede leer ficheros del servidor.
func decodificar(cuerpo []byte, v any) error {
	dec := xml.NewDecoder(bytes.NewReader(cuerpo))
	dec.CharsetReader = charset.NewReaderLabel
	if err := dec.Decode(v); err != nil {
		return fmt.Errorf("XML ilegible: %w", err)
	}
	return nil
}

func elementoRaiz(cuerpo []byte) (string, error) {
	dec := xml.NewDecoder(bytes.NewReader(cuerpo))
	dec.CharsetReader = charset.NewReaderLabel
	for {
		tok, err := dec.Token()
		if err != nil {
			return "", fmt.Errorf("XML ilegible: %w", err)
		}
		if el, ok := tok.(xml.StartElement); ok {
			return el.Name.Local, nil
		}
	}
}

// --- UBL 2.1 ----------------------------------------------------------------

// Las etiquetas van sin prefijo: encoding/xml casa por nombre local, y así da
// igual que el emisor use cbc:, ubl: o el espacio de nombres por defecto.
type ublLeida struct {
	ID          string        `xml:"ID"`
	Fecha       string        `xml:"IssueDate"`
	Vencimiento string        `xml:"DueDate"`
	VencePago   string        `xml:"PaymentMeans>PaymentDueDate"`
	Moneda      string        `xml:"DocumentCurrencyCode"`
	Notas       []string      `xml:"Note"`
	Emisor      ublParteLeida `xml:"AccountingSupplierParty>Party"`
	Receptor    ublParteLeida `xml:"AccountingCustomerParty>Party"`
	// Puede haber dos TaxTotal: uno en la moneda del documento y otro en la del IVA.
	IVA  []ublMoney `xml:"TaxTotal>TaxAmount"`
	Base string     `xml:"LegalMonetaryTotal>TaxExclusiveAmount"`
}

type ublParteLeida struct {
	Nombre    string   `xml:"PartyName>Name"`
	Registral string   `xml:"PartyLegalEntity>RegistrationName"`
	NIFs      []string `xml:"PartyTaxScheme>CompanyID"`
	Legal     string   `xml:"PartyLegalEntity>CompanyID"`
	Calle     string   `xml:"PostalAddress>StreetName"`
	Ciudad    string   `xml:"PostalAddress>CityName"`
	CP        string   `xml:"PostalAddress>PostalZone"`
	Pais      string   `xml:"PostalAddress>Country>IdentificationCode"`
	Correo    string   `xml:"Contact>ElectronicMail"`
}

func (u ublLeida) factura(abono bool) (Recibida, error) {
	f := Recibida{
		Numero: strings.TrimSpace(u.ID), Fecha: strings.TrimSpace(u.Fecha), Moneda: strings.TrimSpace(u.Moneda),
		Vencimiento: Primero(u.Vencimiento, u.VencePago), Nota: strings.Join(u.Notas, "\n"),
		Emisor: u.Emisor.parte(), Receptor: u.Receptor.parte(),
	}
	iva := ""
	for i, t := range u.IVA {
		if i == 0 || t.Moneda == f.Moneda {
			iva = t.Valor
		}
	}
	return f.conImportes(u.Base, iva, abono)
}

func (p ublParteLeida) parte() ParteRecibida {
	return ParteRecibida{
		Nombre: Primero(p.Registral, p.Nombre), NIF: Primero(append(p.NIFs, p.Legal)...),
		Direccion: strings.TrimSpace(p.Calle), Ciudad: strings.TrimSpace(p.Ciudad), CP: strings.TrimSpace(p.CP),
		Pais: strings.TrimSpace(p.Pais), Correo: strings.TrimSpace(p.Correo),
	}
}

// --- UN/CEFACT CII ------------------------------------------------------------

type ciiLeida struct {
	ID      string   `xml:"ExchangedDocument>ID"`
	Tipo    string   `xml:"ExchangedDocument>TypeCode"`
	Fecha   string   `xml:"ExchangedDocument>IssueDateTime>DateTimeString"`
	Notas   []string `xml:"ExchangedDocument>IncludedNote>Content"`
	Acuerdo struct {
		Emisor   ciiParte `xml:"SellerTradeParty"`
		Receptor ciiParte `xml:"BuyerTradeParty"`
	} `xml:"SupplyChainTradeTransaction>ApplicableHeaderTradeAgreement"`
	Liquidacion struct {
		Moneda string     `xml:"InvoiceCurrencyCode"`
		Vence  []string   `xml:"SpecifiedTradePaymentTerms>DueDateDateTime>DateTimeString"`
		Base   string     `xml:"SpecifiedTradeSettlementHeaderMonetarySummation>TaxBasisTotalAmount"`
		IVA    []ublMoney `xml:"SpecifiedTradeSettlementHeaderMonetarySummation>TaxTotalAmount"`
	} `xml:"SupplyChainTradeTransaction>ApplicableHeaderTradeSettlement"`
}

type ciiParte struct {
	Nombre string   `xml:"Name"`
	NIFs   []string `xml:"SpecifiedTaxRegistration>ID"`
	Legal  string   `xml:"SpecifiedLegalOrganization>ID"`
	Calle  string   `xml:"PostalTradeAddress>LineOne"`
	Ciudad string   `xml:"PostalTradeAddress>CityName"`
	CP     string   `xml:"PostalTradeAddress>PostcodeCode"`
	Pais   string   `xml:"PostalTradeAddress>CountryID"`
	Correo string   `xml:"URIUniversalCommunication>URIID"`
}

func (c ciiLeida) factura() (Recibida, error) {
	f := Recibida{
		Numero: strings.TrimSpace(c.ID), Fecha: fecha102(c.Fecha), Moneda: strings.TrimSpace(c.Liquidacion.Moneda),
		Vencimiento: fecha102(Primero(c.Liquidacion.Vence...)), Nota: strings.Join(c.Notas, "\n"),
		Emisor: c.Acuerdo.Emisor.parte(), Receptor: c.Acuerdo.Receptor.parte(),
	}
	iva := ""
	for i, t := range c.Liquidacion.IVA {
		if i == 0 || t.Moneda == f.Moneda {
			iva = t.Valor
		}
	}
	// 381 es la nota de crédito; 380 y el resto, factura.
	return f.conImportes(c.Liquidacion.Base, iva, strings.TrimSpace(c.Tipo) == "381")
}

func (p ciiParte) parte() ParteRecibida {
	return ParteRecibida{
		Nombre: strings.TrimSpace(p.Nombre), NIF: Primero(append(p.NIFs, p.Legal)...),
		Direccion: strings.TrimSpace(p.Calle), Ciudad: strings.TrimSpace(p.Ciudad), CP: strings.TrimSpace(p.CP),
		Pais: strings.TrimSpace(p.Pais), Correo: strings.TrimSpace(p.Correo),
	}
}

// fecha102 pasa el formato 102 de UN/CEFACT (AAAAMMDD) a AAAA-MM-DD.
func fecha102(s string) string {
	s = strings.TrimSpace(s)
	if len(s) == 8 {
		return s[:4] + "-" + s[4:6] + "-" + s[6:]
	}
	return s
}

// --- Facturae 3.2.x -----------------------------------------------------------

type facturaeLeida struct {
	Emisor   facturaeParteLeida `xml:"Parties>SellerParty"`
	Receptor facturaeParteLeida `xml:"Parties>BuyerParty"`
	Facturas []struct {
		Numero string `xml:"InvoiceHeader>InvoiceNumber"`
		Serie  string `xml:"InvoiceHeader>InvoiceSeriesCode"`
		Fecha  string `xml:"InvoiceIssueData>IssueDate"`
		Moneda string `xml:"InvoiceIssueData>InvoiceCurrencyCode"`
		Base   string `xml:"InvoiceTotals>TotalGrossAmountBeforeTaxes"`
		IVA    string `xml:"InvoiceTotals>TotalTaxOutputs"`
		Vence  string `xml:"PaymentDetails>Installment>InstallmentDueDate"`
		Nota   string `xml:"AdditionalData>InvoiceAdditionalInformation"`
	} `xml:"Invoices>Invoice"`
}

type facturaeParteLeida struct {
	Tipo      string `xml:"TaxIdentification>PersonTypeCode"`
	NIF       string `xml:"TaxIdentification>TaxIdentificationNumber"`
	Razon     string `xml:"LegalEntity>CorporateName"`
	Nombre    string `xml:"Individual>Name"`
	Apellido1 string `xml:"Individual>FirstSurname"`
	Apellido2 string `xml:"Individual>SecondSurname"`
	// La dirección cuelga de la persona jurídica o de la física, y va en España
	// o fuera.
	JuridicaES    facturaeDireccionLeida `xml:"LegalEntity>AddressInSpain"`
	JuridicaFuera facturaeDireccionLeida `xml:"LegalEntity>OverseasAddress"`
	FisicaES      facturaeDireccionLeida `xml:"Individual>AddressInSpain"`
	FisicaFuera   facturaeDireccionLeida `xml:"Individual>OverseasAddress"`
	CorreoJur     string                 `xml:"LegalEntity>ContactDetails>ElectronicMail"`
	CorreoFis     string                 `xml:"Individual>ContactDetails>ElectronicMail"`
}

type facturaeDireccionLeida struct {
	Calle       string `xml:"Address"`
	CP          string `xml:"PostCode"`
	Poblacion   string `xml:"Town"`
	CPPoblacion string `xml:"PostCodeAndTown"`
	Pais        string `xml:"CountryCode"`
}

func (fe facturaeLeida) facturas() ([]Recibida, error) {
	if len(fe.Facturas) == 0 {
		return nil, errors.New("el fichero Facturae no trae ninguna factura")
	}
	emisor, receptor := fe.Emisor.parte(), fe.Receptor.parte()
	out := make([]Recibida, 0, len(fe.Facturas))
	for _, x := range fe.Facturas {
		numero := strings.TrimSpace(x.Numero)
		if serie := strings.TrimSpace(x.Serie); serie != "" {
			numero = serie + "/" + numero
		}
		f := Recibida{
			Numero: numero, Fecha: strings.TrimSpace(x.Fecha), Vencimiento: strings.TrimSpace(x.Vence),
			Moneda: strings.TrimSpace(x.Moneda), Nota: strings.TrimSpace(x.Nota), Emisor: emisor, Receptor: receptor,
		}
		// Una rectificativa Facturae ya trae los importes en negativo.
		f, err := f.conImportes(x.Base, x.IVA, false)
		if err != nil {
			return nil, fmt.Errorf("factura %s: %w", numero, err)
		}
		out = append(out, f)
	}
	return out, nil
}

func (p facturaeParteLeida) parte() ParteRecibida {
	d := p.JuridicaES
	for _, otra := range []facturaeDireccionLeida{p.JuridicaFuera, p.FisicaES, p.FisicaFuera} {
		if d.Calle == "" {
			d = otra
		}
	}
	cp, ciudad := strings.TrimSpace(d.CP), strings.TrimSpace(d.Poblacion)
	if d.CPPoblacion != "" {
		// Fuera de España va junto («75001 Paris»): se guarda entero como ciudad.
		ciudad = strings.TrimSpace(d.CPPoblacion)
	}
	nombre := strings.TrimSpace(p.Razon)
	if nombre == "" {
		nombre = strings.Join(strings.Fields(p.Nombre+" "+p.Apellido1+" "+p.Apellido2), " ")
	}
	return ParteRecibida{
		Nombre: nombre, NIF: strings.TrimSpace(p.NIF), Direccion: strings.TrimSpace(d.Calle),
		Ciudad: ciudad, CP: cp, Pais: ISO2(d.Pais), Correo: Primero(p.CorreoJur, p.CorreoFis),
		Persona: strings.TrimSpace(p.Tipo) == "F",
	}
}

// --- Común ---------------------------------------------------------------------

// conImportes pasa base e IVA a céntimos exactos y calcula el total como su suma,
// que es lo que cuadra con el asiento.
func (f Recibida) conImportes(base, iva string, abono bool) (Recibida, error) {
	var err error
	if f.Base, err = importar.CentimosDeDecimal(base); err != nil {
		return f, fmt.Errorf("base imponible: %w", err)
	}
	if f.IVA, err = importar.CentimosDeDecimal(iva); err != nil {
		return f, fmt.Errorf("impuestos: %w", err)
	}
	if abono {
		f.Base, f.IVA = -f.Base, -f.IVA
	}
	f.Total = f.Base + f.IVA
	return f, nil
}

// Comprobar rechaza lo que no se puede guardar sin mentir en el libro. nifOrg es el
// NIF de la organización que la recibe; vacío, no se comprueba el receptor.
func (f Recibida) Comprobar(nifOrg string) error {
	switch {
	case f.Numero == "":
		return errors.New("la factura no trae número")
	case len(f.Fecha) != len("2006-01-02"):
		return fmt.Errorf("fecha de emisión ilegible: %q", f.Fecha)
	case f.Emisor.NIF == "" && f.Emisor.Nombre == "":
		return errors.New("la factura no identifica al emisor")
	case f.Moneda != "" && !strings.EqualFold(f.Moneda, dominio.MonedaPorDefecto):
		// Los documentos de este ERP no llevan divisa: guardar dólares como euros
		// descuadraría el libro sin que nadie lo notara.
		return fmt.Errorf("moneda %s: solo se importan facturas en %s", f.Moneda, dominio.MonedaPorDefecto)
	}
	// Una factura que no va a nombre de esta organización no es un gasto suyo.
	if nifOrg != "" && f.Receptor.NIF != "" && !dominio.MismoNIF(nifOrg, f.Receptor.NIF) {
		return fmt.Errorf("va a nombre de %s (%s), no de esta organización", f.Receptor.Nombre, f.Receptor.NIF)
	}
	return nil
}

// ComprobarAsistencia es lo que exige el asistente de facturas recibidas, más
// estricto que la importación: que vaya a nombre de la organización y en euros
// explícitos. Los mensajes van en inglés, como los errores del servicio.
func (f Recibida) ComprobarAsistencia(nifOrg string) error {
	if nifOrg == "" || f.Receptor.NIF == "" || !dominio.MismoNIF(nifOrg, f.Receptor.NIF) {
		return errors.New("The invoice must identify your organisation as its recipient. Check both tax identification numbers.") //nolint:revive,staticcheck // lo lee la persona: va tal cual en la respuesta del asistente, aquí y en el servicio
	}
	if !strings.EqualFold(f.Moneda, dominio.MonedaPorDefecto) {
		return errors.New("The invoice must explicitly use EUR. Currency conversion is not performed.") //nolint:revive,staticcheck // lo lee la persona: va tal cual en la respuesta del asistente, aquí y en el servicio
	}
	if err := f.Comprobar(nifOrg); err != nil {
		return err
	}
	if (f.IVA > 0 && f.Base > math.MaxInt64-f.IVA) || (f.IVA < 0 && f.Base < math.MinInt64-f.IVA) {
		return errors.New("The invoice total exceeds the supported amount.") //nolint:revive,staticcheck // lo lee la persona: va tal cual en la respuesta del asistente, aquí y en el servicio
	}
	return nil
}

// ClaveExterna es la identidad de origen de la factura: el NIF del proveedor (sin el
// prefijo de país, que UBL pone y Facturae no) o su nombre, y el número. El número lo
// pone el proveedor y solo es único por proveedor; con esta clave, importar dos veces
// el mismo fichero actualiza en vez de duplicar.
func (f Recibida) ClaveExterna() string {
	emisor := f.Emisor.Nombre
	if v := dominio.VariantesNIF(f.Emisor.NIF); len(v) > 0 {
		emisor = v[len(v)-1]
	}
	return emisor + "/" + f.Numero
}

// DatosContacto son los campos de la ficha de `contacto` del emisor, sin validar.
func (p ParteRecibida) DatosContacto() map[string]any {
	tipo := "organizacion"
	if p.Persona {
		tipo = "persona"
	}
	datos := map[string]any{"nombre": Primero(p.Nombre, p.NIF), "tipo": tipo}
	if validation.IsEmail(p.Correo) {
		// En CII esa dirección también puede ser un identificador Peppol: un correo
		// mal formado no puede tumbar la factura entera.
		datos["correo"] = p.Correo
	}
	for campo, valor := range map[string]string{
		"nif": p.NIF, "direccion": p.Direccion, "ciudad": p.Ciudad, "cp": p.CP, "pais": p.Pais,
	} {
		if valor != "" {
			datos[campo] = valor
		}
	}
	return datos
}

// EsXML distingue una factura electrónica de un CSV por su primer carácter.
func EsXML(cuerpo []byte) bool {
	return bytes.HasPrefix(bytes.TrimLeft(cuerpo, " \t\r\n"), []byte("<"))
}
