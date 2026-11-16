/**
 * Tests de Tooltip. Lo que se guarda aquí es que la ayuda LLEGUE a quien no la ve: un
 * tooltip que se pinta pero no se anuncia es decoración.
 *
 * La v2 no envuelve el control: los manejadores van en él mismo (sin fase de captura,
 * que happy-dom no propaga), así que el camino de TECLADO sí se prueba aquí con `focus()`.
 * El panel vive en un portal: se busca en `document`, no dentro del host.
 *
 * Ejecutar: `bun test`.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { expect, test } from 'bun:test';
import { render } from 'preact';
import { IconButton } from './IconButton.js';
import { colocarTooltip, Tooltip } from './Tooltip.js';

if (typeof document === 'undefined') GlobalRegistrator.register();

function montar(ui: preact.JSX.Element): HTMLElement {
  const host = document.createElement('div');
  document.body.appendChild(host);
  render(ui, host);
  return host;
}

/**
 * Espera a que se cumpla la condición, cediendo el turno.
 *
 * Dormir un plazo fijo apenas por encima del retardo del componente (360 ms para
 * uno de 300) falla en cuanto la máquina va cargada —el CI corre todo el
 * workspace en paralelo—: el temporizador se dispara tarde y la aserción llega
 * antes. El síntoma es un rojo intermitente que se lee como una regresión.
 */
// El límite tiene que CABER en el presupuesto de `bun test`, que da 5 s a cada
// test y no se sube en ningún bunfig del repo. Pedir más aquí no espera más: bun
// mata el test antes y el mensaje de abajo no llega a imprimirse nunca, que es
// justo el «timed out after 5000ms» sin causa que se vio en el CI el 2026-08-17.
// Y esperar de verdad tampoco hace falta: `apuntar` abre también por foco, que no
// pasa por el temporizador de 300 ms —es lo que quita el rojo intermitente bajo
// carga—, así que lo que se agota aquí es un fallo real, no una máquina lenta.
async function hasta(cond: () => boolean, que: string, limite = 4000): Promise<void> {
  const fin = Date.now() + limite;
  while (!cond() && Date.now() < fin) await new Promise((r) => setTimeout(r, 10));
  // Agotar el plazo tiene que DECIRLO. Antes se volvía en silencio y fallaba la
  // aserción de después, que informa de un DOM vacío sin distinguir «no ocurrió»
  // de «tardó más de la cuenta»: el rojo se leía como una regresión del componente.
  if (!cond()) throw new Error(`no se cumplió en ${limite} ms: ${que}`);
}

const ayuda = (): HTMLElement | null => document.querySelector('[role="tooltip"]');

test('no añade nodos: el control es el mismo hijo directo, sin envoltorio', () => {
  const host = montar(
    <Tooltip label="Guardar">
      <button type="button">S</button>
    </Tooltip>,
  );
  // El <span> de la v1 es lo que impedía adoptarlo en la cinta: cambiaba la maquetación.
  expect(host.childElementCount).toBe(1);
  expect(host.firstElementChild?.tagName).toBe('BUTTON');
  render(null, host);
});

test('el foco del teclado la abre al instante y la descripción cuelga del CONTROL', async () => {
  const host = montar(
    <Tooltip label="Negrita (Ctrl+B)">
      <button type="button">B</button>
    </Tooltip>,
  );
  const btn = host.querySelector('button')!;
  btn.focus();
  await hasta(() => ayuda() !== null, 'el foco abre la ayuda');
  // Si el atributo se queda en un envoltorio, el lector de pantalla que enfoca el BOTÓN
  // no lee nada: el tooltip se ve y no se anuncia.
  const descrito = btn.getAttribute('aria-describedby');
  expect(descrito).toBeTruthy();
  expect(document.getElementById(descrito!)?.textContent).toContain('Negrita');
  // En un portal: un `overflow` de los antepasados del control ya no la recorta.
  expect(host.contains(ayuda())).toBe(false);
  // Y el DOM del control sigue sin ganar nodos con la ayuda abierta.
  expect(host.childElementCount).toBe(1);
  btn.blur();
  await hasta(() => ayuda() === null, 'perder el foco la cierra');
  expect(btn.hasAttribute('aria-describedby')).toBe(false);
  render(null, host);
});

test('Escape la cierra: es ayuda, no un diálogo que atrape al usuario', async () => {
  const host = montar(
    <Tooltip label="Guardar">
      <button type="button">S</button>
    </Tooltip>,
  );
  host.querySelector('button')!.focus();
  await hasta(() => ayuda() !== null, 'la ayuda se abre');
  // La tecla se REENVÍA mientras se espera: el listener se engancha en un `useEffect`,
  // que corre después de pintar, y bajo carga un único Escape podía caer antes.
  await hasta(() => {
    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }));
    return ayuda() === null;
  }, 'Escape la cierra');
  // El control sigue ahí: Escape descarta la ayuda, no la acción.
  expect(host.querySelector('button')).not.toBeNull();
  host.querySelector('button')!.blur();
  render(null, host);
});

