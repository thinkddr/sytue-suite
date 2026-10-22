// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package erp

import (
	"image/color"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// # La tabla del ERP (el `<table>` de `Lista` en ErpApp.tsx)
//
// NO es `ui.Tabla`, y la diferencia no es de estilo: la de la suite es Table.tsx, con
// letra `text-base`, filas que no se pulsan y columnas a partes iguales. La del ERP es
// un `<table class="w-full border-collapse text-sm">` escrito a mano, con la fila
// entera pulsable, una casilla por fila, «Duplicar» a la derecha y el reparto de
// columnas del NAVEGADOR (`table-layout: auto`): cada columna pide lo que ocupa su
// contenido más largo, y lo que sobra se reparte en proporción a eso.
//
// Medido en el arnés del frontal (ErpApp con el servicio de mentira, 1280×800, rem 14,
// 2026-09-23), facturas de cliente con cuatro filas:
//
//	caja          343 → 1197, `rounded-lg border shadow-xs`     (medido)
//	cabecera      35,75 con la mitad de su filete               (medido)
//	fila          42,5 (41,5 + el filete de `divide-y`), 42 la última (medido)
//	columnas      35 · 122,53 · 198,55 · 103,91 · 117,11 · 98,03 · 109,25 · 67,63
//	casilla       13×13 a 12,38 del borde de la fila            (medido)
//	insignia      17,5 centrada; «Duplicar» `xs`, 24 centrado   (medido)

// Medidas de la tabla del ERP.
const (
	// ColCasillaERP es el `w-10` de la columna de casillas.
	ColCasillaERP float32 = 10 * 0.25 * tema.Rem
	// ColDuplicarERP es el `w-16` de la de «Duplicar»: un MÍNIMO, porque el botón pide
	// más (67,63 medido) y en una tabla automática gana el contenido.
	ColDuplicarERP float32 = 16 * 0.25 * tema.Rem
	// CeldaPX y CeldaPY son el `px-3 py-2.5` de cada `td`; CeldaDupPX, el `px-2` de la
	// de «Duplicar».
	CeldaPX    float32 = 3 * 0.25 * tema.Rem
	CeldaPY    float32 = 2.5 * 0.25 * tema.Rem
	CeldaDupPX float32 = 2 * 0.25 * tema.Rem
	// AltoCabeceraERP es la cabecera con la mitad de su filete (`border-collapse`).
	AltoCabeceraERP float32 = 35.75
	// AltoFilaERP es el contenido de una fila: el botón `xs` de «Duplicar» (24) y el
	// `py-2.5`. Con el filete de `divide-y` entre filas, 42,5. Medido.
	AltoFilaERP float32 = 24 + 2*CeldaPY
	// TrackingCabeceraERP es el `tracking-wider` del botón de orden.
	TrackingCabeceraERP float32 = 0.05
	// AnchoFlechaOrden es el `w-2` del hueco de la flecha y SeparaFlecha su `ml-1`.
	AnchoFlechaOrden float32 = 2 * 0.25 * tema.Rem
	SeparaFlecha     float32 = 1 * 0.25 * tema.Rem
)

// cabeceraCasillaY y filaCasillaY son dónde cae la casilla: el navegador la pone en la
// línea base de su caja de línea, no en el centro, así que va 2,4 px por encima del
// medio. Medido: 8,75 bajo la cabecera y 12,38 bajo el borde de la fila.
const (
	cabeceraCasillaY float32 = 8.75
	filaCasillaY     float32 = 12.38
)

// columnaERP es una columna ya resuelta: qué se pinta y con qué letra.
type columnaERP struct {
	clave   string
	rotulo  string
	derecha bool
	// fija es el ancho de las columnas que no entran en el reparto (casilla).
	fija float32
}

// celdaERP es lo que va en una celda: un texto con su letra, o la insignia del estado.
type celdaERP struct {
	texto    string
	mono     bool
	escalon  tema.Escalon
	tinta    string
	insignia bool
	tono     ui.Tono
}

// celdaDe resuelve la celda (fila, columna) como la pinta la web: `ref` en mono `xs`
// apagado, las cifras en mono a la derecha y el estado en su insignia.
func (l *Lista) celdaDe(r Registro, clave string) celdaERP {
	switch clave {
	case "ref":
		t := r.Ref
		if t == "" {
			t = SinValor
		}
		return celdaERP{texto: t, mono: true, escalon: tema.TextoXS, tinta: "text-muted"}
	case "estado":
		if r.Estado == "" {
			return celdaERP{}
		}
		return celdaERP{texto: RotuloEstado(l.Modelo, r.Estado), insignia: true, tono: tonosEstado[Tono(r.Estado)]}
	}
	for _, c := range l.Modelo.Campos {
		if c.Clave == clave {
			return celdaERP{
				texto: Celda(c, r.Data[clave], l.loc, l.Pagina.Etiquetas), mono: AlineadoDerecha(c.Tipo),
				escalon: tema.TextoSM, tinta: "text",
			}
		}
	}
	return celdaERP{texto: SinValor, escalon: tema.TextoSM, tinta: "text"}
}

// medirTexto incluye el tnum heredado de .erp-workspace table (app.css:433).
// También lo heredan las fechas y las cifras dentro de texto sin font-mono.
func medirTexto(s string, e tema.Escalon, mono bool, p ui.Peso) float32 {
	if !mono {
		return ui.MedirTabular(s, e, p)
	}
	t, _ := fyne.CurrentApp().Driver().RenderedTextSize(s, e.Tam, fyne.TextStyle{Monospace: true}, nil)
	return t.Width
}

// anchoCelda es el max-content de una celda: su texto entero, o su insignia, con el
// `px-3` de los dos lados.
func anchoCelda(c celdaERP) float32 {
	switch {
	case c.insignia:
		return ui.MedidaInsignia(c.texto).Width + 2*CeldaPX
	case c.texto == "":
		return 2 * CeldaPX
	}
	return medirTexto(c.texto, c.escalon, c.mono, ui.Normal) + 2*CeldaPX
}

// minCelda es el min-content: el trozo más largo que no se puede partir. El navegador
// parte por los espacios y DESPUÉS de cada guion, así que «FV-2026-00041» se reparte
// en «FV-», «2026-» y «00041» cuando no cabe.
func minCelda(c celdaERP) float32 {
	if c.insignia || c.texto == "" {
		return anchoCelda(c)
	}
	var mayor float32
	for _, p := range trozos(c.texto) {
		mayor = max(mayor, medirTexto(strings.TrimSpace(p), c.escalon, c.mono, ui.Normal))
	}
	return mayor + 2*CeldaPX
}

// trozos parte un texto por sus oportunidades de corte: tras un espacio y tras un
// guion. Cada trozo lleva detrás su separador, para que unirlos dé el original.
func trozos(s string) []string {
	var out []string
	inicio := 0
	for i, r := range s {
		if r == ' ' || r == '-' {
			out = append(out, s[inicio:i+1])
			inicio = i + 1
		}
	}
	if inicio < len(s) {
		out = append(out, s[inicio:])
	}
	return out
}

// lineasCelda reparte el texto de la celda en el ancho que le toque, de forma codiciosa
// como el navegador. Una sola línea si cabe entero.
func lineasCelda(c celdaERP, cabe float32) []string {
	if c.insignia || c.texto == "" || medirTexto(c.texto, c.escalon, c.mono, ui.Normal) <= cabe {
		return []string{c.texto}
	}
	var lineas []string
	actual := ""
	for _, p := range trozos(c.texto) {
		cand := actual + p
		if actual != "" && medirTexto(strings.TrimRight(cand, " "), c.escalon, c.mono, ui.Normal) > cabe {
			lineas = append(lineas, strings.TrimRight(actual, " "))
			actual = p
			continue
		}
		actual = cand
	}
	return append(lineas, strings.TrimRight(actual, " "))
}

// rotuloCabecera es lo que ocupa el botón de orden de una columna: el rótulo en negrita
// con `tracking-wider` —en caja mixta: la celda lleva `sytue-overline` pero se pinta
// sin mayúsculas, medido—, el `ml-1` y el hueco de la flecha. El `px-1` del botón se lo
// come su `-mx-1`.
func rotuloCabecera(rotulo string) float32 {
	return ui.MedirEspaciado(rotulo, tema.TextoXS, ui.Negrita, TrackingCabeceraERP) + SeparaFlecha + AnchoFlechaOrden
}

// RepartoERP es `table-layout: auto` para la tabla del ERP: el ancho de cada columna
// dados los max-content y min-content de las columnas automáticas y lo que miden las
// fijas.
//
// Tres casos, los del navegador:
//   - Si los max-content caben, cada columna recibe el suyo y lo que sobra se reparte
//     en proporción a ellos (medido: 122,53/103,19 = 198,55/167,21 = 1,187).
//   - Si no caben pero los min-content sí, cada una recibe su mínimo y lo que sobra se
//     reparte en proporción a lo que le falta para su máximo.
//   - Si ni eso, el mínimo, y la tabla desborda (su caja lleva `overflow-x-auto`).
func RepartoERP(total float32, maxs, mins, fijas []float32) []float32 {
	out := make([]float32, len(maxs))
	sobra := total
	var sumMax, sumMin float32
	for i := range maxs {
		if fijas[i] > 0 {
			out[i] = fijas[i]
			sobra -= fijas[i]
			continue
		}
		sumMax += maxs[i]
		sumMin += mins[i]
	}
	for i := range maxs {
		if fijas[i] > 0 {
			continue
		}
		switch {
		case sumMax <= 0:
			out[i] = 0
		case sobra >= sumMax:
			out[i] = maxs[i] * sobra / sumMax
		case sobra > sumMin && sumMax > sumMin:
			out[i] = mins[i] + (maxs[i]-mins[i])*(sobra-sumMin)/(sumMax-sumMin)
		default:
			out[i] = mins[i]
		}
	}
	return out
}

// tablaResuelta es la tabla ya medida para un ancho: sus columnas, cuánto mide cada una
// y el alto de cada fila.
type tablaResuelta struct {
	cols   []columnaERP
	anchos []float32
	celdas [][]celdaERP
	altos  []float32
}

// columnasERP son las de la tabla: la casilla, las de `modelo.lista` y la de
// «Duplicar». El rótulo de `ref` es `#`, y la columna sin rótulo —el estado— dice
// «Ordenar», que es lo que escribe la web (`rotulo || t('list.sortBy')`).
func (l *Lista) columnasERP() []columnaERP {
	porClave := map[string]Campo{}
	for _, c := range l.Modelo.Campos {
		porClave[c.Clave] = c
	}
	cols := []columnaERP{{clave: "", fija: ColCasillaERP}}
	for _, clave := range l.Modelo.Lista {
		col := columnaERP{clave: clave}
		switch clave {
		case "ref":
			col.rotulo = "#"
		case "estado":
			col.rotulo = l.t("list.sortBy")
		default:
			col.rotulo = clave
			if c, hay := porClave[clave]; hay {
				col.rotulo, col.derecha = c.Etiqueta, AlineadoDerecha(c.Tipo)
			}
		}
		cols = append(cols, col)
	}
	return append(cols, columnaERP{clave: "·duplicar"})
}

// resolver mide la tabla para `ancho` (el de la caja menos sus dos bordes).
func (l *Lista) resolver(ancho float32) tablaResuelta {
	cols := l.columnasERP()
	rs := l.registros()
	t := tablaResuelta{cols: cols, celdas: make([][]celdaERP, len(rs)), altos: make([]float32, len(rs))}
	maxs, mins, fijas := make([]float32, len(cols)), make([]float32, len(cols)), make([]float32, len(cols))
	dup := max(ColDuplicarERP, ui.MedidaBoton(l.t("action.duplicate"), ui.TallaXS, 0).Width+2*CeldaDupPX)
	for i, c := range cols {
		switch {
		case c.fija > 0:
			fijas[i] = c.fija
		case c.clave == "·duplicar":
			fijas[i] = dup
		default:
			cab := rotuloCabecera(c.rotulo) + 2*CeldaPX
			maxs[i], mins[i] = cab, cab
		}
	}
	for f, r := range rs {
		t.celdas[f] = make([]celdaERP, len(cols))
		for i, c := range cols {
			if c.fija > 0 || c.clave == "·duplicar" {
				continue
			}
			cel := l.celdaDe(r, c.clave)
			t.celdas[f][i] = cel
			maxs[i] = max(maxs[i], anchoCelda(cel))
			mins[i] = max(mins[i], minCelda(cel))
		}
	}
	t.anchos = RepartoERP(ancho, maxs, mins, fijas)
	for f := range rs {
		alto := AltoFilaERP
		for i, cel := range t.celdas[f] {
			if cel.texto == "" || cel.insignia {
				continue
			}
			n := len(lineasCelda(cel, t.anchos[i]-2*CeldaPX))
			alto = max(alto, float32(n)*cel.escalon.Linea+2*CeldaPY)
		}
		t.altos[f] = alto
	}
	return t
}

// AltoTablaERP es lo que ocupa la caja entera con esas filas: bordes, cabecera, filas
// y los filetes de entre ellas.
func AltoTablaERP(altos []float32) float32 {
	alto := 2*tema.Borde + AltoCabeceraERP
	for i, a := range altos {
		alto += a
		if i < len(altos)-1 {
			alto += tema.Borde
		}
	}
	return alto + tema.Borde/2 // la otra mitad del filete de la cabecera
}

// tablaERP arma la caja de la tabla con su cabecera y sus filas para `ancho`.
func (l *Lista) tablaERP(v tema.Variante, ancho float32) (fyne.CanvasObject, float32) {
	res := l.resolver(max(ancho-2*tema.Borde, 0))
	caja := ui.NuevaCaja(tema.Color("surface", v), tema.Color("border", v), radioTarjeta())
	sombra := ui.NuevaSombra(tema.SombraXS, radioTarjeta())
	lay := &cajaTabla{caja: caja, sombra: sombra, v: v, res: res}
	hijos := make([]fyne.CanvasObject, 0, 4+len(l.registros())*2)
	hijos = append(hijos, sombra.Objeto())
	hijos = append(hijos, caja.Objetos()...)
	lay.cabecera = l.cabeceraERP(res)
	hijos = append(hijos, lay.cabecera)
	for f := range l.registros() {
		fila := l.filaERP(v, res, f)
		lay.filas = append(lay.filas, fila)
		hijos = append(hijos, fila)
	}
	for range max(len(res.altos)-1, 0) {
		d := canvas.NewRectangle(tema.Color("border-subtle", v))
		lay.divisores = append(lay.divisores, d)
		hijos = append(hijos, d)
	}
	lay.filete = canvas.NewRectangle(tema.Color("border", v))
	hijos = append(hijos, lay.filete)
	return container.New(lay, hijos...), AltoTablaERP(res.altos)
}

// cajaTabla coloca la caja, la cabecera, las filas y los filetes. Los filetes van
// CENTRADOS en la frontera entre filas, que es lo que hace `border-collapse`.
type cajaTabla struct {
	caja      *ui.Caja
	sombra    *ui.Sombra
	v         tema.Variante
	res       tablaResuelta
	cabecera  fyne.CanvasObject
	filas     []fyne.CanvasObject
	divisores []*canvas.Rectangle
	filete    *canvas.Rectangle
}

func (c *cajaTabla) MinSize([]fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(0, AltoTablaERP(c.res.altos))
}

func (c *cajaTabla) Layout(_ []fyne.CanvasObject, t fyne.Size) {
	c.sombra.Ajustar(fyne.NewPos(0, 0), t, c.v)
	c.caja.Redimensionar(t)
	dentro := max(t.Width-2*tema.Borde, 0)
	x, y := tema.Borde, tema.Borde
	c.cabecera.Move(fyne.NewPos(x, y))
	c.cabecera.Resize(fyne.NewSize(dentro, AltoCabeceraERP-tema.Borde/2))
	y += AltoCabeceraERP
	c.filete.Move(fyne.NewPos(x, y-tema.Borde/2))
	c.filete.Resize(fyne.NewSize(dentro, tema.Borde))
	y += tema.Borde / 2
	for i, f := range c.filas {
		alto := c.res.altos[i]
		f.Move(fyne.NewPos(x, y))
		f.Resize(fyne.NewSize(dentro, alto))
		y += alto
		if i < len(c.divisores) {
			c.divisores[i].Move(fyne.NewPos(x, y))
			c.divisores[i].Resize(fyne.NewSize(dentro, tema.Borde))
			y += tema.Borde
		}
	}
}

// --- La cabecera ------------------------------------------------------------

// cabeceraERP es la fila de `<th>`: la casilla de «todas» y un botón de orden por
// columna, con la flecha en `indicator` en la ordenada.
func (l *Lista) cabeceraERP(res tablaResuelta) fyne.CanvasObject {
	lay := &filaCeldas{anchos: res.anchos}
	var hijos []fyne.CanvasObject
	rs := l.registros()
	todas := NuevaCasillaNativa(l.t("list.all"), len(rs) > 0 && len(l.Marcadas) == len(rs), func() {
		if l.AlMarcarTodas != nil {
			l.AlMarcarTodas()
		}
	})
	lay.casilla, lay.casillaY = todas, cabeceraCasillaY
	hijos = append(hijos, todas)
	for i, c := range res.cols {
		if c.fija > 0 || c.clave == "·duplicar" {
			continue
		}
		col := c
		b := NuevoBotonOrden(col.rotulo, l.Orden.Campo == col.clave, l.Orden.Asc, func() {
			if l.AlOrdenar != nil {
				l.AlOrdenar(l.Orden.Siguiente(col.clave))
			}
		})
		lay.piezas = append(lay.piezas, piezaEn{
			obj: b, col: i, derecha: col.derecha, alto: tema.TextoXS.Linea, px: CeldaPX - BotonOrdenPX,
		})
		hijos = append(hijos, b)
	}
	return container.New(lay, hijos...)
}

// BotonOrden es el botón de una cabecera: el rótulo en negrita espaciada y, si la
// columna es la ordenada, la flecha. Se pulsa, se enfoca y dice su sentido.
type BotonOrden struct {
	ui.Pulsable
	Rotulo      string
	Activo, Asc bool
}

// NuevoBotonOrden crea el botón de orden de una cabecera.
func NuevoBotonOrden(rotulo string, activo, asc bool, alPulsar func()) *BotonOrden {
	o := &BotonOrden{Rotulo: rotulo, Activo: activo, Asc: asc}
	o.Nombre, o.AlPulsar = rotulo, alPulsar
	o.ExtendBaseWidget(o)
	return o
}

// flechaOrden es `↑`/`↓` en la ordenada y nada en las demás.
func (o *BotonOrden) flechaOrden() string {
	switch {
	case !o.Activo:
		return ""
	case o.Asc:
		return "↑"
	}
	return "↓"
}

// BotonOrdenPX es el `px-1` del botón de orden, que su `-mx-1` saca de la celda.
const BotonOrdenPX float32 = 1 * 0.25 * tema.Rem

// MinSize es el botón: el rótulo con su flecha y el `px-1` de los dos lados, por la
// caja de `text-xs`. Medido: «#» 24,66×14, «Cliente» 55,86×14.
func (o *BotonOrden) MinSize() fyne.Size {
	o.ExtendBaseWidget(o)
	return fyne.NewSize(rotuloCabecera(o.Rotulo)+2*BotonOrdenPX, tema.TextoXS.Linea)
}

// CreateRenderer construye el rótulo y la flecha.
func (o *BotonOrden) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &ordenRenderer{
		o:      o,
		aro:    ui.NuevoAro(tema.RadioSM, ui.SinHueco, v),
		rotulo: ui.NuevoEspaciado(o.Rotulo, tema.TextoXS, ui.Negrita, tema.Color("text", v), TrackingCabeceraERP),
		flecha: canvas.NewText(o.flechaOrden(), tema.Color("indicator", v)),
	}
	r.flecha.TextStyle.Monospace = true
	r.flecha.TextSize = tema.TextoXS.Tam
	r.objetos = append(r.aro.Objetos(), r.rotulo.Objetos()...)
	r.objetos = append(r.objetos, r.flecha)
	r.Refresh()
	return r
}

