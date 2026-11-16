// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calcus

import (
	"image/color"
	"math"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/editor"
	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/formula"
)

// Barra es la barra de fórmulas de SheetCanvas.tsx: el cuadro de nombres con la celda
// activa, la «fx» y el campo con el texto crudo de esa celda.
//
// El cuadro de nombres es `CuadroNombres.tsx`: enseña la celda o el rango y se escribe
// en él —«B12», «Hoja2!A1:C5» o un nombre definido— para saltar allí con Intro
// (`ir_a.go`); Escape vuelve a la hoja sin moverse. Ctrl+G y F5 traen el foco aquí.
//
// Refleja la celda ancla de la rejilla y escribe en ella: Intro guarda y baja una
// fila, que es lo que hace la web.
//
// El ▾ del final la despliega: el campo pasa a ser de tres líneas que reparten la
// fórmula larga en vez de correrla de lado (el `<textarea>` de la web). Intro sigue
// escribiendo la celda, así que lo único que cambia es cuánto se ve. Se recuerda por
// dispositivo, como el resumen de la barra de hojas (`resumen.go`).
type Barra struct {
	widget.BaseWidget

	R *Rejilla
	// ed es el campo vigente: eds[0], el de una línea, o eds[1], el desplegado. Son dos
	// porque el motor de Fyne decide al nacer si reparte y se desplaza en vertical (ver
	// `NuevoEditorPlanoMultilinea`).
	ed         *ui.EditorPlano
	eds        [2]*ui.EditorPlano
	desplegada bool
	// boton es el ▾ que despliega y recoge.
	boton    *editor.BotonCinta
	enfocado bool
	// T traduce los rótulos del ▾; lo pone el lienzo, que es quien tiene el traductor.
	T func(clave string) string
	// AlCambiarAlto avisa de que la barra se ha desplegado o recogido: el lienzo coloca
	// de nuevo la rejilla debajo.
	AlCambiarAlto func()
	// AlEnfocar avisa de que el campo gana o pierde el foco: es cuando empieza y acaba
	// la edición de una fórmula en la barra.
	AlEnfocar func(bool)
	// campo es dónde está el campo, para colgar debajo la lista del autocompletado.
	campo fyne.Position
	rend  *barraRenderer

	// nombre es el editor del cuadro de nombres. Sin el foco escribe en tinta
	// transparente y lo que se ve es el rótulo centrado (el `text-center` del `<input>`
	// de la web); con el foco, el rótulo se esconde y se escribe a la izquierda
	// (`focus:text-left`).
	nombre         *ui.EditorPlano
	nombreEnfocado bool
	// AlIrA salta a lo escrito en el cuadro (`Lienzo.IrA`) y devuelve si se ha ido.
	AlIrA func(texto string) bool
}

var _ fyne.Widget = (*Barra)(nil)

// NuevaBarra la ata a su rejilla.
func NuevaBarra(r *Rejilla) *Barra {
	b := &Barra{R: r}
	b.ExtendBaseWidget(b)
	// El foco de cada campo cuenta solo si es el vigente: al desplegar, el que se va
	// pierde el foco DESPUÉS de que el nuevo lo gane, y eso no es salir de la barra.
	alEnfocar := func(i int) func(bool) {
		return func(f bool) {
			if b.eds[i] != b.ed {
				return
			}
			b.enfocado = f
			if b.AlEnfocar != nil {
				b.AlEnfocar(f)
			}
			b.Refresh()
		}
	}
	// Desplazable: una fórmula más larga que el campo corre como en el `<input>` de la
	// web, con el cursor siempre a la vista.
	b.eds[0] = ui.NuevoEditorPlanoDesplazable(EscalonFx, alEnfocar(0))
	b.eds[1] = ui.NuevoEditorPlanoMultilinea(EscalonFxDesplegada, alEnfocar(1))
	for _, ed := range b.eds {
		ed.Tecla = b.tecla
	}
	b.eds[1].Atajo = b.atajoDesplegada
	b.nombre = ui.NuevoEditorPlanoDesplazable(tema.TextoXS, b.alEnfocarNombre)
	b.nombre.Tecla = b.teclaNombre
	b.desplegada = fxDesplegadaRecordada()
	b.ed = b.eds[b.indice()]
	b.boton = editor.NuevoBotonCinta(iconos.EdChevronAbajo, "", func() { b.Desplegar(!b.desplegada) })
	// La rueda corre el texto: el tinte va con él. Lo que se corre mientras la barra se
	// coloca o se repinta lo tiñe ella misma, y repintar desde ahí no acabaría nunca.
	// Solo el tinte: repintar el editor lo devolvería al cursor y la rueda no correría.
	b.eds[0].AlDesplazar(func() {
		if r := b.rend; r != nil && !r.colocando && r.anchoCampo > 0 && b.ed.Desplazamiento() != r.tinteX {
			r.tintar(b.campo, r.anchoCampo)
			r.recorte.Refresh()
		}
	})
	b.Sincronizar()
	return b
}

