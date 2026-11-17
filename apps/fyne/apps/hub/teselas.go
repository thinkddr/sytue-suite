// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package hub

import (
	"encoding/base64"
	"image/color"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/tokens"
)

// Las tres piezas repetidas del lanzador. Son de ESTA aplicación y no del catálogo
// compartido, igual que en la web: viven en `apps/portal/src/components/Launcher.tsx`
// y no en `packages/ui`. La cáscara sí es la compartida (`ui.Pulsable`).

// ---- La tesela de producto ----

// Medidas de la tesela, del marcado de `TeselaProducto.tsx` y medidas en producción a
// 1280×800 (`inicio.ancho.json`):
//
//	tesela      239,19 × 115,39 · `rounded-lg border p-4 gap-3`
//	marca       40 (`ProductIcon size={40}`)
//	nombre      14/19,25 · 600 · `truncate`
//	descripción 11,375/15,6406 · 400 · `text-sm leading-snug line-clamp-2`
//
// 115,39 = 2 de borde + 28 de `p-4` + 40 + 10,5 de `gap-3` + 19,25 + 15,6406. Sale
// clavado, y por eso la descripción de esas trece teselas cabe en UNA línea.
const (
	// MargenTesela es el `p-4`.
	MargenTesela float32 = 4 * 0.25 * tema.Rem
	// HuecoTesela es el `gap-3` entre la marca y el bloque de texto.
	HuecoTesela float32 = 3 * 0.25 * tema.Rem
	// LineasDescripcion es el `line-clamp-2`.
	LineasDescripcion = 2
)

// La tesela COMPACTA de la forma Marco (`marco:flex-row marco:items-center marco:px-3
// marco:py-2.5` en `TeselaProducto.tsx`): la marca a la izquierda —el `ProductIcon` de 40 al
// 105 %, 42— y, a su derecha, el nombre y la descripción, en UNA línea cada uno.
const (
	// TeselaCompactaPX es el `marco:px-3` y TeselaCompactaPY el `marco:py-2.5`.
	TeselaCompactaPX float32 = 3 * 0.25 * tema.Rem
	TeselaCompactaPY float32 = 2.5 * 0.25 * tema.Rem
	// EscalaMarcaCompacta es el `marco:[&>span:first-child]:scale-105` de la marca.
	EscalaMarcaCompacta float32 = 1.05
	// AltoTeselaCompacta es el borde, el `py-2.5` y la caja de la marca, que es lo más
	// alto de la fila: 62, medido en la web.
	AltoTeselaCompacta float32 = 2*tema.Borde + 2*TeselaCompactaPY + iconos.MarcaLG
)

// radioTesela es el `rounded-card` de la tesela: un token de forma.
func radioTesela() float32 { return tema.Forma("radius-card") }

// EscalonDescripcion es `text-sm leading-snug`: el tamaño de `text-sm` con el
// interlineado 1,375 que le pone `leading-snug`, y no el 1,125 de la escala. Medido:
// 15,6406, no 15,75. Son 0,11 px por línea, que en una rejilla de tres filas de
// teselas son 0,33 — y es la diferencia entre que la tesela mida 115,39 o 115,5.
var EscalonDescripcion = tema.Escalon{Tam: tema.TextoSM.Tam, Linea: 1.375 * tema.TextoSM.Tam}

// TeselaApp es el lanzador de un producto: la marca, el nombre corto y la
// descripción. Pulsarla NO abre una pantalla de este binario: le pide al sistema que
// abra la aplicación de ese producto (ver `productos.go`).
type TeselaApp struct {
	ui.Pulsable

	Producto Producto
	// Descripcion ya traducida (`app.<clave>.description`).
	Descripcion string
	// Rotulo es el `aria-label`/`title`: «{name} — {description}».
	Rotulo string
	// Deshabilitada es el `disabled` de la web: una cuenta personal no tiene SyERP.
	// Se pinta al 40 % y no responde.
	Deshabilitada bool
	// RotuloDeshabilitada es «Desactivado», la insignia de la esquina.
	RotuloDeshabilitada string
	// Vecina es la tesela a la que lleva una flecha, Inicio o Fin, o nil. La pone el
	// lanzador, que es quien conoce la rejilla (`navegarProducto` en `Launcher.tsx`).
	Vecina Vecina
}

