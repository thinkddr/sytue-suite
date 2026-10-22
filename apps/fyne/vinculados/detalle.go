// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package vinculados

import (
	"context"
	"net/mail"
	"slices"
	"strings"
	"time"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/sistema"
)

func (p *Panel) idDetalle() string {
	switch p.servicio {
	case "calendar":
		return p.evento.ID
	case "contacts":
		return p.contacto.ID
	default:
		return p.nota.ID
	}
}

func (p *Panel) cargarDetalle(id string) {
	account, collection := p.cuenta, p.coleccion.ID
	p.trabajar(30*time.Second, func(ctx context.Context) func() {
		var evento api.RemoteEvent
		var contacto api.RemoteContact
		var nota api.RemoteNote
		var err error
		switch p.servicio {
		case "calendar":
			evento, err = p.svc.LinkedEvent(ctx, account, collection, id)
		case "contacts":
			contacto, err = p.svc.LinkedContact(ctx, account, collection, id)
		case "notes":
			nota, err = p.svc.LinkedNote(ctx, account, collection, id)
		}
		return func() {
			if err != nil {
				p.fallar(err, false)
				return
			}
			p.evento, p.contacto, p.nota = evento, contacto, nota
			p.escrituraIncierta, p.nuevoRegistro = false, false
			p.reconectarNecesario = false
			p.paso = "detalle"
			p.formulario = p.construirDetalle()
		}
	})
}

func (p *Panel) campo(key, label, value string, multi bool) fyne.CanvasObject {
	var objeto fyne.CanvasObject
	var entrada *ui.Entrada
	if multi {
		a := ui.NuevaAreaTexto("")
		entrada = &a.Entrada
		objeto = a
	} else {
		entrada = ui.NuevaEntrada("")
		objeto = entrada
	}
	entrada.Rotulo = label
	entrada.PonerTexto(value)
	p.campos[key] = entrada
	return p.columna(p.texto(label), objeto)
}

func (p *Panel) construirDetalle() fyne.CanvasObject {
	p.campos = map[string]*ui.Entrada{}
	p.guardarBoton = nil
	var campos []fyne.CanvasObject
	var readonly bool
	switch p.servicio {
	case "calendar":
		campos, readonly = p.formularioEvento()
	case "contacts":
		campos, readonly = p.formularioContacto()
	case "notes":
		campos, readonly = p.formularioNota()
	}
	if readonly {
		for _, e := range p.campos {
			e.SoloLectura(true)
		}
		campos = append(campos, p.texto(p.t("sharedWithMe.readOnly")))
	} else {
		label := p.t("ui.save")
		if p.servicio == "notes" && !p.nuevoRegistro {
			label = p.dt("capture.add")
		}
		p.guardarBoton = p.boton(label, p.guardar)
		p.guardarBoton.Variante = ui.Primario
		campos = append(campos, p.guardarBoton)
	}
	return p.columna(campos...)
}

func (p *Panel) formularioEvento() ([]fyne.CanvasObject, bool) {
	e := p.evento
	readonly := p.eventoSoloLectura()
	campos := make([]fyne.CanvasObject, 0, 7+len(e.Attendees))
	campos = append(campos, p.campo("title", p.dt("event.title"), e.Summary, false), p.campo("description", p.dt("event.description"), e.Description, true), p.campo("location", p.dt("event.location"), e.Location, false))
	zone := zonaEvento(e)
	p.inicio, p.fin = ui.NuevoCampoFechaHora(ui.CampoMD, e.Start.In(zone)), ui.NuevoCampoFechaHora(ui.CampoMD, e.End.In(zone))
	campos = append(campos, p.texto(p.dt("event.start")), p.inicio.Objeto(), p.texto(p.dt("event.end")), p.fin.Objeto())
	p.diaCompleto = ui.NuevaCasilla(p.dt("event.allDay"), func(value bool) {
		p.inicio.Hora.SoloLectura(value || readonly)
		p.fin.Hora.SoloLectura(value || readonly)
	})
	p.diaCompleto.Marcada = e.AllDay
	if readonly {
		p.diaCompleto.Disable()
	}
	campos = append(campos, p.diaCompleto)
	p.inicio.Hora.SoloLectura(e.AllDay || readonly)
	p.fin.Hora.SoloLectura(e.AllDay || readonly)
	for _, a := range e.Attendees {
		campos = append(campos, p.texto(a.Name+" "+a.Email))
	}
	if readonly {
		for _, v := range []*ui.Entrada{p.inicio.Fecha, p.inicio.Hora, p.fin.Fecha, p.fin.Hora} {
			v.SoloLectura(true)
		}
	}
	return campos, readonly
}

