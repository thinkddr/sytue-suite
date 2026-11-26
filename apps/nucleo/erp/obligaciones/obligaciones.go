// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package obligaciones sabe qué debe presentar una organización, cuándo y dónde:
// las obligaciones fiscales, contables, laborales y mercantiles de cada país de
// la Unión, y el calendario que salen de ellas para un perfil concreto.
//
// SON DATO, igual que el catálogo de módulos. Cada `paises/<CC>.json` declara
// las formas jurídicas del país, sus portales y sus obligaciones; el código no
// sabe qué es el modelo 303 ni la Umsatzsteuer-Voranmeldung, sabe que hay una
// obligación trimestral que vence el día 20 del mes siguiente. Añadir un país
// o corregir un plazo es tocar un fichero, no una migración.
//
// LO QUE ESTÁ VERIFICADO Y LO QUE NO va escrito en el propio dato
// (`verificado`). España está revisada contra el calendario del contribuyente;
// el resto de países es orientativo y la interfaz lo dice: quien lo mira debe
// contrastarlo con su portal antes de fiarse de una fecha. Una fecha de
// vencimiento equivocada cuesta una sanción, así que aquí no se presume.
//
// Los festivos se modelan SOLO donde hay una fuente oficial que los publica cada
// año y se ha contrastado (España: la resolución de días inhábiles del BOE, ver
// `CalendarioLaboral`); en el resto, un vencimiento en festivo se corre al lunes
// siguiente como mucho. Lo que NO se modela, a propósito: regímenes especiales y
// umbrales de facturación. El perfil recoge lo que cambia el calendario de verdad para una
// pyme: forma jurídica, IVA (y su periodicidad), empleados, operaciones
// intracomunitarias y alquileres.
package obligaciones

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"regexp"
	"slices"
	"sort"
	"strings"
	"time"
)

//go:embed paises/*.json
var ficheros embed.FS

// Tipos de forma jurídica. Es la distinción que más calendario cambia: la
// persona física tributa por renta, la sociedad por sociedades y deposita cuentas.
const (
	TipoPersonaFisica = "persona_fisica"
	TipoSociedad      = "sociedad"
)

var (
	// Ambitos clasifica cada obligación para pintarla y filtrarla.
	Ambitos = []string{"fiscal", "contable", "laboral", "mercantil"}
	// Periodicidades admitidas. «anual» cubre también los pagos fraccionados
	// con fechas fijas: son varias fechas al año, no un periodo que se repite.
	Periodicidades = []string{"mensual", "trimestral", "anual"}
	// Condiciones son los interruptores del perfil que una obligación puede
	// exigir (o excluir con `!`). Cerrado: una condición nueva es una pregunta
	// más en el asistente, y cada pregunta de más es gente que no lo termina.
	Condiciones = []string{"iva", "iva_mensual", "empleados", "intracomunitario", "alquiler"}

	claveRe = regexp.MustCompile(`^[a-z][a-z0-9_]{0,39}$`)
	paisRe  = regexp.MustCompile(`^[A-Z]{2}$`)
)

// Forma es una forma jurídica del país. El nombre es un nombre propio (SL,
// GmbH, SARL) y no se traduce: quien elige sabe cómo se llama lo suyo.
type Forma struct {
	Clave  string `json:"clave"`
	Nombre string `json:"nombre"`
	Tipo   string `json:"tipo"`
}

// Portal es la sede electrónica donde se presenta una obligación.
type Portal struct {
	Nombre string `json:"nombre"`
	URL    string `json:"url"`
}

// Sancion describe, en números, qué cuesta llegar tarde. Va en cifras y no en
// prosa para que la interfaz pueda componer la frase en el idioma de quien mira
// y estimar el importe sobre la cuota calculada; `nota` es el detalle legal en
// el idioma del país, para quien quiera la letra pequeña.
type Sancion struct {
	// Tipo: recargo (porcentaje sobre la cuota), fija (importe) o nota (solo texto).
	Tipo string `json:"tipo"`
	// RecargoBasePct se aplica una vez por llegar tarde.
	RecargoBasePct float64 `json:"recargo_base_pct,omitempty"`
	// RecargoPctMes se suma por cada mes (o fracción) de retraso.
	RecargoPctMes float64 `json:"recargo_pct_mes,omitempty"`
	// RecargoMaxPct acota la suma anterior.
	RecargoMaxPct float64 `json:"recargo_max_pct,omitempty"`
	// TrasMeses: número de meses a partir del cual aplica el máximo (informativo).
	TrasMeses int `json:"tras_meses,omitempty"`
	// ConRequerimientoPct es la horquilla [mín, máx] si es la administración
	// quien reclama primero.
	ConRequerimientoPct []float64 `json:"con_requerimiento_pct,omitempty"`
	// FijaCentimos es el importe fijo (tipo fija) o el mínimo (tipo recargo).
	FijaCentimos int64 `json:"fija_centimos,omitempty"`
	// Moneda de los importes; vacía = la del país.
	Moneda string `json:"moneda,omitempty"`
	Nota   string `json:"nota,omitempty"`
}

