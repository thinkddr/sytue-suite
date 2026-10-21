/**
 * El catálogo tiene DOS caras y conviene no confundirlas: lo que se PRESENTA
 * (`suiteApps`, que alimenta el app switcher de las once aplicaciones y el
 * Launcher del portal) y lo que se sabe NOMBRAR (`appForKey`, que resuelve el
 * origen de una notificación o de un resultado de búsqueda).
 *
 * Apagar Sytue DevX —el perfil piloto no lo despliega— quita la tesela, y no
 * puede dejar sin icono ni nombre a algo etiquetado `cloud` que llegue igual.
 *
 * Ejecutar: `bun test`.
 */
import { expect, test } from 'bun:test';
import { readdirSync, readFileSync } from 'node:fs';
import { appDomains } from '@sytue/config';
import {
  APP_SWITCHER_ORDER,
  CATALOGO,
  WORKHUB,
  appForKey,
  appsPresentables,
  hrefDe,
  suiteApps,
  puedeAdministrar,
} from './apps.js';

test('el catálogo lleva la suite entera: filtrar es cosa de quien pinta', () => {
  expect(suiteApps.some((a) => a.key === 'cloud')).toBe(true);
});

test('sin lista de desplegadas se presenta todo', () => {
  // «No se sabe» y «no hay nada» NO son lo mismo: si el Hub no contesta —o no lo
  // declara— se ofrece la suite entera, que es el comportamiento de siempre.
  expect(appsPresentables(suiteApps, undefined)).toEqual(suiteApps);
  expect(appsPresentables(suiteApps, [])).toEqual(suiteApps);
});

test('con lista, solo se presenta lo levantado', () => {
  const vivas = appsPresentables(suiteApps, ['mail', 'docs']).map((a) => a.key);
  expect(vivas).not.toContain('cloud');
  expect(vivas).toContain('mail');
  expect(vivas).toContain('docs');
});

test('Sytue One nunca se filtra: es la vuelta a casa', () => {
  // Aunque el despliegue no la declare, quedarse sin la puerta de vuelta desde
  // cualquier aplicación es peor que cualquier tesela de más.
  expect(appsPresentables(suiteApps, ['mail']).map((a) => a.key)).toContain('one');
});

test('el orden canónico no se toca al filtrar', () => {
  const presentadas = appsPresentables(suiteApps, ['mail', 'docs', 'drive']).map((a) => a.key);
  const esperado = APP_SWITCHER_ORDER.filter((k) => presentadas.includes(k));
  expect(presentadas).toEqual([...esperado]);
});

test('appForKey nombra lo que existe, se presente o no', () => {
  // DevX, aunque su tesela se apague en un despliegue sin él.
  expect(appForKey('cloud')?.name).toBe('Sytue DevX');
  expect(appForKey('erp')?.name).toBe('Sytue ERP');
  expect(appForKey('photos')?.name).toBe('Sytue Photos');
  // Y los que YA estaban fuera del switcher desde antes de esto.
  expect(appForKey('chat')?.name).toBe('Sytue Rooms');
  expect(appForKey('hub')?.name).toBe('Sytue Hub');
});

test('lo que no es de la suite no se inventa', () => {
  expect(appForKey('gmail')).toBeUndefined();
});

test('Docus, Calcus, Diapos y CertiPDF salen con el servicio de Docs, cada uno en su host', () => {
  expect(appsPresentables(suiteApps, ['docs']).map((a) => a.key)).toEqual([
    'one',
    'docs',
    'sheets',
    'slides',
    'pdf',
  ]);
  // ADR 0076: host propio y ninguna puerta con `?kind=` ni bajo `docs.<dominio>`.
  expect(appForKey('docs')?.href).toBe(appDomains.docs);
  expect(new URL(appForKey('sheets')?.href ?? '').hostname.split('.')[0]).toBe('calcus');
  expect(new URL(appForKey('slides')?.href ?? '').hostname.split('.')[0]).toBe('diapos');
  expect(new URL(appForKey('pdf')?.href ?? '').hostname.split('.')[0]).toBe('certipdf');
  expect(appForKey('pdf')?.name).toBe('Sytue CertiPDF');
});