test('sin puntero ni foco no hay nada que anunciar', () => {
  const host = montar(
    <Tooltip label="Guardar">
      <button type="button">S</button>
    </Tooltip>,
  );
  expect(ayuda()).toBeNull();
  expect(host.querySelector('button')!.hasAttribute('aria-describedby')).toBe(false);
  render(null, host);
});

test('el retardo existe y es de GRUPO: la siguiente ayuda abre sin esperar', async () => {
  const host = montar(
    <div>
      <Tooltip label="Uno">
        <button type="button">1</button>
      </Tooltip>
      <Tooltip label="Dos">
        <button type="button">2</button>
      </Tooltip>
    </div>,
  );
  const [uno, dos] = Array.from(host.querySelectorAll('button'));
  uno!.dispatchEvent(new MouseEvent('mouseenter', { bubbles: false }));
  // Rozar el control no basta: todavía no hay nada.
  expect(ayuda()).toBeNull();
  await hasta(() => ayuda()?.textContent === 'Uno', 'vence el retardo');
  uno!.dispatchEvent(new MouseEvent('mouseleave', { bubbles: false }));
  await hasta(() => ayuda() === null, 'salir la cierra');
  dos!.dispatchEvent(new MouseEvent('mouseenter', { bubbles: false }));
  // Recién cerrada la anterior, la vecina abre en el mismo turno, sin los 300 ms.
  await hasta(() => ayuda()?.textContent === 'Dos', 'abre sin retardo', 50);
  dos!.dispatchEvent(new MouseEvent('mouseleave', { bubbles: false }));
  render(null, host);
});

test('conserva los manejadores propios del control', async () => {
  let enfocado = 0;
  const host = montar(
    <Tooltip label="Guardar">
      <button type="button" onFocus={() => enfocado++}>
        S
      </button>
    </Tooltip>,
  );
  host.querySelector('button')!.focus();
  await hasta(() => ayuda() !== null, 'la ayuda se abre');
  expect(enfocado).toBe(1);
  host.querySelector('button')!.blur();
  render(null, host);
});

test('queda dentro de una ventana de 320 px y se voltea si arriba no cabe', () => {
  const antes = window.innerWidth;
  Object.defineProperty(window, 'innerWidth', { value: 320, configurable: true });
  Object.defineProperty(document.documentElement, 'clientWidth', {
    value: 320,
    configurable: true,
  });
  try {
    // Un botón pegado al borde derecho y arriba del todo, como la campana de la barra.
    const ancla = new DOMRect(300, 4, 24, 24);
    const p = colocarTooltip(ancla, { width: 200, height: 24 }, 'top');
    expect(p.left).toBeGreaterThanOrEqual(8);
    expect(p.left + 200).toBeLessThanOrEqual(312);
    expect(p.lado).toBe('bottom');
    expect(p.top).toBeGreaterThanOrEqual(ancla.bottom);
    // La flecha se corre hacia el control, hasta la esquina del panel.
    expect(p.flecha).toBe(194);
    // Pegado al borde izquierdo, igual.
    const izq = colocarTooltip(new DOMRect(0, 200, 24, 24), { width: 200, height: 24 }, 'top');
    expect(izq.left).toBe(8);
    expect(izq.lado).toBe('top');
    // El carril plegado: a la derecha, y a la izquierda si no cabe.
    const der = colocarTooltip(new DOMRect(290, 100, 24, 24), { width: 120, height: 24 }, 'right');
    expect(der.lado).toBe('left');
    expect(der.left + 120).toBeLessThanOrEqual(312);
  } finally {
    Object.defineProperty(window, 'innerWidth', { value: antes, configurable: true });
    delete (document.documentElement as unknown as Record<string, unknown>).clientWidth;
  }
});

test('IconButton ya no emite `title`: su ayuda es visible con teclado', async () => {
  const host = montar(
    <IconButton label="Cerrar el panel">
      <svg />
    </IconButton>,
  );
  const btn = host.querySelector('button')!;
  expect(btn.hasAttribute('title')).toBe(false);
  expect(btn.getAttribute('aria-label')).toBe('Cerrar el panel');
  expect(host.childElementCount).toBe(1);
  btn.focus();
  await hasta(() => ayuda()?.textContent === 'Cerrar el panel', 'el foco muestra la ayuda');
  // La ayuda repite el nombre accesible: enlazarla haría que el lector lo leyera dos veces.
  expect(btn.hasAttribute('aria-describedby')).toBe(false);
  btn.blur();
  render(null, host);
});

test('IconButton con showTitle={false} no muestra ayuda', async () => {
  const host = montar(
    <IconButton label="Cerrar" showTitle={false}>
      <svg />
    </IconButton>,
  );
  host.querySelector('button')!.focus();
  await new Promise((r) => setTimeout(r, 20));
  expect(ayuda()).toBeNull();
  host.querySelector('button')!.blur();
  render(null, host);
});
