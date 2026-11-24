// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package notes

import (
	"slices"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// LA BARRA DE TABLA: sale debajo de la de formato SOLO con el cursor en una celda (no tapa
// nada: empuja el cuerpo), y es la misma pieza que la de formato —una fila, pistas con
// el atajo en el escritorio, 44 con el dedo y lo que no cabe en «Más»—, en grupos:
//
//	filas      insertar encima | debajo
//	columnas   insertar a la izquierda | a la derecha
//	cabeceras  fila de cabecera (`<thead>`) | columna de cabecera (`<th scope="row">`)
//	celda      fondo de la celda (`data-fondo`, los seis colores) | alineación y sangría
//	borrar     la fila | la columna | la tabla
//
// Tab y Mayús+Tab van de celda en celda; Tab en la última añade una fila, como en
// cualquier editor de tablas. Combinar o dividir celdas, numerar filas y repartir el ancho
// de las columnas no están: el modelo no guarda anchos y el formato compartido no tiene
// ni numeración ni combinación propias (ver el informe).

// Los glifos de la barra de tabla: la rejilla con lo que cambia marcado.
const (
	FilaEncimaD   = "M4 13h16v6H4zM12 4v6M9 7h6"
	FilaDebajoD   = "M4 5h16v6H4zM12 14v6M9 17h6"
	ColAntesD     = "M13 4h6v16h-6zM7 9v6M4 12h6"
	ColDespuesD   = "M5 4h6v16H5zM17 9v6M14 12h6"
	BorrarFilaD   = "M4 9h16v6H4zM10 10.5l4 3M14 10.5l-4 3"
	BorrarColD    = "M9 4h6v16H9zM10.5 10l3 4M13.5 10l-3 4"
	CabeceraFilaD = "M4 5h16v14H4zM4 10h16M4 7.5h16"
	CabeceraColD  = "M4 5h16v14H4zM10 5v14M7 5v14"
	FondoCeldaD   = "M4 5h16v14H4zM4 12h16M12 5v14M5.5 6.5h5v4h-5z"
)

// Grupos propios de la barra de tabla (se reparten igual que los de formato).
const (
	grupoTablaFilas grupoBarra = iota
	grupoTablaColumnas
	grupoTablaCabeceras
	grupoTablaCelda
	grupoTablaBorrar
)

// accionesTabla son las de la barra de tabla, en su orden.
func (e *EditorNota) accionesTabla() []accionBarra {
	t := func(k string) string { return e.tr.T(k, nil) }
	orden := func(o string) func(fyne.CanvasObject) {
		return func(fyne.CanvasObject) { e.Orden(o, "") }
	}
	return []accionBarra{
		{clave: "filaEncima", grupo: grupoTablaFilas, prioridad: prioAlta, glifo: FilaEncimaD, rotulo: t("editor.rowAbove"), pulsar: orden("filaEncima")},
		{clave: "filaDebajo", grupo: grupoTablaFilas, prioridad: prioEsencial, glifo: FilaDebajoD, rotulo: t("editor.rowBelow"), teclas: "Tab", pulsar: orden("filaDebajo")},
		{clave: "colAntes", grupo: grupoTablaColumnas, prioridad: prioMedia, glifo: ColAntesD, rotulo: t("editor.colBefore"), pulsar: orden("colAntes")},
		{clave: "colDespues", grupo: grupoTablaColumnas, prioridad: prioEsencial, glifo: ColDespuesD, rotulo: t("editor.colAfter"), pulsar: orden("colDespues")},
		{clave: "cabeceraFila", grupo: grupoTablaCabeceras, prioridad: prioFormato, glifo: CabeceraFilaD, rotulo: t("editor.headerRow"), pulsar: orden("cabeceraFila")},
		{clave: "cabeceraCol", grupo: grupoTablaCabeceras, prioridad: prioColor, glifo: CabeceraColD, rotulo: t("editor.headerColumn"), pulsar: orden("cabeceraCol")},
		{clave: "fondoCelda", grupo: grupoTablaCelda, prioridad: prioColor, glifo: FondoCeldaD, rotulo: t("editor.cellBackground"), abre: true, pulsar: e.menuFondoCelda},
		{clave: "alinearCelda", grupo: grupoTablaCelda, prioridad: prioInsertar, glifo: IzquierdaD, rotulo: t("editor.align"), abre: true, pulsar: e.menuAlinear},
		{clave: "borrarFila", grupo: grupoTablaBorrar, prioridad: prioMedia, glifo: BorrarFilaD, rotulo: t("editor.rowDelete"), pulsar: orden("borrarFila")},
		{clave: "borrarCol", grupo: grupoTablaBorrar, prioridad: prioFormato, glifo: BorrarColD, rotulo: t("editor.colDelete"), pulsar: orden("borrarCol")},
		{clave: "borrarTabla", grupo: grupoTablaBorrar, prioridad: prioAlta, glifo: PapeleraD, rotulo: t("editor.tableDelete"), pulsar: orden("borrarTabla")},
	}
}

// armarBarraTabla es la barra de tabla (ver la cabecera).
func (e *EditorNota) armarBarraTabla() *fyne.Container {
	e.tablaEd = e.nuevaBarraEd(e.accionesTabla())
	return e.tablaEd.c
}

// menuFondoCelda ofrece los seis fondos de celda y «sin fondo», el actual marcado.
func (e *EditorNota) menuFondoCelda(disparador fyne.CanvasObject) {
	bl := e.bloqueActual()
	if !bl.Celda.EnTabla() {
		return
	}
	actual := ""
	if t := bl.Celda.Tabla; bl.Celda.Fila < len(t.Filas) && bl.Celda.Col < len(t.Filas[bl.Celda.Fila].Celdas) {
		actual = fondoDe(t.Filas[bl.Celda.Fila].Celdas[bl.Celda.Col])
	}
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	elementos := []fyne.CanvasObject{}
	opcion := func(rotulo, valor string) {
		el := ui.NuevoElementoMenu(rotulo, iconos.Ninguno, func() {
			e.Orden("fondoCelda", valor)
			e.reenfocar()
		})
		el.Marcada = valor == actual
		if valor != "" {
			fondo, _ := coloresEstado(valor)
			el.PuntoColor, el.TamIcono = tema.Color(fondo, v), ui.LadoPuntoMenu
		}
		elementos = append(elementos, el)
	}
	opcion(e.tr.T("editor.colorDefault", nil), "")
	elementos = append(elementos, ui.NuevoSeparadorMenu())
	for _, c := range FondosPermitidos {
		opcion(e.tr.T(ClavesColorEstado[c], nil), c)
	}
	m := ui.NuevoMenu(elementos...)
	m.Rotulo, m.AlInicio = e.tr.T("editor.cellBackground", nil), true
	m.Mostrar(disparador)
}

// cabeceraDeFila dice si la tabla tiene fila de cabecera: la primera, en `<thead>`.
func cabeceraDeFila(t *Tabla) bool { return len(t.Filas) > 0 && t.Filas[0].Seccion == "thead" }

// cabeceraDeColumna dice si la primera celda de cada fila del cuerpo es `<th scope="row">`.
func cabeceraDeColumna(t *Tabla) bool {
	hay := false
	for _, f := range t.Filas {
		if f.Seccion == "thead" || len(f.Celdas) == 0 {
			continue
		}
		c := f.Celdas[0]
		if !c.Cabecera || attr(c.Atributos, "scope") != "row" {
			return false
		}
		hay = true
	}
	return hay
}

// FormatearTabla cambia la forma de la tabla del cursor sin tocar sus celdas: la fila de
// cabecera, la columna de cabecera o el fondo de la celda (`valor`). Es una `Tabla` nueva
// (las fotos de deshacer comparten la vieja).
func (d *Documento) FormatearTabla(orden, valor string, p Pos) {
	if len(d.Bloques) == 0 || !d.Bloques[p.B].Celda.EnTabla() {
		return
	}
	en := d.Bloques[p.B].Celda
	i, j := d.rangoTabla(p.B)
	vieja := en.Tabla
	filas := make([]FilaTabla, len(vieja.Filas))
	for f, fila := range vieja.Filas {
		fila.Celdas = slices.Clone(fila.Celdas)
		filas[f] = fila
	}
	switch orden {
	case "cabeceraFila":
		if len(filas) == 0 {
			return
		}
		ponerCabeceraFila(filas, !cabeceraDeFila(vieja))
	case "cabeceraCol":
		ponerCabeceraColumna(filas, !cabeceraDeColumna(vieja))
	case "fondoCelda":
		if valor != "" && !slices.Contains(FondosPermitidos, valor) {
			return
		}
		if en.Fila < len(filas) && en.Col < len(filas[en.Fila].Celdas) {
			c := &filas[en.Fila].Celdas[en.Col]
			c.Atributos = conAtributo(c.Atributos, "data-fondo", valor)
		}
	default:
		return
	}
	t := &Tabla{Atributos: vieja.Atributos, Filas: filas}
	for k := i; k < j; k++ {
		d.Bloques[k].Celda.Tabla = t
	}
}

// irACelda es Tab (o Mayús+Tab) en una tabla: el cursor al principio de la celda
// siguiente (o la anterior). Tab en la última añade una fila y va a su primera celda.
func (e *EditorNota) irACelda(atras bool) {
	if !e.enTabla() {
		return
	}
	en := e.bloqueActual().Celda
	i, j := e.doc.rangoTabla(e.cur.B)
	destino := -1
	if atras {
		for k := e.cur.B - 1; k >= i; k-- {
			if e.doc.Bloques[k].Celda != en {
				c := e.doc.Bloques[k].Celda
				for k > i && e.doc.Bloques[k-1].Celda == c {
					k-- // el primer bloque de esa celda
				}
				destino = k
				break
			}
		}
	} else {
		for k := e.cur.B + 1; k < j; k++ {
			if e.doc.Bloques[k].Celda != en {
				destino = k
				break
			}
		}
	}
	switch {
	case destino >= 0:
		e.cur = Pos{destino, inicioEditable(e.doc.Bloques[destino])}
		e.ancla = e.cur
		e.escribiendo, e.pendiente = false, nil
		e.repintar()
	case !atras:
		// Tab en la última celda: una fila nueva debajo, y a su primera celda (la
		// siguiente en el orden de lectura).
		e.Orden("filaDebajo", "")
		e.irACelda(false)
	}
}

// ponerCabeceraFila pone (o quita) la primera fila en `<thead>` con sus celdas `<th>`.
func ponerCabeceraFila(filas []FilaTabla, poner bool) {
	filas[0].Seccion, filas[0].Nueva, filas[0].AtributosSeccion = "tbody", true, nil
	if poner {
		filas[0].Seccion = "thead"
	}
	for c := range filas[0].Celdas {
		filas[0].Celdas[c].Cabecera = poner
		filas[0].Celdas[c].Atributos = conAtributo(filas[0].Celdas[c].Atributos, "scope", "")
	}
	if len(filas) > 1 {
		// Detrás del `<thead>` abre su `<tbody>`; sin cabecera, sigue en el de la primera.
		filas[1].Nueva = poner || filas[1].Seccion != "tbody"
	}
}

// ponerCabeceraColumna pone (o quita) la primera celda de cada fila del cuerpo como
// `<th scope="row">`.
func ponerCabeceraColumna(filas []FilaTabla, poner bool) {
	scope := ""
	if poner {
		scope = "row"
	}
	for f := range filas {
		if filas[f].Seccion == "thead" || len(filas[f].Celdas) == 0 {
			continue
		}
		c := &filas[f].Celdas[0]
		c.Cabecera, c.Atributos = poner, conAtributo(c.Atributos, "scope", scope)
	}
}
