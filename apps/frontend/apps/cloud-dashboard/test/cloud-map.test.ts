/**
 * Self-check de los datos del control-plane que la vista estaba TIRANDO. Cada
 * caso corresponde a una función que no se podía usar desde la web: conectar
 * «git push → despliegue» (secreto del webhook) y verificar un dominio propio
 * (registro TXT). Ejecutar: `bun test`.
 */
import { expect, test } from 'bun:test';
import type { Project as SdkProject } from '@sytue/api-clients';
import type {
  Deployment as ViewDeployment,
  ManagedService as ViewService,
} from '../src/lib/types.js';

// lib/cloud.js arrastra el wiring del BFF, que exige el entorno OIDC al importarse.
// Se rellena ANTES del import dinámico: nada aquí sale a la red (el fetch está
// interceptado), estos valores solo satisfacen la validación de arranque.
process.env.SYTUE_OIDC_ISSUER ??= 'https://id.sytue.test';
process.env.SYTUE_OIDC_CLIENT_ID ??= 'sytue-cloud';
process.env.SYTUE_OIDC_CLIENT_SECRET ??= 'test-secret';
process.env.SYTUE_OIDC_REDIRECT_URI ??= 'https://cloud.sytue.test/auth/callback';
process.env.SYTUE_SESSION_SECRET ??= 'x'.repeat(32);
process.env.SYTUE_CLOUD_API_URL ??= 'https://cloud.sytue.test';

const { mapProject, addDomain, servicesNotYetLive, loadUsage } =
  await import('../src/lib/cloud.js');

const base: SdkProject = {
  id: 'p1',
  slug: 'mi-app',
  name: 'Mi app',
  created_at: '2026-07-28T10:00:00Z',
};

test('mapProject conserva el secreto y la URL del webhook', () => {
  // El control-plane los entrega UNA sola vez, al crear: si el mapeo los pierde
  // aquí, no hay segunda oportunidad de leerlos en ninguna parte.
  const p = mapProject(
    { ...base, webhook_url: 'https://cloud.sytue.test/webhooks/git', webhook_secret: 's3cr3t' },
    [],
  );
  expect(p.webhookUrl).toBe('https://cloud.sytue.test/webhooks/git');
  expect(p.webhookSecret).toBe('s3cr3t');
});

test('sin webhook en la respuesta no se inventan campos vacíos', () => {
  // El listado no trae credenciales: la vista debe distinguir «no vino» de «vino
  // vacío» para no enseñar un secreto en blanco con pinta de real.
  const p = mapProject(base, []);
  expect(p.webhookSecret).toBeUndefined();
  expect(p.webhookUrl).toBeUndefined();
});

/** Sesión mínima: solo se usa el access token para la cabecera Bearer. */
const session = { tokens: { accessToken: 'tok' } } as Parameters<typeof addDomain>[0];

/** Intercepta las dos llamadas del alta: resolver el proyecto y crear el dominio. */
function stubFetch(addResponse: unknown): () => void {
  const real = globalThis.fetch;
  globalThis.fetch = (async (input: string | URL | Request) => {
    const url = String(input instanceof Request ? input.url : input);
    const body = url.includes('/domains') ? addResponse : { projects: [base] };
    return new Response(JSON.stringify(body), {
      status: 200,
      headers: { 'content-type': 'application/json' },
    });
  }) as typeof fetch;
  return () => {
    globalThis.fetch = real;
  };
}

test('addDomain devuelve el TXT de verificación, no solo un booleano', async () => {
  // Sin el TXT el dominio no pasa nunca de «sin verificar» —y sin verificar no hay
  // certificado—, así que perderlo aquí dejaba la función entera inalcanzable.
  const restore = stubFetch({
    domain: { id: 'd1', hostname: 'www.ejemplo.com', status: 'pending' },
    dns: {
      cname_target: 'ingress.sytue.eu',
      txt_record: '_sytue-challenge.www.ejemplo.com',
      txt_value: 'tok-abc123',
    },
  });
  try {
    const res = await addDomain(session, 'mi-app', 'www.ejemplo.com');
    expect(res.live).toBe(true);
    expect(res.domain.txtRecord).toBe('_sytue-challenge.www.ejemplo.com');
    expect(res.domain.txtValue).toBe('tok-abc123');
    // El CNAME sale de la configuración del control-plane, no de un literal de la UI.
    expect(res.domain.cname).toBe('ingress.sytue.eu');
    expect(res.domain.id).toBe('d1');
  } finally {
    restore();
  }
});

