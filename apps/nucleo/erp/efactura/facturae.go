// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package efactura

import (
	"encoding/xml"
	"errors"
	"fmt"
	"math"
	"math/big"
	"regexp"
	"slices"
	"sort"
	"strings"

	"git.sytue.eu/sytue/one/apps/nucleo/erp/dominio"
	"git.sytue.eu/sytue/one/apps/nucleo/validation"
)

// Facturae 3.2.2: la factura electrónica española. Es la que piden FACe y los
// puntos de entrada de las administraciones, y la que importan a3, Sage,
// Holded y casi cualquier programa de contabilidad español.
//
// Sale SIN FIRMAR. La firma XAdES es del emisor, con su certificado, y Sytue no
// lo custodia: el fichero se firma con el programa de la gestoría antes de
// subirlo. Los códigos DIR3 del receptor viajan en AdministrativeCentres.
//
// ponytail: IVA como único impuesto y sin rectificativas; es lo que las facturas
// de este ERP pueden expresar hoy. IGIC (Canarias), retenciones y el bloque
// Corrective llegan cuando el documento tenga líneas, impuestos y factura de origen.

const facturaeNS = "http://www.facturae.gob.es/formato/Versiones/Facturaev3_2_2.xml"

type FacturaeImporte struct {
	Total string `xml:"TotalAmount"`
}

type FacturaeDireccion struct {
	Calle string `xml:"Address"`
	// En España, CP y población por separado y la provincia sale del CP; fuera,
	// juntos.
	CP          string `xml:"PostCode,omitempty"`
	Poblacion   string `xml:"Town,omitempty"`
	CPPoblacion string `xml:"PostCodeAndTown,omitempty"`
	Provincia   string `xml:"Province"`
	Pais        string `xml:"CountryCode"`
}

type FacturaeTitular struct {
	Nombre       string             `xml:"Name,omitempty"`
	Apellido1    string             `xml:"FirstSurname,omitempty"`
	Apellido2    string             `xml:"SecondSurname,omitempty"`
	Razon        string             `xml:"CorporateName,omitempty"`
	EnEspana     *FacturaeDireccion `xml:"AddressInSpain,omitempty"`
	Extranjero   *FacturaeDireccion `xml:"OverseasAddress,omitempty"`
	CorreoDentro string             `xml:"ContactDetails>ElectronicMail,omitempty"`
}

type FacturaeParte struct {
	TipoPersona string           `xml:"TaxIdentification>PersonTypeCode"`
	Residencia  string           `xml:"TaxIdentification>ResidenceTypeCode"`
	NIF         string           `xml:"TaxIdentification>TaxIdentificationNumber"`
	Centros     *FacturaeCentros `xml:"AdministrativeCentres,omitempty"`
	Juridica    *FacturaeTitular `xml:"LegalEntity,omitempty"`
	Fisica      *FacturaeTitular `xml:"Individual,omitempty"`
}

type FacturaeCentros struct {
	Centros []FacturaeCentro `xml:"AdministrativeCentre"`
}

type FacturaeCentro struct {
	Codigo    string             `xml:"CentreCode"`
	Rol       string             `xml:"RoleTypeCode"`
	Direccion *FacturaeDireccion `xml:"AddressInSpain"`
}

type FacturaeImpuesto struct {
	Tipo  string          `xml:"TaxTypeCode"`
	Pct   string          `xml:"TaxRate"`
	Base  FacturaeImporte `xml:"TaxableBase"`
	Cuota FacturaeImporte `xml:"TaxAmount"`
}

