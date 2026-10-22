// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package bases

import (
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
	// anchoColumna es el `w-64` de una columna y margenTablero el `p-2` de columnas y tarjetas.
	anchoColumna  = 16 * float32(tema.Rem)
	margenTablero = tema.Espacio(2)
	// anchoTarjeta es lo que queda dentro de una tarjeta: la columna sin su `p-2` y su
	// borde, y la tarjeta sin los suyos.
	anchoTarjeta = anchoColumna - 4*margenTablero - 4*tema.Borde
)

// caja es `border-border rounded-* border p-2` con su fondo: el contenido dentro, con su margen.
func caja(fondo string, radio float32, v tema.Variante, contenido fyne.CanvasObject) fyne.CanvasObject {
	r := canvas.NewRectangle(tema.Color(fondo, v))
	r.CornerRadius, r.StrokeWidth, r.StrokeColor = radio, tema.Borde, tema.Color("border", v)
	m := margenTablero + tema.Borde
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
//
// ponytail: sin arrastrar y soltar, como en la web. El selector cubre todos los casos; el
// arrastre sería un atajo solo de puntero y se añade si alguien lo pide.
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
	for i, col := range cols {
		hijos := make([]fyne.CanvasObject, 0, 1+len(col.registros))
		hijos = append(hijos, container.New(ui.Margen{Izq: tema.Espacio(1), Der: tema.Espacio(1)},
			container.New(ui.Fila{Hueco: tema.Espacio(1)},
				ui.NuevoRotulo(nombres[i], tema.TextoSM, ui.SemiNegrita, "text"),
				ui.NuevoRotulo("("+strconv.Itoa(len(col.registros))+")", tema.TextoSM, ui.Normal, "text-muted"))))
		for _, r := range col.registros {
			hijos = append(hijos, p.tarjetaDeTablero(v, r, agrupar, otros, nombres, i, puedeEditar))
		}
		columnas[i] = container.New(anchoFijo{Ancho: anchoColumna},
			caja("bg-subtle", tema.RadioLG, v, container.New(ui.Columna{Hueco: tema.Espacio(2)}, hijos...)))
	}
	// `overflow-x-auto`: las columnas se desplazan de lado y la rueda sigue bajando la página.
	fila := container.New(ui.Margen{Abj: tema.Espacio(2)}, container.New(ui.Fila{Hueco: tema.Espacio(3)}, columnas...))
	lado := ui.NuevoDesplazaLado(fila, 0, func() fyne.Scrollable {
		if p.scroll == nil {
			return nil
		}
		return p.scroll
	})
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
func (p *Pantalla) tarjetaDeTablero(v tema.Variante, r api.BasesRecord, agrupar api.BasesField, otros []api.BasesField, columnas []string, actual int, puedeEditar bool) fyne.CanvasObject {
	b := p.abierta
	nombre := etiquetaDeRegistro(b.Campos, r)
	hijos := []fyne.CanvasObject{ui.NuevoParrafo(nombre, tema.TextoSM, ui.SemiNegrita, "text", v).A(anchoTarjeta)}
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
		hijos = append(hijos, mover)
	}
	return caja("surface", tema.RadioMD, v, container.New(ui.Columna{Hueco: tema.Espacio(1)}, hijos...))
}
