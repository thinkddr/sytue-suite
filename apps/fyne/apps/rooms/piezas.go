// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package rooms

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
	"git.sytue.eu/sytue/one/apps/nucleo/tokens"
)

// Las dos marcas de Rooms que son TEXTO y no un glifo, centradas en su caja.
//
// No son iconos y no se dibujan como iconos: en la web son un carácter dentro de un
// `grid place-items-center`, y por eso están aquí y no en `iconos`. Dibujar una
// almohadilla a mano sería otro dibujo, no el mismo.
//
//	«#»  canal público   `text-md leading-none`, `sidebar-fg-muted`   — MEDIDO: 14/14
//	«@»  con menciones   `text-xs leading-none` negrita sobre `danger`, redondo
//
// `ui.Rotulo` no vale para esto: coloca su texto en el origen de la caja que le den, y
// aquí la caja es la de 21 del icono y el texto son 14. La diferencia son 3,5 px, que
// es justo lo que separa una almohadilla alineada de una que flota.
type Marca struct {
	widget.BaseWidget

	// Signo es el carácter: «#» o «@».
	Signo string
	// Escalon es su escalón con `leading-none` (la caja de línea ES el tamaño).
	Escalon tema.Escalon
	Peso    ui.Peso
	// Tinta es el token del carácter.
	Tinta string
	// Fondo es el token del disco de detrás. Vacío: sin disco, que es el «#».
	Fondo string
	// Nombre es lo que se anuncia. Vacío: la marca es decorativa (`aria-hidden`), que
	// es lo que la web dice del «#».
	Nombre string
}

var _ fyne.Widget = (*Marca)(nil)

// NuevaAlmohadilla es el «#» de un canal público: `text-md leading-none` en el token
// que le toque a la fila.
func NuevaAlmohadilla(tinta string) *Marca {
	m := &Marca{
		Signo: "#", Escalon: tema.Escalon{Tam: tema.TextoMD.Tam, Linea: tema.TextoMD.Tam},
		Peso: ui.Medio, Tinta: tinta,
	}
	m.ExtendBaseWidget(m)
	return m
}

// NuevaMencion es el «@» de un canal con menciones: disco de peligro y el signo en
// negrita. Se anuncia con PALABRAS —«2 menciones»—, que es lo que la web pone en su
// `aria-label`: el color rojo por sí solo no comunica nada (WCAG 1.4.1).
func NuevaMencion(nombre string) *Marca {
	m := &Marca{
		Signo: "@", Escalon: tema.Escalon{Tam: tema.TextoXS.Tam, Linea: tema.TextoXS.Tam},
		// Blanco y no `on-primary`: aquel es el rótulo del relleno primario y en oscuro
		// es casi negro; sobre el rojo de peligro, que no cambia con el tema, daría 3,7:1.
		Peso: ui.Negrita, Tinta: "neutral-0", Fondo: "danger", Nombre: nombre,
	}
	m.ExtendBaseWidget(m)
	return m
}

// MedidaMarca es la caja del icono de una fila: la marca ocupa lo mismo que un glifo.
func MedidaMarca() fyne.Size { return fyne.NewSize(CajaIconoFila, CajaIconoFila) }

// AccessibilityLabel es el nombre, si lo tiene.
func (m *Marca) AccessibilityLabel() string { return m.Nombre }

// AccessibilityRole implementa fyne.Accessible.
func (m *Marca) AccessibilityRole() fyne.AccessibleRole { return fyne.AccessibleRoleText }

// CreateRenderer compone el disco y el signo.
func (m *Marca) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &marcaRenderer{m: m, texto: ui.Texto(m.Signo, m.Escalon, m.Peso, tema.Color(m.Tinta, v))}
	if m.Fondo != "" {
		r.disco = canvas.NewCircle(tema.Color(m.Fondo, v))
		r.objetos = append(r.objetos, r.disco)
	}
	r.objetos = append(r.objetos, r.texto)
	r.Refresh()
	return r
}

type marcaRenderer struct {
	m       *Marca
	disco   *canvas.Circle
	texto   *canvas.Text
	objetos []fyne.CanvasObject
}

func (r *marcaRenderer) MinSize() fyne.Size           { return MedidaMarca() }
func (r *marcaRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *marcaRenderer) Destroy()                     {}

// Layout centra el signo en la caja, en los dos ejes (`place-items-center`).
func (r *marcaRenderer) Layout(t fyne.Size) {
	if r.disco != nil {
		r.disco.Move(fyne.NewPos(0, 0))
		r.disco.Resize(t)
	}
	ancho := ui.Medir(r.m.Signo, r.m.Escalon, r.m.Peso).Width
	ui.ColocarTexto(r.texto, r.m.Escalon, (t.Width-ancho)/2, (t.Height-r.m.Escalon.Linea)/2)
}