func (b *Barra) indice() int {
	if b.desplegada {
		return 1
	}
	return 0
}

// Editores son los dos campos, el de una línea y el desplegado: el lienzo engancha a
// los dos la edición de fórmulas.
func (b *Barra) Editores() [2]*ui.EditorPlano { return b.eds }

// Desplegada dice si la barra enseña el campo de tres líneas.
func (b *Barra) Desplegada() bool { return b.desplegada }

// Desplegar despliega o recoge la barra y lo recuerda en el dispositivo. Si se estaba
// escribiendo, el texto, el cursor y el foco pasan al otro campo.
func (b *Barra) Desplegar(v bool) {
	if v == b.desplegada {
		return
	}
	antes := b.ed
	cursor := antes.CursorTextOffset()
	lz := fyne.CurrentApp().Driver().CanvasForObject(b)
	conFoco := lz != nil && lz.Focused() == antes
	b.desplegada = v
	recordarFxDesplegada(v)
	b.ed = b.eds[b.indice()]
	b.ed.PonerTexto(antes.Texto())
	b.ed.SoloLectura(antes.Disabled())
	b.ed.PonerCursor(cursor)
	if b.rend != nil {
		b.rend.recomponer()
	}
	if b.AlCambiarAlto != nil {
		b.AlCambiarAlto()
	}
	b.Refresh()
	if conFoco {
		lz.Focus(b.ed)
	}
}

// Alto es el `h-8` de la barra, o lo que mide desplegada.
func (b *Barra) Alto() float32 {
	if b.desplegada {
		return AltoBarraDesplegada
	}
	return AltoBarra
}

func (b *Barra) t(clave string) string {
	if b.T == nil {
		return clave
	}
	return b.T(clave)
}

// Sincronizar trae a la barra lo que hay en la celda ancla. La llama la rejilla cada
// vez que la selección se mueve o el contenido cambia.
func (b *Barra) Sincronizar() {
	s := b.R.Seleccion()
	texto := b.R.L.Crudo(s.C, s.F)
	if b.R.L.FormulaOculta(s.C, s.F) {
		texto = "" // la fórmula oculta de una hoja protegida no se enseña
	}
	b.ed.PonerTexto(texto)
	// La celda que la protección no deja editar deja la barra cerrada: es el
	// `readOnly={!canEditSel}` del campo de la web.
	b.ed.SoloLectura(b.R.SoloLectura || !b.R.L.Editable(s.C, s.F))
	if b.desplegada {
		b.ed.PonerCursor(0) // como el `<textarea>` nuevo: se lee desde el principio
	}
	b.Refresh()
}

// EnfocarCuadro es Ctrl+G o F5: el foco al cuadro de nombres, con lo que enseña marcado
// para escribir encima.
func (b *Barra) EnfocarCuadro() {
	if lz := fyne.CurrentApp().Driver().CanvasForObject(b); lz != nil {
		lz.Focus(b.nombre)
	}
}

// CuadroEnfocado dice si se está escribiendo en el cuadro de nombres.
func (b *Barra) CuadroEnfocado() bool { return b.nombreEnfocado }

// Cuadro es el editor del cuadro de nombres (pruebas).
func (b *Barra) Cuadro() *ui.EditorPlano { return b.nombre }

func (b *Barra) alEnfocarNombre(f bool) {
	b.nombreEnfocado = f
	if f {
		b.nombre.PonerTexto(b.Rotulo())
		b.nombre.TypedShortcut(&fyne.ShortcutSelectAll{})
	}
	b.Refresh()
}

