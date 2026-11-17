// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// # La hoja de atajos de teclado (`ShortcutsModal.tsx`)
//
// Existe porque los atajos que nadie enseña no los usa nadie. El contenido lo declara
// cada app —son sus teclas y sus palabras—; aquí vive la forma: el diálogo `md`, los
// grupos y las teclas dibujadas como teclas. Nació en Sytue Mail y se subió aquí cuando
// Calendar necesitó la suya: la misma hoja no se copia.

// AtajoDeTeclado es una fila de la hoja: las teclas que hacen lo mismo («n» y «j»),
// ya en el idioma de quien lee («Ctrl+↵»), y lo que hacen.
type AtajoDeTeclado struct {
	Teclas      []string
	Descripcion string
}

// GrupoDeAtajos es un apartado de la hoja, con su rótulo.
type GrupoDeAtajos struct {
	Titulo string
	Atajos []AtajoDeTeclado
}

// MostrarHojaAtajos abre la hoja sobre el lienzo: un diálogo `md` con los grupos y el
// botón `cerrar` («Cerrar», ya traducido) en el pie. Devuelve el diálogo.
func MostrarHojaAtajos(lienzo fyne.Canvas, titulo, cerrar string, grupos []GrupoDeAtajos) *Dialogo {
	ancho := AnchoCuerpoTalla(DialogoMD, fyne.NewSize(lienzo.Size().Width, 0))
	columna := ColumnaTeclasAtajos(grupos, ancho)
	secciones := make([]fyne.CanvasObject, 0, len(grupos))
	for _, g := range grupos {
		secciones = append(secciones, nuevaSeccionAtajos(g, ancho, columna))
	}
	var d *Dialogo
	boton := NuevoBoton(cerrar, func() { d.Cerrar() })
	d = NuevoDialogo(titulo, container.New(Columna{Hueco: huecoGruposAtajos}, secciones...)).Con(DialogoMD, boton)
	d.RotuloCerrar = cerrar
	d.Mostrar(lienzo)
	return d
}

// Las medidas de la hoja: grupos con `gap-5`, el rótulo en `sytue-overline mb-2`, filas
// con `py-1.5` separadas por `divide-y` a `border/60`, `gap-x-4` entre las teclas y lo
// que hacen, y cada tecla la `<Kbd>` del papel (TeclaSuperficie) con `gap-1` entre ellas
// —también entre líneas, cuando una combinación larga baja de línea—.
const (
	huecoGruposAtajos float32 = 5 * 0.25 * tema.Rem
	bajoRotuloAtajos  float32 = 2 * 0.25 * tema.Rem
	rellenoFilaAtajo  float32 = 1.5 * 0.25 * tema.Rem
	huecoFilaAtajo    float32 = 4 * 0.25 * tema.Rem
	huecoTeclas       float32 = 1 * 0.25 * tema.Rem
)

// AltoFilaAtajo es una fila de una sola línea: su `py-1.5` y lo más alto de la tecla
// (17,5, que hoy manda) y la línea de `text-sm`. Una combinación que baja de línea o una
// descripción que se parte la hacen más alta (ver altoFilaAtajo).
var AltoFilaAtajo = 2*rellenoFilaAtajo + max(LadoTecla, tema.TextoSM.Linea)

// ColumnaTeclasAtajos es el ancho de la columna de teclas, UNA para toda la hoja: el
// `grid-cols-[fit-content(50%)_minmax(0,1fr)]` con `subgrid` de `ShortcutsModal.tsx`.
//
// Antes cada fila ponía la descripción tras SUS teclas, así que «?» y «Ctrl K» dejaban
// el texto en dos verticales distintas y la hoja se leía en zigzag. Ahora la columna
// mide lo que la combinación más ancha, sin pasar de media hoja; y nunca menos que la
// tecla más ancha, que no se parte (`fit-content`: el mínimo de su contenido manda).
func ColumnaTeclasAtajos(grupos []GrupoDeAtajos, ancho float32) float32 {
	var entera, minima float32
	for _, g := range grupos {
		for _, a := range g.Atajos {
			anchos := anchosTeclas(a.Teclas)
			fila := float32(0)
			for i, w := range anchos {
				if i > 0 {
					fila += huecoTeclas
				}
				fila += w
				minima = max(minima, w)
			}
			entera = max(entera, fila)
		}
	}
	return max(minima, min(entera, ancho/2))
}

// anchosTeclas es lo que mide cada tecla de la combinación, ya dibujada.
func anchosTeclas(teclas []string) []float32 {
	anchos := make([]float32, len(teclas))
	for i, k := range teclas {
		anchos[i] = MedidaTecla(TeclaSuperficie(k)).Width
	}
	return anchos
}

