/**
 * PaletaDeComandos: el teclado (flechas que dan la vuelta, Intro, Escape, Tab que no se
 * va), el foco que entra en el campo y vuelve al cerrar, y la capa de los diálogos.
 * Ejecutar: `bun test`.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { expect, test } from 'bun:test';
import { render } from 'preact';
import { PaletaDeComandos, type OpcionDePaleta } from './PaletaDeComandos.js';

if (typeof document === 'undefined') GlobalRegistrator.register();

const OPCIONES: OpcionDePaleta[] = [
  { id: 'a', rotulo: 'Resumen', grupo: 'Navegación', icono: 'grid' },
  { id: 'b', rotulo: 'Despliegues', grupo: 'Navegación' },
  { id: 'c', rotulo: 'Crear proyecto', grupo: 'Acciones', pista: 'nuevo' },
];

function montar(extra: {
  onElegir?: (o: OpcionDePaleta) => void;
  onClose?: () => void;
  opciones?: OpcionDePaleta[];
}) {
  const host = document.body.appendChild(document.createElement('div'));
  render(
    <PaletaDeComandos
      open
      onClose={extra.onClose ?? (() => {})}
      etiqueta="Paleta de comandos"
      rotuloCampo="Buscar comandos"
      placeholder="Buscar o saltar a…"
      rotuloResultados="Resultados"
      sinResultados={(q) => `Sin resultados para «${q}».`}
      consulta=""
      onConsulta={() => {}}
      opciones={extra.opciones ?? OPCIONES}
      onElegir={extra.onElegir ?? (() => {})}
      pie={<span>atajos</span>}
    />,
    host,
  );
  return () => {
    render(null, host);
    host.remove();
    document.body.innerHTML = '';
  };
}

/** Pulsa la tecla en lo enfocado y espera a que Preact repinte. */
const tecla = async (key: string): Promise<void> => {
  document.activeElement!.dispatchEvent(
    new KeyboardEvent('keydown', { key, bubbles: true, cancelable: true }),
  );
  await new Promise((r) => setTimeout(r, 20));
};

test('el foco entra en el campo, las flechas dan la vuelta e Intro elige la activa', async () => {
  let elegida = '';
  const quitar = montar({ onElegir: (o) => (elegida = o.id) });
  try {
    const campo = document.activeElement as HTMLInputElement;
    expect(campo.getAttribute('role')).toBe('combobox');
    const activa = () =>
      document.getElementById(campo.getAttribute('aria-activedescendant')!)?.textContent;
    expect(activa()).toContain('Resumen');
    await tecla('ArrowUp'); // desde la primera, a la última
    expect(activa()).toContain('Crear proyecto');
    await tecla('ArrowDown'); // y de la última, a la primera
    expect(activa()).toContain('Resumen');
    await tecla('ArrowDown');
    await tecla('Enter');
    expect(elegida).toBe('b');
    // Tab no sale de la paleta: el campo es lo único enfocable.
    const tab = new KeyboardEvent('keydown', { key: 'Tab', bubbles: true, cancelable: true });
    campo.dispatchEvent(tab);
    expect(tab.defaultPrevented).toBe(true);
  } finally {
    quitar();
  }
});

test('Escape y el velo cierran; sin coincidencias se dice', async () => {
  let cierres = 0;
  const quitar = montar({ onClose: () => cierres++, opciones: [] });
  try {
    await tecla('Escape');
    expect(cierres).toBe(1);
    (document.querySelector('[aria-hidden="true"].absolute') as HTMLElement).click();
    await new Promise((r) => setTimeout(r, 20));
    expect(cierres).toBe(2);
    expect(document.querySelector('[role="listbox"]')!.textContent).toContain('Sin resultados');
  } finally {
    quitar();
  }
});

test('es la capa de los diálogos y en el teléfono una hoja con «Cancelar» y sin atajos', () => {
  const quitar = montar({});
  try {
    const panel = document.querySelector('[role="dialog"]') as HTMLElement;
    // Las clases responsivas se resuelven por CSS (medido con Playwright a 1440 y 390):
    // este test fija el contrato que impide volver al velo y la sombra propios.
    expect(panel.getAttribute('aria-modal')).toBe('true');
    expect(panel.className).toContain('md:rounded-card');
    expect(panel.className).toContain('md:shadow-[var(--shadow-xl)]');
    expect(panel.className).toContain('max-md:rounded-t-card');
    expect(panel.parentElement!.querySelector('.bg-\\[var\\(--sytue-overlay\\)\\]')).not.toBeNull();
    const cancelar = [...panel.querySelectorAll('button')].find((b) =>
      b.className.includes('md:hidden'),
    );
    expect(cancelar?.textContent).toBeTruthy();
    expect(cancelar).toBeDefined();
    expect(panel.lastElementChild!.className).toContain('max-md:hidden'); // los atajos
  } finally {
    quitar();
  }
});
