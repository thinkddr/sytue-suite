// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/nucleo/buscar"
)

// Parrafo es el `<p>` de la suite: un texto que se REPARTE en líneas al ancho que le
// den y que, con tope, recorta con puntos suspensivos como `line-clamp`.
//
// Es un widget y no un puñado de `canvas.Text` porque el reparto depende del ancho, que
// no se sabe hasta el `Layout`: quien lo compone no puede calcular sus líneas al
// construirlo. Y porque el nombre accesible es el párrafo ENTERO, no lo que se ve.
//
// Nació dos veces —`photos.TextoLargo`, que subió aquí como `ui.TextoLargo` para la
// ficha del visor, y `ui.Parrafo`, para la ayuda de «Mi cuenta»— y eran la misma pieza
// con dos nombres: mismo reparto, mismo renderer línea a línea, mismo `Refresh`. Las
// diferencias reales eran dos campos, así que la clase que queda los tiene los dos:
// `Lineas` (el `line-clamp`) y `Ancho` (el reparto declarado). Es lo que pide
// `docs/CONVENTIONS.md` §13.2, y por eso `TextoLargo` ya no existe.
type Parrafo struct {
	widget.BaseWidget

	Texto   string
	Escalon tema.Escalon
	Peso    Peso
	Tinta   color.NRGBA
	// Lineas es el `line-clamp-N`: como mucho N líneas, y la última acaba en elipsis
	// si el texto sigue. Cero es sin tope, que es lo que hace un `<p>` normal.
	Lineas int
	// Ancho es el que se usa para REPARTIR y para declarar el mínimo. Con 0 se reparte
	// al que le dé su padre al colocarlo y el mínimo no declara ancho: dentro de una
	// `Columna` —que suma los mínimos de sus hijos— un párrafo que declarase el ancho
	// que le dieron la última vez impediría encoger. Quien lo mete en una columna de
	// ancho conocido lo dice con `A`.
	Ancho float32
	// Alineacion es text-align. Su valor cero conserva el inicio de línea.
	Alineacion fyne.TextAlign
	// Tachado es el `line-through`: una raya por línea, a lo ancho de lo que se lee en
	// ella (el título de una tarea hecha, que parte en varias).
	Tachado bool
	// Consulta es lo buscado: sus términos se pintan sobre Resalte, con la regla de
	// búsqueda de la suite (`buscar.RangosDeConsulta`), como el `<mark>` de un resultado
	// en la web. Se busca línea a línea: un término partido entre dos líneas no se marca.
	Consulta string
	Resalte  color.NRGBA
}

var (
	_ fyne.Widget     = (*Parrafo)(nil)
	_ fyne.Accessible = (*Parrafo)(nil)
)

// NuevoParrafo crea el párrafo con el color de un token del tema.
func NuevoParrafo(texto string, e tema.Escalon, p Peso, token string, v tema.Variante) *Parrafo {
	return NuevoParrafoConTinta(texto, e, p, tema.Color(token, v))
}

// NuevoParrafoConTinta es NuevoParrafo con el color ya resuelto: lo necesita quien
// pinta sobre una superficie que no es la del token (el cromo de tinta).
func NuevoParrafoConTinta(texto string, e tema.Escalon, p Peso, c color.NRGBA) *Parrafo {
	t := &Parrafo{Texto: texto, Escalon: e, Peso: p, Tinta: c}
	t.ExtendBaseWidget(t)
	return t
}

// A fija el ancho de reparto y devuelve el mismo párrafo, para encadenar.
func (t *Parrafo) A(ancho float32) *Parrafo { t.Ancho = ancho; return t }

// Recortado pone el tope de líneas y devuelve el párrafo, para encadenarlo.
func (t *Parrafo) Recortado(lineas int) *Parrafo { t.Lineas = lineas; return t }

// Tachada lo tacha y devuelve el párrafo, para encadenarlo.
func (t *Parrafo) Tachada() *Parrafo { t.Tachado = true; return t }

// Resaltando marca lo buscado sobre ese fondo y devuelve el párrafo, para encadenarlo.
func (t *Parrafo) Resaltando(consulta string, fondo color.NRGBA) *Parrafo {
	t.Consulta, t.Resalte = consulta, fondo
	return t
}

// AccessibilityRole: es texto.
func (t *Parrafo) AccessibilityRole() fyne.AccessibleRole { return fyne.AccessibleRoleText }

// AccessibilityLabel es el párrafo entero, no lo que quedó tras el recorte. Es lo que
// hace el navegador: `line-clamp` es presentación y el DOM sigue teniendo el texto.
func (t *Parrafo) AccessibilityLabel() string { return t.Texto }

// LineasDe es el reparto del párrafo a un ancho dado, ya con el `line-clamp` aplicado.
// Es la única cuenta: la usan la medida, el renderer y el guardián.
func LineasDe(t *Parrafo, ancho float32) []string {
	if t == nil || t.Texto == "" {
		return nil
	}
	lineas := Repartir(t.Texto, t.Escalon, t.Peso, ancho)
	if t.Lineas <= 0 || len(lineas) <= t.Lineas {
		return lineas
	}
	lineas = lineas[:t.Lineas]
	// La elipsis va al final de la última línea VISIBLE, y si no cabe con ella se le
	// come letras hasta que quepa. Es lo que pinta el navegador al clampar.
	ultima := len(lineas) - 1
	lineas[ultima] = recorta(lineas[ultima]+"…", t.Escalon, t.Peso, ancho)
	return lineas
}

