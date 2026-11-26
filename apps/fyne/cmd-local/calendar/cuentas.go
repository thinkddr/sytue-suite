// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/calendario"
	"git.sytue.eu/sytue/one/apps/fyne/local"
	privateCalendar "git.sytue.eu/sytue/one/apps/nucleo/calendar"
	"git.sytue.eu/sytue/one/apps/nucleo/local/agenda"
)

// Las cuentas de calendario de la persona (A-10, A-12): cada calendario de una cuenta
// CalDAV, Google o Microsoft es uno de aquí, que se lee y se escribe siempre en el
// equipo; `local/agenda` lo reconcilia con la cuenta al abrir, al volver a primer plano,
// cada diez minutos con la app abierta y a mano («Sincronizar ahora» en la sección de
// la cuenta, o «Actualizar ahora» en un calendario de solo lectura). Sin cuentas no se
// hace nada, y toda la red sale por el portero hacia los servidores de cada una.
//
// Las cuentas se dan de alta en Ajustes → Cuentas (`agenda.Registro.AnadirDAV` y
// `ConectarOAuth`), y valen también para Contacts. En la sección de cada cuenta,
// «Crear calendario» lo crea en la cuenta (MKCALENDAR en CalDAV) y, en una cuenta CalDAV
// cuyo servidor no manda las invitaciones, las manda Calendar por correo
// (`invitaciones.go`, A-11), con una cola que reintenta lo que no sale: al arrancar,
// pasada la espera de cada uno y, sin esperar, cuando la red vuelve; las respuestas de
// los invitados, que llegan a Mail, vuelven a sus eventos por la cola de Mail
// (`respuestas.go`). En los eventos de sus calendarios se invita desde el formulario
// (`invitar.go`).

type sincronia struct {
	l     *local.App
	reg   *agenda.Registro
	local agenda.LocalCalendario
	dir   string
	motor *agenda.Motor
	// cals son los calendarios de las cuentas (invitar.go) y buzones, las direcciones de
	// las cuentas de Mail.
	cals    calendarios
	buzones buzonesMail
	// reuniones crea el enlace de la reunión desde el formulario (reunion.go).
	reuniones *reuniones

	// arrancado y sinRed son de la vuelta anterior (solo las toca el motor): la primera
	// vacía la cola entera, y una en que la red vuelve, también.
	arrancado, sinRed bool
	// reintento es el temporizador del próximo reintento de la cola de invitaciones.
	mt        sync.Mutex
	reintento *time.Timer

	p *calendario.Pantalla
	c *calendario.App
	w fyne.Window
}

func nuevaSincronia(l *local.App, repo *privateCalendar.Repository) *sincronia {
	s := &sincronia{
		l:     l,
		reg:   l.Agenda(),
		local: agenda.LocalCalendario{Repo: repo},
		dir:   filepath.Join(l.Espacio.Rutas.Producto("calendar"), "agenda"),
	}
	s.reg.MarcaCopia = texto(l, "sync.copia")
	// Antes de la primera sincronización: los sincronizadores lo toman al crearse.
	s.reg.Correo, s.reg.TextosIMIP = nuevoCorreo(l), textosIMIP(l)
	s.reg.Cola = &agenda.ColaIMIP{Dir: filepath.Join(s.dir, "imip")}
	s.motor = &agenda.Motor{Hacer: s.hacer}
	s.reuniones = nuevasReuniones(s, repo)
	return s
}

// esDeCuenta dice si un calendario de aquí es de una cuenta.
func (s *sincronia) esDeCuenta(id string) bool {
	_, ok := s.cals.de(id)
	return ok
}

// arrancar enseña ya las cuentas (sin red) y empieza a sincronizar.
func (s *sincronia) arrancar(ctx context.Context, a fyne.App, p *calendario.Pantalla, c *calendario.App, w fyne.Window) {
	s.p, s.c, s.w = p, c, w
	p.Cuentas = s.grupos()
	s.motor.Arrancar(ctx)
	a.Lifecycle().SetOnEnteredForeground(s.motor.Ya)
	// Una respuesta de un invitado que Mail deja en su cola pide la vuelta ya (respuestas.go).
	s.vigilarRespuestas(ctx)
}

func (s *sincronia) grupos() []calendario.GrupoCuenta {
	gs := s.reg.Grupos(agenda.Calendario, s.local, s.dir)
	out := make([]calendario.GrupoCuenta, 0, len(gs))
	s.cals.poner(gs)
	for _, g := range gs {
		gc := calendario.GrupoCuenta{Rotulo: g.Cuenta.Rotulo, RotuloSincronizar: texto(s.l, "sync.ahora"), AlSincronizar: s.motor.Ya}
		if g.Cuenta.Estado == agenda.EstadoReconectar {
			gc.Aviso = texto(s.l, "sync.reconectar")
		} else if s.reg.PuedeCrear(g.Cuenta, agenda.Calendario) {
			id := g.Cuenta.ID
			gc.CrearCalendario = func(ctx context.Context, nombre, color string) error {
				if err := s.reg.CrearCalendario(ctx, id, nombre, color); err != nil {
					slog.Warn("calendar: no se pudo crear el calendario en la cuenta", "error", err)
					return err
				}
				// Aparece aquí con la sincronización, en la sección de su cuenta.
				s.motor.Ya()
				return nil
			}
		}
		for _, c := range g.Colecciones {
			gc.Calendarios = append(gc.Calendarios, c.ID)
		}
		out = append(out, gc)
	}
	return out
}

