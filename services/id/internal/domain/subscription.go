// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/commerce"
)

// SuiteSeatPriceCents es el precio contractual de Sytue One: 6 EUR por usuario
// y mes. Es una constante deliberada: ni el navegador ni una variable de entorno
// pueden rebajarlo por error.
const (
	SuiteSeatPriceCents = commerce.ProSeatPriceCents
	SuiteCurrency       = commerce.SytueOneCurrency
)

type SubscriptionStatus string

const (
	SubscriptionPending  SubscriptionStatus = "pending"
	SubscriptionActive   SubscriptionStatus = "active"
	SubscriptionPastDue  SubscriptionStatus = "past_due"
	SubscriptionCanceled SubscriptionStatus = "canceled"
)

// OrgSubscription es el derecho de acceso central de una organización. Los IDs
// del proveedor son referencias opacas; el estado solo cambia tras consultar de
// nuevo la API autenticada de Mollie.
type OrgSubscription struct {
	OrgID                  string
	ProviderCustomerID     string
	ProviderSubscriptionID string
	Status                 SubscriptionStatus
	// Plan es el contrato (commerce.Plan): estándar o pro, por usuario, y en una
	// cuenta personal también el gratuito. Vacío se lee como estándar, que es lo
	// que eran todas las suscripciones antes de existir los planes.
	Plan commerce.Plan
	// Personal indica que la organización es una cuenta personal (organizations.kind):
	// nunca pierde el derecho de uso y, sin pagar, vuelve al plan gratuito.
	Personal bool
	// Education indica una institución educativa (organizations.education): paga
	// el Pro con descuento, y el precio pagado se comprueba contra ese.
	Education      bool
	Seats          int
	UnitPriceCents int
	Currency       string
	PaidUntil      time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// Entitled decide si una organización puede usar la suite.
//
// `grantedSeats` son las plazas CONCEDIDAS vigentes (mig 0031): gestos
// comerciales, pruebas, migraciones y las plazas de los empleados de Sytue. La
// migración ya definía que «el aforo efectivo es `seats + Σ(concesiones
// vigentes)`», pero esta función solo miraba `Seats` y exigía suscripción
// pagada, así que las concesiones no daban acceso a nada: existía la tabla,
// existía la API de plataforma para conceder, y el login seguía dejando la
// sesión SIN organización. Es lo que tenía a la propia Sytue fuera de su suite.
//
// Dos caminos al derecho de uso, y el orden importa:
//
//  1. Las concesiones vigentes cubren a toda la plantilla activa ⇒ derecho, sin
//     mirar el pago. Es el caso de un empleado y el de una prueba: se concedió a
//     propósito, con motivo y con caducidad, y exigirle además una suscripción
//     pagada haría la concesión inútil.
//  2. Si no llegan, hace falta la suscripción pagada y vigente, y el aforo
//     COMBINADO tiene que cubrir a la plantilla. Así una organización que compra
//     20 plazas y recibe 5 de regalo puede tener 25 personas, que es justo para
//     lo que se pensó sumarlas.
//
// El precio y la moneda se siguen comprobando en el camino de pago: son la
// defensa contra una suscripción manipulada, y una concesión no las necesita
// porque no factura nada.
func (s OrgSubscription) Entitled(now time.Time, activeMembers, grantedSeats int) bool {
	// Una cuenta personal NUNCA se queda fuera: la gratuita no paga, y la de pago
	// que deja de pagar vuelve a ser gratuita —lo que cambia es la cuota, que sale
	// de Paid(), no la puerta—. Cerrarle la sesión a alguien por un recibo
	// devuelto sería cerrarle su propio correo y sus propios ficheros.
	if s.Personal {
		return true
	}
	if activeMembers <= 0 {
		activeMembers = 1 // una organización sin miembros activos no deja de necesitar aforo
	}
	if grantedSeats < 0 {
		grantedSeats = 0
	}
	if grantedSeats >= activeMembers {
		return true
	}
	if s.Seats+grantedSeats < activeMembers || s.Seats <= 0 {
		return false
	}
	return s.Paid(now)
}

// EffectivePlan es el plan que rige AHORA: el contratado si está pagado, y el
// gratuito si es una cuenta personal y el pago no está al día. Para una
// organización devuelve siempre el contratado, porque ahí la falta de pago
// cierra la puerta (Entitled) en vez de rebajar la cuota.
func (s OrgSubscription) EffectivePlan(now time.Time) commerce.Plan {
	switch {
	case s.Personal && !s.Paid(now):
		return commerce.PlanPersonalFree
	default:
		return s.contratado()
	}
}

// Paid indica si el contrato está cobrado y vigente. El precio se comprueba
// contra el del plan: es la defensa contra una suscripción manipulada. Un plan
// gratuito nunca está «pagado».
func (s OrgSubscription) Paid(now time.Time) bool {
	precio := s.contratado().UnitPriceCents(s.Education)
	return s.Status == SubscriptionActive && s.Currency == SuiteCurrency && precio > 0 &&
		s.UnitPriceCents == precio && s.PaidUntil.After(now)
}

// contratado es el plan contratado, leyendo el vacío —las suscripciones
// anteriores a los planes— como estándar.
func (s OrgSubscription) contratado() commerce.Plan {
	if s.Plan == "" {
		return commerce.PlanPro
	}
	return s.Plan
}

// StorageBytes es la cuota de almacenamiento que concede el plan vigente: por
// usuario en una cuenta personal y, en una organización, el pool de todos sus
// asientos. 0 si el plan no dice nada (la fija entonces el servicio).
func (s OrgSubscription) StorageBytes(now time.Time) int64 {
	return s.EffectivePlan(now).StorageBytes() * int64(s.asientos())
}

// MailboxBytes es la cuota del buzón POR USUARIO del plan vigente.
func (s OrgSubscription) MailboxBytes(now time.Time) int64 {
	return s.EffectivePlan(now).MailboxBytes()
}

// asientos es por cuántos usuarios se multiplica el pool: uno en una cuenta
// personal y, en una organización, los contratados (nunca menos de uno).
func (s OrgSubscription) asientos() int {
	if s.Personal || s.Seats < 1 {
		return 1
	}
	return s.Seats
}