// lineasTeclas reparte una combinación en la columna: el `flex-wrap` del `<dt>`.
func lineasTeclas(anchos []float32, columna float32) [][]int {
	return RepartirEnLineas(anchos, columna, func(int) float32 { return huecoTeclas })
}

// altoTeclas es lo que ocupan `n` líneas de teclas con su `gap-1` entre ellas.
func altoTeclas(n int) float32 {
	if n == 0 {
		return 0
	}
	return float32(n)*LadoTecla + float32(n-1)*huecoTeclas
}

// seccionAtajos es un grupo de la hoja.
type seccionAtajos struct {
	widget.BaseWidget
	g       GrupoDeAtajos
	ancho   float32
	columna float32
}

func nuevaSeccionAtajos(g GrupoDeAtajos, ancho, columna float32) *seccionAtajos {
	s := &seccionAtajos{g: g, ancho: ancho, columna: columna}
	s.ExtendBaseWidget(s)
	return s
}

// anchoTexto es lo que le queda a la descripción a ese ancho de sección.
func (s *seccionAtajos) anchoTexto(ancho float32) float32 {
	return max(ancho-s.columna-huecoFilaAtajo, 0)
}

// altoFilaAtajo es una fila a ese ancho de descripción: el `py-1.5` y lo más alto de
// sus dos columnas, teclas (con sus líneas) o descripción (con las suyas).
func (s *seccionAtajos) altoFilaAtajo(a AtajoDeTeclado, anchoTexto float32) float32 {
	teclas := altoTeclas(len(lineasTeclas(anchosTeclas(a.Teclas), s.columna)))
	texto := float32(len(Repartir(a.Descripcion, tema.TextoSM, Normal, anchoTexto))) * tema.TextoSM.Linea
	return 2*rellenoFilaAtajo + max(teclas, texto, LadoTecla, tema.TextoSM.Linea)
}

func (s *seccionAtajos) MinSize() fyne.Size {
	alto := tema.TextoXS.Linea + bajoRotuloAtajos
	for i, a := range s.g.Atajos {
		if i > 0 {
			alto += tema.Borde
		}
		alto += s.altoFilaAtajo(a, s.anchoTexto(s.ancho))
	}
	return fyne.NewSize(s.ancho, alto)
}

func (s *seccionAtajos) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &seccionAtajosRenderer{s: s, rotulo: NuevoVersal(s.g.Titulo, tema.TextoXS, SemiNegrita, tema.Color("text-subtle", v))}
	r.objetos = append(r.objetos, r.rotulo.Objetos()...)
	for i, a := range s.g.Atajos {
		if i > 0 {
			f := canvas.NewRectangle(veloAtajos(tema.Color("border", v)))
			r.filetes = append(r.filetes, f)
			r.objetos = append(r.objetos, f)
		}
		fila := make([]*Tecla, 0, len(a.Teclas))
		for _, k := range a.Teclas {
			tecla := TeclaSuperficie(k)
			fila = append(fila, tecla)
			r.objetos = append(r.objetos, tecla)
		}
		// Un párrafo y no un `canvas.Text`: en la hoja del teléfono una descripción larga
		// se parte en líneas, como el `<dd>` de la web, en vez de salirse por el canto.
		texto := NuevoParrafoConTinta(a.Descripcion, tema.TextoSM, Normal, tema.Color("text", v))
		r.teclas, r.textos = append(r.teclas, fila), append(r.textos, texto)
		r.objetos = append(r.objetos, texto)
	}
	r.Refresh()
	return r
}

type seccionAtajosRenderer struct {
	s       *seccionAtajos
	rotulo  *Versal
	filetes []*canvas.Rectangle
	teclas  [][]*Tecla // las de cada fila
	textos  []*Parrafo
	objetos []fyne.CanvasObject
}

