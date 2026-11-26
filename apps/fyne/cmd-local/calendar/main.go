// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Command calendar es Sytue Calendar de Sytue Office: las mismas vistas y el mismo
// formulario de la suite, con los calendarios en este equipo y en claro, sin cuenta y sin
// servidor (apps/fyne/PLAN-EDICION-LOCAL.md §6.7).
//
// Comparado con `cmd/calendar`: no hay `sesion.Abrir` ni `cifrado.NewPrivateApplication`.
// El repositorio de dominio es el MISMO (`apps/nucleo/calendar`), abierto sobre el almacén
// en claro de `local/almacen`, y la fachada `api.Calendars` va enchufada a él.
//
// No se montan: compartir calendarios, recursos y salas, salas de Rooms, notas de evento
// en la nube, «Trae tus calendarios» ni los servicios vinculados: la pantalla los oculta
// (`SinPersonas`). En su lugar, el formulario tiene un campo libre para el enlace de la
// reunión y «Unirse» lo abre (A-06), con un botón que la crea con Meet o Teams donde se
// puede (`reunion.go`, A-13). Invitar a otras personas, solo en los calendarios de una
// cuenta, que es quien manda la invitación (`invitar.go`).
//
// Cuentas (A-10, A-12): los calendarios de las cuentas CalDAV, Google y Microsoft que
// la persona dio de alta en Ajustes → Cuentas se sincronizan con `local/agenda`
// (`cuentas.go`) y salen agrupados por cuenta en el menú.
//
// Con camino local (Anexo A): suscribirse a un `.ics` por URL y refrescarlo (A-03; es
// la única red que hace esta app, y solo a la URL que la persona escribe), contestar a
// una invitación importada con el correo del perfil (A-02, sin mandar nada a nadie),
// la búsqueda con filtro y un calendario de cumpleaños de solo lectura leído de
// Contacts (A-07, K-04).
//
// Importar: un `.ics` que llega por «Abrir con», por los argumentos o soltado en la
// ventana entra en el primer calendario propio por `calendar.Repository.ImportICS`, en el
// equipo, sin HTTP (A-04).
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"path"
	"strings"
	"time"
	_ "time/tzdata" // el selector de zona del evento, también donde el sistema no las trae

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/calendario"
	"git.sytue.eu/sytue/one/apps/fyne/catalogos"
	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/local"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	privateCalendar "git.sytue.eu/sytue/one/apps/nucleo/calendar"
	"git.sytue.eu/sytue/one/apps/nucleo/contacts/lector"
	"git.sytue.eu/sytue/one/apps/nucleo/local/almacen"
	"git.sytue.eu/sytue/one/apps/nucleo/local/exportar"
)

