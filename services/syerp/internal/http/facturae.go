// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package syerphttp

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"math"
	"math/big"
	"net/http"
	"regexp"
	"slices"
	"sort"
	"strings"

	"github.com/go-chi/chi/v5"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/validation"
	"git.sytue.eu/sytue/one/services/syerp/internal/domain"
	"git.sytue.eu/sytue/one/services/syerp/internal/store"
)

// Facturae 3.2.2: la factura electrónica española. Es la que piden FACe y los
// puntos de entrada de las administraciones, y la que importan a3, Sage,
// Holded y casi cualquier programa de contabilidad español.
//
// Sale SIN FIRMAR. La firma XAdES es del emisor, con su certificado, y Sytue no
// lo custodia: el fichero se firma con el programa de la gestoría antes de
// subirlo. Los códigos DIR3 del receptor viajan en AdministrativeCentres.
//
// ponytail: IVA como único impuesto y sin
// rectificativas; es lo que las facturas de este ERP pueden expresar hoy. IGIC
// (Canarias), retenciones y el bloque Corrective llegan cuando el documento
// tenga líneas, impuestos y factura de origen.

const facturaeNS = "http://www.facturae.gob.es/formato/Versiones/Facturaev3_2_2.xml"

type facturaeImporte struct {
	Total string `xml:"TotalAmount"`
}

type facturaeDireccion struct {
	Calle string `xml:"Address"`
	// En España, CP y población por separado y la provincia sale del CP; fuera,
	// juntos.
	CP          string `xml:"PostCode,omitempty"`
	Poblacion   string `xml:"Town,omitempty"`
	CPPoblacion string `xml:"PostCodeAndTown,omitempty"`
	Provincia   string `xml:"Province"`
	Pais        string `xml:"CountryCode"`
}

type facturaeTitular struct {
	Nombre       string             `xml:"Name,omitempty"`
	Apellido1    string             `xml:"FirstSurname,omitempty"`
	Apellido2    string             `xml:"SecondSurname,omitempty"`
	Razon        string             `xml:"CorporateName,omitempty"`
	EnEspana     *facturaeDireccion `xml:"AddressInSpain,omitempty"`
	Extranjero   *facturaeDireccion `xml:"OverseasAddress,omitempty"`
	CorreoDentro string             `xml:"ContactDetails>ElectronicMail,omitempty"`
}

type facturaeParte struct {
	TipoPersona string           `xml:"TaxIdentification>PersonTypeCode"`
	Residencia  string           `xml:"TaxIdentification>ResidenceTypeCode"`
	NIF         string           `xml:"TaxIdentification>TaxIdentificationNumber"`
	Centros     *facturaeCentros `xml:"AdministrativeCentres,omitempty"`
	Juridica    *facturaeTitular `xml:"LegalEntity,omitempty"`
	Fisica      *facturaeTitular `xml:"Individual,omitempty"`
}

type facturaeCentros struct {
	Centros []facturaeCentro `xml:"AdministrativeCentre"`
}

type facturaeCentro struct {
	Codigo    string             `xml:"CentreCode"`
	Rol       string             `xml:"RoleTypeCode"`
	Direccion *facturaeDireccion `xml:"AddressInSpain"`
}

type facturaeImpuesto struct {
	Tipo  string          `xml:"TaxTypeCode"`
	Pct   string          `xml:"TaxRate"`
	Base  facturaeImporte `xml:"TaxableBase"`
	Cuota facturaeImporte `xml:"TaxAmount"`
}

