/**
 * useEnHoja — ¿estamos por debajo de 768, donde un diálogo es la HOJA inferior?
 *
 * Es el `max-md:` de `Modal` preguntado desde JS, para las capas que en el escritorio van
 * ancladas a lo que las abre (la vista rápida de un evento, el selector de emoji) y en el
 * teléfono no caben junto a su ancla: ahí se presentan como la hoja de los diálogos, con
 * `Modal`, en vez de copiar su cromo. Sigue a la ventana: girar el teléfono o estrechar la
 * ventana cambia la presentación sin cerrar nada.
 */
import { useEffect, useState } from 'preact/hooks';

/** El mismo corte que `Modal` (`max-md:`). */
export const CONSULTA_HOJA = '(width < 48rem)';

export function useEnHoja(): boolean {
  const [hoja, setHoja] = useState(
    () => typeof matchMedia === 'function' && matchMedia(CONSULTA_HOJA).matches,
  );
  useEffect(() => {
    if (typeof matchMedia !== 'function') return;
    const m = matchMedia(CONSULTA_HOJA);
    const cambiar = () => setHoja(m.matches);
    cambiar();
    m.addEventListener?.('change', cambiar);
    return () => m.removeEventListener?.('change', cambiar);
  }, []);
  return hoja;
}
