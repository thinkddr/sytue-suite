// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package certipdf

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"math"
	"strconv"
	"sync"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
	xdraw "golang.org/x/image/draw"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

/*
La comparación VISUAL de dos PDF, en el diálogo «Comparar».

Es `CompararVisual.tsx` y `comparacionVisual.ts` de la web, pantalla por pantalla: el
motor (`/pdf/compare` con `modo=visual`) compara los píxeles de cada página y da el
porcentaje y las cajas de lo que cambió, en PUNTOS de página; la imagen de UNA página
—lo quitado en rojo, lo añadido en verde— se pide cuando se mira, no toda de golpe.

Lo que decide esta pantalla por su cuenta es poco: ordenar las regiones en «paradas»
para ir de una a otra, y situar las cajas sobre la imagen. Todo el juicio es del motor.
*/

// RegionVisual es una caja de diferencia, en puntos de página (origen arriba-izquierda).
type RegionVisual struct {
	X     float64 `json:"x"`
	Y     float64 `json:"y"`
	Ancho float64 `json:"ancho"`
	Alto  float64 `json:"alto"`
}

// DiferenciaVisual es una página en la comparación visual (`domain.DiferenciaVisual`).
type DiferenciaVisual struct {
	Pagina int `json:"pagina"`
	// SoloEn es «a» o «b»: la página solo existe en uno de los dos documentos.
	SoloEn           string         `json:"solo_en"`
	Omitida          bool           `json:"omitida"`
	Motivo           string         `json:"motivo"`
	TamanosDistintos bool           `json:"tamanos_distintos"`
	Ancho            float64        `json:"ancho"`
	Alto             float64        `json:"alto"`
	Escala           float64        `json:"escala"`
	Porcentaje       float64        `json:"porcentaje"`
	Regiones         []RegionVisual `json:"regiones"`
	// Imagen es el PNG del resaltado; el JSON lo lleva en base64 y Go lo decodifica solo.
	Imagen []byte `json:"imagen"`
}

// ComparacionVisual es la respuesta de `/pdf/compare?modo=visual` sin `pagina`.
type ComparacionVisual struct {
	PaginasA   int                `json:"paginas_a"`
	PaginasB   int                `json:"paginas_b"`
	Comparadas int                `json:"comparadas"`
	Truncado   bool               `json:"truncado"`
	Paginas    []DiferenciaVisual `json:"paginas"`
	Iguales    bool               `json:"iguales"`
}

// sensibilidades son las del selector, con la tolerancia por canal que manda cada una.
// «Normal» es el defecto del motor (48): absorbe el ruido de antialiasing de un texto
// corrido una fracción de píxel sin perder una coma cambiada.
var sensibilidades = []struct {
	clave      string
	tolerancia int
}{
	{"pdf.dialog.comparar.visual.sens.alta", 24},
	{"pdf.dialog.comparar.visual.sens.normal", 48},
	{"pdf.dialog.comparar.visual.sens.baja", 96},
}

const sensibilidadPorDefecto = 1

// minCampoVisual es el `min-w-48` del campo de la ruta.
const minCampoVisual float32 = 192

// CompararVisual compara los píxeles: el resumen de todo el documento, sin imágenes.
func (d *Documento) CompararVisual(ctx context.Context, otro api.PDFFichero, tolerancia int) (ComparacionVisual, error) {
	var c ComparacionVisual
	doc, _ := d.paraElMotor()
	r, err := d.pdf.Motor(ctx, "/pdf/compare", map[string]string{
		"modo": "visual", "tolerancia": strconv.Itoa(tolerancia),
	}, nil, doc, otro)
	if err != nil {
		return c, err
	}
	err = json.Unmarshal(r.Datos, &c)
	return c, err
}

// CompararVisualPagina es UNA página (1-based) de la comparación, con su resaltado.
func (d *Documento) CompararVisualPagina(ctx context.Context, otro api.PDFFichero, pagina, tolerancia int) (DiferenciaVisual, error) {
	var c struct {
		Pagina DiferenciaVisual `json:"pagina"`
	}
	doc, _ := d.paraElMotor()
	r, err := d.pdf.Motor(ctx, "/pdf/compare", map[string]string{
		"modo": "visual", "tolerancia": strconv.Itoa(tolerancia), "pagina": strconv.Itoa(pagina),
	}, nil, doc, otro)
	if err != nil {
		return c.Pagina, err
	}
	err = json.Unmarshal(r.Datos, &c)
	return c.Pagina, err
}

