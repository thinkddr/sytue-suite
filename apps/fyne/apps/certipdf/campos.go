// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package certipdf

import (
	"bytes"
	"context"
	"os"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/ficheros"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// Propuesta es un campo que el motor cree ver en la página (`op: detectar`): se PROPONE,
// y crearlo es otro paso.
type Propuesta struct {
	Tipo     string `json:"tipo"`
	Pagina   int    `json:"pagina"`
	Rect     Rect   `json:"rect"`
	Etiqueta string `json:"etiqueta,omitempty"`
}

// panelCampos es `Campos` de `Paneles.tsx`: el formulario del documento para rellenarlo,
// o, si no tiene ni un campo, `DetectarCampos`.
type panelCampos struct {
	l *PanelLateral
	// de es el contenido del que salen los valores: si cambia, se vuelven a leer (el
	// `useEffect` de la web sobre `contenido`). Lo escrito sobrevive a cambiar de pestaña.
	de      *Contenido
	valores map[string]string

	// DetectarCampos: nil es que no se ha buscado todavía.
	propuestas []Propuesta
	buscando   bool
	errorDet   string
	detectar   *Accion

	vista *fyne.Container
}

func (l *PanelLateral) campos() fyne.CanvasObject {
	if l.camposP == nil {
		l.camposP = &panelCampos{l: l, vista: container.New(ui.Columna{})}
	}
	c := l.camposP
	if c.de != l.c {
		c.de, c.valores = l.c, map[string]string{}
		for _, x := range c.rellenables() {
			c.valores[x.Nombre] = x.Valor
		}
	}
	c.rehacer()
	return c.vista
}

func (c *panelCampos) t(clave string) string { return c.l.t(clave, nil) }

// rellenables son los campos menos los botones, que no llevan valor.
func (c *panelCampos) rellenables() []Campo {
	var r []Campo
	for _, x := range c.l.c.Campos {
		if x.Tipo != "boton" {
			r = append(r, x)
		}
	}
	return r
}

func (c *panelCampos) rehacer() {
	campos := c.rellenables()
	if len(campos) == 0 {
		// Un impreso SIN campos es justo donde la detección vale: la solicitud en papel
		// escaneada, con rayas y cuadraditos y ni un campo.
		c.vista.Objects = []fyne.CanvasObject{c.detectarCampos()}
	} else {
		c.vista.Objects = []fyne.CanvasObject{c.formulario(campos)}
	}
	c.vista.Refresh()
}

// formulario es el `<form class="flex flex-col gap-2">`.
func (c *panelCampos) formulario(campos []Campo) fyne.CanvasObject {
	l, ancho := c.l, anchoUtil()
	var filas []fyne.CanvasObject
	if !l.c.Permisos.RellenarCampos && l.c.Permisos.SoloConClaveDeDueno {
		filas = append(filas, avisoAmbar(c.t("pdf.blocked.fill"), ancho))
	}
	for _, x := range campos {
		filas = append(filas, c.campoEditable(x, ancho))
	}
	guardar := botonPrimario(c.t("pdf.fields.save"), tema.TextoXS, tema.Espacio(2), tema.Espacio(1.5), func() {
		c.guardar(campos)
	})
	exportar := botonPanel(c.t("pdf.fields.export"), tema.TextoXS, tema.Espacio(1.5), c.exportar)
	importar := botonPanel(c.t("pdf.fields.import"), tema.TextoXS, tema.Espacio(1.5), c.importar)
	aplanar := botonPanel(c.t("pdf.fields.flatten"), tema.TextoXS, tema.Espacio(1.5), c.aplanar)
	botones := []*Accion{guardar, exportar, importar, aplanar}
	for _, b := range botones {
		b.Inerte = l.trabajando()
	}
	filas = append(filas, container.New(ui.Margen{Arr: tema.Espacio(1)}, repartirFila(botones, 0, ancho, tema.Espacio(2))))
	forma := container.New(ui.Columna{Hueco: tema.Espacio(2)}, filas...)
	// El `<form onSubmit>`: Intro en un campo guarda, como «Guardar».
	conIntro(forma, func() {
		if !l.trabajando() {
			c.guardar(campos)
		}
	})
	return forma
}

// campoEditable es `CampoEditable`: la etiqueta con su «Página N» a la derecha, y el
// control que pida el tipo.
func (c *panelCampos) campoEditable(x Campo, ancho float32) fyne.CanvasObject {
	nombre := x.Nombre
	poner := func(v string) { c.valores[nombre] = v }
	valor := c.valores[nombre]
	var control fyne.CanvasObject
	switch {
	case x.Tipo == "casilla" || x.Tipo == "opcion":
		casilla := nuevaCasillaNativa(c.t("pdf.fields.checked"), valor != "" && valor != "Off", func(si bool) {
			if si {
				poner("si")
			} else {
				poner("")
			}
		})
		casilla.esc, casilla.hueco, casilla.tinta = tema.TextoXS, tema.Espacio(2), "text"
		casilla.Inerte = x.SoloLectura
		control = container.New(ui.Margen{Arr: tema.Espacio(0.5)}, container.New(alPrincipio{}, casilla))
	case x.Tipo == "lista" || x.Tipo == "desplegable":
		opciones := append([]string{c.t("pdf.fields.choose")}, x.Opciones...)
		s := ui.NuevoSelector(opciones, func(k int) {
			if k == 0 {
				poner("")
			} else {
				poner(x.Opciones[k-1])
			}
		})
		for k, o := range x.Opciones {
			if o == valor {
				s.Elegida = k + 1
			}
		}
		s.Propio = &ui.SelectorPropio{Campo: campoPanel(tema.Espacio(1)), Alto: ui.AltoSelectNativoXS}
		s.Ancho = ancho
		s.Inerte(x.SoloLectura)
		control = s
	case x.VariasLineas:
		a := ui.NuevaAreaTexto("")
		a.Filas = 3
		a.ConPropia(campoPanel(tema.Espacio(1)))
		a.Radio, a.Ancho = tema.RadioMD, ancho
		a.PonerTexto(valor)
		a.AlCambiar(poner)
		a.Inerte(x.SoloLectura)
		// Un `<textarea>` es `inline-block` y va en una línea: el navegador le deja
		// debajo el trozo de caja de línea que queda bajo la base. Medido a rem16: 6.
		control = container.New(ui.Margen{Abj: 6}, a)
	default:
		e := nuevoCampoPanel("", tema.Espacio(1), tema.RadioMD, ancho)
		e.Contrasena(x.Contrasena)
		e.PonerTexto(valor)
		e.AlCambiar(poner)
		e.Inerte(x.SoloLectura || x.Tipo == "firma")
		control = e
	}
	return container.New(ui.Columna{}, c.etiqueta(x, ancho), control)
}

// etiqueta es el `<label class="text-text-muted text-2xs flex items-baseline
// justify-between gap-2">`: el nombre recortado con su asterisco si es obligatorio, y el
// enlace a la página pegado a la derecha.
func (c *panelCampos) etiqueta(x Campo, ancho float32) fyne.CanvasObject {
	n := x.Pagina + 1
	ir := enlaceBoton(c.l.t("pdf.page.n", i18n.Vars{"n": n}), "primary-text", func() {
		if c.l.AlIrA != nil {
			c.l.AlIrA(n)
		}
	})
	texto := x.Etiqueta
	if texto == "" {
		texto = x.Nombre
	}
	cabe := ancho - MedidaAccion(ir).Width - tema.Espacio(2)
	asterisco := ui.Medir("*", tema.Texto2XS, ui.Normal).Width + tema.Espacio(0.5)
	var izq fyne.CanvasObject
	if x.Obligatorio && ui.Medir(texto, tema.Texto2XS, ui.Normal).Width+asterisco <= cabe {
		izq = container.New(ui.Fila{Hueco: tema.Espacio(0.5)},
			ui.NuevoRotulo(texto, tema.Texto2XS, ui.Normal, "text-muted"),
			ui.NuevoRotulo("*", tema.Texto2XS, ui.Normal, "danger-text"))
	} else {
		// Recortado, el asterisco va detrás de los puntos suspensivos: no se ve, como
		// en la web, donde es parte del mismo `truncate`.
		izq = rotuloRecortado(texto, tema.Texto2XS, ui.Normal, "text-muted", cabe)
	}
	return container.New(aLosLados{}, izq, ir)
}

// guardar manda SOLO lo que cambió (`op: rellenar`): nada, y no se manda nada.
func (c *panelCampos) guardar(campos []Campo) {
	cambiados := cambiosDe(campos, c.valores)
	if len(cambiados) == 0 || c.l.pant == nil {
		return
	}
	c.l.pant.Aplicar(api.PDFCambio{Op: "rellenar", Datos: cambiados}, c.t("pdf.saved.filled"), nil)
}

// ficheroXFDF es el nombre con el que se descargan las respuestas, el de la web
// (`descargarBlob(b, 'datos.xfdf')`), y el de la parte que se le manda al motor al
// importar. Va en dos trozos porque es un nombre de fichero y no un texto: escrito
// entero tiene forma de clave del catálogo, y el guardián de `catalogos` lo buscaría
// en los mensajes.
const ficheroXFDF = "datos" + ".xfdf"

// cambiosDe son los valores que difieren de los del documento.
func cambiosDe(campos []Campo, valores map[string]string) map[string]string {
	r := map[string]string{}
	for _, x := range campos {
		if valores[x.Nombre] != x.Valor {
			r[x.Nombre] = valores[x.Nombre]
		}
	}
	return r
}

// exportar saca las respuestas en XFDF y las deja en descargas como `datos.xfdf`, que es
// lo que hace `descargarBlob`. Un fallo no dice nada, como en la web.
func (c *panelCampos) exportar() {
	p := c.l.pant
	if p == nil || p.doc == nil {
		return
	}
	doc := p.doc
	go func() {
		ctx, parar := context.WithTimeout(p.ctx, plazo)
		defer parar()
		datos, err := doc.Binario(ctx, "/pdf/form", map[string]string{"op": "exportar-datos"})
		if err != nil {
			return
		}
		if carpeta, err := ficheros.Carpeta(); err == nil {
			_, _ = ficheros.Guardar(carpeta, ficheroXFDF, bytes.NewReader(datos))
		}
	}()
}

// importar pide un XFDF con el selector del sistema —filtrado como el `accept=".xfdf,
// application/vnd.adobe.xfdf,text/xml"` de la web— y lo vuelca en una copia.
func (c *panelCampos) importar() {
	p := c.l.pant
	if p == nil || p.doc == nil {
		return
	}
	titulo := c.t("pdf.fields.import")
	go func() {
		rutas, err := ficheros.ElegirConTipos(titulo, "xfdf", "xml")
		if err != nil || len(rutas) == 0 {
			return
		}
		xfdf, err := os.ReadFile(rutas[0])
		if err != nil {
			return
		}
		fyne.Do(func() {
			p.Aplicar(api.PDFCambio{
				Op:     "importar-datos",
				Partes: []api.PDFParte{{Campo: "datos", Nombre: ficheroXFDF, Datos: xfdf}},
				Sufijo: c.t("pdf.suffix.form"),
			}, c.t("pdf.fields.imported"), nil)
		})
	}()
}

// aplanar destruye los campos editables: se pregunta antes, con el diálogo de la casa.
func (c *panelCampos) aplanar() {
	p := c.l.pant
	if p == nil {
		return
	}
	p.confirmarCon(c.t("pdf.fields.flatten.title"), c.t("pdf.fields.flatten.confirm"), c.t("pdf.fields.flatten"), func() {
		p.Aplicar(api.PDFCambio{Op: "aplanar-formulario", Sufijo: c.t("pdf.suffix.flattened")}, "", nil)
	})
}

// ---- DetectarCampos ----

// detectarCampos es `<div class="flex flex-col gap-2 p-1">`: qué pasa, qué se puede hacer
// y, tras buscar, lo encontrado con «Crear estos campos».
func (c *panelCampos) detectarCampos() fyne.CanvasObject {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	m := tema.Espacio(1)
	ancho := anchoUtil() - 2*m
	partes := []fyne.CanvasObject{
		ui.NuevoParrafo(c.t("pdf.fields.empty"), tema.TextoXS, ui.Normal, "text-subtle", v).A(ancho),
		ui.NuevoParrafo(c.t("pdf.detect.help"), tema.Texto2XS, ui.Normal, "text-subtle", v).A(ancho),
	}
	if c.errorDet != "" {
		partes = append(partes, ui.NuevoParrafo(c.errorDet, tema.Texto2XS, ui.Normal, "danger-text", v).A(ancho))
	}
	c.detectar = nil
	switch {
	case c.propuestas == nil:
		c.detectar = botonPanel(c.rotuloDetectar(), tema.TextoXS, tema.Espacio(1.5), c.buscar)
		c.detectar.Inerte = c.buscando
		partes = append(partes, container.New(alPrincipio{}, c.detectar))
	case len(c.propuestas) == 0:
		partes = append(partes, ui.NuevoParrafo(c.t("pdf.detect.none"), tema.Texto2XS, ui.Normal, "text-subtle", v).A(ancho))
	default:
		filas := make([]fyne.CanvasObject, 0, len(c.propuestas))
		for _, pr := range c.propuestas {
			filas = append(filas, c.filaPropuesta(pr, ancho))
		}
		crear := botonPrimario(c.t("pdf.detect.create"), tema.TextoXS, tema.Espacio(3), tema.Espacio(1.5), c.crear)
		cancelar := botonPanel(c.l.t("action.cancel", nil), tema.TextoXS, tema.Espacio(1.5), func() {
			c.propuestas = nil
			c.rehacer()
		})
		partes = append(partes,
			ui.NuevoParrafo(c.l.t("pdf.detect.found", i18n.Vars{"count": len(c.propuestas)}), tema.TextoXS, ui.Normal, "text", v).A(ancho),
			// `max-h-56 overflow-auto space-y-1`.
			container.New(topeAlto{alto: tema.Espacio(56)}, container.NewVScroll(container.New(ui.Columna{Hueco: tema.Espacio(1)}, filas...))),
			repartirFila([]*Accion{crear, cancelar}, -1, ancho, tema.Espacio(2)),
		)
	}
	return container.New(ui.Margen{Izq: m, Arr: m, Der: m, Abj: m}, container.New(ui.Columna{Hueco: tema.Espacio(2)}, partes...))
}

func (c *panelCampos) rotuloDetectar() string {
	if c.buscando {
		return c.t("pdf.detect.searching")
	}
	return c.l.t("pdf.detect.run", i18n.Vars{"n": strconv.Itoa(c.l.PaginaActual)})
}

// alCambiarPagina rehace el rótulo del botón, que dice en qué página va a buscar.
func (c *panelCampos) alCambiarPagina() {
	if c.detectar != nil {
		c.detectar.Texto = c.rotuloDetectar()
		c.detectar.Refresh()
	}
}

// filaPropuesta es el `<li class="bg-bg-subtle flex justify-between gap-2 rounded-sm
// px-2 py-1">`: la etiqueta recortada y el tipo.
func (c *panelCampos) filaPropuesta(pr Propuesta, ancho float32) fyne.CanvasObject {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	px, py := tema.Espacio(2), tema.Espacio(1)
	tipo := c.t(claveDeTipoCampo(pr.Tipo))
	nombre := pr.Etiqueta
	if nombre == "" {
		nombre = c.t("pdf.detect.unnamed")
	}
	cabe := ancho - 2*px - tema.Espacio(2) - ui.Medir(tipo, tema.Texto2XS, ui.Normal).Width
	fondo := canvas.NewRectangle(tema.Color("bg-subtle", v))
	fondo.CornerRadius = tema.RadioSM
	fila := container.New(aLosLados{},
		rotuloRecortado(nombre, tema.Texto2XS, ui.Normal, "text", cabe),
		ui.NuevoRotulo(tipo, tema.Texto2XS, ui.Normal, "text-subtle"))
	return container.NewStack(fondo, container.New(ui.Margen{Izq: px, Arr: py, Der: px, Abj: py}, fila))
}

// claveDeTipoCampo es el nombre del tipo propuesto, con los rótulos del resto.
func claveDeTipoCampo(tipo string) string {
	switch tipo {
	case "casilla":
		return "pdf.field.kind.check"
	case "desplegable":
		return "pdf.field.kind.select"
	case "firma":
		return "pdf.field.kind.signature"
	}
	return "pdf.field.kind.text"
}

// buscar es `op: detectar-campos` sobre la página que se está mirando.
func (c *panelCampos) buscar() {
	p := c.l.pant
	if p == nil || p.doc == nil {
		return
	}
	c.buscando, c.errorDet = true, ""
	c.rehacer()
	doc, pagina := p.doc, c.l.PaginaActual
	go func() {
		ctx, parar := context.WithTimeout(p.ctx, plazo)
		defer parar()
		var r struct {
			Propuestas []Propuesta `json:"propuestas"`
		}
		err := doc.Leer(ctx, "/pdf/form", map[string]string{"op": "detectar", "pagina": strconv.Itoa(pagina)}, &r)
		fyne.Do(func() {
			defer p.avisarRed()
			c.buscando = false
			if err != nil {
				c.errorDet = c.t("pdf.detect.failed")
			} else {
				c.propuestas = append([]Propuesta{}, r.Propuestas...)
			}
			c.rehacer()
		})
	}()
}

// crear escribe los propuestos (`op: crear-campos`, siempre en copia). La etiqueta de al
// lado es el mejor nombre que hay: «DNI» dice algo y «texto3» obliga a adivinar.
func (c *panelCampos) crear() {
	p := c.l.pant
	if p == nil || len(c.propuestas) == 0 {
		return
	}
	datos := make([]map[string]any, len(c.propuestas))
	for i, pr := range c.propuestas {
		nombre := pr.Etiqueta
		if nombre == "" {
			nombre = pr.Tipo + strconv.Itoa(i+1)
		}
		datos[i] = map[string]any{"tipo": pr.Tipo, "nombre": nombre, "pagina": pr.Pagina, "rect": pr.Rect}
	}
	p.Aplicar(api.PDFCambio{Op: "crear-campos", Datos: datos, Sufijo: c.t("pdf.suffix.form")}, c.t("pdf.saved.fieldsCreated"), nil)
}

// ---- piezas ----

// aLosLados es `flex items-center justify-between` con dos hijos: el primero a la
// izquierda y el segundo pegado a la derecha, cada uno a su medida y centrados en alto.
type aLosLados struct{}

func (aLosLados) MinSize(objs []fyne.CanvasObject) fyne.Size {
	a, b := objs[0].MinSize(), objs[1].MinSize()
	return fyne.NewSize(a.Width+b.Width, max(a.Height, b.Height))
}

func (aLosLados) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	a, b := objs[0].MinSize(), objs[1].MinSize()
	objs[0].Move(fyne.NewPos(0, (t.Height-a.Height)/2))
	objs[0].Resize(a)
	objs[1].Move(fyne.NewPos(t.Width-b.Width, (t.Height-b.Height)/2))
	objs[1].Resize(b)
}

