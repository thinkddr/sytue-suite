// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

// paridad-web: apps/frontend/packages/ui/src/components/ConfirmDialog.tsx

import (
	"image/color"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/nucleo/tokens"
)

// Margen es el `padding` de la web como `fyne.Layout`: coloca UN hijo dejando ese aire
// alrededor. Los cuatro lados por separado porque la hoja los pide así (`px-6 py-4`).
//
// Es propio y no `layout.NewPaddedLayout()` porque el de serie usa `theme.Padding()` —
// que el ajuste de tamaño de letra del sistema mueve— y es igual por los cuatro lados.
type Margen struct{ Izq, Arr, Der, Abj float32 }

var _ fyne.Layout = Margen{}

// MinSize suma el margen a lo que pida el hijo.
func (m Margen) MinSize(objs []fyne.CanvasObject) fyne.Size {
	var interior fyne.Size
	for _, o := range objs {
		if o.Visible() {
			interior = interior.Max(o.MinSize())
		}
	}
	return fyne.NewSize(interior.Width+m.Izq+m.Der, interior.Height+m.Arr+m.Abj)
}

// AltoCon mide el contenido después de descontar el margen horizontal.
func (m Margen) AltoCon(objs []fyne.CanvasObject, ancho float32) float32 {
	alto := float32(0)
	for _, o := range objs {
		if o.Visible() {
			alto = max(alto, AltoA(o, max(ancho-m.Izq-m.Der, 0)))
		}
	}
	return alto + m.Arr + m.Abj
}

// Layout deja al hijo todo el hueco menos el margen.
func (m Margen) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	for _, o := range objs {
		if !o.Visible() {
			continue
		}
		o.Move(fyne.NewPos(m.Izq, m.Arr))
		o.Resize(fyne.NewSize(max(t.Width-m.Izq-m.Der, 0), max(t.Height-m.Arr-m.Abj, 0)))
	}
}

// TallaDialogo son los tamaños de Modal.tsx. El valor cero es `md`, como allí.
type TallaDialogo uint8

// ColocacionDialogo conserva el centrado por defecto y permite el panel lateral.
type ColocacionDialogo uint8

const (
	DialogoCentrado ColocacionDialogo = iota
	DialogoDerecha
)

// Las cuatro tallas.
const (
	DialogoMD TallaDialogo = iota
	DialogoSM
	DialogoLG
	DialogoXL
)

// Los anchos MÁXIMOS de `sizes` en Modal.tsx. Van en px literales en la hoja
// (`max-w-[400px]`), así que el rem de 14 no los toca. Por debajo de eso el panel es
// `w-full`.
var anchosDialogo = [...]float32{
	DialogoMD: 560,
	DialogoSM: 400,
	DialogoLG: 720,
	DialogoXL: 1120,
}

// Las medidas de Modal.tsx, en px de CSS.
const (
	// MargenDialogo es el `md:p-4` del contenedor que centra: el aire mínimo entre el
	// panel y el borde de la ventana, por los cuatro lados. En el teléfono no hay: la
	// hoja va a todo el ancho y apoyada abajo.
	MargenDialogo = 4 * 0.25 * tema.Rem
	// FraccionAltoDialogo es `md:max-h-[90%]`. Y es un TOPE, NO UNA MEDIDA: el panel es
	// `flex-col` sin alto fijo, así que se ajusta a su contenido y solo lo recorta
	// cuando se pasa. Tomarlo como alto sacaba a pantalla completa una confirmación
	// de dos líneas —690 px medidos donde tocaban 176—, y es el fallo que
	// `TestElDialogoSeAjustaAlContenidoYNoAlTope` vigila por los dos lados.
	FraccionAltoDialogo = 0.9
	// HuecoHoja es el aire que la hoja deja ARRIBA en el teléfono (`max-md:pt-[2.5rem]`):
	// lo que hace que se lea como una capa encima de la pantalla y no como otra pantalla.
	HuecoHoja = 2.5 * tema.Rem
	// CierreArrastre es `CIERRE_ARRASTRE` de Modal.tsx: lo que hay que bajar la hoja con
	// el dedo para cerrarla.
	CierreArrastre float32 = 80
	// VelocidadCierre es `VELOCIDAD_CIERRE` de Modal.tsx, en px por milisegundo: un tirón
	// rápido cierra aunque no llegue a `CierreArrastre`, como las hojas del sistema en iOS
	// y Android. TironMinimo es lo mínimo que tiene que bajar para que cuente como tirón
	// y no como un toque que tiembla.
	VelocidadCierre float32 = 0.5
	TironMinimo     float32 = 24
)

// cierraArrastre es la regla de Modal.tsx al soltar la hoja: lejos, o rápido.
func cierraArrastre(dy float32, dur time.Duration) bool {
	if dy > CierreArrastre {
		return true
	}
	ms := float32(dur.Milliseconds())
	return dy > TironMinimo && ms > 0 && dy/ms > VelocidadCierre
}

var (
	margenXDialogo    = tema.Espacio(6) // px-6 de cabecera, cuerpo y pie: 24
	margenArrCabecera = tema.Espacio(5) // pt-5 de la cabecera: 20
	margenAbjCabecera = tema.Espacio(2) // pb-2 de la cabecera: 8
	margenYCuerpo     = tema.Espacio(3) // py-3 del cuerpo: 12
	margenArrPie      = tema.Espacio(2) // pt-2 del pie: 8
	// margenFinal es el `pb-5` de la ÚLTIMA banda: el pie, o el cuerpo si no hay pie.
	margenFinal     = tema.Espacio(5)
	huecoCabecera   = tema.Espacio(3) // gap-3 entre el título y el aspa: 12
	HuecoPieDialogo = tema.Espacio(2) // gap-2 entre las acciones del pie: 8
	// El aspa es `<Button variant="ghost" size="sm" iconOnly>`: `h-8 w-8` con radio 6.
	ladoAspa = tema.Espacio(8)
	// El asa de la hoja: `pt-2` y una píldora `h-1 w-9`; con asa, la cabecera es `pt-2`.
	margenAsa = tema.Espacio(2)
	altoAsa   = tema.Espacio(1)
	anchoAsa  = tema.Espacio(9)
	// TituloDialogo es `sytue-h4`: `--text-lg` semi-negrita, o sea 18 / 26.
	TituloDialogo = tema.TextoLG
)

// AltoCabeceraDialogo es `pt-5 pb-2`, la fila de 32 (el aspa) y el filete inferior de
// 1 px: 61. Es la cabecera de escritorio; la de la hoja la mide altoCabecera.
func AltoCabeceraDialogo() float32 {
	return margenArrCabecera + max(TituloDialogo.Linea, SueloTactil(ladoAspa)) + margenAbjCabecera + tema.Borde
}

// enHoja dice si en un hueco de `lienzo` el diálogo es la hoja del teléfono: el `max-md:`
// de Modal.tsx. Un lienzo de cero (sin ventana todavía) no es hoja.
func enHoja(lienzo fyne.Size) bool { return lienzo.Width > 0 && lienzo.Width < Estrecho }

// bordesDialogo son los filetes del PANEL: el de los lados y el de abajo. La hoja solo
// lleva el de arriba (`max-md:border-x-0 max-md:border-b-0`): toca los bordes de la
// pantalla y uno ahí sería una raya contra el canto.
func bordesDialogo(lienzo fyne.Size) (lado, abajo float32) {
	if enHoja(lienzo) {
		return 0, 0
	}
	return tema.Borde, tema.Borde
}

