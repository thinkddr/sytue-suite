// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
// Guardas del arnés desktop. No importa Playwright ni lee credenciales del entorno.
const ID_ORIGIN = 'https://id.sytue.eu';
const MATRIX_ORIGIN = 'https://matrix.sytue.eu';
const CALLBACK = 'http://127.0.0.1:8921/auth/callback';
const PUBLIC_CODES = new Set([
  'authorization_url',
  'authorization_parameters',
  'callback_origin',
  'callback_parameters',
  'callback_state',
  'credentials_missing',
  'login_origin',
  'mfa_required',
  'mfa_origin',
  'login_response',
  'login_request_failed',
  'login_failed',
  'login_timeout',
  'native_consent',
  'unlock_relay',
  'matrix_url',
  'matrix_origin',
  'matrix_login',
  'matrix_callback',
  'matrix_timeout',
]);

class LoginError extends Error {
  constructor(code) {
    super(code);
    this.name = 'NativeLoginError';
  }
}

function reject(code) {
  throw new LoginError(code);
}

function url(value, code) {
  try {
    return new URL(value);
  } catch {
    reject(code);
  }
}

export function codigoPublico(error) {
  return error instanceof LoginError && PUBLIC_CODES.has(error.message)
    ? error.message
    : 'login_failed';
}

export function validarAutorizacion(value) {
  const target = url(value, 'authorization_url');
  if (
    target.origin !== ID_ORIGIN ||
    target.pathname !== '/oauth2/authorize' ||
    target.username ||
    target.password ||
    target.hash
  )
    reject('authorization_url');
  for (const key of [
    'client_id',
    'redirect_uri',
    'state',
    'code_challenge',
    'code_challenge_method',
    'response_type',
  ]) {
    if (target.searchParams.getAll(key).length !== 1) reject('authorization_parameters');
  }
  if (
    target.searchParams.get('client_id') !== 'sytue-desktop' ||
    target.searchParams.get('redirect_uri') !== CALLBACK ||
    target.searchParams.get('response_type') !== 'code' ||
    target.searchParams.get('code_challenge_method') !== 'S256' ||
    !target.searchParams.get('state') ||
    !target.searchParams.get('code_challenge')
  ) {
    reject('authorization_parameters');
  }
  return target.href;
}

// La entrega cifrada de la clave de desbloqueo (ADR 0084): base64url de la clave
// efímera del relevo, el nonce y el texto cifrado para la clave de la app. El tope es
// el de `recoverywrap.OpenDelivery`.
const ENTREGA = /^[A-Za-z0-9_-]{1,4096}$/;

export function validarRetorno(authorization, value) {
  const original = new URL(validarAutorizacion(authorization));
  const target = url(value, 'callback_origin');
  const expected = new URL(CALLBACK);
  if (
    target.origin !== expected.origin ||
    target.pathname !== expected.pathname ||
    target.username ||
    target.password ||
    target.hash
  )
    reject('callback_origin');
  const allowed = new Set([
    'state',
    'code',
    'error',
    'error_description',
    'error_uri',
    'sytue_unlock',
  ]);
  for (const [key] of target.searchParams) {
    if (!allowed.has(key) || target.searchParams.getAll(key).length !== 1) {
      reject('callback_parameters');
    }
  }
  const code = target.searchParams.get('code');
  const error = target.searchParams.get('error');
  if (target.searchParams.get('state') !== original.searchParams.get('state')) {
    reject('callback_state');
  }
  if (
    target.searchParams.has('code') === target.searchParams.has('error') ||
    !(code || error) ||
    (!error &&
      (target.searchParams.has('error_description') || target.searchParams.has('error_uri')))
  ) {
    reject('callback_parameters');
  }
  // La entrega solo viaja con un código: con un error no hay nada que desbloquear.
  if (
    target.searchParams.has('sytue_unlock') &&
    (!code || !ENTREGA.test(target.searchParams.get('sytue_unlock')))
  ) {
    reject('callback_parameters');
  }
  return target.href;
}

/** ¿Trae la vuelta la clave de desbloqueo cifrada para la app? Solo se dice, no se lee. */
export function traeDesbloqueo(value) {
  try {
    return new URL(value).searchParams.has('sytue_unlock');
  } catch {
    return false;
  }
}