// repartirFila es una fila `flex gap-*` de botones en un ancho conocido, con el
// algoritmo de CSS: cada uno a su texto y el que crece (`flex-1`; -1 si ninguno) se
// queda el sobrante. Si no caben, encogen todos a la vez en proporción a su ancho y
// nunca por debajo de su palabra más larga, y su rótulo se parte en renglones. Si ni así
// caben, se salen por la derecha, como en la web (el panel los corta). Todos al alto
// del más alto: el `align-items: stretch`.
func repartirFila(bs []*Accion, crece int, ancho, hueco float32) fyne.CanvasObject {
	n := len(bs)
	maxc, minc, base := make([]float32, n), make([]float32, n), make([]float32, n)
	for i, a := range bs {
		e := 2*a.PX + 2*bordeAccion(a)
		base[i] = ui.Medir(a.Texto, a.Escalon, a.Peso).Width
		maxc[i] = base[i] + e
		minc[i] = palabraMasLarga(a.Texto, a.Escalon, a.Peso) + e
	}
	anchos := anchosFlex(maxc, minc, base, crece, ancho-hueco*float32(n-1))
	var alto float32
	objs := make([]fyne.CanvasObject, n)
	for i, a := range bs {
		a.Ancho, a.Envuelto = anchos[i], true
		alto = max(alto, MedidaAccion(a).Height)
		objs[i] = a
	}
	for _, a := range bs {
		a.Alto = alto
	}
	return container.New(desborda{ancho: ancho}, container.New(ui.Fila{Hueco: hueco}, objs...))
}