// lineasTitulo es el título repartido en lo que le deja la fila a unas bandas de `util`
// de ancho interior (anchoUtil): el `<h2>` de la web pasa de línea, y la banda crece con
// él.
func lineasTitulo(titulo string, util float32) []string {
	hueco, _ := filaCabecera(titulo, util)
	if l := Repartir(titulo, TituloDialogo, SemiNegrita, max(hueco, 0)); len(l) > 0 {
		return l
	}
	return []string{""}
}

// filaCabecera conserva el aspa completa (`shrink-0`) y reparte el título en el
// espacio restante (`min-w-0`). El cierre mantiene su blanco táctil de 44 px. `util` es
// el ancho interior de las bandas: el panel menos sus filetes —la hoja no los lleva a
// los lados— y el `px-6`.
func filaCabecera(_ string, util float32) (hueco, aspa float32) {
	fila := max(util-huecoCabecera, 0)
	aspa = SueloTactil(ladoAspa)
	return max(fila-aspa, 0), aspa
}

// altoCabecera es AltoCabeceraDialogo con el título en las líneas que ocupe y, en la
// hoja, el asa encima.
func altoCabecera(d *Dialogo, ancho float32, lienzo fyne.Size) float32 {
	titulo := float32(len(lineasTitulo(d.Titulo, anchoUtilEn(ancho, lienzo)))) * TituloDialogo.Linea
	return d.arribaCabecera(lienzo) + max(titulo, SueloTactil(ladoAspa)) + margenAbjCabecera + tema.Borde
}

// AltoPieDialogo es lo que ocupa el pie: su filete de `border-t`, el `pt-2 pb-5` y lo
// que midan las acciones. Sin pie, cero, como en la web (`footer ? … : null`).
//
// No es una constante: con una botonera de talla `md` (36) mide 65; con `sm`, 61.
func AltoPieDialogo(pie fyne.CanvasObject) float32 {
	if pie == nil {
		return 0
	}
	return tema.Borde + margenArrPie + pie.MinSize().Height + margenFinal
}

func altoPieDialogo(pie fyne.CanvasObject, ancho float32, hoja bool) float32 {
	if pie == nil {
		return 0
	}
	alto := AltoA(pie, ancho)
	if hoja {
		alto = max(alto, altoAccionHoja(pie))
	}
	return tema.Borde + margenArrPie + alto + margenFinal
}

// Las botoneras corrientes (`Fila`) pasan a PieAcciones: conservan el orden y la
// alineación y pueden partir como flex-wrap. Un pie con layout propio conserva su
// reparto (PieAncho).
func prepararPieDialogo(pie fyne.CanvasObject) {
	if c, ok := pie.(*fyne.Container); ok {
		if fila, ok := c.Layout.(Fila); ok {
			c.Layout = PieAcciones{Hueco: fila.Hueco}
		}
	}
}

// marcarPie dice al pie si va en la hoja del teléfono: es el `max-md:` del pie de
// Modal.tsx, y decide su reparto antes de medirlo.
func marcarPie(pie fyne.CanvasObject, hoja bool) {
	if c, ok := pie.(*fyne.Container); ok {
		if p, ok := c.Layout.(PieAcciones); ok && p.hoja != hoja {
			p.hoja = hoja
			c.Layout = p
		}
	}
}

// PieAcciones es el pie de un diálogo: el `footer` de Modal.tsx con sus acciones SUELTAS
// (sin envoltorio), el principal el último.
//
//   - Escritorio: `flex-wrap justify-end gap-2`, cada acción con su ancho y pegadas a la
//     derecha; las `Izquierda` primeras, a la izquierda (el `md:mr-auto` de la última de
//     ellas: «Eliminar» en el formulario de un evento).
//   - Hoja del teléfono (`PIE_TELEFONO`): a todo el ancho, de dos en dos y a partes iguales,
//     con 44 de alto. Cada una parte de media fila y crece; la que no cabe en media fila
//     (su mínimo es su ancho natural) baja a su propia fila. Con un número impar, la
//     primera va sola arriba.
//
// Hueco es el `gap-2`; con 0, HuecoPieDialogo.
type PieAcciones struct {
	Hueco     float32
	Izquierda int
	hoja      bool
}

var (
	_ fyne.Layout = PieAcciones{}
	_ conAlto     = PieAcciones{}
)

func (p PieAcciones) hueco() float32 {
	if p.Hueco > 0 {
		return p.Hueco
	}
	return HuecoPieDialogo
}

func (p PieAcciones) fila() FilaAuto {
	f := FilaAuto{Hueco: p.hueco(), Partir: true, SaltoFila: HuecoPieDialogo, AlFinal: p.Izquierda == 0}
	if p.Izquierda > 0 {
		f.Derecha = p.Izquierda
	}
	return f
}

// MinSize: el ancho no impone nada (parte, como la fila de escritorio) y el alto es el de
// una fila; el de verdad depende del ancho y lo da AltoCon.
func (p PieAcciones) MinSize(objs []fyne.CanvasObject) fyne.Size {
	if !p.hoja {
		return p.fila().MinSize(objs)
	}
	alto := float32(0)
	for _, o := range visiblesPie(objs) {
		alto = max(alto, altoAccionHoja(o))
	}
	return fyne.NewSize(0, alto)
}

// AltoCon es el alto de sus filas a ese ancho, con el salto entre ellas.
func (p PieAcciones) AltoCon(objs []fyne.CanvasObject, ancho float32) float32 {
	if !p.hoja {
		return p.fila().AltoCon(objs, ancho)
	}
	alto := float32(0)
	for i, f := range p.filasHoja(visiblesPie(objs), ancho) {
		if i > 0 {
			alto += HuecoPieDialogo
		}
		alto += altoFilaHoja(f.objs)
	}
	return alto
}

// Layout reparte las acciones en el ancho del pie.
func (p PieAcciones) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	if !p.hoja {
		p.fila().Layout(objs, t)
		return
	}
	y := float32(0)
	for _, f := range p.filasHoja(visiblesPie(objs), t.Width) {
		h := altoFilaHoja(f.objs)
		x := float32(0)
		for i, o := range f.objs {
			o.Resize(fyne.NewSize(f.anchos[i], h)) // `items-stretch`
			o.Move(fyne.NewPos(x, y))
			x += f.anchos[i] + p.hueco()
		}
		y += h + HuecoPieDialogo
	}
}

// filaHoja es una fila del pie del teléfono: sus acciones y lo que se lleva cada una.
type filaHoja struct {
	objs   []fyne.CanvasObject
	anchos []float32
}

// filasHoja es el `flex-wrap` del pie del teléfono: base de media fila (50 % menos medio
// hueco), mínimo el ancho natural de cada acción, la primera de un número impar sola, y
// lo que sobra en una fila, repartido a partes iguales (`flex-grow` 1).
func (p PieAcciones) filasHoja(objs []fyne.CanvasObject, ancho float32) []filaHoja {
	g := p.hueco()
	mitad := max((ancho-g)/2, 0)
	base := func(i int) float32 {
		if i == 0 && len(objs)%2 == 1 {
			return ancho // `basis-full`
		}
		return min(max(mitad, objs[i].MinSize().Width), ancho)
	}
	var filas []filaHoja
	for i := 0; i < len(objs); {
		f := filaHoja{objs: []fyne.CanvasObject{objs[i]}, anchos: []float32{base(i)}}
		usado := base(i)
		j := i + 1
		for j < len(objs) && usado+g+base(j) <= ancho+0.01 {
			f.objs, f.anchos = append(f.objs, objs[j]), append(f.anchos, base(j))
			usado += g + base(j)
			j++
		}
		if sobra := ancho - usado; sobra > 0 {
			for k := range f.anchos {
				f.anchos[k] += sobra / float32(len(f.anchos))
			}
		}
		filas = append(filas, f)
		i = j
	}
	return filas
}

