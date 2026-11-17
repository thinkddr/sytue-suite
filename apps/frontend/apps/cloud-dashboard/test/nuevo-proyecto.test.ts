/**
 * La lógica del asistente «Nuevo proyecto» que no se ve: la validación de las
 * variables y, sobre todo, el alta REANUDABLE. Lo que importa es que un fallo a
 * mitad no deje al usuario sin salida: reintentar no vuelve a crear el proyecto
 * (sería un 409 contra su propio nombre) ni a guardar lo ya guardado, y nunca se
 * despliega sin la variable que falló. Ejecutar: `bun test`.
 */
import { expect, test } from 'bun:test';
import {
  INITIAL_PIPELINE,
  looksSecret,
  mergeParsed,
  repoName,
  runPipeline,
  toSlug,
  validRepoUrl,
  varErrors,
  filledVars,
  type PipelineInput,
  type PipelineState,
  type VarRow,
} from '../src/lib/nuevo-proyecto.js';

const input: PipelineInput = {
  project: {
    slug: 'tienda',
    name: 'Tienda',
    repo_url: 'https://git.sytue.eu/acme/tienda',
    framework: 'astro',
    build_strategy: '',
    build_command: '',
    install_command: '',
    output_dir: '',
    production_ref: '',
    root_dir: '',
  },
  vars: [
    { key: 'DATABASE_URL', value: 'postgres://x', secret: true },
    { key: 'API_KEY', value: 'abc', secret: true },
    { key: 'PUBLIC_SITE', value: 'https://tienda.eu', secret: false },
  ],
};

/** Control-plane falso: apunta cada llamada y falla donde se le diga. */
function fakePost(failOn?: (url: string, body: Record<string, unknown>) => boolean) {
  const calls: Array<{ url: string; body: Record<string, unknown> }> = [];
  const post = async (url: string, body: Record<string, unknown>) => {
    calls.push({ url, body });
    if (failOn?.(url, body))
      throw Object.assign(new Error('bff_402'), { status: 402, code: 'quota_exceeded' });
    if (url === '/api/projects')
      return {
        ok: true,
        live: true,
        project: { slug: 'tienda', name: 'Tienda', webhookSecret: 's3cr3t' },
      };
    return { ok: true, live: true };
  };
  return { calls, post };
}

test('el alta completa va en orden: proyecto, cada variable y el despliegue', async () => {
  const { calls, post } = fakePost();
  const seen: PipelineState[] = [];
  const s = await runPipeline(input, INITIAL_PIPELINE, post, (x) => seen.push(x));
  expect(calls.map((c) => c.url)).toEqual([
    '/api/projects',
    '/api/env',
    '/api/env',
    '/api/env',
    '/api/deploy',
  ]);
  expect(calls[1]!.body).toEqual({
    project: 'tienda',
    key: 'DATABASE_URL',
    value: 'postgres://x',
    secret: true,
    scope: 'all',
  });
  expect(calls[4]!.body).toEqual({ project: 'tienda' });
  expect(s).toMatchObject({ project: 'done', vars: 'done', varsSaved: 3, deploy: 'done' });
  expect(s.created?.webhookSecret).toBe('s3cr3t');
  expect(s.error).toBeUndefined();
  // El progreso se emite mientras ocurre, no solo al final.
  expect(seen.some((x) => x.vars === 'running' && x.varsSaved === 1)).toBe(true);
});

test('si falla una variable se para ahí: no se despliega sin ella', async () => {
  const { calls, post } = fakePost((url, body) => url === '/api/env' && body.key === 'API_KEY');
  const s = await runPipeline(input, INITIAL_PIPELINE, post, () => {});
  expect(calls.map((c) => c.url)).not.toContain('/api/deploy');
  expect(s).toMatchObject({ project: 'done', vars: 'failed', varsSaved: 1, deploy: 'pending' });
  expect(s.error).toEqual({ phase: 'vars', key: 'API_KEY', status: 402, code: 'quota_exceeded' });
});

test('reintentar sigue donde se paró: ni recrea el proyecto ni repite variables', async () => {
  const first = fakePost((url, body) => url === '/api/env' && body.key === 'API_KEY');
  const stopped = await runPipeline(input, INITIAL_PIPELINE, first.post, () => {});

  const second = fakePost();
  const s = await runPipeline(input, stopped, second.post, () => {});
  expect(second.calls.map((c) => `${c.url} ${String(c.body.key ?? '')}`.trim())).toEqual([
    '/api/env API_KEY',
    '/api/env PUBLIC_SITE',
    '/api/deploy',
  ]);
  expect(s).toMatchObject({ project: 'done', vars: 'done', varsSaved: 3, deploy: 'done' });
  expect(s.error).toBeUndefined();
  // El secreto del webhook llegó en el primer intento y no se pierde al reintentar.
  expect(s.created?.webhookSecret).toBe('s3cr3t');
});

