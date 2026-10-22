/*
 * Las dos superficies no comparten pantallas, aunque compartan binario.
 *
 * `admin` es la consola de UNA organización; `platform` es la administración
 * global de todas. Hasta ahora el mismo host servía las dos cosas:
 * `platform.sytue.eu/users` abría el directorio de la organización a la que
 * resulta que pertenece quien opera la plataforma, con el menú entero al lado.
 *
 * Se prueban las dos mitades de la separación, que son independientes y ninguna
 * basta sola: qué se OFRECE (el menú) y qué se SIRVE (el middleware). Ofrecer sin
 * servir da enlaces a un 404; servir sin ofrecer deja la pantalla accesible a
 * quien escriba la URL.
 */
import { describe, expect, it } from 'bun:test';
import { allowedSections } from './sections.js';
import { esHostDePlataforma, esRutaDePlataforma } from './superficies.js';

describe('menú por superficie', () => {
  it('en plataforma solo ofrece lo global', () => {
    // Las tres pantallas de la plataforma: administrarla, con qué credenciales se
    // presenta ante terceros y la cola de denuncias. Nada de la consola de una
    // organización.
    expect(allowedSections(['owner'], [], true)).toEqual(['platform', 'connections', 'moderation']);
    // Ni siquiera a un owner: el rol es de su organización, y aquí no se
    // administra ninguna en concreto.
    expect(allowedSections(['owner'], [], true)).not.toContain('users');
  });

  it('en admin ofrece la organización y NO la consola global', () => {
    const secciones = allowedSections(['admin'], [], false);
    expect(secciones).toContain('users');
    expect(secciones).toContain('groups');
    expect(secciones).not.toContain('platform');
  });

  it('sin rol de administración filtra por permisos, y sigue sin ofrecer plataforma', () => {
    const secciones = allowedSections([], ['audit.view'], false);
    expect(secciones).toContain('audit');
    expect(secciones).not.toContain('users');
    expect(secciones).not.toContain('platform');
  });
});

describe('qué host es cuál', () => {
  it('reconoce la plataforma por el subdominio, con puerto o sin él', () => {
    expect(esHostDePlataforma('platform.sytue.eu')).toBe(true);
    expect(esHostDePlataforma('platform.sytue.localhost:4321')).toBe(true);
  });

  it('no confunde admin ni un host ausente', () => {
    expect(esHostDePlataforma('admin.sytue.eu')).toBe(false);
    expect(esHostDePlataforma('console.sytue.eu')).toBe(false);
    expect(esHostDePlataforma(null)).toBe(false);
  });
});

describe('qué se sirve desde el host de plataforma', () => {
  it('sirve la consola global y lo que es de la propia sesión', () => {
    for (const ruta of [
      '/',
      '/platform',
      '/conexiones', // sección propia: si no se sirve, su entrada del menú es un 404
      '/account',
      '/auth/login',
      '/auth/callback',
      '/api/platform/orgs',
      '/api/platform-connections/templates', // el prefijo /api/platform lo cubre
      '/moderacion', // la cola de denuncias del operador
      '/api/platform-moderation/cast/reports',
      '/api/me/pim',
    ]) {
      expect(esRutaDePlataforma(ruta)).toBe(true);
    }
  });

  it('NO sirve ninguna pantalla de la consola de organización', () => {
    for (const ruta of [
      '/users',
      '/groups',
      '/roles',
      '/sso',
      '/devices',
      '/compliance',
      '/audit',
      '/tenants',
      '/api/admin/users',
      '/api/admin/groups',
    ]) {
      expect(esRutaDePlataforma(ruta)).toBe(false);
    }
  });

  it('deja pasar los recursos del framework, que no son pantallas de nadie', () => {
    for (const ruta of ['/_astro/index.abc123.js', '/favicon.ico', '/robots.txt']) {
      expect(esRutaDePlataforma(ruta)).toBe(true);
    }
  });
});
