/**
 * Paletas de color de Notas con el nombre de cada tono. El lector de pantalla y
 * el `title` dicen «Verde», no «almohadilla 16794a»: cada hex de una paleta tiene
 * su clave de catálogo. Los estados de etiqueta y los cuadernos usan la paleta de
 * colores de usuario de la suite (`PALETA_DE_USUARIO`, con sus nombres en el catálogo
 * común); aquí queda solo la del color de texto, medida para el papel de la nota.
 */
import type { ColorDeSelector } from '@sytue/ui';
import type { NotesMessageKey, T } from '../i18n/index.js';

/** Color de texto del editor: legibles sobre el papel claro de la nota. */
export const COLORES_DE_TEXTO: readonly string[] = [
  '#b91c1c',
  '#c2410c',
  '#a16207',
  '#15803d',
  '#0e7490',
  '#1d4ed8',
  '#7e22ce',
  '#be185d',
];

const NOMBRE: Record<string, NotesMessageKey> = {
  '#b91c1c': 'colour.red',
  '#c2410c': 'colour.orange',
  '#a16207': 'colour.ochre',
  '#15803d': 'colour.green',
  '#0e7490': 'colour.cyan',
  '#1d4ed8': 'colour.blue',
  '#7e22ce': 'colour.purple',
  '#be185d': 'colour.pink',
};

/** Nombre traducido de un tono de las paletas de Notas. */
export function nombreDeColor(t: T, hex: string): string {
  const k = NOMBRE[hex.toLowerCase()];
  return k ? t(k) : t('notebook.color');
}

/** Una paleta con sus nombres, lista para `SelectorDeColor`. */
export function conNombres(t: T, paleta: ReadonlyArray<string>): ColorDeSelector[] {
  return paleta.map((valor) => ({ valor, nombre: nombreDeColor(t, valor) }));
}
