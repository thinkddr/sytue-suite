// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
//
// Llama al BFF de envío DESDE la página ya autenticada y enseña la respuesta
// entera. El reproductor por interfaz dice «no se ha podido enviar» y un 502;
// esto dice POR QUÉ, que es lo que hace falta para arreglarlo.
//
//   SYTUE_AUDIT_EMAIL=... SYTUE_AUDIT_PASS=... bun run repro-enviar-api.mjs
import { chromium } from 'playwright';
import { activarOrganizacion, codigoTOTP } from './tests/helpers.ts';

const DOMINIO = process.env.SYTUE_BASE_DOMAIN || 'sytue.eu';
const IDP = `id.${DOMINIO}`;
const CRED = {
  email: process.env.SYTUE_AUDIT_EMAIL || '',
  pass: process.env.SYTUE_AUDIT_PASS || '',
  totp: process.env.SYTUE_AUDIT_TOTP || '',
};
if (!CRED.email || !CRED.pass) {
  console.error('Faltan SYTUE_AUDIT_EMAIL / SYTUE_AUDIT_PASS.');
  process.exit(2);
}

const browser = await chromium.launch();
const page = await (await browser.newContext({ viewport: { width: 1280, height: 900 } })).newPage();

await page.goto(`https://mail.${DOMINIO}/`, { waitUntil: 'domcontentloaded' });
await page.waitForTimeout(2500);
if (new URL(page.url()).hostname === IDP) {
  await page.fill('input[name=email]', CRED.email);
  await page.fill('input[name=password]', CRED.pass);
  await page.click('button[type=submit]');
  const totp = page.locator('input[name=code], input[name=totp], input[autocomplete="one-time-code"]');
  if (await totp.first().waitFor({ timeout: 5000 }).then(() => true, () => false)) {
    await totp.first().fill(codigoTOTP(CRED.totp));
    await page.click('button[type=submit]');
  }
  await page.waitForURL((u) => u.hostname !== IDP, { timeout: 60_000 });
  await activarOrganizacion(page);
  await page.waitForTimeout(2000);
}
console.log('sesión en', page.url());

// Las rutas que interesan del BFF, cada una con su cuerpo tal cual.
const sondas = [
  ['GET', '/api/mailboxes', null],
  ['GET', '/api/aliases', null],
  [
    'POST',
    '/api/send',
    {
      to: [{ email: CRED.email }],
      subject: `sonda ${process.env.SYTUE_MARCA || ''}`,
      textBody: 'Sonda del camino de envío.',
    },
  ],
  // La misma sonda pero declarando como remitente la dirección PRIMARIA del
  // buzón. Si esta pasa y la de arriba no, el fallo es que el BFF toma el
  // remitente de la sesión de Sytue ID y no del buzón.
  [
    'POST',
    '/api/send',
    {
      from: process.env.SYTUE_PRIMARIA || '',
      to: [{ email: CRED.email }],
      subject: `sonda primaria ${process.env.SYTUE_MARCA || ''}`,
      textBody: 'Sonda con el remitente del buzón.',
    },
  ],
];

for (const [metodo, ruta, cuerpo] of sondas) {
  const r = await page.evaluate(
    async ([m, u, b]) => {
      try {
        const res = await fetch(u, {
          method: m,
          ...(b
            ? { headers: { 'content-type': 'application/json' }, body: JSON.stringify(b) }
            : {}),
        });
        return { estado: res.status, texto: (await res.text()).slice(0, 1200) };
      } catch (e) {
        return { estado: 0, texto: `EXCEPCIÓN: ${String(e)}` };
      }
    },
    [metodo, ruta, cuerpo],
  );
  console.log(`\n--- ${metodo} ${ruta} → ${r.estado}`);
  console.log(r.texto);
}

await browser.close();
