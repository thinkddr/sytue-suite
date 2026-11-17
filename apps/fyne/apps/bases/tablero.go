// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package bases

import (
	"image/color"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// Las medidas del tablero (Tablero.tsx).
var (
	// anchoColumna es el `w-72` de una columna y margenTablero el `p-3` de columnas y tarjetas.
	anchoColumna  = 18 * float32(tema.Rem)
	margenTablero = tema.Espacio(3)
	// anchoTarjeta es lo que queda dentro de una tarjeta: la columna sin su `p-3` y su
	// borde, y la tarjeta sin los suyos.
	anchoTarjeta = anchoColumna - 4*margenTablero - 4*tema.Borde
)

// caja es `border-border rounded-* border` con su fondo y margen interior.
func caja(fondo color.NRGBA, radio, margen float32, v tema.Variante, contenido fyne.CanvasObject) fyne.CanvasObject {
	r := canvas.NewRectangle(fondo)
	r.CornerRadius, r.StrokeWidth, r.StrokeColor = radio, tema.Borde, tema.Color("border", v)
	m := margen + tema.Borde
	return container.NewStack(r, container.New(ui.Margen{Izq: m, Arr: m, Der: m, Abj: m}, contenido))
}

// anchoFijo es `w-* shrink-0`: el hijo, a ese ancho y a su alto.
type anchoFijo struct{ Ancho float32 }

func (a anchoFijo) MinSize(objs []fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(a.Ancho, objs[0].MinSize().Height)
}

func (a anchoFijo) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	objs[0].Move(fyne.NewPos(0, 0))
	objs[0].Resize(fyne.NewSize(a.Ancho, t.Height))
}