// teclaNombre: Intro salta (lo que no lleva a ninguna parte se queda escrito y marcado,
// como en Excel, para corregirlo); Escape vuelve a la rejilla sin moverse.
func (b *Barra) teclaNombre(k *fyne.KeyEvent) bool {
	switch k.Name {
	case fyne.KeyReturn, fyne.KeyEnter:
		if b.AlIrA != nil && !b.AlIrA(b.nombre.Texto()) {
			b.nombre.TypedShortcut(&fyne.ShortcutSelectAll{})
		}
		return true
	case fyne.KeyEscape:
		b.R.recuperarFoco()
		return true
	default:
		return false
	}
}

// campoX es dónde empieza el campo de la fórmula y dónde acaba por abajo, en las
// coordenadas del lienzo (la barra va arriba del todo).
func (b *Barra) campoX() (x, abajo float32) {
	if b.desplegada {
		return b.campo.X, b.campo.Y + AltoCampoDesplegado
	}
	return b.campo.X, b.campo.Y + AltoEntradaFx
}

// Rotulo es el `refLabel`: la celda ancla, o el rango con sus dos esquinas.
func (b *Barra) Rotulo() string {
	c1, f1, c2, f2 := b.R.Rango()
	if c1 == c2 && f1 == f2 {
		return Ref(c1, f1)
	}
	return Ref(c1, f1) + ":" + Ref(c2, f2)
}

// tecla: Intro escribe y baja; Escape devuelve el campo a lo que hay en la celda.
func (b *Barra) tecla(k *fyne.KeyEvent) bool {
	switch k.Name {
	case fyne.KeyReturn, fyne.KeyEnter:
		// Lo que una validación estricta rechaza se queda en la barra, sin bajar: es el
		// `return` de `onFxKeyDown` tras el aviso.
		s := b.R.Seleccion()
		if !b.R.Escribir(b.ed.Texto()) && !b.R.SoloLectura {
			return true
		}
		b.R.Seleccionar(s.C, s.F+1, false)
		b.R.recuperarFoco() // `gridRef.current?.focus()`: la fórmula se da por acabada
		return true
	case fyne.KeyEscape:
		b.Sincronizar()
		return true
	case fyne.KeyTab:
		// Solo llega del desplegado: `widget.Entry` de varias líneas se queda el
		// tabulador y metería un «\t» en la fórmula. Pasa el foco, como en el de una
		// línea y en el `<textarea>` de la web.
		if lz := fyne.CurrentApp().Driver().CanvasForObject(b); lz != nil {
			if b.ed.Mayus() {
				lz.FocusPrevious()
			} else {
				lz.FocusNext()
			}
		}
		return true
	default:
		return false
	}
}

// atajoDesplegada pega en el campo desplegado sin saltos de línea, que una celda no
// tiene: el de varias líneas los dejaría pasar y el de una línea los cambia por
// espacios, así que se hace lo mismo para que desplegar no cambie lo que se escribe.
func (b *Barra) atajoDesplegada(s fyne.Shortcut) bool {
	p, ok := s.(*fyne.ShortcutPaste)
	if !ok || p.Clipboard == nil {
		return false
	}
	texto := p.Clipboard.Content()
	if !strings.ContainsAny(texto, "\r\n") {
		return false
	}
	texto = strings.NewReplacer("\r\n", " ", "\r", " ", "\n", " ").Replace(texto)
	b.ed.Entry.TypedShortcut(&fyne.ShortcutPaste{Clipboard: portapapelesFijo(texto)})
	return true
}

// portapapelesFijo es un portapapeles que solo da ese texto: el de verdad, ya sin
// saltos de línea, sin tocar lo que el sistema tiene copiado.
type portapapelesFijo string

func (p portapapelesFijo) Content() string { return string(p) }
func (portapapelesFijo) SetContent(string) {}

// MedidaBarra es lo que ocupa la barra: el `h-8` de la web, y de ancho el mínimo en el
// que caben sus cuatro piezas. El ancho real lo pone quien la coloca (`flex-1`).
func MedidaBarra() fyne.Size {
	return fyne.NewSize(2*MargenBarra+AnchoRef+3*HuecoBarra+2*tema.Rem+editor.LadoBotonCinta, AltoBarra)
}

// MinSize es la medida declarada, con el alto de la desplegada si lo está.
func (b *Barra) MinSize() fyne.Size {
	b.ExtendBaseWidget(b)
	return fyne.NewSize(MedidaBarra().Width, b.Alto())
}

