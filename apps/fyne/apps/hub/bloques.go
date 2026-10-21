// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package hub

import (
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// Los bloques de las pantallas de asistente del Hub: una columna de piezas cuyo alto
// depende del ancho —párrafos que se reparten, filas que se parten en el móvil—.
//
// Existen porque «Conecta tus dispositivos» tiene siete juegos de instrucciones, cada
// uno una columna distinta de las mismas piezas, y escribir siete `Layout` a mano es
// escribir siete veces el mismo error. `fyne.Layout` no sirve: su `MinSize` no recibe
// el ancho, y aquí el alto sale de él.
type bloque interface {
	alto(ancho float32) float32
	// poner coloca el bloque con su esquina en (x, y) y ese ancho.
	poner(x, y, ancho float32)
	objetos() []fyne.CanvasObject
}

// columna apila bloques con un hueco (`flex flex-col gap-N`).
type columna struct {
	hueco float32
	hijos []bloque
}

func col(hueco float32, hijos ...bloque) *columna { return &columna{hueco: hueco, hijos: hijos} }

func (c *columna) alto(ancho float32) float32 {
	var h float32
	for i, b := range c.hijos {
		if i > 0 {
			h += c.hueco
		}
		h += b.alto(ancho)
	}
	return h
}

func (c *columna) poner(x, y, ancho float32) {
	for i, b := range c.hijos {
		if i > 0 {
			y += c.hueco
		}
		b.poner(x, y, ancho)
		y += b.alto(ancho)
	}
}

func (c *columna) objetos() []fyne.CanvasObject {
	var out []fyne.CanvasObject
	for _, b := range c.hijos {
		out = append(out, b.objetos()...)
	}
	return out
}

// parrafo es un `<p>` a todo el ancho.
type parrafo struct{ p *ui.Parrafo }

func (b parrafo) alto(ancho float32) float32   { return ui.AltoParrafo(b.p, ancho) }
func (b parrafo) poner(x, y, ancho float32)    { ponerParrafo(b.p, x, y, ancho, true) }
func (b parrafo) objetos() []fyne.CanvasObject { return []fyne.CanvasObject{b.p} }

// texto es un párrafo de apoyo: letra normal en `text-muted`, que es como van todos los
// de estas pantallas.
func texto(t string, e tema.Escalon) parrafo {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	return parrafo{ui.NuevoParrafo(t, e, ui.Normal, "text-muted", v)}
}

// fijo es un objeto a su medida y pegado a la izquierda (`self-start`): un botón o un
// enlace no se estiran.
type fijo struct{ o fyne.CanvasObject }

func (b fijo) alto(float32) float32         { return b.o.MinSize().Height }
func (b fijo) poner(x, y, _ float32)        { ponerObjeto(b.o, x, y, b.o.MinSize(), true) }
func (b fijo) objetos() []fyne.CanvasObject { return []fyne.CanvasObject{b.o} }

// ayuda es un `InlineHelp` a todo el ancho.
type ayuda struct{ a *ui.AyudaEnLinea }

func (b ayuda) alto(ancho float32) float32 { return ui.AltoAyudaEnLinea(b.a, ancho) }
func (b ayuda) poner(x, y, ancho float32) {
	ponerObjeto(b.a, x, y, ui.MedidaAyudaEnLinea(b.a, ancho), true)
}
func (b ayuda) objetos() []fyne.CanvasObject { return []fyne.CanvasObject{b.a} }

func nuevaAyuda(tono ui.TonoAyuda, titulo, cuerpo string) ayuda {
	return ayuda{ui.NuevaAyudaEnLinea(cuerpo).ConTono(tono).ConTitulo(titulo)}
}

// tope acota el ancho de lo de dentro (`max-w-*`).
type tope struct {
	max float32
	b   bloque
}

func (t tope) alto(ancho float32) float32   { return t.b.alto(min(ancho, t.max)) }
func (t tope) poner(x, y, ancho float32)    { t.b.poner(x, y, min(ancho, t.max)) }
func (t tope) objetos() []fyne.CanvasObject { return t.b.objetos() }

// ---- La sección de ajustes, con descripción ----

// bSeccion es una `SettingsSection` dentro del flujo: la tarjeta, el `h2`, la
// descripción opcional (`mt-1 text-sm leading-relaxed`, `max-w-2xl`) y lo de dentro.
type bSeccion struct {
	s      *seccion
	desc   *ui.Parrafo
	cuerpo bloque
}

func nuevaBSeccion(titulo, descripcion string, cuerpo bloque) *bSeccion {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	b := &bSeccion{s: nuevaSeccion(v, titulo), cuerpo: cuerpo}
	if descripcion != "" {
		b.desc = ui.NuevoParrafo(descripcion, smRelajada, ui.Normal, "text-muted", v)
	}
	return b
}

// MargenDescSeccion es el `mt-1` de la descripción de una sección.
const MargenDescSeccion float32 = 1 * 0.25 * tema.Rem

func (b *bSeccion) altoDesc(ancho float32) float32 {
	if b.desc == nil {
		return 0
	}
	return MargenDescSeccion + ui.AltoParrafo(b.desc, min(InteriorSeccion(ancho), AnchoIntro))
}

func (b *bSeccion) alto(ancho float32) float32 {
	return b.s.alto(ancho, b.altoDesc(ancho)+b.cuerpo.alto(InteriorSeccion(ancho)))
}

func (b *bSeccion) poner(x, y, ancho float32) {
	cuerpo := b.altoDesc(ancho) + b.cuerpo.alto(InteriorSeccion(ancho))
	// El título ocupa su sitio y la descripción va pegada a él: el `gap-4` de la
	// tarjeta va DESPUÉS de la cabecera entera, no entre título y descripción.
	cx, cy := b.s.colocar(x, y, ancho, cuerpo)
	if b.desc != nil {
		cy -= HuecoSeccion
		cy = ponerParrafo(b.desc, cx, cy+MargenDescSeccion, min(InteriorSeccion(ancho), AnchoIntro), true) + HuecoSeccion
	}
	b.cuerpo.poner(cx, cy, InteriorSeccion(ancho))
}

func (b *bSeccion) objetos() []fyne.CanvasObject {
	out := b.s.objetos()
	if b.desc != nil {
		out = append(out, b.desc)
	}
	return append(out, b.cuerpo.objetos()...)
}

// ---- El paso numerado de unas instrucciones ----

// pasoNumerado es el `Step` de `ConnectDevices.tsx`: `flex gap-3`, un círculo de 21
// en `bg-bg-subtle` con el número (`text-xs font-semibold text-text-muted`), el título
// (`text-sm font-medium`) y el cuerpo (`mt-0.5 text-sm leading-snug text-text-muted`).
type pasoNumerado struct {
	circulo *canvas.Circle
	numero  *canvas.Text
	titulo  *ui.Parrafo
	cuerpo  *ui.Parrafo
}

// Medidas del paso numerado.
const (
	HuecoPasoNumerado float32 = 3 * 0.25 * tema.Rem
	MargenCuerpoPaso  float32 = 0.5 * 0.25 * tema.Rem
)

// smAjustada es el `text-sm leading-snug`: 1,375 del tamaño.
var smAjustada = tema.Escalon{Tam: tema.TextoSM.Tam, Linea: 1.375 * tema.TextoSM.Tam}

func nuevoPaso(n int, titulo, cuerpo string) *pasoNumerado {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	return &pasoNumerado{
		circulo: canvas.NewCircle(tema.Color("bg-subtle", v)),
		numero:  ui.Texto(fmt.Sprint(n), tema.TextoXS, ui.SemiNegrita, tema.Color("text-muted", v)),
		titulo:  ui.NuevoParrafo(titulo, tema.TextoSM, ui.Medio, "text", v),
		cuerpo:  ui.NuevoParrafo(cuerpo, smAjustada, ui.Normal, "text-muted", v),
	}
}

func (p *pasoNumerado) textos(ancho float32) float32 {
	return max(ancho-ui.LadoCirculoPaso-HuecoPasoNumerado, 0)
}

func (p *pasoNumerado) alto(ancho float32) float32 {
	w := p.textos(ancho)
	return max(ui.LadoCirculoPaso, ui.AltoParrafo(p.titulo, w)+MargenCuerpoPaso+ui.AltoParrafo(p.cuerpo, w))
}

func (p *pasoNumerado) poner(x, y, ancho float32) {
	p.circulo.Move(fyne.NewPos(x, y))
	p.circulo.Resize(fyne.NewSize(ui.LadoCirculoPaso, ui.LadoCirculoPaso))
	w := ui.Medir(p.numero.Text, tema.TextoXS, ui.SemiNegrita).Width
	ui.ColocarTexto(p.numero, tema.TextoXS, x+(ui.LadoCirculoPaso-w)/2, y+(ui.LadoCirculoPaso-tema.TextoXS.Linea)/2)
	tx, tw := x+ui.LadoCirculoPaso+HuecoPasoNumerado, p.textos(ancho)
	ty := ponerParrafo(p.titulo, tx, y, tw, true) + MargenCuerpoPaso
	ponerParrafo(p.cuerpo, tx, ty, tw, true)
}

func (p *pasoNumerado) objetos() []fyne.CanvasObject {
	return []fyne.CanvasObject{p.circulo, p.numero, p.titulo, p.cuerpo}
}

// ---- El resumen del último paso ----

// resumen es el `ReviewSummary` de `@sytue/ui`: una lista clave → valor con borde,
// `divide-y` en `border-subtle`, filas `px-3 py-2.5 gap-3` y, desde `sm:`, la clave en
// una columna de `minmax(0, 11rem)`. Sin «Editar»: el asistente de dispositivos no lo
// usa.
type resumen struct {
	caja    *ui.Caja
	filetes []*canvas.Rectangle
	claves  []*ui.Parrafo
	valores []*ui.Parrafo
	ventana float32
	// derecha es lo que se reserva a la derecha del valor: la columna `auto` del
	// «Editar» del alta, con su `gap-3`. Cero sin ella.
	derecha float32
}

// Medidas del `ReviewSummary`.
const (
	MargenResumenX float32 = 3 * 0.25 * tema.Rem
	MargenResumenY float32 = 2.5 * 0.25 * tema.Rem
	HuecoResumen   float32 = 3 * 0.25 * tema.Rem
	ClaveResumen   float32 = 11 * tema.Rem
)

func nuevoResumen(ventana float32, filas [][2]string) *resumen {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &resumen{
		caja:    ui.NuevaCaja(tema.Color("surface", v), tema.Color("border", v), tema.RadioMD),
		ventana: ventana,
	}
	for i, f := range filas {
		if i > 0 {
			r.filetes = append(r.filetes, canvas.NewRectangle(tema.Color("border-subtle", v)))
		}
		r.claves = append(r.claves, ui.NuevoParrafo(f[0], tema.TextoSM, ui.Normal, "text-muted", v))
		r.valores = append(r.valores, ui.NuevoParrafo(f[1], tema.TextoSM, ui.Normal, "text", v))
	}
	return r
}

// columnas son el ancho de la clave y el del valor en una fila de ese ancho interior.
// Desde `sm:` es la rejilla de `minmax(0, 11rem) 1fr`; por debajo, un `flex` en el que
// la clave (`shrink-0`) mide su texto y el valor se queda con el resto.
func (r *resumen) columnas(i int, interior float32) (clave, valor float32) {
	if r.ventana == 0 || r.ventana >= ui.CorteSM {
		clave = min(ClaveResumen, interior)
	} else {
		clave = min(ui.Medir(r.claves[i].Texto, tema.TextoSM, ui.Normal).Width, interior)
	}
	return clave, max(interior-clave-HuecoResumen-r.derecha, 0)
}

func (r *resumen) altoFila(i int, ancho float32) float32 {
	clave, valor := r.columnas(i, ancho-2*(tema.Borde+MargenResumenX))
	return 2*MargenResumenY + max(ui.AltoParrafo(r.claves[i], clave), ui.AltoParrafo(r.valores[i], valor))
}

func (r *resumen) alto(ancho float32) float32 {
	h := 2 * tema.Borde
	for i := range r.claves {
		if i > 0 {
			h += tema.Borde
		}
		h += r.altoFila(i, ancho)
	}
	return h
}

func (r *resumen) poner(x, y, ancho float32) {
	ui.MoverCaja(r.caja, fyne.NewPos(x, y), fyne.NewSize(ancho, r.alto(ancho)))
	interior := ancho - 2*(tema.Borde+MargenResumenX)
	fy := y + tema.Borde
	for i := range r.claves {
		if i > 0 {
			ColocarDivisor(r.filetes[i-1], x+tema.Borde, fy, ancho-2*tema.Borde)
			fy += tema.Borde
		}
		clave, valor := r.columnas(i, interior)
		cx := x + tema.Borde + MargenResumenX
		ponerParrafo(r.claves[i], cx, fy+MargenResumenY, clave, true)
		ponerParrafo(r.valores[i], cx+clave+HuecoResumen, fy+MargenResumenY, valor, true)
		fy += r.altoFila(i, ancho)
	}
}

func (r *resumen) objetos() []fyne.CanvasObject {
	out := r.caja.Objetos()
	for _, f := range r.filetes {
		out = append(out, f)
	}
	for i := range r.claves {
		out = append(out, r.claves[i], r.valores[i])
	}
	return out
}

// ---- Lo plegado ----

// Detalles es el `<details>` de `ConnectDevices.tsx`: `border rounded-md bg-bg-subtle`,
// el resumen en `px-3 py-2 text-sm font-medium` y lo de dentro en `px-3 pt-1 pb-3`. Se
// abre y se cierra con el ratón, Intro o Espacio, como el del navegador.
//
// ponytail: el navegador pinta su triángulo de `list-item` delante del resumen; aquí va
// el chevron de la suite al mismo tamaño que el texto. Se cambia si alguien mide la
// diferencia y le importa.
type Detalles struct {
	ui.Pulsable
	Resumen string
	Abierto bool
	// AlCambiar avisa de que cambia de alto: la pantalla tiene que volver a colocarse.
	AlCambiar func()

	contenido bloque
}

var _ fyne.Widget = (*Detalles)(nil)

// Medidas del `<details>`.
const (
	MargenDetallesX   float32 = 3 * 0.25 * tema.Rem
	MargenDetallesY   float32 = 2 * 0.25 * tema.Rem
	MargenDentroArr   float32 = 1 * 0.25 * tema.Rem
	MargenDentroAbajo float32 = 3 * 0.25 * tema.Rem
	// HuecoMarcador es el espacio entre el marcador y el resumen.
	HuecoMarcador float32 = 0.25 * tema.Rem
)

func nuevosDetalles(resumen string, contenido bloque) *Detalles {
	d := &Detalles{Resumen: resumen, contenido: contenido}
	d.Nombre = resumen
	d.AlPulsar = func() {
		d.Abierto = !d.Abierto
		d.Refresh()
		if d.AlCambiar != nil {
			d.AlCambiar()
		}
	}
	d.ExtendBaseWidget(d)
	return d
}

// altoResumen es la franja que se pulsa: `py-2` y una línea de `text-sm`.
func altoResumen() float32 { return 2*MargenDetallesY + tema.TextoSM.Linea }

func (d *Detalles) alto(ancho float32) float32 {
	h := 2*tema.Borde + altoResumen()
	if d.Abierto {
		h += MargenDentroArr + d.contenido.alto(ancho-2*(tema.Borde+MargenDetallesX)) + MargenDentroAbajo
	}
	return h
}

func (d *Detalles) poner(x, y, ancho float32) {
	ponerObjeto(d, x, y, fyne.NewSize(ancho, d.alto(ancho)), true)
	if d.Abierto {
		d.contenido.poner(x+tema.Borde+MargenDetallesX, y+tema.Borde+altoResumen()+MargenDentroArr,
			ancho-2*(tema.Borde+MargenDetallesX))
	}
}

// objetos son la caja con su resumen y, si está abierto, lo de dentro, que va aparte
// para que sus botones se puedan pulsar.
func (d *Detalles) objetos() []fyne.CanvasObject {
	out := []fyne.CanvasObject{d}
	if d.Abierto {
		out = append(out, d.contenido.objetos()...)
	}
	return out
}

// CreateRenderer compone la caja, el marcador y el resumen.
func (d *Detalles) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &detallesRenderer{
		d:       d,
		aro:     ui.NuevoAro(tema.RadioMD, ui.ConHueco, v),
		caja:    ui.NuevaCaja(tema.Color("bg-subtle", v), tema.Color("border", v), tema.RadioMD),
		cerrado: iconos.Trazo(iconos.ChevronDerecha, tema.TextoSM.Tam, "text", v),
		abierto: iconos.Trazo(iconos.ChevronAbajo, tema.TextoSM.Tam, "text", v),
		texto:   ui.Texto(d.Resumen, tema.TextoSM, ui.Medio, tema.Color("text", v)),
	}
	r.objetos = append(r.aro.Objetos(), r.caja.Objetos()...)
	r.objetos = append(r.objetos, r.cerrado.Objeto(), r.abierto.Objeto(), r.texto)
	r.Refresh()
	return r
}

