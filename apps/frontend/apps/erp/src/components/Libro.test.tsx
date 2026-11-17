/* SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterEach, beforeEach, expect, test } from 'bun:test';
import { render } from 'preact';
import { act } from 'preact/test-utils';
import { Balance, LibroDiario, reaperturaHasta } from './Libro.js';
import { propuestasDeCierre, rangoDePeriodo } from './periodo.js';
import { formatearFecha } from './formato.js';
import { hoyISO } from './Seguimiento.js';
import type { EstadoCierre } from './tipos.js';
import { useT as traducir } from '../i18n/index.js';

if (typeof document === 'undefined') GlobalRegistrator.register();
const t = traducir('es');
const original = globalThis.fetch;
let host: HTMLDivElement;
let pedidas: string[] = [];
// Las consultas con `desde` contestan 500: un período que falla.
let fallarConPeriodo = false;
/** El libro de cierres que contesta el servicio falso, y lo que se le mandó. */
let cierre: EstadoCierre = { cerrado_hasta: '', historial: [] };
let escritos: { ruta: string; cuerpo: Record<string, string> }[] = [];
beforeEach(() => {
  pedidas = [];
  escritos = [];
  fallarConPeriodo = false;
  cierre = { cerrado_hasta: '', historial: [] };
  host = document.createElement('div');
  document.body.appendChild(host);
  globalThis.fetch = (async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = new URL(String(input), 'https://erp.example.test');
    if (url.pathname.startsWith('/api/contabilidad/')) return servicioDeCierre(url, init);
    pedidas.push(url.pathname + url.search);
    if (fallarConPeriodo && url.searchParams.has('desde')) {
      return new Response(JSON.stringify({ error: { code: 'unavailable', message: 'Down' } }), {
        status: 503,
      });
    }
    const cuerpo =
      url.pathname === '/api/balance'
        ? { cuentas: [], total_debe: 0, total_haber: 0 }
        : { asientos: [] };
    return new Response(JSON.stringify(cuerpo), {
      status: 200,
      headers: { 'content-type': 'application/json' },
    });
  }) as typeof fetch;
});
function servicioDeCierre(url: URL, init?: RequestInit): Response {
  const json = (cuerpo: unknown): Response =>
    new Response(JSON.stringify(cuerpo), {
      status: 200,
      headers: { 'content-type': 'application/json' },
    });
  if ((init?.method ?? 'GET') === 'GET') return json(cierre);
  const cuerpo = JSON.parse(String(init?.body)) as Record<string, string>;
  escritos.push({ ruta: url.pathname, cuerpo });
  const accion = url.pathname.endsWith('/cierre') ? 'cierre' : 'reapertura';
  cierre = {
    cerrado_hasta: cuerpo.hasta ?? '',
    historial: [
      {
        accion,
        hasta: cuerpo.hasta ? `${cuerpo.hasta}T00:00:00Z` : null,
        motivo: cuerpo.motivo ?? '',
        en: '2026-11-12T10:00:00Z',
      },
      ...(cierre.historial ?? []),
    ],
  };
  return json({ cerrado_hasta: cierre.cerrado_hasta });
}
afterEach(async () => {
  await act(async () => render(null, host));
  host.remove();
  document.body.innerHTML = '';
  globalThis.fetch = original;
});
async function esperar(condicion: () => boolean): Promise<void> {
  for (let i = 0; i < 40; i++) {
    if (condicion()) return;
    await act(async () => {
      await new Promise((r) => setTimeout(r, 0));
    });
  }
  expect(condicion()).toBe(true);
}
async function elegirPeriodo(valor: string): Promise<void> {
  const sel = document.querySelector<HTMLSelectElement>(
    `select[aria-label="${t('ledger.period')}"]`,
  );
  if (!sel) throw new Error('No hay selector de período');
  sel.value = valor;
  await act(async () => {
    sel.dispatchEvent(new Event('change', { bubbles: true }));
  });
}

// VISTO EN ROJO: sin pasar `rangoDePeriodo(periodo)` a la consulta, la segunda petición
// sale igual que la primera; sin `periodo` en las dependencias, ni siquiera sale.
test('el libro se acota al período elegido y lo dice cuando no hay nada', async () => {
  await act(async () => render(<LibroDiario locale="es" t={t} />, host));
  await esperar(() => document.body.textContent?.includes(t('ledger.empty')) === true);
  await elegirPeriodo('mes');
  const { desde, hasta } = rangoDePeriodo('mes');
  await esperar(() => document.body.textContent?.includes(t('ledger.emptyPeriod')) === true);
  expect(pedidas).toEqual([
    '/api/libro?limite=50',
    `/api/libro?limite=50&desde=${desde}&hasta=${hasta}`,
  ]);
});

