// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//nolint:tagliatelle // Google Calendar y Microsoft Graph usan estas claves camelCase fijas.
package agenda

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"strings"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/cuentas/dav"
)

// La ocupación de los invitados (A-13): «buscar un hueco» del formulario de Calendar
// cuenta también con cuándo están ocupadas las personas a las que se invita, si el
// proveedor de la cuenta lo sabe. CalDAV lo pregunta por el buzón de salida de la
// planificación (un VFREEBUSY, RFC 6638 §5); Google, con `freeBusy`; Microsoft, con
// `getSchedule`. Solo se pide al abrir «buscar un hueco» con invitados, y solo al
// servidor de la cuenta del calendario del evento.

// Tramo es un rato ocupado, en UTC.
type Tramo = dav.Tramo

// MaxOcupacion es cuántas personas se preguntan a la vez.
const MaxOcupacion = dav.MaxOcupacion

// Ocupacion, si el remoto la implementa, sabe cuándo están ocupadas otras personas:
// sus ratos ocupados entre desde y hasta, por dirección (en minúsculas). Quien el
// proveedor no conoce o no deja ver no está en el mapa.
type Ocupacion interface {
	Ocupacion(ctx context.Context, organizador string, correos []string, desde, hasta time.Time) (map[string][]Tramo, error)
}

// Ocupacion pregunta al proveedor de una cuenta de calendario la ocupación de correos
// entre desde y hasta. ErrNoSoportado si el proveedor no la da.
func (r *Registro) Ocupacion(ctx context.Context, cuenta string, correos []string, desde, hasta time.Time) (map[string][]Tramo, error) {
	var rem Remoto
	if sc := r.sincronizador(cuenta, Calendario); sc != nil {
		rem = sc.Remoto
	} else {
		c, ok := r.cuenta(cuenta)
		if !ok {
			return nil, ErrSinCuenta
		}
		var err error
		if rem, err = r.Remoto(c, Calendario); err != nil {
			return nil, err
		}
	}
	o, ok := rem.(Ocupacion)
	if !ok {
		return nil, ErrNoSoportado
	}
	return o.Ocupacion(ctx, r.Direccion(cuenta), limpiarCorreos(correos), desde, hasta)
}

// limpiarCorreos son las direcciones sin repetir, en minúsculas y como mucho MaxOcupacion.
func limpiarCorreos(cs []string) []string {
	var out []string
	for _, c := range cs {
		c = strings.ToLower(strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(c, "mailto:"), "MAILTO:")))
		if c != "" && strings.Contains(c, "@") && !slices.Contains(out, c) && len(out) < MaxOcupacion {
			out = append(out, c)
		}
	}
	return out
}

// ----- CalDAV -----

// planificacion es la del principal, leída una vez.
func (r *RemotoDAV) planificacion(ctx context.Context) (dav.Planificacion, error) {
	r.mu.Lock()
	if r.plan != nil {
		p := *r.plan
		r.mu.Unlock()
		return p, nil
	}
	r.mu.Unlock()
	p, err := r.Cliente.Planificacion(ctx)
	if err != nil {
		return p, errorDAV(err)
	}
	r.mu.Lock()
	r.plan = &p
	r.mu.Unlock()
	return p, nil
}

// Direccion es la primera `mailto:` del `calendar-user-address-set` (solo calendarios).
func (r *RemotoDAV) Direccion(ctx context.Context) (string, error) {
	if r.Clase != dav.Calendario {
		return "", nil
	}
	p, err := r.planificacion(ctx)
	if err != nil || len(p.Direcciones) == 0 {
		return "", err
	}
	return p.Direcciones[0], nil
}

// Ocupacion pregunta por el buzón de salida; sin él, ErrNoSoportado.
func (r *RemotoDAV) Ocupacion(ctx context.Context, organizador string, correos []string, desde, hasta time.Time) (map[string][]Tramo, error) {
	p, err := r.planificacion(ctx)
	if err != nil {
		return nil, err
	}
	if p.Buzon == "" {
		return nil, ErrNoSoportado
	}
	if organizador == "" && len(p.Direcciones) > 0 {
		organizador = p.Direcciones[0]
	}
	m, err := r.Cliente.Ocupacion(ctx, p.Buzon, organizador, correos, desde, hasta)
	if errors.Is(err, dav.ErrSinPlanificacion) {
		return nil, errors.Join(ErrNoSoportado, err)
	}
	return m, errorDAV(err)
}