// Los campos van en el orden de las secuencias del XSD: encoding/xml los
// escribe en el orden en que se declaran, y un elemento fuera de sitio invalida
// el fichero entero.
type FacturaeDoc struct {
	XMLName xml.Name `xml:"fe:Facturae"`
	NS      string   `xml:"xmlns:fe,attr"`

	Version    string          `xml:"FileHeader>SchemaVersion"`
	Modalidad  string          `xml:"FileHeader>Modality"`
	TipoEmisor string          `xml:"FileHeader>InvoiceIssuerType"`
	Lote       string          `xml:"FileHeader>Batch>BatchIdentifier"`
	Cuantas    int             `xml:"FileHeader>Batch>InvoicesCount"`
	LoteTotal  FacturaeImporte `xml:"FileHeader>Batch>TotalInvoicesAmount"`
	LotePend   FacturaeImporte `xml:"FileHeader>Batch>TotalOutstandingAmount"`
	LoteExig   FacturaeImporte `xml:"FileHeader>Batch>TotalExecutableAmount"`
	LoteMoneda string          `xml:"FileHeader>Batch>InvoiceCurrencyCode"`
	Vendedor   FacturaeParte   `xml:"Parties>SellerParty"`
	Comprador  FacturaeParte   `xml:"Parties>BuyerParty"`
	Factura    FacturaeFactura `xml:"Invoices>Invoice"`
}

type FacturaeFactura struct {
	Numero   string `xml:"InvoiceHeader>InvoiceNumber"`
	TipoDoc  string `xml:"InvoiceHeader>InvoiceDocumentType"`
	Clase    string `xml:"InvoiceHeader>InvoiceClass"`
	Fecha    string `xml:"InvoiceIssueData>IssueDate"`
	Moneda   string `xml:"InvoiceIssueData>InvoiceCurrencyCode"`
	MonedaTx string `xml:"InvoiceIssueData>TaxCurrencyCode"`
	Idioma   string `xml:"InvoiceIssueData>LanguageName"`

	Impuestos []FacturaeImpuesto `xml:"TaxesOutputs>Tax"`

	Bruto     string          `xml:"InvoiceTotals>TotalGrossAmount"`
	AntesImp  string          `xml:"InvoiceTotals>TotalGrossAmountBeforeTaxes"`
	TotalImp  string          `xml:"InvoiceTotals>TotalTaxOutputs"`
	Retenido  string          `xml:"InvoiceTotals>TotalTaxesWithheld"`
	Total     string          `xml:"InvoiceTotals>InvoiceTotal"`
	Pendiente string          `xml:"InvoiceTotals>TotalOutstandingAmount"`
	Exigible  string          `xml:"InvoiceTotals>TotalExecutableAmount"`
	Lineas    []FacturaeLinea `xml:"Items>InvoiceLine"`
	// Adicional lleva la URL del QR tributario: en una factura electrónica va en
	// la información estructurada (Orden HAC/1177/2024, art. 20.2).
	Adicional string `xml:"AdditionalData>InvoiceAdditionalInformation,omitempty"`
}

type FacturaeLinea struct {
	Descripcion string             `xml:"ItemDescription"`
	Cantidad    string             `xml:"Quantity"`
	Unidad      string             `xml:"UnitOfMeasure"`
	Precio      string             `xml:"UnitPriceWithoutTax"`
	Coste       string             `xml:"TotalCost"`
	Bruto       string             `xml:"GrossAmount"`
	Impuestos   []FacturaeImpuesto `xml:"TaxesOutputs>Tax"`
}

