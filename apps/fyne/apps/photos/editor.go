// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Contrapartida de apps/frontend/apps/photos/src/components/Editor.tsx. La pantalla
// completa queda declarada en app_editor.go (pages/edit.astro).

package photos

import (
	"fmt"
	"image/color"
	"math"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// Las medidas del editor, de Editor.tsx.
const (
	// AnchoPanelEditor es el `md:w-80` del `<aside>`.
	AnchoPanelEditor = 20 * tema.Rem
	RellenoEditor    = 3 * 0.25 * tema.Rem // `p-3` del lienzo
	RellenoPanel     = 4 * 0.25 * tema.Rem // `p-4` del panel
	// HuecoPanel es el `gap-4` entre los bloques del panel.
	HuecoPanel = 4 * 0.25 * tema.Rem
)

// AltoPestanaEditor es py-2.5 text-xs border-b-2: la línea compartida y sus bordes.
var AltoPestanaEditor = 2*tema.Espacio(2.5) + tema.TextoXS.Linea + 2*tema.Borde

// Editor son los datos del espacio de trabajo de edición.
type Editor struct {
	Foto api.Photo
	// Receta es la de ahora; Sucia, que no es la guardada.
	Receta api.Receta
	Sucia  bool
	// Lienzo es el lienzo vivo, que la app conserva entre repintados para no perder la
	// fuente ni el marco a medio dibujar; nil, uno vacío.
	Lienzo *LienzoEditor
	// Cargando es «todavía no ha llegado la foto»: la rueda en el hueco del lienzo.
	Cargando, Guardando bool
	// Aviso es el `role="status"` del pie del panel.
	Aviso string
	// VolverDrive cambia «Volver a la galería» por «Volver a Sytue Drive»: la foto se
	// abrió desde una carpeta de Drive y la salida devuelve a donde se entró.
	VolverDrive bool
	// PestanaIA enseña la pestaña de Sytue AI en vez de la de ajustes.
	PestanaIA bool
	// Recortando enciende el modo recorte; HayMarco, que ya hay uno que aplicar.
	Recortando, HayMarco bool
	// Proporcion es la del recorte (0, libre).
	Proporcion float64
	Fondo      FondoLienzo

	// Peticion es el área de texto de Sytue AI, que vive fuera para no perder lo escrito.
	Peticion *AreaFicha
	// Mandos son los cuatro deslizadores, que también viven fuera: la pantalla se rehace
	// a cada movimiento y el que se está arrastrando tiene que seguir siendo el mismo.
	// nil: se crean aquí (lo que hacen los guardianes).
	Mandos map[string]*ui.Deslizador
	// Comparar enseña la foto sin los ajustes de color («Comparar con el original»).
	Comparar bool
	// IACargando, IAError y IAResultado son la llamada a Sytue AI en curso, su fallo y
	// lo que contestó.
	IACargando  bool
	IAError     string
	IAResultado *api.EdicionIA

	AlVolver, AlDeshacer, AlGuardar           func()
	AlComparar                                func()
	AlAjuste                                  func(tipo string, valor float64)
	AlPestana                                 func(ia bool)
	AlRecortar, AlGirar, AlEspejoH, AlEspejoV func()
	AlRestablecer                             func()
	AlProporcion                              func(float64)
	AlAplicarRecorte, AlCancelarRecorte       func()
	AlFondo                                   func(FondoLienzo)
	// AlIA pide a Sytue AI un preajuste (su clave de api.PreajustesIA) o, con "", lo
	// escrito en Peticion.
	AlIA func(accion string)
}

// PantallaEditor monta el editor. NO lleva el marco de la suite: es un espacio de
// trabajo a pantalla completa, y en la web tampoco usa `Marco`.
func PantallaEditor(v Vista, d Editor) *fyne.Container {
	if d.Fondo == "" {
		d.Fondo = FondoCrema
	}
	lienzo := d.Lienzo
	if lienzo == nil {
		lienzo = NuevoLienzoEditor()
	}
	hueco := []fyne.CanvasObject{lienzo}
	if d.Cargando {
		r := ui.NuevaRueda(ui.LadoRueda, "text", v.Variante)
		r.Rotulo = v.Traduce("editor.loading")
		hueco = append(hueco, r)
	}
	movil := v.Ventana < ui.CorteMD
	anchoPanel := AnchoPanelEditor - tema.Borde
	if movil {
		anchoPanel = v.Ventana
	}
	panel := container.NewVScroll(ui.AlAncho(anchoPanel, panelEditor(v, d)))
	// El hueco del lienzo va el ÚLTIMO: el velo del recorte se sale de él y tiene que
	// quedar encima de la cabecera y del panel (ver lienzoEditorRenderer.Layout).
	return container.New(&editorLayout{movil: movil, panel: panel},
		canvas.NewRectangle(tema.Color("bg", v.Variante)),
		cabeceraEditor(v, d),
		canvas.NewRectangle(tema.Color("border", v.Variante)),
		canvas.NewRectangle(tema.Color("bg-subtle", v.Variante)),
		canvas.NewRectangle(tema.Color("border", v.Variante)),
		panel,
		container.New(&huecoLienzo{rueda: d.Cargando}, hueco...),
	)
}

// cabeceraEditor es el `<header>`: la salida, el nombre, el motor y los pasos, deshacer
// y guardar.
func cabeceraEditor(v Vista, d Editor) fyne.CanvasObject {
	clave := "editor.back"
	if d.VolverDrive {
		clave = "editor.backToDrive"
	}
	// El `Button` de la casa y no un enlace de texto: medía 18 de alto, por debajo de los
	// 24 de WCAG 2.5.8 y de los 44 del dedo.
	volver := ui.NuevoBoton(v.Traduce(clave), d.AlVolver).Con(ui.Fantasma, ui.TallaSM).ConIcono(iconos.ChevronIzquierda, 0)
	// Solo los pasos: «CPU» era un dato del motor, no de la foto.
	estado := v.T.T("editor.steps", i18n.Vars{"count": len(d.Receta.Pasos)})
	comparar := ui.NuevoBotonIcono(iconos.Ojo, v.Traduce("editor.compare"), d.AlComparar)
	comparar.Lado, comparar.TamIcono = ui.LadoIconoMD, IconoMando
	comparar.Pulsado, comparar.Inerte = d.Comparar, !conAjusteDeColor(d.Receta)
	if d.Comparar {
		comparar.Trazo = "text" // `bg-selected text-text`
	}
	deshacer := ui.NuevoBotonIcono(iconos.Deshacer, v.Traduce("editor.undo"), d.AlDeshacer)
	deshacer.Lado, deshacer.TamIcono = ui.LadoIconoMD, IconoMando
	deshacer.Inerte = len(d.Receta.Pasos) == 0
	guardar := ui.NuevoBoton(v.Traduce("editor.save"), d.AlGuardar).Con(ui.Primario, ui.TallaSM)
	guardar.Cargando, guardar.Inerte = d.Guardando, !d.Sucia
	textoEstado := ParrafoEn(estado, tema.TextoXS, ui.Normal, "text-muted", v.Variante)
	// El nombre es `truncate`: en un teléfono encoge hasta lo que dejan los demás.
	nombre := ParrafoEn(d.Foto.Name, tema.TextoSM, ui.Medio, "text", v.Variante)
	if v.Ventana > 0 {
		libre := v.Ventana - 2*RellenoVisorX - 5*tema.Espacio(2)
		for _, o := range []fyne.CanvasObject{volver, textoEstado, comparar, deshacer, guardar} {
			libre -= o.MinSize().Width
		}
		ui.Truncar(nombre, d.Foto.Name, tema.TextoSM, ui.Medio, max(libre, 0))
	}
	return container.New(ui.FilaAuto{Hueco: tema.Espacio(2), Derecha: 2}, volver, nombre, textoEstado, comparar, deshacer, guardar)
}

// conAjusteDeColor dice si la receta lleva algún ajuste de color: sin él no hay nada que
// comparar con el original (`receta.pasos.some(esDeColor)` en la web).
func conAjusteDeColor(r api.Receta) bool {
	for _, p := range r.Pasos {
		if !p.EsDeGeometria() {
			return true
		}
	}
	return false
}

// sinColor es la receta con solo su geometría: lo que enseña «Comparar con el original».
func sinColor(r api.Receta) api.Receta {
	g := api.Receta{Version: r.Version}
	for _, p := range r.Pasos {
		if p.EsDeGeometria() {
			g.Pasos = append(g.Pasos, p)
		}
	}
	return g
}

// panelEditor es el `<aside class="flex flex-col gap-4 p-4">`: las dos pestañas y lo que
// toque, el aviso y los atajos.
func panelEditor(v Vista, d Editor) fyne.CanvasObject {
	var cuerpo []ui.Bloque
	if d.PestanaIA {
		cuerpo = pestanaIA(v, d)
	} else {
		cuerpo = pestanaAjustes(v, d)
	}
	bloques := make([]ui.Bloque, 0, 1+len(cuerpo)+2)
	bloques = append(bloques, ui.Bloque{Obj: pestanasEditor(v, d), Margen: tema.Espacio(1) + HuecoPanel})
	for _, b := range cuerpo {
		b.Margen += HuecoPanel
		bloques = append(bloques, dentro(b))
	}
	// `min-h-5`: el aviso ocupa su renglón aunque esté vacío.
	bloques = append(bloques,
		dentro(ui.Bloque{
			Obj:  ParrafoEn(d.Aviso, tema.TextoXS, ui.Normal, "text-muted", v.Variante),
			Alto: func(float32) float32 { return 5 * 0.25 * tema.Rem }, Margen: HuecoPanel,
		}),
		dentro(bloqueLargo(ui.NuevoParrafo(ui.AtajoDelSistema(v.Traduce("editor.shortcuts")), tema.TextoXS, ui.Normal, "text-muted", v.Variante), RellenoPanel)),
	)
	return ui.PilaBloques(bloques...)
}

// dentro mete un bloque en el `p-4` del panel. Las pestañas no lo llevan: van a sangre
// con su `-mx-4 -mt-4`.
func dentro(b ui.Bloque) ui.Bloque {
	o := container.New(ui.Margen{Izq: RellenoPanel, Der: RellenoPanel}, b.Obj)
	alto := b.Alto
	if alto == nil {
		alto = func(ancho float32) float32 { return ui.AltoA(b.Obj, ancho) }
	}
	return ui.Bloque{Obj: o, Margen: b.Margen, Alto: func(ancho float32) float32 { return alto(ancho - 2*RellenoPanel) }}
}

// pestanasEditor son las dos de la cabecera del panel: «Ajustes» y «Sytue AI», a partes
// iguales y con el filete del panel debajo.
func pestanasEditor(v Vista, d Editor) fyne.CanvasObject {
	ajustes := NuevaPestanaEditor(v.Traduce("editor.tabAdjustments"), iconos.Ninguno, !d.PestanaIA, func() {
		if d.AlPestana != nil {
			d.AlPestana(false)
		}
	})
	ia := NuevaPestanaEditor(v.Traduce("editor.tabAi"), iconos.IA, d.PestanaIA, func() {
		if d.AlPestana != nil {
			d.AlPestana(true)
		}
	})
	filete := canvas.NewRectangle(tema.Color("border", v.Variante))
	return container.New(tiraEditor{}, filete, ajustes, ia)
}

// tiraEditor reparte las dos pestañas a partes iguales (`flex-1`) con el `border-b` de
// la tira debajo.
type tiraEditor struct{}

func (tiraEditor) MinSize([]fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(0, AltoPestanaEditor+tema.Borde)
}

func (tiraEditor) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	objs[0].Move(fyne.NewPos(0, AltoPestanaEditor))
	objs[0].Resize(fyne.NewSize(t.Width, tema.Borde))
	mitad := t.Width / 2
	for i, o := range objs[1:] {
		o.Move(fyne.NewPos(float32(i)*mitad, 0))
		o.Resize(fyne.NewSize(mitad, AltoPestanaEditor))
	}
}