// Vecina da la tesela a la que lleva una flecha, Inicio o Fin desde `desde`, o nil. La
// galería son las DOS rejillas, la de producto y la de apps conectadas: el destino
// puede ser de cualquiera de las dos (`[data-hub-galeria]` en la web).
type Vecina func(desde fyne.Focusable, tecla fyne.KeyName) fyne.Focusable

// moverEnGaleria atiende las flechas, Inicio y Fin de una tesela con vecinas; devuelve
// false con cualquier otra tecla, o fuera de una galería, para que siga su camino.
// La sobrescritura va en las teselas y no en el `Pulsable` común: fuera de una rejilla
// las flechas tienen que seguir subiendo a la ventana.
func moverEnGaleria(p *ui.Pulsable, yo fyne.Focusable, vecina Vecina, e *fyne.KeyEvent) bool {
	switch e.Name {
	case fyne.KeyLeft, fyne.KeyRight, fyne.KeyUp, fyne.KeyDown, fyne.KeyHome, fyne.KeyEnd:
	default:
		return false
	}
	if vecina == nil {
		return false
	}
	// Como el `preventDefault` de la web: sin destino (borde de la galería), la tecla
	// se consume igual y el foco no se mueve.
	if d := vecina(yo, e.Name); d != nil && d != yo {
		if c := p.Lienzo(); c != nil {
			c.Focus(d)
		}
	}
	return true
}

var _ fyne.Widget = (*TeselaApp)(nil)

// NuevaTeselaApp crea la tesela.
func NuevaTeselaApp(p Producto, descripcion, rotulo string, alPulsar func()) *TeselaApp {
	t := &TeselaApp{Producto: p, Descripcion: descripcion, Rotulo: rotulo}
	t.Nombre = rotulo
	t.AlPulsar = alPulsar
	t.ExtendBaseWidget(t)
	return t
}

// AccessibilityLabel es el `aria-label` de la web, que junta nombre y descripción: un
// lector de pantalla que solo diga «Mail» no dice a dónde lleva.
func (t *TeselaApp) AccessibilityLabel() string { return t.Rotulo }

// TypedKey mueve el foco entre las teselas con las flechas, Inicio y Fin, como el
// `onKeyDown={navegarProducto}` de la web. Lo demás (Intro, Escape…) lo sigue
// atendiendo el `Pulsable`.
func (t *TeselaApp) TypedKey(e *fyne.KeyEvent) {
	if !moverEnGaleria(&t.Pulsable, t, t.Vecina, e) {
		t.Pulsable.TypedKey(e)
	}
}

// AltoTesela es lo que ocupa una tesela cuya descripción se reparte en `lineas`.
func AltoTesela(lineas int) float32 {
	return 2*tema.Borde + 2*MargenTesela + iconos.MarcaLG + HuecoTesela +
		tema.TextoRaiz.Linea + float32(lineas)*EscalonDescripcion.Linea
}

// LineasDe es en cuántas líneas cae la descripción dentro de una tesela de ese ancho,
// con el tope de `line-clamp-2`.
func LineasDe(descripcion string, ancho float32) int {
	cabe := ancho - 2*tema.Borde - 2*MargenTesela
	return min(len(ui.Repartir(descripcion, EscalonDescripcion, ui.Normal, cabe)), LineasDescripcion)
}

