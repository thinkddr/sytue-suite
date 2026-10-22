// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package erp

import (
	"errors"
	"image/color"
	"regexp"
	"strings"
	"unicode/utf8"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// # Sytue AI en el ERP (`PanelIA`, `CrearConIA`, `InformeIA` y `RellenarIA` de SytueIA.tsx)
//
// La IA PROPONE y la persona decide: nada se guarda, se crea ni se descarga sin pulsar,
// y lo que se enseña antes son los datos de verdad. Las reglas del ERP, ya decididas: la
// IA solo ve el esquema, la ejecución es en memoria y sin SQL, y lo que crea pasa por
// confirmación (aquí, el plan con «Crear todo»).
//
// Medido en el arnés (1280×800, rem 14, 2026-09-23), el panel es `Modal size="lg"` (720)
// sin pie:
//
//	pestañas   `inline-flex rounded-md border`, 161,06×28,25; cada una `px-3 py-1.5 text-sm`
//	           (26,25 de alto), la activa en `bg-selected` y peso 600; `mb-4`
//	petición   `<Textarea rows={3}>`: 73,75
//	botonera   `mt-3 flex flex-wrap gap-2`: «Preparar» `md` y la pista `text-xs`, que salta
//	           de fila si no cabe
//	plan       `space-y-3`: el resumen `text-sm`, los avisos en un `InlineHelp` con lista,
//	           cada paso en `rounded-md border p-3 space-y-2` (40,5 el que ya existe,
//	           169,48 el nuevo con todo) y «Cerrar» y «Crear todo» a la derecha
//	informe    `mt-6 space-y-4`: el título `text-base` con «Descargar Excel» y «Guardar
//	           tablero», y las partes en dos columnas de 331

// Las dos pestañas del panel.
const (
	PestanaCrear   = "crear"
	PestanaInforme = "informe"
)

// Lo que está haciendo el panel: nada, proponer, crear, guardar o bajar el Excel.
const (
	OcupadoProponer = "proponer"
	OcupadoCrear    = "crear"
	OcupadoGuardar  = "guardar"
	OcupadoExcel    = "excel"
)

// Medidas del panel, de HOJA salvo donde dice lo contrario.
const (
	// PestanaIAPX y PestanaIAPY son el `px-3 py-1.5` de una pestaña; PestanasIAMB el
	// `mb-4` del grupo.
	PestanaIAPX  float32 = 3 * 0.25 * tema.Rem
	PestanaIAPY  float32 = 1.5 * 0.25 * tema.Rem
	PestanasIAMB float32 = 4 * 0.25 * tema.Rem
	// FilasPeticionIA es el `rows={3}` de la petición y FilasRellenar el `rows={8}`.
	FilasPeticionIA = 3
	FilasRellenar   = 8
	// MaxPeticionIA y MaxTextoRellenar son los `maxLength` de la web (y los topes del
	// servicio: `maxPeticion` y `maxTextoFicha`).
	MaxPeticionIA    = 2000
	MaxTextoRellenar = 20000
	// HuecoIA3, HuecoIA4 y HuecoIA6 son el `mt-3`/`space-y-3`, el `space-y-4`/`mt-4` y el
	// `mt-6` del panel.
	HuecoIA3 float32 = 3 * 0.25 * tema.Rem
	HuecoIA4 float32 = 4 * 0.25 * tema.Rem
	HuecoIA6 float32 = 6 * 0.25 * tema.Rem
	// PasoIAP es el `p-3` de un paso del plan, y HuecoDlX el `gap-x-4` de sus datos.
	PasoIAP  float32 = 3 * 0.25 * tema.Rem
	HuecoDlX float32 = 4 * 0.25 * tema.Rem
	// ListaAvisosPL es el `pl-4` de la lista de avisos del plan.
	ListaAvisosPL float32 = 4 * 0.25 * tema.Rem
	// LadoRelojIA es el `size={12}` del reloj de la tarea que se creará.
	LadoRelojIA float32 = 12
	// AnchoSM es el `sm:` de la hoja (40 rem de 16): los datos de un paso van en dos
	// columnas a partir de ahí.
	AnchoSM float32 = 640
)

// cuerpoRelajado es `text-sm leading-relaxed`, el del cuerpo de un `InlineHelp`.
var cuerpoRelajado = tema.Escalon{Tam: tema.TextoSM.Tam, Linea: 1.625 * tema.TextoSM.Tam}

// PanelIA es el estado del panel de Sytue AI. Cambiar de pestaña lo vacía todo, como en
// la web, donde cada pestaña es un componente que se desmonta.
type PanelIA struct {
	tr  *i18n.Translator
	loc i18n.Locale

	Pestana string
	// Modelo es la lista abierta, si hay una: el informe se entiende sobre ella.
	Modelo *Modelo
	// Modelos son todos los del catálogo, para pintar lo que se va a crear.
	Modelos []Modelo
	// Ventana es el ancho de la ventana: decide las columnas (`sm:` y `md:`).
	Ventana float32

	// Plan es lo que Sytue AI propone crear; nil, nada todavía.
	Plan *api.ERPPlan
	// Informe es lo que propone como informe, con sus datos; nil, nada todavía.
	Informe *Informe
	// Para es «tablero» o «excel»: con qué botón se pidió el informe.
	Para string
	// Ocupado es lo que está en marcha (`OcupadoProponer`…); vacío, nada.
	Ocupado string
	// Fallo es el `InlineHelp` rojo.
	Fallo string
	// Generacion sube con cada cambio de pestaña: una respuesta que llega tarde a una
	// pestaña que ya no está se tira, que es lo que hace la web al desmontarla.
	Generacion int

	AlCambio   func()
	AlPreparar func(peticion string)
	AlCrear    func()
	AlProponer func(peticion, para string)
	AlGuardar  func()
	AlExcel    func()

	peticion *ui.AreaTexto
}

// NuevoPanelIA abre el panel en «Crear registros», que es la pestaña de la web.
func NuevoPanelIA(tr *i18n.Translator, modelo *Modelo, modelos []Modelo) *PanelIA {
	x := &PanelIA{tr: tr, loc: tr.Locale(), Pestana: PestanaCrear, Modelo: modelo, Modelos: modelos}
	x.montarPeticion()
	return x
}

func (x *PanelIA) t(clave string, vars ...i18n.Vars) string {
	if len(vars) > 0 {
		return x.tr.T(clave, vars[0])
	}
	return x.tr.T(clave, nil)
}

func (x *PanelIA) cambio() {
	if x.AlCambio != nil {
		x.AlCambio()
	}
}

// montarPeticion crea el `<Textarea>` de la pestaña: uno nuevo en cada cambio, que es lo
// que pasa al desmontarse el componente.
func (x *PanelIA) montarPeticion() {
	ph := x.t("ai.actPlaceholder")
	if x.Pestana == PestanaInforme {
		ph = x.t("ai.placeholder")
	}
	a := ui.NuevaAreaTexto(ph)
	a.Filas = FilasPeticionIA
	vacia := true
	a.AlCambiar(func(s string) {
		if utf8.RuneCountInString(s) > MaxPeticionIA {
			a.PonerTexto(string([]rune(s)[:MaxPeticionIA]))
			return
		}
		// Solo se repinta cuando cambia si se puede pedir: repintar en cada letra rehace
		// el panel entero por nada.
		if v := strings.TrimSpace(s) == ""; v != vacia {
			vacia = v
			x.cambio()
		}
	})
	x.peticion = a
}

// Peticion es lo escrito, sin los blancos de los lados.
func (x *PanelIA) Peticion() string { return strings.TrimSpace(x.peticion.Texto()) }

// Campo es el área de la petición, para darle el foco al abrir.
func (x *PanelIA) Campo() fyne.Focusable { return x.peticion }

// Elegir cambia de pestaña y vacía lo de la otra.
func (x *PanelIA) Elegir(p string) {
	if p == x.Pestana {
		return
	}
	x.Pestana, x.Plan, x.Informe, x.Ocupado, x.Fallo, x.Para = p, nil, nil, "", "", ""
	x.Generacion++
	x.montarPeticion()
	x.cambio()
}

// Cuerpo arma el panel al ancho del cuerpo del diálogo.
func (x *PanelIA) Cuerpo(v tema.Variante, ancho float32) fyne.CanvasObject {
	p := &pila{}
	pest, altoPest := x.pestanas(v)
	p.add(pest, altoPest, 0)
	var o fyne.CanvasObject
	var alto float32
	if x.Pestana == PestanaInforme {
		o, alto = x.cuerpoInforme(v, ancho)
	} else {
		o, alto = x.cuerpoCrear(v, ancho)
	}
	p.add(o, alto, PestanasIAMB)
	c, _ := p.objeto()
	return c
}

// pestanas es el `role="tablist"`: dos pestañas pegadas en una caja con borde.
func (x *PanelIA) pestanas(v tema.Variante) (fyne.CanvasObject, float32) {
	crear := NuevaPestanaIA(x.t("ai.tabCreate"), x.Pestana == PestanaCrear, func() { x.Elegir(PestanaCrear) })
	inf := NuevaPestanaIA(x.t("ai.tabReport"), x.Pestana == PestanaInforme, func() { x.Elegir(PestanaInforme) })
	mc, mi := crear.MinSize(), inf.MinSize()
	alto := mc.Height + 2*tema.Borde
	l := &lienzo{}
	// Sin fondo propio: deja ver el del panel (`surface-raised`), que es con lo que se
	// rellena la caja; con relleno transparente, `ui.Caja` enseña su borde por dentro.
	l.caja(ui.NuevaCaja(tema.Color("surface-raised", v), tema.Color("border", v), tema.RadioMD), mc.Width+mi.Width+2*tema.Borde, alto)
	l.poner(crear, tema.Borde, tema.Borde, mc.Width, mc.Height)
	l.poner(inf, tema.Borde+mc.Width, tema.Borde, mi.Width, mi.Height)
	return l.objeto(alto)
}

// areaEnLinea es el `<textarea>` a `ancho` con lo que deja debajo: va en línea y su
// línea guarda 7 px para el descendente a text-base, medidos en el DOM real
// de PanelIA y RellenarIA con raíz de 16 px y Figtree (2026-10-04).
func areaEnLinea(a *ui.AreaTexto, ancho float32) (fyne.CanvasObject, float32) {
	a.Ancho = ancho
	h := a.MinSize().Height
	l := &lienzo{}
	l.poner(a, 0, 0, ancho, h)
	return l.objeto(h + 7)
}

// botonera es `flex flex-wrap items-center gap-2`: las piezas en fila, saltando.
func botonera(ps []pieza, ancho float32) (fyne.CanvasObject, float32) {
	a, h := medidasDe(ps)
	pos, tam := Envolver(a, h, HuecoDos, HuecoDos, ancho)
	l := &lienzo{}
	l.piezas(ps, pos, 0, 0)
	return l.objeto(tam.Height)
}

// parrafoPieza es un `<p>` que entra en una fila que salta: a su ancho natural si cabe
// en la fila y, si no, en la suya a todo el ancho y repartido en líneas.
func parrafoPieza(texto string, e tema.Escalon, token string, v tema.Variante, ancho float32) pieza {
	p := ui.NuevoParrafo(texto, e, ui.Normal, token, v)
	w := min(ui.Medir(texto, e, ui.Normal).Width, ancho)
	return pieza{obj: p, ancho: w, alto: ui.AltoParrafo(p, w)}
}

// ayuda es un `InlineHelp` a todo el ancho.
func ayudaPeligro(texto string, ancho float32) (fyne.CanvasObject, float32) {
	a := ui.NuevaAyudaEnLinea(texto).ConTono(ui.AyudaPeligro)
	return a, ui.AltoAyudaEnLinea(a, ancho)
}

// cuerpoCrear es `CrearConIA` (`space-y-4`): la petición, el fallo y el plan.
func (x *PanelIA) cuerpoCrear(v tema.Variante, ancho float32) (fyne.CanvasObject, float32) {
	p := &pila{}
	area, altoArea := areaEnLinea(x.peticion, ancho)
	p.add(area, altoArea, 0)
	preparar := ui.NuevoBoton(x.t("ai.actPrepare"), func() {
		if x.AlPreparar != nil && x.Peticion() != "" && x.Ocupado == "" {
			x.AlPreparar(x.Peticion())
		}
	}).Con(ui.Primario, ui.TallaMD)
	preparar.Cargando = x.Ocupado == OcupadoProponer
	preparar.Inerte = x.Peticion() == "" || x.Ocupado != ""
	pista := parrafoPieza(x.t("ai.actHint"), tema.TextoXS, "text-subtle", v, ancho)
	b, hb := botonera([]pieza{piezaDe(preparar, preparar.MinSize()), pista}, ancho)
	p.add(b, hb, HuecoIA3)
	if x.Fallo != "" {
		o, h := ayudaPeligro(x.Fallo, ancho)
		p.add(o, h, HuecoIA4)
	}
	if x.Plan != nil {
		o, h := x.plan(v, ancho)
		p.add(o, h, HuecoIA4)
	}
	return p.objeto()
}

// plan es la `<section class="space-y-3">` con lo que se va a crear.
func (x *PanelIA) plan(v tema.Variante, ancho float32) (fyne.CanvasObject, float32) {
	pl := x.Plan
	p := &pila{}
	res := ui.NuevoParrafo(pl.Resumen, tema.TextoSM, ui.Normal, "text", v)
	p.add(res, ui.AltoParrafo(res, ancho), 0)
	if len(pl.Avisos) > 0 {
		o, h := avisosPlan(v, pl.Avisos, ancho)
		p.add(o, h, HuecoIA3)
	}
	pasos := &pila{}
	for i, paso := range pl.Pasos {
		o, h := x.paso(v, paso, ancho)
		arriba := HuecoDos
		if i == 0 {
			arriba = 0
		}
		pasos.add(o, h, arriba)
	}
	o, h := pasos.objeto()
	p.add(o, h, HuecoIA3)
	cerrar := ui.NuevoBoton(x.t("action.close"), func() {
		x.Plan = nil
		x.cambio()
	}).Con(ui.Fantasma, ui.TallaMD)
	cerrar.Inerte = x.Ocupado != ""
	crear := ui.NuevoBoton(x.t("ai.actCreate"), func() {
		if x.AlCrear != nil && x.Ocupado == "" && !pl.Incompleto() {
			x.AlCrear()
		}
	}).Con(ui.Primario, ui.TallaMD)
	crear.Cargando = x.Ocupado == OcupadoCrear
	crear.Inerte = pl.Incompleto() || x.Ocupado != ""
	ps := []pieza{piezaDe(cerrar, cerrar.MinSize()), piezaDe(crear, crear.MinSize())}
	a, hs := medidasDe(ps)
	pos, tam := Envolver(a, hs, HuecoDos, HuecoDos, ancho)
	pie := &lienzo{}
	pie.piezas(ps, pos, ancho-tam.Width, 0) // `justify-end`
	o, h = pie.objeto(tam.Height)
	p.add(o, h, HuecoIA3)
	return p.objeto()
}

// avisosPlan es el `InlineHelp tone="warning"` con la lista de avisos del plan: el
// icono del tono y un `ul.list-disc pl-4 space-y-0.5` en `text-sm leading-relaxed`.
func avisosPlan(v tema.Variante, avisos []string, ancho float32) (fyne.CanvasObject, float32) {
	x0 := tema.Borde + ui.RellenoAyuda + ui.GlifoAyuda + ui.HuecoAyuda
	dentro := max(ancho-x0-ui.RellenoAyuda-tema.Borde, 0)
	// `listaMarcada` lleva el `pl-5` de las listas de Obligaciones; esta, `pl-4`.
	lista, altoLista := listaMarcada(avisos, false, cuerpoRelajado, v, dentro+ListaPL-ListaAvisosPL)
	alto := 2*tema.Borde + 2*ui.RellenoAyuda + max(altoLista, ui.GlifoAyuda)
	l := &lienzo{}
	l.caja(ui.NuevaCaja(tema.Color("warning-bg", v), tema.Color("warning", v), tema.RadioMD), ancho, alto)
	ic := iconos.Trazo(iconos.Aviso, ui.GlifoAyuda, "warning-text", v).Objeto()
	l.poner(ic, tema.Borde+ui.RellenoAyuda, tema.Borde+ui.RellenoAyuda+ui.DesplazaGlifoAyuda, ui.GlifoAyuda, ui.GlifoAyuda)
	l.poner(lista, x0-(ListaPL-ListaAvisosPL), tema.Borde+ui.RellenoAyuda, dentro+ListaPL-ListaAvisosPL, altoLista)
	return l.objeto(alto)
}

// modeloDe es el modelo del catálogo con esa clave.
func (x *PanelIA) modeloDe(clave string) (Modelo, bool) {
	for _, m := range x.Modelos {
		if m.Clave == clave {
			return m, true
		}
	}
	return Modelo{}, false
}

// nombreDePaso es cómo se llama otro paso del plan: lo que ya existía, su nombre, su
// rótulo o su referencia.
func (x *PanelIA) nombreDePaso(ref string) string {
	for _, o := range x.Plan.Pasos {
		if o.Ref != ref {
			continue
		}
		if o.Existente != nil {
			return o.Existente.Etiqueta
		}
		if n, ok := o.Data["nombre"]; ok && n != nil {
			return texto(n)
		}
		if o.Etiqueta != "" {
			return o.Etiqueta
		}
	}
	return ref
}

// paso es `PasoDelPlan`: qué se crea (o qué ya existía), con sus datos y lo que falta.
func (x *PanelIA) paso(v tema.Variante, paso api.ERPPasoPlan, ancho float32) (fyne.CanvasObject, float32) {
	modelo, _ := x.modeloDe(paso.Modelo)
	dentro := max(ancho-2*tema.Borde-2*PasoIAP, 0)
	p := &pila{}
	// La cabecera: el nombre del modelo y si es nuevo o ya estaba.
	rotulo := paso.Etiqueta
	if rotulo == "" {
		rotulo = modelo.Etiqueta
	}
	nombre := ui.Texto(rotulo, tema.TextoSM, ui.SemiNegrita, tema.Color("text", v))
	var ins *ui.Insignia
	if paso.Existente != nil {
		ins = ui.NuevaInsignia(x.t("ai.actExisting", i18n.Vars{"name": paso.Existente.Etiqueta}), ui.InsigniaNeutra)
	} else {
		ins = ui.NuevaInsignia(x.t("ai.actNew"), ui.InsigniaInfo)
	}
	cab := []pieza{
		{obj: nombre, ancho: ui.Medir(rotulo, tema.TextoSM, ui.SemiNegrita).Width, alto: tema.TextoSM.Linea, texto: true},
		piezaDe(ins, ui.MedidaInsignia(ins.Texto)),
	}
	o, h := botonera(cab, dentro)
	p.add(o, h, 0)
	if paso.Existente == nil {
		if o, h := x.datosPaso(v, paso, modelo, dentro); h > 0 {
			p.add(o, h, HuecoDos)
		}
	}
	if len(paso.Faltan) > 0 {
		campos := make([]string, len(paso.Faltan))
		for i, f := range paso.Faltan {
			campos[i] = f
			for _, c := range modelo.Campos {
				if c.Clave == f {
					campos[i] = c.Etiqueta
				}
			}
		}
		o, h := ayudaPeligro(x.t("ai.actMissing", i18n.Vars{"fields": strings.Join(campos, ", ")}), dentro)
		p.add(o, h, HuecoDos)
	}
	for _, a := range paso.Avisos {
		t := ui.NuevoParrafo(a, tema.TextoXS, ui.Normal, "warning-text", v)
		p.add(t, ui.AltoParrafo(t, dentro), HuecoDos)
	}
	if paso.Tarea != "" && paso.Existente == nil {
		frase := x.t("ai.actTask", i18n.Vars{"task": paso.Tarea})
		t := ui.Texto(frase, tema.TextoXS, ui.Normal, tema.Color("text-muted", v))
		ui.Truncar(t, frase, tema.TextoXS, ui.Normal, max(dentro-LadoRelojIA-HuecoUno, 0))
		l := &lienzo{}
		l.poner(iconos.Trazo(iconos.Reloj, LadoRelojIA, "text-muted", v).Objeto(), 0, (tema.TextoXS.Linea-LadoRelojIA)/2, LadoRelojIA, LadoRelojIA)
		l.texto(t, LadoRelojIA+HuecoUno, 0)
		o, h := l.objeto(tema.TextoXS.Linea)
		p.add(o, h, HuecoDos)
	}
	cuerpo, altoCuerpo := p.objeto()
	alto := altoCuerpo + 2*PasoIAP + 2*tema.Borde
	l := &lienzo{}
	l.caja(ui.NuevaCaja(tema.Color("surface-raised", v), tema.Color("border", v), tema.RadioMD), ancho, alto)
	l.poner(cuerpo, tema.Borde+PasoIAP, tema.Borde+PasoIAP, dentro, altoCuerpo)
	return l.objeto(alto)
}

// datosPaso es el `<dl class="grid gap-x-4 gap-y-1 text-xs sm:grid-cols-2">` de un paso
// nuevo: los campos que trae, en el orden del modelo, con su rótulo apagado y el valor
// recortado a una línea.
func (x *PanelIA) datosPaso(v tema.Variante, paso api.ERPPasoPlan, modelo Modelo, ancho float32) (fyne.CanvasObject, float32) {
	type par struct{ dt, dd string }
	var pares []par
	for _, c := range modelo.Campos {
		if ref := paso.Enlaces[c.Clave]; ref != "" {
			pares = append(pares, par{c.Etiqueta + ":", x.nombreDePaso(ref)})
			continue
		}
		if d, hay := paso.Data[c.Clave]; hay && d != "" {
			pares = append(pares, par{c.Etiqueta + ":", Celda(c, d, x.loc, paso.Etiquetas)})
		}
	}
	if len(pares) == 0 {
		return nil, 0
	}
	cols := 1
	if x.Ventana == 0 || x.Ventana >= AnchoSM {
		cols = 2
	}
	w := (ancho - float32(cols-1)*HuecoDlX) / float32(cols)
	l := &lienzo{}
	filas := (len(pares) + cols - 1) / cols
	for i, pr := range pares {
		f, c := i/cols, i%cols
		xx, y := float32(c)*(w+HuecoDlX), float32(f)*(tema.TextoXS.Linea+HuecoUno)
		dt := ui.Texto(pr.dt, tema.TextoXS, ui.Normal, tema.Color("text-subtle", v))
		wdt := ui.Medir(pr.dt, tema.TextoXS, ui.Normal).Width
		dd := ui.Texto(pr.dd, tema.TextoXS, ui.Normal, tema.Color("text", v))
		ui.Truncar(dd, pr.dd, tema.TextoXS, ui.Normal, max(w-wdt-HuecoUno, 0))
		l.texto(dt, xx, y)
		l.texto(dd, xx+wdt+HuecoUno, y)
	}
	return l.objeto(float32(filas)*tema.TextoXS.Linea + float32(filas-1)*HuecoUno)
}

// cuerpoInforme es `InformeIA`: la petición, «Crear tablero» y «Crear Excel», la nota de
// privacidad, el fallo y el informe propuesto con sus datos.
func (x *PanelIA) cuerpoInforme(v tema.Variante, ancho float32) (fyne.CanvasObject, float32) {
	p := &pila{}
	area, altoArea := areaEnLinea(x.peticion, ancho)
	p.add(area, altoArea, 0)
	if x.Modelo != nil {
		sobre := ui.NuevoParrafo(x.t("ai.about", i18n.Vars{"model": x.Modelo.Etiqueta}), tema.TextoXS, ui.Normal, "text-subtle", v)
		p.add(sobre, ui.AltoParrafo(sobre, ancho), HuecoUno)
	}
	listo := x.Peticion() != "" && x.Ocupado == ""
	pedir := func(para string) func() {
		return func() {
			if x.AlProponer != nil && x.Peticion() != "" && x.Ocupado == "" {
				x.AlProponer(x.Peticion(), para)
			}
		}
	}
	tablero := ui.NuevoBoton(x.t("ai.dashboard"), pedir(api.ERPParaTablero)).Con(ui.Primario, ui.TallaMD)
	tablero.Cargando, tablero.Inerte = x.Ocupado == OcupadoProponer && x.Para == api.ERPParaTablero, !listo
	excel := ui.NuevoBoton(x.t("ai.excel"), pedir(api.ERPParaExcel)).Con(ui.Secundario, ui.TallaMD)
	excel.Cargando, excel.Inerte = x.Ocupado == OcupadoProponer && x.Para == api.ERPParaExcel, !listo
	b, hb := botonera([]pieza{piezaDe(tablero, tablero.MinSize()), piezaDe(excel, excel.MinSize())}, ancho)
	p.add(b, hb, HuecoIA3)
	priv := ui.NuevoParrafo(x.t("ai.privacy"), tema.TextoXS, ui.Normal, "text-subtle", v)
	p.add(priv, ui.AltoParrafo(priv, ancho), HuecoIA3)
	if x.Fallo != "" {
		o, h := ayudaPeligro(x.Fallo, ancho)
		p.add(o, h, HuecoIA4)
	}
	if x.Informe != nil {
		o, h := x.informe(v, ancho)
		p.add(o, h, HuecoIA6)
	}
	return p.objeto()
}

// informe es la `<section class="space-y-4">` con el título, las dos acciones y las
// partes. La acción principal es la que se pidió: el Excel si se pidió Excel.
func (x *PanelIA) informe(v tema.Variante, ancho float32) (fyne.CanvasObject, float32) {
	inf := x.Informe
	vExcel, vGuardar := ui.Secundario, ui.Primario
	if x.Para == api.ERPParaExcel {
		vExcel, vGuardar = ui.Primario, ui.Secundario
	}
	excel := ui.NuevoBoton(x.t("ai.download"), x.AlExcel).Con(vExcel, ui.TallaMD)
	excel.Cargando, excel.Inerte = x.Ocupado == OcupadoExcel, x.Ocupado != "" && x.Ocupado != OcupadoExcel
	guardar := ui.NuevoBoton(x.t("ai.save"), x.AlGuardar).Con(vGuardar, ui.TallaMD)
	guardar.Cargando, guardar.Inerte = x.Ocupado == OcupadoGuardar, x.Ocupado != "" && x.Ocupado != OcupadoGuardar
	titulo := ui.Texto(inf.Informe.Titulo, tema.TextoBase, ui.SemiNegrita, tema.Color("text", v))
	ps := []pieza{piezaDe(excel, excel.MinSize()), piezaDe(guardar, guardar.MinSize())}
	cab, altoCab := cabeceraConAcciones(titulo, tema.TextoBase, ps, HuecoDos, ancho)
	p := &pila{}
	p.add(cab, altoCab, 0)
	inf.DosColumnas = x.Ventana == 0 || x.Ventana >= ui.Estrecho
	o, h := inf.Objeto(v, ancho)
	p.add(o, h, HuecoIA4)
	return p.objeto()
}

// cabeceraConAcciones es `flex flex-wrap items-center justify-between gap-*`: el título
// a la izquierda y las acciones a la derecha si caben, y si no, debajo.
func cabeceraConAcciones(titulo *canvas.Text, e tema.Escalon, acciones []pieza, hueco, ancho float32) (fyne.CanvasObject, float32) {
	a, h := medidasDe(acciones)
	pos, tam := Envolver(a, h, HuecoDos, HuecoDos, ancho)
	anchoTit := ui.Medir(titulo.Text, e, ui.SemiNegrita).Width
	l := &lienzo{}
	yTit, xD := float32(0), ancho-tam.Width
	alto := max(e.Linea, tam.Height)
	var yD float32
	if anchoTit+hueco+tam.Width > ancho {
		xD, yD = 0, e.Linea+hueco
		alto = yD + tam.Height
	} else {
		yTit, yD = (alto-e.Linea)/2, (alto-tam.Height)/2
	}
	ui.Truncar(titulo, titulo.Text, e, ui.SemiNegrita, ancho)
	l.texto(titulo, 0, yTit)
	l.piezas(acciones, pos, xD, yD)
	return l.objeto(alto)
}

// NombreExcel es el nombre del fichero: el título sin lo que un sistema de ficheros no
// admite, y «Sytue AI» si se queda en nada.
func NombreExcel(titulo string) string {
	n := strings.TrimSpace(noFichero.ReplaceAllString(titulo, "-"))
	if n == "" {
		n = "Sytue AI"
	}
	return n + ".xlsx"
}

var noFichero = regexp.MustCompile(`[\\/:*?"<>|]+`)

// estadoDe es el código HTTP de un error del servicio; 0, otro error (sin red…).
func estadoDe(err error) int {
	var e *api.Error
	if errors.As(err, &e) {
		return e.Status
	}
	return 0
}

// MensajeIA es `mensajeIA`: el límite, no haberlo entendido, o que no responde.
func MensajeIA(err error, tr *i18n.Translator) string {
	switch estadoDe(err) {
	case 429:
		return tr.T("ai.limit", nil)
	case 422:
		return tr.T("ai.notUnderstood", nil)
	}
	return tr.T("ai.failed", nil)
}

// --- La pestaña -----------------------------------------------------------------------

// PestanaIA es un `role="tab"` del panel: `px-3 py-1.5 text-sm`, la activa en
// `bg-selected` con peso 600 y el resto en `text-muted` que pasa a `text` al pasar.
type PestanaIA struct {
	ui.Pulsable
	Rotulo string
	Activa bool
}

var _ fyne.Widget = (*PestanaIA)(nil)

// NuevaPestanaIA crea la pestaña.
func NuevaPestanaIA(rotulo string, activa bool, al func()) *PestanaIA {
	p := &PestanaIA{Rotulo: rotulo, Activa: activa}
	p.Nombre, p.AlPulsar = rotulo, al
	p.ExtendBaseWidget(p)
	return p
}

func (p *PestanaIA) peso() ui.Peso {
	if p.Activa {
		return ui.SemiNegrita
	}
	return ui.Normal
}

// MinSize es el rótulo con su `px-3 py-1.5`. Medido: 98,19 y 60,88 por 26,25.
func (p *PestanaIA) MinSize() fyne.Size {
	p.ExtendBaseWidget(p)
	return fyne.NewSize(ui.Medir(p.Rotulo, tema.TextoSM, p.peso()).Width+2*PestanaIAPX, tema.TextoSM.Linea+2*PestanaIAPY)
}

// CreateRenderer arma el anillo, el fondo y el rótulo.
func (p *PestanaIA) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &pestanaIARenderer{
		p:      p,
		aro:    ui.NuevoAro(0, ui.SinHueco, v).Abierto(),
		fondo:  canvas.NewRectangle(color.NRGBA{}),
		rotulo: ui.Texto(p.Rotulo, tema.TextoSM, p.peso(), tema.Color("text-muted", v)),
	}
	r.Refresh()
	return r
}