func (r *seccionAtajosRenderer) MinSize() fyne.Size           { return r.s.MinSize() }
func (r *seccionAtajosRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *seccionAtajosRenderer) Destroy()                     {}
func (r *seccionAtajosRenderer) Layout(t fyne.Size) {
	r.rotulo.Colocar(0, 0)
	y := tema.TextoXS.Linea + bajoRotuloAtajos
	anchoTexto := r.s.anchoTexto(t.Width)
	for i, a := range r.s.g.Atajos {
		if i > 0 {
			r.filetes[i-1].Move(fyne.NewPos(0, y))
			r.filetes[i-1].Resize(fyne.NewSize(t.Width, tema.Borde))
			y += tema.Borde
		}
		alto := r.s.altoFilaAtajo(a, anchoTexto)
		// Las dos columnas van centradas en la fila (`items-center`): una combinación de
		// dos líneas junto a una descripción de una, y al revés.
		teclas := r.teclas[i]
		anchos := make([]float32, len(teclas))
		for j, k := range teclas {
			anchos[j] = k.MinSize().Width
		}
		lineas := lineasTeclas(anchos, r.s.columna)
		yt := y + (alto-altoTeclas(len(lineas)))/2
		for _, linea := range lineas {
			x := float32(0)
			for n, j := range linea {
				if n > 0 {
					x += huecoTeclas
				}
				m := teclas[j].MinSize()
				teclas[j].Move(fyne.NewPos(x, yt+(LadoTecla-m.Height)/2))
				teclas[j].Resize(m)
				x += m.Width
			}
			yt += LadoTecla + huecoTeclas
		}
		texto := r.textos[i]
		m := MedidaParrafo(texto, anchoTexto)
		texto.Move(fyne.NewPos(r.s.columna+huecoFilaAtajo, y+(alto-m.Height)/2))
		texto.Resize(m)
		y += alto
	}
}

func (r *seccionAtajosRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r.rotulo.Pintar(tema.Color("text-subtle", v))
	for _, f := range r.filetes {
		f.FillColor = veloAtajos(tema.Color("border", v))
	}
	for _, x := range r.textos {
		x.Tinta = tema.Color("text", v)
	}
	for _, o := range r.objetos {
		o.Refresh()
	}
}

// veloAtajos es el `border/60` de los filetes: el color con su opacidad al 60 %.
func veloAtajos(c color.NRGBA) color.NRGBA {
	c.A = uint8(float32(c.A) * 0.6)
	return c
}

// # La hoja de la SUITE (`HojaDeAtajos.tsx` de @sytue/shell)
//
// «?» en todas las apps: los atajos que declara la app y, al final, el grupo de la
// suite —⌘K, «?» y Esc—, que es de todas y antes no lo enseñaba ninguna. Los textos
// salen del catálogo COMÚN, como el resto de la barra de la suite, en el idioma de la
// app (el de la barra, si aún no hay ninguno).

// comunEn es el catálogo común en ese idioma; vacío, el de la barra.
func comunEn(l i18n.Locale) func(string) string {
	if l != "" {
		if t, err := i18n.New(l, i18n.Common()); err == nil {
			return func(clave string) string { return t.T(clave, nil) }
		}
	}
	return textoComun
}

// GrupoDeAtajosDeLaSuite es el grupo «Toda la suite» de la hoja, en el idioma `l`.
func GrupoDeAtajosDeLaSuite(l i18n.Locale) GrupoDeAtajos {
	tc := comunEn(l)
	return GrupoDeAtajos{
		Titulo: tc("shell.shortcuts.group"),
		Atajos: []AtajoDeTeclado{
			{Teclas: []string{AtajoDelSistema("Ctrl+K")}, Descripcion: tc("shell.shortcuts.search")},
			{Teclas: []string{"?"}, Descripcion: tc("shell.shortcuts.help")},
			{Teclas: []string{"Esc"}, Descripcion: tc("shell.shortcuts.close")},
		},
	}
}

// ComponerHojaDeSuite pone el grupo de la suite detrás de los de la app. La fila de «?»
// que traen las apps (cuando la hoja era suya, la contaban) se quita: ahora es de la
// suite y saldría dos veces. Un grupo que se queda vacío no se pinta.
func ComponerHojaDeSuite(l i18n.Locale, app []GrupoDeAtajos) []GrupoDeAtajos {
	hoja := make([]GrupoDeAtajos, 0, len(app)+1)
	for _, g := range app {
		propios := GrupoDeAtajos{Titulo: g.Titulo}
		for _, a := range g.Atajos {
			if len(a.Teclas) == 1 && a.Teclas[0] == "?" {
				continue
			}
			propios.Atajos = append(propios.Atajos, a)
		}
		if len(propios.Atajos) > 0 {
			hoja = append(hoja, propios)
		}
	}
	return append(hoja, GrupoDeAtajosDeLaSuite(l))
}

// MostrarHojaAtajosDeSuite abre la hoja de «?» con los grupos de la app y el de la
// suite, titulada y cerrada con el catálogo común en el idioma `l` de la app: es la
// misma hoja en todas las apps.
func MostrarHojaAtajosDeSuite(lienzo fyne.Canvas, l i18n.Locale, app []GrupoDeAtajos) *Dialogo {
	tc := comunEn(l)
	return MostrarHojaAtajos(lienzo, tc("shell.shortcuts.title"), tc("ui.close"), ComponerHojaDeSuite(l, app))
}
