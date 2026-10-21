// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package erp

import (
	"image/color"
	"sort"
	"strconv"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// # Obligaciones (`Obligaciones.tsx`)
//
// El calendario fiscal, contable, laboral y mercantil, mes a mes, con «tu fecha» —días
// hábiles antes del límite— y lo que le falta a cada presentación en el ERP. Medido en
// el arnés (1280×800 y 360×740, 2026-09-23):
//
//	página     `max-w-5xl` (896) con `px-6 py-6` y `space-y-6`
//	cabecera   57: título `text-lg` con su insignia, frase `mt-1`; a la derecha el año
//	           (93,45×23), «Añadir avisos…» (196,75×28) y «Editar perfil»; `pb-4` y filete.
//	           En el móvil la derecha baja (`gap-3`) y salta de línea (`gap-2`)
//	ámbitos    píldoras de 21 (`px-2.5 py-1`, `gap-1.5`), la activa en `bg-selected`
//	mes        versalitas de 14, `space-y-2`, lista `rounded-lg border divide-y`
//	fila       `px-4 py-3`; izquierda nombre · periodo · ámbito (17,5), qué es (`mt-0.5`),
//	           lo que falta (`mt-1`), «Cómo se hace», documentos, sanción y la cifra;
//	           derecha (`gap-1`, `items-end`) tu fecha con su insignia, domiciliación,
//	           plazo legal y, con `mt-1`, «Preparar cifras» (24) y «Abrir el portal» (21).
//	           En el móvil la derecha baja (`gap-2`) y se alinea a la izquierda
//	vencidas   `details` con borde; `summary` `px-4 py-3 text-sm`

// Medidas de Obligaciones.
const (
	// AnchoObligaciones es `max-w-5xl`.
	AnchoObligaciones float32 = 64 * tema.Rem
	// ObligGap es el `space-y-6` de la página y el `gap-3` de la cabecera en el móvil.
	ObligGap         float32 = 6 * 0.25 * tema.Rem
	CabeceraObligGap float32 = 3 * 0.25 * tema.Rem
	// HuecoDos es el `gap-2`, el `mt-2` y el `space-y-2` que se repiten en la vista.
	HuecoDos float32 = 2 * 0.25 * tema.Rem
	// HuecoUno es el `gap-1` y el `mt-1`; HuecoMedio el `mt-0.5` y el `space-y-0.5`.
	HuecoUno   float32 = 1 * 0.25 * tema.Rem
	HuecoMedio float32 = 0.5 * 0.25 * tema.Rem
	// PildoraObligPX es el `px-2.5` de las de ámbito y HuecoPildorasOblig su `gap-1.5`.
	PildoraObligPX     float32 = 2.5 * 0.25 * tema.Rem
	HuecoPildorasOblig float32 = 1.5 * 0.25 * tema.Rem
	// FilaOblPX y FilaOblPY son el `px-4 py-3` de una fila; HuecoColumnasObl el `sm:gap-4`.
	FilaOblPX        float32 = 4 * 0.25 * tema.Rem
	FilaOblPY        float32 = 3 * 0.25 * tema.Rem
	HuecoColumnasObl float32 = 4 * 0.25 * tema.Rem
	// HuecoFaltasX es el `gap-x-3` de lo que falta.
	HuecoFaltasX float32 = 3 * 0.25 * tema.Rem
	// ListaPL es el `pl-5` de una lista con viñetas.
	ListaPL float32 = 5 * 0.25 * tema.Rem
	// CalculoPX y CalculoPY son el `px-3 py-2` de la cifra preparada.
	CalculoPX float32 = 3 * 0.25 * tema.Rem
	CalculoPY float32 = 2 * 0.25 * tema.Rem
	// PortalPX es el `px-2` de «Abrir el portal» y AltoPortal su `h-6`.
	PortalPX   float32 = 2 * 0.25 * tema.Rem
	AltoPortal float32 = 6 * 0.25 * tema.Rem
	// AnioPX y AnioPY son el `px-2 py-1` de las flechas del año; AnioTextoPX el `px-1`.
	AnioPX      float32 = 2 * 0.25 * tema.Rem
	AnioPY      float32 = 1 * 0.25 * tema.Rem
	AnioTextoPX float32 = 1 * 0.25 * tema.Rem
	// HuecoFechaInsignia es el `gap-1.5` de «Tu fecha» y su insignia.
	HuecoFechaInsignia float32 = 1.5 * 0.25 * tema.Rem
	// VacioOblIlustracion es el `size={96}` del calendario vacío.
	VacioOblIlustracion float32 = 96
	// CargandoOblPY es el `py-12` de la rueda.
	CargandoOblPY float32 = 12 * 0.25 * tema.Rem
)

// ProsaObligaciones es `max-w-prose` (65 ch) de la guía (`text-sm`), ProsaDocumentos
// el de los documentos (la letra de la página, 14 px) y ProsaNota el de la letra
// pequeña (`text-xs`). Medidos en el arnés con los `details` abiertos (2026-09-24): el
// `ch` de Figtree no escala exacto con la letra, y la proporción daba 582,5 y 436,87.
const (
	ProsaObligaciones float32 = 541.640625
	ProsaDocumentos   float32 = 666.640625
	ProsaNota         float32 = 499.96875
)

// CalculoObl es el estado de «Preparar cifras» de una fila.
type CalculoObl struct {
	Cargando, Fallo bool
	R               api.ERPCalculoObligacion
}

// Obligaciones es el estado de la pantalla.
type Obligaciones struct {
	tr  *i18n.Translator
	loc i18n.Locale

	Anio int
	// Datos es el calendario del año; nil, todavía no ha llegado.
	Datos    *api.ERPObligaciones
	Cargando bool
	Fallo    string
	// Ambito es el filtro de arriba; vacío, todas.
	Ambito string
	// Pais es el de la organización, para la frase de la cabecera.
	Pais        string
	SoloLectura bool
	// Calculos son las cifras pedidas, por `clave|periodo`.
	Calculos map[string]*CalculoObl
	// Abiertos son los `<details>` desplegados, por `que|clave|periodo`.
	Abiertos map[string]bool
	// Adjuntos son los documentos subidos a cada presentación, por su registro
	// (`clave:periodo:documento`); nil, cargando.
	Adjuntos map[string][]api.ERPAdjunto
	// Hoy es la fecha de referencia; cero, la de ahora. Se deja abierta para el guardián.
	Hoy     time.Time
	Ventana float32

	AlAnio   func(anio int)
	AlICS    func()
	AlPortal func(url string)
	// AlEditarPerfil y AlBanco llevan al asistente y al banco. nil: no se pinta el mando.
	AlEditarPerfil func()
	AlBanco        func()
	AlPreparar     func(o api.ERPObligacion)
	AlProceso      func(o api.ERPObligacion)
	Proceso        *ProcesoObligacion
	// AlDocumentos pide los documentos de una presentación al desplegarlos: con
	// cuarenta filas, pedirlos todos al abrir la vista serían cuarenta consultas.
	AlDocumentos   func(o api.ERPObligacion)
	AlSubirDoc     func(registro string)
	AlAbrirAdjunto func(a api.ERPAdjunto)
	AlBorrarAdj    func(registro string, a api.ERPAdjunto)
	// AlCambio repinta tras plegar, desplegar o filtrar.
	AlCambio func()
}

// NuevaObligaciones crea la pantalla en el año en curso.
func NuevaObligaciones(tr *i18n.Translator) *Obligaciones {
	return &Obligaciones{
		tr: tr, loc: tr.Locale(), Anio: time.Now().Year(),
		Calculos: map[string]*CalculoObl{}, Abiertos: map[string]bool{}, Adjuntos: map[string][]api.ERPAdjunto{},
	}
}

func (o *Obligaciones) t(clave string, vars ...i18n.Vars) string {
	if len(vars) > 0 {
		return o.tr.T(clave, vars[0])
	}
	return o.tr.T(clave, nil)
}

func (o *Obligaciones) hoy() time.Time {
	if o.Hoy.IsZero() {
		return time.Now()
	}
	return o.Hoy
}

func (o *Obligaciones) cambio() {
	if o.AlCambio != nil {
		o.AlCambio()
	}
}

// IDObligacion es `clave|periodo`, la clave de una fila.
func IDObligacion(ob api.ERPObligacion) string { return ob.Clave + "|" + ob.Periodo }

// RegistroDocumento es el registro con que se guardan sus ficheros.
func RegistroDocumento(ob api.ERPObligacion, doc string) string {
	return ob.Clave + ":" + ob.Periodo + ":" + doc
}

// --- Lo que se calcula (las funciones exportadas de `Obligaciones.tsx`) -------------

// DiasHasta son los días entre hoy y una fecha, contando en UTC y por días enteros.
func DiasHasta(t, hoy time.Time) int {
	u := t.UTC()
	objetivo := time.Date(u.Year(), u.Month(), u.Day(), 0, 0, 0, 0, time.UTC)
	ahora := time.Date(hoy.Year(), hoy.Month(), hoy.Day(), 0, 0, 0, 0, time.UTC)
	return int(objetivo.Sub(ahora).Hours() / 24)
}

// RotuloDePeriodo es «3T 2026», «septiembre de 2026», «Ejercicio 2026» o «Pago 2, 2026».
func RotuloDePeriodo(periodo string, l i18n.Locale, tr *i18n.Translator) string {
	anio, resto, _ := strings.Cut(periodo, "-")
	switch {
	case len(resto) == 2 && resto[0] == 'T' && resto[1] >= '1' && resto[1] <= '4':
		return tr.T("obligations.period.quarter", i18n.Vars{"n": resto[1:], "year": anio})
	case len(resto) == 2 && resto[0] >= '0' && resto[0] <= '9' && resto[1] >= '0' && resto[1] <= '9':
		a, _ := strconv.Atoi(anio)
		m, _ := strconv.Atoi(resto)
		return i18n.FormatMonthYear(l, time.Date(a, time.Month(m), 1, 0, 0, 0, 0, time.UTC))
	case resto == "A":
		return tr.T("obligations.period.year", i18n.Vars{"year": anio})
	case len(resto) == 2 && resto[1] == 'P' && resto[0] >= '0' && resto[0] <= '9':
		return tr.T("obligations.period.instalment", i18n.Vars{"n": resto[:1], "year": anio})
	}
	return periodo
}

// pct es `String(n)` de JavaScript: sin ceros de más y con punto.
func pct(n float64) string { return strconv.FormatFloat(n, 'f', -1, 64) }

// Dinero es `formatearDinero`: el importe en la moneda del país con el patrón de Intl
// para ese idioma y esa moneda (`i18n.FormatCurrencyIn`): «250,00 SEK» en castellano,
// «SEK 250.00» en inglés, y la corona islandesa sin decimales.
func Dinero(l i18n.Locale, centimos int64, moneda string) string {
	return i18n.FormatCurrencyIn(l, centimos, moneda)
}

// DescribirSancion es la sanción en una frase, compuesta de sus cifras en el idioma de
// quien mira. La nota —la letra pequeña, en el idioma del país— va aparte.
func DescribirSancion(s *api.ERPSancion, moneda string, l i18n.Locale, tr *i18n.Translator) string {
	if s == nil {
		return ""
	}
	divisa := moneda
	if s.Moneda != "" {
		divisa = s.Moneda
	}
	var partes []string
	if s.Tipo == "fija" && s.FijaCentimos != 0 {
		partes = append(partes, tr.T("obligations.penaltyFixed", i18n.Vars{"amount": Dinero(l, s.FijaCentimos, divisa)}))
	}
	if s.Tipo == "recargo" {
		if s.RecargoBasePct != 0 {
			partes = append(partes, tr.T("obligations.penaltyBase", i18n.Vars{"pct": pct(s.RecargoBasePct)}))
		}
		if s.RecargoPctMes != 0 {
			partes = append(partes, tr.T("obligations.penaltyPerMonth", i18n.Vars{"pct": pct(s.RecargoPctMes)}))
		}
		if s.RecargoMaxPct != 0 {
			partes = append(partes, tr.T("obligations.penaltyMax", i18n.Vars{"pct": pct(s.RecargoMaxPct)}))
		}
		if s.FijaCentimos != 0 {
			partes = append(partes, tr.T("obligations.penaltyMin", i18n.Vars{"amount": Dinero(l, s.FijaCentimos, divisa)}))
		}
		if len(s.ConRequerimientoPct) == 2 {
			partes = append(partes, tr.T("obligations.penaltyDemand", i18n.Vars{
				"min": pct(s.ConRequerimientoPct[0]), "max": pct(s.ConRequerimientoPct[1]),
			}))
		}
	}
	return strings.Join(partes, " · ")
}

// EstimarUnMes es lo que costaría UN mes de retraso sobre una cuota, con la regla
// general de la sanción. `false`: no se puede decir.
func EstimarUnMes(s *api.ERPSancion, importe int64) (int64, bool) {
	if s == nil || importe <= 0 {
		return 0, false
	}
	switch s.Tipo {
	case "fija":
		return s.FijaCentimos, s.FijaCentimos != 0
	case "recargo":
		p := s.RecargoBasePct + s.RecargoPctMes
		if s.RecargoMaxPct != 0 && p > s.RecargoMaxPct {
			p = s.RecargoMaxPct
		}
		// `Math.round` de JavaScript: la mitad sube.
		return max(int64(float64(importe)*p/100+0.5), s.FijaCentimos), true
	}
	return 0, false
}

// Falta es una cosa que le falta a una presentación en el ERP.
type Falta struct{ Clave, Texto string }

// Faltas son las frases de lo que falta; vacío, está lista. En el orden de las claves,
// que es el del JSON que manda el servicio.
func Faltas(ob api.ERPObligacion, tr *i18n.Translator) []Falta {
	frases := map[string]string{
		"banco": "obligations.missingBank", "facturas": "obligations.missingInvoices", "nominas": "obligations.missingPayslips",
	}
	claves := make([]string, 0, len(ob.Pendientes))
	for c, n := range ob.Pendientes {
		if n > 0 && frases[c] != "" {
			claves = append(claves, c)
		}
	}
	sort.Strings(claves)
	out := make([]Falta, 0, len(claves))
	for _, c := range claves {
		out = append(out, Falta{c, tr.T(frases[c], i18n.Vars{"count": ob.Pendientes[c]})})
	}
	return out
}

// NombreDePais es `Intl.DisplayNames(…, { type: 'region' })`: «España», «Spain». Sale
// de la tabla que `i18n` genera desde Intl en Linux, así que coincide letra a letra con
// la web en los 26 idiomas.
func NombreDePais(codigo string, l i18n.Locale) string { return i18n.CountryName(l, codigo) }

// textoTabular es un texto con `tabular-nums` dibujado como la variante `tnum`: cada
// cifra centrada en su celda de 0,625 em (`CifraTabular`), y lo que no es cifra seguido
// con su avance propio. Fyne no deja pedir esa variante al conformador, así que se
// coloca cifra a cifra; lo que mide es `anchoTabular`. Devuelve el objeto, que va en su
// caja de línea, y su ancho.
func textoTabular(s string, e tema.Escalon, p ui.Peso, c color.NRGBA) (fyne.CanvasObject, float32) {
	l := &tabular{e: e}
	celda := CifraTabular * e.Tam
	var x float32
	var tramo strings.Builder
	cerrar := func() {
		if tramo.Len() == 0 {
			return
		}
		l.poner(ui.Texto(tramo.String(), e, p, c), x)
		x += ui.Medir(tramo.String(), e, p).Width
		tramo.Reset()
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			tramo.WriteRune(r)
			continue
		}
		cerrar()
		d := string(r)
		l.poner(ui.Texto(d, e, p, c), x+(celda-ui.Medir(d, e, p).Width)/2)
		x += celda
	}
	cerrar()
	return container.New(l, l.objetos...), x
}

