/**
 * Clic derecho → el menú «⋯» del elemento.
 *
 * En un explorador de ficheros el clic derecho es el gesto con el que se piden las
 * acciones, y el menú del navegador («Guardar enlace como…», «Inspeccionar») no sirve
 * de nada sobre una fila que no es un enlace. En vez de una variante de `Dropdown`
 * anclada al puntero, se PULSA el disparador del «⋯» de esa fila: el menú es
 * exactamente el mismo, cuelga de su botón, recibe el foco y Escape lo devuelve al
 * disparador, todo ello ya resuelto en `@sytue/ui`.
 *
 * En el móvil, mantener pulsado también lanza `contextmenu` (Android), así que abre el
 * mismo menú.
 */

/** Atributo que marca el botón «⋯» de una fila. */
export const DISPARADOR_MENU = 'data-menu-trigger';

/**
 * Manejador de `onContextMenu` para una fila o tesela. Sin «⋯» dentro (sin red, una
 * vista sin acciones) no hace nada y deja el menú del navegador: tragarse el gesto sin
 * ofrecer nada a cambio sería peor.
 */
export function abrirMenuDeFila(e: MouseEvent): void {
  const fila = e.currentTarget;
  if (!(fila instanceof Element)) return;
  // El asa de mover arrastra con el dedo: mantenerla pulsada en Android lanza
  // `contextmenu` a mitad del gesto, y abrir el menú ahí taparía el destino.
  if (e.target instanceof Element && e.target.closest('[data-drive-move]')) return;
  const disparador = fila.querySelector<HTMLElement>(`[${DISPARADOR_MENU}]`);
  if (!disparador || (disparador as HTMLButtonElement).disabled) return;
  e.preventDefault();
  // El disparador alterna con `setOpen((v) => !v)`. Un clic derecho en la fila fuera del
  // «⋯» ya ha cerrado el menú en su `mousedown` (es un clic fuera del panel), así que
  // aquí llega cerrado y pulsarlo lo abre. Si sigue ABIERTO es que el clic derecho cayó
  // en el propio «⋯», que no cuenta como fuera: pulsarlo lo cerraría, y se deja estar.
  if (disparador.getAttribute('aria-expanded') === 'true') return;
  disparador.click();
}
