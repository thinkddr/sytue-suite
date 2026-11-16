/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * El asistente de pasarelas (PAGE-01): cancelar no guarda, un rechazo de claves
 * vuelve al paso de claves sin perder lo escrito, y el camino feliz enseña el
 * DataFlow, el webhook, dónde está cada clave y un resumen sin secretos.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterEach, beforeEach, describe, expect, test } from 'bun:test';
import { render } from 'preact';
import { t, type ClaveMensaje } from '../i18n/index';
import { AsistentePasarela } from './AsistentePasarela';
import type { Proveedor } from './tipos';

if (typeof document === 'undefined') GlobalRegistrator.register();

const tr = (clave: ClaveMensaje, params?: Record<string, string | number>) =>
  t('es', clave, params);

let host: HTMLElement;
let fetchOriginal: typeof fetch;
let llamadas: { url: string; method: string; body?: unknown }[];
let cerrado: number;
let guardado: number;

beforeEach(() => {
  fetchOriginal = globalThis.fetch;
  llamadas = [];
  cerrado = 0;
  guardado = 0;
  // Otros ficheros de la misma ejecución dejan avisos en `body` (comparten el DOM de
  // happy-dom), y aquí se busca por `role="alert"` en todo el documento.
  document.body.innerHTML = '';
  host = document.createElement('div');
  document.body.appendChild(host);
});
afterEach(() => {
  render(null, host);
  host.remove();
  globalThis.fetch = fetchOriginal;
});

/** Respuesta del POST de guardar; el GET del webhook siempre contesta. */
const servidor = (guardar: () => Response) => {
  globalThis.fetch = (async (url: string, init?: RequestInit) => {
    const method = init?.method ?? 'GET';
    llamadas.push({
      url,
      method,
      ...(init?.body ? { body: JSON.parse(String(init.body)) } : {}),
    });
    if (url.endsWith('/webhook-url')) {
      return Response.json({ webhook_url: 'https://pagos.ejemplo.es/webhooks/stripe' });
    }
    return guardar();
  }) as unknown as typeof fetch;
};

const montar = (proveedor: Proveedor) =>
  render(
    <AsistentePasarela
      siteId="sitio-1"
      t={tr}
      proveedor={proveedor}
      modoInicial="test"
      onClose={() => {
        cerrado++;
      }}
      onSaved={() => {
        guardado++;
      }}
    />,
    host,
  );

// El diálogo se monta en un portal de `document.body`, no dentro de `host`.
const raiz = () => document.body;
const esperar = async (condicion: () => boolean) => {
  const limite = Date.now() + 3000;
  while (!condicion()) {
    if (Date.now() > limite) throw new Error('La condición no llegó a cumplirse');
    await new Promise((r) => setTimeout(r, 10));
  }
};
const boton = (texto: string) =>
  [...raiz().querySelectorAll('button')].find((b) => b.textContent?.trim() === texto);
const pulsar = async (texto: string) => {
  // Preact pinta en diferido: se deja pasar el render de lo último escrito.
  await new Promise((r) => setTimeout(r, 20));
  await esperar(() => Boolean(boton(texto)) && !boton(texto)?.disabled);
  boton(texto)?.click();
  await new Promise((r) => setTimeout(r, 20));
};
const encabezado = () => raiz().querySelector('h2[tabindex="-1"]')?.textContent ?? '';
const escribir = (id: string, valor: string) => {
  const input = raiz().querySelector<HTMLInputElement>(`#${id}`);
  if (!input) throw new Error(`No está el campo ${id}`);
  input.value = valor;
  input.dispatchEvent(new Event('input', { bubbles: true }));
};
const valorDe = (id: string) => raiz().querySelector<HTMLInputElement>(`#${id}`)?.value;
const posts = () => llamadas.filter((l) => l.method === 'POST');

