// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package verifactu

import (
	"encoding/xml"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"
)

// Espacios de nombres de los esquemas de la AEAT (servicio web v1.0.3).
const (
	nsSuministroLR          = "https://www2.agenciatributaria.gob.es/static_files/common/internet/dep/aplicaciones/es/aeat/tike/cont/ws/SuministroLR.xsd"
	nsSuministroInformacion = "https://www2.agenciatributaria.gob.es/static_files/common/internet/dep/aplicaciones/es/aeat/tike/cont/ws/SuministroInformacion.xsd"
	nsSOAP                  = "http://schemas.xmlsoap.org/soap/envelope/"
)

// IDFactura identifica una factura ante la AEAT: NIF del emisor, serie y número,
// y fecha de expedición.
type IDFactura struct {
	NIF      string
	NumSerie string
	Fecha    time.Time
}

// Parte es el emisor o el destinatario. Un NIF español va en NIF; una empresa de
// fuera, en CodigoPais + TipoID + ID («02» es el NIF-IVA europeo, «04» el
// documento oficial de su país).
type Parte struct {
	Nombre     string
	NIF        string
	CodigoPais string
	TipoID     string
	ID         string
}

// Sistema es el bloque SistemaInformatico: quién fabrica el programa, qué versión
// genera el registro y en qué instalación.
type Sistema struct {
	Version           string
	NumeroInstalacion string
}

// El productor y el sistema no cambian de una instalación a otra: son los de la
// declaración responsable.
const (
	ProductorNombre    = "Sytue Projects, S.L."
	ProductorNIF       = "B88729900"
	ProductorDireccion = "Calle Atenas, 10, 28822 Coslada (Madrid), España"
	ProductorWeb       = "https://sytue.eu"
	ProductorCorreo    = "soporte@sytue.eu"
	NombreSistema      = "Sytue ERP"
	IDSistema          = "SE"
)

// La declaración responsable la suscribe la entidad productora, versión a versión.
// Sin fecha no está suscrita, y VERI*FACTU solo se puede activar contra el entorno
// de pruebas de la AEAT.
const (
	DeclaracionFecha = ""
	DeclaracionLugar = "Coslada (Madrid), España"
)

// Anterior es el registro inmediatamente anterior de la cadena. Nulo en el primero.
type Anterior struct {
	IDFactura
	Huella string
}

// Detalle es una línea del desglose. Sujeta: Calificacion S1 con Tipo (centésimas
// de punto) y Cuota, o S2 (inversión del sujeto pasivo) sin ellos. Exenta:
// Exencion (E1…E8). No sujeta: Calificacion N1 o N2.
type Detalle struct {
	Calificacion string
	Exencion     string
	Tipo         int64
	Base         int64
	Cuota        int64
}

// Alta es un registro de facturación de alta.
type Alta struct {
	// Ref viaja como RefExterna y vuelve en la respuesta: casa cada línea con su
	// registro sin depender del orden.
	Ref          string
	Emisor       Parte
	Factura      IDFactura
	TipoFactura  string
	Rectificadas []IDFactura
	Descripcion  string
	Destinatario *Parte
	Desglose     []Detalle
	CuotaTotal   int64
	ImporteTotal int64
	// Subsanacion corrige un registro ya enviado. Con PorRechazo, uno que la AEAT
	// rechazó y por tanto no tiene (RechazoPrevio=X).
	Subsanacion bool
	PorRechazo  bool
	Anterior    *Anterior
	Sistema     Sistema
	Generado    time.Time
}

// Anulacion es un registro de facturación de anulación.
type Anulacion struct {
	Ref     string
	Factura IDFactura
	// SinRegistroPrevio: la AEAT no tiene el alta (se rechazó). PorRechazo: repite
	// una anulación rechazada.
	SinRegistroPrevio bool
	PorRechazo        bool
	Anterior          *Anterior
	Sistema           Sistema
	Generado          time.Time
}

