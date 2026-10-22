/**
 * Tests de Tooltip. Lo que se guarda aquí es que la ayuda LLEGUE a quien no la ve: un
 * tooltip que se pinta pero no se anuncia es decoración.
 *
 * Se abre por PUNTERO y no por foco a propósito: happy-dom no propaga `focus` en fase de
 * captura, que es como lo escucha el componente, así que un test de teclado aquí daría un
 * falso fallo. El camino de foco se comprueba en los e2e, con navegador de verdad.
 *
 * Ejecutar: `bun test`.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { expect, test } from 'bun:test';
import { render } from 'preact';
import { Tooltip } from './Tooltip.js';

if (typeof document === 'undefined') GlobalRegistrator.register();

function montar(ui: preact.JSX.Element): HTMLElement {
  const host = document.createElement('div');
  document.body.appendChild(host);
  render(ui, host);
  return host;
}

/** Entra el puntero en el envoltorio y espera a que venza el retardo. */
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

/**
 * Abre la ayuda por los DOS caminos que la abren.
 *
 * El foco la abre al instante y el puntero espera 300 ms. Depender solo del
 * temporizador dejaba este test rojo cuando el CI corre todo el workspace a la
 * vez —agotaba los 5 s enteros esperando a que el plazo venciera—, y no es
 * lentitud del componente: sin ese plazo de por medio no hay nada que esperar.
 * Se despachan los dos porque cada uno cubre al otro: si happy-dom propaga el
 * `focus` en captura, el tooltip está en el DOM sin temporizador ninguno; si no
 * lo propaga —que es por lo que este fichero abre por puntero—, queda el camino
 * de siempre. El camino de teclado con navegador de verdad se cubre en los e2e.
 */
async function apuntar(host: HTMLElement): Promise<void> {
  const envoltorio = host.querySelector('span')!;
  envoltorio.dispatchEvent(new FocusEvent('focus'));
  envoltorio.dispatchEvent(new MouseEvent('mouseenter', { bubbles: false }));
  await hasta(() => host.querySelector('[role="tooltip"]') !== null, 'la ayuda se abre');
}

test('la descripción cuelga del CONTROL, no del envoltorio', async () => {
  const host = montar(
    <Tooltip label="Negrita (Ctrl+B)">
      <button type="button">B</button>
    </Tooltip>,
  );
  await apuntar(host);

  const btn = host.querySelector('button')!;
  // Si el atributo se queda en el <span> que envuelve, el lector de pantalla que enfoca
  // el BOTÓN no lee nada: el tooltip se ve y no se anuncia.
  const descrito = btn.getAttribute('aria-describedby');
  expect(descrito).toBeTruthy();
  // Y el id debe existir de verdad: apuntar a la nada no anuncia nada, y no hay forma
  // de notarlo mirando la pantalla.
  expect(host.querySelector(`#${descrito}`)?.textContent).toContain('Negrita');
  render(null, host);
});

test('Escape lo cierra: es ayuda, no un diálogo que atrape al usuario', async () => {
  const host = montar(
    <Tooltip label="Guardar">
      <button type="button">S</button>
    </Tooltip>,
  );
  await apuntar(host);
  expect(host.querySelector('[role="tooltip"]')).not.toBeNull();

  // La tecla se REENVÍA mientras se espera, no se despacha una sola vez.
  //
  // El componente engancha su listener de Escape en un `useEffect`, que Preact
  // ejecuta DESPUÉS de pintar. Entre que la ayuda está en el DOM —que es lo que
  // ve `apuntar`— y que el listener existe hay una ventana, y bajo carga (el CI
  // corre el workspace entero a la vez) esa ventana se abre lo bastante para que
  // el único Escape del test caiga antes: nadie lo escucha, la ayuda no se
  // cierra y el test agota sus 4 s. Salía como un rojo intermitente del CI
  // —2026-09-05, ci-frontend/quality— que aquí nunca se reproducía.
  //
  // Reenviarla no debilita nada: si el componente no cerrara con Escape, esto
  // seguiría agotando el plazo igual.
  await hasta(() => {
    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }));
    return host.querySelector('[role="tooltip"]') === null;
  }, 'Escape la cierra');
  expect(host.querySelector('[role="tooltip"]')).toBeNull();
  // El control sigue ahí: Escape descarta la ayuda, no la acción.
  expect(host.querySelector('button')).not.toBeNull();
  render(null, host);
});

test('sin puntero ni foco no hay nada que anunciar', () => {
  const host = montar(
    <Tooltip label="Guardar">
      <button type="button">S</button>
    </Tooltip>,
  );
  expect(host.querySelector('[role="tooltip"]')).toBeNull();
  expect(host.querySelector('button')!.hasAttribute('aria-describedby')).toBe(false);
  render(null, host);
});

test('el retardo existe: el tooltip no salta con solo rozar el control', () => {
  const host = montar(
    <Tooltip label="Guardar">
      <button type="button">S</button>
    </Tooltip>,
  );
  host.querySelector('span')!.dispatchEvent(new MouseEvent('mouseenter', { bubbles: false }));
  // Inmediatamente después de entrar el puntero todavía no hay nada.
  expect(host.querySelector('[role="tooltip"]')).toBeNull();
  render(null, host);
});