func (p *Panel) formularioContacto() ([]fyne.CanvasObject, bool) {
	c := p.contacto
	campos := []fyne.CanvasObject{p.campo("name", p.dt("form.fullName"), c.Name, false), p.campo("organization", p.dt("field.organization"), c.Organization, false), p.campo("emails", p.dt("field.email"), strings.Join(c.Emails, "\n"), true), p.campo("note", p.dt("field.note"), c.Note, true)}
	// El proveedor conserva los tipos de teléfono y las direcciones estructuradas.
	if len(c.Phones) > 0 {
		campos = append(campos, p.texto(p.dt("field.phone")), p.texto(strings.Join(c.Phones, "\n")))
	}
	if len(c.Addresses) > 0 {
		campos = append(campos, p.texto(p.dt("field.address")), p.texto(strings.Join(c.Addresses, "\n")))
	}
	return campos, p.coleccion.ReadOnly || c.ReadOnly || !p.nuevoRegistro && c.ETag == ""
}

func (p *Panel) formularioNota() ([]fyne.CanvasObject, bool) {
	if p.nuevoRegistro {
		return []fyne.CanvasObject{p.campo("title", p.dt("notebook.title"), "", false), p.campo("text", p.dt("note.newText"), "", true)}, p.coleccion.ReadOnly
	}
	readonly := p.coleccion.ReadOnly || p.nota.ReadOnly || !p.nota.CanAppend
	campos := []fyne.CanvasObject{p.texto(p.nota.Title), p.texto(p.nota.Text)}
	if !readonly {
		campos = append(campos, p.campo("text", p.dt("capture.add"), "", true))
	}
	return campos, readonly
}

func zonaEvento(e api.RemoteEvent) *time.Location {
	if zone, err := time.LoadLocation(e.TimeZone); err == nil {
		return zone
	}
	return e.Start.Location()
}

func (p *Panel) crearNota() { p.crearRegistro() }

func (p *Panel) crearRegistro() {
	if p.ocupado || p.coleccion.ReadOnly {
		return
	}
	p.nuevoRegistro, p.escrituraIncierta = true, false
	p.nota = api.RemoteNote{}
	p.contacto = api.RemoteContact{CollectionID: p.coleccion.ID}
	zoneName := p.coleccion.TimeZone
	zone, err := time.LoadLocation(zoneName)
	if err != nil || zoneName == "" {
		zoneName = sistema.ZonaIANA(time.Local)
		zone, err = time.LoadLocation(zoneName)
	}
	if err != nil {
		zoneName = "UTC"
		zone = time.UTC
	}
	start := time.Now().In(zone).Truncate(time.Minute)
	if start.Year() != p.desde.Year() || start.Month() != p.desde.Month() {
		start = time.Date(p.desde.Year(), p.desde.Month(), 1, 9, 0, 0, 0, zone)
	}
	p.evento = api.RemoteEvent{CollectionID: p.coleccion.ID, Start: start, End: start.Add(time.Hour), TimeZone: zoneName}
	p.paso = "detalle"
	p.formulario = p.construirDetalle()
	p.pintar()
}

func (p *Panel) eventoSoloLectura() bool {
	e := p.evento
	return p.coleccion.ReadOnly || e.ReadOnly || len(e.Recurrence) > 0 || !p.nuevoRegistro && e.ETag == ""
}

func cambio(antes, despues string) *string {
	if antes == despues {
		return nil
	}
	return &despues
}

func lineas(s string) []string {
	var result []string
	for _, line := range strings.Split(s, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			result = append(result, line)
		}
	}
	return result
}

// guardar manda sólo los campos cambiados. Un resultado ambiguo obliga a leer de
// nuevo: un clic de reintento no puede duplicar texto añadido ni una página creada.
func (p *Panel) guardar() {
	if p.ocupado || p.escrituraIncierta || p.coleccion.ReadOnly {
		return
	}
	switch p.servicio {
	case "calendar":
		p.guardarEvento()
	case "contacts":
		p.guardarContacto()
	case "notes":
		p.guardarNota()
	}
}

func (p *Panel) guardarEvento() {
	account, collection := p.cuenta, p.coleccion.ID
	e := p.evento
	if p.eventoSoloLectura() || p.nuevoRegistro && (e.ID != "" || e.ETag != "") {
		return
	}
	patch := api.RemoteEventPatch{Summary: cambio(e.Summary, p.campos["title"].Texto()), Description: cambio(e.Description, p.campos["description"].Texto()), Location: cambio(e.Location, p.campos["location"].Texto())}
	if strings.TrimSpace(p.campos["title"].Texto()) == "" {
		p.fallo = p.dt("event.titleRequired")
		p.pintar()
		return
	}
	times, ok := p.tiemposEvento(e)
	if !ok {
		p.fallo = p.dt("event.endBeforeStart")
		p.pintar()
		return
	}
	patch.Start, patch.End, patch.AllDay, patch.TimeZone = times.Start, times.End, times.AllDay, times.TimeZone
	if patch.Summary == nil && patch.Description == nil && patch.Location == nil && patch.Start == nil && patch.End == nil {
		p.paso = "items"
		p.formulario = nil
		p.pintar()
		return
	}
	escribir := func(ctx context.Context) error {
		_, err := p.svc.SaveLinkedEvent(ctx, account, collection, e.ID, e.ETag, patch)
		return err
	}

	p.enviarCambio(escribir)
}