func (s *sincronia) hacer(ctx context.Context) {
	if len(s.reg.Cuentas()) == 0 {
		return
	}
	// Lo que esperaba en la cola de invitaciones sale antes que lo nuevo; al arrancar,
	// todo, sin mirar su espera.
	s.vaciarCola(ctx, !s.arrancado)
	s.arrancado = true
	// Las respuestas de los invitados que llegaron por correo (REPLY), a sus eventos antes
	// de subir: esta misma vuelta las sube (respuestas.go).
	respuestas := s.aplicarRespuestas(ctx)
	res, errs := s.reg.SincronizarTodo(ctx, agenda.Calendario, s.local, s.dir, texto(s.l, "sync.copia"))
	for _, err := range errs {
		slog.Warn("calendar: no se pudo sincronizar una cuenta", "error", err)
	}
	// La red volvió: la vuelta anterior no llegó a ninguna cuenta (y no por la
	// contraseña) y esta sí. Lo de la cola sale ya, sin esperar.
	sinRed := len(errs) > 0 && len(errs) == len(res) && !slices.ContainsFunc(errs, func(e error) bool { return errors.Is(e, agenda.ErrAcceso) })
	if s.sinRed && !sinRed {
		s.vaciarCola(ctx, true)
	}
	s.sinRed = sinRed
	defer s.programarCola(ctx)
	cambios, conflictos := respuestas, 0
	var invitaciones []error
	for _, r := range res {
		cambios += r.Bajados + r.Borrados + r.Devueltos
		conflictos += len(r.Conflictos)
		invitaciones = append(invitaciones, r.Invitaciones...)
		for _, err := range r.Errores {
			slog.Warn("calendar: un calendario de una cuenta no se sincronizó", "error", err)
		}
	}
	avisos := avisosIMIP(s.l, invitaciones)
	if n := s.reg.Cola.Cuantos(); len(avisos) > 0 && n > 0 {
		avisos = append(avisos, fmt.Sprintf(texto(s.l, "imip.cola"), n))
	}
	grupos := s.grupos()
	if s.p == nil {
		return
	}
	fyne.Do(func() {
		s.p.Cuentas = grupos
		s.c.RecargarCalendarios()
		if cambios > 0 {
			s.c.Recargar()
		}
		if conflictos > 0 && s.w != nil {
			local.Info(s.w, texto(s.l, "sync.conflicto.titulo"), fmt.Sprintf(texto(s.l, "sync.conflicto"), conflictos))
		}
		if len(avisos) > 0 && s.w != nil {
			local.Info(s.w, texto(s.l, "imip.titulo"), strings.Join(avisos, "\n\n"))
		}
	})
}

// vaciarCola manda lo que toca de la cola de invitaciones (todo: sin mirar la espera de
// cada una). Lo que vuelve a fallar se queda para el siguiente reintento, sin otro aviso.
func (s *sincronia) vaciarCola(ctx context.Context, todo bool) {
	inf := s.reg.VaciarIMIP(ctx, todo)
	for _, err := range inf.Errores {
		slog.Warn("calendar: una invitación de la cola volvió a fallar", "error", err)
	}
	if inf.Enviados > 0 {
		slog.Info("calendar: salieron invitaciones que esperaban", "enviadas", inf.Enviados, "quedan", inf.Quedan)
	}
	if inf.Caducados > 0 {
		slog.Warn("calendar: se tiraron invitaciones de hace más de un mes que no salieron", "tiradas", inf.Caducados)
	}
}

// programarCola deja preparado el reintento siguiente de la cola, a su hora (sin
// esperar a la próxima vuelta de la sincronización, que es cada diez minutos).
func (s *sincronia) programarCola(ctx context.Context) {
	var prox time.Time
	if s.reg.Correo != nil {
		if bs, err := s.reg.Correo.Buzones(); err == nil {
			prox = s.reg.Cola.Proximo(bs)
		}
	}
	s.mt.Lock()
	defer s.mt.Unlock()
	if s.reintento != nil {
		s.reintento.Stop()
		s.reintento = nil
	}
	if prox.IsZero() || ctx.Err() != nil {
		return
	}
	s.reintento = time.AfterFunc(max(time.Until(prox), time.Second), func() {
		if ctx.Err() != nil {
			return
		}
		s.vaciarCola(ctx, false)
		s.programarCola(ctx)
	})
}
