// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
//
// ¿Abre Sytue Mail sus datos cifrados? Sonda de la puerta de la bóveda de Mail con la
// cuenta de auditoría, contra producción.
//
// POR QUÉ EXISTE. v4.0.2 seguía sin abrir el correo cifrado con
// `prepare/mail.capacidades.error.Error.crypto_rejected.core.puerto.rechazado.host.authority.revalidate.error.Error.host_port_rejected`,
// y el barrido móvil daba Mail por «ok»: `abrirPuertaCifrado` espera 15 s a que la
// puerta deje de estar «abriendo» y, si no ha terminado, la da por abierta. La
// preparación de Mail (núcleo, autoridad, capacidades) tarda más, así que el barrido
// medía el esqueleto de carga. Aquí se espera al final de verdad: o la app, o el
// código del fallo.
//
// DOS PASADAS, y la segunda solo si la primera no abre:
//   1. Tal cual está desplegado.
//   2. Con el arreglo de `PqcSourcePort.capabilitiesHandlers` inyectado en el JS de Mail
//      (el puerto temporal de capacidades responde a `authority.revalidate`). Dice si el
//      arreglo basta ANTES de publicarlo, y si no, cuál es el siguiente fallo. El
//      handler inyectado no revalida por su cuenta: lo hace el `run` del puerto, antes y
//      después de cada llamada, así que la autoridad se comprueba igual.
// Con el arreglo ya desplegado, la primera pasada abre y la segunda no se hace.
//
// SOLO PRUEBA ALGO SI LA CUENTA ABRE SU CORREO POR LA BÓVEDA (`estadoDelCorreo`). Con el
// correo en Matrix, la puerta de la bóveda no se monta y «abierta» no dice nada: así
// salió la primera tirada contra v4.0.2 (run 46022534651): Mail «abría» con el camino
// de la bóveda roto para todos, porque la cuenta de auditoría no pasaba por él.
//
// En la nube (`mobile.yml` → `workflow_dispatch`, `reproductor=repro-mail-desbloqueo.mjs`).
// Desde el portátil NO: lo que decide es dónde corre el navegador.
//
//   SYTUE_AUDIT_EMAIL=… SYTUE_AUDIT_PASS=… SYTUE_AUDIT_TOTP=… bun run repro-mail-desbloqueo.mjs
import { chromium } from 'playwright';
import { codigoTOTP, porQueNoEntra } from './tests/helpers.ts';
import { PERFIL_CIFRADO, estadoDelCorreo, estadoPuertaCifrado } from './tests/puerta-cifrado.ts';

const DOMINIO = process.env.SYTUE_BASE_DOMAIN || 'sytue.eu';
const IDP = `id.${DOMINIO}`;
// Por el host de organización, como `sembrar-auditoria.mjs`: el flujo OIDC empieza y
// acaba en el mismo host.
const HOST = `${process.env.SYTUE_ORG_HOST || 'corporate'}.${DOMINIO}`;
const BASE = '/mail';
const URL_MAIL = `https://${HOST}${BASE}/`;
const CRED = {
  email: process.env.SYTUE_AUDIT_EMAIL || '',
  pass: process.env.SYTUE_AUDIT_PASS || '',
  totp: process.env.SYTUE_AUDIT_TOTP || '',
};
if (!CRED.email || !CRED.pass) {
  console.error('Faltan SYTUE_AUDIT_EMAIL / SYTUE_AUDIT_PASS.');
  process.exit(2);
}
/**
 * PASAR EL CORREO DE LA CUENTA A LA BÓVEDA (autorizado por el dueño el 2026-11-27).
 * Solo con `SYTUE_TRASPASO_BOVEDA=1`, que pone `traspaso-auditoria-boveda.mjs`: el paso
 * `reproductor` de `mobile.yml` solo acepta un nombre de fichero, y así la sonda de
 * siempre no traspasa nunca por accidente. Es lo que hace el botón «Pasar a la bóveda»
 * de Mail: vacía el correo privado de la cuenta en Matrix y abre una bóveda nueva.
 */
