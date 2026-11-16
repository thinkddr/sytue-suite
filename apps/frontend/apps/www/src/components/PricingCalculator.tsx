/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * PricingCalculator — responde a «¿cuánto me cuesta con N personas?»: plan ×
 * personas, con el IVA aparte y dicho al lado del total.
 *
 * Las cifras salen de `lib/pricing.ts` (espejo comprobado de commerce en Go) y
 * los textos llegan ya traducidos desde la página: si la isla importara el
 * catálogo, los 26 idiomas viajarían al navegador para pintar seis rótulos.
 * Lo único que hace en el cliente es multiplicar y formatear con `Intl`.
 */
import { useState } from 'preact/hooks';
import type { Locale } from '@sytue/i18n';
import { ChoiceCard, FieldGroup, Input, Switch, describedBy } from '@sytue/ui';
import {
  MAX_SEATS,
  MIN_SEATS,
  PAID_PLANS,
  clampSeats,
  educationApplies,
  formatPrice,
  monthlyTotalCents,
  unitPriceCents,
  type PaidPlan,
} from '../lib/pricing';

/** Rótulos de la calculadora; el título y la entradilla los pone la página. */
export interface PricingCalculatorLabels {
  plan: string;
  seats: string;
  seatsHelp: string;
  education: string;
  perUser: string;
  total: string;
  tax: string;
  /** Nombre y almacenamiento de cada plan de pago. */
  plans: Record<PaidPlan, { title: string; storage: string }>;
}

export interface PricingCalculatorProps {
  locale: Locale;
  labels: PricingCalculatorLabels;
  /** Personas con las que arranca: la página la fija para que el HTML ya traiga un total. */
  initialSeats?: number;
}

export function PricingCalculator({ locale, labels, initialSeats = 10 }: PricingCalculatorProps) {
  const [plan, setPlan] = useState<PaidPlan>('pro');
  // Lo que hay ESCRITO y lo que se CALCULA van aparte: si el campo se reescribiera
  // con el valor acotado a cada tecla, borrar para teclear otro número saltaría a 1.
  const [raw, setRaw] = useState(String(initialSeats));
  const [education, setEducation] = useState(false);

  const seats = clampSeats(raw);
  const conDescuento = education && educationApplies(plan);
  const unit = unitPriceCents(plan, conDescuento);
  const total = monthlyTotalCents(plan, seats, conDescuento);

  return (
    <div class="border-border bg-surface grid gap-6 rounded-lg border p-6 md:grid-cols-[3fr_2fr] md:gap-8">
      <form class="flex min-w-0 flex-col gap-5" onSubmit={(e) => e.preventDefault()}>
        <fieldset class="flex min-w-0 flex-col gap-2">
          <legend class="text-text mb-1.5 text-sm font-semibold">{labels.plan}</legend>
          <div class="grid gap-3 sm:grid-cols-2">
            {PAID_PLANS.map((p) => (
              <ChoiceCard
                key={p}
                name="pricing-plan"
                value={p}
                title={labels.plans[p].title}
                description={`${formatPrice(locale, unitPriceCents(p))} · ${labels.plans[p].storage}`}
                selected={plan === p}
                onSelect={() => setPlan(p)}
                dense
              />
            ))}
          </div>
        </fieldset>

        <FieldGroup label={labels.seats} htmlFor="pricing-seats" help={labels.seatsHelp}>
          <Input
            id="pricing-seats"
            type="number"
            inputMode="numeric"
            min={MIN_SEATS}
            max={MAX_SEATS}
            step={1}
            value={raw}
            onInput={(e) => setRaw(e.currentTarget.value)}
            // Al salir del campo se enseña el número con el que se está calculando.
            onBlur={() => setRaw(String(seats))}
            class="max-w-40 tabular-nums"
            aria-describedby={describedBy('pricing-seats', labels.seatsHelp)}
          />
        </FieldGroup>

        {educationApplies(plan) ? (
          <Switch
            label={labels.education}
            checked={education}
            onChange={(e) => setEducation(e.currentTarget.checked)}
          />
        ) : null}
      </form>

      {/* El resultado se anuncia al cambiar (polite: no interrumpe al teclear). */}
      <div
        class="bg-surface-sunken flex min-w-0 flex-col justify-center rounded-lg p-5"
        aria-live="polite"
        aria-atomic="true"
      >
        <p class="text-text-muted text-sm">{labels.total}</p>
        <p class="sytue-h1 text-text mt-1 break-words tabular-nums">{formatPrice(locale, total)}</p>
        <p class="text-text-muted mt-1 text-sm">{labels.tax}</p>
        {/* Rótulo y cifra en dos piezas y sin «:» escrito: la puntuación entre
            ellos cambia de idioma a idioma (en francés lleva espacio delante). */}
        <p class="text-text-muted mt-4 flex flex-wrap items-baseline justify-between gap-x-3 text-sm">
          <span>{labels.perUser}</span>
          <span class="text-text font-semibold tabular-nums">{formatPrice(locale, unit)}</span>
        </p>
      </div>
    </div>
  );
}

export default PricingCalculator;
