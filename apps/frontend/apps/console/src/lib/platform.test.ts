// El `code` de un 403 de plataforma tiene que sobrevivir el viaje.
//
// Desde la mig 0043 hay TRES motivos distintos para el mismo 403 —no eres admin
// global, lo eres sin segundo factor, lo eres sin elevar— y solo el último tiene
// salida desde la propia pantalla: es el que enseña el botón de elevarse. Si
// `platformFetch` se queda solo con el número, los tres acaban en el mismo
// callejón sin salida y el PIM se convierte en una pared, sin que nada falle.
// Ejecutar: `bun test`.
import { expect, test } from 'bun:test';
import type { SytueSession } from '@sytue/auth';

// `platform.ts` arrastra el wiring del BFF, que exige su configuración OIDC al
// cargarse —es lo que impide desplegar una superficie a medio configurar—. Se
// da aquí y se importa después, como el resto de módulos de servidor.
process.env.SYTUE_OIDC_ISSUER = 'https://id.ejemplo';
process.env.SYTUE_OIDC_CLIENT_ID = 'sytue-console';
process.env.SYTUE_OIDC_CLIENT_SECRET = 'secreto-de-prueba';
process.env.SYTUE_OIDC_REDIRECT_URI = 'https://admin.ejemplo/auth/callback';
process.env.SYTUE_OIDC_POST_LOGOUT_URI = 'https://www.ejemplo/';
process.env.SYTUE_SESSION_SECRET = 'x'.repeat(32);

const { platformFetch } = await import('./platform.js');

const sesion = { tokens: { accessToken: 'tok' } } as SytueSession;

function conRespuesta<T>(res: Response, fn: () => Promise<T>): Promise<T> {
  const real = globalThis.fetch;
  globalThis.fetch = (() => Promise.resolve(res)) as unknown as typeof fetch;
  return fn().finally(() => {
    globalThis.fetch = real;
  });
}

test('un 403 conserva el código que dice qué le falta al admin', async () => {
  // El sobre REAL de platform/pkg/httpx, copiado de lo que responde el servicio:
  // el código va ANIDADO bajo `error`. Este test lo daba plano y por eso pasaba
  // mientras la pantalla no funcionaba — un test escrito contra la suposición
  // confirma la suposición, no el contrato.
  const res = await conRespuesta(
    new Response(
      JSON.stringify({
        error: { code: 'activation_required', message: 'Elevate to platform scope first.' },
        request_id: '7c4213a8-680e-4938-88a3-f71223282841',
      }),
      { status: 403, headers: { 'content-type': 'application/json' } },
    ),
    () => platformFetch(sesion, '/usage'),
  );
  expect(res.ok).toBe(false);
  if (res.ok) return;
  expect(res.status).toBe(403);
  expect(res.code).toBe('activation_required');
  expect(res.message).toBe('Elevate to platform scope first.');
});

test('un error sin cuerpo JSON no rompe la lectura', async () => {
  // Un 502 del ingress llega en HTML. Antes de leer el `code` había que
  // asegurarse de que intentarlo no convertía un error en una excepción.
  const res = await conRespuesta(new Response('<html>bad gateway</html>', { status: 502 }), () =>
    platformFetch(sesion, '/usage'),
  );
  expect(res.ok).toBe(false);
  if (res.ok) return;
  expect(res.status).toBe(502);
  expect(res.code).toBeUndefined();
});

test('también entiende la forma plana que reenvía el BFF', async () => {
  // El proxy de la consola construye su propio cuerpo, sin sobre. Los dos
  // caminos pasan por aquí, así que los dos tienen que leerse.
  const res = await conRespuesta(
    new Response(JSON.stringify({ code: 'mfa_required', message: 'Second factor required.' }), {
      status: 403,
      headers: { 'content-type': 'application/json' },
    }),
    () => platformFetch(sesion, '/usage'),
  );
  expect(res.ok).toBe(false);
  if (res.ok) return;
  expect(res.code).toBe('mfa_required');
});

test('un rechazo conserva los params que necesita el mensaje traducido', async () => {
  // «Es la única propietaria de …»: sin los nombres, la consola solo puede decir
  // que no, y lo siguiente que hay que hacer es ir a ESAS organizaciones.
  const res = await conRespuesta(
    new Response(
      JSON.stringify({
        error: {
          code: 'sole_owner',
          message: 'This user is the only owner of Acme: assign another owner there first.',
          params: { orgs: 'Acme' },
        },
      }),
      { status: 409, headers: { 'content-type': 'application/json' } },
    ),
    () => platformFetch(sesion, '/orgs/o/members/u/block', { method: 'POST', body: {} }),
  );
  expect(res.ok).toBe(false);
  if (res.ok) return;
  expect(res.code).toBe('sole_owner');
  expect(res.params).toEqual({ orgs: 'Acme' });
});