type detallesRenderer struct {
	d                *Detalles
	aro              *ui.Aro
	caja             *ui.Caja
	cerrado, abierto *iconos.Dibujo
	texto            *canvas.Text
	objetos          []fyne.CanvasObject
}

func (r *detallesRenderer) MinSize() fyne.Size           { return fyne.NewSize(0, altoResumen()+2*tema.Borde) }
func (r *detallesRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *detallesRenderer) Destroy()                     {}

func (r *detallesRenderer) Layout(t fyne.Size) {
	r.caja.Redimensionar(t)
	// El anillo va alrededor del resumen, que es lo que recibe el foco en la web.
	r.aro.Colocar(fyne.NewSize(t.Width, altoResumen()+2*tema.Borde))
	x, y := tema.Borde+MargenDetallesX, tema.Borde+MargenDetallesY
	lado := tema.TextoSM.Tam
	r.cerrado.Colocar(x, y+(tema.TextoSM.Linea-lado)/2, lado)
	r.abierto.Colocar(x, y+(tema.TextoSM.Linea-lado)/2, lado)
	ui.ColocarTexto(r.texto, tema.TextoSM, x+lado+HuecoMarcador, y)
}

func (r *detallesRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r.cerrado.Ver(!r.d.Abierto)
	r.abierto.Ver(r.d.Abierto)
	r.texto.Text = r.d.Resumen
	r.texto.Refresh()
	r.aro.Pintar(v)
	r.aro.Ver(r.d.Enfocado())
	r.Layout(r.d.Size())
}

