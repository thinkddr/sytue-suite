/**
 * «Quién reaccionó» (Ola 3B): etiqueta con los NOMBRES de los reactores para el
 * tooltip y la etiqueta accesible del chip de reacción, como Slack/Teams/Mattermost.
 *
 * El servicio Go envía la lista de subs que reaccionaron (`subs`) en cada respuesta
 * REST y en cada frame WS; los nombres se resuelven contra el mapa sub -> nombre que
 * el cliente ya descarga para las menciones. Un sub que NO se resuelve nunca se
 * inventa: se cuenta en el «y N más» (o en «N personas» si no se resuelve ninguno).
 *
 * La frase NO se construye pegando trozos: la plantilla completa vive en el catálogo
 * («{names} reacted with {emoji}») porque el orden de los elementos y la concordancia
 * del verbo cambian entre lenguas. Lo único que se compone aquí es la ENUMERACIÓN de
 * nombres, y de eso se encarga `Intl.ListFormat` vía `formatList` (que mete la «y»,
 * la «e» ante i-, el «und» o el «et» según el idioma).
 */
import { formatList, type Locale } from '@sytue/i18n';
import type { T } from '../i18n/index.js';

/** Nombres que se listan antes de agrupar el resto en «y N más» (como Slack). */
const MAX_NAMES = 3;

/**
 * Devuelve «Ana, Luis y 3 más reaccionaron con 👍», o undefined si el mensaje no
 * trae `subs` (servicio antiguo o reacción optimista sin lista): el llamante cae
 * entonces a la etiqueta de solo conteo.
 */
export function reactedByLabel(
  locale: Locale,
  t: T,
  emoji: string,
  subs: string[] | undefined,
  names?: Record<string, string>,
): string | undefined {
  if (!subs || subs.length === 0) return undefined;
  const shown: string[] = [];
  for (const sub of subs) {
    if (shown.length === MAX_NAMES) break;
    const name = names?.[sub];
    if (name) shown.push(name);
  }
  const rest = subs.length - shown.length;
  // Ningún nombre resoluble: se informa del número, sin inventar a nadie.
  if (shown.length === 0) return t('reaction.byCount', { count: rest, emoji });
  const list = formatList(
    locale,
    rest > 0 ? [...shown, t('reaction.andMore', { count: rest })] : shown,
  );
  return t('reaction.byNames', { count: subs.length, names: list, emoji });
}