// Refresh repinta con los colores del tema actual.
func (r *marcaRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r.texto.Color = tema.Color(r.m.Tinta, v)
	r.texto.Refresh()
	if r.disco != nil {
		r.disco.FillColor = tema.Color(r.m.Fondo, v)
		r.disco.Refresh()
	}
	r.Layout(r.m.Size())
}

// atenuado es un token con el alfa bajado, para lo que la hoja escribe con `/N`: el
// filete del botón «Ir a un canal…» es `border-sidebar-fg-muted/40`.
func atenuado(token string, v tema.Variante, parte float32) color.NRGBA {
	c := tema.Color(token, v)
	c.A = uint8(float32(c.A) * parte)
	return c
}

// ---- La pila de caras de la cabecera ----

// Medidas de la pila, en px de CSS. MEDIDO: el botón sale 28 × 69 con dos miembros.
var (
	// LadoCara es el `size="xs"` de cada avatar: 24 px literales.
	LadoCara float32 = 24
	// SolapeCaras es el `-ml-1.5` que las monta unas sobre otras.
	SolapeCaras = tema.Espacio(1.5)
	// AnilloCara es el `ring-2 ring-[var(--sytue-header-bg)]`: el anillo del color de
	// la banda es lo que separa dos avatares apilados sin sombras decorativas.
	AnilloCara float32 = 2
	// CanalonPila es su `px-2` y HuecoPila su `gap-1.5`.
	CanalonPila = tema.Espacio(2)
	HuecoPila   = tema.Espacio(1.5)
	// MaxCaras son las tres que la web enseña (`roster.slice(0, 3)`).
	MaxCaras = 3
)

// PilaCaras es el botón del roster de la cabecera: hasta tres avatares solapados y el
// número de miembros. Los avatares son DECORATIVOS —el botón ya se anuncia con el
// número—, y por eso el nombre accesible es el del botón y no el de cada cara.
type PilaCaras struct {
	ui.Pulsable

	// Caras son los nombres de los que se pintan; se recorta a MaxCaras.
	Caras []string
	// Cuantos es el número de la derecha, ya escrito.
	Cuantos string
	// Abierto es el `aria-pressed`: el panel de miembros está a la vista.
	Abierto bool

	caras   []*ui.Avatar
	anillos []*canvas.Rectangle
}

var _ fyne.Widget = (*PilaCaras)(nil)

// NuevaPilaCaras crea el botón. `nombre` es su `aria-label`, ya traducido.
func NuevaPilaCaras(caras []string, cuantos, nombre string, abierto bool, alPulsar func()) *PilaCaras {
	if len(caras) > MaxCaras {
		caras = caras[:MaxCaras]
	}
	p := &PilaCaras{Caras: caras, Cuantos: cuantos, Abierto: abierto}
	p.Nombre, p.AlPulsar = nombre, alPulsar
	p.ExtendBaseWidget(p)
	return p
}

// MedidaPilaCaras es lo que ocupa. Es la única cuenta: el `MinSize()` del renderer la
// llama y el guardián compara contra ella y contra los píxeles.
func MedidaPilaCaras(caras []string, cuantos string) fyne.Size {
	n := min(len(caras), MaxCaras)
	ancho := 2 * CanalonPila
	if n > 0 {
		ancho += LadoCara + float32(n-1)*(LadoCara-SolapeCaras) + HuecoPila
	}
	ancho += ui.Medir(cuantos, tema.TextoXS, ui.SemiNegrita).Width
	return fyne.NewSize(ancho, LadoAccionCabecera)
}

// CreateRenderer compone la caja, los anillos, los avatares y el número.
func (p *PilaCaras) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &pilaRenderer{
		p:    p,
		aro:  ui.NuevoAroCon("chrome-focus", tema.RadioMD, ui.SinHueco, v).Abierto(),
		caja: ui.NuevaCaja(color.NRGBA{}, color.NRGBA{}, tema.RadioMD),
		num:  ui.Texto(p.Cuantos, tema.TextoXS, ui.SemiNegrita, tema.Color("header-fg", v)),
	}
	r.objetos = append(r.aro.Objetos(), r.caja.Objetos()...)
	for range p.Caras {
		a := canvas.NewRectangle(tema.Color("header-bg", v))
		a.CornerRadius = tema.RadioMD + AnilloCara
		p.anillos = append(p.anillos, a)
	}
	for i, nombre := range p.Caras {
		r.objetos = append(r.objetos, p.anillos[i])
		av := ui.NuevoAvatar(nombre).Con(ui.AvatarXS)
		p.caras = append(p.caras, av)
		r.objetos = append(r.objetos, av)
	}
	r.objetos = append(r.objetos, r.num)
	r.Refresh()
	return r
}