// visiblesPie son las acciones que se ven: la web no pinta las que no van
// (`{isEdit ? … : null}`), así que tampoco cuentan para las parejas.
func visiblesPie(objs []fyne.CanvasObject) []fyne.CanvasObject {
	v := make([]fyne.CanvasObject, 0, len(objs))
	for _, o := range objs {
		if o.Visible() {
			v = append(v, o)
		}
	}
	return v
}

// altoAccionHoja es el `min-h-11` de un botón del pie del teléfono: 44, con dedo o sin él.
func altoAccionHoja(o fyne.CanvasObject) float32 {
	if _, es := o.(*Boton); es {
		return max(o.MinSize().Height, SueloTactilBarra)
	}
	return o.MinSize().Height
}

func altoFilaHoja(objs []fyne.CanvasObject) float32 {
	alto := float32(0)
	for _, o := range objs {
		alto = max(alto, altoAccionHoja(o))
	}
	return alto
}

// MedidaDialogo es lo que va a ocupar el panel dentro de un hueco de `lienzo`. Como en
// el botón y en la tarjeta, es la ÚNICA cuenta: el renderer la llama y los guardianes
// comparan contra ella y contra los píxeles.
//
// Un `lienzo` de cero es «sin ventana todavía»: se devuelve la medida natural, sin
// aplicar ni el `max-w` de la talla ni el `max-h-[90%]`.
//
// Por debajo de 768 es la HOJA, centrada o lateral: todo el ancho, ajustada a su
// contenido y con `HuecoHoja` libre arriba como mucho.
func MedidaDialogo(d *Dialogo, lienzo fyne.Size) fyne.Size {
	ancho := anchoPanel(d.Talla, lienzo)
	hoja := enHoja(lienzo)
	marcarPie(d.Pie, hoja)
	_, abajo := bordesDialogo(lienzo)
	util := anchoUtilEn(ancho, lienzo)
	alto := tema.Borde + abajo + altoCabecera(d, ancho, lienzo) + altoPieDialogo(d.Pie, util, hoja) + d.altoCuerpo(util)
	switch {
	case lienzo.Height <= 0:
	case hoja:
		alto = min(alto, lienzo.Height-HuecoHoja)
	case d.Colocacion == DialogoDerecha:
		alto = lienzo.Height - 2*MargenDialogo
	default:
		alto = min(alto, FraccionAltoDialogo*(lienzo.Height-2*MargenDialogo))
	}
	return fyne.NewSize(max(ancho, 0), max(alto, 0))
}

// Campo lo implementan los controles donde se ESCRIBE o se ELIGE, que es donde tiene
// que caer el foco al abrir un diálogo.
//
// Es la lista `CAMPO` de Modal.tsx traída a Go: input de texto, área de texto, select y
// editable. Deliberadamente NO son campos las casillas, los radios, el selector de
// fichero, el rango ni el color —un diálogo que arranca con una casilla enfocada parece
// que pide marcarla—, así que esos controles no deben implementarla.
//
// Un control que no la implemente sigue siendo enfocable con Tab; solo renuncia a
// recibir el foco INICIAL, y el diálogo cae en el aspa, que es también lo que hace la
// web cuando no encuentra ningún campo.
type Campo interface {
	fyne.Focusable
	// EsCampo no hace nada: marca el tipo.
	EsCampo()
}

// Dialogo es Modal.tsx: el PANEL, sin el velo ni el centrado, que son de la capa.
//
// Se abre con Mostrar, que lo apila en las superposiciones del lienzo con su velo y le
// da el foco. Los 120 diálogos del inventario son 120 usos de este componente, no 120
// diseños: lo que cambia entre ellos es el título, la talla y lo que va dentro.
type Dialogo struct {
	widget.BaseWidget
	Titulo string
	Talla  TallaDialogo
	// Colocacion es placement: centro por defecto o panel derecho con pie fijo.
	Colocacion ColocacionDialogo
	// Cuerpo es lo que va con el `px-6 py-5` alrededor. Se desplaza si no cabe.
	Cuerpo fyne.CanvasObject
	// Pie son las acciones, pegadas a la derecha y con el primario EL ÚLTIMO
	// (medido: «Guardar borrador / Enviar»). nil: sin pie ni filete, como en la web.
	Pie fyne.CanvasObject
	// PieAncho estira el pie a todo el ancho del panel: es el pie que trae su propio
	// `flex w-full justify-between` —«Eliminar» a la izquierda y el resto a la
	// derecha—, que en la web ocupa la fila entera dentro del `justify-end`.
	PieAncho bool
	// RotuloCerrar es el nombre accesible del aspa, YA traducido («ui.close»).
	RotuloCerrar string
	// AlCerrar es `onClose`: lo llaman el velo, Escape y el aspa. Mostrar lo rellena
	// con Cerrar si viene a nil, así que un diálogo corriente funciona sin tocarlo.
	AlCerrar func()
	// AlPulsarAspa es `onCloseButtonClick`: SOLO el aspa, si viene. Existe para el
	// redactor de Sytue Mail —clic fuera y Escape autoguardan, el aspa pregunta— y
	// ningún otro diálogo de la suite lo necesita.
	AlPulsarAspa func()
	// NoDescartable es `dismissable={false}`: el clic en el velo deja de cerrar.
	// Escape y el aspa siguen pidiendo el cierre; en la web también.
	NoDescartable bool
	// Foco es dónde cae el foco al abrir. Vacío: el primer Campo del cuerpo y, si no
	// hay ninguno, el aspa.
	Foco fyne.Focusable
	// clase es la pregunta que hace una Confirmacion o una Pregunta: una nueva de la
	// misma clase CANCELA la que estuviera abierta, como `useConfirm`/`usePrompt`.
	clase claseDialogo

	lienzo fyne.Size // el hueco disponible; lo fija la capa al colocar
	aspa   *Boton
	previo fyne.Focusable
	capa   *capa
	// desplazador es el `overflow-y-auto` del cuerpo, y margen su `px-6 py-5`, dentro: se crean
	// con el diálogo y no con su renderer, para que `Desplazador` y `CambiarCuerpo`
	// valgan antes de pintarlo.
	desplazador *container.Scroll
	margen      *fyne.Container
	fluido      *fyne.Container
	r           *dialogoRenderer
	sinParada
	// Los dos botones de una Confirmacion, para `Ocupado` y `SinConfirmar`.
	cancelar, confirmar *Boton
	// arrastre es lo que el dedo ha bajado la hoja; arrastrando, que el gesto empezó en
	// el asa o la cabecera y es suyo; ajeno, que empezó en otro sitio y no lo es.
	arrastre           float32
	arrastrando, ajeno bool
	inicioArrastre     time.Time
	// teclado es lo que el teclado del teléfono tapa ahora desde abajo de la pantalla
	// (lo pone seguirTeclado): la hoja se apoya ENCIMA de él, como en la web, donde el
	// contenedor mide el viewport visual.
	teclado float32
}