// Facturae arma el documento de la factura. Devuelve un error legible con lo que
// falta: Facturae no admite partes sin dirección, y descubrirlo en FACe es peor.
// receptor nil es una factura sin cliente. Con QR, su URL va en la información
// adicional (Orden HAC/1177/2024, art. 20.2).
func Facturae(emisor Parte, receptor *Parte, f Factura) (FacturaeDoc, error) {
	base, iva := f.Base, f.IVA
	if base < 0 || iva < 0 {
		return FacturaeDoc{}, errors.New("las rectificativas aún no salen como Facturae: falta la referencia a la factura de origen")
	}
	total := base + iva
	fecha := f.Fecha
	if fecha == "" {
		return FacturaeDoc{}, errors.New("la factura no tiene fecha")
	}
	numero := f.numero()

	vendedor, err := parteFacturae("el emisor", emisor.Nombre, emisor.NIF, emisor.Direccion, emisor.CP, emisor.Ciudad, emisor.Pais, "", emisor.Fisica)
	if err != nil {
		return FacturaeDoc{}, err
	}
	if receptor == nil {
		return FacturaeDoc{}, errors.New("la factura no tiene cliente")
	}
	r := *receptor
	comprador, err := parteFacturae(r.Nombre, r.Nombre, strings.TrimSpace(r.NIF), strings.TrimSpace(r.Direccion), strings.TrimSpace(r.CP),
		strings.TrimSpace(r.Ciudad), CodigoPais(r.Pais, emisor.Pais), strings.TrimSpace(r.Correo), r.Fisica)
	if err != nil {
		return FacturaeDoc{}, err
	}

	pct := "0.00"
	if base != 0 {
		// Dos decimales con redondeo entero: 210 de IVA sobre 1000 de base es 21.00.
		centesimas := (iva*10000 + base/2) / base
		pct = fmt.Sprintf("%d.%02d", centesimas/100, centesimas%100)
	}
	impuestos := []FacturaeImpuesto{{Tipo: "01", Pct: pct, Base: FacturaeImporte{importe(base)}, Cuota: FacturaeImporte{importe(iva)}}}
	concepto := f.Notas
	if strings.TrimSpace(concepto) == "" {
		concepto = numero
	}
	if err := centrosFacturae(&comprador, r); err != nil {
		return FacturaeDoc{}, err
	}
	detalle := []FacturaeLinea{{Descripcion: validation.TruncateRunes(concepto, 2500), Cantidad: "1.0", Unidad: "01", Precio: importe(base), Coste: importe(base), Bruto: importe(base), Impuestos: impuestos}}
	if len(f.Lineas) > 0 {
		var err error
		detalle, impuestos, err = detalleFacturae(f.Lineas, base, iva)
		if err != nil {
			return FacturaeDoc{}, err
		}
	}
	pendiente := total
	if f.Estado == "pagado" {
		pendiente = 0
	}
	doc := FacturaeDoc{
		NS: facturaeNS, Version: "3.2.2", Modalidad: "I", TipoEmisor: "EM",
		Lote: validation.TruncateRunes(vendedor.NIF+numero, 70), Cuantas: 1,
		LoteTotal: FacturaeImporte{importe(total)}, LotePend: FacturaeImporte{importe(pendiente)},
		LoteExig: FacturaeImporte{importe(pendiente)}, LoteMoneda: "EUR",
		Vendedor: vendedor, Comprador: comprador,
		Factura: FacturaeFactura{
			Numero: validation.TruncateRunes(numero, 20), TipoDoc: "FC", Clase: "OO",
			Fecha: fecha, Moneda: "EUR", MonedaTx: "EUR", Idioma: "es",
			Impuestos: impuestos,
			Bruto:     importe(base), AntesImp: importe(base), TotalImp: importe(iva), Retenido: "0.00",
			Total: importe(total), Pendiente: importe(pendiente), Exigible: importe(pendiente),
			Lineas: detalle,
		},
	}
	if f.QR != "" {
		doc.Factura.Adicional = "VERI*FACTU. QR tributario: " + f.QR
	}
	return doc, nil
}

// Bytes es el fichero: la cabecera XML y el documento sangrado.
func (d FacturaeDoc) Bytes() ([]byte, error) {
	cuerpo, err := xml.MarshalIndent(d, "", "  ")
	if err != nil {
		return nil, err
	}
	return append([]byte(xml.Header), cuerpo...), nil
}

// Nombre es el nombre del fichero: `facturae-<número>.xml`.
func (d FacturaeDoc) Nombre() string {
	return "facturae-" + strings.ReplaceAll(d.Factura.Numero, "/", "-") + ".xml"
}

// ErrSimplificada es el rechazo de Facturae para una factura simplificada (F2).
var ErrSimplificada = errors.New("una factura simplificada no identifica al comprador")

