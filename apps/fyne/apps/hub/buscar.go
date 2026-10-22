// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// paridad-web: apps/frontend/apps/portal/src/pages/buscar.astro

package hub

import (
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// La búsqueda federada: una sola consulta a Mail, Drive, Docus, Calendar, Contacts y
// Rooms, con los resultados agrupados por producto.
//
// Medido en producción a 1280×800 (`medidas/portal/buscar.ancho.json`, estado sin
// consulta): cabecera de 56 con `mb-6`, y el formulario `mb-8 flex gap-2` con un
// campo de 35 (`h-10`) y el botón «Buscar» a su lado.
//
// Lo que NO se replica es el buscador de la BARRA: la topbar es la excepción de
// paridad de §13.6, y en nativo esta pantalla ES el buscador.
const (
	// MargenBusqueda es el `mb-6` bajo la cabecera y HuecoFormulario el `mb-8` bajo
	// el formulario.
	MargenBusqueda   float32 = 6 * 0.25 * tema.Rem
	HuecoFormulario  float32 = 8 * 0.25 * tema.Rem
	HuecoCampoBoton  float32 = 2 * 0.25 * tema.Rem  // `gap-2`
	AltoCampoBuscar  float32 = 10 * 0.25 * tema.Rem // `h-10`
	HuecoResultados  float32 = 6 * 0.25 * tema.Rem  // `flex flex-col gap-6`
	MargenTituloResu float32 = 2 * 0.25 * tema.Rem  // `mb-2` bajo el rótulo del grupo
	HuecoTituloResu  float32 = 2 * 0.25 * tema.Rem  // `gap-2` entre marca y rótulo
	// LadoMarcaResultado es el `<ProductIcon size={16} />` del encabezado de grupo.
	LadoMarcaResultado float32 = 16
	// MargenResultado es el `px-4` y AltoResultado el `py-3` de cada resultado.
	MargenResultado float32 = 4 * 0.25 * tema.Rem
	AltoResultado   float32 = 3 * 0.25 * tema.Rem
	// HuecoResultado es el `gap-0.5` entre título y resumen.
	HuecoResultado float32 = 0.5 * 0.25 * tema.Rem
)

// GrupoOrigen es un producto con sus resultados, en el orden en que aparecen.
type GrupoOrigen struct {
	Origen string
	Items  []api.HubHit
}

// PorOrigen es el `groupBySource` de la web: agrupa conservando el ranking del Hub,
// así que el primer producto que aparece encabeza la lista.
func PorOrigen(hits []api.HubHit) []GrupoOrigen {
	var orden []string
	cubos := map[string][]api.HubHit{}
	for _, h := range hits {
		if _, ok := cubos[h.Source]; !ok {
			orden = append(orden, h.Source)
		}
		cubos[h.Source] = append(cubos[h.Source], h)
	}
	out := make([]GrupoOrigen, 0, len(orden))
	for _, o := range orden {
		out = append(out, GrupoOrigen{Origen: o, Items: cubos[o]})
	}
	return out
}

// Buscar es la pantalla `/buscar`.
type Buscar struct {
	pagina

	T *i18n.Translator

	// Termino es lo que hay escrito. La web lo recorta a 200 en el borde.
	Termino string
	Estado  Estado
	Items   []api.HubHit
	// Fallidos son los orígenes que NO contestaron. Sin decirlo, «no hay resultados»
	// es una afirmación FALSA cuando el único producto que los tenía se cayó.
	Fallidos []string
	// Total es cuántos hay en el Hub, para el «se muestran 20 de 143».
	Total int

	AlBuscar     func(termino string)
	AlAbrirURL   func(string)
	AlReintentar func()
}

var _ fyne.Widget = (*Buscar)(nil)

// NuevoBuscar crea la pantalla.
func NuevoBuscar(t *i18n.Translator) *Buscar {
	b := &Buscar{T: t}
	b.ExtendBaseWidget(b)
	return b
}

// Resize recuerda el ancho.
func (b *Buscar) Resize(s fyne.Size) {
	if b.recordar(s) {
		b.BaseWidget.Resize(s)
		b.Refresh()
		return
	}
	b.BaseWidget.Resize(s)
}

// AccessibilityRole: la pantalla agrupa.
func (b *Buscar) AccessibilityRole() fyne.AccessibleRole { return fyne.AccessibleRoleContainer }

// CreateRenderer construye la pantalla.
func (b *Buscar) CreateRenderer() fyne.WidgetRenderer {
	r := &buscarRenderer{b: b, fondo: ui.NuevoFondoSuite(tema.De(fyne.CurrentApp().Settings().ThemeVariant()))}
	r.rehacer()
	return r
}

// textosBuscar son las claves propias de esta pantalla. El vacío no es un
// `EmptyState`: la web pinta una línea («No hay resultados para “x”») y, debajo, el
// aviso de orígenes caídos.
var textosBuscar = Textos{
	Fallo:       "search.failedTitle",
	FalloCuerpo: "search.failedBody",
}

type buscarRenderer struct {
	b *Buscar
	v tema.Variante

	fondo    *ui.FondoSuite
	cabecera *cabecera
	campo    *ui.Entrada
	boton    *ui.Boton
	aviso    *ui.Vacio
	// previo es lo que va antes de los grupos: la pista sin consulta, el «sin
	// resultados», el aviso de los productos que no contestaron o el «se muestran 20
	// de 143», en el orden de la web. Nil es que no hay nada.
	previo  bloque
	grupos  []grupoResultados
	objetos []fyne.CanvasObject
}

type grupoResultados struct {
	marca   *iconos.Dibujo
	rotulo  *ui.Versal
	caja    *ui.Caja
	filas   []*FilaResultado
	filetes []*canvas.Rectangle
}

func (r *buscarRenderer) rehacer() {
	b := r.b
	r.v = tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	v, t := r.v, b.T
	_, ancho := CajaContenido(b.Ancho())

	r.fondo.PonerVariante(v)
	r.cabecera = nuevaCabecera(v, t.T("search.overline", nil), t.T("search.heading", nil), "")
	r.cabecera.preparar(v, ancho)

	r.campo = ui.NuevaEntrada(t.T("search.placeholder", nil))
	// `h-10 pl-10`, con la lupa de 18 en `left-3`: no es el `Input` de la suite, son
	// clases sueltas, y por eso tiene su talla.
	r.campo.Talla, r.campo.Icono = ui.CampoBusqueda, iconos.Buscar
	r.campo.PonerTexto(b.Termino)
	r.boton = ui.NuevoBoton(t.T("search.submit", nil), func() {
		if b.AlBuscar != nil {
			b.AlBuscar(recortarTermino(r.campo.Texto()))
		}
	})
	r.previo, r.grupos, r.aviso = nil, nil, nil
	_ = ancho

	switch {
	case b.Termino == "":
		r.previo = tope{AnchoParrafo, texto(t.T("search.hint", nil), tema.TextoBase)}
	case b.Estado == Cargando:
		r.previo = tope{AnchoParrafo, texto(t.T("search.busy", nil), tema.TextoBase)}
	case b.Estado == Fallo || b.Estado == SinConexion || b.Estado == Prohibido:
		r.aviso = Aviso(t, v, b.Estado, textosBuscar, b.AlReintentar)
	case len(b.Items) == 0:
		// `flex flex-col gap-3`: el «sin resultados» y, DEBAJO, qué no contestó. Es
		// donde más importa: sin el aviso, «no hay resultados» es falso cuando el único
		// origen que los tenía se cayó.
		previo := col(HuecoVacioBuscar, texto(t.T("search.empty", i18n.Vars{"query": b.Termino}), tema.TextoBase))
		if c := r.avisoCaidos(v); c != nil {
			previo.hijos = append(previo.hijos, c)
		}
		r.previo = previo
	default:
		// `flex flex-col gap-6`: el aviso, el «se muestran 20 de 143» y los grupos.
		previo := col(HuecoResultados)
		if c := r.avisoCaidos(v); c != nil {
			previo.hijos = append(previo.hijos, c)
		}
		if b.Total > len(b.Items) {
			previo.hijos = append(previo.hijos, texto(t.T("search.showing",
				i18n.Vars{"count": len(b.Items), "total": b.Total}), tema.TextoSM))
		}
		if len(previo.hijos) > 0 {
			r.previo = previo
		}
		for _, g := range PorOrigen(b.Items) {
			r.grupos = append(r.grupos, r.grupoDe(v, g))
		}
	}
	r.recomponer()
}

// recortarTermino es el `slice(0, 200)` que la web hace en el borde antes de llamar
// al Hub, que también valida por su cuenta.
func recortarTermino(s string) string {
	s = strings.TrimSpace(s)
	if r := []rune(s); len(r) > MaxTermino {
		return string(r[:MaxTermino])
	}
	return s
}

// avisoCaidos es el `AvisoOrigenesCaidos`: qué productos no contestaron. Nil si
// contestaron todos.
func (r *buscarRenderer) avisoCaidos(v tema.Variante) bloque {
	if len(r.b.Fallidos) == 0 {
		return nil
	}
	nombres := make([]string, 0, len(r.b.Fallidos))
	for _, c := range r.b.Fallidos {
		if p, ok := ProductoDe(c); ok {
			nombres = append(nombres, p.Nombre)
			continue
		}
		nombres = append(nombres, c)
	}
	return nuevoAvisoCaidos(v, r.b.T.T("search.partial", i18n.Vars{"sources": strings.Join(nombres, r.b.T.T("search.partialJoin", nil))}))
}

// AnchoParrafo es el `max-w-xl` de la pista de la búsqueda: 36rem con el rem 14.
const AnchoParrafo float32 = 36 * tema.Rem

// HuecoVacioBuscar es el `gap-3` entre el «sin resultados» y el aviso.
const HuecoVacioBuscar float32 = 3 * 0.25 * tema.Rem

// Medidas del aviso de orígenes caídos: `flex items-start gap-2 rounded-md border
// px-3 py-2 text-sm`, con el icono de 15 y su `mt-px`.
const (
	MargenCaidosX float32 = 3 * 0.25 * tema.Rem
	MargenCaidosY float32 = 2 * 0.25 * tema.Rem
	HuecoCaidos   float32 = 2 * 0.25 * tema.Rem
	IconoCaidos   float32 = 15
)

// avisoCaidosB es el aviso en su caja NEUTRA —`bg-bg-subtle`, texto `text-muted` e
// icono `text-subtle`—: no ha fallado nada de lo que la persona hizo, falta una parte
// de la respuesta, y un error rojo aquí enseñaría a ignorar los rojos.
type avisoCaidosB struct {
	caja  *ui.Caja
	icono *iconos.Dibujo
	texto *ui.Parrafo
}

func nuevoAvisoCaidos(v tema.Variante, texto string) *avisoCaidosB {
	return &avisoCaidosB{
		caja: ui.NuevaCaja(tema.Color("bg-subtle", v), tema.Color("border", v), tema.RadioMD),
		// El dibujo de la web es el círculo con la exclamación: el `danger` del
		// catálogo, en la tinta apagada.
		icono: iconos.Trazo(iconos.Peligro, IconoCaidos, "text-subtle", v),
		texto: ui.NuevoParrafo(texto, tema.TextoSM, ui.Normal, "text-muted", v),
	}
}

func (a *avisoCaidosB) cabe(ancho float32) float32 {
	return max(ancho-2*(tema.Borde+MargenCaidosX)-IconoCaidos-HuecoCaidos, 0)
}

func (a *avisoCaidosB) alto(ancho float32) float32 {
	return 2*(tema.Borde+MargenCaidosY) + max(tema.Borde+IconoCaidos, ui.AltoParrafo(a.texto, a.cabe(ancho)))
}

func (a *avisoCaidosB) poner(x, y, ancho float32) {
	ui.MoverCaja(a.caja, fyne.NewPos(x, y), fyne.NewSize(ancho, a.alto(ancho)))
	ix, iy := x+tema.Borde+MargenCaidosX, y+tema.Borde+MargenCaidosY
	a.icono.Colocar(ix, iy+tema.Borde, IconoCaidos)
	ponerParrafo(a.texto, ix+IconoCaidos+HuecoCaidos, iy, a.cabe(ancho), true)
}

func (a *avisoCaidosB) objetos() []fyne.CanvasObject {
	return append(a.caja.Objetos(), a.icono.Objeto(), a.texto)
}

func (r *buscarRenderer) grupoDe(v tema.Variante, g GrupoOrigen) grupoResultados {
	p, ok := ProductoDe(g.Origen)
	rotulo := g.Origen
	if ok {
		rotulo = p.NombreCorto()
	}
	out := grupoResultados{
		rotulo: ui.NuevoVersal(rotulo, tema.TextoXS, ui.SemiNegrita, tema.Color("text-muted", v)),
		caja:   ui.NuevaCaja(tema.Color("surface", v), tema.Color("border", v), tema.RadioLG),
	}
	if ok {
		out.marca = iconos.Marca(p.Marca, LadoMarcaResultado, v)
	}
	for _, h := range g.Items {
		url := h.URL
		out.filas = append(out.filas, NuevaFilaResultado(h.Title, ui.DetalleHit(r.b.T.Locale(), h), func() {
			if r.b.AlAbrirURL != nil {
				r.b.AlAbrirURL(url)
			}
		}))
	}
	out.filetes = Divisores(v, len(out.filas))
	return out
}

func (r *buscarRenderer) recomponer() {
	r.objetos = []fyne.CanvasObject{r.fondo}
	r.objetos = append(r.objetos, r.cabecera.objetos()...)
	r.objetos = append(r.objetos, r.campo, r.boton)
	if r.previo != nil {
		r.objetos = append(r.objetos, r.previo.objetos()...)
	}
	if r.aviso != nil {
		r.objetos = append(r.objetos, r.aviso)
	}
	for _, g := range r.grupos {
		if g.marca != nil {
			r.objetos = append(r.objetos, g.marca.Objeto())
		}
		r.objetos = append(r.objetos, g.rotulo.Objetos()...)
		r.objetos = append(r.objetos, g.caja.Objetos()...)
		for _, f := range g.filas {
			r.objetos = append(r.objetos, f)
		}
		for _, f := range g.filetes {
			r.objetos = append(r.objetos, f)
		}
	}
}

func (r *buscarRenderer) MinSize() fyne.Size { return fyne.NewSize(0, r.alto(r.b.Ancho())) }

func (r *buscarRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *buscarRenderer) Destroy()                     {}

// Refresh rehace la pantalla con los datos de ahora.
func (r *buscarRenderer) Refresh() {
	r.rehacer()
	r.Layout(r.b.Size())
	canvas.Refresh(r.b)
}

// alto es lo que ocupa la pantalla entera.
func (r *buscarRenderer) alto(ventana float32) float32 {
	_, ancho := CajaContenido(ventana)
	y := MargenY + r.cabecera.alto(ancho) + MargenBusqueda
	y += AltoCampoBuscar + HuecoFormulario
	if r.previo != nil {
		y += r.previo.alto(ancho)
	}
	if r.aviso != nil {
		y += r.aviso.MedidaVacio(ancho)
	}
	for i, g := range r.grupos {
		if i > 0 || r.previo != nil {
			y += HuecoResultados
		}
		y += r.altoGrupo(g)
	}
	return y + MargenY
}

// altoGrupo es el encabezado del producto más su lista con borde.
func (r *buscarRenderer) altoGrupo(g grupoResultados) float32 {
	alto := max(tema.TextoXS.Linea, LadoMarcaResultado) + MargenTituloResu + 2*tema.Borde
	for i, f := range g.filas {
		if i > 0 {
			alto += tema.Borde // `divide-y`
		}
		alto += f.MinSize().Height
	}
	return alto
}

// Layout recorre lo mismo que `alto`.
func (r *buscarRenderer) Layout(t fyne.Size) {
	if t.Width <= 0 {
		return
	}
	r.fondo.Resize(fyne.NewSize(t.Width, max(t.Height, r.alto(t.Width))))
	x, ancho := CajaContenido(t.Width)
	y := r.cabecera.colocar(x) + MargenBusqueda

	mBoton := r.boton.MinSize()
	r.boton.Move(fyne.NewPos(x+ancho-mBoton.Width, y))
	r.boton.Resize(fyne.NewSize(mBoton.Width, AltoCampoBuscar))
	r.campo.Move(fyne.NewPos(x, y))
	r.campo.Resize(fyne.NewSize(ancho-mBoton.Width-HuecoCampoBoton, AltoCampoBuscar))
	y += AltoCampoBuscar + HuecoFormulario

	if r.previo != nil {
		r.previo.poner(x, y, ancho)
		y += r.previo.alto(ancho)
	}
	if r.aviso != nil {
		r.aviso.Move(fyne.NewPos(x, y))
		r.aviso.Resize(fyne.NewSize(ancho, r.aviso.MedidaVacio(ancho)))
	}
	for i, g := range r.grupos {
		if i > 0 || r.previo != nil {
			y += HuecoResultados
		}
		cabeza := max(tema.TextoXS.Linea, LadoMarcaResultado)
		gx := x
		if g.marca != nil {
			g.marca.Colocar(gx, y+(cabeza-LadoMarcaResultado)/2, LadoMarcaResultado)
			gx += LadoMarcaResultado + HuecoTituloResu
		}
		g.rotulo.Colocar(gx, y+(cabeza-tema.TextoXS.Linea)/2)
		y += cabeza + MargenTituloResu

		alto := r.altoGrupo(g) - cabeza - MargenTituloResu
		ui.MoverCaja(g.caja, fyne.NewPos(x, y), fyne.NewSize(ancho, alto))
		fy := y + tema.Borde
		for j, f := range g.filas {
			if j > 0 {
				ColocarDivisor(g.filetes[j-1], x+tema.Borde, fy, ancho-2*tema.Borde)
				fy += tema.Borde
			}
			h := f.MinSize().Height
			f.Move(fyne.NewPos(x+tema.Borde, fy))
			f.Resize(fyne.NewSize(ancho-2*tema.Borde, h))
			fy += h
		}
		y += alto
	}
}
