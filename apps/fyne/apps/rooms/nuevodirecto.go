// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package rooms

import (
	"errors"
	"image/color"
	"net/http"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/buscar"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
	"git.sytue.eu/sytue/one/apps/nucleo/tokens"
)

// Contrapartida de NewDmModal.tsx, BuscarPersona.tsx y RemovableChip.tsx dentro de
// la pantalla principal declarada en pantalla.go.
//
// «Nuevo mensaje»: diálogo `sm` para abrir un directo con una persona o un grupo de
// hasta siete. Arriba las elegidas en píldoras que se quitan al pulsarlas; la caja
// filtra la gente del equipo por nombre y, si lo escrito es un correo o un @usuario,
// ofrece buscarlo en Sytue One entero (ADR 0040): quien sale de otra organización va
// marcada como tal.

// MaxDirecto es `MAX_OTHERS`: cuántas personas además de ti.
const MaxDirecto = 7

// PareceIdentificador es `pareceIdentificador`: lleva arroba, no lleva espacios y
// tiene más de un carácter. Una regla más fina sería otra que acabaría discrepando de
// la de Sytue ID, que es la que manda.
func PareceIdentificador(q string) bool {
	q = strings.TrimSpace(q)
	return len(q) > 1 && strings.Contains(q, "@") && !strings.ContainsAny(q, " \t\n")
}

// ClaveFalloBusqueda es `claveDeFalloDeBusqueda`: no es lo mismo «no existe» que
// «espera».
func ClaveFalloBusqueda(err error) string {
	var e *api.Error
	if errors.As(err, &e) {
		switch e.Status {
		case http.StatusNotFound:
			return "people.notFound"
		case http.StatusTooManyRequests:
			return "people.tooMany"
		case http.StatusBadRequest:
			return "people.invalid"
		}
	}
	return "people.failed"
}

// Persona es alguien que se puede elegir: del equipo, o de fuera (encontrada por su
// @usuario o su correo).
type Persona struct {
	ID, Nombre string
	Externa    bool
}

// NuevoDirecto es el cuerpo del diálogo y su estado.
type NuevoDirecto struct {
	widget.BaseWidget

	T *i18n.Translator
	// Equipo es la gente del equipo, sin quien mira.
	Equipo   []Persona
	Elegidas []Persona
	// Ocupado es el envío en camino; Fallo, que no se pudo abrir.
	Ocupado, Fallo bool
	// Buscando y ErrorBuscar son los de «Buscar X en Sytue One».
	Buscando    bool
	ErrorBuscar string

	// AlBuscarPersona pide a Sytue ID a quien se escribió; AlAbrir abre el directo.
	AlBuscarPersona func(q string)
	AlAbrir         func([]Persona)

	caja    *ui.Entrada
	abrir   *ui.Boton
	cuerpo  *fyne.Container
	objetos []fyne.CanvasObject
}

var _ fyne.Widget = (*NuevoDirecto)(nil)

// NuevoNuevoDirecto crea el cuerpo vacío.
func NuevoNuevoDirecto(t *i18n.Translator, equipo []Persona) *NuevoDirecto {
	d := &NuevoDirecto{T: t, Equipo: equipo}
	d.ExtendBaseWidget(d)
	d.caja = ui.NuevaEntrada(t.T("dm.searchPlaceholder", nil))
	d.caja.Icono = iconos.Buscar
	d.caja.AlCambiar(func(string) {
		d.ErrorBuscar = "" // un error es de LO QUE se buscó
		d.Refresh()
	})
	d.abrir = ui.NuevoBoton("", func() { d.Confirmar() }).Con(ui.Primario, ui.TallaMD)
	return d
}

// Caja es la de buscar, que recibe el foco.
func (d *NuevoDirecto) Caja() *ui.Entrada { return d.caja }

// BotonAbrir es el primario del pie: «Abrir chat» o «Crear grupo (N)».
func (d *NuevoDirecto) BotonAbrir() *ui.Boton { return d.abrir }

func (d *NuevoDirecto) elegida(id string) bool {
	for _, p := range d.Elegidas {
		if p.ID == id {
			return true
		}
	}
	return false
}

