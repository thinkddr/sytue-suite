/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * La tarjeta de una invitación de fuera en el lector (D-MAIL-04): Sí/Quizá/No
 * contesta, marca lo elegido y dice si a quien organiza no le llegó el aviso.
 * Ejecutar: `bun test`.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterEach, expect, test } from 'bun:test';
import { render } from 'preact';
import { act } from 'preact/test-utils';
import { ToastProvider } from '@sytue/ui';
import { en } from '../i18n/messages/en.js';
import { InvitacionCalendario } from './InvitacionCalendario.js';
import type { ResumenInvitacion } from '../lib/invitacion.js';

if (typeof document === 'undefined') GlobalRegistrator.register();

const t = ((key: keyof typeof en, vars?: Record<string, string>) =>
  String(en[key]).replace(
    /\{(\w+)\}/g,
    (_, k: string) => vars?.[k] ?? '',
  )) as unknown as Parameters<typeof InvitacionCalendario>[0]['t'];

const INV: ResumenInvitacion = {
  uid: 'g-1@google.com',
  summary: 'Revisión',
  start: new Date('2026-11-12T09:00:00Z'),
  end: new Date('2026-11-12T10:00:00Z'),
  allDay: false,
  organizer: 'luis@otro.example',
  organizerName: 'Luis',
};

afterEach(() => {
  document.body.innerHTML = '';
});

function pintar(responder: Parameters<typeof InvitacionCalendario>[0]['responder']) {
  const host = document.createElement('div');
  document.body.appendChild(host);
  render(
    <ToastProvider>
      <InvitacionCalendario
        t={t}
        locale="en"
        emailId="m-1"
        invitacion={INV}
        responder={responder}
      />
    </ToastProvider>,
    host,
  );
  return host;
}

const boton = (host: HTMLElement, texto: string) =>
  [...host.querySelectorAll('button')].find((b) => b.textContent?.trim() === texto)!;

test('pinta qué y quién convoca, y «Quizá» contesta TENTATIVE', async () => {
  const llamadas: string[] = [];
  const host = pintar(async (id, ps) => {
    llamadas.push(`${id}|${ps}`);
    return { uid: INV.uid, partstat: ps, replySent: true };
  });
  expect(host.textContent).toContain('Revisión');
  expect(host.textContent).toContain('Organised by Luis');
  await act(async () => {
    boton(host, en['reader.invite.maybe']).click();
  });
  expect(llamadas).toEqual(['m-1|TENTATIVE']);
  expect(boton(host, en['reader.invite.maybe']).getAttribute('aria-pressed')).toBe('true');
  expect(boton(host, en['reader.invite.yes']).getAttribute('aria-pressed')).toBe('false');
  expect(host.querySelector('[role="status"]')?.textContent).toContain(
    en['reader.invite.tentative'],
  );
  expect(host.textContent).not.toContain(en['reader.invite.replyNotSent']);
});

test('si el aviso no sale por correo, lo dice', async () => {
  const host = pintar(async (_, ps) => ({ uid: INV.uid, partstat: ps, replySent: false }));
  await act(async () => {
    boton(host, en['reader.invite.yes']).click();
  });
  expect(host.textContent).toContain(en['reader.invite.replyNotSent']);
});

test('un fallo no marca ninguna respuesta', async () => {
  const host = pintar(async () => {
    throw new Error('502');
  });
  await act(async () => {
    boton(host, en['reader.invite.no']).click();
  });
  expect(boton(host, en['reader.invite.no']).getAttribute('aria-pressed')).toBe('false');
});
