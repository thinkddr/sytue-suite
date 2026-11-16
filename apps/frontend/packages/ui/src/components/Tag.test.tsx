/**
 * Tests de Tag — el contrato que perdieron las tres copias de la píldora: el aspa con
 * nombre propio, la diana de 24 px que exige WCAG 2.5.8 y el chip que no crece por
 * llevarla.
 * Ejecutar: `bun test src/components/Tag.test.tsx`.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { expect, test } from 'bun:test';
import { render } from 'preact';
import { Tag } from './Tag.js';

// Idempotente: otro fichero de test puede haber registrado ya happy-dom.
if (typeof document === 'undefined') GlobalRegistrator.register();

function montar(ui: preact.JSX.Element): HTMLElement {
  const host = document.createElement('div');
  document.body.appendChild(host);
  render(ui, host);
  return host;
}

test('el aspa es un botón que nombra la etiqueta que quita', () => {
  let quitadas = 0;
  const host = montar(
    <Tag onRemove={() => quitadas++} removeLabel="Quitar la etiqueta Trabajo">
      Trabajo
    </Tag>,
  );
  const aspa = host.querySelector('button')!;
  expect(aspa).not.toBeNull();
  expect(aspa.getAttribute('type')).toBe('button');
  expect(aspa.getAttribute('aria-label')).toBe('Quitar la etiqueta Trabajo');
  aspa.click();
  expect(quitadas).toBe(1);
  render(null, host);
});

test('la diana del aspa no baja de 24 px y sube a 44 con el dedo', () => {
  const host = montar(
    <Tag onRemove={() => {}} removeLabel="Quitar">
      Trabajo
    </Tag>,
  );
  const clases = host.querySelector('button')!.className.split(/\s+/);
  // En px y no en rem: `h-6` son 1.5rem, y con 1 rem en 14 px eran 21, por debajo del
  // suelo de WCAG.
  expect(clases).toContain('min-h-[24px]');
  expect(clases).toContain('min-w-[24px]');
  expect(clases).toContain('pointer-coarse:min-h-[max(44px,2.75rem)]');
  // El margen negativo devuelve el alto del chip: la diana sobresale, la fila no crece.
  expect(clases).toContain('-my-1');
  // Y el alto del chip es el de las copias que sustituye.
  const chip = host.firstElementChild!.className.split(/\s+/);
  expect(chip).toContain('py-0.5');
  expect(chip).toContain('text-sm');
  render(null, host);
});

test('sin onRemove no hay aspa ni hueco para ella', () => {
  const host = montar(<Tag>Trabajo</Tag>);
  expect(host.querySelector('button')).toBeNull();
  const chip = host.firstElementChild!.className.split(/\s+/);
  expect(chip).toContain('px-2.5');
  expect(chip).not.toContain('pe-1');
  render(null, host);
});

test('el aspa se puede apagar mientras se guarda el cambio anterior', () => {
  const host = montar(
    <Tag onRemove={() => {}} removeLabel="Quitar" removeDisabled>
      Trabajo
    </Tag>,
  );
  expect((host.querySelector('button') as HTMLButtonElement).disabled).toBe(true);
  render(null, host);
});

test('el texto se trunca con tope y el nombre entero queda en el title', () => {
  const nombre = 'Proveedores del proyecto de rehabilitación';
  const host = montar(<Tag>{nombre}</Tag>);
  const texto = host.querySelector('[title]')!;
  expect(texto.getAttribute('title')).toBe(nombre);
  expect(texto.className).toContain('truncate');
  expect(texto.className).toContain('max-w-48');
  render(null, host);
});

test('el punto de color es decorativo y solo sale si se pide', () => {
  const sin = montar(<Tag>Trabajo</Tag>);
  expect(sin.querySelector('[aria-hidden="true"]')).toBeNull();
  render(null, sin);

  const con = montar(<Tag dot="var(--sytue-success)">Trabajo</Tag>);
  const punto = con.querySelector('[aria-hidden="true"]') as HTMLElement;
  expect(punto).not.toBeNull();
  expect(punto.style.background).toContain('--sytue-success');
  render(null, con);
});

test('el tono apagado cambia la tinta del texto, no la del chip', () => {
  const host = montar(<Tag tone="muted">Trabajo</Tag>);
  const chip = host.firstElementChild!.className.split(/\s+/);
  expect(chip).toContain('text-text-muted');
  expect(chip).toContain('bg-bg-subtle');
  render(null, host);
});

test('lo que acompaña al texto va entre el texto y el aspa, fuera del tramo que se trunca', () => {
  const host = montar(
    <Tag trailing={<em>Acepta</em>} onRemove={() => {}} removeLabel="Quitar a ana@sytue.eu">
      ana@sytue.eu
    </Tag>,
  );
  const chip = host.firstElementChild!;
  const hijos = [...chip.children];
  const texto = hijos.findIndex((h) => h.getAttribute('title') === 'ana@sytue.eu');
  const anexo = hijos.findIndex((h) => h.querySelector('em') !== null);
  const aspa = hijos.findIndex((h) => h.tagName === 'BUTTON');
  expect(texto).toBeGreaterThanOrEqual(0);
  expect(anexo).toBe(texto + 1);
  expect(aspa).toBe(anexo + 1);
  // Un correo largo se recorta él: la respuesta no puede encoger.
  expect(hijos[anexo]!.className).toContain('shrink-0');
  expect(hijos[texto]!.querySelector('em')).toBeNull();
  render(null, host);
});
