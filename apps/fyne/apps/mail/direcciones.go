// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package mail

import (
	"regexp"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// # El campo de direcciones (`AddressField.tsx`, en su variante `desnudo`)
//
// Lo escrito es UNA cadena —«Ana <ana@x.eu>, luis@y.eu, ma»— y el campo la enseña en
// FICHAS: todo lo que va antes de la última coma es una ficha cerrada y lo último es lo
// que se está escribiendo. Una coma, un punto y coma o Intro cierran la ficha; borrar
// con el campo vacío abre la última para corregirla. Una ficha sin forma de correo se
// pinta en rojo antes de enviar, que es cuando todavía se puede arreglar.
//
// De la hoja: la envoltura es `flex min-h-9 flex-wrap items-center gap-1`; cada ficha,
// `rounded-pill h-7 max-w-[18rem] border pl-2.5 pr-1 gap-1 text-sm` con su aspa de
// `h-5 w-5` y el glifo de 12; el campo, `h-7 min-w-[10rem] flex-1` sin caja.
//
// El autocompletado de Sytue Contacts está en `sugerencias.go`.
const (
	altoFicha      float32 = 7 * 0.25 * tema.Rem
	margenIzqFicha float32 = 2.5 * 0.25 * tema.Rem
	margenDerFicha float32 = 1 * 0.25 * tema.Rem
	huecoFicha     float32 = 1 * 0.25 * tema.Rem
	aspaFicha      float32 = 5 * 0.25 * tema.Rem
	glifoAspaFicha float32 = 12
	maxFicha       float32 = 18 * tema.Rem
	minCampoFicha  float32 = 10 * tema.Rem
	minAltoCampos  float32 = 9 * 0.25 * tema.Rem
)

var formaDeCorreo = regexp.MustCompile(`^[^\s@<>]+@[^\s@<>]+\.[^\s@<>]+$`)

// PartirDirecciones es `partir`: las fichas cerradas y lo que se está escribiendo.
func PartirDirecciones(valor string) (fichas []string, actual string) {
	partes := strings.FieldsFunc(valor+"\x00", func(r rune) bool { return r == ',' || r == ';' })
	ultimo := strings.TrimSuffix(partes[len(partes)-1], "\x00")
	for _, p := range partes[:len(partes)-1] {
		if p = strings.TrimSpace(p); p != "" {
			fichas = append(fichas, p)
		}
	}
	return fichas, strings.TrimLeft(ultimo, " \t")
}

// UnirDireccionesCampo es `unir`.
func UnirDireccionesCampo(fichas []string, actual string) string {
	if len(fichas) == 0 {
		return actual
	}
	return strings.Join(fichas, ", ") + ", " + actual
}

// leerFicha es `leer`: el nombre y el correo de «Nombre <correo>».
func leerFicha(f string) (nombre, correo string) {
	if i, j := strings.LastIndexByte(f, '<'), strings.LastIndexByte(f, '>'); i >= 0 && j > i {
		return strings.Trim(strings.TrimSpace(f[:i]), `"`), strings.TrimSpace(f[i+1 : j])
	}
	return "", strings.TrimSpace(f)
}

// CampoDirecciones es el campo de Para, CC y CCO.
type CampoDirecciones struct {
	widget.BaseWidget
	// Valor es lo escrito, fichas incluidas.
	Valor string
	// RotuloQuitar escribe el nombre accesible del aspa de una ficha, y AvisoNoValida
	// es el `title` de una ficha que no tiene forma de correo. Ya traducidos.
	RotuloQuitar  func(nombre string) string
	AvisoNoValida string
	AlCambiar     func(string)
	AlEnfocar     func(bool)
	// Buscar pide sugerencias para lo que se está escribiendo, o "" para ninguna. Lo
	// pone el buzón (`sugeridor`), que espera y descarta las respuestas viejas.
	Buscar func(q string)

	editor   *ui.EditorPlano
	marca    string
	fichas   []*fichaDireccion
	enfocado bool

	// AlSugerir recibe el panel de sugerencias para colocarlo, o nil para quitarlo.
	AlSugerir func(panel fyne.CanvasObject)

	sugerencias []Sugerencia
	activa      int
	panel       fyne.CanvasObject
	filasSug    []*filaSugerencia
}

var _ fyne.Widget = (*CampoDirecciones)(nil)

// NuevoCampoDirecciones crea el campo con su marca de agua, que solo se ve sin fichas.
func NuevoCampoDirecciones(marca string) *CampoDirecciones {
	c := &CampoDirecciones{marca: marca, activa: -1}
	// El campo ocupa solo lo que dejan el rótulo y CC/CCO: incluso su placeholder
	// debe quedar recortado dentro, como el input de una línea de la web.
	c.editor = ui.NuevoEditorPlanoDesplazable(tema.TextoBase, func(f bool) {
		c.enfocado = f
		if !f {
			c.cerrarSugerencias()
			c.cerrarActual() // `onBlur`: lo escrito se cierra como ficha
		}
		c.pedirSugerencias()
		if c.AlEnfocar != nil {
			c.AlEnfocar(f)
		}
	})
	c.editor.PlaceHolder = marca
	c.editor.AlCambiar(c.teclear)
	c.editor.Tecla = c.tecla
	c.ExtendBaseWidget(c)
	return c
}

// Foco es lo que hay que enfocar para escribir aquí.
func (c *CampoDirecciones) Foco() fyne.Focusable { return c.editor }

// Poner fija el valor desde fuera (abrir un borrador, elegir de la libreta).
func (c *CampoDirecciones) Poner(v string) {
	c.Valor = v
	_, actual := PartirDirecciones(v)
	c.editor.OnChanged = nil
	c.editor.PonerTexto(actual)
	c.editor.AlCambiar(c.teclear)
	c.Refresh()
}

func (c *CampoDirecciones) cambiar(v string) {
	c.Valor = v
	if c.AlCambiar != nil {
		c.AlCambiar(v)
	}
	c.Refresh()
}

// teclear recibe el texto del campo: si lleva un separador, lo de antes se cierra en
// fichas (que es lo que la web hace al pulsar la coma).
func (c *CampoDirecciones) teclear(texto string) {
	fichas, _ := PartirDirecciones(c.Valor)
	if strings.ContainsAny(texto, ",;") {
		nuevas, actual := PartirDirecciones(texto)
		fichas = append(fichas, nuevas...)
		c.editor.OnChanged = nil
		c.editor.PonerTexto(actual)
		c.editor.AlCambiar(c.teclear)
		texto = actual
	}
	c.cambiar(UnirDireccionesCampo(fichas, texto))
	c.pedirSugerencias()
}

// tecla es Intro, que cierra la ficha, y el borrado con el campo vacío, que abre la
// última.
func (c *CampoDirecciones) tecla(k *fyne.KeyEvent) bool {
	if c.teclaSugerencias(k) {
		return true
	}
	fichas, actual := PartirDirecciones(c.Valor)
	switch k.Name {
	case fyne.KeyReturn, fyne.KeyEnter:
		if strings.TrimSpace(actual) == "" {
			return false
		}
		c.cerrarActual()
		return true
	case fyne.KeyBackspace:
		if actual != "" || len(fichas) == 0 {
			return false
		}
		ultima := fichas[len(fichas)-1]
		c.editor.OnChanged = nil
		c.editor.PonerTexto(ultima)
		c.editor.AlCambiar(c.teclear)
		c.cambiar(UnirDireccionesCampo(fichas[:len(fichas)-1], ultima))
		return true
	default:
		return false
	}
}

// cerrarActual es `fijar`: lo escrito pasa a ser una ficha.
func (c *CampoDirecciones) cerrarActual() {
	fichas, actual := PartirDirecciones(c.Valor)
	if strings.TrimSpace(actual) == "" {
		return
	}
	c.editor.OnChanged = nil
	c.editor.PonerTexto("")
	c.editor.AlCambiar(c.teclear)
	c.cambiar(UnirDireccionesCampo(append(fichas, strings.TrimSpace(actual)), ""))
}

func (c *CampoDirecciones) quitar(i int) {
	fichas, actual := PartirDirecciones(c.Valor)
	if i < 0 || i >= len(fichas) {
		return
	}
	c.cambiar(UnirDireccionesCampo(append(fichas[:i:i], fichas[i+1:]...), actual))
}

// colocar reparte fichas y campo con `flex-wrap gap-1`: el campo pide al menos sus
// 10rem y se queda con el resto de su línea. Devuelve el alto, con el `min-h-9`.
func (c *CampoDirecciones) colocar(ancho float32, poner bool) float32 {
	x, y := float32(0), float32(0)
	for _, f := range c.fichas {
		w := min(f.MinSize().Width, maxFicha, ancho)
		if x > 0 && x+w > ancho {
			x, y = 0, y+altoFicha+huecoFicha
		}
		if poner {
			f.Move(fyne.NewPos(x, y))
			f.Resize(fyne.NewSize(w, altoFicha))
		}
		x += w + huecoFicha
	}
	if x > 0 && x+minCampoFicha > ancho {
		x, y = 0, y+altoFicha+huecoFicha
	}
	alto := y + altoFicha
	total := max(alto, minAltoCampos)
	if poner {
		// Centrado como bloque en el `min-h-9` (`items-center`).
		dy := (total - alto) / 2
		for _, f := range c.fichas {
			f.Move(f.Position().AddXY(0, dy))
		}
		fa := c.editor.AltoFuente()
		c.editor.Colocar(fyne.NewPos(x, dy+y+(altoFicha-fa)/2), fyne.NewSize(max(ancho-x, 0), fa))
	}
	return total
}

// AltoCampoDirecciones es lo que mide a ese ancho.
func (c *CampoDirecciones) AltoCampoDirecciones(ancho float32) float32 {
	return c.colocar(ancho, false)
}

func (c *CampoDirecciones) CreateRenderer() fyne.WidgetRenderer {
	r := &campoDireccionesRenderer{c: c}
	r.Refresh()
	return r
}

type campoDireccionesRenderer struct {
	c       *CampoDirecciones
	objetos []fyne.CanvasObject
	ancho   float32
}

func (r *campoDireccionesRenderer) MinSize() fyne.Size {
	return fyne.NewSize(minCampoFicha, r.c.colocar(max(r.ancho, minCampoFicha), false))
}
func (r *campoDireccionesRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *campoDireccionesRenderer) Destroy()                     {}
func (r *campoDireccionesRenderer) Layout(t fyne.Size) {
	r.ancho = t.Width
	r.c.colocar(t.Width, true)
}

func (r *campoDireccionesRenderer) Refresh() {
	c := r.c
	fichas, _ := PartirDirecciones(c.Valor)
	if len(fichas) != len(c.fichas) {
		c.fichas = c.fichas[:0]
		for i := range fichas {
			f := nuevaFichaDireccion(c, i)
			c.fichas = append(c.fichas, f)
		}
	}
	r.objetos = r.objetos[:0]
	for i, f := range c.fichas {
		f.Texto = fichas[i]
		f.Refresh()
		r.objetos = append(r.objetos, f)
	}
	r.objetos = append(r.objetos, c.editor.Objeto())
	// La marca de agua solo se ve sin fichas: con una ya puesta, «nombre@dominio.eu»
	// detrás de ella se lee como una segunda dirección.
	c.editor.PlaceHolder = c.marca
	if len(fichas) > 0 {
		c.editor.PlaceHolder = ""
	}
	c.editor.Refresh()
	if t := c.Size(); t.Width > 0 {
		r.Layout(t)
	}
}

// fichaDireccion es una dirección cerrada.
type fichaDireccion struct {
	widget.BaseWidget
	Texto  string
	campo  *CampoDirecciones
	indice int
	aspa   *ui.BotonIcono
}

func nuevaFichaDireccion(c *CampoDirecciones, i int) *fichaDireccion {
	f := &fichaDireccion{campo: c, indice: i}
	f.aspa = ui.NuevoBotonIcono(iconos.Cerrar, "", func() { c.quitar(f.indice) })
	// `hover:bg-border`, redonda entera, y la tinta de la ficha.
	f.aspa.Lado, f.aspa.TamIcono, f.aspa.Radio = aspaFicha, glifoAspaFicha, tema.Forma("radius-control")
	f.aspa.Trazo, f.aspa.TrazoEncima, f.aspa.FondoEncima = "text", "", "border"
	f.ExtendBaseWidget(f)
	return f
}

func (f *fichaDireccion) valida() bool {
	_, correo := leerFicha(f.Texto)
	return formaDeCorreo.MatchString(correo)
}

func (f *fichaDireccion) rotulo() string {
	nombre, correo := leerFicha(f.Texto)
	if nombre != "" {
		return nombre
	}
	return correo
}

// MinSize es el texto con sus márgenes y el aspa, sin el tope (lo pone quien reparte).
func (f *fichaDireccion) MinSize() fyne.Size {
	return fyne.NewSize(2*tema.Borde+margenIzqFicha+ui.Medir(f.rotulo(), tema.TextoSM, ui.Normal).Width+
		huecoFicha+aspaFicha+margenDerFicha, altoFicha)
}

func (f *fichaDireccion) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &fichaRenderer{
		f:     f,
		caja:  ui.NuevaCaja(tema.Color("bg-subtle", v), tema.Color("border", v), tema.RadioPill),
		texto: ui.Texto("", tema.TextoSM, ui.Normal, tema.Color("text", v)),
	}
	r.objetos = append(r.caja.Objetos(), r.texto, f.aspa)
	r.Refresh()
	return r
}

