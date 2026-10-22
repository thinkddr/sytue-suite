// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package certipdf

import (
	"context"
	"encoding/json"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/buscar"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

/*
El panel BUSCAR (`Buscar` de `Paneles.tsx`): buscar en el texto de TODO el documento y,
debajo, reemplazar.

Buscar va sobre el texto que el motor extrae (`op: texto`), no sobre lo dibujado: el
buscador de un visor solo encontraría lo que ya está pintado, y aquí solo están pintadas
las páginas por las que se ha pasado. Reemplazar cuelga de la búsqueda porque es la misma
pregunta: primero se ve qué hay y qué cambiaría —y si el texto nuevo va a quedar apretado,
porque un PDF no tiene reflujo—, y solo después se cambia, en una copia.
*/

// Coincidencia es un fragmento que casa con la búsqueda, con lo que pasaría al cambiarlo.
type Coincidencia struct {
	Pagina  int    `json:"pagina"`
	ID      int    `json:"id"`
	Antes   string `json:"antes"`
	Despues string `json:"despues"`
	// Cabe es falso si el texto nuevo va a quedar apretado.
	Cabe bool `json:"cabe"`
}

// resultadoBusqueda es una fila de la lista: la página (1-based) y el fragmento.
type resultadoBusqueda struct {
	pagina int
	texto  string
}

// Buscar busca `q` en los fragmentos, sin distinguir mayúsculas. Es la cuenta de
// `buscar` de la web, con su tope: pasados 300 resultados, cada página aporta como mucho
// uno más (el `return` de la web sale de la página, no de la búsqueda).
func Buscar(paginas [][]Fragmento, q string) []resultadoBusqueda {
	aguja := buscar.Plegar(q)
	var out []resultadoBusqueda
	for i, frags := range paginas {
		for _, f := range frags {
			if !strings.Contains(buscar.Plegar(f.Texto), aguja) {
				continue
			}
			out = append(out, resultadoBusqueda{pagina: i + 1, texto: f.Texto})
			if len(out) > 300 {
				break
			}
		}
	}
	return out
}

// panelBuscar es el estado del panel: sobrevive a cambiar de pestaña y a recomponer.
type panelBuscar struct {
	l *PanelLateral

	campo, reemplazo *ui.Entrada
	consulta         string
	resultados       []resultadoBusqueda
	buscando         bool
	abierto          bool
	revisando        bool
	// coincidencias es nil mientras no se haya preguntado qué cambiaría.
	coincidencias []Coincidencia
	revisadas     bool

	vista *fyne.Container
}

func (l *PanelLateral) buscar() fyne.CanvasObject {
	if l.buscarP == nil {
		b := &panelBuscar{l: l}
		// `rounded-md` en la clase, pero Chromium pinta el `type="search"` con su radio
		// propio: medido, 4.
		b.campo = nuevoCampoPanel(l.t("pdf.search.placeholder", nil), tema.Espacio(1.5), 4, anchoUtil())
		b.campo.Busqueda = true // `type="search"`
		b.campo.AlCambiar(b.alEscribir)
		b.reemplazo = nuevoCampoPanel(l.t("pdf.replace.placeholder", nil), tema.Espacio(1.5), tema.RadioMD,
			anchoUtil()-2*tema.Borde-2*tema.Espacio(2))
		b.vista = container.New(ui.Columna{Hueco: tema.Espacio(2)})
		l.buscarP = b
	}
	l.buscarP.rehacer()
	// El foco entra en el campo al abrir la pestaña: es lo que hace que Ctrl+F sirva.
	campo := l.buscarP.campo
	fyne.Do(func() {
		if c := fyne.CurrentApp().Driver().CanvasForObject(campo); c != nil {
			c.Focus(campo)
		}
	})
	return l.buscarP.vista
}

func (b *panelBuscar) t(clave string) string { return b.l.t(clave, nil) }

func (b *panelBuscar) alEscribir(q string) {
	b.consulta = q
	if len([]rune(strings.TrimSpace(q))) < 2 {
		b.resultados = nil
		b.rehacer()
		return
	}
	p := b.l.pant
	if p == nil {
		return
	}
	b.buscando = true
	b.rehacer()
	p.cargarTexto(func(paginas [][]Fragmento) {
		b.buscando = false
		if paginas != nil && b.consulta == q {
			b.resultados = Buscar(paginas, q)
		}
		b.rehacer()
	})
}

// rehacer recompone el panel con su estado; el campo es el MISMO objeto, así que no
// pierde el foco ni el cursor.
func (b *panelBuscar) rehacer() {
	l := b.l
	partes := []fyne.CanvasObject{b.campo}
	if l.sinCapaDeTexto() {
		partes = append(partes, avisoAmbar(b.t("pdf.search.scanned"), anchoUtil()))
	}
	partes = append(partes, b.cajaReemplazar())
	if b.buscando {
		partes = append(partes, ui.NuevoRotulo(b.t("pdf.search.working"), tema.Texto2XS, ui.Normal, "text-subtle"))
	}
	if !b.buscando && len([]rune(strings.TrimSpace(b.consulta))) >= 2 {
		partes = append(partes, ui.NuevoRotulo(
			l.t("pdf.search.count", i18n.Vars{"count": len(b.resultados)}), tema.Texto2XS, ui.Normal, "text-muted"))
	}
	if len(b.resultados) > 0 {
		filas := make([]fyne.CanvasObject, 0, len(b.resultados))
		for _, r := range b.resultados {
			n := r.pagina
			filas = append(filas, nuevaFilaResultado(l.t("pdf.page.n", i18n.Vars{"n": n}), r.texto, anchoUtil(), func() {
				if l.AlIrA != nil {
					l.AlIrA(n)
				}
			}))
		}
		partes = append(partes, container.New(ui.Columna{Hueco: tema.Espacio(1)}, filas...))
	}
	b.vista.Objects = partes
	b.vista.Refresh()
}

// cajaReemplazar es el `<details>` de reemplazar en todo el documento.
func (b *panelBuscar) cajaReemplazar() fyne.CanvasObject {
	l := b.l
	resumen := nuevoDetalles(b.t("pdf.replace.title"), b.abierto, func(a bool) {
		b.abierto = a
		b.rehacer()
	})
	dentro := []fyne.CanvasObject{resumen}
	if b.abierto {
		ancho := anchoUtil() - 2*tema.Borde - 2*tema.Espacio(2)
		rotulo := "pdf.replace.check"
		if b.revisando {
			rotulo = "pdf.replace.checking"
		}
		revisar := botonPanel(b.t(rotulo), tema.Texto2XS, tema.Espacio(1), b.revisar)
		revisar.Inerte = b.revisando || l.trabajando() || strings.TrimSpace(b.consulta) == ""
		cuerpo := append([]fyne.CanvasObject{b.reemplazo, container.New(alPrincipio{}, revisar)}, b.coincidenciasVista(ancho)...)
		dentro = append(dentro, container.New(ui.Margen{Arr: tema.Espacio(2)},
			container.New(ui.Columna{Hueco: tema.Espacio(1.5)}, cuerpo...)))
	}
	return nuevoRecuadroDe(container.New(ui.Columna{}, dentro...), anchoUtil())
}

func (b *panelBuscar) coincidenciasVista(ancho float32) []fyne.CanvasObject {
	l := b.l
	if !b.revisadas {
		return nil
	}
	if len(b.coincidencias) == 0 {
		return []fyne.CanvasObject{ui.NuevoRotulo(b.t("pdf.replace.none"), tema.Texto2XS, ui.Normal, "text-subtle")}
	}
	n := len(b.coincidencias)
	out := []fyne.CanvasObject{ui.NuevoRotulo(l.t("pdf.replace.found", i18n.Vars{"count": n}), tema.Texto2XS, ui.Normal, "text")}
	apretadas := 0
	for _, c := range b.coincidencias {
		if !c.Cabe {
			apretadas++
		}
	}
	// Sin reflujo, un texto más largo se aprieta hasta que se nota: decirlo ANTES.
	if apretadas > 0 {
		out = append(out, avisoAmbar(l.t("pdf.replace.tight", i18n.Vars{"count": apretadas}), ancho))
	}
	filas := make([]fyne.CanvasObject, 0, min(n, 40))
	for _, c := range b.coincidencias[:min(n, 40)] {
		filas = append(filas, b.filaCoincidencia(c, ancho))
	}
	// `max-h-40 overflow-auto space-y-1`.
	lista := container.NewVScroll(container.New(ui.Columna{Hueco: tema.Espacio(1)}, filas...))
	out = append(out, container.New(topeAlto{alto: tema.Espacio(40)}, lista))
	aplicar := botonPrimario(l.t("pdf.replace.apply", i18n.Vars{"count": n}), tema.TextoXS, tema.Espacio(3), tema.Espacio(1.5), b.aplicar)
	aplicar.Inerte = l.trabajando()
	return append(out, container.New(alPrincipio{}, aplicar))
}

// filaCoincidencia es `bg-bg-subtle rounded-sm px-2 py-1`: la página, lo de antes tachado
// y lo de después, en ámbar si va a quedar apretado.
func (b *panelBuscar) filaCoincidencia(c Coincidencia, ancho float32) fyne.CanvasObject {
	l := b.l
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	dentro := ancho - 2*tema.Espacio(2)
	n := c.Pagina + 1
	ir := enlaceBoton(l.t("pdf.page.n", i18n.Vars{"n": n}), "primary-text", func() {
		if l.AlIrA != nil {
			l.AlIrA(n)
		}
	})
	tinta := "text"
	if !c.Cabe {
		tinta = "warning-text"
	}
	fondo := canvas.NewRectangle(tema.Color("bg-subtle", v))
	fondo.CornerRadius = tema.RadioSM
	return container.NewStack(fondo, container.New(ui.Margen{Izq: tema.Espacio(2), Arr: tema.Espacio(1), Der: tema.Espacio(2), Abj: tema.Espacio(1)},
		container.New(ui.Columna{}, container.New(alPrincipio{}, ir),
			nuevoTachado(c.Antes, tema.Texto2XS, "text-subtle", dentro),
			rotuloRecortado(c.Despues, tema.Texto2XS, ui.Normal, tinta, dentro))))
}

// revisar pide qué cambiaría, sin cambiar nada.
func (b *panelBuscar) revisar() {
	p := b.l.pant
	if p == nil || p.doc == nil || strings.TrimSpace(b.consulta) == "" {
		return
	}
	b.revisando = true
	b.rehacer()
	doc := p.doc
	busqueda, _ := json.Marshal(map[string]string{"buscar": b.consulta, "reemplazar": b.reemplazo.Texto()})
	go func() {
		ctx, parar := context.WithTimeout(p.ctx, plazo)
		defer parar()
		var r struct {
			Coincidencias []Coincidencia `json:"coincidencias"`
		}
		err := doc.Leer(ctx, "/pdf/edit", map[string]string{"op": "buscar-reemplazar", "busqueda": string(busqueda)}, &r)
		fyne.Do(func() {
			defer p.avisarRed()
			b.revisando, b.revisadas = false, true
			b.coincidencias = r.Coincidencias
			if err != nil {
				b.coincidencias = nil
			}
			b.rehacer()
		})
	}()
}

// aplicar reemplaza en una copia (`reemplazar-texto` deja SIEMPRE copia).
func (b *panelBuscar) aplicar() {
	p := b.l.pant
	if p == nil {
		return
	}
	busqueda, _ := json.Marshal(map[string]string{"buscar": b.consulta, "reemplazar": b.reemplazo.Texto()})
	p.Aplicar(api.PDFCambio{
		Op:     "reemplazar-texto",
		Campos: map[string]string{"aplicar": "1", "busqueda": string(busqueda)},
		Sufijo: b.t("pdf.suffix.edited"),
	}, b.t("pdf.replace.done"), func(bool) {
		b.coincidencias, b.revisadas = nil, false
		b.rehacer()
	})
}

// ---- piezas de este panel ----

// filaResultado es `hover:bg-bg-subtle w-full rounded-md p-1.5 text-left`: la página en
// `text-2xs text-text-subtle` y el fragmento en `text-xs`, como mucho dos líneas.
type filaResultado struct {
	ui.Pulsable
	pagina string
	parr   *ui.Parrafo
	ancho  float32
}

func nuevaFilaResultado(pagina, texto string, ancho float32, alPulsar func()) *filaResultado {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	f := &filaResultado{
		pagina: pagina, ancho: ancho,
		parr: ui.NuevoParrafo(texto, tema.TextoXS, ui.Normal, "text", v).Recortado(2).A(ancho - 2*tema.Espacio(1.5)),
	}
	f.Nombre, f.AlPulsar = pagina+". "+texto, alPulsar
	f.ExtendBaseWidget(f)
	return f
}

func (f *filaResultado) MinSize() fyne.Size {
	f.ExtendBaseWidget(f)
	return fyne.NewSize(f.ancho, tema.Texto2XS.Linea+f.parr.MinSize().Height+2*tema.Espacio(1.5))
}

func (f *filaResultado) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &filaResultadoRenderer{
		f: f, fondo: canvas.NewRectangle(tema.Color("bg-subtle", v)),
		aro:    ui.NuevoAro(tema.RadioMD, ui.SinHueco, v).Abierto(),
		pagina: ui.Texto(f.pagina, tema.Texto2XS, ui.Normal, tema.Color("text-subtle", v)),
	}
	r.fondo.CornerRadius = tema.RadioMD
	r.Refresh()
	return r
}

