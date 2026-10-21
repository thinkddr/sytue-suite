// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

// paridad-web: apps/frontend/packages/ui/src/components/Badge.tsx
// paridad-web: apps/frontend/packages/ui/src/components/EmptyState.tsx

import (
	"image/color"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/nucleo/tokens"
)

// Las piezas que CUENTAN algo: la insignia, el contador, el medidor de cuota y el
// estado vacío. Ninguna es interactiva en la web, pero las tres primeras se anuncian
// (una insignia de estado y un contador de no leídos son información, no adorno), así
// que llevan cáscara de widget para poder decir lo que son.

// ---- Insignia ----

// Tono son los siete tonos de Badge.tsx. El valor cero es `neutral`, como allí.
type Tono uint8

// Los siete tonos.
const (
	InsigniaNeutra Tono = iota
	InsigniaPrimaria
	InsigniaAcento
	InsigniaExito
	InsigniaAviso
	InsigniaPeligro
	InsigniaInfo
)

// tonosInsignia es el mapa `tones` de Badge.tsx: fondo y tinta por tono, SIEMPRE por token
// semántico. Los escalones crudos (`-50`/`-700`) no los reasigna el bloque oscuro y
// salían como manchas casi blancas con texto marrón.
var tonosInsignia = [...][2]string{
	InsigniaNeutra:   {"bg-subtle", "text-muted"},
	InsigniaPrimaria: {"selected", "primary-text"},
	InsigniaAcento:   {"violet-bg", "violet-text"},
	InsigniaExito:    {"success-bg", "success-text"},
	InsigniaAviso:    {"warning-bg", "warning-text"},
	InsigniaPeligro:  {"danger-bg", "danger-text"},
	InsigniaInfo:     {"info-bg", "info-text"},
}

// Medidas de la insignia, en px de CSS.
const (
	// MargenInsignia es `px-2` y AltoInsignia es `py-1`.
	MargenInsignia float32 = 2 * 0.25 * tema.Rem
	AltoInsignia   float32 = 1 * 0.25 * tema.Rem
)

// escalonInsignia es `text-xs` con `leading-none`: la caja de línea NO es 14, es el
// propio tamaño de letra, porque `leading-none` fija `line-height: 1`.
var escalonInsignia = tema.Escalon{Tam: tema.TextoXS.Tam, Linea: tema.TextoXS.Tam}

// Insignia es el `Badge` de la suite: una etiqueta de estado o un contador.
type Insignia struct {
	widget.BaseWidget

	Texto string
	Tono  Tono
	// Cuadrada es `pill={false}`: esquinas de `rounded-sm` en vez de píldora.
	Cuadrada bool
	// Grupo es la opacidad del contenedor que la envuelve (ver Grupo). Cero, ninguna.
	Grupo Grupo
}

var _ fyne.Widget = (*Insignia)(nil)

// NuevaInsignia crea una insignia neutra en forma de píldora.
func NuevaInsignia(texto string, t Tono) *Insignia {
	i := &Insignia{Texto: texto, Tono: t}
	i.ExtendBaseWidget(i)
	return i
}

// MedidaInsignia es lo que ocupa. Es la única cuenta.
//
// La variante de CONTEO no está aquí: es `Contador`, que llegó con el cromo de
// navegación y ya pinta el círculo de no leídos de `<Badge count>`. Se importa.
func MedidaInsignia(texto string) fyne.Size {
	ancho := Medir(texto, escalonInsignia, SemiNegrita).Width
	return fyne.NewSize(ancho+2*MargenInsignia, 2*AltoInsignia+escalonInsignia.Linea)
}

func (i *Insignia) colores(v tema.Variante) (relleno, tinta color.NRGBA) {
	t := tonosInsignia[i.Tono]
	return i.Grupo.Aplicar(tema.Color(t[0], v), v), i.Grupo.Aplicar(tema.Color(t[1], v), v)
}

func (i *Insignia) radio() float32 {
	if i.Cuadrada {
		return tema.RadioSM
	}
	return tema.RadioPill
}

