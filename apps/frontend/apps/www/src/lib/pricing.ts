/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * pricing — las CIFRAS de los planes de Sytue One para el sitio público: precio
 * por usuario y mes, descuento educativo y revalorización comprometida.
 *
 * La fuente de verdad es `platform/pkg/commerce/subscription.go`: es lo que el
 * checkout cobra y lo que la pasarela valida. Esto es su ESPEJO, y existe solo
 * porque un `.astro` no puede leer Go. `pricing.test.ts` lee aquel fichero y
 * falla si una cifra de aquí no coincide, así que cambiar un precio sin tocar
 * los dos es un rojo, no una web que promete lo que no se cobra.
 *
 * Antes cada idioma escribía «5 €», «€5» o «€ 5» a mano en su catálogo: 52
 * cadenas que había que retocar para subir un precio. Ahora la cifra sale de
 * aquí y la forma (símbolo delante o detrás, coma o punto) la pone `Intl`.
 */
import { formatCurrency, type Locale } from '@sytue/i18n';

/** Divisa contractual (espejo de `commerce.SytueOneCurrency`). */
export const CURRENCY = 'EUR';

/** Planes de pago, en el orden en que se presentan. El gratuito no se calcula. */
export const PAID_PLANS = ['pro', 'enterprise'] as const;
export type PaidPlan = (typeof PAID_PLANS)[number];

/** Precio mensual por usuario, en céntimos (`ProSeatPriceCents`, `EnterpriseSeatPriceCents`). */
export const SEAT_PRICE_CENTS: Readonly<Record<PaidPlan, number>> = {
  pro: 500,
  enterprise: 2500,
};

/** Descuento de las instituciones educativas, solo en Pro (`EducationDiscountPercent`). */
export const EDUCATION_DISCOUNT_PERCENT = 50;

/** El precio por usuario sube `PRICE_RISE_CENTS` cada `PRICE_RISE_YEARS` años, y nada más. */
export const PRICE_RISE_CENTS = 50;
export const PRICE_RISE_YEARS = 4;

/**
 * Límites del campo «personas» de la calculadora. El tope no es comercial: es
 * para que un número absurdo pegado en el campo no pinte un total ilegible.
 */
export const MIN_SEATS = 1;
export const MAX_SEATS = 100_000;

/** El descuento educativo solo existe en Pro: en Enterprise no se ofrece. */
export function educationApplies(plan: PaidPlan): boolean {
  return plan === 'pro';
}

/**
 * Precio unitario que se cobra, con la MISMA aritmética entera que
 * `Plan.UnitPriceCents` en Go: si un día el descuento no diera céntimos
 * exactos, la web redondearía igual que la factura.
 */
export function unitPriceCents(plan: PaidPlan, education = false): number {
  const precio = SEAT_PRICE_CENTS[plan];
  if (education && educationApplies(plan)) {
    return Math.trunc((precio * (100 - EDUCATION_DISCOUNT_PERCENT)) / 100);
  }
  return precio;
}

/**
 * Lo que escribe la persona, convertido en un número de usuarios válido. Un campo
 * vacío o a medio escribir cuenta como una persona —el total nunca sale «NaN €»—
 * y los decimales se truncan: no se contratan medios usuarios.
 */
export function clampSeats(raw: number | string): number {
  const n = typeof raw === 'number' ? raw : Number.parseInt(raw, 10);
  if (!Number.isFinite(n)) return MIN_SEATS;
  return Math.min(MAX_SEATS, Math.max(MIN_SEATS, Math.trunc(n)));
}

/** Total mensual sin IVA, en céntimos: usuarios × precio unitario. */
export function monthlyTotalCents(plan: PaidPlan, seats: number, education = false): number {
  return clampSeats(seats) * unitPriceCents(plan, education);
}

/**
 * Importe en euros con la forma de cada idioma: «€5» en inglés, «5 €» en
 * alemán, «€ 5» en neerlandés. Sin decimales cuando son cero («€25», no
 * «€25.00») y con dos cuando no («€0.50»). `narrowSymbol` porque algunos
 * idiomas (húngaro, islandés, rumano) escriben «EUR» con el símbolo normal, y
 * en un precio de escaparate se espera «€».
 */
export function formatPrice(locale: Locale, cents: number): string {
  const entero = cents % 100 === 0;
  return formatCurrency(locale, cents / 100, CURRENCY, {
    currencyDisplay: 'narrowSymbol',
    minimumFractionDigits: entero ? 0 : 2,
    maximumFractionDigits: entero ? 0 : 2,
  });
}
