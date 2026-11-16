/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Sitio web, relaciones y aniversario: la ficha los enseña (el sitio como enlace que
 * se abre aparte) y el formulario los edita y los manda, con el «https://» puesto.
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
  'UID:ada',
  'FN:Ada',
  'URL;TYPE=work:https://acme.example',
  'RELATED;TYPE=spouse;VALUE=text:William',
  'ANNIVERSARY:2012-06-09',
  'END:VCARD',
].join('\r\n');
const ada: Contact = {
  href: '/ada.vcf',
  uid: 'ada',
  fullName: 'Ada',
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

async function hasta(listo: () => boolean): Promise<void> {
  for (let i = 0; i < 200 && !listo(); i++) await new Promise((r) => setTimeout(r, 10));
}

test('la ficha enseña el sitio web, el aniversario y las personas relacionadas', async () => {
  const host = montar(<ContactDetail t={t} locale="es" contact={ada} groups={[]} readOnly />);
  await Promise.resolve();
  const rotulos = [...host.querySelectorAll('dt')].map((d) => d.textContent?.trim());
  expect(rotulos).toEqual(
    expect.arrayContaining(['field.website', 'field.anniversary', 'field.related']),
  );
  const enlace = host.querySelector<HTMLAnchorElement>('a[href="https://acme.example"]');
  expect(enlace?.target).toBe('_blank');
  expect(enlace?.rel).toContain('noopener');
  expect(host.textContent).toContain('9 de junio de 2012');
  expect(host.textContent).toContain('William');
  expect(host.textContent).toContain('relation.spouse');
});

test('el formulario los edita y los manda, con el esquema completado', async () => {
  const enviados: Record<string, unknown>[] = [];
  globalThis.fetch = (async (_url: string, init?: RequestInit) => {
    enviados.push(JSON.parse(String(init?.body)) as Record<string, unknown>);
    return new Response(JSON.stringify({ ok: true, href: '/ada.vcf', uid: 'ada' }));
  }) as typeof fetch;
  montar(
    <ContactModal
      t={t}
      open
      addressBookUrl="/libreta/"
      editing={{ ...ada, etag: 'e1' }}
      onClose={() => {}}
      onSaved={() => {}}
    />,
  );
  const campo = (id: string): HTMLInputElement | undefined =>
    [...document.querySelectorAll<HTMLInputElement>(`#${id}`)].at(-1);
  await hasta(() => campo('c-website-0')?.value === 'https://acme.example');
  expect(campo('c-related-0')?.value).toBe('William');
  expect(campo('c-anniversary')?.value).toBe('2012-06-09');
  const web = campo('c-website-0')!;
  web.value = 'ada.example';
  web.dispatchEvent(new Event('input', { bubbles: true }));
  await new Promise((r) => setTimeout(r, 0));
  [...document.querySelectorAll('button')]
    .filter((b) => b.textContent?.trim() === 'common.save')
    .at(-1)
    ?.click();
  await hasta(() => enviados.length > 0);
  const input = (enviados[0]?.['contactInput'] ?? {}) as Record<string, unknown>;
  expect(input['urls']).toEqual([{ value: 'https://ada.example', type: 'work' }]);
  expect(input['related']).toEqual([{ value: 'William', type: 'spouse' }]);
  expect(input['anniversary']).toBe('2012-06-09');
});