// Calculo dice cómo sacar del libro la cifra que pide la obligación. Es
// declarativo por el mismo motivo que el asiento contable del catálogo: la
// diferencia entre el IVA a ingresar y el pago fraccionado son dos listas de
// prefijos de cuenta y un porcentaje.
type Calculo struct {
	// Tipo: saldo_neto (haber de unas cuentas menos debe de otras) o
	// porcentaje_resultado (porcentaje sobre ingresos menos gastos).
	Tipo string `json:"tipo"`
	// Haber y Debe son prefijos de cuenta del plan (477, 472, 7, 6).
	Haber      []string `json:"haber"`
	Debe       []string `json:"debe"`
	Porcentaje float64  `json:"porcentaje,omitempty"`
	// Acumulado: el porcentaje va sobre el resultado desde el 1 de enero y se
	// restan los pagos de los periodos anteriores (el 130 español), no sobre el
	// periodo suelto, que tras un trimestre en pérdidas haría pagar de más.
	Acumulado bool `json:"acumulado,omitempty"`
}

// Aplica acota a quién afecta una obligación. Vacío = a todo el mundo del país.
type Aplica struct {
	Tipos  []string `json:"tipos,omitempty"`
	Formas []string `json:"formas,omitempty"`
	// Condiciones del perfil que deben cumplirse; `!x` exige que NO se cumpla.
	Condiciones []string `json:"condiciones,omitempty"`
	// ExceptoRegiones son las regiones donde NO aplica porque ese impuesto lo
	// gestiona otra administración (en España, las haciendas forales, o el IGIC
	// canario en lugar del IVA).
	ExceptoRegiones []string `json:"excepto_regiones,omitempty"`
}

// Guia explica una obligación a quien no sabe de impuestos: qué es, qué tiene que
// tener al día, cómo se hace y qué pasa si no llega. Va en el idioma del país.
type Guia struct {
	Que       string   `json:"que"`
	Necesitas []string `json:"necesitas"`
	Pasos     []string `json:"pasos"`
	SiNo      string   `json:"si_no"`
}

// Territorio es una región con su propia administración para algunos impuestos.
type Territorio struct {
	Nombre string `json:"nombre"`
	Nota   string `json:"nota"`
}

// Documento es un fichero que una obligación pide guardar, con lo que tiene que
// traer para valer. Va en el idioma del país, como la guía.
type Documento struct {
	Clave      string   `json:"clave"`
	Nombre     string   `json:"nombre"`
	Requisitos []string `json:"requisitos"`
}

// RequisitosFactura es lo que tiene que tener una factura para que valga en el
// país, dicho para quien no lo sabe: la completa, la simplificada (un ticket) y la
// de un proveedor de fuera.
type RequisitosFactura struct {
	Fuente       string   `json:"fuente"`
	Completa     []string `json:"completa"`
	Simplificada []string `json:"simplificada"`
	Extranjera   []string `json:"extranjera"`
}

// Preparaciones son las comprobaciones del ERP que una obligación puede pedir
// antes de presentarse: los movimientos del banco justificados, las facturas
// confirmadas y en el libro, y las nóminas confirmadas.
var Preparaciones = []string{"banco", "facturas", "nominas"}