// AltoParrafo es lo que ocupa a un ancho dado: tantas cajas de línea como líneas salgan
// del reparto.
func AltoParrafo(t *Parrafo, ancho float32) float32 {
	return float32(len(LineasDe(t, ancho))) * t.Escalon.Linea
}

// MedidaParrafo es la caja del párrafo a ese ancho. El ancho es un dato de entrada y no
// una medida: un párrafo ocupa el que le den.
func MedidaParrafo(t *Parrafo, ancho float32) fyne.Size {
	return fyne.NewSize(ancho, AltoParrafo(t, ancho))
}

// CreateRenderer arma el párrafo. Termina en Refresh, que es la regla 7 del paquete.
func (t *Parrafo) CreateRenderer() fyne.WidgetRenderer {
	r := &parrafoRenderer{t: t}
	r.Refresh()
	return r
}

type parrafoRenderer struct {
	t       *Parrafo
	lineas  []*canvas.Text
	rayas   []*canvas.Rectangle
	marcas  []marcaParrafo
	ancho   float32
	objetos []fyne.CanvasObject
}

// marcaParrafo es el fondo de un término buscado: en qué línea y entre qué runas.
type marcaParrafo struct {
	r           *canvas.Rectangle
	linea, a, b int
}

// rehacer construye un `canvas.Text` por línea. Solo cuando el ancho CAMBIA: repartir
// en cada Layout costaría una medición por palabra en cada repintado.
func (r *parrafoRenderer) rehacer(ancho float32) {
	if ancho == r.ancho && r.lineas != nil {
		return
	}
	r.ancho = ancho
	r.lineas, r.rayas, r.marcas, r.objetos = nil, nil, nil, nil
	if r.t.Consulta != "" {
		// Los fondos van primero: detrás del texto.
		for i, l := range LineasDe(r.t, ancho) {
			for _, rg := range buscar.RangosDeConsulta([]rune(l), r.t.Consulta) {
				fondo := canvas.NewRectangle(r.t.Resalte)
				fondo.CornerRadius = tema.RadioSM
				r.marcas = append(r.marcas, marcaParrafo{r: fondo, linea: i, a: rg[0], b: rg[1]})
				r.objetos = append(r.objetos, fondo)
			}
		}
	}
	for _, l := range LineasDe(r.t, ancho) {
		c := Texto(l, r.t.Escalon, r.t.Peso, r.t.Tinta)
		r.lineas = append(r.lineas, c)
		r.objetos = append(r.objetos, c)
		if r.t.Tachado {
			raya := canvas.NewRectangle(r.t.Tinta)
			r.rayas = append(r.rayas, raya)
			r.objetos = append(r.objetos, raya)
		}
	}
}

// MinSize declara el ancho solo si el párrafo lo trae puesto; el alto es el del reparto
// al ancho que haya.
func (r *parrafoRenderer) MinSize() fyne.Size {
	if r.t.Ancho > 0 {
		return fyne.NewSize(r.t.Ancho, AltoParrafo(r.t, r.t.Ancho))
	}
	return fyne.NewSize(0, AltoParrafo(r.t, max(r.ancho, 1)))
}

func (r *parrafoRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *parrafoRenderer) Destroy()                     {}

func (r *parrafoRenderer) Layout(t fyne.Size) {
	ancho := t.Width
	if ancho <= 0 {
		ancho = r.t.Ancho
	}
	r.rehacer(ancho)
	e := r.t.Escalon
	xLinea := func(i int) float32 {
		switch r.t.Alineacion {
		case fyne.TextAlignCenter:
			return (ancho - Medir(r.lineas[i].Text, e, r.t.Peso).Width) / 2
		case fyne.TextAlignTrailing:
			return ancho - Medir(r.lineas[i].Text, e, r.t.Peso).Width
		default:
			return 0
		}
	}
	for _, m := range r.marcas {
		texto := []rune(r.lineas[m.linea].Text)
		x := xLinea(m.linea) + Medir(string(texto[:m.a]), e, r.t.Peso).Width
		m.r.Move(fyne.NewPos(x, float32(m.linea)*e.Linea))
		m.r.Resize(fyne.NewSize(Medir(string(texto[m.a:m.b]), e, r.t.Peso).Width, e.Linea))
	}
	for i, l := range r.lineas {
		y := float32(i) * e.Linea
		ColocarTexto(l, e, xLinea(i), y)
		if i < len(r.rayas) {
			// 0,3 em por encima de la base y 0,05 em de grueso (redondeado a un
			// píxel), que es la raya de la Figtree (OS/2).
			grueso := max(float32(1), 0.05*e.Tam)
			r.rayas[i].Move(fyne.NewPos(xLinea(i), y+LineaBase(e, r.t.Peso)-0.3*e.Tam))
			r.rayas[i].Resize(fyne.NewSize(Medir(l.Text, e, r.t.Peso).Width, grueso))
		}
	}
}

func (r *parrafoRenderer) Refresh() {
	r.ancho = 0 // fuerza el reparto: puede haber cambiado el texto
	r.Layout(r.t.Size())
	for _, l := range r.lineas {
		l.Color = r.t.Tinta
		l.Refresh()
	}
	for _, raya := range r.rayas {
		raya.FillColor = r.t.Tinta
		raya.Refresh()
	}
}