var (
	nifRe       = regexp.MustCompile(`^[A-Z0-9]{9}$`)
	tiposFac    = []string{"F1", "F2", "R1", "R2", "R3", "R4", "R5"}
	calificados = []string{"S1", "S2", "N1", "N2"}
	exenciones  = []string{"E1", "E2", "E3", "E4", "E5", "E6", "E7", "E8"}
	tiposID     = []string{"02", "03", "04", "05", "06", "07"}
)

// ErrDatos agrupa lo que no cumple el diseño de registro. El mensaje dice qué, en
// palabras de quien factura.
var ErrDatos = errors.New("VERI*FACTU")

func errDatos(formato string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrDatos, fmt.Sprintf(formato, args...))
}

// tiposIVA son los tipos de IVA que ha habido desde 2023, en centésimas de punto.
var tiposIVA = []int64{2100, 1000, 750, 500, 400, 200}

// TipoIVA deduce el tipo de IVA de una base y su cuota. La AEAT admite 10
// céntimos de diferencia entre la cuota y base × tipo, por redondeos; más allá, la
// cuota no corresponde a ningún tipo y el registro se rechazaría.
func TipoIVA(base, cuota int64) (int64, bool) {
	mejor, dif := int64(0), int64(-1)
	for _, t := range tiposIVA {
		esperada := redondear(base*t, 10000)
		d := max(esperada-cuota, cuota-esperada)
		if dif < 0 || d < dif {
			mejor, dif = t, d
		}
	}
	return mejor, dif >= 0 && dif <= 10
}

// redondear divide redondeando al entero más cercano, también con negativos.
func redondear(n, d int64) int64 {
	if n < 0 {
		return -((-n + d/2) / d)
	}
	return (n + d/2) / d
}

// validarID comprueba lo que la AEAT rechaza de un identificador de factura.
func validarID(id IDFactura) error {
	if !nifRe.MatchString(id.NIF) {
		return errDatos("el NIF %q no tiene 9 letras o cifras", id.NIF)
	}
	if n := len(id.NumSerie); n == 0 || n > 60 {
		return errDatos("el número de factura tiene que tener entre 1 y 60 caracteres")
	}
	for _, r := range id.NumSerie {
		if r < 32 || r > 126 {
			return errDatos("el número de factura solo admite letras sin acentos, cifras y signos básicos")
		}
	}
	if id.Fecha.IsZero() {
		return errDatos("la factura no tiene fecha")
	}
	return nil
}

func validarComun(ref string, sis Sistema, generado time.Time) error {
	switch {
	case ref == "" || len(ref) > 60:
		return errDatos("falta la referencia interna del registro")
	case sis.Version == "" || len(sis.Version) > 50:
		return errDatos("falta la versión del sistema")
	case sis.NumeroInstalacion == "" || len(sis.NumeroInstalacion) > 100:
		return errDatos("falta el número de instalación")
	case generado.IsZero():
		return errDatos("falta el momento de generación")
	}
	return nil
}

// Validar comprueba el alta contra el diseño de registro antes de generarla: lo
// que no pasa aquí lo rechazaría la AEAT, y un registro generado ya no se borra.
func (a Alta) Validar() error {
	if err := validarID(a.Factura); err != nil {
		return err
	}
	if a.Emisor.NIF != a.Factura.NIF {
		return errDatos("el NIF del emisor y el de la factura no coinciden")
	}
	if a.Factura.Fecha.Format(time.DateOnly) > a.Generado.In(madrid).Format(time.DateOnly) {
		return errDatos("la fecha de la factura no puede ser posterior a hoy")
	}
	if n := len([]rune(strings.TrimSpace(a.Emisor.Nombre))); n == 0 || n > 120 {
		return errDatos("el nombre de la organización tiene que tener entre 1 y 120 caracteres")
	}
	if !slices.Contains(tiposFac, a.TipoFactura) {
		return errDatos("tipo de factura %q desconocido", a.TipoFactura)
	}
	if strings.HasPrefix(a.TipoFactura, "R") != (len(a.Rectificadas) > 0) {
		return errDatos("una rectificativa, y solo una rectificativa, dice qué factura rectifica")
	}
	for _, r := range a.Rectificadas {
		if err := validarID(r); err != nil {
			return err
		}
	}
	if a.PorRechazo && !a.Subsanacion {
		return errDatos("un alta por rechazo es siempre una subsanación")
	}
	if n := len([]rune(strings.TrimSpace(a.Descripcion))); n == 0 {
		return errDatos("falta la descripción de la operación")
	}
	if err := a.validarDestinatario(); err != nil {
		return err
	}
	if err := a.validarDesglose(); err != nil {
		return err
	}
	return validarComun(a.Ref, a.Sistema, a.Generado)
}