// parada es un sitio al que ir: una región de una página, o la página entera (región -1)
// si no tiene cajas.
type parada struct{ pagina, region int }

// paradasDe son las paradas del documento en orden de lectura. Una página sin cajas pero
// con algo que decir (solo existe en uno, o no se pudo comparar) cuenta como UNA parada:
// saltarla sería esconder que falta una página.
func paradasDe(paginas []DiferenciaVisual) []parada {
	out := []parada{}
	for _, p := range paginas {
		if len(p.Regiones) == 0 {
			out = append(out, parada{p.Pagina, -1})
		}
		for i := range p.Regiones {
			out = append(out, parada{p.Pagina, i})
		}
	}
	return out
}

// moverParada es la parada siguiente (+1) o anterior (-1), DANDO LA VUELTA en los extremos.
func moverParada(actual, total, paso int) int {
	if total <= 0 {
		return 0
	}
	return ((actual+paso)%total + total) % total
}

// claveAvisoVisual es la frase de una página que no se pudo comparar o que solo está en un
// documento; vacía si no hay nada que avisar.
func claveAvisoVisual(p DiferenciaVisual) string {
	switch {
	case p.SoloEn == "a":
		return "pdf.dialog.comparar.visual.soloA"
	case p.SoloEn == "b":
		return "pdf.dialog.comparar.visual.soloB"
	case p.Omitida && p.Motivo == "demasiado_grande":
		return "pdf.dialog.comparar.visual.omitida.grande"
	case p.Omitida:
		return "pdf.dialog.comparar.visual.omitida.ilegible"
	}
	return ""
}

// porcentajeRedondeado deja los decimales que enseña la web (`porcentajeLegible`): tres por
// debajo de 1, dos por debajo de 10 y uno por encima, y nunca «0» si algo difiere.
func porcentajeRedondeado(v float64) float64 {
	if v <= 0 {
		return 0
	}
	dec := 1.0
	switch {
	case v < 1:
		dec = 3
	case v < 10:
		dec = 2
	}
	k := math.Pow(10, dec)
	return math.Max(math.Round(v*k)/k, 0.001)
}

// paginaVisual es la imagen de una página que se ha mirado, o por qué no la hay.
type paginaVisual struct {
	cargando, fallo bool
	dif             DiferenciaVisual
	img             image.Image
}

// estadoVisual es lo que el diálogo recuerda del modo visual.
type estadoVisual struct {
	sens       int
	trabajando bool
	fallo      bool
	res        *ComparacionVisual
	parada     int
	paginas    map[int]*paginaVisual
	// otro es el segundo documento, bajado una vez para las peticiones de cada página;
	// de qué ruta lo es, para volver a bajarlo si cambia.
	otro     *api.PDFFichero
	rutaOtro string
	// vuelta cuenta las comparaciones: una respuesta que llega tarde no pisa a la nueva.
	vuelta int
}

func (e *estadoVisual) tolerancia() int { return sensibilidades[e.sens].tolerancia }