// pestanaAjustes: los cuatro deslizadores, el recorte, las herramientas y el fondo.
func pestanaAjustes(v Vista, d Editor) []ui.Bloque {
	var bloques []ui.Bloque
	for _, tipo := range Deslizadores {
		bloques = append(bloques, ui.Bloque{Obj: filaAjuste(v, d, tipo)})
	}
	if d.Recortando {
		bloques = append(bloques, ui.Bloque{Obj: cajaRecorte(v, d)})
	}
	recortar := ui.NuevoBotonIcono(iconos.Recorte, v.Traduce("editor.crop"), d.AlRecortar)
	recortar.Pulsado = d.Recortando
	herramientas := make([]fyne.CanvasObject, 0, 5)
	for _, b := range [...]*ui.BotonIcono{
		recortar,
		ui.NuevoBotonIcono(iconos.Rotar, v.Traduce("editor.rotate"), d.AlGirar),
		ui.NuevoBotonIcono(iconos.VoltearH, v.Traduce("editor.flipH"), d.AlEspejoH),
		ui.NuevoBotonIcono(iconos.VoltearV, v.Traduce("editor.flipV"), d.AlEspejoV),
	} {
		b.Lado, b.TamIcono = ui.LadoIconoMD, IconoMando
		herramientas = append(herramientas, b)
	}
	restablecer := ui.NuevoBoton(v.Traduce("editor.reset"), d.AlRestablecer).Con(ui.Fantasma, ui.TallaSM)
	restablecer.Inerte = len(d.Receta.Pasos) == 0
	herramientas = append(herramientas, restablecer)
	bloques = append(bloques,
		ui.Bloque{Obj: container.New(ui.Fila{Hueco: tema.Espacio(2)}, herramientas...)},
		ui.Bloque{Obj: selectorFondo(v, d)},
	)
	return bloques
}