// CreateRenderer construye la tesela. Solo primitivas y piezas de `iconos`.
func (t *TeselaApp) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &teselaRenderer{
		t:      t,
		v:      v,
		sombra: ui.NuevaSombra(tema.SombraSM, radioTesela()).SinInterior(),
		caja:   ui.NuevaCaja(t.relleno(v), t.borde(v), radioTesela()).ConTrazo(),
		aro:    ui.NuevoAro(radioTesela(), ui.SinHueco, v),
		marca:  iconos.Marca(t.Producto.Marca, iconos.MarcaLG, v),
		nombre: ui.Texto(t.Producto.NombreCorto(), tema.TextoRaiz, ui.SemiNegrita, t.tintaNombre(v)),
	}
	r.marca.Apagar(t.Deshabilitada) // `opacity-40 grayscale`
	for range LineasDescripcion {
		r.desc = append(r.desc, ui.Texto("", EscalonDescripcion, ui.Normal, t.tinta(v, "text-muted")))
	}
	// La sombra, la primera: va detrás del relleno, como en `ui.Tarjeta`.
	r.objetos = append([]fyne.CanvasObject{r.sombra.Objeto()}, r.aro.Objetos()...)
	r.objetos = append(r.objetos, r.caja.Objetos()...)
	r.objetos = append(r.objetos, r.marca.Objeto(), r.nombre)
	for _, d := range r.desc {
		r.objetos = append(r.objetos, d)
	}
	if t.Deshabilitada && t.RotuloDeshabilitada != "" {
		// ponytail: la web NO usa aquí `Badge` sino un span suelto de
		// `text-[10px] tracking-wider` CON borde; `ui.Insignia` es Badge.tsx (10,5 px,
		// píldora o `rounded-sm`, sin borde) y no lo clava. No se ha podido medir: la
		// captura de producción es de una cuenta de organización y esta insignia solo
		// sale en una personal, donde SyERP viene apagado. Se cierra midiendo una.
		r.insignia = ui.NuevaInsignia(t.RotuloDeshabilitada, ui.InsigniaNeutra)
		r.insignia.Cuadrada = true
		r.objetos = append(r.objetos, r.insignia)
	}
	return r
}

// tinta aplana el 40 % de `opacity-40` contra la superficie, igual que hace
// `ui.Boton` con su `disabled:opacity-50`: en la web la opacidad se aplica UNA vez
// sobre el grupo, y bajando el alfa capa a capa el interior de la caja sale mal.
func (t *TeselaApp) tinta(v tema.Variante, token string) color.NRGBA {
	c := tema.Color(token, v)
	if !t.Deshabilitada {
		return c
	}
	return tokens.Aplanar(c, 0.4, tema.Color("surface", v))
}

func (t *TeselaApp) tintaNombre(v tema.Variante) color.NRGBA {
	if t.Deshabilitada {
		return t.tinta(v, "text-muted")
	}
	return t.tinta(v, "text")
}

// Cada acceso es una tarjeta de recurso (`.hub-product-card`): la superficie blanca,
// despegada del lienzo con `shadow-sm`, y el color de la marca solo en su icono —la
// tesela del producto ya es un relleno sólido—. Fue la marca al 12 % sobre la superficie
// hasta la forma Carril.
func (t *TeselaApp) fondo(v tema.Variante) color.NRGBA {
	return tema.Color("surface", v)
}

// relleno es el fondo con la opacidad de la tesela apagada.
func (t *TeselaApp) relleno(v tema.Variante) color.NRGBA {
	c := t.fondo(v)
	if t.Deshabilitada {
		return tokens.Aplanar(c, 0.4, tema.Color("surface", v))
	}
	return c
}

func (t *TeselaApp) borde(v tema.Variante) color.NRGBA {
	// `border-border-subtle` y, con el puntero encima, `hover:border-border-strong`: la
	// regla de `.hub-product-card` ya no fija el borde y el hover vuelve a aplicarse
	// (la tarjeta apagada no lo lleva).
	nombre := "border-subtle"
	if t.Encima() && !t.Deshabilitada {
		nombre = "border-strong"
	}
	c := tokens.SobreConAlfa(tokens.Crudo(nombre, v), t.fondo(v))
	if t.Deshabilitada {
		return tokens.Aplanar(c, 0.4, tema.Color("surface", v))
	}
	return c
}

type teselaRenderer struct {
	t        *TeselaApp
	v        tema.Variante
	sombra   *ui.Sombra
	caja     *ui.Caja
	aro      *ui.Aro
	marca    *iconos.Dibujo
	nombre   *canvas.Text
	desc     []*canvas.Text
	insignia *ui.Insignia
	objetos  []fyne.CanvasObject
}

// MinSize es la medida declarada. La rejilla la estira al ancho de columna y al alto
// de la fila (`h-full`), así que el mínimo solo manda cuando no hay rejilla.
func (r *teselaRenderer) MinSize() fyne.Size {
	if tema.EsMarco() {
		return fyne.NewSize(0, AltoTeselaCompacta)
	}
	return fyne.NewSize(0, AltoTesela(LineasDescripcion))
}

func (r *teselaRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *teselaRenderer) Destroy()                     {}