type pestanaIARenderer struct {
	p      *PestanaIA
	aro    *ui.Aro
	fondo  *canvas.Rectangle
	rotulo *canvas.Text
}

func (r *pestanaIARenderer) MinSize() fyne.Size { return r.p.MinSize() }
func (r *pestanaIARenderer) Objects() []fyne.CanvasObject {
	return append(r.aro.Objetos(), r.fondo, r.rotulo)
}
func (r *pestanaIARenderer) Destroy() {}

func (r *pestanaIARenderer) Layout(t fyne.Size) {
	r.aro.Colocar(t)
	r.fondo.Move(fyne.NewPos(0, 0))
	r.fondo.Resize(t)
	ui.ColocarTexto(r.rotulo, tema.TextoSM, PestanaIAPX, (t.Height-tema.TextoSM.Linea)/2)
}

func (r *pestanaIARenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	fondo, tinta := color.NRGBA{}, tema.Color("text-muted", v)
	switch {
	case r.p.Activa:
		fondo, tinta = tema.Color("selected", v), tema.Color("text", v)
	case r.p.Encima():
		tinta = tema.Color("text", v)
	}
	r.fondo.FillColor = fondo
	r.fondo.Refresh()
	r.rotulo.Color, r.rotulo.FontSource = tinta, r.p.peso().Cara()
	r.rotulo.Refresh()
	r.aro.Ver(r.p.Enfocado())
	r.aro.Pintar(v)
}

