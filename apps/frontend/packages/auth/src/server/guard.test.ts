// Check de la puerta cerrada del guard compartido: el 403 que ven las 25 páginas
// protegidas de la suite, y el 302 al login que recibe quien llega SIN sesión.
//
// Antes era `new Response('No autorizado para este recurso.')`: texto plano, un
// solo idioma y sin salida. Que se rompa aquí y no en producción es todo el
// objetivo del fichero, porque el guard es el punto ÚNICO por el que pasan las
// diez apps y una regresión se llevaría por delante a todas a la vez.
// Ejecutar: `bun test`.
import { expect, test } from 'bun:test';
import type { Bff, BffContext } from './bff.js';
import type { SytueRole, SytueSession } from '../types.js';
import { requireSession } from './guard.js';
import { esPlanDePago, sinAdminPorPlan } from '../types.js';
import { appDomains } from '@sytue/config';

function sesion(roles: SytueRole[]): SytueSession {
  return {
    user: { id: 'u1', email: 'ops@sytue.com', name: 'Ops', roles, tenant: 'corporate' },
    // Token opaco a propósito: `permsFromAccessToken` no encuentra `perms` y
    // devuelve [], que es justo el caso de un empleado sin admin delegada.
    tokens: { accessToken: 'no-es-un-jwt', idToken: '', refreshToken: '', expiresAt: 0 },
    clientId: 'sytue-console',
  } as SytueSession;
}

function ctx(headers: Record<string, string> = {}, cookie?: string, ruta = '/sso'): BffContext {
  const url = new URL(`https://admin.sytue.eu${ruta}`);
  return {
    request: new Request(url, { headers }),
    // Jar equivalente al de Astro. NO se prueba por la cabecera `Cookie`: es un
    // nombre prohibido y happy-dom —que otro test del paquete registra a nivel
    // global— la borra al construir el Request, así que el caso pasaría en
    // solitario y fallaría en la suite entera.
    cookies: {
      get: (n: string) =>
        cookie !== undefined && n === 'sytue_locale' ? { value: cookie } : undefined,
      set: () => {},
      delete: () => {},
    },
    url,
  };
}

const bff = (session: SytueSession | null, basePath = ''): Bff =>
  ({ getSession: () => Promise.resolve(session), basePath }) as unknown as Bff;

test('sin el rol exigido devuelve una página HTML 403, no texto plano', async () => {
  const res = await requireSession(bff(sesion(['member'])), ctx(), { roles: ['admin', 'owner'] });
  expect(res).toBeInstanceOf(Response);
  const r = res as Response;
  expect(r.status).toBe(403);
  expect(r.headers.get('content-type')).toContain('text/html');
  const html = await r.text();
  expect(html).toStartWith('<!doctype html>');
  // Salida de vuelta al portal: sin enlace, la puerta cerrada es un callejón.
  //
  // Se compara con el catálogo, no con un host escrito a mano: este check exigía
  // «one.sytue.eu» y se puso en rojo el día que el portal pasó de subdominio a
  // ruta, sin que la puerta hubiera cambiado en nada. Lo que importa es que HAYA
  // salida y que apunte al portal, no cómo se escriba su URL hoy.
  expect(html).toContain(`href="${appDomains.one}/"`);
  expect(html).not.toContain('No autorizado para este recurso.');
});

test('sin sesión devuelve un 302 al login con el returnTo canónico, sin la marca', async () => {
  // La entrada org-ful llega al guard REESCRITA por dentro (`/rooms/channels/x?__org=acme`,
  // ADR 0056): el returnTo sale en forma canónica —`/rooms/acme/...`— y SIN la
  // marca, o tras el login la barra se quedaría en la forma interna.
  const res = await requireSession(
    bff(null, '/rooms'),
    ctx({}, undefined, '/rooms/channels/x?__org=acme'),
  );
  expect(res).toBeInstanceOf(Response);
  const r = res as Response;
  expect(r.status).toBe(302);
  const location = r.headers.get('location') ?? '';
  expect(location).toBe('/rooms/auth/login?returnTo=%2Frooms%2Facme%2Fchannels%2Fx');
  expect(location).not.toContain('__org');
});

