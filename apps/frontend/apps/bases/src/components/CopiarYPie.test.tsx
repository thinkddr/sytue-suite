/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Copiar una celda (Ctrl+C) y el pie con los totales de la vista. Copiar deja en el
 * portapapeles el mismo texto que pegar sabe leer —también de un enlace, por la etiqueta
 * del registro enlazado—; el pie escribe lo que el servicio contó de TODA la vista, no
 * de las filas cargadas.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterAll, afterEach, expect, test } from 'bun:test';
import { render } from 'preact';
import { act } from 'preact/test-utils';
import type { BasesField, BasesRecord, BasesSummary } from '@sytue/api-clients/bases';
import { Rejilla, type RejillaProps } from './Rejilla.js';
import { pieDeColumna } from '../lib/resumen.js';

const registrado = typeof document === 'undefined';
if (registrado) GlobalRegistrator.register();
afterAll(async () => {
  if (registrado) await GlobalRegistrator.unregister();
});
afterEach(() => {
  render(null, document.body);
  document.body.innerHTML = '';
});

const t = ((k: string, v?: Record<string, unknown>) =>
  v ? `${k}:${Object.values(v).join(',')}` : k) as never;

const CAMPOS: BasesField[] = [
  { id: 'nom', table_id: 't1', name: 'Nombre', kind: 'text', position: 0 },
  { id: 'imp', table_id: 't1', name: 'Importe', kind: 'number', position: 1 },
  { id: 'cli', table_id: 't1', name: 'Cliente', kind: 'link', link_table_id: 't2', position: 2 },
  { id: 'hec', table_id: 't1', name: 'Hecho', kind: 'checkbox', position: 3 },
];

const REGISTRO = {
  id: 'r1',
  table_id: 't1',
  rev: 1,
  data: { nom: 'Pan', imp: 2.5, hec: true },
  links: { cli: ['c1'] },
} as unknown as BasesRecord;

function montar(extra: Partial<RejillaProps> = {}): void {
  act(() => {
    render(
      <Rejilla
        t={t}
        nombreTabla="Pedidos"
        campos={CAMPOS}
        registros={[REGISTRO]}
        puedeEditar
        etiquetas={{ t2: new Map([['c1', 'Panadería Ana']]) }}
        onCambiar={async () => null}
        onElegirVarios={() => undefined}
        onAdjuntos={() => undefined}
        onAbrir={() => undefined}
        onNuevo={() => undefined}
        desde={0}
        hayAntes={false}
        onAntes={() => undefined}
        hayMas={false}
        onMas={() => undefined}
        {...extra}
      />,
      document.body,
    );
  });
}

const celda = (fila: number, col: number) =>
  document.querySelector<HTMLElement>(`[data-fila="${fila}"][data-col="${col}"]`)!;

/** Copia la celda y devuelve lo que quedó en el portapapeles (y si se canceló el evento). */
function copiar(el: HTMLElement): { texto: string; cancelado: boolean } {
  let texto = '';
  const nombre = 'oncopy' in document.body ? 'copy' : 'Copy';
  const e = new Event(nombre, { bubbles: true, cancelable: true });
  Object.defineProperty(e, 'clipboardData', {
    value: { setData: (_tipo: string, v: string) => (texto = v) },
  });
  act(() => {
    el.dispatchEvent(e);
  });
  return { texto, cancelado: e.defaultPrevented };
}

test('Ctrl+C copia el texto de la celda, y de un enlace su etiqueta', () => {
  montar();
  expect(copiar(celda(0, 0))).toEqual({ texto: 'Pan', cancelado: true });
  expect(copiar(celda(0, 1)).texto).toBe('2.5');
  expect(copiar(celda(0, 2)).texto).toBe('Panadería Ana');
});

test('sin resumen no hay pie; con él, el recuento y la suma de toda la vista', () => {
  montar();
  expect(document.querySelector('[data-pie-resumen]')).toBeNull();
  render(null, document.body);
  const resumen: BasesSummary = {
    count: 120,
    fields: {
      nom: { filled: 118 },
      imp: { filled: 100, sum: 2500, avg: 25, min: 1, max: 90 },
      cli: { filled: 0 },
      hec: { filled: 40 },
    },
  };
  montar({ resumen });
  const pie = document.querySelector('[data-pie-resumen]');
  expect(pie).not.toBeNull();
  const celdas = [...(pie?.querySelectorAll('th, td') ?? [])].map((c) => c.textContent);
  expect(celdas[0]).toBe('summary.count:120');
  expect(celdas[1]).toBe('summary.filled:118');
  expect(celdas[2]).toContain('summary.sum:');
  // Una columna sin ningún valor no dice «0»: se queda vacía.
  expect(celdas[3]).toBe('');
  expect(celdas[4]).toBe('summary.checked:40');
});

test('de una valoración se da la media, y de una moneda, la suma en su moneda', () => {
  const valoracion: BasesField = { ...CAMPOS[0]!, kind: 'rating' };
  expect(
    pieDeColumna(valoracion, { filled: 2, sum: 9, avg: 4.5, min: 4, max: 5 }, t, 'en')?.texto,
  ).toBe('summary.average:4.5');
  const moneda: BasesField = { ...CAMPOS[1]!, kind: 'currency', currency: 'EUR' };
  const pie = pieDeColumna(moneda, { filled: 2, sum: 10, avg: 5, min: 2, max: 8 }, t, 'en');
  expect(pie?.texto).toBe('summary.sum:€10.00');
  expect(pie?.detalle).toBe('summary.detail:€5.00,€2.00,€8.00');
});
