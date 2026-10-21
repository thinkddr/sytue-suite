/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Ningún texto de Sytue Photos lleva emojis (decisión del dueño: cero emojis en la
 * interfaz). Los ocho preajustes de Sytue AI del editor los llevaban delante en los
 * veintiséis idiomas —«✨ Mejora automática», «🌅 Atardecer cálido»…— y en la app nativa
 * también, que sale de estos catálogos.
 */
import { describe, expect, test } from 'bun:test';
import { readdirSync } from 'node:fs';
import { join } from 'node:path';

const EMOJI = /[\p{Extended_Pictographic}\u{FE0F}]/u;
const dir = join(import.meta.dir, 'messages');

describe('sin emojis', () => {
  for (const f of readdirSync(dir).filter((n) => n.endsWith('.ts'))) {
    test(f, async () => {
      const mod = (await import(join(dir, f))) as Record<string, Record<string, unknown>>;
      const catalogo = Object.values(mod)[0] ?? {};
      const con = Object.entries(catalogo)
        .filter(([, v]) => EMOJI.test(typeof v === 'string' ? v : JSON.stringify(v)))
        .map(([k]) => k);
      expect(con).toEqual([]);
    });
  }
});
