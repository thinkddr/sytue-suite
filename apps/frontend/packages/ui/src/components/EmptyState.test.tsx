/**
 * EmptyState v2 — las variantes que pide la spec §3.17: la marca por nombre y a tamaño
 * fijo, el error que se anuncia, el compacto de paneles y la acción secundaria.
 * Espejo de los `TestElVacio*` de `apps/fyne/ui`. Ejecutar: `bun test`.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { expect, test } from 'bun:test';
import { render, type ComponentChildren } from 'preact';
import { EmptyState, type EmptyStateProps } from './EmptyState.js';

if (typeof document === 'undefined') GlobalRegistrator.register();

function mount(props: EmptyStateProps): HTMLElement {
  const host = document.createElement('div');
  document.body.appendChild(host);
  render(<EmptyState {...props} />, host);
  return host.firstElementChild as HTMLElement;
}

const boton = (texto: string): ComponentChildren => <button type="button">{texto}</button>;

test('por defecto no cambia: <h3> sytue-h4, gap-3 px-6 py-8 y sin rol', () => {
  // Los ~200 usos actuales no se tienen que mover.
  const raiz = mount({ title: 'Nada', description: 'Aún', illustration: <svg /> });
  expect(raiz.className).toBe(
    'flex flex-col items-center justify-center gap-3 px-6 py-8 text-center',
  );
  expect(raiz.getAttribute('role')).toBeNull();
  const h = raiz.querySelector('h3');
  expect(h?.className).toBe('sytue-h4 text-text');
  expect(raiz.querySelector('p')?.className).toContain('max-w-sm text-base');
  expect(raiz.querySelector('.max-w-\\[200px\\]')).not.toBeNull();
});

test('icon pinta el glifo a 32 en text-subtle', () => {
  const raiz = mount({ title: 'Sin correo', icon: 'mail' });
  const svg = raiz.querySelector('svg');
  expect(svg?.getAttribute('width')).toBe('32');
  expect(svg?.getAttribute('aria-hidden')).toBe('true');
  expect(svg?.parentElement?.className).toContain('text-text-subtle');
  expect(svg?.parentElement?.className).toContain('mb-2');
});

test('spot pinta la ilustración de marca a 120', () => {
  const raiz = mount({ title: 'Sin correo', spot: 'emptyMail' });
  const dibujo = raiz.querySelector('span[aria-hidden="true"]') as HTMLElement | null;
  expect(dibujo?.style.maxWidth).toBe('120px');
  expect(dibujo?.querySelector('svg')).not.toBeNull();
});

test('icon gana a spot', () => {
  const raiz = mount({ title: 'x', icon: 'search', spot: 'emptySearch' });
  expect(raiz.querySelector('span[aria-hidden="true"]')).toBeNull();
  expect(raiz.querySelector('svg')?.getAttribute('width')).toBe('32');
});

test('el error se anuncia y lleva el glifo de peligro en danger-text', () => {
  const raiz = mount({ title: 'No se pudo cargar', tone: 'error', action: boton('Reintentar') });
  expect(raiz.getAttribute('role')).toBe('alert');
  const svg = raiz.querySelector('svg');
  expect(svg?.getAttribute('width')).toBe('32');
  expect(svg?.parentElement?.className).toContain('text-danger-text');
  expect(raiz.textContent).toContain('Reintentar');
});

test('el error respeta el spot que se le pide', () => {
  const raiz = mount({ title: 'No se pudo cargar', tone: 'error', spot: 'errorRetry' });
  expect(raiz.getAttribute('role')).toBe('alert');
  expect((raiz.querySelector('span[aria-hidden="true"]') as HTMLElement).style.maxWidth).toBe(
    '120px',
  );
});

test('compacto: text-sm, py-6, glifo de 20 y sin ilustración', () => {
  const raiz = mount({
    title: 'Sin resultados',
    description: 'Prueba otra búsqueda',
    icon: 'search',
    spot: 'emptySearch',
    illustration: <img alt="" />,
    compact: true,
  });
  expect(raiz.className).toContain('py-6');
  expect(raiz.className).not.toContain('py-8');
  expect(raiz.querySelector('h3')?.className).toBe('text-text text-sm font-semibold');
  expect(raiz.querySelector('p')?.className).toContain('text-sm');
  expect(raiz.querySelector('img')).toBeNull();
  expect(raiz.querySelector('span[aria-hidden="true"]')).toBeNull();
  const svg = raiz.querySelector('svg');
  expect(svg?.getAttribute('width')).toBe('20');
  expect(svg?.parentElement?.className).not.toContain('mb-2');
});

test('compacto sin icon no pinta marca; en error, el glifo de peligro', () => {
  expect(mount({ title: 'x', spot: 'emptyMail', compact: true }).querySelector('svg')).toBeNull();
  const error = mount({ title: 'x', spot: 'errorRetry', compact: true, tone: 'error' });
  expect(error.querySelector('svg')?.getAttribute('width')).toBe('20');
});

test('headingLevel cambia el nivel del título sin cambiar su aspecto', () => {
  const raiz = mount({ title: 'Bandeja vacía', headingLevel: 2 });
  expect(raiz.querySelector('h3')).toBeNull();
  expect(raiz.querySelector('h2')?.className).toBe('sytue-h4 text-text');
});

test('la acción secundaria va junto a la principal', () => {
  const raiz = mount({
    title: 'Sin resultados',
    action: boton('Nueva búsqueda'),
    secondaryAction: boton('Quitar filtro'),
  });
  const botones = [...raiz.querySelectorAll('button')].map((b) => b.textContent);
  expect(botones).toEqual(['Nueva búsqueda', 'Quitar filtro']);
  expect(raiz.querySelector('button')?.parentElement?.className).toBe(
    'mt-2 flex flex-wrap items-center justify-center gap-2',
  );
  // Sola, también se pinta.
  const sola = mount({ title: 'x', secondaryAction: boton('Quitar filtro') });
  expect(sola.querySelector('button')?.textContent).toBe('Quitar filtro');
});
