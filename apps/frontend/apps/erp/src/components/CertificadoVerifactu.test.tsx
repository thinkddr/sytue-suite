/* SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterEach, beforeEach, expect, test } from 'bun:test';
import { render } from 'preact';
import { act } from 'preact/test-utils';
import { TarjetaVerifactu } from './Verifactu.js';
import { aBase64, examinar } from './CertificadoVerifactu.js';
import { useT as traducir } from '../i18n/index.js';
import type { EstadoVerifactu } from './tipos.js';

if (typeof document === 'undefined') GlobalRegistrator.register();
const t = traducir('es');

const sinCertificado: EstadoVerifactu = {
  activo: false,
  pendientes: 0,
  con_errores: 0,
  rechazadas: 0,
  motivo: 'sin_certificado',
  entorno: 'produccion',
};
const original = globalThis.fetch;
let host: HTMLDivElement;
let estado: EstadoVerifactu;
let enviados: Record<string, unknown>[];
let contrasenaBuena: string;
beforeEach(() => {
  estado = sinCertificado;
  enviados = [];
  contrasenaBuena = 'secreta';
  host = document.createElement('div');
  document.body.appendChild(host);
  globalThis.fetch = (async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = new URL(String(input), 'https://erp.example.test');
    const response = (status: number, data: unknown) =>
      new Response(JSON.stringify(data), {
        status,
        headers: { 'content-type': 'application/json' },
      });
    if (url.pathname === '/api/verifactu') return response(200, estado);
    if (url.pathname === '/api/verifactu/certificado' && init?.method === 'PUT') {
      const cuerpo = JSON.parse(String(init.body)) as Record<string, unknown>;
      enviados.push(cuerpo);
      if (cuerpo.contrasena !== undefined && cuerpo.contrasena !== contrasenaBuena) {
        return response(422, {
          error: { code: 'certificado_contrasena', message: 'The certificate password…' },
        });
      }
      const { motivo: _motivo, ...resto } = sinCertificado;
      estado = {
        ...resto,
        tiene_certificado: true,
        certificado_nif: 'B89890001',
      };
      return response(200, estado);
    }
    throw new Error('Petición inesperada ' + url.pathname);
  }) as typeof fetch;
});
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
function boton(texto: string): HTMLButtonElement {
  const b = [...document.querySelectorAll('button')].find((x) => x.textContent?.trim() === texto);
  if (!b) throw new Error('Falta botón ' + texto);
  return b;
}
async function pulsar(b: HTMLElement): Promise<void> {
  await act(async () => b.click());
}
async function montar(administrador = true): Promise<void> {
  await act(async () =>
    render(<TarjetaVerifactu administrador={administrador} locale="es" revision={0} t={t} />, host),
  );
  await esperar(() => document.body.textContent?.includes('VERI*FACTU') === true);
}
async function elegir(selector: string, fichero: File): Promise<void> {
  const input = document.querySelector<HTMLInputElement>(selector)!;
  Object.defineProperty(input, 'files', { value: [fichero], configurable: true });
  await act(async () => {
    input.dispatchEvent(new Event('change', { bubbles: true }));
  });
}
async function escribir(selector: string, valor: string): Promise<void> {
  const input = document.querySelector<HTMLInputElement>(selector)!;
  await act(async () => {
    input.value = valor;
    input.dispatchEvent(new Event('input', { bubbles: true }));
  });
}
/** El botón de avance del asistente es siempre el último del pie. */
function avanzar(): HTMLButtonElement {
  return [...document.querySelectorAll<HTMLButtonElement>('[role="dialog"] button')].at(-1)!;
}

// Un `.p12` de mentira: lo único que importa aquí es que NO es PEM.
const p12 = new File(
  [new Uint8Array([0x30, 0x82, 0x0a, 0x5e, 0x02, 0x01, 0x03, 0xff])],
  'empresa.p12',
);