// Obligacion es una obligación recurrente tal como la declara el país.
type Obligacion struct {
	Clave  string `json:"clave"`
	Nombre string `json:"nombre"`
	// Concepto es la clave del diccionario común (`obligacion.<concepto>`): lo
	// que la obligación ES, traducido a los cinco idiomas, frente al nombre
	// propio que le da cada país.
	Concepto     string            `json:"concepto"`
	Ambito       string            `json:"ambito"`
	Periodicidad string            `json:"periodicidad"`
	Aplica       Aplica            `json:"aplica"`
	Plazos       map[string]string `json:"plazos"`
	// MargenDias es cuánto antes del límite conviene tenerlo hecho, en países
	// sin calendario laboral declarado.
	MargenDias int      `json:"margen_dias"`
	Portal     string   `json:"portal"`
	Sancion    *Sancion `json:"sancion,omitempty"`
	Calculo    *Calculo `json:"calculo,omitempty"`
	// Domiciliable: el pago se puede domiciliar, y hacerlo acaba antes que el plazo.
	Domiciliable bool     `json:"domiciliable,omitempty"`
	Guia         *Guia    `json:"guia,omitempty"`
	Preparacion  []string `json:"preparacion,omitempty"`
	// Documentos son los ficheros que conviene subir al ERP cuando se presenta.
	Documentos []Documento `json:"documentos,omitempty"`
}

// Pais es el fichero de un país ya validado.
type Pais struct {
	Pais         string            `json:"pais"`
	Verificado   bool              `json:"verificado"`
	Moneda       string            `json:"moneda"`
	Formas       []Forma           `json:"formas"`
	Portales     map[string]Portal `json:"portales"`
	Obligaciones []Obligacion      `json:"obligaciones"`
	// Calendario cuenta los plazos con los días inhábiles oficiales. Sin él, un
	// límite en fin de semana se corre al lunes y el margen va en días naturales.
	Calendario *CalendarioLaboral `json:"calendario,omitempty"`
	// Territorios son las regiones con otra administración, por región.
	Territorios map[string]Territorio `json:"territorios,omitempty"`
	// RequisitosFactura, donde se han escrito.
	RequisitosFactura *RequisitosFactura `json:"requisitos_factura,omitempty"`
}

// Catalogo son los veintisiete países cargados.
type Catalogo struct {
	paises map[string]*Pais
}

// Cargar lee y valida los ficheros embebidos. `hayTermino` dice si una clave
// existe en el diccionario común: un concepto sin sus cinco idiomas es un error
// de ARRANQUE, como en el catálogo de módulos.
func Cargar(hayTermino func(clave string) bool) (*Catalogo, error) {
	entradas, err := fs.ReadDir(ficheros, "paises")
	if err != nil {
		return nil, fmt.Errorf("obligaciones: leer paises/: %w", err)
	}
	c := &Catalogo{paises: map[string]*Pais{}}
	for _, e := range entradas {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		crudo, err := ficheros.ReadFile("paises/" + e.Name())
		if err != nil {
			return nil, fmt.Errorf("obligaciones: %s: %w", e.Name(), err)
		}
		var p Pais
		if err := json.Unmarshal(crudo, &p); err != nil {
			return nil, fmt.Errorf("obligaciones: %s: %w", e.Name(), err)
		}
		if err := validar(&p, hayTermino); err != nil {
			return nil, fmt.Errorf("obligaciones: %s: %w", e.Name(), err)
		}
		if _, dup := c.paises[p.Pais]; dup {
			return nil, fmt.Errorf("obligaciones: país duplicado %q", p.Pais)
		}
		c.paises[p.Pais] = &p
	}
	if len(c.paises) == 0 {
		return nil, fmt.Errorf("obligaciones: ningún país cargado")
	}
	return c, nil
}

func validar(p *Pais, hayTermino func(string) bool) error {
	if !paisRe.MatchString(p.Pais) {
		return fmt.Errorf("código de país inválido %q", p.Pais)
	}
	if len(p.Moneda) != 3 {
		return fmt.Errorf("moneda inválida %q", p.Moneda)
	}
	if len(p.Formas) == 0 {
		return fmt.Errorf("sin formas jurídicas")
	}
	formas := map[string]bool{}
	for _, f := range p.Formas {
		if !claveRe.MatchString(f.Clave) || formas[f.Clave] {
			return fmt.Errorf("forma %q: clave inválida o duplicada", f.Clave)
		}
		if f.Tipo != TipoPersonaFisica && f.Tipo != TipoSociedad {
			return fmt.Errorf("forma %q: tipo desconocido %q", f.Clave, f.Tipo)
		}
		if strings.TrimSpace(f.Nombre) == "" {
			return fmt.Errorf("forma %q: sin nombre", f.Clave)
		}
		formas[f.Clave] = true
	}
	for k, po := range p.Portales {
		if !claveRe.MatchString(k) || !strings.HasPrefix(po.URL, "https://") || po.Nombre == "" {
			return fmt.Errorf("portal %q: clave, nombre o URL inválidos", k)
		}
	}
	if err := p.validarCalendario(); err != nil {
		return err
	}
	claves := map[string]bool{}
	for i := range p.Obligaciones {
		o := &p.Obligaciones[i]
		if err := validarObligacion(o, p, formas, hayTermino); err != nil {
			return fmt.Errorf("obligación %q: %w", o.Clave, err)
		}
		if claves[o.Clave] {
			return fmt.Errorf("obligación %q duplicada", o.Clave)
		}
		claves[o.Clave] = true
	}
	return nil
}

