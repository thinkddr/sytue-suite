/* SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterEach, beforeEach, expect, test } from 'bun:test';
import { render } from 'preact';
import { act } from 'preact/test-utils';
import { ToastProvider } from '@sytue/ui';
import { EnviarFactura } from './EnviarFactura.js';
import { useT as traducir } from '../i18n/index.js';

if (typeof document === 'undefined') GlobalRegistrator.register();
const t = traducir('es');
const original = globalThis.fetch;
let host: HTMLDivElement;
let previa = { para: 'facturas@cliente.es', enviable: true, disponible: true };
let enviados: Record<string, string>[] = [];

beforeEach(() => {
  previa = { para: 'facturas@cliente.es', enviable: true, disponible: true };
  enviados = [];
  host = document.createElement('div');
  document.body.appendChild(host);
  globalThis.fetch = (async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = new URL(String(input), 'https://erp.example.test');
    const json = (status: number, data: unknown) =>
      new Response(JSON.stringify(data), {
        status,
        headers: { 'content-type': 'application/json' },
      });
    if (url.pathname === '/api/factura_venta/f1/enviar' && init?.method === 'POST') {
      enviados.push(JSON.parse(String(init.body)) as Record<string, string>);
      return json(200, { enviada: true });
    }
    if (url.pathname === '/api/factura_venta/f1/enviar') return json(200, previa);
    return json(404, {});
  }) as typeof fetch;
});

afterEach(() => {
  render(null, host);
  host.remove();
  globalThis.fetch = original;
});

async function pintar(onCerrar = () => {}, onEnviada = () => {}): Promise<void> {
  await act(async () => {
    render(
      <ToastProvider>
        <EnviarFactura
          facturaId="f1"
          referencia="F-2026/0007"
          t={t}
          onCerrar={onCerrar}
          onEnviada={onEnviada}
        />
      </ToastProvider>,
      host,
    );
  });
  await act(async () => {
    await new Promise((r) => setTimeout(r, 0));
  });
}

const boton = (texto: string): HTMLButtonElement =>
  [...document.querySelectorAll('button')].find((b) =>
    b.textContent?.includes(texto),
  ) as HTMLButtonElement;

test('arranca con el correo de facturación del cliente y manda lo escrito', async () => {
  let cerrado = false;
  let enviada = false;
  await pintar(
    () => (cerrado = true),
    () => (enviada = true),
  );
  const para = document.querySelector('input[type="email"]') as HTMLInputElement;
  expect(para.value).toBe('facturas@cliente.es');
  expect(document.body.textContent).toContain(t('send.attachment'));
  const pdf = document.querySelector('a[download]') as HTMLAnchorElement;
  expect(pdf.getAttribute('href')).toContain('/api/factura_venta/f1/pdf');

  await act(async () => {
    boton(t('send.submit')).click();
    await new Promise((r) => setTimeout(r, 0));
  });
  expect(enviados).toEqual([
    {
      para: 'facturas@cliente.es',
      asunto: t('send.defaultSubject', { ref: 'F-2026/0007' }),
      mensaje: t('send.defaultMessage', { ref: 'F-2026/0007' }),
    },
  ]);
  expect(enviada).toBe(true);
  expect(cerrado).toBe(true);
});

test('una dirección mal escrita no sale y lo dice junto al campo', async () => {
  previa = { ...previa, para: '' };
  await pintar();
  const para = document.querySelector('input[type="email"]') as HTMLInputElement;
  await act(async () => {
    para.value = 'no-es-un-correo';
    para.dispatchEvent(new Event('input', { bubbles: true }));
  });
  await act(async () => {
    boton(t('send.submit')).click();
  });
  expect(enviados).toEqual([]);
  expect(document.body.textContent).toContain(t('send.invalidTo'));
  expect(para.getAttribute('aria-invalid')).toBe('true');
});

test('sin servidor de correo lo avisa y ofrece el PDF', async () => {
  previa = { ...previa, disponible: false };
  await pintar();
  expect(document.body.textContent).toContain(t('send.unavailable'));
  expect(boton(t('send.submit')).disabled).toBe(true);
  expect(document.querySelector('a[download]')).not.toBeNull();
});

test('un borrador no se envía', async () => {
  previa = { ...previa, enviable: false };
  await pintar();
  expect(document.body.textContent).toContain(t('send.notIssued'));
  expect(boton(t('send.submit')).disabled).toBe(true);
});

test('el recordatorio de pago lleva su asunto, el importe y el vencimiento, y se marca como tal', async () => {
  await act(async () => {
    render(
      <ToastProvider>
        <EnviarFactura
          facturaId="f1"
          referencia="F-2026/0007"
          t={t}
          recordatorio={{ importe: '121,00 €', vencimiento: '1 nov 2026' }}
          onCerrar={() => {}}
        />
      </ToastProvider>,
      host,
    );
  });
  await act(async () => {
    await new Promise((r) => setTimeout(r, 0));
  });
  expect(document.body.textContent).toContain(t('remind.title', { ref: 'F-2026/0007' }));
  await act(async () => {
    boton(t('send.submit')).click();
    await new Promise((r) => setTimeout(r, 0));
  });
  expect(enviados).toEqual([
    {
      para: 'facturas@cliente.es',
      asunto: t('remind.defaultSubject', { ref: 'F-2026/0007' }),
      mensaje: t('remind.defaultMessage', {
        ref: 'F-2026/0007',
        amount: '121,00 €',
        date: '1 nov 2026',
      }),
      recordatorio: true as unknown as string,
    },
  ]);
  expect(enviados[0]?.mensaje).toContain('121,00 €');
});
