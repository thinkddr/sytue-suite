// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package erp

import (
	"context"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// # Elegir el registro al que apunta una relación (`SelectorRelacion.tsx`)
//
// Una relación se ELIGE, no se teclea: el valor es un identificador y nadie lo acierta
// escribiendo. El control es un botón con el nombre del elegido y su «×»; al pulsarlo
// sale debajo un panel con un buscador y hasta ocho coincidencias, que se piden al
// modelo apuntado con la MISMA consulta que sirve las listas.
//
// Medido en el arnés (2026-09-23): el control 349×31,75 (`px-3 py-2 text-sm`, relleno
// `bg`), el «×» 14,08 de ancho con `px-1`; el panel a 4 px debajo (`mt-1`), `rounded-lg
// border shadow-lg`, su buscador de 30,75 con filete debajo, la lista con `py-1` y cada
// opción `px-3 py-2 text-sm` (29,75).

// Medidas del selector de relaciones.
const (
	// PanelRelacionMT es el `mt-1` del panel bajo el control.
	PanelRelacionMT float32 = 1 * 0.25 * tema.Rem
	// OpcionRelacionPX y OpcionRelacionPY son el `px-3 py-2` de una opción.
	OpcionRelacionPX float32 = 3 * 0.25 * tema.Rem
	OpcionRelacionPY float32 = 2 * 0.25 * tema.Rem
	// ListaRelacionPY es el `py-1` de la lista.
	ListaRelacionPY float32 = 1 * 0.25 * tema.Rem
	// QuitarPX es el `px-1` del «×».
	QuitarPX float32 = 1 * 0.25 * tema.Rem
	// MaxOpcionesRelacion es el `limite: 8` de la consulta.
	MaxOpcionesRelacion = 8
	// EsperaRelacion es lo que se espera a que se deje de teclear.
	EsperaRelacion = 180 * time.Millisecond
)

// AltoOpcionRelacion es una opción: `py-2` sobre la caja de `text-sm`.
var AltoOpcionRelacion = tema.TextoSM.Linea + 2*OpcionRelacionPY

// AltoBuscadorRelacion es el buscador del panel: `py-2` sobre `text-sm` y el filete.
var AltoBuscadorRelacion = tema.TextoSM.Linea + 2*OpcionRelacionPY - tema.Borde

// SelectorRelacion es el control de un campo de relación.
type SelectorRelacion struct {
	ui.Pulsable
	tr *i18n.Translator

	// Modelo es el apuntado; Valor el id elegido y Etiqueta su nombre legible.
	Modelo, Valor, Etiqueta string
	// Buscar pide las coincidencias al servicio. nil: el panel sale vacío.
	Buscar func(ctx context.Context, modelo, q string) ([]Registro, error)
	// AlCambiar recibe el id y el nombre del elegido; los dos vacíos al quitar.
	AlCambiar func(valor, etiqueta string)

	mu       sync.Mutex
	abierto  *ui.Flotante
	opciones []Registro
	cursor   int
	cargando bool
	buscador *ui.EditorPlano
	reloj    *time.Timer
	panel    *panelRelacion
}

// NuevoSelectorRelacion crea el control.
func NuevoSelectorRelacion(tr *i18n.Translator, modelo, valor, etiqueta string, alCambiar func(string, string)) *SelectorRelacion {
	s := &SelectorRelacion{tr: tr, Modelo: modelo, Valor: valor, Etiqueta: etiqueta, AlCambiar: alCambiar}
	s.Nombre = etiqueta
	s.AlPulsar = s.Alternar
	s.ExtendBaseWidget(s)
	return s
}

// rotulo es lo que enseña el botón: el nombre, los ocho primeros del id si no se sabe,
// o «Elegir…».
func (s *SelectorRelacion) rotulo() (string, bool) {
	switch {
	case s.Etiqueta != "":
		return s.Etiqueta, true
	case len(s.Valor) > 8:
		return s.Valor[:8], true
	case s.Valor != "":
		return s.Valor, true
	}
	return s.tr.T("relation.choose", nil), false
}

// MinSize es el campo nativo de la ficha.
func (s *SelectorRelacion) MinSize() fyne.Size {
	s.ExtendBaseWidget(s)
	return fyne.NewSize(0, CampoFicha.Alto())
}

// Tapped quita con el «×» y, en el resto, abre o cierra el panel.
func (s *SelectorRelacion) Tapped(e *fyne.PointEvent) {
	if s.Inerte {
		return
	}
	if s.Valor != "" && e.Position.X >= s.Size().Width-tema.Borde-OpcionRelacionPX-anchoQuitar() {
		s.quitar()
		return
	}
	s.Alternar()
}

func anchoQuitar() float32 { return ui.Medir("×", tema.TextoSM, ui.Normal).Width + 2*QuitarPX }

func (s *SelectorRelacion) quitar() {
	s.Valor, s.Etiqueta = "", ""
	s.Refresh()
	if s.AlCambiar != nil {
		s.AlCambiar("", "")
	}
}

// Alternar abre el panel si está cerrado y lo cierra si está abierto.
func (s *SelectorRelacion) Alternar() {
	if s.Inerte {
		return
	}
	if s.abierto != nil {
		s.cerrar()
		return
	}
	c := fyne.CurrentApp().Driver().CanvasForObject(s)
	if c == nil {
		return
	}
	s.buscador = ui.NuevoEditorPlano(tema.TextoSM, nil)
	s.buscador.PlaceHolder = s.tr.T("relation.search", nil)
	s.buscador.AlCambiar(s.buscarEsperando)
	s.buscador.Tecla = s.teclear
	s.panel = &panelRelacion{s: s}
	s.panel.ExtendBaseWidget(s.panel)
	// `ui.Flotante` y no `widget.NewPopUp`: la sombra del PopUp de Fyne es la de su tema,
	// que la suite apaga, y cambia con el DPI. Esta es la `shadow-md` del panel.
	s.abierto = ui.NuevoFlotante(s.panel, c)
	s.abierto.AlCerrar(s.cerrar)
	s.abierto.Resize(fyne.NewSize(s.Size().Width, s.panel.MinSize().Height))
	pos := fyne.CurrentApp().Driver().AbsolutePositionForObject(s)
	s.abierto.ShowAtPosition(fyne.NewPos(pos.X, pos.Y+s.Size().Height+PanelRelacionMT))
	c.Focus(s.buscador)
	s.pedir("")
}

func (s *SelectorRelacion) cerrar() {
	if s.abierto != nil {
		s.abierto.Hide()
		s.abierto = nil
	}
	s.mu.Lock()
	if s.reloj != nil {
		s.reloj.Stop()
	}
	s.mu.Unlock()
}

// teclear son las flechas, Intro y Escape del buscador.
func (s *SelectorRelacion) teclear(k *fyne.KeyEvent) bool {
	switch k.Name {
	case fyne.KeyDown:
		s.cursor = min(s.cursor+1, max(len(s.opciones)-1, 0))
	case fyne.KeyUp:
		s.cursor = max(s.cursor-1, 0)
	case fyne.KeyReturn, fyne.KeyEnter:
		if s.cursor < len(s.opciones) {
			s.elegir(s.opciones[s.cursor])
		}
		return true
	case fyne.KeyEscape:
		s.cerrar()
		return true
	default:
		return false
	}
	s.repintarPanel()
	return true
}

func (s *SelectorRelacion) elegir(r Registro) {
	s.Valor, s.Etiqueta = r.ID, NombreDe(r)
	s.cerrar()
	s.Refresh()
	if s.AlCambiar != nil {
		s.AlCambiar(s.Valor, s.Etiqueta)
	}
}

// buscarEsperando espera 180 ms a que se deje de teclear (nada con el campo vacío).
func (s *SelectorRelacion) buscarEsperando(q string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.reloj != nil {
		s.reloj.Stop()
	}
	espera := EsperaRelacion
	if q == "" {
		espera = 0
	}
	s.reloj = time.AfterFunc(espera, func() { fyne.Do(func() { s.pedir(q) }) })
}

func (s *SelectorRelacion) pedir(q string) {
	if s.Buscar == nil {
		s.opciones, s.cargando = nil, false
		s.repintarPanel()
		return
	}
	s.cargando = true
	s.repintarPanel()
	buscar, modelo := s.Buscar, s.Modelo
	go func() {
		ctx, cancelar := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancelar()
		rs, err := buscar(ctx, modelo, q)
		fyne.Do(func() {
			if s.buscador == nil || s.buscador.Texto() != q {
				return // llegó tarde: ya se ha escrito otra cosa
			}
			if err != nil {
				rs = nil
			}
			s.opciones, s.cursor, s.cargando = rs, 0, false
			s.repintarPanel()
		})
	}()
}

func (s *SelectorRelacion) repintarPanel() {
	if s.panel == nil || s.abierto == nil {
		return
	}
	s.panel.Refresh()
	s.abierto.Resize(fyne.NewSize(s.Size().Width, s.panel.MinSize().Height))
}

// CreateRenderer construye la caja del campo, el rótulo y el «×».
func (s *SelectorRelacion) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &relacionRenderer{
		s:      s,
		aro:    ui.NuevoAro(tema.RadioMD, ui.SinHueco, v),
		caja:   ui.NuevaCaja(tema.Color("bg", v), tema.Color("border", v), tema.RadioMD),
		rotulo: ui.Texto("", tema.TextoSM, ui.Normal, tema.Color("text", v)),
		equis:  ui.Texto("×", tema.TextoSM, ui.Normal, tema.Color("text-subtle", v)),
	}
	r.Refresh()
	return r
}

