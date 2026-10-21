/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Tabla de rutas del BFF de scripts de usuario (pages/api/scripts.ts): de la petición del
 * navegador (método + parámetros) a LA ruta del servicio Go, o `null`. Una lista cerrada:
 * ningún fragmento de ruta llega del cliente sin pasar por una expresión estricta, así que
 * el BFF no es un proxy abierto hacia /api/v1. Quién puede qué lo decide el servicio.
 */

const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/;
const ENTERO = /^\d{1,9}$/;

export interface RutaScripts {
  /** Ruta relativa a /api/v1. */
  path: string;
  /** Método hacia el servicio. */
  method: 'GET' | 'POST' | 'PUT' | 'DELETE';
  /** ¿Lleva cuerpo JSON? */
  cuerpo: boolean;
}

/** `p` son los parámetros de la query (`sid`, `op`, `v`, `run`, `doc`, `before_id`, `limit`). */
export function rutaScripts(metodo: string, p: URLSearchParams): RutaScripts | null {
  const sid = p.get('sid');
  const op = p.get('op') ?? '';
  const v = p.get('v');
  const run = p.get('run');
  if (sid !== null && !UUID.test(sid)) return null;
  if (v !== null && !ENTERO.test(v)) return null;
  if (run !== null && !ENTERO.test(run)) return null;
  const pagina = (): string | null => {
    const antes = p.get('before_id');
    const limite = p.get('limit');
    if ((antes !== null && !ENTERO.test(antes)) || (limite !== null && !ENTERO.test(limite)))
      return null;
    const q = new URLSearchParams();
    if (antes) q.set('before_id', antes);
    if (limite) q.set('limit', limite);
    const s = q.toString();
    return s ? `?${s}` : '';
  };

  if (metodo === 'GET') {
    if (op === 'quota') return { path: '/script-quota', method: 'GET', cuerpo: false };
    if (op === 'runs') {
      const q = pagina();
      if (q === null) return null;
      const doc = p.get('doc');
      if (doc !== null) {
        if (!UUID.test(doc)) return null;
        return { path: `/documents/${doc}/script-runs${q}`, method: 'GET', cuerpo: false };
      }
      return { path: `/script-runs${q}`, method: 'GET', cuerpo: false };
    }
    if (op !== '') return null;
    return sid
      ? { path: `/scripts/${sid}`, method: 'GET', cuerpo: false }
      : { path: '/scripts', method: 'GET', cuerpo: false };
  }
  if (metodo === 'POST') {
    if (op === '' && sid === null) return { path: '/scripts', method: 'POST', cuerpo: true };
    if (sid === null) return null;
    if (op === 'version') return { path: `/scripts/${sid}/versions`, method: 'POST', cuerpo: true };
    if (v === null) return null;
    if (op === 'propose' || op === 'review' || op === 'run')
      return {
        path: `/scripts/${sid}/versions/${v}/${op === 'run' ? 'runs' : op}`,
        method: 'POST',
        cuerpo: op !== 'propose',
      };
    return null;
  }
  if (metodo === 'PUT') {
    if (run !== null && sid === null && op === '')
      return { path: `/script-runs/${run}`, method: 'PUT', cuerpo: true };
    if (sid !== null && op === 'scope')
      return { path: `/scripts/${sid}/scope`, method: 'PUT', cuerpo: true };
    return null;
  }
  if (metodo === 'DELETE' && sid !== null && op === '')
    return { path: `/scripts/${sid}`, method: 'DELETE', cuerpo: false };
  return null;
}