// palabraMasLarga es el `min-content` de un texto que se puede partir por los espacios.
func palabraMasLarga(s string, e tema.Escalon, p ui.Peso) float32 {
	var m float32
	for _, w := range strings.Fields(s) {
		m = max(m, ui.Medir(w, e, p).Width)
	}
	return m
}

// anchosFlex es el reparto de una fila `flex` de CSS entre elementos con su ancho de
// contenido (`maxc`) y su mínimo (`minc`, el `min-content`) en `libre` (el ancho menos
// los huecos): si sobra, se lo queda el que crece (`flex-1`; -1 si ninguno); si falta,
// encogen todos y ninguno por debajo de su mínimo. Encogen en proporción a su base
// INTERIOR (`base`, sin relleno ni borde): es el «scaled flex shrink factor» de la norma,
// y con el ancho exterior las pestañas de Firmar salían 1,5 px desplazadas.
func anchosFlex(maxc, minc, base []float32, crece int, libre float32) []float32 {
	n := len(maxc)
	var suma float32
	for _, m := range maxc {
		suma += m
	}
	anchos := append([]float32{}, maxc...)
	if libre >= suma {
		if crece >= 0 {
			anchos[crece] += libre - suma
		}
		return anchos
	}
	congelado := make([]bool, n)
	for {
		resto, total := libre, float32(0)
		for i := range n {
			if congelado[i] {
				resto -= anchos[i]
			} else {
				resto -= maxc[i]
				total += base[i]
			}
		}
		viola := false
		for i := range n {
			if congelado[i] {
				continue
			}
			anchos[i] = maxc[i] + resto*base[i]/total
			if anchos[i] < minc[i] {
				anchos[i], congelado[i], viola = minc[i], true, true
			}
		}
		if !viola {
			return anchos
		}
	}
}

// desborda declara el ancho de su caja y deja al hijo con el suyo, aunque se salga: es
// un `overflow: visible`, sin empujar a la columna que lo lleva.
type desborda struct{ ancho float32 }

func (d desborda) MinSize(objs []fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(d.ancho, objs[0].MinSize().Height)
}

func (d desborda) Layout(objs []fyne.CanvasObject, _ fyne.Size) {
	objs[0].Move(fyne.NewPos(0, 0))
	objs[0].Resize(objs[0].MinSize())
}