type ordenRenderer struct {
	o       *BotonOrden
	aro     *ui.Aro
	rotulo  *ui.Versal
	flecha  *canvas.Text
	objetos []fyne.CanvasObject
}

func (r *ordenRenderer) MinSize() fyne.Size           { return r.o.MinSize() }
func (r *ordenRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *ordenRenderer) Destroy()                     {}

func (r *ordenRenderer) Layout(t fyne.Size) {
	// El aro va a la caja del botón, con su `px-1` por los dos lados.
	r.aro.Colocar(t)
	r.rotulo.Colocar(BotonOrdenPX, 0)
	x := BotonOrdenPX + r.rotulo.Ancho() + SeparaFlecha
	ui.ColocarTexto(r.flecha, tema.TextoXS, x, 0)
}

func (r *ordenRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r.aro.Ver(r.o.Enfocado())
	r.aro.Pintar(v)
	r.flecha.Text = r.o.flechaOrden()
	r.flecha.Refresh()
}

// --- Las filas --------------------------------------------------------------

// filaERP es una fila entera: pulsable, con su casilla, sus celdas y «Duplicar».
func (l *Lista) filaERP(v tema.Variante, res tablaResuelta, f int) fyne.CanvasObject {
	r := l.registros()[f]
	fila := NuevaFilaERP(NombreDe(r), func() {
		if l.AlAbrir != nil {
			l.AlAbrir(r)
		}
	})
	fila.Marcada = l.Marcadas[r.ID]
	lay := &filaCeldas{anchos: res.anchos}
	hijos := []fyne.CanvasObject{fila}
	casilla := NuevaCasillaNativa(NombreDe(r), fila.Marcada, func() {
		if l.AlMarcar != nil {
			l.AlMarcar(r.ID)
		}
	})
	lay.casilla, lay.casillaY = casilla, filaCasillaY
	lay.fondo = fila
	hijos = append(hijos, casilla)
	for i, cel := range res.celdas[f] {
		c := res.cols[i]
		if c.fija > 0 || c.clave == "·duplicar" || cel.texto == "" {
			continue
		}
		if cel.insignia {
			ins := ui.NuevaInsignia(cel.texto, cel.tono)
			lay.piezas = append(lay.piezas, piezaEn{obj: ins, col: i, alto: ui.MedidaInsignia(cel.texto).Height})
			hijos = append(hijos, ins)
			continue
		}
		for n, linea := range lineasCelda(cel, res.anchos[i]-2*CeldaPX) {
			tx := textoEn{
				col: i, linea: n, e: cel.escalon, derecha: c.derecha,
				ancho: medirTexto(linea, cel.escalon, cel.mono, ui.Normal),
			}
			if !cel.mono && strings.ContainsAny(linea, "0123456789") {
				// Fyne canvas.Text no activa OpenType tnum: la pieza común mide
				// y pinta cada cifra en el avance tabular de esa misma fuente.
				tx.tabular = ui.NuevoTextoTabular(linea, cel.escalon, ui.Normal, tema.Color(cel.tinta, v))
				hijos = append(hijos, tx.tabular.Objetos()...)
			} else {
				tx.t = textoCelda(linea, cel, v)
				hijos = append(hijos, tx.t)
			}
			lay.textos = append(lay.textos, tx)
		}
		lay.lineas = append(lay.lineas, len(lineasCelda(cel, res.anchos[i]-2*CeldaPX)))
	}
	if l.AlDuplicar != nil {
		dup := ui.NuevoBoton(l.t("action.duplicate"), func() { l.AlDuplicar(r) }).Con(ui.Fantasma, ui.TallaXS)
		lay.piezas = append(lay.piezas, piezaEn{obj: dup, col: len(res.cols) - 1, derecha: true, alto: dup.MinSize().Height, px: CeldaDupPX})
		hijos = append(hijos, dup)
	}
	return container.New(lay, hijos...)
}