// colocarCompacta es la tesela de Marco: la marca a la izquierda y, centrados en su
// alto (`items-center`), el nombre y la descripción en una línea cada uno.
func (r *teselaRenderer) colocarCompacta(t fyne.Size) {
	x := tema.Borde + TeselaCompactaPX
	// `scale-105`: la marca crece desde su centro y no mueve lo de al lado.
	crece := iconos.MarcaLG * (EscalaMarcaCompacta - 1) / 2
	r.marca.Colocar(x-crece, (t.Height-iconos.MarcaLG)/2-crece, iconos.MarcaLG*EscalaMarcaCompacta)
	x += iconos.MarcaLG + HuecoTesela
	cabe := max(t.Width-x-TeselaCompactaPX-tema.Borde, 0)
	y := (t.Height - tema.TextoRaiz.Linea - EscalonDescripcion.Linea) / 2

	cabeNombre := cabe
	if r.insignia != nil {
		m := r.insignia.MinSize()
		r.insignia.Resize(m)
		r.insignia.Move(fyne.NewPos(x+cabe-m.Width, y+(tema.TextoRaiz.Linea-m.Height)/2))
		cabeNombre -= m.Width + 0.25*tema.Rem
	}
	ui.Truncar(r.nombre, r.t.Producto.NombreCorto(), tema.TextoRaiz, ui.SemiNegrita, cabeNombre)
	ui.ColocarTexto(r.nombre, tema.TextoRaiz, x, y)
	y += tema.TextoRaiz.Linea
	for i, d := range r.desc {
		if i > 0 { // `marco:line-clamp-1`
			d.Hide()
			continue
		}
		d.Show()
		ui.Truncar(d, r.t.Descripcion, EscalonDescripcion, ui.Normal, cabe)
		ui.ColocarTexto(d, EscalonDescripcion, x, y)
	}
}

// Layout coloca marca, nombre, insignia y descripción, de arriba abajo
// (`flex-col items-start`), o en fila en la forma Marco.
func (r *teselaRenderer) Layout(t fyne.Size) {
	r.aro.Colocar(t)
	r.sombra.Ajustar(fyne.NewPos(0, 0), t, r.v)
	r.caja.Redimensionar(t)
	if tema.EsMarco() {
		r.colocarCompacta(t)
		return
	}
	x := tema.Borde + MargenTesela
	y := tema.Borde + MargenTesela
	cabe := t.Width - 2*x
	r.marca.Colocar(x, y, iconos.MarcaLG)
	y += iconos.MarcaLG + HuecoTesela

	cabeNombre := cabe
	if r.insignia != nil {
		m := r.insignia.MinSize()
		// `justify-between gap-1`: la insignia a la derecha y el nombre con lo que
		// quede.
		r.insignia.Resize(m)
		r.insignia.Move(fyne.NewPos(t.Width-x-m.Width, y+(tema.TextoRaiz.Linea-m.Height)/2))
		cabeNombre -= m.Width + 0.25*tema.Rem
	}
	ui.Truncar(r.nombre, r.t.Producto.NombreCorto(), tema.TextoRaiz, ui.SemiNegrita, cabeNombre)
	ui.ColocarTexto(r.nombre, tema.TextoRaiz, x, y)
	y += tema.TextoRaiz.Linea

	lineas := ui.Repartir(r.t.Descripcion, EscalonDescripcion, ui.Normal, cabe)
	for i, d := range r.desc {
		if i >= len(lineas) || i >= LineasDescripcion {
			d.Hide()
			continue
		}
		d.Show()
		// La ÚLTIMA línea del `line-clamp-2` se trunca con puntos suspensivos si
		// todavía queda texto por debajo, que es lo que hace `-webkit-line-clamp`.
		texto := lineas[i]
		if i == LineasDescripcion-1 && len(lineas) > LineasDescripcion {
			texto = juntar(lineas[i:])
		}
		ui.Truncar(d, texto, EscalonDescripcion, ui.Normal, cabe)
		ui.ColocarTexto(d, EscalonDescripcion, x, y)
		y += EscalonDescripcion.Linea
	}
}

