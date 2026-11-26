// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package avisos son los recordatorios programados de Sytue Office (PLAN-EDICION-LOCAL
// §5.12): los de Calendar (y, cuando los tenga en local, Notepads), que en la edición
// conectada entrega el servidor y aquí entrega EL SISTEMA, a su hora, sin ningún
// servidor.
//
// Lo que se decide —qué avisos hay en los próximos 30 días, cuáles han cambiado, cuáles
// sobran— se decide aquí, una vez para todas las plataformas ([Avisos.Reprogramar]). Lo
// que programa de verdad es un [Sistema]:
//
//	macOS, iOS   avisos locales programados del sistema (UNUserNotificationCenter)  con la app cerrada: sí
//	Android      AlarmManager + aviso local                                         con la app cerrada: sí
//	Windows      ScheduledToastNotification (escritorio.ProgramarToast)             con la app cerrada: sí
//	Linux        temporizador en el proceso                                         solo con la app abierta
//
// Los tres primeros y Linux los da el `ScheduleNotification` de Fyne (su fork los
// implementa así), salvo Windows, que va por [Windows] porque el de Fyne no escapa el
// texto. Lo enchufa la cáscara (`apps/fyne/local`); este paquete no depende de Fyne.
//
// Lo programado se apunta en `<Ajustes>/avisos.json` (clave → identificador del
// sistema) para poder quitarlo al volver a abrir: el sistema los guarda aunque la app
// se cierre.
package avisos

import (
	"cmp"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"slices"
	"strings"
	"sync"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/local/espacio"
)

// Ventana es hasta dónde se programa por delante. Al abrir la app se vuelve a programar.
const Ventana = 30 * 24 * time.Hour

// Maximo de avisos por origen: iOS no guarda más de 64 por app en total.
const Maximo = 48

// Aviso es un recordatorio.
type Aviso struct {
	// Clave es estable entre aperturas: «evento/inicio/minutos».
	Clave  string    `json:"clave"`
	Titulo string    `json:"titulo"`
	Cuerpo string    `json:"cuerpo,omitempty"`
	Cuando time.Time `json:"cuando"`
	// Enlace es lo que abre al pulsarlo (`sytueoffice-calendar:/…`), donde el sistema lo
	// admite.
	Enlace string `json:"enlace,omitempty"`
}

// Etiqueta es el identificador corto y estable de un aviso (16 hexadecimales): sin
// datos de la persona, porque el sistema lo guarda.
func (a Aviso) Etiqueta(origen string) string {
	h := sha256.Sum256([]byte(origen + "\x00" + a.Clave))
	return hex.EncodeToString(h[:8])
}

// Sistema programa y quita avisos en el sistema operativo.
type Sistema interface {
	// Programar deja el aviso para su hora y devuelve el identificador con que quitarlo.
	Programar(etiqueta string, a Aviso) (id string, err error)
	// Cancelar quita uno programado.
	Cancelar(id string) error
}

type programado struct {
	Origen string    `json:"origen"`
	Clave  string    `json:"clave"`
	ID     string    `json:"id"`
	Cuando time.Time `json:"cuando"`
	Huella string    `json:"huella"`
}

// Avisos lleva la cuenta de lo programado.
type Avisos struct {
	ruta string
	sis  Sistema

	mu          sync.Mutex
	programados []programado
}

// Abrir lee lo programado de ruta (vacía: solo en memoria) y lo programará con s.
func Abrir(ruta string, s Sistema) *Avisos {
	a := &Avisos{ruta: ruta, sis: s}
	// Lo que programa un reloj del proceso murió con el proceso anterior: no se lee, y
	// todo se vuelve a programar.
	if p, ok := s.(interface{ EnProceso() bool }); ok && p.EnProceso() {
		return a
	}
	if ruta != "" {
		var g struct {
			Programados []programado `json:"programados"`
		}
		_ = espacio.LeerJSON(ruta, &g)
		a.programados = g.Programados
	}
	return a
}

func huella(a Aviso) string {
	h := sha256.Sum256([]byte(a.Titulo + "\x00" + a.Cuerpo + "\x00" + a.Enlace + "\x00" + a.Cuando.UTC().Format(time.RFC3339)))
	return hex.EncodeToString(h[:8])
}

