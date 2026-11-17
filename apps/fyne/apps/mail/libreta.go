// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package mail

import (
	"cmp"
	"context"
	"errors"
	"net/url"
	"regexp"
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
func (b *Buzon) ConLibreta(l Libreta, direccion string) *Buzon {
	b.Libreta, b.URLContactos = l, direccion
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

// # «Añadir a Contactos» / «Ver en Contactos» desde el remitente (MAIL-12)
//
// El menú «Más acciones» del lector guarda al remitente en la libreta por defecto con
// su nombre y su correo, o abre su ficha si ya estaba. Al abrir un hilo se le pregunta
// a Contacts si lo tiene; hasta que contesta se ofrece «Añadir», que mira antes de
// crear y no deja dos fichas. Es `MessageReader.tsx` + `pages/api/contacts.ts` de la
// web, y estas reglas, las de `lib/contactoRemitente.ts`.

// FichasContactos es lo que hace falta para dar de alta al remitente: buscarlo, la
// libreta por defecto y crear la ficha. Lo cumple *api.Contacts; un Buscador que no lo
// cumpla deja sin la entrada.
type FichasContactos interface {
	Buscador
	AddressBooks(ctx context.Context) ([]api.AddressBook, error)
	Create(ctx context.Context, libreta string, in api.ContactInput) (api.Contact, error)
}

var _ FichasContactos = (*api.Contacts)(nil)

// limiteFicha es cuántos contactos se miran para encontrar la dirección EXACTA: la
// búsqueda es por subcadena, y `ana@` también trae a `mariana@`. El del BFF.
const limiteFicha = 20

var sinRespuesta = map[string]bool{
	"noreply": true, "donotreply": true, "mailerdaemon": true, "postmaster": true,
	"bounce": true, "bounces": true,
}

// EsSinRespuesta dice si es un buzón que no lee (`no-reply@`, `mailer-daemon@`,
// `bounces+…@`): guardarlo en la libreta solo ensucia el autocompletado de Para.
func EsSinRespuesta(correo string) bool {
	c := strings.ToLower(strings.TrimSpace(correo))
	local, _, _ := strings.Cut(c, "@")
	base, _, _ := strings.Cut(local, "+")
	plana := strings.NewReplacer("-", "", "_", "", ".", "").Replace(base)
	if sinRespuesta[plana] {
		return true
	}
	// `noreply-avisos@`, `no_reply.facturas@`: el mismo buzón con un apellido.
	return reNoReplyConApellido.MatchString(base)
}

var reNoReplyConApellido = regexp.MustCompile(`^no[-_.]?reply[-_.]`)

// OfrecerContacto dice si el menú lleva la entrada: no en Spam (sería premiar al que se
// ha marcado), ni para un `no-reply`, ni para uno mismo —tu respuesta desde Enviados—.
func OfrecerContacto(correo string, enSpam bool, propias []string) bool {
	c := strings.ToLower(strings.TrimSpace(correo))
	if !strings.Contains(c, "@") || enSpam || EsSinRespuesta(c) {
		return false
	}
	return !slices.ContainsFunc(propias, func(p string) bool { return strings.ToLower(strings.TrimSpace(p)) == c })
}

// ContactoConCorreo es el id del contacto que tiene EXACTAMENTE esa dirección, o "".
func ContactoConCorreo(cs []api.Contact, correo string) string {
	c := strings.ToLower(strings.TrimSpace(correo))
	for _, k := range cs {
		if k.ID == "" {
			continue
		}
		for _, e := range k.Emails {
			if strings.ToLower(strings.TrimSpace(e.Email)) == c {
				return k.ID
			}
		}
	}
	return ""
}

// NombreDelContacto es con qué nombre se guarda: el del remitente, o su dirección.
func NombreDelContacto(nombre, correo string) string {
	if n := strings.TrimSpace(nombre); n != "" {
		return n
	}
	return strings.TrimSpace(correo)
}

// fichasContactos es el Buscador si sabe dar de alta, o nil.
func (b *Buzon) fichasContactos() FichasContactos {
	f, _ := b.Contactos.(FichasContactos)
	return f
}

// direccionesPropias son las del «De» y las de las cuentas externas.
func (b *Buzon) direccionesPropias() []string {
	out := b.opcionesDe()
	for _, d := range b.direccionesDeCuentas() {
		out = append(out, d)
	}
	return out
}

// ponerContactos engancha la entrada en el triaje, si hay Contacts que la atienda.
func (b *Buzon) ponerContactos(a *Acciones) {
	if b.fichasContactos() == nil {
		return
	}
	a.FichaDe = func(correo string) (string, bool) {
		if !OfrecerContacto(correo, b.enSpam(), b.direccionesPropias()) {
			return "", false
		}
		return b.fichas[strings.ToLower(strings.TrimSpace(correo))], true
	}
	a.AnadirAContactos = b.anadirAContactos
	a.VerEnContactos = b.verEnContactos
}

// preguntarFicha le pregunta a Contacts, al abrir un hilo, si ya tiene al remitente:
// con ficha, el menú dice «Ver en Contactos». Sin respuesta, se queda en «Añadir».
func (b *Buzon) preguntarFicha(c Conversacion) {
	a := b.P.Acciones
	correo := remitenteDe(c)
	clave := strings.ToLower(correo)
	if a.FichaDe == nil || correo == "" || b.fichas[clave] != "" {
		return
	}
	if _, ofrecer := a.FichaDe(correo); !ofrecer {
		return
	}
	f := b.fichasContactos()
	pedir(b, func(ctx context.Context) ([]api.Contact, error) {
		return f.Search(ctx, correo, limiteFicha)
	}, func(cs []api.Contact, err error) {
		if id := ContactoConCorreo(cs, correo); err == nil && id != "" {
			b.guardarFicha(clave, id)
		}
	})
}

func (b *Buzon) guardarFicha(clave, id string) {
	if b.fichas == nil {
		b.fichas = map[string]string{}
	}
	b.fichas[clave] = id
}

// anadirAContactos crea la ficha en la libreta por defecto, salvo que la dirección ya
// estuviera, y avisa con «Ver». Una segunda pulsación mientras va la primera no hace nada.
func (b *Buzon) anadirAContactos(nombre, correo string) {
	f := b.fichasContactos()
	clave := strings.ToLower(strings.TrimSpace(correo))
	if f == nil || clave == "" || b.anadiendoFicha[clave] {
		return
	}
	if b.anadiendoFicha == nil {
		b.anadiendoFicha = map[string]bool{}
	}
	b.anadiendoFicha[clave] = true
	pedir(b, func(ctx context.Context) (string, error) {
		cs, err := f.Search(ctx, correo, limiteFicha)
		if err != nil {
			return "", err
		}
		if id := ContactoConCorreo(cs, correo); id != "" {
			return id, nil
		}
		libretas, err := f.AddressBooks(ctx)
		if err != nil {
			return "", err
		}
		i := slices.IndexFunc(libretas, func(l api.AddressBook) bool { return l.IsDefault })
		if i < 0 && len(libretas) > 0 {
			i = 0 // el servicio crea la de por defecto al listar: no debería faltar
		}
		if i < 0 {
			return "", errSinLibreta
		}
		k, err := f.Create(ctx, libretas[i].ID, api.ContactInput{
			FormattedName: NombreDelContacto(nombre, correo),
			Emails:        []api.ContactEmail{{Email: strings.TrimSpace(correo), IsPrimary: true}},
		})
		return k.ID, err
	}, func(id string, err error) {
		delete(b.anadiendoFicha, clave)
		t := b.P.T
		if err != nil || id == "" {
			b.avisar(ui.AvisoPeligro, "reader.addToContactsFailed", nil)
			return
		}
		b.guardarFicha(clave, id)
		a := ui.NuevoAviso(ui.AvisoExito, t.T("reader.addedToContacts", nil), "")
		a.RotuloCerrar = t.T("ui.dismissNotification", nil)
		a.RotuloAccion = t.T("reader.viewContact", nil)
		a.AlPulsarAccion = func() { b.verEnContactos(id) }
		b.P.Notificar(a)
	})
}

var errSinLibreta = errors.New("mail: Contacts no tiene libreta por defecto")

// verEnContactos abre la ficha en Sytue Contacts por su esquema (`sytue-contacts:`,
// contactos/enlace.go), o en la web si la app no está instalada.
func (b *Buzon) verEnContactos(id string) {
	if b.Abridor == nil || id == "" {
		return
	}
	org := ""
	if b.Org != nil {
		org = b.Org()
	}
	_ = api.AbrirEn(b.Abridor, cmp.Or(b.Apex, api.ApexProd), "contacts", org, "/contacts/"+url.PathEscape(id))
}
