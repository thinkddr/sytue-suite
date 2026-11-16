/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * La guía de primeros pasos: una organización recién creada ve lo obligatorio
 * pendiente, una completa no ve la tarjeta, y una fuente que no contesta deja su
 * paso «sin datos», nunca «hecho». Ejecutar: `bun test`.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { expect, test } from 'bun:test';
import { render } from 'preact';
import type { OrgInsights, OrgMailDomainsState } from './admin.js';
import type { SetupSources } from './primeros-pasos.js';

if (typeof document === 'undefined') GlobalRegistrator.register();

process.env.SYTUE_OIDC_ISSUER = 'https://id.ejemplo';
process.env.SYTUE_OIDC_CLIENT_ID = 'sytue-console';
process.env.SYTUE_OIDC_CLIENT_SECRET = 'secreto-de-prueba';
process.env.SYTUE_OIDC_REDIRECT_URI = 'https://admin.ejemplo/auth/callback';
process.env.SYTUE_OIDC_POST_LOGOUT_URI = 'https://www.ejemplo/';
process.env.SYTUE_SESSION_SECRET = 'x'.repeat(32);
// Vite lo sustituye al construir; en bun `import.meta.env` es `process.env`.
process.env.BASE_URL = '/';

const { buildSetupSteps, setupComplete, conPlazo } = await import('./primeros-pasos.js');
const { SetupGuideCard } = await import('../components/SetupGuideCard.js');

const insights = (o: Partial<OrgInsights> = {}): OrgInsights => ({
  without_mfa: 0,
  suspended: 0,
  unverified_email: 0,
  invitations_expired: 0,
  invitations_pending: 0,
  ...o,
});

const sinDominio: OrgMailDomainsState = {
  items: [],
  mail_domain: 'acme.sytue.eu',
  base_domain: 'acme.sytue.eu',
};

const conDominio: OrgMailDomainsState = {
  items: [
    {
      id: 'd1',
      host: 'acme.com',
      record_name: '_sytue.acme.com',
      record_value: 'v',
      verified: true,
      primary: true,
    },
  ],
  mail_domain: 'acme.com',
  base_domain: 'acme.sytue.eu',
};

const nueva: SetupSources = {
  mailDomains: { live: true, data: sinDominio },
  insights: { live: true, data: insights({ without_mfa: 1 }) },
  users: { live: true, data: 1 },
  ssoProviders: { live: true, data: 0 },
  apps: { live: true, data: 0 },
};

const completa: SetupSources = {
  mailDomains: { live: true, data: conDominio },
  insights: { live: true, data: insights() },
  users: { live: true, data: 12 },
  ssoProviders: { live: true, data: 0 },
  apps: { live: true, data: 0 },
};

const estados = (s: SetupSources) =>
  Object.fromEntries(buildSetupSteps(s).map((p) => [p.id, p.status]));

test('una organización recién creada tiene lo obligatorio pendiente', () => {
  expect(estados(nueva)).toEqual({
    domain: 'pending',
    mail: 'pending',
    invite: 'pending',
    mfa: 'pending',
    sso: 'optional',
    apps: 'optional',
  });
  expect(setupComplete(buildSetupSteps(nueva))).toBe(false);
});

test('una organización completa da la guía por terminada aunque falten opcionales', () => {
  const pasos = buildSetupSteps(completa);
  expect(pasos.filter((p) => p.required).every((p) => p.status === 'done')).toBe(true);
  expect(setupComplete(pasos)).toBe(true);
});

test('un dominio verificado que no es el principal no da el correo por hecho', () => {
  const item = { ...conDominio.items[0]!, primary: false };
  const s = estados({
    ...completa,
    mailDomains: {
      live: true,
      data: { ...conDominio, items: [item], mail_domain: 'acme.sytue.eu' },
    },
  });
  expect(s.domain).toBe('done');
  expect(s.mail).toBe('pending');
});

test('las invitaciones pendientes cuentan como invitar hecho', () => {
  expect(
    estados({ ...nueva, insights: { live: true, data: insights({ invitations_pending: 3 }) } })
      .invite,
  ).toBe('done');
});

test('una fuente caída deja su paso sin datos, nunca hecho', () => {
  const s = estados({
    mailDomains: { live: false, data: null },
    insights: { live: false, data: insights() },
    users: { live: false, data: 40 },
    ssoProviders: { live: false, data: 0 },
    apps: { live: false, data: 0 },
  });
  expect(s).toEqual({
    domain: 'unknown',
    mail: 'unknown',
    invite: 'unknown',
    mfa: 'unknown',
    sso: 'unknown',
    apps: 'unknown',
  });
  // Y con los obligatorios «sin datos» la tarjeta sigue a la vista.
  expect(
    setupComplete(buildSetupSteps({ ...completa, insights: { live: false, data: insights() } })),
  ).toBe(false);
});

test('conPlazo corta una fuente colgada con su respaldo sin datos', async () => {
  const colgada = new Promise<{ live: boolean }>(() => {});
  expect(await conPlazo(colgada, 5, { live: false })).toEqual({ live: false });
  expect(await conPlazo(Promise.reject(new Error('x')), 50, { live: false })).toEqual({
    live: false,
  });
});

test('la tarjeta pinta los pasos y desaparece al completarse', () => {
  const host = document.createElement('div');
  render(<SetupGuideCard locale="es" steps={buildSetupSteps(nueva)} />, host);
  expect(host.querySelector('[data-testid="setup-guide"]')).not.toBeNull();
  expect(host.querySelector('[data-step="domain"]')?.getAttribute('data-status')).toBe('pending');
  expect(host.querySelector('[data-step="domain"] a')?.getAttribute('href')).toBe('#dominio');
  expect(host.textContent).toContain('0 de 4 hechos');

  render(<SetupGuideCard locale="es" steps={buildSetupSteps(completa)} />, host);
  expect(host.querySelector('[data-testid="setup-guide"]')).toBeNull();
});
