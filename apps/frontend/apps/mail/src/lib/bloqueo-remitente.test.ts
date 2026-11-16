/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * «Bloquear a …» (MAIL-13): un clic crea la regla «de esta dirección → Spam» y mueve
 * el hilo a Spam; si el movimiento falla no queda la regla, y «Deshacer» quita la
 * regla Y devuelve cada mensaje a su carpeta. Ejecutar: `bun test`.
 */
import { expect, test } from 'bun:test';
import type { MailFilterInput } from '@sytue/api-clients/mail';
import {
  bloquearRemitente,
  deshacerBloqueo,
  ofrecerBloqueo,
  reglaDeBloqueo,
  type LlamadasBloqueo,
} from './bloqueoRemitente.js';
import { useT } from '../i18n/index.js';

/** Un servidor simulado: las reglas que tiene y dónde está cada mensaje. */
function servidor(opts: { moverFalla?: boolean } = {}): LlamadasBloqueo & {
  reglas: Map<string, MailFilterInput>;
  carpetas: Map<string, string>;
} {
  const reglas = new Map<string, MailFilterInput>();
  const carpetas = new Map([
    ['m1', 'in'],
    ['m2', 'se'],
  ]);
  let n = 0;
  return {
    reglas,
    carpetas,
    guardarRegla: async (input) => {
      const id = `r${++n}`;
      reglas.set(id, input);
      return { filter: { id } };
    },
    borrarRegla: async (id) => {
      reglas.delete(id);
    },
    mover: async (ids, carpeta) => {
      if (opts.moverFalla) throw new Error('500');
      for (const id of ids) carpetas.set(id, carpeta);
    },
  };
}

test('la regla es la dirección exacta a Spam, con un nombre legible', () => {
  const t = useT('es');
  const regla = reglaDeBloqueo(
    ' ana@example.eu ',
    'junk',
    t('reader.block', { sender: 'ana@example.eu' }),
  );
  expect(regla).toEqual({
    name: 'Bloquear a ana@example.eu',
    enabled: true,
    matchFrom: 'ana@example.eu',
    moveTo: 'junk',
  });
  expect(useT('en')('reader.block', { sender: 'x@y.eu' })).toBe('Block x@y.eu');
});

test('un clic crea la regla y mueve el hilo entero a Spam', async () => {
  const s = servidor();
  const id = await bloquearRemitente(
    reglaDeBloqueo('ana@example.eu', 'junk', 'n'),
    ['m1', 'm2'],
    s,
  );
  expect([...s.reglas.keys()]).toEqual([id]);
  expect(s.reglas.get(id)?.matchFrom).toBe('ana@example.eu');
  expect(Object.fromEntries(s.carpetas)).toEqual({ m1: 'junk', m2: 'junk' });
});

test('si no se puede mover, tampoco queda la regla', async () => {
  const s = servidor({ moverFalla: true });
  const bloqueo = bloquearRemitente(reglaDeBloqueo('ana@example.eu', 'junk', 'n'), ['m1'], s);
  await expect(bloqueo).rejects.toThrow();
  expect(s.reglas.size).toBe(0);
});

test('«Deshacer» quita la regla y devuelve cada mensaje a SU carpeta', async () => {
  const s = servidor();
  const id = await bloquearRemitente(
    reglaDeBloqueo('ana@example.eu', 'junk', 'n'),
    ['m1', 'm2'],
    s,
  );
  const ok = await deshacerBloqueo(
    id,
    new Map([
      ['in', ['m1']],
      ['se', ['m2']],
    ]),
    s,
  );
  expect(ok).toBe(true);
  expect(s.reglas.size).toBe(0);
  expect(Object.fromEntries(s.carpetas)).toEqual({ m1: 'in', m2: 'se' });
});

test('«Deshacer» a medias lo dice, pero quita lo que puede', async () => {
  const s = servidor();
  const id = await bloquearRemitente(reglaDeBloqueo('ana@example.eu', 'junk', 'n'), ['m1'], s);
  const ok = await deshacerBloqueo(id, new Map([['in', ['m1']]]), {
    borrarRegla: s.borrarRegla,
    mover: () => Promise.reject(new Error('500')),
  });
  expect(ok).toBe(false);
  expect(s.reglas.size).toBe(0);
});

test('no se ofrece en Spam, sin dirección ni a uno mismo; a un no-reply sí', () => {
  const ctx = { enSpam: false, propias: ['yo@sytue.eu', 'alias@sytue.eu'] };
  expect(ofrecerBloqueo('ana@example.eu', ctx)).toBe(true);
  expect(ofrecerBloqueo('no-reply@boletin.eu', ctx)).toBe(true);
  expect(ofrecerBloqueo('ana@example.eu', { ...ctx, enSpam: true })).toBe(false);
  expect(ofrecerBloqueo('', ctx)).toBe(false);
  expect(ofrecerBloqueo(' Alias@Sytue.eu ', ctx)).toBe(false);
});