// ---- La fila con botón de copiar ----

// FilaCopia es el `CopyRow` de `ConnectDevices.tsx`: el rótulo (`sytue-meta`, 182 de
// ancho desde `sm:` y la línea entera por debajo), el valor en monoespaciada
// (`font-mono text-xs break-all`) y «Copiar» (`Button ghost sm`, inerte sin valor), en
// una caja `border rounded-md bg-surface px-3 py-2` con `gap-2`.
type FilaCopia struct {
	widget.BaseWidget
	Rotulo, Valor string
	// Ventana decide el `sm:`; cero es ancha.
	Ventana float32
	// RotuloSM es el ancho del rótulo desde `sm:`; cero, el de `ConnectDevices.tsx`
	// (`sm:w-44`). El alta guiada lo lleva a `sm:w-24`.
	RotuloSM float32

	boton *ui.Boton
}

var _ fyne.Widget = (*FilaCopia)(nil)

// Medidas del `CopyRow`.
const (
	MargenCopiaX  float32 = 3 * 0.25 * tema.Rem
	MargenCopiaY  float32 = 2 * 0.25 * tema.Rem
	HuecoCopia    float32 = 2 * 0.25 * tema.Rem
	RotuloCopiaSM float32 = 52 * 0.25 * tema.Rem
)

