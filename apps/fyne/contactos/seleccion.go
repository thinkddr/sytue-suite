// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package contactos

import (
	"log/slog"
	"slices"
	"strings"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// TopeDestinatarios es `TOPE_DESTINATARIOS` de seleccion.ts: cuántos correos caben en
// el enlace a Mail. Van en la dirección (`?to=`), y un correo a más de cien personas es
// una lista de distribución, no un correo.
const TopeDestinatarios = 100

// CorreosDe es `correosDe`: el correo principal de cada contacto, sin repetir (sin
// mirar mayúsculas) y hasta el tope; quien no tiene correo no cuenta. `recortado` dice
// si había más de los que caben.
func CorreosDe(cs []api.Contact) (correos []string, recortado bool) {
	correos = CorreosParaCopiar(cs)
	if len(correos) > TopeDestinatarios {
		return correos[:TopeDestinatarios], true
	}
	return correos, false
}

// CorreosParaCopiar es `correosParaCopiar`: el principal de cada uno, sin repetir y SIN
// tope (el tope es del enlace a Mail, no del portapapeles).
func CorreosParaCopiar(cs []api.Contact) []string {
	var correos []string
	vistos := map[string]bool{}
	for _, c := range cs {
		correo := strings.TrimSpace(correoPrincipal(c))
		clave := strings.ToLower(correo)
		if correo == "" || vistos[clave] {
			continue
		}
		vistos[clave] = true
		correos = append(correos, correo)
	}
	return correos
}

// TextoDeFicha es `textoDeFicha` de la web: el nombre, el cargo con la organización, y
// cada correo, teléfono y dirección en su línea, sin rótulos ni etiquetas.
func TextoDeFicha(c api.Contact) string {
	rol := strings.Join(slices.DeleteFunc([]string{strings.TrimSpace(c.Title), strings.TrimSpace(c.Organization)},
		func(s string) bool { return s == "" }), " · ")
	lineas := []string{c.DisplayName(), rol}
	for _, e := range c.Emails {
		lineas = append(lineas, e.Email)
	}
	for _, t := range c.Phones {
		lineas = append(lineas, t.Phone)
	}
	for _, d := range c.Addresses {
		cp := strings.TrimSpace(strings.Join(slices.DeleteFunc([]string{d.PostalCode, d.Locality},
			func(s string) bool { return strings.TrimSpace(s) == "" }), " "))
		partes := slices.DeleteFunc([]string{strings.TrimSpace(d.Street), cp, strings.TrimSpace(d.Region), strings.TrimSpace(d.Country)},
			func(s string) bool { return s == "" })
		lineas = append(lineas, strings.Join(partes, ", "))
	}
	lineas = slices.DeleteFunc(lineas, func(s string) bool { return strings.TrimSpace(s) == "" })
	return strings.Join(lineas, "\n")
}

// compartirPorCorreo es «Compartir por correo»: Sytue Mail con el nombre de asunto y la
// ficha de cuerpo, por el mismo enlace entre apps que «Enviar correo a todos».
func (a *App) compartirPorCorreo(c api.Contact) {
	ruta := api.RutaRedactar(api.Redactar{Asunto: c.DisplayName(), Cuerpo: TextoDeFicha(c)})
	if err := api.AbrirEn(a.abridor(), a.Apex, "mail", a.org(), ruta); err != nil {
		slog.Warn("contactos: no se pudo abrir Sytue Mail", "error", err)
	}
}

// copiarCorreos es «Copiar los correos» de la selección, para pegarlos donde haga falta.
func (a *App) copiarCorreos(cs []api.Contact) {
	correos := CorreosParaCopiar(cs)
	if len(correos) == 0 {
		a.avisar(ui.AvisoInfo, "batch.noEmail", "")
		return
	}
	fyne.CurrentApp().Clipboard().SetContent(strings.Join(correos, ", "))
	a.avisar(ui.AvisoExito, "batch.emailsCopied", "")
}

// correoATodos es «Enviar correo a todos»: el redactor de Sytue Mail con un correo por
// persona en «Para» (`api.RutaRedactar`, el `action=compose` que lee Mail). Sin ningún
// correo no abre un redactor vacío: lo dice; con más de los que caben, avisa de que van
// los primeros.
func (a *App) correoATodos(cs []api.Contact) {
	correos, recortado := CorreosDe(cs)
	if len(correos) == 0 {
		a.avisar(ui.AvisoInfo, "batch.noEmail", "")
		return
	}
	if recortado {
		a.avisos.Notificar(ui.NuevoAviso(ui.AvisoAdvertencia,
			a.Pantalla.t("batch.emailCapped", i18n.Vars{"count": len(correos)}), ""))
	}
	ruta := api.RutaRedactar(api.Redactar{Para: strings.Join(correos, ", ")})
	if err := api.AbrirEn(a.abridor(), a.Apex, "mail", a.org(), ruta); err != nil {
		slog.Warn("contactos: no se pudo abrir Sytue Mail", "error", err)
	}
}

// exportarContactos es «Exportar (.vcf)» de la selección o de una etiqueta: sus vCards
// en un fichero, en segundo plano (`guardarVCards`).
func (a *App) exportarContactos(nombre string, cs []api.Contact) {
	if len(cs) == 0 {
		return
	}
	cs = slices.Clone(cs)
	a.programar(func(scope cargaContacts) { a.guardarVCards(scope, nombre, cs) })
}

// miembrosEtiquetaActiva son los contactos de la libreta que llevan la etiqueta por la
// que se filtra, sin la búsqueda ni los destacados: lo que «Enviar correo a todos» y
// «Exportar» del menú de la etiqueta entienden por «todos».
func (a *App) miembrosEtiquetaActiva() (api.ContactGroup, []api.Contact) {
	p := a.Pantalla
	for _, g := range p.Etiquetas {
		if g.ID == p.EtiquetaActiva {
			return g, filtrar(a.todos, "", g.Members, true, false, nil)
		}
	}
	return api.ContactGroup{}, nil
}
