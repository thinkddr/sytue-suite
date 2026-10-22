// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package contactos

import (
	"image"
	"regexp"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// El formulario de crear y editar un contacto (`ContactModal.tsx`): un diálogo `md`
// con la vista previa arriba, el nombre, los correos y los teléfonos con su etiqueta y
// su principal, las direcciones, el cumpleaños, la organización y la nota.
//
//	cuerpo      `flex flex-col gap-4`                                        hoja
//	previa      `flex items-center gap-3 border-b pb-4`, avatar `lg` (48)    hoja
//	            nombre `text-base font-semibold`, debajo la organización
//	            `text-sm text-muted` o «Vista previa» en `text-subtle`
//	fieldset    `flex flex-col gap-2`, leyenda `text-sm font-semibold mb-1`  hoja
//	fila        `flex flex-wrap items-center gap-2`: etiqueta `w-28`, valor  hoja
//	            `grow basis-48`, «Principal» si hay más de una, y el aspa
//	            (`ghost sm iconOnly`, 16)
//	dirección   `rounded-md border p-3 flex-col gap-2`, dos rejillas de dos  hoja
//	pie         `flex w-full justify-between`: «Eliminar» (`danger`) a la     hoja
//	            izquierda al editar; «Cancelar» y «Guardar» a la derecha
//
// Medido en producción: el diálogo es de 560 y su pie de 60,5, «Cancelar» 79 × 31,5,
// «Guardar» 71 × 31,5, el aspa de quitar 28 × 28 y «Añadir correo» 113 × 28.
var (
	// HuecoFormulario es el `gap-4` del cuerpo.
	HuecoFormulario = tema.Espacio(4)
	// HuecoValores es el `gap-2` de un fieldset y de cada una de sus filas.
	HuecoFieldset = tema.Espacio(2)
	// MargenLeyenda es el `mb-1` de la leyenda: el `<legend>` no es hijo del flex del
	// fieldset, así que entre él y la primera fila va su margen y NO el `gap-2`.
	MargenLeyenda = tema.Espacio(1)
	// AnchoTipo es el `w-28` del selector de etiqueta (en `w-24`, «Trabajo» no cabía).
	AnchoTipo = tema.Espacio(28)
	// BaseValor es el `basis-48` del campo del valor.
	BaseValor = tema.Espacio(48)
	// HuecoPrevia y RellenoPrevia son el `gap-3` y el `pb-4` de la vista previa.
	HuecoPrevia   = tema.Espacio(3)
	RellenoPrevia = tema.Espacio(4)
	// RellenoDireccion es el `p-3` de la caja de una dirección.
	RellenoDireccion = tema.Espacio(3)
)

// FilasNota es el `rows={3}` de la nota.
const FilasNota = 3

// El asistente (`AsistenteFicha.tsx`): un área de `rows={3}` y, debajo y a la derecha,
// «Rellenar» (`secondary sm`, con el icono «ai» de 16), separados por `gap-2`. Manda
// como mucho `MAX` (4000) caracteres: una firma, no un hilo de correo entero.
const (
	FilasAsistente = 3
	MaxAsistente   = 4000
	IconoAsistente = 16
)

// HuecoAsistente es su `gap-2`.
var HuecoAsistente = tema.Espacio(2)

// tipoOpcion es una etiqueta de correo, teléfono o dirección: el VALOR es el de la
// vCard (viaja al servicio y no se traduce) y la clave, con la que se pinta.
type tipoOpcion struct{ valor, clave string }

var (
	tiposCorreo = []tipoOpcion{{"home", "type.home"}, {"work", "type.work"}, {"other", "type.other"}}
	tiposTel    = []tipoOpcion{
		{"cell", "type.mobile"},
		{"home", "type.home"},
		{"work", "type.work"},
		{"fax", "type.fax"},
		{"other", "type.other"},
	}
	// «Otra» y no «Otro»: en español el adjetivo concuerda con «dirección».
	tiposDireccion = []tipoOpcion{{"home", "type.home"}, {"work", "type.work"}, {"other", "type.otherAddress"}}
)

// indiceTipo es `normalizeType`: la posición de la etiqueta en la lista, o la de
// «other» si la vCard trae una que el editor no ofrece.
func indiceTipo(tipo string, ops []tipoOpcion) int {
	for _, t := range []string{strings.ToLower(tipo), "other"} {
		for i, o := range ops {
			if o.valor == t {
				return i
			}
		}
	}
	return 0
}

// correoValido es la comprobación mínima de forma de la web (`/^\S+@\S+\.\S+$/`); la
// de verdad la hace el servicio.
var correoValido = regexp.MustCompile(`^\S+@\S+\.\S+$`)

type filaValor struct {
	tipo  *ui.Selector
	valor *ui.Entrada
}

type filaDireccion struct {
	tipo                            *ui.Selector
	calle, ciudad, region, cp, pais *ui.Entrada
}

// formulario es el estado del diálogo mientras está abierto.
type formulario struct {
	p      *Pantalla
	lienzo fyne.Canvas
	// contacto es el que se edita; nil es un alta.
	contacto *api.Contact
	d        *ui.Dialogo
	cuerpo   *fyne.Container

	nombre                             *ui.Entrada
	correos, telefonos                 []*filaValor
	principalCorreo, principalTelefono int
	direcciones                        []*filaDireccion
	cumple, org                        *ui.Entrada
	nota                               *ui.AreaTexto

	errNombre        string
	errCorreo        int // la fila del primer correo mal escrito, o -1
	confirmarBorrado bool

	avatar   *ui.Avatar
	selector *ui.SelectorAvatar
	// La foto: la que ya tiene (al editar), la elegida en el formulario —que se sube al
	// guardar, porque al crear aún no hay UID— y si se ha quitado.
	fotoActual  image.Image
	fotoNueva   string // data URI
	fotoQuitada bool
	// subirFoto abre el selector de imágenes; nil: «Subir una foto…» apagada.
	subirFoto                      func(listo func(dataURI string))
	previaNombre, previaOrg        *canvas.Text
	textoOrg                       string // lo que va en previaOrg, sin recortar
	previaCont                     *fyne.Container
	borrar, cancelar, guardarBoton *ui.Boton

	// alGuardar y alBorrar hablan con el servicio. Reciben `listo`, que se llama en
	// el hilo de la interfaz con el resultado.
	alGuardar func(in api.ContactInput, listo func(error))
	alBorrar  func(listo func(error))

	// El asistente: lo pegado, «Rellenar» y si no ha encontrado nada. alAsistir pide la
	// ficha al servicio de IA y llama a `listo` en el hilo de la interfaz; nil, no hay
	// asistente.
	ia        *ui.AreaTexto
	rellenar  *ui.Boton
	errIA     bool
	alAsistir func(texto string, listo func(api.FichaPropuesta, bool))
}

// nuevoFormulario prepara el diálogo para un contacto (nil: uno nuevo), con una fila
// vacía de correo y otra de teléfono si no tiene ninguno, como la web.
func (p *Pantalla) nuevoFormulario(c *api.Contact) *formulario {
	f := &formulario{p: p, contacto: c, errCorreo: -1}
	f.nombre = ui.NuevaEntrada(p.t("form.fullNamePlaceholder", nil))
	f.org = ui.NuevaEntrada(p.t("form.organizationPlaceholder", nil))
	f.cumple = ui.CampoFecha(ui.CampoMD)
	f.nota = ui.NuevaAreaTexto(p.t("form.notePlaceholder", nil))
	f.nota.Filas = FilasNota

	var correos []api.ContactEmail
	var tels []api.ContactPhone
	if c != nil {
		f.nombre.PonerTexto(c.FormattedName)
		f.org.PonerTexto(c.Organization)
		f.nota.PonerTexto(c.Note)
		if c.Birthday != nil {
			f.cumple.PonerTexto(ui.EscribirFecha(*c.Birthday))
		}
		correos, tels = c.Emails, c.Phones
		for _, a := range c.Addresses {
			f.direcciones = append(f.direcciones, f.nuevaDireccion(a))
		}
	}
	for i, e := range correos {
		f.correos = append(f.correos, f.nuevaFila(tiposCorreo, indiceTipo(e.Type, tiposCorreo),
			e.Email, "form.emailPlaceholder"))
		if e.IsPrimary {
			f.principalCorreo = i
		}
	}
	for i, t := range tels {
		f.telefonos = append(f.telefonos, f.nuevaFila(tiposTel, indiceTipo(t.Type, tiposTel),
			t.Phone, "form.phonePlaceholder"))
		if t.IsPrimary {
			f.principalTelefono = i
		}
	}
	if len(f.correos) == 0 {
		f.correos = []*filaValor{f.nuevaFila(tiposCorreo, 0, "", "form.emailPlaceholder")}
	}
	if len(f.telefonos) == 0 {
		f.telefonos = []*filaValor{f.nuevaFila(tiposTel, 0, "", "form.phonePlaceholder")}
	}
	// La vista previa sigue a lo que se escribe.
	f.nombre.AlCambiar(func(string) {
		if f.errNombre != "" {
			f.errNombre = ""
			f.rehacer()
		}
		f.pintarPrevia()
	})
	f.org.AlCambiar(func(string) { f.pintarPrevia() })
	f.cumple.AlCambiar(func(string) {
		if f.cumple.Invalida {
			f.cumple.Invalida = false
			f.cumple.Refresh()
		}
	})
	return f
}

func (f *formulario) nuevaFila(ops []tipoOpcion, tipo int, valor, marca string) *filaValor {
	x := &filaValor{
		tipo:  ui.NuevoSelector(f.rotulos(ops), nil),
		valor: ui.NuevaEntrada(f.p.t(marca, nil)),
	}
	x.tipo.Elegida, x.tipo.Ancho = tipo, AnchoTipo
	x.valor.PonerTexto(valor)
	x.valor.AlCambiar(func(string) {
		if f.errCorreo >= 0 {
			f.errCorreo = -1
			f.rehacer()
		}
	})
	return x
}

func (f *formulario) nuevaDireccion(a api.ContactAddress) *filaDireccion {
	campo := func(marca, valor string) *ui.Entrada {
		e := ui.NuevaEntrada(f.p.t(marca, nil))
		e.PonerTexto(valor)
		return e
	}
	x := &filaDireccion{
		tipo:   ui.NuevoSelector(f.rotulos(tiposDireccion), nil),
		calle:  campo("form.street", a.Street),
		ciudad: campo("form.city", a.Locality),
		region: campo("form.region", a.Region),
		cp:     campo("form.postalCode", a.PostalCode),
		pais:   campo("form.country", a.Country),
	}
	x.tipo.Elegida, x.tipo.Ancho = indiceTipo(a.Type, tiposDireccion), AnchoTipo
	return x
}

func (f *formulario) rotulos(ops []tipoOpcion) []string {
	r := make([]string, len(ops))
	for i, o := range ops {
		r[i] = f.p.t(o.clave, nil)
	}
	return r
}

// dialogo monta el diálogo entero en ese lienzo. El cuerpo es un contenedor que se
// rehace cuando cambian las filas; los campos no, así que lo escrito y el foco se
// conservan.
func (f *formulario) dialogo(lienzo fyne.Canvas) *ui.Dialogo {
	f.lienzo = lienzo
	titulo := "form.newTitle"
	if f.contacto != nil {
		titulo = "form.editTitle"
	}
	f.cuerpo = container.New(ui.Columna{Hueco: HuecoFormulario})
	f.cancelar = ui.NuevoBoton(f.p.t("common.cancel", nil), func() { f.d.Descartar() }).Con(ui.Secundario, ui.TallaMD)
	f.guardarBoton = ui.NuevoBoton(f.p.t("common.save", nil), f.guardar).Con(ui.Primario, ui.TallaMD)
	// «Eliminar» a la izquierda en escritorio; en el teléfono, sola arriba y Cancelar |
	// Guardar debajo (`ui.PieAcciones`, el pie de Modal.tsx).
	pie := container.New(ui.PieAcciones{}, f.cancelar, f.guardarBoton)
	if f.contacto != nil {
		f.borrar = ui.NuevoBoton(f.p.t("common.delete", nil), f.pulsarBorrar).Con(ui.Peligro, ui.TallaMD)
		pie = container.New(ui.PieAcciones{Izquierda: 1}, f.borrar, f.cancelar, f.guardarBoton)
	}
	f.d = ui.NuevoDialogo(f.p.t(titulo, nil), f.cuerpo).Con(ui.DialogoMD, pie)
	f.d.RotuloCerrar = f.p.t("ui.close", nil)
	f.d.AlCerrar = func() {
		if !f.ocupado() {
			f.d.Cerrar()
		}
	}
	f.rehacer()
	return f.d
}

// ocupado es si hay una petición en vuelo: mientras tanto el diálogo no se descarta
// (`dismissable={busy === null}`).
func (f *formulario) ocupado() bool {
	return f.guardarBoton.Cargando || (f.borrar != nil && f.borrar.Cargando)
}

// rehacer vuelve a colocar el cuerpo con las filas de ahora.
func (f *formulario) rehacer() {
	f.nombre.Invalida = f.errNombre != ""
	for i, x := range f.correos {
		x.valor.Invalida = i == f.errCorreo
		x.valor.Refresh()
	}
	f.nombre.Refresh()
	var errCorreo string
	if f.errCorreo >= 0 {
		errCorreo = f.p.t("form.emailInvalid", nil)
	}
	f.cuerpo.Objects = []fyne.CanvasObject{f.previa()}
	// El asistente va ARRIBA porque es el atajo a la ficha entera: se pega la firma, se
	// rellenan los campos y se revisa hacia abajo.
	if f.alAsistir != nil {
		f.cuerpo.Objects = append(f.cuerpo.Objects, f.asistente())
	}
	f.cuerpo.Objects = append(f.cuerpo.Objects,
		ui.GrupoCampo(f.p.t("form.fullName", nil), true, f.nombre, "", f.errNombre),
		f.listaValores("field.email", &f.correos, &f.principalCorreo, tiposCorreo,
			"form.emailPlaceholder", "form.addEmail", "form.removeEmail", errCorreo),
		f.listaValores("field.phone", &f.telefonos, &f.principalTelefono, tiposTel,
			"form.phonePlaceholder", "form.addPhone", "form.removePhone", ""),
		f.listaDirecciones(),
		// ponytail: sin la ayuda de `form.birthdayPartial`. La web la enseña cuando la
		// vCard trae un cumpleaños SIN año, y la API REST no lo da (el servicio lo
		// descarta al proyectar, ver cumpleanos.go): aquí nunca se sabe que lo hay.
		ui.GrupoCampo(f.p.t("field.birthday", nil), false, f.cumple, "", ""),
		ui.GrupoCampo(f.p.t("field.organization", nil), false, f.org, "", ""),
		ui.GrupoCampo(f.p.t("field.note", nil), false, f.nota, "", ""),
	)
	f.cuerpo.Refresh()
	recolocar(f.lienzo, f.d)
}

// asistente es `AsistenteFicha`: pegar una firma de correo y que la ficha salga
// rellena. NO GUARDA NADA: rellena el formulario y ahí se queda, para revisarlo.
//
// ponytail: el `aria-label` del área («Pega una firma o una tarjeta») no llega al
// lector de pantalla: `ui.AreaTexto` no es `fyne.Accessible` todavía (ver listaValores).
func (f *formulario) asistente() fyne.CanvasObject {
	if f.ia == nil {
		f.ia = ui.NuevaAreaTexto(f.p.t("form.aiPlaceholder", nil))
		f.ia.Filas = FilasAsistente
		f.rellenar = ui.NuevoBoton(f.p.t("form.aiFill", nil), f.pedirFicha).
			Con(ui.Secundario, ui.TallaSM).ConIcono(iconos.IA, IconoAsistente)
		f.rellenar.Inerte = true
		f.ia.AlCambiar(func(s string) {
			f.rellenar.Inerte = strings.TrimSpace(s) == ""
			f.rellenar.Refresh()
			if f.errIA {
				f.errIA = false
				f.rehacer()
			}
		})
	}
	hijos := []fyne.CanvasObject{f.ia}
	if f.errIA {
		v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
		hijos = append(hijos, ui.NuevoParrafo(f.p.t("form.aiError", nil), tema.TextoXS, ui.Normal, "danger-text", v).
			A(anchoCuerpo(f.lienzo.Size(), 560)))
	}
	// `flex justify-end`: el botón a su tamaño y a la derecha.
	hijos = append(hijos, container.New(filaEntre{}, container.New(ui.Fila{}), f.rellenar))
	return container.New(ui.Columna{Hueco: HuecoAsistente}, hijos...)
}

// pedirFicha manda lo pegado al asistente y, si vuelve una ficha, la aplica y vacía el
// área; si no, lo dice debajo.
func (f *formulario) pedirFicha() {
	bloque := []rune(strings.TrimSpace(f.ia.Texto()))
	if len(bloque) == 0 || f.rellenar.Cargando || f.alAsistir == nil {
		return
	}
	f.rellenar.Cargando, f.errIA = true, false
	f.rellenar.Refresh()
	f.alAsistir(string(bloque[:min(len(bloque), MaxAsistente)]), func(ficha api.FichaPropuesta, ok bool) {
		f.rellenar.Cargando = false
		f.rellenar.Refresh()
		if !ok {
			f.errIA = true
			f.rehacer()
			return
		}
		f.aplicar(ficha)
		f.ia.PonerTexto("")
	})
}

// aplicar es `aplicarPropuesta`: el nombre y la organización se sustituyen, la nota se
// añade a la que hubiera, y los correos y teléfonos se suman a los ya escritos.
func (f *formulario) aplicar(p api.FichaPropuesta) {
	f.nombre.PonerTexto(p.Nombre)
	f.errNombre = ""
	if p.Organizacion != "" {
		f.org.PonerTexto(p.Organizacion)
	}
	if p.Nota != "" {
		if n := f.nota.Texto(); strings.TrimSpace(n) != "" {
			f.nota.PonerTexto(n + "\n" + p.Nota)
		} else {
			f.nota.PonerTexto(p.Nota)
		}
	}
	f.correos = f.fusionar(f.correos, p.Correos, tiposCorreo, "work", "form.emailPlaceholder")
	f.errCorreo = -1
	f.telefonos = f.fusionar(f.telefonos, p.Telefonos, tiposTel, "cell", "form.phonePlaceholder")
	f.principalCorreo = min(f.principalCorreo, len(f.correos)-1)
	f.principalTelefono = min(f.principalTelefono, len(f.telefonos)-1)
	f.pintarPrevia()
	f.rehacer()
}

// fusionar es `fusionar` de ContactModal.tsx: añade los valores nuevos a las filas sin
// duplicar ni perder lo tecleado. Las filas vacías se van: son el hueco que deja el
// formulario al crear.
func (f *formulario) fusionar(filas []*filaValor, nuevos []string, ops []tipoOpcion, tipo, marca string) []*filaValor {
	var fuera []*filaValor
	ya := map[string]bool{}
	for _, x := range filas {
		if v := strings.TrimSpace(x.valor.Texto()); v != "" {
			fuera = append(fuera, x)
			ya[strings.ToLower(v)] = true
		}
	}
	for _, v := range nuevos {
		if l := strings.ToLower(v); !ya[l] {
			ya[l] = true
			fuera = append(fuera, f.nuevaFila(ops, indiceTipo(tipo, ops), v, marca))
		}
	}
	if len(fuera) == 0 {
		return filas
	}
	return fuera
}

// previa es la vista previa: el avatar y el nombre que se van escribiendo.
//
// El avatar es el AvatarPicker `lg`: lo elegido se ve ya y se sube al guardar.
func (f *formulario) previa() fyne.CanvasObject {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	if f.avatar == nil {
		f.avatar = ui.NuevoAvatar("").Con(ui.AvatarLG)
		f.selector = ui.NuevoSelectorAvatar(f.avatar)
		f.p.rotularSelector(f.selector)
		f.selector.AlElegirColor = func(token string) {
			if uri, err := fotoPredefinida(f.nombrePrevia(), token); err == nil {
				f.fotoNueva, f.fotoQuitada = uri, false
				f.pintarPrevia()
			}
		}
		if f.subirFoto != nil {
			f.selector.AlSubir = func() {
				f.subirFoto(func(uri string) {
					f.fotoNueva, f.fotoQuitada = uri, false
					f.pintarPrevia()
				})
			}
		}
		f.selector.AlQuitar = func() {
			f.fotoNueva, f.fotoQuitada = "", true
			f.pintarPrevia()
		}
		f.previaNombre = ui.Texto("", tema.TextoBase, ui.SemiNegrita, tema.Color("text", v))
		f.previaOrg = ui.Texto("", tema.TextoSM, ui.Normal, tema.Color("text-muted", v))
	}
	f.pintarPrevia()
	filete := canvas.NewRectangle(tema.Color("border", v))
	f.previaCont = container.New(&previaLayout{f: f}, f.selector, f.previaNombre, f.previaOrg, filete)
	return f.previaCont
}

// nombrePrevia es `fullName.trim() || t('form.newTitle')`.
func (f *formulario) nombrePrevia() string {
	if n := strings.TrimSpace(f.nombre.Texto()); n != "" {
		return n
	}
	return f.p.t("form.newTitle", nil)
}

func (f *formulario) pintarPrevia() {
	if f.avatar == nil {
		return
	}
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	nombre := f.nombrePrevia()
	f.avatar.Nombre = nombre
	switch {
	case f.fotoNueva != "":
		f.avatar.Foto = imagenDeURI(f.fotoNueva)
	case f.fotoQuitada:
		f.avatar.Foto = nil
	default:
		f.avatar.Foto = f.fotoActual
	}
	f.selector.TieneFoto = f.avatar.Foto != nil
	f.selector.Refresh()
	f.previaNombre.Text = nombre
	f.textoOrg = strings.TrimSpace(f.org.Texto())
	f.previaOrg.Color = tema.Color("text-muted", v)
	if f.textoOrg == "" {
		f.textoOrg, f.previaOrg.Color = f.p.t("form.preview", nil), tema.Color("text-subtle", v)
	}
	f.previaOrg.Text = f.textoOrg
	f.previaNombre.Refresh()
	f.previaOrg.Refresh()
	if f.previaCont != nil {
		f.previaCont.Refresh() // vuelve a recortar al ancho
	}
}

// previaLayout coloca el avatar y los dos textos (truncados) con el filete de abajo.
type previaLayout struct{ f *formulario }

func (l *previaLayout) altoTextos() float32 { return tema.TextoBase.Linea + tema.TextoSM.Linea }

func (l *previaLayout) MinSize([]fyne.CanvasObject) fyne.Size {
	av := ui.MedidaAvatar(ui.AvatarLG)
	return fyne.NewSize(av.Width, max(av.Height, l.altoTextos())+RellenoPrevia+tema.Borde)
}

func (l *previaLayout) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	av := ui.MedidaAvatar(ui.AvatarLG)
	alto := max(av.Height, l.altoTextos())
	objs[0].Move(fyne.NewPos(0, (alto-av.Height)/2))
	objs[0].Resize(av)
	x := av.Width + HuecoPrevia
	cabe := max(t.Width-x, 0)
	y := (alto - l.altoTextos()) / 2
	f := l.f
	ui.Truncar(f.previaNombre, f.nombrePrevia(), tema.TextoBase, ui.SemiNegrita, cabe)
	ui.ColocarTexto(f.previaNombre, tema.TextoBase, x, y)
	ui.Truncar(f.previaOrg, f.textoOrg, tema.TextoSM, ui.Normal, cabe)
	ui.ColocarTexto(f.previaOrg, tema.TextoSM, x, y+tema.TextoBase.Linea)
	objs[3].Move(fyne.NewPos(0, t.Height-tema.Borde))
	objs[3].Resize(fyne.NewSize(t.Width, tema.Borde))
}

