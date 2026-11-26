/**
 * Las dos decisiones de la bandeja de notificaciones que no son de pintar, aparte para
 * probarlas sin montar la isla.
 */
import type { Notification } from '@sytue/api-clients';
import { TIPOS_CRITICOS } from '@sytue/shell';
import { silenciable } from './reglasAvisos.js';

/**
 * Si un aviso admite «Silenciar avisos como este»: su origen se puede callar (Sytue ID
 * no) y su tipo no es de los que piden actuar (`TIPOS_CRITICOS`), que el Hub entrega
 * siempre. Ofrecerlo ahí sería un botón que no hace nada. La regla que se guarda es la
 * de «Qué quieres recibir» por tipo: origen y `kind` = el tipo del aviso.
 */
export function silenciableDesdeElAviso(n: Pick<Notification, 'source' | 'type'>): boolean {
  return Boolean(n.type) && silenciable(n.source) && !TIPOS_CRITICOS.has(n.type ?? '');
}

/**
 * Si un clic en un aviso es de los que el navegador abre APARTE (Ctrl/⌘, Mayús, la
 * rueda): entonces no se navega en esta pestaña, solo se marca leído y se deja al
 * navegador hacer lo suyo. Antes se cancelaba siempre y el aviso se abría encima de
 * la bandeja, perdiendo el sitio.
 */
export function clicAparte(
  e: Pick<MouseEvent, 'ctrlKey' | 'metaKey' | 'shiftKey' | 'button'>,
): boolean {
  return e.ctrlKey || e.metaKey || e.shiftKey || e.button === 1;
}