// CreateRenderer compone fondo, rótulo, «fx» y campo.
func (b *Barra) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &barraRenderer{
		b:       b,
		fondo:   canvas.NewRectangle(tema.Color("surface", v)),
		linea:   canvas.NewRectangle(tema.Color("border", v)),
		rotulo:  ui.Texto("", tema.TextoXS, ui.Medio, tema.Color("text", v)),
		refCaja: canvas.NewRectangle(tema.Color("bg-subtle", v)),
		// `.sytue-meta italic`: `text-xs` en `text-subtle`, en cursiva.
		fx:        ui.Texto("fx", tema.TextoXS, ui.Normal, tema.Color("text-subtle", v)),
		aro:       ui.NuevoAro(tema.RadioSM, ui.SinHueco, v),
		aroNombre: ui.NuevoAro(tema.RadioSM, ui.SinHueco, v),
		// El fondo del campo. En la web el `<input>` es `bg-transparent` sobre la
		// barra, que es `bg-surface`, así que pintar la superficie aquí da el MISMO
		// píxel… y hace falta por otra razón: `ui.Aro` es un rectángulo RELLENO del
		// color del anillo, y cuenta con que el control tenga encima una caja opaca
		// que solo le deje ver el reborde. Un campo transparente no la tiene, y el
		// aro salía como un bloque azul que se comía la fórmula entera. Se vio
		// mirando el PNG; ningún test de medida lo habría cazado.
		campo: canvas.NewRectangle(tema.Color("surface", v)),
	}
	r.refCaja.CornerRadius = tema.RadioSM
	r.refCaja.StrokeWidth = tema.Borde
	r.refCaja.StrokeColor = tema.Color("border", v)
	r.fx.FontSource = tema.Cursiva
	r.campo.CornerRadius = tema.RadioSM // `rounded-sm`
	r.recorte = nuevoRecorte(nil)
	r.recomponer()
	b.rend = r
	return r
}

type barraRenderer struct {
	b              *Barra
	fondo          *canvas.Rectangle
	linea          *canvas.Rectangle
	rotulo, fx     *canvas.Text
	campo, refCaja *canvas.Rectangle
	aro            *ui.Aro
	aroNombre      *ui.Aro // el foco del cuadro de nombres
	tinte          tinte
	recorte        *recorte // el `overflow-hidden` del fondo tintado
	objetos        []fyne.CanvasObject
	ultimoAncho    float32
	// colocando es que el Layout o el Refresh están en marcha, y tinteX el
	// desplazamiento con el que se tiñó por última vez.
	colocando  bool
	tinteX     float32
	anchoCampo float32
}

// recomponer ordena los objetos: el tinte de la fórmula va entre el fondo del campo y el
// editor, que lo deja ver con su texto transparente.
func (r *barraRenderer) recomponer() {
	// Cada aro va DEBAJO de la caja opaca de su control, que solo le deja ver el reborde.
	o := append([]fyne.CanvasObject{r.fondo, r.linea}, r.aroNombre.Objetos()...)
	o = append(o, r.refCaja, r.rotulo, r.b.nombre.Objeto(), r.fx)
	o = append(o, r.aro.Objetos()...)
	r.recorte.objs = r.tinte.objetos()
	o = append(o, r.campo, r.recorte, r.b.ed.Objeto(), r.b.boton)
	r.objetos = o
}

func (r *barraRenderer) MinSize() fyne.Size           { return r.b.MinSize() }
func (r *barraRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *barraRenderer) Destroy()                     {}

// anchoVentana es el ancho del lienzo en el que vive la barra, o 0 si aún no tiene.
func (r *barraRenderer) anchoVentana() float32 {
	if fyne.CurrentApp() == nil {
		return 0
	}
	if c := fyne.CurrentApp().Driver().CanvasForObject(r.b); c != nil {
		return c.Size().Width
	}
	return 0
}

