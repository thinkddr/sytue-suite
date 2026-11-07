/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * El «De» del redactor (fase B, punto 6): lista Sytue, sus alias y las cuentas
 * externas que envían, y una externa sale por `/external-mail/{id}/send` con
 * `expected_address`, no por el envío de Sytue. Lo mismo que
 * `TestElDeOfreceLasCuentasExternasYEnviaPorSuProveedor` en Fyne. Ejecutar: `bun test`.
 *
 * VISTO EN ROJO (2026-11-06): sin `externalAccounts` en `remitentes`, la cuenta no
 * salía en el «De»; sin la rama de `handleSend`, el correo iba a `/api/send` con la
 * dirección externa como alias; sin `!cuentaDe`, «Programar» seguía a la vista. Deshecho.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterEach, beforeEach, expect, test } from 'bun:test';
import { render } from 'preact';
import { act } from 'preact/test-utils';
import type { ExternalMailAccount } from '@sytue/api-clients';
import { en } from '../i18n/messages/en.js';

if (typeof document === 'undefined') GlobalRegistrator.register({ url: 'https://sytue.test/mail' });
else
  (window as unknown as { happyDOM?: { setURL: (url: string) => void } }).happyDOM?.setURL(
    'https://sytue.test/mail',
  );

const peticiones: { url: string; body: unknown }[] = [];
beforeEach(() => {
  globalThis.fetch = (async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = String(input instanceof Request ? input.url : input);
    peticiones.push({ url, body: init?.body ? JSON.parse(String(init.body)) : null });
    const body = url.includes('/external-mail/') ? { sent_copy_saved: true } : { id: 'd1' };
    return new Response(JSON.stringify(body), {
      status: 200,
      headers: { 'content-type': 'application/json' },
    });
  }) as unknown as typeof fetch;
});
afterEach(() => {
  render(null, document.body);
  document.body.innerHTML = '';
  peticiones.length = 0;
});

const { ComposeModal } = await import('./ComposeModal.js');
const cuenta: ExternalMailAccount = {
  instance_id: 'g1',
  provider: 'gmail',
  name: 'Gmail',
  address: 'ana@gmail.com',
  connected: true,
  capabilities: {
    read: true,
    send: true,
    actions: true,
    attachments: true,
    continuous_sync: false,
  },
};

async function esperar(): Promise<void> {
  for (let i = 0; i < 6; i++)
    await act(async () => {
      await new Promise((r) => setTimeout(r, 0));
    });
}

async function abrir(from?: string): Promise<void> {
  await act(async () => {
    render(
      <ComposeModal
        locale="en"
        open
        ownerId="ana"
        initial={{
          to: 'luis@example.com',
          subject: 'Hola',
          body: 'Qué tal',
          ...(from ? { from } : {}),
        }}
        draftMailboxId="drafts"
        fromOptions={['ana@sytue.eu', 'ventas@sytue.eu']}
        externalAccounts={[
          cuenta,
          {
            ...cuenta,
            instance_id: 'cal',
            address: 'cal@gmail.com',
            capabilities: { ...cuenta.capabilities, send: false },
          },
        ]}
        onClose={() => undefined}
      />,
      document.body,
    );
  });
  await esperar();
}

function botonPorTexto(texto: string): HTMLButtonElement | undefined {
  return [...document.querySelectorAll<HTMLButtonElement>('button')].find(
    (b) => b.textContent?.trim() === texto,
  );
}

test('el «De» lista Sytue, sus alias y las cuentas que envían', async () => {
  await abrir();
  const opciones = [...document.querySelectorAll<HTMLOptionElement>('#compose-from option')];
  expect(opciones.map((o) => o.textContent)).toEqual([
    'ana@sytue.eu',
    'ventas@sytue.eu',
    'ana@gmail.com',
  ]);
  expect(document.querySelector(`[aria-label="${en['compose.schedule']}"]`)).not.toBeNull();
});

test('desde una cuenta externa sale por su proveedor, con la dirección que se espera', async () => {
  await abrir('externa:g1');
  expect(document.querySelector<HTMLSelectElement>('#compose-from')?.value).toBe('externa:g1');
  // Programar es de la cola de Sytue.
  expect(document.querySelector(`[aria-label="${en['compose.schedule']}"]`)).toBeNull();
  await act(async () => {
    botonPorTexto(en['compose.send'])?.click();
  });
  await esperar();
  const envios = peticiones.filter((p) => p.url.includes('/send'));
  // Una sola petición de envío, y es la de la cuenta (con o sin la base de la app).
  expect(envios).toHaveLength(1);
  expect(new URL(envios[0]!.url).pathname).toMatch(/(^|\/)api\/external-mail\/g1\/send$/);
  expect(envios[0]?.body).toMatchObject({
    expected_address: 'ana@gmail.com',
    to: [{ email: 'luis@example.com' }],
    subject: 'Hola',
  });
});