func (d *NuevoDirecto) lleno() bool { return len(d.Elegidas) >= MaxDirecto }

// Conmutar es `toggle`: quita a quien ya está, y añade si no se ha llegado al tope.
func (d *NuevoDirecto) Conmutar(p Persona) {
	d.Fallo = false
	for i, x := range d.Elegidas {
		if x.ID == p.ID {
			d.Elegidas = append(d.Elegidas[:i:i], d.Elegidas[i+1:]...)
			d.Refresh()
			return
		}
	}
	if !d.lleno() {
		d.Elegidas = append(d.Elegidas, p)
	}
	d.Refresh()
}

// Encontrada es `addPersona`: la persona que devolvió Sytue ID va a las píldoras,
// marcada como de fuera si no es del equipo. La caja se vacía.
func (d *NuevoDirecto) Encontrada(p Persona) {
	d.caja.PonerTexto("")
	if d.elegida(p.ID) {
		return
	}
	for _, m := range d.Equipo {
		if m.ID == p.ID {
			d.Conmutar(m)
			return
		}
	}
	p.Externa = true
	d.Conmutar(p)
}

// Confirmar es `confirm`: sin nadie elegido, u ocupado, no hace nada.
func (d *NuevoDirecto) Confirmar() {
	if len(d.Elegidas) == 0 || d.Ocupado || d.AlAbrir == nil {
		return
	}
	d.Ocupado, d.Fallo = true, false
	d.Refresh()
	d.AlAbrir(append([]Persona(nil), d.Elegidas...))
}

// Candidatas es `candidates`: la gente del equipo cuyo nombre contiene lo escrito, a
// lo sumo cien.
func (d *NuevoDirecto) Candidatas() []Persona {
	q := buscar.Plegar(strings.TrimSpace(d.caja.Texto()))
	var out []Persona
	for _, m := range d.Equipo {
		if q == "" || strings.Contains(buscar.Plegar(m.Nombre), q) {
			out = append(out, m)
		}
		if len(out) == 100 {
			break
		}
	}
	return out
}

// construir rehace la columna: `flex flex-col gap-3`.
func (d *NuevoDirecto) construir(v tema.Variante) {
	t := d.T
	q := strings.TrimSpace(d.caja.Texto())
	var hijos []fyne.CanvasObject
	if len(d.Elegidas) > 0 {
		chips := make([]fyne.CanvasObject, 0, len(d.Elegidas))
		for _, p := range d.Elegidas {
			rotulo := p.Nombre
			if p.Externa {
				rotulo = t.T("people.externalName", i18n.Vars{"name": p.Nombre})
			}
			chips = append(chips, NuevoChipQuitable(rotulo, t.T("action.removePerson", i18n.Vars{"name": p.Nombre}),
				ui.NuevoAvatar(p.Nombre).Con(ui.AvatarXS), func() { d.Conmutar(p) }))
		}
		hijos = append(hijos, container.New(&filaEnvuelta{hueco: tema.Espacio(1.5)}, chips...))
	}
	hijos = append(hijos, d.caja)
	if PareceIdentificador(q) {
		rotulo := t.T("people.search", i18n.Vars{"query": q})
		if d.Buscando {
			rotulo = t.T("people.searching", nil)
		}
		b := NuevaFilaBuscarPersona(rotulo, func() {
			if d.AlBuscarPersona != nil {
				d.AlBuscarPersona(q)
			}
		})
		b.Inerte = d.Buscando || d.Ocupado || d.lleno()
		hijos = append(hijos, b)
		if d.ErrorBuscar != "" {
			hijos = append(hijos, ui.NuevoRotulo(d.ErrorBuscar, tema.TextoSM, ui.Normal, "danger-text"))
		}
	}
	if d.Fallo {
		hijos = append(hijos, ui.NuevoRotulo(t.T("dm.openFailed", nil), tema.TextoSM, ui.Normal, "danger-text"))
	}
	if d.lleno() {
		hijos = append(hijos, ui.NuevoRotulo(t.T("dm.maxPeople", i18n.Vars{"count": MaxDirecto}), tema.TextoXS, ui.Normal, "text-muted"))
	}
	candidatas := d.Candidatas()
	switch {
	case len(candidatas) == 0 && PareceIdentificador(q):
	case len(candidatas) == 0:
		titulo, ilus := t.T("dm.empty", nil), iconos.VacioContactos
		if q != "" {
			titulo, ilus = t.T("dm.emptyQuery", nil), iconos.VacioBusqueda
		}
		// ponytail: sin el «Invitar personas» que la web enseña a quien administra:
		// abre la consola, y la sesión nativa todavía no sabe los roles de quien mira.
		vac := ui.NuevoVacio(titulo, "")
		vac.Ilustracion = iconos.Pintar(ilus, v).A(LadoIlustracionBusqueda).Objeto()
		hijos = append(hijos, vac)
	default:
		filas := make([]fyne.CanvasObject, 0, len(candidatas))
		for _, p := range candidatas {
			f := NuevaFilaPersona(p, d.elegida(p.ID), func() { d.Conmutar(p) })
			f.Inerte = d.Ocupado || (!f.Marcada && d.lleno())
			filas = append(filas, f)
		}
		hijos = append(hijos, container.New(ui.Columna{Hueco: tema.Espacio(0.5)}, filas...))
	}
	d.cuerpo = container.New(ui.Columna{Hueco: tema.Espacio(3)}, hijos...)
	d.abrir.Texto = t.T("dm.openChat", nil)
	if n := len(d.Elegidas); n > 1 {
		d.abrir.Texto = t.T("dm.createGroup", i18n.Vars{"count": n})
	}
	d.abrir.Inerte, d.abrir.Cargando = len(d.Elegidas) == 0, d.Ocupado
	d.abrir.Refresh()
}

