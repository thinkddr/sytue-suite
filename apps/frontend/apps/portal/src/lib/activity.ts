/**
 * Agrupación por DÍA de la actividad reciente (Sytue Hub, GET /recent).
 * Función pura, sin dependencias de runtime — se prueba en test/activity.test.ts.
 *
 * El Hub devuelve los hits de todos los productos ya ordenados por recencia; lo
 * que falta para leerlos es el corte por día, que es como se mira una actividad:
 * «hoy», «ayer» y luego fechas.
 */
import type { SearchResult } from '@sytue/api-clients';

export interface DayGroup {
  /** Clave estable `YYYY-MM-DD` en la zona del navegador que renderiza. */
  day: string;
  /** Instante representativo del grupo, para formatear el encabezado. */
  at: Date;
  items: SearchResult[];
}

/**
 * Agrupa por día natural conservando el orden que trae el Hub. Los hits SIN
 * marca de tiempo van a un grupo propio al final (`day: ''`): no se inventa una
 * fecha para colocarlos, porque eso los mezclaría con actividad real de un día
 * concreto y nadie podría distinguirlo después.
 */
export function groupByDay(results: readonly SearchResult[]): DayGroup[] {
  const orden: string[] = [];
  const cubos = new Map<string, DayGroup>();

  for (const r of results) {
    const at = r.timestamp !== undefined ? new Date(r.timestamp) : undefined;
    // `toISOString` daría el día en UTC y a las 00:30 de Madrid enseñaría «ayer».
    const day = at ? claveLocal(at) : '';
    let grupo = cubos.get(day);
    if (!grupo) {
      grupo = { day, at: at ?? new Date(0), items: [] };
      cubos.set(day, grupo);
      orden.push(day);
    }
    grupo.items.push(r);
  }

  // Los sin fecha, al final, vengan donde vengan en el orden del Hub.
  return orden.sort((a, b) => (a === '' ? 1 : b === '' ? -1 : 0)).map((d) => cubos.get(d)!);
}

/** `YYYY-MM-DD` en hora LOCAL (no UTC). */
function claveLocal(d: Date): string {
  const mes = String(d.getMonth() + 1).padStart(2, '0');
  const dia = String(d.getDate()).padStart(2, '0');
  return `${d.getFullYear()}-${mes}-${dia}`;
}

/**
 * Etiqueta del encabezado de un grupo: «hoy» y «ayer» son las dos que de verdad
 * se leen de un vistazo; el resto es la fecha, que la formatea quien pinta con
 * el locale del usuario. Devuelve la CLAVE de traducción, no el texto: esta
 * función no sabe de idiomas y no debe.
 */
export function dayLabelKey(day: string, hoy: Date = new Date()): 'today' | 'yesterday' | 'date' {
  if (day === '') return 'date';
  if (day === claveLocal(hoy)) return 'today';
  const ayer = new Date(hoy);
  ayer.setDate(ayer.getDate() - 1);
  return day === claveLocal(ayer) ? 'yesterday' : 'date';
}
