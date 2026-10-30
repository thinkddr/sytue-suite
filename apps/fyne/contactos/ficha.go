// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package contactos

import (
	"slices"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"

	"git.sytue.eu/sytue/one/apps/fyne/agenda"
	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// La ficha de un contacto (`ContactDetail.tsx`): cabecera de identidad y, debajo, la
// tarjeta con sus datos.
//
//	envoltorio  `mx-auto w-full max-w-3xl p-4 sm:p-6 lg:p-8`             hoja
//	cabecera    `flex flex-col gap-4 sm:flex-row sm:items-center`        hoja
//	avatar      `xl` (64)                                                hoja
//	nombre      `.sytue-h2`: `text-2xl` (24,5/31,5) en NEGRITA            hoja
//	organiz.    `mt-0.5 text-base text-text-muted`                       hoja
//	acciones    `flex gap-2`: ★ (31,5), Enviar correo, Editar, «⋯»        hoja
//	tarjeta     `mt-6`, `Card` con `CardTitle` y el `<dl>` SIN CardBody   hoja
//	fila de dl  `grid-cols-[6.5rem_1fr] gap-4 px-4 py-3`, y desde `sm:`  hoja
//	            `[8rem_1fr]` y `px-6`; separadas por `divide-y`
const (
	// AnchoFicha es el `max-w-3xl` con el rem de 14.
	AnchoFicha = 48 * tema.Rem
	// HuecoIdentidad es el `gap-4` entre avatar, nombre y acciones.
	HuecoIdentidad = 4 * 0.25 * tema.Rem
	// MargenOrganizacion es el `mt-0.5` de la organización.
	MargenOrganizacion = 0.5 * 0.25 * tema.Rem
	// MargenTarjetaFicha es el `mt-6`.
	MargenTarjetaFicha = 6 * 0.25 * tema.Rem
	// LadoEstrellaFicha es el `h-9 w-9` del botón de destacar.
	LadoEstrellaFicha = 9 * 0.25 * tema.Rem
	// HuecoCampo es el `gap-4` entre el rótulo y el valor de una fila del `<dl>`, y
	// RellenoVCampo su `py-3`.
	HuecoCampo    = 4 * 0.25 * tema.Rem
	RellenoVCampo = 3 * 0.25 * tema.Rem
	// HuecoValores es el `gap-1` entre dos correos o dos teléfonos, HuecoTipo el
	// `gap-x-2` entre un valor y su etiqueta («Móvil») y HuecoDirecciones el `gap-2`
	// entre dos direcciones.
	HuecoValores     = 0.25 * tema.Rem
	HuecoTipo        = 2 * 0.25 * tema.Rem
	HuecoDirecciones = 2 * 0.25 * tema.Rem
	// HuecoChips es el `gap-1.5` de las etiquetas.
	HuecoChips = 1.5 * 0.25 * tema.Rem
)

// RellenoFicha es el `p-4 sm:p-6 lg:p-8`. Los cortes son de VENTANA y con rem 16
// (`ui.CorteSM`); con Ventana a 0 se asume el escritorio ancho.
func RellenoFicha(ventana float32) float32 {
	switch {
	case ventana == 0 || ventana >= ui.CorteLG:
		return 8 * 0.25 * tema.Rem
	case ventana >= ui.CorteSM:
		return 6 * 0.25 * tema.Rem
	}
	return 4 * 0.25 * tema.Rem
}

// ColumnaRotulo y RellenoHCampo son el `grid-cols-[6.5rem_1fr]` y el `px-4` de una fila
// del `<dl>`, que desde `sm:` pasan a 8rem y `px-6`.
func ColumnaRotulo(ventana float32) float32 {
	if ventana == 0 || ventana >= ui.CorteSM {
		return 8 * tema.Rem
	}
	return 6.5 * tema.Rem
}

// RellenoHCampo es el `px-4 sm:px-6` de una fila del `<dl>`.
func RellenoHCampo(ventana float32) float32 {
	if ventana == 0 || ventana >= ui.CorteSM {
		return 6 * 0.25 * tema.Rem
	}
	return 4 * 0.25 * tema.Rem
}

// ficha es la columna de la derecha: el contacto elegido, o el estado vacío.
func (p *Pantalla) ficha() fyne.CanvasObject {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	if p.Seleccionado == nil {
		// «Selecciona un contacto», con la ilustración de la web y su acción de crear.
		x := ui.NuevoVacio(p.t("detail.emptyTitle", nil), p.t("detail.emptyDescription", nil))
		x.Ilustracion = iconos.Pintar(iconos.VacioContactos, v).Objeto()
		if !p.SinConexion && p.Activa != nil {
			x.Accion = ui.NuevoBoton(p.t("nav.newContact", nil), p.AlCrearContacto).
				Con(ui.Primario, ui.TallaSM).ConIcono(iconos.Mas, 0)
		}
		// El bloque ilustrado puede medir más que el área útil en horizontal.
		// Conservar su alto y desplazarlo mantiene alcanzable «Nuevo contacto».
		return container.NewVScroll(x)
	}
	c := *p.Seleccionado
	cuerpo := agenda.EnColumna(0,
		p.identidad(c, v),
		agenda.Espaciador(MargenTarjetaFicha),
		p.tarjetaDatos(c, v),
	)
	// `mx-auto w-full max-w-3xl p-*`: la ficha se centra y se acota.
	return container.NewVScroll(container.New(&acotado{Max: AnchoFicha, Relleno: RellenoFicha(p.Ventana)}, cuerpo))
}

// anchoTarjeta es lo que mide la tarjeta de la ficha: el hueco de la columna menos su
// relleno, sin pasar del `max-w-3xl`. Hace falta ANTES de colocar porque la nota se
// reparte en líneas y un párrafo necesita saber su ancho para decir su alto.
func (p *Pantalla) anchoTarjeta() float32 {
	ventana := p.Ventana
	if ventana == 0 {
		ventana = ui.CorteXL
	}
	hueco := ui.AnchoHoja(ventana, ui.AnchoMenu(false), 0)
	if ventana >= ui.Estrecho {
		hueco -= ui.AcotarAnchoPanel(ui.AnchoPanel(ventana), hueco)
	}
	hueco -= 2 * RellenoFicha(p.Ventana)
	return max(min(hueco, AnchoFicha), 0)
}

// anchoValor es la columna `1fr` de una fila del `<dl>`.
func (p *Pantalla) anchoValor() float32 {
	return max(p.anchoTarjeta()-2*tema.Borde-2*RellenoHCampo(p.Ventana)-ColumnaRotulo(p.Ventana)-HuecoCampo, 0)
}

// identidad es la cabecera: avatar, nombre y organización, y las acciones.
func (p *Pantalla) identidad(c api.Contact, v tema.Variante) fyne.CanvasObject {
	av := p.avatarDeFicha(c)
	nombre := ui.Texto(c.DisplayName(), ui.EscalonTitulo(2), ui.PesoTitulo(2), tema.Color("text", v))
	var org *canvas.Text
	if c.Organization != "" {
		org = ui.Texto(c.Organization, tema.TextoBase, ui.Normal, tema.Color("text-muted", v))
	}
	objs := []fyne.CanvasObject{av, nombre, p.accionesFicha(c)}
	if org != nil {
		objs = append(objs, org)
	}
	estrecha := p.Ventana > 0 && p.Ventana < ui.CorteSM
	return container.New(&identidad{
		nombre: nombre, org: org, completo: c.DisplayName(), orgCompleta: c.Organization, estrecha: estrecha,
	}, objs...)
}

// accionesFicha son las de la cabecera: dos frecuentes con nombre y el resto en «⋯».
// Cuatro botones de texto seguidos envolvían a dos líneas en cuanto la etiqueta
// crecía («Zusammenführen mit…»), que es por lo que la web las agrupa.
func (p *Pantalla) accionesFicha(c api.Contact) fyne.CanvasObject {
	var acciones []fyne.CanvasObject
	destacado := p.Destacados[api.CleanUID(c.UID)]
	if !p.SinConexion {
		rotulo := p.t("detail.star", nil)
		if destacado {
			rotulo = p.t("detail.unstar", nil)
		}
		acciones = append(acciones, nuevoBotonEstrella(destacado, rotulo, func() {
			if p.AlDestacar != nil {
				p.AlDestacar(c)
			}
		}))
	}
	correo := correoPrincipal(c)
	if correo != "" {
		acciones = append(acciones, ui.NuevoBoton(p.t("detail.sendEmail", nil), func() {
			if p.AlEnviarCorreo != nil {
				p.AlEnviarCorreo(correo)
			}
		}).Con(ui.Secundario, ui.TallaSM))
	}
	if !p.SinConexion {
		acciones = append(acciones, ui.NuevoBoton(p.t("common.edit", nil), func() {
			if p.AlEditar != nil {
				p.AlEditar(c)
			}
		}).Con(ui.Primario, ui.TallaSM))
	}
	mas := ui.NuevoBoton(p.t("detail.moreActions", nil), nil).Con(ui.Fantasma, ui.TallaSM)
	mas.SoloIcono, mas.Icono, mas.TamIcono = true, iconos.Puntos, 16
	mas.AlPulsar = func() {
		m := ui.NuevoMenu(p.elementosMas(c, correo)...)
		m.Rotulo = p.t("detail.moreActions", nil)
		m.Mostrar(mas)
	}
	acciones = append(acciones, mas)
	return agenda.EnFila(tema.Espacio(2), acciones...)
}

// elementosMas es el menú «⋯»: reunión y copiar correo si hay correo, exportar
// siempre, y combinar si la libreta tiene con quién.
func (p *Pantalla) elementosMas(c api.Contact, correo string) []fyne.CanvasObject {
	var e []fyne.CanvasObject
	accion := func(clave string, f func()) {
		e = append(e, ui.NuevoElementoMenu(p.t(clave, nil), iconos.Ninguno, f))
	}
	if correo != "" {
		accion("detail.meeting", func() {
			if p.AlReunion != nil {
				p.AlReunion(c, correo)
			}
		})
		accion("detail.copyEmail", func() {
			if p.AlCopiarCorreo != nil {
				p.AlCopiarCorreo(correo)
			}
		})
	}
	accion("detail.export", func() {
		if p.AlExportar != nil {
			p.AlExportar(c)
		}
	})
	if p.AlCombinar != nil && !p.SinConexion && p.Total > 1 {
		accion("detail.merge", func() { p.AlCombinar(c) })
	}
	return e
}

// tarjetaDatos es la `<Card>` con el `<dl>`: correo, teléfono, organización y, solo si
// los hay, dirección, cumpleaños y nota; y siempre las etiquetas. Una ficha con tres
// guiones de más no informa de nada, que es por lo que la web tampoco los pinta.
func (p *Pantalla) tarjetaDatos(c api.Contact, v tema.Variante) fyne.CanvasObject {
	campos := []fyne.CanvasObject{
		p.campo(p.t("field.email", nil), p.valoresTipados(correosTipados(c), "primary-text", "mailto:"), v, true),
		p.campo(p.t("field.phone", nil), p.valoresTipados(telefonosTipados(c), "text", "tel:"), v, false),
		p.campo(p.t("field.organization", nil), textoORaya(c.Organization), v, false),
	}
	if len(c.Addresses) > 0 {
		campos = append(campos, p.campo(p.t("field.address", nil), p.direcciones(c.Addresses), v, false))
	}
	if c.Birthday != nil {
		campos = append(campos, p.campo(p.t("field.birthday", nil),
			ui.NuevoRotulo(i18n.FormatDateLong(p.locale(), *c.Birthday), tema.TextoBase, ui.Normal, "text"), v, false))
	}
	if strings.TrimSpace(c.Note) != "" {
		campos = append(campos, p.campo(p.t("field.note", nil), p.nota(c.Note, v), v, false))
	}
	campos = append(campos, p.campo(p.t("field.labels", nil), p.etiquetasDe(c), v, false))
	t := ui.NuevaTarjeta(p.t("detail.infoTitle", nil), agenda.EnColumna(0, campos...))
	t.SinRelleno = true
	return t
}

// valorTipado es un correo o un teléfono con su etiqueta de vCard («cell», «work»).
type valorTipado struct {
	valor, tipo string
	principal   bool
}

// principalPrimero es `primaryFirst` de la web: el principal delante, y el resto en su
// orden. Es el que abre «Enviar correo».
func principalPrimero(vs []valorTipado) []valorTipado {
	fuera := make([]valorTipado, 0, len(vs))
	for _, x := range vs {
		if x.principal {
			fuera = append(fuera, x)
		}
	}
	for _, x := range vs {
		if !x.principal {
			fuera = append(fuera, x)
		}
	}
	return fuera
}

func correosTipados(c api.Contact) []valorTipado {
	vs := make([]valorTipado, 0, len(c.Emails))
	for _, e := range c.Emails {
		if e.Email != "" {
			vs = append(vs, valorTipado{e.Email, e.Type, e.IsPrimary})
		}
	}
	return principalPrimero(vs)
}

func telefonosTipados(c api.Contact) []valorTipado {
	vs := make([]valorTipado, 0, len(c.Phones))
	for _, t := range c.Phones {
		if t.Phone != "" {
			vs = append(vs, valorTipado{t.Phone, t.Type, t.IsPrimary})
		}
	}
	return principalPrimero(vs)
}

// correoPrincipal es el que abre «Enviar correo»: el marcado como principal, o el
// primero.
func correoPrincipal(c api.Contact) string {
	if vs := correosTipados(c); len(vs) > 0 {
		return vs[0].valor
	}
	return ""
}

// claveDeTipo es `TYPE_KEY` de la web: la etiqueta de vCard a su clave de catálogo.
var claveDeTipo = map[string]string{
	"home": "type.home", "work": "type.work", "cell": "type.mobile", "mobile": "type.mobile",
	"fax": "type.fax", "other": "type.other",
}

// rotuloDeTipo es `typeLabel`: un TYPE que no está en el conjunto del editor —otros
// clientes CardDAV escriben «iPhone», «school»— se enseña TAL CUAL, porque inventarle
// una traducción sería mentir sobre el dato. En una dirección, «otra» concuerda con
// «dirección».
func (p *Pantalla) rotuloDeTipo(tipo string, direccion bool) string {
	clave, ok := claveDeTipo[strings.ToLower(tipo)]
	if !ok {
		return tipo
	}
	if direccion && clave == "type.other" {
		clave = "type.otherAddress"
	}
	return p.t(clave, nil)
}

// valoresTipados es la columna de correos o de teléfonos: cada uno como enlace
// (`mailto:`/`tel:`, que abre el sistema, como el navegador) con su etiqueta al lado.
// Sin valores, la raya.
func (p *Pantalla) valoresTipados(vs []valorTipado, token, esquema string) fyne.CanvasObject {
	if len(vs) == 0 {
		return textoORaya("")
	}
	filas := make([]fyne.CanvasObject, 0, len(vs))
	for _, x := range vs {
		destino := esquema + x.valor
		enl := ui.NuevoEnlace(x.valor, tema.TextoBase, ui.Normal, token, func() {
			if p.AlAbrirEnlace != nil {
				p.AlAbrirEnlace(destino)
			}
		})
		filas = append(filas, container.New(filaTipada{},
			enl, ui.NuevoRotulo(p.rotuloDeTipo(x.tipo, false), tema.TextoXS, ui.Normal, "text-subtle")))
	}
	return agenda.EnColumna(HuecoValores, filas...)
}

// direcciones son las postales, cada una con su etiqueta encima y en líneas al estilo
// postal: calle / CP y ciudad / región / país (`addressLines` de la web).
func (p *Pantalla) direcciones(ds []api.ContactAddress) fyne.CanvasObject {
	bloques := make([]fyne.CanvasObject, 0, len(ds))
	for _, d := range ds {
		postales := lineasPostales(d)
		lineas := make([]fyne.CanvasObject, 0, len(postales)+1)
		lineas = append(lineas, ui.NuevoRotulo(p.rotuloDeTipo(d.Type, true), tema.TextoXS, ui.Normal, "text-subtle"))
		for _, l := range postales {
			lineas = append(lineas, ui.NuevoRotulo(l, tema.TextoBase, ui.Normal, "text"))
		}
		bloques = append(bloques, agenda.EnColumna(0, lineas...))
	}
	return agenda.EnColumna(HuecoDirecciones, bloques...)
}

// lineasPostales es `addressLines`: calle, «CP ciudad», región y país, sin las vacías.
func lineasPostales(d api.ContactAddress) []string {
	var fuera []string
	for _, l := range []string{
		d.Street,
		strings.TrimSpace(strings.Join(noVacias(d.PostalCode, d.Locality), " ")),
		d.Region, d.Country,
	} {
		if l = strings.TrimSpace(l); l != "" {
			fuera = append(fuera, l)
		}
	}
	return fuera
}

func noVacias(s ...string) []string {
	var fuera []string
	for _, x := range s {
		if x != "" {
			fuera = append(fuera, x)
		}
	}
	return fuera
}

// nota es la nota con sus saltos de línea (`whitespace-pre-line`): cada línea es un
// párrafo que se reparte al ancho de la columna, y una línea en blanco sigue ocupando
// su renglón.
func (p *Pantalla) nota(n string, v tema.Variante) fyne.CanvasObject {
	ancho := p.anchoValor()
	var lineas []fyne.CanvasObject
	for _, l := range strings.Split(strings.TrimSpace(n), "\n") {
		if strings.TrimSpace(l) == "" {
			lineas = append(lineas, agenda.Espaciador(tema.TextoBase.Linea))
			continue
		}
		lineas = append(lineas, ui.NuevoParrafo(l, tema.TextoBase, ui.Normal, "text", v).A(ancho))
	}
	return agenda.EnColumna(0, lineas...)
}

// etiquetasDe es `GroupChips`: las etiquetas del contacto como píldoras y el «Añadir»
// con las que le faltan. Sin red no hay ni aspas ni «Añadir».
func (p *Pantalla) etiquetasDe(c api.Contact) fyne.CanvasObject {
	var miembro, libres []api.ContactGroup
	for _, g := range p.Etiquetas {
		if slices.Contains(g.Members, c.UID) {
			miembro = append(miembro, g)
		} else {
			libres = append(libres, g)
		}
	}
	escribe := p.AlEtiquetar != nil && !p.SinConexion
	var hijos []fyne.CanvasObject
	if len(miembro) == 0 {
		hijos = append(hijos, ui.NuevoRotulo(p.t("detail.noLabels", nil), tema.TextoBase, ui.Normal, "text-subtle"))
	}
	for _, g := range miembro {
		var quitar func()
		if escribe {
			grupo := g
			quitar = func() { p.AlEtiquetar(grupo, c) }
		}
		hijos = append(hijos, nuevoChip(g.Name, p.t("detail.removeLabel", i18n.Vars{"label": g.Name}), quitar))
	}
	if escribe {
		anadir := ui.NuevoBoton(p.t("common.add", nil), nil).Con(ui.Fantasma, ui.TallaSM).ConIcono(iconos.Mas, 14)
		anadir.AlPulsar = func() {
			m := ui.NuevoMenu(p.menuAnadirEtiqueta(c, libres)...)
			m.Rotulo, m.AlInicio = p.t("detail.addLabel", nil), true
			m.Mostrar(anadir)
		}
		hijos = append(hijos, anadir)
	}
	// ponytail: las píldoras van en UNA fila; la web las deja envolver (`flex-wrap`).
	// Con las dos o tres etiquetas que lleva un contacto caben de sobra en los 502 de
	// la columna; con muchas, la última se sale. El relevo es un layout que reparta
	// por filas y declare su alto por ancho, como `ui.Parrafo`.
	return container.New(filaCentrada{Hueco: HuecoChips}, hijos...)
}

// menuAnadirEtiqueta son las etiquetas que el contacto aún no tiene, o el aviso de que
// no queda ninguna (o de que la libreta no tiene).
func (p *Pantalla) menuAnadirEtiqueta(c api.Contact, libres []api.ContactGroup) []fyne.CanvasObject {
	if len(libres) == 0 {
		clave := "detail.allLabelsAssigned"
		if len(p.Etiquetas) == 0 {
			clave = "detail.noLabelsAvailable"
		}
		margen := ui.Margen{Izq: tema.Espacio(3), Der: tema.Espacio(3), Arr: tema.Espacio(2), Abj: tema.Espacio(2)}
		return []fyne.CanvasObject{conRelleno(margen, ui.NuevoRotulo(p.t(clave, nil), tema.TextoSM, ui.Normal, "text-muted"))}
	}
	e := make([]fyne.CanvasObject, 0, len(libres))
	for _, g := range libres {
		grupo := g
		e = append(e, ui.NuevoElementoMenu(g.Name, iconos.Ninguno, func() { p.AlEtiquetar(grupo, c) }))
	}
	return e
}

// textoORaya es el valor en `text`, o la raya en `text-subtle` si no hay.
func textoORaya(s string) fyne.CanvasObject {
	if s == "" {
		return ui.NuevoRotulo("—", tema.TextoBase, ui.Normal, "text-subtle")
	}
	return ui.NuevoRotulo(s, tema.TextoBase, ui.Normal, "text")
}

// campo es una fila del `<dl>`: el rótulo en su columna fija y el valor en el resto,
// con el filete de `divide-y` encima de todas menos de la primera.
func (p *Pantalla) campo(rotulo string, valor fyne.CanvasObject, v tema.Variante, primero bool) fyne.CanvasObject {
	fila := container.New(&filaCampo{
		rotulo: ColumnaRotulo(p.Ventana), relleno: RellenoHCampo(p.Ventana),
	}, ui.NuevoRotulo(rotulo, tema.TextoSM, ui.Medio, "text-muted"), valor)
	if primero {
		return fila
	}
	return container.New(&filetArriba{}, fila, canvas.NewRectangle(tema.Color("border", v)))
}

// filaCampo es el `grid grid-cols-[8rem_1fr] items-start gap-4 px-6 py-3`.
type filaCampo struct{ rotulo, relleno float32 }

var _ fyne.Layout = (*filaCampo)(nil)

func (f *filaCampo) MinSize(objs []fyne.CanvasObject) fyne.Size {
	if len(objs) < 2 {
		return fyne.Size{}
	}
	dt, dd := objs[0].MinSize(), objs[1].MinSize()
	return fyne.NewSize(2*f.relleno+f.rotulo+HuecoCampo+dd.Width, 2*RellenoVCampo+max(dt.Height, dd.Height))
}

func (f *filaCampo) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	if len(objs) < 2 {
		return
	}
	objs[0].Move(fyne.NewPos(f.relleno, RellenoVCampo))
	objs[0].Resize(fyne.NewSize(f.rotulo, objs[0].MinSize().Height))
	x := f.relleno + f.rotulo + HuecoCampo
	objs[1].Move(fyne.NewPos(x, RellenoVCampo))
	objs[1].Resize(fyne.NewSize(max(t.Width-x-f.relleno, 0), objs[1].MinSize().Height))
}

