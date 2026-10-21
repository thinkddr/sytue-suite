// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package drive

import (
	"image/color"
	"net/http"
	"slices"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"golang.org/x/text/collate"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// # «Mover a…»
//
// `MoveDialog` de FileBrowser.tsx: un selector de carpeta destino que navega el árbol
// —solo carpetas— con su ruta encima para subir. La carpeta que se mueve no sale en la
// lista (no puede ser su propio destino) y «Mover aquí» se apaga en la carpeta donde ya
// está, que sería no hacer nada.
//
// Medidas de la hoja: la ruta `mb-2 text-base` con los segmentos de `px-2 py-1`; la caja
// de la lista `h-56` (196) con `border rounded-md`; cada carpeta `border-b px-3 py-2
// gap-2`, con el glifo de 16 en `primary-text` y el galón de 14 en `text-subtle`; y la
// línea de debajo `mt-2 text-sm text-subtle`.

// Medidas del selector.
const (
	// AltoCajaMover es el `h-56` de la lista: fija, para que navegar no haga saltar el
	// diálogo.
	AltoCajaMover float32 = 56 * 0.25 * tema.Rem
	// SeparaMover es el `mb-2` de la ruta y el `mt-2` de la línea de destino.
	SeparaMover float32 = 2 * 0.25 * tema.Rem
	// LadoGlifoCarpetaMover es el `size={16}` de la carpeta y LadoGalonMover el 14 del
	// galón.
	LadoGlifoCarpetaMover float32 = 16
	LadoGalonMover        float32 = 14
)

// AltoFilaMover es lo que ocupa una carpeta de la lista: `py-2` a cada lado de la línea
// de `text-base` más el `border-b`.
var AltoFilaMover = 2*2*0.25*tema.Rem + tema.TextoBase.Linea + tema.Borde

// selectorCarpeta es el estado del diálogo. Se pinta entero en cada cambio, como la web.
type selectorCarpeta struct {
	p  *Pantalla
	el Elemento
	// lote es la selección, en «Mover la selección a…» (`BatchMoveDialog`); nil es
	// mover un elemento. En lote se excluyen como destino las carpetas marcadas —no se
	// mueven dentro de sí mismas— y «aquí» es la carpeta que se está mirando.
	lote []Elemento
	// aqui es la ruta de la carpeta que se estaba mirando al abrir el selector.
	aqui string
	// ruta son las carpetas desde la raíz hasta el destino, sin la raíz, que pinta la
	// primera miga. La raíz es la de verdad: `raiz` es su id.
	ruta []Elemento
	raiz string
	// carpetas son las subcarpetas del destino; nil mientras carga.
	carpetas []Elemento
	fallo    bool
	moviendo bool

	cuerpo    *fyne.Container
	confirmar *ui.Boton
	d         *ui.Dialogo
}

// destino es el id de la carpeta elegida; vacío mientras no se sepa la raíz.
func (s *selectorCarpeta) destino() string {
	if n := len(s.ruta); n > 0 {
		return s.ruta[n-1].ID
	}
	return s.raiz
}

// rutaDestino es el camino de la carpeta elegida, como lo escribe la web: `/` es la raíz.
func (s *selectorCarpeta) rutaDestino() string {
	var b strings.Builder
	for _, c := range s.ruta {
		b.WriteByte('/')
		b.WriteString(c.Nombre)
	}
	if b.Len() == 0 {
		return "/"
	}
	return b.String()
}

// EsMismaCarpeta dice si mover `origen` a `destino` no cambiaría nada: los dos son
// caminos de Drive y el padre de `origen` es `destino`. La raíz se escribe `/`.
func EsMismaCarpeta(origen, destino string) bool {
	padre := "/"
	if i := strings.LastIndexByte(origen, '/'); i > 0 {
		padre = origen[:i]
	}
	return padre == destino
}

// pedirMover abre el selector sobre la raíz.
func (p *Pantalla) pedirMover(el Elemento) {
	p.abrirSelector(&selectorCarpeta{p: p, el: el}, p.tr.T("move.title", i18n.Vars{"name": el.Nombre}))
}

// pedirMoverLote es «Mover» de la barra de selección.
func (p *Pantalla) pedirMoverLote() {
	sel := p.seleccionados()
	if len(sel) == 0 {
		return
	}
	s := &selectorCarpeta{p: p, lote: sel, aqui: p.rutaAqui()}
	p.abrirSelector(s, p.tr.T("files.batch.moveTitle", i18n.Vars{"count": len(sel)}))
}

// rutaAqui es el camino de la carpeta que se está mirando, `/` en la raíz.
func (p *Pantalla) rutaAqui() string {
	var b strings.Builder
	for _, c := range p.explor.Ruta {
		b.WriteByte('/')
		b.WriteString(c.Nombre)
	}
	if b.Len() == 0 {
		return "/"
	}
	return b.String()
}

func (p *Pantalla) abrirSelector(s *selectorCarpeta, titulo string) {
	if p.win == nil || p.svc == nil {
		return
	}
	s.cuerpo = container.NewStack()
	cancelar := ui.NuevoBoton(p.tr.T("files.cancel", nil), func() {
		if !s.moviendo {
			s.d.Cerrar()
		}
	}).Con(ui.Fantasma, ui.TallaMD)
	s.confirmar = ui.NuevoBoton(p.tr.T("move.here", nil), s.mover).Con(ui.Primario, ui.TallaMD)
	pie := container.New(ui.Fila{Hueco: ui.HuecoPieDialogo}, cancelar, s.confirmar)
	s.d = ui.NuevoDialogo(titulo, s.cuerpo).Con(ui.DialogoMD, pie)
	s.d.RotuloCerrar = p.tr.T("ui.close", nil)
	s.pintar()
	s.d.Mostrar(p.win.Canvas())
	go s.cargar()
}

// ir cambia de destino: nivel -1 es la raíz; si no, la carpeta i de la ruta, o una hija
// nueva si se pasa `hija`.
func (s *selectorCarpeta) ir(nivel int, hija *Elemento) {
	switch {
	case hija != nil:
		s.ruta = append(s.ruta, *hija)
	case nivel < 0:
		s.ruta = nil
	case nivel < len(s.ruta):
		s.ruta = s.ruta[:nivel+1]
	}
	s.carpetas, s.fallo = nil, false
	s.pintar()
	go s.cargar()
}

// cargar pide las subcarpetas del destino. Las carpetas llegan enteras en la primera
// página —solo los ficheros se paginan—, así que no hace falta seguir el cursor.
func (s *selectorCarpeta) cargar() {
	ctx, cancelar := s.p.ctx()
	defer cancelar()
	var destino string
	fyne.DoAndWait(func() { destino = s.destino() })
	if destino == "" {
		raiz, err := s.p.svc.Root(ctx)
		if err != nil {
			enHilo(func() { s.fallo = true; s.pintar() })
			return
		}
		destino = raiz.ID
		enHilo(func() { s.raiz = raiz.ID })
	}
	hijos, err := s.p.svc.Children(ctx, destino, "")
	carpetas := make([]Elemento, 0, len(hijos.Folders))
	for _, f := range hijos.Folders {
		if !s.excluida(f.ID) {
			carpetas = append(carpetas, DeCarpeta(f))
		}
	}
	OrdenarPorNombre(carpetas, s.p.tr.Locale())
	enHilo(func() {
		if s.destino() != destino {
			return // se navegó a otra mientras llegaba esta
		}
		s.carpetas, s.fallo = carpetas, err != nil
		s.pintar()
	})
}

// excluida dice si una carpeta no puede ser destino: la que se mueve, o una de las
// marcadas en un lote.
func (s *selectorCarpeta) excluida(id string) bool {
	if s.lote == nil {
		return id == s.el.ID
	}
	for _, el := range s.lote {
		if el.Carpeta && el.ID == id {
			return true
		}
	}
	return false
}

// OrdenarPorNombre ordena con el idioma de quien mira, no con uno fijo: la ñ, los
// acentos y la ö no se colocan igual en todas las lenguas (`localeCompare` de la web).
func OrdenarPorNombre(els []Elemento, l i18n.Locale) {
	c := collate.New(l.Tag())
	slices.SortStableFunc(els, func(a, b Elemento) int { return c.CompareString(a.Nombre, b.Nombre) })
}

// mover confirma. Un 409 es que en el destino ya hay algo con ese nombre, y se dice.
func (s *selectorCarpeta) mover() {
	if s.moviendo || s.noHaceNada() || s.carpetas == nil {
		return
	}
	s.moviendo = true
	s.confirmar.Cargando = true
	s.confirmar.Refresh()
	destino, el, p := s.destino(), s.el, s.p
	if s.lote != nil {
		s.moverLote(destino)
		return
	}
	go func() {
		ctx, cancelar := p.ctx()
		defer cancelar()
		err := p.svc.Move(ctx, claseDe(el), el.ID, destino)
		enHilo(func() {
			s.moviendo = false
			s.confirmar.Cargando = false
			s.confirmar.Refresh()
			if err == nil {
				s.d.Cerrar()
			}
		})
		if err != nil {
			clave := "move.failed"
			if codigo(err) == http.StatusConflict {
				clave = "move.conflict"
			}
			p.avisar(ui.AvisoPeligro, p.tr.T(clave, i18n.Vars{"name": el.Nombre}), "")
			return
		}
		p.avisar(ui.AvisoExito, p.tr.T("move.done", i18n.Vars{"name": el.Nombre}), "")
		p.recargarVista()
	}()
}

// moverLote mueve la selección en UNA operación del servicio.
func (s *selectorCarpeta) moverLote(destino string) {
	p := s.p
	ficheros, carpetas := partir(s.lote)
	n := len(s.lote)
	go func() {
		ctx, cancelar := p.ctx()
		defer cancelar()
		_, err := p.svc.BatchMove(ctx, ficheros, carpetas, destino)
		enHilo(func() {
			s.moviendo = false
			s.confirmar.Cargando = false
			s.confirmar.Refresh()
			if err == nil {
				s.d.Cerrar()
				clear(p.explor.Seleccion)
				p.ancla = -1
			}
		})
		if err != nil {
			clave := "files.batch.moveFailed"
			if codigo(err) == http.StatusConflict {
				clave = "files.batch.moveConflict"
			}
			p.avisar(ui.AvisoPeligro, p.tr.T(clave, nil), "")
			return
		}
		p.avisar(ui.AvisoExito, p.tr.T("files.batch.moveDone", i18n.Vars{"count": n}), "")
		p.recargarVista()
	}()
}

// noHaceNada es mover a la carpeta donde ya está (o, en lote, a la que se mira).
func (s *selectorCarpeta) noHaceNada() bool {
	if s.lote != nil {
		return s.rutaDestino() == s.aqui
	}
	return EsMismaCarpeta(s.p.rutaDe(s.el), s.rutaDestino())
}

// pintar rehace el cuerpo con el estado de ahora y apaga «Mover aquí» si toca.
func (s *selectorCarpeta) pintar() {
	v := s.p.variante()
	t := s.p.tr
	nombres := make([]string, 0, 1+len(s.ruta))
	nombres = append(nombres, t.T("nav.myDrive", nil))
	for _, c := range s.ruta {
		nombres = append(nombres, c.Nombre)
	}
	migas := ui.NuevasMigas(t.T("move.destinationNav", nil), nombres, func(i int) { s.ir(i-1, nil) })
	migas.Estilo = ui.MigasEnDialogo

	linea := t.T("move.noop", nil)
	if s.lote != nil {
		linea = t.T("files.batch.here", nil)
	}
	if !s.noHaceNada() {
		donde := s.rutaDestino()
		if donde == "/" {
			donde = t.T("nav.myDrive", nil)
		}
		linea = t.T("move.destination", i18n.Vars{"path": donde})
	}
	pie := ui.Texto(linea, tema.TextoSM, ui.Normal, tema.Color("text-subtle", v))

	caja := s.caja(v)
	s.cuerpo.Objects = []fyne.CanvasObject{container.New(&columnaMover{migas: migas, caja: caja, pie: pie}, migas, caja, pie)}
	s.cuerpo.Refresh()
	s.confirmar.Inerte = s.noHaceNada() || s.carpetas == nil
	s.confirmar.Refresh()
}

// caja es la lista con borde, o lo que haya en su lugar: la rueda, el fallo o «no hay
// subcarpetas».
func (s *selectorCarpeta) caja(v tema.Variante) fyne.CanvasObject {
	borde := ui.NuevaCaja(color.NRGBA{}, tema.Color("border", v), tema.RadioMD)
	var dentro fyne.CanvasObject
	t := s.p.tr
	switch {
	case s.fallo:
		reintentar := ui.NuevoBoton(t.T("files.retry", nil), func() { s.ir(len(s.ruta)-1, nil) }).Con(ui.Secundario, ui.TallaSM)
		texto := ui.Texto(t.T("move.loadFailed", nil), tema.TextoBase, ui.Normal, tema.Color("text-muted", v))
		dentro = container.NewCenter(container.New(ui.Columna{Hueco: SeparaMover, Encaja: true}, texto, reintentar))
	case s.carpetas == nil:
		r := ui.NuevaRueda(20, "text-muted", v)
		r.Rotulo = t.T("move.loadingFolders", nil)
		dentro = container.NewCenter(r)
	case len(s.carpetas) == 0:
		dentro = container.NewCenter(ui.Texto(t.T("move.noSubfolders", nil), tema.TextoBase, ui.Normal, tema.Color("text-muted", v)))
	default:
		filas := make([]fyne.CanvasObject, 0, len(s.carpetas))
		for _, c := range s.carpetas {
			carpeta := c
			filas = append(filas, nuevaFilaMover(carpeta.Nombre, func() { s.ir(0, &carpeta) }))
		}
		dentro = container.NewVScroll(container.New(ui.Columna{}, filas...))
	}
	return container.New(&dentroDeCaja{}, append(borde.Objetos(), dentro)...)
}

// dentroDeCaja pone el borde a tamaño completo y el contenido un píxel hacia dentro.
type dentroDeCaja struct{}

var _ fyne.Layout = (*dentroDeCaja)(nil)

func (*dentroDeCaja) MinSize([]fyne.CanvasObject) fyne.Size { return fyne.NewSize(0, AltoCajaMover) }

func (*dentroDeCaja) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	for i, o := range objs {
		if i == len(objs)-1 {
			o.Move(fyne.NewPos(tema.Borde, tema.Borde))
			o.Resize(fyne.NewSize(max(t.Width-2*tema.Borde, 0), max(t.Height-2*tema.Borde, 0)))
			continue
		}
		o.Move(fyne.NewPos(0, 0))
		o.Resize(t)
	}
}