type filaResultadoRenderer struct {
	f      *filaResultado
	fondo  *canvas.Rectangle
	aro    *ui.Aro
	pagina *canvas.Text
}

func (r *filaResultadoRenderer) MinSize() fyne.Size { return r.f.MinSize() }
func (r *filaResultadoRenderer) Destroy()           {}
func (r *filaResultadoRenderer) Objects() []fyne.CanvasObject {
	return r.aro.Antes(r.fondo, r.pagina, r.f.parr)
}

func (r *filaResultadoRenderer) Layout(t fyne.Size) {
	r.aro.Colocar(t)
	r.fondo.Resize(t)
	m := tema.Espacio(1.5)
	ui.ColocarTexto(r.pagina, tema.Texto2XS, m, m)
	r.f.parr.Move(fyne.NewPos(m, m+tema.Texto2XS.Linea))
	r.f.parr.Resize(fyne.NewSize(t.Width-2*m, r.f.parr.MinSize().Height))
}

func (r *filaResultadoRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r.fondo.Hidden = !r.f.Encima()
	r.fondo.FillColor = tema.Color("bg-subtle", v)
	r.fondo.Refresh()
	r.pagina.Color = tema.Color("text-subtle", v)
	r.pagina.Refresh()
	r.aro.Pintar(v)
	r.aro.Ver(r.f.Enfocado())
	r.Layout(r.f.Size())
}

