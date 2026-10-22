// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calendario

import (
	"context"
	"net/url"
	"regexp"
	"strings"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// Los dos enlaces que abren Calendar desde fuera, los mismos que atiende la web al
// montarse (CalendarApp.tsx), por el esquema `sytue-calendar:` de su FyneApp.toml:
//
//   - `?action=create&title=&description=&start=&attendees=` es el «crear evento desde un
//     correo» que compone Sytue Hub y el «Crear reunión» de la ficha de un contacto:
//     abre el formulario de crear, ya relleno (`readCreateIntent`).
//   - `?date=<dtstart>[&all_day=1]` es el enlace a un evento del buscador global de la
//     suite: la vista de día en esa fecha (`fechaDelEnlace`).
//   - `/events/<id>` es el resultado de un evento en el buscador del Hub: la web lo
//     resuelve en el servidor (`pages/events/[id].ts`) pidiendo el evento y llevando a
//     `?date=`; aquí se pide igual y se va a su día. Si no es un UUID, no existe o no se
//     puede leer, se queda donde estaba, como la web vuelve a la portada.

// correoEnlace es `EMAIL_RE`, para sanear los invitados que traiga el enlace.
var correoEnlace = regexp.MustCompile(`^[^\s@]+@[^\s@]+\.[^\s@]+$`)

// LeerIntencion es `readCreateIntent`: lo que venga en el enlace se trata como dato no
// confiable. El título y la descripción se recortan, `start` se lee estricto (si no vale,
// la siguiente hora en punto), y los invitados se quedan en correos válidos, en
// minúsculas, sin repetir y como mucho cien. ok=false si no es un intent de crear.
func LeerIntencion(q url.Values, ahora time.Time, loc *time.Location) (Edicion, bool) {
	if q.Get("action") != "create" {
		return Edicion{}, false
	}
	e := Edicion{Titulo: recortar(q.Get("title"), 500), Descripcion: recortar(q.Get("description"), 5000)}
	inicio, ok := leerInstante(q.Get("start"), loc)
	if !ok {
		// `roundToNextHour`: a la hora en punto siguiente.
		n := ahora.In(loc)
		inicio = time.Date(n.Year(), n.Month(), n.Day(), n.Hour()+1, 0, 0, 0, loc)
	}
	e.Inicio, e.Fin = inicio, inicio.Add(time.Hour)
	vistos := map[string]bool{}
	for _, c := range strings.Split(q.Get("attendees"), ",") {
		c = strings.ToLower(strings.TrimSpace(c))
		if !correoEnlace.MatchString(c) || vistos[c] || len(e.Invitados) == 100 {
			continue
		}
		vistos[c] = true
		e.Invitados = append(e.Invitados, c)
	}
	return e, true
}

// FechaDelEnlace es `fechaDelEnlace`: el día del enlace de un evento, o ok=false si no
// trae una fecha válida. Un día completo va por su FECHA, sin que la zona lo mueva.
func FechaDelEnlace(q url.Values, loc *time.Location) (time.Time, bool) {
	s := q.Get("date")
	if len(s) < 10 {
		return time.Time{}, false
	}
	if q.Get("all_day") == "1" {
		d, err := time.Parse(time.DateOnly, s[:10])
		if err != nil {
			return time.Time{}, false
		}
		return time.Date(d.Year(), d.Month(), d.Day(), 12, 0, 0, 0, loc), true
	}
	return leerInstante(s, loc)
}

// leerInstante lee lo que entiende `new Date(…)` de un enlace: RFC 3339, o una fecha y
// hora sin zona —que son de la zona de visualización—, o una fecha sola.
func leerInstante(s string, loc *time.Location) (time.Time, bool) {
	s = strings.TrimSpace(s)
	for _, f := range []string{time.RFC3339, "2006-01-02T15:04:05", "2006-01-02T15:04", time.DateOnly} {
		if t, err := time.ParseInLocation(f, s, loc); err == nil {
			return t.In(loc), true
		}
	}
	return time.Time{}, false
}

func recortar(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		return string(r[:n])
	}
	return s
}

// AtenderEnlace atiende un enlace del esquema de Calendar.
func (a *App) AtenderEnlace(u *url.URL) {
	if u == nil {
		return
	}
	p := a.P
	if api.LeerInvitaciones(u) { // el aviso de una invitación
		a.abrirInvitaciones()
		return
	}
	if id, ok := api.LeerCalendario(u); ok { // el aviso de una petición de edición
		a.verPeticionesDe(id)
		return
	}
	if id, ok := api.LeerEvento(u); ok {
		if a.Cal != nil {
			a.lanzar(func() {
				ctx, parar := context.WithTimeout(context.Background(), plazo)
				defer parar()
				ev, err := a.Cal.GetEvent(ctx, id)
				if err != nil || ev.DTStart.IsZero() {
					return
				}
				a.enUI(func() { a.irAlDia(inicioResultado(ev, p.zona())) })
			})
		}
		return
	}
	q := u.Query()
	if e, ok := LeerIntencion(q, time.Now(), p.zona()); ok {
		if !p.SinConexion {
			a.abrirFormulario(e)
		}
		return
	}
	if dia, ok := FechaDelEnlace(q, p.zona()); ok {
		a.irAlDia(dia)
	}
}