// CreateRenderer compone la insignia.
func (i *Insignia) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	relleno, tinta := i.colores(v)
	r := &insigniaRenderer{
		i:     i,
		caja:  NuevaCaja(relleno, color.NRGBA{}, i.radio()),
		texto: Texto(i.Texto, escalonInsignia, SemiNegrita, tinta),
	}
	r.objetos = append(r.caja.Objetos(), r.texto)
	return r
}

// AccessibilityLabel es el texto de la insignia: comunica con PALABRAS, no solo con
// color, que es la regla de Badge.tsx (WCAG 1.4.1).
func (i *Insignia) AccessibilityLabel() string { return i.Texto }

// AccessibilityRole implementa fyne.Accessible.
func (i *Insignia) AccessibilityRole() fyne.AccessibleRole { return fyne.AccessibleRoleText }

type insigniaRenderer struct {
	i       *Insignia
	caja    *Caja
	texto   *canvas.Text
	objetos []fyne.CanvasObject
}

func (r *insigniaRenderer) MinSize() fyne.Size { return MedidaInsignia(r.i.Texto) }

func (r *insigniaRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *insigniaRenderer) Destroy()                     {}

func (r *insigniaRenderer) Layout(t fyne.Size) {
	// El radio de píldora se recorta al medio alto, como hace `border-radius: 999px`.
	MoverCaja(r.caja, fyne.NewPos(0, 0), t)
	ancho := Medir(r.i.Texto, escalonInsignia, SemiNegrita).Width
	ColocarTexto(r.texto, escalonInsignia, (t.Width-ancho)/2, (t.Height-escalonInsignia.Linea)/2)
}

func (r *insigniaRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	relleno, tinta := r.i.colores(v)
	r.caja.Pintar(relleno, color.NRGBA{})
	r.texto.Text = r.i.Texto
	r.texto.Color = tinta
	for _, o := range r.objetos {
		o.Refresh()
	}
}

// ---- Medidor ----

// Umbrales de aviso de Meter.tsx, en fracción usada. Fijos y no configurables: dos
// productos que avisan a distinto porcentaje enseñan a desconfiar de los dos.
const (
	UmbralAviso = 0.8
	UmbralLleno = 0.95
)

// Medidas del medidor, en px de CSS.
const (
	// HuecoMedidor es el `gap-1.5` de la columna.
	HuecoMedidor float32 = 1.5 * 0.25 * tema.Rem
	// AltoBarra es el `h-1.5` de la barra.
	AltoBarra float32 = 1.5 * 0.25 * tema.Rem
)

// Medidor es el `Meter` de la suite: cuánto se lleva gastado de lo que hay.
//
// No formatea nada. `Porcentaje` y `Cuanto` llegan YA escritos por quien llama,
// porque el signo «%» y su separación los pone el idioma («95%» en español, «95 %» en
// francés) y el conector de la cantidad también. Igual que en la web.
type Medidor struct {
	widget.BaseWidget

	// Rotulo es el nombre visible («Almacenamiento»). Se trunca: «Speicherplatz» es
	// casi el doble de largo que «Espacio».
	Rotulo string
	// Fraccion es lo usado entre el tope, de 0 a 1.
	Fraccion float64
	// Porcentaje es esa fracción ya escrita en el idioma del usuario.
	Porcentaje string
	// Cuanto es el uso en palabras: «1,5 GB de 15 GB».
	Cuanto string
	// Nota es el aviso en PALABRAS («casi lleno», «lleno»), que es lo que hace que
	// el aviso llegue a quien no percibe el color. Vacía no pinta nada.
	Nota string
	// SobreCromo es el `onChrome`: el medidor va en el pie del menú de tinta y usa
	// la rampa `sidebar-*`.
	SobreCromo bool
}

var _ fyne.Widget = (*Medidor)(nil)

// NuevoMedidor crea el medidor.
func NuevoMedidor(rotulo string, fraccion float64, porcentaje string) *Medidor {
	m := &Medidor{Rotulo: rotulo, Fraccion: fraccion, Porcentaje: porcentaje}
	m.ExtendBaseWidget(m)
	return m
}

// Nivel es el tramo en el que cae el medidor.
type Nivel uint8

// Los tres niveles.
const (
	NivelOK Nivel = iota
	NivelAviso
	NivelLleno
)