func (a Alta) validarDestinatario() error {
	if a.TipoFactura == "F2" || a.TipoFactura == "R5" {
		if a.Destinatario != nil {
			return errDatos("una factura simplificada no lleva destinatario")
		}
		return nil
	}
	d := a.Destinatario
	if d == nil {
		return errDatos("la factura necesita el nombre y el NIF del cliente")
	}
	if n := len([]rune(strings.TrimSpace(d.Nombre))); n == 0 || n > 120 {
		return errDatos("el nombre del cliente tiene que tener entre 1 y 120 caracteres")
	}
	if d.NIF != "" {
		if !nifRe.MatchString(d.NIF) {
			return errDatos("el NIF del cliente %q no tiene 9 letras o cifras", d.NIF)
		}
		return nil
	}
	switch {
	case d.ID == "" || len(d.ID) > 20:
		return errDatos("el cliente no tiene NIF ni número de identificación de su país")
	case !slices.Contains(tiposID, d.TipoID):
		return errDatos("tipo de identificación del cliente %q desconocido", d.TipoID)
	case d.TipoID != "02" && len(d.CodigoPais) != 2:
		return errDatos("falta el país del cliente")
	case d.CodigoPais == "ES" && d.TipoID != "03" && d.TipoID != "07":
		return errDatos("un cliente de España necesita su NIF")
	}
	return nil
}

func (a Alta) validarDesglose() error {
	if len(a.Desglose) == 0 || len(a.Desglose) > 12 {
		return errDatos("el desglose tiene que tener entre 1 y 12 líneas")
	}
	var cuotas, bases int64
	for _, d := range a.Desglose {
		switch {
		case d.Exencion != "":
			if !slices.Contains(exenciones, d.Exencion) || d.Calificacion != "" || d.Cuota != 0 || d.Tipo != 0 {
				return errDatos("una operación exenta no lleva IVA")
			}
		case !slices.Contains(calificados, d.Calificacion):
			return errDatos("calificación de la operación %q desconocida", d.Calificacion)
		case d.Calificacion != "S1" && (d.Cuota != 0 || d.Tipo != 0):
			return errDatos("una operación no sujeta o con inversión del sujeto pasivo no lleva IVA en la factura")
		case d.Calificacion == "S1" && d.Tipo <= 0:
			return errDatos("una operación sujeta lleva su tipo de IVA")
		}
		cuotas += d.Cuota
		bases += d.Base
	}
	if cuotas != a.CuotaTotal {
		return errDatos("la cuota total no es la suma del desglose")
	}
	if bases+cuotas != a.ImporteTotal {
		return errDatos("el importe total no es la base más el IVA")
	}
	return nil
}

// Huella calcula la huella del alta.
func (a Alta) Huella() string {
	return HuellaAlta(a.Factura.NIF, a.Factura.NumSerie, Fecha(a.Factura.Fecha), a.TipoFactura,
		Importe(a.CuotaTotal), Importe(a.ImporteTotal), huellaDe(a.Anterior), FechaHora(a.Generado))
}

// Validar comprueba la anulación.
func (a Anulacion) Validar() error {
	if err := validarID(a.Factura); err != nil {
		return err
	}
	if a.SinRegistroPrevio && a.PorRechazo {
		return errDatos("una anulación sin alta previa no puede repetir una anulación rechazada")
	}
	return validarComun(a.Ref, a.Sistema, a.Generado)
}