// Reprogramar deja programados exactamente los avisos de `lista` de ese origen que caen
// entre ahora y [Ventana] (como mucho [Maximo], los más próximos): quita los que ya no
// están o han cambiado, y programa los nuevos. Lo que ya estaba igual no se toca.
func (a *Avisos) Reprogramar(origen string, lista []Aviso, ahora time.Time) error {
	if a == nil || a.sis == nil {
		return nil
	}
	var quiero []Aviso
	for _, av := range lista {
		if av.Cuando.After(ahora) && !av.Cuando.After(ahora.Add(Ventana)) && av.Clave != "" {
			quiero = append(quiero, av)
		}
	}
	slices.SortFunc(quiero, func(x, y Aviso) int { return cmp.Or(x.Cuando.Compare(y.Cuando), strings.Compare(x.Clave, y.Clave)) })
	quiero = slices.CompactFunc(quiero, func(x, y Aviso) bool { return x.Clave == y.Clave })
	if len(quiero) > Maximo {
		quiero = quiero[:Maximo]
	}
	porClave := map[string]Aviso{}
	for _, av := range quiero {
		porClave[av.Clave] = av
	}

	a.mu.Lock()
	defer a.mu.Unlock()
	var errs []error
	var quedan []programado
	ya := map[string]bool{}
	for _, p := range a.programados {
		if p.Origen != origen {
			quedan = append(quedan, p)
			continue
		}
		av, sigue := porClave[p.Clave]
		if sigue && huella(av) == p.Huella && p.Cuando.After(ahora) {
			quedan = append(quedan, p)
			ya[p.Clave] = true
			continue
		}
		if err := a.sis.Cancelar(p.ID); err != nil {
			errs = append(errs, err)
		}
	}
	for _, av := range quiero {
		if ya[av.Clave] {
			continue
		}
		id, err := a.sis.Programar(av.Etiqueta(origen), av)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		quedan = append(quedan, programado{Origen: origen, Clave: av.Clave, ID: id, Cuando: av.Cuando, Huella: huella(av)})
	}
	a.programados = quedan
	if a.ruta != "" {
		if err := espacio.EscribirJSON(a.ruta, struct {
			Programados []programado `json:"programados"`
		}{quedan}); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// Programados son las claves y horas de lo programado de un origen, en orden.
func (a *Avisos) Programados(origen string) []Aviso {
	a.mu.Lock()
	defer a.mu.Unlock()
	var out []Aviso
	for _, p := range a.programados {
		if p.Origen == origen {
			out = append(out, Aviso{Clave: p.Clave, Cuando: p.Cuando})
		}
	}
	slices.SortFunc(out, func(x, y Aviso) int { return x.Cuando.Compare(y.Cuando) })
	return out
}

// Temporizador es el [Sistema] de en el proceso: lo que usa Linux, donde el sistema no
// programa avisos. Solo avisa mientras la app sigue abierta.
type Temporizador struct {
	// Avisar enseña el aviso al llegar su hora.
	Avisar func(Aviso)

	mu     sync.Mutex
	relojs map[string]*time.Timer
}

// Programar arranca un reloj.
func (t *Temporizador) Programar(etiqueta string, a Aviso) (string, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.relojs == nil {
		t.relojs = map[string]*time.Timer{}
	}
	if r := t.relojs[etiqueta]; r != nil {
		r.Stop()
	}
	t.relojs[etiqueta] = time.AfterFunc(time.Until(a.Cuando), func() {
		t.mu.Lock()
		delete(t.relojs, etiqueta)
		avisar := t.Avisar
		t.mu.Unlock()
		if avisar != nil {
			avisar(a)
		}
	})
	return etiqueta, nil
}

// EnProceso: lo programado no sobrevive a cerrar la app.
func (*Temporizador) EnProceso() bool { return true }

// Cancelar para el reloj.
func (t *Temporizador) Cancelar(id string) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if r := t.relojs[id]; r != nil {
		r.Stop()
		delete(t.relojs, id)
	}
	return nil
}