// NivelDe es la cuenta de Meter.tsx.
func NivelDe(fraccion float64) Nivel {
	switch {
	case fraccion >= UmbralLleno:
		return NivelLleno
	case fraccion >= UmbralAviso:
		return NivelAviso
	}
	return NivelOK
}

// MedidaMedidor es el alto que ocupa: la fila de rótulo, la barra y —si hay— el pie,
// separados por `gap-1.5`.
func MedidaMedidor(conPie bool) float32 {
	alto := tema.TextoXS.Linea + HuecoMedidor + AltoBarra
	if conPie {
		alto += HuecoMedidor + tema.TextoXS.Linea
	}
	return alto
}

func (m *Medidor) conPie() bool { return m.Cuanto != "" || m.Nota != "" }

// PorCiento es el entero que la web usa para el ancho del relleno y para
// `aria-valuenow`. El relleno se pinta con el ENTERO, no con la fracción: una barra
// al 33,7 % mide 33 % en el navegador.
func (m *Medidor) PorCiento() int {
	f := min(max(m.Fraccion, 0), 1)
	return int(f*100 + 0.5)
}

func (m *Medidor) tokenRelleno() string {
	switch NivelDe(m.Fraccion) {
	case NivelLleno:
		return "danger"
	case NivelAviso:
		if m.SobreCromo {
			// `bg-on-chrome-warning`, que dentro del carril es su ámbar.
			return "sidebar-indicator"
		}
		return "warning"
	case NivelOK:
	}
	if m.SobreCromo {
		return "sidebar-fg-muted" // `bg-on-chrome-muted` dentro del carril
	}
	return "text-subtle"
}

// colorCanal es el del canal vacío: `bg-border`, y sobre el carril `bg-sidebar-fg/15`,
// porque el borde de la hoja es tinta oscura y sobre el color no se vería.
func (m *Medidor) colorCanal(v tema.Variante) color.NRGBA {
	if m.SobreCromo {
		return tokens.ConAlfa(tema.Color("sidebar-fg", v), 0.15)
	}
	return tema.Color("border", v)
}

func (m *Medidor) tokenAtenuado() string {
	if m.SobreCromo {
		return "sidebar-fg-muted"
	}
	return "text-muted"
}

// CreateRenderer compone el medidor.
func (m *Medidor) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	atenuado := tema.Color(m.tokenAtenuado(), v)
	r := &medidorRenderer{
		m:      m,
		rotulo: Texto(m.Rotulo, tema.TextoXS, Normal, atenuado),
		pct:    Texto(m.Porcentaje, tema.TextoXS, Normal, atenuado),
		canal:  canvas.NewRectangle(m.colorCanal(v)),
		lleno:  canvas.NewRectangle(tema.Color(m.tokenRelleno(), v)),
	}
	r.canal.CornerRadius = AltoBarra / 2
	r.lleno.CornerRadius = AltoBarra / 2
	r.objetos = []fyne.CanvasObject{r.rotulo, r.pct, r.canal, r.lleno}
	if m.conPie() {
		tinta := tema.Color("text", v)
		if m.SobreCromo {
			tinta = tema.Color("sidebar-fg", v)
		}
		r.cuanto = Texto(m.Cuanto, tema.TextoXS, Normal, tinta)
		r.nota = Texto(m.notaEscrita(), tema.TextoXS, Normal, atenuado)
		r.objetos = append(r.objetos, r.cuanto, r.nota)
	}
	return r
}

// notaEscrita es el ` · <nota>` que la web añade tras la cantidad.
func (m *Medidor) notaEscrita() string {
	if m.Nota == "" {
		return ""
	}
	if m.Cuanto == "" {
		return m.Nota
	}
	return " · " + m.Nota
}

// AccessibilityLabel es todo lo que se ve, junto: quien no ve la barra se entera del
// uso y del aviso sin recomponerlo de tres nodos.
func (m *Medidor) AccessibilityLabel() string {
	partes := []string{m.Rotulo, m.Cuanto, m.Porcentaje, m.Nota}
	s := ""
	for _, p := range partes {
		if p == "" {
			continue
		}
		if s != "" {
			s += " · "
		}
		s += p
	}
	return s
}