// conAsa: la hoja lleva asa si se puede bajar con el dedo, que es lo mismo que cerrarse
// con un clic fuera (`dismissable`).
func (d *Dialogo) conAsa(lienzo fyne.Size) bool { return enHoja(lienzo) && !d.NoDescartable }

// arribaCabecera es lo que hay encima de la fila del título: `pt-5`, o el asa y `pt-2`.
func (d *Dialogo) arribaCabecera(lienzo fyne.Size) float32 {
	if d.conAsa(lienzo) {
		return margenAsa + altoAsa + margenAsa
	}
	return margenArrCabecera
}

// anchoUtil es el ancho de lo que va dentro de las bandas: el panel menos sus filetes y
// el `px-6`.
func (d *Dialogo) anchoUtil(ancho float32) float32 { return anchoUtilEn(ancho, d.lienzo) }

// AnchoCuerpoDialogo es el ancho de lo que va dentro del cuerpo de `d` en un hueco de
// `lienzo`: el panel menos sus filetes —la hoja del teléfono no los lleva a los lados— y
// el `px-6`. Es la cuenta que cada pantalla hacía a mano para repartir su cuerpo, y con
// la hoja se quedaba 2 px corta: las pestañas de «Firmar» de CertiPDF salían 1 px más
// estrechas que en la web.
func AnchoCuerpoDialogo(d *Dialogo, lienzo fyne.Size) float32 {
	return AnchoCuerpoTalla(d.Talla, lienzo)
}

// AnchoCuerpoTalla es AnchoCuerpoDialogo antes de que exista el diálogo: lo que necesita
// quien reparte el cuerpo al construirlo. Un ancho de escritorio fijo (560 menos lo suyo)
// pintado en la hoja de 390 dejaba los textos medidos para 510 en un hueco de 342.
func AnchoCuerpoTalla(t TallaDialogo, lienzo fyne.Size) float32 {
	return anchoUtilEn(anchoPanel(t, lienzo), lienzo)
}

// anchoPanel es el del panel: la hoja, todo el lienzo; si no, su talla sin pasar del
// lienzo menos el margen.
func anchoPanel(t TallaDialogo, lienzo fyne.Size) float32 {
	ancho := anchosDialogo[t]
	switch {
	case enHoja(lienzo):
		ancho = lienzo.Width
	case lienzo.Width > 0:
		ancho = min(ancho, lienzo.Width-2*MargenDialogo)
	}
	return max(ancho, 0)
}

func anchoUtilEn(ancho float32, lienzo fyne.Size) float32 {
	lado, _ := bordesDialogo(lienzo)
	return max(ancho-2*lado-2*margenXDialogo, 0)
}

// margenCuerpo es el `px-6 py-3` del cuerpo, o `pb-5` abajo si es la última banda.
func (d *Dialogo) margenCuerpo() Margen {
	abajo := margenYCuerpo
	if d.Pie == nil {
		abajo = margenFinal
	}
	return Margen{Izq: margenXDialogo, Arr: margenYCuerpo, Der: margenXDialogo, Abj: abajo}
}

// altoCuerpo es lo que pide el cuerpo con su margen, a `util` de ancho interior.
func (d *Dialogo) altoCuerpo(util float32) float32 {
	if d.Cuerpo == nil {
		return 0
	}
	m := d.margenCuerpo()
	return AltoA(d.Cuerpo, util) + m.Arr + m.Abj
}

// claseDialogo distingue las dos preguntas imperativas de la web, que llevan cada una su
// hook y su «pendiente»: una confirmación nueva cancela la confirmación abierta, pero no
// una pregunta, y al revés.
type claseDialogo uint8

const (
	dialogoCorriente claseDialogo = iota
	claseConfirmar                // useConfirm.tsx
	clasePreguntar                // usePrompt.tsx
)

// cancelarPendiente es el `actual.current?.resolver(false)` de useConfirm: si ya había
// una pregunta de la misma clase abierta en la ventana, se contesta «cancelar» antes de
// abrir la nueva. Dos diálogos encima del otro no se pueden contestar, y el de debajo
// se quedaba esperando una respuesta que no iba a llegar.
func (d *Dialogo) cancelarPendiente(c fyne.Canvas) {
	if d.clase == dialogoCorriente {
		return
	}
	for _, o := range append([]fyne.CanvasObject{}, c.Overlays().List()...) {
		cp, es := o.(*capa)
		if !es {
			continue
		}
		if otro, es := cp.panel.(*Dialogo); es && otro != d && otro.clase == d.clase {
			otro.Descartar()
			otro.Cerrar() // por si su `alCancelar` no lo cierra
		}
	}
}

// sinParada es el `tabIndex={-1}` del panel: se le puede dar el foco por código, pero
// no es una parada del Tab. Fyne no distingue las dos cosas: un `Focusable` entra en la
// cadena de Tab y el panel era una parada invisible. Lo que sí salta la cadena es un
// `Disableable` inhabilitado, así que el panel se declara inhabilitado SALVO en la
// pregunta que hace `Focus` al dárselo a mano (`dar`). Que la siguiente conteste otra
// vez «inhabilitado» apaga además el teclado virtual del teléfono, que un panel no pide.
type sinParada struct{ pase bool }

// Disabled es true salvo una vez, justo después de `dar`.
func (s *sinParada) Disabled() bool {
	if s.pase {
		s.pase = false
		return false
	}
	return true
}

// Enable y Disable no hacen nada: el estado lo lleva `dar`.
func (s *sinParada) Enable()  {}
func (s *sinParada) Disable() {}

// dar le da el foco al panel `yo` en `c`.
func (s *sinParada) dar(c fyne.Canvas, yo fyne.Focusable) {
	s.pase = true
	enfocar(c, yo)
	s.pase = false
}

// Ocupado es el `busy` de ConfirmDialog: cancelar se apaga y confirmar sale con su
// rueda (`disabled={busy}` y `loading={busy}`). Solo en una Confirmacion.
func (d *Dialogo) Ocupado(si bool) {
	if d.confirmar == nil {
		return
	}
	d.cancelar.Inerte, d.confirmar.Cargando = si, si
	d.cancelar.Refresh()
	d.confirmar.Refresh()
}

// SinConfirmar es `confirmDisabled`: falta algo para poder confirmar.
func (d *Dialogo) SinConfirmar(si bool) {
	if d.confirmar == nil {
		return
	}
	d.confirmar.Inerte = si
	d.confirmar.Refresh()
}

var (
	_ fyne.Widget      = (*Dialogo)(nil)
	_ fyne.Tappable    = (*Dialogo)(nil)
	_ fyne.Focusable   = (*Dialogo)(nil)
	_ fyne.Disableable = (*Dialogo)(nil)
	_ fyne.Draggable   = (*Dialogo)(nil)
	_ Descartable      = (*Dialogo)(nil)
)

// NuevoDialogo crea un diálogo de talla md.
func NuevoDialogo(titulo string, cuerpo fyne.CanvasObject) *Dialogo {
	d := &Dialogo{Titulo: titulo, Cuerpo: cuerpo, RotuloCerrar: textoComun("ui.close")}
	d.ExtendBaseWidget(d)
	return d
}