// CreateRenderer monta la columna.
func (d *NuevoDirecto) CreateRenderer() fyne.WidgetRenderer {
	r := &nuevoDirectoRenderer{d: d}
	r.Refresh()
	return r
}

type nuevoDirectoRenderer struct{ d *NuevoDirecto }

func (r *nuevoDirectoRenderer) MinSize() fyne.Size           { return r.d.cuerpo.MinSize() }
func (r *nuevoDirectoRenderer) Objects() []fyne.CanvasObject { return r.d.objetos }
func (r *nuevoDirectoRenderer) Destroy()                     {}
func (r *nuevoDirectoRenderer) Layout(t fyne.Size)           { r.d.cuerpo.Resize(t) }

func (r *nuevoDirectoRenderer) Refresh() {
	r.d.construir(tema.De(fyne.CurrentApp().Settings().ThemeVariant()))
	r.d.objetos = []fyne.CanvasObject{r.d.cuerpo}
	r.Layout(r.d.Size())
}

// filaEnvuelta es `flex flex-wrap gap-*`: en fila y, cuando no cabe, a la siguiente.
type filaEnvuelta struct {
	hueco float32
	ancho float32
}

func (f *filaEnvuelta) MinSize(objs []fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(0, f.colocar(objs, f.ancho, false))
}

func (f *filaEnvuelta) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	f.ancho = t.Width
	f.colocar(objs, t.Width, true)
}

// colocar reparte los hijos en líneas de `ancho` y devuelve el alto total. Sin ancho
// todavía, cada hijo va en su línea.
func (f *filaEnvuelta) colocar(objs []fyne.CanvasObject, ancho float32, mover bool) float32 {
	var x, y, linea float32
	for _, o := range objs {
		m := o.MinSize()
		if x > 0 && (ancho <= 0 || x+m.Width > ancho) {
			x, y, linea = 0, y+linea+f.hueco, 0
		}
		if mover {
			o.Move(fyne.NewPos(x, y))
			o.Resize(m)
		}
		x += m.Width + f.hueco
		linea = max(linea, m.Height)
	}
	return y + linea
}

// ---- Las piezas del diálogo ----

// ChipQuitable es `RemovableChip`: la píldora entera es el botón que la quita.
// `rounded-pill border bg-bg-subtle inline-flex items-center gap-1.5 py-1 pr-2
// text-sm font-medium`, `pl-1.5` con avatar, el rótulo hasta `10rem` y la «×» en
// `text-muted`; `hover:bg-selected`.
type ChipQuitable struct {
	ui.Pulsable
	Rotulo  string
	Delante fyne.CanvasObject

	caja    *ui.Caja
	aro     *ui.Aro
	texto   *canvas.Text
	aspa    *canvas.Text
	objetos []fyne.CanvasObject
}

