// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
//
// Reproduce el envío de un correo CONTRA PRODUCCIÓN y cuenta qué pasa de verdad.
//
// Existe porque «no se pueden enviar correos» no dice dónde se rompe: el botón
// puede no llamar a nada, el BFF puede devolver 502, el servicio Go puede
// aceptarlo y no entregarlo, o puede entregarlo y el correo no llegar. Este
// script mira las TRES capas —consola, red y la respuesta del BFF— y las imprime.
//
// El correo se envía a la PROPIA cuenta de pruebas: escribe en producción, pero
// solo en su propio buzón.
//
//   SYTUE_AUDIT_EMAIL=... SYTUE_AUDIT_PASS=... bun run repro-enviar-correo.mjs
//
// Lo normal es lanzarlo por `auditar-prod.sh`, que saca la credencial del
// llavero para que no quede escrita en el historial del shell.
import { chromium } from 'playwright';
import { activarOrganizacion, codigoTOTP } from './tests/helpers.ts';

const DOMINIO = process.env.SYTUE_BASE_DOMAIN || 'sytue.eu';
const IDP = `id.${DOMINIO}`;
const CORREO = `https://mail.${DOMINIO}/`;
const CRED = {
  email: process.env.SYTUE_AUDIT_EMAIL || '',
  pass: process.env.SYTUE_AUDIT_PASS || '',
  totp: process.env.SYTUE_AUDIT_TOTP || '',
};
if (!CRED.email || !CRED.pass) {
  console.error('Faltan SYTUE_AUDIT_EMAIL / SYTUE_AUDIT_PASS.');
  process.exit(2);
}

const MARCA = `repro-envio ${process.env.SYTUE_MARCA || 'sin-marca'}`;

const browser = await chromium.launch();
const ctx = await browser.newContext({ viewport: { width: 1440, height: 950 } });
const page = await ctx.newPage();

// --- lo que se quiere ver -------------------------------------------------
const consola = [];
const red = [];
page.on('console', (m) => {
  if (m.type() === 'error' || m.type() === 'warning') consola.push(`[${m.type()}] ${m.text()}`);
});
page.on('pageerror', (e) => consola.push(`[pageerror] ${e.message}`));
page.on('response', async (r) => {
  const u = new URL(r.url());
  if (!/\/api\//.test(u.pathname)) return;
  const entrada = { metodo: r.request().method(), ruta: u.pathname, estado: r.status() };
  if (r.status() >= 400 || /send|submission|draft/i.test(u.pathname)) {
    entrada.cuerpo = await r.text().catch(() => '<ilegible>');
    if (entrada.cuerpo.length > 600) entrada.cuerpo = `${entrada.cuerpo.slice(0, 600)}…`;
  }
  red.push(entrada);
});

async function login() {
  await page.goto(CORREO, { waitUntil: 'domcontentloaded' });
  await page.waitForTimeout(2500);
  if (new URL(page.url()).hostname !== IDP) return;
  await page.fill('input[name=email]', CRED.email);
  await page.fill('input[name=password]', CRED.pass);
  await page.click('button[type=submit]');
  const totp = page.locator('input[name=code], input[name=totp], input[autocomplete="one-time-code"]');
  if (await totp.first().waitFor({ timeout: 5000 }).then(() => true, () => false)) {
    if (!CRED.totp) throw new Error('la cuenta pide segundo factor y no hay TOTP');
    await totp.first().fill(codigoTOTP(CRED.totp));
    await page.click('button[type=submit]');
  }
  await page.waitForURL((u) => u.hostname !== IDP, { timeout: 60_000 });
  await activarOrganizacion(page);
  await page.waitForTimeout(2500);
}

function volcar(titulo, lista) {
  console.log(`\n=== ${titulo} (${lista.length}) ===`);
  for (const x of lista) console.log(typeof x === 'string' ? x : JSON.stringify(x));
}

try {
  await login();
  console.log('sesión iniciada en', page.url());

  await page.getByRole('button', { name: /Redactar|Compose|Nuevo mensaje|New message/i }).first().click();
  const dialogo = page.getByRole('dialog').first();
  await dialogo.waitFor({ timeout: 20_000 });
  await page.waitForTimeout(1200);

  // Destinatario: la propia cuenta. Escribir en producción, sí; molestar a
  // alguien, no.
  const para = dialogo.locator('input[type=email], input[name=to], input[placeholder*="ara"], input[placeholder*="o"]').first();
  await para.click();
  await para.fill(CRED.email);
  await page.keyboard.press('Enter');

  const asunto = dialogo.locator('input[name=subject], input[placeholder*="sunto"], input[placeholder*="ubject"]').first();
  await asunto.fill(MARCA);

  // El cuerpo es un contenteditable (el editor enriquecido), no un textarea.
  const cuerpo = dialogo.locator('[contenteditable="true"], textarea').first();
  await cuerpo.click();
  await page.keyboard.type('Prueba de envío automatizada. Si esto llega, el camino funciona.');

  await page.screenshot({ path: '/tmp/envio-antes.png' });

  const enviar = dialogo.getByRole('button', { name: /^Enviar$|^Send$/i }).first();
  console.log('botón Enviar visible:', await enviar.isVisible().catch(() => false));
  console.log('botón Enviar activo:', await enviar.isEnabled().catch(() => false));
  await enviar.click();
  await page.waitForTimeout(6000);
  await page.screenshot({ path: '/tmp/envio-despues.png' });

  console.log('\n¿sigue abierto el compositor?:', await dialogo.isVisible().catch(() => false));
  const avisos = await page.evaluate(() =>
    [...document.querySelectorAll('[role="status"], [role="alert"], [aria-live]')]
      .map((n) => n.textContent?.trim())
      .filter((t) => t),
  );
  volcar('avisos en pantalla', avisos);
  volcar('consola', consola);
  volcar('red /api', red);
} catch (e) {
  console.error('\nFALLÓ EL GUION:', e.message);
  await page.screenshot({ path: '/tmp/envio-error.png' }).catch(() => {});
  volcar('consola', consola);
  volcar('red /api', red);
  process.exitCode = 1;
} finally {
  await browser.close();
}
