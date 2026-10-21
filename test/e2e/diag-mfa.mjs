// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
//
// Por qué el IdP rechaza el segundo factor SOLO desde el navegador.
//
// El mismo secreto, en el mismo flujo y con el mismo reloj, entra por `curl` y no
// por Playwright. Y no se puede distinguir desde fuera: `handleMFAVerify` contesta
// el MISMO mensaje para tres causas —código incorrecto, continuación ausente o ya
// consumida, y navegador distinto— así que el log solo dice «rechazado».
//
// Esto captura lo que ninguna de las dos partes enseña: la petición REAL que sale
// del navegador (campos y cookies, sin valores), la respuesta del IdP y el texto
// que acaba pintando. Con eso se sabe cuál de las tres causas es.
//
// Se ejecuta en la nube, con `reproductor=diag-mfa.mjs`. NUNCA imprime la
// contraseña, la semilla ni el código: solo si están presentes y qué longitud
// tienen, que es lo único que hace falta para descartarlos.
import { chromium } from 'playwright';
import { codigoTOTP } from './tests/helpers.ts';

const DOMINIO = process.env.SYTUE_BASE_DOMAIN || 'sytue.eu';
const IDP = `id.${DOMINIO}`;
const CRED = {
  email: process.env.SYTUE_AUDIT_EMAIL || '',
  pass: process.env.SYTUE_AUDIT_PASS || '',
  totp: process.env.SYTUE_AUDIT_TOTP || '',
};
if (!CRED.email || !CRED.pass || !CRED.totp) {
  console.error('Faltan SYTUE_AUDIT_EMAIL / PASS / TOTP.');
  process.exit(2);
}

const browser = await chromium.launch();
const ctx = await browser.newContext({ viewport: { width: 1280, height: 900 } });
const page = await ctx.newPage();

/** Oculta el valor y deja lo que sirve para diagnosticar. */
const resumir = (v) => (v ? `«${v.length} caracteres»` : '(vacío)');

// Se registran las peticiones al IdP: qué campos lleva el formulario y con qué
// cookies sale. Un `code` ausente o una cookie de continuación que no viaja
// explican el rechazo sin necesidad de mirar la base de datos.
page.on('request', (req) => {
  const u = new URL(req.url());
  if (u.hostname !== IDP || req.method() !== 'POST') return;
  const datos = req.postData() ?? '';
  const campos = datos
    .split('&')
    .map((p) => p.split('=')[0])
    .filter(Boolean);
  // `headers()` OMITE las cabeceras protegidas —Cookie entre ellas— así que un
  // «cookies=[]» ahí no significa que no viajen. Hay que pedir `allHeaders()`.
  void req.allHeaders().then((h) => {
    const cookies = (h['cookie'] ?? '')
      .split(';')
      .map((c) => c.trim().split('=')[0])
      .filter(Boolean);
    console.log(`→ POST ${u.pathname}  campos=[${campos}]  cookies=[${cookies}]`);
  });
});

page.on('response', async (res) => {
  const u = new URL(res.url());
  if (u.hostname !== IDP) return;
  if (!['/login/', '/login/mfa', '/oauth2/authorize'].includes(u.pathname)) return;
  const loc = res.headers()['location'] ?? '';
  console.log(`← ${res.status()} ${u.pathname}${u.search ? '?…' : ''}  location=${loc.slice(0, 90)}`);
});

console.log(`· credenciales: email=${resumir(CRED.email)} pass=${resumir(CRED.pass)} totp=${resumir(CRED.totp)}`);
console.log(`· reloj: ${new Date().toISOString()} · paso ${Math.floor(Date.now() / 1000 / 30)}`);

await page.goto(`https://docus.${DOMINIO}/docus`, { waitUntil: 'domcontentloaded' });
await page.waitForTimeout(2000);
console.log(`· tras entrar en docs: ${page.url().slice(0, 100)}`);

await page.fill('input[name=email]', CRED.email);
await page.fill('input[name=password]', CRED.pass);
await page.click('button[type=submit]');
await page.waitForURL((u) => /[?&]step_up=1/.test(u.href), { timeout: 20_000 }).catch(() => {});
console.log(`· tras la contraseña: ${page.url().slice(0, 100)}`);

// El User-Agent de las dos peticiones: si difiere entre el paso 1 y el 2, el
// binding del token de continuación lo rechaza y el mensaje es el mismo.
const ua = await page.evaluate(() => navigator.userAgent);
console.log(`· user-agent del navegador: ${ua}`);

const campo = page.locator('input[name=code], input[autocomplete="one-time-code"]');
await campo.first().waitFor({ timeout: 30_000 });
const codigo = codigoTOTP(CRED.totp);
console.log(`· código generado con paso ${Math.floor(Date.now() / 1000 / 30)} (no se imprime)`);
await campo.first().fill(codigo);
await page.click('button[type=submit]');
// Se espera a SALIR del IdP, que es el final real del flujo. Con 4 s fijos se
// capturaba un estado intermedio y se leía como «se quedó en el login».
await page
  .waitForURL((u) => new URL(u).hostname !== IDP, { timeout: 30_000 })
  .catch(() => {});

// LAS COOKIES QUE EL NAVEGADOR GUARDÓ DE VERDAD. Es la medición que faltaba: si
// la de sesión no está aquí, el `Set-Cookie` del MFA no se aceptó y por eso
// `/oauth2/authorize` no ve sesión y devuelve al login. Solo los nombres y sus
// atributos; los valores no se imprimen.
const guardadas = await ctx.cookies();
console.log(
  `· cookies del navegador: ${guardadas
    .map((c) => `${c.name}[${c.domain}${c.path};ss=${c.sameSite};sec=${c.secure}]`)
    .join(' ') || '(ninguna)'}`,
);
console.log(`· tras el código: ${page.url().slice(0, 110)}`);
const texto = ((await page.locator('body').innerText().catch(() => '')) || '')
  .replace(/\s+/g, ' ')
  .slice(0, 400);
console.log(`· pantalla: «${texto}»`);

// LA PRUEBA QUE PARTE EL PROBLEMA EN DOS. Se repite el MISMO paso por HTTP desde
// el mismo runner y con un código nuevo: si aquí entra, el IdP y la credencial
// están bien y lo que falla es lo que el navegador manda de más o de menos.
console.log('· repitiendo el flujo por HTTP desde este mismo runner…');
const ctx2 = await browser.newContext();
const api = ctx2.request;
const r1 = await api.post(`https://${IDP}/login/`, {
  form: { email: CRED.email, password: CRED.pass, return_to: '/account' },
  maxRedirects: 0,
});
console.log(`  ← ${r1.status()} location=${(r1.headers()['location'] ?? '').slice(0, 60)}`);
const r2 = await api.post(`https://${IDP}/login/mfa`, {
  form: { code: codigoTOTP(CRED.totp), return_to: '/account' },
  maxRedirects: 0,
});
const loc2 = r2.headers()['location'] ?? '';
console.log(`  ← ${r2.status()} location=${loc2.slice(0, 60)}`);
console.log(
  loc2.includes('step_up=1')
    ? '  ⇒ TAMBIÉN rechaza por HTTP: el problema es la credencial o el IdP'
    : '  ⇒ por HTTP SÍ entra: el problema está en lo que manda el navegador',
);

await browser.close();
