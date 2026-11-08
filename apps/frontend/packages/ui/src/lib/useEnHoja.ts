/**
 * useEnHoja — ¿estamos por debajo de 768, donde un diálogo es la HOJA inferior?
 *
 * Es el `max-md:` de `Modal` preguntado desde JS, para las capas que en el escritorio van
 * ancladas a lo que las abre (la vista rápida de un evento, el selector de emoji) y en el
 * teléfono no caben junto a su ancla: ahí se presentan como la hoja de los diálogos, con
 * `Modal`, en vez de copiar su cromo. Sigue a la ventana: girar el teléfono o estrechar la
 * ventana cambia la presentación sin cerrar nada.
 *
 * `useEstrecha` es lo mismo para el corte `sm:` (640): el teléfono en vertical, donde una
 * vista entera cambia de forma (el mes compacto de Calendar). Las dos son `useConsulta`.
 */
import { useEffect, useState } from 'preact/hooks';

/** El mismo corte que `Modal` (`max-md:`). */
export const CONSULTA_HOJA = '(width < 48rem)';

/**
 * El corte `sm:` de Tailwind (`max-sm:`). En una media query el rem es el del navegador
 * (16), no la raíz de la suite: 40rem son 640 px de ventana, como `ui.CorteSM` en Fyne.
 */
export const CONSULTA_ESTRECHA = '(width < 40rem)';

/**
 * ¿Se cumple la media query? Sigue a la ventana. `inicial` es lo que vale en el primer
 * render: sin darlo se pregunta ya a `matchMedia` (lo que hacía `useEnHoja`); con `false`
 * el primer render coincide con el del servidor y la respuesta llega en el efecto, que es
 * lo que hace falta cuando cambia el MARCADO de una isla `client:load` y no solo una capa
 * que se abre después.
 */
export function useConsulta(consulta: string, inicial?: boolean): boolean {
  const [cumple, setCumple] = useState(() =>
    inicial !== undefined
      ? inicial
      : typeof matchMedia === 'function' && matchMedia(consulta).matches,
  );
  useEffect(() => {
    if (typeof matchMedia !== 'function') return;
    const m = matchMedia(consulta);
    const cambiar = () => setCumple(m.matches);
    cambiar();
    m.addEventListener?.('change', cambiar);
    return () => m.removeEventListener?.('change', cambiar);
  }, [consulta]);
  return cumple;
}

export function useEnHoja(): boolean {
  return useConsulta(CONSULTA_HOJA);
}

/**
 * ¿Ventana por debajo de `sm:`? Arranca en `false` (lo que pinta el servidor, que no sabe
 * el ancho) y lee `matchMedia` en el efecto: así la hidratación no ve dos árboles.
 */
export function useEstrecha(): boolean {
  return useConsulta(CONSULTA_ESTRECHA, false);
}
