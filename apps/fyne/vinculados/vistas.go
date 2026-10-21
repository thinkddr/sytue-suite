// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package vinculados

import (
	"cmp"
	"slices"
	"strings"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

func (p *Panel) proveedorDisponible(key string) bool {
	for _, provider := range p.cuentas.Providers {
		if provider.Provider == key {
			ready, _ := capacidad(provider, p.servicio)
			return ready
		}
	}
	return false
}

func (p *Panel) cuerpoCuentas() []fyne.CanvasObject {
	var cuerpo []fyne.CanvasObject
	for _, c := range p.cuentas.Items {
		if c.Provider != "gmail" && c.Provider != "microsoft" && c.Provider != "yahoo" {
			continue
		}
		f := func() { p.elegirCuenta(c) }
		label := cmp.Or(c.Address, c.Name)
		reconectar := !c.Connected || !slices.Contains(c.Services, p.servicio)
		if reconectar {
			label += " · " + p.t("externalMail.connect")
			f = func() { p.prepararConsentimiento(c.Provider, c.InstanceID) }
		}
		card := p.eleccion(api.MailProviderNames[c.Provider], label, f)
		if reconectar {
			card.Inerte = p.ocupado || !p.proveedorDisponible(c.Provider)
		}
		cuerpo = append(cuerpo, card)
	}
	return append(cuerpo, p.boton(p.t("externalMail.connect"), func() {
		p.pendienteProvider, p.pendienteID, p.esperadaID = "", "", ""
		p.paso = "proveedores"
		p.pintar()
	}), p.boton(p.t("nav.refresh"), func() { p.cargarCuentas(p.esperadaID) }))
}

func (p *Panel) cuerpoProveedores() []fyne.CanvasObject {
	service := map[string]string{"calendar": "dataflow.item.events", "contacts": "dataflow.item.contacts", "notes": "app.notes.description"}[p.servicio]
	cuerpo := []fyne.CanvasObject{p.texto(p.t("linkedServices.choose")), p.texto(p.t("dataflow.item.mail") + " · " + p.t(service))}
	for _, provider := range p.cuentas.Providers {
		if provider.Authorization != "oauth2" {
			continue
		}
		ready, reason := capacidad(provider, p.servicio)
		card := p.eleccion(api.MailProviderNames[provider.Provider], "", func() { p.prepararConsentimiento(provider.Provider, "") })
		card.Inerte = p.ocupado || !ready
		if !ready {
			key := "linkedServices.notSupported"
			if !provider.Available || len(provider.Services) == 0 {
				key = "externalMail.unavailable"
			} else if strings.Contains(reason, "admin") || strings.Contains(reason, "keep") {
				key = "linkedServices.keepAdmin"
			}
			card.Descripcion = p.t(key)
		}
		cuerpo = append(cuerpo, card)
	}
	return cuerpo
}

func (p *Panel) prepararConsentimiento(provider, instance string) {
	p.consentProvider, p.consentInstance, p.volverConsentimiento = provider, instance, p.paso
	p.consentServices = p.serviciosConsentir(provider, instance)
	p.paso = "consentimiento"
	p.pintar()
}

// El consentimiento de Mail reemplaza la selección. Otra app sólo puede ampliarla;
// retirar un servicio sigue siendo una acción explícita en el asistente de Mail.
func (p *Panel) serviciosConsentir(provider, instance string) []string {
	result := []string{p.servicio}
	accounts := append(slices.Clone(p.cuentas.Items), p.cuenta)
	for _, account := range accounts {
		if account.InstanceID != instance || account.Provider != provider || instance == "" {
			continue
		}
		for _, service := range account.Services {
			if service != "mail" && service != "calendar" && service != "contacts" && service != "notes" {
				continue
			}
			if !slices.Contains(result, service) {
				result = append(result, service)
			}
		}
	}
	return result
}

func (p *Panel) cuerpoConsentimiento() []fyne.CanvasObject {
	keys := map[string]string{"mail": "dataflow.item.mail", "calendar": "dataflow.item.events", "contacts": "dataflow.item.contacts", "notes": "app.notes.description"}
	cuerpo := []fyne.CanvasObject{p.texto(api.MailProviderNames[p.consentProvider]), p.texto(p.t("externalMail.oauthBody")), p.texto(p.t("linkedServices.services"))}
	for _, service := range p.consentServices {
		cuerpo = append(cuerpo, p.texto(p.t(keys[service])))
	}
	return append(cuerpo, p.boton(p.t("externalMail.authorize"), func() { p.conectar(p.consentProvider, p.consentInstance) }))
}

func (p *Panel) cuerpoColecciones() []fyne.CanvasObject {
	var cuerpo []fyne.CanvasObject
	for _, c := range p.colecciones {
		cuerpo = append(cuerpo, p.eleccion(c.Name, "", func() { p.coleccion = c; p.cargarItems(false) }))
	}
	if !p.ocupado && len(p.colecciones) == 0 {
		cuerpo = append(cuerpo, p.texto(p.t("ui.noResults")))
	}
	return append(cuerpo, p.boton(p.t("nav.refresh"), func() { p.elegirCuenta(p.cuenta) }))
}

func (p *Panel) cuerpoItems() []fyne.CanvasObject {
	cuerpo := []fyne.CanvasObject{p.texto(p.coleccion.Name)}
	if p.servicio == "calendar" {
		cuerpo = append(cuerpo, p.texto(i18n.FormatMonthYear(p.tr.Locale(), p.desde)), p.boton(p.dt("nav.prevMonth"), func() { p.desde = p.desde.AddDate(0, -1, 0); p.hasta = p.desde.AddDate(0, 1, 0); p.cargarItems(false) }), p.boton(p.dt("nav.nextMonth"), func() { p.desde = p.hasta; p.hasta = p.desde.AddDate(0, 1, 0); p.cargarItems(false) }))
	}
	for _, e := range p.eventos {
		cuerpo = append(cuerpo, p.eleccion(cmp.Or(e.Summary, p.dt("event.untitled")), i18n.FormatDateTime(p.tr.Locale(), e.Start.Local()), func() { p.cargarDetalle(e.ID) }))
	}
	for _, c := range p.contactos {
		cuerpo = append(cuerpo, p.eleccion(c.Name, strings.Join(c.Emails, ", "), func() { p.cargarDetalle(c.ID) }))
	}
	for _, n := range p.notas {
		cuerpo = append(cuerpo, p.eleccion(cmp.Or(n.Title, p.dt("note.untitled")), "", func() { p.cargarDetalle(n.ID) }))
	}
	if !p.ocupado && len(p.eventos)+len(p.contactos)+len(p.notas) == 0 {
		cuerpo = append(cuerpo, p.texto(p.t("ui.noResults")))
	}
	if p.cursor != "" {
		cuerpo = append(cuerpo, p.boton(p.t("common.loadMore"), func() { p.cargarItems(true) }))
	}
	if !p.coleccion.ReadOnly {
		label := map[string]string{"calendar": "nav.createEvent", "contacts": "nav.newContact", "notes": "note.new"}[p.servicio]
		cuerpo = append(cuerpo, p.boton(p.dt(label), p.crearRegistro))
	}
	return append(cuerpo, p.boton(p.t("nav.refresh"), func() { p.cargarItems(false) }))
}

func (p *Panel) cuerpoDetalle() []fyne.CanvasObject {
	for _, campo := range p.campos {
		campo.Inerte(p.ocupado || p.escrituraIncierta)
	}
	if p.inicio != nil && p.servicio == "calendar" {
		for _, campo := range []*ui.Entrada{p.inicio.Fecha, p.inicio.Hora, p.fin.Fecha, p.fin.Hora} {
			campo.Inerte(p.ocupado || p.escrituraIncierta)
		}
		if p.diaCompleto != nil {
			if p.ocupado || p.escrituraIncierta || p.eventoSoloLectura() {
				p.diaCompleto.Disable()
			} else {
				p.diaCompleto.Enable()
			}
		}
	}
	if p.guardarBoton != nil {
		p.guardarBoton.Inerte = p.ocupado || p.escrituraIncierta
		p.guardarBoton.Refresh()
	}
	var cuerpo []fyne.CanvasObject
	if p.formulario != nil {
		cuerpo = append(cuerpo, p.formulario)
	}
	if p.escrituraIncierta {
		cuerpo = append(cuerpo, p.boton(p.t("nav.refresh"), func() {
			if p.nuevoRegistro {
				p.cargarItems(false)
			} else {
				p.cargarDetalle(p.idDetalle())
			}
		}))
	}
	return cuerpo
}