// Los campos van en el orden de las secuencias del XSD: encoding/xml los
// escribe en el orden en que se declaran, y un elemento fuera de sitio invalida
// el fichero entero.
type facturaeDoc struct {
	XMLName xml.Name `xml:"fe:Facturae"`
	NS      string   `xml:"xmlns:fe,attr"`

	Version    string          `xml:"FileHeader>SchemaVersion"`
	Modalidad  string          `xml:"FileHeader>Modality"`
	TipoEmisor string          `xml:"FileHeader>InvoiceIssuerType"`
	Lote       string          `xml:"FileHeader>Batch>BatchIdentifier"`
	Cuantas    int             `xml:"FileHeader>Batch>InvoicesCount"`
	LoteTotal  facturaeImporte `xml:"FileHeader>Batch>TotalInvoicesAmount"`
	LotePend   facturaeImporte `xml:"FileHeader>Batch>TotalOutstandingAmount"`
	LoteExig   facturaeImporte `xml:"FileHeader>Batch>TotalExecutableAmount"`
	LoteMoneda string          `xml:"FileHeader>Batch>InvoiceCurrencyCode"`
	Vendedor   facturaeParte   `xml:"Parties>SellerParty"`
	Comprador  facturaeParte   `xml:"Parties>BuyerParty"`
	Factura    facturaeFactura `xml:"Invoices>Invoice"`
}

type facturaeFactura struct {
	Numero   string `xml:"InvoiceHeader>InvoiceNumber"`
	TipoDoc  string `xml:"InvoiceHeader>InvoiceDocumentType"`
	Clase    string `xml:"InvoiceHeader>InvoiceClass"`
	Fecha    string `xml:"InvoiceIssueData>IssueDate"`
	Moneda   string `xml:"InvoiceIssueData>InvoiceCurrencyCode"`
	MonedaTx string `xml:"InvoiceIssueData>TaxCurrencyCode"`
	Idioma   string `xml:"InvoiceIssueData>LanguageName"`

	Impuestos []facturaeImpuesto `xml:"TaxesOutputs>Tax"`

	Bruto     string          `xml:"InvoiceTotals>TotalGrossAmount"`
	AntesImp  string          `xml:"InvoiceTotals>TotalGrossAmountBeforeTaxes"`
	TotalImp  string          `xml:"InvoiceTotals>TotalTaxOutputs"`
	Retenido  string          `xml:"InvoiceTotals>TotalTaxesWithheld"`
	Total     string          `xml:"InvoiceTotals>InvoiceTotal"`
	Pendiente string          `xml:"InvoiceTotals>TotalOutstandingAmount"`
	Exigible  string          `xml:"InvoiceTotals>TotalExecutableAmount"`
	Lineas    []facturaeLinea `xml:"Items>InvoiceLine"`
	// Adicional lleva la URL del QR tributario: en una factura electrónica va en
	// la información estructurada (Orden HAC/1177/2024, art. 20.2).
	Adicional string `xml:"AdditionalData>InvoiceAdditionalInformation,omitempty"`
}

type facturaeLinea struct {
	Descripcion string             `xml:"ItemDescription"`
	Cantidad    string             `xml:"Quantity"`
	Unidad      string             `xml:"UnitOfMeasure"`
	Precio      string             `xml:"UnitPriceWithoutTax"`
	Coste       string             `xml:"TotalCost"`
	Bruto       string             `xml:"GrossAmount"`
	Impuestos   []facturaeImpuesto `xml:"TaxesOutputs>Tax"`
}