// vistaVisual es la pestaña «Visual».
func (d *dialogoComparar) vistaVisual(ancho float32) fyne.CanvasObject {
	a := d.a
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	e := d.visual()
	hueco := tema.Espacio(2)

	opciones := make([]string, len(sensibilidades))
	for i, s := range sensibilidades {
		opciones[i] = a.t(s.clave)
	}
	sel := selectorDialogo(opciones, e.sens, 0, func(i int) { e.sens = i; a.rehacer() })
	conSens := container.New(filaCentrada{tema.Espacio(1.5)},
		ui.NuevoRotulo(a.t("pdf.dialog.comparar.visual.sens"), tema.TextoXS, ui.Normal, "text-muted"), sel)
	comparar := botonPrimario(a.t("pdf.dialog.comparar.run"), tema.TextoXS, tema.Espacio(3), tema.Espacio(1.5), d.compararVisual)
	comparar.Inerte = e.trabajando
	d.entrada(d.compararVisual)
	// `flex flex-wrap items-center gap-2` con el campo `min-w-48 flex-1`: si el campo no
	// llega a 192 px, pasa solo a la primera línea y el selector y el botón a la segunda.
	var formulario fyne.CanvasObject
	if resto := ancho - 2*hueco - conSens.MinSize().Width - comparar.MinSize().Width; resto >= minCampoVisual {
		d.otro.Ancho = resto
		formulario = container.New(filaCentrada{hueco}, d.otro, conSens, comparar)
	} else {
		d.otro.Ancho = ancho
		formulario = container.New(ui.Columna{Hueco: hueco}, d.otro, container.New(filaCentrada{hueco}, conSens, comparar))
	}

	partes := []fyne.CanvasObject{
		ui.NuevoParrafo(a.t("pdf.dialog.comparar.visual.help"), tema.TextoXS, ui.Normal, "text-muted", v).A(ancho),
		formulario,
	}
	if e.fallo {
		partes = append(partes, ui.NuevoParrafo(a.t("pdf.dialog.comparar.error"), tema.TextoXS, ui.Normal, "danger-text", v).A(ancho))
	}
	if r := e.res; r != nil {
		if r.Iguales {
			partes = append(partes, ui.NuevoParrafo(a.t("pdf.dialog.comparar.visual.same"), tema.TextoXS, ui.Normal, "success-text", v).A(ancho))
		} else {
			partes = append(partes, ui.NuevoParrafo(a.p.tv("pdf.dialog.comparar.visual.summary",
				i18n.Vars{"n": len(r.Paginas), "total": max(r.PaginasA, r.PaginasB)}), tema.TextoXS, ui.Medio, "text", v).A(ancho))
		}
		if r.Truncado {
			partes = append(partes, avisoAmbar(a.p.tv("pdf.dialog.comparar.visual.truncado", i18n.Vars{"n": r.Comparadas}), ancho))
		}
		for _, p := range r.Paginas {
			if k := claveAvisoVisual(p); k != "" {
				partes = append(partes, avisoAmbar(a.p.tv(k, i18n.Vars{"n": p.Pagina}), ancho))
			}
		}
		if paradas := paradasDe(r.Paginas); len(paradas) > 0 {
			partes = append(partes, d.navegacionVisual(ancho, paradas))
		}
	}
	return container.New(ui.Columna{Hueco: tema.Espacio(3)}, partes...)
}

// navegacionVisual es la cabecera de la página que se mira, con Anterior y Siguiente, y
// debajo la imagen con sus cajas.
func (d *dialogoComparar) navegacionVisual(ancho float32, paradas []parada) fyne.CanvasObject {
	a := d.a
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	e := d.visual()
	e.parada = min(e.parada, len(paradas)-1)
	actual := paradas[e.parada]
	pag := e.paginas[actual.pagina]

	titulo := a.p.tv("pdf.page.n", i18n.Vars{"n": actual.pagina})
	if pag != nil && !pag.cargando && !pag.fallo && pag.dif.SoloEn == "" && !pag.dif.Omitida {
		titulo += " · " + a.p.tv("pdf.dialog.comparar.visual.pct", i18n.Vars{"pct": porcentajeRedondeado(pag.dif.Porcentaje)})
	}
	if pag != nil && actual.region >= 0 && len(pag.dif.Regiones) > 0 {
		titulo += " · " + a.p.tv("pdf.dialog.comparar.visual.stop", i18n.Vars{"i": actual.region + 1, "n": len(pag.dif.Regiones)})
	}
	ir := func(paso int) func() {
		return func() {
			e.parada = moverParada(e.parada, len(paradas), paso)
			d.cargarPagina(paradas[e.parada].pagina)
			a.rehacer()
		}
	}
	anterior := botonPanel(a.t("nav.previous"), tema.TextoXS, tema.Espacio(1), ir(-1))
	siguiente := botonPanel(a.t("nav.nextItem"), tema.TextoXS, tema.Espacio(1), ir(1))
	anterior.PX, siguiente.PX = tema.Espacio(2.5), tema.Espacio(2.5)
	anterior.Inerte, siguiente.Inerte = len(paradas) < 2, len(paradas) < 2
	botones := container.New(ui.Fila{Hueco: tema.Espacio(1.5)}, anterior, siguiente)
	// `flex-wrap justify-between gap-2`: juntos se separan hasta los extremos; si
	// los botones bajan, su fila empieza a la izquierda. El título también envuelve.
	rotulo := ui.NuevoParrafo(titulo, tema.TextoXS, ui.Medio, "text", v).
		A(min(ui.Medir(titulo, tema.TextoXS, ui.Medio).Width, ancho))
	derecha := 1
	if rotulo.MinSize().Width+tema.Espacio(2)+botones.MinSize().Width > ancho {
		derecha = 2
	}
	cab := ui.AlAnchoFluido(ancho, container.New(ui.FilaAuto{
		Hueco: tema.Espacio(2), Derecha: derecha, Partir: true, SaltoFila: tema.Espacio(2),
	}, rotulo, botones))

	partes := []fyne.CanvasObject{cab}
	switch {
	case pag == nil || pag.cargando:
		partes = append(partes, ui.NuevoParrafo(a.t("pdf.page.loading"), tema.TextoXS, ui.Normal, "text-muted", v).A(ancho))
	case pag.fallo:
		partes = append(partes, ui.NuevoParrafo(a.t("pdf.dialog.comparar.error"), tema.TextoXS, ui.Normal, "danger-text", v).A(ancho))
	case pag.img != nil:
		partes = append(partes, d.vistaDePagina(ancho, pag, actual))
	}
	return container.New(ui.Columna{Hueco: tema.Espacio(2)}, partes...)
}

