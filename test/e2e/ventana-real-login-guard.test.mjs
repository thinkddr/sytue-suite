// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
import assert from 'node:assert/strict';
import test from 'node:test';
import {
  autorizarDesktop,
  autorizarMatrix,
  codigoPublico,
  enlaceDeConsentimiento,
  traeDesbloqueo,
  validarAutorizacion,
  validarRetorno,
  validarRetornoMatrix,
  validarSSOMatrix,
} from './ventana-real-login-guard.mjs';

const callback = 'http://127.0.0.1:8921/auth/callback';
const authorization = () =>
  `https://id.sytue.eu/oauth2/authorize?${new URLSearchParams({
    client_id: 'sytue-desktop',
    redirect_uri: callback,
    state: 'synthetic-state',
    response_type: 'code',
    code_challenge: 'synthetic-pkce',
    code_challenge_method: 'S256',
    sytue_unlock_key: 'synthetic-public-key',
  })}`;
const returned = `${callback}?code=synthetic-code&state=synthetic-state`;
const entrega = 'BHN5bnRoZXRpYy1kZWxpdmVyeQ';
const returnedUnlocked = `${returned}&sytue_unlock=${entrega}`;

/**
 * Un navegador de mentira con el Sytue ID de la ADR 0084: la primera /authorize pinta el
 * formulario; las siguientes, la pregunta de NAT-02 (si `consentPage`) y el RELEVO, que
 * navega él solo a la vuelta (la ruta la recoge) salvo que se quede parado (`relayStuck`).
 */
function fake({
  landing = 'https://id.sytue.eu/login/',
  responseLocation = '/',
  secondLocation = '/',
  callbackLocation = returnedUnlocked,
  failedGoto = false,
  foreignPost = false,
  callbackDuringCredentials = false,
  consentPage,
  relayStuck = false,
} = {}) {
  const calls = {
    fills: [],
    gotos: [],
    posts: [],
    aborted: 0,
    closed: 0,
    otp: 0,
    organization: 0,
    fulfilled: [],
    screenshots: 0,
  };
  let current;
  let handler;
  let clicks = 0;
  let authorizes = 0;
  const route = (target, method = 'GET') => ({
    request: () => ({
      method: () => method,
      url: () => target,
      isNavigationRequest: () => true,
    }),
    abort: async () => {
      calls.aborted++;
    },
    continue: async () => {},
    fetch: async () => ({
      status: () => 302,
      headers: () => ({ location: clicks === 1 ? responseLocation : secondLocation }),
    }),
    fulfill: async (value) => {
      calls.fulfilled.push(value);
    },
  });
  const page = {
    route: async (_, value) => {
      handler = value;
    },
    on: () => {},
    goto: async (value) => {
      calls.gotos.push(value);
      if (failedGoto)
        throw new Error('sensitive-error https://foreign.test/?secret=synthetic-password');
      if (!value.includes('oauth2/authorize')) {
        current = value;
        return { text: async () => '' };
      }
      authorizes++;
      if (authorizes === 1) {
        current = landing;
        return { text: async () => '<form action="/login/">' };
      }
      current = value;
      if (consentPage !== undefined && !value.includes('sytue_consent'))
        return { text: async () => consentPage };
      if (!relayStuck) await handler(route(callbackLocation));
      return { text: async () => '<script type="application/json" id="e2e-relay">' };
    },
    evaluate: async () => relayStuck,
    url: () => current,
    waitForSelector: async () => {},
    fill: async (selector, value) => {
      calls.fills.push({ selector, value });
    },
    click: async () => {
      clicks++;
      if (callbackDuringCredentials) {
        await handler(route(returned));
        return;
      }
      const target = foreignPost
        ? 'https://foreign.test/steal'
        : `https://id.sytue.eu/login/${clicks === 1 ? '' : 'mfa'}`;
      calls.posts.push(target);
      await handler(route(target, 'POST'));
    },
    waitForURL: async () => {},
    screenshot: async () => {
      calls.screenshots++;
    },
    close: async () => {
      calls.closed++;
    },
  };
  const context = { newPage: async () => page };
  return { context, calls };
}
const options = () => ({
  email: 'synthetic@invalid.test',
  clave: 'synthetic-password',
  timeoutMs: 2_000,
  relevoMs: 1_000,
});

