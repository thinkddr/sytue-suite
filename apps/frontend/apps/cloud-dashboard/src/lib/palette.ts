/**
 * Lógica pura de la paleta de comandos (⌘K). Aislada del componente para poder
 * probarla sin DOM: el filtrado por tokens es la única lógica no trivial de
 * CommandPalette (el teclado y el índice activo los lleva `PaletaDeComandos`). docs/CONVENTIONS.md §14.
 */
import { type IconName } from '@sytue/ui';
import { plegar } from '@sytue/i18n';

/**
 * Grupo de la paleta, YA TRADUCIDO. Es texto visible y además entra en el
 * filtrado (buscar «actions» debe encontrar el grupo Acciones en su idioma), así
 * que aquí viaja la etiqueta y no un identificador.
 */
export type PaletteGroup = string;

export interface PaletteCommand {
  id: string;
  group: PaletteGroup;
  /** Texto visible y principal para la búsqueda. */
  label: string;
  icon: IconName;
  /** Contexto a la derecha (slug del proyecto, atajo…). */
  hint?: string;
  /** Términos extra buscables, no visibles (alias, sinónimos). */
  keywords?: string;
  /** Efecto real al ejecutar (navegar o mutar); nunca un no-op. */
  run: () => void | Promise<void>;
}

/**
 * Filtra por tokens: sin distinción de mayúsculas, cada palabra de la consulta
 * debe aparecer como subcadena en label + keywords + grupo (AND). Preserva el
 * orden de entrada (que ya viene agrupado). Consulta vacía → todos.
 */
export function filterCommands<T extends { label: string; keywords?: string; group: string }>(
  commands: T[],
  query: string,
): T[] {
  const tokens = plegar(query).split(/\s+/).filter(Boolean);
  if (tokens.length === 0) return commands;
  return commands.filter((c) => {
    const hay = plegar(`${c.label} ${c.keywords ?? ''} ${c.group}`);
    return tokens.every((t) => hay.includes(t));
  });
}