// vistaDePagina es la imagen con sus cajas, los avisos de tamaño y la leyenda.
func (d *dialogoComparar) vistaDePagina(ancho float32, pag *paginaVisual, actual parada) fyne.CanvasObject {
	a := d.a
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	var partes []fyne.CanvasObject
	if pag.dif.TamanosDistintos {
		partes = append(partes, avisoAmbar(a.t("pdf.dialog.comparar.visual.sizes"), ancho))
	}
	if pag.dif.Escala < 0.5 {
		partes = append(partes, avisoAmbar(a.t("pdf.dialog.comparar.visual.baja"), ancho))
	}
	vista := nuevaVistaDiferencias(pag.img, pag.dif, actual.region, ancho, func(i int) {
		e := d.visual()
		for j, q := range paradasDe(e.res.Paginas) {
			if q.pagina == pag.dif.Pagina && q.region == i {
				e.parada = j
			}
		}
		a.rehacer()
	})
	partes = append(partes, vista)
	cuadro := func(tinta string) fyne.CanvasObject {
		r := canvas.NewRectangle(tema.Color(tinta, v))
		r.CornerRadius = tema.RadioSM
		r.SetMinSize(fyne.NewSize(10, 10))
		return r
	}
	leyenda := container.New(ui.Fila{Hueco: tema.Espacio(4)},
		container.New(ui.Fila{Hueco: tema.Espacio(1.5)}, cuadro("danger-bg"),
			ui.NuevoRotulo(a.t("pdf.dialog.comparar.visual.leyenda.a"), tema.Texto2XS, ui.Normal, "text-muted")),
		container.New(ui.Fila{Hueco: tema.Espacio(1.5)}, cuadro("success-bg"),
			ui.NuevoRotulo(a.t("pdf.dialog.comparar.visual.leyenda.b"), tema.Texto2XS, ui.Normal, "text-muted")))
	partes = append(partes, leyenda)
	return container.New(ui.Columna{Hueco: tema.Espacio(2)}, partes...)
}

// visual da el estado del modo visual, creándolo la primera vez.
func (d *dialogoComparar) visual() *estadoVisual {
	if d.v == nil {
		d.v = &estadoVisual{sens: sensibilidadPorDefecto, paginas: map[int]*paginaVisual{}}
	}
	return d.v
}

// compararVisual pide la comparación; lo que falle se dice con una sola frase.
func (d *dialogoComparar) compararVisual() {
	p, e := d.a.p, d.visual()
	otra := d.otro.Texto()
	if e.trabajando || p.doc == nil || (d.otro != nil && !d.otro.Requerida()) || otra == "" {
		return
	}
	e.vuelta++
	mia := e.vuelta
	e.trabajando, e.fallo, e.res, e.parada = true, false, nil, 0
	e.paginas = map[int]*paginaVisual{}
	d.a.rehacer()
	doc, tol := p.doc, e.tolerancia()
	go func() {
		ctx, parar := context.WithTimeout(p.ctx, plazo)
		defer parar()
		otro, err := doc.TraerOtro(ctx, otra)
		var r ComparacionVisual
		if err == nil {
			r, err = doc.CompararVisual(ctx, otro, tol)
		}
		fyne.Do(func() {
			defer p.avisarRed()
			if mia != e.vuelta {
				return
			}
			e.trabajando = false
			if err != nil {
				e.fallo = true
			} else {
				e.res, e.otro, e.rutaOtro = &r, &otro, otra
				if ps := paradasDe(r.Paginas); len(ps) > 0 {
					d.cargarPagina(ps[0].pagina)
				}
			}
			d.a.rehacer()
		})
	}()
}

