// Check de que el SDK de Sytue DevX desenvuelve el sobre de las listas del
// control plane ({"projects":[...]}, {"deployments":[...]}, {"services":[...]},
// {"env":[...]}) a un array plano. Regresión: antes casteaba el sobre a array,
// con lo que .find()/.map() en el consumidor reventaba y la carga live caía a
// seed (ids no-UUID), dejando el rollback inerte. Ejecutar: `bun test`.
import { expect, test } from 'bun:test';
import { CloudClient, type SizingAdvice, type Deployment, type ManagedService } from './index.js';

/** CloudClient con un fetch inyectado que siempre responde `body` (200 JSON). */
function clientReturning(body: unknown): CloudClient {
  const fetchImpl = (async () =>
    new Response(JSON.stringify(body), {
      status: 200,
      headers: { 'content-type': 'application/json' },
    })) as unknown as typeof fetch;
  return new CloudClient({
    baseUrl: 'https://cloud.test',
    auth: { getAccessToken: () => 'tok' },
    fetchImpl,
  });
}

test('listProjects desenvuelve {projects:[...]}', async () => {
  const c = clientReturning({
    projects: [{ id: 'p1', slug: 's', name: 'S', organization: 'org/x', created_at: '' }],
  });
  const out = await c.listProjects();
  expect(Array.isArray(out)).toBe(true);
  expect(out).toHaveLength(1);
  expect(out[0]!.id).toBe('p1');
});

test('listDeployments desenvuelve {deployments:[...]} e ignora next_cursor', async () => {
  const c = clientReturning({
    deployments: [{ id: 'd1', projectId: 'p1', env: 'production', status: 'ready', createdAt: '' }],
    next_cursor: 'abc',
  });
  const out = await c.listDeployments('p1');
  expect(out).toHaveLength(1);
  expect(out[0]!.id).toBe('d1');
});

test('listServices y listEnv desenvuelven sus sobres', async () => {
  const services = await clientReturning({
    services: [
      { id: 'sv1', projectId: 'p1', kind: 'postgres', name: 'db', status: 'ready', createdAt: '' },
    ],
  }).listServices('p1');
  expect(services).toHaveLength(1);

  const env = await clientReturning({ env: [{ key: 'K', secret: false }] }).listEnv('p1');
  expect(env).toHaveLength(1);
  expect(env[0]!.key).toBe('K');
});

test('sobre vacío o ausente -> array vacío (no lanza)', async () => {
  expect(await clientReturning({}).listProjects()).toEqual([]);
  expect(await clientReturning({ deployments: null }).listDeployments('p1')).toEqual([]);
});

test('listUptimeChecks desenvuelve {uptime_checks:[...]}', async () => {
  const c = clientReturning({
    uptime_checks: [
      {
        id: 'u1',
        project_id: 'p1',
        name: 'API',
        target_url: 'https://x.eu',
        interval_seconds: 60,
        enabled: true,
        next_run_at: '',
        last_status: 'up',
        last_latency_ms: 42,
        created_at: '',
      },
    ],
  });
  const out = await c.listUptimeChecks('p1');
  expect(out).toHaveLength(1);
  expect(out[0]!.id).toBe('u1');
  expect(await clientReturning({}).listUptimeChecks('p1')).toEqual([]);
});

test('listAlertPolicies desenvuelve {alert_policies:[...]}', async () => {
  const c = clientReturning({
    alert_policies: [
      {
        id: 'a1',
        event: 'deploy.failed',
        channel_url: 'https://hooks.slack.com/x',
        enabled: true,
        created_at: '',
      },
    ],
  });
  const out = await c.listAlertPolicies();
  expect(out).toHaveLength(1);
  expect(out[0]!.event).toBe('deploy.failed');
  expect(await clientReturning({}).listAlertPolicies()).toEqual([]);
});

