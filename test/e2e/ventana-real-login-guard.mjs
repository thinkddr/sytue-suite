// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
// Guardas del arnés desktop. No importa Playwright ni lee credenciales del entorno.
const ID_ORIGIN = 'https://id.sytue.eu';
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
  const allowed = new Set(['state', 'code', 'error', 'error_description', 'error_uri']);
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

// El contexto y los ayudantes se inyectan para probar el flujo real sin navegador,
// red o cuentas. La CLI sigue entregando la URL de vuelta a ventana-real.sh por IPC.
export async function autorizarDesktop(
  ctx,
  authorization,
  { email, clave, semilla, organizacion, activarOrganizacion, codigoTOTP, timeoutMs = 120_000 },
) {
  const destination = validarAutorizacion(authorization);
  if (!email || !clave) reject('credentials_missing');
  let timer;
  let page;
  let credentialsPhase = true;
  let resolvePost;
  let rejectPost;
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
      await page.route('**/*', async (route) => {
        const request = route.request();
        try {
          const target = new URL(request.url());
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
      const response = await ctx.request.get(destination, { maxRedirects: 0 });
      return validarRetorno(destination, response.headers().location ?? '');
    };
    const expiry = new Promise((_, fail) => {
      timer = setTimeout(() => fail(new LoginError('login_timeout')), timeoutMs);
      timer.unref?.();
    });
    return await Promise.race([run(), expiry]);
  } catch (error) {
    throw new LoginError(codigoPublico(error));
  } finally {
    clearTimeout(timer);
    await page?.close().catch(() => {});
  }
}
