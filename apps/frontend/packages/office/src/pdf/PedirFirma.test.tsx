/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * PEDIR FIRMA A VARIAS PERSONAS, EN ORDEN (pieza D-CERT-01).
 *
 * Lo que se fija: que se pueden añadir, reordenar y quitar personas, y que lo que
 * sale hacia el BFF es la lista EN EL ORDEN que se ve, con la caducidad y los
 * recordatorios elegidos; y que las pedidas de una cadena se enseñan juntas, en su
 * orden, con quién espera turno y cuándo caduca.
 *
 * Y (pieza CERT-07) que el diálogo está hecho con las piezas de la suite: rótulos
 * enlazados a su control, ayuda y error por `aria-describedby`, el aviso de otra
 * organización como bloque de ayuda y el estado de cada pedida como insignia.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterEach, beforeEach, expect, test } from 'bun:test';
import { render } from 'preact';
import { act } from 'preact/test-utils';
import { docsT } from '../i18n';
import type { PeticionFirma } from './api';
import { PedirFirma, mover, tonoDeEstado } from './PedirFirma';

if (typeof document === 'undefined') GlobalRegistrator.register();

const t = docsT('es');
let host: HTMLDivElement;
let enviados: Record<string, unknown>[];
let pedidas: PeticionFirma[];
// Lo que responde el servicio a un POST: por defecto, creada.
let responder: (cuerpo: Record<string, unknown>) => Response;
const fetchOriginal = globalThis.fetch;

const fila = (p: Partial<PeticionFirma>): PeticionFirma => ({
  id: 'x',
  requester: 'ana',
  recipient: 'r',
  file_id: 'f',
  path: '/PDF/Contrato.pdf',
  status: 'pendiente',
  created_at: '2026-11-01T10:00:00Z',
  ...p,
});

beforeEach(() => {
  host = document.createElement('div');
  document.body.appendChild(host);
  enviados = [];
  pedidas = [];
  responder = () => Response.json({ chain: 'c', requests: [] }, { status: 201 });
  globalThis.fetch = ((_: string, init?: RequestInit) => {
    if (init?.method === 'POST') {
      const cuerpo = JSON.parse(String(init.body)) as Record<string, unknown>;
      enviados.push(cuerpo);
      return Promise.resolve(responder(cuerpo));
    }
    return Promise.resolve(Response.json({ received: [], sent: pedidas }));
  }) as typeof fetch;
});
afterEach(() => {
  act(() => render(null, host));
  host.remove();
  globalThis.fetch = fetchOriginal;
});

async function montar(): Promise<void> {
  act(() => {
    render(<PedirFirma t={t} path="/PDF/Contrato.pdf" onCerrar={() => undefined} />, host);
  });
  // La lista se pide al montar (el efecto corre al cerrar el primer act) y llega por
  // fetch → json → estado: unas vueltas del bucle.
  await act(async () => {
    for (let i = 0; i < 5; i++) await new Promise((r) => setTimeout(r, 0));
  });
}

const boton = (nombre: string) =>
  [...host.querySelectorAll('button')].find(
    (b) => b.getAttribute('aria-label') === nombre || b.textContent?.trim() === nombre,
  )!;

function escribir(i: number, valor: string): void {
  const input = host.querySelectorAll<HTMLInputElement>('ol input')[i]!;
  act(() => {
    input.value = valor;
    input.dispatchEvent(new Event('input', { bubbles: true }));
  });
}

/** El control de un rótulo, por su `for`: así lo encuentra también el lector de pantalla. */
function control<T extends HTMLElement>(rotulo: string): T {
  const label = [...host.querySelectorAll('label')].find((l) => l.textContent?.startsWith(rotulo))!;
  return document.getElementById(label.htmlFor) as T;
}

function elegir(rotulo: string, valor: string): void {
  const select = control<HTMLSelectElement>(rotulo);
  act(() => {
    select.value = valor;
    select.dispatchEvent(new Event('change', { bubbles: true }));
  });
}

test('mover cambia de puesto sin salirse de la lista', () => {
  expect(mover(['a', 'b', 'c'], 2, -1)).toEqual(['a', 'c', 'b']);
  expect(mover(['a', 'b', 'c'], 0, 1)).toEqual(['b', 'a', 'c']);
  expect(mover(['a', 'b'], 0, -1)).toEqual(['a', 'b']);
});