func main() {
	a := local.NuevaApp("eu.sytue.office.calendar")
	l, err := local.Abrir(a, "calendar")
	if err != nil {
		slog.Error("calendar", "error", err)
		os.Exit(1)
	}
	tema.ElegirCromo(tema.CromoPapel)
	t, err := l.Traductor(catalogos.Calendar())
	if err != nil {
		// Un catálogo embebido roto es un fallo de compilación disfrazado.
		panic(err)
	}

	ctx := context.Background()
	repo, rec, err := abrirRepositorio(ctx, l)
	if err != nil {
		slog.Error("calendar: no se pudo abrir el almacén local", "error", err)
		os.Exit(1)
	}
	// Contestar invitaciones con el correo del perfil; suscribirse con la red que pide
	// la persona.
	repo.Correo = l.Espacio.Perfil.Correo
	repo.Descargar = descargadorICS(l)
	cuentas := nuevaSincronia(l, repo)
	datos := &conCumpleanos{
		rec:        rec,
		cuentas:    cuentas,
		Repository: repo,
		lector:     lector.Nuevo(l.Espacio.Rutas.Producto("contacts"), l.Espacio.ID),
		nombre:     func() string { return texto(l, "cumple.calendario") },
		titulo: func(nombre string, edad int) string {
			if edad > 0 {
				return fmt.Sprintf(texto(l, "cumple.edad"), nombre, edad)
			}
			return fmt.Sprintf(texto(l, "cumple.titulo"), nombre)
		},
	}
	facade, err := api.NewCalendars("", nil, nil).WithPrivateData(datos)
	if err != nil {
		slog.Error("calendar", "error", err)
		os.Exit(1)
	}

	dev := a.Driver().Device()
	w := a.NewWindow(t.T("page.title", nil))
	w.SetPadded(false)
	w.Resize(fyne.NewSize(1280, 800))

	p := &calendario.Pantalla{
		T: t, Cursor: time.Now(), Vista: calendario.VistaMes,
		Ventana:     calendario.VentanaInicial(dev.IsMobile(), fyne.IsVertical(dev.Orientation())),
		SinPersonas: true, PistaEnlace: texto(l, "enlace.pista"),
	}
	// Invitar desde el formulario en los calendarios de una cuenta, con la libreta de
	// Contacts de este equipo (A-11, A-13).
	p.Invitar = cuentas.invitar(datos.lector.Libreta())
	// Y el enlace de la reunión: Meet o Teams del proveedor del calendario, o la cuenta de
	// mensajería que la cree (A-13, reunion.go).
	p.Reunion = cuentas.reuniones.pantalla(l)
	c := calendario.NuevaAppConVida(ctx, p, facade)
	// «¿Vas a ir?» sale en los eventos importados donde el correo del perfil es invitado.
	c.Correo = strings.ToLower(l.Espacio.Perfil.Correo)
	// La vista y los calendarios ocultos se recuerdan en los ajustes de este equipo (A-08).
	c.Almacen = l.Personal()
	// IA: frase → propuesta de evento, con la IA que la persona haya configurado (A-09).
	c.Asistente = l.Asistir()
	l.AlCambiarIA(func() { c.Asistente = l.Asistir() })
	// «Unirse» abre el enlace de la reunión en el sistema; no hay salas de Rooms (Salas nil).
	c.AbrirEnlace = func(enlace string) { _ = api.AbrirFuera(a, enlace) }

	l.AlAbrirFichero(func(ctx context.Context, ap api.Apertura) error {
		return importar(ctx, l, repo, c, w, ap)
	}, func(err error) {
		local.Info(w, texto(l, "ics.titulo"), texto(l, "ics.fallo"))
	})
	l.AceptarFicheros(w)
	// «Exportar en formatos abiertos»: cada calendario propio en su `.ics`.
	l.AlExportarTodo(func(ctx context.Context, carpeta string) error { return exportar.Calendarios(ctx, repo, carpeta) })
	// «Guardar en la nube»: el primer calendario propio como `.ics`.
	l.AlExportar(func() (string, []byte, error) {
		cal, err := primerCalendario(context.Background(), repo)
		if err != nil {
			return "", nil, err
		}
		datos, err := repo.ExportICS(context.Background(), cal.ID)
		return nombreDe(cal), datos, err
	})

	w.SetContent(l.Montar(w, iconos.Calendar, c.Objeto()))
	w.Canvas().SetOnTypedRune(func(r rune) {
		if len(w.Canvas().Overlays().List()) == 0 {
			c.Atajo(r)
		}
	})
	l.AlCerrar(c.Cerrar)
	c.Arrancar()
	// Recordatorios en el sistema (§5.12), y el evento de un aviso pulsado o del Hub.
	recordar(ctx, l, repo)
	l.AlAbrirElemento(irAlEvento(ctx, repo, func(d time.Time) {
		if p.AlElegirDia != nil {
			p.AlElegirDia(d)
		}
	}))
	// «Crear evento» de Mail y de Notepads: `sytueoffice-calendar:/events/new?action=create&…`
	// abre el formulario relleno (lo lee estricto `calendario.LeerIntencion`).
	l.AlAbrirEnlace(func(u *url.URL) { c.AtenderEnlace(conParticipantes(l, u)) })
	c.Reloj()
	// Las cuentas de calendario (CalDAV, Google, Microsoft): ya, y luego solas (A-10).
	cuentas.arrancar(ctx, a, p, c, w)
	w.ShowAndRun()
}