// AccessibilityRole es `progressbar`, que es el role de la web.
func (m *Medidor) AccessibilityRole() fyne.AccessibleRole {
	return fyne.AccessibleRoleText
}

type medidorRenderer struct {
	m            *Medidor
	rotulo, pct  *canvas.Text
	canal, lleno *canvas.Rectangle
	cuanto, nota *canvas.Text
	objetos      []fyne.CanvasObject
}

func (r *medidorRenderer) MinSize() fyne.Size {
	return fyne.NewSize(2*tema.Rem, MedidaMedidor(r.m.conPie()))
}

func (r *medidorRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *medidorRenderer) Destroy()                     {}

func (r *medidorRenderer) Layout(t fyne.Size) {
	m := r.m
	e := tema.TextoXS
	// `justify-between`: el porcentaje pegado a la derecha, el rótulo a la
	// izquierda y truncado con lo que le deje el hueco de `gap-2`.
	anchoPct := Medir(m.Porcentaje, e, Normal).Width
	const huecoFila = 2 * 0.25 * tema.Rem
	Truncar(r.rotulo, m.Rotulo, e, Normal, t.Width-anchoPct-huecoFila)
	ColocarTexto(r.rotulo, e, 0, 0)
	ColocarTexto(r.pct, e, t.Width-anchoPct, 0)

	y := e.Linea + HuecoMedidor
	r.canal.Resize(fyne.NewSize(t.Width, AltoBarra))
	r.canal.Move(fyne.NewPos(0, y))
	r.lleno.Resize(fyne.NewSize(t.Width*float32(m.PorCiento())/100, AltoBarra))
	r.lleno.Move(fyne.NewPos(0, y))

	if r.cuanto == nil {
		return
	}
	y += AltoBarra + HuecoMedidor
	ColocarTexto(r.cuanto, e, 0, y)
	ColocarTexto(r.nota, e, Medir(m.Cuanto, e, Normal).Width, y)
}

func (r *medidorRenderer) Refresh() {
	m := r.m
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	atenuado := tema.Color(m.tokenAtenuado(), v)
	r.rotulo.Color, r.pct.Color = atenuado, atenuado
	r.pct.Text = m.Porcentaje
	r.canal.FillColor = r.m.colorCanal(v)
	r.lleno.FillColor = tema.Color(m.tokenRelleno(), v)
	if r.cuanto != nil {
		r.cuanto.Text = m.Cuanto
		r.nota.Text = m.notaEscrita()
		r.nota.Color = atenuado
	}
	for _, o := range r.objetos {
		o.Refresh()
	}
}

// ---- Estado vacío ----

// Medidas del estado vacío, en px de CSS.
const (
	// HuecoVacio es `gap-3`, MargenXVacio `px-6` y MargenYVacio `py-8`.
	HuecoVacio   float32 = 3 * 0.25 * tema.Rem
	MargenXVacio float32 = 6 * 0.25 * tema.Rem
	MargenYVacio float32 = 8 * 0.25 * tema.Rem
	// SeparacionExtra es el `mb-2` de la ilustración y el `mt-2` de la acción, que
	// se SUMAN al `gap-3` de la columna: entre el dibujo y el título hay 17,5, no
	// 10,5.
	SeparacionExtra float32 = 2 * 0.25 * tema.Rem
	// AnchoDescripcion es el `max-w-sm` del párrafo: 24rem con el rem de 14.
	AnchoDescripcion float32 = 24 * tema.Rem
)

// El título es `.sytue-h4`: `--text-lg` en semi-negrita (15,75 / 22,75), con la escala
// y el peso de la forma (`EscalonTitulo(4)` y `PesoTitulo(4)`).