// filaAjuste es un deslizador con su rótulo y su valor (`toFixed(2)`), en `flex-col
// gap-1 text-sm`. Doble clic lo devuelve a 0, como `onDblClick` en la web.
func filaAjuste(v Vista, d Editor, tipo string) fyne.CanvasObject {
	clave := claveAjuste(tipo)
	valor := valorDe(d.Receta, tipo)
	signo := ""
	if valor > 0 {
		signo = "+"
	}
	txtValor := signo + i18n.FormatFixed("en", valor, 2)
	// La fila del rótulo mide su caja de línea (`text-sm`, 15,75) y no la de la fuente.
	rotulo := container.New(ui.FilaAuto{Hueco: tema.Espacio(2), Derecha: 1, AlturaFila: tema.TextoSM.Linea},
		ParrafoEn(v.Traduce(clave), tema.TextoSM, ui.Normal, "text", v.Variante),
		// `toFixed(2)` con signo: como en la web.
		ParrafoEn(txtValor, tema.TextoSM, ui.Normal, "text-muted", v.Variante),
	)
	s := d.Mandos[tipo]
	if s == nil {
		minimo, maximo := rangoAjuste(tipo)
		s = ui.NuevoDeslizador(minimo, maximo, valor, func(x float64) {
			if d.AlAjuste != nil {
				d.AlAjuste(tipo, x)
			}
		}).Con(v.Traduce(clave))
		s.Bipolar = true
		s.TextoValor = txtValor
		s.AlDobleClic = func() {
			if d.AlAjuste != nil {
				d.AlAjuste(tipo, 0)
			}
		}
	} else {
		s.Bipolar = true
		s.TextoValor = txtValor
		if s.Valor != valor {
			// Deshacer, restablecer o Sytue AI lo han movido: se coloca sin avisar.
			s.Valor = valor
			s.Refresh()
		}
	}
	// El `<input type="range">` del editor mide 16 de alto en Chromium (medido contra la
	// web con el servicio simulado): el deslizador se centra en ese hueco.
	return ui.PilaBloques(ui.Bloque{Obj: rotulo, Margen: tema.Espacio(1)}, ui.Bloque{Obj: container.New(fijo{alto: altoRango}, s)})
}

// altoRango es la caja del `<input type="range">` del editor, medida en Chromium.
const altoRango = 16

// rangoAjuste es el recorrido de cada deslizador, el de `RANGOS` en receta.ts: la
// exposición de −5 a 5 y las otras tres de −1 a 1. Darles uno común dejaría la
// exposición moviéndose cinco veces más despacio que en la web.
func rangoAjuste(tipo string) (float64, float64) {
	if tipo == api.PasoExposicion {
		return -5, 5
	}
	return -1, 1
}