type pilaRenderer struct {
	p       *PilaCaras
	aro     *ui.Aro
	caja    *ui.Caja
	num     *canvas.Text
	objetos []fyne.CanvasObject
}

func (r *pilaRenderer) MinSize() fyne.Size {
	return MedidaPilaCaras(r.p.Caras, r.p.Cuantos)
}
func (r *pilaRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *pilaRenderer) Destroy()                     {}

// Layout pone las caras solapadas y el número detrás, todo centrado en vertical.
func (r *pilaRenderer) Layout(t fyne.Size) {
	p := r.p
	r.aro.Colocar(t)
	r.caja.Redimensionar(t)
	x := CanalonPila
	y := (t.Height - LadoCara) / 2
	for i, av := range p.caras {
		p.anillos[i].Move(fyne.NewPos(x-AnilloCara, y-AnilloCara))
		p.anillos[i].Resize(fyne.NewSize(LadoCara+2*AnilloCara, LadoCara+2*AnilloCara))
		av.Move(fyne.NewPos(x, y))
		av.Resize(fyne.NewSize(LadoCara, LadoCara))
		x += LadoCara - SolapeCaras
	}
	if len(p.caras) > 0 {
		x += SolapeCaras + HuecoPila
	}
	ui.ColocarTexto(r.num, tema.TextoXS, x, (t.Height-tema.TextoXS.Linea)/2)
}

// Refresh repinta el velo del estado (`hover:bg-chrome-hover`, `aria-pressed`).
func (r *pilaRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	fondo := color.NRGBA{}
	switch {
	case r.p.Encima():
		fondo = tema.Color("chrome-hover", v)
	case r.p.Abierto:
		fondo = tema.Color("chrome-active", v)
	}
	r.caja.Pintar(fondo, color.NRGBA{})
	r.aro.Ver(r.p.Enfocado())
	r.aro.Pintar(v)
	r.num.Color = tema.Color("header-fg", v)
	r.num.Refresh()
	for _, a := range r.p.anillos {
		a.FillColor = tema.Color("header-bg", v)
		a.Refresh()
	}
	r.Layout(r.p.Size())
}

// ---- La píldora de una reacción ----

// Medidas de la píldora de reacción: `rounded-control border px-2 py-0.5 text-xs
// font-medium` con `gap-1` entre el emoji y el número. HOJA (el barrido cazó la barra
// de reacciones rápidas —24,5 × 24,5— y no las puestas, que en el canal medido no
// había).
var (
	// AltoReaccion es la caja de línea más el `py-0.5` y el filete de los dos lados.
	AltoReaccion = tema.TextoXS.Linea + 2*tema.Espacio(0.5) + 2*tema.Borde
	// CanalonReaccion es su `px-2` y HuecoReaccion su `gap-1`.
	CanalonReaccion = tema.Espacio(2)
	HuecoReaccion   = tema.Espacio(1)
)

// Reaccion es la píldora de un emoji con su conteo. Se anuncia con palabras —«añadir
// 👍 2» / «quitar la mía»—, que es lo que la web pone en su `aria-label`: `aria-pressed`
// por sí solo no dice cuál es.
type PildoraReaccion struct {
	ui.Pulsable

	Reaccion Reaccion
	// alEncima avisa a la fila: sobre la píldora el mensaje sigue «encima».
	alEncima func(bool)
	// pista es «quién reaccionó», el `title` de la web. Fyne da el `hover` al control y
	// no a la pista que lo envuelve, así que la píldora se lo pasa.
	pista *ui.Pista
}

// MouseIn es el `:hover` de la píldora, y avisa a la fila y a la pista.
func (p *PildoraReaccion) MouseIn(e *desktop.MouseEvent) {
	p.Pulsable.MouseIn(e)
	if p.pista != nil {
		p.pista.MouseIn(e)
	}
	if p.alEncima != nil {
		p.alEncima(true)
	}
}

// MouseOut apaga el `:hover` y suelta a la fila y a la pista.
func (p *PildoraReaccion) MouseOut() {
	p.Pulsable.MouseOut()
	if p.pista != nil {
		p.pista.MouseOut()
	}
	if p.alEncima != nil {
		p.alEncima(false)
	}
}

// ConPista envuelve la píldora con «quién reaccionó», si se sabe. Es lo que va en la
// fila.
func (p *PildoraReaccion) ConPista() fyne.CanvasObject {
	if p.Reaccion.Quien == "" {
		return p
	}
	p.pista = ui.NuevaPista(p.Reaccion.Quien, p)
	return p.pista
}