test('el sumas y saldos también', async () => {
  await act(async () => render(<Balance locale="es" t={t} />, host));
  await esperar(() => document.body.textContent?.includes(t('ledger.empty')) === true);
  await elegirPeriodo('anioAnterior');
  const { desde, hasta } = rangoDePeriodo('anioAnterior');
  await esperar(() => pedidas.length === 2);
  expect(pedidas).toEqual(['/api/balance', `/api/balance?desde=${desde}&hasta=${hasta}`]);
});

function selector(): HTMLSelectElement | null {
  return document.querySelector<HTMLSelectElement>(`select[aria-label="${t('ledger.period')}"]`);
}

// VISTO EN ROJO: devolviendo solo la rueda mientras carga (o solo el error), el
// `<select>` se desmonta al cambiar de período: el foco cae al `body` y, con un 500,
// el selector desaparece y no hay forma de volver a «Todo».
for (const [nombre, Pantalla] of [
  ['libro', LibroDiario],
  ['balance', Balance],
] as const) {
  test(`${nombre}: el selector sigue ahí, con el foco, al cambiar y si el período falla`, async () => {
    fallarConPeriodo = true;
    await act(async () => render(<Pantalla locale="es" t={t} />, host));
    await esperar(() => document.body.textContent?.includes(t('ledger.empty')) === true);
    const antes = selector();
    antes?.focus();
    expect(document.activeElement).toBe(antes);
    await elegirPeriodo('mes');
    expect(selector()).toBe(antes);
    expect(document.activeElement).toBe(antes);
    await esperar(() => document.querySelector('[role="alert"]') !== null);
    expect(document.querySelector('[role="alert"]')?.textContent).toContain(t('error.offline'));
    expect(selector()).toBe(antes);
    expect(document.activeElement).toBe(antes);
    fallarConPeriodo = false;
    await elegirPeriodo('todo');
    await esperar(() => document.body.textContent?.includes(t('ledger.empty')) === true);
    expect(document.querySelector('[role="alert"]')).toBeNull();
  });
}

// --- El cierre del ejercicio -------------------------------------------------------

const botones = (texto: string): HTMLButtonElement[] =>
  [...document.querySelectorAll('button')].filter((b) => b.textContent?.trim() === texto);
const texto = (): string => document.body.textContent ?? '';
async function pulsar(b: HTMLElement | undefined): Promise<void> {
  if (!b) throw new Error('No está el botón');
  await act(async () => {
    b.click();
  });
}
async function teclear(campo: HTMLInputElement, valor: string): Promise<void> {
  campo.value = valor;
  await act(async () => {
    campo.dispatchEvent(new Event('input', { bubbles: true }));
  });
}
function confirmarDelDialogo(rotulo: string): HTMLButtonElement {
  const b = botones(rotulo).find((x) => x.closest('[role="dialog"]'));
  if (!b) throw new Error(`No está «${rotulo}» en el diálogo`);
  return b;
}

const cerradoA2025: EstadoCierre = {
  cerrado_hasta: '2025-12-31',
  historial: [
    { accion: 'cierre', hasta: '2025-12-31T00:00:00Z', motivo: '', en: '2026-02-01T09:00:00Z' },
    { accion: 'cierre', hasta: '2024-12-31T00:00:00Z', motivo: '', en: '2025-02-01T09:00:00Z' },
  ],
};

test('reabrir deshace un paso: vuelve al corte anterior, no abre todo el libro', () => {
  expect(reaperturaHasta(cerradoA2025)).toBe('2024-12-31');
  expect(
    reaperturaHasta({ cerrado_hasta: '2024-12-31', historial: cerradoA2025.historial!.slice(1) }),
  ).toBe('');
  // Tras una reapertura, la siguiente no vuelve a un corte POSTERIOR al vigente.
  const reabierto: EstadoCierre = {
    cerrado_hasta: '2024-12-31',
    historial: [
      {
        accion: 'reapertura',
        hasta: '2024-12-31T00:00:00Z',
        motivo: 'x',
        en: '2026-03-01T09:00:00Z',
      },
      ...cerradoA2025.historial!,
    ],
  };
  expect(reaperturaHasta(reabierto)).toBe('');
});

// VISTO EN ROJO: sin la franja del cierre en el libro, nada de esto se pintaba.
test('sin permiso se ve hasta cuándo está cerrado, pero no se ofrece cerrar ni reabrir', async () => {
  cierre = cerradoA2025;
  await act(async () => render(<LibroDiario locale="es" t={t} />, host));
  const fecha = formatearFecha('2025-12-31', 'es');
  await esperar(() => texto().includes(t('close.closedUntil', { fecha })));
  expect(botones(t('close.close'))).toHaveLength(0);
  expect(botones(t('close.reopen'))).toHaveLength(0);
});