// filaTipada es `TypedRow`: `flex items-baseline gap-x-2`, el valor y su etiqueta en
// la MISMA línea base. Con Figtree (ascendente 0,95 em y caja de 1,2 em) la línea base
// cae en `L/2 + 0,35 × tamaño` de cada caja de línea; la etiqueta, más pequeña, baja
// lo que difieren.
type filaTipada struct{}

var _ fyne.Layout = filaTipada{}

// DesfaseTipo es cuánto baja la etiqueta (`text-xs`) para compartir línea base con el
// valor (`text-base`): 3,24 px.
var DesfaseTipo = lineaBase(tema.TextoBase) - lineaBase(tema.TextoXS)

func lineaBase(e tema.Escalon) float32 { return e.Linea/2 + 0.35*e.Tam }

func (filaTipada) MinSize(objs []fyne.CanvasObject) fyne.Size {
	if len(objs) < 2 {
		return fyne.Size{}
	}
	a, b := objs[0].MinSize(), objs[1].MinSize()
	return fyne.NewSize(a.Width+HuecoTipo+b.Width, max(a.Height, DesfaseTipo+b.Height))
}

func (filaTipada) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	if len(objs) < 2 {
		return
	}
	a, b := objs[0].MinSize(), objs[1].MinSize()
	ancho := min(a.Width, t.Width)
	objs[0].Move(fyne.NewPos(0, 0))
	objs[0].Resize(fyne.NewSize(ancho, a.Height))
	objs[1].Move(fyne.NewPos(ancho+HuecoTipo, DesfaseTipo))
	objs[1].Resize(b)
}