// Vacio es el `EmptyState`: marca, título, descripción y acción, centrados en los DOS
// ejes.
//
// En vertical también: la web es `flex flex-col items-center justify-center` y quien lo
// coloca le da `h-full`, así que el bloque queda en medio del hueco libre y no pegado
// arriba. Con el contenido más alto que la caja se apoya en el `py-8`, que es lo que
// hace `justify-center` cuando no sobra sitio.
//
// El icono va SUELTO, sin tesela de fondo: el rectángulo beige que llevaba antes no
// agrupaba, no separaba y no indicaba estado, y llamaba más la atención que el texto
// que sí dice qué hacer.
type Vacio struct {
	widget.BaseWidget

	Titulo      string
	Descripcion string
	// Ilustracion es opcional y se coloca centrada, con su propio tamaño acotado a
	// 200 de ancho como hace la web. Trae su color: un spot de `branding/illustrations`.
	Ilustracion fyne.CanvasObject
	// Glifo es un icono del catálogo en el sitio de la ilustración, y el color lo pone
	// el estado vacío: el `text-text-subtle` del `<div>` que la envuelve en EmptyState,
	// que el `<Icon>` hereda por `currentColor`. Gana a Ilustracion.
	Glifo *iconos.Dibujo
	// Accion es el botón principal, opcional.
	Accion fyne.CanvasObject
}

var _ fyne.Widget = (*Vacio)(nil)

// NuevoVacio crea el estado vacío.
func NuevoVacio(titulo, descripcion string) *Vacio {
	v := &Vacio{Titulo: titulo, Descripcion: descripcion}
	v.ExtendBaseWidget(v)
	return v
}

// ilustracion es lo que va arriba: el glifo que tiñe el estado vacío o la ilustración
// que trae su color.
func (v *Vacio) ilustracion() fyne.CanvasObject {
	if v.Glifo != nil {
		return v.Glifo.Objeto()
	}
	return v.Ilustracion
}

// MedidaVacio es el alto que ocupa a un ancho dado. Depende del ancho porque el título
// y la descripción se reparten en líneas.
func (v *Vacio) MedidaVacio(ancho float32) float32 {
	alto := 2 * MargenYVacio
	if il := v.ilustracion(); il != nil {
		alto += medidaIlustracion(il).Height + SeparacionExtra + HuecoVacio
	}
	alto += float32(len(v.lineasTitulo(ancho))) * EscalonTitulo(4).Linea
	if v.Descripcion != "" {
		alto += HuecoVacio + float32(len(v.lineas(ancho)))*tema.TextoBase.Linea
	}
	if v.Accion != nil {
		alto += HuecoVacio + SeparacionExtra + v.Accion.MinSize().Height
	}
	return alto
}

func (v *Vacio) lineas(ancho float32) []string {
	cabe := min(ancho-2*MargenXVacio, AnchoDescripcion)
	return Repartir(v.Descripcion, tema.TextoBase, Normal, cabe)
}

// lineasTitulo es el `<h3>` repartido en lo que deja el `px-6`: sin `max-w`, pasa de
// línea cuando no cabe en la columna, como cualquier título en la web. Hasta el
// 2026-09-23 iba en una línea y su ancho mandaba en el `MinSize`, así que un título largo
// ensanchaba el hueco en vez de partirse.
func (v *Vacio) lineasTitulo(ancho float32) []string {
	if ancho <= 0 {
		return []string{v.Titulo}
	}
	if l := Repartir(v.Titulo, EscalonTitulo(4), PesoTitulo(4), ancho-2*MargenXVacio); len(l) > 0 {
		return l
	}
	return []string{v.Titulo}
}

// medidaIlustracion es lo que ocupa la ilustración: lo mayor entre su mínimo y el
// tamaño que ya tiene.
//
// El segundo término NO es una precaución. La ilustración de todos los estados de la
// suite es un `iconos.Dibujo`, cuya primitiva es un `canvas.Raster`, y el mínimo de un
// ráster es **1×1**: lo lleva el tamaño que le den, que en un Dibujo se fija al
// construirlo. Con `Resize(MinSize())` el icono de 40 px de «sin conexión» y de «no se
// pudo cargar» se quedaba en UN PÍXEL —en ninguna app se veía, ni aquí ni en Sytue
// Drive, que lo usa igual—. Se vio mirando la captura, no fallando un test.
func medidaIlustracion(o fyne.CanvasObject) fyne.Size {
	m, t := o.MinSize(), o.Size()
	return fyne.NewSize(max(m.Width, t.Width), max(m.Height, t.Height))
}

