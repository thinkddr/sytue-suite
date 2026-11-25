/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Alta manual de un SP SAML desde la consola: los fallos se dicen en el campo ANTES de
 * enviar (la clave privada pegada en lugar del certificado, sobre todo), el alta llega
 * a la BFF con la forma de Sytue ID, el paso final enseña lo que hay que pegar en la
 * app, y un 409 devuelve al paso del entity ID. Ejecutar: `bun test`.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterEach, expect, test } from 'bun:test';
import { render } from 'preact';
import { act } from 'preact/test-utils';
import type { SamlSp } from '../lib/types.js';

if (typeof document === 'undefined') GlobalRegistrator.register();

const { default: RegisterSamlSpWizard, pasoDelRechazo } = await import('./RegisterSamlSpWizard.js');
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

let creados: SamlSp[] = [];
async function montar() {
  creados = [];
  respuestas();
  host = document.createElement('div');
  document.body.appendChild(host);
  await act(async () =>
    render(
      <RegisterSamlSpWizard
        t={t}
        onCreated={(sp) => creados.push(sp)}
        onCancel={() => {}}
        onDone={() => {}}
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
  await act(async () => {
    b.click();
    await tick();
  });
}
async function escribir(id: string, valor: string) {
  const el = document.getElementById(id) as HTMLInputElement | HTMLTextAreaElement | null;
  if (!el) throw new Error(`no hay campo «${id}»`);
  await act(async () => {
    el.value = valor;
    el.dispatchEvent(new Event('input', { bubbles: true }));
    el.dispatchEvent(new Event('blur'));
  });
}
const texto = () => document.body.textContent ?? '';

const SP: SamlSp = {
  id: 'sp1',
  name: 'Gastos',
  entityId: 'urn:gastos',
  acsUrl: 'https://gastos.acme.es/acs',
  nameIdFormat: 'urn:oasis:names:tc:SAML:1.1:nameid-format:emailAddress',
  idpInitiated: false,
  enabled: true,
  configured: true,
  createdAt: '2026-11-25T10:00:00Z',
  idpEntityId: 'https://id.sytue.eu/saml/idp/acme',
  idpMetadataUrl: 'https://id.sytue.eu/saml/idp/acme/metadata',
  idpSsoUrl: 'https://id.sytue.eu/saml/idp/sso',
  idpSloUrl: 'https://id.sytue.eu/saml/idp/slo',
  initiateUrl: 'https://id.sytue.eu/saml/idp/initiate/sp1',
};

async function rellenarApp() {
  await escribir('saml-name', 'Gastos');
  await escribir('saml-entity', 'urn:gastos');
  await escribir('saml-acs', 'https://gastos.acme.es/acs');
}

test('el ACS en http no deja avanzar y lo dice en el campo', async () => {
  await montar();
  await escribir('saml-name', 'Gastos');
  await escribir('saml-entity', 'urn:gastos');
  await escribir('saml-acs', 'http://gastos.acme.es/acs');
  await pulsar(sigue());
  expect(texto()).toContain(t('wizard.saml.err.acsInsecure'));
  expect(texto()).toContain(t('wizard.saml.app.heading'));
  expect(llamadas).toHaveLength(0);
});

test('una clave privada en el certificado se para en el navegador y no se envía', async () => {
  await montar();
  await rellenarApp();
  await pulsar(sigue());
  expect(texto()).toContain(t('wizard.saml.identity.heading'));
  await escribir('saml-cert', '-----BEGIN PRIVATE KEY-----\nMIGH\n-----END PRIVATE KEY-----');
  await pulsar(sigue());
  expect(texto()).toContain(t('wizard.saml.err.certPrivateKey'));
  expect(texto()).toContain(t('wizard.saml.identity.heading'));
  expect(llamadas).toHaveLength(0);
});

test('el alta llega a la BFF con la forma de Sytue ID y el final enseña qué pegar', async () => {
  await montar();
  await rellenarApp();
  await pulsar(sigue());
  await escribir('saml-attr-groups', 'memberOf');
  await pulsar(sigue());
  expect(texto()).toContain(t('wizard.saml.review.heading'));
  respuestas(json(201, SP));
  await pulsar(t('wizard.saml.review.submit'));
  expect(llamadas).toEqual([
    {
      url: '/api/admin/apps/saml-sps',
      method: 'POST',
      body: {
        name: 'Gastos',
        entity_id: 'urn:gastos',
        acs_url: 'https://gastos.acme.es/acs',
        cert_pem: '',
        name_id_format: 'urn:oasis:names:tc:SAML:1.1:nameid-format:emailAddress',
        attribute_mapping: { groups: 'memberOf' },
        idp_initiated: false,
        relay_state_default: '',
        enabled: true,
      },
    },
  ]);
  expect(creados.map((s) => s.id)).toEqual(['sp1']);
  expect(texto()).toContain(t('wizard.saml.done.heading', { name: 'Gastos' }));
  expect(texto()).toContain(SP.idpMetadataUrl);
  expect(texto()).toContain(SP.idpSsoUrl);
  // Sin SSO iniciado por el IdP, la URL de inicio no se ofrece.
  expect(texto()).not.toContain(SP.initiateUrl);
});

test('un 409 vuelve al paso del entity ID con el motivo pegado al campo', async () => {
  await montar();
  await rellenarApp();
  await pulsar(sigue());
  await pulsar(sigue());
  respuestas(json(409, { error: 'upstream', code: 'conflict' }));
  await pulsar(t('wizard.saml.review.submit'));
  expect(texto()).toContain(t('wizard.saml.app.heading'));
  expect(texto()).toContain(t('wizard.saml.fix.conflict'));
  expect(creados).toHaveLength(0);
});

test('cada rechazo lleva al paso que lo corrige', () => {
  expect(pasoDelRechazo(409, 'conflict')).toBe('app');
  expect(pasoDelRechazo(400, 'acsInsecure')).toBe('app');
  expect(pasoDelRechazo(400, 'certNotPem')).toBe('identity');
  expect(pasoDelRechazo(400, undefined)).toBe('identity');
  expect(pasoDelRechazo(502, undefined)).toBeNull();
});