// columnaMover apila la ruta, la caja y la línea de destino con sus `mb-2`/`mt-2`.
type columnaMover struct {
	migas *ui.Migas
	caja  fyne.CanvasObject
	pie   *canvas.Text
}

var _ fyne.Layout = (*columnaMover)(nil)

func (c *columnaMover) MinSize([]fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(0, ui.MigasEnDialogo.Alto()+SeparaMover+AltoCajaMover+SeparaMover+tema.TextoSM.Linea)
}

func (c *columnaMover) Layout(_ []fyne.CanvasObject, t fyne.Size) {
	c.migas.Move(fyne.NewPos(0, 0))
	c.migas.Resize(fyne.NewSize(min(c.migas.MinSize().Width, t.Width), ui.MigasEnDialogo.Alto()))
	y := ui.MigasEnDialogo.Alto() + SeparaMover
	c.caja.Move(fyne.NewPos(0, y))
	c.caja.Resize(fyne.NewSize(t.Width, AltoCajaMover))
	y += AltoCajaMover + SeparaMover
	ui.Truncar(c.pie, c.pie.Text, tema.TextoSM, ui.Normal, t.Width)
	ui.ColocarTexto(c.pie, tema.TextoSM, 0, y)
}

// filaMover es una carpeta de la lista del selector: se pulsa para entrar en ella.
type filaMover struct {
	ui.Pulsable
	nombre string
}

