// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package indice

import (
	"cmp"
	"encoding/json"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/calendar/event"
	"git.sytue.eu/sytue/one/apps/nucleo/local/almacen"
)

// # Hoy (HU-04)
//
// La tarjeta «Hoy» del Hub: lo que la persona tiene que ver al empezar el día, leído de
// los almacenes de las demás apps en solo lectura, como el resto de este paquete:
//
//   - los eventos del día de Calendar, con las repeticiones ya expandidas
//     (`calendar/event.ExpandEvents`, lo mismo que hace la app);
//   - las tareas abiertas y los recordatorios del día de Notepads;
//   - el correo sin leer de Recibidos, del índice de Mail.

// Topes de la tarjeta: lo de más se ve en su app.
const (
	MaxTareasHoy  = 8
	MaxCorreosHoy = 5
)

// Ocurrencia es una vez de un evento: la de un evento suelto o una de su serie.
type Ocurrencia struct {
	Elemento  // Fecha es el inicio de esta vez
	Fin       time.Time
	TodoElDia bool
}

// Hoy es lo del día.
type Hoy struct {
	// Eventos son las ocurrencias que tocan el día, por su inicio.
	Eventos []Ocurrencia
	// Tareas son las tareas abiertas: primero las que vencen hoy o ya han vencido (Fecha
	// es el vencimiento), luego las demás de la más nueva a la más vieja; TotalTareas,
	// cuántas hay en total.
	Tareas      []Elemento
	TotalTareas int
	// Recordatorios son los de las notas que suenan hoy, por su hora.
	Recordatorios []Elemento
	// SinLeer es el correo por leer de Recibidos; Correos, los más recientes.
	SinLeer int
	Correos []Elemento
	// Fallos son las fuentes que no se han podido leer.
	Fallos map[string]error
}

// LeerHoy lee lo del día de `ahora` (en su zona) de la carpeta de datos.
func LeerHoy(datos string, ahora time.Time) Hoy {
	h := Hoy{Fallos: map[string]error{}}
	inicio := time.Date(ahora.Year(), ahora.Month(), ahora.Day(), 0, 0, 0, 0, ahora.Location())
	fin := inicio.AddDate(0, 0, 1)

	if filas, err := almacen.Leer(filepath.Join(datos, "Datos", "calendar")); err != nil {
		h.Fallos["calendar"] = err
	} else {
		h.Eventos = ocurrencias(leerCalendario(filas), inicio, fin)
	}
	if filas, err := almacen.Leer(filepath.Join(datos, "Datos", "notes")); err != nil {
		h.Fallos["notes"] = err
	} else {
		n := leerNotas(filas)
		h.Tareas, h.TotalTareas = n.tareas(fin)
		h.Recordatorios = n.recordatoriosEntre(inicio, fin)
	}
	if ruta := rutaCorreo(datos); ruta != "" {
		var err error
		if h.SinLeer, h.Correos, err = sinLeer(ruta, MaxCorreosHoy); err != nil {
			h.Fallos["mail"] = err
		}
	}
	return h
}

// Vacio dice que no hay nada para hoy.
func (h Hoy) Vacio() bool {
	return len(h.Eventos) == 0 && len(h.Tareas) == 0 && len(h.Recordatorios) == 0 && h.SinLeer == 0
}

// ----- Calendar -----

// calendarioLeido son los eventos vivos de los calendarios vivos y el iCalendar de cada
// objeto (de donde salen sus recordatorios).
type calendarioLeido struct {
	eventos []event.Event
	datos   map[string]string // ObjectID → VCALENDAR
}