/**
 * El «Permitir» de la pantalla de consentimiento de las apps nativas (NAT-02,
 * services/id/internal/http/authorize_nativa.go): la primera vez en cada dispositivo
 * Sytue ID pregunta antes de dar el código, y el arnés estrena dispositivo en cada
 * tirada. El enlace solo se sigue si es LA MISMA autorización —mismo origen y ruta,
 * los mismos parámetros con los mismos valores— con `sytue_consent` añadido y nada más.
 */
export function enlaceDeConsentimiento(authorization, html) {
  const original = new URL(validarAutorizacion(authorization));
  const hallado = /<a class="cta" href="([^"]+)"/.exec(String(html ?? ''));
  if (!hallado) reject('native_consent');
  // `html.EscapeString` de Go: &amp; &lt; &gt; &#39; &#34;, y &amp; la última.
  const href = hallado[1]
    .replaceAll('&#34;', '"')
    .replaceAll('&#39;', "'")
    .replaceAll('&lt;', '<')
    .replaceAll('&gt;', '>')
    .replaceAll('&amp;', '&');
  const target = url(new URL(href, ID_ORIGIN).href, 'native_consent');
  if (
    target.origin !== ID_ORIGIN ||
    target.pathname !== original.pathname ||
    target.username ||
    target.password ||
    target.hash
  )
    reject('native_consent');
  const claves = new Set([...original.searchParams.keys(), ...target.searchParams.keys()]);
  for (const clave of claves) {
    const antes = original.searchParams.getAll(clave);
    const ahora = target.searchParams.getAll(clave);
    if (clave === 'sytue_consent') {
      if (antes.length !== 0 || ahora.length !== 1 || !ahora[0]) reject('native_consent');
    } else if (antes.length !== ahora.length || antes.some((v, i) => v !== ahora[i])) {
      reject('native_consent');
    }
  }
  if (!target.searchParams.has('sytue_consent')) reject('native_consent');
  return target.href;
}

function loginPage(value) {
  const target = url(value, 'login_origin');
  if (
    target.origin !== ID_ORIGIN ||
    target.username ||
    target.password ||
    target.hash ||
    !['/login/', '/login/mfa'].includes(target.pathname)
  )
    reject('login_origin');
}

function mfaPage(value, authorization) {
  const target = url(new URL(value, authorization).href, 'mfa_origin');
  if (
    target.origin !== ID_ORIGIN ||
    target.username ||
    target.password ||
    target.hash ||
    !['/login/', '/login/mfa'].includes(target.pathname) ||
    target.searchParams.getAll('step_up').length !== 1 ||
    target.searchParams.get('step_up') !== '1'
  )
    reject('mfa_origin');
  return target.href;
}

const espera = (ms) => new Promise((resolve) => setTimeout(resolve, ms));

/**
 * Lo que queda de la autorización una vez dentro (ADR 0084): ir a /authorize EN EL
 * NAVEGADOR, no con una petición suelta, porque lo que vuelve ya no es un 302 con el
 * código sino la página del RELEVO, cuyo guion abre en el navegador el sobre de la clave
 * de desbloqueo con la KEK que derivó la página de inicio de sesión y la añade, cifrada
 * para la clave efímera de la app, a la vuelta al loopback (`sytue_unlock`). Antes, la
 * pregunta de NAT-02 la primera vez en cada dispositivo. Se espera a que el guion del
 * relevo navegue a la vuelta —que el arnés recoge sin entregarla— y no se corta antes:
 * cortar es entrar sin desbloquear.
 */