// leyenda es el `<legend>` de un fieldset.
func (f *formulario) leyenda(clave string) fyne.CanvasObject {
	return ui.NuevoRotulo(f.p.t(clave, nil), tema.TextoSM, ui.SemiNegrita, "text")
}

// fieldset es la leyenda, a `mb-1` las filas, y el resto separado por `gap-2`.
func fieldset(leyenda fyne.CanvasObject, filas ...fyne.CanvasObject) fyne.CanvasObject {
	return container.New(ui.Columna{Hueco: MargenLeyenda}, leyenda,
		container.New(ui.Columna{Hueco: HuecoFieldset}, filas...))
}

// botonAnadir es el «Añadir …» de debajo de cada lista: `ghost sm` con su «+» de 14,
// que no se estira (va dentro de un `<div>`).
func botonAnadir(texto string, alPulsar func()) fyne.CanvasObject {
	b := ui.NuevoBoton(texto, alPulsar).Con(ui.Fantasma, ui.TallaSM).ConIcono(iconos.Mas, 14)
	return container.New(ui.Fila{}, b)
}

// botonQuitar es el aspa de una fila: `ghost sm iconOnly` con el «close» de 16.
func botonQuitar(rotulo string, alPulsar func()) *ui.Boton {
	b := ui.NuevoBoton("", alPulsar).Con(ui.Fantasma, ui.TallaSM)
	b.SoloIcono, b.Icono, b.TamIcono, b.Rotulo = true, iconos.Cerrar, 16, rotulo
	return b
}

