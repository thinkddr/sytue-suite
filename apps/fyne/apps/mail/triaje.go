// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package mail

import (
	"cmp"
	"context"
	"errors"
	"maps"
	"net/url"
	"slices"
	"strings"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// # El triaje (MailApp.tsx, «Triaje»)
//
// Todo es OPTIMISTA, como en la web: la lista cambia al pulsar y la petición va
// detrás. Si el servidor rechaza, se deshace y se AVISA —un triaje que se revierte en
// silencio se lee como un fallo de la vista, y la persona repite la acción—.
//
// Y todo va por conversación ENTERA: destacar, leer, mover y etiquetar un hilo es
// hacerlo a todos sus mensajes en UNA llamada en lote (`id = ANY`), no N.

// carpetaDeRol es la carpeta del sistema con ese rol, si la cuenta la tiene.
func (b *Buzon) carpetaDeRol(rol string) (Carpeta, bool) {
	for _, m := range b.carpetas {
		if m.Role == rol {
			return carpetaDe(m), true
		}
	}
	return Carpeta{}, false
}

// carpetaVista es `currentBoxId`: la carpeta que se está mirando. En búsqueda no hay
// ninguna —los resultados vienen de todas— y con una etiqueta tampoco, que cruza
// carpetas.
func (b *Buzon) carpetaVista() string {
	if b.consulta != "" || b.etiquetaID != "" {
		return ""
	}
	return b.carpetaID
}

// rolVista es el rol de la carpeta que se está mirando, o vacío.
func (b *Buzon) rolVista() string {
	id := b.carpetaVista()
	for _, m := range b.carpetas {
		if m.ID == id && id != "" {
			return m.Role
		}
	}
	return ""
}

// ponerAcciones monta los manejadores del triaje para la vista de ahora. Es el bloque
// de `onArchive` … `moveTargets` de MailApp.tsx, regla por regla.
func (b *Buzon) ponerAcciones() {
	a := b.P.Acciones
	*a = Acciones{Etiquetas: a.Etiquetas}
	defer b.ponerLote()
	b.P.Bandeja.ConVaciar = b.consultaVacia() && b.botonVaciarVisible()
	if b.P.Bandeja.SoloLectura {
		return // un buzón compartido o sin red: nada que confirmar en el servidor
	}
	if b.externa != nil && b.consulta == "" {
		b.ponerAccionesExternas(a)
		return
	}
	actual := b.carpetaVista()
	a.Destacar, a.Leido = b.alternarDestacado, b.alternarLeido
	a.CrearRegla = func(c Conversacion) { b.crearReglaDesde(c) }
	// «Bloquear a …»: con carpeta Spam y fuera de ella (dentro, todo lo suyo ya está
	// ahí). La regla es de la cuenta de Sytue: una vista externa no llega hasta aquí.
	if spam, hay := b.carpetaDeRol("junk"); hay && b.rolVista() != "junk" {
		a.Bloquear = func(c Conversacion) { b.bloquear(c, spam) }
		a.PuedeBloquear = func(correo string) bool { return OfrecerBloqueo(correo, b.direccionesPropias()) }
	}
	a.BuscarRemitente = b.buscarRemitente
	a.Etiquetar = b.etiquetar
	b.ponerContactos(a)
	// Los destinos de triaje no incluyen la carpeta en la que ya se está: en la
	// Papelera no hay «Mover a la papelera».
	destino := func(rol string) func(Conversacion) {
		c, hay := b.carpetaDeRol(rol)
		if !hay || c.ID == actual {
			return nil
		}
		return func(x Conversacion) { b.moverConvs([]Conversacion{x}, c) }
	}
	a.Archivar, a.Papelera, a.Spam = destino("archive"), destino("trash"), destino("junk")
	rol := b.rolVista()
	// «No es spam» solo DENTRO de Spam, y devuelve a Recibidos.
	if entrada, hay := b.carpetaDeRol("inbox"); hay && rol == "junk" {
		a.NoSpam = func(x Conversacion) { b.moverConvs([]Conversacion{x}, entrada) }
	}
	// El borrado definitivo, solo donde no hay vuelta atrás que perder: Papelera y
	// Spam. Fuera, «borrar» es mover a la papelera.
	if rol == "trash" || rol == "junk" {
		a.Purgar = func(x Conversacion) { b.pedirPurga([]Conversacion{x}) }
	}
	a.Mover = func(x Conversacion, c Carpeta) { b.moverConvs([]Conversacion{x}, c) }
	for _, m := range b.carpetas {
		// Ni «Pospuestos»: lo que entra ahí sin posponerlo no tiene fecha de vuelta.
		if m.ID != actual && m.Role != "drafts" && m.Role != "snoozed" {
			a.Destinos = append(a.Destinos, carpetaDe(m))
		}
	}
	b.ponerPosponer(a)
}

// consultaVacia dice que se mira una carpeta y no resultados de búsqueda.
func (b *Buzon) consultaVacia() bool { return b.consulta == "" }

// avisar pone un aviso en la esquina, ya traducido.
func (b *Buzon) avisar(tono ui.TonoAviso, clave string, vars i18n.Vars) {
	b.P.Avisar(tono, b.P.T.T(clave, vars), "")
}

// idsDe son todos los mensajes de unas conversaciones.
func idsDe(cs []Conversacion) []string {
	var ids []string
	for _, c := range cs {
		for _, m := range c.Mensajes {
			ids = append(ids, m.ID)
		}
	}
	return ids
}

func (b *Buzon) alternarDestacado(c Conversacion) {
	b.marcarConvs([]Conversacion{c}, api.MailFlagFlagged, !c.Destacada, "error.star")
}

// alternarLeido es `handleToggleSeen`: un hilo con algo sin leer pasa entero a leído, y
// uno leído, a no leído. Marcar como no leído el que está abierto lo cierra: dejarlo a
// la vista sería contradictorio.
func (b *Buzon) alternarLeido(c Conversacion) {
	leer := c.SinLeer
	if !leer && b.hiloID == c.HiloID {
		b.cerrarHilo()
	}
	b.marcarConvs([]Conversacion{c}, api.MailFlagSeen, leer, "error.seen")
}

// marcarConvs es `setKeywordOnConvs`: pone o quita una marca a todos los mensajes, y si
// el servidor dice que no, cada uno vuelve a como ESTABA —no todos al contrario: en un
// hilo a medio leer, deshacer «leído» no puede dejar sin leer los que ya lo estaban—.
func (b *Buzon) marcarConvs(cs []Conversacion, marca string, poner bool, fallo string) {
	// Los de una cuenta POP3, mezclados en un buzón unificado, no se marcan.
	cs = slices.DeleteFunc(slices.Clone(cs), func(c Conversacion) bool { return b.sinAcciones(c.Ultimo.ID) })
	ids := idsDe(cs)
	if len(ids) == 0 {
		return
	}
	var tenian []string
	for _, c := range cs {
		for _, m := range c.Mensajes {
			if (marca == api.MailFlagSeen && m.Leido) || (marca == api.MailFlagFlagged && m.Destacado) {
				tenian = append(tenian, m.ID)
			}
		}
	}
	b.marcar(ids, marca, poner)
	dirs := b.direccionesDeCuentas()
	pedir(b, func(ctx context.Context) (struct{}, error) {
		return struct{}{}, b.marcarEnServidor(ctx, ids, marca, poner, dirs)
	}, func(_ struct{}, err error) {
		if err != nil {
			b.marcar(ids, marca, false)
			b.marcar(tenian, marca, true)
			b.avisar(ui.AvisoPeligro, fallo, nil)
			return
		}
		// SOLO con «leído»: es la única marca que mueve el número del lateral.
		if marca == api.MailFlagSeen {
			b.refrescarContadoresDe(ids)
		}
	})
}

// quitarDeLaVista saca unos mensajes de las dos listas y suelta el hilo abierto y la
// selección si eran de ellos. Devuelve cómo estaban, para deshacerlo.
func (b *Buzon) quitarDeLaVista(cs []Conversacion) (antes, antesR []api.MailMessage) {
	fuera := map[string]bool{}
	for _, id := range idsDe(cs) {
		fuera[id] = true
	}
	antes, antesR = slices.Clone(b.mensajes), slices.Clone(b.resultados)
	quitar := func(m api.MailMessage) bool { return fuera[m.ID] }
	b.mensajes = slices.DeleteFunc(slices.Clone(b.mensajes), quitar)
	b.resultados = slices.DeleteFunc(slices.Clone(b.resultados), quitar)
	if slices.ContainsFunc(cs, func(c Conversacion) bool { return c.HiloID == b.hiloID }) {
		b.cerrarHilo()
	}
	b.P.Bandeja.Marcadas = map[string]bool{}
	return antes, antesR
}

// moverConvs es `batchMoveConvs`: saca los hilos de la vista, los mueve en UNA llamada
// y refresca los números del lateral. Si falla, vuelven y se avisa con el nombre de la
// carpeta, que es lo que dice qué no ha pasado.
func (b *Buzon) moverConvs(cs []Conversacion, dest Carpeta) {
	ids := idsDe(cs)
	if len(ids) == 0 {
		return
	}
	vista := b.vista
	// El origen se calcula ANTES de mover: después, el mensaje ya dice el destino.
	origenes := origenesDeMovimiento(cs, b.carpetaVista(), dest.ID)
	antes, antesR := b.quitarDeLaVista(cs)
	b.pintarLista()
	carpeta := RotuloCarpeta(b.P.T, dest.Nombre, dest.Rol)
	pedir(b, func(ctx context.Context) (struct{}, error) {
		return struct{}{}, b.S.MoveMany(ctx, ids, dest.ID)
	}, func(_ struct{}, err error) {
		if err != nil {
			if vista == b.vista {
				b.mensajes, b.resultados = antes, antesR
				b.pintarLista()
			}
			b.avisar(ui.AvisoPeligro, "error.move", i18n.Vars{"folder": carpeta})
			return
		}
		b.refrescarContadores()
		if len(origenes) > 0 {
			b.avisarDeshacer(carpeta, origenes)
		}
	})
}

// origenesDeMovimiento es el de la web: los mensajes agrupados por la carpeta de la
// que SALEN, para que «Deshacer» devuelva cada uno a la suya. En una búsqueda o una
// etiqueta la vista cruza carpetas y un hilo tiene su respuesta en Enviados: volver
// todo a la vista sería dejarla en Recibidos. Sin carpeta en el sobre se cae a la que
// se mira; lo que ya estaba en el destino, o no tiene origen, no se toca.
func origenesDeMovimiento(cs []Conversacion, vista, destino string) map[string][]string {
	origenes := map[string][]string{}
	for _, c := range cs {
		for _, m := range c.Mensajes {
			origen := cmp.Or(m.CarpetaID, vista)
			if origen == "" || origen == destino {
				continue
			}
			origenes[origen] = append(origenes[origen], m.ID)
		}
	}
	return origenes
}

// avisarDeshacer es el aviso de un triaje confirmado, con «Deshacer»: archivar o
// tirar con una tecla se hace sin querer.
func (b *Buzon) avisarDeshacer(carpeta string, origenes map[string][]string) {
	t := b.P.T
	u := &triajeDeshacible{origenes}
	b.ultimoTriaje = u
	a := ui.NuevoAviso(ui.AvisoExito, t.T("triage.moved", i18n.Vars{"folder": carpeta}), "")
	a.RotuloCerrar = t.T("ui.dismissNotification", nil)
	a.RotuloAccion = t.T("compose.undo", nil)
	a.AlPulsarAccion = func() { b.deshacerTriaje(u) }
	b.P.Notificar(a)
}

// triajeDeshacible es un movimiento confirmado con su vuelta atrás. Va por puntero
// para saber si el que se deshace desde un aviso viejo es aún el último.
type triajeDeshacible struct{ origenes map[string][]string }

// deshacerTriaje es el «Deshacer» del aviso y «z»: una vez deshecho, «z» no lo repite.
func (b *Buzon) deshacerTriaje(u *triajeDeshacible) {
	if b.ultimoTriaje == u {
		b.ultimoTriaje = nil
	}
	b.deshacerMovimiento(u.origenes)
}

// deshacerMovimiento devuelve los mensajes a sus carpetas, una llamada en lote por
// carpeta, y vuelve a pedir la lista y los números: el hilo vuelve a donde estaba sin
// reconstruirlo a mano, que con varias carpetas sería adivinar. En una búsqueda lo que
// se ve son los resultados, así que se relanza la búsqueda, y la carpeta de debajo
// —que también perdió los movidos— se pide al salir de ella.
func (b *Buzon) deshacerMovimiento(origenes map[string][]string) {
	pedir(b, func(ctx context.Context) (struct{}, error) {
		return struct{}{}, b.devolverACarpetas(ctx, origenes)
	}, func(_ struct{}, err error) { b.trasDeshacer(err) })
}

// devolverACarpetas mueve cada grupo de mensajes a su carpeta, una llamada por carpeta.
func (b *Buzon) devolverACarpetas(ctx context.Context, origenes map[string][]string) error {
	for _, carpeta := range slices.Sorted(maps.Keys(origenes)) {
		if err := b.S.MoveMany(ctx, origenes[carpeta], carpeta); err != nil {
			return err
		}
	}
	return nil
}

// trasDeshacer avisa de cómo fue un «Deshacer» y vuelve a pedir lo que se ve.
func (b *Buzon) trasDeshacer(err error) {
	if err != nil {
		b.avisar(ui.AvisoPeligro, "error.undoMove", nil)
	} else {
		b.avisar(ui.AvisoExito, "triage.undone", nil)
	}
	if b.consulta != "" {
		b.listaCaducada = true
		b.buscarYa(b.consulta)
	} else {
		b.cargarLista()
	}
	b.refrescarContadores()
}

// ---- «Bloquear a …» (MAIL-13) ----

// OfrecerBloqueo es `ofrecerBloqueo` de la web (lib/bloqueoRemitente.ts): sin dirección
// o a uno mismo no —una regla con tu dirección mandaría a Spam tus respuestas en cada
// hilo—; a un no-reply sí, que los boletines sin baja son justo a quien se bloquea. Lo
// de Spam lo decide ponerAcciones, que ahí no pone Bloquear.
func OfrecerBloqueo(correo string, propias []string) bool {
	c := strings.ToLower(strings.TrimSpace(correo))
	if !strings.Contains(c, "@") {
		return false
	}
	return !slices.ContainsFunc(propias, func(p string) bool { return strings.ToLower(strings.TrimSpace(p)) == c })
}

// ReglaDeBloqueo es `reglaDeBloqueo`: la dirección EXACTA (no su dominio: bloquear a una
// persona no puede llevarse a Spam a toda su empresa) a la carpeta Spam, con el mismo
// rótulo del menú por nombre para que en «Reglas» se lea qué es y de dónde salió.
func ReglaDeBloqueo(t *i18n.Translator, correo, spam string) api.MailFilter {
	correo = strings.TrimSpace(correo)
	return api.MailFilter{
		Name:      t.T("reader.block", i18n.Vars{"sender": correo}),
		Enabled:   true,
		MatchFrom: correo,
		MoveTo:    spam,
	}
}

// bloquear es `bloquearDesde` de MailApp: optimista como el triaje —el hilo sale de la
// vista al pulsar—, crea la regla y mueve el hilo a Spam. Todo o nada: si el movimiento
// falla se borra la regla recién creada, porque un aviso de error con la regla puesta
// dejaría a la persona creyendo que no ha bloqueado a nadie. El aviso trae «Deshacer».
func (b *Buzon) bloquear(c Conversacion, spam Carpeta) {
	remitente := remitenteDe(c)
	cs := []Conversacion{c}
	ids := idsDe(cs)
	if remitente == "" || len(ids) == 0 {
		return
	}
	vista := b.vista
	origenes := origenesDeMovimiento(cs, b.carpetaVista(), spam.ID)
	antes, antesR := b.quitarDeLaVista(cs)
	b.pintarLista()
	regla := ReglaDeBloqueo(b.P.T, remitente, spam.ID)
	pedir(b, func(ctx context.Context) (string, error) {
		f, err := b.S.CreateFilter(ctx, regla)
		if err != nil {
			return "", err
		}
		if err := b.S.MoveMany(ctx, ids, spam.ID); err != nil {
			_ = b.S.DeleteFilter(ctx, f.ID)
			return "", err
		}
		return f.ID, nil
	}, func(id string, err error) {
		if err != nil {
			if vista == b.vista {
				b.mensajes, b.resultados = antes, antesR
				b.pintarLista()
			}
			b.avisar(ui.AvisoPeligro, "reader.blockFailed", nil)
			return
		}
		b.refrescarContadores()
		t := b.P.T
		a := ui.NuevoAviso(ui.AvisoExito, t.T("reader.blocked", i18n.Vars{"sender": remitente}), "")
		a.RotuloCerrar = t.T("ui.dismissNotification", nil)
		a.RotuloAccion = t.T("compose.undo", nil)
		a.AlPulsarAccion = func() { b.deshacerBloqueo(id, origenes) }
		b.P.Notificar(a)
	})
}

// deshacerBloqueo es «Deshacer» del bloqueo, entero: quita la regla y devuelve cada
// mensaje a SU carpeta. Se intentan las dos cosas aunque una falle —que quede la regla
// sin el hilo, o al revés, es peor que medio deshecho avisado—.
func (b *Buzon) deshacerBloqueo(regla string, origenes map[string][]string) {
	pedir(b, func(ctx context.Context) (struct{}, error) {
		errRegla := b.S.DeleteFilter(ctx, regla)
		return struct{}{}, errors.Join(errRegla, b.devolverACarpetas(ctx, origenes))
	}, func(_ struct{}, err error) { b.trasDeshacer(err) })
}

// etiquetar es `handleToggleLabel`: pone o quita la etiqueta a TODO el hilo. Si se
// quita la que se está mirando, el hilo sale de la lista.
func (b *Buzon) etiquetar(c Conversacion, etiqueta string, poner bool) {
	ids := idsDe([]Conversacion{c})
	if len(ids) == 0 {
		return
	}
	vista := b.vista
	antes, antesR := slices.Clone(b.mensajes), slices.Clone(b.resultados)
	b.marcar(ids, etiqueta, poner)
	if !poner && b.etiquetaID == etiqueta {
		b.quitarDeLaVista([]Conversacion{c})
		b.pintarLista()
	}
	pedir(b, func(ctx context.Context) (struct{}, error) {
		return struct{}{}, b.S.ApplyLabel(ctx, etiqueta, ids, poner)
	}, func(_ struct{}, err error) {
		if err != nil {
			if vista == b.vista {
				b.mensajes, b.resultados = antes, antesR
				b.pintarLista()
			}
			b.avisar(ui.AvisoPeligro, "error.label", nil)
			return
		}
		b.cargarEtiquetas() // los contadores del lateral
	})
}

// ---- El borrado definitivo ----

// pedirPurga abre la confirmación. Se pregunta SIEMPRE, también con un solo mensaje:
// no hay papelera detrás de la papelera. Y dice CUÁNTOS van a caer, que es lo que no
// se ve al borrar un hilo entero desde el lector.
func (b *Buzon) pedirPurga(cs []Conversacion) {
	n := len(idsDe(cs))
	if n == 0 {
		return
	}
	lienzo := b.P.Lienzo()
	if lienzo == nil {
		return
	}
	t := b.P.T
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	cuerpo := ui.NuevoParrafo(t.T("purge.body", i18n.Vars{"count": n}), tema.TextoSM, ui.Normal, "text", v).
		A(AnchoCuerpoDialogo(ui.DialogoSM, lienzo.Size().Width))
	var d *ui.Dialogo
	var confirmar *ui.Boton
	d = ui.Confirmacion(t.T("purge.title", nil), t.T("common.cancel", nil), t.T("purge.confirm", nil), true,
		cuerpo, func() {
			if confirmar == nil || !confirmar.Cargando {
				d.Cerrar()
			}
		}, func() { b.purgar(cs, d, confirmar) })
	d.RotuloCerrar = t.T("ui.close", nil)
	confirmar = botonConfirmar(d)
	d.Mostrar(lienzo)
}

// purgar es `confirmPurge`: borra de verdad —el servicio no tiene lote, así que uno a
// uno, como hace el BFF de la web— y SOLO al confirmarse quita de la vista. Un 404
// cuenta como hecho: el mensaje ya no está, que es lo que se pedía.
func (b *Buzon) purgar(cs []Conversacion, d *ui.Dialogo, boton *ui.Boton) {
	ids := idsDe(cs)
	if boton != nil {
		boton.Cargando = true
		boton.Refresh()
	}
	pedir(b, func(ctx context.Context) (int, error) {
		hechos := 0
		for _, id := range ids {
			if err := b.S.Delete(ctx, id); err != nil && !noEsta(err) {
				return hechos, err
			}
			hechos++
		}
		return hechos, nil
	}, func(hechos int, err error) {
		if d != nil {
			d.Cerrar()
		}
		if err != nil {
			b.avisar(ui.AvisoPeligro, "purge.failed", nil)
			b.cargarLista() // lo que sí cayó ya no está: se vuelve a pedir la verdad
			return
		}
		b.quitarDeLaVista(cs)
		b.avisar(ui.AvisoExito, "purge.done", i18n.Vars{"count": hechos})
		// Y se recarga: es lo que refresca el medidor de espacio, que es por lo que
		// casi todo el mundo llega hasta aquí.
		b.cargarLista()
		b.refrescarContadores()
		b.cargarCuota()
	})
}

// botonConfirmar es el botón de la acción del pie de una Confirmacion: el ÚLTIMO, que
// es donde lo pone `ui.Confirmacion`. Hace falta para ponerlo a cargar mientras
// trabaja, que es el `loading={purging}` de la web.
func botonConfirmar(d *ui.Dialogo) *ui.Boton {
	pie, ok := d.Pie.(*fyne.Container)
	if !ok || len(pie.Objects) == 0 {
		return nil
	}
	b, _ := pie.Objects[len(pie.Objects)-1].(*ui.Boton)
	return b
}

// AnchoCuerpoDialogo es el ancho de reparto del cuerpo de un diálogo en un lienzo de
// ese ancho: el tope de su talla, el lienzo menos el `p-4` que lo centra o, en el
// teléfono, la hoja a todo el ancho; y dentro de eso los filetes y el `px-6` del cuerpo.
func AnchoCuerpoDialogo(t ui.TallaDialogo, lienzo float32) float32 {
	d := ui.NuevoDialogo("", nil).Con(t, nil)
	return ui.AnchoCuerpoDialogo(d, fyne.NewSize(lienzo, 0))
}

// ---- Darse de baja (List-Unsubscribe) ----

// consultarBaja pregunta si el último mensaje del hilo ofrece darse de baja. Solo
// booleanos: la URL o la dirección de baja nunca llegan al cliente, las usa el
// servidor. Sin respuesta, no hay botón.
func (b *Buzon) consultarBaja(id string) {
	b.P.Lector.Baja, b.P.Lector.DandoDeBaja = false, false
	// Uno de una cuenta externa no está en Sytue: su baja no se puede preguntar aquí.
	if id == "" || b.P.Bandeja.SoloLectura || esExterno(id) {
		return
	}
	hilo := b.hiloID
	pedir(b, func(ctx context.Context) (api.MailBaja, error) {
		return b.S.Unsubscribe(ctx, id)
	}, func(baja api.MailBaja, err error) {
		if err != nil || hilo != b.hiloID {
			return
		}
		b.P.Lector.Baja = baja.Hay()
		b.P.Lector.Refresh()
	})
}

// darseDeBaja es `handleUnsubscribe`: el servidor elige el método seguro y contesta
// en qué quedó, y eso es lo que se dice. Hecha o pedida, el botón se va.
func (b *Buzon) darseDeBaja(m Mensaje) {
	l := b.P.Lector
	if l.DandoDeBaja {
		return
	}
	l.DandoDeBaja = true
	l.Refresh()
	pedir(b, func(ctx context.Context) (string, error) {
		return b.S.DoUnsubscribe(ctx, m.ID)
	}, func(estado string, err error) {
		l.DandoDeBaja = false
		switch {
		case err != nil || estado == api.BajaFallida:
			b.avisar(ui.AvisoPeligro, "reader.unsubscribeFailed", nil)
		case estado == api.BajaEnviada:
			l.Baja = false
			b.avisar(ui.AvisoExito, "reader.unsubscribeSent", nil)
		default:
			l.Baja = false
			b.avisar(ui.AvisoExito, "reader.unsubscribeDone", nil)
		}
		l.Refresh()
	})
}

// ---- Crear un evento desde el correo (el intent del Hub) ----

// Intents es lo que Sytue Mail le pide al Hub. Lo cumple *api.Hub.
type Intents interface {
	ResolveIntent(ctx context.Context, tipo string, params map[string]string) (api.HubIntent, error)
}

var _ Intents = (*api.Hub)(nil)

// crearEvento es `handleCreateEvent`: los invitados son los participantes del mensaje
// (remitente y destinatarios, sin repetir), y el Hub compone el enlace a Sytue
// Calendar con el asunto, el extracto y ellos. Se abre en SU app; sin app instalada,
// en la web, que es lo que la web hace con una pestaña nueva.
func (b *Buzon) crearEvento(m Mensaje) {
	l := b.P.Lector
	if l.CreandoEvento || b.Hub == nil {
		return
	}
	var invitados []string
	for _, d := range append(slices.Clone(m.De), m.Para...) {
		if e := strings.ToLower(strings.TrimSpace(d.Correo)); e != "" && !slices.Contains(invitados, e) {
			invitados = append(invitados, e)
		}
	}
	params := map[string]string{"message_id": m.ID, "subject": m.Asunto, "body": m.Extracto}
	if len(invitados) > 0 {
		params["attendees"] = strings.Join(invitados, ",")
	}
	l.CreandoEvento = true
	l.Refresh()
	pedir(b, func(ctx context.Context) (api.HubIntent, error) {
		return b.Hub.ResolveIntent(ctx, "create-event-from-mail", params)
	}, func(r api.HubIntent, err error) {
		l.CreandoEvento = false
		l.Refresh()
		switch {
		case err != nil:
			b.avisar(ui.AvisoPeligro, "reader.eventFailed", nil)
		case r.URL != "" && b.Abridor != nil:
			if api.AbrirURL(b.Abridor, cmp.Or(r.Target, "calendar"), r.URL) != nil {
				b.avisar(ui.AvisoPeligro, "reader.eventFailed", nil)
			}
		default:
			b.avisar(ui.AvisoExito, "reader.eventCreated", nil)
		}
	})
}

// compartirEnRooms es `handleShareToRooms`: la pantalla de compartir de Sytue Rooms
// con el asunto y el ENLACE del correo como mensaje propuesto —nunca su cuerpo: la
// sala puede tener gente que no debería leerlo—; allí se elige la sala y se publica.
// Como en Drive nativo, la otra app se llama por su esquema (`api.RutaCompartir`) y
// sin ella instalada se cae a la misma página en la web; el Hub solo compondría ese
// enlace, y la autorización la aplica Rooms al publicar.
func (b *Buzon) compartirEnRooms(m Mensaje) {
	if b.Abridor == nil || m.ID == "" {
		return
	}
	apex, org := cmp.Or(b.Apex, api.ApexProd), ""
	if b.Org != nil {
		org = b.Org()
	}
	ruta := api.RutaCompartir(api.Compartir{Origen: "mail", ID: m.ID, Texto: TextoParaRooms(
		cmp.Or(m.Asunto, b.P.T.T("common.noSubject", nil)), api.AppURL(apex, "mail", org, "/messages/"+url.PathEscape(m.ID)))})
	if api.AbrirEn(b.Abridor, apex, "rooms", org, ruta) != nil {
		b.avisar(ui.AvisoPeligro, "reader.roomsFailed", nil)
	}
}

// TextoParaRooms es `textoParaRooms` de la web: el asunto y, debajo, el enlace.
func TextoParaRooms(asunto, enlace string) string { return asunto + "\n" + enlace }

// buscarRemitente es `onSearchSender` de MailApp: la consulta `from:` de esa dirección
// en el buscador, que es quien busca (y cierra el hilo, como al teclear).
func (b *Buzon) buscarRemitente(correo string) {
	b.P.Bandeja.PonerConsulta(ComponerConsulta(OpcionesBusqueda{De: correo}))
}

// crearReglaDesde es `crearReglaDesde` de MailApp: el asistente con el remitente del
// último mensaje como condición y como nombre.
func (b *Buzon) crearReglaDesde(c Conversacion) *gestorReglas {
	de := remitenteDe(c)
	return b.verReglas(&BorradorRegla{Nombre: de, De: de})
}