// MaxNombresReaccion es `MAX_NAMES`: cuántos nombres dice «quién reaccionó».
const MaxNombresReaccion = 3

// QuienReacciono es `reactedByLabel`: «Ana, Beto y 2 más reaccionaron con 👍». Sin
// ningún nombre que se sepa, solo cuántos —sin inventar a nadie—; sin `subs`, nada.
func QuienReacciono(t *i18n.Translator, l i18n.Locale, emoji string, subs []string, nombres map[string]string) string {
	if len(subs) == 0 {
		return ""
	}
	var vistos []string
	for _, s := range subs {
		if len(vistos) == MaxNombresReaccion {
			break
		}
		if n := nombres[s]; n != "" {
			vistos = append(vistos, n)
		}
	}
	resto := len(subs) - len(vistos)
	if len(vistos) == 0 {
		return t.T("reaction.byCount", i18n.Vars{"count": resto, "emoji": emoji})
	}
	if resto > 0 {
		vistos = append(vistos, t.T("reaction.andMore", i18n.Vars{"count": resto}))
	}
	return t.T("reaction.byNames", i18n.Vars{"count": len(subs), "names": i18n.FormatList(l, vistos), "emoji": emoji})
}

var _ fyne.Widget = (*PildoraReaccion)(nil)

// NuevaReaccion crea la píldora.
func NuevaReaccion(t *i18n.Translator, r Reaccion, alPulsar func()) *PildoraReaccion {
	clave := "reaction.add"
	if r.Mia {
		clave = "reaction.removeMine"
	}
	p := &PildoraReaccion{Reaccion: r}
	que := r.Emoji + " " + Contador(r.Veces)
	if r.Quien != "" {
		que = r.Quien
	}
	p.Nombre = t.T(clave, i18n.Vars{"what": que})
	p.AlPulsar = alPulsar
	p.ExtendBaseWidget(p)
	return p
}

// MedidaReaccion es lo que ocupa. Es la única cuenta.
func MedidaReaccion(r Reaccion) fyne.Size {
	ancho := 2*CanalonReaccion + 2*tema.Borde + HuecoReaccion + anchoEmojiReaccion(r.Emoji) +
		ui.Medir(Contador(r.Veces), tema.TextoXS, ui.Medio).Width
	return fyne.NewSize(ancho, AltoReaccion)
}

// propioDeReaccion es el fichero de una reacción que es un `:nombre:` de la
// organización: la píldora lleva su imagen (`EmojiToken`).
func propioDeReaccion(e string) (string, bool) {
	if m := reSoloPropio.FindStringSubmatch(e); m != nil {
		return ficheroPropio(m[1])
	}
	return "", false
}

// anchoEmojiReaccion es lo que ocupa el emoji de la píldora: su glifo o la imagen.
//
// ponytail: la imagen (1,375 em de `text-xs`) no crece la píldora, que se queda en su
// `AltoReaccion`; en la web la línea se estira un par de píxeles.
func anchoEmojiReaccion(e string) float32 {
	if _, ok := propioDeReaccion(e); ok {
		return anchoEmojiPropio(tema.TextoXS)
	}
	return ui.MedirConCara(e, tema.TextoXS, caraEmojiReaccion(e)).Width
}

// El glifo Unicode usa la misma fuente embebida del selector, también al medir.
// Un :nombre: que no se resolvió sigue siendo texto legible con Figtree.
func caraEmojiReaccion(e string) fyne.Resource {
	if reSoloPropio.MatchString(e) {
		return ui.Medio.Cara()
	}
	return tema.Emoji()
}

func (p *PildoraReaccion) tokens() (fondo, borde, tinta string) {
	if p.Reaccion.Mia {
		return "selected", "primary", "primary-text"
	}
	return "surface", "border", "text-muted"
}

// relleno es el fondo de la píldora YA compuesto sobre el papel de la fila: `selected`
// es un tinte translúcido y `ui.Caja` pinta el filete como un rectángulo relleno
// DEBAJO del de dentro, así que sin componerlo la píldora propia salía del color del
// filete —tinta— en vez del azul claro de la web.
func (p *PildoraReaccion) relleno(fondo string, v tema.Variante) color.NRGBA {
	return tokens.SobreSRGB(tema.Color(fondo, v), tema.Color("surface", v))
}

