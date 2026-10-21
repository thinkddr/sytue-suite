/**
 * Desenvoltura del stream de logs en vivo (sin DOM: corre en la isla Preact y se
 * comprueba con `bun test`).
 *
 * EL FORMATO DE ARRIBA es SSE de manual: el control-plane emite UNA LÍNEA DE LOG
 * POR EVENTO, `data: <línea>\n\n`. Aquí solo hay que recomponer los eventos que la
 * red parta entre dos lecturas, que es lo único que TCP no garantiza.
 *
 * Hubo una época en que el servidor troceaba por BYTES (bloques de 4 KiB envueltos
 * de una pieza) y este módulo cargaba con un despiece a medida para recomponer las
 * líneas partidas. Se arregló en el servidor, que es donde estaba el fallo: aquel
 * marcado incumplía la especificación —y el contrato publicado en el OpenAPI— y
 * cualquier cliente estricto, como el `EventSource` de api-clients, se comía casi
 * todo el log en silencio.
 */
import type { LogLevel, LogLine } from './types.js';

/** Separador de eventos SSE. */
const FRAME_SEP = '\n\n';
/** Prefijo del campo de datos. */
const FRAME_HEAD = 'data: ';

/**
 * Líneas completas del stream; el evento que aún no ha llegado entero vuelve como
 * `carry` para el trozo siguiente.
 */
export function takeLines(carry: string, chunk: string): { lines: string[]; carry: string } {
  const frames = (carry + chunk).split(FRAME_SEP);
  // El último trozo nunca está cerrado por un separador: sigue en vuelo.
  const rest = frames.pop() ?? '';
  const lines: string[] = [];
  for (const f of frames) {
    // Todo lo que no sea un campo `data:` se ignora, como manda la especificación:
    // así los comentarios de keep-alive (`: ping`) no se cuelan como líneas de log.
    if (f.startsWith(FRAME_HEAD)) lines.push(f.slice(FRAME_HEAD.length));
  }
  return { lines, carry: rest };
}

/** Nivel por heurística sobre el texto — mismo criterio que el log de build del servidor. */
export function levelOf(message: string): LogLevel {
  if (/\b(error|fatal|failed|panic)\b/i.test(message)) return 'error';
  if (/\bwarn/i.test(message)) return 'warn';
  return 'info';
}

/** Línea de runtime lista para el visor (el control-plane no marca nivel ni hora). */
export function runtimeLine(message: string): LogLine {
  return { ts: '', level: levelOf(message), source: 'runtime', message };
}

/**
 * Añade líneas al buffer conservando solo las últimas `max`: un servicio con
 * tráfico real escupe miles de líneas por minuto y el buffer no puede crecer sin
 * límite en la pestaña del navegador.
 */
export function appendCapped(prev: LogLine[], incoming: LogLine[], max: number): LogLine[] {
  if (incoming.length === 0) return prev;
  const next = prev.concat(incoming);
  return next.length > max ? next.slice(next.length - max) : next;
}