// Huella calcula la huella de la anulación.
func (a Anulacion) Huella() string {
	return HuellaAnulacion(a.Factura.NIF, a.Factura.NumSerie, Fecha(a.Factura.Fecha), huellaDe(a.Anterior), FechaHora(a.Generado))
}

func huellaDe(a *Anterior) string {
	if a == nil {
		return ""
	}
	return a.Huella
}

// escritor arma XML en el orden exacto de las secuencias del XSD. A mano y no con
// encoding/xml: el orden es lo que valida la AEAT, y aquí se lee de arriba abajo.
type escritor struct{ b strings.Builder }

func (e *escritor) abrir(nombre string)  { e.b.WriteString("<sum1:" + nombre + ">") }
func (e *escritor) cerrar(nombre string) { e.b.WriteString("</sum1:" + nombre + ">") }

func (e *escritor) campo(nombre, valor string) {
	e.abrir(nombre)
	_ = xml.EscapeText(&e.b, []byte(strings.TrimSpace(valor)))
	e.cerrar(nombre)
}

func (e *escritor) si(cond bool, nombre, valor string) {
	if cond {
		e.campo(nombre, valor)
	}
}

func (e *escritor) id(emisor, serie, fecha string, f IDFactura) {
	e.campo(emisor, f.NIF)
	e.campo(serie, f.NumSerie)
	e.campo(fecha, Fecha(f.Fecha))
}

func (e *escritor) encadenamiento(a *Anterior) {
	e.abrir("Encadenamiento")
	if a == nil {
		e.campo("PrimerRegistro", "S")
	} else {
		e.abrir("RegistroAnterior")
		e.id("IDEmisorFactura", "NumSerieFactura", "FechaExpedicionFactura", a.IDFactura)
		e.campo("Huella", a.Huella)
		e.cerrar("RegistroAnterior")
	}
	e.cerrar("Encadenamiento")
}

// sistema escribe el bloque SistemaInformatico. Sytue ERP solo funciona como
// VERI*FACTU y, como producto, lleva la facturación de muchas organizaciones
// (TipoUsoPosibleMultiOT). Cada organización es una facturación propia con su
// número de instalación, así que el indicador de varias va a «N»: la AEAT lo
// calcula por cliente del SaaS, no por el SaaS entero (FAQ para desarrolladores, 4).
func (e *escritor) sistema(s Sistema) {
	e.abrir("SistemaInformatico")
	e.campo("NombreRazon", ProductorNombre)
	e.campo("NIF", ProductorNIF)
	e.campo("NombreSistemaInformatico", NombreSistema)
	e.campo("IdSistemaInformatico", IDSistema)
	e.campo("Version", s.Version)
	e.campo("NumeroInstalacion", s.NumeroInstalacion)
	e.campo("TipoUsoPosibleSoloVerifactu", "S")
	e.campo("TipoUsoPosibleMultiOT", "S")
	e.campo("IndicadorMultiplesOT", "N")
	e.cerrar("SistemaInformatico")
}

func (e *escritor) parte(p Parte) {
	e.campo("NombreRazon", truncar(p.Nombre, 120))
	if p.NIF != "" {
		e.campo("NIF", p.NIF)
		return
	}
	e.abrir("IDOtro")
	e.si(p.CodigoPais != "", "CodigoPais", p.CodigoPais)
	e.campo("IDType", p.TipoID)
	e.campo("ID", p.ID)
	e.cerrar("IDOtro")
}

func (e *escritor) detalle(d Detalle) {
	e.abrir("DetalleDesglose")
	e.campo("Impuesto", "01")
	e.campo("ClaveRegimen", "01")
	if d.Exencion != "" {
		e.campo("OperacionExenta", d.Exencion)
	} else {
		e.campo("CalificacionOperacion", d.Calificacion)
	}
	sujeta := d.Calificacion == "S1"
	e.si(sujeta, "TipoImpositivo", Porcentaje(d.Tipo))
	e.campo("BaseImponibleOimporteNoSujeto", Importe(d.Base))
	e.si(sujeta, "CuotaRepercutida", Importe(d.Cuota))
	e.cerrar("DetalleDesglose")
}

