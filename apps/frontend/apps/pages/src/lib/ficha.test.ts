/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 */
import { describe, expect, it } from 'bun:test';
import {
  exigeRegistro,
  legalFaltante,
  leerFallo,
  ejecutarPublicacion,
  lineas,
  mensajeDeAjustes,
  mensajeDeDominio,
  mensajeDePublicacion,
  primerEsquemaInvalido,
  primerOrigenInvalido,
} from './ficha';

describe('contrato HTTP de Pages', () => {
  it('lee el código y el reinicio dentro del sobre canónico de errores', async () => {
    const res = Response.json(
      {
        error: {
          code: 'ai_quota_exceeded',
          details: { reset_at: '2026-11-01T00:00:00Z' },
        },
      },
      { status: 429 },
    );
    expect(await leerFallo(res)).toEqual({
      status: 429,
      code: 'ai_quota_exceeded',
      details: { reset_at: '2026-11-01T00:00:00Z' },
    });
  });

  it('una preview viene de publicar por el BFF y conserva la URL tokenizada', async () => {
    const original = globalThis.fetch;
    const llamadas: { url: string; kind: unknown }[] = [];
    const url = 'https://aurora.sites.sytue.eu/_vista/token-real/';
    globalThis.fetch = (async (entrada: string, init?: RequestInit) => {
      llamadas.push({ url: entrada, kind: JSON.parse(String(init?.body)) });
      return Response.json({ url });
    }) as typeof fetch;
    try {
      expect(await ejecutarPublicacion('sitio-1', 'preview')).toEqual({ ok: true, url });
      expect(llamadas[0]?.url).toContain('/api/publicar/sitio-1');
      expect(llamadas[0]?.kind).toEqual({ kind: 'preview' });
    } finally {
      globalThis.fetch = original;
    }
  });

  it('una preview sin URL no se presenta como completada', async () => {
    const original = globalThis.fetch;
    globalThis.fetch = (async () => Response.json({})) as unknown as typeof fetch;
    try {
      expect(await ejecutarPublicacion('sitio-1', 'preview')).toEqual({
        ok: false,
        fallo: { status: 502, code: 'bad_gateway' },
      });
    } finally {
      globalThis.fetch = original;
    }
  });
});

const completo = {
  holder_name: 'Panadería Aurora',
  tax_id: '12345678Z',
  country: 'ES',
  address: { street: 'Calle Mayor 1', postal_code: '28001', city: 'Madrid' },
  email: 'legal@aurora.es',
};

describe('legalFaltante', () => {
  it('vacío: faltan todos los obligatorios, en el orden del formulario', () => {
    expect(legalFaltante(undefined)).toEqual([
      'holder_name',
      'tax_id',
      'country',
      'street',
      'postal_code',
      'city',
      'email',
    ]);
  });

  it('completo: nada falta; un espacio no cuenta como dato', () => {
    expect(legalFaltante(completo)).toEqual([]);
    expect(legalFaltante({ ...completo, email: '   ' })).toEqual(['email']);
  });

  it('una sociedad española exige el Registro Mercantil; un autónomo, no', () => {
    const sociedad = { ...completo, tax_id: 'b12345678' };
    expect(exigeRegistro(sociedad)).toBe(true);
    expect(legalFaltante(sociedad)).toEqual(['registry']);
    expect(legalFaltante({ ...sociedad, registry: 'Madrid, T. 1' })).toEqual([]);
    expect(exigeRegistro({ ...sociedad, country: 'FR' })).toBe(false);
  });
});

describe('mensajes de rechazo', () => {
  it('el código manda sobre el estado', () => {
    expect(mensajeDePublicacion({ status: 422, code: 'legal_data_missing' })).toBe(
      'publish.err_legal',
    );
    expect(mensajeDePublicacion({ status: 409, code: 'too_many_previews' })).toBe(
      'publish.err_previews',
    );
  });

  it('sin código se decide por el estado', () => {
    expect(mensajeDePublicacion({ status: 403, code: '' })).toBe('publish.err_forbidden');
    expect(mensajeDePublicacion({ status: 413, code: '' })).toBe('publish.err_too_large');
    expect(mensajeDePublicacion({ status: 500, code: '' })).toBe('publish.err_generic');
  });

  it('dominios', () => {
    expect(mensajeDeDominio({ status: 409, code: 'domain_taken' })).toBe('domains.err_taken');
    expect(mensajeDeDominio({ status: 500, code: '' })).toBe('domains.err_generic');
  });

  it('ajustes: el identificador repetido tiene su propio mensaje', () => {
    expect(mensajeDeAjustes({ status: 409, code: 'slug_conflict' })).toBe(
      'settings.err_slug_taken',
    );
    expect(mensajeDeAjustes({ status: 400, code: 'invalid_input' })).toBe('settings.err_invalid');
    expect(mensajeDeAjustes({ status: 500, code: '' })).toBe('settings.err_generic');
  });
});

describe('orígenes y esquemas', () => {
  it('lineas descarta espacios y líneas vacías', () => {
    expect(lineas(' https://a.es \n\n  miapp\n')).toEqual(['https://a.es', 'miapp']);
  });

  it('primerOrigenInvalido da la posición desde 1, o 0 si todo vale', () => {
    expect(primerOrigenInvalido(['https://a.es', 'https://b.es/ruta'])).toBe(0);
    expect(primerOrigenInvalido(['https://a.es', 'http://b.es'])).toBe(2);
    expect(primerOrigenInvalido(['a.es'])).toBe(1);
  });

  it('primerEsquemaInvalido: empieza por letra y solo lleva letras, cifras, + - .', () => {
    expect(primerEsquemaInvalido(['miapp', 'sytue.app', 'a+b-c'])).toBe(0);
    expect(primerEsquemaInvalido(['miapp', '1app'])).toBe(2);
    expect(primerEsquemaInvalido(['mi app'])).toBe(1);
  });
});