// CreateRenderer compone la píldora.
func (p *PildoraReaccion) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	fondo, borde, tinta := p.tokens()
	r := &reaccionRenderer{
		p:     p,
		aro:   ui.NuevoAro(tema.Forma("radius-control"), ui.SinHueco, v),
		caja:  ui.NuevaCaja(p.relleno(fondo, v), tema.Color(borde, v), tema.Forma("radius-control")),
		emoji: ui.TextoConCara(p.Reaccion.Emoji, tema.TextoXS, caraEmojiReaccion(p.Reaccion.Emoji), tema.Color(tinta, v)),
		veces: ui.Texto(Contador(p.Reaccion.Veces), tema.TextoXS, ui.Medio, tema.Color(tinta, v)),
	}
	r.objetos = append(r.aro.Objetos(), r.caja.Objetos()...)
	r.objetos = append(r.objetos, r.emoji, r.veces)
	if f, ok := propioDeReaccion(p.Reaccion.Emoji); ok {
		r.emoji.Text = ""
		if img := imagenPropia(f); img != nil {
			r.img = canvas.NewImageFromImage(img)
			r.img.FillMode = canvas.ImageFillContain
			r.objetos = append(r.objetos, r.img)
		}
	}
	r.Refresh()
	return r
}

type reaccionRenderer struct {
	p            *PildoraReaccion
	aro          *ui.Aro
	caja         *ui.Caja
	emoji, veces *canvas.Text
	img          *canvas.Image
	objetos      []fyne.CanvasObject
}

func (r *reaccionRenderer) MinSize() fyne.Size           { return MedidaReaccion(r.p.Reaccion) }
func (r *reaccionRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *reaccionRenderer) Destroy()                     {}

func (r *reaccionRenderer) Layout(t fyne.Size) {
	r.aro.Colocar(t)
	r.caja.Redimensionar(t)
	y := (t.Height - tema.TextoXS.Linea) / 2
	x := CanalonReaccion + tema.Borde
	ui.ColocarTexto(r.emoji, tema.TextoXS, x, y)
	if r.img != nil {
		lado := anchoEmojiPropio(tema.TextoXS) - 2
		r.img.Move(fyne.NewPos(x+1, yEmojiPropio(tema.TextoXS, ui.Medio, y)))
		r.img.Resize(fyne.NewSize(lado, lado))
	}
	x += anchoEmojiReaccion(r.p.Reaccion.Emoji) + HuecoReaccion
	ui.ColocarTexto(r.veces, tema.TextoXS, x, y)
}

func (r *reaccionRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	fondo, borde, tinta := r.p.tokens()
	if r.p.Encima() {
		borde = "border-strong"
		if r.p.Reaccion.Mia {
			borde = "primary-text"
		}
	}
	r.caja.Pintar(r.p.relleno(fondo, v), tema.Color(borde, v))
	r.aro.Ver(r.p.Enfocado())
	r.aro.Pintar(v)
	for _, c := range []*canvas.Text{r.emoji, r.veces} {
		c.Color = tema.Color(tinta, v)
		c.Refresh()
	}
	r.Layout(r.p.Size())
}

// ---- El «+» tras las reacciones ----

// AltoMasReaccion es el `h-6` del «+» que sigue a las reacciones: 21, algo más alto que
// una píldora (19,5).
var AltoMasReaccion = tema.Espacio(6)

// MasReaccion es el `EmojiPickerButton` del final de la tira (`reaction.addOne`):
// `rounded-control border border-border bg-surface px-2 text-text-muted`, con un «+» en
// `text-sm font-semibold`. Abre el selector para reaccionar con cualquiera.
type MasReaccion struct {
	ui.Pulsable
	alEncima func(bool)
}

// NuevoMasReaccion crea el «+».
func NuevoMasReaccion(t *i18n.Translator, alPulsar func()) *MasReaccion {
	b := &MasReaccion{}
	b.Nombre, b.AlPulsar = t.T("reaction.addOne", nil), alPulsar
	b.ExtendBaseWidget(b)
	return b
}

// MouseIn es su `:hover`, y avisa a la fila.
func (b *MasReaccion) MouseIn(e *desktop.MouseEvent) {
	b.Pulsable.MouseIn(e)
	if b.alEncima != nil {
		b.alEncima(true)
	}
}

// MouseOut lo apaga.
func (b *MasReaccion) MouseOut() {
	b.Pulsable.MouseOut()
	if b.alEncima != nil {
		b.alEncima(false)
	}
}

// MedidaMasReaccion es `px-2`, el «+» y el filete, a `h-6`.
func MedidaMasReaccion() fyne.Size {
	return fyne.NewSize(2*CanalonReaccion+2*tema.Borde+ui.Medir("+", tema.TextoSM, ui.SemiNegrita).Width, AltoMasReaccion)
}