// listaValores es `ValueList`: las filas de correos o de teléfonos.
//
// ponytail: los `aria-label` de cada control («Etiqueta del correo 1», «Correo 1») no
// llegan al lector de pantalla: `ui.Selector` y `ui.Entrada` no son `fyne.Accessible`
// todavía. Cuando lo sean, las claves `form.*TypeLabel` y `form.*Label` ya están.
func (f *formulario) listaValores(leyenda string, lista *[]*filaValor, principal *int, ops []tipoOpcion,
	marca, anadir, rotuloQuitar, err string,
) fyne.CanvasObject {
	filas := *lista
	hijos := make([]fyne.CanvasObject, 0, len(filas)+2)
	for i, x := range filas {
		indice := i
		vars := i18n.Vars{"index": i + 1}
		piezas := []fyne.CanvasObject{x.tipo, x.valor}
		if len(filas) > 1 {
			radio := ui.NuevoRadio(f.p.t("form.primary", nil), func(bool) {
				*principal = indice
				f.rehacer()
			})
			radio.Marcada = *principal == i
			piezas = append(piezas, radio)
		}
		piezas = append(piezas, botonQuitar(f.p.t(rotuloQuitar, vars), func() { f.quitarFila(lista, indice, principal) }))
		hijos = append(hijos, container.New(&filaFlexible{Hueco: HuecoFieldset, Crece: 1, Base: BaseValor}, piezas...))
	}
	if err != "" {
		hijos = append(hijos, ui.LineaError(err))
	}
	hijos = append(hijos, botonAnadir(f.p.t(anadir, nil), func() {
		*lista = append(*lista, f.nuevaFila(ops, 0, "", marca))
		f.rehacer()
	}))
	return fieldset(f.leyenda(leyenda), hijos...)
}

