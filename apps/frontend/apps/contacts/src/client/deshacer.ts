/**
 * «Deshacer» del aviso de «movido a la papelera» (uno o en lote).
 *
 * UNA petición al lote de restaurar del servicio (`/api/batch`, action `restore`), que
 * revive en una sola operación lo último que se tiró de cada UID: las dos copias de un
 * UID que vive en dos libretas, y no una copia más vieja. Antes se pedía la papelera
 * —que el servicio corta en 200— y se restauraba fila a fila, así que un lote grande
 * no volvía entero. Lo mismo, con los mismos nombres, en `apps/fyne/contactos/deshacer.go`.
 */
import { batchRestore } from './contactsApi.js';

/**
 * Restaura lo que se acaba de mandar a la papelera. `tirados` es lo que el servicio dijo
 * que tiró (no la lista de UID: uno ajeno no se tira y uno en dos libretas cuenta dos),
 * y con eso se compara lo que vuelve. Lo que falta (ya purgado, o un UID vivo que choca)
 * se cuenta, en vez de dar por hecho lo que no se ha hecho.
 */
export async function deshacerPapelera(
  uids: string[],
  tirados: number,
): Promise<{ restaurados: number; fallidos: number }> {
  const { restored } = await batchRestore(uids);
  return { restaurados: restored, fallidos: Math.max(tirados - restored, 0) };
}