test('setEnv envía is_secret y scope, que son los nombres que decodifica Go', async () => {
  // Se enviaba `secret`, un campo que el control-plane descarta en silencio: TODA
  // variable guardada desde la web quedaba como NO secreta y con el ámbito por
  // defecto, justo lo contrario de lo que había pedido el usuario.
  let sent: Record<string, unknown> = {};
  const fetchImpl = (async (_url: string, init?: RequestInit) => {
    sent = JSON.parse(String(init?.body ?? '{}')) as Record<string, unknown>;
    return new Response('{}', { status: 200, headers: { 'content-type': 'application/json' } });
  }) as unknown as typeof fetch;
  const c = new CloudClient({
    baseUrl: 'https://cloud.test',
    auth: { getAccessToken: () => 'tok' },
    fetchImpl,
  });
  await c.setEnv('p1', { key: 'API_TOKEN', value: 'v', secret: true, scope: 'preview' });
  expect(sent.is_secret).toBe(true);
  expect(sent.scope).toBe('preview');
  expect(sent.secret).toBeUndefined();
});

test('createProjectFromTemplate envía EXACTAMENTE los campos que decodifica Go', async () => {
  // El fallo que ya se coló una vez (setEnv mandando `secret` donde Go lee
  // `is_secret`) no lo caza una lista escrita a mano en el test: quien elige mal el
  // nombre en el SDK lo elige igual de mal aquí. Así que la referencia se LEE del
  // struct Go; si alguno de los dos lados cambia, esto se pone rojo.
  const goSrc = await Bun.file(
    `${import.meta.dir}/../../../../../../services/cloud/internal/http/handlers_from_template.go`,
  ).text();
  const struct = goSrc.match(/type createFromTemplateRequest struct \{([\s\S]*?)\n\}/)?.[1];
  expect(struct).toBeTruthy();
  const goFields = [...struct!.matchAll(/`json:"([^",]+)/g)].map((m) => m[1]!).sort();

  let sent: Record<string, unknown> = {};
  let url = '';
  const fetchImpl = (async (u: string, init?: RequestInit) => {
    url = u;
    sent = JSON.parse(String(init?.body ?? '{}')) as Record<string, unknown>;
    return new Response('{}', { status: 201, headers: { 'content-type': 'application/json' } });
  }) as unknown as typeof fetch;
  const c = new CloudClient({
    baseUrl: 'https://cloud.test',
    auth: { getAccessToken: () => 'tok' },
    fetchImpl,
  });
  await c.createProjectFromTemplate({
    template_id: 'go-api-postgres',
    slug: 'mi-api',
    name: 'Mi API',
    repo_url: 'https://github.com/acme/api',
    answers: { REGION: 'eu' },
  });

  expect(url).toBe('https://cloud.test/api/v1/projects:from-template');
  expect(Object.keys(sent).sort()).toEqual(goFields);
  expect(sent.template_id).toBe('go-api-postgres');
  expect(sent.repo_url).toBe('https://github.com/acme/api');
  expect(sent.answers).toEqual({ REGION: 'eu' });
});

test('addDomain devuelve las instrucciones DNS completas, TXT incluido', async () => {
  // El tipo solo declaraba cname_target, así que el TXT de verificación —lo único
  // que permite verificar un dominio propio— se caía por el camino.
  const c = clientReturning({
    domain: { id: 'd1', project_id: 'p1', hostname: 'www.ejemplo.com', status: 'pending' },
    dns: {
      cname_target: 'ingress.sytue.eu',
      txt_record: '_sytue-challenge.www.ejemplo.com',
      txt_value: 'tok-abc123',
    },
  });
  const out = await c.addDomain('p1', 'www.ejemplo.com');
  expect(out.dns?.txt_record).toBe('_sytue-challenge.www.ejemplo.com');
  expect(out.dns?.txt_value).toBe('tok-abc123');
  expect(out.dns?.cname_target).toBe('ingress.sytue.eu');
});

test('SizingAdvice declara EXACTAMENTE los campos que serializa Go', async () => {
  // Misma red que arriba, en el sentido de LECTURA: aquí el riesgo no es enviar un
  // nombre mal —eso da un 400 ruidoso— sino declarar `monthlyDeltaEur` donde Go
  // manda `monthly_delta_eur` y que el ahorro salga `undefined` en la tarjeta sin
  // que nada falle. Es el mismo bug latente que ya tienen ManagedService y
  // Deployment.
  const goSrc = await Bun.file(
    `${import.meta.dir}/../../../../../../services/cloud/internal/domain/rightsizing.go`,
  ).text();
  const struct = goSrc.match(/type SizingAdvice struct \{([\s\S]*?)\n\}/)?.[1];
  expect(struct).toBeTruthy();
  const goFields = [...struct!.matchAll(/`json:"([^",]+)/g)].map((m) => m[1]!).sort();

  // Un objeto con todas las claves del tipo: si al tipo le falta una que Go manda,
  // TypeScript no compila; si le sobra, tampoco.
  const advice: SizingAdvice = {
    action: 'down',
    reason: 'over_provisioned',
    from: 'standard',
    to: 'basic',
    monthly_delta_eur: -3.285,
    p95_memory_percent: 12,
    peak_memory_percent: 30,
    p95_cpu_milli: 50,
    samples: 400,
    window_hours: 100,
    // Opcional en Go (`omitempty`) pero se declara igual: si el SDK no lo tuviera,
    // el «las apps como la tuya usan el 12 %» saldría undefined sin que nada fallara.
    benchmark: { p50_memory_percent: 12, p90_memory_percent: 30, tenants: 8, projects: 19 },
  };
  expect(Object.keys(advice).sort()).toEqual(goFields);
});