test('acepta únicamente autorización desktop, PKCE S256 y callback exactos', () => {
  assert.equal(validarAutorizacion(authorization()), authorization());
  for (const target of [
    'https://foreign.test/oauth2/authorize',
    authorization().replace('https:', 'http:'),
    authorization().replace('id.sytue.eu', 'id.sytue.eu.foreign.test'),
    authorization().replace('id.sytue.eu', 'name:password@id.sytue.eu'),
    authorization().replace('sytue-desktop', 'sytue-mobile'),
    authorization().replace('S256', 'plain'),
    `${authorization()}&state=second`,
    `${authorization()}&redirect_uri=https://foreign.test/`,
    `${authorization()}#private`,
  ])
    assert.throws(() => validarAutorizacion(target));
});

test('el retorno conserva state y rechaza puertos, rutas, fragmentos y parámetros ambiguos', () => {
  assert.equal(validarRetorno(authorization(), returned), returned);
  for (const target of [
    returned.replace(':8921', ':8922'),
    returned.replace('/auth/callback?', '/auth/callback/extra?'),
    returned.replace('127.0.0.1', '127.0.0.1.foreign.test'),
    returned.replace('synthetic-state', 'wrong'),
    `${returned}&state=second`,
    `${returned}&code=second`,
    `${returned}&error=denied`,
    `${returned}#private`,
    `${returned}&other=private`,
    `${returned}&error_description=private`,
  ])
    assert.throws(() => validarRetorno(authorization(), target));
});

test('la vuelta admite la entrega cifrada del relevo (ADR 0084) y nada que se le parezca', () => {
  assert.equal(validarRetorno(authorization(), returnedUnlocked), returnedUnlocked);
  assert.equal(traeDesbloqueo(returnedUnlocked), true);
  assert.equal(traeDesbloqueo(returned), false);
  for (const target of [
    `${returnedUnlocked}&sytue_unlock=${entrega}`,
    `${returned}&sytue_unlock=`,
    `${returned}&sytue_unlock=con%20espacios`,
    `${returned}&sytue_unlock=${'A'.repeat(4097)}`,
    `${callback}?error=access_denied&state=synthetic-state&sytue_unlock=${entrega}`,
  ])
    assert.throws(() => validarRetorno(authorization(), target), /callback_parameters/);
});

test('rechaza el destino extraño antes de crear página o introducir datos', async () => {
  let pages = 0;
  await assert.rejects(
    autorizarDesktop(
      {
        newPage: async () => {
          pages++;
        },
      },
      'https://foreign.test/',
      options(),
    ),
  );
  assert.equal(pages, 0);
});

test('una redirección extraña no recibe email ni contraseña', async () => {
  const { context, calls } = fake({ landing: 'https://foreign.test/login/' });
  await assert.rejects(autorizarDesktop(context, authorization(), options()), /login_origin/);
  assert.deepEqual(calls.fills, []);
  assert.equal(calls.closed, 1);
});

test('bloquea un POST de login fuera de ID', async () => {
  const { context, calls } = fake({ foreignPost: true });
  await assert.rejects(autorizarDesktop(context, authorization(), options()), /login_origin/);
  assert.equal(calls.aborted, 1);
});

test('una vuelta al loopback mientras se teclean las credenciales no sale ni se recoge', async () => {
  const { context, calls } = fake({ callbackDuringCredentials: true });
  await assert.rejects(autorizarDesktop(context, authorization(), options()), /login_origin/);
  assert.equal(calls.aborted, 1);
  assert.deepEqual(calls.fulfilled, []);
});

test('login correcto espera al relevo en el navegador y entrega la vuelta con la clave', async () => {
  const { context, calls } = fake();
  assert.equal(await autorizarDesktop(context, authorization(), options()), returnedUnlocked);
  assert.equal(calls.fills.length, 2);
  assert.equal(calls.closed, 1);
  // La segunda /authorize va por el NAVEGADOR (el relevo es un guion de la página), y la
  // vuelta la recoge la ruta sin llegar a la app.
  assert.deepEqual(calls.gotos, [authorization(), authorization()]);
  // El 204 del envío del formulario (que no navega) y el 200 de la vuelta recogida.
  assert.deepEqual(
    calls.fulfilled.map((r) => r.status),
    [204, 200],
  );
});

test('sin relevo (Sytue ID redirige como antes) la vuelta sale igual, sin clave', async () => {
  const { context } = fake({ callbackLocation: returned });
  assert.equal(await autorizarDesktop(context, authorization(), options()), returned);
});

test('un relevo que pide algo a la persona no se decide desde el arnés', async () => {
  let pantalla = '';
  const { context, calls } = fake({ relayStuck: true });
  await assert.rejects(
    autorizarDesktop(context, authorization(), {
      ...options(),
      alFallar: async (_, code) => {
        pantalla = code;
      },
    }),
    /unlock_relay/,
  );
  assert.equal(pantalla, 'unlock_relay');
  assert.equal(calls.closed, 1);
});