test('cerrar: propone el fin del período acabado, no deja fecha futura y avisa de la consecuencia', async () => {
  await act(async () => render(<LibroDiario locale="es" t={t} puedeCerrar />, host));
  await esperar(() => botones(t('close.close')).length === 1);
  expect(botones(t('close.reopen'))).toHaveLength(0); // abierto: nada que reabrir
  await pulsar(botones(t('close.close'))[0]);

  const campo = document.querySelector<HTMLInputElement>('[role="dialog"] input[type="date"]');
  if (!campo) throw new Error('No está el campo de fecha');
  expect(campo.max).toBe(hoyISO());
  const [primera] = propuestasDeCierre();
  expect(campo.value).toBe(primera!.hasta);
  expect(texto()).toContain(
    t('close.consequence', { fecha: formatearFecha(primera!.hasta, 'es') }),
  );

  // Una fecha futura tecleada a mano no se puede confirmar.
  await teclear(campo, '2999-12-31');
  expect(texto()).toContain(t('close.futureDate'));
  expect(confirmarDelDialogo(t('close.closeConfirm')).disabled).toBe(true);

  await teclear(campo, primera!.hasta);
  const confirmar = confirmarDelDialogo(t('close.closeConfirm'));
  expect(confirmar.disabled).toBe(false);
  await pulsar(confirmar);
  await esperar(() =>
    texto().includes(t('close.closedUntil', { fecha: formatearFecha(primera!.hasta, 'es') })),
  );
  expect(escritos).toEqual([
    { ruta: '/api/contabilidad/cierre', cuerpo: { hasta: primera!.hasta } },
  ]);
  expect(document.querySelector('[role="dialog"]')).toBeNull();
});

test('cerrar hacia atrás no se ofrece: la fecha tiene que avanzar el corte', async () => {
  cierre = { cerrado_hasta: hoyISO(), historial: [] };
  await act(async () => render(<LibroDiario locale="es" t={t} puedeCerrar />, host));
  await esperar(() => botones(t('close.close')).length === 1);
  await pulsar(botones(t('close.close'))[0]);
  const campo = document.querySelector<HTMLInputElement>('[role="dialog"] input[type="date"]');
  expect(campo?.value).toBe('');
  await teclear(campo!, '2020-01-31');
  expect(texto()).toContain(t('close.notAfterCurrent', { fecha: formatearFecha(hoyISO(), 'es') }));
  expect(confirmarDelDialogo(t('close.closeConfirm')).disabled).toBe(true);
});

test('reabrir exige motivo y vuelve al corte anterior; el historial lo cuenta', async () => {
  cierre = cerradoA2025;
  await act(async () => render(<LibroDiario locale="es" t={t} puedeCerrar />, host));
  await esperar(() => botones(t('close.reopen')).length === 1);
  await pulsar(botones(t('close.reopen'))[0]);
  expect(texto()).toContain(t('close.reopenTo', { fecha: formatearFecha('2024-12-31', 'es') }));
  const aceptar = confirmarDelDialogo(t('close.reopenConfirm'));
  expect(aceptar.disabled).toBe(true);
  const campo = document.querySelector<HTMLInputElement>('#sytue-prompt');
  await teclear(campo!, '   ');
  expect(confirmarDelDialogo(t('close.reopenConfirm')).disabled).toBe(true);
  await teclear(campo!, 'Factura de diciembre mal fechada');
  await pulsar(confirmarDelDialogo(t('close.reopenConfirm')));
  await esperar(() => escritos.length === 1);
  expect(escritos[0]).toEqual({
    ruta: '/api/contabilidad/reapertura',
    cuerpo: { hasta: '2024-12-31', motivo: 'Factura de diciembre mal fechada' },
  });
  await esperar(() =>
    texto().includes(t('close.closedUntil', { fecha: formatearFecha('2024-12-31', 'es') })),
  );

  const historial = botones(t('close.history'))[0];
  expect(historial?.getAttribute('aria-expanded')).toBe('false');
  await pulsar(historial);
  expect(historial?.getAttribute('aria-expanded')).toBe('true');
  const filas = [...document.querySelectorAll(`ol[aria-label="${t('close.history')}"] li`)];
  expect(filas).toHaveLength(3);
  expect(filas[0]?.textContent).toContain(
    t('close.historyReopened', { fecha: formatearFecha('2024-12-31', 'es') }),
  );
  expect(filas[0]?.textContent).toContain('Factura de diciembre mal fechada');
});