// validarCalendario comprueba los días inhábiles y los territorios, que solo
// declara un país con calendario laboral.
func (p *Pais) validarCalendario() error {
	if p.Calendario != nil {
		if err := p.Calendario.validar(); err != nil {
			return err
		}
	}
	for k, t := range p.Territorios {
		if t.Nombre == "" || t.Nota == "" {
			return fmt.Errorf("territorio %q sin nombre o nota", k)
		}
	}
	return nil
}

func validarObligacion(o *Obligacion, p *Pais, formas map[string]bool, hayTermino func(string) bool) error {
	if !claveRe.MatchString(o.Clave) || strings.TrimSpace(o.Nombre) == "" {
		return fmt.Errorf("clave o nombre inválidos")
	}
	if !hayTermino("obligacion." + o.Concepto) {
		return fmt.Errorf("concepto %q sin rótulo en terminos.json", o.Concepto)
	}
	if !slices.Contains(Ambitos, o.Ambito) {
		return fmt.Errorf("ámbito desconocido %q", o.Ambito)
	}
	if !slices.Contains(Periodicidades, o.Periodicidad) {
		return fmt.Errorf("periodicidad desconocida %q", o.Periodicidad)
	}
	if _, ok := p.Portales[o.Portal]; !ok {
		return fmt.Errorf("portal desconocido %q", o.Portal)
	}
	if o.MargenDias < 0 || o.MargenDias > 120 {
		return fmt.Errorf("margen_dias fuera de rango")
	}
	if err := o.Aplica.validar(formas); err != nil {
		return err
	}
	if err := o.validarSancionYCalculo(); err != nil {
		return err
	}
	if err := o.validarGuia(); err != nil {
		return err
	}
	// Los plazos se comprueban GENERANDO un año: es la única forma de saber que
	// cada regla se entiende y que no falta ningún trimestre.
	if _, err := o.instancias(2026); err != nil {
		return fmt.Errorf("plazos: %w", err)
	}
	return nil
}

// validarGuia comprueba lo que se le enseña a quien presenta: la guía, lo que hay
// que preparar en el ERP y los documentos que se suben.
func (o *Obligacion) validarGuia() error {
	for _, prep := range o.Preparacion {
		if !slices.Contains(Preparaciones, prep) {
			return fmt.Errorf("preparación desconocida %q", prep)
		}
	}
	if g := o.Guia; g != nil && (g.Que == "" || len(g.Pasos) == 0 || g.SiNo == "") {
		return fmt.Errorf("guía incompleta")
	}
	for _, d := range o.Documentos {
		if !claveRe.MatchString(d.Clave) || d.Nombre == "" || len(d.Requisitos) == 0 {
			return fmt.Errorf("documento %q incompleto", d.Clave)
		}
	}
	return nil
}

func (a Aplica) validar(formas map[string]bool) error {
	for _, t := range a.Tipos {
		if t != TipoPersonaFisica && t != TipoSociedad {
			return fmt.Errorf("aplica.tipos: %q", t)
		}
	}
	for _, f := range a.Formas {
		if !formas[f] {
			return fmt.Errorf("aplica.formas: forma desconocida %q", f)
		}
	}
	for _, c := range a.Condiciones {
		if !slices.Contains(Condiciones, strings.TrimPrefix(c, "!")) {
			return fmt.Errorf("aplica.condiciones: condición desconocida %q", c)
		}
	}
	return nil
}

func (o *Obligacion) validarSancionYCalculo() error {
	if o.Sancion != nil {
		switch o.Sancion.Tipo {
		case "recargo", "fija", "nota":
		default:
			return fmt.Errorf("sanción: tipo desconocido %q", o.Sancion.Tipo)
		}
	}
	if o.Calculo == nil {
		return nil
	}
	switch o.Calculo.Tipo {
	case "saldo_neto":
	case "porcentaje_resultado":
		if o.Calculo.Porcentaje <= 0 || o.Calculo.Porcentaje > 100 {
			return fmt.Errorf("cálculo: porcentaje fuera de rango")
		}
	default:
		return fmt.Errorf("cálculo: tipo desconocido %q", o.Calculo.Tipo)
	}
	if len(o.Calculo.Haber)+len(o.Calculo.Debe) == 0 {
		return fmt.Errorf("cálculo: sin cuentas")
	}
	return nil
}