// quitarFila quita una fila y reajusta el principal como la web: si era él, pasa a la
// primera; si iba detrás, baja uno.
func (f *formulario) quitarFila(lista *[]*filaValor, i int, principal *int) {
	*lista = append((*lista)[:i:i], (*lista)[i+1:]...)
	f.errCorreo = -1
	switch {
	case *principal == i:
		*principal = 0
	case *principal > i:
		*principal--
	}
	f.rehacer()
}

// listaDirecciones es `AddressList`: cada dirección en su caja, sin ninguna al crear.
func (f *formulario) listaDirecciones() fyne.CanvasObject {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	hijos := make([]fyne.CanvasObject, 0, len(f.direcciones)+1)
	for i, x := range f.direcciones {
		indice := i
		vars := i18n.Vars{"index": i + 1}
		quitar := botonQuitar(f.p.t("form.removeAddress", vars), func() {
			f.direcciones = append(f.direcciones[:indice:indice], f.direcciones[indice+1:]...)
			f.rehacer()
		})
		dos := func(a, b fyne.CanvasObject) fyne.CanvasObject {
			return container.New(&dosColumnas{Hueco: HuecoFieldset}, a, b)
		}
		dentro := container.New(ui.Columna{Hueco: HuecoFieldset},
			container.New(filaEntre{}, x.tipo, quitar),
			x.calle, dos(x.ciudad, x.region), dos(x.cp, x.pais))
		caja := ui.NuevaCaja(tema.Color("surface-raised", v), tema.Color("border", v), tema.RadioMD)
		hijos = append(hijos, container.New(&enCaja{caja: caja, margen: RellenoDireccion + tema.Borde},
			append(caja.Objetos(), dentro)...))
	}
	hijos = append(hijos, botonAnadir(f.p.t("form.addAddress", nil), func() {
		f.direcciones = append(f.direcciones, f.nuevaDireccion(api.ContactAddress{Type: "home"}))
		f.rehacer()
	}))
	return fieldset(f.leyenda("field.address"), hijos...)
}