// CreateRenderer compone el estado vacío.
func (v *Vacio) CreateRenderer() fyne.WidgetRenderer {
	t := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &vacioRenderer{v: v}
	// El título existe desde el principio, en una línea: se reparte en Layout, pero quien
	// lea el estado vacío antes de colocarlo —un guardián, un lector de pantalla— tiene
	// que encontrarlo.
	r.rehacerTitulo(0)
	if il := v.ilustracion(); il != nil {
		r.objetos = append(r.objetos, il)
	}
	if v.Glifo != nil {
		v.Glifo.Tenir("text-subtle", t)
	}
	if v.Accion != nil {
		r.objetos = append(r.objetos, v.Accion)
	}
	return r
}

// AccessibilityLabel junta título y descripción.
func (v *Vacio) AccessibilityLabel() string {
	if v.Descripcion == "" {
		return v.Titulo
	}
	return v.Titulo + ". " + v.Descripcion
}

// AccessibilityRole implementa fyne.Accessible.
func (v *Vacio) AccessibilityRole() fyne.AccessibleRole { return fyne.AccessibleRoleText }

type vacioRenderer struct {
	v       *Vacio
	titulo  []*canvas.Text // una por línea del `<h3>`
	parrafo []*canvas.Text
	ultimo  fyne.Size // lo último que se colocó, para recolocar al cambiar de piezas
	objetos []fyne.CanvasObject
}

// rehacerPiezas mete en la lista la ilustración y la acción que el estado vacío tenga
// AHORA.
//
// Se rehace y no se monta una vez porque las dos se ponen después de construir el
// widget —una pantalla crea el vacío y luego le cuelga su botón—, y un objeto que no
// está en `Objects()` no se pinta ni se coloca: el botón aparecía en la esquina
// superior izquierda de la pantalla.
func (r *vacioRenderer) rehacerPiezas() bool {
	v := r.v
	tiene := func(o fyne.CanvasObject) bool {
		for _, x := range r.objetos {
			if x == o {
				return true
			}
		}
		return false
	}
	cambia := false
	for _, o := range []fyne.CanvasObject{v.ilustracion(), v.Accion} {
		if o != nil && !tiene(o) {
			r.objetos = append(r.objetos, o)
			cambia = true
		}
	}
	// Y al revés: lo que ya no está colgado del widget sale de la lista.
	vivos := r.objetos[:0]
	for _, o := range r.objetos {
		if (o == v.ilustracion() || o == v.Accion) && o != nil {
			vivos = append(vivos, o)
			continue
		}
		if esDe(o, r.titulo) || esDe(o, r.parrafo) {
			vivos = append(vivos, o)
			continue
		}
		cambia = true
	}
	r.objetos = vivos
	return cambia
}

// MinSize es el `min-content` de la columna: lo que ocupa la palabra más larga del título
// —un `<h3>` que se reparte no puede ser más estrecho—, con el `px-6`. De alto, el que da
// el ancho que ya tenga (o, sin colocar, el del título en una línea).
func (r *vacioRenderer) MinSize() fyne.Size {
	ancho := float32(0)
	for _, p := range strings.Fields(r.v.Titulo) {
		ancho = max(ancho, Medir(p, EscalonTitulo(4), PesoTitulo(4)).Width)
	}
	ancho += 2 * MargenXVacio
	ref := r.v.Size().Width
	if ref <= 0 {
		ref = Medir(r.v.Titulo, EscalonTitulo(4), PesoTitulo(4)).Width + 2*MargenXVacio
	}
	return fyne.NewSize(ancho, r.v.MedidaVacio(max(ref, ancho)))
}

func (r *vacioRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *vacioRenderer) Destroy()                     {}