var _ fyne.Widget = (*ChipQuitable)(nil)

// Medidas de la píldora. HOJA.
var (
	RellenoChip               = tema.Espacio(1)
	CanalonChip               = tema.Espacio(2)
	CanalonChipAvatar         = tema.Espacio(1.5)
	HuecoChip                 = tema.Espacio(1.5)
	AnchoRotuloChip   float32 = 10 * tema.Rem
)

// NuevoChipQuitable crea la píldora; `nombre` es su nombre accesible.
func NuevoChipQuitable(rotulo, nombre string, delante fyne.CanvasObject, alQuitar func()) *ChipQuitable {
	c := &ChipQuitable{Rotulo: rotulo, Delante: delante}
	c.Nombre, c.AlPulsar = nombre, alQuitar
	c.ExtendBaseWidget(c)
	return c
}

func (c *ChipQuitable) anchoRotulo() float32 {
	return min(ui.Medir(c.Rotulo, tema.TextoSM, ui.Medio).Width, AnchoRotuloChip)
}

// MinSize es la medida de la píldora: manda el avatar de 24 o la línea de 15,75.
func (c *ChipQuitable) MinSize() fyne.Size {
	c.ExtendBaseWidget(c)
	izq, alto := CanalonChip, tema.TextoSM.Linea
	ancho := c.anchoRotulo() + HuecoChip + ui.Medir("×", tema.TextoSM, ui.Medio).Width
	if c.Delante != nil {
		m := c.Delante.MinSize()
		izq, alto = CanalonChipAvatar, max(alto, m.Height)
		ancho += m.Width + HuecoChip
	}
	return fyne.NewSize(2*tema.Borde+izq+ancho+tema.Espacio(2), 2*tema.Borde+2*RellenoChip+alto)
}

// CreateRenderer compone la píldora.
func (c *ChipQuitable) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	c.aro = ui.NuevoAro(tema.Forma("radius-control"), ui.SinHueco, v)
	c.caja = ui.NuevaCaja(tema.Color("bg-subtle", v), tema.Color("border", v), tema.Forma("radius-control"))
	c.texto = ui.Texto(c.Rotulo, tema.TextoSM, ui.Medio, tema.Color("text", v))
	c.aspa = ui.Texto("×", tema.TextoSM, ui.Medio, tema.Color("text-muted", v))
	c.objetos = c.aro.Antes(c.caja.Objetos()...)
	if c.Delante != nil {
		c.objetos = append(c.objetos, c.Delante)
	}
	c.objetos = append(c.objetos, c.texto, c.aspa)
	r := &chipRenderer{c: c}
	r.Refresh()
	return r
}

type chipRenderer struct{ c *ChipQuitable }

func (r *chipRenderer) MinSize() fyne.Size           { return r.c.MinSize() }
func (r *chipRenderer) Objects() []fyne.CanvasObject { return r.c.objetos }
func (r *chipRenderer) Destroy()                     {}

func (r *chipRenderer) Layout(t fyne.Size) {
	c := r.c
	c.aro.Colocar(t)
	c.caja.Redimensionar(t)
	x := tema.Borde + CanalonChip
	if c.Delante != nil {
		x = tema.Borde + CanalonChipAvatar
		m := c.Delante.MinSize()
		c.Delante.Move(fyne.NewPos(x, (t.Height-m.Height)/2))
		c.Delante.Resize(m)
		x += m.Width + HuecoChip
	}
	y := (t.Height - tema.TextoSM.Linea) / 2
	ui.Truncar(c.texto, c.Rotulo, tema.TextoSM, ui.Medio, AnchoRotuloChip)
	ui.ColocarTexto(c.texto, tema.TextoSM, x, y)
	ui.ColocarTexto(c.aspa, tema.TextoSM, x+c.anchoRotulo()+HuecoChip, y)
}