// Admitida dice si la factura puede salir como factura electrónica (Facturae o
// UBL). Las dos salen hoy con un comprador identificado y un solo tipo de IVA: una
// simplificada no tiene comprador, y una factura con líneas de varios tipos saldría
// con un tipo medio que no existe.
//
// ponytail: se rechaza en vez de sacar una línea por línea de factura; relevo:
// líneas y impuestos por tipo en las dos plantillas.
func Admitida(f Factura) error {
	if f.Simplificada() {
		return errors.New("una factura simplificada no sale como factura electrónica: no identifica al comprador")
	}
	if len(dominio.Desglose(f.Lineas)) > 1 {
		return errors.New("las facturas con líneas de varios tipos de IVA aún no salen como factura electrónica")
	}
	return nil
}

// parteFacturae arma emisor o receptor. `quien` nombra la parte en el error.
func parteFacturae(quien, nombre, nif, calle, cp, ciudad, pais, correo string, fisica bool) (FacturaeParte, error) {
	nombre, calle, cp, ciudad = strings.TrimSpace(nombre), strings.TrimSpace(calle), strings.TrimSpace(cp), strings.TrimSpace(ciudad)
	pais = strings.ToUpper(strings.TrimSpace(pais))
	variantes := dominio.VariantesNIF(nif)
	// Todo lo que falta de una vez: corregir la ficha y volver a descargar para
	// descubrir lo siguiente es la clase de ida y vuelta que hace odiar un ERP.
	var faltan []string
	for _, f := range []struct {
		vacio bool
		que   string
	}{{nombre == "", "el nombre"}, {len(variantes) == 0, "el NIF"}, {calle == "", "la dirección"}} {
		if f.vacio {
			faltan = append(faltan, f.que)
		}
	}
	switch len(faltan) {
	case 0:
	case 1:
		return FacturaeParte{}, fmt.Errorf("falta %s de %s", faltan[0], quien)
	default:
		return FacturaeParte{}, fmt.Errorf("faltan %s y %s de %s", strings.Join(faltan[:len(faltan)-1], ", "), faltan[len(faltan)-1], quien)
	}
	alfa3, ok := paisesISO3[pais]
	if !ok {
		return FacturaeParte{}, fmt.Errorf("el país de %s (%q) no tiene código para Facturae", quien, pais)
	}

	p := FacturaeParte{TipoPersona: "J", Residencia: "E"}
	dir := &FacturaeDireccion{Calle: validation.TruncateRunes(calle, 80), Pais: alfa3}
	titular := &FacturaeTitular{CorreoDentro: validation.TruncateRunes(strings.TrimSpace(correo), 60)}
	switch pais {
	case "ES":
		p.Residencia = "R"
		// El NIF español va sin prefijo de país.
		p.NIF = variantes[len(variantes)-1]
		provincia, ok := provinciaDeCP(cp)
		if !ok {
			return FacturaeParte{}, fmt.Errorf("el código postal de %s (%q) no es español de cinco cifras", quien, cp)
		}
		dir.CP, dir.Poblacion, dir.Provincia = cp, validation.TruncateRunes(ciudad, 50), provincia
		titular.EnEspana = dir
	default:
		if dominio.MiembroUE(pais) {
			p.Residencia = "U"
		}
		// Fuera de España, el NIF-IVA con su prefijo (Grecia usa EL).
		p.NIF = variantes[0]
		if p.Residencia == "U" && len(variantes) == 1 {
			p.NIF = dominio.PrefijoIVA(pais) + p.NIF
		}
		dir.CPPoblacion = validation.TruncateRunes(strings.TrimSpace(cp+" "+ciudad), 50)
		dir.Provincia = validation.TruncateRunes(Primero(ciudad, pais), 20)
		titular.Extranjero = dir
	}
	if fisica {
		p.TipoPersona = "F"
		// ponytail: nombre, primer apellido y el resto como segundo. Un nombre
		// compuesto («José Luis») se parte mal; la ficha no separa los apellidos.
		partes := strings.Fields(nombre)
		titular.Nombre = validation.TruncateRunes(partes[0], 40)
		titular.Apellido1 = titular.Nombre
		if len(partes) > 1 {
			titular.Apellido1 = validation.TruncateRunes(partes[1], 40)
			titular.Apellido2 = validation.TruncateRunes(strings.Join(partes[2:], " "), 40)
		}
		p.Fisica = titular
	} else {
		titular.Razon = validation.TruncateRunes(nombre, 80)
		p.Juridica = titular
	}
	return p, nil
}

