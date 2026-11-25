// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package service

import (
	"context"
	"slices"
	"strings"

	"git.sytue.eu/sytue/one/platform/pkg/peticiones"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
)

// La política de cada organización para el contacto con gente de fuera (mig 0083), como
// la de las peticiones de firma de CertiPDF (ADR 0059): abierta, cerrada o solo con las
// organizaciones de confianza. Se aplica en tres sitios:
//
//   - al abrir una conversación con alguien de fuera o invitarle (personasDeFuera), con
//     la de la organización de quien la abre: se le dice, es su propia regla;
//   - al llevar a alguien a la anfitriona (Anfitrion), con las DOS: la de su sesión
//     tiene que admitir a la anfitriona y la de la anfitriona, a la suya. Si no, la
//     petición se queda en su organización y la conversación no existe para ella;
//   - en las listas de quien viene de fuera (conversaciones e invitaciones), igual.

// OrganizacionesDePersona dice en qué organizaciones activas está alguien y si tiene
// cuenta personal (Sytue ID, /internal/people/orgs). Se inyecta para probar sin él.
type OrganizacionesDePersona func(ctx context.Context, subject string) (orgs []string, personal bool, err error)

// SetOrganizacionesDePersona cambia de dónde salen las organizaciones de una persona.
func (s *Service) SetOrganizacionesDePersona(f OrganizacionesDePersona) { s.orgsDePersona = f }

func (s *Service) organizacionesDe(ctx context.Context, subject string) ([]string, bool, error) {
	if s.orgsDePersona != nil {
		return s.orgsDePersona(ctx, subject)
	}
	return peticiones.OrganizacionesDe(ctx, s.cfg.IDURL, s.cfg.MatrixIdentitySecret, subject)
}

// PoliticaContactoExterno es la política de la organización del contexto.
func (s *Service) PoliticaContactoExterno(ctx context.Context) (domain.PoliticaContactoExterno, error) {
	if s.pstore == nil {
		return domain.PoliticaContactoExterno{Modo: domain.ContactoAbierto, Confianza: []string{}}, nil
	}
	return s.pstore.PoliticaContactoExterno(ctx)
}

// GuardarPoliticaContactoExterno valida y guarda la política de la organización del
// contexto: un modo conocido y, de confianza, hasta MaxConfianzaContacto nombres de
// organización válidos, sin repetir y sin la propia. Sin modo de confianza la lista se
// vacía: no se guarda lo que no aplica.
func (s *Service) GuardarPoliticaContactoExterno(ctx context.Context, p domain.PoliticaContactoExterno, por string) (domain.PoliticaContactoExterno, error) {
	t, err := tenancy.From(ctx)
	if err != nil {
		return p, err
	}
	limpia, err := PoliticaContactoValida(p, t.Slug)
	if err != nil {
		return p, err
	}
	return s.pstore.GuardarPoliticaContactoExterno(ctx, limpia, por)
}

// PoliticaContactoValida recorta y comprueba una política (ver GuardarPoliticaContactoExterno).
// Un nombre que no es de organización devuelve *OrganizacionNoValida.
func PoliticaContactoValida(in domain.PoliticaContactoExterno, propia string) (domain.PoliticaContactoExterno, error) {
	out := domain.PoliticaContactoExterno{Modo: in.Modo, Confianza: []string{}}
	switch in.Modo {
	case domain.ContactoAbierto, domain.ContactoCerrado:
		return out, nil
	case domain.ContactoConfianza:
	default:
		return out, domain.ErrInvalidInput
	}
	if len(in.Confianza) > domain.MaxConfianzaContacto {
		return out, domain.ErrInvalidInput
	}
	for _, o := range in.Confianza {
		o = strings.ToLower(strings.TrimSpace(o))
		t, err := tenancy.New(o)
		if err != nil {
			return out, &OrganizacionNoValida{Org: o}
		}
		if t.Slug != propia && !slices.Contains(out.Confianza, t.Slug) {
			out.Confianza = append(out.Confianza, t.Slug)
		}
	}
	return out, nil
}

// OrganizacionNoValida es un nombre de la lista de confianza que no es de organización.
type OrganizacionNoValida struct{ Org string }

func (e *OrganizacionNoValida) Error() string {
	return "rooms: no es un nombre de organización: " + e.Org
}

// Unwrap la deja pasar como entrada inválida a quien no la distinga.
func (e *OrganizacionNoValida) Unwrap() error { return domain.ErrInvalidInput }

// contactoDeSalida aplica la política de la organización de quien abre la conversación
// a las personas de fuera que añade: cerrada, a nadie; de confianza, a quien esté en
// alguna de las de confianza. Sin poder preguntar a Sytue ID en qué organizaciones
// está, no se abre: fail-closed.
func (s *Service) contactoDeSalida(ctx context.Context, fuera []string) error {
	if s.pstore == nil || len(fuera) == 0 {
		return nil
	}
	p, err := s.pstore.PoliticaContactoExterno(ctx)
	if err != nil {
		return err
	}
	switch p.Modo {
	case domain.ContactoCerrado:
		return domain.ErrContactoExternoCerrado
	case domain.ContactoConfianza:
		for _, sub := range fuera {
			orgs, _, err := s.organizacionesDe(ctx, sub)
			if err != nil || !p.AdmiteAlguna(orgs) {
				return domain.ErrContactoExternoCerrado
			}
		}
	}
	return nil
}

// contactoPermitido: las políticas de las dos organizaciones dejan que alguien de
// `propia` esté en una conversación de `anfitriona`. Un error de lectura cuenta como
// que no: fail-closed.
func (s *Service) contactoPermitido(ctx context.Context, propia, anfitriona string) bool {
	if s.pstore == nil || propia == anfitriona {
		return true
	}
	ok, err := s.pstore.ContactoPermitido(ctx, propia, anfitriona)
	return err == nil && ok
}