func (b *MasReaccion) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &masReaccionRenderer{
		b:    b,
		aro:  ui.NuevoAro(tema.Forma("radius-control"), ui.SinHueco, v),
		caja: ui.NuevaCaja(tema.Color("surface", v), tema.Color("border", v), tema.Forma("radius-control")),
		mas:  ui.Texto("+", tema.TextoSM, ui.SemiNegrita, tema.Color("text-muted", v)),
	}
	r.objetos = append(r.aro.Objetos(), r.caja.Objetos()...)
	r.objetos = append(r.objetos, r.mas)
	return r
}

type masReaccionRenderer struct {
	b       *MasReaccion
	aro     *ui.Aro
	caja    *ui.Caja
	mas     *canvas.Text
	objetos []fyne.CanvasObject
}

func (r *masReaccionRenderer) MinSize() fyne.Size           { return MedidaMasReaccion() }
func (r *masReaccionRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *masReaccionRenderer) Destroy()                     {}

func (r *masReaccionRenderer) Layout(t fyne.Size) {
	r.aro.Colocar(t)
	r.caja.Redimensionar(t)
	// `leading-none`: la caja de línea del «+» es su propio tamaño.
	ui.ColocarTexto(r.mas, tema.TextoSM, (t.Width-ui.Medir("+", tema.TextoSM, ui.SemiNegrita).Width)/2,
		(t.Height-tema.TextoSM.Linea)/2)
}

func (r *masReaccionRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	fondo, borde := "surface", "border"
	if r.b.Encima() {
		fondo, borde = "bg-subtle", "border-strong"
	}
	r.caja.Pintar(tema.Color(fondo, v), tema.Color(borde, v))
	r.aro.Ver(r.b.Enfocado())
	r.aro.Pintar(v)
	r.Layout(r.b.Size())
}

// ---- Tu estado, en la cabecera del carril ----

// Medidas del disparador de «tu estado» (`StatusMenu.tsx`): `inline-flex max-w-[9rem]
// items-center gap-1.5 rounded-md px-1.5 py-1 text-xs font-medium` con un punto
// `h-2 w-2`. MEDIDO: 59,75 × 21 en (22,75, 66,5) con «En línea»; los 22,75 son el
// canalón del carril (14) más el `px-2.5` de la cabecera (8,75).
//
// Antes era un botón fantasma `sm` a todo el ancho: medía 28 de alto, así que la
// cabecera salía de 49 en vez de 42 y TODO el carril bajaba 7 px.
var (
	// SangriaEstado es el `px-2.5` del `div` de la cabecera que lo contiene.
	SangriaEstado = tema.Espacio(2.5)
	// CanalonEstado es su `px-1.5` y RellenoEstado su `py-1`.
	CanalonEstado = tema.Espacio(1.5)
	RellenoEstado = tema.Espacio(1)
	// HuecoEstado es el `gap-1.5` entre el punto y el texto; PuntoEstado el `h-2 w-2`.
	HuecoEstado = tema.Espacio(1.5)
	PuntoEstado = tema.Espacio(2)
	// MaxEstado es el `max-w-[9rem]`: un estado personalizado largo se trunca.
	MaxEstado float32 = 9 * tema.Rem
)

// EstadoPropio es el disparador de «tu estado» del carril: el punto de color y, al
// lado, el estado personalizado o, sin él, el nombre del estado. El punto NUNCA va
// solo: el texto lo acompaña siempre (WCAG 1.4.1).
type EstadoPropio struct {
	ui.Pulsable

	Presencia ui.Presencia
	// Texto es lo que se lee al lado del punto, ya traducido.
	Texto string

	aro     *ui.Aro
	caja    *ui.Caja
	punto   *canvas.Circle
	texto   *canvas.Text
	objetos []fyne.CanvasObject
}

var _ fyne.Widget = (*EstadoPropio)(nil)

// NuevoEstadoPropio crea el disparador. `nombre` es su `title`, ya traducido
// («Tu estado: En línea»).
func NuevoEstadoPropio(p ui.Presencia, texto, nombre string, alPulsar func()) *EstadoPropio {
	e := &EstadoPropio{Presencia: p, Texto: texto}
	e.Nombre, e.AlPulsar = nombre, alPulsar
	e.ExtendBaseWidget(e)
	return e
}

// MedidaEstadoPropio es lo que ocupa. Es la única cuenta.
func MedidaEstadoPropio(texto string) fyne.Size {
	ancho := 2*CanalonEstado + PuntoEstado + HuecoEstado + ui.Medir(texto, tema.TextoXS, ui.Medio).Width
	return fyne.NewSize(min(ancho, MaxEstado), tema.TextoXS.Linea+2*RellenoEstado)
}