// tabular coloca los trozos de un textoTabular en su caja de línea.
type tabular struct {
	e       tema.Escalon
	objetos []fyne.CanvasObject
	xs      []float32
}

func (t *tabular) poner(x *canvas.Text, en float32) {
	t.objetos, t.xs = append(t.objetos, x), append(t.xs, en)
}

func (t *tabular) MinSize([]fyne.CanvasObject) fyne.Size { return fyne.NewSize(0, t.e.Linea) }

func (t *tabular) Layout(_ []fyne.CanvasObject, _ fyne.Size) {
	for i, o := range t.objetos {
		if x, ok := o.(*canvas.Text); ok {
			ui.ColocarTexto(x, t.e, t.xs[i], 0)
		}
	}
}

// mes es un grupo por el mes de «tu fecha».
type mes struct {
	titulo string
	filas  []api.ERPObligacion
}

// Lista son las obligaciones del ámbito elegido, por «tu fecha».
func (o *Obligaciones) Lista() []api.ERPObligacion {
	if o.Datos == nil {
		return nil
	}
	var out []api.ERPObligacion
	for _, ob := range o.Datos.Obligaciones {
		if o.Ambito == "" || ob.Ambito == o.Ambito {
			out = append(out, ob)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Recomendada.Before(out[j].Recomendada) })
	return out
}

