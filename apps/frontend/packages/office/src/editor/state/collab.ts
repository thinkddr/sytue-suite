/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Colaboración del editor Sytue Docs (BLUEPRINT.md §4.4).
 *
 * TRANSPORTE DE ENTRADA de ops: WebSocket (push) con sondeo como FALLBACK.
 * - WS: `GET /api/collab?id=` (BFF) resuelve la sesión httpOnly y devuelve la
 *   URL del WS del servicio Go con el access token ya en la query (el cliente
 *   nunca maneja la sesión). Al abrir se envía `{"type":"hello","last_seq":N}`
 *   y el servidor reenvía las ops del hueco (replay); después cada lote llega
 *   en tiempo real como `{"type":"ops",...}` (protocolo:
 *   services/docs/internal/collab/protocol.go). Si la conexión no abre o se
 *   cae, se reintenta con backoff exponencial acotado (patrón de
 *   apps/rooms/src/client/chatApi.ts) y mientras tanto manda el fallback.
 * - Fallback (contrato del auditor 2026-07-03 §1/§4): mientras el WS no esté
 *   abierto, cada 4 s (solo con la pestaña visible) se piden las ops con
 *   seq > cursor vía `GET /api/ops` (proxy BFF same-origin). Es la red de
 *   seguridad para una conexión caída o una red que bloquea WebSocket, no el
 *   camino normal: el handshake SÍ autentica desde `e81b2b0d` —`RequireAuth`
 *   acepta `?access_token=` también en el upgrade, no solo en SSE (ver
 *   `platform/pkg/authn/middleware.go` y su test del upgrade)—, así que el WS
 *   entra y el sondeo se apaga. Este comentario decía lo contrario durante
 *   semanas después de arreglarse: si vuelve a haber sondeo permanente, mide el
 *   handshake antes de creerle a la documentación.
 *
 * Ambos transportes alimentan el mismo reparto de un lote de ops ajenas (el eco
 * de las propias se filtra por actor):
 *  - `insert`/`delete` → `onRemoteOps`: la secuencia RGA del cliente
 *    (./coedicion.ts) las fusiona por carácter. Es el camino normal.
 *  - `set` sobre `<clave>:lww` → `onLwwOverride`: el emisor declara que su
 *    `set` de contenido manda sobre cualquier secuencia (documento heredado,
 *    siembra o cambio demasiado grande). Se avisa SIEMPRE, se pueda adoptar o
 *    no: si no, un par seguiría fusionando sobre un estado ya descartado.
 *  - `set` sobre la clave de contenido → `onRemoteContent`, con la regla LWW de
 *    siempre: se adopta si su lamport supera al local Y no hay edición local en
 *    vuelo (recarga suave: contentEpoch++ en useDocument); si el usuario
 *    escribe, no se adopta y su próximo autosave, con lamport mayor, gana.
 *    En hojas y presentaciones (`onRemoteSet`) no se aplica esa regla: cada
 *    escritura ajena va tal cual a la fusión a tres bandas de ./fusion.ts.
 * El ENVÍO sigue por POST /api/ops (autosave); el servidor difunde a la sala WS.
 *
 * Presencia REAL por conexión: cada ~10 s se hace `PUT /api/presence` con el
 * actor de esta pestaña; el servidor lo registra (TTL 30 s) y devuelve las
 * conexiones presentes. Se mantiene INCLUSO con el WS abierto: el registro
 * HTTP es la única fuente que también ve a los clientes en fallback (la
 * presencia de la sala WS solo ve conexiones WS). Cuenta al LECTOR PASIVO:
 * abrir el documento basta. Se excluye la propia pestaña y se deduplica por
 * usuario (dos pestañas del mismo usuario → 1 avatar del otro).
 */
import type { CursorPos, Peer, PeerCursor, SequencedOp } from '../types';
import { parsePeerCursor } from './cursors';
import { claveLww, opDeSecuencia } from './coedicion';
import type { OpRGA } from './rga';