// Desplazador es el desplazamiento del cuerpo (`overflow-y-auto`). Lo necesita quien
// mete dentro algo que se desplaza de lado —una tabla ancha del panel de Sytue AI— para
// no mover las dos cosas con el mismo gesto.
func (d *Dialogo) Desplazador() *container.Scroll {
	if d.desplazador == nil {
		// El margen va DENTRO del desplazamiento, como en la web: `px-6 py-5` está en
		// el mismo elemento que `overflow-y-auto`, así que el aire de abajo se desplaza
		// con el contenido y no queda pegado al pie.
		d.margen = container.New(d.margenCuerpo())
		if d.Cuerpo != nil {
			d.fluido = AlAnchoFluido(d.anchoUtil(MedidaDialogo(d, d.lienzo).Width), d.Cuerpo)
			d.margen.Objects = []fyne.CanvasObject{d.fluido}
		}
		d.desplazador = container.NewVScroll(d.margen)
	}
	if d.r != nil && d.r.desbordando && d.r.desborde != nil {
		return d.r.desborde
	}
	return d.desplazador
}

// CambiarCuerpo pone otro cuerpo en el diálogo, abierto o no, y lo vuelve a medir: el
// paso siguiente de un asistente sin cerrar y abrir otro diálogo.
func (d *Dialogo) CambiarCuerpo(cuerpo fyne.CanvasObject) {
	d.Cuerpo = cuerpo
	d.Desplazador()
	d.margen.Objects = nil
	if cuerpo != nil {
		d.fluido = AlAnchoFluido(d.anchoUtil(MedidaDialogo(d, d.lienzo).Width), cuerpo)
		d.margen.Objects = []fyne.CanvasObject{d.fluido}
	}
	d.desplazador.Offset = fyne.Position{}
	d.cambiado()
}

// CambiarPie pone otro pie (o ninguno, con nil) y vuelve a medir.
func (d *Dialogo) CambiarPie(pie fyne.CanvasObject) {
	prepararPieDialogo(pie)
	d.Pie = pie
	d.cambiado()
}

func (d *Dialogo) cambiado() {
	if d.r != nil {
		d.r.rehacer()
		d.r.Layout(d.Size())
	}
	d.Desplazador().Refresh()
	d.Recolocar()
}

// Con devuelve el mismo diálogo con otra talla y su pie, para que una pantalla se lea
// como el JSX del que sale.
func (d *Dialogo) Con(t TallaDialogo, pie fyne.CanvasObject) *Dialogo {
	prepararPieDialogo(pie)
	d.Talla, d.Pie = t, pie
	return d
}

// ALaDerecha elige el mismo panel de Modal placement="right".
func (d *Dialogo) ALaDerecha() *Dialogo {
	d.Colocacion = DialogoDerecha
	return d
}

// radio es el `rounded-card` del panel: el de las tarjetas de la forma (14 en Carril, 16
// en Marco), centrado o lateral. Era `rounded-xl` (20), la superficie más redonda de la
// suite. En la hoja es el mismo, pero solo arriba (ver `Layout`).
func (d *Dialogo) radio(fyne.Size) float32 { return tema.Forma("radius-card") }

// esquinaRecta es «sin redondear» para una esquina suelta de `canvas.Rectangle`.
//
// ponytail: Fyne lee un radio de esquina 0 como «el del rectángulo» (`GetCornerRadius`),
// así que la esquina recta de la hoja se pide con un radio que no se ve.
const esquinaRecta = 1e-3

// Mostrar apila el diálogo sobre el lienzo con su velo, lo centra y le da el foco.
//
// El velo se crea AQUÍ y se retira en Cerrar: no hay ninguna capa puesta mientras no
// haya diálogo. Una capa permanente —aunque fuese transparente— deja la aplicación
// entera sin poder pulsar, y es un fallo que no se ve en una captura.
func (d *Dialogo) Mostrar(c fyne.Canvas) {
	if c == nil || d.capa != nil {
		return
	}
	d.cancelarPendiente(c)
	if d.AlCerrar == nil {
		d.AlCerrar = d.Cerrar
	}
	var alPulsarVelo func()
	if !d.NoDescartable {
		alPulsarVelo = d.Descartar
	}
	d.capa = nuevaCapa(NuevoVelo(true, alPulsarVelo), d, func(lienzo fyne.Size) (fyne.Position, fyne.Size) {
		if enHoja(lienzo) {
			lienzo.Height -= d.tapadoPorTeclado(lienzo)
		}
		d.lienzo = lienzo
		tam := MedidaDialogo(d, lienzo)
		switch {
		case enHoja(lienzo):
			// `max-md:items-end`: la hoja apoyada abajo. Lo que el dedo la haya bajado se
			// suma aquí, y al soltar vuelve a cero.
			return fyne.NewPos(0, lienzo.Height-tam.Height+d.arrastre), tam
		case d.Colocacion == DialogoDerecha:
			return fyne.NewPos(lienzo.Width-MargenDialogo-tam.Width, MargenDialogo), tam
		}
		// `items-center justify-center` del contenedor que lo envuelve.
		return fyne.NewPos((lienzo.Width-tam.Width)/2, (lienzo.Height-tam.Height)/2), tam
	})
	// `md:shadow-[var(--shadow-xl)]` de Modal.tsx, con el radio del mismo panel. La hoja
	// no la lleva; si la ventana se estrecha con el diálogo abierto, la esconde `Layout`.
	//
	// ponytail: una ventana que se ensancha con la hoja abierta deja la tarjeta sin
	// sombra hasta el siguiente diálogo; crearla en `Layout` si alguna vez se nota.
	if !enHoja(c.Size()) {
		d.capa.sombra = NuevaSombra(tema.SombraXL, d.radio(c.Size())).SinInterior()
	}
	d.capa.alDescartar = d.Descartar
	d.previo = mostrar(c, d.capa)
	if enHoja(d.capa.Size()) {
		// Lo que queda debajo de la capa hasta el borde de la pantalla —la barra de inicio
		// del iPhone, que `InteractiveArea` deja fuera— es de la hoja, no de la pantalla
		// de debajo: la `env(safe-area-inset-bottom)` de la web.
		d.capa.faldon = canvas.NewRectangle(tema.Color("panel-surface", tema.De(fyne.CurrentApp().Settings().ThemeVariant())))
		d.capa.Refresh()
	}
	switch f := d.focoInicial(); {
	case f == fyne.Focusable(d):
		d.dar(c, d)
	case f == fyne.Focusable(d.aspa) && d.previo == nil:
		// `:focus-visible` hereda la modalidad: abierto con el ratón, el aspa recibe el
		// foco sin el anillo; con el teclado (había algo enfocado, y en Fyne el foco
		// solo llega por teclado o por código), con él. Ver `Boton.enfocarSinAro`.
		d.aspa.enfocarSinAro(c)
	default:
		enfocar(c, f)
	}
	// `sytue-modal-in`: 180 ms desde `translateY(10px) scale(.98)` y transparente, con la
	// opacidad de GRUPO del panel entero (ver fundido.go). La hoja sube desde abajo
	// (`sytue-hoja-in`, 220 ms desde `translateY(100%)`).
	switch {
	case d.capa.faldon != nil:
		d.capa.entrar(c, DuracionHoja, d.Size().Height, 1, 1)
	case d.Colocacion == DialogoDerecha:
		d.capa.entrarLateral(c, DuracionDialogo, 16)
	default:
		d.capa.entrar(c, DuracionDialogo, DesplazamientoDialogo, EscalaEntrada, 0.5)
	}
}