// ISO2 pasa el código de país de Facturae (ISO 3166-1 alfa-3) al de la ficha.
func ISO2(alfa3 string) string {
	alfa3 = strings.ToUpper(strings.TrimSpace(alfa3))
	for a2, a3 := range paisesISO3 {
		if a3 == alfa3 {
			return a2
		}
	}
	return ""
}

var paisesISO3 = func() map[string]string {
	pares := strings.Fields(`AD AND AE ARE AL ALB AR ARG AT AUT AU AUS BA BIH BE BEL BG BGR BR BRA CA CAN
		CH CHE CL CHL CN CHN CO COL CY CYP CZ CZE DE DEU DK DNK DZ DZA EC ECU EE EST EG EGY ES ESP FI FIN
		FR FRA GB GBR GI GIB GR GRC HR HRV HU HUN IE IRL IL ISR IN IND IS ISL IT ITA JP JPN KR KOR LI LIE
		LT LTU LU LUX LV LVA MA MAR MC MCO MD MDA ME MNE MK MKD MT MLT MX MEX NL NLD NO NOR NZ NZL PE PER
		PL POL PT PRT RO ROU RS SRB RU RUS SA SAU SE SWE SG SGP SI SVN SK SVK SM SMR TN TUN TR TUR UA UKR
		US USA UY URY VA VAT VE VEN ZA ZAF`)
	m := make(map[string]string, len(pares)/2)
	for i := 0; i+1 < len(pares); i += 2 {
		m[pares[i]] = pares[i+1]
	}
	return m
}()

// provinciaDeCP: las dos primeras cifras del código postal español son la provincia.
func provinciaDeCP(cp string) (string, bool) {
	if len(cp) != 5 || strings.Trim(cp, "0123456789") != "" {
		return "", false
	}
	var n int
	_, _ = fmt.Sscanf(cp[:2], "%d", &n)
	if n < 1 || n > len(provincias) {
		return "", false
	}
	return provincias[n-1], true
}

var provincias = []string{
	"Araba/Álava", "Albacete", "Alacant/Alicante", "Almería", "Ávila", "Badajoz", "Illes Balears", "Barcelona",
	"Burgos", "Cáceres", "Cádiz", "Castelló/Castellón", "Ciudad Real", "Córdoba", "A Coruña", "Cuenca", "Girona",
	"Granada", "Guadalajara", "Gipuzkoa", "Huelva", "Huesca", "Jaén", "León", "Lleida", "La Rioja", "Lugo",
	"Madrid", "Málaga", "Murcia", "Navarra", "Ourense", "Asturias", "Palencia", "Las Palmas", "Pontevedra",
	"Salamanca", "S.C. Tenerife", "Cantabria", "Segovia", "Sevilla", "Soria", "Tarragona", "Teruel", "Toledo",
	"València/Valencia", "Valladolid", "Bizkaia", "Zamora", "Zaragoza", "Ceuta", "Melilla",
}

// Los tres centros son una unidad: un DIR3 parcial se rechaza, nunca se trunca.
var dir3Re = regexp.MustCompile(`^[A-Z][A-Z0-9]{8}$`)

func centrosFacturae(p *FacturaeParte, receptor Parte) error {
	claves := []string{"dir3_contable", "dir3_gestor", "dir3_tramitadora"}
	codigos := make([]string, len(claves))
	alguno := false
	for i := range claves {
		codigos[i] = strings.ToUpper(strings.TrimSpace(receptor.DIR3[i]))
		alguno = alguno || codigos[i] != ""
	}
	if !alguno {
		return nil
	}
	if p.Juridica == nil || p.Juridica.EnEspana == nil {
		return errors.New("DIR3 requiere un receptor jurídico con dirección española")
	}
	p.Centros = &FacturaeCentros{}
	for i, codigo := range codigos {
		if !dir3Re.MatchString(codigo) {
			return fmt.Errorf("completa los tres códigos DIR3 (9 caracteres): %s", claves[i])
		}
		p.Centros.Centros = append(p.Centros.Centros, FacturaeCentro{Codigo: codigo, Rol: fmt.Sprintf("%02d", i+1), Direccion: p.Juridica.EnEspana})
	}
	return nil
}

