// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package certipdf

import (
	"cmp"
	"context"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

/*
PEDIR FIRMA (`PedirFirma.tsx` y la franja de `PdfApp.tsx`, ADR 0051 §7): a otra persona,
por su correo o su @usuario, sobre el PDF abierto. Quien la recibe ve una franja al
abrirlo —«X te pide firmar este documento», con Rechazar y Firmar— y al firmar, la copia
firmada se entrega sola; el servicio la baja y comprueba que lleva una firma nueva.

Si es de OTRA organización (ADR 0057), el servicio no crea nada hasta que quien pide lo
confirma: el diálogo explica que el PDF sale por un enlace de Drive de solo lectura y, al
confirmar, se comparte y se pide en el mismo paso. Quien la recibe se trae el PDF a su
unidad desde la portada, y quien la pidió, la copia firmada.
*/

// buscarPeticion mira si alguien me ha pedido firmar este PDF. Un fallo no estorba a
// abrirlo: sin la lista no hay franja, y ya.
func (p *Pantalla) buscarPeticion(doc *Documento) {
	if doc == nil || doc.pdf == nil {
		return
	}
	go func() {
		ctx, parar := context.WithTimeout(p.ctx, plazo)
		defer parar()
		ps, err := doc.pdf.PeticionesFirma(ctx)
		fyne.Do(func() {
			if err != nil || p.doc != doc {
				return
			}
			if pend, ok := ps.PendienteDe(doc.Ruta()); ok {
				p.peticion = &pend
				p.Recomponer()
			}
		})
	}()
}

// entregarFirma da por firmada la petición con el PDF firmado de `ruta`, que ya está en
// Drive.
func (p *Pantalla) entregarFirma(pet api.PDFPeticionFirma, ruta string) {
	doc := p.doc
	if doc == nil {
		return
	}
	go func() {
		ctx, parar := context.WithTimeout(p.ctx, plazo)
		defer parar()
		_, err := doc.pdf.EntregarFirma(ctx, pet.ID, ruta)
		fyne.Do(func() {
			if p.doc != doc {
				return
			}
			nombre := i18n.Vars{"name": pet.NombrePide}
			if err != nil {
				p.sinEntregar = ruta
				p.Error = p.tv("pdf.signatureRequest.notDelivered", nombre) + " " + p.mensajeDeError(err, "pdf.error.apply")
			} else {
				p.peticion, p.sinEntregar = nil, ""
				p.Aviso = p.tv("pdf.signatureRequest.delivered", nombre)
			}
			p.Recomponer()
		})
	}()
}

// rechazarFirma es «Rechazar» de la franja.
func (p *Pantalla) rechazarFirma(pet api.PDFPeticionFirma) {
	doc := p.doc
	if doc == nil {
		return
	}
	go func() {
		ctx, parar := context.WithTimeout(p.ctx, plazo)
		defer parar()
		_, err := doc.pdf.RechazarFirma(ctx, pet.ID)
		fyne.Do(func() {
			if p.doc != doc {
				return
			}
			if err != nil {
				p.Error = p.mensajeDeError(err, "pdf.error.apply")
			} else {
				p.peticion, p.Aviso = nil, p.t("pdf.signatureRequest.declined")
			}
			p.Recomponer()
		})
	}()
}

// bloquearFirma es «Bloquear» de la franja, ya confirmado: a quien pidió no se le dice.
func (p *Pantalla) bloquearFirma(pet api.PDFPeticionFirma) {
	doc := p.doc
	if doc == nil {
		return
	}
	go func() {
		ctx, parar := context.WithTimeout(p.ctx, plazo)
		defer parar()
		err := doc.pdf.BloquearFirma(ctx, pet.ID)
		fyne.Do(func() {
			if p.doc != doc {
				return
			}
			p.bloqueando = false
			if err != nil {
				p.Error = p.mensajeDeError(err, "pdf.error.apply")
			} else {
				p.peticion, p.Aviso = nil, p.tv("pdf.signatureRequest.blocked", i18n.Vars{"name": pet.NombrePide})
			}
			p.Recomponer()
		})
	}()
}

// franjaPeticion es la de `bg-info-bg text-info-text`: quién pide firmar, su mensaje,
// «Volver a entregar» si el firmado no llegó, «Rechazar» y «Firmar».
func (p *Pantalla) franjaPeticion() fyne.CanvasObject {
	if p.peticion == nil {
		return nil
	}
	pet := *p.peticion
	texto := p.tv("pdf.signatureRequest.banner", i18n.Vars{"name": pet.NombrePide})
	if m := strings.TrimSpace(pet.Mensaje); m != "" {
		texto += " · " + m
	}
	if pet.Externo {
		texto += " · " + p.tv("pdf.signatureRequest.externalNote", i18n.Vars{"name": pet.NombrePide})
	}
	var acciones []fyne.CanvasObject
	if pet.Externo && p.bloqueando {
		// La confirmación va en la propia franja, como en la web.
		texto = p.tv("pdf.signatureRequest.blockConfirm", i18n.Vars{"name": pet.NombrePide})
		bloquear := accionDeFranja(p.t("pdf.signatureRequest.block"), "info-text", true, func() { p.bloquearFirma(pet) })
		volver := accionDeFranja(p.t("pdf.cancel"), "info-text", false, func() { p.bloqueando = false; p.Recomponer() })
		return nuevaFranja("info-bg", "info-text", texto, p.anchoVentana, bloquear, volver)
	}
	if pet.Externo {
		acciones = append(acciones, accionDeFranja(p.t("pdf.signatureRequest.block"), "info-text", false,
			func() { p.bloqueando = true; p.Recomponer() }))
	}
	if p.sinEntregar != "" {
		ruta := p.sinEntregar
		acciones = append(acciones, accionDeFranja(p.t("pdf.signatureRequest.retry"), "info-text", false,
			func() { p.entregarFirma(pet, ruta) }))
	}
	acciones = append(acciones, accionDeFranja(p.t("pdf.signatureRequest.decline"), "info-text", false,
		func() { p.rechazarFirma(pet) }))
	firmar := NuevaAccion(p.t("pdf.signatureRequest.sign"), func() { p.abrirDialogo(DialogoFirmar) })
	firmar.PX, firmar.PY = tema.Espacio(3), tema.Espacio(1)
	firmar.Fondo, firmar.FondoEncima, firmar.Tinta = "primary", "primary-hover", "on-primary"
	firmar.Inerte = p.Trabajando
	f := nuevaFranja("info-bg", "info-text", texto, p.anchoVentana, append(acciones, firmar)...)
	f.entreAcciones = tema.Espacio(2)
	return f
}

// dialogoPedirFirma es `PedirFirma`: el correo, el mensaje, y cómo van las pedidas de
// este documento.
type dialogoPedirFirma struct {
	a            *dialogoAbierto
	email        *ui.Entrada
	mensaje      *ui.AreaTexto
	error, hecho string
	enviando     bool
	pedidas      []api.PDFPeticionFirma
	cargadas     bool
	// fuera es el nombre de la persona cuando es de otra organización, mientras se
	// pregunta si compartir el PDF por Drive, y confirmar lo que se manda si dice que sí.
	fuera, confirmar string
}

func nuevoDialogoPedirFirma(a *dialogoAbierto) *dialogoPedirFirma {
	d := &dialogoPedirFirma{a: a, email: entradaDialogo("", 0)}
	d.email.AlCambiar(func(string) {
		if d.fuera != "" {
			d.fuera, d.confirmar = "", ""
			d.a.rehacer()
		}
	})
	d.mensaje = ui.NuevaAreaTexto("")
	d.mensaje.Filas = 3
	d.mensaje.ConPropia(campoDialogo())
	d.mensaje.Radio = tema.RadioMD
	d.recargar()
	return d
}

// recargar trae las pedidas de ESTE documento.
func (d *dialogoPedirFirma) recargar() {
	doc := d.a.p.doc
	if doc == nil || doc.pdf == nil {
		return
	}
	go func() {
		ctx, parar := context.WithTimeout(d.a.p.ctx, plazo)
		defer parar()
		ps, err := doc.pdf.PeticionesFirma(ctx)
		fyne.Do(func() {
			d.cargadas, d.pedidas = true, nil
			if err == nil {
				for _, x := range ps.Pedidas {
					if x.Ruta == doc.Ruta() {
						d.pedidas = append(d.pedidas, x)
					}
				}
			}
			d.a.rehacer()
		})
	}()
}

func (d *dialogoPedirFirma) enviar() {
	doc := d.a.p.doc
	email := strings.TrimSpace(d.email.Texto())
	if doc == nil || email == "" || d.enviando {
		return
	}
	d.enviando, d.error, d.hecho = true, "", ""
	d.a.rehacer()
	mensaje, confirmar, nombre := strings.TrimSpace(d.mensaje.Texto()), d.confirmar, cmp.Or(d.fuera, email)
	go func() {
		ctx, parar := context.WithTimeout(d.a.p.ctx, plazo)
		defer parar()
		_, err := doc.pdf.PedirFirma(ctx, doc.Ruta(), email, mensaje, confirmar)
		fyne.Do(func() {
			d.enviando = false
			if quien, conf, ok := api.PDFFirmaDeFuera(err); ok {
				d.fuera, d.confirmar = cmp.Or(quien, email), conf
			} else if err != nil {
				d.error = d.a.p.mensajeDeError(err, "pdf.error.apply")
			} else {
				d.hecho = d.a.p.tv("pdf.dialog.pedirFirma.sent", i18n.Vars{"name": nombre})
				d.email.PonerTexto("")
				d.mensaje.PonerTexto("")
				d.fuera, d.confirmar = "", ""
				d.recargar()
			}
			d.a.rehacer()
		})
	}()
}

// traer se trae la copia firmada por alguien de otra organización y la abre.
func (d *dialogoPedirFirma) traer(id string) {
	doc := d.a.p.doc
	if doc == nil {
		return
	}
	go func() {
		ctx, parar := context.WithTimeout(d.a.p.ctx, plazo)
		defer parar()
		x, err := doc.pdf.TraerFirmado(ctx, id)
		fyne.Do(func() {
			if err != nil {
				d.error = d.a.p.mensajeDeError(err, "pdf.error.apply")
				d.a.rehacer()
				return
			}
			d.a.cerrar()
			if d.a.p.AlAbrirRuta != nil {
				d.a.p.AlAbrirRuta(x.RutaFirmada)
			}
		})
	}()
}

func (d *dialogoPedirFirma) cancelar(id string) {
	doc := d.a.p.doc
	if doc == nil {
		return
	}
	go func() {
		ctx, parar := context.WithTimeout(d.a.p.ctx, plazo)
		defer parar()
		err := doc.pdf.CancelarFirma(ctx, id)
		fyne.Do(func() {
			if err != nil {
				d.error = d.a.p.mensajeDeError(err, "pdf.error.apply")
				d.a.rehacer()
				return
			}
			d.recargar()
		})
	}()
}

func (d *dialogoPedirFirma) vista(ancho float32) fyne.CanvasObject {
	a := d.a
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	d.email.Ancho, d.mensaje.Ancho = ancho, ancho
	partes := []fyne.CanvasObject{
		ui.NuevoParrafo(a.t("pdf.dialog.pedirFirma.help"), tema.TextoXS, ui.Normal, "text-muted", v).A(ancho),
		conRotulo(a.t("pdf.dialog.pedirFirma.email"), d.email),
		conRotulo(a.t("pdf.dialog.pedirFirma.message"), d.mensaje),
	}
	if d.error != "" {
		partes = append(partes, ui.NuevoParrafo(d.error, tema.TextoXS, ui.Normal, "danger-text", v).A(ancho))
	}
	if d.hecho != "" {
		partes = append(partes, ui.NuevoParrafo(d.hecho, tema.TextoXS, ui.Normal, "success-text", v).A(ancho))
	}
	cancelar := a.cerrar
	rotulo := a.t("pdf.dialog.pedirFirma.send")
	if d.fuera != "" {
		// `bg-info-bg text-info-text rounded-md p-3`: quién es y qué va a pasar.
		dentro := ancho - 2*tema.Espacio(3)
		aviso := container.New(ui.Columna{Hueco: tema.Espacio(1)},
			ui.NuevoParrafo(a.p.tv("pdf.dialog.pedirFirma.external.title", i18n.Vars{"name": d.fuera}), tema.TextoXS, ui.SemiNegrita, "info-text", v).A(dentro),
			ui.NuevoParrafo(a.t("pdf.dialog.pedirFirma.external.help"), tema.TextoXS, ui.Normal, "info-text", v).A(dentro))
		fondo := canvas.NewRectangle(tema.Color("info-bg", v))
		fondo.CornerRadius = tema.RadioMD
		partes = append(partes, container.NewStack(fondo, container.New(ui.Margen{Izq: tema.Espacio(3), Der: tema.Espacio(3), Arr: tema.Espacio(3), Abj: tema.Espacio(3)}, aviso)))
		cancelar = func() { d.fuera, d.confirmar = "", ""; a.rehacer() }
		rotulo = a.t("pdf.dialog.pedirFirma.external.confirm")
	}
	cerrar := botonPanel(a.t("pdf.cancel"), tema.TextoXS, tema.Espacio(1.5), cancelar)
	cerrar.PX = tema.Espacio(3)
	if d.enviando {
		rotulo = a.t("pdf.working")
	}
	enviar := botonPrimario(rotulo, tema.TextoXS, tema.Espacio(3), tema.Espacio(1.5), d.enviar)
	enviar.Inerte = d.enviando
	a.enviar = d.enviar
	fila := container.New(ui.Fila{Hueco: tema.Espacio(2)}, cerrar, enviar)
	partes = append(partes, container.New(ui.Margen{Izq: ancho - fila.MinSize().Width}, fila))

	lista := []fyne.CanvasObject{ui.NuevoRotulo(a.t("pdf.dialog.pedirFirma.list"), tema.TextoXS, ui.SemiNegrita, "text")}
	if d.cargadas && len(d.pedidas) == 0 {
		lista = append(lista, notaDialogo(a.t("pdf.dialog.pedirFirma.none"), ancho))
	}
	for _, x := range d.pedidas {
		lista = append(lista, d.fila(x))
	}
	partes = append(partes, container.New(ui.Margen{Arr: tema.Espacio(3)},
		container.New(ui.Columna{Hueco: tema.Espacio(1.5)}, lista...)))
	return container.New(ui.Columna{Hueco: tema.Espacio(3)}, partes...)
}

// fila es una pedida: a quién, cómo va y lo que se puede hacer con ella.
func (d *dialogoPedirFirma) fila(x api.PDFPeticionFirma) fyne.CanvasObject {
	a := d.a
	estado := ui.NuevoRotulo(a.t("pdf.signatureRequest.status."+x.Estado), tema.TextoXS, ui.Normal, "text-muted")
	acciones := []fyne.CanvasObject{estado}
	switch x.Estado {
	case api.PDFFirmaFirmada:
		if x.Externo && x.RutaFirmada == "" {
			id := x.ID
			acciones = append(acciones, accionDeFranja(a.t("pdf.dialog.pedirFirma.fetchSigned"), "text", false, func() { d.traer(id) }))
		}
		if x.RutaFirmada != "" {
			ruta := x.RutaFirmada
			acciones = append(acciones, accionDeFranja(a.t("pdf.dialog.pedirFirma.openSigned"), "text", false, func() {
				a.cerrar()
				if a.p.AlAbrirRuta != nil {
					a.p.AlAbrirRuta(ruta)
				}
			}))
		}
	case api.PDFFirmaPendiente:
		id := x.ID
		acciones = append(acciones, accionDeFranja(a.t("pdf.dialog.pedirFirma.cancel"), "text", false, func() { d.cancelar(id) }))
	}
	derecha := container.New(ui.Fila{Hueco: tema.Espacio(2)}, acciones...)
	quien := ui.NuevoRotulo(x.CorreoDestino, tema.TextoXS, ui.Normal, "text")
	return container.NewBorder(nil, nil, quien, derecha)
}