test('con una sola persona no hay orden, y sale como siempre', async () => {
  await montar();
  expect(host.querySelectorAll('ol input')).toHaveLength(1);
  expect(host.textContent).not.toContain(t('pdf.dialog.pedirFirma.orderHelp'));
  escribir(0, 'irati@acme.eu');
  await act(async () => {
    host.querySelector('form')!.dispatchEvent(new Event('submit', { cancelable: true }));
    await new Promise((r) => setTimeout(r, 0));
  });
  expect(enviados[0]).toMatchObject({
    op: 'pedir',
    firmantes: ['irati@acme.eu'],
    caducaDias: 0,
    recordarDias: 0,
  });
  expect(host.textContent).toContain(t('pdf.dialog.pedirFirma.sent', { name: 'irati@acme.eu' }));
});

test('varias personas salen en el orden que se ve, con caducidad y recordatorios', async () => {
  await montar();
  act(() => boton(t('pdf.dialog.pedirFirma.addSigner')).click());
  act(() => boton(t('pdf.dialog.pedirFirma.addSigner')).click());
  expect(host.querySelectorAll('ol input')).toHaveLength(3);
  expect(host.textContent).toContain(t('pdf.dialog.pedirFirma.orderHelp'));
  escribir(0, 'irati@acme.eu');
  escribir(1, 'jon@acme.eu');
  escribir(2, 'quitar@acme.eu');
  act(() => boton(t('pdf.dialog.pedirFirma.removeSigner', { n: 3 })).click());
  // Jon sube: firma primero.
  act(() => boton(t('pdf.dialog.pedirFirma.moveUp', { n: 2 })).click());
  expect(boton(t('pdf.dialog.pedirFirma.moveUp', { n: 1 })).disabled).toBe(true);
  elegir(t('pdf.dialog.pedirFirma.expiryLabel'), '14');
  elegir(t('pdf.dialog.pedirFirma.remindLabel'), '3');
  await act(async () => {
    host.querySelector('form')!.dispatchEvent(new Event('submit', { cancelable: true }));
    await new Promise((r) => setTimeout(r, 0));
  });
  expect(enviados).toHaveLength(1);
  expect(enviados[0]).toMatchObject({
    firmantes: ['jon@acme.eu', 'irati@acme.eu'],
    caducaDias: 14,
    recordarDias: 3,
  });
  expect(host.textContent).toContain(t('pdf.dialog.pedirFirma.sentChain', { name: 'jon@acme.eu' }));
});

test('las pedidas de una cadena salen juntas, en orden, con quién espera y cuándo caduca', async () => {
  pedidas = [
    fila({
      id: 'b',
      chain: 'c1',
      step: 1,
      status: 'en_espera',
      recipient_email: 'jon@acme.eu',
      expires_at: '2026-11-20T10:00:00Z',
    }),
    fila({
      id: 'a',
      chain: 'c1',
      status: 'pendiente',
      recipient_email: 'irati@acme.eu',
      expires_at: '2026-11-20T10:00:00Z',
    }),
    fila({
      id: 's',
      status: 'caducada',
      recipient_email: 'leire@acme.eu',
      created_at: '2026-10-01T10:00:00Z',
    }),
    fila({ id: 'o', path: '/PDF/Otro.pdf', recipient_email: 'otro@acme.eu' }),
  ];
  await montar();
  const texto = host.textContent ?? '';
  expect(texto).not.toContain('otro@acme.eu');
  expect(texto.indexOf('1. irati@acme.eu')).toBeGreaterThan(-1);
  expect(texto.indexOf('2. jon@acme.eu')).toBeGreaterThan(texto.indexOf('1. irati@acme.eu'));
  expect(texto).toContain(t('pdf.signatureRequest.status.en_espera'));
  expect(texto).toContain(t('pdf.signatureRequest.status.caducada'));
  expect(texto).toContain(t('pdf.dialog.pedirFirma.expiresOn', { date: '' }).trim());
  // Cancelar solo en la del turno: cancela la cadena entera.
  expect(
    [...host.querySelectorAll('button')].filter(
      (b) => b.textContent === t('pdf.dialog.pedirFirma.cancel'),
    ),
  ).toHaveLength(1);
});

async function enviarFormulario(): Promise<void> {
  await act(async () => {
    host.querySelector('form')!.dispatchEvent(new Event('submit', { cancelable: true }));
    for (let i = 0; i < 5; i++) await new Promise((r) => setTimeout(r, 0));
  });
}

