/* SPDX-License-Identifier: Apache-2.0 */

/**
 * ¿Viene el mensaje del documento propietario del Worker dedicado?
 *
 * Un `DedicatedWorkerGlobalScope` solo recibe lo que le envía, con
 * `worker.postMessage`, el documento que lo creó: ninguna otra ventana ni origen
 * tiene una referencia a él. Esos mensajes llegan con `origin` vacío (son mensajes
 * de puerto, no de ventana). Aun así se comprueba de forma explícita: se aceptan el
 * origen vacío y el propio origen del Worker, y se descarta cualquier otro, de modo
 * que si este código acabara alguna vez escuchando en otro contexto (un Worker
 * compartido, una ventana) no aceptaría órdenes de un origen ajeno. La validación
 * de la forma de cada mensaje sigue siendo cosa del manejador.
 */
export function isOwnerWorkerMessage(
  event: Pick<MessageEvent<unknown>, 'origin'>,
  ownOrigin: string,
): boolean {
  return event.origin === '' || event.origin === ownOrigin;
}
