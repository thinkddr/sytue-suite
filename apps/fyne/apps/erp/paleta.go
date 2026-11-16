// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
// paridad-web: apps/frontend/packages/ui/src/components/PaletaDeComandos.tsx

package erp

import (
	"cmp"
	"image/color"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// # La paleta (⌘K)
//
// La tercera decisión de interfaz del ERP: sustituye a los menús anidados. No hay que
// saber en qué submenú vive «albaranes», basta con escribirlo.
//
// NO es `ui.Dialogo`, y no por capricho: es `PaletaDeComandos` de @sytue/ui, la misma
// capa que la de Sytue Cloud. Su cabecera es el CAMPO —no un título con aspa— y el foco
// no sale de él. Pero lleva lo de un diálogo: el velo, la superficie del panel, el radio
// de tarjeta de la forma y la sombra `xl`; y en el teléfono, la HOJA a toda la altura,
// con «Cancelar» junto al campo.
//
// Las medidas salen de Chromium (arnés del barrido, `medir-paleta.mjs`, 2026-10-16):
// a 1280×800 el panel va en (360, 96) con 560 de ancho; a 390×844, en (0, 40) a todo el
// ancho y hasta abajo.

// Medidas de la paleta, en px de CSS.
const (
	// AnchoPaleta es `md:max-w-[560px]`: px literales, el rem no los toca.
	AnchoPaleta float32 = 560
	// FraccionArribaPaleta es el `md:pt-[12dvh]` y FraccionAltoPaleta el `md:max-h-[70dvh]`.
	FraccionArribaPaleta float32 = 0.12
	FraccionAltoPaleta   float32 = 0.70
	// MaxCoincidencias es el `.slice(0, 12)` de las áreas en la web.
	MaxCoincidencias = 12
	// MaxRegistrosPaleta es el `MAX_REGISTROS_PALETA` de la web: los registros que se
	// piden al servicio por consulta.
	MaxRegistrosPaleta = 8
	// MinimoBusquedaRegistros son las letras desde las que se pregunta al servicio: con
	// una casaría media base de datos y no le diría nada a nadie.
	MinimoBusquedaRegistros = 2
	// IconoPaleta es la lupa de 16.
	IconoPaleta float32 = 16
)

var (
	// PaletaPX es el `px-4` de la fila del campo; HuecoCampoPaleta su `gap-2.5`.
	PaletaPX         = tema.Espacio(4)
	HuecoCampoPaleta = tema.Espacio(2.5)
	// AltoCampoPaleta es el `h-12` del campo; en la hoja, `max-md:h-14` con `pt-1` encima.
	AltoCampoPaleta     = tema.Espacio(12)
	AltoCampoPaletaHoja = tema.Espacio(14)
	ArribaCampoHoja     = tema.Espacio(1)
	// ListaPaletaP es el `p-1.5` de la lista; FilaPaletaPX y FilaPaletaPY los `px-2.5 py-2`
	// de una opción, sobre `text-base`.
	ListaPaletaP = tema.Espacio(1.5)
	FilaPaletaPX = tema.Espacio(2.5)
	FilaPaletaPY = tema.Espacio(2)
	// AltoVacioPaleta es «Nada coincide»: `py-6` sobre `text-sm`.
	AltoVacioPaleta = tema.TextoSM.Linea + 2*tema.Espacio(6)
	// GrupoPaletaPX y GrupoPaletaPT/PB son el `px-2.5 pt-3 pb-1` del rótulo de un grupo
	// (`sytue-overline`); el primero de la lista lleva `first:pt-1`.
	GrupoPaletaPX       = tema.Espacio(2.5)
	GrupoPaletaPT       = tema.Espacio(3)
	GrupoPaletaPTPrimer = tema.Espacio(1)
	GrupoPaletaPB       = tema.Espacio(1)
)

// AltoGrupoPaleta es lo que ocupa el rótulo de un grupo: su caja de `text-xs` con su
// aire. El primero lleva menos arriba, que ya tiene el `p-1.5` de la lista.
func AltoGrupoPaleta(primero bool) float32 {
	arriba := GrupoPaletaPT
	if primero {
		arriba = GrupoPaletaPTPrimer
	}
	return arriba + tema.TextoXS.Linea + GrupoPaletaPB
}

// AltoFilaPaleta es `py-2` sobre la caja de `text-base`; con el dedo, los 44 de
// `pointer-coarse:min-h-11`.
func AltoFilaPaleta() float32 {
	alto := tema.TextoBase.Linea + 2*FilaPaletaPY
	if ui.EsTactil() {
		return max(alto, ui.SueloTactilBarra)
	}
	return alto
}

// OpcionDestino es una fila de la paleta: una pantalla, un área o un registro, con el
// grupo bajo el que se lee. Lleva exactamente uno de los tres punteros.
type OpcionDestino struct {
	// Grupo es el rótulo del grupo, ya traducido.
	Grupo  string
	Rotulo string
	// Pista va a la derecha: el área de un modelo, la referencia de un registro.
	Pista string

	Pantalla *Destino
	Modelo   *Modelo
	Registro *Registro
}

// Paleta es el estado del buscador de destinos.
//
// Llega a los tres sitios a los que se va en un ERP, en este orden y agrupados, como
// la web: las PANTALLAS que no son modelos (Libro diario, Banco, Tareas…, las mismas
// que el menú enseña a esta persona), las ÁREAS del catálogo y los REGISTROS
// («FV-2026-0012», «Acme S.L.»), que busca el servicio con sus permisos.
type Paleta struct {
	tr *i18n.Translator

	// Modelos son TODOS los del catálogo, encendidos o no: la web no los filtra por
	// módulo activo, y por eso la paleta llega donde el menú no.
	Modelos []Modelo
	// Areas es el nombre de cada área por su clave: la pista de cada opción dice
	// «Contabilidad», no la clave interna («consolidacion»).
	Areas map[string]string
	// Pantallas son los destinos fijos del menú (Inicio, Tareas, Libro diario…): la
	// pantalla los pone al rellenar el menú, con sus mismas condiciones.
	Pantallas []Destino
	// Registros son lo que contestó el servicio a la consulta de ahora, y Buscando
	// dice que aún no ha contestado: mientras, no se afirma que no haya nada.
	Registros []Registro
	Buscando  bool
	// Consulta es lo escrito y Cursor la fila resaltada.
	Consulta string
	Cursor   int

	AlElegir func(Modelo)
	// AlIr abre una pantalla y AlRegistro la ficha de un registro.
	AlIr       func(Destino)
	AlRegistro func(Registro)
	// AlBuscar pide al servicio los registros de lo escrito (ya sin espacios a los
	// lados). Llega con cada consulta, también con las cortas, para que quien la
	// atiende suelte la anterior.
	AlBuscar func(string)
	AlCerrar func()
	// AlMover pide repintar: cambió lo escrito o la fila resaltada.
	AlMover func()

	// campo es el MISMO de una pintada a otra: cada flecha repinta la lista, y un campo
	// nuevo en cada repintado perdería el foco —y con él las flechas— a la primera.
	campo *ui.Entrada
	// desplazado es dónde estaba la lista: cada flecha rehace el árbol, y sin
	// recordarlo la lista volvería arriba a cada pulsación.
	desplazado float32
}

// NuevaPaleta crea la paleta cerrada sobre un catálogo.
func NuevaPaleta(tr *i18n.Translator, modelos []Modelo) *Paleta {
	p := &Paleta{tr: tr, Modelos: modelos}
	p.campo = ui.NuevaEntrada(tr.T("palette.placeholder", nil))
	p.campo.AlCambiar(func(s string) {
		if s == p.Consulta {
			return
		}
		p.Escribir(s)
		p.mover()
	})
	p.campo.AlTeclear = p.Teclear
	return p
}

func (p *Paleta) mover() {
	if p.AlMover != nil {
		p.AlMover()
	}
}

// Teclear son las teclas del campo de la web: las flechas recorren, Intro abre lo
// resaltado y Escape cierra. Devuelve si se quedó la tecla; el resto va al campo.
func (p *Paleta) Teclear(k *fyne.KeyEvent) bool {
	switch k.Name {
	case fyne.KeyDown:
		p.Mover(1)
		p.mover()
	case fyne.KeyUp:
		p.Mover(-1)
		p.mover()
	case fyne.KeyReturn, fyne.KeyEnter:
		if o, hay := p.Elegido(); hay {
			p.elegir(o)
		}
	case fyne.KeyEscape:
		if p.AlCerrar != nil {
			p.AlCerrar()
		}
	default:
		return false
	}
	return true
}

// Campo es el buscador de la paleta, para darle el foco al abrirla (el `autofocus`).
func (p *Paleta) Campo() fyne.Focusable { return p.campo }

// Coincidencias son los modelos que casan con lo escrito, como mucho doce.
func (p *Paleta) Coincidencias() []Modelo {
	out := make([]Modelo, 0, MaxCoincidencias)
	for _, m := range p.Modelos {
		if !Coincide(m.Etiqueta, p.Consulta) {
			continue
		}
		out = append(out, m)
		if len(out) == MaxCoincidencias {
			break
		}
	}
	return out
}

// buscaRegistros dice si lo escrito llega para preguntar al servicio.
func (p *Paleta) buscaRegistros() bool {
	return len([]rune(strings.TrimSpace(p.Consulta))) >= MinimoBusquedaRegistros
}

// Opciones son todas las filas, en el orden en que se leen: las pantallas, las áreas
// (como mucho doce) y los registros. Las dos primeras se filtran aquí, sin tildes, sobre
// los rótulos ya traducidos; los registros ya vienen filtrados por el servicio.
func (p *Paleta) Opciones() []OpcionDestino {
	var out []OpcionDestino
	grupo := p.tr.T("palette.groupViews", nil)
	for i := range p.Pantallas {
		d := &p.Pantallas[i]
		if Coincide(d.Rotulo, p.Consulta) {
			out = append(out, OpcionDestino{Grupo: grupo, Rotulo: d.Rotulo, Pantalla: d})
		}
	}
	grupo = p.tr.T("palette.groupAreas", nil)
	porClave := map[string]Modelo{}
	for _, m := range p.Modelos {
		porClave[m.Clave] = m
	}
	for _, m := range p.Coincidencias() {
		area := p.Areas[m.Modulo]
		if area == "" {
			area = m.Modulo
		}
		out = append(out, OpcionDestino{Grupo: grupo, Rotulo: m.Etiqueta, Pista: area, Modelo: &m})
	}
	if !p.buscaRegistros() {
		return out
	}
	grupo = p.tr.T("palette.groupRecords", nil)
	for i := range p.Registros {
		r := &p.Registros[i]
		// Solo lo que se puede abrir aquí: un registro de un modelo que el catálogo no
		// trae no tendría ficha que enseñar.
		m, hay := porClave[r.Modelo]
		if !hay {
			continue
		}
		rotulo := cmp.Or(r.Etiqueta, r.Ref, r.ID)
		pista := m.Etiqueta
		if r.Ref != "" && r.Ref != rotulo {
			pista = r.Ref + " · " + m.Etiqueta
		}
		out = append(out, OpcionDestino{Grupo: grupo, Rotulo: rotulo, Pista: pista, Registro: r})
	}
	return out
}

// Elegido es el que se abriría con Intro. El segundo valor es falso si no hay ninguno.
func (p *Paleta) Elegido() (OpcionDestino, bool) {
	c := p.Opciones()
	if len(c) == 0 {
		return OpcionDestino{}, false
	}
	return c[min(max(p.Cursor, 0), len(c)-1)], true
}

// elegir lleva a donde dice la opción.
func (p *Paleta) elegir(o OpcionDestino) {
	switch {
	case o.Pantalla != nil && p.AlIr != nil:
		p.AlIr(*o.Pantalla)
	case o.Modelo != nil && p.AlElegir != nil:
		p.AlElegir(*o.Modelo)
	case o.Registro != nil && p.AlRegistro != nil:
		p.AlRegistro(*o.Registro)
	}
}

// Mover baja o sube el cursor, que es lo que hacen las flechas.
func (p *Paleta) Mover(d int) {
	n := len(p.Opciones())
	if n == 0 {
		p.Cursor = 0
		return
	}
	// Da la vuelta por los extremos, como las flechas de `PaletaDeComandos`.
	p.Cursor = ((p.Cursor+d)%n + n) % n
}

// Escribir cambia la consulta y vuelve el cursor al principio, como en la web: con la
// lista recalculada, dejar el cursor donde estaba resalta otra cosa. Con dos letras o
// más pide los registros; con menos, suelta los que hubiera.
//
// Los registros de la consulta anterior se sueltan al cambiarla: mientras llega la
// respuesta nueva, la paleta dice «buscando» y no enseña los de otra búsqueda.
func (p *Paleta) Escribir(s string) {
	otra := strings.TrimSpace(s) != strings.TrimSpace(p.Consulta)
	p.Consulta, p.Cursor, p.desplazado = s, 0, 0
	if p.campo.Texto() != s {
		p.campo.PonerTexto(s)
	}
	switch {
	case !p.buscaRegistros():
		p.Registros, p.Buscando = nil, false
	case otra && p.AlBuscar != nil:
		p.Registros, p.Buscando = nil, true
	default:
		p.Buscando = p.AlBuscar != nil
	}
	if p.AlBuscar != nil {
		p.AlBuscar(strings.TrimSpace(s))
	}
}

// Encontrados recoge lo que contestó el servicio a `q`. Si entretanto se ha escrito
// otra cosa no se toca nada y devuelve falso: una respuesta lenta de «ac» no pisa la
// de «acme».
func (p *Paleta) Encontrados(q string, regs []Registro) bool {
	if !p.buscaRegistros() || strings.TrimSpace(p.Consulta) != q {
		return false
	}
	p.Registros, p.Buscando = regs, false
	return true
}

// Objeto arma el velo y el panel. `lienzo` es el tamaño de la ventana: de él salen la
// hoja del teléfono, el centrado y el 12 % de arriba.
func (p *Paleta) Objeto(v tema.Variante, lienzo fyne.Size) fyne.CanvasObject {
	l := &panelPaleta{hoja: lienzo.Width > 0 && lienzo.Width < ui.Estrecho}
	// El velo OSCURECE y cierra al pulsarlo: el token `overlay`, el de los diálogos.
	l.velo = ui.NuevoVelo(true, p.AlCerrar)
	// `menu-surface` y no `panel-surface`: en el modo traslúcido el panel de los diálogos
	// deja ver lo de detrás, y la web lo compensa con el desenfoque de
	// `sytue-glass-surface`, que aquí no hay; la lista tiene que leerse sobre cualquier
	// pantalla (`TestLaPaletaTraslucidaOcultaLaPantalla`).
	l.caja = ui.NuevaCaja(tema.Color("menu-surface", v), tema.Color("border", v), tema.Forma("radius-card"))
	if !l.hoja {
		l.sombra = ui.NuevaSombra(tema.SombraXL, tema.Forma("radius-card"))
	}
	l.filete = canvas.NewRectangle(tema.Color("border-subtle", v))
	l.lupa = iconos.Trazo(iconos.Buscar, IconoPaleta, "text-subtle", v).Objeto()
	l.campo = p.campo
	hijos := []fyne.CanvasObject{l.velo}
	if l.sombra != nil {
		hijos = append(hijos, l.sombra.Objeto())
	}
	hijos = append(hijos, l.caja.Objetos()...)
	hijos = append(hijos, l.lupa, l.campo, l.filete)
	if l.hoja {
		// Con el dedo no hay Escape: la salida va donde está el pulgar al escribir.
		l.cancelar = ui.NuevoBoton(p.tr.T("ui.cancel", nil), p.AlCerrar).Con(ui.Fantasma, ui.TallaMD)
		hijos = append(hijos, l.cancelar)
	}
	opciones := p.Opciones()
	if len(opciones) == 0 {
		vacio := p.tr.T("relation.empty", nil)
		if p.Buscando {
			vacio = p.tr.T("palette.searching", nil)
		}
		l.vacio = ui.NuevoParrafo(vacio, tema.TextoSM, ui.Normal, "text-muted", v)
		hijos = append(hijos, l.vacio)
	}
	// Las filas van en su propia columna desplazable: con las pantallas, las doce áreas
	// y los registros ya no caben siempre en el `max-h-[70dvh]`.
	// El resaltado es el de `Elegido`, acotado como el `Math.min(…)` de la web: si la
	// respuesta del servicio acorta la lista con el cursor en un registro, la última fila
	// sigue resaltada y es la que abre Intro.
	cursor := min(max(p.Cursor, 0), len(opciones)-1)
	l.lista = &listaPaleta{cursor: cursor}
	for i, o := range opciones {
		if i == 0 || opciones[i-1].Grupo != o.Grupo {
			l.lista.grupos = append(l.lista.grupos, grupoPaleta{
				antesDe: i, primero: i == 0,
				rotulo: ui.NuevoVersal(o.Grupo, tema.TextoXS, ui.SemiNegrita, tema.Color("text-subtle", v)),
			})
		}
		indice, opcion := i, o
		f := NuevaOpcionPaleta(o.Rotulo, o.Pista, i == cursor, func() { p.elegir(opcion) })
		// `onMouseEnter={() => setActiva(n)}`: el ratón mueve el resaltado, y así Intro
		// abre lo que se está señalando y no otra cosa.
		f.AlEntrar = func() {
			if p.Cursor != indice {
				p.Cursor = indice
				p.mover()
			}
		}
		l.lista.filas = append(l.lista.filas, f)
	}
	if len(opciones) > 0 {
		var objs []fyne.CanvasObject
		for _, g := range l.lista.grupos {
			objs = append(objs, g.rotulo.Objetos()...)
		}
		for _, f := range l.lista.filas {
			objs = append(objs, f)
		}
		l.desplaza = container.NewVScroll(container.New(l.lista, objs...))
		l.desplaza.Offset.Y = p.desplazado
		l.desplaza.OnScrolled = func(o fyne.Position) { p.desplazado = o.Y }
		hijos = append(hijos, l.desplaza)
	}
	return container.New(l, hijos...)
}

// grupoPaleta es el rótulo de un grupo (`sytue-overline`), que va antes de la fila
// `antesDe`.
type grupoPaleta struct {
	antesDe int
	primero bool
	rotulo  *ui.Versal
}

// listaPaleta coloca los rótulos de grupo y las filas una debajo de otra, y sabe dónde
// cae la fila del cursor para que la lista la tenga a la vista.
type listaPaleta struct {
	grupos []grupoPaleta
	filas  []*OpcionPaleta
	cursor int
}

var _ fyne.Layout = (*listaPaleta)(nil)

// colocar recorre la lista; `poner` recibe cada fila con su `y`, y devuelve el alto.
func (l *listaPaleta) colocar(ancho float32, poner bool) (alto, arribaCursor, abajoCursor float32) {
	y, g := float32(0), 0
	for i, f := range l.filas {
		for g < len(l.grupos) && l.grupos[g].antesDe == i {
			gr := l.grupos[g]
			arriba := GrupoPaletaPT
			if gr.primero {
				arriba = GrupoPaletaPTPrimer
			}
			if poner {
				gr.rotulo.Colocar(GrupoPaletaPX, y+arriba)
				gr.rotulo.Recortar(max(ancho-2*GrupoPaletaPX, 0))
			}
			y += AltoGrupoPaleta(gr.primero)
			g++
		}
		if poner {
			f.Move(fyne.NewPos(0, y))
			f.Resize(fyne.NewSize(ancho, AltoFilaPaleta()))
		}
		if i == l.cursor {
			arribaCursor, abajoCursor = y, y+AltoFilaPaleta()
		}
		y += AltoFilaPaleta()
	}
	return y, arribaCursor, abajoCursor
}

func (l *listaPaleta) MinSize([]fyne.CanvasObject) fyne.Size {
	alto, _, _ := l.colocar(0, false)
	return fyne.NewSize(0, alto)
}

func (l *listaPaleta) Layout(_ []fyne.CanvasObject, t fyne.Size) { l.colocar(t.Width, true) }

// OpcionPaleta es una opción de la paleta: lo que se lee y, a la derecha, su pista (el
// área de un modelo, la referencia de un registro). La activa lleva `bg-selected` con
// `rounded-md`.
type OpcionPaleta struct {
	ui.Pulsable

	Rotulo string
	// Pista va a la derecha, apagada.
	Pista string
	// Resaltada es la opción activa.
	Resaltada bool
	// AlEntrar avisa de que el ratón ha entrado, para mover el cursor.
	AlEntrar func()
}

var _ fyne.Widget = (*OpcionPaleta)(nil)

// NuevaOpcionPaleta crea una fila de la paleta.
func NuevaOpcionPaleta(rotulo, pista string, resaltada bool, alPulsar func()) *OpcionPaleta {
	f := &OpcionPaleta{Rotulo: rotulo, Pista: pista, Resaltada: resaltada}
	f.Nombre, f.AlPulsar = rotulo, alPulsar
	f.ExtendBaseWidget(f)
	return f
}

// MouseIn mueve el cursor a esta fila además del hover de `Pulsable`.
func (f *OpcionPaleta) MouseIn(e *desktop.MouseEvent) {
	f.Pulsable.MouseIn(e)
	if f.AlEntrar != nil {
		f.AlEntrar()
	}
}

// CreateRenderer construye el fondo y los dos textos.
func (f *OpcionPaleta) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &opcionPaletaRenderer{
		f:      f,
		fondo:  ui.NuevaCaja(color.NRGBA{}, color.NRGBA{}, tema.RadioMD),
		nombre: ui.Texto(f.Rotulo, tema.TextoBase, ui.Normal, tema.Color("text", v)),
		modulo: ui.Texto(f.Pista, tema.TextoXS, ui.Normal, tema.Color("text-subtle", v)),
	}
	r.objetos = append(r.fondo.Objetos(), r.nombre, r.modulo)
	r.Refresh()
	return r
}

