/**
 * Tests del SDK — su contrato es la promesa del producto: cero configuración,
 * ningún borrado sin destinatario, y errores que dicen QUÉ hacer.
 * Ejecutar: `bun test`.
 */
import { expect, test } from 'bun:test';
import { createClient, SytueError } from './index.js';
import { configFromEnv } from './config.js';
import { messages, normalizeLanguage, LANGUAGES } from './messages.js';
import { LANGUAGES as EXPORTED } from './index.js';

/** Captura la petición que sale y responde lo que se le diga. */
function stubFetch(reply: (req: Request) => Response | Promise<Response>): {
  calls: Request[];
  restore: () => void;
} {
  const real = globalThis.fetch;
  const calls: Request[] = [];
  globalThis.fetch = (async (input: RequestInfo | URL, init?: RequestInit) => {
    const req = new Request(input instanceof Request ? input : String(input), init);
    calls.push(req);
    return reply(req);
  }) as typeof fetch;
  return { calls, restore: () => (globalThis.fetch = real) };
}

const json = (body: unknown, status = 200, headers: Record<string, string> = {}) =>
  new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json', ...headers },
  });

const cliente = (language = 'es') =>
  createClient({ restUrl: 'https://api.ejemplo.test', restToken: 'tok', language });

// --- Cero configuración -----------------------------------------------------

test('la configuración sale del entorno que inyecta la plataforma', () => {
  // Es LA promesa del SDK: sin URL que copiar ni clave que pegar. El controlador
  // escribe estas variables en el Deployment al desplegar (controller/deploy.go).
  const cfg = configFromEnv({
    POSTGREST_URL: 'https://rest.interno',
    PGRST_JWT: 'jwt',
    SYTUE_PROJECT: 'mi-app',
    SYTUE_DEPLOYMENT_TARGET: 'preview',
  });
  expect(cfg.restUrl).toBe('https://rest.interno');
  expect(cfg.restToken).toBe('jwt');
  expect(cfg.project).toBe('mi-app');
  expect(cfg.target).toBe('preview');
});

test('sin base de datos el error dice qué hacer, no «fetch failed»', async () => {
  // El fallo número uno del primer día: crear la base con la app ya corriendo.
  // Las credenciales se inyectan AL DESPLEGAR, así que hay que redesplegar — y
  // eso es lo que tiene que leer quien se lo encuentra.
  const c = createClient({ language: 'es' });
  expect(c.hasDatabase).toBe(false);
  await expect(c.table('posts').all()).rejects.toThrow(/no tiene base de datos/i);
  await expect(c.table('posts').all()).rejects.toThrow(/vuelve a desplegar/i);
});

// --- Los mensajes, en los cinco idiomas ------------------------------------

test('el idioma por defecto es inglés, que es la base', async () => {
  // Sin LANG en el entorno: `createClient` lee el del proceso, así que en una
  // máquina en español el mensaje salía en español y este test fallaba sin que
  // el código estuviera mal. Se vio en el piloto (LANG=es_ES.UTF-8).
  const c = createClient({ language: 'en' });
  await expect(c.table('posts').all()).rejects.toThrow(/no database yet/i);
});

test('el idioma sale del entorno del contenedor', () => {
  // LANG lo trae cualquier imagen; SYTUE_LANG permite fijarlo sin tocar el resto.
  expect(configFromEnv({ LANG: 'fr_FR.UTF-8' }).language).toBe('fr_FR.UTF-8');
  expect(configFromEnv({ SYTUE_LANG: 'de', LANG: 'fr' }).language).toBe('de');
});

test('un locale completo se reduce a su idioma, y lo desconocido cae en inglés', () => {
  expect(normalizeLanguage('es-ES')).toBe('es');
  expect(normalizeLanguage('fr_FR.UTF-8')).toBe('fr');
  expect(normalizeLanguage('C')).toBe('en');
  expect(normalizeLanguage(undefined)).toBe('en');
});