type fichaRenderer struct {
	f       *fichaDireccion
	caja    *ui.Caja
	texto   *canvas.Text
	objetos []fyne.CanvasObject
}

func (r *fichaRenderer) MinSize() fyne.Size           { return r.f.MinSize() }
func (r *fichaRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *fichaRenderer) Destroy()                     {}
func (r *fichaRenderer) Layout(t fyne.Size) {
	r.caja.Redimensionar(t)
	x := tema.Borde + margenIzqFicha
	fin := t.Width - tema.Borde - margenDerFicha - aspaFicha
	ui.Truncar(r.texto, r.f.rotulo(), tema.TextoSM, ui.Normal, max(fin-huecoFicha-x, 0))
	ui.ColocarTexto(r.texto, tema.TextoSM, x, (t.Height-tema.TextoSM.Linea)/2)
	r.f.aspa.Move(fyne.NewPos(fin, (t.Height-aspaFicha)/2))
	r.f.aspa.Resize(fyne.NewSize(aspaFicha, aspaFicha))
}

func (r *fichaRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	relleno, borde, tinta := tema.Color("bg-subtle", v), tema.Color("border", v), "text"
	if !r.f.valida() {
		relleno, borde, tinta = tema.Color("danger-bg", v), tema.Color("danger", v), "danger-text"
	}
	r.caja.Pintar(relleno, borde)
	r.texto.Color = tema.Color(tinta, v)
	r.f.aspa.Trazo = tinta
	if c := r.f.campo; c.RotuloQuitar != nil {
		r.f.aspa.Rotulo = c.RotuloQuitar(r.f.rotulo())
	}
	for _, o := range r.objetos {
		o.Refresh()
	}
	if t := r.f.Size(); t.Width > 0 {
		r.Layout(t)
	}
}