async function hastaLaVuelta(page, destination, vuelta, timeoutMs) {
  let respuesta;
  try {
    respuesta = await page.goto(destination, { waitUntil: 'domcontentloaded' });
  } catch (error) {
    // El relevo puede salir hacia la vuelta antes de que acabe el `goto`.
    if (!vuelta.lista()) throw error;
  }
  if (!vuelta.lista()) {
    const html = await respuesta?.text?.().catch(() => '');
    if (/<a class="cta" href="/.test(html ?? '')) {
      const permitir = enlaceDeConsentimiento(destination, html);
      try {
        await page.goto(permitir, { waitUntil: 'domcontentloaded' });
      } catch (error) {
        if (!vuelta.lista()) throw error;
      }
    }
  }
  const limite = Date.now() + timeoutMs;
  while (!vuelta.lista()) {
    // El relevo se queda quieto, sin volver solo, cuando pide algo a la persona:
    // «la contraseña ya no abre el sobre» (con «empezar de cero») o «entra una vez con
    // la contraseña». El arnés no decide por ella: falla y lo dice.
    const parado = await page
      .evaluate(() =>
        ['e2e-lost', 'e2e-passkey-first'].some((id) => {
          const el = document.getElementById(id);
          return el && !el.hidden;
        }),
      )
      .catch(() => false);
    if (parado) reject('unlock_relay');
    if (Date.now() > limite) reject('login_timeout');
    await espera(250);
  }
  return vuelta.valor();
}

/** Una espera de la vuelta que el manejador de rutas resuelve. */
function esperaDeVuelta() {
  let valor = '';
  return {
    lista: () => valor !== '',
    valor: () => valor,
    poner: (v) => {
      if (!valor) valor = v;
    },
  };
}

// El contexto y los ayudantes se inyectan para probar el flujo real sin navegador,
// red o cuentas. La CLI sigue entregando la URL de vuelta a ventana-real.sh por IPC.
export async function autorizarDesktop(
  ctx,
  authorization,
  {
    email,
    clave,
    semilla,
    organizacion,
    activarOrganizacion,
    codigoTOTP,
    alFallar,
    timeoutMs = 180_000,
    relevoMs = 60_000,
  },
) {
  const destination = validarAutorizacion(authorization);
  if (!email || !clave) reject('credentials_missing');
  const callbackOrigin = new URL(CALLBACK).origin;
  let timer;
  let page;
  let credentialsPhase = true;
  let resolvePost;
  let rejectPost;
  const vuelta = esperaDeVuelta();
  const nextPost = () => {
    const reply = new Promise((resolve, fail) => {
      resolvePost = resolve;
      rejectPost = fail;
    });
    // Una ruta puede rechazar antes de que termine click(); el await conserva el error.
    reply.catch(() => {});
    return reply;
  };
  try {
    const run = async () => {
      page = await ctx.newPage();
      let posted = nextPost();
      // Si la vuelta llega como redirección y la ruta no la recoge, el navegador la lleva
      // él al loopback; se apunta igual (la app ya la tiene y la segunda entrega sobra).
      page.on?.('request', (request) => {
        if (!credentialsPhase && new URL(request.url()).origin === callbackOrigin)
          vuelta.poner(request.url());
      });
      await page.route('**/*', async (route) => {
        const request = route.request();
        try {
          const target = new URL(request.url());
          // La vuelta al loopback NO se entrega desde aquí: lleva un código de un solo uso
          // y se devuelve por IPC a ventana-real.sh, que se la da a la app.
          if (target.origin === callbackOrigin) {
            if (credentialsPhase) {
              await route.abort();
              rejectPost(new LoginError('login_origin'));
              return;
            }
            vuelta.poner(request.url());
            await route.fulfill({
              status: 200,
              contentType: 'text/plain; charset=utf-8',
              body: 'Sytue: vuelta recogida por el arnés.',
            });
            return;
          }
          if (
            credentialsPhase &&
            target.origin !== ID_ORIGIN &&
            (request.method() === 'POST' || request.isNavigationRequest())
          ) {
            await route.abort();
            rejectPost(new LoginError('login_origin'));
            return;
          }
          if (
            !credentialsPhase ||
            request.method() !== 'POST' ||
            target.origin !== ID_ORIGIN ||
            !['/login/', '/login/mfa'].includes(target.pathname)
          ) {
            await route.continue();
            return;
          }
          const response = await route.fetch({ maxRedirects: 0 });
          resolvePost({ status: response.status(), location: response.headers().location ?? '' });
          await route.fulfill({ status: 204 });
        } catch {
          rejectPost(new LoginError('login_request_failed'));
        }
      });
      await page.goto(destination, { waitUntil: 'domcontentloaded' });
      await page.waitForSelector('input[name=password]', { timeout: 30_000 });
      loginPage(page.url());
      await page.fill('input[name=email]', email);
      loginPage(page.url());
      await page.fill('input[name=password]', clave);
      loginPage(page.url());
      // El formulario deriva la KEK (Argon2id de 64 MiB, e2e/login.js) ANTES de enviarse
      // y la deja en el almacén de id.<dominio>; el envío llega después a la ruta de arriba.
      await page.click('button[type=submit]');
      let reply = await posted;
      const stepUp = new URL(reply.location || '/', destination);
      if (reply.status === 302 && stepUp.searchParams.get('step_up') === '1') {
        if (!semilla || typeof codigoTOTP !== 'function') reject('mfa_required');
        const target = mfaPage(reply.location, destination);
        posted = nextPost();
        await page.goto(target, { waitUntil: 'domcontentloaded' });
        loginPage(page.url());
        await page.fill('input[name=code]', codigoTOTP(semilla));
        loginPage(page.url());
        await page.click('form[action="/login/mfa"] button[type=submit]');
        reply = await posted;
      }
      if (reply.status !== 302) reject('login_response');
      credentialsPhase = false;
      if (organizacion) {
        if (typeof activarOrganizacion !== 'function') reject('login_failed');
        await page.goto('https://one.sytue.eu/one/', { waitUntil: 'domcontentloaded' });
        await page.waitForURL((target) => target.hostname !== 'id.sytue.eu', { timeout: 60_000 });
        await activarOrganizacion(page);
      }
      return validarRetorno(destination, await hastaLaVuelta(page, destination, vuelta, relevoMs));
    };
    const expiry = new Promise((_, fail) => {
      timer = setTimeout(() => fail(new LoginError('login_timeout')), timeoutMs);
      timer.unref?.();
    });
    return await Promise.race([run(), expiry]);
  } catch (error) {
    const code = codigoPublico(error);
    // Lo que se ve cuando se para DESPUÉS de las credenciales (el relevo, la pregunta de
    // NAT-02): nunca con el formulario de la contraseña delante.
    if (!credentialsPhase && page && typeof alFallar === 'function') {
      await Promise.resolve(alFallar(page, code)).catch(() => {});
    }
    throw new LoginError(code);
  } finally {
    clearTimeout(timer);
    await page?.close().catch(() => {});
  }
}

