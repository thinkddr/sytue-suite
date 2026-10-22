/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * useDocument — capa de datos del editor Sytue Docs (BLUEPRINT.md §4.3/§4.4,
 * sobre el contrato verificado del auditor 2026-07-03).
 *
 * Mecanismo de persistencia: el contenido vive en UN registro LWW por documento
 * (clave CONTENT_KEY[kind]) y guardar emite una op
 * `{"kind":"set", dot, key, value, lamport}` vía `POST /api/ops?id=` (proxy
 * BFF same-origin: el servicio Go no expone CORS). Autosave con debounce de
 * 1500 ms; rename optimista vía `POST /api/rename` (proxy del PATCH del
 * servicio); estados 'saved' | 'saving' | 'dirty' | 'error' con reintento.
 *
 * Ese registro sigue siendo lo que lee el backend (export, PDF, import,
 * búsqueda), pero ya no es lo único que se manda: en documentos de texto el
 * mismo lote lleva ADEMÁS las ops por carácter de la secuencia RGA
 * (./coedicion.ts), que es lo que permite que dos personas escriban a la vez
 * sin que el último guardado borre al otro. Al recibir manda la fusión, no el
 * `set` ajeno — salvo que venga marcado como autoritativo. Las reglas y sus
 * límites están en la cabecera de ./coedicion.ts; el transporte, en ./collab.ts.
 *
 * Hojas y presentaciones no tienen secuencia: su guardado es condicional
 * (`base_seq`, 409 si otro escribió entretanto) y lo que se manda es la fusión a
 * tres bandas de lo propio con lo último del servidor (./fusion.ts).
 */
import { useCallback, useEffect, useMemo, useRef, useState } from 'preact/hooks';
import { descargarExport } from '../exportName';
import { useToast } from '@sytue/ui';
import { currentConnectivity } from '@sytue/offline';
import { useT } from '../context';
import { puedeEscribir } from '@sytue/api-clients';
import type { DocKind, DocumentMeta, DocumentRole } from '@sytue/api-clients';
import { CONTENT_KEY, type TemaPrevia } from '../types';
import { NOMBRE, aplicarMarca, productoDeKind } from '../../lib/producto';
import type {
  CommentItem,
  CursorDots,
  CursorPos,
  DocStateResponse,
  ExportFormat,
  Peer,
  PeerCursor,
  SaveState,
  SequencedOp,
  VersionItem,
} from '../types';
import { startCollab, type CollabHandle } from './collab';
import { fijarPropietario, fijarYo, olvidarProteccion, sobreProteccion } from './proteccion';
import { cabeEnUnLote, claveLww, CoEdicion } from './coedicion';
import { CoedicionModelo, type Registro } from './fusion';
import type { OpRGA } from './rga';
import {
  cacheLocalContent,
  cacheLocalTitle,
  cacheServerDocument,
  markContentSynced,
  markTitleSynced,
  readOfflineDocument,
} from './offline';

/** Debounce del autosave (§4.4). */
const AUTOSAVE_MS = 1500;
/** Reintento tras fallo de red/5xx (§4.4). */
const RETRY_MS = 5000;
/** Debounce del rename optimista (§4.4). */
const RENAME_MS = 800;
/**
 * Vueltas de «409 → releer → fusionar → reintentar» de un mismo guardado. Con
 * varias personas guardando la misma hoja a la vez puede tocar más de una; si se
 * agotan, el reintento programado (RETRY_MS) sigue intentándolo.
 */
const MAX_VUELTAS_409 = 4;

const JSON_HEADERS = { 'content-type': 'application/json' } as const;

function requireOnline(): void {
  if (!navigator.onLine) throw new Error('offline');
}

/** Recuento de palabras del blueprint (solo kind document; resto 0). */
function countWords(html: string, kind: DocKind): number {
  if (kind !== 'document') return 0;
  return (
    html
      // Datos embebidos (p. ej. el origen de combinación en <script type=json>) no
      // son texto del documento: su contenido no cuenta como palabras.
      .replace(/<script[\s\S]*?<\/script>/gi, ' ')
      .replace(/<[^>]+>/g, ' ')
      .trim()
      .split(/\s+/)
      .filter(Boolean).length
  );
}

/** El registro de contenido del estado del servidor, como lo usa CoedicionModelo. */
function registroDe(st: DocStateResponse, key: string): Registro {
  const r = st.state.registers[key];
  return {
    valor: typeof r?.value === 'string' ? r.value : '',
    lamport: r?.lamport ?? 0,
    dot: r?.dot ?? null,
  };
}

/* Firma del blueprint §4.3, tal cual. */
export interface UseDocumentResult {
  status: 'loading' | 'ready' | 'error';
  doc: DocumentMeta | null;
  /** Origen del documento («drive:<uuid>») o '' si nació en Sytue Docs. */
  sourceRef: string;
  /** Cursores anclados a la identidad de los caracteres; solo en documentos de texto. */
  cursorDots: CursorDots;
  /** Mi papel en el documento, tal y como lo dice el servicio. */
  role: DocumentRole;
  /**
   * ¿Hay que impedir escribir? `true` también MIENTRAS CARGA: pintar un lienzo
   * editable y quitárselo medio segundo después es peor que esperar a saberlo.
   */
  soloLectura: boolean;
  /** Ya pedí editarlo y el dueño aún no ha contestado (la franja lo dice). */
  edicionPedida: boolean;
  savingToDrive: boolean;
  /** Devuelve lo editado al fichero de Drive del que salió (nueva versión allí). */
  saveToDrive: () => Promise<void>;
  title: string;
  setTitle: (next: string) => void;
  save: SaveState;
  connection: 'online' | 'offline';
  content: string;
  contentEpoch: number;
  onContentInput: (next: string) => void;
  /** Envía ya el autosave pendiente (pagehide → sendBeacon). */
  flush: () => void;
  /** Adelanta el autosave en cola y lo espera por el camino normal (Ctrl/Cmd+S). */
  saveNow: () => void;
  words: number;
  peers: Peer[];
  peerCursors: PeerCursor[];
  sendCursor: (pos: CursorPos) => void;
  comments: CommentItem[];
  addComment: (body: string, anchor?: unknown) => Promise<void>;
  resolveComment: (id: string, resolved: boolean) => Promise<void>;
  removeComment: (id: string) => Promise<void>;
  versions: VersionItem[];
  loadVersions: () => Promise<void>;
  createVersion: (label?: string) => Promise<void>;
  restoreVersion: (versionId: string) => Promise<void>;
  /** Aplica un tema (catálogo común o «marca») sin tocar el contenido; ver `applyTheme`. */
  applyTheme: (themeId: string, brandVersion?: number) => Promise<void>;
  /** Diff de lo que cambiaría un tema, sin escribir; null si no se pudo calcular. */
  previewTheme: (themeId: string, brandVersion?: number) => Promise<TemaPrevia | null>;
  exportAs: (format: ExportFormat, plantilla?: boolean) => Promise<void>;
}

