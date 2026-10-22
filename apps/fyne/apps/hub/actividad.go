// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// paridad-web: apps/frontend/apps/portal/src/pages/actividad.astro

package hub

import (
	"image/color"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// La actividad reciente de TODOS los productos, cortada por día.
//
// Medido en producción a 1280×800 (`medidas/portal/actividad.ancho.json`):
//
//	cabecera        1238 × 82,25 (con su párrafo de entrada), `mb-6`
//	filtros         1238 × 28,25, píldoras con `gap-2`
//	grupos          `flex flex-col gap-8`; cada grupo, encabezado de 22
//	                (`overline` + `pb-2` + filete) + `mb-3` + filas
//	fila            50,75 · `rounded-md px-3 py-2.5 gap-3`, marca de 18
//
// Un grupo de tres filas mide 191,75 = 22 + 10,5 + 3 × 50,75 + 2 × 3,5. Sale clavado.
const (
	// MargenCabeceraActividad es el `mb-6` bajo la cabecera y bajo los filtros.
	MargenCabeceraActividad float32 = 6 * 0.25 * tema.Rem
	// HuecoGrupos es el `gap-8` entre días.
	HuecoGrupos float32 = 8 * 0.25 * tema.Rem
	// HuecoFilas es el `gap-1` dentro de un día.
	HuecoFilas float32 = 1 * 0.25 * tema.Rem
	// MargenEncabezado es el `pb-2` del encabezado de día y HuecoEncabezado su `mb-3`.
	MargenEncabezado float32 = 2 * 0.25 * tema.Rem
	HuecoEncabezado  float32 = 3 * 0.25 * tema.Rem
	// HuecoFiltros es el `gap-2` entre píldoras.
	HuecoFiltros float32 = 2 * 0.25 * tema.Rem
)

// GrupoDeDia es un día de actividad, con el orden que trae el Hub.
type GrupoDeDia struct {
	// Dia es la clave `YYYY-MM-DD` en hora LOCAL. Vacía son los que no traen fecha:
	// no se les inventa una, porque los mezclaría con actividad de un día concreto.
	Dia   string
	Fecha time.Time
	Items []api.HubHit
}

// PorDia es el `groupByDay` de `lib/activity.ts`: agrupa conservando el orden del Hub
// y manda al final los que no traen fecha.
func PorDia(hits []api.HubHit, zona *time.Location) []GrupoDeDia {
	var orden []string
	cubos := map[string]*GrupoDeDia{}
	for _, h := range hits {
		var clave string
		var fecha time.Time
		if h.Timestamp != nil {
			fecha = h.Timestamp.In(zona)
			clave = fecha.Format("2006-01-02")
		}
		g, ok := cubos[clave]
		if !ok {
			g = &GrupoDeDia{Dia: clave, Fecha: fecha}
			cubos[clave] = g
			orden = append(orden, clave)
		}
		g.Items = append(g.Items, h)
	}
	out := make([]GrupoDeDia, 0, len(orden))
	var sinFecha []GrupoDeDia
	for _, c := range orden {
		if c == "" {
			sinFecha = append(sinFecha, *cubos[c])
			continue
		}
		out = append(out, *cubos[c])
	}
	return append(out, sinFecha...)
}

// EtiquetaDia es el `etiquetaDia` de la página: «Hoy», «Ayer» o la fecha larga.
func EtiquetaDia(t *i18n.Translator, g GrupoDeDia, ahora time.Time) string {
	if g.Dia == "" {
		return i18n.FormatDate(t.Locale(), g.Fecha)
	}
	switch g.Dia {
	case ahora.Format("2006-01-02"):
		return t.T("activity.today", nil)
	case ahora.AddDate(0, 0, -1).Format("2006-01-02"):
		return t.T("activity.yesterday", nil)
	}
	return i18n.FormatDate(t.Locale(), g.Fecha)
}

// Actividad es la pantalla `/actividad`.
type Actividad struct {
	pagina

	T     *i18n.Translator
	Ahora time.Time

	Estado Estado
	Items  []api.HubHit
	// Origen es el filtro por producto; vacío es «Todo».
	Origen string

	AlFiltrar    func(origen string)
	AlAbrirURL   func(string)
	AlReintentar func()
}

var _ fyne.Widget = (*Actividad)(nil)

// NuevaActividad crea la pantalla.
func NuevaActividad(t *i18n.Translator, ahora time.Time) *Actividad {
	a := &Actividad{T: t, Ahora: ahora}
	a.ExtendBaseWidget(a)
	return a
}

// Resize recuerda el ancho: el alto depende de él (el párrafo de entrada se parte).
func (a *Actividad) Resize(s fyne.Size) {
	if a.recordar(s) {
		a.BaseWidget.Resize(s)
		a.Refresh()
		return
	}
	a.BaseWidget.Resize(s)
}

// AccessibilityRole: la pantalla agrupa.
func (a *Actividad) AccessibilityRole() fyne.AccessibleRole { return fyne.AccessibleRoleContainer }

// CreateRenderer construye la pantalla.
func (a *Actividad) CreateRenderer() fyne.WidgetRenderer {
	r := &actividadRenderer{a: a, fondo: ui.NuevoFondoSuite(tema.De(fyne.CurrentApp().Settings().ThemeVariant()))}
	r.rehacer()
	return r
}

// textosActividad son las claves propias de esta pantalla.
var textosActividad = Textos{
	Vacio:       "activity.empty",
	VacioCuerpo: "activity.intro",
	Fallo:       "activity.failedTitle",
	FalloCuerpo: "activity.failedBody",
}

type actividadRenderer struct {
	a *Actividad
	v tema.Variante

	fondo     *ui.FondoSuite
	cabecera  *cabecera
	filtros   []*Pildora
	grupos    []grupoPintado
	aviso     *ui.Vacio
	esqueleto []fyne.CanvasObject
	objetos   []fyne.CanvasObject
}

// grupoPintado es un día ya construido: su encabezado, su filete y sus filas.
type grupoPintado struct {
	titulo *ui.Versal
	filete *canvas.Rectangle
	filas  []*FilaActividad
}

func (r *actividadRenderer) rehacer() {
	a := r.a
	r.v = tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	v, t := r.v, a.T
	_, ancho := CajaContenido(a.Ancho())

	r.fondo.PonerVariante(v)
	r.cabecera = nuevaCabecera(v, t.T("activity.overline", nil), t.T("activity.heading", nil),
		t.T("activity.intro", nil))
	r.cabecera.preparar(v, ancho)
	r.filtros, r.grupos, r.aviso, r.esqueleto = nil, nil, nil, nil

	// Los filtros solo salen si hay más de un producto en la tanda o ya hay uno
	// puesto: ofrecer un filtro que no filtra es ruido.
	presentes := r.presentes()
	if len(presentes) > 1 || a.Origen != "" {
		r.filtros = append(r.filtros, r.pildora("", t.T("activity.all", nil), iconos.One, false))
		for _, clave := range presentes {
			p, ok := ProductoDe(clave)
			rotulo := clave
			if ok {
				rotulo = p.NombreCorto()
			}
			r.filtros = append(r.filtros, r.pildora(clave, rotulo, p.Marca, ok))
		}
	}

	switch a.Estado {
	case Cargando:
		for range 5 {
			r.esqueleto = append(r.esqueleto, ui.NuevoEsqueleto(0, MedidaActividad(true)))
		}
	case SinNada:
		r.aviso = Aviso(t, v, SinNada, textosActividad, nil)
	case Fallo, SinConexion, Prohibido:
		r.aviso = Aviso(t, v, a.Estado, textosActividad, a.AlReintentar)
	case Listo:
		for _, g := range PorDia(a.Items, a.Ahora.Location()) {
			r.grupos = append(r.grupos, r.grupoDe(v, g))
		}
	}
	r.recomponer()
}

// presentes son los productos que HAY en esta tanda, sin repetir y en orden de
// aparición.
func (r *actividadRenderer) presentes() []string {
	var out []string
	visto := map[string]bool{}
	for _, h := range r.a.Items {
		if !visto[h.Source] {
			visto[h.Source] = true
			out = append(out, h.Source)
		}
	}
	return out
}

func (r *actividadRenderer) pildora(clave, rotulo string, marca iconos.Producto, conMarca bool) *Pildora {
	p := NuevaPildora(rotulo, clave == r.a.Origen, func() {
		if r.a.AlFiltrar != nil {
			r.a.AlFiltrar(clave)
		}
	})
	if conMarca && clave != "" {
		p.Marca, p.ConMarca = marca, true
	}
	return p
}

func (r *actividadRenderer) grupoDe(v tema.Variante, g GrupoDeDia) grupoPintado {
	out := grupoPintado{
		titulo: ui.NuevoVersal(EtiquetaDia(r.a.T, g, r.a.Ahora), tema.TextoXS, ui.SemiNegrita,
			tema.Color("text-subtle", v)),
		filete: canvas.NewRectangle(tema.Color("border", v)),
	}
	for _, h := range g.Items {
		out.filas = append(out.filas, r.filaDe(h))
	}
	return out
}

func (r *actividadRenderer) filaDe(h api.HubHit) *FilaActividad {
	p, ok := ProductoDe(h.Source)
	enlace := h.URL
	f := NuevaFilaActividad(h.Title, ui.DetalleHit(r.a.T.Locale(), h), func() {
		if r.a.AlAbrirURL != nil {
			r.a.AlAbrirURL(enlace)
		}
	})
	f.Conocido = ok
	if ok {
		f.Marca, f.Origen = p.Marca, p.NombreCorto()
	} else {
		f.Origen = h.Source
	}
	if h.Timestamp != nil {
		f.Cuando = i18n.FormatTimeAgo(r.a.T.Locale(), *h.Timestamp, r.a.Ahora)
	}
	return f
}

func (r *actividadRenderer) recomponer() {
	r.objetos = []fyne.CanvasObject{r.fondo}
	r.objetos = append(r.objetos, r.cabecera.objetos()...)
	for _, p := range r.filtros {
		r.objetos = append(r.objetos, p)
	}
	r.objetos = append(r.objetos, r.esqueleto...)
	if r.aviso != nil {
		r.objetos = append(r.objetos, r.aviso)
	}
	for _, g := range r.grupos {
		r.objetos = append(r.objetos, g.titulo.Objetos()...)
		r.objetos = append(r.objetos, g.filete)
		for _, f := range g.filas {
			r.objetos = append(r.objetos, f)
		}
	}
}

func (r *actividadRenderer) MinSize() fyne.Size { return fyne.NewSize(0, r.alto(r.a.Ancho())) }

func (r *actividadRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *actividadRenderer) Destroy()                     {}

// Refresh rehace la pantalla con los datos de ahora. Ver el comentario del lanzador:
// entre un estado y otro cambia CUÁNTOS objetos hay, no sus colores.
func (r *actividadRenderer) Refresh() {
	r.rehacer()
	r.Layout(r.a.Size())
	canvas.Refresh(r.a)
}

// AltoEncabezadoDia es el encabezado de un día: la caja de línea del rótulo, su `pb-2`
// y el filete, que en `border-box` va dentro.
func AltoEncabezadoDia() float32 {
	return tema.TextoXS.Linea + MargenEncabezado + tema.Borde
}

// alto es lo que ocupa la pantalla entera. Es la ÚNICA cuenta.
func (r *actividadRenderer) alto(ventana float32) float32 {
	_, ancho := CajaContenido(ventana)
	y := MargenY + r.cabecera.alto(ancho) + MargenCabeceraActividad
	if len(r.filtros) > 0 {
		y += MedidaPildora("").Height + MargenCabeceraActividad
	}
	switch {
	case len(r.esqueleto) > 0:
		y += float32(len(r.esqueleto))*MedidaActividad(true) + float32(len(r.esqueleto)-1)*HuecoFilas
	case r.aviso != nil:
		y += r.aviso.MedidaVacio(ancho)
	default:
		for i, g := range r.grupos {
			if i > 0 {
				y += HuecoGrupos
			}
			y += AltoEncabezadoDia() + HuecoEncabezado
			for j, f := range g.filas {
				if j > 0 {
					y += HuecoFilas
				}
				y += f.MinSize().Height
			}
		}
	}
	return y + MargenY
}

// Layout recorre lo mismo que `alto`, en el mismo orden.
func (r *actividadRenderer) Layout(t fyne.Size) {
	if t.Width <= 0 {
		return
	}
	r.fondo.Resize(fyne.NewSize(t.Width, max(t.Height, r.alto(t.Width))))
	x, ancho := CajaContenido(t.Width)
	y := r.cabecera.colocar(x) + MargenCabeceraActividad

	if len(r.filtros) > 0 {
		// `flex-wrap gap-2`: las píldoras van en fila y saltan de línea al no caber,
		// que es lo que hace la web en un teléfono.
		px, alto := x, MedidaPildora("").Height
		filas := float32(1)
		for _, p := range r.filtros {
			m := p.MinSize()
			if px > x && px+m.Width > x+ancho {
				px = x
				y += alto + HuecoFiltros
				filas++
			}
			p.Move(fyne.NewPos(px, y))
			p.Resize(m)
			px += m.Width + HuecoFiltros
		}
		y += alto + MargenCabeceraActividad
	}

	for i, e := range r.esqueleto {
		e.Move(fyne.NewPos(x, y+float32(i)*(MedidaActividad(true)+HuecoFilas)))
		e.Resize(fyne.NewSize(ancho, MedidaActividad(true)))
	}
	if r.aviso != nil {
		r.aviso.Move(fyne.NewPos(x, y))
		r.aviso.Resize(fyne.NewSize(ancho, r.aviso.MedidaVacio(ancho)))
	}
	for i, g := range r.grupos {
		if i > 0 {
			y += HuecoGrupos
		}
		g.titulo.Colocar(x, y)
		g.filete.Move(fyne.NewPos(x, y+tema.TextoXS.Linea+MargenEncabezado))
		g.filete.Resize(fyne.NewSize(ancho, tema.Borde))
		y += AltoEncabezadoDia() + HuecoEncabezado
		for j, f := range g.filas {
			if j > 0 {
				y += HuecoFilas
			}
			h := f.MinSize().Height
			f.Move(fyne.NewPos(x, y))
			f.Resize(fyne.NewSize(ancho, h))
			y += h
		}
	}
}

// ---- La píldora de filtro ----

// Medidas de la píldora (`rounded-control border px-3 py-1.5 text-sm font-medium`),
// medidas en producción: 28,25 de alto y de 63,25 a 93,25 de ancho, con `gap-1.5`
// entre la marca de 14 y el rótulo.
const (
	MargenPildora float32 = 3 * 0.25 * tema.Rem
	AltoPildora   float32 = 1.5 * 0.25 * tema.Rem
	HuecoPildora  float32 = 1.5 * 0.25 * tema.Rem
	// LadoMarcaPildora es el `<ProductIcon size={14} />`.
	LadoMarcaPildora float32 = 14
)

// MedidaPildora es lo que ocupa una píldora con ese rótulo (sin marca).
func MedidaPildora(rotulo string) fyne.Size {
	ancho := ui.Medir(rotulo, tema.TextoSM, ui.Medio).Width + 2*MargenPildora + 2*tema.Borde
	return fyne.NewSize(ancho, 2*AltoPildora+tema.TextoSM.Linea+2*tema.Borde)
}

// Pildora es un filtro por producto de la pantalla de actividad.
type Pildora struct {
	ui.Pulsable

	Rotulo string
	Activa bool
	// ConMarca enseña la tesela del producto delante del rótulo.
	ConMarca bool
	Marca    iconos.Producto
}

var _ fyne.Widget = (*Pildora)(nil)

// NuevaPildora crea el filtro.
func NuevaPildora(rotulo string, activa bool, alPulsar func()) *Pildora {
	p := &Pildora{Rotulo: rotulo, Activa: activa}
	p.Nombre = rotulo
	p.AlPulsar = alPulsar
	p.ExtendBaseWidget(p)
	return p
}

// CreateRenderer compone la píldora.
func (p *Pildora) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	relleno, tinta, borde := p.colores(v)
	r := &pildoraRenderer{
		p:      p,
		aro:    ui.NuevoAro(tema.Forma("radius-control"), ui.SinHueco, v).Abierto(),
		caja:   ui.NuevaCaja(relleno, borde, tema.Forma("radius-control")),
		rotulo: ui.Texto(p.Rotulo, tema.TextoSM, ui.Medio, tinta),
	}
	r.objetos = append(r.aro.Objetos(), r.caja.Objetos()...)
	if p.ConMarca {
		r.marca = iconos.Marca(p.Marca, LadoMarcaPildora, v)
		r.objetos = append(r.objetos, r.marca.Objeto())
	}
	r.objetos = append(r.objetos, r.rotulo)
	return r
}

