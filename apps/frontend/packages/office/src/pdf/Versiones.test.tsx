/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * «VERSIONES» DEL PDF (pieza CERT-09).
 *
 * Lo que se fija: que se ve el historial con la actual marcada, que una anterior se
 * abre en otra pestaña sin tocar el documento, que restaurar PREGUNTA antes —y con
 * firmas avisa de que la última puede no estar— y solo con un sí se pide al BFF, y
 * que un «no puedes» del servicio se dice con su frase y no recarga nada.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterEach, beforeEach, expect, test } from 'bun:test';
import { render } from 'preact';
import { act } from 'preact/test-utils';
import type { FileVersion } from '@sytue/api-clients';
import { docsT } from '../i18n';
import { Versiones } from './Versiones';

if (typeof document === 'undefined') GlobalRegistrator.register();

const t = docsT('es');
const RUTA = '/PDF/Contrato.pdf';
let host: HTMLDivElement;
let enviados: Record<string, unknown>[];
let lista: FileVersion[];
let responderPost: () => Response;
let restauradas: number[];
const fetchOriginal = globalThis.fetch;

beforeEach(() => {
  host = document.createElement('div');
  document.body.appendChild(host);
  enviados = [];
  restauradas = [];
  lista = [
    { version: 3, sizeBytes: 3000, isCurrent: true, createdAt: '2026-11-10T10:00:00Z' },
    { version: 2, sizeBytes: 2000, isCurrent: false, createdAt: '2026-11-09T10:00:00Z' },
    { version: 1, sizeBytes: 1000, isCurrent: false, author: 'ana' },
  ];
  responderPost = () => Response.json({ ok: true });
  globalThis.fetch = ((url: string, init?: RequestInit) => {
    if (init?.method === 'POST') {
      enviados.push({ url, ...(JSON.parse(String(init.body)) as Record<string, unknown>) });
      return Promise.resolve(responderPost());
    }
    return Promise.resolve(Response.json({ versions: lista }));
  }) as typeof fetch;
});

afterEach(() => {
  act(() => render(null, host));
  host.remove();
  // El diálogo de confirmar va por un portal al <body>: que no quede para el siguiente.
  document.body.innerHTML = '';
  globalThis.fetch = fetchOriginal;
});

async function esperar(): Promise<void> {
  await act(async () => {
    for (let i = 0; i < 6; i++) await new Promise((r) => setTimeout(r, 0));
  });
}

async function montar(firmado = false): Promise<void> {
  act(() => {
    render(
      <Versiones
        t={t}
        locale="es"
        path={RUTA}
        firmado={firmado}
        onRestaurada={(v) => restauradas.push(v)}
      />,
      host,
    );
  });
  await esperar();
}

const botonEn = (raiz: ParentNode, nombre: string) =>
  [...raiz.querySelectorAll('button')].find(
    (b) => b.getAttribute('aria-label') === nombre || b.textContent?.trim() === nombre,
  );

/** El diálogo de confirmar (el `ConfirmDialog` de la suite), si está abierto. */
const confirmacion = () => document.querySelector('[role="dialog"], [role="alertdialog"]');

test('lista el historial con la actual marcada y sin acciones sobre ella', async () => {
  await montar();
  const filas = [...host.querySelectorAll('li')];
  expect(filas.map((f) => f.querySelector('.font-medium')?.textContent)).toEqual([
    'Versión 3',
    'Versión 2',
    'Versión 1',
  ]);
  expect(filas[0]!.getAttribute('aria-current')).toBe('true');
  expect(filas[0]!.textContent).toContain(t('pdf.versions.current'));
  // Sobre la vigente no hay nada que abrir ni restaurar: ya es lo que se ve.
  expect(filas[0]!.querySelector('a, button')).toBeNull();
  expect(filas[2]!.textContent).toContain('ana');
});

test('«Abrir» es un enlace a esa versión en otra pestaña, no una acción sobre el documento', async () => {
  await montar();
  const abrir = host.querySelector<HTMLAnchorElement>(
    `a[aria-label="${t('pdf.versions.openAria', { version: 2 })}"]`,
  );
  expect(abrir).not.toBeNull();
  expect(abrir!.getAttribute('target')).toBe('_blank');
  const url = new URL(abrir!.getAttribute('href')!, 'https://x.test');
  expect(url.pathname).toEndWith('/api/pdf-versiones');
  expect(url.searchParams.get('path')).toBe(RUTA);
  expect(url.searchParams.get('version')).toBe('2');
  expect(enviados).toEqual([]);
});

test('restaurar pregunta antes y solo con un sí lo pide; luego avisa al visor', async () => {
  await montar();
  act(() => botonEn(host, t('pdf.versions.restoreAria', { version: 2 }))!.click());
  await esperar();
  const dialogo = confirmacion();
  expect(dialogo?.textContent).toContain(t('pdf.versions.confirm', { version: 2 }));
  // Sin firmas no se habla de firmas: el aviso asustaría sin motivo.
  expect(dialogo?.textContent).not.toContain(t('pdf.versions.confirm.signed'));
  expect(enviados).toEqual([]);

  act(() => botonEn(dialogo!, t('pdf.versions.restore'))!.click());
  await esperar();
  expect(enviados).toEqual([{ url: '/api/pdf-versiones', path: RUTA, version: 2 }]);
  expect(restauradas).toEqual([2]);
});

test('con firmas, la pregunta avisa de que la versión puede no llevar la última', async () => {
  await montar(true);
  act(() => botonEn(host, t('pdf.versions.restoreAria', { version: 1 }))!.click());
  await esperar();
  expect(confirmacion()?.textContent).toContain(t('pdf.versions.confirm.signed'));
});

test('decir que no a la pregunta no toca nada', async () => {
  await montar();
  act(() => botonEn(host, t('pdf.versions.restoreAria', { version: 2 }))!.click());
  await esperar();
  // El otro botón del diálogo, el que no es «Restaurar» (su rótulo es del catálogo común).
  const cancelar = [...confirmacion()!.querySelectorAll('button')].find(
    (b) => b.textContent?.trim() && b.textContent.trim() !== t('pdf.versions.restore'),
  );
  act(() => cancelar!.click());
  await esperar();
  expect(confirmacion()).toBeNull();
  expect(enviados).toEqual([]);
  expect(restauradas).toEqual([]);
});

test('sin permiso para restaurar se dice con su frase y el visor no recarga', async () => {
  responderPost = () => Response.json({ error: 'forbidden' }, { status: 403 });
  await montar();
  act(() => botonEn(host, t('pdf.versions.restoreAria', { version: 2 }))!.click());
  await esperar();
  act(() => botonEn(confirmacion()!, t('pdf.versions.restore'))!.click());
  await esperar();
  expect(host.querySelector('[role="alert"]')?.textContent).toBe(t('pdf.versions.denied'));
  expect(restauradas).toEqual([]);
});

test('con solo la versión actual se explica cuándo aparecerán las anteriores', async () => {
  lista = [{ version: 1, sizeBytes: 10, isCurrent: true }];
  await montar();
  expect(host.querySelector('li')).toBeNull();
  expect(host.textContent).toContain(t('pdf.versions.empty'));
});
