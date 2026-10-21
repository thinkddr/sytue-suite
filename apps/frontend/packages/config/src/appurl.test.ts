/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 */
import { describe, expect, test } from 'bun:test';
import {
  adminUrl,
  appDomains,
  appUrl,
  driveFileUrl,
  driveFolderUrl,
  enlaceDocumento,
  volverA,
  type ProductKey,
} from './tokens.js';

// El prefijo `/admin` NO es opcional ni depende del host: la consola se sirve
// bajo él también en `admin.<dominio>` (chart del frontend, `path: /admin`, y el
// `base` de su astro.config). Enlazar al host pelado da un 404 que encima no lo
// parece —el host resuelve y el Ingress enruta—, así que se lee como «la consola
// está caída». Pasó con cinco enlaces a la vez: el catálogo de apps desde Rooms,
// tres del onboarding del portal y dos de los ajustes de Cloud.
describe('adminUrl', () => {
  test('siempre antepone el prefijo bajo el que se sirve la consola', () => {
    expect(adminUrl('apps')).toBe(`${appDomains.console}/admin/apps`);
    expect(adminUrl()).toBe(`${appDomains.console}/admin`);
  });

  test('da igual escribir la ruta con barra inicial o sin ella', () => {
    expect(adminUrl('/users')).toBe(adminUrl('users'));
    expect(adminUrl('//users')).toBe(adminUrl('users'));
  });

  test('nunca deja una barra suelta al final', () => {
    expect(adminUrl('')).not.toMatch(/\/$/);
    expect(adminUrl()).not.toMatch(/\/$/);
  });
});

// El mismo fallo que Admin lo tenían el portal (tres enlaces: notificaciones,
// bienvenida y descargas) y cualquier otra app: TODAS se sirven bajo prefijo.
// Traefik solo redirige la raíz, así que el enlace profundo sin prefijo es 404.
describe('appUrl', () => {
  test('antepone el prefijo de cada app', () => {
    expect(appUrl('one', 'notificaciones')).toBe(`${appDomains.one}/one/notificaciones`);
    expect(appUrl('mail')).toBe(`${appDomains.mail}/mail`);
    expect(appUrl('rooms', 'canal/x')).toBe(`${appDomains.rooms}/rooms/canal/x`);
  });

  // ADR 0076: cada producto de la ofimática tiene su host y su prefijo; ninguno vuelve a
  // `docs.<dominio>/docs`, que desapareció sin redirecciones.
  test('Docus, Calcus, Diapos y CertiPDF tienen host y prefijo propios', () => {
    expect(new URL(appUrl('docs', 'edit?id=1', 'acme')).pathname).toBe('/docus/acme/edit');
    expect(new URL(appUrl('sheets')).hostname.split('.')[0]).toBe('calcus');
    expect(appUrl('slides')).toBe(`${appDomains.slides}/diapos`);
    expect(appUrl('pdf', 'pdf?path=%2Fa.pdf')).toBe(`${appDomains.pdf}/certipdf/pdf?path=%2Fa.pdf`);
    for (const k of ['docs', 'sheets', 'slides', 'pdf'] as const) {
      expect(new URL(appUrl(k)).hostname.startsWith('docs.')).toBe(false);
    }
  });

  // Quien enlaza un documento sabe su tipo y lo manda a SU producto; sin tipo, a Docus,
  // que lo reenvía al suyo. Visto en rojo con `enlaceDocumento` mandándolo todo a `docs`.
  test('enlaceDocumento abre cada tipo en su producto', () => {
    expect(enlaceDocumento('h1', 'spreadsheet', 'acme')).toBe(
      `${appDomains.sheets}/calcus/acme/edit?id=h1`,
    );
    expect(enlaceDocumento('p1', 'presentation')).toBe(`${appDomains.slides}/diapos/edit?id=p1`);
    expect(enlaceDocumento('d1', 'document')).toBe(`${appDomains.docs}/docus/edit?id=d1`);
    expect(enlaceDocumento('x', null)).toBe(`${appDomains.docs}/docus/edit?id=x`);
  });

  test('Sytue ID no lleva prefijo: lo sirve un binario Go, no Astro', () => {
    expect(appUrl('id', 'account')).toBe(`${appDomains.id}/account`);
  });

  // El prefijo es el `base` del astro.config de cada app, y nada los ataba: a
  // Notepads le faltaba, y todo lo que se enlazaba con `appUrl('notes', …)` —el
  // `/p/{token}` de un cuaderno publicado, la nota compartida en Rooms— era el 404
  // de Traefik. Se compara con lo que hay en disco, app a app.
  test('cada app se enlaza bajo el `base` de su astro.config', async () => {
    const apps = new URL('../../../apps/', import.meta.url).pathname;
    // Directorio → clave de producto, cuando no se llaman igual. `null` = no se enlaza
    // con appUrl, y el porqué.
    const clave: Record<string, ProductKey | null> = {
      portal: 'one',
      'cloud-dashboard': 'cloud',
      // «Mi cuenta» vive en account.<apex>/account; la clave `id` es el IdP en Go.
      account: null,
      // La ofimática, una app por producto (ADR 0076): el directorio no es la clave.
      docus: 'docs',
      calcus: 'sheets',
      diapos: 'slides',
      certipdf: 'pdf',
    };
    let vistas = 0;
    for await (const f of new Bun.Glob('*/astro.config.mjs').scan({ cwd: apps })) {
      const base = /\bbase:\s*'([^']+)'/.exec(await Bun.file(apps + f).text())?.[1];
      if (!base) continue; // www: sitio público en la raíz
      const dir = f.slice(0, f.indexOf('/'));
      const key = dir in clave ? clave[dir] : (dir as ProductKey);
      if (key === null) continue;
      expect(key in appDomains ? `${dir}: ${new URL(appUrl(key)).pathname}` : dir).toBe(
        `${dir}: ${base}`,
      );
      vistas++;
    }
    expect(vistas).toBeGreaterThan(10);
  });
});

