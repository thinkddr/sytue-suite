/**
 * Tests de Wizard — el contrato del asistente guiado: el avance respeta la
 * validación del paso, un paso con acción propia NO avanza solo, y el paso
 * terminal (`done`) no deja volver atrás ni muestra la barra de acciones.
 * Ejecutar: `bun test`.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { expect, test } from 'bun:test';
import { render } from 'preact';
import { I18nProvider } from '@sytue/i18n/preact';
import { Wizard } from './Wizard.js';
import type { WizardStep } from './Wizard.js';

// Idempotente: otro fichero de test puede haber registrado ya happy-dom.
if (typeof document === 'undefined') GlobalRegistrator.register();

const steps = (over: Partial<WizardStep>[] = []): WizardStep[] =>
  [
    { id: 'a', title: 'Elegir', content: <p>uno</p> },
    { id: 'b', title: 'Configurar', content: <p>dos</p> },
    { id: 'c', title: 'Listo', content: <p>tres</p>, done: true },
  ].map((s, i) => ({ ...s, ...over[i] })) as WizardStep[];

function button(host: HTMLElement, label: string): HTMLButtonElement | undefined {
  return Array.from(host.querySelectorAll('button')).find((b) => b.textContent?.includes(label));
}

test('«Continuar» avanza al paso siguiente', () => {
  const host = document.createElement('div');
  document.body.appendChild(host);
  let current = 0;
  render(
    <I18nProvider locale="es">
      <Wizard steps={steps()} current={0} onCurrentChange={(i) => (current = i)} />
    </I18nProvider>,
    host,
  );

  button(host, 'Continuar')?.click();
  expect(current).toBe(1);
  render(null, host);
});

// El cromo del asistente (atrás / continuar / cancelar) estuvo en español FIJO, así
// que una consola en inglés mezclaba idiomas en el pie de todos sus asistentes.
// Nadie lo detectaba porque los tests montaban el Wizard sin proveedor y buscaban
// el botón por su literal español, que era justo el defecto.
test('el cromo del asistente habla el idioma de la interfaz', () => {
  const host = document.createElement('div');
  document.body.appendChild(host);
  render(
    <I18nProvider locale="en">
      <Wizard steps={steps()} current={1} onCurrentChange={() => undefined} />
    </I18nProvider>,
    host,
  );

  expect(button(host, 'Continue')).toBeDefined();
  expect(button(host, 'Back')).toBeDefined();
  expect(button(host, 'Continuar')).toBeUndefined();
  render(null, host);
});

test('canContinue=false bloquea el avance', () => {
  const host = document.createElement('div');
  document.body.appendChild(host);
  let current = 0;
  render(
    <I18nProvider locale="es">
      <Wizard
        steps={steps([{ canContinue: false }])}
        current={0}
        onCurrentChange={(i) => (current = i)}
      />
    </I18nProvider>,
    host,
  );

  expect(button(host, 'Continuar')?.disabled).toBe(true);
  expect(current).toBe(0);
  render(null, host);
});

test('un paso con onNext ejecuta su acción en vez de avanzar', () => {
  const host = document.createElement('div');
  document.body.appendChild(host);
  let current = 0;
  let ran = 0;
  render(
    <Wizard
      steps={steps([{ nextLabel: 'Activar', onNext: () => void ran++ }])}
      current={0}
      onCurrentChange={(i) => (current = i)}
    />,
    host,
  );

  button(host, 'Activar')?.click();
  expect(ran).toBe(1);
  expect(current).toBe(0);
  render(null, host);
});

test('el paso terminal no tiene barra de acciones ni vuelta atrás', () => {
  const host = document.createElement('div');
  document.body.appendChild(host);
  render(
    <Wizard steps={steps()} current={2} onCurrentChange={() => {}} onCancel={() => {}} />,
    host,
  );

  expect(button(host, 'Atrás')).toBeUndefined();
  expect(button(host, 'Cancelar')).toBeUndefined();
  // El progreso tampoco es navegable (la acción ya se aplicó).
  expect(host.querySelectorAll('ol button').length).toBe(0);
  render(null, host);
});

test('el progreso deja volver a un paso ya completado', () => {
  const host = document.createElement('div');
  document.body.appendChild(host);
  let current = 1;
  render(<Wizard steps={steps()} current={1} onCurrentChange={(i) => (current = i)} />, host);

  const back = host.querySelector<HTMLButtonElement>('ol button');
  expect(back).not.toBeNull();
  back?.click();
  expect(current).toBe(0);
  render(null, host);
});