func leerCalendario(filas []almacen.Fila) calendarioLeido {
	type vivo struct {
		DeletedAt *time.Time `json:"deleted_at"`
	}
	c := calendarioLeido{datos: map[string]string{}}
	vivos := map[string]bool{}
	var evs []event.Event
	for _, f := range filas {
		var a agregado
		if json.Unmarshal(f.JSON, &a) != nil {
			continue
		}
		for _, r := range a.Rows {
			var v vivo
			if json.Unmarshal(r.JSON, &v) != nil || v.DeletedAt != nil {
				continue
			}
			switch r.Kind {
			case "calendar":
				vivos[r.ID] = true
			case "event":
				var ev event.Event
				if json.Unmarshal(r.JSON, &ev) == nil {
					ev.Attendees = nil
					evs = append(evs, ev)
				}
			case "object":
				var o struct {
					Data string `json:"data"`
				}
				if json.Unmarshal(r.JSON, &o) == nil && o.Data != "" {
					c.datos[r.ID] = o.Data
				}
			}
		}
	}
	for _, ev := range evs {
		if vivos[ev.CalendarID.String()] {
			c.eventos = append(c.eventos, ev)
		}
	}
	return c
}

// ocurrencias son las veces de los eventos que tocan el día [desde, hasta), por su
// inicio. Un evento de todo el día es de una FECHA, no de unas horas (va a medianoche
// UTC, como en `calendar`): cuenta si su fecha es la del día en la zona de `desde`, sin
// que la diferencia horaria lo pase al día de al lado.
func ocurrencias(c calendarioLeido, desde, hasta time.Time) []Ocurrencia {
	dia := time.Date(desde.Year(), desde.Month(), desde.Day(), 0, 0, 0, 0, time.UTC)
	var out []Ocurrencia
	for _, i := range event.ExpandEvents(c.eventos, desde.AddDate(0, 0, -1), hasta.AddDate(0, 0, 1)) {
		if i.Event.AllDay {
			ini := time.Date(i.Start.UTC().Year(), i.Start.UTC().Month(), i.Start.UTC().Day(), 0, 0, 0, 0, time.UTC)
			if ini.After(dia) || !i.End.UTC().After(dia) {
				continue
			}
		} else if !i.Start.Before(hasta) || !i.End.After(desde) && !i.Start.Equal(desde) {
			continue
		}
		out = append(out, Ocurrencia{
			Elemento: Elemento{
				Producto: "calendar", Clase: Evento, ID: i.Event.ID.String(), Titulo: i.Event.Summary,
				Detalle: i.Event.Location, Fecha: i.Start,
			},
			Fin: i.End, TodoElDia: i.Event.AllDay,
		})
	}
	slices.SortStableFunc(out, func(a, b Ocurrencia) int {
		// Los de todo el día, delante.
		if a.TodoElDia != b.TodoElDia {
			if a.TodoElDia {
				return -1
			}
			return 1
		}
		return cmp.Or(a.Fecha.Compare(b.Fecha), strings.Compare(a.Titulo, b.Titulo))
	})
	return out
}

// ----- Notepads -----

type notaLeida struct {
	ID          string     `json:"id"`
	Title       string     `json:"title"`
	Kind        string     `json:"kind"`
	Checked     bool       `json:"checked"`
	CancelledAt *time.Time `json:"cancelled_at"`
	NotebookID  string     `json:"notebook_id"`
	Cipher      string     `json:"content_cipher"`
	CreatedAt   time.Time  `json:"created_at"`
	DeletedAt   *time.Time `json:"deleted_at"`
}

type recordatorioLeido struct {
	NoteID    string     `json:"note_id"`
	DueAt     time.Time  `json:"due_at"`
	AllDay    bool       `json:"all_day"`
	RRule     string     `json:"rrule"`
	AfterDays int        `json:"after_days"`
	FiredAt   *time.Time `json:"fired_at"`
	DeletedAt *time.Time `json:"deleted_at"`
}

// notasLeidas son las notas legibles (ni borradas ni de un cuaderno cifrado) y los
// recordatorios que siguen en pie, con la regla de `notes.ListReminders`: uno que ya
// sonó solo sigue si repite y es de una tarea por hacer.
type notasLeidas struct {
	notas         map[string]notaLeida
	orden         []string
	recordatorios []recordatorioLeido
}