func nuevaFilaMover(nombre string, alPulsar func()) *filaMover {
	f := &filaMover{nombre: nombre}
	f.Nombre, f.AlPulsar = nombre, alPulsar
	f.ExtendBaseWidget(f)
	return f
}

// MinSize es el alto de la hoja; el ancho lo pone la caja.
func (f *filaMover) MinSize() fyne.Size {
	if ui.EsTactil() {
		return fyne.NewSize(0, max(AltoFilaMover, ui.SueloTactilBarra)) // `pointer-coarse:min-h-11`
	}
	return fyne.NewSize(0, AltoFilaMover)
}

// CreateRenderer: velo, glifo de carpeta, nombre, galón y filete.
func (f *filaMover) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &filaMoverRenderer{
		f:      f,
		velo:   canvas.NewRectangle(color.NRGBA{}),
		glifo:  iconos.Trazo(iconos.DriveCarpeta, LadoGlifoCarpetaMover, "primary-text", v),
		nombre: ui.Texto(f.nombre, tema.TextoBase, ui.Medio, tema.Color("text", v)),
		galon:  iconos.Trazo(iconos.ChevronDerecha, LadoGalonMover, "text-subtle", v),
		linea:  canvas.NewRectangle(conAlfaBorde(v)),
	}
	r.objetos = []fyne.CanvasObject{r.velo, r.glifo.Objeto(), r.nombre, r.galon.Objeto(), r.linea}
	r.Refresh()
	return r
}

