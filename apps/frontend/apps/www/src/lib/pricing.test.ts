// Checks de las cifras de precios del sitio público. Ejecuta: `bun test`.
//
// El motivo de este fichero: el precio vivía en DOS sitios que nadie comparaba.
// Go (`platform/pkg/commerce/subscription.go`) es lo que se cobra; la web lo
// escribía a mano en 26 catálogos. Subir el precio en Go dejaba la web
// prometiendo el viejo. Ahora la web lee `pricing.ts`, y este test lee el Go.
import { expect, test } from 'bun:test';
import { readFileSync, readdirSync } from 'node:fs';
import { join } from 'node:path';
import { LOCALES, type Locale } from '@sytue/i18n';
import {
  CURRENCY,
  EDUCATION_DISCOUNT_PERCENT,
  MAX_SEATS,
  PRICE_RISE_CENTS,
  PRICE_RISE_YEARS,
  SEAT_PRICE_CENTS,
  clampSeats,
  formatPrice,
  monthlyTotalCents,
  unitPriceCents,
} from './pricing';

const GO = readFileSync(
  join(import.meta.dir, '../../../../../../platform/pkg/commerce/subscription.go'),
  'utf8',
);

/** Valor entero de una constante de Go (`Nombre = 500` o `Nombre int = 500`). */
function constanteGo(nombre: string): number {
  const m = GO.match(new RegExp(`^\\s*${nombre}(?:\\s+\\w+)?\\s*=\\s*(\\d+)\\s*$`, 'm'));
  if (!m) throw new Error(`subscription.go ya no declara ${nombre}`);
  return Number(m[1]);
}

test('las cifras son las mismas que cobra commerce (subscription.go)', () => {
  expect(SEAT_PRICE_CENTS.pro).toBe(constanteGo('ProSeatPriceCents'));
  expect(SEAT_PRICE_CENTS.enterprise).toBe(constanteGo('EnterpriseSeatPriceCents'));
  expect(EDUCATION_DISCOUNT_PERCENT).toBe(constanteGo('EducationDiscountPercent'));
  expect(PRICE_RISE_CENTS).toBe(constanteGo('PriceRiseCents'));
  expect(PRICE_RISE_YEARS).toBe(constanteGo('PriceRiseYears'));
  expect(GO).toContain(`SytueOneCurrency = "${CURRENCY}"`);
});

test('el descuento educativo es solo de Pro, como Plan.UnitPriceCents', () => {
  expect(unitPriceCents('pro')).toBe(500);
  expect(unitPriceCents('pro', true)).toBe(250);
  expect(unitPriceCents('enterprise', true)).toBe(unitPriceCents('enterprise'));
});

test('el total es personas × precio unitario, sin IVA', () => {
  expect(monthlyTotalCents('pro', 12)).toBe(6000);
  expect(monthlyTotalCents('enterprise', 3)).toBe(7500);
  expect(monthlyTotalCents('pro', 10, true)).toBe(2500);
});

test('un campo vacío, negativo o con decimales nunca da un total absurdo', () => {
  expect(clampSeats('')).toBe(1);
  expect(clampSeats('abc')).toBe(1);
  expect(clampSeats(0)).toBe(1);
  expect(clampSeats(-5)).toBe(1);
  expect(clampSeats(7.9)).toBe(7);
  expect(clampSeats('42')).toBe(42);
  expect(clampSeats(10 ** 9)).toBe(MAX_SEATS);
  expect(clampSeats(Number.NaN)).toBe(1);
});

// Intl separa símbolo y cifra con espacios duros (U+00A0, U+202F); los
// catálogos, con espacios normales. Se comparan sin espacios de ningún tipo.
const sinEspacios = (s: string) => s.replace(/\s/gu, '');

test('el formato es el de cada idioma, con «€» y sin decimales de sobra', () => {
  expect(formatPrice('en', 500)).toBe('€5');
  expect(formatPrice('en', 50)).toBe('€0.50');
  expect(sinEspacios(formatPrice('es', 2500))).toBe('25€');
  expect(sinEspacios(formatPrice('nl', 500))).toBe('€5');
  // Sin `narrowSymbol` estos tres escribían «5 EUR».
  for (const l of ['hu', 'is', 'ro'] as Locale[]) {
    expect(formatPrice(l, 500)).toContain('€');
  }
});

/**
 * La revalorización va en una frase traducida a mano («sube 0,50 € cada cuatro
 * años»): no se puede interpolar sin rehacer 26 traducciones. Lo que sí se
 * puede es comprobar que la cifra escrita en cada idioma es la de Go.
 */
test('la revalorización y el descuento educativo que promete cada idioma son los de commerce', async () => {
  const dir = join(import.meta.dir, '../i18n/messages');
  const ficheros = readdirSync(dir).filter((f) => f.endsWith('.ts'));
  expect(ficheros.length).toBe(LOCALES.length);
  for (const fichero of ficheros) {
    const locale = fichero.replace(/\.ts$/, '') as Locale;
    const modulo = (await import(join(dir, fichero))) as Record<string, Record<string, string>>;
    const catalogo = modulo[locale]!;
    const frase = catalogo['home.pricing.commitment.body']!;
    // El descuento educativo también va escrito en una frase («50 % de
    // descuento en Pro»): la cifra tiene que ser la de `EducationDiscountPercent`.
    const educacion = catalogo['home.pricing.education']!;
    expect({
      locale,
      ok: sinEspacios(frase).includes(sinEspacios(formatPrice(locale, PRICE_RISE_CENTS))),
      descuento: new RegExp(`(^|\\D)${EDUCATION_DISCOUNT_PERCENT}(\\D|$)`, 'u').test(educacion),
    }).toEqual({
      locale,
      ok: true,
      descuento: true,
    });
  }
});