// pulsarBorrar es la eliminación en dos pasos: el primer clic cambia el rótulo a
// «Confirmar eliminación» y el segundo borra.
func (f *formulario) pulsarBorrar() {
	if !f.confirmarBorrado {
		f.confirmarBorrado = true
		f.borrar.Texto = f.p.t("form.confirmDelete", nil)
		f.borrar.Refresh()
		recolocar(f.lienzo, f.d) // «Confirmar eliminación» es más largo
		return
	}
	if f.alBorrar == nil {
		return
	}
	f.enVuelo(f.borrar, true)
	f.alBorrar(func(err error) {
		f.enVuelo(f.borrar, false)
		f.confirmarBorrado = false
		f.borrar.Texto = f.p.t("common.delete", nil)
		f.borrar.Refresh()
		if err == nil {
			f.d.Cerrar()
		}
	})
}

// enVuelo marca la acción en curso con su rueda y apaga las demás.
func (f *formulario) enVuelo(b *ui.Boton, si bool) {
	for _, x := range []*ui.Boton{f.borrar, f.cancelar, f.guardarBoton} {
		if x == nil {
			continue
		}
		x.Inerte = si && x != b
		x.Cargando = si && x == b
		x.Refresh()
	}
}

// guardar valida, arma la petición y la manda.
func (f *formulario) guardar() {
	in, ok := f.entrada()
	if !ok || f.alGuardar == nil {
		return
	}
	f.enVuelo(f.guardarBoton, true)
	f.alGuardar(in, func(err error) {
		f.enVuelo(f.guardarBoton, false)
		if err == nil {
			f.d.Cerrar()
		}
	})
}

