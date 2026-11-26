// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package main

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/contactos"
	"git.sytue.eu/sytue/one/apps/fyne/local"
	privateContacts "git.sytue.eu/sytue/one/apps/nucleo/contacts"
	"git.sytue.eu/sytue/one/apps/nucleo/local/agenda"
)

// Las cuentas de contactos de la persona (K-05, K-06): cada libreta de una cuenta
// CardDAV, Google o Microsoft es una de aquí, que se lee y se escribe siempre en el
// equipo (sus grupos de CardDAV son sus etiquetas, y una libreta que la cuenta no deja
// escribir no se edita aquí, con su aviso sobre la lista); `local/agenda` la reconcilia con la cuenta al abrir, al volver a primer plano,
// cada diez minutos con la app abierta y a mano («Sincronizar ahora» en la sección de
// la cuenta). Sin cuentas no se hace nada. Son las mismas cuentas que las de Calendar
// (`<Ajustes>/agenda.json`), que se dan de alta en Ajustes → Cuentas.
//
// Las etiquetas suben también a Google (sus grupos de contactos) y a Outlook (las
// categorías de los contactos), y «Nueva libreta» en la sección de una cuenta CardDAV o
// de Microsoft la crea en la cuenta (el MKCOL extendido; una carpeta de contactos).
// Google no tiene más que una libreta: allí no sale.

type sincronia struct {
	l     *local.App
	reg   *agenda.Registro
	local agenda.LocalContactos
	dir   string
	motor *agenda.Motor

	c *contactos.App
	w fyne.Window
}

func nuevaSincronia(l *local.App, repo privateContacts.LibretasExternas) *sincronia {
	s := &sincronia{
		l:     l,
		reg:   l.Agenda(),
		local: agenda.LocalContactos{Repo: repo},
		dir:   filepath.Join(l.Espacio.Rutas.Producto("contacts"), "agenda"),
	}
	s.reg.MarcaCopia = texto(l, "sync.copia")
	s.motor = &agenda.Motor{Hacer: s.hacer}
	return s
}

func (s *sincronia) arrancar(ctx context.Context, a fyne.App, c *contactos.App, w fyne.Window) {
	s.c, s.w = c, w
	c.Pantalla.Cuentas = s.grupos()
	s.motor.Arrancar(ctx)
	a.Lifecycle().SetOnEnteredForeground(s.motor.Ya)
}

func (s *sincronia) grupos() []contactos.GrupoCuenta {
	grupos := s.reg.Grupos(agenda.Contactos, s.local, s.dir)
	out := make([]contactos.GrupoCuenta, 0, len(grupos))
	for _, g := range grupos {
		gc := contactos.GrupoCuenta{
			Rotulo: g.Cuenta.Rotulo, RotuloSincronizar: texto(s.l, "sync.ahora"), AlSincronizar: s.motor.Ya,
			RotuloSoloLectura: texto(s.l, "sync.sololectura"), AvisoSoloLectura: texto(s.l, "sync.sololectura.aviso"), SoloLectura: map[string]bool{},
		}
		if g.Cuenta.Estado == agenda.EstadoReconectar {
			gc.Aviso = texto(s.l, "sync.reconectar")
		} else if s.reg.PuedeCrear(g.Cuenta, agenda.Contactos) {
			id := g.Cuenta.ID
			gc.CrearLibreta = func(ctx context.Context, nombre string) error {
				if err := s.reg.CrearLibreta(ctx, id, nombre); err != nil {
					slog.Warn("contacts: no se pudo crear la libreta en la cuenta", "error", err)
					return err
				}
				// Aparece aquí con la sincronización, en la sección de su cuenta.
				s.motor.Ya()
				return nil
			}
		}
		for _, c := range g.Colecciones {
			gc.Libretas = append(gc.Libretas, c.ID)
			gc.SoloLectura[c.ID] = c.SoloLectura
		}
		out = append(out, gc)
	}
	return out
}