// Paises devuelve los países cargados, por código.
func (c *Catalogo) Paises() []*Pais {
	out := make([]*Pais, 0, len(c.paises))
	for _, p := range c.paises {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Pais < out[j].Pais })
	return out
}

// Pais devuelve un país por su código ISO.
func (c *Catalogo) Pais(codigo string) (*Pais, bool) {
	p, ok := c.paises[codigo]
	return p, ok
}

// Forma busca una forma jurídica del país.
func (p *Pais) Forma(clave string) (Forma, bool) {
	for _, f := range p.Formas {
		if f.Clave == clave {
			return f, true
		}
	}
	return Forma{}, false
}

// Perfil es lo que el asistente recoge de la organización y decide su calendario.
type Perfil struct {
	Pais        string
	Forma       string
	Condiciones map[string]bool
	// Region es la comunidad del domicilio: cambia festivos y, en los territorios
	// con hacienda propia, qué impuestos se presentan aquí.
	Region string
}

// PerfilDe arma el perfil de una organización a partir de su ficha: la región
// sale del código postal cuando el país tiene calendario laboral.
func (c *Catalogo) PerfilDe(pais, forma, cp string, condiciones map[string]bool) Perfil {
	perfil := Perfil{Pais: pais, Forma: forma, Condiciones: condiciones}
	if p, ok := c.paises[pais]; ok {
		perfil.Region = p.Calendario.Region(cp)
	}
	return perfil
}

func (o *Obligacion) aplicaA(f Forma, cond map[string]bool, region string) bool {
	if region != "" && slices.Contains(o.Aplica.ExceptoRegiones, region) {
		return false
	}
	if len(o.Aplica.Tipos) > 0 && !slices.Contains(o.Aplica.Tipos, f.Tipo) {
		return false
	}
	if len(o.Aplica.Formas) > 0 && !slices.Contains(o.Aplica.Formas, f.Clave) {
		return false
	}
	for _, c := range o.Aplica.Condiciones {
		if negada := strings.HasPrefix(c, "!"); negada {
			if cond[c[1:]] {
				return false
			}
		} else if !cond[c] {
			return false
		}
	}
	return true
}

// Aplican devuelve las obligaciones del perfil, en el orden del fichero.
func (c *Catalogo) Aplican(perfil Perfil) ([]Obligacion, error) {
	p, ok := c.paises[perfil.Pais]
	if !ok {
		return nil, fmt.Errorf("país desconocido %q", perfil.Pais)
	}
	f, ok := p.Forma(perfil.Forma)
	if !ok {
		return nil, fmt.Errorf("forma jurídica desconocida %q", perfil.Forma)
	}
	var out []Obligacion
	for _, o := range p.Obligaciones {
		if o.aplicaA(f, perfil.Condiciones, perfil.Region) {
			out = append(out, o)
		}
	}
	return out, nil
}

// Instancia es una obligación en un periodo concreto: la fila del calendario.
type Instancia struct {
	Clave        string    `json:"clave"`
	Nombre       string    `json:"nombre"`
	Concepto     string    `json:"concepto"`
	Ambito       string    `json:"ambito"`
	Periodicidad string    `json:"periodicidad"`
	Periodo      string    `json:"periodo"`
	Limite       time.Time `json:"limite"`
	Recomendada  time.Time `json:"recomendada"`
	Portal       Portal    `json:"portal"`
	Sancion      *Sancion  `json:"sancion,omitempty"`
	Calculable   bool      `json:"calculable"`
	// LimiteDomiciliacion es el último día para presentar domiciliando el pago.
	LimiteDomiciliacion *time.Time  `json:"limite_domiciliacion,omitempty"`
	Guia                *Guia       `json:"guia,omitempty"`
	Preparacion         []string    `json:"preparacion,omitempty"`
	Documentos          []Documento `json:"documentos,omitempty"`
	// CalendarioProvisional: el año no tiene aún resolución de días inhábiles.
	CalendarioProvisional bool `json:"calendario_provisional,omitempty"`
}