// Refresh repinta con el estado del puntero y del foco.
func (r *teselaRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r.v = v
	r.sombra.CambiarRadio(radioTesela()) // la forma se cambia en caliente
	r.caja.CambiarRadio(radioTesela())
	r.aro.CambiarRadio(radioTesela())
	r.caja.Pintar(r.t.relleno(v), r.t.borde(v))
	r.aro.Pintar(v)
	r.aro.Ver(r.t.Enfocado())
	r.marca.Tenir("product-tile-ring", v)
	r.nombre.Color = r.t.tintaNombre(v)
	r.nombre.Refresh()
	for _, d := range r.desc {
		d.Color = r.t.tinta(v, "text-muted")
		d.Refresh()
	}
}

// juntar pega las líneas que sobran del `line-clamp` en una sola, para que el
// truncado las considere: sin esto, la última línea acabaría sin puntos suspensivos
// aunque quede texto detrás.
func juntar(lineas []string) string {
	out := lineas[0]
	for _, l := range lineas[1:] {
		out += " " + l
	}
	return out
}

// ---- La tesela de una aplicación conectada ----

// La tesela de una aplicación de terceros (`TeselaConectada` de `TeselaProducto.tsx`) es
// la de producto: la misma caja, las mismas medidas, la forma compacta de Marco y su
// sitio en las flechas. Iba con caja propia —`rounded-lg border-border`, un cuadro de 42
// y sin forma compacta— y, al lado de las propias, se leía como de otro sitio.
//
// Su marca es un distintivo NEUTRO con la geometría de `ProductIcon size={40}` (de 4 a
// 36, radio del 22,5 %): el logotipo lo pone un tercero y su forma es impredecible, así
// que va dentro, a 20, sin teñirlo con el color de la app. Sin icono, la inicial del
// nombre sobre ese neutro, en vez de una caja gris vacía.
const (
	// LadoIconoConectada es el `<img width={20} height={20}>` dentro del distintivo.
	LadoIconoConectada float32 = 20
	// MargenDistintivo y RadioDistintivo son el `inset: 4px` y el `border-radius: 9px`
	// del distintivo sobre su caja de 40 (los de `ProductIcon`), en proporción.
	MargenDistintivo = 0.1
	RadioDistintivo  = 0.225
)

// TeselaConectada es el lanzador de una aplicación conectada por la organización.
// Pulsarla abre su dirección de lanzamiento en el navegador (`target="_blank"`).
type TeselaConectada struct {
	ui.Pulsable

	App api.AppConectada
	// Rotulo es el `aria-label`/`title`: «{name} — {description}».
	Rotulo string
	// Vecina es la de `TeselaApp`: la pone el lanzador, y la galería es una sola.
	Vecina Vecina
}

var _ fyne.Widget = (*TeselaConectada)(nil)

// NuevaTeselaConectada crea la tesela.
func NuevaTeselaConectada(a api.AppConectada, rotulo string, alPulsar func()) *TeselaConectada {
	t := &TeselaConectada{App: a, Rotulo: rotulo}
	t.Nombre, t.AlPulsar = rotulo, alPulsar
	t.ExtendBaseWidget(t)
	return t
}

// AccessibilityLabel junta nombre y descripción, como la de producto.
func (t *TeselaConectada) AccessibilityLabel() string { return t.Rotulo }

// TypedKey: las flechas, Inicio y Fin recorren la galería entera, como la de producto.
func (t *TeselaConectada) TypedKey(e *fyne.KeyEvent) {
	if !moverEnGaleria(&t.Pulsable, t, t.Vecina, e) {
		t.Pulsable.TypedKey(e)
	}
}

// InicialDe es la letra de reserva de una app sin icono: la primera LETRA, en
// mayúscula (`inicialDe` de la web: «écija» da «É» y no medio carácter).
func InicialDe(nombre string) string {
	for _, r := range strings.TrimSpace(nombre) {
		return strings.ToUpper(string(r))
	}
	return "?"
}

// iconoDe convierte el `data:image/svg+xml;base64,…` del manifiesto en un dibujo, con
// las mismas correcciones de `oksvg` que las marcas de la suite (`iconos.DeSVGAjeno`) y
// encajado sin deformar en su cuadro, como el `<img width=20 height=20>` de la web. Sin
// icono, o con uno que no se deja leer, nil: la tesela pone la inicial.
func iconoDe(a api.AppConectada) *iconos.Dibujo {
	const prefijo = "data:image/svg+xml;base64,"
	if !strings.HasPrefix(a.IconURL, prefijo) {
		return nil
	}
	svg, err := base64.StdEncoding.DecodeString(a.IconURL[len(prefijo):])
	if err != nil || len(svg) == 0 {
		return nil
	}
	d, ok := iconos.DeSVGAjeno(svg, LadoIconoConectada)
	if !ok {
		return nil
	}
	return d
}