// CreateRenderer compone el velo, el punto y el texto.
func (e *EstadoPropio) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	e.aro = ui.NuevoAroCon("sidebar-focus", tema.Forma("radius-control"), ui.SinHueco, v)
	e.caja = ui.NuevaCaja(color.NRGBA{}, color.NRGBA{}, tema.Forma("radius-control"))
	e.punto = canvas.NewCircle(color.NRGBA{})
	e.texto = ui.Texto(e.Texto, tema.TextoXS, ui.Medio, tema.Color("sidebar-fg-muted", v))
	e.objetos = append(e.aro.Objetos(), e.caja.Objetos()...)
	e.objetos = append(e.objetos, e.punto, e.texto)
	r := &estadoRenderer{e: e}
	r.Refresh()
	return r
}

type estadoRenderer struct{ e *EstadoPropio }

func (r *estadoRenderer) MinSize() fyne.Size           { return MedidaEstadoPropio(r.e.Texto) }
func (r *estadoRenderer) Objects() []fyne.CanvasObject { return r.e.objetos }
func (r *estadoRenderer) Destroy()                     {}

func (r *estadoRenderer) Layout(t fyne.Size) {
	e := r.e
	r.e.aro.Colocar(t)
	e.caja.Redimensionar(t)
	x := CanalonEstado
	e.punto.Move(fyne.NewPos(x, (t.Height-PuntoEstado)/2))
	e.punto.Resize(fyne.NewSize(PuntoEstado, PuntoEstado))
	x += PuntoEstado + HuecoEstado
	ui.Truncar(e.texto, e.Texto, tema.TextoXS, ui.Medio, max(t.Width-x-CanalonEstado, 0))
	ui.ColocarTexto(e.texto, tema.TextoXS, x, RellenoEstado)
}

// Refresh repinta el velo (`hover:bg-sidebar-hover hover:text-sidebar-fg`) y el punto.
func (r *estadoRenderer) Refresh() {
	e := r.e
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	fondo, tinta := color.NRGBA{}, tema.Color("sidebar-fg-muted", v)
	if e.Encima() {
		fondo, tinta = tema.Color("sidebar-hover-bg", v), tema.Color("sidebar-fg", v)
	}
	e.caja.Pintar(fondo, color.NRGBA{})
	e.aro.Ver(e.Enfocado())
	e.aro.Pintar(v)
	if tk := e.Presencia.Token(); tk != "" {
		e.punto.FillColor = tema.Color(tk, v)
	}
	e.punto.Refresh()
	e.texto.Color = tinta
	e.texto.Refresh()
	r.Layout(e.Size())
}

// ---- «Ir a un canal…» ----

// BotonIrACanal es la caja de arriba del carril: un BOTÓN con pinta de campo, porque lo
// que abre es el conmutador rápido y no un filtro de la lista. `mx-2.5 mb-2 flex h-8
// w-[calc(100%-1.25rem)] items-center gap-2 rounded-md border px-2.5 text-sm`, filete
// `sidebar-fg-muted/40` y tinta `sidebar-fg-muted`, que pasan a `sidebar-hover` y
// `sidebar-fg` al pasar por encima. MEDIDO: 213,5 × 28 en (22,75, 105).
//
// No es `ui.FilaLateral` con `Borde`, que es como estaba, por dos medidas: aquella se
// estira a los 231 del carril sin el `mx-2.5` y mide 28,5 (su glifo de 18 manda sobre
// el `py-1.5`). Y por un color: `ui.Caja` pinta el filete como un rectángulo RELLENO
// debajo del relleno, así que con el fondo transparente de la web la caja entera salía
// del color del filete. Aquí el relleno es el del carril, opaco, y el velo de `hover`
// se compone encima.
//
// A la derecha lleva el `<kbd class="text-2xs border-sidebar-fg-muted/40 rounded border
// px-1 font-mono">Ctrl+G</kbd>`, que es `ui.Tecla`. La tecla sigue además en el nombre
// accesible, como el `title` de la web: el `kbd` es decoración.
//
// ponytail: la pastilla de la web no tiene relleno y deja ver el botón; `ui.Caja` pinta
// el filete como rectángulo relleno DEBAJO del de dentro, así que aquí el de dentro es
// `sidebar-bg`, opaco. En reposo sale exacto; al pasar por encima, el interior de la
// pastilla no recibe el 8 % blanco de `sidebar-hover`. Se arregla cuando `ui.Caja`
// sepa pintar un filete hueco.
type BotonIrACanal struct {
	ui.Pulsable

	Rotulo string

	aro     *ui.Aro
	caja    *ui.Caja
	glifo   *iconos.Dibujo
	texto   *canvas.Text
	tecla   *ui.Tecla
	objetos []fyne.CanvasObject
}