func (r *chipRenderer) Refresh() {
	c := r.c
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	fondo := tema.Color("bg-subtle", v)
	if c.Encima() {
		fondo = tokens.SobreSRGB(tema.Color("selected", v), tema.Color("surface", v))
	}
	c.caja.Pintar(fondo, tema.Color("border", v))
	c.aro.Ver(c.Enfocado())
	c.aro.Pintar(v)
	r.Layout(c.Size())
}

// FilaPersona es una persona del equipo en la lista: la casilla (`h-4 w-4 rounded
// border`, rellena de `primary` con el tic blanco de 12 cuando está elegida), el avatar
// `sm` y el nombre `text-base font-medium`. `px-2 py-2 gap-3 rounded-md`,
// `hover:bg-bg-subtle` y al 40 % cuando no se puede elegir.
type FilaPersona struct {
	ui.Pulsable
	Persona Persona
	Marcada bool

	fondo   *canvas.Rectangle
	casilla *ui.Caja
	tic     *iconos.Dibujo
	avatar  *ui.Avatar
	nombre  *canvas.Text
	objetos []fyne.CanvasObject
}

var (
	_ fyne.Widget       = (*FilaPersona)(nil)
	_ desktop.Hoverable = (*FilaPersona)(nil)
)

// NuevaFilaPersona crea la fila.
func NuevaFilaPersona(p Persona, marcada bool, alPulsar func()) *FilaPersona {
	f := &FilaPersona{Persona: p, Marcada: marcada}
	f.Nombre, f.AlPulsar = p.Nombre, alPulsar
	f.ExtendBaseWidget(f)
	return f
}

// MedidaFilaPersona: manda el avatar de 32 con el `py-2`.
func MedidaFilaPersona() fyne.Size {
	return fyne.NewSize(0, 2*tema.Espacio(2)+ui.MedidaAvatar(ui.AvatarSM).Height)
}

// CreateRenderer compone la fila.
func (f *FilaPersona) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	f.fondo = canvas.NewRectangle(color.NRGBA{})
	f.fondo.CornerRadius = tema.RadioMD
	f.casilla = ui.NuevaCaja(color.NRGBA{}, tema.Color("border", v), tema.RadioSM)
	f.tic = iconos.Trazo(iconos.Tic, iconos.TallaXXS, "on-primary", v)
	f.avatar = ui.NuevoAvatar(f.Persona.Nombre).Con(ui.AvatarSM)
	f.nombre = ui.Texto(f.Persona.Nombre, tema.TextoBase, ui.Medio, tema.Color("text", v))
	f.objetos = append([]fyne.CanvasObject{f.fondo}, f.casilla.Objetos()...)
	f.objetos = append(f.objetos, f.tic.Objeto(), f.avatar, f.nombre)
	r := &filaPersonaRenderer{f: f}
	r.Refresh()
	return r
}

type filaPersonaRenderer struct{ f *FilaPersona }

func (r *filaPersonaRenderer) MinSize() fyne.Size           { return MedidaFilaPersona() }
func (r *filaPersonaRenderer) Objects() []fyne.CanvasObject { return r.f.objetos }
func (r *filaPersonaRenderer) Destroy()                     {}

func (r *filaPersonaRenderer) Layout(t fyne.Size) {
	f := r.f
	f.fondo.Resize(t)
	x := tema.Espacio(2)
	lado := tema.Espacio(4)
	ui.MoverCaja(f.casilla, fyne.NewPos(x, (t.Height-lado)/2), fyne.NewSize(lado, lado))
	f.tic.Colocar(x+(lado-iconos.TallaXXS)/2, (t.Height-iconos.TallaXXS)/2, iconos.TallaXXS)
	x += lado + tema.Espacio(3)
	m := ui.MedidaAvatar(ui.AvatarSM)
	f.avatar.Move(fyne.NewPos(x, (t.Height-m.Height)/2))
	f.avatar.Resize(m)
	x += m.Width + tema.Espacio(3)
	ui.Truncar(f.nombre, f.Persona.Nombre, tema.TextoBase, ui.Medio, max(t.Width-x-tema.Espacio(2), 0))
	ui.ColocarTexto(f.nombre, tema.TextoBase, x, (t.Height-tema.TextoBase.Linea)/2)
}