func nuevaFilaCopia(t *i18n.Translator, rotulo, valor string, copiar func(string)) *FilaCopia {
	f := &FilaCopia{Rotulo: rotulo, Valor: valor}
	f.boton = ui.NuevoBoton(t.T("devices.copy", nil), func() { copiar(f.Valor) }).Con(ui.Fantasma, ui.TallaSM)
	f.boton.Inerte = valor == ""
	f.ExtendBaseWidget(f)
	return f
}

func (f *FilaCopia) ancha() bool { return f.Ventana == 0 || f.Ventana >= ui.CorteSM }

// valorVisible es lo que se escribe: la web pone una raya si no hay valor.
func (f *FilaCopia) valorVisible() string {
	if f.Valor == "" {
		return "—"
	}
	return f.Valor
}

// geometria reparte la fila de ese ancho: el ancho del rótulo, el del valor y dónde
// empieza la línea del valor (0 si va en la misma que el rótulo).
func (f *FilaCopia) geometria(ancho float32) (rotulo, valor, bajada float32) {
	interior := max(ancho-2*(tema.Borde+MargenCopiaX), 0)
	boton := f.boton.MinSize().Width
	if f.ancha() {
		rotulo = RotuloCopiaSM
		if f.RotuloSM > 0 {
			rotulo = f.RotuloSM
		}
		rotulo = min(rotulo, interior)
		return rotulo, max(interior-rotulo-boton-2*HuecoCopia, 0), 0
	}
	// Por debajo de `sm:` el rótulo es `w-full`: ocupa su línea y el resto baja.
	rotulo = interior
	return rotulo, max(interior-boton-HuecoCopia, 0), float32(len(ui.Repartir(f.Rotulo, tema.TextoXS, ui.Normal, rotulo)))*tema.TextoXS.Linea + HuecoCopia
}