// --- El alta del dispositivo de Matrix -----------------------------------------------

const MATRIX_SSO = /^\/_matrix\/client\/v3\/login\/sso\/redirect(\/[A-Za-z0-9._~-]+)?$/;
const MATRIX_VUELTA = /^\/matrix\/callback\/[A-Za-z0-9]{8,64}$/;

/**
 * La URL del SSO de Matrix que abre una app al configurar el dispositivo
 * (`apps/nucleo/matrix/login.go`, `Login`): el homeserver de la suite, la ruta del SSO y
 * una vuelta al loopback de esa app (`127.0.0.1:<puerto>/matrix/callback/<nonce>`), y
 * nada más que lo que esa función pone.
 */
export function validarSSOMatrix(value) {
  const target = url(value, 'matrix_url');
  if (
    target.origin !== MATRIX_ORIGIN ||
    !MATRIX_SSO.test(target.pathname) ||
    target.username ||
    target.password ||
    target.hash
  )
    reject('matrix_url');
  for (const [key] of target.searchParams) {
    if (
      !['redirectUrl', 'sytue_tenant', 'sytue_subject'].includes(key) ||
      target.searchParams.getAll(key).length !== 1
    )
      reject('matrix_url');
  }
  const back = url(target.searchParams.get('redirectUrl') ?? '', 'matrix_url');
  if (
    back.protocol !== 'http:' ||
    back.hostname !== '127.0.0.1' ||
    !/^[0-9]{2,5}$/.test(back.port) ||
    !MATRIX_VUELTA.test(back.pathname) ||
    back.search ||
    back.hash ||
    back.username ||
    back.password
  )
    reject('matrix_url');
  return { href: target.href, vuelta: back.href };
}

