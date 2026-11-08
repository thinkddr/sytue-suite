// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package bases

import (
	"image/color"
	"strconv"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"

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
// tarjeta por registro. Una tarjeta se mueve con su selector «Mover a», que vale igual
// con ratón y con teclado. Mover es escribir ese campo en el registro: lo decide y lo
// valida el servicio, como en la rejilla.
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

// tarjetaDeTablero es la tarjeta de un registro: su nombre, dos campos más y, si se puede
// editar, el selector que la mueve de columna.
func (p *Pantalla) tarjetaDeTablero(v tema.Variante, r api.BasesRecord, agrupar api.BasesField, otros []api.BasesField, columnas []string, actual int, puedeEditar bool, asa *ui.AsaArrastre) fyne.CanvasObject {
	b := p.abierta
	nombre := etiquetaDeRegistro(b.Campos, r)
	titulo := ui.NuevoParrafo(nombre, tema.TextoSM, ui.SemiNegrita, "text", v).A(anchoTarjeta)
	var cabeza fyne.CanvasObject = titulo
	if asa != nil {
		titulo.A(anchoTarjeta - asa.MinSize().Width - tema.Espacio(2))
		cabeza = container.New(ui.FilaAuto{Hueco: tema.Espacio(2), Derecha: 1}, titulo, asa)
	}
	hijos := []fyne.CanvasObject{cabeza}
	// Como la web (`.slice(1, 3)`): el primero con valor suele ser el propio nombre.
	conValor := 0
	for _, c := range otros {
		texto := textoDeValor(r.Data[c.ID])
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
	if puedeEditar {
		mover := ui.NuevoSelector(columnas, func(i int) {
			if i == actual {
				return
			}
			var valor any
			if i < len(agrupar.Choices) {
				valor = agrupar.Choices[i]
			}
			p.cambiar(r, map[string]any{agrupar.ID: valor}, nil)
		}).Con(ui.CampoSM)
		mover.Rotulo, mover.Elegida = p.t("board.moveTo", i18n.Vars{"name": nombre}), actual
		if asa != nil {
			asa.AlPulsar = mover.Alternar
		}
		hijos = append(hijos, mover)
	}
	return caja(tema.Color("surface", v), tema.RadioMD, margenTablero, v, container.New(ui.Columna{Hueco: tema.Espacio(2)}, hijos...))
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