// --- La vuelta a Drive ---

describe('volverA', () => {
  test('acepta una ruta de Drive', () => {
    expect(volverA('/Documentos/2026')).toBe('/Documentos/2026');
    expect(volverA('/')).toBe('/');
  });

  test('RECHAZA lo que llevaría fuera: es la verja contra el redirector abierto', () => {
    expect(volverA('https://sitio-de-quien-sea/')).toBeUndefined();
    // Esquema heredado: es una URL absoluta y empieza por «/».
    expect(volverA('//sitio-de-quien-sea/')).toBeUndefined();
    expect(volverA('javascript:alert(1)')).toBeUndefined();
    expect(volverA('Documentos')).toBeUndefined();
  });

  test('sin parámetro no hay vuelta', () => {
    expect(volverA(null)).toBeUndefined();
    expect(volverA('')).toBeUndefined();
    expect(volverA(undefined)).toBeUndefined();
  });
});

describe('driveFolderUrl', () => {
  test('lleva a la carpeta, con host y prefijo de Drive', () => {
    const u = new URL(driveFolderUrl('/Documentos/informes 2026'));
    expect(u.pathname).toBe('/drive/');
    expect(u.searchParams.get('path')).toBe('/Documentos/informes 2026');
  });

  test('con org la organización encabeza el path y la query se conserva (ADR 0056)', () => {
    const u = new URL(driveFolderUrl('/Documentos/informes 2026', 'acme'));
    expect(u.pathname).toBe('/drive/acme/');
    expect(u.searchParams.get('path')).toBe('/Documentos/informes 2026');
  });

  test('la raíz va sin parámetro, que es lo que significa', () => {
    expect(driveFolderUrl('/')).not.toContain('?');
    expect(driveFolderUrl('')).not.toContain('?');
  });
});

describe('driveFileUrl', () => {
  test('es el enlace canónico del fichero, con el prefijo de Drive', () => {
    expect(driveFileUrl('a/b')).toBe(`${appDomains.drive}/drive/files/a%2Fb`);
  });
});
