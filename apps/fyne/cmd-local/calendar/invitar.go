// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package main

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"git.sytue.eu/sytue/one/apps/fyne/calendario"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/contacts/lector"
	"git.sytue.eu/sytue/one/apps/nucleo/local/agenda"
)

// Invitar desde el formulario (A-11, A-13). La pantalla sigue sin personas (no hay
// servicio de Sytue que avise a nadie), pero en un calendario de una CUENTA vuelve el
// campo de los invitados (`calendario.Pantalla.Invitar`): el evento se guarda aquí con
// sus ATTENDEE y su ORGANIZER —la dirección de la persona en esa cuenta— y la
// sincronización lo sube, en el acto. La invitación la manda quien sabe: Google
// (`sendUpdates=all`), Microsoft (al guardar con asistentes), el servidor CalDAV que
// planifica o, si no planifica, Calendar por correo con la cuenta de Mail de esa
// dirección (iMIP, `agenda.Cartero`, con su cola de reintentos).
//
// En un calendario de solo este equipo no se invita, y el formulario lo dice: el evento
// no está en ningún servidor, así que nadie recibiría sus cambios hechos en otro equipo,
// y las respuestas de los invitados llegarían a Mail sin evento de una cuenta al que
// devolverlas (con un CalDAV que no planifica, Mail las deja para Calendar, que las pone
// en el evento: respuestas.go). Mandar correos desde un calendario que la persona quiso
// dejar en el equipo sería además una sorpresa. Para invitar, se elige un calendario de
// una cuenta.
//
// El autocompletado es el de la libreta de Contacts de este equipo, leída en solo
// lectura (`lector.Libreta`), y la ocupación de los invitados en «buscar un hueco» la
// da el proveedor de la cuenta del calendario (`agenda.Registro.Ocupacion`).

// infoCalendario es lo que se sabe de un calendario de una cuenta sin leer la red.
type infoCalendario struct {
	cuenta      agenda.Cuenta
	planifica   bool
	soloLectura bool
}

// calendarios es la tabla de los calendarios de las cuentas, que se rehace con cada
// sincronización (fuera del hilo de la interfaz: leerla nunca espera a la red).
type calendarios struct {
	mu  sync.Mutex
	por map[string]infoCalendario
}

func (c *calendarios) poner(gs []agenda.Grupo) {
	m := map[string]infoCalendario{}
	for _, g := range gs {
		for _, x := range g.Colecciones {
			m[x.ID] = infoCalendario{cuenta: g.Cuenta, planifica: x.Planifica, soloLectura: x.SoloLectura}
		}
	}
	c.mu.Lock()
	c.por = m
	c.mu.Unlock()
}

func (c *calendarios) de(id string) (infoCalendario, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	i, ok := c.por[id]
	return i, ok
}

// buzonesMail son las direcciones de las cuentas de Mail, leídas como mucho cada medio
// minuto (la pista del formulario las mira al pintarse).
type buzonesMail struct {
	mu    sync.Mutex
	leido time.Time
	dirs  []string
}

func (s *sincronia) hayMail(direccion string) bool {
	b := &s.buzones
	b.mu.Lock()
	defer b.mu.Unlock()
	if time.Since(b.leido) > 30*time.Second {
		b.leido, b.dirs = time.Now(), nil
		if bs, err := s.reg.Correo.Buzones(); err == nil {
			for _, x := range bs {
				b.dirs = append(b.dirs, strings.ToLower(strings.TrimSpace(x.Direccion)))
			}
		}
	}
	buscada := strings.ToLower(direccion)
	for _, d := range b.dirs {
		if d == buscada && d != "" {
			return true
		}
	}
	return false
}

