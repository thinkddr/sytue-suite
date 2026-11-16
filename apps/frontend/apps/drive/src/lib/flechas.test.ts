/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 */
/**
 * Las flechas por la lista y la cuadrícula de Drive. La cuenta (qué fila o qué tesela
 * toca) es de `@sytue/ui` (`navegacionRejilla.test.ts`); aquí se vigila el fuente para
 * que las dos vistas la usen. La pareja nativa es `TestLasFlechas*` de `apps/fyne/apps/drive`.
 */
import { expect, test } from 'bun:test';
import { readFileSync } from 'node:fs';

const explorador = readFileSync(new URL('../components/FileBrowser.tsx', import.meta.url), 'utf8');

test('la lista lleva la fila a la vista con scrollToIndex y la enfoca al pintarse', () => {
  expect(explorador).toMatch(
    /moverFocoEnListaVirtual\(\s*e,\s*index,\s*items\.length,\s*\(i\) => handle\.current\?\.scrollToIndex\(i, \{ align: 'auto' \}\),\s*\(i\) => lista\?\.querySelector<HTMLElement>\(`\[data-drive-fila="\$\{i\}"\]`\) \?\? null,/,
  );
  expect(explorador).toContain('data-drive-fila={index}');
  expect(explorador).toContain('handleRef={handle}');
  // Intro y Espacio siguen abriendo; las demás teclas van a la lista.
  expect(explorador).toMatch(/onOpen\(item\);\s*\} else onMoverFoco\?\.\(e, index\);/);
});

test('la cuadrícula (y la franja de carpetas) mueve el foco por las teselas', () => {
  expect(explorador).toMatch(
    /moverFocoEnRejilla\(e, \[\s*\.\.\.e\.currentTarget\.querySelectorAll<HTMLElement>\(':scope > li > \[data-drive-item\]'\),\s*\]\)/,
  );
});
