/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * «Hacer una copia» (src/lib/copia.ts): el camino común de la tesela y del menú Archivo.
 * Ejecutar: cd apps/frontend/packages/office && bun test test/copia.test.ts
 */
import { afterEach, describe, expect, test } from 'bun:test';
import { hacerCopia } from '../src/lib/copia';
import { en } from '../src/i18n/messages/en';
import { es } from '../src/i18n/messages/es';

const ID = '3f2b6d1c-8a4e-4c1b-9d2e-5a6b7c8d9e0f';
const NUEVO = '9b1deb4d-3b7d-4bad-9bdd-2b0d7b3dcb6d';
const fetchOriginal = globalThis.fetch;
afterEach(() => {
  globalThis.fetch = fetchOriginal;
});

function servir(res: Response): { url: string; cuerpo: unknown }[] {
  const vistas: { url: string; cuerpo: unknown }[] = [];
  globalThis.fetch = ((url: string, init?: RequestInit) => {
    vistas.push({ url, cuerpo: JSON.parse(String(init?.body)) });
    return Promise.resolve(res);
  }) as typeof fetch;
  return vistas;
}

describe('hacerCopia', () => {
  // VISTO EN ROJO (2026-11-05): sin `copy_of` en el cuerpo, el BFF creaba un documento en
  // blanco con el título de la copia y este test lo decía.
  test('pide al BFF la copia del original, con su tipo y el título traducido', async () => {
    const vistas = servir(new Response(JSON.stringify({ id: NUEVO }), { status: 201 }));
    const id = await hacerCopia({ id: ID, kind: 'spreadsheet', title: 'Presupuesto' }, 'Copia');
    expect(id).toBe(NUEVO);
    expect(vistas).toEqual([
      { url: '/api/create', cuerpo: { kind: 'spreadsheet', title: 'Copia', copy_of: ID } },
    ]);
  });

  test('un no del BFF, o una respuesta sin id, es un error: no hay a dónde ir', async () => {
    servir(new Response('{"error":"create_failed"}', { status: 502 }));
    await expect(hacerCopia({ id: ID, kind: 'document', title: 'T' }, 'C')).rejects.toThrow();
    servir(new Response('{}', { status: 201 }));
    await expect(hacerCopia({ id: ID, kind: 'document', title: 'T' }, 'C')).rejects.toThrow();
  });

  test('el título de la copia es una frase entera del catálogo, con el original dentro', () => {
    expect(en['copy.title']).toContain('{title}');
    expect(es['copy.title']).toBe('Copia de {title}');
  });
});