func (r *vacioRenderer) Layout(t fyne.Size) {
	v := r.v
	r.ultimo = t
	// Título y descripción se reparten en líneas cada vez que cambia el ancho, así que
	// los objetos de texto se reconstruyen aquí y no en CreateRenderer.
	r.rehacerTitulo(t.Width)
	r.rehacerParrafo(t.Width)
	r.rehacerPiezas()

	// `justify-center`: el bloque va centrado en el alto disponible, con el `py-8` de
	// suelo cuando no cabe. `MedidaVacio` ya incluye los dos márgenes.
	y := max((t.Height-(r.v.MedidaVacio(t.Width)-2*MargenYVacio))/2, MargenYVacio)
	if il := v.ilustracion(); il != nil {
		m := medidaIlustracion(il)
		m.Width = min(m.Width, 200)
		il.Resize(m)
		il.Move(fyne.NewPos((t.Width-m.Width)/2, y))
		y += m.Height + SeparacionExtra + HuecoVacio
	}
	for _, l := range r.titulo { // `text-center`: cada línea en el centro
		a := Medir(l.Text, EscalonTitulo(4), PesoTitulo(4)).Width
		ColocarTexto(l, EscalonTitulo(4), (t.Width-a)/2, y)
		y += EscalonTitulo(4).Linea
	}

	if len(r.parrafo) > 0 {
		y += HuecoVacio
		for _, l := range r.parrafo {
			a := Medir(l.Text, tema.TextoBase, Normal).Width
			ColocarTexto(l, tema.TextoBase, (t.Width-a)/2, y)
			y += tema.TextoBase.Linea
		}
	}
	if v.Accion != nil {
		y += HuecoVacio + SeparacionExtra
		m := v.Accion.MinSize()
		v.Accion.Resize(m)
		v.Accion.Move(fyne.NewPos((t.Width-m.Width)/2, y))
	}
}

func (r *vacioRenderer) rehacerParrafo(ancho float32) {
	if r.v.Descripcion == "" {
		return
	}
	lineas := r.v.lineas(ancho)
	if len(lineas) == len(r.parrafo) {
		for i, l := range lineas {
			r.parrafo[i].Text = l
		}
		return
	}
	t := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	// Se quitan los viejos del árbol y se ponen los nuevos.
	vivos := r.objetos[:0]
	for _, o := range r.objetos {
		if !esDe(o, r.parrafo) {
			vivos = append(vivos, o)
		}
	}
	r.parrafo = nil
	for _, l := range lineas {
		c := Texto(l, tema.TextoBase, Normal, tema.Color("text-muted", t))
		r.parrafo = append(r.parrafo, c)
		vivos = append(vivos, c)
	}
	r.objetos = vivos
}

// rehacerTitulo reparte el título en las líneas que le toquen a este ancho, reutilizando
// los textos si el número de líneas no cambia.
func (r *vacioRenderer) rehacerTitulo(ancho float32) {
	lineas := r.v.lineasTitulo(ancho)
	if len(lineas) == len(r.titulo) {
		for i, l := range lineas {
			r.titulo[i].Text = l
		}
		return
	}
	t := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	vivos := r.objetos[:0]
	for _, o := range r.objetos {
		if !esDe(o, r.titulo) {
			vivos = append(vivos, o)
		}
	}
	r.titulo = nil
	for _, l := range lineas {
		c := Texto(l, EscalonTitulo(4), PesoTitulo(4), tema.Color("text", t))
		r.titulo = append(r.titulo, c)
		vivos = append(vivos, c)
	}
	r.objetos = vivos
}

func esDe(o fyne.CanvasObject, lista []*canvas.Text) bool {
	for _, x := range lista {
		if fyne.CanvasObject(x) == o {
			return true
		}
	}
	return false
}

func (r *vacioRenderer) Refresh() {
	t := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	if r.ultimo.Width > 0 {
		r.rehacerTitulo(r.ultimo.Width) // un título que cambia vuelve a repartirse
	} else {
		r.rehacerTitulo(0)
	}
	for _, l := range r.titulo {
		l.Color = tema.Color("text", t)
	}
	for _, l := range r.parrafo {
		l.Color = tema.Color("text-muted", t)
	}
	if r.v.Glifo != nil {
		r.v.Glifo.Tenir("text-subtle", t)
	}
	// Una ilustración o una acción puestas después de montar el widget entran aquí, y
	// hay que volver a colocar: `Layout` solo corre al redimensionar.
	if r.rehacerPiezas() && r.ultimo.Width > 0 {
		r.Layout(r.ultimo)
	}
	for _, o := range r.objetos {
		o.Refresh()
	}
}