// filaCentrada pone los hijos a su tamaño, en fila y centrados en vertical: el
// `flex items-center gap-*` de la web.
type filaCentrada struct{ Hueco float32 }

var _ fyne.Layout = filaCentrada{}

func (f filaCentrada) MinSize(objs []fyne.CanvasObject) fyne.Size {
	var ancho, alto float32
	for i, o := range objs {
		m := o.MinSize()
		if i > 0 {
			ancho += f.Hueco
		}
		ancho += m.Width
		alto = max(alto, m.Height)
	}
	return fyne.NewSize(ancho, alto)
}

func (f filaCentrada) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	var x float32
	for _, o := range objs {
		m := o.MinSize()
		o.Move(fyne.NewPos(x, (t.Height-m.Height)/2))
		o.Resize(m)
		x += m.Width + f.Hueco
	}
}

// filetArriba pone el segundo hijo como filete de un píxel ARRIBA: el `divide-y`.
type filetArriba struct{}

var _ fyne.Layout = filetArriba{}

func (filetArriba) MinSize(objs []fyne.CanvasObject) fyne.Size {
	if len(objs) == 0 {
		return fyne.Size{}
	}
	m := objs[0].MinSize()
	return fyne.NewSize(m.Width, m.Height+tema.Borde)
}

func (filetArriba) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	if len(objs) == 0 {
		return
	}
	if len(objs) > 1 {
		objs[1].Move(fyne.NewPos(0, 0))
		objs[1].Resize(fyne.NewSize(t.Width, tema.Borde))
	}
	objs[0].Move(fyne.NewPos(0, tema.Borde))
	objs[0].Resize(fyne.NewSize(t.Width, max(t.Height-tema.Borde, 0)))
}