// alPrincipio es `self-start`: el hijo a su ancho, pegado a la izquierda.
type alPrincipio struct{}

func (alPrincipio) MinSize(objs []fyne.CanvasObject) fyne.Size { return objs[0].MinSize() }

func (alPrincipio) Layout(objs []fyne.CanvasObject, _ fyne.Size) {
	objs[0].Move(fyne.NewPos(0, 0))
	objs[0].Resize(objs[0].MinSize())
}

// topeAlto es `max-h-*`: el hijo mide lo que pida, hasta el tope; pasado, se desplaza.
type topeAlto struct{ alto float32 }

func (t topeAlto) MinSize(objs []fyne.CanvasObject) fyne.Size {
	m := contenidoDe(objs[0])
	return fyne.NewSize(m.Width, min(m.Height, t.alto))
}

func (t topeAlto) Layout(objs []fyne.CanvasObject, s fyne.Size) {
	objs[0].Move(fyne.NewPos(0, 0))
	objs[0].Resize(s)
}

// recuadroDe es un [recuadro] (`border rounded-md p-2`) que llena el ancho del panel.
func nuevoRecuadroDe(dentro fyne.CanvasObject, ancho float32) fyne.CanvasObject {
	return container.New(anchoFijo{ancho: ancho}, nuevoRecuadro(dentro))
}

// anchoFijo le da al hijo un ancho fijo y su alto mínimo.
type anchoFijo struct{ ancho float32 }

func (a anchoFijo) MinSize(objs []fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(a.ancho, objs[0].MinSize().Height)
}

func (a anchoFijo) Layout(objs []fyne.CanvasObject, s fyne.Size) {
	objs[0].Move(fyne.NewPos(0, 0))
	objs[0].Resize(fyne.NewSize(a.ancho, s.Height))
}

var _ fyne.Widget = (*filaResultado)(nil)
