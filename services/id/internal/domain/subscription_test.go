// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"testing"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/commerce"
)

// Una cuenta personal nunca se queda fuera, y sin pagar vuelve al plan gratuito:
// lo que cambia es la cuota, no la puerta.
func TestPersonalNuncaSeQuedaFuera(t *testing.T) {
	ahora := time.Now()
	gratis := OrgSubscription{Personal: true, Plan: commerce.PlanPersonalFree, Status: SubscriptionPending, Seats: 1}
	if !gratis.Entitled(ahora, 1, 0) || gratis.EffectivePlan(ahora) != commerce.PlanPersonalFree {
		t.Fatal("la gratuita entra sin pagar")
	}
	if gratis.StorageBytes(ahora) != commerce.PersonalFreeStorageBytes || gratis.MailboxBytes(ahora) != commerce.PersonalFreeMailboxBytes {
		t.Fatalf("cuotas de la gratuita: %d %d", gratis.StorageBytes(ahora), gratis.MailboxBytes(ahora))
	}
	pro := OrgSubscription{
		Personal: true, Plan: commerce.PlanEnterprise, Status: SubscriptionActive, Seats: 1,
		UnitPriceCents: commerce.EnterpriseSeatPriceCents, Currency: SuiteCurrency, PaidUntil: ahora.Add(24 * time.Hour),
	}
	if !pro.Paid(ahora) || pro.EffectivePlan(ahora) != commerce.PlanEnterprise || pro.StorageBytes(ahora) != commerce.EnterpriseStorageBytes {
		t.Fatalf("pro pagado: %+v", pro)
	}
	pro.PaidUntil = ahora.Add(-time.Hour)
	if pro.Paid(ahora) || pro.EffectivePlan(ahora) != commerce.PlanPersonalFree || !pro.Entitled(ahora, 1, 0) {
		t.Fatalf("pro caducado vuelve a gratuito y sigue entrando: %+v", pro)
	}
	// Una institución educativa paga el Pro a mitad, y ese es el precio que se comprueba.
	edu := OrgSubscription{
		Education: true, Plan: commerce.PlanPro, Status: SubscriptionActive, Seats: 30,
		UnitPriceCents: commerce.PlanPro.UnitPriceCents(true), Currency: SuiteCurrency, PaidUntil: ahora.Add(time.Hour),
	}
	if !edu.Paid(ahora) || edu.UnitPriceCents != 250 {
		t.Fatalf("educativa: %+v", edu)
	}
	edu.UnitPriceCents = commerce.ProSeatPriceCents
	if edu.Paid(ahora) {
		t.Fatal("una educativa que paga el precio completo no coincide con su contrato")
	}
	// Un precio manipulado no es un pago.
	pro.PaidUntil = ahora.Add(time.Hour)
	pro.UnitPriceCents = 1
	if pro.Paid(ahora) {
		t.Fatal("el precio se comprueba contra el del plan")
	}
}

// Una organización paga por usuario y su pool es la suma de los asientos.
func TestOrganizacionPagaPorAsientos(t *testing.T) {
	ahora := time.Now()
	org := OrgSubscription{
		Plan: commerce.PlanPro, Status: SubscriptionActive, Seats: 20,
		UnitPriceCents: commerce.ProSeatPriceCents, Currency: SuiteCurrency, PaidUntil: ahora.Add(time.Hour),
	}
	if !org.Entitled(ahora, 20, 0) || org.Entitled(ahora, 21, 0) {
		t.Fatal("los asientos tienen que cubrir a la plantilla")
	}
	if org.StorageBytes(ahora) != 20*commerce.ProStorageBytes || org.MailboxBytes(ahora) != commerce.ProMailboxBytes {
		t.Fatalf("pool: %d buzón: %d", org.StorageBytes(ahora), org.MailboxBytes(ahora))
	}
	org.PaidUntil = ahora.Add(-time.Hour)
	if org.Entitled(ahora, 20, 0) {
		t.Fatal("una organización sin pagar se queda fuera; no baja a gratuito")
	}
	if org.EffectivePlan(ahora) != commerce.PlanPro {
		t.Fatal("el plan contratado no cambia por no pagar")
	}
	// Las concesiones dan acceso sin pago.
	if !org.Entitled(ahora, 20, 20) {
		t.Fatal("20 plazas concedidas cubren a 20 personas")
	}
	// Sin plan (suscripciones anteriores a los planes) se lee como estándar.
	vieja := OrgSubscription{
		Status: SubscriptionActive, Seats: 3, UnitPriceCents: commerce.ProSeatPriceCents,
		Currency: SuiteCurrency, PaidUntil: ahora.Add(time.Hour),
	}
	if vieja.EffectivePlan(ahora) != commerce.PlanPro || !vieja.Entitled(ahora, 3, 0) {
		t.Fatalf("sin plan = estándar: %+v", vieja)
	}
}

func TestPersonalSlugEsEstableYValido(t *testing.T) {
	a, b := PersonalSlug("Carla@Ejemplo.com"), PersonalSlug("carla@ejemplo.com")
	if a != b || len(a) != len(PersonalSlugPrefix)+16 || !SlugAllowedForOrg("acme") || SlugAllowedForOrg(a) {
		t.Fatalf("slug personal: %q %q", a, b)
	}
}