// textoCelda es el `canvas.Text` de una línea de celda, con su letra.
func textoCelda(s string, c celdaERP, v tema.Variante) *canvas.Text {
	if !c.mono {
		return ui.Texto(s, c.escalon, ui.Normal, tema.Color(c.tinta, v))
	}
	t := canvas.NewText(s, tema.Color(c.tinta, v))
	t.TextStyle.Monospace = true
	t.TextSize = c.escalon.Tam
	return t
}

// piezaEn es un objeto colocado en una columna, centrado en vertical.
type piezaEn struct {
	obj     fyne.CanvasObject
	col     int
	derecha bool
	alto    float32
	// px es el relleno horizontal de su celda; cero es el `px-3` de siempre.
	px float32
}

// textoEn es una línea de texto de una celda.
type textoEn struct {
	t       *canvas.Text
	tabular *ui.TextoTabular
	col     int
	linea   int
	e       tema.Escalon
	derecha bool
	ancho   float32
}

// filaCeldas coloca las piezas de una fila (o de la cabecera) en sus columnas. Las
// celdas van con `vertical-align: middle`: el bloque de líneas de cada una, centrado.
type filaCeldas struct {
	anchos   []float32
	fondo    fyne.CanvasObject
	casilla  fyne.CanvasObject
	casillaY float32
	piezas   []piezaEn
	textos   []textoEn
	lineas   []int
}