func (r *barraRenderer) Layout(t fyne.Size) {
	defer r.ocupar()()
	r.ultimoAncho = t.Width
	r.fondo.Move(fyne.NewPos(0, 0))
	r.fondo.Resize(t)
	// `border-b`, que con `box-sizing: border-box` va DENTRO del `h-8`.
	r.linea.Move(fyne.NewPos(0, t.Height-tema.Borde))
	r.linea.Resize(fyne.NewSize(t.Width, tema.Borde))

	// `items-center` sobre el alto del contenido, que es el `h-8` menos su borde.
	// Desplegada es `items-start py-1`: cada pieza arriba, bajo el `py-1`, y la fila
	// mide lo que el campo de tres líneas. `centro` da la y de algo de ese alto.
	contenido := t.Height - tema.Borde
	centro := func(alto float32) float32 { return (contenido - alto) / 2 }
	if r.b.desplegada {
		centro = func(float32) float32 { return MargenBarraDesplegada }
	}
	x := MargenBarra
	// El rótulo va centrado en su `w-24` (`w-16` por debajo de `sm`), y lo que no quepa
	// en su `px-1` se corta con puntos (`truncate`); el editor del cuadro ocupa la misma
	// caja.
	anchoRef := anchoCuadroNombres(r.anchoVentana())
	cuadro := fyne.NewPos(x, centro(AltoRotulo))
	r.refCaja.Move(cuadro)
	r.refCaja.Resize(fyne.NewSize(anchoRef, AltoRotulo))
	r.aroNombre.ColocarEn(cuadro, fyne.NewSize(anchoRef, AltoRotulo))
	ancho := ui.Truncar(r.rotulo, r.b.Rotulo(), tema.TextoXS, ui.Medio, anchoRef-2*MargenCelda)
	yRotulo := cuadro.Y + (AltoRotulo-tema.TextoXS.Linea)/2
	ui.ColocarTexto(r.rotulo, tema.TextoXS, x+(anchoRef-ancho)/2, yRotulo)
	altoNombre := r.b.nombre.AltoFuente()
	r.b.nombre.Colocar(fyne.NewPos(x+MargenCelda, cuadro.Y+(AltoRotulo-altoNombre)/2),
		fyne.NewSize(anchoRef-2*MargenCelda, altoNombre))
	x += anchoRef + HuecoBarra

	// La «fx» es `leading-6`: su caja mide lo que el rótulo y va a su altura.
	anchoFx := ui.Medir("fx", tema.TextoXS, ui.Normal).Width
	ui.ColocarTexto(r.fx, tema.TextoXS, x, yRotulo)
	x += anchoFx + HuecoBarra

	// El ▾, `BOTON_DE_BARRA`, al final de la fila.
	lado := r.b.boton.MinSize().Width
	r.b.boton.Move(fyne.NewPos(t.Width-MargenBarra-lado, centro(lado)))
	r.b.boton.Resize(fyne.NewSize(lado, lado))
	anchoCampo := max(t.Width-MargenBarra-lado-HuecoBarra-x, 0)

	if r.b.desplegada {
		// El `<textarea>`: `h-15` bajo su `mt-0.5`, con su `px-1` y su `leading-5`.
		caja := fyne.NewPos(x, MargenBarraDesplegada+BajadaCampoDesplegado)
		tam := fyne.NewSize(anchoCampo, AltoCampoDesplegado)
		r.campo.Move(caja)
		r.campo.Resize(tam)
		r.aro.ColocarEn(caja, tam)
		r.b.ed.Colocar(fyne.NewPos(caja.X+MargenCelda, caja.Y), fyne.NewSize(max(anchoCampo-2*MargenCelda, 0), AltoCampoDesplegado))
		r.b.campo, r.anchoCampo = caja, anchoCampo
		r.tintar(caja, anchoCampo)
		return
	}
	// El campo: `h-5` centrado, y dentro su `px-1`. El editor ocupa solo la caja de
	// línea de su FUENTE, centrada en el `h-5`, que es lo que hace el navegador.
	caja := fyne.NewPos(x, centro(AltoEntradaFx))
	r.campo.Move(caja)
	r.campo.Resize(fyne.NewSize(anchoCampo, AltoEntradaFx))
	r.aro.ColocarEn(caja, fyne.NewSize(anchoCampo, AltoEntradaFx))
	alto := r.b.ed.AltoFuente()
	r.b.ed.Colocar(fyne.NewPos(caja.X+MargenCelda, caja.Y+(AltoEntradaFx-alto)/2),
		fyne.NewSize(max(anchoCampo-2*MargenCelda, 0), alto))
	r.b.campo, r.anchoCampo = caja, anchoCampo
	r.tintar(caja, anchoCampo)
}