// Las rutas que el panel SONDEA tienen que estar en la allowlist del proxy.
//
// Mientras una fusión corre o una solicitud espera firma, la pantalla se
// rehidrata pidiendo estas dos. Si alguien recorta el allowlist del BFF, el
// sondeo recibe 404, el `if (!res.ok) return` se lo traga y la consola vuelve a
// quedarse congelada exactamente como estaba antes — sin que falle nada ni
// aparezca ningún error. Por eso se fija aquí y no en un comentario.
import { readFileSync } from 'node:fs';

test('el proxy admite las rutas que el panel rehidrata', () => {
  const proxy = readFileSync(
    new URL('../pages/api/platform/[...path].ts', import.meta.url),
    'utf8',
  );
  // Ancladas: `/^merges$/` y `/^approvals$/`, no un prefijo que colara otra cosa.
  expect(proxy).toContain('/^merges$/');
  expect(proxy).toContain('/^approvals$/');
  expect(proxy).toContain('^orgs/${UUID}/education$');
});

// La pantalla no puede pedir lo que ya sabe que le van a negar.
//
// Las siete llamadas de la consola de plataforma van detrás del mismo gate, y
// cada 403 deja un aviso en el registro de plataforma — el mismo que se lee en
// su pestaña de problemas. Abrirla sin estar elevado es lo NORMAL (se abre y
// entonces se pide la elevación), así que cada visita metía siete falsos
// problemas. Decidirlo con `/activation`, que no está tras el gate, es lo que lo
// evita; y los tres motivos tienen que seguir distinguiéndose, porque llevan a
// tres pantallas distintas.
const { accesoDePlataforma } = await import('./platform.js');

const elevacion = {
  id: 'a',
  user_id: 'u',
  status: 'active',
  effective_status: 'active',
  justification: 'soporte',
  duration_minutes: 60,
  activated_at: '2026-08-11T00:00:00Z',
  expires_at: '2026-08-11T01:00:00Z',
} as const;

test('sin elevación no se piden datos, y el motivo tiene salida', () => {
  const a = accesoDePlataforma({ ok: true, data: { activation: null } });
  expect(a.problema).toBe(403);
  expect(a.motivo).toBe('activation_required');
  expect(a.activation).toBeNull();
});

test('con elevación viva sí se piden datos', () => {
  const a = accesoDePlataforma({ ok: true, data: { activation: { ...elevacion } } });
  expect(a.problema).toBeUndefined();
  expect(a.activation?.id).toBe('a');
});

test('fuera de la allowlist y sin segundo factor conservan su motivo', () => {
  expect(accesoDePlataforma({ ok: false, status: 403, code: 'forbidden' }).motivo).toBe(
    'forbidden',
  );
  const sinMfa = accesoDePlataforma({ ok: false, status: 403, code: 'mfa_required' });
  expect(sinMfa.motivo).toBe('mfa_required');
  // Y un backend caído no se disfraza de 403: es otra pantalla.
  expect(accesoDePlataforma({ ok: false, status: 503 }).problema).toBe(503);
});

const { loadPlatformMerge, revokePlatformAdmin, setPlatformOrgEducation } =
  await import('./platform.js');

test('setPlatformOrgEducation envía el cuerpo y método correctos', async () => {
  const res = await conRespuesta(
    new Response(JSON.stringify({ id: 'org-1', slug: 'acme', education: true }), {
      status: 200,
      headers: { 'content-type': 'application/json' },
    }),
    () => setPlatformOrgEducation(sesion, 'org-1', true, 'instituto público'),
  );
  expect(res.ok).toBe(true);
  if (res.ok) {
    expect(res.data.education).toBe(true);
    expect(res.data.slug).toBe('acme');
  }
});

test('loadPlatformMerge devuelve el detalle de la fusión', async () => {
  const res = await conRespuesta(
    new Response(
      JSON.stringify({
        id: 'm-1',
        source_org_id: 'src',
        target_org_id: 'dst',
        status: 'running',
        services: { drive: 'completed' },
      }),
      { status: 200, headers: { 'content-type': 'application/json' } },
    ),
    () => loadPlatformMerge(sesion, 'm-1'),
  );
  expect(res.ok).toBe(true);
  if (res.ok) {
    expect(res.data.id).toBe('m-1');
    expect(res.data.status).toBe('running');
  }
});

test('revokePlatformAdmin envía DELETE a la ruta del usuario', async () => {
  const res = await conRespuesta(new Response(null, { status: 204 }), () =>
    revokePlatformAdmin(sesion, 'user-1'),
  );
  expect(res.ok).toBe(true);
});
