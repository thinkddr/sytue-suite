/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Copiar desde la ficha (`CopyValue`): cada correo, cada teléfono y cada dirección
 * llevan su botón, con el valor en el nombre accesible, que copia ESE valor —la
 * dirección entera, en líneas— y lo dice.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterEach, expect, test } from 'bun:test';
import { render } from 'preact';
import { I18nProvider } from '@sytue/i18n/preact';
import { ToastProvider } from '@sytue/ui';
import type { Contact } from '../client/contactsApi.js';
import { ContactDetail } from './ContactDetail.js';

if (typeof document === 'undefined') GlobalRegistrator.register();

// La clave con el valor, para distinguir los botones entre sí.
const t = ((k: string, v?: { value?: string }) => (v?.value ? `${k}:${v.value}` : k)) as never;
const ana: Contact = {
  href: '/a.vcf',
  uid: 'a',
  fullName: 'Ana',
  emails: ['ana@x.eu'],
  phones: ['+34 600 000 000'],
  vcard: [
    'BEGIN:VCARD',
    'VERSION:4.0',
    'FN:Ana',
    'EMAIL;TYPE=work:ana@x.eu',
    'EMAIL;TYPE=home:ana@casa.eu',
    'TEL;TYPE=cell:+34 600 000 000',
    'ADR;TYPE=home:;;Calle 1;Madrid;;28001;España',
    'END:VCARD',
  ].join('\r\n'),
};
const hosts: HTMLElement[] = [];

afterEach(() => {
  for (const h of hosts.splice(0)) {
    render(null, h);
    h.remove();
  }
});

async function montar(niega = false): Promise<{ host: HTMLElement; copiado: string[] }> {
  const copiado: string[] = [];
  Object.defineProperty(navigator, 'clipboard', {
    configurable: true,
    value: {
      writeText: async (s: string) => {
        if (niega) throw new DOMException('denegado', 'NotAllowedError');
        copiado.push(s);
      },
    },
  });
  const host = document.createElement('div');
  document.body.appendChild(host);
  hosts.push(host);
  render(
    <I18nProvider locale="es">
      <ToastProvider>
        <ContactDetail t={t} locale="es" contact={ana} groups={[]} readOnly />
      </ToastProvider>
    </I18nProvider>,
    host,
  );
  await Promise.resolve();
  return { host, copiado };
}

function boton(host: HTMLElement, label: string): HTMLButtonElement {
  const b = host.querySelector<HTMLButtonElement>(`button[aria-label="${label}"]`);
  if (!b) throw new Error(`no hay botón «${label}»`);
  return b;
}

test('cada correo, teléfono y dirección se copia con un clic', async () => {
  const { host, copiado } = await montar();
  for (const label of [
    'detail.copyValue:ana@x.eu',
    'detail.copyValue:ana@casa.eu',
    'detail.copyValue:+34 600 000 000',
    'detail.copyValue:Calle 1',
  ])
    boton(host, label).click();
  await new Promise((r) => setTimeout(r, 0));
  expect(copiado).toEqual([
    'ana@x.eu',
    'ana@casa.eu',
    '+34 600 000 000',
    'Calle 1\n28001 Madrid\nEspaña',
  ]);
});

test('el aviso dice qué se ha copiado', async () => {
  const { host } = await montar();
  boton(host, 'detail.copyValue:+34 600 000 000').click();
  await new Promise((r) => setTimeout(r, 0));
  expect(document.body.textContent).toContain('detail.phoneCopied');
  boton(host, 'detail.copyValue:Calle 1').click();
  await new Promise((r) => setTimeout(r, 0));
  expect(document.body.textContent).toContain('detail.addressCopied');
});

test('si el navegador niega el portapapeles, se dice y no se da por copiado', async () => {
  const { host, copiado } = await montar(true);
  boton(host, 'detail.copyValue:ana@x.eu').click();
  await new Promise((r) => setTimeout(r, 0));
  expect(copiado).toEqual([]);
  expect(document.body.textContent).toContain('detail.copyFailed');
  expect(document.body.textContent).not.toContain('detail.emailCopied');
});