// entrada es `handleSave` hasta la petición: valida el nombre, la forma de los correos
// y la fecha, y arma el contacto ENTERO. Al editar parte de `Contact.Input`, porque el
// PUT sustituye el contacto y lo que no vaya se pierde (apellidos, cargo, foto…).
func (f *formulario) entrada() (api.ContactInput, bool) {
	var in api.ContactInput
	if f.contacto != nil {
		in = f.contacto.Input()
	}
	nombre := strings.TrimSpace(f.nombre.Texto())
	f.errNombre = ""
	if nombre == "" {
		f.errNombre = f.p.t("form.nameRequired", nil)
	}
	f.errCorreo = -1
	for i, x := range f.correos {
		if v := strings.TrimSpace(x.valor.Texto()); v != "" && !correoValido.MatchString(v) {
			f.errCorreo = i
			break
		}
	}
	cumple, vacia, fechaOK := ui.LeerFecha(f.cumple.Texto(), time.UTC)
	// ponytail: una fecha mal tecleada solo pone el borde en rojo: el catálogo no tiene
	// frase para ello porque en la web el `<input type="date">` no deja escribirla.
	f.cumple.Invalida = !vacia && !fechaOK
	f.cumple.Refresh()
	f.rehacer()
	if f.errNombre != "" || f.errCorreo >= 0 || f.cumple.Invalida {
		return in, false
	}

	in.FormattedName, in.Organization, in.Note = nombre, strings.TrimSpace(f.org.Texto()), f.nota.Texto()
	if strings.TrimSpace(in.Note) == "" {
		in.Note = ""
	}
	in.Birthday = ""
	if !vacia {
		in.Birthday = cumple.Format(time.DateOnly)
	}
	in.Emails = nil
	for _, x := range tipados(f.correos, f.principalCorreo, tiposCorreo) {
		in.Emails = append(in.Emails, api.ContactEmail{Email: x.valor, Type: x.tipo, IsPrimary: x.principal})
	}
	in.Phones = nil
	for _, x := range tipados(f.telefonos, f.principalTelefono, tiposTel) {
		in.Phones = append(in.Phones, api.ContactPhone{Phone: x.valor, Type: x.tipo, IsPrimary: x.principal})
	}
	in.Addresses = nil
	for _, d := range f.direcciones {
		a := api.ContactAddress{
			Type:   tiposDireccion[d.tipo.Elegida].valor,
			Street: strings.TrimSpace(d.calle.Texto()), Locality: strings.TrimSpace(d.ciudad.Texto()),
			Region: strings.TrimSpace(d.region.Texto()), PostalCode: strings.TrimSpace(d.cp.Texto()),
			Country: strings.TrimSpace(d.pais.Texto()),
		}
		if a.Street+a.Locality+a.Region+a.PostalCode+a.Country != "" {
			in.Addresses = append(in.Addresses, a)
		}
	}
	return in, true
}

