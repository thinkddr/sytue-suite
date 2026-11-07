/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * El chip «Guardado hace 2 min» de la cabecera (layout/EditorHeader.tsx) se pulsa: abre
 * el historial de versiones —el mismo desplegable que «Historial» de Colaboración
 * (HistorialVersiones.tsx)— y, con el guardado en error, reintenta ya. Sin red es texto.
 *
 * VISTO EN ROJO (2026-11-05): con el chip como <span>, no había botón que pulsar y el
 * historial no se pedía.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterEach, expect, test } from 'bun:test';
import { render } from 'preact';
import { docsT } from '../i18n';
import { EditorHeader } from './layout/EditorHeader';
import type { EditorContext, SaveState } from './types';

if (typeof document === 'undefined') GlobalRegistrator.register();

const tick = (): Promise<void> => new Promise((r) => setTimeout(r, 0));
const fetchOriginal = globalThis.fetch;
afterEach(() => {
  globalThis.fetch = fetchOriginal;
  render(null, document.body);
});

function ctxDe(
  save: SaveState,
  connection: 'online' | 'offline',
  llamadas: { versiones: number; guardar: number },
): EditorContext {
  return {
    t: docsT('es'),
    locale: 'es',
    kind: 'document',
    doc: { id: '3f2b6d1c-8a4e-4c1b-9d2e-5a6b7c8d9e0f', title: 'Acta' },
    title: 'Acta',
    save,
    connection,
    peers: [],
    peerCursors: [],
    versions: [{ id: 'v1', label: 'Antes del cierre', created_at: '2026-11-05T10:00:00Z' }],
    loadVersions: () => {
      llamadas.versiones++;
      return Promise.resolve();
    },
    saveNow: () => {
      llamadas.guardar++;
    },
    setTitle: () => undefined,
    exportAs: () => Promise.resolve(),
    soloLectura: false,
    esPropietario: true,
    sourceRef: '',
  } as unknown as EditorContext;
}

function montar(save: SaveState, connection: 'online' | 'offline' = 'online') {
  // La cabecera pregunta por la llamada en curso: aquí no hay ninguna.
  globalThis.fetch = (() => Promise.resolve(new Response('{}'))) as unknown as typeof fetch;
  const llamadas = { versiones: 0, guardar: 0 };
  document.body.innerHTML = '';
  render(<EditorHeader ctx={ctxDe(save, connection, llamadas)} />, document.body);
  const chip = [...document.querySelectorAll('button')].find((b) =>
    b.textContent?.includes(
      save === 'error' ? 'Error al guardar' : save === 'saving' ? 'Guardando' : 'Guardado',
    ),
  );
  return { chip, llamadas };
}

test('guardado: el chip abre el historial de versiones y lo pide', async () => {
  const { chip, llamadas } = montar('saved');
  expect(chip).toBeDefined();
  expect(chip!.getAttribute('aria-haspopup')).toBe('menu');
  expect(chip!.title).toBe('Historial de versiones');
  chip!.click();
  await tick();
  expect(llamadas.versiones).toBe(1);
  expect(document.body.textContent).toContain('Antes del cierre');
  expect(llamadas.guardar).toBe(0);
});

test('en error: el chip reintenta el guardado, sin abrir nada', async () => {
  const { chip, llamadas } = montar('error');
  expect(chip).toBeDefined();
  expect(chip!.title).toBe('Reintentar el guardado');
  expect(chip!.getAttribute('aria-haspopup')).toBeNull();
  chip!.click();
  await tick();
  expect(llamadas).toEqual({ versiones: 0, guardar: 1 });
});

test('sin red: el chip es texto, no un botón', () => {
  const { chip } = montar('saved', 'offline');
  expect(chip).toBeUndefined();
  expect(document.body.textContent).toContain('Guardado en este dispositivo');
});

/* Una región viva que se inserta ya con texto no la anuncia ningún lector: el estado
   del chip tiene que cambiar DENTRO del mismo nodo aria-live, también al cambiar de
   rama (menú de historial → botón de reintento → texto sin red). VISTO EN ROJO
   (2026-11-06) con el aria-live dentro de cada rama: el nodo de 'error' era otro. */
test('el anuncio del chip vive en un solo nodo aria-live que no se desmonta', async () => {
  const { llamadas } = montar('saving');
  const vivaDe = (texto: string): Element | undefined =>
    [...document.querySelectorAll('[aria-live]')].find((n) => n.textContent?.includes(texto));
  const antes = vivaDe('Guardando');
  expect(antes).toBeDefined();
  render(<EditorHeader ctx={ctxDe('error', 'online', llamadas)} />, document.body);
  await tick();
  expect(vivaDe('Error al guardar')).toBe(antes);
  render(<EditorHeader ctx={ctxDe('error', 'offline', llamadas)} />, document.body);
  await tick();
  expect(vivaDe('Guardado en este dispositivo')).toBe(antes);
  // Y una sola región dice el estado: dos anunciarían cada cambio dos veces.
  expect(
    [...document.querySelectorAll('[aria-live]')].filter((n) =>
      n.textContent?.includes('Guardado en este dispositivo'),
    ),
  ).toHaveLength(1);
});