// abrirRepositorio abre el repositorio de Calendar de la edición conectada, sin tocarlo,
// sobre el almacén en claro de este equipo. La primera vez crea un calendario personal:
// la pantalla sin ninguno no deja crear eventos.
func abrirRepositorio(ctx context.Context, l *local.App) (*privateCalendar.Repository, *almacen.Recursos, error) {
	rec, err := l.Almacen(ctx, "calendar")
	if err != nil {
		return nil, nil, err
	}
	repo, err := nuevoRepositorio(rec)
	if err != nil {
		return nil, nil, err
	}
	l.AlCerrar(repo.Close) // antes que el almacén: AlCerrar va del último al primero
	// Y antes, se espera a la compactación en el acto, que lo refresca.
	l.AlCerrar(rec.PararCompactar)
	if err := almacen.Preparar(ctx, repo); err != nil {
		return nil, nil, err
	}
	cals, err := repo.List(ctx)
	if err != nil {
		return nil, nil, err
	}
	if len(cals) == 0 {
		zona := time.Local.String()
		if _, err := time.LoadLocation(zona); err != nil || zona == "Local" {
			zona = "UTC"
		}
		if _, err := repo.CreateCalendar(ctx, api.CalendarInput{Name: "personal", DisplayName: texto(l, "personal"), Timezone: zona}); err != nil {
			slog.Warn("calendar: no se pudo crear el calendario personal", "error", err)
		}
	}
	return repo, rec, nil
}

// nuevoRepositorio es el repositorio de Calendar de la edición conectada sobre el almacén
// de este equipo, con lo que solo pide la edición local: guardar un evento —o partir su
// serie con «solo este» o «este y los siguientes»— no pisa lo que contestaron los
// invitados mientras el formulario estaba abierto (la respuesta que llega por correo,
// `respuestas.go`, o la que baja de la cuenta), ni sube su SEQUENCE si no cambia lo que
// ellos ven (`calendar.Repository.ConservarRespuestas`); lo que no cambió se queda
// escrito como vino de la cuenta (`ConservarForma`), y lo que la persona cambia en la
// serie llega a sus excepciones (`SerieALasExcepciones`).
func nuevoRepositorio(rec *almacen.Recursos) (*privateCalendar.Repository, error) {
	repo, err := privateCalendar.New(rec.Config)
	if err != nil {
		return nil, err
	}
	repo.ConservarRespuestas = true
	repo.ConservarForma = true
	repo.SerieALasExcepciones = true
	return repo, nil
}

// conParticipantes copia los participantes del enlace a la descripción: sin personas
// (§6.7) el formulario no enseña invitados, y así no se pierde a quién iba el correo.
func conParticipantes(l *local.App, u *url.URL) *url.URL {
	q := u.Query()
	if q.Get("action") != "create" || strings.TrimSpace(q.Get("attendees")) == "" {
		return u
	}
	var limpios []string
	for _, c := range strings.Split(q.Get("attendees"), ",") {
		if c = strings.TrimSpace(c); c != "" && len(limpios) < 100 {
			limpios = append(limpios, c)
		}
	}
	d := strings.TrimSpace(q.Get("description"))
	if d != "" {
		d += "\n\n"
	}
	q.Set("description", d+texto(l, "participantes")+" "+strings.Join(limpios, ", "))
	v := *u
	v.RawQuery = q.Encode()
	return &v
}

var errSinCalendario = errors.New("calendar: no hay ningún calendario propio")

