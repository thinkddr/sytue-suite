// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
import assert from 'node:assert/strict';
import test from 'node:test';
import {
  autorizarDesktop,
  codigoPublico,
  validarAutorizacion,
  validarRetorno,
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
  })}`;
const returned = `${callback}?code=synthetic-code&state=synthetic-state`;

function fake({
  landing = 'https://id.sytue.eu/login/',
  responseLocation = '/',
  secondLocation = '/',
  callbackLocation = returned,
  failedGoto = false,
  foreignPost = false,
} = {}) {
  const calls = { fills: [], gotos: [], posts: [], aborted: 0, closed: 0, otp: 0, organization: 0 };
  let current;
  let handler;
  let clicks = 0;
  const page = {
    route: async (_, value) => {
      handler = value;
    },
    goto: async (value) => {
      calls.gotos.push(value);
      if (failedGoto)
        throw new Error('sensitive-error https://foreign.test/?secret=synthetic-password');
      current = value.includes('oauth2/authorize') ? landing : value;
    },
    url: () => current,
    waitForSelector: async () => {},
    fill: async (selector, value) => {
      calls.fills.push({ selector, value });
    },
    click: async () => {
      clicks++;
      const target = foreignPost
        ? 'https://foreign.test/steal'
        : `https://id.sytue.eu/login/${clicks === 1 ? '' : 'mfa'}`;
      calls.posts.push(target);
      await handler({
        request: () => ({
          method: () => 'POST',
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
        fulfill: async () => {},
      });
    },
    waitForURL: async () => {},
    close: async () => {
      calls.closed++;
    },
  };
  const context = {
    newPage: async () => page,
    request: {
      get: async (value, options) => {
        assert.equal(value, authorization());
        assert.deepEqual(options, { maxRedirects: 0 });
        return { headers: () => ({ location: callbackLocation }) };
      },
    },
  };
  return { context, calls };
}
const options = () => ({
  email: 'synthetic@invalid.test',
  clave: 'synthetic-password',
  timeoutMs: 2_000,
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

test('login correcto entrega el callback por IPC y cierra la página', async () => {
  const { context, calls } = fake();
  assert.equal(await autorizarDesktop(context, authorization(), options()), returned);
  assert.equal(calls.fills.length, 2);
  assert.equal(calls.closed, 1);
  assert.deepEqual(calls.gotos, [authorization()]);
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
  assert.equal(await autorizarDesktop(context, authorization(), opts), returned);
  assert.equal(calls.otp, 1);
  assert.equal(calls.fills.at(-1).selector, 'input[name=code]');
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
    returned,
  );
  assert.equal(calls.organization, 1);
  assert.deepEqual(calls.gotos, [authorization(), 'https://one.sytue.eu/one/']);
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