// Cerrar retira el diálogo y devuelve el foco a quien lo tenía.
//
// No anima: la web tampoco. `sytue-modal-in` solo tiene entrada y el componente se
// desmonta.
func (d *Dialogo) Cerrar() {
	if d.capa == nil {
		return
	}
	ocultar(lienzoDe(d.capa), d.capa, d.previo)
	d.capa, d.previo = nil, nil
}

// Recolocar vuelve a medir el diálogo abierto y a centrarlo cuando su cuerpo cambia de
// alto —una fila más, la línea de un error—: en la web lo hace el navegador, porque el
// panel no tiene alto fijo. Sin esto, un cuerpo que crecía al tope del `max-h-[90%]` no
// volvía a medirse, porque el panel no cambiaba de tamaño.
func (d *Dialogo) Recolocar() {
	if d.capa != nil {
		d.capa.Refresh()
	}
	// El pie se vuelve a repartir aunque no cambie de tamaño: un botón que cambia de
	// rótulo («Eliminar» → «Confirmar eliminación» en Contacts) se quedaba con su ancho
	// viejo y se leía «Confirmar», porque un contenedor del mismo tamaño no se recoloca.
	if d.Pie != nil {
		d.Pie.Refresh()
	}
}

// Descartar es `onClose`: lo piden el velo, Escape y el aspa. Quien lo recibe decide si
// cierra o pregunta antes.
func (d *Dialogo) Descartar() {
	if d.AlCerrar != nil {
		d.AlCerrar()
	}
}

// focoInicial es la regla de Modal.tsx: el primer CAMPO del cuerpo, si no el primer
// ENFOCABLE —que es el aspa, la primera del DOM— y, si ni eso, el panel
// (`(first ?? panel)?.focus()`).
//
// Hasta el 2026-09-23 el peldaño del aspa no se tomaba («abrir con el foco en cerrar
// invita a cerrarlo»); era una decisión documentada contra la web, y la paridad manda.
// Lo que sí se copia es que abrir con el ratón no enciende el anillo: ver Mostrar.
//
// Con el DEDO, el panel y nada más, como `conElDedo()` de la web: enfocar un campo saca
// el teclado encima de la hoja antes de que nadie haya decidido escribir. Un `Foco`
// puesto a mano manda igual.
//
// El recorrido baja por los `*fyne.Container` y se detiene en cada widget, que es donde
// viven los controles. Un control COMPUESTO que quiera el foco inicial implementa Campo
// él mismo; no se le entra dentro.
func (d *Dialogo) focoInicial() fyne.Focusable {
	if d.Foco != nil {
		return d.Foco
	}
	if EsTactil() {
		return d
	}
	if c := primerCampo(d.Cuerpo); c != nil {
		return c
	}
	if d.aspa != nil {
		return d.aspa
	}
	return d
}

func primerCampo(o fyne.CanvasObject) fyne.Focusable {
	switch v := o.(type) {
	case nil:
		return nil
	case Campo:
		return v
	case *fyne.Container:
		for _, hijo := range v.Objects {
			if c := primerCampo(hijo); c != nil {
				return c
			}
		}
	}
	return nil
}

// Tapped no hace nada, y por eso está: se traga el clic que cae en el panel y fuera de
// un control, para que no lo recoja el velo de debajo y cierre el diálogo.
func (d *Dialogo) Tapped(*fyne.PointEvent) {}

// FocusGained y FocusLost no pintan nada: el panel recibe el foco para poder atender a
// Escape, no para verse enfocado. El anillo es de los controles.
func (d *Dialogo) FocusGained() {}

// FocusLost no hace nada.
func (d *Dialogo) FocusLost() {}

// TypedRune ignora el texto: el panel no escribe.
func (d *Dialogo) TypedRune(rune) {}

// TypedKey cierra con Escape y con el «atrás» del teléfono.
func (d *Dialogo) TypedKey(e *fyne.KeyEvent) {
	if EsCierre(e) {
		d.Descartar()
	}
}

// Dragged baja la hoja con el dedo, como el asa de Modal.tsx: solo en la hoja con asa,
// solo con el dedo y solo si el gesto EMPEZÓ en el asa o en la cabecera. El cuerpo
// tiene su propio desplazamiento, que es quien recibe el arrastre que empieza ahí.
func (d *Dialogo) Dragged(ev *fyne.DragEvent) {
	if d.ajeno || d.capa == nil || ev == nil {
		return
	}
	if !d.arrastrando {
		inicio := ev.Position.Y - ev.Dragged.DY
		if !d.conAsa(d.lienzo) || !EsTactil() || inicio > altoCabecera(d, d.Size().Width, d.lienzo) {
			d.ajeno = true
			return
		}
		d.arrastrando = true
		d.inicioArrastre = time.Now()
	}
	// Solo hacia abajo: hacia arriba la hoja ya está en su sitio.
	d.arrastre = max(0, d.arrastre+ev.Dragged.DY)
	d.capa.Refresh()
}

// DragEnd suelta la hoja: más de `CierreArrastre`, o un tirón rápido, pide el cierre
// (`onClose`, que puede preguntar antes); menos, vuelve a su sitio.
func (d *Dialogo) DragEnd() {
	cerrar := d.arrastrando && cierraArrastre(d.arrastre, time.Since(d.inicioArrastre))
	d.arrastre, d.arrastrando, d.ajeno = 0, false, false
	if d.capa != nil {
		d.capa.Refresh()
	}
	if cerrar {
		d.Descartar()
	}
}

// CreateRenderer construye la geometría. Solo primitivas y widgets de este paquete.
func (d *Dialogo) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &dialogoRenderer{
		d:              d,
		caja:           NuevaCaja(tema.Color("panel-surface", v), tema.Color("border", v), d.radio(d.lienzo)).ConTrazo(),
		fileteCabecera: canvas.NewRectangle(fileteDialogo(v)),
		titulo:         NuevoParrafoConTinta(d.Titulo, TituloDialogo, SemiNegrita, tema.Color("text", v)),
		asa:            canvas.NewRectangle(tema.Color("border-strong", v)),
	}
	r.asa.CornerRadius = altoAsa / 2
	// El aspa es, literalmente, el `<Button variant="ghost" size="sm" iconOnly>` de
	// Modal.tsx: azul de primary-text y los mismos estados del botón fantasma.
	d.aspa = &Boton{
		Variante: Fantasma, Talla: TallaSM, SoloIcono: true,
		Icono: iconos.Cerrar, TamIcono: iconos.TallaMD,
		Rotulo: d.RotuloCerrar, AlPulsar: d.pulsarAspa,
	}
	d.aspa.ExtendBaseWidget(d.aspa)

	r.cuerpo = d.Desplazador()
	// Sin la sombra de borde que Fyne pinta en un desplazamiento: en la web es un
	// `overflow-y-auto` a secas (petición de Sytue Calendar).
	r.envoltorio = container.NewThemeOverride(r.cuerpo, tema.SinSombras{})
	r.filete = canvas.NewRectangle(fileteDialogo(v))
	d.r = r
	r.rehacer()
	return r
}