// claveAjuste es el rótulo de cada deslizador (`ETIQUETA` de Editor.tsx).
func claveAjuste(tipo string) string {
	switch tipo {
	case api.PasoContraste:
		return "editor.contrast"
	case api.PasoSaturacion:
		return "editor.saturation"
	case api.PasoTemperatura:
		return "editor.temperature"
	}
	return "editor.exposure"
}

// cajaRecorte es el `role="group"` del recorte: `bg-bg-subtle rounded-sm p-2 gap-2`, la
// pista, las proporciones y Aplicar/Cancelar.
func cajaRecorte(v Vista, d Editor) fyne.CanvasObject {
	proporciones := make([]fyne.CanvasObject, len(Proporciones))
	for i, p := range Proporciones {
		rotulo := etiquetaProporcion(p)
		if p == 0 {
			rotulo = v.Traduce("editor.cropFree")
		}
		b := NuevoBotonEditor(rotulo, func() {
			if d.AlProporcion != nil {
				d.AlProporcion(p)
			}
		})
		// `rounded-sm border px-2 py-0.5 text-xs`; la elegida en `border-primary
		// bg-selected text-primary-text`.
		b.MargenX, b.MargenY, b.Radio, b.Elegido = tema.Espacio(2), tema.Espacio(0.5), tema.RadioSM, d.Proporcion == p
		b.Estilo, b.Debajo = estiloOpcion, "bg-subtle" // van dentro de la caja `bg-bg-subtle`
		proporciones[i] = b
	}
	aplicar := ui.NuevoBoton(v.Traduce("editor.cropApply"), d.AlAplicarRecorte).Con(ui.Primario, ui.TallaSM)
	aplicar.Inerte = !d.HayMarco
	return recuadro(v, "bg-subtle", "", tema.RadioSM,
		ui.Margen{Izq: tema.Espacio(2), Der: tema.Espacio(2), Arr: tema.Espacio(2), Abj: tema.Espacio(2)},
		ui.PilaBloques(
			bloqueLargo(ui.NuevoParrafo(v.Traduce("editor.cropHint"), tema.TextoXS, ui.Normal, "text-muted", v.Variante), tema.Espacio(2)),
			ui.Bloque{Obj: container.New(ui.Fila{Hueco: tema.Espacio(1)}, proporciones...), Margen: tema.Espacio(2)},
			ui.Bloque{Obj: container.New(ui.Fila{Hueco: tema.Espacio(2)}, aplicar,
				ui.NuevoBoton(v.Traduce("editor.cropCancel"), d.AlCancelarRecorte).Con(ui.Fantasma, ui.TallaSM))},
		))
}

// etiquetaProporcion es `ETIQUETA_PROPORCION`: el mismo texto en todos los idiomas.
func etiquetaProporcion(p float64) string {
	switch p {
	case 1:
		return "1:1"
	case 4.0 / 3:
		return "4:3"
	case 3.0 / 2:
		return "3:2"
	case 16.0 / 9:
		return "16:9"
	}
	return ""
}

// estiloOpcion es el de una opción que se elige (proporción y fondo): la elegida en
// `border-primary bg-selected text-primary-text`, las demás en `border-border` y
// `hover:bg-bg-subtle`.
//
// Los fondos se componen sobre el `bg` del panel: `selected` es un tinte translúcido, y
// la caja de `ui` pinta el borde DEBAJO del relleno, así que un relleno transparente
// dejaría ver el color del borde entero.
func estiloOpcion(v tema.Variante, b *BotonEditor) (fondo, borde, tinta color.NRGBA) {
	panel := tema.Color("bg", v)
	if b.Debajo != "" {
		panel = tema.Color(b.Debajo, v)
	}
	if b.Elegido {
		return sobre(tema.Color("selected", v), panel), tema.Color("primary", v), tema.Color("primary-text", v)
	}
	fondo = panel
	if b.Encima() {
		fondo = tema.Color("bg-subtle", v)
	}
	tinta = tema.Color("text-muted", v)
	if b.TintaNormal != "" {
		tinta = tema.Color(b.TintaNormal, v)
	}
	return fondo, tema.Color("border", v), tinta
}

// ---- El fondo del lienzo ----

// Las muestras de los cuatro fondos son LITERALES de la web, no tokens: son el color de
// un papel (`bg-[#faf9f6]`, `border-[#d8d3c6]`…) y los grises por defecto de Tailwind 4
// (`gray-300`, `gray-700`) y el `#cbd5e1` del damero.
var (
	bordeMuestra = map[FondoLienzo]color.NRGBA{
		FondoCrema:  {R: 0xd8, G: 0xd3, B: 0xc6, A: 255},
		FondoBlanco: {R: 0xd1, G: 0xd5, B: 0xdc, A: 255},
		FondoOscuro: {R: 0x36, G: 0x41, B: 0x53, A: 255},
		FondoDamero: {R: 0xd1, G: 0xd5, B: 0xdc, A: 255},
	}
	cuadroDamero = color.NRGBA{R: 0xcb, G: 0xd5, B: 0xe1, A: 255}
)

