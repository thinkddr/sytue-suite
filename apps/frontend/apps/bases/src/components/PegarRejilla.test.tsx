/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * PEGAR Y SOLTAR EN LA REJILLA, como en Fyne: Ctrl+V sobre la celda con el foco escribe
 * el bloque copiado de una hoja (las filas que sobran son registros nuevos y lo que no
 * encaja se dice a la vista); los ficheros soltados sobre una fila van a ella, con la
 * columna sobre la que caen, y mientras se arrastran la rejilla se recuadra.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterAll, afterEach, expect, test } from 'bun:test';
import { render } from 'preact';
import { act } from 'preact/test-utils';
import type { BasesField, BasesRecord, BasesValue } from '@sytue/api-clients/bases';
import { DialogoImportar } from './Dialogos.js';
import { Rejilla, type RejillaProps } from './Rejilla.js';

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
  { id: 'eda', table_id: 't1', name: 'Edad', kind: 'number', position: 1 },
  { id: 'adj', table_id: 't1', name: 'Ficheros', kind: 'attachment', position: 2 },
];

const reg = (id: string, nom: string, eda: number): BasesRecord =>
  ({ id, table_id: 't1', rev: 1, data: { nom, eda }, links: {} }) as unknown as BasesRecord;

const REGISTROS = [reg('r1', 'P1', 1), reg('r2', 'P2', 2), reg('r3', 'P3', 3)];

interface Hechos {
  celdas: [string, string, BasesValue][];
  nuevos: Record<string, BasesValue>[];
  soltados: [string[], string, number][];
}

function montar(extra: Partial<RejillaProps> = {}): Hechos {
  const h: Hechos = { celdas: [], nuevos: [], soltados: [] };
  act(() => {
    render(
      <Rejilla
        t={t}
        nombreTabla="Personas"
        campos={CAMPOS}
        registros={REGISTROS}
        puedeEditar
        etiquetas={{}}
        onCambiar={async (r, data) => {
          for (const [k, v] of Object.entries(data)) h.celdas.push([r.id, k, v]);
          return null;
        }}
        onCrearVarios={async (filas) => {
          h.nuevos.push(...filas);
          return null;
        }}
        onSoltarFicheros={(fs, r, col) => h.soltados.push([fs.map((f) => f.name), r.id, col])}
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
  return h;
}

const celda = (fila: number, col: number) =>
  document.querySelector<HTMLElement>(`[data-fila="${fila}"][data-col="${col}"]`)!;

// Preact escucha `dragover` si el elemento tiene `ondragover`; happy-dom no lo tiene, y
// entonces el nombre va tal cual («DragOver»). Se manda el que escucha.
const NOMBRES: Record<string, string> = { dragover: 'DragOver', drop: 'Drop', paste: 'Paste' };

function evento(tipo: string, campo: 'clipboardData' | 'dataTransfer', valor: object): Event {
  const nombre = `on${tipo}` in document.body ? tipo : NOMBRES[tipo]!;
  const e = new Event(nombre, { bubbles: true, cancelable: true });
  Object.defineProperty(e, campo, { value: valor });
  return e;
}

async function pegarEn(el: HTMLElement, texto: string): Promise<Event> {
  const e = evento('paste', 'clipboardData', { getData: () => texto });
  await act(async () => {
    el.dispatchEvent(e);
    await new Promise((r) => setTimeout(r, 0));
  });
  return e;
}

test('pegar 3×2 desde A1 escribe seis celdas por onCambiar', async () => {
  const h = montar();
  const e = await pegarEn(celda(0, 0), 'A\t10\r\nB\t20\r\nC\t30\r\n');
  expect(e.defaultPrevented).toBe(true);
  expect(h.celdas).toEqual([
    ['r1', 'nom', 'A'],
    ['r1', 'eda', 10],
    ['r2', 'nom', 'B'],
    ['r2', 'eda', 20],
    ['r3', 'nom', 'C'],
    ['r3', 'eda', 30],
  ]);
  expect(h.nuevos).toEqual([]);
  expect(document.querySelector('[data-aviso-pegado]')).toBeNull();
});

test('las filas que sobran se crean y lo que no encaja se avisa a la vista', async () => {
  const h = montar();
  await pegarEn(celda(2, 0), 'Ana\tcuarenta\nBea\t30');
  expect(h.celdas).toEqual([['r3', 'nom', 'Ana']]);
  expect(h.nuevos).toEqual([{ nom: 'Bea', eda: 30 }]);
  expect(document.querySelector('[data-aviso-pegado]')?.textContent).toBe('grid.pasteSkipped:1');
});

test('quien solo lee no pega', async () => {
  const h = montar({ puedeEditar: false });
  const e = await pegarEn(celda(0, 0), 'A');
  expect(e.defaultPrevented).toBe(false);
  expect(h.celdas).toEqual([]);
});

test('los ficheros soltados van a la fila y la columna donde caen, con recuadro mientras tanto', async () => {
  const h = montar();
  const fichero = new File(['hola'], 'factura.txt');
  const dt = { types: ['Files'], files: [fichero], dropEffect: '', getData: () => '' };
  const encima = evento('dragover', 'dataTransfer', dt);
  act(() => {
    celda(1, 1).dispatchEvent(encima);
  });
  expect(encima.defaultPrevented).toBe(true);
  expect(document.querySelector('[data-soltando]')).not.toBeNull();
  expect(document.querySelector('tr[data-suelta]')?.contains(celda(1, 0))).toBe(true);
  act(() => {
    celda(1, 1).dispatchEvent(evento('drop', 'dataTransfer', dt));
  });
  expect(h.soltados).toEqual([[['factura.txt'], 'r2', 1]]);
  expect(document.querySelector('[data-soltando]')).toBeNull();
});

test('soltar texto sobre una celda lo pega desde ella', async () => {
  const h = montar();
  const dt = { types: ['text/plain'], files: [], dropEffect: '', getData: () => 'Dani' };
  await act(async () => {
    celda(2, 0).dispatchEvent(evento('drop', 'dataTransfer', dt));
    await new Promise((r) => setTimeout(r, 0));
  });
  expect(h.celdas).toEqual([['r3', 'nom', 'Dani']]);
  expect(h.soltados).toEqual([]);
});

test('un CSV soltado abre «Importar» con el fichero leído y el nombre de la tabla puesto', async () => {
  const importados: [string, string][] = [];
  const csv = new File(['Cliente\nAcme\n'], 'Proveedores 2026.csv', { type: 'text/csv' });
  await act(async () => {
    render(
      <DialogoImportar
        t={t}
        fichero={csv}
        error=""
        ocupado={false}
        onImportar={(n, c) => importados.push([n, c])}
        onCerrar={() => undefined}
      />,
      document.body,
    );
  });
  // `File.text()` resuelve fuera del render: se espera a que el diálogo lo haya leído.
  for (let i = 0; i < 50 && !document.querySelector<HTMLInputElement>('#import-nombre')?.value; i++)
    await act(() => new Promise((r) => setTimeout(r, 5)));
  const nombre = document.querySelector<HTMLInputElement>('#import-nombre')!;
  expect(nombre.value).toBe('Proveedores 2026');
  expect(document.body.textContent).toContain('Proveedores 2026.csv');
  const importar = [...document.querySelectorAll('button')].find(
    (b) => b.textContent?.trim() === 'import.run',
  )!;
  act(() => importar.click());
  expect(importados).toEqual([['Proveedores 2026', 'Cliente\nAcme\n']]);
});