// identidad es el `flex flex-col gap-4 sm:flex-row sm:items-center` de la cabecera de
// la ficha: avatar, bloque de texto que se lleva lo que sobra (y TRUNCA), y acciones
// que no encogen. Por debajo de `sm:` se apilan.
//
// Objetos: avatar, nombre, acciones y, si la hay, organización.
type identidad struct {
	nombre, org           *canvas.Text
	completo, orgCompleta string // lo que se trunca: el `Text` ya puede llevar la elipsis
	estrecha              bool
}

var _ fyne.Layout = (*identidad)(nil)

func (l *identidad) altoTextos() float32 {
	alto := ui.EscalonTitulo(2).Linea
	if l.org != nil {
		alto += MargenOrganizacion + tema.TextoBase.Linea
	}
	return alto
}

func (l *identidad) MinSize(objs []fyne.CanvasObject) fyne.Size {
	if len(objs) < 3 {
		return fyne.Size{}
	}
	av, ac := objs[0].MinSize(), objs[2].MinSize()
	if l.estrecha {
		return fyne.NewSize(max(av.Width, ac.Width), av.Height+l.altoTextos()+ac.Height+2*HuecoIdentidad)
	}
	return fyne.NewSize(av.Width+ac.Width+2*HuecoIdentidad, max(av.Height, ac.Height, l.altoTextos()))
}