// objetoTablero es Tablero.tsx: una columna por opción del campo de selección y una
// tarjeta por registro. Mover es escribir ese campo en el registro: lo decide y lo valida
// el servicio, como en la rejilla.
//
// La tarjeta se pulsa para abrir su ficha. Se mueve con el asa o, sin arrastrar (WCAG
// 2.5.7), desde «Mover a» de su menú «⋯»: el selector que iba siempre a la vista en cada
// tarjeta saturaba la columna. Al pie de cada columna, «Añadir tarjeta» la crea ya con el
// valor de esa columna y abre su ficha.
func (p *Pantalla) objetoTablero(v tema.Variante, agrupar api.BasesField, puedeEditar bool) fyne.CanvasObject {
	b := p.abierta
	cols := columnasDeTablero(agrupar, b.vent.registros())
	nombres := make([]string, len(cols))
	for i, c := range cols {
		nombres[i] = c.valor
		if c.valor == "" {
			nombres[i] = p.t("board.noValue")
		}
	}
	// Lo que una tarjeta enseña bajo su nombre: otros dos campos con valor.
	otros := make([]api.BasesField, 0, len(b.Campos))
	for _, c := range b.Campos {
		if c.ID != agrupar.ID && c.Kind != api.BasesKindLink && c.Kind != api.BasesKindCheckbox {
			otros = append(otros, c)
		}
	}
	columnas := make([]fyne.CanvasObject, len(cols))
	arrastre := &arrastreTablero{p: p, campo: agrupar, columnas: columnas}
	arrastre.valores = make([]string, len(cols))
	arrastre.aros = make([]*canvas.Rectangle, len(cols))
	// `bg-bg-subtle/50`: el gris a medias.
	fondoColumna := tema.Color("bg-subtle", v)
	fondoColumna.A /= 2
	for i, col := range cols {
		arrastre.valores[i] = col.valor
		hijos := make([]fyne.CanvasObject, 0, 1+len(col.registros))
		hijos = append(hijos, container.New(ui.FilaAuto{Hueco: tema.Espacio(2), Derecha: 2, Partir: true, SaltoFila: tema.Espacio(1)},
			ui.NuevaInsignia(nombres[i], tonoDeOpcion(agrupar, col.valor)),
			ui.NuevoRotulo("("+strconv.Itoa(len(col.registros))+")", tema.TextoXS, ui.Normal, "text-muted")))
		for _, r := range col.registros {
			var asa *ui.AsaArrastre
			if puedeEditar {
				asa = arrastre.asa(r, i)
			}
			hijos = append(hijos, p.tarjetaDeTablero(v, r, agrupar, otros, nombres, i, puedeEditar, asa))
		}
		if puedeEditar {
			if len(col.registros) == 0 {
				hijos = append(hijos, columnaVacia(p.t("board.empty")))
			}
			valor := col.valor
			anadir := botonSM(p.t("board.add"), ui.Fantasma, func() { p.anadirEnColumna(agrupar, valor) })
			anadir.Icono, anadir.TamIcono, anadir.Atenuado = iconos.Mas, 14, true
			anadir.Rotulo = p.t("board.addIn", i18n.Vars{"name": nombres[i]})
			hijos = append(hijos, container.New(ui.Fila{}, anadir))
		}
		aro := canvas.NewRectangle(color.NRGBA{})
		aro.StrokeColor, aro.StrokeWidth, aro.CornerRadius = tema.Color("indicator", v), tema.AnilloFoco, tema.RadioLG
		aro.Hide()
		arrastre.aros[i] = aro
		columnas[i] = container.New(anchoFijo{Ancho: anchoColumna},
			container.NewStack(caja(fondoColumna, tema.RadioLG, margenTablero, v, container.New(ui.Columna{Hueco: tema.Espacio(3)}, hijos...)), aro))
	}
	// `overflow-x-auto`: las columnas se desplazan de lado y la rueda sigue bajando la página.
	fila := container.New(ui.Margen{Abj: tema.Espacio(2)}, container.New(ui.Fila{Hueco: tema.Espacio(3)}, columnas...))
	lado := ui.NuevoDesplazaLado(fila, 0, func() fyne.Scrollable {
		if p.scroll == nil {
			return nil
		}
		return p.scroll
	})
	arrastre.lado = lado
	hijos := []fyne.CanvasObject{}
	if !puedeEditar {
		hijos = append(hijos, ui.NuevoRotulo(p.t("grid.readOnly"), tema.TextoXS, ui.Normal, "text-muted"))
	}
	hijos = append(hijos, lado)
	// Las ventanas de registros son las de la rejilla: el tablero pinta lo que hay en memoria.
	pie := []fyne.CanvasObject{}
	if b.vent.primera > 0 {
		pie = append(pie, botonSM(p.t("grid.loadPrevious"), ui.Fantasma, func() { p.cargarRegistros("antes") }))
	}
	if b.vent.siguiente != "" {
		pie = append(pie, botonSM(p.t("grid.loadMore"), ui.Fantasma, func() { p.cargarRegistros("mas") }))
	}
	if len(pie) > 0 {
		hijos = append(hijos, container.New(ui.Fila{Hueco: tema.Espacio(2)}, pie...))
	}
	return container.New(ui.Columna{Hueco: tema.Espacio(2)}, hijos...)
}