type relacionRenderer struct {
	s             *SelectorRelacion
	aro           *ui.Aro
	caja          *ui.Caja
	rotulo, equis *canvas.Text
}

func (r *relacionRenderer) MinSize() fyne.Size { return r.s.MinSize() }
func (r *relacionRenderer) Objects() []fyne.CanvasObject {
	objs := append(r.aro.Objetos(), r.caja.Objetos()...)
	return append(objs, r.rotulo, r.equis)
}
func (r *relacionRenderer) Destroy() {}

func (r *relacionRenderer) Layout(t fyne.Size) {
	r.aro.Colocar(t)
	r.caja.Redimensionar(t)
	x := tema.Borde + OpcionRelacionPX
	der := t.Width - tema.Borde - OpcionRelacionPX
	if !r.equis.Hidden {
		der -= anchoQuitar()
		ui.ColocarTexto(r.equis, tema.TextoSM, der+QuitarPX, (t.Height-tema.TextoSM.Linea)/2)
		der -= 2 * 0.25 * tema.Rem // el `gap-2`
	}
	texto, _ := r.s.rotulo()
	ui.Truncar(r.rotulo, texto, tema.TextoSM, ui.Normal, der-x)
	ui.ColocarTexto(r.rotulo, tema.TextoSM, x, (t.Height-tema.TextoSM.Linea)/2)
}