func (f *filaCeldas) MinSize([]fyne.CanvasObject) fyne.Size { return fyne.NewSize(0, 0) }

func (f *filaCeldas) xDe(col int) float32 {
	var x float32
	for i := range col {
		x += f.anchos[i]
	}
	return x
}

func (f *filaCeldas) Layout(_ []fyne.CanvasObject, t fyne.Size) {
	if f.fondo != nil {
		f.fondo.Move(fyne.NewPos(0, 0))
		f.fondo.Resize(t)
	}
	if f.casilla != nil {
		f.casilla.Resize(fyne.NewSize(LadoCasillaNativa, LadoCasillaNativa))
		f.casilla.Move(fyne.NewPos((f.anchos[0]-LadoCasillaNativa)/2, f.casillaY))
	}
	for _, p := range f.piezas {
		px := p.px
		if px == 0 {
			px = CeldaPX
		}
		m := p.obj.MinSize()
		x := f.xDe(p.col) + px
		if p.derecha {
			x = f.xDe(p.col) + f.anchos[p.col] - px - m.Width
		}
		p.obj.Resize(fyne.NewSize(m.Width, p.alto))
		p.obj.Move(fyne.NewPos(x, (t.Height-p.alto)/2))
	}
	// Cuántas líneas tiene cada celda, para centrar su bloque.
	porCol := map[int]int{}
	for _, tx := range f.textos {
		porCol[tx.col] = max(porCol[tx.col], tx.linea+1)
	}
	for _, tx := range f.textos {
		bloque := float32(porCol[tx.col]) * tx.e.Linea
		y := (t.Height-bloque)/2 + float32(tx.linea)*tx.e.Linea
		x := f.xDe(tx.col) + CeldaPX
		if tx.derecha {
			x = f.xDe(tx.col) + f.anchos[tx.col] - CeldaPX - tx.ancho
		}
		if tx.tabular != nil {
			tx.tabular.Colocar(x, y, tx.e)
		} else {
			ui.ColocarTexto(tx.t, tx.e, x, y)
		}
	}
}