// lineasValor parte el valor en cualquier carácter (`break-all`).
func (f *FilaCopia) lineasValor(ancho float32) []string {
	return partirMono(f.valorVisible(), tema.TextoXS.Tam, ancho)
}

func (f *FilaCopia) alto(ancho float32) float32 {
	rotulo, valor, bajada := f.geometria(ancho)
	altoRotulo := float32(len(ui.Repartir(f.Rotulo, tema.TextoXS, ui.Normal, rotulo))) * tema.TextoXS.Linea
	fila := max(float32(len(f.lineasValor(valor)))*tema.TextoXS.Linea, f.boton.MinSize().Height)
	if bajada == 0 {
		fila = max(fila, altoRotulo)
	}
	return 2*(tema.Borde+MargenCopiaY) + bajada + fila
}

func (f *FilaCopia) poner(x, y, ancho float32) {
	ponerObjeto(f, x, y, fyne.NewSize(ancho, f.alto(ancho)), true)
}

func (f *FilaCopia) objetos() []fyne.CanvasObject { return []fyne.CanvasObject{f} }

// partirMono parte un texto monoespaciado donde se acabe el ancho, carácter a
// carácter, que es lo que hace `break-all` con una URL.
func partirMono(s string, tam, ancho float32) []string {
	var lineas []string
	var actual []rune
	for _, c := range s {
		prueba := string(append(actual, c))
		if len(actual) > 0 && medirMono(prueba, tam) > ancho {
			lineas = append(lineas, string(actual))
			actual = actual[:0]
		}
		actual = append(actual, c)
	}
	return append(lineas, string(actual))
}