// CreateRenderer compone la caja de la tesela de producto, el distintivo con su imagen
// o su inicial, el nombre y la descripción.
func (t *TeselaConectada) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &teselaConectadaRenderer{
		t:          t,
		v:          v,
		sombra:     ui.NuevaSombra(tema.SombraSM, radioTesela()).SinInterior(),
		caja:       ui.NuevaCaja(tema.Color("surface", v), t.borde(v), radioTesela()).ConTrazo(),
		aro:        ui.NuevoAro(radioTesela(), ui.SinHueco, v),
		distintivo: canvas.NewRectangle(tema.Color("bg-subtle", v)),
		nombre:     ui.Texto(t.App.Name, tema.TextoRaiz, ui.SemiNegrita, tema.Color("text", v)),
	}
	r.distintivo.StrokeColor = tema.Color("border-subtle", v)
	r.distintivo.StrokeWidth = tema.Borde
	r.objetos = append([]fyne.CanvasObject{r.sombra.Objeto()}, r.aro.Objetos()...)
	r.objetos = append(r.objetos, r.caja.Objetos()...)
	r.objetos = append(r.objetos, r.distintivo)
	if d := iconoDe(t.App); d != nil {
		r.icono = d
		r.objetos = append(r.objetos, d.Objeto())
	} else {
		r.inicial = ui.Texto(InicialDe(t.App.Name), tema.TextoRaiz, ui.SemiNegrita,
			tema.Color("text-muted", v))
		r.objetos = append(r.objetos, r.inicial)
	}
	r.objetos = append(r.objetos, r.nombre)
	for range LineasDescripcion {
		d := ui.Texto("", EscalonDescripcion, ui.Normal, tema.Color("text-muted", v))
		r.desc = append(r.desc, d)
		r.objetos = append(r.objetos, d)
	}
	return r
}

// borde es el de la tesela de producto: `border-border-subtle` y, con el puntero
// encima, `hover:border-border-strong`.
func (t *TeselaConectada) borde(v tema.Variante) color.NRGBA {
	nombre := "border-subtle"
	if t.Encima() {
		nombre = "border-strong"
	}
	return tokens.SobreConAlfa(tokens.Crudo(nombre, v), tema.Color("surface", v))
}

type teselaConectadaRenderer struct {
	t          *TeselaConectada
	v          tema.Variante
	sombra     *ui.Sombra
	caja       *ui.Caja
	aro        *ui.Aro
	distintivo *canvas.Rectangle
	icono      *iconos.Dibujo
	inicial    *canvas.Text
	nombre     *canvas.Text
	desc       []*canvas.Text
	objetos    []fyne.CanvasObject
}

// MinSize es el de la tesela de producto: las dos rejillas miden lo mismo.
func (r *teselaConectadaRenderer) MinSize() fyne.Size {
	if tema.EsMarco() {
		return fyne.NewSize(0, AltoTeselaCompacta)
	}
	return fyne.NewSize(0, AltoTesela(LineasDescripcion))
}
func (r *teselaConectadaRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *teselaConectadaRenderer) Destroy()                     {}

// colocarMarca pone el distintivo en la caja de la marca de lado `lado` con su esquina
// en (x, y), y dentro, centrados, el icono o la inicial.
func (r *teselaConectadaRenderer) colocarMarca(x, y, lado float32) {
	m := lado * MargenDistintivo
	r.distintivo.CornerRadius = lado * RadioDistintivo
	r.distintivo.Move(fyne.NewPos(x+m, y+m))
	r.distintivo.Resize(fyne.NewSize(lado-2*m, lado-2*m))
	cx, cy := x+lado/2, y+lado/2
	if r.icono != nil {
		// `xMidYMid`: el dibujo ya encajado, centrado, y con el `scale-105` de Marco.
		tam := r.icono.Medida()
		esc := lado / iconos.MarcaLG
		tam = fyne.NewSize(tam.Width*esc, tam.Height*esc)
		r.icono.ColocarCaja(cx-tam.Width/2, cy-tam.Height/2, tam)
	}
	if r.inicial != nil {
		w := ui.Medir(r.inicial.Text, tema.TextoRaiz, ui.SemiNegrita).Width
		ui.ColocarTexto(r.inicial, tema.TextoRaiz, cx-w/2, cy-tema.TextoRaiz.Linea/2)
	}
}