describe('asistente para conectar una pasarela de pago', () => {
  test('Stripe: modo, webhook, dónde está cada clave, resumen sin secretos y hecho', async () => {
    servidor(() => Response.json({ provider: 'stripe', mode: 'test', active: true }));
    montar('stripe');

    // Paso 1: el modo y, antes de pedir nada, qué datos salen hacia Stripe.
    expect(encabezado()).toBe(tr('store.wz_mode_heading'));
    expect(raiz().querySelector(`section[aria-label]`)?.textContent).toContain('Stripe');
    await pulsar(tr('new.next'));

    // Paso 2: la dirección que hay que pegar en el panel del proveedor.
    expect(encabezado()).toBe(tr('store.wz_webhook_heading', { nombre: 'Stripe' }));
    // Va en el CopyField común: entera, en monoespaciada y con su botón de copiar.
    await esperar(
      () =>
        raiz().querySelector('code')?.textContent === 'https://pagos.ejemplo.es/webhooks/stripe',
    );
    await pulsar(tr('new.next'));

    // Paso 3: cada clave dice dónde está en el panel de Stripe.
    expect(encabezado()).toBe(tr('store.wz_keys_heading', { nombre: 'Stripe' }));
    const pista = raiz().querySelector('#pasarela-secret_key-hint');
    expect(pista?.textContent).toBe(tr('store.hint_stripe_secret'));
    expect(
      raiz().querySelector('#pasarela-secret_key')?.getAttribute('aria-describedby'),
    ).toContain('pasarela-secret_key-hint');
    expect(raiz().querySelector('#pasarela-webhook_secret-hint')?.textContent).toBe(
      tr('store.hint_stripe_webhook'),
    );
    // Sin rellenar no se avanza, y se dice qué falta.
    await pulsar(tr('new.next'));
    expect(encabezado()).toBe(tr('store.wz_keys_heading', { nombre: 'Stripe' }));
    expect(raiz().textContent).toContain(tr('store.field_required'));
    expect(raiz().textContent).toContain(tr('store.stripe_notice_required'));
    escribir('pasarela-secret_key', 'sk_test_123');
    escribir('pasarela-webhook_secret', 'whsec_abc');
    raiz().querySelector<HTMLInputElement>('input[type="checkbox"]')?.click();
    await pulsar(tr('new.next'));

    // Paso 4: el resumen no enseña los secretos y aún no se ha guardado nada.
    expect(encabezado()).toBe(tr('store.wz_check_heading'));
    expect(raiz().textContent).not.toContain('sk_test_123');
    expect(raiz().textContent).not.toContain('whsec_abc');
    expect(raiz().textContent).toContain(tr('store.wz_secret_hidden'));
    expect(posts()).toHaveLength(0);
    await pulsar(tr('store.gateway_save'));

    // Paso 5: hecho.
    await esperar(() => encabezado() === tr('store.wz_done_heading', { nombre: 'Stripe' }));
    expect(guardado).toBe(1);
    expect(posts()).toHaveLength(1);
    expect(posts()[0]?.body).toEqual({
      provider: 'stripe',
      mode: 'test',
      credentials: { secret_key: 'sk_test_123', webhook_secret: 'whsec_abc' },
      public_config: {},
      notice_accepted: true,
    });
    await pulsar(tr('store.wz_finish'));
    expect(cerrado).toBe(1);
  });

  test('un rechazo de las claves vuelve al paso de claves sin perder lo escrito', async () => {
    servidor(() =>
      Response.json(
        { error: { code: 'invalid_gateway_credentials', message: 'Invalid request.' } },
        { status: 400 },
      ),
    );
    montar('mollie');
    await pulsar(tr('new.next'));
    // Mollie no tiene paso de webhook: del modo se va directo a las claves.
    expect(encabezado()).toBe(tr('store.wz_keys_heading', { nombre: 'Mollie' }));
    escribir('pasarela-api_key', 'test_malisima');
    escribir('pasarela-profile_id', 'pfl_123');
    await pulsar(tr('new.next'));
    await pulsar(tr('store.gateway_save'));

    await esperar(() => encabezado() === tr('store.wz_keys_heading', { nombre: 'Mollie' }));
    const alerta = raiz().querySelector('[role="alert"]');
    expect(alerta?.textContent).toContain(tr('store.wz_keys_rejected', { nombre: 'Mollie' }));
    expect(alerta?.textContent).toContain(tr('store.gateway_save_error'));
    // El `message` de httpx va en inglés base: no se cuela en la interfaz.
    expect(alerta?.textContent).not.toContain('Invalid request.');
    expect(valorDe('pasarela-api_key')).toBe('test_malisima');
    expect(valorDe('pasarela-profile_id')).toBe('pfl_123');
    expect(guardado).toBe(0);
  });

  test('otro fallo al guardar se queda en «Comprobar» con el motivo', async () => {
    servidor(() => Response.json({ error: { code: 'internal' } }, { status: 500 }));
    montar('redsys');
    await pulsar(tr('new.next'));
    escribir('pasarela-merchant_code', '999008881');
    escribir('pasarela-terminal', '1');
    escribir('pasarela-secret_key', 'c2VjcmV0bw==');
    await pulsar(tr('new.next'));
    await pulsar(tr('store.gateway_save'));
    await esperar(() => Boolean(raiz().querySelector('[role="alert"]')));
    expect(encabezado()).toBe(tr('store.wz_check_heading'));
    expect(raiz().querySelector('[role="alert"]')?.textContent).toContain(
      tr('store.gateway_save_error'),
    );
    expect(guardado).toBe(0);
  });

  test('cancelar en cualquier paso no guarda nada', async () => {
    servidor(() => Response.json({}));
    montar('adyen');
    await pulsar(tr('new.next'));
    await pulsar(tr('new.next'));
    escribir('pasarela-api_key', 'AQE...');
    await pulsar(tr('store.cancel'));
    expect(cerrado).toBe(1);
    expect(guardado).toBe(0);
    expect(posts()).toHaveLength(0);
  });
});
