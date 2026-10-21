/**
 * Test de las utilidades de foco de `styles.css`.
 *
 * Aquí vive ahora la garantía de accesibilidad que antes repetía cada componente en su
 * propia cadena de clases: 163 copias de la misma sombra, de las que una había perdido
 * el segundo anillo sin que nada lo detectara. Con una sola definición, este fichero es
 * el único sitio donde puede volver a romperse — así que se comprueba el TEXTO del CSS,
 * no el de ningún componente.
 *
 * Ejecutar: `bun test`.
 */
import { expect, test } from 'bun:test';

const css = await Bun.file(new URL('./styles.css', import.meta.url)).text();

/** Cuerpo de una regla por su selector, con los saltos y sangrías colapsados. */
function rule(selector: string): string {
  const i = css.indexOf(selector + ' {');
  expect(i, `no existe la regla ${selector}`).toBeGreaterThan(-1);
  return css.slice(i, css.indexOf('}', i)).replace(/\s+/g, ' ');
}

test('.sytue-focus-ring es UN anillo y nada más', () => {
  const r = rule('.sytue-focus-ring:focus-visible');
  expect(r).toContain('var(--sytue-focus-ring)');
  // El contorno neutro que acompañaba al latón se retiró: existía porque el latón
  // viejo no llegaba al 3:1 de WCAG 1.4.11, y el token nuevo sí (≥3,17:1 sobre las
  // ocho superficies). Lo que se veía de él era una línea negra alrededor del campo.
  expect(r).not.toContain('var(--sytue-focus-contrast)');
  // Sin `outline: none` se pintarían los dos: el del navegador y el nuestro.
  expect(r).toContain('outline: none');
});

test('la variante inset abre el hueco ANTES del anillo, o se funde con el relleno', () => {
  const r = rule('.sytue-focus-ring-inset:focus-visible');
  expect(r).toContain('var(--sytue-surface)');
  // El orden importa: primero el hueco del color de la superficie, luego el anillo.
  // Al revés, el anillo queda pegado al relleno de acento y deja de leerse.
  expect(r.indexOf('var(--sytue-surface)')).toBeLessThan(r.indexOf('var(--sytue-focus-ring)'));
});

test('label y meta fijan el COLOR: sin él serían text-xs/text-2xs con otro nombre', () => {
  // Es lo que distingue a estos dos de display/h1-h4/overline, que solo dan tipografía.
  // Son roles semánticos —la etiqueta de un control, el dato accesorio—, y el valor está
  // en la combinación: en el código medido, tamaño y color viajaban siempre juntos.
  const label = rule('.sytue-label');
  expect(label).toContain('var(--text-xs)');
  expect(label).toContain('var(--sytue-text-muted)');

  const meta = rule('.sytue-meta');
  expect(meta).toContain('var(--text-xs)');
  expect(meta).toContain('var(--sytue-text-subtle)');
});

test('meta y label comparten tamaño y se distinguen por el COLOR', () => {
  // Ambos son texto de apoyo a 12 px; lo que los separa es el peso visual, no la escala:
  // `label` nombra un control (muted) y `meta` es el dato accesorio (subtle, más tenue).
  // Si alguien iguala también los colores, los dos roles dejan de decir cosas distintas.
  expect(rule('.sytue-meta')).toContain('--text-xs)');
  expect(rule('.sytue-label')).toContain('--text-xs)');
  expect(rule('.sytue-meta')).toContain('--sytue-text-subtle');
  expect(rule('.sytue-label')).toContain('--sytue-text-muted');
});

test('la variante densa es MÁS pequeña que el rol base, o no sería una densidad', () => {
  // 11 px para herramientas densas (el editor: ribbon y paneles); 12 px para las apps de
  // producto. Igualarlos borraría la distinción que justifica que existan las dos.
  expect(rule('.sytue-meta-dense')).toContain('--text-2xs');
  expect(rule('.sytue-meta-dense')).toContain('--sytue-text-subtle');
  expect(rule('.sytue-meta')).not.toContain('--text-2xs');
});

test('las utilidades NO van tras :where(), que las dejaría en especificidad cero', () => {
  // `:where()` existe para que un componente pueda ajustar la regla global — y esa misma
  // comodidad hace que cualquier `shadow-*` de base la apague sin error ni aviso. Estas
  // dos son explícitas a propósito.
  for (const sel of ['.sytue-focus-ring:focus-visible', '.sytue-focus-ring-inset:focus-visible']) {
    expect(css.slice(Math.max(0, css.indexOf(sel) - 120), css.indexOf(sel))).not.toContain(
      ':where(',
    );
  }
});

test('al imprimir se va el cromo y el contenido deja de estar recortado', () => {
  // No había NI UNA regla de impresión en la suite: Ctrl+P sacaba la barra
  // superior, el carril y la cinta, y del contenido solo la primera pantalla
  // —los lienzos viven en contenedores con altura de viewport y `overflow`, que
  // en papel significan «recorta aquí»—. En un procesador de textos, imprimir es
  // una de las razones de existir del producto.
  const bloque = css.slice(css.indexOf('@media print'));
  expect(bloque).toBeTruthy();
  // El cromo se oculta por su ROL, que ya está en el marcado compartido, para no
  // depender de que cada app se acuerde de poner una clase.
  for (const sel of ["[role='banner']", "[role='toolbar']", "[role='tablist']"]) {
    expect(bloque).toContain(sel);
  }
  // Y lo que de verdad arreglaba el recorte.
  expect(bloque).toContain('overflow: visible !important');
  expect(bloque).toContain('max-height: none !important');
});

test('un diálogo abierto se imprime ÉL, no la página de debajo', () => {
  // Lo que se está mirando es el diálogo; imprimir el fondo saca además el velo,
  // que en papel es una mancha gris sobre el contenido.
  const bloque = css.slice(css.indexOf('@media print'));
  expect(bloque).toContain('dialog[open]');
  expect(bloque).toContain(':has(dialog[open])');
});