// --- Rellenar con Sytue AI (`RellenarIA`) ------------------------------------------------

// RellenarIA es el diálogo de «Rellenar con Sytue AI»: se pega un texto —un correo, un
// pedido— o se elige el PDF de la factura, y Sytue AI rellena la ficha. No guarda nada:
// la persona revisa los campos antes de guardar.
type RellenarIA struct {
	tr     *i18n.Translator
	Modelo Modelo
	// Ocupado es la rueda de «Rellenar»; Fallo, el `InlineHelp` rojo.
	Ocupado bool
	Fallo   string

	AlCambio   func()
	AlRellenar func(texto string)
	// AlElegirPDF abre el selector del sistema; nil, no hay y no se ofrece.
	AlElegirPDF func()

	area *ui.AreaTexto
}

// NuevoRellenarIA crea el diálogo vacío.
func NuevoRellenarIA(tr *i18n.Translator, m Modelo) *RellenarIA {
	x := &RellenarIA{tr: tr, Modelo: m}
	x.area = ui.NuevaAreaTexto("")
	x.area.Filas = FilasRellenar
	vacia := true
	x.area.AlCambiar(func(s string) {
		if utf8.RuneCountInString(s) > MaxTextoRellenar {
			x.area.PonerTexto(string([]rune(s)[:MaxTextoRellenar]))
			return
		}
		if v := strings.TrimSpace(s) == ""; v != vacia {
			vacia = v
			if x.AlCambio != nil {
				x.AlCambio()
			}
		}
	})
	return x
}