type opcionPaletaRenderer struct {
	f              *OpcionPaleta
	fondo          *ui.Caja
	nombre, modulo *canvas.Text
	objetos        []fyne.CanvasObject
}

func (r *opcionPaletaRenderer) MinSize() fyne.Size           { return fyne.NewSize(0, AltoFilaPaleta()) }
func (r *opcionPaletaRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *opcionPaletaRenderer) Destroy()                     {}

func (r *opcionPaletaRenderer) Layout(t fyne.Size) {
	r.fondo.Redimensionar(t)
	// La pista: `max-w-[40%] truncate`, pegada a la derecha.
	pista := min(ui.Medir(r.f.Pista, tema.TextoXS, ui.Normal).Width, 0.4*max(t.Width-2*FilaPaletaPX, 0))
	ui.Truncar(r.modulo, r.f.Pista, tema.TextoXS, ui.Normal, pista)
	ui.ColocarTexto(r.modulo, tema.TextoXS, t.Width-FilaPaletaPX-pista, (t.Height-tema.TextoXS.Linea)/2)
	cabe := max(t.Width-2*FilaPaletaPX-pista-HuecoCampoPaleta, 0)
	// El nombre SIN recortar, siempre: truncar sobre lo truncado deja «…» para siempre
	// en cuanto el primer layout llega con ancho cero.
	ui.Truncar(r.nombre, r.f.Rotulo, tema.TextoBase, ui.Normal, cabe)
	ui.ColocarTexto(r.nombre, tema.TextoBase, FilaPaletaPX, (t.Height-tema.TextoBase.Linea)/2)
}