func (l *identidad) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	if len(objs) < 3 {
		return
	}
	av, acciones := objs[0], objs[2]
	mAv, mAc := av.MinSize(), acciones.MinSize()
	colocarTextos := func(x, y, cabe float32) {
		ui.Truncar(l.nombre, l.completo, ui.EscalonTitulo(2), ui.PesoTitulo(2), cabe)
		ui.ColocarTexto(l.nombre, ui.EscalonTitulo(2), x, y)
		if l.org != nil {
			ui.Truncar(l.org, l.orgCompleta, tema.TextoBase, ui.Normal, cabe)
			ui.ColocarTexto(l.org, tema.TextoBase, x, y+ui.EscalonTitulo(2).Linea+MargenOrganizacion)
		}
	}
	if l.estrecha {
		av.Move(fyne.NewPos(0, 0))
		av.Resize(mAv)
		y := mAv.Height + HuecoIdentidad
		colocarTextos(0, y, t.Width)
		acciones.Move(fyne.NewPos(0, y+l.altoTextos()+HuecoIdentidad))
		acciones.Resize(mAc)
		return
	}
	alto := max(mAv.Height, mAc.Height, l.altoTextos())
	av.Move(fyne.NewPos(0, (alto-mAv.Height)/2))
	av.Resize(mAv)
	acciones.Move(fyne.NewPos(t.Width-mAc.Width, (alto-mAc.Height)/2))
	acciones.Resize(mAc)
	x := mAv.Width + HuecoIdentidad
	colocarTextos(x, (alto-l.altoTextos())/2, max(t.Width-mAc.Width-HuecoIdentidad-x, 0))
}