// muestraFondo es el círculo de color de un fondo (`rounded-full border`), de ese lado
// y con la baldosa del damero de `baldosa` px (`bg-[length:…]`): la de la barra
// flotante (h-6, 8), la del panel (h-3,5, 6) y la del menú del visor (h-3, 6).
//
// Es un SVG por `iconos.DeSVG`, que se pinta a los píxeles del dispositivo y se deja
// estirar (el `scale-110` de la elegida), y porque es lo que `ui.ElementoMenu` acepta
// como glifo propio. Se pinta en el orden de CSS: el relleno, los cuadros del damero
// —con la baldosa empezando en la caja de relleno, 1 px dentro— y el borde encima.
// `oksvg` no sabe de `clipPath` ni de `pattern`, así que los cuadros se recortan aquí
// al círculo de dentro del borde.
func muestraFondo(f FondoLienzo, lado, baldosa float32) *iconos.Dibujo {
	hex := func(c color.NRGBA) string { return fmt.Sprintf("#%02x%02x%02x", c.R, c.G, c.B) }
	l, r, paso := float64(lado), float64(lado)/2, float64(baldosa)
	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %[1]g %[1]g" width="%[1]g" height="%[1]g">`, l)
	fmt.Fprintf(&b, `<circle cx="%[1]g" cy="%[1]g" r="%[1]g" fill="%[2]s"/>`, r, hex(bordeMuestra[f]))
	fmt.Fprintf(&b, `<circle cx="%[1]g" cy="%[1]g" r="%[2]g" fill="%[3]s"/>`, r, r-1, hex(f.colorFondo()))
	if f == FondoDamero {
		// El 0 %–25 % del gradiente cónico es el cuarto de arriba a la derecha de cada
		// baldosa, y el 50 %–75 %, el de abajo a la izquierda.
		for y := 1.0; y < l-1; y += paso {
			for x := 1.0; x < l-1; x += paso {
				for _, o := range [][2]float64{{x + paso/2, y}, {x, y + paso/2}} {
					q := paso / 2
					cuadro := [][2]float64{{o[0], o[1]}, {o[0] + q, o[1]}, {o[0] + q, o[1] + q}, {o[0], o[1] + q}}
					p := recortarACirculo(cuadro, r, r-1)
					if len(p) < 3 {
						continue
					}
					fmt.Fprintf(&b, `<path fill="%s" d="M`, hex(cuadroDamero))
					for i, v := range p {
						if i > 0 {
							b.WriteString(" L")
						}
						fmt.Fprintf(&b, "%.3f %.3f", v[0], v[1])
					}
					b.WriteString(` Z"/>`)
				}
			}
		}
	}
	b.WriteString(`</svg>`)
	return iconos.DeSVG([]byte(b.String()), fyne.NewSize(lado, lado))
}

// recortarACirculo es Sutherland–Hodgman del polígono contra el círculo de centro
// (c, c) y radio r, hecho un polígono de 64 lados: a 24 px la diferencia con el arco no
// llega a un píxel de dispositivo.
func recortarACirculo(poli [][2]float64, c, r float64) [][2]float64 {
	const lados = 64
	cruz := func(a, b [2]float64) float64 { return a[0]*b[1] - a[1]*b[0] }
	resta := func(a, b [2]float64) [2]float64 { return [2]float64{a[0] - b[0], a[1] - b[1]} }
	for i := 0; i < lados && len(poli) > 0; i++ {
		a0, a1 := 2*math.Pi*float64(i)/lados, 2*math.Pi*float64(i+1)/lados
		p := [2]float64{c + r*math.Cos(a0), c + r*math.Sin(a0)}
		e := resta([2]float64{c + r*math.Cos(a1), c + r*math.Sin(a1)}, p)
		dentro := func(v [2]float64) bool { return cruz(e, resta(v, p)) >= 0 }
		var sale [][2]float64
		for j, cur := range poli {
			prev := poli[(j+len(poli)-1)%len(poli)]
			if dentro(cur) != dentro(prev) {
				d := resta(cur, prev)
				t := -cruz(e, resta(prev, p)) / cruz(e, d)
				sale = append(sale, [2]float64{prev[0] + t*d[0], prev[1] + t*d[1]})
			}
			if dentro(cur) {
				sale = append(sale, cur)
			}
		}
		poli = sale
	}
	return poli
}

// selectorFondo es el bloque del fondo del panel: `border-t pt-3 gap-1.5 text-xs
// font-medium`, el rótulo con la rejilla y los cuatro en `grid-cols-2 gap-1.5`.
func selectorFondo(v Vista, d Editor) fyne.CanvasObject {
	rotulo := container.New(ui.Fila{Hueco: tema.Espacio(1)},
		iconos.Trazo(iconos.Rejilla, 13, "text-muted", v.Variante).Objeto(),
		ParrafoEn(v.Traduce("editor.canvasBackground"), tema.TextoXS, ui.Medio, "text-muted", v.Variante))
	opciones := make([]fyne.CanvasObject, len(FondosLienzo))
	for i, f := range FondosLienzo {
		b := NuevoBotonEditor(v.Traduce(f.clave()), func() {
			if d.AlFondo != nil {
				d.AlFondo(f)
			}
		})
		// `rounded-md border px-2.5 py-1.5 text-xs gap-2` con la muestra de 3,5.
		b.MargenX, b.MargenY, b.Radio, b.Hueco = tema.Espacio(2.5), tema.Espacio(1.5), tema.RadioMD, tema.Espacio(2)
		b.Glifo, b.LadoGlifo = muestraFondo(f, tema.Espacio(3.5), 6).Objeto(), tema.Espacio(3.5)
		b.Elegido, b.TintaNormal, b.Estilo = f == d.Fondo, "text", estiloOpcion
		if b.Elegido {
			b.Peso = ui.SemiNegrita
		}
		opciones[i] = b
	}
	filete := canvas.NewRectangle(tema.Color("border", v.Variante))
	return ui.PilaBloques(
		ui.Bloque{Obj: filete, Alto: func(float32) float32 { return tema.Borde }, Margen: tema.Espacio(3)},
		ui.Bloque{Obj: rotulo, Margen: tema.Espacio(1.5)},
		ui.Bloque{Obj: container.New(ui.Rejilla{Fijas: 2, Hueco: tema.Espacio(1.5)}, opciones...)},
	)
}