func (r *opcionPaletaRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	fondo := color.NRGBA{}
	if r.f.Resaltada {
		fondo = tema.Color("selected", v)
	}
	r.fondo.Pintar(fondo, color.NRGBA{})
	r.nombre.Color = tema.Color("text", v)
	r.nombre.Refresh()
	r.modulo.Color = tema.Color("text-subtle", v)
	r.modulo.Refresh()
}

// Medidas de FilaPaleta, la fila del selector de relaciones.
var (
	paletaPXRelacion = tema.Espacio(4)
	altoFilaRelacion = tema.TextoSM.Linea + 2*tema.Espacio(2)
)

// FilaPaleta es la opción del selector de relaciones (era la de la paleta antes de que
// pasara a `PaletaDeComandos`): el nombre y, apagado a la derecha, la clave del módulo. Es un BOTÓN en la web —se pulsa, se enfoca y se anuncia—, así
// que aquí es un widget y no un contenedor: la regla de `ui.go` es elegir por
// comportamiento.
type FilaPaleta struct {
	ui.Pulsable

	Modelo Modelo
	// Resaltada es la fila del cursor: `bg-menu-selected text-text`.
	Resaltada bool
	// AlEntrar avisa de que el ratón ha entrado, para mover el cursor.
	AlEntrar func()
	// Margen es el relleno horizontal; cero es el `px-4` de la paleta. Las opciones
	// del selector de relaciones, que son la misma fila, llevan `px-3`.
	Margen float32
}