// cargarPagina pide la imagen de una página si no se tiene ya.
func (d *dialogoComparar) cargarPagina(n int) {
	p, e := d.a.p, d.visual()
	if e.paginas[n] != nil || e.otro == nil || p.doc == nil || e.res == nil {
		return
	}
	// Una página que solo está en un documento, o que no se pudo comparar, no tiene imagen
	// que pedir: el aviso de arriba ya lo dice todo.
	for _, r := range e.res.Paginas {
		if r.Pagina == n && claveAvisoVisual(r) != "" {
			return
		}
	}
	e.paginas[n] = &paginaVisual{cargando: true}
	mia, doc, otro, tol := e.vuelta, p.doc, *e.otro, e.tolerancia()
	go func() {
		ctx, parar := context.WithTimeout(p.ctx, plazo)
		defer parar()
		dif, err := doc.CompararVisualPagina(ctx, otro, n, tol)
		var img image.Image
		if err == nil && len(dif.Imagen) > 0 {
			img, _ = png.Decode(bytes.NewReader(dif.Imagen))
		}
		fyne.Do(func() {
			defer p.avisarRed()
			if mia != e.vuelta {
				return
			}
			pg := &paginaVisual{dif: dif, img: img, fallo: err != nil}
			dif.Imagen = nil // ya está decodificada
			pg.dif = dif
			e.paginas[n] = pg
			d.a.rehacer()
		})
	}()
}

// entrada crea, si hace falta, el campo de la ruta del otro documento (compartido por los
// dos modos: cambiar de pestaña no lo borra) y fija su Intro.
func (d *dialogoComparar) entrada(alIntro func()) {
	if d.otro == nil {
		c := campoDialogo()
		d.otro = ui.NuevaEntrada("/Documentos/contrato-v2.pdf").ConPropia(c)
		d.otro.Radio = tema.RadioMD
	}
	d.otro.AlTeclear = func(k *fyne.KeyEvent) bool {
		if k.Name == fyne.KeyReturn || k.Name == fyne.KeyEnter {
			alIntro()
			return true
		}
		return false
	}
}

// --- La imagen con sus cajas ---------------------------------------------------

// vistaDiferencias pinta el resaltado de una página a todo el ancho, con una caja por
// región (la actual, rellena) y la posibilidad de pulsar una para ir a ella.
type vistaDiferencias struct {
	widget.BaseWidget
	img      image.Image
	dif      DiferenciaVisual
	actual   int
	ancho    float32
	alElegir func(int)

	mu    sync.Mutex
	hecha image.Image
}

func nuevaVistaDiferencias(img image.Image, dif DiferenciaVisual, actual int, ancho float32, alElegir func(int)) *vistaDiferencias {
	w := &vistaDiferencias{img: img, dif: dif, actual: actual, ancho: ancho, alElegir: alElegir}
	w.ExtendBaseWidget(w)
	return w
}

var _ fyne.Tappable = (*vistaDiferencias)(nil)

// alto es lo que mide la imagen a este ancho: la proporción del lienzo comparado.
func (w *vistaDiferencias) alto() float32 {
	if w.dif.Ancho <= 0 {
		return 0
	}
	return w.ancho * float32(w.dif.Alto/w.dif.Ancho)
}

func (w *vistaDiferencias) MinSize() fyne.Size { return fyne.NewSize(w.ancho, w.alto()) }

// Tapped elige la región bajo el dedo, la más pequeña si se solapan.
func (w *vistaDiferencias) Tapped(e *fyne.PointEvent) {
	s := w.Size()
	interior := fyne.NewSize(s.Width-2*tema.Borde, s.Height-2*tema.Borde)
	if interior.Width <= 0 || interior.Height <= 0 || w.alElegir == nil {
		return
	}
	mejor, area := -1, math.MaxFloat64
	for i, r := range w.dif.Regiones {
		x := tema.Borde + fraccionVisual(r.X, w.dif.Ancho)*interior.Width
		y := tema.Borde + fraccionVisual(r.Y, w.dif.Alto)*interior.Height
		ancho := fraccionVisual(r.Ancho, w.dif.Ancho) * interior.Width
		alto := fraccionVisual(r.Alto, w.dif.Alto) * interior.Height
		if e.Position.X >= x && e.Position.X <= x+ancho && e.Position.Y >= y && e.Position.Y <= y+alto && r.Ancho*r.Alto < area {
			mejor, area = i, r.Ancho*r.Alto
		}
	}
	if mejor >= 0 {
		w.alElegir(mejor)
	}
}