test('si falla tecleando las credenciales no se fotografía la página', async () => {
  let llamado = false;
  const { context } = fake({ foreignPost: true });
  await assert.rejects(
    autorizarDesktop(context, authorization(), {
      ...options(),
      alFallar: () => {
        llamado = true;
      },
    }),
  );
  assert.equal(llamado, false);
});

test('MFA conserva el flujo en ID y usa el ayudante TOTP existente', async () => {
  const { context, calls } = fake({ responseLocation: '/login/?step_up=1' });
  const opts = {
    ...options(),
    semilla: 'synthetic-otp',
    codigoTOTP: (seed) => {
      assert.equal(seed, 'synthetic-otp');
      calls.otp++;
      return '000000';
    },
  };
  assert.equal(await autorizarDesktop(context, authorization(), opts), returnedUnlocked);
  assert.equal(calls.otp, 1);
  assert.equal(calls.fills.at(-1).selector, 'input[name=code]');
  assert.deepEqual(calls.gotos, [
    authorization(),
    'https://id.sytue.eu/login/?step_up=1',
    authorization(),
  ]);
});

test('MFA sin semilla no navega ni inventa un segundo factor', async () => {
  const { context, calls } = fake({ responseLocation: '/login/?step_up=1' });
  await assert.rejects(autorizarDesktop(context, authorization(), options()), /mfa_required/);
  assert.equal(calls.fills.length, 2);
  assert.deepEqual(calls.gotos, [authorization()]);
});

test('una Location MFA extraña no recibe el segundo factor', async () => {
  const { context, calls } = fake({ responseLocation: 'https://foreign.test/login/?step_up=1' });
  await assert.rejects(
    autorizarDesktop(context, authorization(), {
      ...options(),
      semilla: 'synthetic-otp',
      codigoTOTP: () => '000000',
    }),
    /mfa_origin/,
  );
  assert.equal(calls.fills.length, 2);
  assert.deepEqual(calls.gotos, [authorization()]);
});

test('un callback incorrecto no sale del arnés', async () => {
  const { context } = fake({ callbackLocation: `${callback}?code=private&state=wrong` });
  await assert.rejects(autorizarDesktop(context, authorization(), options()), /callback_state/);
});

test('la organización usa el ayudante existente solo después del login', async () => {
  const { context, calls } = fake();
  const activate = async (page) => {
    assert.equal(page.url(), 'https://one.sytue.eu/one/');
    calls.organization++;
  };
  assert.equal(
    await autorizarDesktop(context, authorization(), {
      ...options(),
      organizacion: 'synthetic-organization',
      activarOrganizacion: activate,
    }),
    returnedUnlocked,
  );
  assert.equal(calls.organization, 1);
  assert.deepEqual(calls.gotos, [authorization(), 'https://one.sytue.eu/one/', authorization()]);
});

test('errores del navegador nunca publican cuerpo, URL, contraseña ni detalles', async () => {
  const { context } = fake({ failedGoto: true });
  await assert.rejects(autorizarDesktop(context, authorization(), options()), (error) => {
    assert.equal(error.message, 'login_failed');
    assert.equal(codigoPublico(error), 'login_failed');
    assert.doesNotMatch(String(error), /sensitive-error|foreign|password/);
    return true;
  });
  assert.equal(codigoPublico(new Error('synthetic-password')), 'login_failed');
});

// La pantalla de NAT-02 tal como la escribe el IdP: el enlace con `html.EscapeString`.
const permitir = `${authorization()}&sytue_consent=synthetic-signature`;
const pantallaConsentimiento = (href = permitir) =>
  `<main><p class="sub">synthetic@invalid.test</p><a class="cta" href="${href
    .replace('https://id.sytue.eu', '')
    .replaceAll(
      '&',
      '&amp;',
    )}">Permitir</a><p class="alt"><a class="btn-link" href="${callback}?error=access_denied&amp;state=synthetic-state">No</a></p></main>`;