// colores son los de la web: activa en `bg-primary` con la tinta de `on-primary`;
// inactiva con borde de `border` y texto apagado, que al pasar el puntero suben a
// `text-subtle` y `text`.
//
// La inactiva NO tiene fondo en la web y enseña el de la página, que es `bg`. Aquí se
// pinta ese `bg` a propósito y no un transparente: `ui.Caja` dibuja el borde como un
// rectángulo LLENO por detrás del relleno, así que con el relleno transparente la
// píldora entera salía del color del borde.
func (p *Pildora) colores(v tema.Variante) (relleno, tinta, borde color.NRGBA) {
	if p.Activa {
		return tema.Color("primary", v), tema.Color("on-primary", v), tema.Color("primary", v)
	}
	if p.Encima() {
		return tema.Color("bg", v), tema.Color("text", v), tema.Color("text-subtle", v)
	}
	return tema.Color("bg", v), tema.Color("text-muted", v), tema.Color("border", v)
}

type pildoraRenderer struct {
	p       *Pildora
	aro     *ui.Aro
	caja    *ui.Caja
	marca   *iconos.Dibujo
	rotulo  *canvas.Text
	objetos []fyne.CanvasObject
}

func (r *pildoraRenderer) MinSize() fyne.Size {
	m := MedidaPildora(r.p.Rotulo)
	if r.p.ConMarca {
		m.Width += LadoMarcaPildora + HuecoPildora
	}
	return m
}

func (r *pildoraRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *pildoraRenderer) Destroy()                     {}

// Layout centra marca y rótulo.
func (r *pildoraRenderer) Layout(t fyne.Size) {
	r.aro.Colocar(t)
	r.caja.Redimensionar(t)
	x := tema.Borde + MargenPildora
	if r.marca != nil {
		r.marca.Colocar(x, (t.Height-LadoMarcaPildora)/2, LadoMarcaPildora)
		x += LadoMarcaPildora + HuecoPildora
	}
	ui.ColocarTexto(r.rotulo, tema.TextoSM, x, (t.Height-tema.TextoSM.Linea)/2)
}

// Refresh repinta con el estado del puntero y del foco.
func (r *pildoraRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	relleno, tinta, borde := r.p.colores(v)
	r.caja.Pintar(relleno, borde)
	r.rotulo.Color = tinta
	r.rotulo.Refresh()
	r.aro.Pintar(v)
	r.aro.Ver(r.p.Enfocado())
}