// Facturae — GET /api/v1/{modelo}/{id}/facturae — la factura emitida como
// Facturae 3.2.2 sin firmar.
func (h *Handlers) Facturae(w http.ResponseWriter, r *http.Request) {
	m := h.modeloDe(w, r)
	if m == nil {
		return
	}
	if m.Contable == nil || !m.Contable.Cobro {
		httpx.Fail(w, r, errNoContable.WithMessage("Only issued invoices can be exported as Facturae."))
		return
	}
	reg, err := h.store.Obtener(r.Context(), m.Clave, chi.URLParam(r, "id"))
	if err != nil {
		httpx.Fail(w, r, errorDeLectura(err))
		return
	}
	if cadena(reg.Data["tipo_factura"]) == "F2" {
		httpx.Fail(w, r, httpx.ErrUnprocessable.WithMessage("una factura simplificada no identifica al comprador"))
		return
	}
	lineas, err := h.store.LineasDeFactura(r.Context(), reg.ID)
	if err != nil {
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
	personaFisica := false
	if pais, ok := h.obligaciones.Pais(org.Pais); ok {
		if forma, ok := pais.Forma(org.Forma); ok {
			personaFisica = forma.Tipo == "persona_fisica"
		}
	}
	doc, err := facturaeDe(org, personaFisica, reg, m.Contable.CampoBase, m.Contable.CampoIVA, cliente, lineas...)
	if err != nil {
		httpx.Fail(w, r, httpx.ErrUnprocessable.WithMessage(err.Error()))
		return
	}
	_, urlQR, err := h.registrosDeFactura(r.Context(), reg)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if urlQR != "" {
		doc.Factura.Adicional = "VERI*FACTU. QR tributario: " + urlQR
	}
	cuerpo, err := xml.MarshalIndent(doc, "", "  ")
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="facturae-%s.xml"`, strings.ReplaceAll(doc.Factura.Numero, "/", "-")))
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(xml.Header))
	_, _ = w.Write(cuerpo)
}

// efacturaAdmitida dice si la factura puede salir como factura electrónica
// (Facturae o UBL). Las dos salen hoy con un comprador identificado y un solo
// tipo de IVA: una simplificada no tiene comprador, y una factura con líneas de
// varios tipos saldría con un tipo medio que no existe.
//
// ponytail: se rechaza en vez de sacar una línea por línea de factura; relevo:
// líneas y impuestos por tipo en las dos plantillas.
func (h *Handlers) efacturaAdmitida(ctx context.Context, reg store.Registro) error {
	if cadena(reg.Data["tipo_factura"]) == "F2" {
		return httpx.ErrUnprocessable.WithMessage("una factura simplificada no sale como factura electrónica: no identifica al comprador")
	}
	lineas, err := h.store.LineasDeFactura(ctx, reg.ID)
	if err != nil {
		return err
	}
	if len(domain.Desglose(lineas)) > 1 {
		return httpx.ErrUnprocessable.WithMessage("las facturas con líneas de varios tipos de IVA aún no salen como factura electrónica")
	}
	return nil
}

// facturaeDe arma el documento. Devuelve un error legible con lo que falta:
// Facturae no admite partes sin dirección, y descubrirlo en FACe es peor.
func facturaeDe(org store.Organizacion, orgFisica bool, reg store.Registro, campoBase, campoIVA string, cliente store.Registro, lineas ...domain.LineaFactura) (facturaeDoc, error) {
	base, _ := centimos(reg.Data[campoBase])
	iva, _ := centimos(reg.Data[campoIVA])
	if base < 0 || iva < 0 {
		return facturaeDoc{}, errors.New("las rectificativas aún no salen como Facturae: falta la referencia a la factura de origen")
	}
	total := base + iva
	fecha := cadena(reg.Data["fecha"])
	if fecha == "" {
		return facturaeDoc{}, errors.New("la factura no tiene fecha")
	}
	numero := reg.Ref
	if numero == "" {
		numero = reg.ID
	}
	texto := func(k string) string { s, _ := cliente.Data[k].(string); return strings.TrimSpace(s) }

	vendedor, err := parteFacturae("el emisor", org.Nombre, org.NIF, org.Direccion, org.CP, org.Ciudad, org.Pais, "", orgFisica)
	if err != nil {
		return facturaeDoc{}, err
	}
	if cliente.ID == "" {
		return facturaeDoc{}, errors.New("la factura no tiene cliente")
	}
	comprador, err := parteFacturae(cliente.Etiqueta, cliente.Etiqueta, texto("nif"), texto("direccion"), texto("cp"),
		texto("ciudad"), codigoPais(texto("pais"), org.Pais), texto("correo"), texto("tipo") == "persona")
	if err != nil {
		return facturaeDoc{}, err
	}

	pct := "0.00"
	if base != 0 {
		// Dos decimales con redondeo entero: 210 de IVA sobre 1000 de base es 21.00.
		centesimas := (iva*10000 + base/2) / base
		pct = fmt.Sprintf("%d.%02d", centesimas/100, centesimas%100)
	}
	impuestos := []facturaeImpuesto{{Tipo: "01", Pct: pct, Base: facturaeImporte{importe(base)}, Cuota: facturaeImporte{importe(iva)}}}
	concepto := cadena(reg.Data["notas"])
	if strings.TrimSpace(concepto) == "" {
		concepto = numero
	}
	if err := centrosFacturae(&comprador, cliente); err != nil {
		return facturaeDoc{}, err
	}
	detalle := []facturaeLinea{{Descripcion: validation.TruncateRunes(concepto, 2500), Cantidad: "1.0", Unidad: "01", Precio: importe(base), Coste: importe(base), Bruto: importe(base), Impuestos: impuestos}}
	if len(lineas) > 0 {
		var err error
		detalle, impuestos, err = detalleFacturae(lineas, base, iva)
		if err != nil {
			return facturaeDoc{}, err
		}
	}
	pendiente := total
	if reg.Estado == "pagado" {
		pendiente = 0
	}
	return facturaeDoc{
		NS: facturaeNS, Version: "3.2.2", Modalidad: "I", TipoEmisor: "EM",
		Lote: validation.TruncateRunes(vendedor.NIF+numero, 70), Cuantas: 1,
		LoteTotal: facturaeImporte{importe(total)}, LotePend: facturaeImporte{importe(pendiente)},
		LoteExig: facturaeImporte{importe(pendiente)}, LoteMoneda: "EUR",
		Vendedor: vendedor, Comprador: comprador,
		Factura: facturaeFactura{
			Numero: validation.TruncateRunes(numero, 20), TipoDoc: "FC", Clase: "OO",
			Fecha: fecha, Moneda: "EUR", MonedaTx: "EUR", Idioma: "es",
			Impuestos: impuestos,
			Bruto:     importe(base), AntesImp: importe(base), TotalImp: importe(iva), Retenido: "0.00",
			Total: importe(total), Pendiente: importe(pendiente), Exigible: importe(pendiente),
			Lineas: detalle,
		},
	}, nil
}

// parteFacturae arma emisor o receptor. `quien` nombra la parte en el error.
func parteFacturae(quien, nombre, nif, calle, cp, ciudad, pais, correo string, fisica bool) (facturaeParte, error) {
	nombre, calle, cp, ciudad = strings.TrimSpace(nombre), strings.TrimSpace(calle), strings.TrimSpace(cp), strings.TrimSpace(ciudad)
	pais = strings.ToUpper(strings.TrimSpace(pais))
	variantes := domain.VariantesNIF(nif)
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
		return facturaeParte{}, fmt.Errorf("falta %s de %s", faltan[0], quien)
	default:
		return facturaeParte{}, fmt.Errorf("faltan %s y %s de %s", strings.Join(faltan[:len(faltan)-1], ", "), faltan[len(faltan)-1], quien)
	}
	alfa3, ok := paisesISO3[pais]
	if !ok {
		return facturaeParte{}, fmt.Errorf("el país de %s (%q) no tiene código para Facturae", quien, pais)
	}

	p := facturaeParte{TipoPersona: "J", Residencia: "E"}
	dir := &facturaeDireccion{Calle: validation.TruncateRunes(calle, 80), Pais: alfa3}
	titular := &facturaeTitular{CorreoDentro: validation.TruncateRunes(strings.TrimSpace(correo), 60)}
	switch pais {
	case "ES":
		p.Residencia = "R"
		// El NIF español va sin prefijo de país.
		p.NIF = variantes[len(variantes)-1]
		provincia, ok := provinciaDeCP(cp)
		if !ok {
			return facturaeParte{}, fmt.Errorf("el código postal de %s (%q) no es español de cinco cifras", quien, cp)
		}
		dir.CP, dir.Poblacion, dir.Provincia = cp, validation.TruncateRunes(ciudad, 50), provincia
		titular.EnEspana = dir
	default:
		if domain.MiembroUE(pais) {
			p.Residencia = "U"
		}
		// Fuera de España, el NIF-IVA con su prefijo (Grecia usa EL).
		p.NIF = variantes[0]
		if p.Residencia == "U" && len(variantes) == 1 {
			p.NIF = domain.PrefijoIVA(pais) + p.NIF
		}
		dir.CPPoblacion = validation.TruncateRunes(strings.TrimSpace(cp+" "+ciudad), 50)
		dir.Provincia = validation.TruncateRunes(primero(ciudad, pais), 20)
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

// iso2 pasa el código de país de Facturae (ISO 3166-1 alfa-3) al de la ficha.
func iso2(alfa3 string) string {
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

func centrosFacturae(p *facturaeParte, cliente store.Registro) error {
	claves := []string{"dir3_contable", "dir3_gestor", "dir3_tramitadora"}
	codigos := make([]string, len(claves))
	alguno := false
	for i, k := range claves {
		codigos[i] = strings.ToUpper(strings.TrimSpace(cadena(cliente.Data[k])))
		alguno = alguno || codigos[i] != ""
	}
	if !alguno {
		return nil
	}
	if p.Juridica == nil || p.Juridica.EnEspana == nil {
		return errors.New("DIR3 requiere un receptor jurídico con dirección española")
	}
	p.Centros = &facturaeCentros{}
	for i, codigo := range codigos {
		if !dir3Re.MatchString(codigo) {
			return fmt.Errorf("completa los tres códigos DIR3 (9 caracteres): %s", claves[i])
		}
		p.Centros.Centros = append(p.Centros.Centros, facturaeCentro{Codigo: codigo, Rol: fmt.Sprintf("%02d", i+1), Direccion: p.Juridica.EnEspana})
	}
	return nil
}

// La cabecera conserva el redondeo por tipo de IVA del dominio. Las líneas
// reparten los céntimos por mayores restos: nunca aparece una base negativa
// cuando muchas líneas pequeñas acumulan más de un céntimo de redondeo.
func detalleFacturae(lineas []domain.LineaFactura, base, iva int64) ([]facturaeLinea, []facturaeImpuesto, error) {
	var total int64
	for _, l := range lineas {
		if l.Importe < 0 || l.Importe > math.MaxInt64-total || !slices.Contains(domain.TiposIVA, l.Tipo) {
			return nil, nil, errors.New("Facturae requiere líneas con importe no negativo y tipo de IVA válido")
		}
		total += l.Importe
	}
	grupos := domain.Desglose(lineas)
	var bases, cuotas int64
	impuestos := make([]facturaeImpuesto, 0, len(grupos))
	for _, g := range grupos {
		bases += g.Base
		cuotas += g.Cuota
		impuestos = append(impuestos, facturaeImpuesto{Tipo: "01", Pct: fmt.Sprintf("%d.00", g.Tipo), Base: facturaeImporte{importe(g.Base)}, Cuota: facturaeImporte{importe(g.Cuota)}})
	}
	if bases != base || cuotas != iva {
		return nil, nil, errors.New("la cabecera de la factura no cuadra con sus líneas")
	}
	basesPorLinea := basesLineasFacturae(lineas, grupos)
	detalle := make([]facturaeLinea, 0, len(lineas))
	for i, l := range lineas {
		cantidad, ok := new(big.Rat).SetString(l.Cantidad)
		if !ok || cantidad.Sign() <= 0 {
			return nil, nil, errors.New("Facturae requiere líneas con cantidad positiva e importe no negativo")
		}
		b := basesPorLinea[i]
		precio := new(big.Rat).Quo(big.NewRat(b, 100), cantidad).FloatString(8)
		impuesto := facturaeImpuesto{Tipo: "01", Pct: fmt.Sprintf("%d.00", l.Tipo), Base: facturaeImporte{importe(b)}, Cuota: facturaeImporte{importe(l.Importe - b)}}
		detalle = append(detalle, facturaeLinea{Descripcion: validation.TruncateRunes(l.Descripcion, 2500), Cantidad: l.Cantidad, Unidad: "01", Precio: precio, Coste: importe(b), Bruto: importe(b), Impuestos: []facturaeImpuesto{impuesto}})
	}
	return detalle, impuestos, nil
}

// Parte del suelo de cada base exacta y asigna los céntimos restantes a las
// fracciones mayores del grupo. Un empate mantiene el orden de la factura;
// cada base queda entre cero y el importe de su línea, incluido el IVA al 0 %.
func basesLineasFacturae(lineas []domain.LineaFactura, grupos []domain.GrupoIVA) []int64 {
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