export function useDocument(documentId: string, userId: string): UseDocumentResult {
  const toast = useToast();
  const t = useT();
  const eid = encodeURIComponent(documentId);

  const [status, setStatus] = useState<'loading' | 'ready' | 'error'>('loading');
  const [doc, setDoc] = useState<DocumentMeta | null>(null);
  // Fichero de Drive del que salió el documento («drive:<uuid>»), vacío si nació
  // aquí. Decide si se ofrece devolver los cambios a su fichero.
  const [sourceRef, setSourceRef] = useState('');
  /**
   * Mi papel en este documento, que lo dice el SERVICIO.
   *
   * Arranca en 'none' y no en 'editor': hasta que la carga conteste no se sabe si
   * se puede escribir, y suponer que sí es justo el fallo que esto cierra —dejar
   * teclear a un lector y contárselo al guardar, con el trabajo ya hecho—.
   */
  const [role, setRole] = useState<DocumentRole>('none');
  // El rol también en un ref: `onContentInput` va en un `useCallback` con
  // dependencias acotadas y leer el estado desde ahí daría el valor de cuando se
  // creó la función, no el de ahora — con lo que la barrera se abriría sola.
  const roleRef = useRef<DocumentRole>('none');
  const [edicionPedida, setEdicionPedida] = useState(false);
  const [savingToDrive, setSavingToDrive] = useState(false);
  const [title, setTitleState] = useState('');
  const [save, setSaveState] = useState<SaveState>('saved');
  // 'online' de salida, y el estado real en el efecto de más abajo: este
  // inicializador corre también en el servidor, que no puede saber si hay red, y
  // discrepar en el primer render hace que Preact hidrate emparejando por
  // posición. Ver packages/offline/src/preact.ts. marker-b2
  const [connection, setConnection] = useState<'online' | 'offline'>('online');
  const [content, setContent] = useState('');
  const [contentEpoch, setContentEpoch] = useState(0);
  const [words, setWords] = useState(0);
  const [peers, setPeers] = useState<Peer[]>([]);
  const [peerCursors, setPeerCursors] = useState<PeerCursor[]>([]);
  const [comments, setComments] = useState<CommentItem[]>([]);
  const [versions, setVersions] = useState<VersionItem[]>([]);

  /* --- Identidad CRDT y contadores (refs: no re-renderizan) --- */
  const actorRef = useRef<string>(crypto.randomUUID());
  const counterRef = useRef(0);
  const lamportRef = useRef(0);
  const lastSeqRef = useRef(0);
  const kindRef = useRef<DocKind>('document');
  const keyRef = useRef(''); // '' = aún sin cargar: no se puede guardar
  /** HTML/JSON pendiente de guardar; null = nada en cola (¡ref!, §4.4). */
  const pendingRef = useRef<string | null>(null);
  const saveTimerRef = useRef<number | undefined>(undefined);
  const retryTimerRef = useRef<number | undefined>(undefined);
  const titleTimerRef = useRef<number | undefined>(undefined);
  const confirmedTitleRef = useRef('');
  const pendingTitleRef = useRef<string | null>(null);
  const metaRef = useRef<DocumentMeta | null>(null);
  const collabRef = useRef<CollabHandle | null>(null);
  /** Espejo de `save` para decisiones síncronas (adopción LWW en collab). */
  const saveRef = useRef<SaveState>('saved');
  /** Secuencia RGA del documento; null en hojas y presentaciones (solo LWW). */
  const coRef = useRef<CoEdicion | null>(null);
  /** Ops por carácter emitidas y aún sin 2xx: se reenvían tal cual (son idempotentes). */
  const opsPendientesRef = useRef<OpRGA[]>([]);
  /** Nº de colaboradores presentes; sembrar la secuencia solo se hace a solas. */
  const peersRef = useRef(0);
  /** Co-edición de hojas y presentaciones (./fusion.ts); null en documentos de texto. */
  const modeloRef = useRef<CoedicionModelo | null>(null);
  /** El parche en vuelo (hojas y presentaciones): su diff y su op_id, para reenviarlo igual. */
  const parcheRef = useRef<{ huella: string; opId: string } | null>(null);
  /** El servidor rechazó un parche (400): el resto de la sesión guarda el registro entero. */
  const sinParchesRef = useRef(false);

  const setSave = useCallback((next: SaveState): void => {
    saveRef.current = next;
    setSaveState(next);
  }, []);

  /** Publica la posición local de cursor/selección (referencia estable: el
   *  transporte agrupa por tiempo y es no-op si el WS no está abierto). */
  const sendCursor = useCallback((pos: CursorPos): void => {
    collabRef.current?.sendCursor(pos);
  }, []);

  /** Cursores anclados a los caracteres: estables para el lienzo; leen la secuencia viva. */
  const cursorDots = useMemo<CursorDots>(
    () => ({
      aPuntos: (n, off) => coRef.current?.puntosDeOffsets(n, off) ?? null,
      aOffsets: (n, dots) => coRef.current?.offsetsDePuntos(n, dots) ?? null,
    }),
    [],
  );

  const fetchState = useCallback(async (): Promise<DocStateResponse> => {
    const res = await fetch(`/api/state?id=${eid}`);
    if (!res.ok) throw new Error(`state ${res.status}`);
    return (await res.json()) as DocStateResponse;
  }, [eid]);

  /* --- Autosave (§4.4): ops por carácter + registro LWW, counter/lamport monótonos --- */

  /**
   * Cuerpo de POST /api/ops para `value`. Lleva SIEMPRE el `set` del registro
   * —es lo que lee el backend— y, en documentos con la secuencia en sincronía,
   * las ops por carácter que la llevan hasta `value`.
   *
   * Las ops se acumulan hasta recibir un 2xx: reintentar re-diffando daría un
   * diff vacío (la secuencia ya avanzó) y la edición se perdería en silencio.
   * Reenviar las mismas es seguro: sus dots son estables y el CRDT es idempotente.
   *
   * En hojas y presentaciones el `set` lleva la fusión de `value` con lo último
   * del servidor y el lote, `base_seq`; `confirmar` se llama con la seq del 2xx.
   */
  const buildOpsBody = useCallback(
    (
      value: string,
    ): {
      cuerpo: string;
      confirmar: (seq: number, ops?: unknown[] | null) => void;
      parche: boolean;
    } => {
      const co = coRef.current;
      // Un lote es AUTORITATIVO si su emisor no tenía la secuencia en sincronía
      // (documento heredado, siembra o renuncia por tamaño): los pares anulan la
      // suya en vez de fusionar sobre un estado que ya no existe.
      let autoritativo = co !== null && !co.sincronizada;
      if (co !== null) {
        const nuevas = co.opsLocales(value, lamportRef.current, peersRef.current === 0);
        lamportRef.current += nuevas.length; // opsLocales reparte lamportBase+1…+n
        opsPendientesRef.current.push(...nuevas);
      }
      // Monotonicidad: counter y lamport se incrementan en CADA intento (§4.4). El
      // counter es único por actor, compartido con los dots de la secuencia.
      counterRef.current = Math.max(counterRef.current, co?.contador ?? 0) + 1;
      co?.fijarContador(counterRef.current);
      lamportRef.current += 1;
      const dot = { a: actorRef.current, c: counterRef.current };
      const lamport = lamportRef.current;
      const modelo = modeloRef.current;
      // Hojas y presentaciones: lo que cambió (una celda, una caja) y no el modelo
      // entero. El servidor lo aplica sobre lo último que tiene, en orden de llegada,
      // sin 409. Si no se deja decir así, o el servidor lo rechazó antes, va el registro.
      const cambios = modelo === null || sinParchesRef.current ? null : modelo.cambios(value);
      if (modelo !== null && cambios !== null && cambios.length > 0) {
        // Mismo diff que el intento anterior sin confirmar = mismo parche (el servidor no
        // lo repite); un diff distinto es otro parche, aunque la base sea la misma.
        const huella = JSON.stringify(cambios);
        const previo = parcheRef.current;
        const opId = previo?.huella === huella ? previo.opId : crypto.randomUUID();
        parcheRef.current = { huella, opId };
        const op = {
          kind: 'patch',
          key: keyRef.current,
          op_id: opId,
          base: modelo.baseSeq,
          cambios,
        };
        const proteccion = sobreProteccion();
        const confirmar = (seq: number, ops?: unknown[] | null): void => {
          parcheRef.current = null;
          const mio = ops
            ?.map((o) => (o as { op?: Record<string, unknown> }).op)
            .find((o) => o?.op_id === opId);
          if (mio === undefined || typeof mio.value !== 'string') {
            modelo.confirmarSinOp(value, seq);
            return;
          }
          const lam = Number(mio.lamport);
          lamportRef.current = Math.max(lamportRef.current, lam);
          modelo.confirmarParche(value, mio.value, seq, lam, mio.dot as { a: string; c: number });
        };
        return {
          cuerpo: JSON.stringify({ ops: [op], ...(proteccion ? { proteccion } : {}) }),
          confirmar,
          parche: true,
        };
      }
      const enviado = modelo === null ? value : modelo.aEnviar(value);
      const confirmar = (seq: number): void => modelo?.confirmar(value, enviado, seq, lamport, dot);
      parcheRef.current = null;
      const set = { kind: 'set', dot, key: keyRef.current, value: enviado, lamport };
      const marca = { kind: 'set', dot, key: claveLww(keyRef.current), value: lamport, lamport };
      const base = modelo === null ? {} : { base_seq: modelo.baseSeq };
      // Contraseñas de protección de hoja/libro: fuera de las operaciones, para que no
      // entren en el registro que se sirve a todo lector (state/proteccion.ts).
      const proteccion = sobreProteccion();
      const cuerpo = (marcado: boolean, chars: OpRGA[]): string =>
        JSON.stringify({
          ops: marcado ? [...chars, marca, set] : [...chars, set],
          ...base,
          ...(proteccion ? { proteccion } : {}),
        });

      const body = cuerpo(autoritativo, opsPendientesRef.current);
      if (cabeEnUnLote(body)) return { cuerpo: body, confirmar, parche: false };
      // Un cambio enorme (pegar un artículo, reformatear el documento entero) da
      // una op por carácter y no cabe en el cuerpo que acepta el servicio. Se
      // renuncia al canal por carácter en esta sesión y manda el registro, como
      // antes; al recargar, la secuencia se vuelve a evaluar. Trocear el envío
      // sería peor: una fila y una seq por carácter en el servidor.
      co?.anular();
      opsPendientesRef.current = [];
      autoritativo = co !== null;
      return { cuerpo: cuerpo(autoritativo, []), confirmar, parche: false };
    },
    [],
  );

  /**
   * Hojas y presentaciones: si el servidor tiene algo que el editor aún no
   * enseña y no hay cambios propios pendientes, se enseña (recarga suave). Con
   * cambios pendientes no se toca el editor: el próximo guardado lo fusiona.
   */
  const adoptarModelo = useCallback((): void => {
    const modelo = modeloRef.current;
    if (modelo === null || pendingRef.current !== null) return;
    const value = modelo.adoptar();
    if (value === null) return;
    setContent(value);
    setContentEpoch((e) => e + 1);
    const meta = metaRef.current;
    if (meta !== null) {
      void cacheServerDocument(userId, meta, value, lamportRef.current, lastSeqRef.current).catch(
        () => undefined,
      );
    }
  }, [userId]);

  /**
   * El servidor rechazó un cambio por la protección de hoja/libro: el modelo local
   * ya no es el del documento. Se adopta el estado del servidor (misma recarga suave
   * que tras restaurar una versión), se olvida la contraseña tecleada y se explica.
   */
  const adoptarServidor = useCallback(
    async (codigo: 'protected' | 'protection_password'): Promise<void> => {
      olvidarProteccion();
      clearTimeout(saveTimerRef.current);
      pendingRef.current = null;
      opsPendientesRef.current = [];
      try {
        const st = await fetchState();
        lamportRef.current = Math.max(lamportRef.current, st.state.lamport);
        lastSeqRef.current = Math.max(lastSeqRef.current, st.seq);
        collabRef.current?.advance(st.seq);
        const raw = st.state.registers[keyRef.current]?.value;
        const value = typeof raw === 'string' ? raw : '';
        // Hojas: sin secuencia por carácter. Documentos (secciones protegidas): la
        // secuencia se reconstruye desde el estado del servidor, como al restaurar.
        coRef.current =
          kindRef.current === 'document'
            ? new CoEdicion(actorRef.current, st.state.elements, value)
            : null;
        // El modelo de fusión (hojas) vuelve a partir de lo que tiene el servidor: lo
        // rechazado no se escribió, así que ni la base ni el ancestro de antes valen ya.
        const reg = registroDe(st, keyRef.current);
        if (reg.dot !== null) modeloRef.current?.recibir(reg.valor, st.seq, reg.lamport, reg.dot);
        modeloRef.current?.adoptar();
        setContent(value);
        setContentEpoch((e) => e + 1);
        setWords(countWords(value, kindRef.current));
        const meta = metaRef.current;
        if (meta !== null) {
          void markContentSynced(userId, meta, value, lamportRef.current, lastSeqRef.current).catch(
            () => undefined,
          );
        }
        setSave('saved');
      } catch {
        setSave('error');
      }
      if (kindRef.current === 'document') {
        toast.notify({
          tone: 'warning',
          title: t('doc.msg.sectionProtected'),
          description: t('doc.msg.sectionProtectedDesc'),
        });
        return;
      }
      toast.notify({
        tone: 'warning',
        title: t(
          codigo !== 'protected'
            ? 'sheet.msg.wrongPassword'
            : kindRef.current === 'presentation'
              ? 'slides.msg.protected'
              : 'sheet.msg.protectedSheet',
        ),
        description: t('sheet.msg.protectionDenied'),
      });
    },
    [fetchState, setSave, toast, t, userId],
  );

  const doSave = useCallback(
    async (vueltas = 0): Promise<void> => {
      const value = pendingRef.current;
      if (value === null || keyRef.current === '') return;
      if (!navigator.onLine) {
        setConnection('offline');
        setSave('dirty');
        return;
      }
      setSave('saving');
      try {
        const lote = buildOpsBody(value);
        const res = await fetch(`/api/ops?id=${eid}`, {
          method: 'POST',
          headers: JSON_HEADERS,
          body: lote.cuerpo,
        });
        if (res.ok) {
          setConnection('online');
          // Confirmadas: no reenviarlas. Va antes del `pendingRef === value` de
          // abajo porque el servidor ya las tiene aunque se haya seguido tecleando.
          opsPendientesRef.current = [];
          olvidarProteccion(); // el servidor ya tiene el hash; la contraseña no se guarda más
          const data = (await res.json()) as { seq?: number; ops?: unknown[] | null };
          if (typeof data.seq === 'number') {
            lastSeqRef.current = Math.max(lastSeqRef.current, data.seq);
            collabRef.current?.advance(data.seq, data.ops?.length ?? 0);
            lote.confirmar(data.seq, data.ops);
          }
          if (pendingRef.current === value) {
            pendingRef.current = null;
            setSave(pendingTitleRef.current === null ? 'saved' : 'dirty');
            setWords(countWords(value, kindRef.current));
            const meta = metaRef.current;
            if (meta !== null) {
              void markContentSynced(
                userId,
                meta,
                value,
                lamportRef.current,
                lastSeqRef.current,
              ).catch(() => undefined);
            }
            // Lo ajeno que entró en la fusión pasa al editor (y a la copia offline,
            // después de marcarla limpia: el servidor tiene la fusión, no `value`).
            adoptarModelo();
          }
          // Si se tecleó durante el vuelo, sigue 'dirty' y el debounce ya está rearmado.
          return;
        }
        if (res.status === 401) {
          // Sesión caducada: sin reintento (§4.4); recargar la página re-autentica.
          setSave('error');
          return;
        }
        if (res.status === 403) {
          // La protección de hoja/libro la cumple el servidor: si rechazó el cambio, lo
          // que el editor enseña no es lo que hay. Se adopta lo del servidor y se dice por qué.
          const codigo = await res
            .json()
            .then((b: { error?: { code?: string } }) => b.error?.code)
            .catch(() => undefined);
          if (codigo === 'protected' || codigo === 'protection_password') {
            await adoptarServidor(codigo);
            return;
          }
        }
        if (res.status === 400 && lote.parche) {
          // Un parche que el servidor no entiende (otra versión, contenido que no es el que
          // creíamos): no se insiste; se guarda el registro entero, que es lo de siempre.
          sinParchesRef.current = true;
          return doSave(vueltas);
        }
        if (res.status >= 400 && res.status < 500) {
          const conflicto = res.status === 409;
          if (vueltas >= (conflicto ? MAX_VUELTAS_409 : 1)) {
            // 409 que no cede: al reintento programado de abajo.
            if (conflicto) throw new Error('ops 409');
            // 4xx persistente (p. ej. rol sin permiso de escritura): no reintentar en bucle.
            setSave('error');
            return;
          }
          // p. ej. 409 seq obsoleta: re-GET state, adoptar contadores y reintentar (§4.4).
          // En hojas y presentaciones el 409 es «otro guardó desde tu base»: lo
          // releído entra en la fusión del reintento.
          const st = await fetchState();
          lamportRef.current = Math.max(lamportRef.current, st.state.lamport);
          lastSeqRef.current = Math.max(lastSeqRef.current, st.seq);
          const reg = registroDe(st, keyRef.current);
          if (reg.dot !== null) modeloRef.current?.recibir(reg.valor, st.seq, reg.lamport, reg.dot);
          return doSave(vueltas + 1);
        }
        throw new Error(`ops ${res.status}`); // 5xx → reintento programado
      } catch {
        if (!navigator.onLine) {
          setConnection('offline');
          setSave('dirty');
          return;
        }
        setSave('error');
        clearTimeout(retryTimerRef.current);
        retryTimerRef.current = window.setTimeout(() => {
          void doSave();
        }, RETRY_MS);
      }
    },
    [eid, userId, buildOpsBody, fetchState, setSave, adoptarModelo, adoptarServidor],
  );

  const onContentInput = useCallback(
    (next: string): void => {
      // Segunda mitad de la barrera de solo lectura (la primera es `exec` del
      // contexto): aquí no se persiste nada. Si un cambio colara por otra vía —un
      // pegado, una extensión del navegador—, llegar hasta este punto guardaría el
      // trabajo de alguien que no tiene permiso para escribirlo.
      if (!puedeEscribir(roleRef.current)) return;
      pendingRef.current = next;
      setSave('dirty');
      const meta = metaRef.current;
      if (meta !== null) {
        void cacheLocalContent(userId, meta, next, lamportRef.current, lastSeqRef.current).catch(
          () => undefined,
        );
      }
      // Contador en vivo al teclear (no esperar al autosave debounced de 1,5s).
      setWords(countWords(next, kindRef.current));
      clearTimeout(saveTimerRef.current);
      saveTimerRef.current = window.setTimeout(() => {
        void doSave();
      }, AUTOSAVE_MS);
    },
    // `userId` entra en las dependencias: es la clave con la que se cachea el
    // contenido offline, y un callback memoizado con el usuario anterior habría
    // escrito el borrador en la caché de OTRA cuenta al cambiar de sesión.
    [doSave, setSave, userId],
  );

  /**
   * Vuelca al editor las ops ajenas recibidas mientras se escribía. Solo con el
   * documento guardado: si hubiera una edición local en vuelo, el editor y la
   * secuencia dejarían de coincidir y el siguiente diff local borraría lo que
   * acaba de llegar del otro. Lo pendiente espera al próximo 'saved'.
   */
  const fusionarRemoto = useCallback((): void => {
    const co = coRef.current;
    if (co === null || saveRef.current !== 'saved') return;
    const fusion = co.fusionar();
    if (fusion === null) return;
    setContent(fusion);
    setContentEpoch((e) => e + 1);
    setWords(countWords(fusion, kindRef.current));
    // El registro LWW todavía tiene la vista del emisor, no la fusión; el
    // backend lee de ahí, así que se reescribe por el camino normal del autosave.
    pendingRef.current = fusion;
    setSave('dirty');
    const meta = metaRef.current;
    if (meta !== null) {
      void cacheLocalContent(userId, meta, fusion, lamportRef.current, lastSeqRef.current).catch(
        () => undefined,
      );
    }
    clearTimeout(saveTimerRef.current);
    saveTimerRef.current = window.setTimeout(() => {
      void doSave();
    }, AUTOSAVE_MS);
  }, [doSave, setSave, userId]);

  // Al quedar guardado se vuelca lo que llegó mientras se escribía (y, tras la
  // propia fusión, no queda nada pendiente: no hay bucle).
  useEffect(() => {
    if (save === 'saved') fusionarRemoto();
  }, [save, fusionarRemoto]);

  /**
   * Guarda YA lo que hubiera en cola, por el camino normal del autosave (no por
   * sendBeacon como `flush`): así el chip de la cabecera pasa por «Guardando…» y
   * termina en «Guardado», que es la respuesta que espera quien pulsa Ctrl/Cmd+S.
   * Sin nada pendiente es un no-op (doSave sale con `value === null`).
   */
  const saveNow = useCallback((): void => {
    clearTimeout(saveTimerRef.current);
    void doSave();
  }, [doSave]);

  const flush = useCallback((): void => {
    const value = pendingRef.current;
    if (value === null || keyRef.current === '') return;
    clearTimeout(saveTimerRef.current);
    // sendBeacon sobrevive al pagehide y viaja con la cookie same-origin (§4.4).
    const meta = metaRef.current;
    if (meta !== null) {
      void cacheLocalContent(userId, meta, value, lamportRef.current, lastSeqRef.current).catch(
        () => undefined,
      );
    }
    // Offline: no fingir que sendBeacon guardó nada. Online se envía como
    // best-effort, pero el borrador permanece en IndexedDB hasta recibir un 2xx.
    if (navigator.onLine) {
      navigator.sendBeacon(
        `/api/ops?id=${eid}`,
        new Blob([buildOpsBody(value).cuerpo], { type: 'application/json' }),
      );
    }
  }, [eid, userId, buildOpsBody]);

  useEffect(() => {
    const onPageHide = (): void => flush();
    window.addEventListener('pagehide', onPageHide);
    return () => window.removeEventListener('pagehide', onPageHide);
  }, [flush]);

  /* --- Título: optimista + debounce 800 ms; error → revertir + toast (§4.4) --- */

  const doRename = useCallback(async (): Promise<void> => {
    const value = pendingTitleRef.current;
    const meta = metaRef.current;
    if (value === null || meta === null) return;
    if (!navigator.onLine) {
      setConnection('offline');
      setSave('dirty');
      return;
    }
    try {
      const res = await fetch('/api/rename', {
        method: 'POST',
        headers: JSON_HEADERS,
        body: JSON.stringify({ id: documentId, title: value }),
      });
      if (!res.ok) throw new Error(`rename ${res.status}`);
      setConnection('online');
      if (pendingTitleRef.current === value) {
        pendingTitleRef.current = null;
        confirmedTitleRef.current = value;
        metaRef.current = { ...meta, title: value };
        void markTitleSynced(userId, documentId, value).catch(() => undefined);
        if (pendingRef.current === null) setSave('saved');
      }
    } catch {
      // El título optimista también es parte del borrador: se conserva y se
      // reintentará en el evento `online`, sin revertir lo que ve el usuario.
      void cacheLocalTitle(userId, meta, value).catch(() => undefined);
      setSave('dirty');
      if (!navigator.onLine) setConnection('offline');
      else {
        clearTimeout(titleTimerRef.current);
        titleTimerRef.current = window.setTimeout(() => void doRename(), RETRY_MS);
      }
    }
  }, [documentId, userId, setSave]);

  const setTitle = useCallback(
    (next: string): void => {
      const value = next.trim();
      if (value === '') return;
      setTitleState(next);
      pendingTitleRef.current = value;
      setSave('dirty');
      const meta = metaRef.current;
      if (meta !== null) void cacheLocalTitle(userId, meta, value).catch(() => undefined);
      clearTimeout(titleTimerRef.current);
      titleTimerRef.current = window.setTimeout(() => void doRename(), RENAME_MS);
    },
    [doRename, userId, setSave],
  );

  useEffect(() => {
    const producto = productoDeKind(metaRef.current?.kind);
    if (title !== '') document.title = `${title} — ${NOMBRE[producto]}`;
    aplicarMarca(producto);
  }, [title]);

  useEffect(() => {
    // Al montar hay que MIRAR el estado, no solo esperar eventos: quien abre el
    // editor ya sin red no recibe ningún `offline` —ya estaba— y el indicador se
    // quedaba en «conectado» hasta que fallara el primer guardado.
    setConnection(currentConnectivity());
    const onOnline = (): void => {
      setConnection('online');
      void doSave();
      void doRename();
    };
    const onOffline = (): void => setConnection('offline');
    window.addEventListener('online', onOnline);
    window.addEventListener('offline', onOffline);
    return () => {
      window.removeEventListener('online', onOnline);
      window.removeEventListener('offline', onOffline);
    };
  }, [doSave, doRename]);

  /* --- Comentarios (proxy BFF /api/comments; lanzan en fallo: el caller pone el toast) --- */

  const refreshComments = useCallback(async (): Promise<void> => {
    const res = await fetch(`/api/comments?id=${eid}`);
    if (!res.ok) return; // lista informativa: no tumba el editor
    const data = (await res.json()) as { comments?: CommentItem[] | null };
    setComments(data.comments ?? []);
  }, [eid]);

  const addComment = useCallback(
    async (body: string, anchor: unknown = null): Promise<void> => {
      requireOnline();
      // El ancla es jsonb libre en el backend (doc_comments.anchor): el editor
      // guarda `{ mark, text }` cuando el comentario nace de una selección, o
      // null para un comentario de documento sin ancla.
      const res = await fetch(`/api/comments?id=${eid}`, {
        method: 'POST',
        headers: JSON_HEADERS,
        body: JSON.stringify({ body, anchor }),
      });
      if (!res.ok) throw new Error(`comments ${res.status}`);
      await refreshComments();
    },
    [eid, refreshComments],
  );

  const resolveComment = useCallback(
    async (id: string, resolved: boolean): Promise<void> => {
      requireOnline();
      const res = await fetch(`/api/comments?id=${eid}&commentId=${encodeURIComponent(id)}`, {
        method: 'PUT',
        headers: JSON_HEADERS,
        body: JSON.stringify({ resolved }),
      });
      if (!res.ok) throw new Error(`resolve ${res.status}`);
      await refreshComments();
    },
    [eid, refreshComments],
  );

  const removeComment = useCallback(
    async (id: string): Promise<void> => {
      requireOnline();
      const res = await fetch(`/api/comments?id=${eid}&commentId=${encodeURIComponent(id)}`, {
        method: 'DELETE',
      });
      if (!res.ok) throw new Error(`delete ${res.status}`); // 403 → toast del caller
      await refreshComments();
    },
    [eid, refreshComments],
  );

  /* --- Versiones (proxy BFF /api/versions) --- */

  const loadVersions = useCallback(async (): Promise<void> => {
    requireOnline();
    const res = await fetch(`/api/versions?id=${eid}`);
    if (!res.ok) throw new Error(`versions ${res.status}`);
    const data = (await res.json()) as { versions?: VersionItem[] | null };
    setVersions(data.versions ?? []);
  }, [eid]);

  const createVersion = useCallback(
    async (label?: string): Promise<void> => {
      requireOnline();
      const res = await fetch(`/api/versions?id=${eid}`, {
        method: 'POST',
        headers: JSON_HEADERS,
        body: JSON.stringify(label === undefined ? {} : { label }),
      });
      if (!res.ok) throw new Error(`version ${res.status}`);
      await loadVersions().catch(() => undefined); // refresco best-effort
    },
    [eid, loadVersions],
  );

  /**
   * Adopta el estado del servidor tras una escritura suya (restaurar una versión,
   * aplicar un tema): re-GET del estado y recarga suave del contenido y los contadores
   * (§4.4). Las operaciones que acaba de escribir el servicio no son «cambio remoto», y la
   * edición en vuelo pierde (LWW): lo que manda es lo que el servidor escribió.
   */
  const adoptarEstadoDelServidor = useCallback(async (): Promise<void> => {
    const st = await fetchState();
    lamportRef.current = st.state.lamport;
    lastSeqRef.current = st.seq;
    collabRef.current?.advance(st.seq); // las ops del restore no son «cambio remoto»
    clearTimeout(saveTimerRef.current);
    pendingRef.current = null; // LWW: la restauración manda sobre la edición en vuelo
    setSave('saved');
    const raw = st.state.registers[keyRef.current]?.value;
    const value = typeof raw === 'string' ? raw : '';
    // La secuencia describía la versión anterior: se reconstruye desde el
    // estado restaurado (el servicio reinserta los elementos en su Diff).
    opsPendientesRef.current = [];
    coRef.current =
      kindRef.current === 'document'
        ? new CoEdicion(actorRef.current, st.state.elements, value)
        : null;
    modeloRef.current =
      kindRef.current === 'document'
        ? null
        : new CoedicionModelo(registroDe(st, keyRef.current), st.seq, value);
    setContent(value);
    setContentEpoch((e) => e + 1);
    setWords(countWords(value, kindRef.current));
    await loadVersions().catch(() => undefined);
  }, [fetchState, loadVersions, setSave]);

  const restoreVersion = useCallback(
    async (versionId: string): Promise<void> => {
      requireOnline();
      const res = await fetch(`/api/versions?id=${eid}&restore=${encodeURIComponent(versionId)}`, {
        method: 'POST',
        headers: JSON_HEADERS,
        body: JSON.stringify(sobreProteccion() ? { proteccion: sobreProteccion() } : {}),
      });
      olvidarProteccion();
      if (res.status === 403) {
        // Restaurar sobre una hoja protegida pide lo mismo que quitar la protección.
        toast.notify({
          tone: 'warning',
          title: t('sheet.msg.protectedSheet'),
          description: t('sheet.msg.protectionDenied'),
        });
        return;
      }
      if (!res.ok) throw new Error(`restore ${res.status}`);
      await adoptarEstadoDelServidor();
    },
    [eid, adoptarEstadoDelServidor, toast, t],
  );

  /**
   * Aplica un tema del catálogo común o la marca de la organización al documento
   * (`POST /api/themes`). El servicio cambia SOLO la presentación —títulos, cabeceras,
   * tipografías—, deja un punto de restauración «Antes del tema …» y devuelve las
   * operaciones que escribió; aquí se guarda lo pendiente ANTES (el tema se aplica al
   * estado del servidor) y se adopta el resultado como al restaurar una versión.
   * Nunca lanza: lo que falle se cuenta en un aviso.
   */
  const applyTheme = useCallback(
    async (themeId: string, brandVersion?: number): Promise<void> => {
      try {
        requireOnline();
        clearTimeout(saveTimerRef.current);
        await doSave();
        const res = await fetch(`/api/themes?id=${eid}`, {
          method: 'POST',
          headers: JSON_HEADERS,
          body: JSON.stringify(
            brandVersion ? { theme: themeId, brand_version: brandVersion } : { theme: themeId },
          ),
        });
        if (!res.ok) throw new Error(`theme ${res.status}`);
        const out = (await res.json()) as { changed?: boolean };
        if (out.changed) await adoptarEstadoDelServidor();
        toast.notify({
          tone: 'success',
          title: t(out.changed ? 'theme.msg.applied' : 'theme.msg.unchanged'),
        });
      } catch {
        toast.notify({ tone: 'danger', title: t('theme.msg.failed') });
      }
    },
    [eid, doSave, adoptarEstadoDelServidor, toast, t],
  );

  const previewTheme = useCallback(
    async (themeId: string, brandVersion?: number): Promise<TemaPrevia | null> => {
      try {
        await doSave(); // el servicio compara con SU estado: guardar lo pendiente antes
        const res = await fetch(`/api/themes?id=${eid}`, {
          method: 'POST',
          headers: JSON_HEADERS,
          body: JSON.stringify({
            theme: themeId,
            preview: true,
            ...(brandVersion ? { brand_version: brandVersion } : {}),
          }),
        });
        return res.ok ? ((await res.json()) as TemaPrevia) : null;
      } catch {
        return null;
      }
    },
    [eid, doSave],
  );

  /* --- Exportación: los callers no hacen .catch → el error se resuelve aquí con
     toast (nunca lanza, para que el estado «generando…» de la cabecera cierre). --- */

  const exportAs = useCallback(
    async (format: ExportFormat, plantilla = false): Promise<void> => {
      try {
        requireOnline();
        // Guardar lo pendiente ANTES de exportar: el backend materializa desde el
        // estado del servidor, así que un autosave en cola dejaría el fichero viejo/vacío.
        clearTimeout(saveTimerRef.current);
        await doSave();
        await descargarExport(eid, title, kindRef.current, format, plantilla);
      } catch {
        toast.notify({ tone: 'danger', title: t('doc.msg.exportFailed') });
      }
    },
    [eid, title, doSave, toast, t],
  );

  /**
   * Devuelve los cambios al fichero de Drive del que salió el documento.
   *
   * Guarda lo pendiente ANTES, igual que la exportación: el servicio serializa
   * desde el estado del servidor, así que un autosave en cola devolvería a Drive
   * una versión anterior a lo que se está viendo en pantalla.
   */
  const saveToDrive = useCallback(async (): Promise<void> => {
    if (!sourceRef) return;
    setSavingToDrive(true);
    try {
      requireOnline();
      clearTimeout(saveTimerRef.current);
      await doSave();
      const res = await fetch(`/api/save-to-drive?id=${eid}`, { method: 'POST' });
      if (!res.ok) throw new Error(`save-to-drive ${res.status}`);
      const { name } = (await res.json()) as { name: string };
      toast.notify({
        tone: 'success',
        title: t('doc.msg.savedToDrive'),
        description: t('doc.msg.savedToDriveIn', { name }),
      });
    } catch {
      toast.notify({ tone: 'danger', title: t('doc.msg.saveToDriveFailed') });
    } finally {
      setSavingToDrive(false);
    }
  }, [eid, sourceRef, doSave, toast, t]);

  /* --- Carga inicial + arranque de colaboración (§4.4) --- */

  useEffect(() => {
    let alive = true;
    setStatus('loading');
    setSave('saved');
    pendingRef.current = null;
    pendingTitleRef.current = null;
    metaRef.current = null;
    (async () => {
      if (!documentId) throw new Error('sin id');
      const cached = await readOfflineDocument(userId, documentId).catch(() => null);

      let meta: DocumentMeta;
      let initial: string;
      let initialLamport: number;
      let initialSeq: number;
      let contentDirty = false;
      let titleDirty = false;
      /** Secuencia RGA materializada del servidor; sin ella se arranca en LWW. */
      let elementos: unknown = null;
      /** Hojas y presentaciones: el registro del servidor y el ancestro del editor. */
      let servidor: Registro;
      let ancestro: string | null;

      try {
        const [metaRes, stateRes] = await Promise.all([
          fetch(`/api/documents?id=${encodeURIComponent(documentId)}`),
          fetch(`/api/state?id=${encodeURIComponent(documentId)}`),
        ]);
        if (!metaRes.ok || !stateRes.ok) throw new Error('carga fallida');
        // El BFF pasa el JSON del servicio tal cual, así que los campos llegan en
        // snake_case: `source_ref` es el fichero de Drive del que salió el
        // documento, y es lo que decide si se pueden devolver allí los cambios.
        const metaJson = (await metaRes.json()) as {
          document: DocumentMeta & { source_ref?: string };
          role?: DocumentRole;
          edit_requested?: boolean;
        };
        const serverMeta = metaJson.document;
        setRole(metaJson.role ?? 'none');
        roleRef.current = metaJson.role ?? 'none';
        fijarPropietario(roleRef.current === 'owner');
        fijarYo(userId);
        setEdicionPedida(metaJson.edit_requested === true);
        setSourceRef(serverMeta.source_ref ?? serverMeta.sourceRef ?? '');
        const st = (await stateRes.json()) as DocStateResponse;
        const key = CONTENT_KEY[serverMeta.kind];
        servidor = registroDe(st, key);
        const serverContent = servidor.valor;

        elementos = st.state.elements;
        contentDirty = cached?.contentDirty ?? false;
        titleDirty = cached?.titleDirty ?? false;
        meta = titleDirty && cached ? { ...serverMeta, title: cached.meta.title } : serverMeta;
        initial = contentDirty && cached ? cached.content : serverContent;
        // Un borrador sin guardar parte de la base que se guardó con él; sin ella
        // (copias de antes), manda lo propio entero, como siempre.
        ancestro = contentDirty ? (cached?.base ?? null) : serverContent;
        initialLamport = Math.max(st.state.lamport, cached?.lamport ?? 0);
        initialSeq = Math.max(st.seq, cached?.seq ?? 0);
        void cacheServerDocument(userId, serverMeta, serverContent, st.state.lamport, st.seq).catch(
          () => undefined,
        );
        const fromOfflineCache =
          metaRes.headers.get('x-sytue-offline') === 'cache' ||
          stateRes.headers.get('x-sytue-offline') === 'cache';
        setConnection(fromOfflineCache ? 'offline' : 'online');
      } catch {
        if (cached === null) throw new Error('sin copia offline');
        meta = cached.meta;
        initial = cached.content;
        initialLamport = cached.lamport;
        initialSeq = cached.seq;
        contentDirty = cached.contentDirty;
        titleDirty = cached.titleDirty;
        // Sin red: lo último que se supo del servidor es la copia limpia (o la base
        // del borrador); si ha cambiado, el primer guardado dará 409 y se relee.
        ancestro = contentDirty ? (cached.base ?? null) : cached.content;
        servidor = { valor: ancestro ?? '', lamport: cached.lamport, dot: null };
        setConnection('offline');
      }

      if (!alive) return;
      const key = CONTENT_KEY[meta.kind];
      metaRef.current = meta;
      kindRef.current = meta.kind;
      keyRef.current = key;
      lamportRef.current = initialLamport;
      lastSeqRef.current = initialSeq;
      counterRef.current = 0;
      peersRef.current = 0;
      opsPendientesRef.current = [];
      // Solo los documentos de texto van por carácter: en hojas y presentaciones
      // el contenido es JSON.stringify(modelo) y fusionar dos JSON carácter a
      // carácter da un JSON que no parsea, que es peor que perder una edición.
      coRef.current =
        meta.kind === 'document' ? new CoEdicion(actorRef.current, elementos, initial) : null;
      modeloRef.current =
        meta.kind === 'document' ? null : new CoedicionModelo(servidor, initialSeq, ancestro);
      confirmedTitleRef.current = meta.title;
      pendingRef.current = contentDirty ? initial : null;
      pendingTitleRef.current = titleDirty ? meta.title : null;

      setDoc(meta);
      setTitleState(meta.title);
      setContent(initial);
      setContentEpoch((e) => e + 1);
      setWords(countWords(initial, meta.kind));
      setSave(contentDirty || titleDirty ? 'dirty' : 'saved');
      setStatus('ready');

      collabRef.current = startCollab({
        documentId,
        actor: actorRef.current,
        contentKey: key,
        sinceSeq: initialSeq,
        getLamport: () => lamportRef.current,
        canAdopt: () => saveRef.current === 'saved',
        onRemoteContent: (value, lamport) => {
          lamportRef.current = lamport;
          // Con la secuencia en sincronía manda la fusión, no la vista del que
          // guardó: adoptar su `set` borraría lo que el usuario escribió a la vez.
          if (coRef.current?.sincronizada === true) return;
          setContent(value);
          setContentEpoch((e) => e + 1);
          setWords(countWords(value, kindRef.current));
          const currentMeta = metaRef.current;
          if (currentMeta !== null) {
            void cacheServerDocument(userId, currentMeta, value, lamport, lastSeqRef.current).catch(
              () => undefined,
            );
          }
        },
        onRemoteOps: (ops) => {
          coRef.current?.recibir(ops);
          fusionarRemoto(); // no-op si hay edición local en vuelo
        },
        ...(meta.kind === 'document'
          ? {}
          : {
              onRemoteSet: (so: SequencedOp) => {
                // El próximo guardado tiene que ganar el LWW del registro.
                lamportRef.current = Math.max(lamportRef.current, so.op.lamport);
                const v = so.op.value;
                modeloRef.current?.recibir(
                  typeof v === 'string' ? v : '',
                  so.seq,
                  so.op.lamport,
                  so.op.dot,
                );
                adoptarModelo(); // no-op con cambios propios pendientes
              },
            }),
        onLwwOverride: () => {
          // El emisor reescribió el documento entero: su `set` manda y nuestras
          // ops por carácter describen un estado que ya no existe.
          coRef.current?.anular();
          opsPendientesRef.current = [];
        },
        onPeers: (next) => {
          peersRef.current = next.length;
          setPeers(next);
        },
        onPeerCursors: (next) => setPeerCursors(next),
        onRole: (next) => {
          const r = next as DocumentRole;
          roleRef.current = r;
          fijarPropietario(r === 'owner');
          setRole(r);
          if (puedeEscribir(r)) setEdicionPedida(false);
        },
      });

      if (contentDirty && navigator.onLine) {
        saveTimerRef.current = window.setTimeout(() => void doSave(), 0);
      }
      if (titleDirty && navigator.onLine) {
        titleTimerRef.current = window.setTimeout(() => void doRename(), 0);
      }
      if (navigator.onLine) void refreshComments();
    })().catch(() => {
      if (alive) setStatus('error');
    });
    return () => {
      alive = false;
      collabRef.current?.stop();
      collabRef.current = null;
      clearTimeout(saveTimerRef.current);
      clearTimeout(retryTimerRef.current);
      clearTimeout(titleTimerRef.current);
    };
    // refreshComments/setSave son estables (useCallback sobre [eid]); documentId manda.
    // Añadir doSave/doRename recargaría el documento entero —y reabriría la sesión de
    // colaboración— cada vez que cambiara una de esas funciones. Es un efecto de montaje.
    // oxlint-disable-next-line react-hooks/exhaustive-deps
  }, [documentId, userId]);

  return {
    status,
    doc,
    title,
    setTitle,
    save,
    connection,
    content,
    contentEpoch,
    onContentInput,
    flush,
    saveNow,
    words,
    peers,
    peerCursors,
    sendCursor,
    comments,
    addComment,
    resolveComment,
    removeComment,
    versions,
    loadVersions,
    createVersion,
    restoreVersion,
    applyTheme,
    previewTheme,
    exportAs,
    /** No vacío si el documento salió de un fichero de Sytue Drive. */
    sourceRef,
    cursorDots,
    /** Mi papel en el documento, tal y como lo dice el servicio. */
    role,
    /**
     * ¿Se puede escribir? Mientras la carga no conteste es `false`: pintar un
     * lienzo editable y quitárselo medio segundo después es peor que esperar.
     */
    soloLectura: !puedeEscribir(role),
    edicionPedida,
    savingToDrive,
    saveToDrive,
  };
}