func (r *filaPersonaRenderer) Refresh() {
	f := r.f
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	f.fondo.FillColor = color.NRGBA{}
	if f.Encima() && !f.Inerte {
		f.fondo.FillColor = tema.Color("bg-subtle", v)
	}
	f.fondo.Refresh()
	relleno, borde := color.NRGBA{}, tema.Color("border", v)
	if f.Marcada {
		relleno, borde = tema.Color("primary", v), tema.Color("primary", v)
	}
	f.casilla.Pintar(relleno, borde)
	f.tic.Ver(f.Marcada)
	// `disabled:opacity-40`.
	c := tema.Color("text", v)
	if f.Inerte {
		c.A = uint8(float32(c.A) * 0.4)
	}
	f.nombre.Color = c
	f.nombre.Refresh()
	r.Layout(f.Size())
}

// FilaBuscarPersona es «Buscar X en Sytue One»: el globo de 16 en su disco de 28
// (`bg-bg-subtle rounded-full`), el rótulo `text-sm` y `px-2 py-2 gap-3 rounded-md`.
type FilaBuscarPersona struct {
	ui.Pulsable
	Rotulo string

	fondo, disco *canvas.Rectangle
	globo        *iconos.Dibujo
	texto        *canvas.Text
}

var _ fyne.Widget = (*FilaBuscarPersona)(nil)

// NuevaFilaBuscarPersona crea la fila.
func NuevaFilaBuscarPersona(rotulo string, alPulsar func()) *FilaBuscarPersona {
	f := &FilaBuscarPersona{Rotulo: rotulo}
	f.Nombre, f.AlPulsar = rotulo, alPulsar
	f.ExtendBaseWidget(f)
	return f
}

// CreateRenderer compone la fila.
func (f *FilaBuscarPersona) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	f.fondo = canvas.NewRectangle(color.NRGBA{})
	f.fondo.CornerRadius = tema.RadioMD
	f.disco = canvas.NewRectangle(tema.Color("bg-subtle", v))
	f.disco.CornerRadius = LadoMarcaCanal / 2
	f.globo = iconos.Trazo(iconos.Globo, iconos.TallaSM, "text-muted", v)
	f.texto = ui.Texto(f.Rotulo, tema.TextoSM, ui.Normal, tema.Color("text", v))
	r := &filaBuscarRenderer{f: f}
	r.Refresh()
	return r
}

type filaBuscarRenderer struct{ f *FilaBuscarPersona }

func (r *filaBuscarRenderer) MinSize() fyne.Size {
	return fyne.NewSize(0, 2*tema.Espacio(2)+LadoMarcaCanal)
}

func (r *filaBuscarRenderer) Objects() []fyne.CanvasObject {
	f := r.f
	return []fyne.CanvasObject{f.fondo, f.disco, f.globo.Objeto(), f.texto}
}
func (r *filaBuscarRenderer) Destroy() {}

func (r *filaBuscarRenderer) Layout(t fyne.Size) {
	f := r.f
	f.fondo.Resize(t)
	x := tema.Espacio(2)
	f.disco.Move(fyne.NewPos(x, (t.Height-LadoMarcaCanal)/2))
	f.disco.Resize(fyne.NewSize(LadoMarcaCanal, LadoMarcaCanal))
	f.globo.Colocar(x+(LadoMarcaCanal-iconos.TallaSM)/2, (t.Height-iconos.TallaSM)/2, iconos.TallaSM)
	x += LadoMarcaCanal + tema.Espacio(3)
	ui.Truncar(f.texto, f.Rotulo, tema.TextoSM, ui.Normal, max(t.Width-x-tema.Espacio(2), 0))
	ui.ColocarTexto(f.texto, tema.TextoSM, x, (t.Height-tema.TextoSM.Linea)/2)
}

func (r *filaBuscarRenderer) Refresh() {
	f := r.f
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	f.fondo.FillColor = color.NRGBA{}
	if f.Encima() && !f.Inerte {
		f.fondo.FillColor = tema.Color("bg-subtle", v)
	}
	f.fondo.Refresh()
	c := tema.Color("text", v)
	if f.Inerte {
		c.A = uint8(float32(c.A) * 0.6) // `disabled:opacity-60`
	}
	f.texto.Color = c
	f.texto.Refresh()
	r.Layout(f.Size())
}