test('el 403 habla el idioma negociado (cookie > Accept-Language > inglés)', async () => {
  const texto = async (headers: Record<string, string>, cookie?: string): Promise<string> => {
    const res = (await requireSession(bff(sesion(['member'])), ctx(headers, cookie), {
      roles: ['admin'],
    })) as Response;
    return res.text();
  };

  expect(await texto({})).toContain('lang="en"');
  expect(await texto({ 'accept-language': 'fr-FR,fr;q=0.9' })).toContain('lang="fr"');
  // La preferencia explícita del usuario manda sobre lo que pida el navegador.
  expect(await texto({ 'accept-language': 'fr' }, 'de')).toContain('lang="de"');
  // Cookie con un idioma que no hablamos: se ignora y decide el navegador.
  expect(await texto({ 'accept-language': 'it' }, 'ja')).toContain('lang="it"');

  const en = await texto({});
  const de = await texto({ 'accept-language': 'de' });
  expect(en).not.toBe(de); // si el catálogo se quedara a medias, saldrían iguales
});

test('con el rol exigido no hay puerta cerrada: pasa a la página', async () => {
  const res = await requireSession(bff(sesion(['admin'])), ctx(), { roles: ['admin', 'owner'] });
  expect(res).not.toBeInstanceOf(Response);
  expect((res as { session: SytueSession }).session.user.email).toBe('ops@sytue.com');
});

// --- clase de organización y plan -----------------------------------------------
//
// `orgKinds` llevaba desde que existe sin un solo test, y la regla nueva —Sytue
// Admin solo con suscripción— se apoya en lo mismo: la cuenta personal es `owner`
// de su propia organización, así que por rol entraría en todo. Las sesiones de
// abajo son las que acuña el login de verdad: `orgKind` y `plan` del token.

function personal(plan: string | undefined, roles: SytueRole[] = ['owner']): SytueSession {
  const s = sesion(roles);
  return { ...s, user: { ...s.user, orgKind: 'personal', plan } } as SytueSession;
}

test('orgKinds: ["org"] cierra la superficie a la cuenta personal aunque sea owner', async () => {
  const res = await requireSession(bff(personal('pro')), ctx(), { orgKinds: ['org'] });
  expect(res).toBeInstanceOf(Response);
  expect((res as Response).status).toBe(403);
});

test('orgKinds: sin el claim de clase se trata como organización y entra', async () => {
  const res = await requireSession(bff(sesion(['owner'])), ctx(), { orgKinds: ['org'] });
  expect(res).not.toBeInstanceOf(Response);
});

test('una cuenta personal GRATUITA no entra donde se pide rol o permiso, aunque sea owner', async () => {
  for (const plan of ['personal_free', undefined, '']) {
    const res = await requireSession(bff(personal(plan)), ctx(), { roles: ['admin', 'owner'] });
    expect(res).toBeInstanceOf(Response);
    expect((res as Response).status).toBe(403);
    const porPermiso = await requireSession(bff(personal(plan)), ctx(), { perms: ['apps.manage'] });
    expect((porPermiso as Response).status).toBe(403);
  }
});

test('una cuenta personal DE PAGO sí entra: es owner y la suscripción la habilita', async () => {
  for (const plan of ['pro', 'enterprise']) {
    const res = await requireSession(bff(personal(plan)), ctx(), { roles: ['admin', 'owner'] });
    expect(res).not.toBeInstanceOf(Response);
  }
});

test('sin pedir rol ni permiso, la gratuita pasa: la regla solo cierra la administración', async () => {
  // Es lo que hacen Mail, Drive o Cuenta: exigen sesión, no un rol. Cerrarles la
  // puerta a la cuenta gratuita sería dejarla sin suite entera, no sin Admin.
  const res = await requireSession(bff(personal('personal_free')), ctx());
  expect(res).not.toBeInstanceOf(Response);
});

test('esPlanDePago y sinAdminPorPlan: lo que falta o no se conoce no abre puertas', () => {
  expect(esPlanDePago({ plan: 'pro' })).toBe(true);
  expect(esPlanDePago({ plan: 'enterprise' })).toBe(true);
  expect(esPlanDePago({ plan: 'personal_free' })).toBe(false);
  expect(esPlanDePago({ plan: undefined })).toBe(false);
  expect(esPlanDePago({ plan: 'premium' })).toBe(false);
  // `standard` es el nombre ANTERIOR a la migración 0064: ya no lo emite nadie
  // y no puede volver a colarse como plan de pago.
  expect(esPlanDePago({ plan: 'standard' })).toBe(false);
  expect(sinAdminPorPlan({ orgKind: 'personal', plan: 'personal_free' })).toBe(true);
  expect(sinAdminPorPlan({ orgKind: 'personal', plan: 'pro' })).toBe(false);
  expect(sinAdminPorPlan({ orgKind: 'personal', plan: 'enterprise' })).toBe(false);
  // Una organización nunca se cierra por plan: el suyo no acota Admin.
  expect(sinAdminPorPlan({ orgKind: 'org', plan: undefined })).toBe(false);
  expect(sinAdminPorPlan({ orgKind: undefined, plan: undefined })).toBe(false);
});