// Calendario genera las instancias del año para el perfil, ordenadas por límite.
//
// Con calendario laboral (España), el límite se corre al primer hábil siguiente
// si cae en inhábil, y `Recomendada` —«tu fecha»— es `margen_habiles` días
// hábiles antes del ÚLTIMO día útil: el de domiciliación si el pago se puede
// domiciliar, que acaba antes. Sin él, un límite en fin de semana se corre al
// lunes y el margen va en días naturales.
func (c *Catalogo) Calendario(perfil Perfil, anio int) ([]Instancia, error) {
	aplican, err := c.Aplican(perfil)
	if err != nil {
		return nil, err
	}
	p := c.paises[perfil.Pais]
	var out []Instancia
	for i := range aplican {
		o := &aplican[i]
		inst, err := o.instancias(anio)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", o.Clave, err)
		}
		for _, in := range inst {
			fila := Instancia{
				Clave: o.Clave, Nombre: o.Nombre, Concepto: o.Concepto, Ambito: o.Ambito,
				Periodicidad: o.Periodicidad, Periodo: in.periodo,
				Portal:      p.Portales[o.Portal],
				Sancion:     o.Sancion,
				Calculable:  o.Calculo != nil,
				Guia:        o.Guia,
				Preparacion: o.Preparacion,
				Documentos:  o.Documentos,
			}
			if cal := p.Calendario; cal != nil {
				limite, prov1 := cal.siguienteHabil(in.limite)
				ultimoUtil := limite
				if o.Domiciliable {
					dom := cal.limiteDomiciliacion(limite)
					fila.LimiteDomiciliacion, ultimoUtil = &dom, dom
				}
				tuFecha, prov2 := cal.habilesAntes(ultimoUtil, cal.MargenHabiles, perfil.Region)
				fila.Limite, fila.Recomendada, fila.CalendarioProvisional = limite, tuFecha, prov1 || prov2
			} else {
				fila.Limite = siguienteHabil(in.limite)
				fila.Recomendada = fila.Limite.AddDate(0, 0, -o.MargenDias)
			}
			out = append(out, fila)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if !out[i].Limite.Equal(out[j].Limite) {
			return out[i].Limite.Before(out[j].Limite)
		}
		return out[i].Clave < out[j].Clave
	})
	return out, nil
}

// Obligacion busca una obligación del país por clave.
func (p *Pais) Obligacion(clave string) (*Obligacion, bool) {
	for i := range p.Obligaciones {
		if p.Obligaciones[i].Clave == clave {
			return &p.Obligaciones[i], true
		}
	}
	return nil, false
}

func siguienteHabil(t time.Time) time.Time {
	switch t.Weekday() {
	case time.Saturday:
		return t.AddDate(0, 0, 2)
	case time.Sunday:
		return t.AddDate(0, 0, 1)
	default:
		return t
	}
}

// Estimar calcula, en céntimos de la moneda de la sanción, lo que costaría
// presentar `dias` tarde una cuota de `importe`. El segundo valor es false si
// la sanción no se puede poner en cifras (solo tiene nota).
//
// Es una ESTIMACIÓN con la regla general: no contempla reducciones por pronto
// pago, requerimientos ni intereses de demora, y así lo dice la interfaz.
func Estimar(s *Sancion, importe int64, dias int) (int64, bool) {
	if s == nil || dias <= 0 {
		return 0, false
	}
	switch s.Tipo {
	case "fija":
		return s.FijaCentimos, true
	case "recargo":
		meses := (dias + 29) / 30
		pct := s.RecargoBasePct + s.RecargoPctMes*float64(meses)
		if s.RecargoMaxPct > 0 && pct > s.RecargoMaxPct {
			pct = s.RecargoMaxPct
		}
		est := int64(float64(importe) * pct / 100)
		if est < s.FijaCentimos {
			est = s.FijaCentimos
		}
		return est, true
	}
	return 0, false
}

// PagoFraccionado es lo que toca pagar en el último periodo de `acumulados` (el
// resultado desde el 1 de enero al cierre de cada periodo, en orden): el
// porcentaje de lo acumulado menos lo pagado antes, y nunca menos de cero.
func PagoFraccionado(acumulados []int64, pct float64) int64 {
	var pagado, pago int64
	for _, r := range acumulados {
		pago = max(0, int64(float64(max(r, 0))*pct/100)-pagado)
		pagado += pago
	}
	return pago
}