// --- La organización en las teselas (ADR 0056) -------------------------------
//
// Cada clic de una tesela pagaba el 302 local del middleware del destino —y las
// sin `ruta`, además, el redirect de Traefik de la raíz al prefijo—. Con la org
// del navegador, el href sale canónico y aterriza directo. Lo consumen el
// conmutador (`AppSwitcher`) y el Launcher del Hub.

test('hrefDe con org compone dominio + prefijo + org: aterriza sin 302', () => {
  expect(new URL(hrefDe('mail', 'acme')).pathname).toBe('/mail/acme');
  expect(new URL(hrefDe('one', 'acme')).pathname).toBe('/one/acme');
  // Admin: host propio `admin.*` y prefijo `/admin`.
  expect(new URL(hrefDe('console', 'acme')).pathname).toBe('/admin/acme');
  // La ofimática: cada producto con su host y su prefijo (ADR 0076).
  expect(new URL(hrefDe('sheets', 'acme')).pathname).toBe('/calcus/acme');
  expect(new URL(hrefDe('pdf', 'acme')).pathname).toBe('/certipdf/acme');
});

test('hrefDe sin org (SSR, contexto sin sesión) sale como siempre', () => {
  // Sin `ruta`, el dominio pelado: lo que siempre apuntó, redirect de Traefik
  // mediante. La org es opcional y null-safe: null, undefined y '' son «sin org».
  expect(hrefDe('mail')).toBe(appDomains.mail);
  expect(hrefDe('mail', null)).toBe(appDomains.mail);
  expect(hrefDe('mail', undefined)).toBe(appDomains.mail);
  expect(hrefDe('mail', '')).toBe(appDomains.mail);
  expect(hrefDe('docs')).toBe(appDomains.docs);
  expect(hrefDe('pdf')).toBe(appDomains.pdf);
});

test('Pages se ofrece cuando está desplegado y abre Mis sitios con la organización', () => {
  expect(appsPresentables(suiteApps, ['pages']).map((a) => a.key)).toEqual(['one', 'pages']);
  expect(appsPresentables(suiteApps, ['mail']).map((a) => a.key)).not.toContain('pages');
  expect(CATALOGO.get('pages')?.nativa).toBe(false);
  expect(hrefDe('pages')).toBe(`${appDomains.pages}/web`);
  expect(hrefDe('pages', 'acme')).toBe(`${appDomains.pages}/web/acme`);
});

test('hrefDe NO cuelga la org de un host sin prefijo (Sytue ID)', () => {
  // `id` lo sirve un binario Go con sus rutas en la raíz: no hay prefijo tras
  // el que escopiar la org, y componerla colaría el slug como ruta del IdP.
  expect(hrefDe('id', 'acme')).toBe(appDomains.id);
  expect(hrefDe('id')).toBe(appDomains.id);
});

// --- Quién ve la tesela de Sytue Admin ---------------------------------------
//
// NO es la comprobación de seguridad: el gate está en el servidor. Esto decide
// si se PINTA la puerta, porque ofrecer una que va a dar un 403 es mala interfaz.

test('puedeAdministrar: owner y admin entran', () => {
  expect(puedeAdministrar(['owner'], [])).toBe(true);
  expect(puedeAdministrar(['admin'], [])).toBe(true);
});

test('puedeAdministrar: un miembro corriente no', () => {
  expect(puedeAdministrar(['member'], [])).toBe(false);
  expect(puedeAdministrar([], [])).toBe(false);
  expect(puedeAdministrar(undefined, undefined)).toBe(false);
});

// El caso que impide que esto sea un `roles.includes('admin')` de una línea: el
// administrador DELEGADO no tiene el rol, pero entra y trabaja. Esconderle la
// tesela lo dejaría sin puerta a lo único que puede administrar.
test('puedeAdministrar: el administrador delegado entra aunque no tenga el rol', () => {
  expect(puedeAdministrar(['member'], ['users.read'])).toBe(true);
});

test('puedeAdministrar: facturación sin permisos delegados no administra', () => {
  expect(puedeAdministrar(['billing'], [])).toBe(false);
});

