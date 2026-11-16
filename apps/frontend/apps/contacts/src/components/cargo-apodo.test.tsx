/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Cargo y apodo: la ficha pone «Cargo · Empresa» bajo el nombre y el apodo como un
 * campo más, y el formulario los enseña, los deja cambiar y los manda al guardar; lo
 * que no se toca no viaja, para que la vCard conserve la línea original.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterEach, expect, test } from 'bun:test';
import { render } from 'preact';
import { I18nProvider } from '@sytue/i18n/preact';
import { ToastProvider } from '@sytue/ui';
import type { Contact } from '../client/contactsApi.js';
import { ContactDetail } from './ContactDetail.js';
import { ContactModal } from './ContactModal.js';

if (typeof document === 'undefined') GlobalRegistrator.register();

const t = ((k: string) => k) as never;
const vcard = [
  'BEGIN:VCARD',
  'VERSION:4.0',
  'UID:ana',
  'FN:Ana García',
  'N:García;Ana;;;',
  'ORG:Acme',
  'TITLE:Directora',
  'NICKNAME;LANGUAGE=es:Anita',
  'END:VCARD',
].join('\r\n');
const ana: Contact = {
  href: '/ana.vcf',
  uid: 'ana',
  fullName: 'Ana García',
  organization: 'Acme',
  emails: [],
  phones: [],
  vcard,
};
const hosts: HTMLElement[] = [];
const fetchReal = globalThis.fetch;

afterEach(() => {
  globalThis.fetch = fetchReal;
  for (const h of hosts.splice(0)) {
    render(null, h);
    h.remove();
  }
});

function montar(nodo: preact.ComponentChild): HTMLElement {
  const host = document.createElement('div');
  document.body.appendChild(host);
  hosts.push(host);
  render(
    <I18nProvider locale="es">
      <ToastProvider>{nodo}</ToastProvider>
    </I18nProvider>,
    host,
  );
  return host;
}

const espera = (): Promise<void> => new Promise((r) => setTimeout(r, 0));

/** Espera (hasta 2 s) a que `listo` se cumpla: los efectos del modal no tienen un tic fijo. */
async function hasta(listo: () => boolean): Promise<void> {
  for (let i = 0; i < 200 && !listo(); i++) await new Promise((r) => setTimeout(r, 10));
}

test('la ficha pone «Cargo · Empresa» bajo el nombre y el apodo como campo', async () => {
  const host = montar(<ContactDetail t={t} locale="es" contact={ana} groups={[]} readOnly />);
  await Promise.resolve();
  expect(host.querySelector('header h2 + p')?.textContent).toBe('Directora · Acme');
  const rotulos = [...host.querySelectorAll('dt')].map((d) => d.textContent?.trim());
  expect(rotulos).toContain('field.nickname');
  expect(host.textContent).toContain('Anita');
});

test('sin cargo ni apodo, la ficha enseña solo la empresa y no pinta el campo vacío', async () => {
  const sin: Contact = { ...ana, vcard: vcard.replace(/\r\nTITLE[^\r]*|\r\nNICKNAME[^\r]*/g, '') };
  const host = montar(<ContactDetail t={t} locale="es" contact={sin} groups={[]} readOnly />);
  await Promise.resolve();
  expect(host.querySelector('header h2 + p')?.textContent).toBe('Acme');
  expect(host.textContent).not.toContain('field.nickname');
});

/** Abre el formulario de `ana`, deja que `cambiar` toque los campos y guarda. */
async function guardarCon(
  cambiar: (campo: (id: string) => HTMLInputElement) => void,
): Promise<Record<string, unknown>> {
  const enviados: Record<string, unknown>[] = [];
  globalThis.fetch = (async (_url: string, init?: RequestInit) => {
    enviados.push(JSON.parse(String(init?.body)) as Record<string, unknown>);
    return new Response(JSON.stringify({ ok: true, href: '/ana.vcf', uid: 'ana' }));
  }) as typeof fetch;
  montar(
    <ContactModal
      t={t}
      open
      addressBookUrl="/libreta/"
      editing={{ ...ana, etag: 'e1' }}
      onClose={() => {}}
      onSaved={() => {}}
    />,
  );
  // El último con ese id: el del modal recién abierto, aunque otro test haya dejado uno.
  const campo = (id: string): HTMLInputElement => {
    const el = [...document.querySelectorAll<HTMLInputElement>(`#${id}`)].at(-1);
    if (!el) throw new Error(`no hay #${id}`);
    return el;
  };
  await hasta(() => document.querySelector(`#c-title`) !== null && campo('c-title').value !== '');
  expect(campo('c-title').value).toBe('Directora');
  expect(campo('c-nickname').value).toBe('Anita');
  cambiar(campo);
  await espera();
  const guardar = [...document.querySelectorAll('button')]
    .filter((b) => b.textContent?.trim() === 'common.save')
    .at(-1);
  guardar?.click();
  await hasta(() => enviados.length > 0);
  return (enviados[0]?.['contactInput'] ?? {}) as Record<string, unknown>;
}

function escribir(el: HTMLInputElement, valor: string): void {
  el.value = valor;
  el.dispatchEvent(new Event('input', { bubbles: true }));
}

test('el formulario manda el cargo y el apodo que se cambian, y vacíos los borra', async () => {
  const cambiado = await guardarCon((campo) => {
    escribir(campo('c-title'), '  Directora general ');
    escribir(campo('c-nickname'), '');
  });
  expect(cambiado['title']).toBe('Directora general');
  expect(cambiado['nickname']).toBe('');
});

test('si no se tocan, el formulario no los manda (la vCard conserva su línea)', async () => {
  const igual = await guardarCon(() => {});
  expect('title' in igual).toBe(false);
  expect('nickname' in igual).toBe(false);
});

test('la ficha enseña un cumpleaños sin año (`--MM-DD`) como «15 de febrero»', async () => {
  const conBday = (bday: string): Contact => ({
    ...ana,
    vcard: vcard.replace('END:VCARD', `BDAY:${bday}\r\nEND:VCARD`),
  });
  const host = montar(
    <ContactDetail t={t} locale="es" contact={conBday('--0215')} groups={[]} readOnly />,
  );
  await Promise.resolve();
  const rotulos = [...host.querySelectorAll('dt')].map((d) => d.textContent?.trim());
  expect(rotulos).toContain('field.birthday');
  expect(host.textContent).toContain('15 de febrero');
  expect(host.textContent).not.toContain('15 de febrero de');
});
