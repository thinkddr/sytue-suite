// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
//
// ¿Por qué no sale el correo? Sonda del camino de envío ENTERO, paso a paso, para
// poder decir DÓNDE muere en vez de «no se ha podido enviar».
//
// El camino tiene cuatro tramos y cada uno falla distinto:
//
//   1. sesión      — la cuenta entra en Sytue Mail (OIDC + segundo factor).
//   2. identidad   — qué dirección es la PRIMARIA del buzón. El servicio rechaza
//                    con 403 cualquier `From` que no lo sea, y la de la sesión
//                    (ops@sytue.com) NO tiene por qué serlo.
//   3. submission  — POST /api/send: si contesta 2xx, el mensaje quedó ENCOLADO.
//   4. entrega     — el worker lo saca por SMTP. Se comprueba mandándoselo A SÍ
//                    MISMO y esperando a que aparezca en la bandeja: eso recorre
//                    cola → SMTP saliente → MX → entrega local. Si el 3 va bien y
//                    el 4 no, el fallo está en la entrega, no en el compositor.
//
// Corre en la nube (workflow `mobile.yml`, entrada `reproductor`), que es donde
// arranca el Chromium; contra el dominio que diga SYTUE_BASE_DOMAIN.
//
//   SYTUE_AUDIT_EMAIL=… SYTUE_AUDIT_PASS=… SYTUE_AUDIT_TOTP=… \
//     bun run repro-mail-envio.mjs
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

/** Cuánto se espera a que el mensaje dé la vuelta y aparezca en la bandeja. */
const ESPERA_MS = 120_000;
const MARCA = `sonda-envio-${Date.now()}`;

const fallos = [];
const anota = (ok, que) => {
  console.log(`${ok ? '✓' : '✗'} ${que}`);
  if (!ok) fallos.push(que);
};

const browser = await chromium.launch();
const page = await (await browser.newContext({ viewport: { width: 1280, height: 900 } })).newPage();

try {
  // --- 1. Sesión ------------------------------------------------------------
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
  }
  await page.waitForTimeout(2000);
  anota(new URL(page.url()).hostname !== IDP, `sesión abierta en ${page.url()}`);

  // Todo lo demás va por el BFF DESDE la página, que es como lo hace el cliente:
  // con la cookie de sesión y sin que el token toque el navegador.
  const bff = (ruta, opciones) =>
    page.evaluate(
      async ([u, o]) => {
        try {
          const res = await fetch(u, o ?? undefined);
          const texto = await res.text();
          let json;
          try {
            json = JSON.parse(texto);
          } catch {
            json = null;
          }
          return { status: res.status, json, texto: texto.slice(0, 400) };
        } catch (e) {
          return { status: 0, json: null, texto: String(e) };
        }
      },
      [ruta, opciones ?? null],
    );

  // --- 2. Identidad ---------------------------------------------------------
  const carpetas = await bff('/api/mailboxes');
  anota(carpetas.status === 200, `GET /api/mailboxes → ${carpetas.status}`);
  const lista = carpetas.json?.mailboxes ?? carpetas.json?.list ?? [];
  const bandeja = lista.find((m) => (m.role ?? '').toLowerCase() === 'inbox') ?? lista[0];
  anota(Boolean(bandeja?.id), `bandeja de entrada localizada (${bandeja?.name ?? '—'})`);

  const alias = await bff('/api/aliases');
  const primaria = alias.json?.primary || '';
  anota(
    Boolean(primaria),
    `dirección primaria del buzón: ${primaria || 'NO la publica /api/aliases'}` +
      (primaria && primaria.toLowerCase() !== CRED.email.toLowerCase()
        ? `  (≠ la de la sesión, ${CRED.email} — el From se toma de la primaria)`
        : ''),
  );
  const destino = primaria || CRED.email;

  // --- 3. Submission --------------------------------------------------------
  // A SÍ MISMO: recorre el camino entero y se puede comprobar la llegada.
  const envio = await bff('/api/send', {
    method: 'POST',
    headers: { 'content-type': 'application/json' },
    body: JSON.stringify({
      to: [{ email: destino }],
      subject: `Sonda de envío ${MARCA}`,
      textBody: `Sonda automática del camino de envío. ${MARCA}`,
    }),
  });
  anota(
    envio.status >= 200 && envio.status < 300,
    `POST /api/send → ${envio.status} ${envio.status >= 300 ? envio.texto : '(encolado)'}`,
  );
  if (envio.status >= 300) {
    console.log('\n→ Muere en la SUBMISSION: el mensaje no llega ni a la cola.');
  }

  // --- 4. Entrega -----------------------------------------------------------
  let llego = false;
  const hasta = Date.now() + ESPERA_MS;
  while (!llego && Date.now() < hasta) {
    await page.waitForTimeout(5000);
    const bandejaMsgs = await bff(`/api/emails?mailboxId=${encodeURIComponent(bandeja?.id ?? '')}&limit=25`);
    const correos = bandejaMsgs.json?.emails ?? [];
    llego = correos.some((e) => (e.subject ?? '').includes(MARCA));
  }
  anota(llego, `el correo vuelve a la bandeja en ${ESPERA_MS / 1000}s`);
  if (!llego && envio.status < 300) {
    console.log(
      '\n→ Muere en la ENTREGA: el servicio aceptó y encoló el mensaje, pero no sale.',
      '\n  Sitios a mirar, en este orden:',
      '\n   · el worker de la cola (services/mail/internal/delivery/worker.go): ¿logs de «rebote» o «diferir»?',
      '\n   · salida TCP al 25 desde el pod (los proveedores lo bloquean por defecto);',
      '\n   · el MX del propio dominio, si el destino es local.',
    );
  }
} catch (e) {
  anota(false, `excepción: ${String(e).slice(0, 300)}`);
  await page.screenshot({ path: '/tmp/repro-mail-envio.png' }).catch(() => {});
} finally {
  await browser.close();
}

console.log(fallos.length === 0 ? '\nEl correo sale y vuelve.' : `\n${fallos.length} fallo(s).`);
process.exit(fallos.length === 0 ? 0 : 1);