func (s *sincronia) hacer(ctx context.Context) {
	if len(s.reg.Cuentas()) == 0 {
		return
	}
	res, errs := s.reg.SincronizarTodo(ctx, agenda.Contactos, s.local, s.dir, texto(s.l, "sync.copia"))
	for _, err := range errs {
		slog.Warn("contacts: no se pudo sincronizar una cuenta", "error", err)
	}
	cambios, conflictos := 0, 0
	for _, r := range res {
		cambios += r.Bajados + r.Borrados
		conflictos += len(r.Conflictos)
		for _, err := range r.Errores {
			slog.Warn("contacts: una libreta de una cuenta no se sincronizó", "error", err)
		}
	}
	grupos := s.grupos()
	if s.c == nil {
		return
	}
	fyne.Do(func() {
		antes := libretasDe(s.c.Pantalla.Cuentas)
		s.c.Pantalla.Cuentas = grupos
		// Una libreta nueva (creada aquí en la cuenta, o allí) también recarga la lista.
		if cambios > 0 || antes != libretasDe(grupos) || len(errs) > 0 {
			s.c.IniciarCarga()
		}
		if conflictos > 0 && s.w != nil {
			local.Info(s.w, texto(s.l, "sync.conflicto.titulo"), fmt.Sprintf(texto(s.l, "sync.conflicto"), conflictos))
		}
	})
}

// Textos propios de esta edición (§5.14): español e inglés aquí, los otros veinticuatro
// idiomas en textos_idiomas.go.
var textos = map[string][2]string{ // clave: {es, en}
	"libreta":          {"Contactos", "Contacts"},
	"sync.ahora":       {"Sincronizar ahora", "Sync now"},
	"sync.reconectar":  {"Hay que volver a entrar en esta cuenta (Ajustes → Cuentas).", "Sign in to this account again (Settings → Accounts)."},
	"sync.copia":       {"(copia en conflicto)", "(conflicted copy)"},
	"sync.sololectura": {"Solo lectura", "Read-only"},
	"sync.sololectura.aviso": {
		"Esta libreta es de solo lectura en su cuenta: aquí no se puede cambiar.",
		"This address book is read-only in its account, so it can’t be changed here.",
	},
	"migrar": {"Poniendo al día tus contactos", "Updating your contacts"},
	"migrar.texto": {
		"Sytue Contacts está pasando tus contactos a su libreta nueva, más rápida. Solo se hace una vez, y la de antes se guarda.",
		"Sytue Contacts is moving your contacts to its new, faster address book. This happens only once, and your old address book is kept.",
	},
	"migrar.fallo": {
		"No se han podido pasar tus contactos a la libreta nueva. Siguen como estaban; se volverá a intentar al abrir Sytue Contacts.",
		"Your contacts could not be moved to the new address book. They are as they were; it will be tried again when you open Sytue Contacts.",
	},
	"sync.conflicto.titulo": {"Cambios en los dos sitios", "Changed in both places"},
	"sync.conflicto":        {"%d contactos cambiaron aquí y en la cuenta a la vez. La versión de la cuenta queda en su sitio y la de este equipo, como «(copia en conflicto)».", "%d contacts changed here and in the account at the same time. The account’s version stays in place and this device’s is kept as “(conflicted copy)”."},
}

// texto es el texto propio de clave en el idioma de la app; lo que falte cae al inglés.
func texto(l *local.App, clave string) string {
	return local.Propios{Base: textos, Traducciones: traducciones}.En(l.Idioma(), clave)
}

// libretasDe cuenta las cuentas y sus libretas: si cambia, hay que recargar la lista.
func libretasDe(gs []contactos.GrupoCuenta) string {
	var b strings.Builder
	for _, g := range gs {
		b.WriteString(g.Rotulo + ":" + strings.Join(g.Libretas, ",") + ";")
	}
	return b.String()
}