// fondosFlotantes es la barra que flota sobre el lienzo, abajo a la izquierda:
// `bg-surface/90 border-border/80 rounded-full border px-2.5 py-1.5 shadow-md gap-1.5`,
// el rótulo (`max-sm:hidden`) y las cuatro muestras de `h-6 w-6`. En traslúcido
// conserva el papel opaco de los paneles de opciones, para leer sobre cualquier foto.
func fondosFlotantes(v Vista, d Editor) fyne.CanvasObject {
	hijos := []fyne.CanvasObject{iconos.Trazo(iconos.Rejilla, 13, "text-muted", v.Variante).Objeto()}
	if v.Ventana >= ui.CorteSM {
		hijos = append(hijos, ParrafoEn(v.Traduce("editor.canvasBackground"), tema.TextoXS, ui.Medio, "text-muted", v.Variante))
	}
	rotulo := container.New(ui.Margen{Der: tema.Espacio(1)}, container.New(ui.Fila{Hueco: tema.Espacio(1)}, hijos...))
	muestras := make([]fyne.CanvasObject, 0, 1+len(FondosLienzo))
	muestras = append(muestras, rotulo)
	for _, f := range FondosLienzo {
		muestras = append(muestras, NuevaMuestraFondo(v, f, f == d.Fondo, func() {
			if d.AlFondo != nil {
				d.AlFondo(f)
			}
		}))
	}
	superficie := tema.Color("surface", v.Variante)
	fondo := conAlfa(superficie, 0.9)
	if v.Variante == tema.Translucido {
		fondo = tema.Color("menu-surface", v.Variante)
	}
	borde := conAlfa(tema.Color("border", v.Variante), 0.8)
	barra := recuadroCon(fondo, borde, tema.RadioPill,
		ui.Margen{Izq: tema.Espacio(2.5), Der: tema.Espacio(2.5), Arr: tema.Espacio(1.5), Abj: tema.Espacio(1.5)},
		container.New(ui.Fila{Hueco: tema.Espacio(1.5)}, muestras...))
	if d.Lienzo == nil || v.Variante == tema.Translucido {
		// La placa opaca ya tapa la imagen: desenfocarla debajo no cambia nada.
		return conSombra(tema.SombraMD, tema.RadioPill, barra)
	}
	// `backdrop-blur-md`: el lienzo de debajo, desenfocado, detrás del `bg-surface/90`.
	// La coloca huecoLienzo, que es quien sabe dónde cae sobre el lienzo.
	fd := NuevoFondoDesenfocado(SigmaBackdropMD, tema.RadioPill, d.Lienzo.debajo(tema.Color("bg-subtle", v.Variante)))
	return conSombra(tema.SombraMD, tema.RadioPill, container.New(capaDesenfocada{fd}, fd, barra))
}

// huecoLienzo coloca el lienzo en todo el hueco (ya con su `p-3`) y, encima, la rueda
// en el centro mientras llega la foto o la barra de fondos a `bottom-4 left-4`.
type huecoLienzo struct{ rueda bool }

func (*huecoLienzo) MinSize([]fyne.CanvasObject) fyne.Size { return fyne.NewSize(0, 0) }

func (h *huecoLienzo) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	objs[0].Move(fyne.NewPos(RellenoEditor, RellenoEditor))
	objs[0].Resize(fyne.NewSize(max(t.Width-2*RellenoEditor, 0), max(t.Height-2*RellenoEditor, 0)))
	if len(objs) < 2 {
		return
	}
	m := objs[1].MinSize()
	pos := fyne.NewPos(tema.Espacio(4), t.Height-tema.Espacio(4)-m.Height)
	if h.rueda {
		pos = fyne.NewPos((t.Width-m.Width)/2, (t.Height-m.Height)/2)
	}
	objs[1].Move(pos)
	objs[1].Resize(m)
	// El desenfoque de la barra, en las coordenadas del lienzo (que empieza en el `p-3`).
	if fd := desenfocadoEn(objs[1]); fd != nil {
		fd.Colocar(pos.SubtractXY(RellenoEditor, RellenoEditor), m.Width)
	}
}

// ---- La pestaña de Sytue AI ----

// NuevaAreaPeticion es el `<textarea rows={3} class="rounded-sm border p-2 text-xs
// bg-surface">` de la petición libre a Sytue AI.
func NuevaAreaPeticion(v Vista) *AreaFicha {
	return NuevaAreaFicha(v.Traduce("editor.aiPromptPlaceholder"), tema.TextoXS, 3,
		tema.Espacio(2), tema.Espacio(2), tema.RadioSM, "surface")
}

// clavesPreajuste son los ocho de `PREAJUSTES_IA`, en su orden: la clave que se le manda
// al servicio y su rótulo.
var clavesPreajuste = [...][2]string{
	{"mejora-auto", "editor.aiPresetAuto"},
	{"atardecer", "editor.aiPresetGolden"},
	{"cinematografico", "editor.aiPresetCinematic"},
	{"bn-dramatico", "editor.aiPresetBw"},
	{"vivido", "editor.aiPresetVivid"},
	{"vintage", "editor.aiPresetVintage"},
	{"reencuadre", "editor.aiPresetCrop"},
	{"noche", "editor.aiPresetNight"},
}