// ----- Google -----

// Direccion es el id del calendario principal de la persona, que es su correo.
func (g *GoogleCalendar) Direccion(context.Context) (string, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.primario, nil
}

// Ocupacion es `freeBusy` (Calendar API v3): el calendario principal de cada persona,
// que se llama como su correo.
func (g *GoogleCalendar) Ocupacion(ctx context.Context, _ string, correos []string, desde, hasta time.Time) (map[string][]Tramo, error) {
	out := map[string][]Tramo{}
	if len(correos) == 0 {
		return out, nil
	}
	type item struct {
		ID string `json:"id"`
	}
	cuerpo := struct {
		TimeMin string `json:"timeMin"`
		TimeMax string `json:"timeMax"`
		Items   []item `json:"items"`
	}{TimeMin: desde.UTC().Format(time.RFC3339), TimeMax: hasta.UTC().Format(time.RFC3339)}
	for _, c := range correos {
		cuerpo.Items = append(cuerpo.Items, item{ID: c})
	}
	var r struct {
		Calendars map[string]struct {
			Busy []struct {
				Start string `json:"start"`
				End   string `json:"end"`
			} `json:"busy"`
			Errors []struct {
				Reason string `json:"reason"`
			} `json:"errors"`
		} `json:"calendars"`
	}
	if err := g.api.pedir(ctx, http.MethodPost, g.Base+"/freeBusy", cuerpo, nil, &r); err != nil {
		return nil, err
	}
	for id, c := range r.Calendars {
		quien := strings.ToLower(id)
		if len(c.Errors) > 0 || !slices.Contains(correos, quien) {
			continue // notFound, o no comparte su ocupación
		}
		tramos := []Tramo{}
		for _, b := range c.Busy {
			ini, err1 := time.Parse(time.RFC3339Nano, b.Start)
			fin, err2 := time.Parse(time.RFC3339Nano, b.End)
			if err1 == nil && err2 == nil && fin.After(ini) {
				tramos = append(tramos, Tramo{Desde: ini.UTC(), Hasta: fin.UTC()})
			}
		}
		out[quien] = tramos
	}
	return out, nil
}

// ----- Microsoft Graph -----

// Direccion es el correo de la cuenta de Microsoft (/me).
func (g *GraphCalendario) Direccion(ctx context.Context) (string, error) {
	return g.yo(ctx), nil
}

// Ocupacion es `getSchedule` del calendario de la persona, con las horas en UTC. Lo que
// Graph da como libre no cuenta; «quizá» (tentative) va como provisional.
func (g *GraphCalendario) Ocupacion(ctx context.Context, _ string, correos []string, desde, hasta time.Time) (map[string][]Tramo, error) {
	out := map[string][]Tramo{}
	if len(correos) == 0 {
		return out, nil
	}
	cuerpo := map[string]any{
		"schedules":                correos,
		"startTime":                horaGraph{DateTime: desde.UTC().Format("2006-01-02T15:04:05"), TimeZone: "UTC"},
		"endTime":                  horaGraph{DateTime: hasta.UTC().Format("2006-01-02T15:04:05"), TimeZone: "UTC"},
		"availabilityViewInterval": 30,
	}
	var r struct {
		Value []struct {
			ScheduleID string `json:"scheduleId"`
			Items      []struct {
				Status string    `json:"status"`
				Start  horaGraph `json:"start"`
				End    horaGraph `json:"end"`
			} `json:"scheduleItems"`
			Error *struct {
				Message string `json:"message"`
			} `json:"error"`
		} `json:"value"`
	}
	if err := g.api.pedir(ctx, http.MethodPost, g.Base+"/me/calendar/getSchedule", cuerpo, cabUTC, &r); err != nil {
		return nil, err
	}
	for _, v := range r.Value {
		quien := strings.ToLower(v.ScheduleID)
		if v.Error != nil || !slices.Contains(correos, quien) {
			continue
		}
		tramos := []Tramo{}
		for _, it := range v.Items {
			if it.Status == "free" {
				continue
			}
			ini, fin := it.Start.instante(), it.End.instante()
			if !ini.IsZero() && fin.After(ini) {
				tramos = append(tramos, Tramo{Desde: ini.UTC(), Hasta: fin.UTC(), Provisional: it.Status == "tentative"})
			}
		}
		out[quien] = tramos
	}
	return out, nil
}
