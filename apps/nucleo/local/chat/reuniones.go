// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package chat

import (
	"context"
	"errors"
	"slices"
	"strings"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// Reuniones y presencia de las cuentas que las tienen (Teams y Google Meet), repartidas
// por el Agregador.

// Reuniones crea enlaces de reunión de su proveedor. Sytue Office no tiene llamadas
// propias: la reunión se abre con la app oficial (D-21) y aquí se crea su enlace y se
// publica en la conversación.
type Reuniones interface {
	// ProveedorReunion es el nombre del servicio («Microsoft Teams», «Google Meet»).
	ProveedorReunion() string
	// CrearReunion crea la reunión (asunto, si el proveedor lo admite) y da su enlace.
	CrearReunion(ctx context.Context, asunto string) (string, error)
}

// Presentes da la presencia de la gente de la cuenta.
type Presentes interface {
	Presencias(ctx context.Context, personas []string) ([]api.Presencia, error)
}

// cuentaReunion es la cuenta que crea la reunión de canal: la suya si sabe; si no, la
// primera que sepa (un enlace de Meet se puede mandar a un canal de Slack).
func (a *Agregador) cuentaReunion(canal string) (Reuniones, bool) {
	if cuenta, _, ok := partirCanal(canal); ok {
		if p, err := a.prov(cuenta); err == nil {
			if r, ok := p.(Reuniones); ok {
				return r, true
			}
		}
	}
	for _, cuenta := range a.Cuentas() {
		if p, err := a.prov(cuenta); err == nil {
			if r, ok := p.(Reuniones); ok {
				return r, true
			}
		}
	}
	return nil, false
}

// ProveedorReunion es el servicio con el que se crearía una reunión en canal, o "" si no
// hay ninguna cuenta que las cree.
func (a *Agregador) ProveedorReunion(canal string) string {
	if r, ok := a.cuentaReunion(canal); ok {
		return r.ProveedorReunion()
	}
	return ""
}

// CrearReunion crea una reunión con la cuenta de ProveedorReunion y publica su enlace en
// el canal (con la cuenta del canal). titulo es el asunto, donde lo hay.
func (a *Agregador) CrearReunion(ctx context.Context, canal, titulo string) (api.RoomsMessage, error) {
	r, ok := a.cuentaReunion(canal)
	if !ok {
		return api.RoomsMessage{}, ErrNoDisponible
	}
	enlace, err := r.CrearReunion(ctx, titulo)
	if err != nil {
		return api.RoomsMessage{}, err
	}
	return a.PostWith(ctx, canal, enlace, api.RoomsSend{})
}

// Presencias es la presencia de esas personas en las cuentas que la dan. Cada una se
// pregunta a su cuenta si viene prefijada; si no, a todas las que dan presencia (cada
// proveedor descarta lo que no es suyo). Lo que no sabe nadie no sale.
func (a *Agregador) Presencias(ctx context.Context, personas ...string) ([]api.Presencia, error) {
	porCuenta := map[string][]string{}
	var sueltas []string
	cuentas := a.Cuentas()
	for _, p := range personas {
		if c, x, ok := strings.Cut(p, sep); ok && slices.Contains(cuentas, c) {
			porCuenta[c] = append(porCuenta[c], x)
			continue
		}
		sueltas = append(sueltas, p)
	}
	var out []api.Presencia
	var errs []error
	for _, cuenta := range cuentas {
		p, err := a.prov(cuenta)
		if err != nil {
			continue
		}
		pr, ok := p.(Presentes)
		if !ok {
			continue
		}
		propias := porCuenta[cuenta]
		ps, err := pr.Presencias(ctx, append(slices.Clone(propias), sueltas...))
		if err != nil {
			errs = append(errs, err)
		}
		for _, x := range ps {
			if slices.Contains(propias, x.Subject) {
				x.Subject = cuenta + sep + x.Subject
			}
			out = append(out, x)
		}
	}
	if len(out) == 0 && len(errs) > 0 {
		return nil, errors.Join(errs...)
	}
	return out, nil
}
