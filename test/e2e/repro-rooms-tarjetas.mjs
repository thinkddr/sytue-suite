// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
//
// Objetos ricos de Sytue Rooms (encuesta y tarea) EN UN NAVEGADOR DE VERDAD.
//
// Los tests de Go comprueban que el voto se guarda y que la RLS aísla; los de
// bun, que el recuento optimista cuadra. Ninguno de los dos ve lo único que le
// importa a quien usa el chat: que el menú «+» tenga la entrada, que la tarjeta
// se PINTE bajo el mensaje y que al pulsar una opción el porcentaje cambie.
// Un mensaje enviado con su tarjeta perdida por el camino se ve exactamente
// igual que uno normal.
//
// Corre en la nube (workflow `mobile.yml`, entrada `reproductor`), que es donde
// arranca el Chromium; contra el dominio que diga SYTUE_BASE_DOMAIN.
//
//   SYTUE_AUDIT_EMAIL=… SYTUE_AUDIT_PASS=… SYTUE_AUDIT_TOTP=… \
//     bun run repro-rooms-tarjetas.mjs
//
// Crea dos mensajes en el canal donde caiga la sesión y LOS BORRA al terminar,
// pase lo que pase. La cuenta de auditoría tiene autorización expresa para
// escribir en producción; dejar rastro no entra en esa autorización.
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

// Marca única: distingue lo de esta ejecución de lo que haya en el canal, y
// permite encontrar los dos mensajes para borrarlos al final.
const MARCA = `sonda-tarjetas-${Date.now()}`;
const PREGUNTA = `¿Sonda de encuesta? ${MARCA}`;
const TAREA = `Sonda de tarea ${MARCA}`;

const fallos = [];
const anota = (ok, que) => {
  console.log(`${ok ? '✓' : '✗'} ${que}`);
  if (!ok) fallos.push(que);
};

const browser = await chromium.launch();
// En inglés a propósito: es el catálogo base y el que fija los rótulos que
// busca este guion. Con el idioma del navegador de turno, los selectores
// dependerían de qué runner lo ejecuta.
const ctx = await browser.newContext({
  locale: 'en-GB',
  extraHTTPHeaders: { 'accept-language': 'en-GB,en;q=0.9' },
  viewport: { width: 1440, height: 900 },
});
const page = await ctx.newPage();