func (r *relacionRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	texto, elegido := r.s.rotulo()
	r.rotulo.Text = texto
	r.rotulo.Color = tema.Color("text-subtle", v)
	if elegido {
		r.rotulo.Color = tema.Color("text", v)
	}
	r.equis.Hidden = r.s.Valor == ""
	r.caja.Pintar(tema.Color("bg", v), tema.Color("border", v))
	r.aro.Ver(r.s.Enfocado())
	r.aro.Pintar(v)
	r.Layout(r.s.Size())
	canvas.Refresh(r.s)
}

// --- El panel -----------------------------------------------------------------

// panelRelacion es lo que sale debajo: el buscador y la lista.
type panelRelacion struct {
	widget.BaseWidget
	s *SelectorRelacion
}

func (p *panelRelacion) altoLista() float32 {
	s := p.s
	if (s.cargando && len(s.opciones) == 0) || len(s.opciones) == 0 {
		// La rueda de 16 con `py-4`, o «Nada coincide» con `px-3 py-3`.
		if s.cargando {
			return 2*4*0.25*tema.Rem + 16
		}
		return tema.TextoSM.Linea + 2*3*0.25*tema.Rem
	}
	return float32(len(s.opciones)) * AltoOpcionRelacion
}

// MinSize es el buscador, su filete y la lista con su `py-1`, dentro del borde. Medido
// sin coincidencias: 76,5.
func (p *panelRelacion) MinSize() fyne.Size {
	p.ExtendBaseWidget(p)
	return fyne.NewSize(0, 2*tema.Borde+AltoBuscadorRelacion+tema.Borde+2*ListaRelacionPY+p.altoLista())
}

