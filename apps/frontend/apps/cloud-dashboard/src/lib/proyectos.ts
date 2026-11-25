/**
 * Orden de la lista de proyectos. Puro, para probarlo en Bun (test/proyectos.test.ts).
 */
import type { Locale } from '@sytue/i18n';
import type { Project } from './types.js';

/** Criterios de orden que ofrece la pantalla. */
export type OrdenProyectos = 'recent' | 'name' | 'paused';

/**
 * Ordena sin tocar la lista original.
 *
 * - `recent`: los creados más tarde primero (lo último en lo que se trabaja).
 * - `name`: alfabético con la colación del idioma de la persona —«Árbol» va con la A,
 *   no detrás de la Z como haría una comparación de códigos—.
 * - `paused`: los pausados arriba, para repasar qué se apagó y sigue reservando
 *   dominio y base de datos; dentro de cada grupo, por nombre.
 */
export function ordenarProyectos(
  projects: Project[],
  orden: OrdenProyectos,
  locale: Locale,
): Project[] {
  const col = new Intl.Collator(locale, { sensitivity: 'base', numeric: true });
  const porNombre = (a: Project, b: Project) => col.compare(a.name, b.name);
  const copia = [...projects];
  switch (orden) {
    case 'name':
      return copia.sort(porNombre);
    case 'paused':
      return copia.sort(
        (a, b) => Number(b.paused === true) - Number(a.paused === true) || porNombre(a, b),
      );
    case 'recent':
      return copia.sort(
        (a, b) => Date.parse(b.createdAt) - Date.parse(a.createdAt) || porNombre(a, b),
      );
  }
}