func (d *Dialogo) pulsarAspa() {
	if d.AlPulsarAspa != nil {
		d.AlPulsarAspa()
		return
	}
	d.Descartar()
}

type dialogoRenderer struct {
	d              *Dialogo
	caja           *Caja
	fileteCabecera *canvas.Rectangle
	titulo         *Parrafo
	filete         *canvas.Rectangle
	// asa es la píldora de la hoja (`h-1 w-9 bg-border-strong`); fuera de la hoja no está.
	asa        *canvas.Rectangle
	cuerpo     *container.Scroll
	envoltorio *container.ThemeOverride // el cuerpo sin sombras de borde
	objetos    []fyne.CanvasObject
	// Con muy poca altura, cabecera y acciones tampoco caben juntas: entonces
	// se desplaza el panel entero en lugar de solaparlas y perder botones.
	partes          *fyne.Container
	desborde        *container.Scroll
	envoltorioPanel *container.ThemeOverride
	desbordando     bool
	altoContenido   float32
	// filetes dice si el cuerpo se desplaza: es lo único que enciende los filetes de
	// cabecera y pie, como `desborda` en la web.
	filetes bool
}

// rehacer arma los objetos según haya cuerpo y pie AHORA: `CambiarCuerpo` y
// `CambiarPie` los cambian con el diálogo abierto.
func (r *dialogoRenderer) rehacer() {
	d := r.d
	partes := []fyne.CanvasObject{r.fileteCabecera, r.titulo, d.aspa}
	if d.conAsa(d.lienzo) {
		partes = append(partes, r.asa)
	}
	if d.Cuerpo != nil {
		if r.desbordando {
			// Un solo desplazador bajo el dedo. Uno interior sin recorrido puede
			// quedarse el arrastre e impedir alcanzar el pie del panel exterior.
			partes = append(partes, d.margen)
		} else {
			partes = append(partes, r.envoltorio)
		}
	}
	if d.Pie != nil {
		partes = append(partes, r.filete, d.Pie)
	}
	if r.partes != nil {
		r.partes.Objects = partes
	}
	if r.desbordando {
		r.objetos = append(r.caja.Objetos(), r.envoltorioPanel)
	} else {
		r.objetos = append(r.caja.Objetos(), partes...)
	}
}

// El mínimo de este contenido es su altura completa, sin imponer el ancho de
// escritorio de un campo. El Scroll exterior lo deja todo alcanzable en poca altura.
type contenidoDialogo struct{ r *dialogoRenderer }

func (c contenidoDialogo) MinSize([]fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(0, c.r.altoContenido)
}

func (c contenidoDialogo) Layout(_ []fyne.CanvasObject, t fyne.Size) {
	c.r.disponer(t)
}

// IsClip marca el panel como recortante: es el `overflow-hidden` de la web. Sin esto,
// un cuerpo más ancho de la cuenta pinta por fuera del borde redondeado.
//
// Lo que Fyne recorta es un RECTÁNGULO, no la esquina redonda. Por eso la banda de
// tinta se redondea ella sola (ver CreateRenderer): el recorte no lo haría.
func (r *dialogoRenderer) IsClip() {}

func (r *dialogoRenderer) MinSize() fyne.Size { return MedidaDialogo(r.d, r.d.lienzo) }

func (r *dialogoRenderer) Objects() []fyne.CanvasObject { return r.objetos }

func (r *dialogoRenderer) Destroy() {}

// Layout reparte cabecera, cuerpo y pie dentro del panel.
func (r *dialogoRenderer) Layout(t fyne.Size) {
	d := r.d
	hoja := enHoja(d.lienzo)
	marcarPie(d.Pie, hoja)
	radio := d.radio(d.lienzo)
	r.caja.CambiarRadio(radio)
	// La hoja redondea solo arriba y no lleva filetes a los lados ni abajo: la caja se
	// pinta 1 px más ancha por cada lado y 1 px más alta, y el recorte del panel
	// (`IsClip`) se queda con lo de dentro.
	abajo := float32(0)
	if hoja {
		abajo = esquinaRecta
	}
	for _, o := range r.caja.Objetos() {
		if rect, es := o.(*canvas.Rectangle); es {
			rect.BottomLeftCornerRadius, rect.BottomRightCornerRadius = abajo, abajo
		}
	}
	if hoja {
		b := tema.Borde
		r.caja.Redimensionar(fyne.NewSize(t.Width+2*b, t.Height+b))
		for _, o := range r.caja.Objetos() {
			o.Move(o.Position().SubtractXY(b, 0))
		}
	} else {
		r.caja.Redimensionar(t)
	}
	if s := d.capa.sombraDe(); s != nil {
		s.CambiarRadio(radio)
		if hoja {
			s.Objeto().Hide()
		} else {
			s.Objeto().Show()
		}
	}
	if d.margen != nil {
		d.margen.Layout = d.margenCuerpo()
	}
	util := d.anchoUtil(t.Width)
	if d.fluido != nil {
		d.fluido.Layout = anchoFluido{alAncho(util)}
	}
	_, bordeAbajo := bordesDialogo(d.lienzo)
	cab := altoCabecera(d, t.Width, d.lienzo)
	pie := altoPieDialogo(d.Pie, util, hoja)
	cuerpo := d.altoCuerpo(util)
	fijo := tema.Borde + bordeAbajo + cab + pie
	r.altoContenido = fijo + cuerpo
	// Si queda al menos un blanco táctil para el cuerpo, cabecera y acciones
	// permanecen fijas. Si ni eso cabe, se desplazan juntas, sin superponerse.
	r.desbordando = t.Height < fijo+min(cuerpo, SueloTactilBarra)
	// Los filetes, solo si el cuerpo se desplaza por dentro.
	r.filetes = !r.desbordando && d.Cuerpo != nil && cuerpo > t.Height-fijo+0.01
	if r.desbordando {
		if r.desborde == nil {
			r.partes = container.New(contenidoDialogo{r})
			r.desborde = container.NewVScroll(r.partes)
			r.envoltorioPanel = container.NewThemeOverride(r.desborde, tema.SinSombras{})
		}
		r.rehacer()
		r.partes.Refresh()
		r.envoltorioPanel.Move(fyne.Position{})
		r.envoltorioPanel.Resize(t)
		r.desborde.Refresh()
		return
	}
	r.rehacer()
	r.disponer(t)
}