func (f *FilaPaleta) margen() float32 {
	if f.Margen > 0 {
		return f.Margen
	}
	return paletaPXRelacion
}

var _ fyne.Widget = (*FilaPaleta)(nil)

// NuevaFilaPaleta crea una fila de la paleta.
func NuevaFilaPaleta(m Modelo, resaltada bool, alPulsar func()) *FilaPaleta {
	f := &FilaPaleta{Modelo: m, Resaltada: resaltada}
	f.Nombre, f.AlPulsar = m.Etiqueta, alPulsar
	f.ExtendBaseWidget(f)
	return f
}

// MouseIn mueve el cursor a esta fila además del hover de `Pulsable`.
func (f *FilaPaleta) MouseIn(e *desktop.MouseEvent) {
	f.Pulsable.MouseIn(e)
	if f.AlEntrar != nil {
		f.AlEntrar()
	}
}

// MedidaFilaPaleta es el `py-2` sobre la caja de `text-sm`; el ancho lo pone el panel.
func MedidaFilaPaleta(*FilaPaleta) fyne.Size { return fyne.NewSize(0, altoFilaRelacion) }

// CreateRenderer construye el fondo y los dos textos.
func (f *FilaPaleta) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &filaPaletaRenderer{
		f:      f,
		aro:    ui.NuevoAro(0, ui.SinHueco, v),
		fondo:  ui.NuevaCaja(color.NRGBA{}, color.NRGBA{}, 0),
		nombre: ui.Texto(f.Modelo.Etiqueta, tema.TextoSM, ui.Normal, tema.Color("text-muted", v)),
		modulo: ui.Texto(f.Modelo.Modulo, tema.TextoXS, ui.Normal, tema.Color("text-subtle", v)),
	}
	r.objetos = append(r.aro.Objetos(), r.fondo.Objetos()...)
	r.objetos = append(r.objetos, r.nombre, r.modulo)
	r.Refresh()
	return r
}