func medirMono(s string, tam float32) float32 {
	return fyne.MeasureText(s, tam, fyne.TextStyle{Monospace: true}).Width
}

// CreateRenderer compone la caja, el rótulo, el valor y el botón.
func (f *FilaCopia) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &filaCopiaRenderer{
		f:      f,
		caja:   ui.NuevaCaja(tema.Color("surface", v), tema.Color("border", v), tema.RadioMD),
		rotulo: ui.NuevoParrafo(f.Rotulo, tema.TextoXS, ui.Normal, "text-subtle", v),
	}
	r.Refresh()
	return r
}

type filaCopiaRenderer struct {
	f       *FilaCopia
	caja    *ui.Caja
	rotulo  *ui.Parrafo
	valor   []*canvas.Text
	objetos []fyne.CanvasObject
}

func (r *filaCopiaRenderer) MinSize() fyne.Size           { return fyne.NewSize(0, r.f.alto(r.f.Size().Width)) }
func (r *filaCopiaRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *filaCopiaRenderer) Destroy()                     {}

func (r *filaCopiaRenderer) Layout(t fyne.Size) {
	f := r.f
	r.caja.Redimensionar(t)
	anchoRotulo, anchoValor, bajada := f.geometria(t.Width)
	lineas := f.lineasValor(anchoValor)
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	if len(lineas) != len(r.valor) {
		r.valor = nil
		for range lineas {
			// La cara de Figtree que pone `ui.Texto` gana al estilo: sin quitarla, el
			// valor sale en Figtree aunque diga monoespaciada.
			c := ui.Texto("", tema.TextoXS, ui.Normal, tema.Color("text", v))
			c.FontSource, c.TextStyle.Monospace = nil, true
			r.valor = append(r.valor, c)
		}
		r.objetos = append(r.caja.Objetos(), r.rotulo, f.boton)
		for _, c := range r.valor {
			r.objetos = append(r.objetos, c)
		}
	}
	x := tema.Borde + MargenCopiaX
	arriba := tema.Borde + MargenCopiaY
	altoRotulo := ui.AltoParrafo(r.rotulo, anchoRotulo)
	altoValor := float32(len(lineas)) * tema.TextoXS.Linea
	boton := f.boton.MinSize()
	fila := max(altoValor, boton.Height)
	if bajada == 0 {
		fila = max(fila, altoRotulo)
		ponerParrafo(r.rotulo, x, arriba+(fila-altoRotulo)/2, anchoRotulo, true)
	} else {
		ponerParrafo(r.rotulo, x, arriba, anchoRotulo, true)
	}
	vy := arriba + bajada + (fila-altoValor)/2
	vx := x
	if bajada == 0 {
		vx += anchoRotulo + HuecoCopia
	}
	for i, c := range r.valor {
		c.Text = lineas[i]
		c.TextSize = tema.TextoXS.Tam
		c.Refresh()
		ui.ColocarTexto(c, tema.TextoXS, vx, vy+float32(i)*tema.TextoXS.Linea)
	}
	ponerObjeto(f.boton, vx+anchoValor+HuecoCopia, arriba+bajada+(fila-boton.Height)/2, boton, true)
}

func (r *filaCopiaRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r.caja.Pintar(tema.Color("surface", v), tema.Color("border", v))
	r.rotulo.Texto = r.f.Rotulo
	r.rotulo.Refresh()
	r.valor = nil // fuerza el reparto: puede haber cambiado el valor
	r.Layout(r.f.Size())
}