// --- Nomenclatura canónica v4 (encargo del 2026-09-21) -----------------------

test('los nombres v4: Hub, Admin, DevX y CertiPDF; los legacy no vuelven', () => {
  expect(CATALOGO.get('one')?.name).toBe('Sytue Hub');
  expect(CATALOGO.get('console')?.name).toBe('Sytue Admin');
  expect(CATALOGO.get('cloud')?.name).toBe('Sytue DevX');
  expect(CATALOGO.get('pdf')?.name).toBe('Sytue CertiPDF');
  const nombres = [...CATALOGO.values()].map((e) => e.name);
  // Construidos por trozos a propósito: el renombrado se hizo con un script de
  // búsqueda y reemplazo, y escritos enteros se los habría llevado por delante,
  // dejando el test comprobando que los nombres NUEVOS no están.
  for (const legacy of ['Cloud', 'certi' + 'Docs', 'Console', 'Portal']) {
    expect(nombres).not.toContain('Sytue ' + legacy);
  }
  // «Sytue Docs» no nombra nada: era a la vez la suite (WorkHub) y el editor (Docus).
  expect(nombres).not.toContain('Sytue Docs');
});

test('Hub ≠ WorkHub: la entrada común no es la suite ofimática', () => {
  expect(CATALOGO.get('one')?.suite).toBe('hub');
  expect(WORKHUB).toEqual(['docs', 'sheets', 'slides', 'pdf', 'notes', 'photos']);
  expect(WORKHUB).not.toContain('one');
});

test('la clave técnica NO cambió: rompería subdominios, client_id e ingresses', () => {
  // El renombrado v4 es de nombre VISIBLE. Si alguien renombra una clave, esto cae,
  // y con ello `appDomains`, los charts, los client_id y los enlaces ya guardados.
  expect([...CATALOGO.keys()]).toEqual([
    'one',
    'mail',
    'drive',
    'calendar',
    'contacts',
    'docs',
    'sheets',
    'slides',
    'pdf',
    'notes',
    'photos',
    'rooms',
    'cast',
    'erp',
    'pages',
    'bases',
    'cloud',
    'console',
    'id',
  ]);
});

test('Sytue Bases se presenta como disponible: va encendida por defecto', () => {
  // Decisión del dueño del 2026-10-04. Si vuelve a `desarrollo`, el sitio público le
  // quita el botón de abrir mientras el despliegue la sigue levantando.
  expect(CATALOGO.get('bases')?.disponibilidad).toBe('disponible');
});

test('DevX no se presenta como disponible: está fuera del sitio público', () => {
  // Se retiró el 2026-09-15 «mientras dure la fase piloto» (8907c2f9f) y el umbrella
  // solo lo instala con CLOUD_ENABLED. Presentarlo disponible sería inventar estado.
  expect(CATALOGO.get('cloud')?.disponibilidad).toBe('desarrollo');
});

test('`nativa` es lo que de verdad se empaqueta: las apps de apps/fyne/cmd', () => {
  // Sin esto `nativa` sería una opinión. Lo que se publica es cada `cmd/<app>` con su
  // FyneApp.toml (apps/fyne/scripts/empaquetar.sh), y su esquema `sytue-<clave>` dice
  // de qué producto es: el directorio lleva el nombre comercial (Docus edita `docs`).
  const cmd = new URL('../../../../../fyne/cmd/', import.meta.url);
  const empaquetadas = new Set(
    readdirSync(cmd)
      .map((app) => {
        try {
          return readFileSync(new URL(`${app}/FyneApp.toml`, cmd), 'utf8');
        } catch {
          return '';
        }
      })
      .map((toml) => /^\s*esquema\s*=\s*"sytue-([a-z-]+)"/m.exec(toml)?.[1])
      .filter((clave): clave is string => Boolean(clave)),
  );
  expect(empaquetadas.size).toBeGreaterThan(5); // los ficheros se leyeron de verdad
  for (const e of CATALOGO.values()) {
    expect([e.key, e.nativa]).toEqual([e.key, empaquetadas.has(e.key)]);
  }
});