try {
  await page.goto(`https://rooms.${DOMINIO}/`, { waitUntil: 'domcontentloaded' });
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
  await page.waitForTimeout(3000);
  console.log('sesión en', page.url());

  // Esperar a que la dirección lleve el canal: Rooms la sincroniza sola al abrir
  // uno (pushState en ChatApp), y de ahí saca el canal la limpieza del final. Sin
  // esto la sonda podría empezar a escribir antes de que haya canal activo.
  await page.waitForURL(/\/channels\/[0-9a-f-]{36}/, { timeout: 60_000 });
  console.log('canal', page.url());

  // --- Encuesta ------------------------------------------------------------
  await page.getByRole('button', { name: 'Add to message' }).first().click();
  const entradaEncuesta = page.getByRole('menuitem', { name: 'Poll' });
  anota(await entradaEncuesta.isVisible().catch(() => false), 'el menú «+» ofrece Poll');
  await entradaEncuesta.click();

  const dialogo = page.getByRole('dialog');
  await dialogo.waitFor({ state: 'visible', timeout: 10_000 });
  await dialogo.getByLabel(/^Question/).fill(PREGUNTA);
  await dialogo.getByLabel('Option 1').fill('sí');
  await dialogo.getByLabel('Option 2').fill('no');
  await dialogo.getByRole('button', { name: 'Send' }).click();
  await dialogo.waitFor({ state: 'hidden', timeout: 10_000 });

  // El mensaje aparece con su pregunta Y con la tarjeta debajo. Si la tarjeta se
  // hubiera perdido, la pregunta saldría igual: por eso se comprueban las dos.
  await page.getByText(PREGUNTA).first().waitFor({ timeout: 20_000 });
  anota(true, 'la pregunta se publica');

  const opcionSi = page.getByRole('button', { name: /^sí/ }).last();
  anota(
    await opcionSi.waitFor({ timeout: 15_000 }).then(() => true, () => false),
    'la tarjeta pinta sus opciones bajo el mensaje',
  );

  // Votar: el botón queda pulsado y el resultado sube a 1 · 100%.
  await opcionSi.click();
  await page.waitForTimeout(1500);
  anota((await opcionSi.getAttribute('aria-pressed')) === 'true', 'la opción votada queda aria-pressed');
  const textoOpcion = (await opcionSi.innerText()).replace(/\s+/g, ' ');
  anota(/1 · 100%/.test(textoOpcion), `el resultado sube a 1 · 100% (leído: «${textoOpcion}»)`);

  // Desvotar: vuelve a cero y la opción deja de estar pulsada.
  await opcionSi.click();
  await page.waitForTimeout(1500);
  anota((await opcionSi.getAttribute('aria-pressed')) === 'false', 'al desvotar deja de estar pulsada');

  // --- Tarea ---------------------------------------------------------------
  await page.getByRole('button', { name: 'Add to message' }).first().click();
  await page.getByRole('menuitem', { name: 'Task' }).click();
  const dlgTarea = page.getByRole('dialog');
  await dlgTarea.waitFor({ state: 'visible', timeout: 10_000 });
  await dlgTarea.getByLabel(/^Task/).first().fill(TAREA);
  await dlgTarea.getByRole('button', { name: 'Send' }).click();
  await dlgTarea.waitFor({ state: 'hidden', timeout: 10_000 });

  await page.getByText(TAREA).first().waitFor({ timeout: 20_000 });
  const casilla = page.getByRole('checkbox', { name: /Mark as done|Done/ }).last();
  anota(
    await casilla.waitFor({ timeout: 15_000 }).then(() => true, () => false),
    'la tarea pinta su casilla',
  );
  await casilla.click();
  await page.waitForTimeout(1500);
  anota((await casilla.getAttribute('aria-checked')) === 'true', 'la tarea se marca como hecha');
  await casilla.click();
  await page.waitForTimeout(1000);
  anota((await casilla.getAttribute('aria-checked')) === 'false', 'la tarea se reabre');

  // Que no quede solo en pantalla: releer el canal desde el BFF y comprobar que
  // el servidor devuelve la tarjeta. Un render optimista que nunca se persistió
  // pasaría todo lo anterior.
  const desdeElServidor = await page.evaluate(async (pregunta) => {
    const canal = new URL(location.href).pathname.split('/')[2] || '';
    if (!canal) return { error: 'sin canal en la URL' };
    const res = await fetch(`/api/messages?channelId=${encodeURIComponent(canal)}`, {
      headers: { accept: 'application/json' },
    });
    if (!res.ok) return { error: `HTTP ${res.status}` };
    const { messages = [] } = await res.json();
    const m = messages.find((x) => x.body === pregunta);
    return { encontrado: Boolean(m), kind: m?.card?.kind ?? null, opciones: m?.card?.options ?? null };
  }, PREGUNTA);
  anota(
    desdeElServidor.encontrado && desdeElServidor.kind === 'poll' && desdeElServidor.opciones?.length === 2,
    `el servidor devuelve la tarjeta al releer (${JSON.stringify(desdeElServidor)})`,
  );
} catch (e) {
  anota(false, `excepción: ${String(e).slice(0, 300)}`);
  await page.screenshot({ path: '/tmp/repro-rooms-tarjetas.png' }).catch(() => {});
} finally {
  // Limpieza: borra por el BFF los mensajes de esta ejecución, con la marca por
  // delante para no tocar nada más. Se intenta aunque la prueba haya fallado.
  const borrados = await page
    .evaluate(async (marca) => {
      const canal = new URL(location.href).pathname.split('/')[2] || '';
      if (!canal) return 0;
      const res = await fetch(`/api/messages?channelId=${encodeURIComponent(canal)}`, {
        headers: { accept: 'application/json' },
      });
      if (!res.ok) return 0;
      const { messages = [] } = await res.json();
      let n = 0;
      for (const m of messages.filter((x) => (x.body || '').includes(marca))) {
        const r = await fetch('/api/message', {
          method: 'POST',
          headers: { 'content-type': 'application/json' },
          body: JSON.stringify({ messageId: m.id, delete: true }),
        });
        if (r.ok) n++;
      }
      return n;
    }, MARCA)
    .catch(() => 0);
  console.log(`limpieza: ${borrados} mensaje(s) borrado(s)`);
  await browser.close();
}

console.log(fallos.length === 0 ? '\nTodo en verde.' : `\n${fallos.length} fallo(s).`);
process.exit(fallos.length === 0 ? 0 : 1);