// generar entrega la imagen a los píxeles de dispositivo que pide el lienzo, remuestreada
// con Catmull-Rom (el bilineal aproximado de Fyne emborrona el texto, ver `pagina.go`).
func (w *vistaDiferencias) generar(ancho, alto int) image.Image {
	w.mu.Lock()
	defer w.mu.Unlock()
	if ancho <= 0 || alto <= 0 {
		return image.NewNRGBA(image.Rect(0, 0, 1, 1))
	}
	if w.hecha != nil && w.hecha.Bounds().Dx() == ancho && w.hecha.Bounds().Dy() == alto {
		return w.hecha
	}
	dst := image.NewRGBA(image.Rect(0, 0, ancho, alto))
	xdraw.CatmullRom.Scale(dst, dst.Bounds(), w.img, w.img.Bounds(), xdraw.Src, nil)
	w.hecha = dst
	return dst
}

func (w *vistaDiferencias) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &vistaDiferenciasRenderer{w: w}
	r.papel = canvas.NewRaster(w.generar)
	r.papel.ScaleMode = canvas.ImageScalePixels
	r.borde = canvas.NewRectangle(color.Transparent)
	r.borde.StrokeColor, r.borde.StrokeWidth = tema.Color("border", v), tema.Borde
	for i := range w.dif.Regiones {
		c := canvas.NewRectangle(color.Transparent)
		primario := tema.Color("primary", v)
		c.CornerRadius = tema.RadioSM
		c.StrokeWidth = 2
		if i == w.actual {
			c.StrokeColor = primario
			relleno := primario
			relleno.A = 38 // `bg-primary/15`
			c.FillColor = relleno
		} else {
			trazo := primario
			trazo.A = 128 // `border-primary/50`
			c.StrokeColor = trazo
		}
		r.cajas = append(r.cajas, c)
	}
	return r
}

type vistaDiferenciasRenderer struct {
	w     *vistaDiferencias
	papel *canvas.Raster
	borde *canvas.Rectangle
	cajas []*canvas.Rectangle
}

// fraccionVisual conserva los tres decimales del porcentaje de `cajaEnPorcentaje`.
func fraccionVisual(valor, total float64) float32 {
	if total <= 0 {
		return 0
	}
	return float32(math.Round(min(1, max(0, valor/total))*100000) / 100000)
}

func (r *vistaDiferenciasRenderer) Layout(s fyne.Size) {
	// El `border` ocupa un píxel: tanto el <img> como las cajas absolutas usan
	// el interior del contenedor, cuya proporción incluye ese borde.
	interior := fyne.NewSize(max(0, s.Width-2*tema.Borde), max(0, s.Height-2*tema.Borde))
	r.papel.Move(fyne.NewPos(tema.Borde, tema.Borde))
	r.papel.Resize(interior)
	r.borde.Move(fyne.NewPos(0, 0))
	r.borde.Resize(s)
	d := r.w.dif
	if d.Ancho <= 0 || d.Alto <= 0 {
		return
	}
	for i, c := range r.cajas {
		g := d.Regiones[i]
		c.Move(fyne.NewPos(tema.Borde+fraccionVisual(g.X, d.Ancho)*interior.Width, tema.Borde+fraccionVisual(g.Y, d.Alto)*interior.Height))
		c.Resize(fyne.NewSize(fraccionVisual(g.Ancho, d.Ancho)*interior.Width, fraccionVisual(g.Alto, d.Alto)*interior.Height))
	}
}

func (r *vistaDiferenciasRenderer) MinSize() fyne.Size { return r.w.MinSize() }

func (r *vistaDiferenciasRenderer) Refresh() {
	r.papel.Refresh()
	canvas.Refresh(r.w)
}

func (r *vistaDiferenciasRenderer) Objects() []fyne.CanvasObject {
	out := make([]fyne.CanvasObject, 0, 2+len(r.cajas))
	out = append(out, r.papel, r.borde)
	for _, c := range r.cajas {
		out = append(out, c)
	}
	return out
}

func (r *vistaDiferenciasRenderer) Destroy() {}