// XML valida el alta y la escribe como <sum1:RegistroAlta>, con su espacio de
// nombres declarado para que el fragmento se guarde y se lea solo.
func (a Alta) XML() (string, error) {
	if err := a.Validar(); err != nil {
		return "", err
	}
	var e escritor
	e.b.WriteString(`<sum1:RegistroAlta xmlns:sum1="` + nsSuministroInformacion + `">`)
	e.campo("IDVersion", "1.0")
	e.abrir("IDFactura")
	e.id("IDEmisorFactura", "NumSerieFactura", "FechaExpedicionFactura", a.Factura)
	e.cerrar("IDFactura")
	e.campo("RefExterna", a.Ref)
	e.campo("NombreRazonEmisor", truncar(a.Emisor.Nombre, 120))
	e.si(a.Subsanacion, "Subsanacion", "S")
	e.si(a.PorRechazo, "RechazoPrevio", "X")
	e.campo("TipoFactura", a.TipoFactura)
	if len(a.Rectificadas) > 0 {
		e.campo("TipoRectificativa", "I")
		e.abrir("FacturasRectificadas")
		for _, r := range a.Rectificadas {
			e.abrir("IDFacturaRectificada")
			e.id("IDEmisorFactura", "NumSerieFactura", "FechaExpedicionFactura", r)
			e.cerrar("IDFacturaRectificada")
		}
		e.cerrar("FacturasRectificadas")
	}
	e.campo("DescripcionOperacion", truncar(a.Descripcion, 500))
	if a.Destinatario != nil {
		e.abrir("Destinatarios")
		e.abrir("IDDestinatario")
		e.parte(*a.Destinatario)
		e.cerrar("IDDestinatario")
		e.cerrar("Destinatarios")
	}
	e.abrir("Desglose")
	for _, d := range a.Desglose {
		e.detalle(d)
	}
	e.cerrar("Desglose")
	e.campo("CuotaTotal", Importe(a.CuotaTotal))
	e.campo("ImporteTotal", Importe(a.ImporteTotal))
	e.encadenamiento(a.Anterior)
	e.sistema(a.Sistema)
	e.campo("FechaHoraHusoGenRegistro", FechaHora(a.Generado))
	e.campo("TipoHuella", "01")
	e.campo("Huella", a.Huella())
	e.b.WriteString("</sum1:RegistroAlta>")
	return e.b.String(), nil
}

// XML valida la anulación y la escribe como <sum1:RegistroAnulacion>.
func (a Anulacion) XML() (string, error) {
	if err := a.Validar(); err != nil {
		return "", err
	}
	var e escritor
	e.b.WriteString(`<sum1:RegistroAnulacion xmlns:sum1="` + nsSuministroInformacion + `">`)
	e.campo("IDVersion", "1.0")
	e.abrir("IDFactura")
	e.id("IDEmisorFacturaAnulada", "NumSerieFacturaAnulada", "FechaExpedicionFacturaAnulada", a.Factura)
	e.cerrar("IDFactura")
	e.campo("RefExterna", a.Ref)
	e.si(a.SinRegistroPrevio, "SinRegistroPrevio", "S")
	e.si(a.PorRechazo, "RechazoPrevio", "S")
	e.encadenamiento(a.Anterior)
	e.sistema(a.Sistema)
	e.campo("FechaHoraHusoGenRegistro", FechaHora(a.Generado))
	e.campo("TipoHuella", "01")
	e.campo("Huella", a.Huella())
	e.b.WriteString("</sum1:RegistroAnulacion>")
	return e.b.String(), nil
}

func truncar(s string, n int) string {
	r := []rune(strings.TrimSpace(s))
	if len(r) > n {
		r = r[:n]
	}
	return strings.TrimSpace(string(r))
}
