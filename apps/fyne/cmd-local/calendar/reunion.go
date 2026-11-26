// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"git.sytue.eu/sytue/one/apps/fyne/calendario"
	"git.sytue.eu/sytue/one/apps/fyne/local"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	privateCalendar "git.sytue.eu/sytue/one/apps/nucleo/calendar"
	"git.sytue.eu/sytue/one/apps/nucleo/local/agenda"
	"git.sytue.eu/sytue/one/apps/nucleo/local/chat"
	"git.sytue.eu/sytue/one/apps/nucleo/local/red"
)

// El enlace de la reunión desde el formulario (la otra mitad de A-13), bajo el campo libre
// del enlace (`calendario.Pantalla.Reunion`):
//
//   - en un evento de un calendario de Google, «Añadir Google Meet»; en uno de Microsoft,
//     «Reunión de Teams». El enlace lo crea el proveedor al subir el evento (Calendar API
//     `conferenceData.createRequest`; Graph `isOnlineMeeting`), sin pedir otro permiso: al
//     guardar, el evento queda marcado (`agenda.PropReunion`, con
//     `calendar.Repository.PonerExtension`), la sincronización lo pide y lo que vuelve
//     —el evento con su enlace— reemplaza al de aquí;
//   - en los demás (CalDAV, este equipo), si hay una cuenta de mensajería (Rooms) con
//     Teams o Meet, la reunión se crea ya con ella —con su token, por el portero, sin
//     abrir Rooms— y el enlace se pega en el campo;
//   - si no, el campo libre de siempre.

// vigenciaPedida es cuánto vale una reunión pedida en el formulario: la pide el botón y
// se apunta al guardar; un guardado que falla y se deja estar no la lleva a otro día.
const vigenciaPedida = 10 * time.Minute

type pedidaReunion struct {
	tipo   string // agenda.ReunionMeet o agenda.ReunionTeams
	cuando time.Time
}

// reuniones son las pedidas al guardar y la cuenta de mensajería que crea reuniones.
type reuniones struct {
	s    *sincronia
	repo *privateCalendar.Repository

	mu      sync.Mutex
	pedidas map[string]pedidaReunion // por UID del evento
	// leido y proveedor son la última vez que se miraron las cuentas de mensajería y el
	// nombre del servicio con que se crearía una reunión ("" si ninguna).
	leido     time.Time
	proveedor string
}

func nuevasReuniones(s *sincronia, repo *privateCalendar.Repository) *reuniones {
	return &reuniones{s: s, repo: repo, pedidas: map[string]pedidaReunion{}}
}

// registroChat son las cuentas de mensajería de la edición (las de Rooms), que hablan
// por el portero con sus servidores; nil si no se pueden leer.
func (r *reuniones) registroChat() *chat.Registro {
	l := r.s.l
	reg, err := chat.AbrirRegistro(l.Espacio.Ajuste("chat-cuentas.json"), l.Secretos)
	if err != nil {
		return nil
	}
	reg.Cliente = l.Red.Cliente(red.Chat)
	reg.ClienteOAuth = l.Red.Cliente(red.OAuth)
	reg.Clientes = l.Nube.Clientes
	return reg
}

// proveedorChat es el servicio («Microsoft Teams», «Google Meet») de la primera cuenta de
// mensajería que crea reuniones, sin leer la red (como mucho cada medio minuto).
func (r *reuniones) proveedorChat() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	if time.Since(r.leido) < 30*time.Second {
		return r.proveedor
	}
	r.leido, r.proveedor = time.Now(), ""
	if reg := r.registroChat(); reg != nil {
		for _, c := range reg.Cuentas() {
			if c.Estado == chat.EstadoReconectar {
				continue
			}
			switch c.Proveedor {
			case chat.ProveedorTeams:
				r.proveedor = "Microsoft Teams"
			case chat.ProveedorGoogle:
				r.proveedor = "Google Meet"
			}
			if r.proveedor != "" {
				break
			}
		}
	}
	return r.proveedor
}

