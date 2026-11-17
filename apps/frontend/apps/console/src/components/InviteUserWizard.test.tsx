/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Invitar a una persona es un asistente: a quién, qué podrá hacer, repaso y hecho.
 * Comprueba el recorrido con `fetch` simulado, que el foco cae en el encabezado de
 * cada paso y que cada error de Sytue ID vuelve al paso que lo causa.
 * Ejecutar: `bun test`.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterEach, expect, test } from 'bun:test';
import { render } from 'preact';
import { act } from 'preact/test-utils';

if (typeof document === 'undefined') GlobalRegistrator.register();

const {
  default: InviteUserWizard,
  pasoDelError,
  separarDirecciones,
  MAX_TANDA,
} = await import('./InviteUserWizard.js');
const { useT: traductor } = await import('../i18n/index.js');
const t = traductor('es');

let llamadas: { url: string; method: string; body: Record<string, unknown> }[] = [];
function respuestas(...rs: Response[]) {
  llamadas = [];
  globalThis.fetch = (async (url: string, init?: RequestInit) => {
    llamadas.push({
      url,
      method: init?.method ?? 'GET',
      body: JSON.parse(String(init?.body ?? '{}')),
    });
    return rs.shift() ?? new Response(null, { status: 500 });
  }) as unknown as typeof fetch;
}
const json = (status: number, body: unknown) =>
  new Response(JSON.stringify(body), { status, headers: { 'content-type': 'application/json' } });

let host: HTMLElement | null = null;
afterEach(async () => {
  if (host) await act(async () => render(null, host!));
  host = null;
  document.body.innerHTML = '';
});
let invitadas = 0;
let cerrado = false;
async function montar(canGrantOwner = false) {
  invitadas = 0;
  cerrado = false;
  host = document.createElement('div');
  document.body.appendChild(host);
  await act(async () =>
    render(
      <InviteUserWizard
        t={t}
        locale="es"
        mailDomain="acme.es"
        canGrantOwner={canGrantOwner}
        onInvited={() => invitadas++}
        onClose={() => (cerrado = true)}
      />,
      host!,
    ),
  );
}
const tick = () => new Promise((r) => setTimeout(r, 0));
const boton = (texto: string) =>
  [...document.querySelectorAll('button')].find((b) => b.textContent?.trim() === texto);
const sigue = () => (boton('Continuar') ? 'Continuar' : 'Continue');
async function pulsar(texto: string) {
  const b = boton(texto);
  if (!b) throw new Error(`no hay botón «${texto}»`);
  if (b.disabled) throw new Error(`el botón «${texto}» está deshabilitado`);
  await act(async () => {
    b.click();
    await tick();
  });
}
async function escribir(id: string, valor: string) {
  const i = document.getElementById(id) as HTMLInputElement;
  await act(async () => {
    i.value = valor;
    i.dispatchEvent(new Event('input', { bubbles: true }));
  });
}
async function elegir(titulo: string) {
  const label = [...document.querySelectorAll('label')].find((l) =>
    l.textContent?.includes(titulo),
  );
  const input = label?.querySelector('input');
  if (!input) throw new Error(`no hay opción «${titulo}»`);
  await act(async () => {
    input.click();
    await tick();
  });
}
const texto = () => document.body.textContent ?? '';
const enFoco = () => document.activeElement?.textContent ?? '';

test('el recorrido entero llega al BFF y acaba con la caducidad y «Invitar a otra»', async () => {
  await montar();
  expect(texto()).toContain(t('users.inviteWizard.who.heading'));
  // La pista del campo está ENLAZADA, no es un párrafo suelto.
  const correo = document.getElementById('invite-email')!;
  expect(correo.getAttribute('aria-describedby')).toBe('invite-email-hint');
  expect(document.getElementById('invite-email-hint')?.textContent).toContain(
    t('users.inviteWizard.who.emailHint'),
  );
  expect(texto()).toContain('@acme.es');
  // Sin un correo con pinta de correo no se avanza.
  expect(boton(sigue())?.disabled).toBe(true);
  await escribir('invite-email', '  ana@fuera.com ');
  await escribir('invite-mailbox', 'ana.ruiz');
  await pulsar(sigue());

  expect(enFoco()).toContain(t('users.inviteWizard.role.heading'));
  // El rol ya no es un desplegable: tarjetas con su consecuencia.
  expect(document.querySelector('select')).toBeNull();
  expect(texto()).toContain(t('users.inviteWizard.role.adminBody'));
  // Quien no es propietario no ve la opción de conceder propietario.
  expect(texto()).not.toContain(t('users.inviteWizard.role.ownerBody'));
  await elegir(t('role.admin'));
  await pulsar(sigue());

  expect(enFoco()).toContain(t('users.inviteWizard.review.heading'));
  expect(texto()).toContain('ana.ruiz@acme.es');
  expect(texto()).toContain(t('role.admin'));
  expect(texto()).toContain(t('users.inviteWizard.review.accessLink'));

  respuestas(
    json(201, { email: 'ana@fuera.com', role: 'admin', expires_at: '2026-11-17T10:00:00Z' }),
  );
  await pulsar(t('users.inviteSubmit'));
  expect(llamadas).toEqual([
    {
      url: '/api/admin/invitations',
      method: 'POST',
      body: { email: 'ana@fuera.com', role: 'admin', mailbox_local: 'ana.ruiz' },
    },
  ]);
  expect(invitadas).toBe(1);
  expect(enFoco()).toContain(t('users.inviteWizard.done.heading'));
  expect(texto()).toContain('ana@fuera.com');
  expect(texto()).toContain('2026');

  // «Invitar a otra» vuelve al principio con todo vacío.
  await pulsar(t('users.inviteWizard.done.another'));
  expect(texto()).toContain(t('users.inviteWizard.who.heading'));
  expect((document.getElementById('invite-email') as HTMLInputElement).value).toBe('');
  expect(cerrado).toBe(false);
});