// Texto es lo pegado, sin los blancos de los lados.
func (x *RellenarIA) Texto() string { return strings.TrimSpace(x.area.Texto()) }

// Campo es el área, para darle el foco al abrir.
func (x *RellenarIA) Campo() fyne.Focusable { return x.area }

// Cuerpo es la pista, el área, el PDF y el fallo.
func (x *RellenarIA) Cuerpo(v tema.Variante, ancho float32) fyne.CanvasObject {
	p := &pila{}
	pista := ui.NuevoParrafo(x.tr.T("ai.fillHint", i18n.Vars{"model": x.Modelo.Etiqueta}), tema.TextoSM, ui.Normal, "text-muted", v)
	p.add(pista, ui.AltoParrafo(pista, ancho), 0)
	area, altoArea := areaEnLinea(x.area, ancho)
	p.add(area, altoArea, HuecoIA3)
	if x.AlElegirPDF != nil {
		// El `<label class="flex flex-wrap items-center gap-2 text-sm text-muted">` con
		// el icono, la frase y el `<input type="file" class="text-xs">`. El control de
		// fichero lo dibuja el navegador con sus propios textos; aquí es el selector del
		// sistema detrás de un «Subir» del catálogo, en `text-xs` para que la fila mida
		// lo mismo (15,75, medido).
		ic := iconos.Trazo(iconos.Subir, 14, "text-muted", v).Objeto()
		frase := x.tr.T("ai.fillFromPdf", nil)
		t := ui.Texto(frase, tema.TextoSM, ui.Normal, tema.Color("text-muted", v))
		subir := x.tr.T("docs.upload", nil)
		b := ui.NuevoEnlace(subir, tema.TextoXS, ui.Medio, "primary-text", func() {
			if !x.Ocupado {
				x.AlElegirPDF()
			}
		})
		b.Boton = true
		ps := []pieza{
			piezaDe(ic, fyne.NewSize(14, 14)),
			{obj: t, ancho: ui.Medir(frase, tema.TextoSM, ui.Normal).Width, alto: tema.TextoSM.Linea, texto: true},
			piezaDe(b, fyne.NewSize(ui.Medir(subir, tema.TextoXS, ui.Medio).Width, tema.TextoXS.Linea)),
		}
		o, h := botonera(ps, ancho)
		p.add(o, h, HuecoIA3)
	}
	if x.Fallo != "" {
		o, h := ayudaPeligro(x.Fallo, ancho)
		p.add(o, h, HuecoIA3)
	}
	c, _ := p.objeto()
	return c
}

// Pie son «Cerrar» y «Rellenar».
func (x *RellenarIA) Pie(alCerrar func()) fyne.CanvasObject {
	c := ui.NuevoBoton(x.tr.T("action.close", nil), alCerrar).Con(ui.Fantasma, ui.TallaMD)
	r := ui.NuevoBoton(x.tr.T("ai.fillAction", nil), func() {
		if x.AlRellenar != nil && x.Texto() != "" && !x.Ocupado {
			x.AlRellenar(x.Texto())
		}
	}).Con(ui.Primario, ui.TallaMD)
	r.Cargando, r.Inerte = x.Ocupado, x.Texto() == ""
	return container.New(ui.Fila{Hueco: ui.HuecoPieDialogo}, c, r)
}