func (r *dialogoRenderer) disponer(t fyne.Size) {
	d := r.d
	b := tema.Borde
	lado, abajo := bordesDialogo(d.lienzo)
	interior := max(t.Width-2*lado, 0)
	util := d.anchoUtil(t.Width)

	// La cabecera comparte el relleno del panel y acaba con su filete, que solo se ve
	// cuando el cuerpo se desplaza.
	cab := altoCabecera(d, t.Width, d.lienzo)
	r.fileteCabecera.Move(fyne.NewPos(lado, cab))
	r.fileteCabecera.Resize(fyne.NewSize(interior, b))
	esconder(r.fileteCabecera, !r.filetes)
	if d.conAsa(d.lienzo) {
		r.asa.Move(fyne.NewPos((t.Width-anchoAsa)/2, b+margenAsa))
		r.asa.Resize(fyne.NewSize(anchoAsa, altoAsa))
	}

	lineas := float32(len(lineasTitulo(d.Titulo, util))) * TituloDialogo.Linea
	alto := SueloTactil(ladoAspa)
	fila := max(lineas, alto)
	filaY := b + d.arribaCabecera(d.lienzo)
	// `items-center`: el aspa se centra en la fila, también con el título en dos líneas.
	// El título cede ancho al aspa; el blanco del cierre nunca encoge.
	hueco, aspa := filaCabecera(d.Titulo, util)
	d.aspa.Resize(fyne.NewSize(aspa, alto))
	d.aspa.Move(fyne.NewPos(t.Width-lado-margenXDialogo-aspa, filaY+(fila-alto)/2))
	r.titulo.Texto, r.titulo.Ancho = d.Titulo, hueco
	r.titulo.Move(fyne.NewPos(lado+margenXDialogo, filaY+(fila-lineas)/2))
	r.titulo.Resize(fyne.NewSize(hueco, lineas))

	hoja := enHoja(d.lienzo)
	pie := altoPieDialogo(d.Pie, util, hoja)
	if d.Cuerpo != nil {
		hueco := fyne.NewSize(interior, max(t.Height-b-abajo-cab-pie, 0))
		if r.desbordando {
			d.margen.Move(fyne.NewPos(lado, b+cab))
			d.margen.Resize(hueco)
		} else {
			r.envoltorio.Move(fyne.NewPos(lado, b+cab))
			r.envoltorio.Resize(hueco)
			r.cuerpo.Refresh()
		}
	}
	if d.Pie != nil {
		y := t.Height - abajo - pie
		r.filete.Move(fyne.NewPos(lado, y))
		r.filete.Resize(fyne.NewSize(interior, b))
		esconder(r.filete, !r.filetes)
		// `justify-end`: las acciones pegadas a la derecha, centradas en su fila; o la
		// fila entera, si el pie la pide (o es un único botón suelto en la hoja del
		// teléfono: `PIE_TELEFONO` lo estira a `basis-full` y `min-h-11`).
		m := d.Pie.MinSize()
		m.Width = min(m.Width, util)
		_, esBoton := d.Pie.(*Boton)
		ancho := d.PieAncho || (hoja && esBoton)
		if c, ok := d.Pie.(*fyne.Container); ok {
			if fila, ok := c.Layout.(FilaAuto); ok && fila.Partir {
				ancho = true
			}
			if _, ok := c.Layout.(PieAcciones); ok {
				ancho = true
			}
		}
		if ancho {
			m.Width = util
		}
		m.Height = AltoA(d.Pie, m.Width)
		if hoja {
			m.Height = max(m.Height, altoAccionHoja(d.Pie))
		}
		d.Pie.Resize(m)
		d.Pie.Move(fyne.NewPos(t.Width-lado-margenXDialogo-m.Width, y+b+margenArrPie))
	}
}

// Refresh repinta con los colores del tema actual y RECOLOCA: es lo que usa Recolocar
// para que el cuerpo desplazable vuelva a medirse.
func (r *dialogoRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r.caja.Pintar(tema.Color("panel-surface", v), tema.Color("border", v))
	r.fileteCabecera.FillColor = fileteDialogo(v)
	r.fileteCabecera.Refresh()
	r.asa.FillColor = tema.Color("border-strong", v)
	r.asa.Refresh()
	if f := r.d.capa.faldonDe(); f != nil {
		f.FillColor = tema.Color("panel-surface", v)
		f.Refresh()
	}
	r.titulo.Tinta = tema.Color("text", v)
	if r.filete != nil {
		r.filete.FillColor = fileteDialogo(v)
		r.filete.Refresh()
	}
	// Un Refresh es «cambió lo de dentro»: la capa vuelve a medir el panel. Muchas
	// pantallas cambian el cuerpo y llaman a d.Refresh() —no a Recolocar— y el panel se
	// quedaba medido para lo de antes (Notepads: «Etiquetas» se abría con 106 px y la
	// lista cortada). Sin pasar por capa.Refresh, que refresca el panel otra vez.
	if c := r.d.capa; c != nil && c.Size().Width > 0 {
		(&capaRenderer{c: c}).Layout(c.Size())
	}
	r.Layout(r.d.Size())
	r.titulo.Refresh()
	if r.d.Cuerpo != nil {
		r.d.Desplazador().Refresh()
	}
}

// Confirmacion es ConfirmDialog.tsx: el mismo diálogo `sm` con el pie que se repetía en
// cada sitio —cancelar a la izquierda, la acción a la derecha— para las veintiuna
// llamadas a `confirm()`/`prompt()` que la suite tenía repartidas.
//
// No es un diálogo distinto: es un Dialogo con el pie ya montado. Lo que va DENTRO lo
// pone quien llama: un párrafo para una confirmación, o los campos que hagan falta
// cuando además hay que recoger un dato.
//
// `peligro` para lo que destruye, desconecta o suspende. No es decoración: es lo que
// distingue «guardar» de «desconectar el dominio de correo de la organización».
//
// El rótulo de confirmar es un VERBO, no «Aceptar»: quien lo lee tiene que saber qué va
// a pasar sin volver al título.
func Confirmacion(titulo, rotuloCancelar, rotuloConfirmar string, peligro bool,
	cuerpo fyne.CanvasObject, alCancelar, alConfirmar func(),
) *Dialogo {
	d, _ := confirmacion(titulo, rotuloCancelar, rotuloConfirmar, peligro, cuerpo, alCancelar, alConfirmar)
	return d
}

// confirmacion es Confirmacion devolviendo además el botón de confirmar, que es lo que
// necesita quien tiene que apagarlo mientras falte un dato (ver Pregunta).
func confirmacion(titulo, rotuloCancelar, rotuloConfirmar string, peligro bool,
	cuerpo fyne.CanvasObject, alCancelar, alConfirmar func(),
) (*Dialogo, *Boton) {
	// Sin `alCancelar`, cancelar CIERRA: un botón de cancelar que no hace nada es un
	// diálogo del que solo se sale confirmando.
	var d *Dialogo
	if alCancelar == nil {
		alCancelar = func() { d.Cerrar() }
	}
	variante := Primario
	if peligro {
		variante = Peligro
	}
	// Los dos de la talla POR DEFECTO (31,5, `px-4`, texto de 12,25): la del pie de
	// cualquier otro diálogo de la suite. Eran `sm` (28) hasta el PR #836 de la web, y
	// una confirmación se veía más pequeña que el formulario de al lado. El primario va
	// EL ÚLTIMO: es el orden medido en producción, y el que espera cualquiera que lea
	// de izquierda a derecha.
	cancelar := NuevoBoton(rotuloCancelar, alCancelar).Con(Fantasma, TallaMD)
	confirmar := NuevoBoton(rotuloConfirmar, alConfirmar).Con(variante, TallaMD)
	pie := container.New(Fila{Hueco: HuecoPieDialogo}, cancelar, confirmar)

	d = NuevoDialogo(titulo, cuerpo).Con(DialogoSM, pie)
	d.AlCerrar = alCancelar
	d.cancelar, d.confirmar = cancelar, confirmar
	d.clase = claseConfirmar
	return d, confirmar
}

// La línea va encima del panel ya pintado: sólo el borde exterior necesita
// componer panel+tinta antes de pintar.
func fileteDialogo(v tema.Variante) color.NRGBA {
	if tema.Color("panel-surface", v).A != 255 {
		return tokens.Crudo("border-subtle", v)
	}
	return BordeSobre("border-subtle", "panel-surface", v)
}
