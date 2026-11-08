/**
 * Lo que se hace elemento a elemento porque el servicio no lo tiene en lote:
 * deshacer un envío a la papelera de varios elementos y vaciar la papelera.
 *
 * El servicio restaura y purga POR ID (`/files/{id}/restore`, `/folders/{id}/purge`),
 * y cada llamada pasa por su propia autorización: purgar en un solo gesto no se salta
 * el permiso de nadie, y lo que se niega se queda donde estaba y se cuenta aparte.
 * Vive fuera del componente porque los dos fallos que vigila no se ven: mandar las
 * cien llamadas a la vez y dar por hecho lo que salió mal.
 */

/** Un elemento al que se puede llamar por id: lo que piden restaurar y purgar. */
export interface PorId {
  id: string;
  kind: 'file' | 'folder';
  name: string;
}

/**
 * Lo de una selección que la papelera sabe devolver: lo que tiene id. Sin id no hay a
 * qué llamar (la papelera trabaja por id, no por ruta), igual que en el «Deshacer» de
 * un solo elemento.
 */
export function conId(
  items: readonly { id?: string | null; isDirectory: boolean; name: string }[],
): PorId[] {
  const out: PorId[] = [];
  for (const it of items) {
    if (it.id) out.push({ id: it.id, kind: it.isDirectory ? 'folder' : 'file', name: it.name });
  }
  return out;
}

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

/**
 * Lo que está a la vista cuando llega un «Deshacer». El aviso dura 10 s y en ese rato
 * se puede cambiar de carpeta o de sección: recargar la ruta capturada al tirar pintaba
 * la carpeta vieja bajo la miga de la nueva. Se lee de una referencia viva, no del
 * render que creó el aviso.
 */
export interface VistaViva {
  montada: boolean;
  ruta: string;
}

/** Recarga la carpeta que se ve ahora; si el explorador ya no está, nada. */
export async function recargarLaVista(
  vista: { readonly current: VistaViva },
  cargar: (ruta: string) => Promise<void>,
): Promise<void> {
  if (vista.current.montada) await cargar(vista.current.ruta);
}

// Con el explorador desmontado (se ha ido a la Papelera), quien tiene que enterarse de
// que algo ha vuelto es la papelera: si no, sigue enseñando como borrado lo restaurado.
const restauraciones = new EventTarget();

/** Avisa de que algo ha salido de la papelera por un camino que no es la propia papelera. */
export function avisarRestaurados(): void {
  restauraciones.dispatchEvent(new Event('restaurados'));
}

/** Se suscribe a {@link avisarRestaurados}; devuelve la baja, para la limpieza del efecto. */
export function alRestaurar(fn: () => void): () => void {
  restauraciones.addEventListener('restaurados', fn);
  return () => restauraciones.removeEventListener('restaurados', fn);
}