test('un 409 vuelve al paso del correo y se pinta en su pista', async () => {
  await montar(true);
  await escribir('invite-email', 'ya@acme.es');
  await pulsar(sigue());
  // Un propietario sí puede conceder propietario.
  expect(texto()).toContain(t('users.inviteWizard.role.ownerBody'));
  await pulsar(sigue());
  respuestas(json(409, { error: 'conflict' }));
  await pulsar(t('users.inviteSubmit'));
  expect(enFoco()).toContain(t('users.inviteWizard.who.heading'));
  const pista = document.getElementById('invite-email-hint')!;
  expect(pista.textContent).toContain(t('users.toast.alreadyMember'));
  expect(document.getElementById('invite-email')?.getAttribute('aria-invalid')).toBe('true');
  // Corregir el campo borra el error.
  await escribir('invite-email', 'otra@acme.es');
  expect(pista.textContent).not.toContain(t('users.toast.alreadyMember'));
  expect(invitadas).toBe(0);
});

test('un 403 vuelve al paso del rol; un 503 acaba avisando de que el correo no salió', async () => {
  await montar(true);
  await escribir('invite-email', 'jefa@acme.es');
  await pulsar(sigue());
  await elegir(t('role.owner'));
  await pulsar(sigue());
  respuestas(json(403, { error: 'forbidden' }));
  await pulsar(t('users.inviteSubmit'));
  expect(enFoco()).toContain(t('users.inviteWizard.role.heading'));
  expect(texto()).toContain(t('users.toast.roleForbidden'));

  await elegir(t('role.member'));
  await pulsar(sigue());
  respuestas(new Response(null, { status: 503 }));
  await pulsar(t('users.inviteSubmit'));
  expect(texto()).toContain(t('users.toast.inviteNotSent'));
  expect(invitadas).toBe(1);
});

test('con contraseña inicial, el 400 nombra la contraseña y el 409 dice que la cuenta ya existe', async () => {
  await montar();
  await escribir('invite-email', 'nueva@acme.es');
  await escribir('invite-password', 'corta');
  await pulsar(sigue());
  await pulsar(sigue());
  expect(texto()).toContain(t('users.inviteWizard.review.accessPassword'));
  respuestas(json(400, { error: 'bad_request' }));
  await pulsar(t('users.inviteWizard.review.create'));
  expect(llamadas[0]?.body.password).toBe('corta');
  expect(enFoco()).toContain(t('users.inviteWizard.who.heading'));
  const pista = document.getElementById('invite-email-hint')!;
  expect(pista.textContent).toContain(t('users.inviteWizard.who.invalidWithPassword'));

  await pulsar(sigue());
  await pulsar(sigue());
  respuestas(json(409, { error: 'conflict' }));
  await pulsar(t('users.inviteWizard.review.create'));
  // El paso se volvió a montar: la pista es otro nodo.
  const otra = document.getElementById('invite-email-hint')!;
  expect(otra.textContent).toContain(t('users.inviteWizard.who.accountExists'));
  expect(otra.textContent).not.toContain(t('users.toast.alreadyMember'));
  expect(invitadas).toBe(0);
});

test('cada código de la API tiene su paso', () => {
  expect(pasoDelError('400')).toBe(0);
  expect(pasoDelError('409')).toBe(0);
  expect(pasoDelError('403')).toBe(1);
  expect(pasoDelError('500')).toBe(2);
});

// --- Varias personas a la vez ---