// tintar pinta la fórmula de la barra con sus referencias de colores (`fxIsFormula`):
// siempre que el texto empiece por «=», se esté editando o no. El texto del campo se
// vuelve transparente y el cursor conserva la tinta del tema.
//
// Los trozos van dentro de un recorte del tamaño del editor y corridos lo que él haya
// corrido su texto: es el `scrollLeft` que la web copia del `<input>` a su fondo. Así una
// fórmula más larga que el campo también se ve tintada, y la rueda sobre el campo, que
// corre el texto sin mover el cursor, lo repinta en el momento (`AlDesplazar`).
func (r *barraRenderer) tintar(caja fyne.Position, ancho float32) {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	texto := r.b.ed.Texto()
	esc := tema.Escalon{Tam: EscalonFx.Tam, Linea: AltoEntradaFx}
	r.recorte.Move(fyne.NewPos(caja.X+MargenCelda, caja.Y))
	r.recorte.Resize(fyne.NewSize(max(ancho-2*MargenCelda, 0), AltoEntradaFx))
	pintado, crecio := false, false
	// ponytail: desplegada, la fórmula va sin tinte —en la tinta del tema—, porque el
	// tinte de aquí pinta una sola línea y el campo de tres reparte por palabras. Los
	// marcos de colores sobre las celdas siguen saliendo; si se echa de menos, repartir
	// los trozos con el mismo corte que `widget.Entry`.
	if strings.HasPrefix(texto, "=") && !r.b.desplegada {
		pintado, crecio = r.tinte.pintar(texto, formula.ExtractRefsCon(texto, r.b.R.L.ResolverParaResaltar(r.b.R.sel)),
			func(i int) color.NRGBA { return ColorRefFx(i, v) }, tema.Color("text", v),
			esc, -r.tinteDesde(), 0, math.MaxFloat32)
	} else {
		r.tinte.pintar("", nil, nil, color.NRGBA{}, esc, 0, 0, 0)
	}
	if pintado {
		r.b.ed.Tinta(color.NRGBA{}, tema.Color("text", v))
	} else {
		r.b.ed.Tinta(nil, nil)
	}
	if crecio {
		r.recomponer()
	}
}

// tinteDesde es el desplazamiento del editor, que queda apuntado para AlDesplazar.
func (r *barraRenderer) tinteDesde() float32 {
	r.tinteX = r.b.ed.Desplazamiento()
	return r.tinteX
}

// ocupar marca la barra ocupada y devuelve cómo soltarla; anidado, suelta el de fuera.
func (r *barraRenderer) ocupar() func() {
	antes := r.colocando
	r.colocando = true
	return func() { r.colocando = antes }
}

func (r *barraRenderer) Refresh() {
	defer r.ocupar()()
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r.fondo.FillColor = tema.Color("surface", v)
	r.linea.FillColor = tema.Color("border", v)
	r.rotulo.Text = r.b.Rotulo()
	r.rotulo.Color = tema.Color("text", v)
	if r.b.nombreEnfocado {
		r.rotulo.Hide()
		r.b.nombre.Tinta(nil, nil)
	} else {
		r.rotulo.Show()
		r.b.nombre.Tinta(color.Transparent, color.Transparent)
	}
	r.aroNombre.Pintar(v)
	r.aroNombre.Ver(r.b.nombreEnfocado)
	r.refCaja.FillColor = tema.Color("bg-subtle", v)
	r.refCaja.StrokeColor = tema.Color("border", v)
	r.fx.Color = tema.Color("text-subtle", v)
	r.campo.FillColor = tema.Color("surface", v)
	r.aro.Pintar(v)
	r.aro.Ver(r.b.enfocado)
	// El ▾ dice lo que hará, como el `aria-label` de la web, y apunta hacia allí.
	if r.b.desplegada {
		r.b.boton.Icono, r.b.boton.Rotulo = iconos.EdChevronArriba, r.b.t("sheet.formulaBar.collapse")
	} else {
		r.b.boton.Icono, r.b.boton.Rotulo = iconos.EdChevronAbajo, r.b.t("sheet.formulaBar.expand")
	}
	if r.ultimoAncho > 0 {
		r.Layout(fyne.NewSize(r.ultimoAncho, r.b.Alto()))
	}
	for _, o := range r.objetos {
		o.Refresh()
	}
}

// anchoCuadroNombres es `w-16 sm:w-24`: el del cuadro de nombres según el ancho de la
// ventana (0 = sin ventana todavía, que se trata como ancha).
func anchoCuadroNombres(ventana float32) float32 {
	if ventana > 0 && ventana < ui.CorteSM {
		return AnchoRefEstrecho
	}
	return AnchoRef
}
