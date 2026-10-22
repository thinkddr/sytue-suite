/**
 * Meter — su contrato: el aviso se dice con PALABRAS, no solo con el color.
 *
 * Es la diferencia exacta entre las dos copias que se fusionaron aquí: la de Drive
 * pintaba siempre el mismo gris, así que un buzón al 99 % no avisaba de nada. Un
 * umbral que solo existe como color no llega a quien no lo percibe (WCAG 1.4.1), y
 * un `role="progressbar"` sin nombre tampoco. Ejecutar: `bun test`.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { expect, test } from 'bun:test';
import { render } from 'preact';
import type { VNode } from 'preact';
import { I18nProvider } from '@sytue/i18n/preact';
import { Meter } from './Meter.js';

// Idempotente: otro fichero de test puede haber registrado ya happy-dom.
if (typeof document === 'undefined') GlobalRegistrator.register();

/** Con idioma explícito: el texto del aviso es lo que se está comprobando. */
function mount(children: VNode): HTMLElement {
  const host = document.createElement('div');
  document.body.appendChild(host);
  render(<I18nProvider locale="es">{children}</I18nProvider>, host);
  return host;
}

function barra(host: HTMLElement): HTMLElement {
  return host.querySelector<HTMLElement>('[role="progressbar"]')!;
}

test('holgado: ni una palabra de alarma', () => {
  const host = mount(<Meter value={10} max={100} label="Almacenamiento" />);
  expect(barra(host).getAttribute('aria-valuenow')).toBe('10');
  expect(host.textContent).not.toContain('lleno');
});

test('al 80 % avisa, y al 95 % dice que está lleno', () => {
  expect(mount(<Meter value={85} max={100} label="Almacenamiento" />).textContent).toContain(
    'casi lleno',
  );
  // «lleno» a secas, no «casi lleno»: son estados distintos y el texto los separa.
  const critico = mount(<Meter value={99} max={100} label="Almacenamiento" />).textContent!;
  expect(critico).toContain('lleno');
  expect(critico).not.toContain('casi lleno');
});

test('el porcentaje lo escribe el idioma, no una plantilla', () => {
  // El signo «%» y su separación son del locale, y el espacio que mete `Intl` en
  // español es un espacio estrecho de no separación (U+202F), no el de la barra
  // espaciadora. Justo lo que no sale de concatenar `${pct}%` a mano, que es lo
  // que hacía la copia de Drive.
  const texto = mount(<Meter value={85} max={100} label="X" />).textContent!;
  expect(texto).toMatch(/85\s%/u);
  expect(texto).not.toContain('85%');
});

test('el nombre accesible lleva el uso, el porcentaje y el aviso', () => {
  const host = mount(
    <Meter value={99} max={100} label="Almacenamiento" valueText="14,9 GB de 15 GB" />,
  );
  const nombre = barra(host).getAttribute('aria-label')!;
  expect(nombre).toContain('Almacenamiento');
  expect(nombre).toContain('14,9 GB de 15 GB');
  expect(nombre).toContain('lleno');
});

test('sin cuota configurada no se pinta nada', () => {
  // Un tope de cero es como el backend dice «sin límite»: una barra al 0 % o al
  // 100 % ahí sería mentira, en cualquiera de los dos sentidos.
  expect(mount(<Meter value={5} max={0} label="Almacenamiento" />).innerHTML).toBe('');
});

test('pasarse del tope no desborda la barra', () => {
  expect(
    barra(mount(<Meter value={300} max={100} label="X" />)).getAttribute('aria-valuenow'),
  ).toBe('100');
});