// tipoDe es lo que pide el proveedor de un calendario de una cuenta ("" si no la crea él).
func (r *reuniones) tipoDe(cal string) string {
	i, ok := r.s.cals.de(cal)
	switch {
	case !ok || i.soloLectura:
		return ""
	case i.cuenta.Proveedor == agenda.ProveedorGoogle:
		return agenda.ReunionMeet
	case i.cuenta.Proveedor == agenda.ProveedorMicrosoft:
		return agenda.ReunionTeams
	}
	return ""
}

// pantalla es lo que se da al formulario.
func (r *reuniones) pantalla(l *local.App) *calendario.Reunion {
	return &calendario.Reunion{
		Boton: func(cal string) string {
			if i, ok := r.s.cals.de(cal); ok && i.soloLectura {
				return ""
			}
			switch r.tipoDe(cal) {
			case agenda.ReunionMeet:
				return texto(l, "reunion.meet")
			case agenda.ReunionTeams:
				return texto(l, "reunion.teams")
			}
			if p := r.proveedorChat(); p != "" {
				return fmt.Sprintf(texto(l, "reunion.rooms"), p)
			}
			return ""
		},
		Pendiente: func(cal string) string {
			switch r.tipoDe(cal) {
			case agenda.ReunionMeet:
				return texto(l, "reunion.meet.pendiente")
			case agenda.ReunionTeams:
				return texto(l, "reunion.teams.pendiente")
			}
			return ""
		},
		Crear: func(ctx context.Context, _, titulo string, inicio, fin time.Time) (string, error) {
			return r.crearConChat(ctx, titulo, inicio, fin)
		},
		Pedir: func(cal, uid string) {
			if t := r.tipoDe(cal); t != "" && uid != "" {
				r.mu.Lock()
				r.pedidas[uid] = pedidaReunion{tipo: t, cuando: time.Now()}
				r.mu.Unlock()
			}
		},
	}
}

var errSinCuentaDeReuniones = errors.New("calendar: no hay ninguna cuenta de mensajería que cree reuniones")

// crearConChat crea la reunión con la primera cuenta de mensajería que sepa: Teams con
// el título y las horas del evento; Meet (que no tiene asunto ni horas), una sala.
func (r *reuniones) crearConChat(ctx context.Context, titulo string, inicio, fin time.Time) (string, error) {
	reg := r.registroChat()
	if reg == nil {
		return "", errSinCuentaDeReuniones
	}
	for _, c := range reg.Cuentas() {
		if c.Estado == chat.EstadoReconectar || (c.Proveedor != chat.ProveedorTeams && c.Proveedor != chat.ProveedorGoogle) {
			continue
		}
		p, err := reg.Abrir(c)
		if err != nil {
			slog.Warn("calendar: no se pudo abrir la cuenta de mensajería", "cuenta", c.Rotulo, "error", err)
			continue
		}
		if t, ok := p.(*chat.Teams); ok && t.G != nil {
			return t.G.CrearReunion(ctx, titulo, inicio, fin)
		}
		if cr, ok := p.(chat.Reuniones); ok {
			return cr.CrearReunion(ctx, titulo)
		}
	}
	return "", errSinCuentaDeReuniones
}

// marcar, tras guardar ev, le pone la marca de la reunión pedida en el formulario (si se
// pidió hace poco y su calendario es del proveedor que la crea): la sincronización la
// pide al subirlo.
func (r *reuniones) marcar(ctx context.Context, ev api.Event) {
	if r == nil || ev.UID == "" {
		return
	}
	r.mu.Lock()
	p, ok := r.pedidas[ev.UID]
	delete(r.pedidas, ev.UID)
	for uid, x := range r.pedidas {
		if time.Since(x.cuando) > vigenciaPedida {
			delete(r.pedidas, uid)
		}
	}
	r.mu.Unlock()
	if !ok || time.Since(p.cuando) > vigenciaPedida || r.tipoDe(ev.CalendarID) != p.tipo {
		return
	}
	if err := r.repo.PonerExtension(ctx, ev.ID, agenda.PropReunion, p.tipo); err != nil {
		slog.Warn("calendar: no se pudo pedir la reunión del evento", "error", err)
	}
}