test('los cinco catálogos están completos: ninguno cae a inglés por descuido', async () => {
  // Un idioma al que le falte una clave devolvería el texto en inglés sin que
  // nada fallara, y la función —«el error dice qué hacer»— desaparecería sin
  // ruido para ese cliente.
  // La lista pública y la interna tienen que ser la misma: si el paquete exporta
  // menos idiomas de los que tiene, nadie puede pedirlos por su nombre.
  expect(EXPORTED).toEqual(LANGUAGES);
  const base = messages('en');
  for (const lang of LANGUAGES) {
    const m = messages(lang);
    expect(Object.keys(m).sort()).toEqual(Object.keys(base).sort());
    if (lang === 'en') continue;
    expect(m.noDatabase).not.toBe(base.noDatabase);
    expect(m.noTable('posts')).not.toBe(base.noTable('posts'));
    expect(m.denied('posts')).not.toBe(base.denied('posts'));
  }
});

test('el error de tabla inexistente llega traducido', async () => {
  const s = stubFetch(() => json({ code: '42P01' }, 404));
  try {
    await expect(cliente('de').table('posts').all()).rejects.toThrow(/Es gibt keine Tabelle/);
    await expect(cliente('it').table('posts').all()).rejects.toThrow(/Non esiste la tabella/);
  } finally {
    s.restore();
  }
});

test('isPreview distingue una vista previa de producción', () => {
  expect(createClient({ target: 'preview' }).isPreview).toBe(true);
  expect(createClient({ target: 'production' }).isPreview).toBe(false);
  // Sin entorno declarado se asume producción: es la suposición segura para
  // decidir si mandar un correo de verdad.
  expect(createClient({}).isPreview).toBe(false);
});

// --- Nada de SQL ------------------------------------------------------------

test('all() con filtros construye la consulta sin que nadie escriba «eq.»', async () => {
  const s = stubFetch(() => json([{ id: 1 }]));
  try {
    await cliente().table('posts').where({ publicado: true }).newest().limit(5).all();
    const url = new URL(s.calls[0]!.url);
    expect(url.pathname).toBe('/posts');
    expect(url.searchParams.get('publicado')).toBe('eq.true');
    expect(url.searchParams.get('order')).toBe('created_at.desc');
    expect(url.searchParams.get('limit')).toBe('5');
    expect(s.calls[0]!.headers.get('authorization')).toBe('Bearer tok');
  } finally {
    s.restore();
  }
});

test('where({ campo: null }) pregunta por nulo, no por la cadena «null»', async () => {
  const s = stubFetch(() => json([]));
  try {
    await cliente().table('posts').where({ borrado_el: null }).all();
    expect(new URL(s.calls[0]!.url).searchParams.get('borrado_el')).toBe('is.null');
  } finally {
    s.restore();
  }
});

test('search busca la palabra, no obliga a escribir un patrón', async () => {
  const s = stubFetch(() => json([]));
  try {
    await cliente().table('posts').search('titulo', 'hola').all();
    expect(new URL(s.calls[0]!.url).searchParams.get('titulo')).toBe('ilike.*hola*');
  } finally {
    s.restore();
  }
});

test('first() devuelve null en vez de lanzar cuando no hay nada', async () => {
  const s = stubFetch(() => json([]));
  try {
    expect(await cliente().table('posts').first()).toBeNull();
  } finally {
    s.restore();
  }
});

test('encadenar no arrastra filtros de una consulta anterior', async () => {
  // Las APIs encadenadas que MUTAN producen este fallo: guardas la consulta base
  // en una variable, la reutilizas y aparecen filtros que no pusiste.
  const s = stubFetch(() => json([]));
  try {
    const base = cliente().table('posts').where({ autor: 'ana' });
    await base.where({ publicado: true }).all();
    await base.all();
    const segunda = new URL(s.calls[1]!.url).searchParams;
    expect(segunda.get('autor')).toBe('eq.ana');
    expect(segunda.get('publicado')).toBeNull();
  } finally {
    s.restore();
  }
});