/** Intervalo del sondeo de contenido de fallback (§4.4). */
const POLL_MS = 4000;
/** Intervalo de heartbeat de presencia; el TTL del servidor es 30 s (§4.4). */
const HEARTBEAT_MS = 10_000;
/** Tope del backoff exponencial de reconexión del WS (2s, 4s, … , 30s). */
const WS_BACKOFF_MAX_MS = 30_000;
/** Throttle del envío del cursor local (§4.4): flanco inicial + de parada. */
const CURSOR_THROTTLE_MS = 80;

/** Contrato v1; version ausente conserva servidores anteriores al handshake. */
export const DOC_PROTOCOL_VERSION = 1;

export interface CollabOptions {
  documentId: string;
  /** Actor local (dot.a): sus ops no cuentan como cambio remoto; identifica esta conexión en presencia. */
  actor: string;
  /** Clave del registro LWW del contenido (CONTENT_KEY[kind]). */
  contentKey: string;
  /** Cursor inicial: seq devuelta por GET /api/state en la carga. */
  sinceSeq: number;
  /** Lamport local actual; solo se adopta una op ajena con lamport superior. */
  getLamport: () => number;
  /** true si NO hay edición local en vuelo (save === 'saved'); si no, gana el próximo autosave. */
  canAdopt: () => boolean;
  /** Op ajena de contenido adoptada (LWW): valor + lamport de la op ganadora del lote. */
  onRemoteContent: (value: string, lamport: number) => void;
  /** Ops ajenas de secuencia (insert/delete) del lote, en orden de llegada. */
  onRemoteOps?: (ops: OpRGA[]) => void;
  /** Hojas y presentaciones: cada `set` ajeno del contenido, en orden de llegada.
   *  Con él no se usa la regla LWW de `onRemoteContent` (lo decide ./fusion.ts). */
  onRemoteSet?: (op: SequencedOp) => void;
  /** El lote trae un `set` de contenido AUTORITATIVO: hay que anular la secuencia. */
  onLwwOverride?: () => void;
  /** Lista viva de colaboradores; se emite solo cuando cambia su composición. */
  onPeers: (peers: Peer[]) => void;
  /** Cursores efímeros de los peers (WS): se emite la lista completa al cambiar
   *  (upsert por sub + poda de los que se van). No se emite si el WS no abre. */
  onPeerCursors?: (cursors: PeerCursor[]) => void;
  /** Mi rol ha cambiado mientras tengo el documento abierto (el dueño concedió o
   *  quitó un permiso): es lo que quita la franja de solo lectura sin recargar. */
  onRole?: (role: string) => void;
  /** Solo para pruebas; sondeo de contenido, por defecto 4000 ms. */
  intervalMs?: number;
  /** Solo para pruebas; heartbeat de presencia, por defecto 10000 ms. */
  heartbeatMs?: number;
  /** Solo para pruebas; base del backoff de reconexión WS, por defecto 1000 ms. */
  wsBackoffBaseMs?: number;
  /** Solo para pruebas; throttle del envío de cursor, por defecto 80 ms. */
  cursorThrottleMs?: number;
}

export interface CollabHandle {
  /**
   * Avanza el cursor tras un autosave propio de `n` ops que acaban en `seq`
   * (evita re-leer lo recién enviado). Si entre el cursor y ese lote hay ops
   * ajenas que aún no han llegado, NO avanza: saltárselas era perderlas en el
   * sondeo y en el replay del WS. Sin `n`, avanza siempre (estado recién leído).
   */
  advance: (seq: number, n?: number) => void;
  /** Fuerza un tick inmediato del sondeo de fallback (no-op con el WS abierto). */
  poll: () => Promise<void>;
  /** Publica la posición local de cursor/selección (throttled ~80 ms + flanco de
   *  parada); no-op si el WS no está abierto (degradación sin romper). */
  sendCursor: (pos: CursorPos) => void;
  /** Detiene todos los canales (WS, sondeo, presencia) y libera timers. */
  stop: () => void;
}

/** Miembro presente devuelto por PUT /api/presence. */
interface PresenceMember {
  conn: string;
  sub: string;
}