const TRASPASO = process.env.SYTUE_TRASPASO_BOVEDA === '1';
/** Lo que puede tardar la preparación de Mail con el núcleo sin caché (más, si traspasa). */
const ESPERA_MS = TRASPASO ? 600_000 : 240_000;
/** Tanto tiempo seguido sin puerta para darla por abierta (y no una página a medio hidratar). */
const ABIERTA_MS = 10_000;

const browser = await chromium.launch();
// Sin service worker: si sirviera el JS de caché, el arreglo de la segunda pasada no
// llegaría a la página.
const ctx = await browser.newContext({ viewport: { width: 1280, height: 900 }, serviceWorkers: 'block' });
const page = await ctx.newPage();

// El JS de Mail que sirve producción. `capabilitiesHandlers` es el arreglo de v4.0.3
// (los nombres de método no se minifican): sin él, el camino de la bóveda está roto y
// no se traspasa nada.
const jsMail = [];
page.on('response', (res) => {
  const u = new URL(res.url());
  if (u.hostname === HOST && u.pathname.startsWith(`${BASE}/`) && u.pathname.endsWith('.js'))
    jsMail.push(res.text().catch(() => ''));
});
const sirveArreglo = async () =>
  (await Promise.all(jsMail)).some((texto) => texto.includes('capabilitiesHandlers'));

// El código del fallo que deja la puerta en la consola (`PqVaultGate`).
const avisos = [];
page.on('console', (m) => {
  const t = m.text();
  if (t.startsWith('[sytue]')) avisos.push(t.slice(0, 400));
});

async function entrar() {
  if (new URL(page.url()).hostname !== IDP) return;
  await page.fill('input[name=email]', CRED.email);
  await page.fill('input[name=password]', CRED.pass);
  await page.click('button[type=submit]');
  // El segundo factor se reconoce por la URL (`step_up=1`, ver sembrar-auditoria.mjs).
  const pide = () => /[?&]step_up=1/.test(page.url());
  await page.waitForURL(pide, { timeout: 15_000 }).catch(() => {});
  const campo = page.locator('input[name=code], input[name=totp], input[autocomplete="one-time-code"]').first();
  if (await campo.waitFor({ timeout: pide() ? 30_000 : 5_000 }).then(() => true, () => false)) {
    if (!CRED.totp) throw new Error('la cuenta pide segundo factor y no hay SYTUE_AUDIT_TOTP');
    await campo.fill(codigoTOTP(CRED.totp));
    await page.click('button[type=submit]');
  }
  try {
    await page.waitForURL((u) => u.hostname !== IDP, { timeout: 60_000 });
  } catch {
    const porque = porQueNoEntra(page.url());
    throw new Error(`el login no sale del IdP: ${porque || page.url().slice(0, 160)}`);
  }
}

const enMail = (u) => u.hostname === HOST && u.pathname.startsWith(BASE);

async function abrirMail() {
  await page.goto(URL_MAIL, { waitUntil: 'domcontentloaded' });
  await page.waitForURL((u) => enMail(u) || u.hostname === IDP, { timeout: 60_000 }).catch(() => {});
  await entrar();
  await page.waitForURL(enMail, { timeout: 60_000 }).catch(() => {});
}