type filaPaletaRenderer struct {
	f              *FilaPaleta
	aro            *ui.Aro
	fondo          *ui.Caja
	nombre, modulo *canvas.Text
	objetos        []fyne.CanvasObject
}

func (r *filaPaletaRenderer) MinSize() fyne.Size           { return MedidaFilaPaleta(r.f) }
func (r *filaPaletaRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *filaPaletaRenderer) Destroy()                     {}

func (r *filaPaletaRenderer) Layout(t fyne.Size) {
	r.aro.Colocar(t)
	r.fondo.Redimensionar(t)
	anchoMod := ui.Medir(r.f.Modelo.Modulo, tema.TextoXS, ui.Normal).Width
	ui.ColocarTexto(r.modulo, tema.TextoXS, t.Width-r.f.margen()-anchoMod,
		(t.Height-tema.TextoXS.Linea)/2)
	cabe := max(t.Width-2*r.f.margen()-anchoMod-tema.Espacio(2), 0)
	// El nombre SIN recortar, siempre: truncar sobre lo truncado deja «…» para siempre
	// en cuanto el primer layout llega con ancho cero.
	ui.Truncar(r.nombre, r.f.Modelo.Etiqueta, tema.TextoSM, ui.Normal, cabe)
	ui.ColocarTexto(r.nombre, tema.TextoSM, r.f.margen(), (t.Height-tema.TextoSM.Linea)/2)
}

