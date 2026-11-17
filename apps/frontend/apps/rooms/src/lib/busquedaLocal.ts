/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Búsqueda de mensajes EN EL DISPOSITIVO, para los canales cifrados de extremo a extremo.
 *
 * El servidor no ve el texto, así que no puede indexarlo (y no debe: sería guardar en
 * claro lo que el cifrado protege). Se busca en la historia que el cliente ya descifra:
 * los últimos mensajes de cada canal, cargados al abrir el panel y guardados SOLO en
 * memoria —nada de localStorage ni IndexedDB—; al cerrar el panel desaparecen. Por eso
 * el panel dice «en los últimos N mensajes»: la búsqueda es honesta sobre su alcance.
 *
 * Operadores: los mismos que el backend (`from:`, `in:`, `before:`, `after:`, `has:file`);
 * cualquier otro se busca como texto. Sin acentos ni mayúsculas, todos los términos (Y).
 */
import { plegar } from '@sytue/i18n';
import type { ChatMessage } from './chat.js';

/** Mensajes por canal que se cargan para buscar (una página del servicio). */
export const MENSAJES_POR_CANAL = 50;
/** Tope de resultados, el mismo que el backend. */
export const MAX_RESULTADOS = 30;
/** «Archivos del canal» con cifrado: cuántos mensajes de ESE canal se recorren hacia atrás
 *  (D-ROOM-03). Mil cubre meses de un canal normal sin tener a la persona esperando. */
export const MENSAJES_ARCHIVOS_CANAL = 1000;
/** …y cuántos adjuntos se enseñan: es una lista de archivos, no un fragmento de búsqueda. */
export const MAX_ARCHIVOS_CANAL = 200;

interface Consulta {
  terminos: string[];
  de: string[];
  en: string[];
  antes?: string;
  despues?: string;
  conFichero: boolean;
}

/** Minúsculas y sin diacríticos: «Reunión» encuentra «reunion». */
export function normalizar(s: string): string {
  return plegar(s);
}

export function analizarConsulta(q: string): Consulta {
  const c: Consulta = { terminos: [], de: [], en: [], conFichero: false };
  for (const pieza of q.trim().split(/\s+/)) {
    if (!pieza) continue;
    const m = /^(from|in|before|after|has):(.*)$/i.exec(pieza);
    const op = m?.[1]?.toLowerCase();
    const valor = normalizar((m?.[2] ?? '').replace(/^[@#]/, ''));
    if (!m || !valor) c.terminos.push(normalizar(pieza));
    else if (op === 'from') c.de.push(valor);
    else if (op === 'in') c.en.push(valor);
    else if (op === 'before') c.antes = valor;
    else if (op === 'after') c.despues = valor;
    else if (valor === 'file') c.conFichero = true;
    else c.terminos.push(normalizar(pieza));
  }
  return c;
}

/**
 * Filtra `mensajes` con la consulta `q`. `canales` traduce id → nombre para `in:#nombre`.
 * Devuelve los más recientes primero, hasta MAX_RESULTADOS. Los que no se pudieron
 * descifrar no se buscan: su texto no es el del mensaje.
 */
export function buscarEnLocal(
  q: string,
  mensajes: readonly ChatMessage[],
  canales: Record<string, string>,
  max: number = MAX_RESULTADOS,
): ChatMessage[] {
  const c = analizarConsulta(q);
  if (!c.terminos.length && !c.de.length && !c.en.length && !c.conFichero) return [];
  const fuera = mensajes.filter((m) => {
    if (m.decryptionFailed || m.pending || m.failed) return false;
    if (c.conFichero && !m.attachments?.length) return false;
    const dia = m.createdAt.slice(0, 10);
    if (c.antes && !(dia < c.antes)) return false;
    if (c.despues && !(dia > c.despues)) return false;
    if (c.de.length) {
      const autor = normalizar(`${m.author.name} ${m.author.id}`);
      if (!c.de.every((d) => autor.includes(d))) return false;
    }
    if (c.en.length) {
      const canal = normalizar(canales[m.channelId] ?? m.channelId);
      if (!c.en.some((e) => canal === e || m.channelId === e)) return false;
    }
    if (c.terminos.length) {
      const texto = normalizar([m.body, ...(m.attachments ?? []).map((a) => a.name)].join(' '));
      if (!c.terminos.every((t) => texto.includes(t))) return false;
    }
    return true;
  });
  return fuera.sort((a, b) => b.createdAt.localeCompare(a.createdAt)).slice(0, max);
}

/**
 * Carga la última página de cada canal con `cargar` (que devuelve ya descifrado). Un canal
 * que falla no tumba la búsqueda en los demás. Solo en memoria: el que llama lo guarda en
 * un estado del panel y lo suelta al desmontarlo.
 */
export async function cargarHistoriaLocal(
  canales: readonly string[],
  cargar: (channelId: string) => Promise<{ messages: ChatMessage[] }>,
): Promise<ChatMessage[]> {
  const paginas = await Promise.all(
    canales.map((id) =>
      cargar(id).then(
        (r) => r.messages.slice(-MENSAJES_POR_CANAL),
        () => [] as ChatMessage[],
      ),
    ),
  );
  return paginas.flat();
}

/**
 * Historia profunda de UN canal para «Archivos del canal» con cifrado (D-ROOM-03): pide
 * páginas hacia atrás con `cargar(id, before)` —el mismo cursor por fecha que «cargar
 * anteriores»— hasta reunir `tope` mensajes o llegar al principio. Se para también si una
 * página no trae nada más viejo, para no dar vueltas sobre el mismo cursor. Como
 * `cargarHistoriaLocal`, solo en memoria. Si falla una página intermedia se devuelve lo
 * ya reunido (mejor parte de los archivos que ninguno); si falla la primera, el error.
 */
export async function cargarHistoriaDelCanal(
  channelId: string,
  cargar: (channelId: string, before?: string) => Promise<{ messages: ChatMessage[] }>,
  tope: number = MENSAJES_ARCHIVOS_CANAL,
): Promise<ChatMessage[]> {
  const vistos = new Set<string>();
  const reunidos: ChatMessage[] = [];
  let antes: string | undefined;
  while (reunidos.length < tope) {
    let pagina: ChatMessage[];
    try {
      pagina = (await cargar(channelId, antes)).messages;
    } catch (e) {
      if (antes === undefined) throw e;
      break;
    }
    const nuevos = pagina.filter((m) => !vistos.has(m.id));
    if (!nuevos.length) break;
    for (const m of nuevos) {
      vistos.add(m.id);
      reunidos.push(m);
    }
    const masViejo = nuevos.reduce((a, b) => (b.createdAt < a.createdAt ? b : a));
    if (antes !== undefined && masViejo.createdAt >= antes) break;
    antes = masViejo.createdAt;
  }
  return reunidos.sort((a, b) => a.createdAt.localeCompare(b.createdAt)).slice(-tope);
}
