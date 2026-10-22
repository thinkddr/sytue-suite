/**
 * Contrato del catálogo. El catálogo inglés es la FUENTE DE VERDAD: de él se
 * derivan las claves válidas y la forma de cada mensaje, y los otros cuatro se
 * declaran `: Catalog`, así que TypeScript falla si a uno le falta una clave,
 * le sobra, o convierte en texto plano un mensaje que en inglés depende de una
 * cantidad. Objetivo: que un francés sin traducir rompa `astro check`, en vez de
 * salir en inglés en producción sin que nadie se entere (docs/CONVENTIONS.md §13.1).
 */
import type { en } from './en.js';

/**
 * Formas de un mensaje con cantidad. Las categorías las nombra CLDR y las elige
 * `Intl.PluralRules`; `other` es obligatoria porque es la única que existe en
 * todos los idiomas (y hace de red si una lengua usa una categoría que el
 * traductor no rellenó).
 */
export type PluralForms = Partial<Record<Intl.LDMLPluralRule, string>> & { other: string };

export type MessageKey = keyof typeof en;

export type Catalog = {
  [K in MessageKey]: (typeof en)[K] extends string ? string : PluralForms;
};