// --- Ningún cambio sin destinatario ----------------------------------------

test('add devuelve la fila creada CON su id, no un 201 vacío', async () => {
  const s = stubFetch(() => json([{ id: 7, titulo: 'Hola' }], 201));
  try {
    const fila = await cliente().table('posts').add({ titulo: 'Hola' });
    expect(fila).toEqual({ id: 7, titulo: 'Hola' });
    expect(s.calls[0]!.headers.get('prefer')).toContain('return=representation');
  } finally {
    s.restore();
  }
});

test('update apunta a UNA fila: no existe la forma de reescribir la tabla', async () => {
  const s = stubFetch(() => json([{ id: 3, titulo: 'Otro' }]));
  try {
    await cliente().table('posts').update(3, { titulo: 'Otro' });
    const req = s.calls[0]!;
    expect(req.method).toBe('PATCH');
    expect(new URL(req.url).searchParams.get('id')).toBe('eq.3');
  } finally {
    s.restore();
  }
});

test('remove apunta a UNA fila y dice si existía', async () => {
  const s = stubFetch(() => json([{ id: 3 }]));
  try {
    expect(await cliente().table('posts').remove(3)).toBe(true);
    expect(new URL(s.calls[0]!.url).searchParams.get('id')).toBe('eq.3');
  } finally {
    s.restore();
  }
  const vacio = stubFetch(() => json([]));
  try {
    expect(await cliente().table('posts').remove(99)).toBe(false);
  } finally {
    vacio.restore();
  }
});

test('la columna que identifica la fila se puede cambiar', async () => {
  const s = stubFetch(() => json([{ email: 'ana@ejemplo.com' }]));
  try {
    await cliente().table('usuarios', { key: 'email' }).find('ana@ejemplo.com');
    expect(new URL(s.calls[0]!.url).searchParams.get('email')).toBe('eq.ana@ejemplo.com');
  } finally {
    s.restore();
  }
});

// --- Errores que dicen qué hacer -------------------------------------------

test('una tabla que no existe se explica en castellano, con el nombre', async () => {
  const s = stubFetch(() =>
    json({ code: '42P01', message: 'relation "postss" does not exist' }, 404),
  );
  try {
    await expect(cliente().table('postss').all()).rejects.toThrow(/No existe la tabla «postss»/);
  } finally {
    s.restore();
  }
});

test('un 403 se explica como lo que casi siempre es: una política RLS', async () => {
  const s = stubFetch(() => json({ message: 'permission denied' }, 403));
  try {
    await expect(cliente().table('posts').all()).rejects.toThrow(/RLS/);
  } finally {
    s.restore();
  }
});

test('un duplicado no dice «23505»', async () => {
  const s = stubFetch(() => json({ code: '23505', message: 'duplicate key' }, 409));
  try {
    await expect(cliente().table('posts').add({ slug: 'hola' })).rejects.toThrow(
      /Ya hay una fila con ese valor/,
    );
  } finally {
    s.restore();
  }
});

test('el error conserva la respuesta cruda para quien sí quiera mirarla', async () => {
  const s = stubFetch(() => json({ code: '23502', message: 'null value in column "titulo"' }, 400));
  try {
    await cliente().table('posts').add({});
    throw new Error('debería haber lanzado');
  } catch (e) {
    expect(e).toBeInstanceOf(SytueError);
    expect((e as SytueError).detail).toMatchObject({ code: '23502' });
  } finally {
    s.restore();
  }
});

test('count() lee el total de la cabecera y no se trae las filas', async () => {
  const s = stubFetch(() => json([{ id: 1 }], 200, { 'content-range': '0-0/42' }));
  try {
    expect(await cliente().table('posts').count()).toBe(42);
    expect(s.calls[0]!.headers.get('prefer')).toContain('count=exact');
  } finally {
    s.restore();
  }
});