test('el consentimiento nativo (NAT-02) solo acepta la misma autorización con su firma', () => {
  assert.equal(enlaceDeConsentimiento(authorization(), pantallaConsentimiento()), permitir);
  // El IdP reordena los parámetros (`url.Values.Encode`): el orden no importa.
  const ordenada = `https://id.sytue.eu/oauth2/authorize?${new URLSearchParams(
    [...new URL(permitir).searchParams].sort(([a], [b]) => a.localeCompare(b)),
  )}`;
  assert.equal(enlaceDeConsentimiento(authorization(), pantallaConsentimiento(ordenada)), ordenada);
  for (const html of [
    '<main>sin enlace</main>',
    pantallaConsentimiento(authorization()),
    pantallaConsentimiento(`${permitir}&sytue_consent=second`),
    pantallaConsentimiento(`${permitir}&scope=extra`),
    pantallaConsentimiento(permitir.replace('synthetic-state', 'other-state')),
    pantallaConsentimiento(
      permitir.replace('code_challenge=synthetic-pkce', 'code_challenge=other'),
    ),
    pantallaConsentimiento(
      permitir.replace(encodeURIComponent(callback), 'https%3A%2F%2Fforeign.test%2F'),
    ),
    pantallaConsentimiento(
      permitir.replace('sytue_unlock_key=synthetic-public-key', 'sytue_unlock_key=other'),
    ),
    pantallaConsentimiento(permitir.replace('/oauth2/authorize', '/oauth2/other')),
    pantallaConsentimiento(`${permitir}#private`),
    `<a class="cta" href="https://foreign.test/oauth2/authorize?${new URL(permitir).search.slice(1).replaceAll('&', '&amp;')}">`,
    `<a class="cta" href="//foreign.test/oauth2/authorize?${new URL(permitir).search.slice(1).replaceAll('&', '&amp;')}">`,
  ])
    assert.throws(
      () => enlaceDeConsentimiento(authorization(), html),
      /native_consent|authorization/,
    );
});

test('con la pregunta de NAT-02 el login dice que sí y espera al relevo', async () => {
  const { context, calls } = fake({ consentPage: pantallaConsentimiento() });
  assert.equal(await autorizarDesktop(context, authorization(), options()), returnedUnlocked);
  assert.deepEqual(calls.gotos, [authorization(), authorization(), permitir]);
  assert.equal(calls.closed, 1);
});

test('un «Permitir» que no es esta autorización no se sigue', async () => {
  const { context, calls } = fake({
    consentPage: pantallaConsentimiento(permitir.replace('synthetic-state', 'other-state')),
  });
  await assert.rejects(autorizarDesktop(context, authorization(), options()), /native_consent/);
  assert.deepEqual(calls.gotos, [authorization(), authorization()]);
});

// --- El SSO de Matrix del alta del dispositivo ------------------------------------------

const vueltaMatrix = 'http://127.0.0.1:41234/matrix/callback/ABCDEFGH23456789';
const sso = (extra = {}) =>
  `https://matrix.sytue.eu/_matrix/client/v3/login/sso/redirect/oidc-sytue?${new URLSearchParams({
    redirectUrl: vueltaMatrix,
    ...extra,
  })}`;
const conToken = `${vueltaMatrix}?loginToken=synthetic-login-token`;

test('el SSO de Matrix solo se sigue hacia el homeserver y de vuelta al loopback de la app', () => {
  assert.equal(validarSSOMatrix(sso()).vuelta, vueltaMatrix);
  assert.equal(
    validarSSOMatrix(sso({ sytue_tenant: 'synthetic-org', sytue_subject: 'synthetic-sub' })).href,
    sso({ sytue_tenant: 'synthetic-org', sytue_subject: 'synthetic-sub' }),
  );
  validarSSOMatrix(sso().replace('/oidc-sytue', ''));
  for (const target of [
    sso().replace('matrix.sytue.eu', 'matrix.sytue.eu.foreign.test'),
    sso().replace('https:', 'http:'),
    sso().replace('/login/sso/redirect', '/login/sso/other'),
    `${sso()}&redirectUrl=${encodeURIComponent(vueltaMatrix)}`,
    `${sso()}&other=1`,
    `${sso()}#private`,
    sso({ redirectUrl: 'https://foreign.test/matrix/callback/ABCDEFGH23456789' }),
    sso({ redirectUrl: 'http://localhost:41234/matrix/callback/ABCDEFGH23456789' }),
    sso({ redirectUrl: 'http://127.0.0.1:41234/auth/callback' }),
    sso({ redirectUrl: `${vueltaMatrix}?x=1` }),
    sso({ redirectUrl: 'http://127.0.0.1/matrix/callback/ABCDEFGH23456789' }),
  ])
    assert.throws(() => validarSSOMatrix(target), /matrix_url/);
});

test('la vuelta del SSO de Matrix es la pedida y solo lleva su loginToken', () => {
  assert.equal(validarRetornoMatrix(sso(), conToken), conToken);
  for (const target of [
    conToken.replace(':41234', ':41235'),
    conToken.replace('ABCDEFGH23456789', 'OTRONONCE2345678'),
    `${conToken}&loginToken=second`,
    `${conToken}&other=1`,
    `${conToken}#private`,
    vueltaMatrix,
    `${vueltaMatrix}?loginToken=`,
  ])
    assert.throws(() => validarRetornoMatrix(sso(), target), /matrix_callback/);
});

