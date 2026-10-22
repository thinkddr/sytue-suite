import { describe, expect, it } from 'bun:test';
import { existsSync, readdirSync, readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';

import { cabecerasCerradas } from './astro-middleware.js';
import {
  applySecurityHeaders,
  SYTUE_PERMISSIONS_POLICY_ROOMS,
  SYTUE_SECURITY_HEADERS,
} from './astro-security.js';
import { appDomains, integrationsDomain } from './tokens.js';

describe('applySecurityHeaders', () => {
  it('protege del clickjacking por las dos vías (CSP y cabecera heredada)', () => {
    const headers = new Headers();
    applySecurityHeaders(headers);
    expect(headers.get('content-security-policy')).toContain("frame-ancestors 'none'");
    expect(headers.get('x-frame-options')).toBe('DENY');
  });

  it('cierra los vectores de inyección que no dependen de un inventario por app', () => {
    const headers = new Headers();
    applySecurityHeaders(headers);
    const csp = headers.get('content-security-policy') ?? '';
    expect(csp).toContain("object-src 'none'");
    expect(csp).toContain("base-uri 'self'");
    expect(csp).toContain("form-action 'self'");
  });

  it('no filtra la URL completa a terceros ni deja capacidades abiertas', () => {
    const headers = new Headers();
    applySecurityHeaders(headers);
    expect(headers.get('referrer-policy')).toBe('strict-origin-when-cross-origin');
    expect(headers.get('permissions-policy')).toContain('camera=()');
    expect(headers.get('permissions-policy')).toContain('microphone=()');
    expect(headers.get('permissions-policy')).toContain('geolocation=()');
    expect(headers.get('cross-origin-opener-policy')).toBe('same-origin');
  });

  it('Rooms concede cámara y micrófono a su propio origen, no al mundo', () => {
    const headers = new Headers();
    applySecurityHeaders(headers, { 'permissions-policy': SYTUE_PERMISSIONS_POLICY_ROOMS });
    const policy = headers.get('permissions-policy') ?? '';
    expect(policy).toContain('camera=(self)');
    expect(policy).toContain('microphone=(self)');
    expect(policy).toContain('display-capture=(self)');
    // Lo que Rooms no necesita sigue apagado.
    expect(policy).toContain('geolocation=()');
    expect(policy).toContain('usb=()');
    // El resto de superficies NO abren la cámara.
    expect(SYTUE_SECURITY_HEADERS['permissions-policy']).toContain('camera=()');
  });

  it('no delega en el borde lo que puede emitir la aplicación', () => {
    /*
     * Estas dos NO estaban, porque un comentario decía que las ponía el borde
     * «para toda la zona». No las ponía: `sytue-edge-security` está declarado en
     * el chart y no lo engancha ningún router en el piloto. Medido contra
     * producción el 2026-09-21 sobre un 404 servido por el middleware de la app
     * (`one.sytue.eu/api/v1/apps`): salían las otras seis y estas dos no.
     *
     * El test no comprueba el borde —no puede—, comprueba que la aplicación no
     * dependa de él: una cabecera que solo existe si un objeto de
     * infraestructura está bien enganchado es una cabecera que un día no está.
     */
    const headers = new Headers();
    applySecurityHeaders(headers);
    expect(headers.get('x-content-type-options')).toBe('nosniff');
    expect(headers.get('strict-transport-security')).toContain('includeSubDomains');
  });

  it('un override vacío omite la cabecera en vez de emitirla en blanco', () => {
    const headers = new Headers();
    applySecurityHeaders(headers, { 'x-frame-options': '' });
    expect(headers.get('x-frame-options')).toBeNull();
    expect(headers.get('referrer-policy')).not.toBeNull();
  });

  it('NO pisa la CSP `sandbox` que una ruta ya puso a un adjunto en línea', () => {
    // Varias rutas de BFF (Drive content/shared/team, Rooms attachment/gifs, Mail
    // attachments…) sirven una imagen o un PDF en línea con
    // `content-security-policy: sandbox` para que un SVG con `<script>` no ejecute
    // en el origen de la app. Pisar esa cabecera con la CSP de la casa reabría el
    // XSS. Se conserva el `sandbox` y, además, se añade la base, que solo endurece.
    const headers = new Headers({ 'content-security-policy': 'sandbox' });
    applySecurityHeaders(headers);
    const csp = headers.get('content-security-policy') ?? '';
    expect(csp).toContain('sandbox');
    expect(csp).toContain("frame-ancestors 'none'");
  });

  it("respeta la forma más estricta `default-src 'none'; sandbox` de Mail", () => {
    const headers = new Headers({ 'content-security-policy': "default-src 'none'; sandbox" });
    applySecurityHeaders(headers);
    expect(headers.get('content-security-policy') ?? '').toContain('sandbox');
  });

  it('la CSP base de una app se SUMA al `sandbox` de la ruta; no lo pisa', () => {
    // FE-03: la CSP cerrada va como base, no como override. Si la pasara como
    // override, Drive, Mail y Rooms perderían el `sandbox` de sus adjuntos en línea.
    // Verlo en rojo: en `middlewareDeIdioma`, pasa la base como `overrides`.
    const headers = new Headers({ 'content-security-policy': 'sandbox' });
    applySecurityHeaders(headers, {}, cabecerasCerradas('https://hub.ejemplo.test'));
    const csp = headers.get('content-security-policy') ?? '';
    expect(csp).toContain('sandbox');
    expect(csp).toContain("connect-src 'self' https://hub.ejemplo.test");
  });

  it('un override de CSP SÍ sustituye a la base (decisión por ruta)', () => {
    // La vista previa de Notes/Photos y el editor WOPI de Docs pasan su propia CSP
    // por `overrides`: esa gana, también sobre una CSP previa en la respuesta.
    const headers = new Headers({ 'content-security-policy': 'sandbox' });
    applySecurityHeaders(headers, {
      'content-security-policy': "frame-ancestors 'self'; script-src 'none'",
    });
    const csp = headers.get('content-security-policy') ?? '';
    expect(csp).toContain("frame-ancestors 'self'");
    expect(csp).not.toContain('sandbox');
  });
});

/**
 * Las cabeceras solo valen si TODAS las apps SSR las aplican: durante mucho
 * tiempo solo `www` las emitía —porque solo `www` usa el servidor estático que
 * las pone— y las nueve superficies con sesión se servían sin ninguna. Este
 * test recorre el árbol de apps para que una app nueva (o un middleware que
 * pierda la llamada en un refactor) rompa el build en vez de salir a producción
 * desnuda.
 */
describe('cableado en las apps SSR', () => {
  const appsDir = fileURLToPath(new URL('../../../apps/', import.meta.url));
  // Una app es un directorio con su `astro.config.mjs`: es lo que Astro sirve. Un
  // directorio que todavía solo trae `public/` —la marca deja ahí los favicons de un
  // producto ANTES de que exista su app, como Sytue Cast— no sirve nada y no tiene
  // middleware que cablear; en cuanto aparezca el `astro.config.mjs` entra en la lista.
  const ssrApps = readdirSync(appsDir, { withFileTypes: true })
    .filter(
      (entry) =>
        entry.isDirectory() &&
        entry.name !== 'www' &&
        existsSync(`${appsDir}${entry.name}/astro.config.mjs`),
    )
    .map((entry) => entry.name);

  it('encuentra las apps SSR', () => {
    expect(ssrApps.length).toBeGreaterThanOrEqual(9);
  });

  /*
   * Se admiten las TRES formas de cablearlo, y ninguna más. Antes se buscaba el
   * literal `applySecurityHeaders`, pero el cuerpo del middleware ya no está en
   * cada app: lo pone `@sytue/config/astro-middleware`, que es quien llama.
   *
   * Importar `negociarIdioma` NO cuenta a propósito: esa función solo resuelve el
   * idioma. Una app que la use y se olvide de las cabeceras es exactamente la
   * regresión que este test existe para cazar, y aceptar el nombre del módulo a
   * secas la dejaría pasar.
   */
  it.each(ssrApps)('%s aplica las cabeceras de seguridad en su middleware', (app) => {
    const propio = readFileSync(`${appsDir}${app}/src/middleware.ts`, 'utf8');
    // Las apps de la ofimática reexportan el de su paquete: se mira ese, que es el que corre.
    const middleware = /export \{ onRequest \} from '@sytue\/office\/middleware'/.test(propio)
      ? readFileSync(
          fileURLToPath(new URL('../../office/src/middleware.ts', import.meta.url)),
          'utf8',
        )
      : propio;
    const cableado =
      // el handler estándar, tal cual
      /export \{ onRequest \} from '@sytue\/config\/astro-middleware'/.test(middleware) ||
      // el handler estándar con cabeceras propias por petición
      middleware.includes('middlewareDeIdioma(') ||
      // middleware propio que llama a la función él mismo
      middleware.includes('applySecurityHeaders(');
    expect(cableado, `${app}/src/middleware.ts no aplica las cabeceras de la suite`).toBe(true);
  });
});

/**
 * LA CSP CERRADA DEJA PASAR A LA SHELL, Y A NADIE MÁS.
 *
 * Sytue Web y Sytue Bases cierran `connect-src` e `img-src`, y las dos nacieron
 * dejando fuera al Hub: la campana no traía avisos y no había presencia, sin un error
 * a la vista (producción, 2026-10-05). Bases, por copiar el fichero de Web.
 *
 * Verlo en rojo: deja la conexión en `'self'` a secas en `cabecerasCerradas` y el primer
 * test nombra al Hub; quita `hubBaseUrl()` de la llamada del middleware de una de las
 * dos apps, o devuélvele su `lib/cabeceras.ts`, y falla el último.
 */
describe('cabecerasCerradas', () => {
  const HUB = 'https://hub.ejemplo.test';
  /** Las fuentes de una directiva de la política, sin el nombre. */
  const fuentes = (directiva: string, hub = `${HUB}/`): string[] => {
    const csp = cabecerasCerradas(hub)['content-security-policy'] ?? '';
    const linea = csp.split('; ').find((d) => d.startsWith(`${directiva} `));
    return linea ? linea.split(' ').slice(1) : [];
  };

  it('deja a la shell hablar con el Hub y con el gateway de integraciones', () => {
    // El Hub: avisos, su flujo, presencia, búsqueda y la lista de apps desplegadas.
    expect(fuentes('connect-src')).toContain(HUB);
    // `useProductMap` del conmutador de aplicaciones.
    expect(fuentes('connect-src')).toContain(integrationsDomain);
    expect(fuentes('connect-src')).toContain("'self'");
  });

  it('deja cargar el logo de la organización, que sirve Sytue ID', () => {
    expect(fuentes('img-src')).toContain(appDomains.id);
  });

  it('sigue cerrada: orígenes exactos, ni comodines ni esquemas enteros', () => {
    expect(fuentes('connect-src').toSorted()).toEqual(
      ["'self'", HUB, integrationsDomain].toSorted(),
    );
    expect(fuentes('img-src').toSorted()).toEqual(
      ["'self'", 'data:', 'blob:', appDomains.id].toSorted(),
    );
    expect(fuentes('script-src')).toEqual(["'self'", "'unsafe-inline'"]);
    for (const directiva of ['connect-src', 'img-src', 'script-src']) {
      for (const fuente of fuentes(directiva)) {
        expect(fuente).not.toContain('*');
        expect(['https:', 'http:', 'wss:', 'ws:']).not.toContain(fuente);
      }
    }
  });

  it('conserva las cuatro directivas de la suite', () => {
    const csp = cabecerasCerradas(HUB)['content-security-policy'] ?? '';
    expect(csp.startsWith(`${SYTUE_SECURITY_HEADERS['content-security-policy']}; `)).toBe(true);
  });

  it('un Hub mal configurado no tumba la app: se omite', () => {
    expect(fuentes('connect-src', 'no-es-una-url')).toEqual(["'self'", integrationsDomain]);
  });

  /*
   * FE-03 (auditoría del 2026-10-06): las apps con la CSP cerrada, por IGUALDAD. Falla si una
   * app pierde la cabecera en un refactor y también si alguien la cierra sin pasar por aquí,
   * que es cuando hay que medir sus orígenes (`test/e2e/csp-origenes.mjs`, en el piloto).
   * Verlo en rojo: quita `cabecerasCerradas` del middleware de cualquiera de estas apps.
   */
  const CERRADAS = [
    'account',
    'bases',
    'calcus',
    'calendar',
    'cast',
    'certipdf',
    'cloud-dashboard',
    'console',
    'contacts',
    'diapos',
    'docus',
    'drive',
    'erp',
    'mail',
    'notes',
    'pages',
    'photos',
    'portal',
    'rooms',
  ];

  it('las apps con la CSP cerrada son estas, con el Hub que le pasan a la shell', () => {
    // El Hub es lo que reciben las islas de cada app (`<… hubBaseUrl>`). Con otro valor
    // —o con una política escrita a mano— se dejaría pasar a un Hub que no es el que se
    // usa, o a ninguno. Va como BASE (segundo argumento), no como excepción por ruta: así
    // no pisa el `sandbox` de los adjuntos.
    const appsDir = fileURLToPath(new URL('../../../apps/', import.meta.url));
    const fuente = (app: string): string => {
      const propio = readFileSync(`${appsDir}${app}/src/middleware.ts`, 'utf8');
      // Las cuatro de la ofimática reexportan el de su paquete: se mira ese, que es el que corre.
      return /export \{ onRequest \} from '@sytue\/office\/middleware'/.test(propio)
        ? readFileSync(
            fileURLToPath(new URL('../../office/src/middleware.ts', import.meta.url)),
            'utf8',
          )
        : propio;
    };
    const cerradas = readdirSync(appsDir).filter(
      (app) =>
        existsSync(`${appsDir}${app}/src/middleware.ts`) &&
        fuente(app).includes('cabecerasCerradas'),
    );
    expect(cerradas.toSorted()).toEqual(CERRADAS);
    for (const app of cerradas) {
      const middleware = fuente(app);
      expect(middleware, `${app}: cabecerasCerradas sin el Hub de la shell`).toMatch(
        /cabecerasCerradas\(\s*(hubBaseUrl\(\)|serverEnv\(import\.meta\.env\)\.SYTUE_HUB_BASE_URL)/,
      );
      expect(middleware, `${app}: la CSP cerrada no va como base de middlewareDeIdioma`).toMatch(
        /middlewareDeIdioma\([^]*?,\s*(\(\) =>\s*cabecerasCerradas|cerrada)\b|applySecurityHeaders\([^,]+,\s*\{\},\s*cerrada\(\)\)/,
      );
      // Y en las que llaman a `applySecurityHeaders` a mano, NINGUNA llamada sin ella (la
      // consola tiene dos: la del 404 de plataforma también cuenta).
      expect(middleware, `${app}: una respuesta sale sin la CSP cerrada`).not.toMatch(
        /applySecurityHeaders\([^,()]+\)/,
      );
    }
  });

  // XSS-06: el lienzo de Sytue Web pinta los incrustados de un diseño (vídeo, mapa, Cast)
  // dentro de su origen; un iframe hacia cualquier otro sitio no debe cargar.
  it('encuadra solo los orígenes que se le dan; sin ellos, como estaba', () => {
    expect(cabecerasCerradas(HUB)['content-security-policy']).not.toContain('frame-src');
    const csp = cabecerasCerradas(HUB, { marcos: ['https://player.vimeo.com'] })[
      'content-security-policy'
    ];
    expect(csp?.split('; ').find((d) => d.startsWith('frame-src '))).toBe(
      "frame-src 'self' https://player.vimeo.com",
    );
  });

  it('Sytue Web le pasa los orígenes de los incrustados del renderizador', () => {
    const web = readFileSync(
      fileURLToPath(new URL('../../../apps/pages/src/middleware.ts', import.meta.url)),
      'utf8',
    );
    expect(web).toContain('cabecerasCerradas(hubBaseUrl(), { marcos: MARCOS })');
    expect(web).toContain('ORIGENES_DE_INCRUSTADOS');
  });

  it('Rooms y Cast abren el SFU de videollamada y directo, y Mail cierra img-src sin https:', () => {
    const rooms = readFileSync(
      fileURLToPath(new URL('../../../apps/rooms/src/middleware.ts', import.meta.url)),
      'utf8',
    );
    const cast = readFileSync(
      fileURLToPath(new URL('../../../apps/cast/src/middleware.ts', import.meta.url)),
      'utf8',
    );
    const mail = readFileSync(
      fileURLToPath(new URL('../../../apps/mail/src/middleware.ts', import.meta.url)),
      'utf8',
    );
    expect(rooms).toContain('wss://rooms-sfu.${suiteApex}');
    expect(cast).toContain('wss://rooms-sfu.${suiteApex}');
    expect(mail).not.toContain("'https:'");
  });
});

/**
 * Un test que nadie ejecuta es peor que no tenerlo: ocupa sitio, se lee como
 * cobertura y no avisa de nada. El CI corre `bun run test`, que recorre el
 * workspace y SALTA EN SILENCIO todo paquete sin script `test` — así que cinco
 * apps acumularon 943 tests dormidos (49 ficheros solo en Docs) sin que nada lo
 * dijera. Esto lo convierte en un fallo del build.
 */
describe('ninguna app deja sus tests sin ejecutar', () => {
  const appsDir = fileURLToPath(new URL('../../../apps/', import.meta.url));
  const apps = readdirSync(appsDir, { withFileTypes: true })
    .filter((entry) => entry.isDirectory())
    .map((entry) => entry.name);

  const tieneTests = (app: string): boolean => {
    const busca = (dir: string): boolean => {
      let entradas;
      try {
        entradas = readdirSync(`${appsDir}${app}/${dir}`, { withFileTypes: true });
      } catch {
        return false;
      }
      return entradas.some((e) =>
        e.isDirectory()
          ? e.name !== 'node_modules' && busca(`${dir}/${e.name}`)
          : /\.test\.tsx?$/.test(e.name),
      );
    };
    return busca('src') || busca('test');
  };

  it.each(apps)('%s declara el script `test` si tiene ficheros de test', (app) => {
    if (!tieneTests(app)) return;
    const pkg = JSON.parse(readFileSync(`${appsDir}${app}/package.json`, 'utf8')) as {
      scripts?: Record<string, string>;
    };
    expect(pkg.scripts?.test).toBeString();
  });
});
