/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Tres cosas de Mi cuenta que no se ven en una captura y que un lector de
 * pantalla o el tema oscuro sí delatan. Se comprueban sobre el fuente porque
 * son reglas de escritura: cualquier tarjeta nueva tiene que cumplirlas.
 */
import { describe, expect, test } from 'bun:test';
import { readdirSync, readFileSync } from 'node:fs';
import { join } from 'node:path';
import { en } from '../i18n/messages/en.js';

const SRC = join(import.meta.dir, '..');
const leer = (ruta: string) => readFileSync(join(SRC, ruta), 'utf8');
const componentes = readdirSync(join(SRC, 'components'))
  .filter((f) => f.endsWith('.tsx'))
  .map((f) => ({ fichero: f, fuente: leer(join('components', f)) }));

describe('contraste', () => {
  // `--sytue-danger` y `--sytue-success` son RELLENOS: en oscuro el rojo se queda
  // en ~3,17:1 sobre la superficie y un texto necesita 4,5:1. La tinta legible es
  // la variante `-text` (también en `hover:`).
  test('ningún token semántico de relleno se usa como color de texto', () => {
    const malos = componentes.flatMap(({ fichero, fuente }) =>
      fuente
        .split('\n')
        .map((linea, i) => ({ linea, n: i + 1 }))
        .filter(({ linea }) => /\btext-(danger|success|warning)(?![-\w])/.test(linea))
        .map(({ n, linea }) => `${fichero}:${n}: ${linea.trim()}`),
    );
    expect(malos).toEqual([]);
  });
});

describe('anuncios de estado', () => {
  // Un resultado («Guardado», «No se pudo…») que aparece sin región viva solo lo
  // ve quien mira; quien usa lector de pantalla se queda sin saber si funcionó.
  // Se exige `role="status"` o `role="alert"` en el elemento que lo pinta o en
  // su contenedor inmediato (unas pocas líneas por encima).
  const RESULTADO =
    /t\([`'](?:profile\.(?:saved|failed)|recovery\.(?:sent|removed|\$\{fallo\})|security\.password\.done|plan\.error|[a-z_]+\.failed)[`']\)/;

  test('cada mensaje de resultado está dentro de una región que se anuncia', () => {
    const mudos: string[] = [];
    let vistos = 0;
    for (const { fichero, fuente } of componentes) {
      const lineas = fuente.split('\n');
      lineas.forEach((linea, i) => {
        // `return t(…)` es lógica (el texto de un error que luego se pinta), no JSX.
        if (!RESULTADO.test(linea) || /\breturn\b/.test(linea)) return;
        vistos++;
        const contexto = lineas.slice(Math.max(0, i - 4), i + 1).join('\n');
        if (!/role="(status|alert)"/.test(contexto)) mudos.push(`${fichero}:${i + 1}`);
      });
    }
    // Que la expresión siga encontrando los mensajes: si un renombrado la deja
    // sin casos, el test pasaría sin vigilar nada.
    expect(vistos).toBeGreaterThanOrEqual(10);
    expect(mudos).toEqual([]);
  });
});

describe('cabecera traducida', () => {
  test('el <title> y la descripción salen del catálogo, no de un literal', () => {
    const layout = leer('layouts/AppLayout.astro');
    expect(layout).toContain("t('app.title')");
    expect(layout).toContain("content={t('app.description')}");
    expect(layout).not.toMatch(/title = '[^']*'/);
    expect(layout).not.toMatch(/name="description"\s+content="/);
    // La página no debe pisar el título traducido con uno fijo.
    expect(leer('pages/index.astro')).not.toMatch(/<AppLayout[^>]*title="/);
  });

  test('cada idioma traduce la descripción en vez de copiar el inglés', () => {
    const dir = join(SRC, 'i18n', 'messages');
    for (const f of readdirSync(dir).filter((x) => x.endsWith('.ts') && x !== 'en.ts')) {
      const fuente = readFileSync(join(dir, f), 'utf8');
      const m = /'app\.description':\s*'([^']+)'/.exec(fuente);
      expect(m, f).not.toBeNull();
      expect(m?.[1], f).not.toBe(en['app.description']);
    }
  });
});