test('Deployment y ManagedService declaran los campos que serializa Go', async () => {
  // El bug que esto cierra: el SDK declaraba `projectId`, `commitSha`, `createdAt`
  // y un `env` que NO EXISTE (Go manda `target`). Nada fallaba — los campos salían
  // undefined en silencio, que es la peor forma de romperse. Misma familia que el
  // `secret` / `is_secret` que dejó sin cifrar todas las variables creadas desde la
  // web.
  const goSrc = await Bun.file(
    `${import.meta.dir}/../../../../../../services/cloud/internal/domain/domain.go`,
  ).text();

  const tagsOf = (name: string) => {
    const struct = goSrc.match(new RegExp(`type ${name} struct \\{([\\s\\S]*?)\\n\\}`))?.[1];
    expect(struct).toBeTruthy();
    // `json:"-"` es un campo que Go NO serializa (Tenant): no debe estar en el SDK.
    return [...struct!.matchAll(/`json:"([^",]+)/g)].map((m) => m[1]!).filter((f) => f !== '-');
  };

  // Los alias camelCase siguen declarados como @deprecated para no romper a los
  // consumidores de golpe, así que la comprobación es de INCLUSIÓN: todo lo que Go
  // manda tiene que estar. Lo que sobra en el SDK son los alias, y se listan aquí
  // para que añadir uno nuevo sea una decisión consciente.
  const LEGACY = new Set([
    'projectId',
    'createdAt',
    'commitSha',
    'prNumber',
    'finishedAt',
    'env',
    'displayName',
    'repoUrl',
  ]);

  const check = (goType: string, sdkKeys: string[]) => {
    const goFields = tagsOf(goType);
    const missing = goFields.filter((f) => !sdkKeys.includes(f));
    expect(missing).toEqual([]);
    const extra = sdkKeys.filter((k) => !goFields.includes(k) && !LEGACY.has(k));
    expect(extra).toEqual([]);
  };

  const deployment: Deployment = {
    id: 'd',
    project_id: 'p',
    target: 'production',
    status: 'ready',
    url: 'https://x',
    commit_sha: 'abc',
    commit_message: 'fix: el carrito',
    git_ref: 'main',
    build_id: 'b',
    image_ref: '',
    pr_number: 1,
    replicas: 1,
    created_by: 'sub',
    created_at: 't',
    updated_at: 't',
    ready_at: 't',
    error: '',
  };
  check('Deployment', Object.keys(deployment));

  const service: ManagedService = {
    id: 's',
    project_id: 'p',
    kind: 'postgres',
    name: 'db',
    status: 'ready',
    config: {},
    target: 'production',
    created_at: 't',
    updated_at: 't',
  };
  check('ManagedService', Object.keys(service));
});