// Ambitos son los que hay, en el orden en que aparecen, con su rótulo.
func (o *Obligaciones) Ambitos() [][2]string {
	if o.Datos == nil {
		return nil
	}
	var out [][2]string
	vistos := map[string]bool{}
	for _, ob := range o.Datos.Obligaciones {
		if !vistos[ob.Ambito] {
			vistos[ob.Ambito] = true
			out = append(out, [2]string{ob.Ambito, ob.AmbitoRotulo})
		}
	}
	return out
}

func (o *Obligaciones) porMes(filas []api.ERPObligacion) []mes {
	var out []mes
	for _, ob := range filas {
		d := ob.Recomendada.UTC()
		if n := len(out); n > 0 {
			u := out[n-1].filas[0].Recomendada.UTC()
			if u.Year() == d.Year() && u.Month() == d.Month() {
				out[n-1].filas = append(out[n-1].filas, ob)
				continue
			}
		}
		out = append(out, mes{titulo: i18n.FormatMonthYear(o.loc, d), filas: []api.ERPObligacion{ob}})
	}
	return out
}

// Meses reparte la lista en los meses que vienen y los ya vencidos. En el año en curso
// lo vencido va al final y plegado: en septiembre, abrir la vista y encontrar ocho meses
// de «vencida hace 195 días» antes de la primera fecha que importa era leer el
// calendario al revés. Un año pasado se enseña entero y en orden.
func (o *Obligaciones) Meses() (proximos, vencidos []mes) {
	var lista []api.ERPObligacion
	for _, ob := range o.Lista() {
		if !obligacionArchivada(ob) {
			lista = append(lista, ob)
		}
	}
	hoy := o.hoy()
	if o.Anio < hoy.Year() {
		return o.porMes(lista), nil
	}
	var p, v []api.ERPObligacion
	for _, ob := range lista {
		if DiasHasta(ob.Limite, hoy) >= 0 {
			p = append(p, ob)
		} else {
			v = append(v, ob)
		}
	}
	return o.porMes(p), o.porMes(v)
}

// --- La pantalla -------------------------------------------------------------