func (p *panelRelacion) CreateRenderer() fyne.WidgetRenderer {
	r := &panelRelacionRenderer{p: p}
	r.rehacer()
	return r
}

type panelRelacionRenderer struct {
	p       *panelRelacion
	caja    *ui.Caja
	sombra  *ui.Sombra
	filete  *canvas.Rectangle
	lista   *fyne.Container
	objetos []fyne.CanvasObject
}

func (r *panelRelacionRenderer) rehacer() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	s := r.p.s
	r.caja = ui.NuevaCaja(tema.Color("menu-surface", v), tema.Color("border", v), radioTarjeta())
	r.sombra = ui.NuevaSombra(tema.SombraLG, radioTarjeta())
	r.filete = canvas.NewRectangle(tema.Color("border", v))
	var filas []fyne.CanvasObject
	switch {
	case s.cargando && len(s.opciones) == 0:
		filas = append(filas, centrado(ui.NuevaRueda(16, "text-muted", v), r.p.altoLista()))
	case len(s.opciones) == 0:
		t := ui.Texto(s.tr.T("relation.empty", nil), tema.TextoSM, ui.Normal, tema.Color("text-subtle", v))
		filas = append(filas, container.New(&textoEn1{t: t, x: OpcionRelacionPX, alto: r.p.altoLista()}, t))
	default:
		for i, o := range s.opciones {
			reg, idx := o, i
			f := NuevaFilaPaleta(Modelo{Etiqueta: NombreDe(reg)}, i == s.cursor, func() { s.elegir(reg) })
			f.AlEntrar = func() {
				if s.cursor != idx {
					s.cursor = idx
					s.repintarPanel()
				}
			}
			f.Margen = OpcionRelacionPX
			filas = append(filas, f)
		}
	}
	r.lista = container.New(&apilado{alto: AltoOpcionRelacion}, filas...)
	r.objetos = append([]fyne.CanvasObject{r.sombra.Objeto()}, r.caja.Objetos()...)
	r.objetos = append(r.objetos, s.buscador.Objeto(), r.filete, r.lista)
}

func (r *panelRelacionRenderer) MinSize() fyne.Size           { return r.p.MinSize() }
func (r *panelRelacionRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *panelRelacionRenderer) Destroy()                     {}

func (r *panelRelacionRenderer) Layout(t fyne.Size) {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r.sombra.Ajustar(fyne.NewPos(0, 0), t, v)
	r.caja.Redimensionar(t)
	dentro := t.Width - 2*tema.Borde
	r.p.s.buscador.Colocar(fyne.NewPos(tema.Borde+OpcionRelacionPX, tema.Borde),
		fyne.NewSize(dentro-2*OpcionRelacionPX, AltoBuscadorRelacion))
	y := tema.Borde + AltoBuscadorRelacion
	r.filete.Move(fyne.NewPos(tema.Borde, y))
	r.filete.Resize(fyne.NewSize(dentro, tema.Borde))
	y += tema.Borde + ListaRelacionPY
	r.lista.Move(fyne.NewPos(tema.Borde, y))
	r.lista.Resize(fyne.NewSize(dentro, r.p.altoLista()))
}

func (r *panelRelacionRenderer) Refresh() {
	r.rehacer()
	r.Layout(r.p.Size())
	canvas.Refresh(r.p)
}

// apilado pone sus hijos uno debajo de otro con el mismo alto.
type apilado struct{ alto float32 }

func (a *apilado) MinSize(objs []fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(0, float32(len(objs))*a.alto)
}

func (a *apilado) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	for i, o := range objs {
		o.Move(fyne.NewPos(0, float32(i)*a.alto))
		o.Resize(fyne.NewSize(t.Width, a.alto))
	}
}

// textoEn1 es un texto suelto en una caja de alto dado, centrado en vertical.
type textoEn1 struct {
	t    *canvas.Text
	x    float32
	alto float32
}

func (c *textoEn1) MinSize([]fyne.CanvasObject) fyne.Size { return fyne.NewSize(0, c.alto) }

func (c *textoEn1) Layout(_ []fyne.CanvasObject, t fyne.Size) {
	e := escalonDe(c.t)
	ui.ColocarTexto(c.t, e, c.x, (t.Height-e.Linea)/2)
}
