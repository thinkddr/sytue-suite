/**
 * Lo que una app hace elemento a elemento porque el servicio no lo tiene en lote:
 * deshacer un envío a la papelera de varios elementos, vaciarla.
 *
 * Vive en `@sytue/ui` y no en cada app porque los dos fallos que vigila no se ven y
 * se repiten igual en Drive y en Fotos: mandar las cien llamadas a la vez y dar por
 * hecho lo que salió mal.
 */

/** Cuántas llamadas van a la vez: las justas para no esperar en fila ni saturar al BFF. */
export const EN_PARALELO = 4;

/**
 * Hace `hacer` con cada elemento, de `enParalelo` en `enParalelo`, y separa lo que salió
 * de lo que no. Nunca rechaza: un fallo no corta el resto, que es lo que se espera de
 * «deshacer» o de «vaciar» (lo que se pudo, se hizo; lo que no, se dice).
 */
export async function enTandas<T>(
  items: readonly T[],
  hacer: (x: T) => Promise<unknown>,
  enParalelo = EN_PARALELO,
): Promise<{ hechos: T[]; fallidos: T[] }> {
  const hechos: T[] = [];
  const fallidos: T[] = [];
  let siguiente = 0;
  const trabajador = async () => {
    while (siguiente < items.length) {
      const x = items[siguiente++] as T;
      try {
        await hacer(x);
        hechos.push(x);
      } catch {
        fallidos.push(x);
      }
    }
  };
  const n = Math.max(1, Math.min(enParalelo, items.length));
  await Promise.all(Array.from({ length: n }, trabajador));
  return { hechos, fallidos };
}