// Objeto arma la pantalla.
func (o *Obligaciones) Objeto(v tema.Variante) fyne.CanvasObject {
	ancho := anchoUtilDe(o.Ventana, AnchoObligaciones)
	p := &pila{}
	cab, altoCab := o.cabecera(v, ancho)
	p.add(cab, altoCab, 0)
	if d := o.Datos; d != nil && d.Territorio != nil {
		a := ui.NuevaAyudaEnLinea(d.Territorio.Nota).ConTono(ui.AyudaAviso).ConTitulo(d.Territorio.Nombre)
		p.add(a, ui.AltoAyudaEnLinea(a, ancho), ObligGap)
	}
	pista := o.t("obligations.yourDateHint")
	for _, ob := range o.Lista() {
		if ob.CalendarioProvisional {
			pista += " " + o.t("obligations.provisional")
			break
		}
	}
	par := ui.NuevoParrafo(pista, tema.TextoXS, ui.Normal, "text-subtle", v)
	p.add(par, ui.AltoParrafo(par, ancho), ObligGap)
	if amb := o.Ambitos(); len(amb) > 1 {
		ob, alto := o.ambitos(amb, ancho)
		p.add(ob, alto, ObligGap)
	}
	if o.Fallo != "" {
		bnd := &banda{
			caja:     ui.NuevaCaja(tema.Color("danger-bg", v), sinColor, tema.RadioMD),
			texto:    ui.Texto(o.Fallo, tema.TextoSM, ui.Normal, tema.Color("danger-text", v)),
			completo: o.Fallo,
		}
		p.add(container.New(bnd, append(bnd.caja.Objetos(), bnd.texto)...), AltoBandaAviso, ObligGap)
	}
	proximos, vencidos := o.Meses()
	switch {
	case o.Datos == nil && (o.Cargando || o.Fallo == ""):
		alto := 24 + 2*CargandoOblPY
		p.add(centrado(ui.NuevaRueda(24, "text-muted", v), alto), alto, ObligGap)
	case len(proximos)+len(vencidos)+len(o.Archivadas()) == 0 && o.Datos != nil:
		x := ui.NuevoVacio(o.t("obligations.empty", i18n.Vars{"year": strconv.Itoa(o.Anio)}), "")
		x.Ilustracion = iconos.Pintar(iconos.VacioCalendario, v).A(VacioOblIlustracion).Objeto()
		alto := x.MedidaVacio(ancho)
		p.add(centrado(x, alto), alto, ObligGap)
	default:
		for _, m := range proximos {
			ob, alto := o.mes(v, m, ancho)
			p.add(ob, alto, ObligGap)
		}
		if len(vencidos) > 0 {
			ob, alto := o.vencidas(v, vencidos, ancho)
			p.add(ob, alto, ObligGap)
		}
	}
	if archivadas := o.Archivadas(); len(archivadas) > 0 {
		ob, alto := o.mesesPlegados(v, archivadas, ancho, "archivadas", "process.archivedList")
		p.add(ob, alto, ObligGap)
	}
	ob, alto := p.objeto()
	return enPagina(ob, alto, AnchoObligaciones, MargenLista)
}

func (o *Obligaciones) sm() bool { return o.Ventana == 0 || o.Ventana >= ui.CorteSM }

// cabecera es el título con «Revisado» u «Orientativo», la frase y, a la derecha, el
// año, el calendario y «Editar perfil».
func (o *Obligaciones) cabecera(v tema.Variante, ancho float32) (fyne.CanvasObject, float32) {
	l := &lienzo{}
	// La derecha: el año, «Añadir avisos…» y «Editar perfil».
	anio, ma := o.pasoAnio(v)
	ics := ui.NuevoBoton(o.t("obligations.ics"), o.AlICS).Con(ui.Secundario, ui.TallaSM)
	ics.ConIcono(iconos.Calendario, 14)
	ics.MargenIcono = HuecoUno
	ics.Inerte = o.AlICS == nil
	der := []pieza{piezaDe(anio, ma), piezaDe(ics, ics.MinSize())}
	if o.AlEditarPerfil != nil {
		ed := ui.NuevoBoton(o.t("obligations.editProfile"), o.AlEditarPerfil).Con(ui.Fantasma, ui.TallaSM)
		der = append(der, piezaDe(ed, ed.MinSize()))
	}
	anchos, altos := medidasDe(der)
	var anchoDer float32
	for i, a := range anchos {
		anchoDer += a
		if i > 0 {
			anchoDer += HuecoDos
		}
	}
	anchoIzq := ancho
	if o.sm() {
		anchoIzq = max(ancho-anchoDer-CabeceraObligGap, 0)
	}
	// La izquierda: el título con su insignia, que saltan si no caben, y la frase.
	titulo := ui.Texto(o.t("obligations.title"), tema.TextoLG, ui.SemiNegrita, tema.Color("text", v))
	izq := []pieza{{titulo, ui.Medir(titulo.Text, tema.TextoLG, ui.SemiNegrita).Width, tema.TextoLG.Linea, true}}
	if d := o.Datos; d != nil {
		rot, tono := o.t("obligations.indicative"), ui.InsigniaNeutra
		if d.Verificado {
			rot, tono = o.t("obligations.verified"), ui.InsigniaExito
		}
		izq = append(izq, piezaDe(ui.NuevaInsignia(rot, tono), ui.MedidaInsignia(rot)))
	}
	ai, hi := medidasDe(izq)
	pos, ocupa := Envolver(ai, hi, HuecoDos, HuecoDos, anchoIzq)
	l.piezas(izq, pos, 0, 0)
	frase := ui.NuevoParrafo(o.t("obligations.subtitle", i18n.Vars{"country": NombreDePais(o.Pais, o.loc)}),
		tema.TextoSM, ui.Normal, "text-muted", v)
	altoFrase := ui.AltoParrafo(frase, anchoIzq)
	yFrase := ocupa.Height + HuecoUno
	l.poner(frase, 0, yFrase, anchoIzq, altoFrase)
	altoIzq := yFrase + altoFrase
	var alto float32
	if o.sm() {
		posD, ocupaD := Envolver(anchos, altos, HuecoDos, HuecoDos, anchoDer)
		l.piezas(der, posD, ancho-anchoDer, 0)
		alto = max(altoIzq, ocupaD.Height)
	} else {
		posD, ocupaD := Envolver(anchos, altos, HuecoDos, HuecoDos, ancho)
		l.piezas(der, posD, 0, altoIzq+CabeceraObligGap)
		alto = altoIzq + CabeceraObligGap + ocupaD.Height
	}
	alto += CabeceraPB
	l.poner(canvas.NewRectangle(tema.Color("border", v)), 0, alto, ancho, tema.Borde)
	return l.objeto(alto + tema.Borde)
}

// pasoAnio es el grupo `‹ 2026 ›` con su borde, y lo que mide.
func (o *Obligaciones) pasoAnio(v tema.Variante) (fyne.CanvasObject, fyne.Size) {
	a := o.Anio
	menos := NuevoPulsador("", tema.TextoSM, ui.Normal, func() { o.irAnio(a - 1) })
	menos.Nombre, menos.Icono, menos.PX, menos.PY = strconv.Itoa(a-1), iconos.ChevronIzquierda, AnioPX, AnioPY
	mas := NuevoPulsador("", tema.TextoSM, ui.Normal, func() { o.irAnio(a + 1) })
	mas.Nombre, mas.Icono, mas.PX, mas.PY = strconv.Itoa(a+1), iconos.ChevronDerecha, AnioPX, AnioPY
	texto, wt := textoTabular(strconv.Itoa(a), tema.TextoSM, ui.Medio, tema.Color("text", v))
	mm := menos.MinSize()
	at := wt + 2*AnioTextoPX
	ancho, alto := 2*tema.Borde+2*mm.Width+at, mm.Height+2*tema.Borde
	caja := ui.NuevaCaja(tema.Color("bg", v), tema.Color("border", v), tema.RadioMD)
	l := &lienzo{}
	l.caja(caja, ancho, alto)
	l.poner(menos, tema.Borde, tema.Borde, mm.Width, mm.Height)
	l.poner(texto, tema.Borde+mm.Width+AnioTextoPX, (alto-tema.TextoSM.Linea)/2, wt, tema.TextoSM.Linea)
	l.poner(mas, tema.Borde+mm.Width+at, tema.Borde, mm.Width, mm.Height)
	ob, _ := l.objeto(alto)
	return ob, fyne.NewSize(ancho, alto)
}