// pestanaIA: el título, la petición libre, los preajustes y lo que contesta.
func pestanaIA(v Vista, d Editor) []ui.Bloque {
	titulo := container.New(ui.Fila{Hueco: tema.Espacio(1.5)},
		iconos.Trazo(iconos.IA, iconos.TallaSM, "text", v.Variante).Objeto(),
		ParrafoEn(v.Traduce("editor.aiTitle"), tema.TextoSM, ui.SemiNegrita, "text", v.Variante))
	campo := d.Peticion
	if campo == nil {
		campo = NuevaAreaPeticion(v)
	}
	// `<Icon name="ai" class="size-3.5" />` y apagado sin petición o mientras contesta.
	aplicar := ui.NuevoBoton(v.Traduce("editor.aiApply"), func() {
		if d.AlIA != nil {
			d.AlIA("")
		}
	}).Con(ui.Primario, ui.TallaSM).ConIcono(iconos.IA, 3.5*0.25*tema.Rem)
	aplicar.Cargando = d.IACargando
	aplicar.Inerte = d.IACargando || strings.TrimSpace(campo.Texto()) == ""
	preajustes := make([]fyne.CanvasObject, len(clavesPreajuste))
	for i, p := range clavesPreajuste {
		b := NuevoBotonEditor(v.Traduce(p[1]), func() {
			if d.AlIA != nil {
				d.AlIA(p[0])
			}
		})
		// `bg-surface border-border rounded-sm border px-2.5 py-2 text-left text-xs`.
		b.MargenX, b.MargenY, b.Radio, b.Inerte = tema.Espacio(2.5), tema.Espacio(2), tema.RadioSM, d.IACargando
		b.Estilo = func(v tema.Variante, b *BotonEditor) (color.NRGBA, color.NRGBA, color.NRGBA) {
			fondo := tema.Color("surface", v)
			if b.Encima() {
				fondo = tema.Color("bg-subtle", v)
			}
			tinta := tema.Color("text", v)
			if b.Inerte {
				tinta = conAlfa(tinta, 0.4) // `disabled:opacity-40`
			}
			return fondo, tema.Color("border", v), tinta
		}
		preajustes[i] = b
	}
	bloques := []ui.Bloque{
		{Obj: ui.PilaBloques(
			ui.Bloque{Obj: titulo, Margen: tema.Espacio(0.5)},
			bloqueLargo(ui.NuevoParrafo(v.Traduce("editor.aiDescription"), tema.TextoXS, ui.Normal, "text-muted", v.Variante), 0),
		)},
		{Obj: ui.PilaBloques(ui.Bloque{Obj: campo, Margen: tema.Espacio(2)}, ui.Bloque{Obj: aplicar})},
		{Obj: ui.PilaBloques(
			ui.Bloque{Obj: versalEn(v.Traduce("editor.aiPresets"), "text-muted", v.Variante), Margen: tema.Espacio(1.5)},
			ui.Bloque{Obj: container.New(ui.Rejilla{Fijas: 2, Hueco: tema.Espacio(1.5)}, preajustes...)},
		)},
	}
	if d.IACargando {
		r := ui.NuevaRueda(ui.LadoRueda, "text-muted", v.Variante)
		r.Rotulo = v.Traduce("editor.aiProcessing")
		fila := container.NewCenter(container.New(ui.Fila{Hueco: tema.Espacio(2)}, r,
			ParrafoEn(v.Traduce("editor.aiProcessing"), tema.TextoXS, ui.Normal, "text-muted", v.Variante)))
		bloques = append(bloques, ui.Bloque{Obj: recuadro(v, "bg-subtle", "", tema.RadioSM,
			ui.Margen{Izq: tema.Espacio(3), Der: tema.Espacio(3), Arr: tema.Espacio(3), Abj: tema.Espacio(3)}, fila)})
	}
	if d.IAError != "" {
		// `text-danger-text bg-danger-bg rounded-sm p-2`: la pareja de error de la suite.
		// El rojo de relleno sobre su velo al 10 % medía 4,42:1 en claro y 2,96 en oscuro
		// (web, PR #840).
		bloques = append(bloques, ui.Bloque{Obj: recuadro(v, "danger-bg", "", tema.RadioSM,
			ui.Margen{Izq: tema.Espacio(2), Der: tema.Espacio(2), Arr: tema.Espacio(2), Abj: tema.Espacio(2)},
			ui.PilaBloques(bloqueLargo(ui.NuevoParrafo(d.IAError, tema.TextoXS, ui.Normal, "danger-text", v.Variante), 0)))})
	}
	if d.IAResultado != nil {
		bloques = append(bloques, ui.Bloque{Obj: tarjetaResultadoIA(v, d)})
	}
	return bloques
}