function fakeMatrix({ aterriza = 'vuelta', vuelta = conToken } = {}) {
  const calls = { gotos: [], fulfilled: [], aborted: 0, closed: 0, screenshots: 0, fills: [] };
  let handler;
  let current;
  let go;
  const page = {
    route: async (_, value) => {
      handler = value;
    },
    on: () => {},
    fill: async (selector, value) => {
      calls.fills.push({ selector, value });
    },
    click: async () => {
      if (aterriza === 'stepup') await go(vuelta);
    },
    waitForURL: async () => {},
    goto: async (value) => {
      calls.gotos.push(value);
      go = async (target) =>
        handler({
          request: () => ({
            method: () => 'GET',
            url: () => target,
            isNavigationRequest: () => true,
          }),
          abort: async () => {
            calls.aborted++;
          },
          continue: async () => {
            current = target;
          },
          fulfill: async (r) => {
            calls.fulfilled.push(r);
          },
        });
      if (aterriza === 'vuelta') await go(vuelta);
      if (aterriza === 'login')
        await go('https://id.sytue.eu/login/?return_to=%2Foauth2%2Fauthorize');
      if (aterriza === 'stepup')
        await go('https://id.sytue.eu/login/?step_up=1&return_to=%2Foauth2%2Fauthorize');
      if (aterriza === 'fuera') await go('https://foreign.test/');
      return { text: async () => '' };
    },
    url: () => current ?? 'https://matrix.sytue.eu/',
    screenshot: async () => {
      calls.screenshots++;
    },
    close: async () => {
      calls.closed++;
    },
  };
  return { context: { newPage: async () => page }, calls };
}

test('el SSO de Matrix recoge la vuelta sin entregarla y cierra la página', async () => {
  const { context, calls } = fakeMatrix();
  assert.equal(await autorizarMatrix(context, sso(), { timeoutMs: 1_000 }), conToken);
  assert.equal(calls.fulfilled.length, 1);
  assert.equal(calls.closed, 1);
});

test('el SSO de Matrix no teclea nada si Sytue ID pide entrar', async () => {
  const { context } = fakeMatrix({ aterriza: 'login' });
  await assert.rejects(autorizarMatrix(context, sso(), { timeoutMs: 1_000 }), /matrix_login/);
});

test('si Sytue ID pide solo el segundo factor, el SSO de Matrix lo da una vez y sigue', async () => {
  const { context, calls } = fakeMatrix({ aterriza: 'stepup' });
  assert.equal(
    await autorizarMatrix(context, sso(), {
      timeoutMs: 1_000,
      semilla: 'synthetic-otp',
      codigoTOTP: () => '000000',
    }),
    conToken,
  );
  assert.deepEqual(calls.fills, [{ selector: 'input[name=code]', value: '000000' }]);
});

test('sin semilla, el segundo factor del SSO de Matrix no se inventa', async () => {
  const { context, calls } = fakeMatrix({ aterriza: 'stepup' });
  await assert.rejects(autorizarMatrix(context, sso(), { timeoutMs: 1_000 }), /matrix_login/);
  assert.deepEqual(calls.fills, []);
});

test('el SSO de Matrix no navega fuera del homeserver y de Sytue ID', async () => {
  const { context, calls } = fakeMatrix({ aterriza: 'fuera' });
  await assert.rejects(autorizarMatrix(context, sso(), { timeoutMs: 1_000 }), /matrix_origin/);
  assert.equal(calls.aborted, 1);
});

test('una vuelta de Matrix con otro nonce no sale del arnés', async () => {
  const { context } = fakeMatrix({ vuelta: conToken.replace('ABCDEFGH23456789', 'OTRO2345') });
  // El origen es el mismo (la ruta la recoge), pero no es la vuelta pedida.
  await assert.rejects(autorizarMatrix(context, sso(), { timeoutMs: 1_000 }), /matrix_callback/);
});

test('el SSO de Matrix que no vuelve se corta con su código', async () => {
  let codigo = '';
  const { context, calls } = fakeMatrix({ aterriza: 'nada' });
  await assert.rejects(
    autorizarMatrix(context, sso(), {
      timeoutMs: 300,
      alFallar: (_, code) => {
        codigo = code;
      },
    }),
    /matrix_timeout/,
  );
  assert.equal(codigo, 'matrix_timeout');
  assert.equal(calls.closed, 1);
});