// FilaERP es el fondo pulsable de una fila: `hover:bg-bg-subtle`, `bg-surface-raised`
// si está marcada y el aro por DENTRO (`sytue-focus-ring-inset`). Abre la ficha con el
// ratón, con Intro y con Espacio, que son las tres teclas de la web.
type FilaERP struct {
	ui.Pulsable
	Marcada bool
}

// NuevaFilaERP crea el fondo de una fila.
func NuevaFilaERP(nombre string, alPulsar func()) *FilaERP {
	f := &FilaERP{}
	f.Nombre, f.AlPulsar = nombre, alPulsar
	f.ExtendBaseWidget(f)
	return f
}

// MinSize: la fila la mide su tabla.
func (f *FilaERP) MinSize() fyne.Size {
	f.ExtendBaseWidget(f)
	return fyne.NewSize(0, AltoFilaERP)
}

// CreateRenderer construye el fondo y el aro.
func (f *FilaERP) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &filaERPRenderer{f: f, fondo: canvas.NewRectangle(color.NRGBA{}), aro: canvas.NewRectangle(color.NRGBA{})}
	r.aro.StrokeWidth = 2
	r.aro.StrokeColor = tema.Color("focus-ring", v)
	r.Refresh()
	return r
}

type filaERPRenderer struct {
	f          *FilaERP
	fondo, aro *canvas.Rectangle
}

func (r *filaERPRenderer) MinSize() fyne.Size { return r.f.MinSize() }
func (r *filaERPRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{r.fondo, r.aro}
}
func (r *filaERPRenderer) Destroy() {}

func (r *filaERPRenderer) Layout(t fyne.Size) {
	r.fondo.Resize(t)
	// El aro va POR DENTRO: dos píxeles de trazo centrados a uno del borde.
	r.aro.Move(fyne.NewPos(1, 1))
	r.aro.Resize(fyne.NewSize(max(t.Width-2, 0), max(t.Height-2, 0)))
}

func (r *filaERPRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	fondo := color.NRGBA{}
	switch {
	case r.f.Encima():
		fondo = tema.Color("bg-subtle", v)
	case r.f.Marcada:
		fondo = tema.Color("surface-raised", v)
	}
	r.fondo.FillColor = fondo
	r.fondo.Refresh()
	r.aro.Hidden = !r.f.Enfocado()
	r.aro.Refresh()
}