test('un fallo del alta se queda en el proyecto, con su código real', async () => {
  const { calls, post } = fakePost((url) => url === '/api/projects');
  const s = await runPipeline(input, INITIAL_PIPELINE, post, () => {});
  expect(calls).toHaveLength(1);
  expect(s.project).toBe('failed');
  expect(s.error).toMatchObject({ phase: 'project', status: 402, code: 'quota_exceeded' });
});

test('sin control-plane no se simula nada: ni variables ni despliegue', async () => {
  const calls: string[] = [];
  const s = await runPipeline(
    input,
    INITIAL_PIPELINE,
    async (url) => {
      calls.push(url);
      return { ok: true, live: false, project: { slug: 'tienda' } };
    },
    () => {},
  );
  expect(calls).toEqual(['/api/projects']);
  expect(s).toMatchObject({ project: 'pending', live: false, deploy: 'pending' });
});

test('si solo falla el despliegue, reintentar solo vuelve a desplegar', async () => {
  const first = fakePost((url) => url === '/api/deploy');
  const stopped = await runPipeline(input, INITIAL_PIPELINE, first.post, () => {});
  expect(stopped).toMatchObject({ vars: 'done', deploy: 'failed' });
  const second = fakePost();
  await runPipeline(input, stopped, second.post, () => {});
  expect(second.calls.map((c) => c.url)).toEqual(['/api/deploy']);
});

const row = (id: number, key: string, value: string, secret = false): VarRow => ({
  id,
  key,
  value,
  secret,
});

test('variables: formato de clave, valor vacío y claves repetidas, por fila', () => {
  const errs = varErrors([
    row(1, 'OK_KEY', 'x'),
    row(2, '1MAL', 'x'),
    row(3, 'SIN_VALOR', '  '),
    row(4, 'DUP', 'a'),
    row(5, 'DUP', 'b'),
    row(6, '', ''), // la fila recién añadida no es un error
  ]);
  expect([...errs.entries()]).toEqual([
    [2, 'keyFormat'],
    [3, 'valueRequired'],
    [4, 'duplicate'],
    [5, 'duplicate'],
  ]);
  expect(filledVars([row(1, ' A ', 'v'), row(2, '', '')])).toEqual([
    { key: 'A', value: 'v', secret: false },
  ]);
});

test('pegar un .env actualiza las claves que ya estaban y marca los secretos', () => {
  let id = 10;
  const out = mergeParsed(
    [row(1, 'API_KEY', 'viejo', true), row(2, '', '')],
    [
      { key: 'API_KEY', value: 'nuevo' },
      { key: 'DB_PASSWORD', value: 'p' },
      { key: 'PUBLIC_URL', value: 'https://x' },
    ],
    () => id++,
  );
  expect(out).toEqual([
    row(1, 'API_KEY', 'nuevo', true),
    row(10, 'DB_PASSWORD', 'p', true),
    row(11, 'PUBLIC_URL', 'https://x', false),
  ]);
});

test('lo que parece secreto y lo que el framework publica', () => {
  expect(looksSecret('STRIPE_SECRET_KEY')).toBe(true);
  expect(looksSecret('GITHUB_TOKEN')).toBe(true);
  expect(looksSecret('DATABASE_URL')).toBe(true);
  expect(looksSecret('NEXT_PUBLIC_API_KEY')).toBe(false);
  expect(looksSecret('PORT')).toBe(false);
  expect(looksSecret('NODE_ENV')).toBe(false);
});

test('origen: URL https del repositorio y nombre propuesto', () => {
  expect(validRepoUrl('https://git.sytue.eu/acme/tienda')).toBe(true);
  expect(validRepoUrl('git@git.sytue.eu:acme/tienda')).toBe(false);
  expect(validRepoUrl('https://')).toBe(false);
  expect(validRepoUrl('http://git.sytue.eu/acme/tienda')).toBe(false);
  expect(repoName('https://github.com/acme/mi-tienda.git')).toBe('mi-tienda');
  expect(repoName('https://git.sytue.eu/acme/web/')).toBe('web');
  expect(toSlug('Mi Tienda Rápida')).toBe('mi-tienda-rapida');
});