// invitar es lo que se da a la pantalla.
func (s *sincronia) invitar(lib *lector.Libreta) *calendario.Invitar {
	return &calendario.Invitar{
		En: func(cal string) bool {
			i, ok := s.cals.de(cal)
			return ok && !i.soloLectura
		},
		Pista:    s.pistaInvitar,
		PistaSin: texto(s.l, "invitar.local"),
		Sugerir: func(ctx context.Context, q string, n int) ([]calendario.SugerenciaInvitado, error) {
			ds, err := lib.Buscar(ctx, q, n)
			out := make([]calendario.SugerenciaInvitado, 0, len(ds))
			for _, d := range ds {
				out = append(out, calendario.SugerenciaInvitado{Nombre: d.Nombre, Correo: d.Correo})
			}
			return out, err
		},
		Ocupacion: func(ctx context.Context, cal string, correos []string, desde, hasta time.Time) (map[string][]api.Hueco, error) {
			i, ok := s.cals.de(cal)
			if !ok {
				return nil, agenda.ErrSinCuenta
			}
			m, err := s.reg.Ocupacion(ctx, i.cuenta.ID, correos, desde, hasta)
			if err != nil {
				return nil, err
			}
			out := make(map[string][]api.Hueco, len(m))
			for k, ts := range m {
				hs := []api.Hueco{}
				for _, t := range ts {
					hs = append(hs, api.Hueco{Start: t.Desde, End: t.Hasta})
				}
				out[k] = hs
			}
			return out, nil
		},
		ResumenOcupacion: func(con, sin []string) string {
			var partes []string
			if len(con) > 0 {
				partes = append(partes, fmt.Sprintf(texto(s.l, "ocupacion.con"), strings.Join(con, ", ")))
			}
			if len(sin) > 0 {
				partes = append(partes, fmt.Sprintf(texto(s.l, "ocupacion.sin"), strings.Join(sin, ", ")))
			}
			return strings.Join(partes, " ")
		},
	}
}

// pistaInvitar dice, bajo el campo, por dónde saldrá la invitación en ese calendario.
func (s *sincronia) pistaInvitar(cal string) string {
	i, ok := s.cals.de(cal)
	if !ok {
		return ""
	}
	switch {
	case i.cuenta.Proveedor == agenda.ProveedorGoogle:
		return texto(s.l, "invitar.google")
	case i.cuenta.Proveedor == agenda.ProveedorMicrosoft:
		return texto(s.l, "invitar.outlook")
	case i.planifica:
		return texto(s.l, "invitar.servidor")
	}
	dir := s.reg.Direccion(i.cuenta.ID)
	if dir != "" && s.reg.Correo != nil && s.hayMail(dir) {
		return fmt.Sprintf(texto(s.l, "invitar.correo"), dir)
	}
	return fmt.Sprintf(texto(s.l, "invitar.sinmail"), cmpOr(dir, i.cuenta.Rotulo))
}

func cmpOr(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// conOrganizador pone de ORGANIZER la dirección de la persona en la cuenta del
// calendario a un evento nuevo (o sin organizador) con invitados: es a quien contestan
// y quien manda los cambios. Uno que organiza otro no se toca.
func (s *sincronia) conOrganizador(cal string, in api.EventInput) api.EventInput {
	if s == nil || in.Organizer != "" || len(in.Attendees) == 0 {
		return in
	}
	i, ok := s.cals.de(cal)
	if !ok || i.soloLectura {
		return in
	}
	if dir := s.reg.Direccion(i.cuenta.ID); dir != "" {
		in.Organizer = "mailto:" + dir
	}
	return in
}

// marcarReunion pone al evento recién guardado la reunión pedida en el formulario
// (reunion.go), antes de que la sincronización lo suba.
func (s *sincronia) marcarReunion(ctx context.Context, ev api.Event) {
	if s != nil {
		s.reuniones.marcar(ctx, ev)
	}
}

// trasEscribir pide sincronizar en cuanto se pueda tras cambiar un evento de una cuenta:
// sus invitaciones salen ya, no a los diez minutos.
func (s *sincronia) trasEscribir(cal string) {
	if s == nil || s.motor == nil {
		return
	}
	if _, ok := s.cals.de(cal); ok {
		s.motor.Ya()
	}
}