func (r *filaPaletaRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	tinta, fondo := tema.Color("text-muted", v), color.NRGBA{}
	if r.f.Resaltada {
		tinta, fondo = tema.Color("text", v), tema.Color("menu-selected", v)
	}
	r.aro.Ver(r.f.Enfocado())
	r.aro.Pintar(v)
	r.fondo.Pintar(fondo, color.NRGBA{})
	r.nombre.Color = tinta
	r.nombre.Refresh()
	r.modulo.Color = tema.Color("text-subtle", v)
	r.modulo.Refresh()
}

// panelPaleta coloca el velo a pantalla completa y el panel: a 12 dvh en escritorio, o
// la hoja del teléfono desde `ui.HuecoHoja` hasta abajo.
type panelPaleta struct {
	hoja     bool
	velo     fyne.CanvasObject
	sombra   *ui.Sombra
	caja     *ui.Caja
	lupa     fyne.CanvasObject
	campo    *ui.Entrada
	cancelar *ui.Boton
	filete   *canvas.Rectangle
	vacio    *ui.Parrafo
	lista    *listaPaleta
	desplaza *container.Scroll
}

var _ fyne.Layout = (*panelPaleta)(nil)

// altoCabecera es la fila del campo con su filete de abajo.
func (l *panelPaleta) altoCabecera() float32 {
	if l.hoja {
		return ArribaCampoHoja + AltoCampoPaletaHoja + tema.Borde
	}
	return AltoCampoPaleta + tema.Borde
}