// primerCalendario es el primero que se puede escribir: donde entra lo importado.
func primerCalendario(ctx context.Context, repo *privateCalendar.Repository) (api.Calendar, error) {
	cals, err := repo.List(ctx)
	if err != nil {
		return api.Calendar{}, err
	}
	for _, c := range cals {
		if !c.ReadOnly {
			return c, nil
		}
	}
	return api.Calendar{}, errSinCalendario
}

func importar(ctx context.Context, l *local.App, repo *privateCalendar.Repository, c *calendario.App, w fyne.Window, ap api.Apertura) error {
	nombre, datos, err := local.LeerLocal(ap.Local)
	if err != nil {
		return err
	}
	if ext := strings.ToLower(path.Ext(strings.ReplaceAll(nombre, `\`, "/"))); ext != ".ics" && ext != ".ical" && ext != ".ifb" {
		return fmt.Errorf("calendar: %q no es un calendario (.ics)", nombre)
	}
	cal, err := primerCalendario(ctx, repo)
	if err != nil {
		return err
	}
	res, err := repo.ImportICS(ctx, cal.ID, datos)
	if err != nil {
		return err
	}
	fyne.Do(func() {
		c.Recargar()
		msg := fmt.Sprintf(texto(l, "ics.hecho"), res.Importados, cal.DisplayName)
		if res.Omitidos+res.Fallidos > 0 {
			msg += " " + fmt.Sprintf(texto(l, "ics.fuera"), res.Omitidos+res.Fallidos)
		}
		local.Info(w, texto(l, "ics.titulo"), msg)
	})
	return nil
}

func nombreDe(c api.Calendar) string {
	n := strings.TrimSpace(c.DisplayName)
	if n == "" {
		n = "calendario"
	}
	// Sin separadores de ruta ni caracteres que Windows no admite en un nombre.
	n = strings.Map(func(r rune) rune {
		if strings.ContainsRune(`/\:*?"<>|`, r) || r < ' ' {
			return '_'
		}
		return r
	}, n)
	return n + ".ics"
}

