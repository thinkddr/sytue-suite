// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package mail

import (
	"context"
	"slices"
	"strings"

	"golang.org/x/text/collate"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/buscar"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// # «Elegir de Contactos» (`LibretaModal.tsx` + `/api/libreta`)
//
// El icono de Contactos de Para, CC y CCO abre la libreta entera —solo los contactos
// con correo—, se filtra por nombre o correo y se marcan varias personas, que entran
// en ESE campo con el nombre delante del correo. La libreta se pide una vez por
// redacción: filtrar es cosa de aquí.

// Libreta es lo que el diálogo le pide a Contacts. Lo cumple *api.Contacts.
type Libreta interface {
	All(ctx context.Context, limite, offset int) ([]api.Contact, int, error)
}

var _ Libreta = (*api.Contacts)(nil)

// PaginaLibreta es el `limit` del BFF: una página de quinientos, sin recorrer más.
const PaginaLibreta = 500

// EntradaLibreta es un contacto con sus correos, el principal primero y sin repetir.
type EntradaLibreta struct {
	Nombre  string
	Correos []string
}

// EntradasLibreta es lo que el BFF hace con `/contacts/all`.
func EntradasLibreta(cs []api.Contact) []EntradaLibreta {
	var out []EntradaLibreta
	for _, c := range cs {
		var correos []string
		for _, s := range SugerenciasDe([]api.Contact{c}) {
			correos = append(correos, s.Correo)
		}
		if len(correos) > 0 {
			out = append(out, EntradaLibreta{Nombre: strings.TrimSpace(c.FormattedName), Correos: correos})
		}
	}
	return out
}

// FilasLibreta es `filas`: una por correo, filtradas por nombre o correo (sin
// mayúsculas) y en el orden del idioma por nombre, o por correo si no lo hay.
func FilasLibreta(l i18n.Locale, libreta []EntradaLibreta, q string) []Sugerencia {
	texto := buscar.Plegar(strings.TrimSpace(q))
	var out []Sugerencia
	for _, c := range libreta {
		for _, e := range c.Correos {
			if texto == "" || strings.Contains(buscar.Plegar(c.Nombre), texto) || strings.Contains(buscar.Plegar(e), texto) {
				out = append(out, Sugerencia{Nombre: c.Nombre, Correo: e})
			}
		}
	}
	col := collate.New(l.Tag(), collate.Loose)
	clave := func(s Sugerencia) string {
		if s.Nombre != "" {
			return s.Nombre
		}
		return s.Correo
	}
	slices.SortStableFunc(out, func(a, b Sugerencia) int { return col.CompareString(clave(a), clave(b)) })
	return out
}

// AnadirDeLibreta es `anyadirDeLibreta`: lo elegido va detrás de lo que hubiera, con
// su coma, listo para seguir escribiendo.
func AnadirDeLibreta(previo string, elegidos []Sugerencia) string {
	partes := make([]string, 0, len(elegidos))
	for _, s := range elegidos {
		partes = append(partes, s.Ficha())
	}
	texto := strings.Join(partes, ", ")
	base := strings.TrimRight(strings.TrimSpace(previo), ",; ")
	if base == "" {
		return texto + ", "
	}
	return base + ", " + texto + ", "
}

// ConLibreta engancha «Elegir de Contactos». `url` es la de Sytue Contacts, para el
// botón «Abrir Sytue Contacts» de una libreta vacía.
func (b *Buzon) ConLibreta(l Libreta, url string) *Buzon {
	b.Libreta, b.URLContactos = l, url
	return b
}

type dialogoLibreta struct {
	b         *Buzon
	rd        *redaccion
	campo     *CampoDirecciones
	d         *ui.Dialogo
	caja      *fyne.Container
	ancho     float32
	buscar    *ui.Entrada
	anadir    *ui.Boton
	marcados  map[string]Sugerencia
	orden     []string // el orden en que se marcaron
	altoLista float32
}

// verLibreta abre la libreta para ese campo del redactor.
func (b *Buzon) verLibreta(rd *redaccion, campo *CampoDirecciones) *dialogoLibreta {
	lienzo := b.P.Lienzo()
	if lienzo == nil || b.Libreta == nil {
		return nil
	}
	t := b.P.T
	g := &dialogoLibreta{
		b: b, rd: rd, campo: campo, ancho: AnchoCuerpoDialogo(ui.DialogoMD, lienzo.Size().Width),
		marcados: map[string]Sugerencia{}, altoLista: 0.5 * lienzo.Size().Height, // `max-h-[50dvh]`
	}
	g.buscar = ui.NuevaEntrada(t.T("compose.addressBookSearch", nil))
	g.buscar.Icono = iconos.Buscar
	g.buscar.AlCambiar(func(string) { g.pintar() })
	g.caja = container.New(ui.Columna{Hueco: tema.Espacio(3)})
	cancelar := ui.NuevoBoton(t.T("common.cancel", nil), nil).Con(ui.Fantasma, ui.TallaMD)
	g.anadir = ui.NuevoBoton("", g.elegir).Con(ui.Primario, ui.TallaMD)
	g.d = ui.NuevoDialogo(t.T("compose.addressBookTitle", nil), g.caja).
		Con(ui.DialogoMD, container.New(ui.Fila{Hueco: ui.HuecoPieDialogo}, cancelar, g.anadir))
	cancelar.AlPulsar = g.d.Cerrar
	g.d.RotuloCerrar = t.T("ui.close", nil)
	g.d.Foco = g.buscar
	g.pintar()
	g.d.Mostrar(lienzo)
	rd.libretaAbierta = g
	if rd.libreta == nil && !rd.falloLibreta {
		pedir(b, func(ctx context.Context) ([]api.Contact, error) {
			cs, _, err := b.Libreta.All(ctx, PaginaLibreta, 0)
			return cs, err
		}, func(cs []api.Contact, err error) {
			if err != nil {
				rd.falloLibreta = true
			} else {
				rd.libreta = EntradasLibreta(cs)
				if rd.libreta == nil {
					rd.libreta = []EntradaLibreta{}
				}
			}
			g.pintar()
		})
	}
	return g
}

func (g *dialogoLibreta) alternar(s Sugerencia) {
	k := strings.ToLower(s.Correo)
	if _, esta := g.marcados[k]; esta {
		delete(g.marcados, k)
		g.orden = slices.DeleteFunc(g.orden, func(x string) bool { return x == k })
	} else {
		g.marcados[k] = s
		g.orden = append(g.orden, k)
	}
	g.pintar()
}

func (g *dialogoLibreta) elegir() {
	if len(g.orden) == 0 {
		return
	}
	elegidos := make([]Sugerencia, 0, len(g.orden))
	for _, k := range g.orden {
		elegidos = append(elegidos, g.marcados[k])
	}
	g.campo.Poner(AnadirDeLibreta(g.campo.Valor, elegidos))
	if g.campo.AlCambiar != nil {
		g.campo.AlCambiar(g.campo.Valor)
	}
	g.d.Cerrar()
}

func (g *dialogoLibreta) pintar() {
	t, l := g.b.P.T, g.b.P.Locale
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	g.anadir.Texto = t.T("compose.addressBookAdd", i18n.Vars{"count": len(g.orden)})
	g.anadir.Inerte = len(g.orden) == 0
	g.anadir.Refresh()

	var dentro fyne.CanvasObject
	switch {
	case g.rd.falloLibreta:
		dentro = ui.NuevoParrafo(t.T("compose.addressBookLoadFailed", nil), tema.TextoSM, ui.Normal, "danger-text", v).A(g.ancho)
	case g.rd.libreta == nil:
		rueda := ui.NuevaRueda(22, "text-subtle", v)
		dentro = container.New(ui.Margen{Arr: tema.Espacio(8), Abj: tema.Espacio(8)}, container.NewCenter(rueda))
	case len(g.rd.libreta) == 0:
		vacio := ui.NuevoVacio(t.T("compose.addressBookTitle", nil), t.T("compose.addressBookEmpty", nil))
		if g.b.Abridor != nil && g.b.URLContactos != "" {
			vacio.Accion = ui.NuevoBoton(t.T("compose.addressBookOpenContacts", nil), func() {
				_ = api.AbrirURL(g.b.Abridor, "contacts", g.b.URLContactos)
			}).Con(ui.Secundario, ui.TallaSM)
		}
		dentro = vacio
	default:
		filas := FilasLibreta(l, g.rd.libreta, g.buscar.Texto())
		if len(filas) == 0 {
			dentro = ui.NuevoVacio(t.T("compose.addressBookNoMatch", i18n.Vars{"query": g.buscar.Texto()}), "")
			break
		}
		objs := make([]fyne.CanvasObject, 0, 2*len(filas))
		alto := float32(0)
		for i, s := range filas {
			if i > 0 {
				objs = append(objs, fileteLibreta(v))
				alto += tema.Borde
			}
			_, marcado := g.marcados[strings.ToLower(s.Correo)]
			f := nuevaFilaLibreta(s, marcado, func() { g.alternar(s) })
			objs = append(objs, f)
			alto += f.MinSize().Height
		}
		lista := container.NewVScroll(container.New(ui.Columna{}, objs...))
		caja := ui.NuevaCaja(tema.Color("surface-raised", v), tema.Color("border", v), tema.RadioMD)
		dentro = container.New(&enCaja{caja: caja}, append(caja.Objetos(),
			container.New(fijo{alto: min(alto, g.altoLista-2*tema.Borde)}, lista))...)
	}
	g.caja.Objects = []fyne.CanvasObject{g.buscar, dentro}
	g.caja.Refresh()
	if g.d != nil {
		g.d.Recolocar()
	}
}

// fileteLibreta es el `divide-border/60`.
func fileteLibreta(v tema.Variante) fyne.CanvasObject {
	c := tema.Color("border", v)
	c.A = uint8(float32(c.A) * 0.6)
	r := canvas.NewRectangle(c)
	r.SetMinSize(fyne.NewSize(0, tema.Borde))
	return r
}

// filaLibreta es el `<label>` de una persona: `flex items-center gap-3 px-3 py-2`, la
// casilla de 16, el avatar sm y el nombre (`text-sm font-medium`) con el correo
// debajo (`text-xs text-muted`); toda la fila marca y desmarca.
type filaLibreta struct {
	ui.Pulsable
	s       Sugerencia
	casilla *ui.Eleccion
	avatar  *ui.Avatar
}

const (
	margenXLibreta float32 = 3 * 0.25 * tema.Rem
	margenYLibreta float32 = 2 * 0.25 * tema.Rem
	huecoLibreta   float32 = 3 * 0.25 * tema.Rem
	casillaLibreta float32 = 16
)

func nuevaFilaLibreta(s Sugerencia, marcado bool, alternar func()) *filaLibreta {
	f := &filaLibreta{s: s}
	f.Nombre, f.AlPulsar = s.Ficha(), alternar
	f.casilla = ui.NuevaCasilla("", func(bool) { alternar() })
	f.casilla.Marcada = marcado
	nombre := s.Nombre
	if nombre == "" {
		nombre = s.Correo
	}
	f.avatar = ui.NuevoAvatar(nombre).Con(ui.AvatarSM)
	f.ExtendBaseWidget(f)
	return f
}

// MinSize: el `py-2` alrededor de lo más alto, que es el avatar de 32.
func (f *filaLibreta) MinSize() fyne.Size {
	texto := tema.TextoSM.Linea
	if f.s.Nombre != "" {
		texto += tema.TextoXS.Linea
	}
	return fyne.NewSize(0, 2*margenYLibreta+max(ui.MedidaAvatar(ui.AvatarSM).Height, texto))
}

func (f *filaLibreta) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &filaLibretaRenderer{f: f, fondo: canvas.NewRectangle(tema.Color("bg-subtle", v))}
	arriba := f.s.Nombre
	if arriba == "" {
		arriba = f.s.Correo
	}
	r.arriba = textoRecortado(arriba, tema.TextoSM, ui.Medio, "text", v)
	if f.s.Nombre != "" {
		r.abajo = textoRecortado(f.s.Correo, tema.TextoXS, ui.Normal, "text-muted", v)
	}
	r.Refresh()
	return r
}