// AltoPanel es lo que ocupa el panel en escritorio: la cabecera y la lista con su
// `p-1.5`, sin pasar del `max-h-[70dvh]`. En la hoja, todo el alto que hay.
func (l *panelPaleta) AltoPanel(t fyne.Size) float32 {
	if l.hoja {
		return max(t.Height-ui.HuecoHoja, 0)
	}
	lista := AltoVacioPaleta
	if l.vacio == nil {
		lista, _, _ = l.lista.colocar(0, false)
	}
	return min(2*tema.Borde+l.altoCabecera()+lista+2*ListaPaletaP, FraccionAltoPaleta*t.Height)
}

func (l *panelPaleta) MinSize([]fyne.CanvasObject) fyne.Size { return fyne.Size{} }

func (l *panelPaleta) Layout(_ []fyne.CanvasObject, t fyne.Size) {
	l.velo.Move(fyne.NewPos(0, 0))
	l.velo.Resize(t)
	b := tema.Borde
	ancho := min(AnchoPaleta, t.Width-2*tema.Espacio(4))
	x, y := (t.Width-ancho)/2, t.Height*FraccionArribaPaleta
	lado := b // el filete de los lados; la hoja no lo lleva
	if l.hoja {
		ancho, x, y, lado = t.Width, 0, ui.HuecoHoja, 0
	}
	alto := l.AltoPanel(t)
	if l.hoja {
		// Redonda solo arriba, sin filetes a los lados ni abajo: la caja 1 px más ancha
		// por cada lado y más alta, como la hoja de `ui.Dialogo`.
		ui.MoverCaja(l.caja, fyne.NewPos(-b, y), fyne.NewSize(ancho+2*b, alto+b))
		for _, o := range l.caja.Objetos() {
			if r, es := o.(*canvas.Rectangle); es {
				// ponytail: Fyne lee un radio de esquina 0 como «el del rectángulo».
				r.BottomLeftCornerRadius, r.BottomRightCornerRadius = 1e-3, 1e-3
			}
		}
	} else {
		ui.MoverCaja(l.caja, fyne.NewPos(x, y), fyne.NewSize(ancho, alto))
		l.sombra.Ajustar(fyne.NewPos(x, y), fyne.NewSize(ancho, alto), tema.De(fyne.CurrentApp().Settings().ThemeVariant()))
	}

	// La fila del campo: la lupa, el campo que crece y, en la hoja, «Cancelar».
	arriba := y + b
	altoCampo := AltoCampoPaleta
	if l.hoja {
		arriba += ArribaCampoHoja
		altoCampo = AltoCampoPaletaHoja
	}
	cx := x + lado + PaletaPX
	l.lupa.Move(fyne.NewPos(cx, arriba+(altoCampo-IconoPaleta)/2))
	l.lupa.Resize(fyne.NewSize(IconoPaleta, IconoPaleta))
	cx += IconoPaleta + HuecoCampoPaleta
	fin := x + ancho - lado - PaletaPX
	if l.cancelar != nil {
		m := l.cancelar.MinSize()
		// `-mr-2`: el botón se mete en el `px-4` para que su rótulo quede alineado.
		fin += tema.Espacio(2)
		l.cancelar.Resize(m)
		l.cancelar.Move(fyne.NewPos(fin-m.Width, arriba+(altoCampo-m.Height)/2))
		fin -= m.Width + HuecoCampoPaleta
	}
	l.campo.Move(fyne.NewPos(cx, arriba))
	l.campo.Resize(fyne.NewSize(max(fin-cx, 0), altoCampo))
	l.filete.Move(fyne.NewPos(x+lado, arriba+altoCampo))
	l.filete.Resize(fyne.NewSize(ancho-2*lado, b))

	fy := arriba + altoCampo + b + ListaPaletaP
	if l.vacio != nil {
		w := ancho - 2*lado - 2*ListaPaletaP
		l.vacio.Move(fyne.NewPos(x+lado+ListaPaletaP+(w-ui.Medir(l.vacio.Texto, tema.TextoSM, ui.Normal).Width)/2, fy+tema.Espacio(6)))
		l.vacio.Resize(fyne.NewSize(w, tema.TextoSM.Linea))
	}
	if l.desplaza != nil {
		// La lista llena lo que queda del panel menos su `p-1.5` de abajo, y la fila
		// del cursor siempre a la vista (el `scrollIntoView({ block: 'nearest' })`).
		visible := max(y+alto-b-ListaPaletaP-fy, 0)
		l.desplaza.Move(fyne.NewPos(x+lado+ListaPaletaP, fy))
		l.desplaza.Resize(fyne.NewSize(ancho-2*lado-2*ListaPaletaP, visible))
		total, arriba, abajo := l.lista.colocar(0, false)
		off := l.desplaza.Offset.Y
		if arriba < off {
			off = arriba
		} else if abajo > off+visible {
			off = abajo - visible
		}
		off = min(max(off, 0), max(total-visible, 0))
		if off != l.desplaza.Offset.Y {
			l.desplaza.ScrollToOffset(fyne.NewPos(0, off))
		}
	}
}