/** La vuelta del SSO de Matrix: la de la URL pedida, con su `loginToken` y nada más. */
export function validarRetornoMatrix(sso, value) {
  const { vuelta } = validarSSOMatrix(sso);
  const expected = new URL(vuelta);
  const target = url(value, 'matrix_callback');
  if (
    target.origin !== expected.origin ||
    target.pathname !== expected.pathname ||
    target.username ||
    target.password ||
    target.hash
  )
    reject('matrix_callback');
  for (const [key] of target.searchParams) {
    if (key !== 'loginToken' || target.searchParams.getAll(key).length !== 1)
      reject('matrix_callback');
  }
  const token = target.searchParams.get('loginToken') ?? '';
  if (!token || token.length > 4096) reject('matrix_callback');
  return target.href;
}

/**
 * Completa el SSO de Matrix con la sesión de Sytue ID que dejó el inicio de sesión de la
 * tirada (el contexto llega con ese estado): Synapse → /authorize de Sytue ID → Synapse →
 * vuelta al loopback de la app con un `loginToken` de un solo uso, que se devuelve por IPC
 * como la del inicio de sesión. Aquí NO se teclea la contraseña: si Sytue ID pide entrar,
 * falla. Si pide solo el segundo factor (step-up), lo da una vez con el ayudante TOTP.
 */
export async function autorizarMatrix(
  ctx,
  sso,
  { timeoutMs = 90_000, alFallar, semilla, codigoTOTP } = {},
) {
  const { href, vuelta: back } = validarSSOMatrix(sso);
  const callbackOrigin = new URL(back).origin;
  const vuelta = esperaDeVuelta();
  let page;
  let fuera = false;
  try {
    page = await ctx.newPage();
    page.on?.('request', (request) => {
      if (new URL(request.url()).origin === callbackOrigin) vuelta.poner(request.url());
    });
    await page.route('**/*', async (route) => {
      const request = route.request();
      try {
        const target = new URL(request.url());
        if (target.origin === callbackOrigin) {
          vuelta.poner(request.url());
          await route.fulfill({
            status: 200,
            contentType: 'text/plain; charset=utf-8',
            body: 'Sytue: vuelta recogida por el arnés.',
          });
          return;
        }
        if (
          (request.isNavigationRequest() || request.method() === 'POST') &&
          ![MATRIX_ORIGIN, ID_ORIGIN].includes(target.origin)
        ) {
          fuera = true;
          await route.abort();
          return;
        }
        await route.continue();
      } catch {
        await route.abort().catch(() => {});
      }
    });
    await page.goto(href, { waitUntil: 'domcontentloaded' }).catch((error) => {
      if (!vuelta.lista()) throw error;
    });
    const limite = Date.now() + timeoutMs;
    let segundoFactor = false;
    while (!vuelta.lista()) {
      if (fuera) reject('matrix_origin');
      const aqui = url(page.url(), 'matrix_origin');
      if (aqui.origin === ID_ORIGIN && ['/login/', '/login/mfa'].includes(aqui.pathname)) {
        const pasoDos = aqui.searchParams.get('step_up') === '1';
        if (!pasoDos || segundoFactor || !semilla || typeof codigoTOTP !== 'function')
          reject('matrix_login');
        segundoFactor = true;
        const antes = page.url();
        await page.fill('input[name=code]', codigoTOTP(semilla));
        await page.click('form[action="/login/mfa"] button[type=submit]');
        // Que el envío navegue antes de volver a mirar dónde está: si no, la página del
        // segundo factor se leería otra vez como «pide entrar».
        await page.waitForURL((u) => u.href !== antes, { timeout: 30_000 }).catch(() => {});
      }
      if (Date.now() > limite) reject('matrix_timeout');
      await espera(250);
    }
    return validarRetornoMatrix(sso, vuelta.valor());
  } catch (error) {
    const code = codigoPublico(error);
    if (page && typeof alFallar === 'function') {
      await Promise.resolve(alFallar(page, code)).catch(() => {});
    }
    throw new LoginError(code);
  } finally {
    await page?.close().catch(() => {});
  }
}
