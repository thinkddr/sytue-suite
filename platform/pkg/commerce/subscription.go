// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package commerce contiene contratos comerciales compartidos entre servicios.
// No habla con proveedores ni decide derechos de acceso: esa responsabilidad
// sigue siendo de Sytue ID.
//
// Los precios y las cuotas son CONSTANTES a propósito: ni el navegador ni una
// variable de entorno pueden rebajarlos por error. Cambiar un precio es cambiar
// este fichero, con su revisión y su historia.
package commerce

// SytueOneCurrency es la divisa contractual de toda suscripción.
const SytueOneCurrency = "EUR"

// Plan identifica el contrato comercial de una organización. Es texto y no un
// entero para que en la base de datos y en el token se lea sin descifrar.
//
// Son tres, y los mismos para una organización (por usuario) y para una cuenta
// personal (un solo usuario): lo que cambia entre planes es el almacenamiento
// por usuario, no lo que se puede usar. Decidido el 2026-09-13 (ADR 0038).
type Plan string

const (
	// PlanPersonalFree es la cuenta personal gratuita: una persona, sin
	// organización propia —su organización es su correo—, con
	// PersonalFreeStorageBytes. No existe para organizaciones.
	PlanPersonalFree Plan = "personal_free"
	// PlanPro cuesta ProSeatPriceCents por usuario y mes, con ProStorageBytes
	// por usuario. Es el plan de toda organización nueva. Las instituciones
	// educativas lo tienen con EducationDiscountPercent de descuento.
	PlanPro Plan = "pro"
	// PlanEnterprise cuesta EnterpriseSeatPriceCents por usuario y mes, con
	// EnterpriseStorageBytes por usuario, e incluye Soporte Premium.
	PlanEnterprise Plan = "enterprise"
)

const (
	// ProSeatPriceCents es el precio mensual por usuario de PlanPro.
	ProSeatPriceCents = 500
	// EnterpriseSeatPriceCents es el precio mensual por usuario de PlanEnterprise.
	EnterpriseSeatPriceCents = 2500
	// EducationDiscountPercent es el descuento de las instituciones educativas
	// en PlanPro. Lo concede la plataforma (organizations.education) y lo aplica
	// el checkout: el precio que se cobra es UnitPriceCents(plan, education).
	EducationDiscountPercent = 50
	// PriceRiseCents es la revalorización comprometida de por vida: el precio por
	// usuario sube esta cantidad cada PriceRiseYears años, y nada más.
	PriceRiseCents = 50
	PriceRiseYears = 4

	// Cuotas de ALMACENAMIENTO por usuario. Es la cuota de Drive, que es donde
	// viven los ficheros, las fotos (sus originales) y los documentos: los tres
	// productos escriben en el mismo sitio, así que una sola cifra los cubre. En
	// una organización se multiplica por los asientos (pool de la organización).
	PersonalFreeStorageBytes int64 = 5 << 30   // 5 GiB
	ProStorageBytes          int64 = 200 << 30 // 200 GiB
	EnterpriseStorageBytes   int64 = 1 << 40   // 1 TiB

	// Cuotas del BUZÓN de correo por usuario. Van aparte del almacenamiento
	// porque Sytue Mail lleva su propio contador por cuenta y no escribe en
	// Drive; sumarlas en un único número pediría un libro mayor común que hoy no
	// existe. Se declaran aquí, juntas, para que el reparto se vea.
	PersonalFreeMailboxBytes int64 = 1 << 30  // 1 GiB
	ProMailboxBytes          int64 = 10 << 30 // 10 GiB
	EnterpriseMailboxBytes   int64 = 25 << 30 // 25 GiB
)

// PersonalUpgradeAvailable abre o cierra la contratación de un plan de pago
// desde una cuenta personal. Está CERRADA por decisión del 2026-09-13: el
// registro gratuito sale antes que el cobro. Mientras sea false, Sytue ID
// rechaza el checkout de una cuenta personal (503) y la app de Cuenta no ofrece
// el paso. Abrirla es cambiar esta línea, con su revisión.
const PersonalUpgradeAvailable = false

// ValidPlan indica si el plan es uno de los contratos conocidos.
func ValidPlan(p Plan) bool {
	switch p {
	case PlanPersonalFree, PlanPro, PlanEnterprise:
		return true
	}
	return false
}

// UnitPriceCents es el precio por usuario que se cobra: el del plan, con el
// descuento educativo si la organización lo tiene y el plan lo admite.
func (p Plan) UnitPriceCents(education bool) int {
	precio := p.PriceCents()
	if education && p == PlanPro {
		return precio * (100 - EducationDiscountPercent) / 100
	}
	return precio
}

// ValidSeatPriceCents indica si un precio unitario es el de algún plan de pago,
// con o sin descuento educativo. Es lo que comprueba la pasarela de pago, que
// conoce importes y no planes.
func ValidSeatPriceCents(cents int) bool {
	for _, p := range []Plan{PlanPro, PlanEnterprise} {
		if cents == p.UnitPriceCents(false) || cents == p.UnitPriceCents(true) {
			return true
		}
	}
	return false
}

// Free indica si el plan no cuesta nada.
func (p Plan) Free() bool { return p == PlanPersonalFree }

// PriceCents devuelve el precio mensual por usuario del plan. El gratuito
// cuesta cero y un plan desconocido también: nunca se cobra lo que no se sabe
// qué es.
func (p Plan) PriceCents() int {
	switch p {
	case PlanPro:
		return ProSeatPriceCents
	case PlanEnterprise:
		return EnterpriseSeatPriceCents
	default:
		return 0
	}
}

// StorageBytes devuelve la cuota de almacenamiento (Drive) POR USUARIO del
// plan, y 0 para un plan desconocido (la cuota sale entonces de la
// configuración del servicio).
func (p Plan) StorageBytes() int64 {
	switch p {
	case PlanPersonalFree:
		return PersonalFreeStorageBytes
	case PlanPro:
		return ProStorageBytes
	case PlanEnterprise:
		return EnterpriseStorageBytes
	default:
		return 0
	}
}

// MailboxBytes es la cuota del buzón POR USUARIO del plan, y 0 para un plan
// desconocido.
func (p Plan) MailboxBytes() int64 {
	switch p {
	case PlanPersonalFree:
		return PersonalFreeMailboxBytes
	case PlanPro:
		return ProMailboxBytes
	case PlanEnterprise:
		return EnterpriseMailboxBytes
	default:
		return 0
	}
}