// CifraTabular es el avance de una cifra con `tabular-nums` en Figtree: 0,625 em, medido
// en el año de Obligaciones (28,45 para «2026» a 11,375).
const CifraTabular float32 = 0.625

// anchoTabular es lo que mide un texto con `tabular-nums`: las cifras a su avance fijo y
// lo demás como siempre.
func anchoTabular(s string, e tema.Escalon, p ui.Peso) float32 {
	var cifras int
	resto := strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			cifras++
			return -1
		}
		return r
	}, s)
	return float32(cifras)*CifraTabular*e.Tam + ui.Medir(resto, e, p).Width
}

func (o *Obligaciones) irAnio(a int) {
	if o.AlAnio != nil {
		o.AlAnio(a)
	}
}

// ambitos son las píldoras «Todas · Fiscal · Laboral…».
func (o *Obligaciones) ambitos(amb [][2]string, ancho float32) (fyne.CanvasObject, float32) {
	ps := make([]pieza, 0, 1+len(amb))
	for _, a := range append([][2]string{{"", o.t("obligations.scope.all")}}, amb...) {
		clave := a[0]
		p := NuevaPildora(a[1], o.Ambito == clave, func() {
			o.Ambito = clave
			o.cambio()
		})
		p.Ligera = true
		ps = append(ps, piezaDe(p, MedidaPildora(p)))
	}
	an, al := medidasDe(ps)
	pos, ocupa := Envolver(an, al, HuecoPildorasOblig, HuecoPildorasOblig, ancho)
	l := &lienzo{}
	l.piezas(ps, pos, 0, 0)
	return l.objeto(ocupa.Height)
}

// mes es el título del mes en versalitas y su lista.
func (o *Obligaciones) mes(v tema.Variante, m mes, ancho float32) (fyne.CanvasObject, float32) {
	p := &pila{}
	titulo := ui.NuevoVersal(m.titulo, tema.TextoXS, ui.SemiNegrita, tema.Color("text-subtle", v))
	p.add(titulo.Objeto(), tema.TextoXS.Linea, 0)
	filas := make([]bloque, 0, len(m.filas))
	for _, ob := range m.filas {
		f, alto := o.fila(v, ob, ancho-2*tema.Borde)
		filas = append(filas, bloque{f, alto, 0})
	}
	lista, alto := listaEnCaja(v, filas, radioTarjeta())
	p.add(lista, alto, HuecoDos)
	return p.objeto()
}

// listaEnCaja es `ul.rounded-*.border.divide-y`.
func listaEnCaja(v tema.Variante, filas []bloque, radio float32) (fyne.CanvasObject, float32) {
	caja := ui.NuevaCaja(tema.Color("bg", v), tema.Color("border", v), radio)
	col := &columnaFilas{}
	var hijos []fyne.CanvasObject
	for i, f := range filas {
		col.filas = append(col.filas, f)
		hijos = append(hijos, f.obj)
		if i > 0 {
			d := canvas.NewRectangle(tema.Color("border", v))
			col.filetes = append(col.filetes, d)
			hijos = append(hijos, d)
		}
	}
	lista := container.New(col, hijos...)
	return container.New(&enCaja{caja: caja, hijo: lista}, append(caja.Objetos(), lista)...), col.alto() + 2*tema.Borde
}

// vencidas es el `details` del final: «N ya vencidas» y, desplegado, sus meses.
func (o *Obligaciones) vencidas(v tema.Variante, meses []mes, ancho float32) (fyne.CanvasObject, float32) {
	return o.mesesPlegados(v, meses, ancho, "vencidas", "obligations.overdueSection")
}

func (o *Obligaciones) mesesPlegados(v tema.Variante, meses []mes, ancho float32, clave, rotulo string) (fyne.CanvasObject, float32) {
	n := 0
	for _, m := range meses {
		n += len(m.filas)
	}
	abierto := o.Abiertos[clave]
	s := NuevoPulsador(o.t(rotulo, i18n.Vars{"count": n}), tema.TextoSM, ui.Medio, func() {
		o.Abiertos[clave] = !o.Abiertos[clave]
		o.cambio()
	})
	s.PX, s.PY, s.Marca, s.Radio = FilaOblPX, FilaOblPY, marcaDe(abierto), radioTarjeta()
	ms := s.MinSize()
	l := &lienzo{}
	caja := ui.NuevaCaja(tema.Color("bg", v), tema.Color("border", v), radioTarjeta())
	interior := ancho - 2*tema.Borde
	alto := tema.Borde + ms.Height
	var dentro []fyne.CanvasObject
	var altos []float32
	if abierto {
		for i, m := range meses {
			ob, a := o.mes(v, m, interior-2*FilaOblPX)
			if i > 0 {
				alto += ObligGap
			}
			dentro, altos = append(dentro, ob), append(altos, a)
			alto += a
		}
		alto += FilaOblPX // el `pb-4`
	}
	alto += tema.Borde
	l.caja(caja, ancho, alto)
	l.poner(s, tema.Borde, tema.Borde, interior, ms.Height)
	y := tema.Borde + ms.Height
	for i, ob := range dentro {
		if i > 0 {
			y += ObligGap
		}
		l.poner(ob, tema.Borde+FilaOblPX, y, interior-2*FilaOblPX, altos[i])
		y += altos[i]
	}
	return l.objeto(alto)
}

// fila es una presentación.
func (o *Obligaciones) fila(v tema.Variante, ob api.ERPObligacion, ancho float32) (fyne.CanvasObject, float32) {
	interior := ancho - 2*FilaOblPX
	der, anchoDer, altoDer := o.derecha(v, ob, interior)
	anchoIzq := interior
	if o.sm() {
		anchoIzq = max(interior-anchoDer-HuecoColumnasObl, 0)
	}
	izq, altoIzq := o.izquierda(v, ob, anchoIzq)
	l := &lienzo{}
	l.poner(izq, FilaOblPX, FilaOblPY, anchoIzq, altoIzq)
	alto := max(altoIzq, altoDer)
	if o.sm() {
		l.poner(der, FilaOblPX+interior-anchoDer, FilaOblPY, anchoDer, altoDer)
	} else {
		l.poner(der, FilaOblPX, FilaOblPY+altoIzq+HuecoDos, interior, altoDer)
		alto = altoIzq + HuecoDos + altoDer
	}
	return l.objeto(alto + 2*FilaOblPY)
}

