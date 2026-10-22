/**
 * Agrupación de la línea de tiempo por mes de captura.
 *
 * Separada del componente porque es la única lógica pura de la app y la que
 * tiene casos que se rompen en silencio: un enero de 2019 pegado a un enero de
 * 2026 sin que nadie lo note, o un grupo que aparece dos veces porque la lista
 * llegó desordenada de otra página.
 */
import type { Locale } from '@sytue/i18n';
import { formatDate } from '@sytue/i18n';

export interface ConFecha {
  takenAt: string;
}

export interface Grupo<T extends ConFecha> {
  /** Rótulo visible: mes y año, en el idioma de la sesión. */
  titulo: string;
  /** Clave estable `AAAA-MM`, independiente del idioma: es la que decide la agrupación. */
  clave: string;
  fotos: T[];
}

/**
 * Agrupa por mes, en el orden en que llegan las fotos.
 *
 * LA CLAVE ES `AAAA-MM` Y NO EL RÓTULO. Agrupar por el texto traducido haría
 * que dos meses distintos con el mismo nombre —enero de 2019 y enero de 2026—
 * se distinguieran solo si el formato incluye el año, y que cambiar de idioma
 * pudiera partir o unir grupos. La clave se calcula sobre la fecha; el rótulo se
 * pinta después.
 *
 * SOLO SE UNE CON EL GRUPO ANTERIOR. La línea de tiempo llega ordenada del
 * servidor y se acumula página a página; si una foto de un mes ya cerrado
 * apareciera más tarde, abriría un grupo nuevo en vez de reordenar lo pintado —
 * que movería lo que la persona está mirando—. Es deliberado.
 */
export function porMes<T extends ConFecha>(fotos: readonly T[], locale: Locale): Grupo<T>[] {
  const grupos: Grupo<T>[] = [];
  for (const f of fotos) {
    const d = new Date(f.takenAt);
    const clave = `${d.getUTCFullYear()}-${String(d.getUTCMonth() + 1).padStart(2, '0')}`;
    const ultimo = grupos[grupos.length - 1];
    if (ultimo && ultimo.clave === clave) {
      ultimo.fotos.push(f);
      continue;
    }
    grupos.push({
      clave,
      titulo: conInicial(
        formatDate(locale, d, { year: 'numeric', month: 'long', timeZone: 'UTC' }),
        locale,
      ),
      fotos: [f],
    });
  }
  return grupos;
}

/**
 * Solo la PRIMERA letra en mayúscula. En castellano, francés e italiano los
 * meses van en minúscula y `Intl` los devuelve así; la cabecera llevaba
 * `capitalize`, que en CSS es palabra a palabra, y salía «Septiembre De 2026».
 */
export function conInicial(s: string, locale: Locale): string {
  return s.charAt(0).toLocaleUpperCase(locale) + s.slice(1);
}
