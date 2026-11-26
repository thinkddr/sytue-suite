/**
 * Lógica del historial de despliegues que no depende de la pantalla: qué fila es la
 * producción viva, qué verbo admite cada una, cómo se filtra y qué dice el conjunto
 * de cómo entrega el equipo. Sin DOM ni red: se prueba en Bun y sirve igual en el
 * SSR (mapeo de datos vivos) que en la isla (filtros y métricas).
 */
import { contiene } from '@sytue/i18n';
import type { DeployStatus, Deployment } from './types.js';

/**
 * Marca como `current` la producción que está sirviendo.
 *
 * El control-plane no manda un «actual»: lo que garantiza es que, al promocionar
 * una producción, retira las demás (`retired`), así que la viva es la producción
 * `ready`. Si hubiera más de una —un despliegue nuevo que aún no retiró al
 * anterior—, manda la más reciente, que es la primera de la lista (llega de más
 * nueva a más vieja). Sin esto, con datos reales ninguna fila decía «Producción»
 * y el menú ofrecía «volver» a la versión que ya estaba publicada.
 */
export function marcarProduccionActual(list: Deployment[]): Deployment[] {
  const viva = list.find((d) => d.target === 'production' && d.status === 'ready');
  return list.map((d) => (d === viva ? { ...d, current: true } : d));
}

/**
 * El verbo que admite una fila, o null si no admite ninguno.
 *
 * - Una preview lista se PUBLICA (crea una producción con su imagen).
 * - Una producción anterior —lista o ya reemplazada— se RECUPERA con un rollback:
 *   el control-plane la vuelve a poner en `ready` y retira las demás.
 * - La producción viva, lo que está en curso y lo que falló no admiten nada.
 */
export function accionDisponible(d: Deployment): 'promote' | 'rollback' | null {
  if (d.current) return null;
  if (d.target === 'preview') return d.status === 'ready' ? 'promote' : null;
  return d.status === 'ready' || d.status === 'retired' ? 'rollback' : null;
}

/**
 * Rama o etiqueta que se puede desplegar. La MISMA regla que `domain.ValidGitRef`
 * del control-plane (que es la que manda y protege el contexto de compilación):
 * repetirla aquí solo sirve para decir «así no» en el formulario y no tras un 400.
 */
export function refValida(ref: string): boolean {
  return /^[A-Za-z0-9._][A-Za-z0-9._/-]{0,254}$/.test(ref) && !ref.includes('..');
}

/** SHA de commit, corto o completo (SHA-1 o SHA-256): `domain.ValidCommitSHA`. */
export function commitValido(sha: string): boolean {
  return /^[0-9a-fA-F]{7,64}$/.test(sha);
}

export interface FiltroDespliegues {
  /** Texto libre: rama, commit, mensaje o autor (sin acentos ni mayúsculas). */
  q: string;
  /** Estado, o '' para todos. */
  estado: DeployStatus | '';
}

/** Filtra el historial por texto y estado. Con el filtro vacío devuelve la misma lista. */
export function filtrarDespliegues(list: Deployment[], f: FiltroDespliegues): Deployment[] {
  const q = f.q.trim();
  if (!q && !f.estado) return list;
  return list.filter((d) => {
    if (f.estado && d.status !== f.estado) return false;
    if (!q) return true;
    return (
      contiene(d.branch, q) ||
      contiene(d.commitSha, q) ||
      contiene(d.commitMessage, q) ||
      contiene(d.author, q) ||
      (d.pr !== undefined && contiene(`#${d.pr}`, q))
    );
  });
}

export interface MetricasEntrega {
  /** Despliegues sobre los que se calcula (los que hay cargados). */
  muestra: number;
  /** Producciones que llegaron a publicarse, por semana. null sin ninguna. */
  produccionPorSemana: number | null;
  /** Fracción (0–1) de despliegues TERMINADOS que acabaron bien. null sin terminados. */
  tasaExito: number | null;
  /** Mediana de la duración de la compilación, en segundos. null sin datos. */
  duracionMediana: number | null;
  /** Fecha ISO de la última producción publicada. null si no hay. */
  ultimaProduccion: string | null;
}

const DIA_MS = 24 * 60 * 60 * 1000;

/**
 * Las cuatro cifras que dicen cómo entrega un equipo (la versión corta de las DORA):
 * con qué frecuencia llega algo a producción, cuántos despliegues salen bien, cuánto
 * tarda una compilación y cuándo se publicó lo último.
 *
 * Se calcula sobre lo cargado (los últimos despliegues), no sobre un mes fijo: la
 * pantalla lo dice con la muestra, para que nadie lea «2 por semana» como un dato
 * del trimestre. Lo cancelado no cuenta ni a favor ni en contra: no llegó a probarse.
 */
export function metricasDeEntrega(list: Deployment[], ahora: number): MetricasEntrega {
  const publicadas = list.filter(
    (d) => d.target === 'production' && (d.status === 'ready' || d.status === 'retired'),
  );
  const terminados = list.filter(
    (d) => d.status === 'ready' || d.status === 'retired' || d.status === 'error',
  );
  const bien = terminados.filter((d) => d.status !== 'error').length;

  const fechas = list.map((d) => Date.parse(d.createdAt)).filter((n) => Number.isFinite(n));
  const masVieja = fechas.length > 0 ? Math.min(...fechas) : ahora;
  // Al menos un día de ventana: con todo hecho hoy, «7 por semana» por dos
  // despliegues de esta mañana sería una cifra inventada.
  const semanas = Math.max(1, (ahora - masVieja) / DIA_MS) / 7;

  const duraciones = list
    .map((d) => d.durationSec)
    .filter((n): n is number => typeof n === 'number' && n > 0)
    .sort((a, b) => a - b);

  const ultima = publicadas
    .map((d) => d.createdAt)
    .filter((f) => Number.isFinite(Date.parse(f)))
    .sort((a, b) => Date.parse(b) - Date.parse(a))[0];

  return {
    muestra: list.length,
    produccionPorSemana: publicadas.length > 0 ? publicadas.length / semanas : null,
    tasaExito: terminados.length > 0 ? bien / terminados.length : null,
    duracionMediana: mediana(duraciones),
    ultimaProduccion: ultima ?? null,
  };
}

function mediana(ordenados: number[]): number | null {
  if (ordenados.length === 0) return null;
  const m = Math.floor(ordenados.length / 2);
  return ordenados.length % 2 === 1
    ? (ordenados[m] ?? null)
    : Math.round(((ordenados[m - 1] ?? 0) + (ordenados[m] ?? 0)) / 2);
}