/** Espera al final de la puerta: `abierta`, `fallo` (con su código) o `cerrada`. */
async function esperarPuerta() {
  avisos.length = 0;
  await abrirMail();
  let reentrado = false,
    abiertaDesde = 0;
  const hasta = Date.now() + ESPERA_MS;
  while (Date.now() < hasta) {
    await page.waitForTimeout(2000);
    const codigo =
      (await page
        .evaluate(() => document.querySelector('[data-sytue-boveda-fallo]')?.getAttribute('data-sytue-boveda-fallo') ?? '')
        .catch(() => '')) ||
      (avisos.find((a) => a.includes('no abrió:'))?.split('no abrió:')[1]?.trim() ?? '');
    if (codigo) return { estado: 'fallo', codigo };
    const estado = await page.evaluate(estadoPuertaCifrado, PERFIL_CIFRADO).catch(() => 'abriendo');
    if (estado === 'cerrada') {
      // Sin la clave de desbloqueo en este contexto: se vuelve a entrar como lo haría
      // la persona (`/auth/login?unlock=1`), una sola vez.
      if (reentrado) {
        const texto = ((await page.locator('body').innerText().catch(() => '')) || '').replace(/\s+/g, ' ');
        return { estado: 'cerrada', codigo: '', texto: texto.slice(0, 300) };
      }
      reentrado = true;
      await page.goto(`https://${HOST}${BASE}/auth/login?unlock=1&returnTo=${encodeURIComponent(`${BASE}/`)}`, {
        waitUntil: 'domcontentloaded',
      });
      await page.waitForURL((u) => enMail(u) || u.hostname === IDP, { timeout: 60_000 }).catch(() => {});
      await entrar();
      await page.waitForURL(enMail, { timeout: 60_000 }).catch(() => {});
      abiertaDesde = 0;
      continue;
    }
    if (estado === 'abierta') {
      abiertaDesde ||= Date.now();
      if (Date.now() - abiertaDesde >= ABIERTA_MS) return { estado: 'abierta', codigo: '' };
    } else abiertaDesde = 0;
  }
  return { estado: 'agotado', codigo: '' };
}

function informar(etiqueta, r) {
  console.log(`\n── ${etiqueta}: ${r.estado}`);
  if (r.codigo) console.log(`   código del fallo: ${r.codigo}`);
  if (r.texto) console.log(`   la puerta dice: ${r.texto}`);
  for (const a of avisos) console.log(`   consola: ${a}`);
}

/**
 * El arreglo, sobre el JS minificado: el puerto temporal de capacidades que solo lleva
 * `source.capabilities`. El de Matrix ya empieza por `authority.revalidate` (v4.0.2) y
 * no casa.
 */