// tipados es `toDetailed`: las filas con valor, con exactamente un principal (el
// elegido si sigue teniendo valor, o el primero).
func tipados(filas []*filaValor, principal int, ops []tipoOpcion) []valorTipado {
	var fuera []valorTipado
	hay := false
	for i, x := range filas {
		v := strings.TrimSpace(x.valor.Texto())
		if v == "" {
			continue
		}
		fuera = append(fuera, valorTipado{valor: v, tipo: ops[x.tipo.Elegida].valor, principal: i == principal})
		hay = hay || i == principal
	}
	if len(fuera) > 0 && !hay {
		fuera[0].principal = true
	}
	return fuera
}

// ---- Layouts del formulario ----

// filaEntre es `flex justify-between items-center`: el primero a la izquierda y el
// segundo a la derecha, cada uno a su tamaño. Ancho, si lo hay, es el `w-full`.
type filaEntre struct{ Ancho float32 }

func (f filaEntre) MinSize(objs []fyne.CanvasObject) fyne.Size {
	m := ui.Fila{}.MinSize(objs)
	return fyne.NewSize(max(m.Width, f.Ancho), m.Height)
}

// Layout coloca los dos; si no caben, el primero ENCOGE (el `min-w-0` de la web, que
// es lo que deja truncar un nombre largo antes que pisar los botones).
func (filaEntre) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	var ultimo float32
	if n := len(objs); n > 1 {
		ultimo = objs[n-1].MinSize().Width + HuecoEntre
	}
	for i, o := range objs {
		m := o.MinSize()
		x := float32(0)
		if i == len(objs)-1 && i > 0 {
			x = t.Width - m.Width
		} else {
			m.Width = max(min(m.Width, t.Width-ultimo), 0)
		}
		o.Move(fyne.NewPos(x, (t.Height-m.Height)/2))
		o.Resize(m)
	}
}

