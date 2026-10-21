// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
)

// ErrAliasExists indica que la dirección de alias ya está en uso (por otra cuenta
// primaria o por otro alias, en cualquier tenant: el espacio de direcciones es
// global para la entrega entrante). No se permite secuestrar una dirección ajena.
var ErrAliasExists = errors.New("mail: la dirección de alias ya está en uso")

// ErrAliasReservado: la dirección suplantaría a la plataforma (admin@, soporte@,
// sytue-*@ en el dominio raíz). Ver platform/pkg/mailaddr.
var ErrAliasReservado = errors.New("mail: la dirección de alias está reservada")

// ErrAliasLimite: una cuenta común (personal, o miembro sin rol de administración)
// solo puede tener un alias. Frena la fábrica de direcciones desechables con la
// que se abusa de un dominio compartido.
var ErrAliasLimite = errors.New("mail: límite de alias alcanzado")

// ErrBuzonesLimite: la organización ya tiene todos los buzones compartidos que puede.
var ErrBuzonesLimite = errors.New("mail: límite de buzones compartidos alcanzado")

// Alias es una dirección adicional que entrega a la misma cuenta que su dueño. La
// dirección se valida contra los dominios que sirve el sistema y es única en todo
// el espacio de direcciones (una dirección entrega a una sola cuenta).
type Alias struct {
	ID        uuid.UUID `json:"id"`
	Tenant    string    `json:"-"`
	AccountID uuid.UUID `json:"-"`
	Address   string    `json:"address"`
	CreatedAt time.Time `json:"created_at"`
}

// Vacation es la configuración de autorespuesta / fuera-de-oficina de una cuenta.
// Rango [StartsAt, EndsAt] opcional (nil = sin límite por ese extremo).
type Vacation struct {
	Enabled   bool       `json:"enabled"`
	Subject   string     `json:"subject"`
	Body      string     `json:"body"`
	StartsAt  *time.Time `json:"starts_at,omitempty"`
	EndsAt    *time.Time `json:"ends_at,omitempty"`
	UpdatedAt time.Time  `json:"updated_at,omitempty"`
}

// ActiveAt indica si la autorespuesta debe dispararse en el instante t: activada y
// dentro del rango de fechas (los extremos nil no acotan). Base del gate de entrega.
func (v Vacation) ActiveAt(t time.Time) bool {
	if !v.Enabled {
		return false
	}
	if v.StartsAt != nil && t.Before(*v.StartsAt) {
		return false
	}
	if v.EndsAt != nil && t.After(*v.EndsAt) {
		return false
	}
	return true
}

// AliasDomainAllowed valida que la dirección de alias está en un dominio que el
// sistema sirve para esta cuenta: el dominio de su propia dirección primaria, uno
// de los dominios propios de su organización, el subdominio de esta o el dominio
// principal configurado. Impide crear un alias en un dominio ajeno (que el MX no
// serviría) o secuestrar el espacio de otro dominio. `primaryDomain` es el
// dominio global del servicio.
//
// LOS DOMINIOS DE LA ORGANIZACIÓN llegan en el token (claim `mail_domains`) y no
// se pueden deducir aquí: de una dirección se sabe su dominio, no los OTROS que
// esa organización tiene conectados. Sin ellos, una empresa con dos dominios
// —dos marcas, o el .es y el .com— no podía tener una dirección en el segundo:
// Sytue Mail la rechazaba por ajena. Sytue ID solo pone en ese claim los
// VERIFICADOS, que es donde se decide la titularidad.
//
// EL SUBDOMINIO DE LA ORGANIZACIÓN no es un tercer caso cualquiera: Sytue ID le da
// a cada persona una dirección en `<slug-de-su-organización>.<dominio de la suite>`
// SIEMPRE, también cuando su dirección principal vive en el dominio propio del
// cliente, porque es la que la suite usa para su cableado interno y la sirve
// nuestro MX. Sin reconocerla aquí, esa dirección se rechazaba con 400 y el
// reconciliador de buzones la reintentaba cada treinta segundos para siempre.
//
// Va acotado al tenant del llamante —no a cualquier subdominio del servicio—
// porque `otra-organización.sytue.eu` sí sería secuestrar el espacio de otro.
//
// EL DOMINIO RAÍZ DESNUDO (`sytue.eu`) es el caso delicado: ahí viven las
// direcciones institucionales —`security@`, `abuse@`, `postmaster@`— y quien las
// controla habla en nombre de toda la suite, no de su organización. Por eso solo
// lo abre `rootTenant`. Sin esa condición, cualquier organización recién dada de
// alta podía reclamar `security@sytue.eu` por orden de llegada.
func AliasDomainAllowed(aliasAddr string, acc Account, primaryDomain, rootTenant string, orgDomains []string) bool {
	a, err := ParseAddress(aliasAddr)
	if err != nil {
		return false
	}
	served := map[string]struct{}{}
	if p, err := ParseAddress(acc.Address); err == nil {
		served[p.Domain] = struct{}{}
	}
	for _, d := range orgDomains {
		if d = strings.ToLower(strings.TrimSpace(d)); d != "" {
			served[d] = struct{}{}
		}
	}
	if d := strings.ToLower(strings.TrimSpace(primaryDomain)); d != "" {
		tenant := strings.ToLower(strings.TrimSpace(acc.Tenant))
		// El raíz solo para el tenant institucional. `rootTenant` vacío no abre
		// la mano a todos: deja el dominio desnudo sin dueño, que es el lado
		// seguro de equivocarse.
		if root := strings.ToLower(strings.TrimSpace(rootTenant)); root != "" && tenant == root {
			served[d] = struct{}{}
		}
		if tenant != "" {
			served[tenant+"."+d] = struct{}{}
		}
	}
	_, ok := served[a.Domain]
	return ok
}