// acotado es el `mx-auto w-full max-w-3xl p-*`: el contenido se centra y no pasa del
// ancho máximo.
type acotado struct {
	Max     float32
	Relleno float32
}

var _ fyne.Layout = (*acotado)(nil)

func (a *acotado) MinSize(objs []fyne.CanvasObject) fyne.Size {
	var ancho, alto float32
	for _, o := range objs {
		m := o.MinSize()
		ancho, alto = max(ancho, m.Width), alto+m.Height
	}
	return fyne.NewSize(ancho+2*a.Relleno, alto+2*a.Relleno)
}

func (a *acotado) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	util := min(max(t.Width-2*a.Relleno, 0), a.Max)
	x := (t.Width - util) / 2
	y := a.Relleno
	for _, o := range objs {
		h := o.MinSize().Height
		o.Move(fyne.NewPos(x, y))
		o.Resize(fyne.NewSize(util, h))
		y += h
	}
}

// avatarDeFicha es `ContactPhoto`: con red, el AvatarPicker (colores predefinidos,
// subir una foto, quitarla); sin ella, el avatar a secas.
func (p *Pantalla) avatarDeFicha(c api.Contact) fyne.CanvasObject {
	av := ui.NuevoAvatar(c.DisplayName()).Con(ui.AvatarXL)
	av.Foto = p.Foto
	if p.SinConexion {
		return av
	}
	s := ui.NuevoSelectorAvatar(av)
	p.rotularSelector(s)
	s.TieneFoto = p.Foto != nil
	s.AlElegirColor = func(token string) {
		if p.AlFotoColor != nil {
			p.AlFotoColor(c, token)
		}
	}
	if p.AlSubirFoto != nil && p.PuedeImportar {
		s.AlSubir = func() { p.AlSubirFoto(c) }
	}
	s.AlQuitar = func() {
		if p.AlQuitarFoto != nil {
			p.AlQuitarFoto(c)
		}
	}
	return s
}

// rotularSelector pone los rótulos de `labels` de AvatarPicker.
func (p *Pantalla) rotularSelector(s *ui.SelectorAvatar) {
	s.RotuloCambiar, s.RotuloAnadir = p.t("photo.change", nil), p.t("photo.add", nil)
	s.RotuloColores, s.RotuloSubir, s.RotuloQuitar = p.t("photo.presets", nil), p.t("photo.upload", nil), p.t("photo.remove", nil)
}
