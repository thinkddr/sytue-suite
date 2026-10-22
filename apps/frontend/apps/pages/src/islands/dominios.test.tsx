/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterEach, beforeEach, describe, expect, test } from 'bun:test';
import { render } from 'preact';
import { Dominios } from './SitioDetalleApp';
import { t } from '../i18n/index';
import type { DominioSitio } from '../lib/dominios';

if (typeof document === 'undefined') GlobalRegistrator.register();
let host: HTMLElement;
let fetchOriginal: typeof fetch;
let publicaciones = 0;
let cambios = 0;

beforeEach(() => {
  fetchOriginal = globalThis.fetch;
  publicaciones = 0;
  cambios = 0;
  host = document.createElement('div');
  document.body.appendChild(host);
});
afterEach(() => {
  render(null, host);
  host.remove();
  globalThis.fetch = fetchOriginal;
});

const montar = () =>
  render(
    <Dominios
      siteId="sitio-1"
      locale="es"
      t={(clave, params) => t('es', clave, params)}
      onPublicar={() => {
        publicaciones++;
      }}
      onCambio={() => {
        cambios++;
      }}
    />,
    host,
  );
const esperar = async (condicion: () => boolean) => {
  const limite = Date.now() + 3000;
  while (!condicion()) {
    if (Date.now() > limite) throw new Error('La condición no llegó a cumplirse');
    await new Promise((r) => setTimeout(r, 10));
  }
};
const boton = (texto: string) =>
  [...host.querySelectorAll('button')].find((b) => b.textContent?.includes(texto));
const completados = () =>
  [...host.querySelectorAll('ol[aria-label="Progreso de conexión del dominio"] li')].filter((li) =>
    li.textContent?.includes('Completado'),
  ).length;
const inicial: DominioSitio = {
  id: 'dom-1',
  host: 'www.aurora.es',
  status: 'pending',
  detail: '',
  primary: false,
  txt: { name: '_sytue.aurora.es', value: 'sytue-verification=token-del-servidor' },
  target: {
    type: 'CNAME',
    name: 'www.aurora.es',
    value: 'sitios.ejemplo.es',
    cname: 'sitios.ejemplo.es',
    a: '192.0.2.10',
  },
};

describe('asistencia para conectar un dominio', () => {
  test('muestra los registros exactos y avanza solo después de comprobar DNS y HTTPS', async () => {
    let actual = { ...inicial };
    const llamadas: { url: string; method: string }[] = [];
    globalThis.fetch = (async (url: string, init?: RequestInit) => {
      const method = init?.method ?? 'GET';
      llamadas.push({ url, method });
      if (method === 'GET') return Response.json([actual]);
      if (url.endsWith('/check')) {
        actual =
          actual.status === 'pending'
            ? { ...actual, status: 'verified', detail: 'certificate' }
            : { ...actual, status: 'active', detail: '' };
      }
      if (url.endsWith('/primary')) actual = { ...actual, primary: true };
      return Response.json(actual);
    }) as unknown as typeof fetch;
    montar();
    await esperar(() => !!host.querySelector('#dom-1-target-value'));
    expect((host.querySelector('#dom-1-target-value') as HTMLInputElement).value).toBe(
      'sitios.ejemplo.es',
    );
    expect((host.querySelector('#dom-1-txt-value') as HTMLInputElement).value).toBe(
      'sytue-verification=token-del-servidor',
    );
    expect(completados()).toBe(1);
    expect(boton('Hacer principal')).toBeUndefined();
    boton('Comprobar DNS y HTTPS')?.click();
    await esperar(() => completados() === 2);
    expect(host.textContent).toContain('El certificado HTTPS todavía no está listo');
    expect(boton('Hacer principal')).toBeUndefined();
    await esperar(() => !boton('Comprobar DNS y HTTPS')?.disabled);
    boton('Comprobar DNS y HTTPS')?.click();
    await esperar(() => completados() === 3);
    expect(boton('Hacer principal')).toBeDefined();
    await esperar(() => !boton('Hacer principal')?.disabled);
    boton('Hacer principal')?.click();
    await esperar(() => completados() === 4);
    boton('Ir a publicar')?.click();
    expect(publicaciones).toBe(1);
    expect(cambios).toBe(3);
    expect(llamadas.filter((l) => l.method === 'POST').map((l) => l.url.split('/api/')[1])).toEqual(
      [
        'sites/sitio-1/domains/dom-1/check',
        'sites/sitio-1/domains/dom-1/check',
        'sites/sitio-1/domains/dom-1/primary',
      ],
    );
  });

  test('un rechazo de permisos no muestra alta ni registros ficticios', async () => {
    globalThis.fetch = (async () =>
      Response.json({ error: { code: 'forbidden' } }, { status: 403 })) as unknown as typeof fetch;
    montar();
    await esperar(
      () =>
        !!host.textContent?.includes(
          'Solo quienes administran la organización pueden gestionar dominios.',
        ),
    );
    expect(host.querySelector('form')).toBeNull();
    expect(host.querySelector('ol')).toBeNull();
  });

  test('una comprobación rechazada mantiene los hitos pendientes', async () => {
    globalThis.fetch = (async (_url: string, init?: RequestInit) =>
      init?.method === 'POST'
        ? Response.json({ error: { code: 'unavailable' } }, { status: 503 })
        : Response.json([inicial])) as unknown as typeof fetch;
    montar();
    await esperar(() => !!boton('Comprobar DNS y HTTPS'));
    boton('Comprobar DNS y HTTPS')?.click();
    await esperar(() => !!host.querySelector('[role="alert"]'));
    expect(completados()).toBe(1);
    expect(host.textContent).not.toContain('Dominios actualizados.');
  });
});