const SOLO_CAPACIDADES =
  /handlers:\{\s*(["'`])source\.capabilities\1\s*:\s*([\w$]+)\.handlers\[\s*(["'`])source\.capabilities\3\s*\]\s*\}/g;
let parches = 0;
async function inyectarArreglo() {
  await ctx.route(
    (u) => u.hostname.endsWith(DOMINIO) && u.pathname.endsWith('.js'),
    async (route) => {
      const res = await route.fetch();
      const texto = await res.text();
      let n = 0;
      const nuevo = texto.replace(SOLO_CAPACIDADES, (_m, _q, fuente) => {
        n++;
        return (
          'handlers:{"authority.revalidate":async(r,c)=>(c.assertOpen(),new Uint8Array),' +
          `"source.capabilities":${fuente}.handlers["source.capabilities"]}`
        );
      });
      if (!n) return route.fulfill({ response: res });
      parches += n;
      console.log(`   parche aplicado en ${new URL(route.request().url()).pathname} (${n})`);
      const headers = { ...res.headers() };
      delete headers['content-encoding'];
      delete headers['content-length'];
      return route.fulfill({ status: res.status(), headers, body: nuevo });
    },
  );
}

/**
 * Pide el traspaso como el botón «Pasar a la bóveda» (`pedirTraspaso` de
 * apps/mail/src/client/almacenCorreo.ts): la marca de la pestaña. La siguiente carga de
 * Mail abre la bóveda nueva y la olvida en cuanto el correo abre.
 */
async function pedirTraspaso() {
  await abrirMail();
  await page.waitForTimeout(5000);
  const { almacen: antes, entrada } = await page
    .evaluate(estadoDelCorreo, BASE)
    .catch(() => ({ almacen: 'dudoso', entrada: 'dudoso' }));
  console.log(
    `   antes del traspaso, la cuenta abre su correo por «${antes}»; entrada privada «${entrada}»`,
  );
  if (antes === 'boveda') return;
  // Las dos cosas que la bóveda nueva necesita para abrir, comprobadas ANTES de vaciar
  // nada: el arreglo del puerto (v4.0.3) en el JS y la entrada privada montada en el
  // servicio (v4.0.5). La segunda faltaba con la primera ya desplegada.
  if (!(await sirveArreglo()))
    throw new Error(
      'producción no sirve todavía el arreglo de v4.0.3 (no hay `capabilitiesHandlers` en el JS de ' +
        'Mail): el traspaso acabaría en la bóveda rota. No se ha tocado nada.',
    );
  if (entrada !== 'lista')
    throw new Error(
      `el servicio de Mail dice que la entrada privada está «${entrada}»: la bóveda nueva no ` +
        'abriría (`…core.entrada_no_lista`). No se ha tocado nada.',
    );
  if (antes !== 'matrix')
    throw new Error(`la autoridad de Rooms contesta «${antes}»: no se pide un traspaso a ciegas`);
  const pedido = await page.evaluate(async (base) => {
    const sesion = await (await fetch(`${base}/auth/session`)).json();
    const actor = sesion?.user?.id;
    if (typeof actor !== 'string' || !actor) return false;
    sessionStorage.setItem(`sytue.mail.traspaso:${sesion.tenant ?? ''}:${actor}`, '1');
    return true;
  }, BASE);
  if (!pedido) throw new Error('sin sesión de Mail: no se puede pedir el traspaso');
  console.log('   traspaso pedido: Mail abre una bóveda nueva y deja de usar Matrix');
}

let salida = 1;
try {
  if (TRASPASO) await pedirTraspaso();
  const tal = await esperarPuerta();
  // Sin la bóveda, «abierta» no dice nada de ella: Matrix o la pantalla de «Pasar a la
  // bóveda» tampoco llevan la marca de la puerta (ver `estadoDelCorreo`).
  const { almacen, entrada } = await page
    .evaluate(estadoDelCorreo, BASE)
    .catch(() => ({ almacen: 'dudoso', entrada: 'dudoso' }));
  console.log(`\n   la cuenta abre su correo por «${almacen}»; entrada privada «${entrada}»`);
  informar('Mail tal cual está desplegado', tal);
  if (tal.estado === 'abierta' && almacen !== 'boveda') {
    console.log(
      '\n✗ La cuenta no abre su correo por la bóveda: esta sonda no prueba el camino de la bóveda.',
    );
  } else if (tal.estado === 'abierta') {
    console.log('\n✓ Mail abre sus datos cifrados en la bóveda.');
    salida = 0;
  } else if (TRASPASO) {
    // Con el traspaso no se inyecta nada: la bóveda nueva se abre con el código servido.
    console.log('\n✗ El traspaso no terminó de abrir la bóveda: el código de arriba es el fallo.');
  } else {
    if (tal.codigo.includes('host.authority.revalidate'))
      console.log('   → reproducido: el puerto temporal de capacidades no responde a authority.revalidate.');
    await inyectarArreglo();
    const arreglado = await esperarPuerta();
    informar('Mail con el arreglo inyectado', arreglado);
    if (!parches) {
      console.log('\n✗ No se encontró en el JS de Mail el puerto a arreglar: la segunda pasada no prueba nada.');
    } else if (arreglado.estado === 'abierta') {
      console.log('\n✓ Con el arreglo, Mail abre sus datos cifrados.');
      salida = 0;
    } else {
      console.log('\n✗ Con el arreglo sigue sin abrir: el código de arriba es el SIGUIENTE fallo.');
    }
  }
} catch (e) {
  console.log(`\n✗ excepción: ${String(e).slice(0, 400)}`);
  await page.screenshot({ path: '/tmp/repro-mail-desbloqueo.png' }).catch(() => {});
} finally {
  await browser.close();
}
process.exit(salida);