type filaLibretaRenderer struct {
	f             *filaLibreta
	fondo         *canvas.Rectangle
	arriba, abajo fyne.CanvasObject
}

func (r *filaLibretaRenderer) MinSize() fyne.Size { return r.f.MinSize() }
func (r *filaLibretaRenderer) Objects() []fyne.CanvasObject {
	objs := []fyne.CanvasObject{r.fondo, r.f.casilla, r.f.avatar, r.arriba}
	if r.abajo != nil {
		objs = append(objs, r.abajo)
	}
	return objs
}
func (r *filaLibretaRenderer) Destroy() {}

func (r *filaLibretaRenderer) Layout(t fyne.Size) {
	r.fondo.Resize(t)
	x := margenXLibreta
	mc := r.f.casilla.MinSize()
	r.f.casilla.Move(fyne.NewPos(x, (t.Height-mc.Height)/2))
	r.f.casilla.Resize(mc)
	x += casillaLibreta + huecoLibreta
	ma := ui.MedidaAvatar(ui.AvatarSM)
	r.f.avatar.Move(fyne.NewPos(x, (t.Height-ma.Height)/2))
	r.f.avatar.Resize(ma)
	x += ma.Width + huecoLibreta
	texto := tema.TextoSM.Linea
	if r.abajo != nil {
		texto += tema.TextoXS.Linea
	}
	y := (t.Height - texto) / 2
	w := max(t.Width-x-margenXLibreta, 0)
	r.arriba.Move(fyne.NewPos(x, y))
	r.arriba.Resize(fyne.NewSize(w, tema.TextoSM.Linea))
	if r.abajo != nil {
		r.abajo.Move(fyne.NewPos(x, y+tema.TextoSM.Linea))
		r.abajo.Resize(fyne.NewSize(w, tema.TextoXS.Linea))
	}
}

// Refresh: `hover:bg-bg-subtle`.
func (r *filaLibretaRenderer) Refresh() {
	r.fondo.Hidden = !r.f.Encima()
	r.fondo.Refresh()
	r.f.casilla.Refresh()
	r.Layout(r.f.Size())
}
