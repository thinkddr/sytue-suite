/**
 * useFitInViewport — empuja un panel flotante lo justo para que quepa entero en
 * la ventana. Los visores propios pueden acotar también el eje vertical.
 *
 * Por qué hace falta: un panel anclado con `right-0` al borde derecho de su
 * DISPARADOR empieza en `disparador.right − ancho`. Si el disparador no está en
 * el borde de la pantalla —la campana de la barra superior no lo está— esa
 * coordenada es NEGATIVA en un teléfono estrecho, y un desbordamiento a la
 * izquierda no genera scroll: se RECORTA. Lo que queda fuera no se alcanza ni
 * desplazando. Medido en producción a 320 px: el centro de notificaciones
 * arrancaba en −86 px de un panel de 330 (el título se leía «ones»).
 *
 * El `max-w-[calc(100vw-16px)]` que llevan los paneles es condición NECESARIA
 * —sin él, empujar por la izquierda sacaría el borde derecho— pero no
 * suficiente: con max-w el panel seguía en −60 px.
 *
 * Dos trampas comprobadas en la propia página:
 *   – Se escribe en la propiedad independiente `translate`, NUNCA en
 *     `transform`: el keyframe `sytue-pop-in` (styles.css) anima `transform`
 *     con `both`, así que su estado final `transform: none` pisa cualquier
 *     `transform` en línea y el panel no se movía ni un píxel.
 *   – `margin-left` tampoco sirve: en una caja absoluta con `right:0` y
 *     `left:auto`, el margen lo absorbe el `left` calculado.
 */
import { useLayoutEffect } from 'preact/hooks';
import { limitesViewport, observarViewport } from './viewport.js';

/** Aire mínimo entre el panel y el borde de la ventana (px). */
const MARGEN = 8;

export function useFitInViewport(
  ref: { current: HTMLElement | null },
  /**
   * Falsy mientras el panel no está montado. Cualquier otro valor lo activa, y
   * al cambiar vuelve a medir: los llamantes que voltean el anclaje (Popover)
   * pasan aquí su colocación para que el empujón no se calcule sobre la vieja.
   */
  abierto: unknown,
  { vertical = false }: { vertical?: boolean } = {},
): void {
  // El ancla puede moverse al renderizar sin cambiar el tamaño del panel. Se
  // mide tras cada commit; el cleanup retira siempre los observadores anteriores.
  useLayoutEffect(() => {
    const el = ref.current;
    if (!abierto || !el) return;
    const anterior = {
      translate: el.style.translate,
      maxWidth: el.style.maxWidth,
      maxHeight: el.style.maxHeight,
      overflowY: el.style.overflowY,
    };
    const medir = (): void => {
      const v = limitesViewport(MARGEN);
      el.style.maxWidth = `${v.width}px`;
      if (vertical) {
        // La clase puede imponer un límite menor (p. ej. 60dvh en la cinta).
        // Se vuelve a medir sin el tope de la ronda anterior para poder crecer
        // otra vez cuando desaparezca el teclado.
        el.style.maxHeight = anterior.maxHeight;
        const deClase = Number.parseFloat(getComputedStyle(el).maxHeight);
        el.style.maxHeight = `${Number.isFinite(deClase) ? Math.min(deClase, v.height) : v.height}px`;
        el.style.overflowY = 'auto';
      }
      el.style.translate = ''; // medir desde el anclaje, sin acumular el empujón anterior
      const r = el.getBoundingClientRect();
      // Una caja oculta o sin motor de layout no se empuja a ciegas.
      if (!r.width) return;
      const dx = r.left < v.left ? v.left - r.left : r.right > v.right ? v.right - r.right : 0;
      const dy = vertical
        ? r.top < v.top
          ? v.top - r.top
          : r.bottom > v.bottom
            ? v.bottom - r.bottom
            : 0
        : 0;
      if (dy) el.style.translate = `${Math.round(dx)}px ${Math.round(dy)}px`;
      else if (dx) el.style.translate = `${Math.round(dx)}px`;
    };
    medir();
    const dejarDeObservar = observarViewport(medir);
    const observer = new ResizeObserver(medir);
    observer.observe(el);
    return () => {
      dejarDeObservar();
      observer.disconnect();
      el.style.translate = anterior.translate;
      el.style.maxWidth = anterior.maxWidth;
      if (vertical) {
        el.style.maxHeight = anterior.maxHeight;
        el.style.overflowY = anterior.overflowY;
      }
    };
  });
}