// Textos propios de esta edición, que no tienen clave en el catálogo de la web (§5.14):
// español e inglés aquí, los otros veinticuatro idiomas en textos_idiomas.go.
var textos = map[string][2]string{ // clave: {es, en}
	"personal":              {"Personal", "Personal"},
	"ics.titulo":            {"Importar calendario", "Import calendar"},
	"ics.hecho":             {"%d eventos importados en «%s».", "%d events imported into “%s”."},
	"ics.fuera":             {"%d ya estaban o no se pudieron leer.", "%d were already there or could not be read."},
	"ics.fallo":             {"Este fichero no se puede importar. Usa un calendario iCalendar (.ics).", "This file cannot be imported. Use an iCalendar (.ics) file."},
	"enlace.pista":          {"Pega el enlace de la reunión (Meet, Teams, Zoom, Jitsi…). «Unirse» lo abre en el navegador.", "Paste the meeting link (Meet, Teams, Zoom, Jitsi…). “Join” opens it in the browser."},
	"cumple.calendario":     {"Cumpleaños", "Birthdays"},
	"cumple.titulo":         {"Cumpleaños de %s", "%s’s birthday"},
	"cumple.edad":           {"Cumpleaños de %s (%d)", "%s’s birthday (%d)"},
	"sync.ahora":            {"Sincronizar ahora", "Sync now"},
	"sync.reconectar":       {"Hay que volver a entrar en esta cuenta (Ajustes → Cuentas).", "Sign in to this account again (Settings → Accounts)."},
	"sync.copia":            {"(copia en conflicto)", "(conflicted copy)"},
	"sync.conflicto.titulo": {"Cambios en los dos sitios", "Changed in both places"},
	"sync.conflicto":        {"%d eventos cambiaron aquí y en la cuenta a la vez. La versión de la cuenta queda en su sitio y la de este equipo, como «(copia en conflicto)».", "%d events changed here and in the account at the same time. The account’s version stays in place and this device’s is kept as “(conflicted copy)”."},
	"participantes":         {"Participantes:", "Participants:"},
	// El correo de las invitaciones (iMIP) y el aviso de lo que no salió.
	"imip.invitacion":   {"Invitación: %s", "Invitation: %s"},
	"imip.actualizada":  {"Invitación actualizada: %s", "Updated invitation: %s"},
	"imip.cancelada":    {"Cancelado: %s", "Canceled: %s"},
	"imip.cuando":       {"Cuándo", "When"},
	"imip.donde":        {"Dónde", "Where"},
	"imip.organiza":     {"Organiza", "Organizer"},
	"imip.titulo":       {"Invitaciones sin enviar", "Invitations not sent"},
	"imip.sinremitente": {"Los cambios de «%s» aún no se han mandado a los invitados: no hay ninguna cuenta en Mail con la dirección %s. Añádela en Mail y saldrán solos.", "The changes to “%s” haven’t been sent to the guests yet: there is no account in Mail with the address %s. Add it in Mail and they’ll go out on their own."},
	"imip.fallo":        {"No se pudieron mandar a los invitados los cambios de «%s» desde %s. Se volverá a intentar solo; si sigue sin salir, revisa esa cuenta en Mail.", "The changes to “%s” couldn’t be sent to the guests from %s. It will be retried automatically; if it still doesn’t go out, check that account in Mail."},
	"imip.cola":         {"Invitaciones por enviar: %d.", "Invitations waiting to be sent: %d."},
	// Invitar desde el formulario: por dónde sale la invitación y la ocupación de los
	// invitados en «buscar un hueco».
	"invitar.local":    {"Para invitar a otras personas, guarda el evento en un calendario de una cuenta (Ajustes → Cuentas).", "To invite people, save the event in a calendar from an account (Settings → Accounts)."},
	"invitar.google":   {"Google manda la invitación a cada persona al sincronizar.", "Google sends the invitation to each person when it syncs."},
	"invitar.outlook":  {"Outlook manda la invitación a cada persona al sincronizar.", "Outlook sends the invitation to each person when it syncs."},
	"invitar.servidor": {"El servidor de la cuenta manda la invitación a cada persona al sincronizar.", "The account’s server sends the invitation to each person when it syncs."},
	"invitar.correo":   {"La invitación sale por correo desde %s (tu cuenta de Mail) al sincronizar.", "The invitation is sent by email from %s (your Mail account) when it syncs."},
	"invitar.sinmail":  {"La invitación saldrá por correo cuando añadas en Mail la cuenta de %s.", "The invitation will be sent by email once you add the %s account in Mail."},
	"ocupacion.con":    {"Con la ocupación de: %s.", "Includes the availability of: %s."},
	"ocupacion.sin":    {"No se sabe si están libres: %s.", "Availability unknown for: %s."},
	// El enlace de la reunión desde el formulario (A-13): Meet o Teams del proveedor del
	// calendario, que lo crea al subir el evento, o la cuenta de mensajería que lo crea ya.
	"reunion.meet":            {"Añadir Google Meet", "Add Google Meet"},
	"reunion.teams":           {"Reunión de Teams", "Teams meeting"},
	"reunion.rooms":           {"Crear reunión de %s", "Create a %s meeting"},
	"reunion.meet.pendiente":  {"Google creará el enlace de Meet cuando guardes el evento y se sincronice.", "Google will create the Meet link once you save the event and it syncs."},
	"reunion.teams.pendiente": {"Outlook creará el enlace de Teams cuando guardes el evento y se sincronice.", "Outlook will create the Teams link once you save the event and it syncs."},
}

// texto es el texto propio de clave en el idioma de la app; lo que falte cae al inglés.
func texto(l *local.App, clave string) string {
	return local.Propios{Base: textos, Traducciones: traducciones}.En(l.Idioma(), clave)
}