// tarjetaDeTablero es la tarjeta de un registro: su nombre, que se pulsa para abrir la
// ficha, dos campos más y el menú «⋯» con «Abrir» y, si se puede editar, «Mover a» cada
// una de las otras columnas.
func (p *Pantalla) tarjetaDeTablero(v tema.Variante, r api.BasesRecord, agrupar api.BasesField, otros []api.BasesField, columnas []string, actual int, puedeEditar bool, asa *ui.AsaArrastre) fyne.CanvasObject {
	b := p.abierta
	nombre := etiquetaDeRegistro(b.Campos, r)
	abrir := func() { p.abrirRegistro(r, false) }
	rotulo := p.t("board.actions", i18n.Vars{"name": nombre})
	var menu *ui.BotonIcono
	menu = ui.NuevoBotonIcono(iconos.Puntos, rotulo, func() {
		m := ui.NuevoMenu(p.accionesDeTarjeta(r, nombre, agrupar, columnas, actual, puedeEditar, abrir)...)
		m.Rotulo, m.AlInicio = rotulo, false
		m.Mostrar(menu)
	})
	menu.Lado = ui.LadoIconoSM
	// El título es el botón de la tarjeta: lo que se pulsa con el ratón y lo que alcanza
	// el teclado, con su nombre accesible de «Abrir …».
	ancho := anchoTarjeta - menu.MinSize().Width - tema.Espacio(2)
	partes := []fyne.CanvasObject{}
	if asa != nil {
		// Pulsar el asa sin arrastrar abre el menú, donde está «Mover a».
		asa.AlPulsar = menu.AlPulsar
		ancho -= asa.MinSize().Width + tema.Espacio(2)
		partes = append(partes, asa)
	}
	titulo := ui.NuevoParrafo(nombre, tema.TextoSM, ui.SemiNegrita, "text", v).A(ancho - 2*tema.Espacio(2))
	pulsable := ui.NuevaFilaPulsable(p.t("board.open", i18n.Vars{"name": nombre}), titulo, abrir)
	pulsable.AltoMinimo = tema.TextoSM.Linea
	partes = append(partes, container.New(anchoFijo{Ancho: ancho}, pulsable), menu)
	hijos := []fyne.CanvasObject{container.New(ui.FilaAuto{Hueco: tema.Espacio(2), Derecha: len(partes) - 1}, partes...)}
	if quienes := p.enTarjeta(b, r.ID); len(quienes) > 0 {
		hijos = append(hijos, etiquetaPresentes(p.t("rt.inCell", i18n.Vars{"names": strings.Join(quienes, ", ")}), v))
	}
	// Como la web (`.slice(1, 3)`): el primero con valor suele ser el propio nombre.
	conValor := 0
	for _, c := range otros {
		texto := textoVisible(c, r.Data[c.ID], p.tr.Locale())
		if texto == "" {
			continue
		}
		conValor++
		if conValor == 1 {
			continue
		}
		if conValor > 3 {
			break
		}
		hijos = append(hijos, ui.NuevoParrafo(c.Name+": "+texto, tema.TextoXS, ui.Normal, "text-muted", v).A(anchoTarjeta).Recortado(1))
	}
	return caja(tema.Color("surface", v), tema.RadioMD, margenTablero, v, container.New(ui.Columna{Hueco: tema.Espacio(2)}, hijos...))
}

// accionesDeTarjeta es el menú «⋯» de una tarjeta: «Abrir» y, para quien escribe, «Mover
// a» cada una de las otras columnas. Se arma en cada apertura: un elemento recuerda el
// menú en el que va.
func (p *Pantalla) accionesDeTarjeta(r api.BasesRecord, nombre string, agrupar api.BasesField, columnas []string, actual int, puedeEditar bool, abrir func()) []fyne.CanvasObject {
	elementos := []fyne.CanvasObject{ui.NuevoElementoMenu(p.t("board.open", i18n.Vars{"name": nombre}), iconos.Maximizar, abrir)}
	if !puedeEditar {
		return elementos
	}
	elementos = append(elementos, ui.NuevoSeparadorMenu())
	for i, col := range columnas {
		if i == actual {
			continue
		}
		var valor any
		if i < len(agrupar.Choices) {
			valor = agrupar.Choices[i]
		}
		elementos = append(elementos, ui.NuevoElementoMenu(p.t("board.moveToColumn", i18n.Vars{"name": col}), iconos.ChevronDerecha, func() {
			p.cambiar(r, map[string]any{agrupar.ID: valor}, nil)
		}))
	}
	return elementos
}

// columnaVacia es el aviso de una columna sin tarjetas: un recuadro discontinuo que
// invita a soltar una ahí (`border-dashed p-3 text-center text-xs`).
func columnaVacia(texto string) fyne.CanvasObject {
	m := margenTablero + tema.Borde
	return container.NewStack(
		ui.NuevoBordeDiscontinuo(tema.Borde, tema.RadioMD, "border"),
		container.New(ui.Margen{Izq: m, Arr: m, Der: m, Abj: m},
			container.NewCenter(ui.NuevoRotulo(texto, tema.TextoXS, ui.Normal, "text-muted"))))
}