type filaMoverRenderer struct {
	f            *filaMover
	velo, linea  *canvas.Rectangle
	glifo, galon *iconos.Dibujo
	nombre       *canvas.Text
	objetos      []fyne.CanvasObject
}

func (r *filaMoverRenderer) MinSize() fyne.Size           { return r.f.MinSize() }
func (r *filaMoverRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *filaMoverRenderer) Destroy()                     {}

func (r *filaMoverRenderer) Layout(t fyne.Size) {
	r.velo.Move(fyne.NewPos(0, 0))
	r.velo.Resize(t)
	contenido := t.Height - tema.Borde
	r.linea.Move(fyne.NewPos(0, contenido))
	r.linea.Resize(fyne.NewSize(t.Width, tema.Borde))
	margen, hueco := ui.MargenFila, float32(2*0.25*tema.Rem) // px-3, gap-2
	x := margen
	r.glifo.Colocar(x, (contenido-LadoGlifoCarpetaMover)/2, LadoGlifoCarpetaMover)
	x += LadoGlifoCarpetaMover + hueco
	derecha := t.Width - margen - LadoGalonMover
	r.galon.Colocar(derecha, (contenido-LadoGalonMover)/2, LadoGalonMover)
	ui.Truncar(r.nombre, r.f.nombre, tema.TextoBase, ui.Medio, max(derecha-hueco-x, 0))
	ui.ColocarTexto(r.nombre, tema.TextoBase, x, (contenido-tema.TextoBase.Linea)/2)
}

