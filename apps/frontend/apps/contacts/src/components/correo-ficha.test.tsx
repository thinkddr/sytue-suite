/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * «Enviar correo» de la ficha y «Enviar correo a todos» de la selección eligen el MISMO
 * correo para cada contacto: los dos leen `correoPrincipal` (`client/seleccion.ts`), y no
 * cada uno su copia de la regla del principal, que acabaría divergiendo.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterEach, expect, test } from 'bun:test';
import { render } from 'preact';
import { I18nProvider } from '@sytue/i18n/preact';
import { ToastProvider } from '@sytue/ui';
import type { Contact } from '../client/contactsApi.js';
import { correosDe } from '../client/seleccion.js';
import { ContactDetail } from './ContactDetail.js';

if (typeof document === 'undefined') GlobalRegistrator.register();

const t = ((k: string) => k) as never;
const hosts: HTMLElement[] = [];

afterEach(() => {
  for (const h of hosts.splice(0)) {
    render(null, h);
    h.remove();
  }
});

function contacto(uid: string, vcardEmails: string[], emails: string[] = []): Contact {
  const vcard = ['BEGIN:VCARD', 'VERSION:4.0', `FN:${uid}`, ...vcardEmails, 'END:VCARD'].join(
    '\r\n',
  );
  return { href: `/${uid}.vcf`, uid, fullName: uid, emails, phones: [], vcard };
}

/** El destinatario con el que «Enviar correo» de la ficha abre Sytue Mail. */
async function correoDeLaFicha(c: Contact): Promise<string | null> {
  const host = document.createElement('div');
  document.body.appendChild(host);
  hosts.push(host);
  render(
    <I18nProvider locale="es">
      <ToastProvider>
        <ContactDetail t={t} locale="es" contact={c} groups={[]} readOnly />
      </ToastProvider>
    </I18nProvider>,
    host,
  );
  await Promise.resolve();
  const boton = [...host.querySelectorAll('button')].find(
    (b) => b.textContent?.trim() === 'detail.sendEmail',
  );
  if (!boton) return null;
  const abiertas: string[] = [];
  const abrir = window.open;
  window.open = ((u: string) => {
    abiertas.push(u);
    return null;
  }) as typeof window.open;
  try {
    boton.click();
  } finally {
    window.open = abrir;
  }
  return new URL(abiertas[0] ?? '', 'https://x.invalid').searchParams.get('to');
}

test('la ficha y «Enviar correo a todos» eligen el mismo correo', async () => {
  for (const c of [
    contacto('pref', ['EMAIL;TYPE=home:ana@casa.eu', 'EMAIL;TYPE=work;PREF=1:ana@obra.eu']),
    contacto('primero', ['EMAIL;TYPE=home:luis@casa.eu', 'EMAIL;TYPE=work:luis@obra.eu']),
    contacto('sinVcard', [], ['eva@x.eu']),
  ]) {
    expect({ uid: c.uid, correo: await correoDeLaFicha(c) }).toEqual({
      uid: c.uid,
      correo: correosDe([c]).correos[0] ?? null,
    });
  }
});