// anadirEnColumna crea un registro con el valor de esa columna en el campo del tablero
// (la de «sin valor», vacío) y abre su ficha, como «Añadir registro» en la rejilla.
func (p *Pantalla) anadirEnColumna(agrupar api.BasesField, valor string) {
	var datos map[string]any
	if valor != "" {
		datos = map[string]any{agrupar.ID: valor}
	}
	p.crearRegistro(datos)
}

type arrastreTablero struct {
	p        *Pantalla
	campo    api.BasesField
	valores  []string
	columnas []fyne.CanvasObject
	aros     []*canvas.Rectangle
	lado     *ui.DesplazaLado
	scroll   ui.AutoScrollArrastre
}

func (a *arrastreTablero) destino(pos fyne.Position) int {
	if a.lado == nil || !contienePunto(a.lado, pos) {
		return -1
	}
	for i, o := range a.columnas {
		if contienePunto(o, pos) {
			return i
		}
	}
	return -1
}

func contienePunto(o fyne.CanvasObject, pos fyne.Position) bool {
	inicio := fyne.CurrentApp().Driver().AbsolutePositionForObject(o)
	tam := o.Size()
	return pos.X >= inicio.X && pos.Y >= inicio.Y && pos.X < inicio.X+tam.Width && pos.Y < inicio.Y+tam.Height
}

func (a *arrastreTablero) marcar(pos fyne.Position, actual int) {
	destino := a.destino(pos)
	for i, aro := range a.aros {
		if i == destino && i != actual {
			aro.Show()
		} else {
			aro.Hide()
		}
	}
}

func (a *arrastreTablero) limpiar() {
	a.scroll.Parar()
	for _, aro := range a.aros {
		aro.Hide()
	}
}

func (a *arrastreTablero) asa(r api.BasesRecord, actual int) *ui.AsaArrastre {
	asa := ui.NuevaAsaArrastre(a.p.t("board.moveTo", i18n.Vars{"name": etiquetaDeRegistro(a.p.abierta.Campos, r)}))
	base := a.p.abierta
	asa.Puede = func() bool { return a.p.abierta == base && base.Activa == r.TableID && puedeEscribir(base.rol()) }
	asa.AlArrastrar = func(pos fyne.Position) {
		a.marcar(pos, actual)
		a.scroll.Actualizar(a.lado, pos, func(dx, dy float32) {
			a.lado.Scrolled(&fyne.ScrollEvent{Scrolled: fyne.NewDelta(-dx, 0)})
			if a.p.scroll != nil {
				a.p.scroll.Scrolled(&fyne.ScrollEvent{Scrolled: fyne.NewDelta(0, -dy)})
			}
			a.marcar(pos, actual)
		})
	}
	asa.AlSoltar = func(pos fyne.Position) {
		destino := a.destino(pos)
		a.limpiar()
		if destino < 0 || destino == actual {
			return
		}
		var valor any
		if a.valores[destino] != "" {
			valor = a.valores[destino]
		}
		a.p.cambiar(r, map[string]any{a.campo.ID: valor}, nil)
	}
	asa.AlCancelar = a.limpiar
	return asa
}

// etiquetaPresentes es quién está en una tarjeta (`bg-selected text-primary-text
// self-start rounded px-1 text-xs`): a su ancho, a la izquierda.
func etiquetaPresentes(texto string, v tema.Variante) fyne.CanvasObject {
	fondo := canvas.NewRectangle(tema.Color("selected", v))
	fondo.CornerRadius = tema.RadioSM
	rotulo := ui.NuevoRotulo(texto, tema.TextoXS, ui.Normal, "primary-text")
	m := tema.Espacio(1)
	return container.New(ui.Fila{}, container.NewStack(fondo, container.New(ui.Margen{Izq: m, Der: m}, rotulo)))
}