test('separar lo pegado: comas, líneas y CSV; lo que no lleva @ se cuenta aparte', () => {
  expect(separarDirecciones('ana@acme.es, Luis@acme.es;\n"marta@acme.es"\t<ana@ACME.es>')).toEqual({
    direcciones: ['ana@acme.es', 'Luis@acme.es', 'marta@acme.es'],
    ignorados: 0,
  });
  // Un CSV con cabecera y nombres: solo salen las direcciones.
  expect(separarDirecciones('nombre,correo\nAna,ana@acme.es\nLuis,luis@acme.es\n')).toEqual({
    direcciones: ['ana@acme.es', 'luis@acme.es'],
    ignorados: 4,
  });
  // Mal escrita pero con @: se envía, para que Sytue ID diga cuál.
  expect(separarDirecciones('ana@').direcciones).toEqual(['ana@']);
});

test('«Varias personas» invita en tanda y enseña el resultado de cada dirección', async () => {
  await montar(true);
  await elegir(t('users.inviteWizard.mode.many'));
  // Los campos de UNA persona no están: ni buzón ni contraseña en tanda.
  expect(document.getElementById('invite-email')).toBeNull();
  expect(document.getElementById('invite-password')).toBeNull();
  const area = document.getElementById('invite-emails')!;
  expect(area.getAttribute('aria-describedby')).toBe('invite-emails-hint invite-emails-count');
  expect(boton(sigue())?.disabled).toBe(true);

  await escribir(
    'invite-emails',
    'nombre,correo\nAna,ana@acme.es\nLuis,luis@acme.es\nya@acme.es mal@',
  );
  expect(document.getElementById('invite-emails-count')?.textContent).toContain(
    t('users.inviteWizard.many.count', { count: 4 }),
  );
  await pulsar(sigue());
  // Propietario no se reparte en tanda, aunque quien invita lo sea.
  expect(texto()).not.toContain(t('users.inviteWizard.role.ownerBody'));
  await elegir(t('role.billing'));
  await pulsar(sigue());
  expect(texto()).toContain('ana@acme.es, luis@acme.es, ya@acme.es, mal@');
  expect(texto()).toContain(t('users.inviteWizard.review.accessLink'));

  respuestas(
    json(200, {
      sent: 2,
      results: [
        { email: 'mal@', status: 'invalid', detail: 'This does not look like an email address.' },
        { email: 'ana@acme.es', status: 'sent', expires_at: '2026-11-17T10:00:00Z' },
        { email: 'luis@acme.es', status: 'sent', expires_at: '2026-11-17T10:00:00Z' },
        { email: 'ya@acme.es', status: 'already_member' },
      ],
    }),
  );
  await pulsar(t('users.inviteWizard.many.submit', { count: 4 }));
  expect(llamadas).toEqual([
    {
      url: '/api/admin/invitations',
      method: 'POST',
      body: { emails: ['ana@acme.es', 'luis@acme.es', 'ya@acme.es', 'mal@'], role: 'billing' },
    },
  ]);
  expect(invitadas).toBe(1);
  expect(enFoco()).toContain(t('users.inviteWizard.done.heading'));
  expect(texto()).toContain(t('users.inviteWizard.many.doneTitle', { sent: 2, total: 4 }));
  const filas = [...document.querySelectorAll('li')].map((li) => li.textContent ?? '');
  expect(filas.find((f) => f.includes('ya@acme.es'))).toContain(
    t('users.inviteWizard.many.status.already_member'),
  );
  // El motivo es el de la consola, traducido, no el texto en inglés de la API.
  expect(filas.find((f) => f.includes('mal@'))).toContain(
    t('users.inviteWizard.many.body.invalid'),
  );
  expect(texto()).not.toContain('does not look like');

  await pulsar(t('users.inviteWizard.done.another'));
  expect((document.getElementById('invite-emails') as HTMLTextAreaElement).value).toBe('');
});

test('más de 25 no deja continuar y lo dice en la pista; un 429 se queda en el repaso', async () => {
  await montar();
  await elegir(t('users.inviteWizard.mode.many'));
  const muchas = Array.from({ length: MAX_TANDA + 1 }, (_, i) => `p${i}@acme.es`).join('\n');
  await escribir('invite-emails', muchas);
  expect(boton(sigue())?.disabled).toBe(true);
  expect(document.getElementById('invite-emails-hint')?.textContent).toContain(
    t('users.inviteWizard.many.tooMany', { count: MAX_TANDA + 1 }),
  );

  await escribir('invite-emails', 'a@acme.es b@acme.es');
  await pulsar(sigue());
  await pulsar(sigue());
  respuestas(json(429, { error: 'too_many_requests' }));
  await pulsar(t('users.inviteWizard.many.submit', { count: 2 }));
  expect(enFoco()).toContain(t('users.inviteWizard.review.heading'));
  expect(texto()).toContain(t('users.inviteWizard.many.rateLimited'));
  expect(invitadas).toBe(0);
});