// tarjetaResultadoIA es la tarjeta de lo que hizo Sytue AI: `bg-bg-subtle border
// rounded-sm p-3 gap-2 text-xs`, el título con el enlace a los ajustes, la explicación
// en cursiva y una píldora por ajuste.
func tarjetaResultadoIA(v Vista, d Editor) fyne.CanvasObject {
	r := d.IAResultado
	titulo := container.New(ui.Fila{Hueco: tema.Espacio(1)},
		iconos.Trazo(iconos.IA, 12.25, "text", v.Variante).Objeto(),
		ParrafoEn(v.Traduce("editor.aiResultTitle"), tema.TextoXS, ui.Medio, "text", v.Variante))
	ver := NuevoEnlace(v.Traduce("editor.aiViewManual")+" →", func() {
		if d.AlPestana != nil {
			d.AlPestana(false)
		}
	})
	// Hereda el `text-xs` de la tarjeta, con `hover:underline`.
	ver.Con(tema.TextoXS, ui.Normal)
	ver.Tinta, ver.SubrayadoEncima = "primary", true
	// `italic`, con la Figtree Italic de verdad (ui.Cursiva), la misma que carga la web
	// desde `@sytue/ui/styles.css`.
	explicacion := ui.NuevoParrafo("«"+r.Explicacion+"»", tema.Escalon{Tam: tema.TextoXS.Tam, Linea: 1.625 * tema.TextoXS.Tam}, ui.Cursiva, "text-muted", v.Variante)
	var pildoras []fyne.CanvasObject
	for _, a := range [...]struct {
		rotulo string
		valor  *float64
	}{{"Exp", r.Ajustes.Exposicion}, {"Cont", r.Ajustes.Contraste}, {"Sat", r.Ajustes.Saturacion}, {"Temp", r.Ajustes.Temperatura}} {
		if a.valor == nil || *a.valor == 0 {
			continue
		}
		pildoras = append(pildoras, pildoraIA(v, a.rotulo+": "+conSigno(*a.valor)))
	}
	if r.Recorte != nil {
		pildoras = append(pildoras, pildoraIA(v, v.Traduce("editor.crop")))
	}
	bloques := []ui.Bloque{
		{Obj: container.New(ui.FilaAuto{Hueco: tema.Espacio(2), Derecha: 1}, titulo, ver), Margen: tema.Espacio(2)},
		{Obj: explicacion, Alto: func(a float32) float32 { return ui.AltoParrafo(explicacion, a) }, Margen: tema.Espacio(2)},
	}
	if len(pildoras) > 0 {
		bloques = append(bloques, ui.Bloque{Obj: container.New(ui.Margen{Arr: tema.Espacio(1)}, container.New(ui.Fila{Hueco: tema.Espacio(1)}, pildoras...))})
	}
	return recuadro(v, "bg-subtle", "border", tema.RadioSM,
		ui.Margen{Izq: tema.Espacio(3), Der: tema.Espacio(3), Arr: tema.Espacio(3), Abj: tema.Espacio(3)}, ui.PilaBloques(bloques...))
}

// conSigno es el `+0.5` / `-0.3` de las píldoras: el número como lo escribe
// JavaScript, con su signo delante si es positivo.
func conSigno(x float64) string {
	s := strconv.FormatFloat(x, 'f', -1, 64)
	if x > 0 {
		return "+" + s
	}
	return s
}

// pildoraIA es `bg-surface border-border rounded border px-1.5 py-0.5 text-[11px]`.
func pildoraIA(v Vista, texto string) fyne.CanvasObject {
	return recuadro(v, "surface", "border", tema.RadioSM,
		ui.Margen{Izq: tema.Espacio(1.5), Der: tema.Espacio(1.5), Arr: tema.Espacio(0.5), Abj: tema.Espacio(0.5)},
		ParrafoEn(texto, escalon11, ui.Normal, "text", v.Variante))
}

// ---- El reparto de la pantalla ----

// editorLayout: la cabecera con su filete, y debajo el lienzo sobre `bg-subtle` y el
// panel. Desde `md:` el panel va a la derecha con `w-80` y su filete a la izquierda; por
// debajo va DEBAJO, a todo el ancho y con el filete arriba, y el lienzo se queda con al
// menos el 40 % del alto de la ventana (`max-md:min-h-[40dvh]`).
type editorLayout struct {
	movil bool
	panel *container.Scroll
}

func (*editorLayout) MinSize([]fyne.CanvasObject) fyne.Size { return fyne.NewSize(0, 0) }

func (l *editorLayout) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	objs[0].Resize(t)
	alto := AltoCabeceraVisor()
	objs[1].Move(fyne.NewPos(RellenoVisorX, RellenoVisorY))
	objs[1].Resize(fyne.NewSize(max(t.Width-2*RellenoVisorX, 0), LadoMandoVisor))
	objs[2].Move(fyne.NewPos(0, alto-tema.Borde))
	objs[2].Resize(fyne.NewSize(t.Width, tema.Borde))

	resto := max(t.Height-alto, 0)
	var lienzo, filete, panel fyne.Position
	var tamLienzo, tamFilete, tamPanel fyne.Size
	if l.movil {
		// El panel mide lo que pide su contenido, pero cede hasta dejarle al lienzo su
		// 40 % de la ventana (`max-md:shrink max-md:min-h-0 overflow-y-auto`).
		pide := l.panel.Content.MinSize().Height + tema.Borde
		altoPanel := min(pide, max(resto-0.4*t.Height, 0))
		altoLienzo := resto - altoPanel
		lienzo, tamLienzo = fyne.NewPos(0, alto), fyne.NewSize(t.Width, altoLienzo)
		filete, tamFilete = fyne.NewPos(0, alto+altoLienzo), fyne.NewSize(t.Width, tema.Borde)
		panel, tamPanel = fyne.NewPos(0, alto+altoLienzo+tema.Borde), fyne.NewSize(t.Width, max(altoPanel-tema.Borde, 0))
	} else {
		ancho := max(t.Width-AnchoPanelEditor, 0)
		lienzo, tamLienzo = fyne.NewPos(0, alto), fyne.NewSize(ancho, resto)
		filete, tamFilete = fyne.NewPos(ancho, alto), fyne.NewSize(tema.Borde, resto)
		panel, tamPanel = fyne.NewPos(ancho+tema.Borde, alto), fyne.NewSize(AnchoPanelEditor-tema.Borde, resto)
	}
	for i, p := range [...]struct {
		pos fyne.Position
		tam fyne.Size
	}{{lienzo, tamLienzo}, {filete, tamFilete}, {panel, tamPanel}, {lienzo, tamLienzo}} {
		objs[3+i].Move(p.pos)
		objs[3+i].Resize(p.tam)
	}
}