// HuecoEntre es el `gap-3` de las filas `justify-between` de esta app.
var HuecoEntre = tema.Espacio(3)

// filaFlexible es `flex flex-wrap items-center gap-*` con UN hijo que crece (`grow
// basis-*`): los hijos se reparten en líneas sin pasarse del ancho —el de crecer cuenta
// con su base— y el que crece se lleva lo que sobre en la suya. En el móvil (un diálogo
// de 332) el aspa de un correo baja a la línea de abajo, como en la web.
//
// Mide su alto CON el ancho que le dieron en el último reparto: como el párrafo, una
// fila que envuelve no sabe lo que mide hasta que sabe lo que tiene.
type filaFlexible struct {
	Hueco float32
	Crece int
	Base  float32
	ancho float32
}

func (f *filaFlexible) base(i int, o fyne.CanvasObject) float32 {
	if i == f.Crece {
		return f.Base
	}
	return o.MinSize().Width
}

// lineas reparte los índices de los hijos en líneas para un ancho.
func (f *filaFlexible) lineas(objs []fyne.CanvasObject, ancho float32) [][]int {
	var fuera [][]int
	linea := make([]int, 0, len(objs))
	usado := float32(0)
	for i, o := range objs {
		b := f.base(i, o)
		if len(linea) > 0 && ancho > 0 && usado+f.Hueco+b > ancho {
			fuera, linea, usado = append(fuera, linea), make([]int, 0, len(objs)), 0
		}
		if len(linea) > 0 {
			usado += f.Hueco
		}
		linea, usado = append(linea, i), usado+b
	}
	return append(fuera, linea)
}

func (f *filaFlexible) altoLinea(objs []fyne.CanvasObject, l []int) float32 {
	var alto float32
	for _, i := range l {
		alto = max(alto, objs[i].MinSize().Height)
	}
	return alto
}

func (f *filaFlexible) MinSize(objs []fyne.CanvasObject) fyne.Size {
	var ancho, alto float32
	for i, o := range objs {
		ancho = max(ancho, f.base(i, o))
	}
	for n, l := range f.lineas(objs, f.ancho) {
		if n > 0 {
			alto += f.Hueco
		}
		alto += f.altoLinea(objs, l)
	}
	return fyne.NewSize(ancho, alto)
}

func (f *filaFlexible) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	f.ancho = t.Width
	var y float32
	for _, l := range f.lineas(objs, t.Width) {
		alto := f.altoLinea(objs, l)
		sobra := t.Width
		for n, i := range l {
			sobra -= f.base(i, objs[i])
			if n > 0 {
				sobra -= f.Hueco
			}
		}
		var x float32
		for _, i := range l {
			m := objs[i].MinSize()
			if i == f.Crece {
				m.Width = f.Base + max(sobra, 0)
			}
			objs[i].Move(fyne.NewPos(x, y+(alto-m.Height)/2))
			objs[i].Resize(m)
			x += m.Width + f.Hueco
		}
		y += alto + f.Hueco
	}
}

// dosColumnas es `grid grid-cols-2 gap-*`: dos mitades iguales.
type dosColumnas struct{ Hueco float32 }

func (d *dosColumnas) MinSize(objs []fyne.CanvasObject) fyne.Size {
	var ancho, alto float32
	for _, o := range objs {
		m := o.MinSize()
		ancho, alto = max(ancho, m.Width), max(alto, m.Height)
	}
	return fyne.NewSize(2*ancho+d.Hueco, alto)
}

func (d *dosColumnas) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	mitad := (t.Width - d.Hueco) / 2
	for i, o := range objs {
		o.Move(fyne.NewPos(float32(i)*(mitad+d.Hueco), 0))
		o.Resize(fyne.NewSize(mitad, o.MinSize().Height))
	}
}

// enCaja pone un hijo dentro de una caja con borde, con ese margen por dentro.
type enCaja struct {
	caja   *ui.Caja
	margen float32
}

func (e *enCaja) MinSize(objs []fyne.CanvasObject) fyne.Size {
	m := objs[len(objs)-1].MinSize()
	return fyne.NewSize(m.Width+2*e.margen, m.Height+2*e.margen)
}

func (e *enCaja) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	e.caja.Redimensionar(t)
	dentro := objs[len(objs)-1]
	dentro.Move(fyne.NewPos(e.margen, e.margen))
	dentro.Resize(fyne.NewSize(max(t.Width-2*e.margen, 0), max(t.Height-2*e.margen, 0)))
}