func leerNotas(filas []almacen.Fila) notasLeidas {
	n := notasLeidas{notas: map[string]notaLeida{}}
	cifrados := map[string]bool{}
	var notas []notaLeida
	var recs []recordatorioLeido
	for _, f := range filas {
		switch f.Entidad {
		case "notebook":
			var nb struct {
				ID        string `json:"id"`
				Encrypted bool   `json:"encrypted"`
			}
			if json.Unmarshal(f.JSON, &nb) == nil && nb.Encrypted {
				cifrados[cmp.Or(nb.ID, f.ID)] = true
			}
		case "note":
			var x notaLeida
			if json.Unmarshal(f.JSON, &x) == nil && x.DeletedAt == nil && x.Cipher == "" {
				x.ID = cmp.Or(x.ID, f.ID)
				notas = append(notas, x)
			}
		case "reminder":
			var r recordatorioLeido
			if json.Unmarshal(f.JSON, &r) == nil && r.DeletedAt == nil && r.NoteID != "" && !r.DueAt.IsZero() {
				recs = append(recs, r)
			}
		}
	}
	for _, x := range notas {
		if !cifrados[x.NotebookID] {
			n.notas[x.ID] = x
			n.orden = append(n.orden, x.ID)
		}
	}
	for _, r := range recs {
		if nota, ok := n.notas[r.NoteID]; ok && r.vigente(nota) {
			n.recordatorios = append(n.recordatorios, r)
		}
	}
	slices.SortFunc(n.recordatorios, func(a, b recordatorioLeido) int {
		return cmp.Or(a.DueAt.Compare(b.DueAt), strings.Compare(a.NoteID, b.NoteID))
	})
	return n
}

// vigente dice si el recordatorio de una nota legible sigue en pie: si no ha sonado o,
// si ya sonó, si repite y es de una tarea por hacer.
func (r recordatorioLeido) vigente(nota notaLeida) bool {
	repite := r.RRule != "" || r.AfterDays > 0
	return r.FiredAt == nil || nota.Kind == "todo" && !nota.Checked && repite
}

func (n notasLeidas) elemento(id string, fecha time.Time) Elemento {
	x := n.notas[id]
	return Elemento{Producto: "notes", Clase: Nota, ID: x.ID, Titulo: x.Title, Fecha: fecha}
}

// tareas son las tareas por hacer: primero las que vencen antes de `hasta` (por su
// vencimiento), luego las demás de la más nueva a la más vieja.
func (n notasLeidas) tareas(hasta time.Time) ([]Elemento, int) {
	vence := map[string]time.Time{}
	for _, r := range n.recordatorios {
		if _, ya := vence[r.NoteID]; !ya {
			vence[r.NoteID] = r.DueAt
		}
	}
	var out []Elemento
	for _, id := range n.orden {
		x := n.notas[id]
		if x.Kind != "todo" || x.Checked || x.CancelledAt != nil {
			continue
		}
		fecha := x.CreatedAt
		if v, ok := vence[id]; ok && v.Before(hasta) {
			fecha = v
		}
		out = append(out, n.elemento(id, fecha))
	}
	vencida := func(e Elemento) bool { v, ok := vence[e.ID]; return ok && v.Before(hasta) }
	slices.SortStableFunc(out, func(a, b Elemento) int {
		va, vb := vencida(a), vencida(b)
		switch {
		case va && vb:
			return a.Fecha.Compare(b.Fecha)
		case va:
			return -1
		case vb:
			return 1
		}
		return b.Fecha.Compare(a.Fecha)
	})
	return out[:min(len(out), MaxTareasHoy)], len(out)
}

// recordatoriosEntre son los recordatorios que suenan en [desde, hasta).
func (n notasLeidas) recordatoriosEntre(desde, hasta time.Time) []Elemento {
	var out []Elemento
	for _, r := range n.recordatorios {
		if !r.DueAt.Before(desde) && r.DueAt.Before(hasta) {
			out = append(out, n.elemento(r.NoteID, r.DueAt))
		}
	}
	return out
}