test('una respuesta sin instrucciones DNS no fabrica registros', async () => {
  // Copiar un registro inventado no verificaría nada: mejor no ofrecerlo.
  const restore = stubFetch({ domain: { id: 'd2', hostname: 'x.ejemplo.com', status: 'pending' } });
  try {
    const res = await addDomain(session, 'mi-app', 'x.ejemplo.com');
    expect(res.domain.txtValue).toBeUndefined();
    expect(res.domain.cname).toBeUndefined();
  } finally {
    restore();
  }
});

test('cero consumo NO se rellena con el seed de demostración', async () => {
  // Una organización recién creada consume 0. Devolver el seed aquí con live:true
  // dejaba mudo al DemoBanner y presentaba 128 min / 342 GB·h / 18 GB inventados
  // como consumo real, alimentando el estimador de coste de /usage y /billing.
  const real = globalThis.fetch;
  globalThis.fetch = (async (_input: string | URL | Request) =>
    new Response(JSON.stringify({ usage: [] }), {
      status: 200,
      headers: { 'content-type': 'application/json' },
    })) as typeof fetch;
  try {
    const res = await loadUsage(session, 'es');
    expect(res.live).toBe(true);
    expect(res.data).toEqual([]);
  } finally {
    globalThis.fetch = real;
  }
});

// --- «Tu app aún no ve este servicio» --------------------------------------
// Las credenciales de un servicio gestionado se inyectan AL DESPLEGAR, así que
// crear una base de datos con la app corriendo la deja aprovisionada y a la vez
// inalcanzable. El criterio decide cuándo avisar.

const dep = (createdAt: string, updatedAt?: string): ViewDeployment => ({
  id: 'd',
  project: 'p',
  status: 'ready',
  target: 'production',
  branch: 'main',
  commitSha: 'abc1234',
  commitMessage: 'x',
  author: '',
  url: 'https://x',
  createdAt,
  ...(updatedAt ? { updatedAt } : {}),
});

const svc = (createdAt: string, status: ViewService['status'] = 'running'): ViewService => ({
  kind: 'postgres',
  name: 'db',
  status,
  createdAt,
});

test('avisa del servicio creado DESPUÉS del despliegue vivo', () => {
  expect(
    servicesNotYetLive([svc('2026-08-03T12:00:00Z')], dep('2026-08-03T10:00:00Z')),
  ).toHaveLength(1);
});

test('no avisa del que ya existía cuando se desplegó', () => {
  expect(servicesNotYetLive([svc('2026-08-03T09:00:00Z')], dep('2026-08-03T10:00:00Z'))).toEqual(
    [],
  );
});

test('usa updatedAt y no createdAt: tras un rollback el despliegue SÍ tiene credenciales', () => {
  // Creado a las 08:00 y repromocionado a las 14:00; el servicio nació a las 12:00.
  // Mirando createdAt avisaría de más y mandaría a redesplegar sin motivo.
  expect(
    servicesNotYetLive(
      [svc('2026-08-03T12:00:00Z')],
      dep('2026-08-03T08:00:00Z', '2026-08-03T14:00:00Z'),
    ),
  ).toEqual([]);
});

test('calla mientras el servicio se aprovisiona: aún no hay credenciales que entregar', () => {
  expect(
    servicesNotYetLive([svc('2026-08-03T12:00:00Z', 'provisioning')], dep('2026-08-03T10:00:00Z')),
  ).toEqual([]);
});

test('sin despliegue vivo no avisa: no habría nada que redesplegar', () => {
  expect(servicesNotYetLive([svc('2026-08-03T12:00:00Z')], undefined)).toEqual([]);
});