// Refresh es `hover:bg-bg-subtle` y `focus-visible:bg-bg-subtle`: el foco se ve igual
// que el puntero, sin anillo, como en la web.
func (r *filaMoverRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r.velo.FillColor = color.NRGBA{}
	if r.f.Encima() || r.f.Enfocado() {
		r.velo.FillColor = tema.Color("bg-subtle", v)
	}
	r.linea.FillColor = conAlfaBorde(v)
	r.glifo.Tenir("primary-text", v)
	r.galon.Tenir("text-subtle", v)
	r.nombre.Color = tema.Color("text", v)
	for _, o := range r.objetos {
		o.Refresh()
	}
}

// moverArrastrando es soltar lo arrastrado sobre una carpeta: una operación de lote con
// lo que de verdad cambia de sitio, y el aviso con cuántos se movieron.
func (p *Pantalla) moverArrastrando(origenes []Elemento, destino Elemento) {
	mover := Movibles(origenes, destino)
	if len(mover) == 0 || !p.ocupar() {
		return
	}
	defer p.soltar()
	ficheros, carpetas := partir(mover)
	ctx, cancelar := p.ctx()
	defer cancelar()
	lote, err := p.svc.BatchMove(ctx, ficheros, carpetas, destino.ID)
	if err != nil {
		// Se dice CUÁNTOS iban: «ha fallado» a secas deja sin saber si hay que repetir la
		// operación entera o solo lo que quedaba.
		p.avisar(ui.AvisoPeligro, p.tr.T("files.move.failed", i18n.Vars{"count": lote.Total()}), "")
	} else {
		p.avisar(ui.AvisoExito, p.tr.T("files.move.done", i18n.Vars{"count": lote.Total()}), "")
	}
	enHilo(func() { clear(p.explor.Seleccion); p.ancla = -1 })
	p.recargarVista()
}
