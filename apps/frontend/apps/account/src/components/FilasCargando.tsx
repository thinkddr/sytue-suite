/*
 * Lo que se ve mientras una lista de Mi cuenta carga: filas fantasma con la MISMA
 * geometría que las de verdad (una línea `text-sm`, otra `text-xs` y la acción a la
 * derecha), para que al llegar los datos la tarjeta no cambie de alto ni salte.
 *
 * Un «Cargando…» suelto ocupaba una línea y la lista, al llegar, empujaba hacia
 * abajo todo lo que venía detrás en la columna.
 */
import type { JSX } from 'preact';
import { Skeleton } from '@sytue/ui';

export function FilasCargando({
  etiqueta,
  filas = 2,
  accion = 'icono',
}: {
  /** Nombre accesible de la región que carga (el `common.loading` de la app). */
  etiqueta: string;
  filas?: number;
  /** `icono`: un aspa de 24 px. `boton`: un botón con texto («Revocar», «Quitar»). */
  accion?: 'icono' | 'boton';
}): JSX.Element {
  return (
    <ul class="flex flex-col gap-3" aria-busy="true" aria-label={etiqueta}>
      {Array.from({ length: filas }, (_, i) => (
        <li key={i} class="flex items-start justify-between gap-3" data-fila-cargando>
          <div class="flex min-w-0 flex-1 flex-col gap-1">
            {/* Las alturas son las de la interlínea de `text-sm` y `text-xs`. */}
            <Skeleton width={i % 2 ? '45%' : '60%'} height="1.25rem" />
            <Skeleton width={i % 2 ? '35%' : '40%'} height="1rem" />
          </div>
          {accion === 'icono' ? (
            <Skeleton width="24px" height="24px" class="shrink-0" />
          ) : (
            <Skeleton width="5rem" height="2rem" class="shrink-0" />
          )}
        </li>
      ))}
    </ul>
  );
}

export default FilasCargando;
