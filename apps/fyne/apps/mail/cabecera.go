// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package mail

import (
	"cmp"
	"fmt"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// La barra de la cabecera del lector (`<header>` de MessageReader.tsx), de la hoja:
//
//	barra   `mt-4 flex flex-wrap gap-2`: Responder, Responder a todos, Reenviar,
//	        [Darse de baja] y, con `ms-auto`, el grupo de triaje
//	grupo   `flex flex-wrap items-center gap-1`: destacar, archivar, papelera,
//	        [No es spam], «Más acciones», «Etiquetar» y «Mover a»
//	chips   `mt-3 flex flex-wrap items-center gap-1.5`: las etiquetas del hilo
//
// Todos los botones son `size="sm"` (28 de alto, `px-3`) y los glifos de 16. Por
// debajo de 768 px de VENTANA (`max-md:`) los rótulos de todos menos «Responder» pasan
// a `sr-only`: siguen siendo el nombre accesible, pero el botón se queda en su glifo.
// Con los rótulos, la barra ocupaba cuatro filas en un teléfono antes de ver el correo.
const (
	HuecoBarraLector  float32 = 2 * 0.25 * tema.Rem
	HuecoTriajeLector float32 = 1 * 0.25 * tema.Rem
	AireChipsLector   float32 = 3 * 0.25 * tema.Rem
	HuecoChipsLector  float32 = 1.5 * 0.25 * tema.Rem
)

// Los chips de etiqueta de la cabecera son la `<Tag size="xs" tone="muted">` de
// MessageReader.tsx: `ui.Etiqueta` densa y apagada, con el punto del color de la etiqueta
// y el nombre en `max-w-[12rem]`.
const maxNombreChip float32 = 12 * tema.Rem

// estrecha es `max-md:`: la VENTANA por debajo de 768 px del rem de 16.
func estrecha(o fyne.CanvasObject, respaldo float32) bool {
	return ventana(o, respaldo) < ui.Estrecho
}

// botonCabecera es un botón `sm` de la cabecera con uno de los glifos de Mail delante
// y, si `detras`, el galón de un desplegable. `soloGlifo` es el rótulo en `sr-only`.
func botonCabecera(texto, d string, variante ui.Variante, token string, lleno, soloGlifo bool, detras iconos.Icono, alPulsar func()) *ui.Boton {
	b := ui.NuevoBoton(texto, alPulsar).Con(variante, ui.TallaSM)
	b.Rotulo = texto
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	if token == "" {
		token = tintaBoton(variante)
	}
	if lleno {
		b.Glifo = GlifoRelleno(d, TallaGlifoLector, token, v)
	} else {
		b.Glifo = Glifo(d, TallaGlifoLector, token, v)
	}
	b.TamIcono = TallaGlifoLector
	b.IconoDetras = detras
	if soloGlifo {
		b.Texto = ""
	}
	return b
}

// botonIconoCabecera es un `iconOnly` `sm`: cuadrado de 28 con el glifo centrado.
func botonIconoCabecera(rotulo, d, token string, lleno bool, alPulsar func()) *ui.Boton {
	b := botonCabecera(rotulo, d, ui.Fantasma, token, lleno, false, iconos.Ninguno, alPulsar)
	b.SoloIcono = true
	return b
}

// claveBarra resume todo lo que cambia la barra: si no cambia, no se rehace, y un
// botón que se rehace pierde el hover y el foco. Lo que la cambia es el estado del
// hilo y de la vista, no cada repintado.
func (x *Lector) claveBarra(estrecha bool) string {
	a, c := x.Acciones, x.Conversacion
	var b strings.Builder
	fmt.Fprintf(&b, "%v|%v|%v|%v|%v|%v|%v|%s|%v%v%v%v", x.SoloLectura, estrecha, x.Baja, x.DandoDeBaja,
		x.CreandoEvento, c.Destacada, c.SinLeer, c.HiloID,
		x.AlNavegar != nil, x.HayAnterior, x.HaySiguiente, x.CargandoSiguiente)
	fmt.Fprintf(&b, "|%v|%d", x.Silenciada, x.PospuestaHasta.Unix())
	if a != nil {
		fmt.Fprintf(&b, "|%v%v%v", a.Posponer != nil, a.QuitarPosposicion != nil, a.Silenciar != nil)
		fmt.Fprintf(&b, "|%v%v%v%v%v%v%v|%d|%d", a.Archivar != nil, a.Papelera != nil, a.Spam != nil,
			a.NoSpam != nil, a.Purgar != nil, a.CrearRegla != nil, a.Etiquetar != nil,
			len(a.Destinos), len(a.Etiquetas))
		for _, e := range EtiquetasDe(c, a.Etiquetas) {
			b.WriteString("|" + e.ID + e.Nombre + e.Color)
		}
		// El punto de una etiqueta sin color es `border-strong`, que cambia con el tema: los
		// chips se rehacen al cambiarlo para que no se quede el del tema anterior.
		fmt.Fprintf(&b, "|%v", tema.De(fyne.CurrentApp().Settings().ThemeVariant()))
	}
	return b.String()
}

// barra monta la fila de botones de la cabecera.
func (x *Lector) barra(estrecha bool) *fyne.Container {
	t := x.T
	objs := []fyne.CanvasObject{
		// «Responder» conserva el rótulo también en el móvil: es la acción del lector.
		botonCabecera(t.T("reader.reply", nil), ResponderD, ui.Primario, "", false, false, iconos.Ninguno,
			func() { x.responder(Responder) }),
		botonCabecera(t.T("reader.replyAll", nil), ResponderTodosD, ui.Secundario, "", false, estrecha, iconos.Ninguno,
			func() { x.responder(ResponderATodos) }),
		botonCabecera(t.T("reader.forward", nil), ReenviarD, ui.Secundario, "", false, estrecha, iconos.Ninguno,
			func() { x.responder(Reenviar) }),
	}
	if !x.SoloLectura && x.AlCrearEvento != nil {
		// Con rótulo también en el móvil: un «+» a secas junto a responder y reenviar
		// no dice qué añade. El «+» es el `Icon` de 20 por defecto, no un glifo de 16.
		evento := ui.NuevoBoton(t.T("reader.createEvent", nil), func() {
			if len(x.Conversacion.Mensajes) > 0 {
				x.AlCrearEvento(x.Conversacion.Mensajes[0])
			}
		}).Con(ui.Fantasma, ui.TallaSM)
		evento.Icono, evento.Cargando = iconos.Mas, x.CreandoEvento
		objs = append(objs, evento)
	}
	if !x.SoloLectura && x.Baja {
		baja := botonCabecera(t.T("reader.unsubscribe", nil), BajaD, ui.Fantasma, "", false, estrecha, iconos.Ninguno,
			func() {
				if x.AlDarseDeBaja != nil && len(x.Conversacion.Mensajes) > 0 {
					x.AlDarseDeBaja(x.Conversacion.Mensajes[0])
				}
			})
		baja.Cargando = x.DandoDeBaja
		objs = append(objs, baja)
	}
	if !x.SoloLectura && x.Acciones != nil {
		objs = append(objs, x.grupoTriaje(estrecha))
	}
	return container.New(ui.Flujo{Hueco: HuecoBarraLector, UltimoADerecha: true}, objs...)
}

// navegacion son «Más reciente» y «Más antigua», los `iconOnly` `sm` con galón de 16
// de la fila del asunto (`flex shrink-0 gap-1 self-start`). Van ahí y no en la barra:
// la barra ya está al límite de ancho, y en un teléfono, que es quien más los
// necesita, dos botones más la partían en otra fila. Nil sin `AlNavegar`.
func (x *Lector) navegacion() *fyne.Container {
	if x.AlNavegar == nil {
		return nil
	}
	t := x.T
	boton := func(clave string, icono iconos.Icono, siguiente, hay bool) *ui.Boton {
		b := ui.NuevoBoton(t.T(clave, nil), func() { x.AlNavegar(siguiente) }).Con(ui.Fantasma, ui.TallaSM)
		b.Rotulo, b.SoloIcono, b.Icono, b.TamIcono = b.Texto, true, icono, 16
		b.Inerte = !hay
		return b
	}
	anterior := boton("reader.newer", iconos.ChevronArriba, false, x.HayAnterior)
	siguiente := boton("reader.older", iconos.ChevronAbajo, true, x.HaySiguiente)
	siguiente.Cargando = x.CargandoSiguiente
	return container.New(ui.Flujo{Hueco: HuecoTriajeLector}, anterior, siguiente)
}

// grupoTriaje es el `<span role="group">` del triaje del hilo abierto. Destacar,
// archivar y papelera siguen a un clic —son el triaje de todos los días y tienen
// atajo—; lo menos frecuente va al menú «Más acciones», que es lo que evita que la
// cabecera se parta en tres filas en un portátil.
func (x *Lector) grupoTriaje(estrecha bool) *fyne.Container {
	t, a, c := x.T, x.Acciones, x.Conversacion
	var objs []fyne.CanvasObject
	if a.Destacar != nil {
		// Sin destacar, la tinta del fantasma (`token` vacío), como sus vecinos.
		clave, token := "action.star", ""
		if c.Destacada {
			clave, token = "action.unstar", "warning-text"
		}
		objs = append(objs, botonIconoCabecera(t.T(clave, nil), EstrellaD, token, c.Destacada,
			func() { a.Destacar(x.Conversacion) }))
	}
	if a.Archivar != nil {
		objs = append(objs, botonIconoCabecera(t.T("action.archive", nil), ArchivarD, "", false,
			func() { a.Archivar(x.Conversacion) }))
	}
	// «Posponer» va junto a archivar, como en la web: es la otra forma de quitarse un
	// correo de delante.
	if a.Posponer != nil {
		objs = append(objs, x.botonPosponer())
	}
	if a.Papelera != nil {
		objs = append(objs, botonIconoCabecera(t.T("action.trash", nil), PapeleraD, "", false,
			func() { a.Papelera(x.Conversacion) }))
	}
	if a.NoSpam != nil {
		objs = append(objs, botonCabecera(t.T("action.notSpam", nil), SpamD, ui.Fantasma, "", false, false,
			iconos.Ninguno, func() { a.NoSpam(x.Conversacion) }))
	}
	mas := ui.NuevoBoton(t.T("reader.moreActions", nil), nil).Con(ui.Fantasma, ui.TallaSM)
	mas.Rotulo, mas.SoloIcono, mas.Icono, mas.TamIcono = mas.Texto, true, iconos.Puntos, 16
	mas.AlPulsar = func() { x.menuMas().Mostrar(mas) }
	objs = append(objs, mas)
	x.botonEtiquetar, x.botonMover = nil, nil
	// Las etiquetas son de Sytue: uno de una cuenta externa no las lleva.
	if a.Etiquetar != nil && !esExterno(c.Ultimo.ID) {
		var etiquetar *ui.Boton
		etiquetar = botonCabecera(t.T("action.label", nil), EtiquetaD, ui.Fantasma, "", false, estrecha,
			iconos.ChevronAbajo, func() { x.menuEtiquetas().Mostrar(etiquetar) })
		objs = append(objs, etiquetar)
		x.botonEtiquetar = etiquetar
	}
	if a.Mover != nil && len(a.Destinos) > 0 {
		var mover *ui.Boton
		mover = botonCabecera(t.T("action.moveTo", nil), CarpetaD, ui.Fantasma, "", false, estrecha,
			iconos.ChevronAbajo, func() { x.menuMover().Mostrar(mover) })
		objs = append(objs, mover)
		x.botonMover = mover
	}
	return container.New(ui.Flujo{Hueco: HuecoTriajeLector}, objs...)
}

// AbrirEtiquetar y AbrirMover son los atajos «l» y «v»: pulsan el disparador del
// menú, que se abre donde se abriría con el ratón. Devuelven si había menú que abrir
// (sin hilo, en solo lectura o sin destinos, no lo hay).
func (x *Lector) AbrirEtiquetar() bool { return pulsarSiHay(x.botonEtiquetar) }

// AbrirMover es «v»; ver AbrirEtiquetar.
func (x *Lector) AbrirMover() bool { return pulsarSiHay(x.botonMover) }

func pulsarSiHay(b *ui.Boton) bool {
	if b == nil || b.AlPulsar == nil || fyne.CurrentApp().Driver().CanvasForObject(b) == nil {
		return false
	}
	b.AlPulsar()
	return true
}

// elementoGlifo es un `MenuItem` con uno de los glifos de Mail en `icon`: el hueco del
// icono lo tiñe la web de `text-text-muted` (o `text-danger-text` en el peligroso).
func elementoGlifo(texto, d string, peligro bool, alPulsar func()) *ui.ElementoMenu {
	token := "text-muted"
	if peligro {
		token = "danger-text"
	}
	e := ui.NuevoElementoMenu(texto, iconos.Ninguno, alPulsar)
	e.Glifo = Glifo(d, TallaGlifoLector, token, tema.De(fyne.CurrentApp().Settings().ThemeVariant()))
	e.TamIcono = TallaGlifoLector
	e.Peligro = peligro
	return e
}

// elementoBloquear es «Bloquear a …» con el nombre del remitente, si se ofrece: con
// acción puesta, fuera de una cuenta externa y no a uno mismo.
// elementoCopiarDireccion es «Copiar la dirección» del remitente: la de verdad, para
// pegarla en otro sitio, que el nombre visible no siempre la deja ver.
func (x *Lector) elementoCopiarDireccion() (*ui.ElementoMenu, bool) {
	remitente := remitenteDe(x.Conversacion)
	if remitente == "" || x.AlCopiarDireccion == nil {
		return nil, false
	}
	e := ui.NuevoElementoMenu(x.T.T("reader.copyAddress", nil), iconos.Copias,
		func() { x.AlCopiarDireccion(remitente) })
	e.TamIcono = TallaGlifoLector
	return e, true
}

func (x *Lector) elementoBloquear() (*ui.ElementoMenu, bool) {
	a := x.Acciones
	remitente := remitenteDe(x.Conversacion)
	if a.Bloquear == nil || a.PuedeBloquear == nil || esExterno(x.Conversacion.Ultimo.ID) ||
		!a.PuedeBloquear(remitente) {
		return nil, false
	}
	quien := cmp.Or(strings.TrimSpace(x.Conversacion.Ultimo.De[0].Nombre), remitente)
	return elementoGlifo(x.T.T("reader.block", i18n.Vars{"sender": quien}), SpamD, false,
		func() { a.Bloquear(x.Conversacion) }), true
}

// menuMas es «Más acciones»: marcar como no leído, crear una regla, spam y el borrado
// definitivo, en ese orden.
func (x *Lector) menuMas() *ui.Menu {
	t, a := x.T, x.Acciones
	els := []fyne.CanvasObject{
		// Es el mismo conmutador que el de la fila (`onMarkUnread={handleToggleSeen}`):
		// un hilo abierto ya está leído, así que lo deja sin leer y lo cierra.
		elementoGlifo(t.T("action.markUnread", nil), SinLeerD, false, func() {
			if a.Leido != nil {
				a.Leido(x.Conversacion)
			}
		}),
	}
	if x.AlCompartirEnRooms != nil {
		els = append(els, elementoGlifo(t.T("reader.shareToRooms", nil), ReenviarD, false, func() {
			if len(x.Conversacion.Mensajes) > 0 {
				x.AlCompartirEnRooms(x.Conversacion.Mensajes[0])
			}
		}))
	}
	if a.CrearRegla != nil && !esExterno(x.Conversacion.Ultimo.ID) {
		els = append(els, elementoGlifo(t.T("reader.createRule", nil), CarpetaD, false,
			func() { a.CrearRegla(x.Conversacion) }))
	}
	if e, hay := x.elementoCopiarDireccion(); hay {
		els = append(els, e)
	}
	// La búsqueda es la del servicio de Sytue: lo de una cuenta externa no está en ella,
	// y ofrecerlo daría «sin resultados» por respuesta.
	if remitente := remitenteDe(x.Conversacion); a.BuscarRemitente != nil && remitente != "" &&
		!esExterno(x.Conversacion.Ultimo.ID) {
		e := ui.NuevoElementoMenu(t.T("reader.searchFromSender", nil), iconos.Buscar,
			func() { a.BuscarRemitente(remitente) })
		e.TamIcono = TallaGlifoLector
		els = append(els, e)
	}
	// Ni en Spam, ni a un no-reply, ni a uno mismo (OfrecerContacto, libreta.go).
	if remitente := remitenteDe(x.Conversacion); a.FichaDe != nil && remitente != "" {
		if id, ofrecer := a.FichaDe(remitente); ofrecer && id != "" && a.VerEnContactos != nil {
			e := ui.NuevoElementoMenu(t.T("reader.viewInContacts", nil), iconos.UsuarioVisto,
				func() { a.VerEnContactos(id) })
			e.TamIcono = TallaGlifoLector
			els = append(els, e)
		} else if ofrecer && a.AnadirAContactos != nil {
			nombre := x.Conversacion.Ultimo.De[0].Nombre
			e := ui.NuevoElementoMenu(t.T("reader.addToContacts", nil), iconos.Usuario,
				func() { a.AnadirAContactos(nombre, remitente) })
			e.TamIcono = TallaGlifoLector
			els = append(els, e)
		}
	}
	// «Bloquear a …» va junto a «Marcar como spam», que es su pariente: esto además
	// manda a Spam lo que esa dirección escriba después.
	if e, hay := x.elementoBloquear(); hay {
		els = append(els, e)
	}
	els = append(els, x.elementosPosponer()...)
	if a.Spam != nil {
		els = append(els, elementoGlifo(t.T("action.spam", nil), SpamD, false,
			func() { a.Spam(x.Conversacion) }))
	}
	if a.Purgar != nil {
		els = append(els, elementoGlifo(t.T("action.purge", nil), PapeleraD, true,
			func() { a.Purgar(x.Conversacion) }))
	}
	m := ui.NuevoMenu(els...)
	m.Rotulo = t.T("reader.moreActions", nil)
	return m
}

// remitenteDe es quien escribe el hilo según la carpeta que se mira, como en «Crear una
// regla»: el último del hilo completo podría ser tu propia respuesta desde Enviados.
func remitenteDe(c Conversacion) string {
	if len(c.Ultimo.De) == 0 {
		return ""
	}
	return strings.TrimSpace(c.Ultimo.De[0].Correo)
}

// circulo es la ruta de un círculo que llena su caja con el trazo de 2 del glifo: el
// punto de color de una etiqueta dentro de un menú.
const circulo = "M12 1a11 11 0 1 0 0 22a11 11 0 1 0 0-22Z"

// menuEtiquetas es «Etiquetar la conversación»: una entrada por etiqueta con su punto,
// y el tic en las que el hilo ya lleva. Pulsar conmuta.
func (x *Lector) menuEtiquetas() *ui.Menu {
	t, a, c := x.T, x.Acciones, x.Conversacion
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	puestas := map[string]bool{}
	for _, e := range EtiquetasDe(c, a.Etiquetas) {
		puestas[e.ID] = true
	}
	var els []fyne.CanvasObject
	for _, e := range a.Etiquetas {
		on := puestas[e.ID]
		it := ui.NuevoElementoMenu(e.Nombre, iconos.Ninguno, func() { a.Etiquetar(x.Conversacion, e.ID, !on) })
		it.Glifo = iconos.TrazoDeRutaRelleno(circulo, puntoMenu, ColorEtiqueta(e.Color, v))
		it.TamIcono = puntoMenu
		it.Marcada, it.TintaTic = on, "primary-text"
		els = append(els, it)
	}
	if len(els) == 0 {
		// «No tienes etiquetas todavía.»: un párrafo, no una acción.
		els = append(els, container.New(ui.Margen{
			Izq: tema.Espacio(3), Der: tema.Espacio(3), Arr: tema.Espacio(2), Abj: tema.Espacio(2),
		}, ui.NuevoRotulo(t.T("reader.noLabelsYet", nil), tema.TextoSM, ui.Normal, "text-subtle")))
	}
	m := ui.NuevoMenu(els...)
	m.Rotulo = t.T("reader.labelConversation", nil)
	return m
}

// puntoMenu es el `h-2.5 w-2.5` del punto de una etiqueta en un menú.
const puntoMenu float32 = 2.5 * 0.25 * tema.Rem

// menuMover es «Mover a otra carpeta»: las carpetas destino con el glifo de carpeta.
func (x *Lector) menuMover() *ui.Menu {
	t, a := x.T, x.Acciones
	els := make([]fyne.CanvasObject, 0, len(a.Destinos))
	for _, c := range a.Destinos {
		els = append(els, elementoGlifo(RotuloCarpeta(t, c.Nombre, c.Rol), CarpetaD, false,
			func() { a.Mover(x.Conversacion, c) }))
	}
	m := ui.NuevoMenu(els...)
	m.Rotulo = t.T("reader.moveToFolder", nil)
	return m
}

// chips monta la fila de etiquetas del hilo. Nil si no lleva ninguna.
func (x *Lector) chips() *fyne.Container {
	if x.Acciones == nil {
		return nil
	}
	var objs []fyne.CanvasObject
	quitable := !x.SoloLectura && x.Acciones.Etiquetar != nil
	for _, e := range EtiquetasDe(x.Conversacion, x.Acciones.Etiquetas) {
		objs = append(objs, nuevoChipLector(x.T, e, quitable, func() {
			x.Acciones.Etiquetar(x.Conversacion, e.ID, false)
		}))
	}
	if len(objs) == 0 {
		return nil
	}
	return container.New(ui.Flujo{Hueco: HuecoChipsLector}, objs...)
}

// nuevoChipLector es una etiqueta aplicada, en la cabecera del lector. Sin `quitable`
// (solo lectura) no hay aspa: no hay nada que quitar.
func nuevoChipLector(t *i18n.Translator, e Etiqueta, quitable bool, alQuitar func()) *ui.Etiqueta {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	ch := ui.NuevaEtiqueta(e.Nombre)
	ch.Punto, ch.Apagada, ch.Densa, ch.Tope = ColorEtiqueta(e.Color, v), true, true, maxNombreChip
	if quitable {
		ch.ConAspa(t.T("reader.removeLabel", i18n.Vars{"label": e.Nombre}), alQuitar)
	}
	return ch
}
