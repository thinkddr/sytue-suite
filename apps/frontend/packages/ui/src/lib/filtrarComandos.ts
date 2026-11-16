/**
 * El filtrado de la paleta de órdenes (⌘K), junto a `PaletaDeComandos`.
 *
 * Nació en Sytue Cloud; Sytue Admin necesitaba exactamente lo mismo y copiarlo
 * habría sido la segunda versión de una regla que debe ser una sola (qué casa con
 * qué). Pliega acentos y mayúsculas con `plegar`: «reunion» encuentra «Reunión».
 */
import { plegar } from '@sytue/i18n';

/**
 * Filtra por tokens: cada palabra de la consulta debe aparecer como subcadena en
 * label + keywords + grupo (AND). Conserva el orden de entrada (que ya llega
 * agrupado). Consulta vacía → todos.
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
