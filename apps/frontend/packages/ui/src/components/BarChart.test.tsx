/**
 * Tests de BarChart — su contrato: un día sin actividad se ve igualmente (una
 * barra de alto 0 sería indistinguible de «ese día no existe»), las cifras
 * llegan a un lector de pantalla, y una serie vacía no pinta un lienzo en blanco.
 * Ejecutar: `bun test`.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { expect, test } from 'bun:test';
import { render } from 'preact';
import type { VNode } from 'preact';
import { BarChart } from './BarChart.js';

// Idempotente: otro fichero de test puede haber registrado ya happy-dom.
if (typeof document === 'undefined') GlobalRegistrator.register();

function mount(children: VNode): HTMLElement {
  const host = document.createElement('div');
  document.body.appendChild(host);
  render(children, host);
  return host;
}

function bars(host: HTMLElement): SVGRectElement[] {
  return Array.from(host.querySelectorAll<SVGRectElement>('rect'));
}

test('un día a cero se sigue viendo', () => {
  // Sin suelo, el 0 daría alto 0 y el usuario no podría distinguir «ese día no
  // hubo visitas» de «ese día no está en la serie», que es información distinta.
  const host = mount(
    <BarChart
      points={[
        { label: 'lun', value: 0 },
        { label: 'mar', value: 10 },
      ]}
      seriesLabel="visitas"
    />,
  );
  const [cero, diez] = bars(host);
  expect(Number(cero!.getAttribute('height'))).toBeGreaterThan(0);
  expect(Number(diez!.getAttribute('height'))).toBeGreaterThan(
    Number(cero!.getAttribute('height')),
  );
});

test('todo a cero no hace desaparecer el gráfico', () => {
  // El máximo se acota a 1: si fuese 0, dividir por él daría NaN y ninguna barra
  // se pintaría justo cuando el dato («no hay tráfico») importa.
  const host = mount(
    <BarChart
      points={[
        { label: 'a', value: 0 },
        { label: 'b', value: 0 },
      ]}
      seriesLabel="visitas"
    />,
  );
  expect(bars(host)).toHaveLength(2);
  for (const b of bars(host)) expect(Number(b.getAttribute('height'))).toBeGreaterThan(0);
});

test('las alturas son proporcionales al máximo', () => {
  const host = mount(
    <BarChart
      points={[
        { label: 'a', value: 50 },
        { label: 'b', value: 100 },
      ]}
      seriesLabel="visitas"
      height={100}
    />,
  );
  const [mitad, tope] = bars(host);
  expect(Number(tope!.getAttribute('height'))).toBe(100);
  expect(Number(mitad!.getAttribute('height'))).toBe(50);
});

test('un lector de pantalla lee las cifras, no «gráfico»', () => {
  // Un <svg role="img"> solo puede anunciar su etiqueta; la tabla oculta es lo
  // que hace que los datos existan para quien no ve el dibujo.
  const host = mount(
    <BarChart
      points={[{ label: '3 ago', value: 42 }]}
      seriesLabel="páginas vistas"
      tableHeaders={['Día', 'Páginas vistas']}
    />,
  );
  const table = host.querySelector('table');
  expect(table?.className).toContain('sr-only');
  expect(table?.textContent).toContain('3 ago');
  expect(table?.textContent).toContain('42');
  expect(host.querySelector('svg')?.getAttribute('role')).toBe('img');
});

test('una serie vacía dice que está vacía en vez de pintar un lienzo en blanco', () => {
  const host = mount(<BarChart points={[]} seriesLabel="visitas" emptyLabel="Sin datos" />);
  expect(host.querySelector('svg')).toBeNull();
  expect(host.textContent).toContain('Sin datos');
});