var _ fyne.Widget = (*BotonIrACanal)(nil)

// NuevoBotonIrACanal crea la caja. `nombre` es su `title` («Ir a un canal… (Ctrl+G)»).
func NuevoBotonIrACanal(rotulo, nombre string, alPulsar func()) *BotonIrACanal {
	b := &BotonIrACanal{Rotulo: rotulo}
	b.Nombre, b.AlPulsar = nombre, alPulsar
	b.ExtendBaseWidget(b)
	return b
}

// MedidaIrACanal es lo que ocupa: el ancho útil del carril menos el `mx-2.5` de los
// dos lados, por el `h-8`. Es la única cuenta.
func MedidaIrACanal() fyne.Size {
	return fyne.NewSize(ui.AnchoUtilMenu-2*MargenIrACanal, AltoIrACanal)
}

// CreateRenderer compone la caja, la lupa y el rótulo.
func (b *BotonIrACanal) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	b.aro = ui.NuevoAroCon("chrome-focus", tema.Forma("radius-control"), ui.SinHueco, v)
	b.caja = ui.NuevaCaja(ui.FondoCarril(v), filetesIrACanal(v), tema.Forma("radius-control"))
	b.glifo = iconos.Trazo(iconos.Buscar, GlifoIrACanal, "sidebar-fg-muted", v)
	b.texto = ui.Texto(b.Rotulo, tema.TextoSM, ui.Normal, tema.Color("sidebar-fg-muted", v))
	b.tecla = ui.NuevaTecla(ui.AtajoDelSistema("Ctrl+G"), tema.Texto2XS, ui.Normal)
	b.tecla.Fondo, b.tecla.Tinta = ui.TokenCarril(), "sidebar-fg-muted"
	b.tecla.Borde, b.tecla.OpacidadBorde = "sidebar-fg-muted", 0.4
	b.tecla.RellenoX = tema.Espacio(1) // `px-1`; sin `py`, la caja es la de la línea
	b.objetos = append(b.aro.Objetos(), b.caja.Objetos()...)
	b.objetos = append(b.objetos, b.glifo.Objeto(), b.texto, b.tecla)
	r := &irACanalRenderer{b: b}
	r.Refresh()
	return r
}

// filetesIrACanal es `border-sidebar-fg-muted/40` ya compuesto sobre el carril, porque
// `ui.Caja` pinta el filete como un rectángulo relleno DEBAJO del relleno.
func filetesIrACanal(v tema.Variante) color.NRGBA {
	return tokens.SobreSRGB(atenuado("sidebar-fg-muted", v, 0.4), ui.FondoCarril(v))
}

type irACanalRenderer struct{ b *BotonIrACanal }

func (r *irACanalRenderer) MinSize() fyne.Size           { return MedidaIrACanal() }
func (r *irACanalRenderer) Objects() []fyne.CanvasObject { return r.b.objetos }
func (r *irACanalRenderer) Destroy()                     {}

func (r *irACanalRenderer) Layout(t fyne.Size) {
	b := r.b
	b.aro.Colocar(t)
	b.caja.Redimensionar(t)
	x := tema.Borde + CanalonIrACanal
	b.glifo.Colocar(x, (t.Height-GlifoIrACanal)/2, GlifoIrACanal)
	x += GlifoIrACanal + HuecoIrACanal
	k := ui.MedidaTecla(b.tecla)
	xk := t.Width - tema.Borde - CanalonIrACanal - k.Width
	b.tecla.Resize(k)
	b.tecla.Move(fyne.NewPos(xk, (t.Height-k.Height)/2))
	ui.Truncar(b.texto, b.Rotulo, tema.TextoSM, ui.Normal, max(xk-HuecoIrACanal-x, 0))
	ui.ColocarTexto(b.texto, tema.TextoSM, x, (t.Height-tema.TextoSM.Linea)/2)
}

func (r *irACanalRenderer) Refresh() {
	b := r.b
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	fondo, tinta := ui.FondoCarril(v), "sidebar-fg-muted"
	if b.Encima() {
		fondo, tinta = tokens.SobreSRGB(tema.Color("sidebar-hover-bg", v), fondo), "sidebar-fg"
	}
	b.caja.Pintar(fondo, filetesIrACanal(v))
	b.aro.Ver(b.Enfocado())
	b.aro.Pintar(v)
	b.glifo.Tenir(tinta, v)
	b.texto.Color = tema.Color(tinta, v)
	b.texto.Refresh()
	b.tecla.Tinta = tinta
	b.tecla.Refresh()
	r.Layout(b.Size())
}
