// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package erp

import (
	"slices"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"golang.org/x/text/collate"
	"golang.org/x/text/language"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// # El asistente de arranque (`Onboarding.tsx`)
//
// País, forma jurídica, datos del emisor y situación: de esas respuestas sale el
// calendario de obligaciones y la lista de áreas que conviene encender. Cinco pasos
// cortos y no un formulario, dentro del `Wizard` de la suite. Medido en el arnés
// (1280×800, 2026-09-23):
//
//	página     `max-w-4xl` (742 útiles), título `text-lg` con `mb-4`
//	asistente  `gap-6`: los pasos (24,5), el encabezado `sytue-h3` (26,25), el contenido
//	           (`max-w-xl`, 504) y el pie `border-t pt-4` con «Ahora no», «Atrás» y
//	           «Continuar» (`md`, 31,5)
//	país       el campo (52,5) y, con `gap-4`, la insignia y la frase del calendario
//	forma      dos tarjetas de elección (82,28, `sm:grid-cols-2 gap-3`) y la forma
//	datos      cinco campos (`gap-4`); código postal y ciudad en `1fr 2fr`
//	situación  los conmutadores con `gap-3`
//	revisar    la lista `rounded-md border` con filas de 37,75 (`px-3 py-2.5`)

// Medidas del asistente.
const (
	// AnchoAsistente es `max-w-4xl` y AnchoPasoAsistente el `max-w-xl` del contenido.
	AnchoAsistente     float32 = 56 * tema.Rem
	AnchoPasoAsistente float32 = 36 * tema.Rem
	// HuecoCampos es el `gap-4` de los campos; HuecoConmutadores el `gap-3`.
	HuecoCampos       float32 = 4 * 0.25 * tema.Rem
	HuecoConmutadores float32 = 3 * 0.25 * tema.Rem
	// PieAsistentePT es el `pt-4` del pie.
	PieAsistentePT float32 = 4 * 0.25 * tema.Rem
	// RevisionPX y RevisionPY son el `px-3 py-2.5` de una fila del resumen, y
	// RotuloRevision el `minmax(0,11rem)` de su rótulo.
	RevisionPX     float32 = 3 * 0.25 * tema.Rem
	RevisionPY     float32 = 2.5 * 0.25 * tema.Rem
	RotuloRevision float32 = 11 * tema.Rem
	// BajaRevision alinea text-sm con la línea base del dd sin clase (22px).
	// Chromium, ReviewSummary real y rem16: el botón queda 3px bajo el dd.
	BajaRevision float32 = 3
	// HuecoListo es el `gap-5` del último paso.
	HuecoListo float32 = 5 * 0.25 * tema.Rem
)

// Los pasos, en su orden.
const (
	PasoPais = iota
	PasoForma
	PasoDatos
	PasoSituacion
	PasoRevisar
	PasoListo
)

// Condiciones son los interruptores del perfil, en el orden de la web.
var Condiciones = []string{"iva", "iva_mensual", "empleados", "intracomunitario", "alquiler"}

var rotuloCondicion = map[string]string{
	"iva": "onboarding.situation.iva", "iva_mensual": "onboarding.situation.ivaMonthly",
	"empleados": "onboarding.situation.employees", "intracomunitario": "onboarding.situation.intra",
	"alquiler": "onboarding.situation.rent",
}

// paisPorIdioma es el país que se propone según el idioma: lo más probable, no una certeza.
var paisPorIdioma = map[i18n.Locale]string{"es": "ES", "fr": "FR", "de": "DE", "it": "IT", "en": "IE"}

// ModulosRecomendados son las áreas que un negocio como este enciende el primer día.
func ModulosRecomendados(tipo string, empleados bool) []string {
	base := []string{"contactos", "facturacion", "contabilidad", "gastos"}
	if tipo == "sociedad" {
		base = []string{"contactos", "ventas", "compras", "facturacion", "contabilidad", "gastos"}
	}
	if empleados {
		base = append(base, "rrhh", "nominas")
	}
	return base
}

// Asistente es el estado del asistente. Los campos y los conmutadores son widgets que
// duran lo que el asistente —no se rehacen al repintar— para no perder el foco ni el
// cursor mientras se escribe.
type Asistente struct {
	tr  *i18n.Translator
	loc i18n.Locale

	// Paises son los que conoce el servicio; nil, cargando.
	Paises []api.ERPPais
	Paso   int
	Pais   string
	Tipo   string
	Forma  string
	Cond   map[string]bool
	// Tocado enseña el error de la razón social; Ocupado es el guardado en curso.
	Tocado, Ocupado bool
	// Total son las obligaciones del año que salieron; -1, no se sabe.
	Total int
	// Anio es el del calendario que se acaba de montar.
	Anio int
	// Modulos y Activos son los del catálogo, para las áreas recomendadas.
	Modulos              []Modulo
	Activos              []string
	Activando, Activados bool
	Ventana              float32

	AlGuardar       func(o api.ERPOrganizacion)
	AlVerCalendario func()
	AlICS           func()
	AlActivar       func(claves []string)
	AlCancelar      func()
	AlCambio        func()

	selPais, selForma *ui.Selector
	campos            map[string]*ui.Entrada
	conmutadores      map[string]*ui.Conmutador
}

// NuevoAsistente lo abre vacío, o con el perfil que ya hay para editarlo.
func NuevoAsistente(tr *i18n.Translator, org *api.ERPOrganizacion, anio int) *Asistente {
	a := &Asistente{tr: tr, loc: tr.Locale(), Total: -1, Anio: anio, Cond: map[string]bool{"iva": true}}
	a.Pais = paisPorIdioma[a.loc]
	if a.Pais == "" {
		a.Pais = "ES"
	}
	a.campos = map[string]*ui.Entrada{}
	for _, c := range []string{"nombre", "nif", "direccion", "cp", "ciudad"} {
		e := ui.NuevaEntrada("")
		if c == "nombre" {
			e.AlCambiar(func(string) {
				if a.Tocado {
					a.cambio()
				}
			})
		}
		a.campos[c] = e
	}
	a.conmutadores = map[string]*ui.Conmutador{}
	for _, c := range Condiciones {
		clave := c
		a.conmutadores[c] = ui.NuevoConmutador(a.t(rotuloCondicion[c]), func(si bool) {
			a.Cond[clave] = si
			// Sin IVA no hay periodicidad de IVA que elegir.
			if clave == "iva" && !si {
				a.Cond["iva_mensual"] = false
			}
			a.cambio()
		})
	}
	if org != nil {
		a.Pais, a.Forma = org.Pais, org.Forma
		for c, v := range org.Condiciones {
			a.Cond[c] = v
		}
		for c, v := range map[string]string{"nombre": org.Nombre, "nif": org.NIF, "direccion": org.Direccion, "cp": org.CP, "ciudad": org.Ciudad} {
			a.campos[c].PonerTexto(v)
		}
	}
	return a
}

func (a *Asistente) t(clave string, vars ...i18n.Vars) string {
	if len(vars) > 0 {
		return a.tr.T(clave, vars[0])
	}
	return a.tr.T(clave, nil)
}

func (a *Asistente) cambio() {
	if a.AlCambio != nil {
		a.AlCambio()
	}
}

func (a *Asistente) sm() bool { return a.Ventana == 0 || a.Ventana >= ui.CorteSM }

// Cargados pone los países que llegaron y, al editar, el tipo de la forma guardada.
func (a *Asistente) Cargados(ps []api.ERPPais) {
	a.Paises = ps
	if f, ok := a.formaElegida(); ok {
		a.Tipo = f.Tipo
	}
}

func (a *Asistente) infoPais() (api.ERPPais, bool) {
	for _, p := range a.Paises {
		if p.Pais == a.Pais {
			return p, true
		}
	}
	return api.ERPPais{}, false
}

func (a *Asistente) formaElegida() (api.ERPFormaJuridica, bool) {
	p, _ := a.infoPais()
	for _, f := range p.Formas {
		if f.Clave == a.Forma {
			return f, true
		}
	}
	return api.ERPFormaJuridica{}, false
}

func (a *Asistente) formasDelTipo() []api.ERPFormaJuridica {
	p, _ := a.infoPais()
	var out []api.ERPFormaJuridica
	for _, f := range p.Formas {
		if f.Tipo == a.Tipo {
			out = append(out, f)
		}
	}
	return out
}

// ElegirPais cambia el país: una SL no existe en Alemania, así que la forma se olvida.
func (a *Asistente) ElegirPais(codigo string) { a.Pais, a.Forma = codigo, "" }

// ElegirTipo cambia el tipo y, si solo hay una forma de ese tipo, la elige.
func (a *Asistente) ElegirTipo(tipo string) {
	a.Tipo, a.Forma = tipo, ""
	if fs := a.formasDelTipo(); len(fs) == 1 {
		a.Forma = fs[0].Clave
	}
}

// Recomendados son las áreas que se proponen y que el catálogo tiene.
func (a *Asistente) Recomendados() []string {
	if a.Tipo == "" {
		return nil
	}
	var out []string
	for _, c := range ModulosRecomendados(a.Tipo, a.Cond["empleados"]) {
		if slices.ContainsFunc(a.Modulos, func(m Modulo) bool { return m.Clave == c }) {
			out = append(out, c)
		}
	}
	return out
}

// Pendientes son las recomendadas que aún están apagadas.
func (a *Asistente) Pendientes() []string {
	return slices.DeleteFunc(a.Recomendados(), func(c string) bool { return slices.Contains(a.Activos, c) })
}

// Perfil es lo que se guarda, con los textos recortados.
func (a *Asistente) Perfil() api.ERPOrganizacion {
	v := func(c string) string { return strings.TrimSpace(a.campos[c].Texto()) }
	cond := map[string]bool{}
	for k, s := range a.Cond {
		cond[k] = s
	}
	return api.ERPOrganizacion{
		Nombre: v("nombre"), NIF: v("nif"), Direccion: v("direccion"), CP: v("cp"), Ciudad: v("ciudad"),
		Pais: a.Pais, Forma: a.Forma, Condiciones: cond,
	}
}

func (a *Asistente) errNombre() string {
	if strings.TrimSpace(a.campos["nombre"].Texto()) == "" {
		return a.t("form.required")
	}
	return ""
}

// puedeSeguir es el `canContinue` del paso.
func (a *Asistente) puedeSeguir() bool {
	switch a.Paso {
	case PasoPais:
		_, ok := a.infoPais()
		return ok
	case PasoForma:
		_, ok := a.formaElegida()
		return ok
	}
	return true
}

// Seguir es «Continuar»: el `onNext` del paso o el siguiente.
func (a *Asistente) Seguir() {
	switch a.Paso {
	case PasoDatos:
		if a.errNombre() != "" {
			a.Tocado = true
			a.cambio()
			return
		}
	case PasoRevisar:
		if !a.Ocupado && a.AlGuardar != nil {
			a.Ocupado = true
			a.AlGuardar(a.Perfil())
		}
		a.cambio()
		return
	}
	if a.Paso < PasoListo {
		a.Paso++
	}
	a.cambio()
}

// --- La pantalla -------------------------------------------------------------

// Objeto arma el asistente.
func (a *Asistente) Objeto(v tema.Variante) fyne.CanvasObject {
	ancho := anchoUtilDe(a.Ventana, AnchoAsistente)
	p := &pila{}
	if a.Paises == nil {
		alto := 24 + 2*CargandoOblPY
		p.add(centrado(ui.NuevaRueda(24, "text-muted", v), alto), alto, 0)
		o, altoP := p.objeto()
		return enPagina(o, altoP, AnchoAsistente, MargenLista)
	}
	p.add(textoSolo(ui.Texto(a.t("onboarding.title"), tema.TextoLG, ui.SemiNegrita, tema.Color("text", v))), tema.TextoLG.Linea, 0)
	terminal := a.Paso == PasoListo
	pasos := ui.NuevosPasos(a.t("onboarding.step.country"), a.t("onboarding.step.form"), a.t("onboarding.step.details"),
		a.t("onboarding.step.situation"), a.t("onboarding.step.review"), a.t("onboarding.step.done"))
	pasos.Actual, pasos.Ventana = a.Paso, a.Ventana
	pasos.RotuloIr = func(n int) string { return a.t("ui.wizard.goToStep", i18n.Vars{"n": n}) }
	if !terminal && !a.Ocupado {
		pasos.AlElegir = func(i int) { a.Paso = i; a.cambio() }
	}
	p.add(pasos, ui.AltoPasos, HuecoCampos)
	encabezados := []string{
		"onboarding.country.heading", "onboarding.form.heading", "onboarding.details.heading",
		"onboarding.situation.heading", "onboarding.review.heading", "onboarding.done.heading",
	}
	h := ui.NuevoParrafo(a.t(encabezados[a.Paso]), ui.EscalonTitulo(3), ui.PesoTitulo(3), "text", v)
	p.add(h, ui.AltoParrafo(h, ancho), ObligGap)
	cw := min(ancho, AnchoPasoAsistente)
	var o fyne.CanvasObject
	var alto float32
	switch a.Paso {
	case PasoPais:
		o, alto = a.pasoPais(v, cw)
	case PasoForma:
		o, alto = a.pasoForma(cw)
	case PasoDatos:
		o, alto = a.pasoDatos(cw)
	case PasoSituacion:
		o, alto = a.pasoSituacion(cw)
	case PasoRevisar:
		o, alto = a.pasoRevisar(v, cw)
	default:
		o, alto = a.pasoListo(v, cw)
	}
	// El contenido va en su `max-w-xl`, no a todo el ancho de la pila.
	caja := &lienzo{}
	caja.poner(o, 0, 0, cw, alto)
	o, _ = caja.objeto(alto)
	p.add(o, alto, ObligGap)
	if !terminal {
		o, alto := a.pie(v, ancho)
		p.add(o, alto, ObligGap)
	}
	obj, altoP := p.objeto()
	return enPagina(obj, altoP, AnchoAsistente, MargenLista)
}

// ponerCampo es un campo con su rótulo, lo que mide y dónde va.
func ponerCampo(l *lienzo, g *fyne.Container, x, y, w float32) float32 {
	h := g.MinSize().Height
	l.poner(g, x, y, w, h)
	return h
}

func (a *Asistente) pasoPais(v tema.Variante, cw float32) (fyne.CanvasObject, float32) {
	orden := collate.New(language.Make(string(a.loc)))
	type opcion struct{ codigo, nombre string }
	ops := make([]opcion, 0, len(a.Paises))
	for _, p := range a.Paises {
		ops = append(ops, opcion{p.Pais, NombreDePais(p.Pais, a.loc)})
	}
	slices.SortFunc(ops, func(x, y opcion) int { return orden.CompareString(x.nombre, y.nombre) })
	nombres := make([]string, len(ops))
	elegida := -1
	for i, o := range ops {
		nombres[i] = o.nombre
		if o.codigo == a.Pais {
			elegida = i
		}
	}
	if a.selPais == nil {
		a.selPais = ui.NuevoSelector(nil, nil)
	}
	s := a.selPais
	s.Opciones, s.Elegida, s.Ancho = nombres, elegida, cw
	s.AlElegir = func(i int) { a.ElegirPais(ops[i].codigo); a.cambio() }
	l := &lienzo{}
	y := ponerCampo(l, ui.GrupoCampo(a.t("onboarding.step.country"), true, s, "", ""), 0, 0, cw)
	if info, ok := a.infoPais(); ok {
		rot, tono, frase := a.t("obligations.indicative"), ui.InsigniaNeutra, a.t("onboarding.country.indicative")
		if info.Verificado {
			rot, tono, frase = a.t("obligations.verified"), ui.InsigniaExito, a.t("onboarding.country.verified")
		}
		m := ui.MedidaInsignia(rot)
		y += HuecoCampos
		l.poner(ui.NuevaInsignia(rot, tono), 0, y, m.Width, m.Height)
		f := ui.NuevoParrafo(frase, tema.TextoSM, ui.Normal, "text-muted", v)
		wf := max(cw-m.Width-HuecoDos, 0)
		hf := ui.AltoParrafo(f, wf)
		l.poner(f, m.Width+HuecoDos, y, wf, hf)
		y += max(m.Height, hf)
	}
	return l.objeto(y)
}

func (a *Asistente) pasoForma(cw float32) (fyne.CanvasObject, float32) {
	l := &lienzo{}
	tipos := []struct {
		valor, titulo, cuerpo string
		icono                 iconos.Icono
	}{
		{"persona_fisica", "onboarding.form.individual", "onboarding.form.individualBody", iconos.Usuario},
		{"sociedad", "onboarding.form.company", "onboarding.form.companyBody", iconos.Edificio},
	}
	w := cw
	if a.sm() {
		w = (cw - HuecoTarjetas) / 2
	}
	var altoFila float32
	tarjetas := make([]*ui.TarjetaEleccion, len(tipos))
	for i, t := range tipos {
		valor := t.valor
		c := ui.NuevaTarjetaEleccion(a.t(t.titulo), a.t(t.cuerpo), func() { a.ElegirTipo(valor); a.cambio() }).A(w)
		c.Icono, c.Elegida = t.icono, a.Tipo == valor
		tarjetas[i] = c
		altoFila = max(altoFila, ui.AltoTarjetaEleccion(c, w))
	}
	var y float32
	for i, c := range tarjetas {
		if a.sm() {
			l.poner(c, float32(i)*(w+HuecoTarjetas), 0, w, altoFila)
			continue
		}
		h := ui.AltoTarjetaEleccion(c, w)
		l.poner(c, 0, y, w, h)
		y += h + HuecoTarjetas
	}
	if a.sm() {
		y = altoFila
	} else {
		y -= HuecoTarjetas
	}
	if a.Tipo != "" {
		fs := a.formasDelTipo()
		ops := make([]string, 0, 1+len(fs))
		ops = append(ops, a.t("relation.choose"))
		elegida := 0
		for i, f := range fs {
			ops = append(ops, f.Nombre)
			if f.Clave == a.Forma {
				elegida = i + 1
			}
		}
		if a.selForma == nil {
			a.selForma = ui.NuevoSelector(nil, nil)
		}
		s := a.selForma
		s.Opciones, s.Elegida, s.Ancho = ops, elegida, cw
		s.AlElegir = func(i int) {
			a.Forma = ""
			if i > 0 {
				a.Forma = fs[i-1].Clave
			}
			a.cambio()
		}
		y += HuecoCampos
		y += ponerCampo(l, ui.GrupoCampo(a.t("onboarding.form.specific"), true, s, "", ""), 0, y, cw)
	}
	return l.objeto(y)
}

func (a *Asistente) pasoDatos(cw float32) (fyne.CanvasObject, float32) {
	l := &lienzo{}
	campo := func(clave string, ancho float32) *ui.Entrada {
		e := a.campos[clave]
		e.Ancho = ancho
		return e
	}
	nombre := campo("nombre", cw)
	errNombre := ""
	if a.Tocado {
		errNombre = a.errNombre()
	}
	nombre.Invalida = errNombre != ""
	y := ponerCampo(l, ui.GrupoCampo(a.t("onboarding.details.name"), true, nombre, "", errNombre), 0, 0, cw)
	for _, c := range []struct{ clave, rotulo string }{{"nif", "onboarding.details.taxId"}, {"direccion", "onboarding.details.address"}} {
		y += HuecoCampos
		y += ponerCampo(l, ui.GrupoCampo(a.t(c.rotulo), false, campo(c.clave, cw), "", ""), 0, y, cw)
	}
	y += HuecoCampos
	// `grid sm:grid-cols-[1fr_2fr] gap-4`.
	if a.sm() {
		w1 := (cw - HuecoCampos) / 3
		h := ponerCampo(l, ui.GrupoCampo(a.t("onboarding.details.postcode"), false, campo("cp", w1), "", ""), 0, y, w1)
		ponerCampo(l, ui.GrupoCampo(a.t("onboarding.details.city"), false, campo("ciudad", 2*w1), "", ""), w1+HuecoCampos, y, 2*w1)
		y += h
	} else {
		y += ponerCampo(l, ui.GrupoCampo(a.t("onboarding.details.postcode"), false, campo("cp", cw), "", ""), 0, y, cw)
		y += HuecoCampos
		y += ponerCampo(l, ui.GrupoCampo(a.t("onboarding.details.city"), false, campo("ciudad", cw), "", ""), 0, y, cw)
	}
	return l.objeto(y)
}

func (a *Asistente) pasoSituacion(ancho float32) (fyne.CanvasObject, float32) {
	l := &lienzo{}
	var y float32
	primero := true
	for _, c := range Condiciones {
		if c == "iva_mensual" && !a.Cond["iva"] {
			continue
		}
		if !primero {
			y += HuecoConmutadores
		}
		primero = false
		s := a.conmutadores[c]
		s.Encendido = a.Cond[c]
		s.AnchoEtiqueta = max(ancho-ui.AnchoCarril-ui.HuecoConmutador, 1)
		m := s.MinSize()
		l.poner(s, 0, y, m.Width, m.Height)
		y += m.Height
	}
	return l.objeto(y)
}

// Revision son las filas del resumen: rótulo y valor.
func (a *Asistente) Revision() [][2]string {
	forma := SinValor
	if f, ok := a.formaElegida(); ok {
		forma = f.Nombre
	}
	o := a.Perfil()
	oGuion := func(s string) string {
		if s == "" {
			return SinValor
		}
		return s
	}
	var cond []string
	for _, c := range Condiciones {
		if a.Cond[c] {
			cond = append(cond, a.t(rotuloCondicion[c]))
		}
	}
	return [][2]string{
		{a.t("onboarding.step.country"), NombreDePais(a.Pais, a.loc)},
		{a.t("onboarding.form.specific"), forma},
		{a.t("onboarding.details.name"), oGuion(o.Nombre)},
		{a.t("onboarding.details.taxId"), oGuion(o.NIF)},
		{a.t("onboarding.step.situation"), oGuion(strings.Join(cond, " · "))},
	}
}

// pasoDeRevision es el paso al que vuelve el «Editar» de cada fila de Revision: el
// país, la forma, los datos (nombre y NIF) y la situación.
var pasoDeRevision = []int{PasoPais, PasoForma, PasoDatos, PasoDatos, PasoSituacion}

// pasoRevisar es `ReviewSummary`: la lista con cada dato, su rótulo y el «Editar» que
// devuelve al paso donde se rellenó (`text-sm font-semibold text-primary-text`,
// `hover:underline`).
func (a *Asistente) pasoRevisar(v tema.Variante, cw float32) (fyne.CanvasObject, float32) {
	filas := a.Revision()
	dentro := cw - 2*tema.Borde - 2*RevisionPX
	rotuloEditar := a.t("onboarding.review.edit")
	anchoEditar := ui.Medir(rotuloEditar, tema.TextoSM, ui.SemiNegrita).Width
	bloques := make([]bloque, 0, len(filas))
	for i, f := range filas {
		l := &lienzo{}
		wr := min(RotuloRevision, ui.Medir(f[0], tema.TextoSM, ui.Normal).Width)
		xv := wr + HuecoTarjetas
		if a.sm() {
			wr, xv = RotuloRevision, RotuloRevision+HuecoTarjetas
		}
		rot := ui.NuevoParrafo(f[0], tema.TextoSM, ui.Normal, "text-muted", v)
		hr := ui.AltoParrafo(rot, wr)
		esNIF := i == 3 && f[1] != SinValor
		wv := max(dentro-xv-HuecoTarjetas-anchoEditar, 0)
		paso := pasoDeRevision[i]
		editar := ui.NuevoEnlace(rotuloEditar, tema.TextoSM, ui.SemiNegrita, "primary-text", func() {
			a.Paso = paso
			a.cambio()
		})
		// El `<span class="sr-only">` con el rótulo: «Editar País».
		editar.Boton, editar.Nombre = true, rotuloEditar+" "+f[0]
		l.poner(editar, RevisionPX+dentro-anchoEditar, RevisionPY+BajaRevision, anchoEditar, tema.TextoSM.Linea)
		var hv float32
		if esNIF {
			t := canvas.NewText(f[1], tema.Color("text", v))
			t.TextSize, t.TextStyle = tema.TextoXS.Tam, fyne.TextStyle{Monospace: true}
			l.poner(t, RevisionPX+xv, RevisionPY+BajaRevision+1, wv, tema.TextoXS.Linea)
			hv = tema.TextoXS.Linea
		} else {
			val := ui.NuevoParrafo(f[1], tema.TextoSM, ui.Normal, "text", v)
			hv = ui.AltoParrafo(val, wv)
			l.poner(val, RevisionPX+xv, RevisionPY+BajaRevision, wv, hv)
		}
		l.poner(rot, RevisionPX, RevisionPY+BajaRevision, wr, hr)
		alto := 2*RevisionPY + max(tema.TextoRaiz.Linea, BajaRevision+max(hr, hv))
		o, _ := l.objeto(alto)
		bloques = append(bloques, bloque{o, alto, 0})
	}
	caja := ui.NuevaCaja(tema.Color("surface", v), tema.Color("border", v), tema.RadioMD)
	col := &columnaFilas{}
	var hijos []fyne.CanvasObject
	for i, b := range bloques {
		col.filas = append(col.filas, b)
		hijos = append(hijos, b.obj)
		if i > 0 {
			d := canvas.NewRectangle(tema.Color("border-subtle", v))
			col.filetes = append(col.filetes, d)
			hijos = append(hijos, d)
		}
	}
	lista := container.New(col, hijos...)
	return container.New(&enCaja{caja: caja, hijo: lista}, append(caja.Objetos(), lista)...), col.alto() + 2*tema.Borde
}

func (a *Asistente) pasoListo(v tema.Variante, cw float32) (fyne.CanvasObject, float32) {
	l := &lienzo{}
	var y float32
	if a.Total >= 0 {
		f := ui.NuevoParrafo(a.t("onboarding.done.count", i18n.Vars{"count": a.Total, "year": strconv.Itoa(a.Anio)}), tema.TextoBase, ui.Normal, "text", v)
		h := ui.AltoParrafo(f, cw)
		l.poner(f, 0, y, cw, h)
		y += h + HuecoListo
	}
	cal := ui.NuevoBoton(a.t("onboarding.done.calendar"), a.AlVerCalendario).Con(ui.Primario, ui.TallaMD)
	cal.ConIcono(iconos.Calendario, 14)
	cal.MargenIcono = HuecoUno
	ics := ui.NuevoBoton(a.t("onboarding.done.ics"), a.AlICS).Con(ui.Secundario, ui.TallaMD)
	ics.ConIcono(iconos.Descargar, 14)
	ics.MargenIcono = HuecoUno
	ps := []pieza{piezaDe(cal, cal.MinSize()), piezaDe(ics, ics.MinSize())}
	an, al := medidasDe(ps)
	pos, ocupa := Envolver(an, al, HuecoDos, HuecoDos, cw)
	l.piezas(ps, pos, 0, y)
	y += ocupa.Height
	if rec := a.Recomendados(); len(rec) > 0 {
		o, h := a.recomendadas(v, rec, cw)
		y += HuecoListo
		l.poner(o, 0, y, cw, h)
		y += h
	}
	return l.objeto(y)
}

// recomendadas es la tarjeta de las áreas que se proponen, con «Encender…».
func (a *Asistente) recomendadas(v tema.Variante, rec []string, cw float32) (fyne.CanvasObject, float32) {
	l := &lienzo{}
	x0 := tema.Borde + TarjetaPX
	dentro := cw - 2*x0
	y := x0
	l.texto(ui.Texto(a.t("onboarding.done.modules"), tema.TextoSM, ui.SemiNegrita, tema.Color("text", v)), x0, y)
	y += tema.TextoSM.Linea + HuecoUno
	cuerpo := ui.NuevoParrafo(a.t("onboarding.done.modulesBody"), tema.TextoSM, ui.Normal, "text-muted", v)
	h := ui.AltoParrafo(cuerpo, dentro)
	l.poner(cuerpo, x0, y, dentro, h)
	y += h + HuecoTarjetas
	ps := make([]pieza, 0, len(rec))
	for _, c := range rec {
		nombre := c
		for _, m := range a.Modulos {
			if m.Clave == c {
				nombre = m.Etiqueta
			}
		}
		tono := ui.InsigniaNeutra
		if a.Activados || slices.Contains(a.Activos, c) {
			tono = ui.InsigniaExito
		}
		ps = append(ps, piezaDe(ui.NuevaInsignia(nombre, tono), ui.MedidaInsignia(nombre)))
	}
	an, al := medidasDe(ps)
	pos, ocupa := Envolver(an, al, HuecoPildorasOblig, HuecoPildorasOblig, dentro)
	l.piezas(ps, pos, x0, y)
	y += ocupa.Height
	if len(a.Pendientes()) > 0 && !a.Activados {
		b := ui.NuevoBoton(a.t("onboarding.done.enableAll"), func() {
			if !a.Activando && a.AlActivar != nil {
				a.Activando = true
				a.AlActivar(a.Pendientes())
				a.cambio()
			}
		}).Con(ui.Secundario, ui.TallaSM)
		b.Cargando = a.Activando
		m := b.MinSize()
		y += HuecoTarjetas
		l.poner(b, x0, y, m.Width, m.Height)
		y += m.Height
	}
	y += x0
	// La caja va la primera, debajo de lo demás.
	caja := ui.NuevaCaja(tema.Color("bg", v), tema.Color("border", v), radioTarjeta())
	l.puestos = append([]puesto{{o: container.New(&soloCaja{caja}, caja.Objetos()...), w: cw, h: y}}, l.puestos...)
	return l.objeto(y)
}

// pie es `border-t pt-4`: «Ahora no» a la izquierda y, a la derecha, «Atrás» y
// «Continuar» (o el rótulo del paso).
func (a *Asistente) pie(v tema.Variante, ancho float32) (fyne.CanvasObject, float32) {
	cancelar := ui.NuevoBoton(a.t("onboarding.cancel"), a.AlCancelar).Con(ui.Fantasma, ui.TallaMD)
	cancelar.Inerte = a.Ocupado || a.AlCancelar == nil
	var der []pieza
	if a.Paso > 0 {
		atras := ui.NuevoBoton(a.t("ui.wizard.back"), func() { a.Paso--; a.cambio() }).Con(ui.Secundario, ui.TallaMD)
		atras.ConIcono(iconos.ChevronIzquierda, 16)
		atras.Inerte = a.Ocupado
		der = append(der, piezaDe(atras, atras.MinSize()))
	}
	sigue := a.t("ui.wizard.next")
	if a.Paso == PasoRevisar {
		sigue = a.t("onboarding.review.save")
	}
	seguir := ui.NuevoBoton(sigue, a.Seguir).Con(ui.Primario, ui.TallaMD)
	seguir.Cargando, seguir.Inerte = a.Ocupado, !a.puedeSeguir()
	der = append(der, piezaDe(seguir, seguir.MinSize()))
	return pieGuiado(v, cancelar, der, ancho)
}