test('sin certificado ya no dice «todavía no está disponible»: ofrece subirlo', async () => {
  await montar();
  expect(document.body.textContent).toContain(t('vf.reason.certificate'));
  expect(document.body.textContent).not.toContain('Todavía no está disponible');
  expect(boton(t('vf.cert.upload'))).toBeTruthy();
});

test('quien no administra ve el motivo pero no el botón', async () => {
  await montar(false);
  expect(document.body.textContent).toContain(t('vf.reason.certificate'));
  expect(() => boton(t('vf.cert.upload'))).toThrow();
});

test('el .p12 con contraseña equivocada se queda en su paso y luego se guarda', async () => {
  contrasenaBuena = 'secreta';
  await montar();
  await pulsar(boton(t('vf.cert.upload')));
  await esperar(() => Boolean(document.querySelector('#vf-certificado')));
  expect(avanzar().disabled).toBe(true);

  await elegir('#vf-certificado', p12);
  await esperar(() => !avanzar().disabled);
  await pulsar(avanzar());
  await esperar(() => Boolean(document.querySelector('#vf-contrasena')));
  expect(avanzar().disabled).toBe(true); // sin contraseña no se intenta abrir

  await escribir('#vf-contrasena', 'otra');
  await pulsar(boton(t('vf.cert.save')));
  await esperar(() => document.body.textContent?.includes(t('vf.error.wrongPassword')) === true);
  expect(document.querySelector('#vf-contrasena')?.getAttribute('aria-invalid')).toBe('true');

  await escribir('#vf-contrasena', 'secreta');
  await pulsar(boton(t('vf.cert.save')));
  await esperar(() => document.body.textContent?.includes(t('vf.cert.saved')) === true);

  expect(enviados).toHaveLength(2);
  expect(enviados[1]).toEqual({
    fichero: aBase64(await p12.arrayBuffer()),
    contrasena: 'secreta',
    sello: false,
  });
  // La tarjeta ya dice con qué certificado se presenta.
  expect(document.body.textContent).toContain(t('vf.cert.current', { nif: 'B89890001' }));
});

test('un PEM sin clave pide el fichero de la clave y no pide contraseña', async () => {
  await montar();
  await pulsar(boton(t('vf.cert.upload')));
  await esperar(() => Boolean(document.querySelector('#vf-certificado')));
  const cert = '-----BEGIN CERTIFICATE-----\nAAAA\n-----END CERTIFICATE-----\n';
  const clave = '-----BEGIN PRIVATE KEY-----\nBBBB\n-----END PRIVATE KEY-----\n';
  await elegir('#vf-certificado', new File([cert], 'empresa.pem'));
  await esperar(() => Boolean(document.querySelector('#vf-clave')));
  expect(avanzar().disabled).toBe(true);
  await elegir('#vf-clave', new File([clave], 'empresa.key'));
  await esperar(() => !avanzar().disabled);
  await pulsar(avanzar());
  await esperar(() => Boolean(document.body.textContent?.includes(t('vf.cert.seal'))));
  expect(document.querySelector('#vf-contrasena')).toBeNull();
  await pulsar(boton(t('vf.cert.save')));
  await esperar(() => enviados.length === 1);
  expect(enviados[0]).toEqual({ fichero: btoa(cert), clave: btoa(clave), sello: false });
});

test('examinar no se fía de la extensión y aBase64 aguanta ficheros grandes', async () => {
  expect(await examinar(new Blob(['-----BEGIN CERTIFICATE-----']))).toMatchObject({
    pem: true,
    conClave: false,
  });
  expect(
    await examinar(new Blob(['-----BEGIN EC PRIVATE KEY-----\n-----BEGIN CERTIFICATE-----'])),
  ).toMatchObject({ pem: true, conClave: true });
  expect((await examinar(p12)).pem).toBe(false);
  const grande = new Uint8Array(200_000).fill(65);
  expect(aBase64(grande.buffer)).toBe(btoa('A'.repeat(200_000)));
});
