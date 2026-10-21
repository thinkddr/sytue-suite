/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 */
import { expect, test } from 'bun:test';
import { bloquesDe, textoPlano, trozosDe } from './markdown.js';

test('la negrita y la cursiva salen como tramos, sin marcadores', () => {
  expect(trozosDe('hola **mundo** y *adiós*')).toEqual([
    { texto: 'hola ' },
    { texto: 'mundo', negrita: true },
    { texto: ' y ' },
    { texto: 'adiós', cursiva: true },
  ]);
});

// `2 * 3 = 6` no debe perder su asterisco por parecerse a una cursiva.
test('un marcador impar se queda como texto', () => {
  expect(trozosDe('2 * 3 = 6')).toEqual([{ texto: '2 * 3 = 6' }]);
});

test('los títulos con almohadilla salen con su nivel', () => {
  expect(bloquesDe('# Uno\n## Dos\n### Tres').map((b) => b.tipo)).toEqual(['h1', 'h2', 'h3']);
});

// El caso del informe: el modelo escribe `**Introducción**` como si fuera un
// título. Sin esta regla, el documento salía sin un solo título de verdad.
test('un párrafo que es solo negrita se trata como título', () => {
  const b = bloquesDe('**Introducción**');
  expect(b[0]?.tipo).toBe('h2');
  expect(b[0]?.trozos).toEqual([{ texto: 'Introducción' }]);
});

test('una frase larga en negrita sigue siendo un párrafo', () => {
  const larga = `**${'x'.repeat(120)}**`;
  expect(bloquesDe(larga)[0]?.tipo).toBe('p');
});

test('las listas distinguen viñeta de numerada', () => {
  const b = bloquesDe('- uno\n* dos\n1. tres\n2) cuatro');
  expect(b.map((x) => x.tipo)).toEqual(['li', 'li', 'li', 'li']);
  expect(b.map((x) => x.ordenada)).toEqual([false, false, true, true]);
});

test('las líneas en blanco no producen bloques', () => {
  expect(bloquesDe('uno\n\n\ndos')).toHaveLength(2);
});

// Degradación correcta: lo que no se reconoce se queda como texto. Peor formato,
// nunca contenido perdido.
test('lo que no se reconoce sobrevive como párrafo', () => {
  const b = bloquesDe('| tabla | rara |');
  expect(b[0]?.tipo).toBe('p');
  expect(b[0]?.trozos[0]?.texto).toContain('tabla');
});

test('el texto plano conserva todo y marca las listas', () => {
  expect(textoPlano(bloquesDe('# T\n- uno\ntexto **fuerte**'))).toBe('T\n• uno\ntexto fuerte');
});

test('el documento del informe sale con títulos y lista', () => {
  const tipos = bloquesDe(
    '**Regulación más reciente**\n\n**Introducción**\nLa Unión Europea (UE) ha sido pionera.\n\n1. **Ámbito de aplicación**\n   - El GDPR se aplica a todas.',
  ).map((b) => b.tipo);
  expect(tipos).toEqual(['h2', 'h2', 'p', 'li', 'li']);
});