func (p *Panel) tiemposEvento(e api.RemoteEvent) (api.RemoteEventPatch, bool) {
	patch := api.RemoteEventPatch{}
	start, end := e.Start, e.End
	zone := zonaEvento(e)
	for _, f := range []struct {
		campo   *ui.CampoFechaHora
		antes   time.Time
		destino *time.Time
		patch   **time.Time
	}{{p.inicio, e.Start, &start, &patch.Start}, {p.fin, e.End, &end, &patch.End}} {
		if f.campo.Fecha.Texto() == ui.EscribirFecha(f.antes.In(zone)) && f.campo.Hora.Texto() == ui.EscribirHora(f.antes.In(zone)) {
			continue
		}
		v, ok := f.campo.Leer(zone)
		if !ok {
			return patch, false
		}
		*f.destino = v
		*f.patch = &v
	}
	allDay := e.AllDay
	if p.diaCompleto != nil {
		allDay = p.diaCompleto.Marcada
	}
	if allDay {
		sameDay := start.In(zone).Format(time.DateOnly) == end.In(zone).Format(time.DateOnly) && end.After(start)
		start = time.Date(start.In(zone).Year(), start.In(zone).Month(), start.In(zone).Day(), 0, 0, 0, 0, zone)
		end = time.Date(end.In(zone).Year(), end.In(zone).Month(), end.In(zone).Day(), 0, 0, 0, 0, zone)
		if !e.AllDay && sameDay {
			end = start.AddDate(0, 0, 1)
		}
	}
	if !end.After(start) {
		return patch, false
	}
	if p.nuevoRegistro || patch.Start != nil || patch.End != nil || allDay != e.AllDay {
		patch.Start, patch.End, patch.AllDay = &start, &end, &allDay
		zoneName := e.TimeZone
		if _, err := time.LoadLocation(zoneName); err != nil || zoneName == "" {
			zoneName = sistema.ZonaIANA(zone)
		}
		patch.TimeZone = &zoneName
	}
	return patch, true
}

func (p *Panel) guardarContacto() {
	account, collection := p.cuenta, p.coleccion.ID
	c := p.contacto
	if c.ReadOnly || !p.nuevoRegistro && c.ETag == "" || p.nuevoRegistro && (c.ID != "" || c.ETag != "") {
		return
	}
	if strings.TrimSpace(p.campos["name"].Texto()) == "" {
		p.fallo = p.dt("form.nameRequired")
		p.pintar()
		return
	}
	patch := api.RemoteContactPatch{Name: cambio(c.Name, p.campos["name"].Texto()), Organization: cambio(c.Organization, p.campos["organization"].Texto()), Note: cambio(c.Note, p.campos["note"].Texto())}
	emails := lineas(p.campos["emails"].Texto())
	if !slices.Equal(c.Emails, emails) {
		for _, email := range emails {
			address, err := mail.ParseAddress(email)
			if err != nil || address.Address != email {
				p.fallo = p.dt("form.emailInvalid")
				p.pintar()
				return
			}
		}
		patch.Emails = &emails
	}
	if patch.Name == nil && patch.Organization == nil && patch.Note == nil && patch.Emails == nil {
		p.paso = "items"
		p.formulario = nil
		p.pintar()
		return
	}
	escribir := func(ctx context.Context) error {
		_, err := p.svc.SaveLinkedContact(ctx, account, collection, c.ID, c.ETag, patch)
		return err
	}

	p.enviarCambio(escribir)
}

func (p *Panel) guardarNota() {
	account, collection := p.cuenta, p.coleccion.ID
	var escribir func(context.Context) error
	text := p.campos["text"]
	if text == nil || strings.TrimSpace(text.Texto()) == "" {
		return
	}
	value := text.Texto()
	if p.nuevoRegistro {
		title := p.campos["title"].Texto()
		escribir = func(ctx context.Context) error {
			_, err := p.svc.CreateLinkedNote(ctx, account, collection, title, value)
			return err
		}
	} else {
		n := p.nota
		if n.ReadOnly || !n.CanAppend {
			return
		}
		escribir = func(ctx context.Context) error { return p.svc.AppendLinkedNote(ctx, account, collection, n.ID, value) }
	}
	p.enviarCambio(escribir)
}

func (p *Panel) enviarCambio(escribir func(context.Context) error) {
	p.trabajar(30*time.Second, func(ctx context.Context) func() {
		err := escribir(ctx)
		return func() {
			if err != nil {
				p.fallar(err, true)
				return
			}
			p.formulario, p.campos = nil, nil
			p.paso = "items"
			p.cargarItems(false)
		}
	})
}