test('a alguien de otra organización no se le pide nada hasta confirmar (409 → confirmar)', async () => {
  responder = (cuerpo) =>
    cuerpo['confirm'] === 's-leire'
      ? Response.json({ chain: 'c', requests: [] }, { status: 201 })
      : Response.json(
          {
            code: 'pdf_signature_recipient_external',
            message: 'x',
            details: { name: 'Leire', confirm: 's-leire' },
          },
          { status: 409 },
        );
  await montar();
  escribir(0, '@leire');
  await enviarFormulario();
  expect(enviados).toHaveLength(1);
  // El aviso es el bloque informativo de la suite, se anuncia, y el botón lo enlaza.
  const aviso = host.querySelector<HTMLElement>('[role="alert"]')!;
  expect(aviso.textContent).toContain(t('pdf.dialog.pedirFirma.external.title', { name: 'Leire' }));
  expect(aviso.querySelector('.bg-info-bg')).not.toBeNull();
  const confirmar = boton(t('pdf.dialog.pedirFirma.external.confirm'));
  expect(confirmar.getAttribute('aria-describedby')).toBe(aviso.id);
  expect(host.textContent).not.toContain(t('pdf.dialog.pedirFirma.sent', { name: 'Leire' }));

  await enviarFormulario();
  expect(enviados).toHaveLength(2);
  expect(enviados[1]).toMatchObject({ firmantes: ['@leire'], confirm: 's-leire' });
  expect(host.textContent).toContain(t('pdf.dialog.pedirFirma.sent', { name: 'Leire' }));
  expect(host.textContent).not.toContain(t('pdf.dialog.pedirFirma.external.confirm'));
});

test('los campos tienen su rótulo y llevan enlazadas la ayuda y el error', async () => {
  responder = () => Response.json({ code: 'x', message: 'No existe esa persona' }, { status: 400 });
  await montar();
  const correo = control<HTMLInputElement>(t('pdf.dialog.pedirFirma.email'));
  expect(correo.tagName).toBe('INPUT');
  expect(control(t('pdf.dialog.pedirFirma.message')).tagName).toBe('TEXTAREA');
  const ayuda = () =>
    (correo.getAttribute('aria-describedby') ?? '')
      .split(' ')
      .map((id) => document.getElementById(id)?.textContent ?? '');
  expect(ayuda()).toContain(t('pdf.dialog.pedirFirma.help'));
  escribir(0, 'nadie@acme.eu');
  await enviarFormulario();
  const error = host.querySelector('[role="alert"]')!;
  expect(error.textContent).not.toBe('');
  expect(ayuda()).toContain(error.textContent!);
  // En una cadena, la ayuda del orden también.
  act(() => boton(t('pdf.dialog.pedirFirma.addSigner')).click());
  const segundo = host.querySelectorAll<HTMLInputElement>('ol input')[1]!;
  const ids = (segundo.getAttribute('aria-describedby') ?? '').split(' ');
  expect(ids.map((id) => document.getElementById(id)?.textContent)).toContain(
    t('pdf.dialog.pedirFirma.orderHelp'),
  );
});

test('el estado de cada pedida es una insignia con el tono que le toca', async () => {
  expect(tonoDeEstado('pendiente')).toBe('info');
  expect(tonoDeEstado('firmada')).toBe('success');
  expect(tonoDeEstado('rechazada')).toBe('danger');
  expect(tonoDeEstado('cancelada')).toBe('neutral');
  pedidas = [
    fila({ id: 'a', status: 'pendiente', recipient_email: 'a@acme.eu' }),
    fila({ id: 'b', status: 'firmada', recipient_email: 'b@acme.eu', signed_path: '/PDF/f.pdf' }),
    fila({ id: 'c', status: 'rechazada', recipient_email: 'c@acme.eu' }),
  ];
  await montar();
  const insignia = (estado: PeticionFirma['status']) =>
    [...host.querySelectorAll('section .rounded-pill')].find(
      (s) => s.textContent === t(`pdf.signatureRequest.status.${estado}`),
    )!;
  expect(insignia('pendiente').className).toContain('bg-info-bg');
  expect(insignia('firmada').className).toContain('bg-success-bg');
  expect(insignia('rechazada').className).toContain('bg-danger-bg');
  // «Abrir el firmado» es un enlace con la forma del botón, y «Cancelar petición», un botón.
  const abrir = [...host.querySelectorAll('section a')].find(
    (a) => a.textContent === t('pdf.dialog.pedirFirma.openSigned'),
  )!;
  expect(abrir.getAttribute('href')).toContain(encodeURIComponent('/PDF/f.pdf'));
  expect(abrir.className).toContain('rounded-control');
  expect(boton(t('pdf.dialog.pedirFirma.cancel')).className).toContain('rounded-control');
});