// derecha es la columna de las fechas y los mandos: `items-end` en escritorio,
// `items-start` en el móvil. Devuelve también su ancho, que es el de su pieza más ancha.
func (o *Obligaciones) derecha(v tema.Variante, ob api.ERPObligacion, interior float32) (fyne.CanvasObject, float32, float32) {
	hoy := o.hoy()
	fecha := func(t time.Time) string { return i18n.FormatDayMonth(o.loc, t.UTC()) }
	// «Tu fecha 27 sept» y su insignia.
	rot, tono := o.cuanto(DiasHasta(ob.Limite, hoy), DiasHasta(ob.Recomendada, hoy))
	tuFecha := textoPieza(o.t("obligations.yourDate")+" "+fecha(ob.Recomendada), tema.TextoSM, ui.SemiNegrita, "text", v)
	ins := piezaDe(ui.NuevaInsignia(rot, tono), ui.MedidaInsignia(rot))
	an1 := tuFecha.ancho + HuecoFechaInsignia + ins.ancho
	al1 := max(tuFecha.alto, ins.alto)
	// Las fechas de debajo.
	var lineas []pieza
	if ob.LimiteDomiciliacion != nil {
		lineas = append(lineas, textoPieza(o.t("obligations.directDebit", i18n.Vars{"date": fecha(*ob.LimiteDomiciliacion)}),
			tema.TextoXS, ui.Normal, "text-subtle", v))
	}
	lineas = append(lineas, textoPieza(o.t("obligations.legalDeadline", i18n.Vars{"date": fecha(ob.Limite)}),
		tema.TextoXS, ui.Normal, "text-subtle", v))
	// «Preparar cifras» y «Abrir el portal».
	var mandos []pieza
	if ob.Calculable {
		c := o.Calculos[IDObligacion(ob)]
		b := ui.NuevoBoton(o.t("obligations.prepare"), func() {
			if o.AlPreparar != nil {
				o.AlPreparar(ob)
			}
		}).Con(ui.Secundario, ui.TallaXS)
		b.Cargando = c != nil && c.Cargando
		b.Inerte = o.AlPreparar == nil
		mandos = append(mandos, piezaDe(b, b.MinSize()))
	}
	url := ob.Portal.URL
	portal := NuevoPulsador(o.t("obligations.portal"), tema.TextoXS, ui.Medio, func() {
		if o.AlPortal != nil {
			o.AlPortal(url)
		}
	})
	portal.Icono, portal.PX, portal.PY, portal.Radio = iconos.FlechaArribaDerecha, PortalPX, (AltoPortal-tema.TextoXS.Linea)/2, tema.Forma("radius-control")
	portal.LadoIcono = 12
	mandos = append(mandos, piezaDe(portal, portal.MinSize()))
	var an4, al4 float32
	for i, m := range mandos {
		an4 += m.ancho
		if i > 0 {
			an4 += HuecoUno
		}
		al4 = max(al4, m.alto)
	}
	ancho := max(an1, an4)
	for _, p := range lineas {
		ancho = max(ancho, p.ancho)
	}
	sm := o.sm()
	x := func(w float32) float32 {
		if sm {
			return ancho - w
		}
		return 0
	}
	l := &lienzo{}
	x0 := x(an1)
	l.pieza(tuFecha, x0, (al1-tuFecha.alto)/2)
	l.pieza(ins, x0+tuFecha.ancho+HuecoFechaInsignia, (al1-ins.alto)/2)
	y := al1
	for _, p := range lineas {
		y += HuecoUno
		l.pieza(p, x(p.ancho), y)
		y += p.alto
	}
	y += 2 * HuecoUno // el `gap-1` y el `mt-1`
	xm := x(an4)
	for _, m := range mandos {
		l.pieza(m, xm, y) // `flex` sin `items-center`: arriba
		xm += m.ancho + HuecoUno
	}
	y += al4
	ob2, _ := l.objeto(y)
	if !sm {
		ancho = interior
	}
	return ob2, ancho, y
}

// cuanto es la insignia de «tu fecha»: lo que queda o lo que se pasó, y su tono.
func (o *Obligaciones) cuanto(dias, diasTu int) (string, ui.Tono) {
	tono := ui.InsigniaNeutra
	switch {
	case dias < 0 || diasTu < 0:
		tono = ui.InsigniaPeligro
	case diasTu <= 7:
		tono = ui.InsigniaAviso
	}
	switch {
	case dias < 0:
		return o.t("obligations.overdue", i18n.Vars{"count": -dias}), tono
	case diasTu == 0:
		return o.t("obligations.today"), tono
	case diasTu < 0 && dias == 0:
		return o.t("obligations.lastDay"), tono
	case diasTu < 0:
		return o.t("obligations.lastDays", i18n.Vars{"count": dias}), tono
	}
	return o.t("obligations.daysLeft", i18n.Vars{"count": diasTu}), tono
}

// izquierda es el nombre, qué es, lo que falta, la guía, los documentos, la sanción y
// la cifra preparada.
func (o *Obligaciones) izquierda(v tema.Variante, ob api.ERPObligacion, ancho float32) (fyne.CanvasObject, float32) {
	p := &pila{}
	// Nombre · periodo · ámbito, que saltan si no caben.
	cab := []pieza{
		textoPieza(ob.Nombre, tema.TextoSM, ui.SemiNegrita, "text", v),
		textoPieza("· "+RotuloDePeriodo(ob.Periodo, o.loc, o.tr), tema.TextoSM, ui.Normal, "text-muted", v),
		piezaDe(ui.NuevaInsignia(ob.AmbitoRotulo, ui.InsigniaNeutra), ui.MedidaInsignia(ob.AmbitoRotulo)),
	}
	if obligacionArchivada(ob) {
		cab = append(cab, piezaDe(ui.NuevaInsignia(o.t("process.archived"), ui.InsigniaExito), ui.MedidaInsignia(o.t("process.archived"))))
	}
	an, al := medidasDe(cab)
	pos, ocupa := Envolver(an, al, HuecoDos, HuecoDos, ancho)
	lc := &lienzo{}
	lc.piezas(cab, pos, 0, 0)
	oc, _ := lc.objeto(ocupa.Height)
	p.add(oc, ocupa.Height, 0)
	que := ob.ConceptoRotulo
	if ob.Guia != nil {
		que = ob.Guia.Que
	}
	q := ui.NuevoParrafo(que, tema.TextoXS, ui.Normal, "text-muted", v)
	p.add(q, ui.AltoParrafo(q, ancho), HuecoMedio)
	o2, alto := o.etapasProceso(v, ob, ancho)
	p.add(o2, alto, HuecoDos)
	// Lo que falta, o «Todo listo».
	if ob.Pendientes != nil && DiasHasta(ob.Limite, o.hoy()) >= 0 && !obligacionArchivada(ob) {
		o2, alto := o.estado(v, ob, ancho)
		p.add(o2, alto, HuecoUno)
	}
	rotuloProceso := "process.start"
	if obligacionArchivada(ob) {
		rotuloProceso = "process.viewReceipt"
	} else if ob.Proceso != nil && ob.Proceso.Fase > 0 {
		rotuloProceso = "process.resume"
	}
	b := ui.NuevoBoton(o.t(rotuloProceso), func() {
		if o.AlProceso != nil {
			o.AlProceso(ob)
		}
	}).Con(ui.Secundario, ui.TallaSM)
	b.Inerte = o.AlProceso == nil
	p.add(b, b.MinSize().Height, HuecoDos)
	o.detallesIzquierda(p, v, ob, ancho)
	return p.objeto()
}

