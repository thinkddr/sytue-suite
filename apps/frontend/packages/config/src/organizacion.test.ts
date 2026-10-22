/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * El parsing y la composición de la organización en el path (ADR 0056).
 *
 * La ambigüedad «¿primer segmento = org o ruta?» no se resuelve mirando listas
 * de rutas: por construcción, la URL canónica SIEMPRE lleva la org primero y
 * las sin org son transicionales. Aquí lo que se guarda es el CONTRATO de
 * parsing que el middleware y los helpers de ruta de las apps comparten.
 */
import { afterEach, describe, expect, it } from 'bun:test';
import {
  canonicaConOrg,
  MARCA_ORG,
  orgActual,
  orgDeRuta,
  partirRuta,
  RE_SLUG_ORG,
  rutaConOrg,
  SIN_ORG_SIEMPRE,
} from './organizacion.js';

describe('orgDeRuta', () => {
  it('el primer segmento es la org cuando es un slug válido', () => {
    expect(orgDeRuta('acme/channels/x')).toBe('acme');
    expect(orgDeRuta('acme')).toBe('acme');
    expect(orgDeRuta('acme/')).toBe('acme');
  });

  it('sin un slug válido devuelve null, no inventa', () => {
    expect(orgDeRuta('')).toBeNull();
    // Un slug válido ES una org para esta función pura —«channels» es página de
    // Rooms y lo excluye el sinOrg del middleware, no el parsing: la exclusión
    // vive arriba, no aquí (ADR 0056).
    expect(orgDeRuta('channels/x')).toBe('channels');
    // El slug lo fija tenancy: mayúsculas, guion en los extremos o símbolos no.
    expect(orgDeRuta('Acme/x')).toBeNull();
    expect(orgDeRuta('-acme/x')).toBeNull();
    expect(orgDeRuta('ac-/x')).toBeNull();
    expect(orgDeRuta('a'.repeat(64) + '/x')).toBeNull();
  });
});

describe('RE_SLUG_ORG', () => {
  it('acepta exactamente lo que acepta platform/pkg/tenancy', () => {
    expect(RE_SLUG_ORG.test('a')).toBe(true);
    expect(RE_SLUG_ORG.test('mi-org-1')).toBe(true);
    expect(RE_SLUG_ORG.test('')).toBe(false);
    expect(RE_SLUG_ORG.test('mi_org')).toBe(false);
    expect(RE_SLUG_ORG.test('org-')).toBe(false);
    expect(RE_SLUG_ORG.test('-org')).toBe(false);
    expect(RE_SLUG_ORG.test('a'.repeat(63))).toBe(true);
    expect(RE_SLUG_ORG.test('a'.repeat(64))).toBe(false);
  });
});

describe('partirRuta', () => {
  it('separa org y resto pelando la barra', () => {
    expect(partirRuta('acme/channels/x')).toEqual({ org: 'acme', resto: 'channels/x' });
    expect(partirRuta('acme')).toEqual({ org: 'acme', resto: '' });
    expect(partirRuta('acme/')).toEqual({ org: 'acme', resto: '' });
  });

  it('todo slug válido encabeza una URL canónica; lo inválido queda intacto', () => {
    // Mismo contrato puro: solo los segmentos SIN forma de slug dejan org null.
    expect(partirRuta('channels/x')).toEqual({ org: 'channels', resto: 'x' });
    expect(partirRuta('/edit?id=1')).toEqual({ org: null, resto: 'edit?id=1' });
  });
});

describe('rutaConOrg', () => {
  it('compone base + org + ruta, y sin org deja la ruta como estaba', () => {
    expect(rutaConOrg('/rooms', 'acme', '/channels/x')).toBe('/rooms/acme/channels/x');
    expect(rutaConOrg('/rooms', 'acme', '/')).toBe('/rooms/acme/');
    expect(rutaConOrg('/rooms', 'acme', '/edit?id=1')).toBe('/rooms/acme/edit?id=1');
    expect(rutaConOrg('/rooms', null, '/channels/x')).toBe('/rooms/channels/x');
    expect(rutaConOrg('/rooms', undefined, '/edit')).toBe('/rooms/edit');
  });
});