// La cabecera conserva el redondeo por tipo de IVA del dominio. Las líneas
// reparten los céntimos por mayores restos: nunca aparece una base negativa
// cuando muchas líneas pequeñas acumulan más de un céntimo de redondeo.
func detalleFacturae(lineas []dominio.LineaFactura, base, iva int64) ([]FacturaeLinea, []FacturaeImpuesto, error) {
	var total int64
	for _, l := range lineas {
		if l.Importe < 0 || l.Importe > math.MaxInt64-total || !slices.Contains(dominio.TiposIVA, l.Tipo) {
			return nil, nil, errors.New("Facturae requiere líneas con importe no negativo y tipo de IVA válido")
		}
		total += l.Importe
	}
	grupos := dominio.Desglose(lineas)
	var bases, cuotas int64
	impuestos := make([]FacturaeImpuesto, 0, len(grupos))
	for _, g := range grupos {
		bases += g.Base
		cuotas += g.Cuota
		impuestos = append(impuestos, FacturaeImpuesto{Tipo: "01", Pct: fmt.Sprintf("%d.00", g.Tipo), Base: FacturaeImporte{importe(g.Base)}, Cuota: FacturaeImporte{importe(g.Cuota)}})
	}
	if bases != base || cuotas != iva {
		return nil, nil, errors.New("la cabecera de la factura no cuadra con sus líneas")
	}
	basesPorLinea := basesLineasFacturae(lineas, grupos)
	detalle := make([]FacturaeLinea, 0, len(lineas))
	for i, l := range lineas {
		cantidad, ok := new(big.Rat).SetString(l.Cantidad)
		if !ok || cantidad.Sign() <= 0 {
			return nil, nil, errors.New("Facturae requiere líneas con cantidad positiva e importe no negativo")
		}
		b := basesPorLinea[i]
		precio := new(big.Rat).Quo(big.NewRat(b, 100), cantidad).FloatString(8)
		impuesto := FacturaeImpuesto{Tipo: "01", Pct: fmt.Sprintf("%d.00", l.Tipo), Base: FacturaeImporte{importe(b)}, Cuota: FacturaeImporte{importe(l.Importe - b)}}
		detalle = append(detalle, FacturaeLinea{Descripcion: validation.TruncateRunes(l.Descripcion, 2500), Cantidad: l.Cantidad, Unidad: "01", Precio: precio, Coste: importe(b), Bruto: importe(b), Impuestos: []FacturaeImpuesto{impuesto}})
	}
	return detalle, impuestos, nil
}

// Parte del suelo de cada base exacta y asigna los céntimos restantes a las
// fracciones mayores del grupo. Un empate mantiene el orden de la factura;
// cada base queda entre cero y el importe de su línea, incluido el IVA al 0 %.
func basesLineasFacturae(lineas []dominio.LineaFactura, grupos []dominio.GrupoIVA) []int64 {
	bases := make([]int64, len(lineas))
	restos := make([]int64, len(lineas))
	indices := map[int64][]int{}
	for i, l := range lineas {
		numerador := new(big.Int).Mul(big.NewInt(l.Importe), big.NewInt(100))
		cociente, resto := new(big.Int), new(big.Int)
		cociente.QuoRem(numerador, big.NewInt(100+l.Tipo), resto)
		bases[i], restos[i] = cociente.Int64(), resto.Int64()
		indices[l.Tipo] = append(indices[l.Tipo], i)
	}
	for _, g := range grupos {
		orden := indices[g.Tipo]
		pendiente := g.Base
		for _, i := range orden {
			pendiente -= bases[i]
		}
		sort.SliceStable(orden, func(i, j int) bool { return restos[orden[i]] > restos[orden[j]] })
		for _, i := range orden {
			if pendiente == 0 {
				break
			}
			bases[i]++
			pendiente--
		}
	}
	return bases
}