// detallesIzquierda añade guía, documentos, sanción y cifras después del proceso.
func (o *Obligaciones) detallesIzquierda(p *pila, v tema.Variante, ob api.ERPObligacion, ancho float32) {
	id := IDObligacion(ob)
	if ob.Guia != nil {
		o2, alto := o.plegable("guia|"+id, o.t("obligations.howTo"), ancho, nil)
		p.add(o2, alto, HuecoUno)
		if o.Abiertos["guia|"+id] {
			o2, alto := o.guia(v, ob.Guia, min(ancho, ProsaObligaciones))
			p.add(o2, alto, HuecoDos)
		}
	}
	if len(ob.Documentos) > 0 {
		o2, alto := o.plegable("docs|"+id, o.t("obligations.documents"), ancho, func() {
			if o.AlDocumentos != nil {
				o.AlDocumentos(ob)
			}
		})
		p.add(o2, alto, HuecoUno)
		if o.Abiertos["docs|"+id] {
			o2, alto := o.documentos(v, ob, min(ancho, ProsaDocumentos))
			p.add(o2, alto, HuecoDos)
		}
	}
	moneda := "EUR"
	if o.Datos != nil && o.Datos.Moneda != "" {
		moneda = o.Datos.Moneda
	}
	if s := DescribirSancion(ob.Sancion, moneda, o.loc, o.tr); s != "" {
		o2, alto := o.sancion(v, ob, s, ancho)
		p.add(o2, alto, HuecoUno)
	}
	if c := o.Calculos[id]; c != nil && !c.Cargando && !c.Fallo {
		o2, alto := o.cifra(v, ob, c.R, moneda, ancho)
		p.add(o2, alto, HuecoDos)
	}
}

// estado es «Todo listo en Sytue» o lo que falta, con «Ir a Banco» si es el banco.
func (o *Obligaciones) estado(v tema.Variante, ob api.ERPObligacion, ancho float32) (fyne.CanvasObject, float32) {
	faltas := Faltas(ob, o.tr)
	l := &lienzo{}
	if len(faltas) == 0 {
		ps := conIcono(iconos.Tic, o.t("obligations.ready"), "success-text", v)
		l.pieza(ps, 0, 0)
		return l.objeto(tema.TextoXS.Linea)
	}
	var ps []pieza
	banco := false
	for _, f := range faltas {
		ps = append(ps, conIcono(iconos.Aviso, f.Texto, "warning-text", v))
		banco = banco || f.Clave == "banco"
	}
	if banco && o.AlBanco != nil {
		b := ui.NuevoEnlace(o.t("obligations.goToBank"), tema.TextoXS, ui.Medio, "text", o.AlBanco)
		b.Boton = true
		ps = append(ps, piezaDe(b, fyne.NewSize(ui.Medir(b.Texto, tema.TextoXS, ui.Medio).Width, tema.TextoXS.Linea)))
	}
	an, al := medidasDe(ps)
	pos, ocupa := Envolver(an, al, HuecoFaltasX, HuecoUno, ancho)
	l.piezas(ps, pos, 0, 0)
	return l.objeto(ocupa.Height)
}

// conIcono es un icono de 12 y su frase con `gap-1`, en la tinta del tono.
func conIcono(i iconos.Icono, texto, token string, v tema.Variante) pieza {
	ic := iconos.Trazo(i, 12, token, v)
	t := ui.Texto(texto, tema.TextoXS, ui.Normal, tema.Color(token, v))
	w := ui.Medir(texto, tema.TextoXS, ui.Normal).Width
	l := &lienzo{}
	l.poner(ic.Objeto(), 0, (tema.TextoXS.Linea-12)/2, 12, 12)
	l.texto(t, 12+HuecoUno, 0)
	ob, _ := l.objeto(tema.TextoXS.Linea)
	return pieza{ob, 12 + HuecoUno + w, tema.TextoXS.Linea, false}
}

// plegable es el `<summary>` de un `<details>`: «Cómo se hace», «Documentos…».
func (o *Obligaciones) plegable(clave, texto string, ancho float32, alAbrir func()) (fyne.CanvasObject, float32) {
	abierto := o.Abiertos[clave]
	s := NuevoPulsador(texto, tema.TextoXS, ui.Medio, func() {
		o.Abiertos[clave] = !o.Abiertos[clave]
		if o.Abiertos[clave] && alAbrir != nil {
			alAbrir()
		}
		o.cambio()
	})
	s.Marca, s.Radio = marcaDe(abierto), tema.RadioSM
	l := &lienzo{}
	l.poner(s, 0, 0, ancho, tema.TextoXS.Linea)
	return l.objeto(tema.TextoXS.Linea)
}

func marcaDe(abierto bool) Marca {
	if abierto {
		return MarcaAbierta
	}
	return MarcaCerrada
}

// guia es «Qué necesitas», «Pasos» y «Si llega tarde» (`space-y-2 text-sm`).
func (o *Obligaciones) guia(v tema.Variante, g *api.ERPGuia, ancho float32) (fyne.CanvasObject, float32) {
	p := &pila{}
	bloque := func(titulo string, cuerpo fyne.CanvasObject, alto float32) {
		arriba := HuecoDos
		if len(p.bloques) == 0 {
			arriba = 0
		}
		t := ui.Texto(titulo, tema.TextoXS, ui.SemiNegrita, tema.Color("text", v))
		p.add(textoSolo(t), tema.TextoXS.Linea, arriba)
		p.add(cuerpo, alto, HuecoUno)
	}
	if len(g.Necesitas) > 0 {
		ob, alto := listaMarcada(g.Necesitas, false, tema.TextoSM, v, ancho)
		bloque(o.t("obligations.need"), ob, alto)
	}
	ob, alto := listaMarcada(g.Pasos, true, tema.TextoSM, v, ancho)
	bloque(o.t("obligations.steps"), ob, alto)
	si := ui.NuevoParrafo(g.SiNo, tema.TextoSM, ui.Normal, "text-muted", v)
	bloque(o.t("obligations.ifLate"), si, ui.AltoParrafo(si, ancho))
	return p.objeto()
}

// documentos son los ficheros de cada documento que pide la presentación
// (`space-y-4`).
func (o *Obligaciones) documentos(v tema.Variante, ob api.ERPObligacion, ancho float32) (fyne.CanvasObject, float32) {
	p := &pila{}
	for i, d := range ob.Documentos {
		reg := RegistroDocumento(ob, d.Clave)
		adj, ok := o.Adjuntos[reg]
		if !ok {
			adj = nil
		}
		b := &Documentos{
			tr: o.tr, loc: o.loc, Titulo: d.Nombre, Requisitos: d.Requisitos, Adjuntos: adj,
			Escribe: !o.SoloLectura, Ancho: ancho, AlAbrir: o.AlAbrirAdjunto,
			AlBorrar: func(a api.ERPAdjunto) {
				if o.AlBorrarAdj != nil {
					o.AlBorrarAdj(reg, a)
				}
			},
		}
		if o.AlSubirDoc != nil {
			b.AlSubir = func() { o.AlSubirDoc(reg) }
		}
		var arriba float32 = 4 * 0.25 * tema.Rem // el `space-y-4`
		if i == 0 {
			arriba = 0
		}
		x, alto := b.Objeto(v)
		p.add(x, alto, arriba)
	}
	return p.objeto()
}

