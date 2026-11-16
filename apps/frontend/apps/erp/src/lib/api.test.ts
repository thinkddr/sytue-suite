/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 */
import { afterEach, describe, expect, it } from 'bun:test';
import { readdirSync, readFileSync } from 'node:fs';
import { join } from 'node:path';
import { en } from '../i18n/messages/en.js';
import { es } from '../i18n/messages/es.js';
import { ERRORES, ErrorApi, api, claveDeError, esSinPermiso, mensajeDeError } from './api.js';

const t = (clave: string): string => `«${clave}»`;

/** Los `code` que el servicio declara con `httpx.NewError`, leídos de su código. */
function codigosDelServicio(): string[] {
  const dir = join(import.meta.dir, '../../../../../../services/syerp/internal/http');
  const codigos = new Set<string>();
  for (const fichero of readdirSync(dir)) {
    if (!fichero.endsWith('.go') || fichero.endsWith('_test.go')) continue;
    const fuente = readFileSync(join(dir, fichero), 'utf8');
    for (const m of fuente.matchAll(/NewError\(http\.Status[A-Za-z]+,\s*"([a-z_]+)"/g)) {
      codigos.add(m[1] as string);
    }
  }
  return [...codigos].sort();
}

describe('ERRORES', () => {
  it('tiene fila para cada código que el servicio puede mandar', () => {
    const codigos = codigosDelServicio();
    // Si la lectura dejara de encontrar códigos, el test pasaría sin mirar nada.
    expect(codigos.length).toBeGreaterThan(40);
    const sinFila = codigos.filter(
      // `sin_verbo_` es una familia (`sin_verbo_asentar`…) y va por prefijo.
      (c) => c !== 'sin_verbo_' && !(c in ERRORES),
    );
    expect(sinFila).toEqual([]);
    expect(claveDeError(new ErrorApi('x', 403, {}, 'sin_verbo_asentar'))).toBe(
      'error.code.actionNotAllowed',
    );
  });

  it('apunta a claves que existen y están traducidas en todos los idiomas', async () => {
    const dir = join(import.meta.dir, '../i18n/messages');
    const claves = [
      ...new Set([
        ...Object.values(ERRORES),
        'error.code.actionNotAllowed',
        'error.code.checkFields',
      ]),
    ];
    for (const fichero of readdirSync(dir)) {
      const idioma = fichero.replace(/\.ts$/, '');
      const modulo = (await import(join(dir, fichero))) as Record<string, Record<string, unknown>>;
      const catalogo = modulo[idioma] as Record<string, unknown>;
      for (const clave of claves) {
        expect({ idioma, clave, hay: typeof catalogo[clave] === 'string' }).toEqual({
          idioma,
          clave,
          hay: true,
        });
      }
    }
    // Traducidas de verdad, no el inglés copiado.
    const copiadas = claves.filter(
      (c) => (es as Record<string, unknown>)[c] === (en as Record<string, unknown>)[c],
    );
    expect(copiadas).toEqual([]);
  });
});

describe('mensajeDeError', () => {
  it('traduce el 403 sin_permiso en vez de pintar el respaldo en inglés', () => {
    const err = new ErrorApi('You do not have access to this module.', 403, {}, 'sin_permiso');
    expect(esSinPermiso(err)).toBe(true);
    expect(mensajeDeError(err, t)).toBe('«error.sinPermiso»');
  });

  it('traduce los errores de certificado de VERI*FACTU', () => {
    expect(mensajeDeError(new ErrorApi('invalid', 422, {}, 'certificado_invalido'), t)).toBe(
      '«vf.error.invalidCertificate»',
    );
    expect(
      mensajeDeError(new ErrorApi('mismatch', 422, {}, 'certificado_nif_no_coincide'), t),
    ).toBe('«vf.error.certificateTaxIdMismatch»');
  });

  it('traduce los del libro, el banco y Sytue AI por su código', () => {
    const ingles = 'That accounting period is closed. Reopen it first, or post with a later date.';
    expect(mensajeDeError(new ErrorApi(ingles, 409, {}, 'periodo_cerrado'), t)).toBe(
      '«error.code.periodClosed»',
    );
    expect(mensajeDeError(new ErrorApi('x', 409, {}, 'ya_justificado'), t)).toBe(
      '«error.code.alreadyJustified»',
    );
    expect(mensajeDeError(new ErrorApi('x', 429, {}, 'ia_limite'), t)).toBe('«error.code.aiLimit»');
  });

  it('nunca enseña la frase inglesa del servicio', () => {
    // Un código que nadie ha traducido todavía…
    expect(mensajeDeError(new ErrorApi('Something new.', 409, {}, 'codigo_nuevo'), t)).toBe(
      '«error.generic»',
    );
    // …ni un error sin código.
    expect(mensajeDeError(new ErrorApi('HTTP 502', 502), t)).toBe('«error.generic»');
    expect(esSinPermiso(new ErrorApi('x', 403))).toBe(false);
  });

  it('la validación con detalle por campo manda a mirar los campos', () => {
    const conCampos = new ErrorApi(
      'validation failed: fecha',
      422,
      { fecha: 'YYYY-MM-DD' },
      'unprocessable',
    );
    expect(mensajeDeError(conCampos, t)).toBe('«error.code.checkFields»');
    expect(mensajeDeError(new ErrorApi('x', 422, {}, 'unprocessable'), t)).toBe(
      '«error.code.unprocessable»',
    );
  });

  it('conserva el texto de un error que lanza la propia pantalla, ya traducido', () => {
    expect(mensajeDeError(new Error('Reanuda la venta'), t)).toBe('Reanuda la venta');
    // …pero no el corte de red, cuyo texto pone el navegador en inglés.
    expect(mensajeDeError(new TypeError('Failed to fetch'), t)).toBe('«error.generic»');
    expect(mensajeDeError('nada', t)).toBe('«error.generic»');
  });
});

describe('api', () => {
  const fetchOriginal = globalThis.fetch;
  afterEach(() => {
    globalThis.fetch = fetchOriginal;
  });

  function responde(estado: number, cuerpo: unknown): void {
    globalThis.fetch = (async () =>
      new Response(JSON.stringify(cuerpo), { status: estado })) as unknown as typeof fetch;
  }

  it('lee el código del sobre del servicio', async () => {
    responde(409, { error: { code: 'ya_asentado', message: 'already posted' } });
    const err = (await api('v1/x').catch((e: unknown) => e)) as ErrorApi;
    expect(err.codigo).toBe('ya_asentado');
    expect(mensajeDeError(err, t)).toBe('«error.code.alreadyPosted»');
  });

  it('lee el código suelto del BFF cuando el servicio no contesta', async () => {
    responde(502, { error: 'upstream_unavailable' });
    const err = (await api('v1/x').catch((e: unknown) => e)) as ErrorApi;
    expect(mensajeDeError(err, t)).toBe('«error.offline»');
  });

  it('lee el código suelto del BFF cuando la sesión ya no vale', async () => {
    responde(401, { error: 'unauthenticated' });
    const err = (await api('v1/x').catch((e: unknown) => e)) as ErrorApi;
    expect(err.codigo).toBe('unauthenticated');
    expect(mensajeDeError(err, t)).toBe('«error.code.sessionExpired»');
  });
});