// colocarCompacta es la de `teselaRenderer`: marca a la izquierda, nombre y
// descripción en una línea cada uno.
func (r *teselaConectadaRenderer) colocarCompacta(t fyne.Size) {
	x := tema.Borde + TeselaCompactaPX
	crece := iconos.MarcaLG * (EscalaMarcaCompacta - 1) / 2
	r.colocarMarca(x-crece, (t.Height-iconos.MarcaLG)/2-crece, iconos.MarcaLG*EscalaMarcaCompacta)
	x += iconos.MarcaLG + HuecoTesela
	cabe := max(t.Width-x-TeselaCompactaPX-tema.Borde, 0)
	y := (t.Height - tema.TextoRaiz.Linea - EscalonDescripcion.Linea) / 2
	ui.Truncar(r.nombre, r.t.App.Name, tema.TextoRaiz, ui.SemiNegrita, cabe)
	ui.ColocarTexto(r.nombre, tema.TextoRaiz, x, y)
	y += tema.TextoRaiz.Linea
	for i, d := range r.desc {
		if i > 0 { // `marco:line-clamp-1`
			d.Hide()
			continue
		}
		d.Show()
		ui.Truncar(d, r.t.App.Description, EscalonDescripcion, ui.Normal, cabe)
		ui.ColocarTexto(d, EscalonDescripcion, x, y)
	}
}

func (r *teselaConectadaRenderer) Layout(t fyne.Size) {
	r.aro.Colocar(t)
	r.sombra.Ajustar(fyne.NewPos(0, 0), t, r.v)
	r.caja.Redimensionar(t)
	if tema.EsMarco() {
		r.colocarCompacta(t)
		return
	}
	x, y := tema.Borde+MargenTesela, tema.Borde+MargenTesela
	cabe := t.Width - 2*x
	r.colocarMarca(x, y, iconos.MarcaLG)
	y += iconos.MarcaLG + HuecoTesela
	ui.Truncar(r.nombre, r.t.App.Name, tema.TextoRaiz, ui.SemiNegrita, cabe)
	ui.ColocarTexto(r.nombre, tema.TextoRaiz, x, y)
	y += tema.TextoRaiz.Linea
	lineas := ui.Repartir(r.t.App.Description, EscalonDescripcion, ui.Normal, cabe)
	for i, d := range r.desc {
		if i >= len(lineas) {
			d.Hide()
			continue
		}
		d.Show()
		texto := lineas[i]
		if i == LineasDescripcion-1 && len(lineas) > LineasDescripcion {
			texto = juntar(lineas[i:])
		}
		ui.Truncar(d, texto, EscalonDescripcion, ui.Normal, cabe)
		ui.ColocarTexto(d, EscalonDescripcion, x, y)
		y += EscalonDescripcion.Linea
	}
}

// Refresh repinta con el tema, la forma, el puntero y el foco, como la de producto.
func (r *teselaConectadaRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r.v = v
	r.sombra.CambiarRadio(radioTesela())
	r.caja.CambiarRadio(radioTesela())
	r.aro.CambiarRadio(radioTesela())
	r.caja.Pintar(tema.Color("surface", v), r.t.borde(v))
	r.aro.Pintar(v)
	r.aro.Ver(r.t.Enfocado())
	r.distintivo.FillColor = tema.Color("bg-subtle", v)
	r.distintivo.StrokeColor = tema.Color("border-subtle", v)
	r.distintivo.Refresh()
	if r.inicial != nil {
		r.inicial.Color = tema.Color("text-muted", v)
		r.inicial.Refresh()
	}
	r.nombre.Color = tema.Color("text", v)
	r.nombre.Refresh()
	for _, d := range r.desc {
		d.Color = tema.Color("text-muted", v)
		d.Refresh()
	}
	r.Layout(r.t.Size())
}