/** Mensaje del servidor por el WS (services/docs/internal/collab/protocol.go). */
interface WsServerMessage {
  type: 'welcome' | 'ops' | 'ack' | 'presence' | 'role' | 'error';
  version?: number;
  code?: string;
  /** role: mi rol nuevo en el documento. */
  role?: string;
  ops?: SequencedOp[] | null;
  /** presence (roster de entrada/salida): subs presentes en la sala. */
  presence?: { sub?: string }[] | null;
  /** presence (difusión de "cursor"): {sub, name, cursor} de UN peer (carga opaca). */
  cursor?: unknown;
}

/** Arranca la colaboración (contenido + presencia). Llamar SOLO en cliente (useEffect). */
export function startCollab(options: CollabOptions): CollabHandle {
  let since = options.sinceSeq;
  let stopped = false;
  let inFlight = false;
  /** Huella de la última lista de peers emitida, para no re-renderizar sin cambios. */
  let lastPeerKey: string | null = null;

  /* --- Estado del canal WS --- */
  let ws: WebSocket | null = null;
  let wsIncompatible = false;
  /** true entre 'open' y 'close': el sondeo de fallback queda en pausa. */
  let wsUp = false;
  let wsAttempt = 0;
  let wsTimer: ReturnType<typeof setTimeout> | null = null;

  /* --- Cursores remotos (efímeros, solo WS) ---
   * Mapa por sub del último cursor de cada peer; se poda con el roster de
   * presencia (join/leave) y al caerse el WS. El envío local va throttled. */
  const peerCursors = new Map<string, PeerCursor>();
  let pendingCursor: CursorPos | null = null;
  let cursorTimer: ReturnType<typeof setTimeout> | null = null;
  let lastCursorAt = 0;

  const emitPeerCursors = (): void => options.onPeerCursors?.(Array.from(peerCursors.values()));

  /** Envía ya el último cursor pendiente por el WS (si sigue abierto). */
  const flushCursor = (): void => {
    if (cursorTimer !== null) {
      clearTimeout(cursorTimer);
      cursorTimer = null;
    }
    if (pendingCursor === null || ws === null || !wsUp) return;
    ws.send(JSON.stringify({ type: 'cursor', cursor: pendingCursor }));
    pendingCursor = null;
    lastCursorAt = Date.now();
  };

  /** Publica la posición local con throttle de flanco inicial + de parada. */
  const sendCursor = (pos: CursorPos): void => {
    pendingCursor = pos;
    if (!wsUp || ws === null) return; // sin WS: se descarta (degradación sin romper)
    const wait = (options.cursorThrottleMs ?? CURSOR_THROTTLE_MS) - (Date.now() - lastCursorAt);
    if (wait <= 0) {
      flushCursor(); // pasó el intervalo: envío inmediato
    } else if (cursorTimer === null) {
      // Coalesce: un solo envío de la última posición al vencer el intervalo.
      cursorTimer = setTimeout(() => {
        cursorTimer = null;
        flushCursor();
      }, wait);
    }
  };

  /** Aplica un mensaje 'presence': roster (poda) y/o cursor de un peer (upsert). */
  const applyPresence = (msg: WsServerMessage): void => {
    let changed = false;
    if (Array.isArray(msg.presence)) {
      // Roster autoritativo de subs presentes: poda los cursores de quien se fue.
      const present = new Set<string>();
      for (const p of msg.presence) if (typeof p.sub === 'string') present.add(p.sub);
      for (const id of peerCursors.keys())
        if (!present.has(id)) {
          peerCursors.delete(id);
          changed = true;
        }
    }
    if (msg.cursor !== undefined) {
      const pc = parsePeerCursor(msg.cursor); // valida sub + posición; color estable por sub
      if (pc !== null) {
        peerCursors.set(pc.id, pc);
        changed = true;
      }
    }
    if (changed) emitPeerCursors();
  };

  /** Descarta los cursores remotos (WS caído/parado): quedarían obsoletos. */
  const clearPeerCursors = (): void => {
    if (peerCursors.size === 0) return;
    peerCursors.clear();
    emitPeerCursors();
  };

  const emitPeers = (peers: Peer[]): void => {
    const key = peers
      .map((p) => p.sub)
      .sort()
      .join('\n');
    if (key === lastPeerKey) return;
    lastPeerKey = key;
    options.onPeers(peers);
  };

  /** Clave del registro que marca el `set` de contenido como autoritativo. */
  const marcaLww = claveLww(options.contentKey);

  /** Reparto de un lote de ops secuenciadas (push WS, replay o sondeo). */
  const applyOps = (ops: SequencedOp[]): void => {
    let winner: SequencedOp | null = null;
    let autoritativo = false;
    const secuencia: OpRGA[] = [];
    for (const so of ops) {
      if (so.seq > since) since = so.seq;
      if (so.actor === options.actor) continue; // op propia (eco del autosave)
      if (so.op.kind !== 'set') {
        const op = opDeSecuencia(so.op);
        if (op !== null) secuencia.push(op);
        continue;
      }
      if (so.op.key === marcaLww) {
        autoritativo = true;
        continue;
      }
      if (so.op.key !== options.contentKey) continue;
      if (options.onRemoteSet) options.onRemoteSet(so);
      else if (winner === null || so.op.lamport > winner.op.lamport) winner = so;
    }
    // El aviso va PRIMERO y sin condiciones: la secuencia hay que anularla
    // aunque el `set` que la sustituye no se pueda adoptar todavía.
    if (autoritativo) options.onLwwOverride?.();
    if (secuencia.length > 0) options.onRemoteOps?.(secuencia);
    if (winner !== null && winner.op.lamport > options.getLamport() && options.canAdopt()) {
      // El valor del registro LWW es `unknown` (CRDT genérico); el contenido siempre
      // es texto, pero estrechamos por si llega otra cosa (evita "[object Object]").
      const remote = winner.op.value;
      options.onRemoteContent(typeof remote === 'string' ? remote : '', winner.op.lamport);
    }
  };

  // --- Contenido, FALLBACK: sondeo de GET /api/ops mientras el WS no está abierto ---
  const poll = async (): Promise<void> => {
    if (stopped || inFlight || wsUp) return;
    // Solo con la pestaña visible (§4.4); el intervalo sigue vivo y retoma al volver.
    if (typeof document !== 'undefined' && document.visibilityState !== 'visible') return;
    inFlight = true;
    try {
      const res = await fetch(
        `/api/ops?id=${encodeURIComponent(options.documentId)}&since=${since}`,
        { headers: { accept: 'application/json' } },
      );
      if (res.status === 401) {
        // Sesión caducada: sondear cada 4 s solo haría ruido; el chip de
        // guardado ya informa del estado de sesión en el próximo autosave.
        stop();
        options.onPeers([]);
        return;
      }
      if (!res.ok) return; // fallo transitorio (5xx, proxy): reintenta el próximo tick
      const data = (await res.json()) as { ops?: SequencedOp[] | null };
      applyOps(data.ops ?? []);
    } catch {
      // Red caída: silencio y siguiente tick.
    } finally {
      inFlight = false;
    }
  };

  // --- Contenido, canal PRIMARIO: WS de colaboración (push) ---
  const scheduleReconnect = (): void => {
    if (stopped || wsTimer !== null) return;
    wsAttempt += 1;
    const delay = Math.min((options.wsBackoffBaseMs ?? 1000) * 2 ** wsAttempt, WS_BACKOFF_MAX_MS);
    wsTimer = setTimeout(() => {
      wsTimer = null;
      void connectWs();
    }, delay);
  };

  const connectWs = async (): Promise<void> => {
    if (stopped) return;
    let url = '';
    try {
      const res = await fetch(`/api/collab?id=${encodeURIComponent(options.documentId)}`, {
        headers: { accept: 'application/json' },
      });
      if (res.status === 401) {
        // Sesión caducada: mismo tratamiento que el sondeo.
        stop();
        options.onPeers([]);
        return;
      }
      if (res.ok) url = ((await res.json()) as { url?: string }).url ?? '';
    } catch {
      // Red caída: backoff abajo; el sondeo sigue cubriendo mientras tanto.
    }
    if (stopped) return; // pudo pararse durante el fetch
    if (url === '') {
      scheduleReconnect();
      return;
    }

    const socket = new WebSocket(url);
    ws = socket;
    socket.addEventListener('open', () => {
      if (stopped) return;
      wsAttempt = 0;
      wsUp = true;
      // Replay del hueco: el servidor reenvía las ops con seq > last_seq.
      socket.send(
        JSON.stringify({ type: 'hello', version: DOC_PROTOCOL_VERSION, last_seq: since }),
      );
    });
    socket.addEventListener('message', (ev) => {
      if (stopped || wsIncompatible || ws !== socket || typeof ev.data !== 'string') return;
      let msg: WsServerMessage;
      try {
        msg = JSON.parse(ev.data) as WsServerMessage;
      } catch {
        return; // frame no-JSON: se ignora
      }
      if (
        (msg.version !== undefined && msg.version !== 0 && msg.version !== DOC_PROTOCOL_VERSION) ||
        msg.code === 'protocol_version'
      ) {
        // No aplicar operaciones de otro contrato ni reconectar en bucle.
        // El sondeo REST conserva el contrato existente y cubre la lectura.
        wsIncompatible = true;
        socket.close();
        return;
      }
      if (msg.type === 'ops') applyOps(msg.ops ?? []);
      else if (msg.type === 'presence')
        applyPresence(msg); // roster + cursores efímeros
      else if (msg.type === 'role' && msg.role) options.onRole?.(msg.role);
      // welcome/ack/error: sin efecto aquí — el autosave confirma por REST y la
      // presencia (avatares) viaja por el heartbeat HTTP (ver cabecera).
    });
    socket.addEventListener('close', () => {
      wsUp = false;
      if (ws === socket) ws = null;
      clearPeerCursors(); // los cursores remotos quedarían obsoletos hasta reconectar
      void poll(); // cubrir el hueco ya, sin esperar al próximo tick del fallback
      if (!wsIncompatible) scheduleReconnect();
    });
    socket.addEventListener('error', () => socket.close());
  };

  // --- Presencia: heartbeat por conexión a PUT /api/presence ---
  const heartbeat = async (): Promise<void> => {
    if (stopped) return;
    try {
      const res = await fetch(`/api/presence?id=${encodeURIComponent(options.documentId)}`, {
        method: 'PUT',
        headers: { 'content-type': 'application/json' },
        body: JSON.stringify({ conn: options.actor }),
      });
      if (res.status === 401) {
        stop();
        options.onPeers([]);
        return;
      }
      if (!res.ok) return; // fallo transitorio: reintenta el próximo tick
      const data = (await res.json()) as { members?: PresenceMember[] | null };
      const now = Date.now();
      const seen = new Set<string>();
      const peers: Peer[] = [];
      for (const m of data.members ?? []) {
        if (m.conn === options.actor) continue; // esta pestaña no se muestra a sí misma
        if (seen.has(m.sub)) continue; // dedup por usuario (varias pestañas ajenas → 1 avatar)
        seen.add(m.sub);
        peers.push({ sub: m.sub, lastSeen: now });
      }
      emitPeers(peers);
    } catch {
      // Red caída: silencio y siguiente tick.
    }
  };

  const pollTimer = setInterval(() => {
    void poll();
  }, options.intervalMs ?? POLL_MS);
  const beatTimer = setInterval(() => {
    void heartbeat();
  }, options.heartbeatMs ?? HEARTBEAT_MS);

  const stop = (): void => {
    stopped = true;
    clearInterval(pollTimer);
    clearInterval(beatTimer);
    if (wsTimer !== null) {
      clearTimeout(wsTimer);
      wsTimer = null;
    }
    if (cursorTimer !== null) {
      clearTimeout(cursorTimer);
      cursorTimer = null;
    }
    wsUp = false;
    ws?.close();
    ws = null;
  };

  // Registrar presencia ya al abrir (no esperar al primer intervalo): el avatar
  // aparece en cuanto la otra pestaña también ha hecho su primer heartbeat.
  void heartbeat();
  // Canal primario: intentar el WS ya; hasta su 'open' (o si nunca abre) manda el sondeo.
  void connectWs();

  const advance = (seq: number, n?: number): void => {
    if (n === undefined || seq - n <= since) since = Math.max(since, seq);
  };

  return { advance, poll, sendCursor, stop };
}