describe('SIN_ORG_SIEMPRE', () => {
  it('la plomería del BFF y del navegador no se escopia nunca', () => {
    // El middleware y los helpers de ruta la consultan: si un prefijo común se
    // pierde aquí, las peticiones a la API empezarían a llevar org y los
    // favicons, un 302 por carga con sesión.
    for (const prefijo of ['api', 'auth', 'healthz', 'sw.js', '_astro']) {
      expect(SIN_ORG_SIEMPRE).toContain(prefijo);
    }
  });
});

describe('canonicaConOrg', () => {
  it('compone la canónica con la org activa, query incluida', () => {
    expect(canonicaConOrg(new URL('https://one.sytue.eu/one/datos'), '/one', 'acme')).toBe(
      '/one/acme/datos',
    );
    expect(
      canonicaConOrg(
        new URL('https://rooms.sytue.eu/rooms/channels/x?message=y'),
        '/rooms',
        'acme',
      ),
    ).toBe('/rooms/acme/channels/x?message=y');
  });

  it('sin org en la sesión no hay canónica que componer', () => {
    expect(canonicaConOrg(new URL('https://one.sytue.eu/one/datos'), '/one', null)).toBeNull();
    expect(canonicaConOrg(new URL('https://one.sytue.eu/one/datos'), '/one', undefined)).toBeNull();
  });

  it('la marca acredita que la petición ya llegó por el rewrite: no se toca', () => {
    expect(
      canonicaConOrg(new URL(`https://one.sytue.eu/one/datos?${MARCA_ORG}=acme`), '/one', 'acme'),
    ).toBeNull();
  });

  it('una marca INVÁLIDA o vacía no acredita nada y no bloquea el salto canónico', () => {
    // Aceptar `has()` a secas dejaba la página fuera de la canónica PARA SIEMPRE:
    // cualquier `?__org=` vacío o artesanal en un enlace rompía la convergencia.
    expect(
      canonicaConOrg(new URL(`https://one.sytue.eu/one/datos?${MARCA_ORG}=Acme`), '/one', 'acme'),
    ).toBe('/one/acme/datos?__org=Acme');
    expect(
      canonicaConOrg(new URL(`https://one.sytue.eu/one/datos?${MARCA_ORG}=`), '/one', 'acme'),
    ).toBe('/one/acme/datos?__org=');
  });

  it('fuera del prefijo de la app (servida en la raíz del host), nada', () => {
    expect(canonicaConOrg(new URL('https://one.sytue.eu/datos'), '/one', 'acme')).toBeNull();
    expect(canonicaConOrg(new URL('https://one.sytue.eu/one'), '/one', 'acme')).toBeNull();
  });
});

describe('orgActual', () => {
  /** `orgActual` lee `window.location`: se finge con la URL de verdad. */
  function conVentana(url: string): void {
    (globalThis as { window?: unknown }).window = { location: new URL(url) };
  }

  afterEach(() => {
    delete (globalThis as { window?: unknown }).window;
  });

  it('en SSR no hay URL de navegador y devuelve null', () => {
    // Sin `window` en el global (bun no la trae): el bucle de arriba ya lo borró.
    expect(orgActual('/rooms')).toBeNull();
  });

  it('lee la org del primer segmento tras el prefijo', () => {
    conVentana('https://rooms.sytue.eu/rooms/acme/channels/x');
    expect(orgActual('/rooms')).toBe('acme');
  });

  it('fuera del prefijo de la app, null', () => {
    conVentana('https://rooms.sytue.eu/otra/acme');
    expect(orgActual('/rooms')).toBeNull();
  });

  it('lo que no es slug no es org', () => {
    conVentana('https://rooms.sytue.eu/rooms/api/channels');
    expect(orgActual('/rooms')).toBe('api'); // el middleware lo deja pasar: sinOrg
    conVentana('https://rooms.sytue.eu/rooms/.well-known/assetlinks.json');
    expect(orgActual('/rooms')).toBeNull();
  });

  it('la marca __org manda sobre un path sin org (la vuelta del login)', () => {
    // El rewrite interno sirve `/rooms/channels/x` con `?__org=acme`: mirar el
    // primer segmento confundiría la RUTA `channels` con una org.
    conVentana('https://rooms.sytue.eu/rooms/channels/x?__org=acme&rooms=r1');
    expect(orgActual('/rooms')).toBe('acme');
  });

  it('una marca que no es slug no cuenta', () => {
    conVentana('https://rooms.sytue.eu/rooms/channels/x?__org=NOPE');
    expect(orgActual('/rooms')).toBe('channels');
  });
});