// sancion es «Fuera de plazo: …» (`text-xs text-subtle`, con el rótulo en `font-medium`)
// y, si hay letra pequeña, su `details` en línea.
func (o *Obligaciones) sancion(v tema.Variante, ob api.ERPObligacion, frase string, ancho float32) (fyne.CanvasObject, float32) {
	// Cada palabra es una pieza y el espacio es el hueco: así se reparte en líneas con
	// dos pesos en la misma frase, que `Parrafo` no hace.
	var ps []pieza
	for i, w := range strings.Fields(o.t("obligations.penalty") + ": " + frase) {
		peso := ui.Normal
		if i < len(strings.Fields(o.t("obligations.penalty")+":")) {
			peso = ui.Medio
		}
		ps = append(ps, textoPieza(w, tema.TextoXS, peso, "text-subtle", v))
	}
	nota := ""
	if ob.Sancion != nil {
		nota = ob.Sancion.Nota
	}
	clave := "nota|" + IDObligacion(ob)
	abierto := o.Abiertos[clave]
	if nota != "" {
		s := o.letraPequena(clave, abierto)
		m := s.MinSize()
		l := &lienzo{}
		anchoNota, altoNota := m.Width, m.Height
		l.poner(s, 0, HuecoMedio, m.Width, m.Height)
		if abierto {
			anchoNota = min(ancho, ProsaNota)
			n := ui.NuevoParrafo(nota, tema.TextoXS, ui.Normal, "text-subtle", v)
			h := ui.AltoParrafo(n, anchoNota)
			l.poner(n, 0, HuecoMedio+m.Height, anchoNota, h)
			altoNota += h
		}
		box, _ := l.objeto(altoNota + HuecoMedio)
		ps = append(ps, pieza{box, anchoNota, altoNota + HuecoMedio, false})
	}
	an, al := medidasDe(ps)
	pos, ocupa := Envolver(an, al, ui.Medir(" ", tema.TextoXS, ui.Normal).Width, 0, ancho)
	// inline-block alinea la última línea por la base: la letra pequeña no
	// fuerza una fila nueva y el texto que comparte su fila baja hasta su base.
	for i := range pos {
		for j := range pos {
			if !ps[j].texto && pos[i].Y >= pos[j].Y && pos[i].Y < pos[j].Y+al[j] {
				if ps[i].texto {
					pos[i].Y = pos[j].Y + al[j] - al[i]
				}
			}
		}
	}
	l := &lienzo{}
	l.piezas(ps, pos, 0, 0)
	alto := ocupa.Height
	return l.objeto(alto)
}

func (o *Obligaciones) letraPequena(clave string, abierto bool) *Pulsador {
	s := NuevoPulsador(o.t("obligations.penaltyNote"), tema.TextoXS, ui.Normal, func() {
		o.Abiertos[clave] = !o.Abiertos[clave]
		o.cambio()
	})
	s.Marca, s.Punteado, s.Radio = marcaDe(abierto), true, tema.RadioSM
	s.Token, s.TokenEncima = "text-subtle", "text-subtle"
	return s
}

// cifra es el recuadro de «Preparar cifras»: el periodo, el importe y lo que costaría
// un mes de retraso.
func (o *Obligaciones) cifra(v tema.Variante, ob api.ERPObligacion, c api.ERPCalculoObligacion, moneda string, ancho float32) (fyne.CanvasObject, float32) {
	dentro := ancho - 2*CalculoPX
	p := &pila{}
	add := func(texto string, e tema.Escalon, peso ui.Peso, token string, arriba float32) {
		x := ui.NuevoParrafo(texto, e, peso, token, v)
		p.add(x, ui.AltoParrafo(x, dentro), arriba)
	}
	add(o.t("obligations.prepared", i18n.Vars{"from": c.Desde, "to": c.Hasta}), tema.TextoXS, ui.Normal, "text-muted", 0)
	if c.Asentado {
		add(o.t("obligations.result")+": "+Dinero(o.loc, c.ResultadoCentimos, moneda), tema.TextoSM, ui.SemiNegrita, "text", HuecoMedio)
		if n, ok := EstimarUnMes(ob.Sancion, c.ResultadoCentimos); ok {
			divisa := moneda
			if ob.Sancion.Moneda != "" {
				divisa = ob.Sancion.Moneda
			}
			add(o.t("obligations.penaltyEstimate", i18n.Vars{"amount": Dinero(o.loc, n, divisa)}),
				tema.TextoXS, ui.Normal, "text-subtle", HuecoMedio)
		}
	} else {
		add(o.t("obligations.notPosted"), tema.TextoXS, ui.Normal, "text-muted", HuecoMedio)
	}
	cuerpo, altoCuerpo := p.objeto()
	alto := altoCuerpo + 2*CalculoPY
	l := &lienzo{}
	l.caja(ui.NuevaCaja(tema.Color("bg-subtle", v), sinColor, tema.RadioMD), ancho, alto)
	l.poner(cuerpo, CalculoPX, CalculoPY, dentro, altoCuerpo)
	return l.objeto(alto)
}

// listaMarcada es `ul.list-disc` u `ol.list-decimal` con `pl-5 space-y-0.5`: la viñeta
// o el número por fuera, a la izquierda del texto.
//
// ponytail: la viñeta es un círculo de 0,35 em a 0,845 em del texto, medido en Chrome
// a `text-sm`; el navegador la dibuja con la métrica de la letra y puede quedar a
// medio píxel en otro escalón.
func listaMarcada(items []string, ordenada bool, e tema.Escalon, v tema.Variante, ancho float32) (fyne.CanvasObject, float32) {
	const token = "text-muted" // `text-text-muted`: el de todas las listas de la web
	l := &lienzo{}
	tinta := tema.Color(token, v)
	dentro := max(ancho-ListaPL, 0)
	var y float32
	for i, s := range items {
		if i > 0 {
			y += HuecoMedio
		}
		par := ui.NuevoParrafo(s, e, ui.Normal, token, v)
		h := ui.AltoParrafo(par, dentro)
		l.poner(par, ListaPL, y, dentro, h)
		if ordenada {
			n := strconv.Itoa(i+1) + "."
			w := ui.Medir(n, e, ui.Normal).Width
			l.texto(ui.Texto(n, e, ui.Normal, tinta), ListaPL-ui.Medir(" ", e, ui.Normal).Width-w, y)
		} else {
			d := 0.35 * e.Tam
			c := canvas.NewCircle(tinta)
			l.poner(c, ListaPL-0.845*e.Tam-d, y+e.Linea/2+0.035*e.Tam-d/2, d, d)
		}
		y += h
	}
	return l.objeto(y)
}
