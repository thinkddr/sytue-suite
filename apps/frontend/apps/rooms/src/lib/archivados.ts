import type { Channel } from './chat.js';
import type { MessageKey } from '../i18n/index.js';

/**
 * Pestaña «Archivados» de «Explorar canales»: verlos, desarchivarlos y renombrarlos.
 * Las reglas viven aquí, sin DOM, para probarlas; su gemelo en Fyne está en
 * apps/fyne/apps/rooms/archivados.go (`SoloArchivados`, `ClaveErrorArchivado`,
 * `NombreNuevo`).
 */

/** Los canales archivados, sin directos (un DM no se archiva), por nombre. */
export function soloArchivados(channels: readonly Channel[]): Channel[] {
  return channels
    .filter((c) => c.archived && c.kind !== 'direct')
    .sort((a, b) => a.name.localeCompare(b.name));
}

/** El aviso de un fallo al desarchivar o renombrar, según lo que dijo el servicio. */
export function claveErrorArchivado(status: number | undefined): MessageKey {
  if (status === 403) return 'archived.forbidden';
  if (status === 409) return 'archived.nameTaken';
  if (status === 400) return 'archived.invalidName';
  return 'archived.failed';
}

/** El nombre que se manda al renombrar, o null si no hay nada que mandar (vacío o igual). */
export function nombreNuevo(actual: string, escrito: string): string | null {
  const nombre = escrito.trim();
  return nombre && nombre !== actual ? nombre : null;
}

/**
 * La fila que recibe el foco cuando `quitado` sale de la lista (se desarchivó): la
 * siguiente, o la anterior si era la última, o null si la lista se queda vacía. Sin
 * esto el foco caía al cuerpo del diálogo y, con teclado, había que volver a tabular
 * desde el principio (docs/design.md §2.6).
 */
export function filaTrasQuitar(ids: readonly string[], quitado: string): string | null {
  const i = ids.indexOf(quitado);
  if (i < 0) return null;
  return ids[i + 1] ?? ids[i - 1] ?? null;
}
