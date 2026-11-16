/**
 * Los avisos del Hub, sin Preact: lo que comparten la campana de la shell y la
 * bandeja de One. Carga, cuenta, flujo SSE con reconexión y «leído».
 *
 * Existe porque las dos pantallas lo tenían escrito cada una a su manera y la de
 * One se quedaba sorda en silencio: sin escuchar `error`, cuando el flujo se cortaba
 * —el Hub lo cortaba a los 30 s hasta el 2026-09-24— no volvía hasta recargar.
 */
import type { HubClient, Notification, NotificationSource } from '@sytue/api-clients';
import { catalogs, formatDateTime, translate, type Locale, type MessageKey } from '@sytue/i18n';

/** Lo que se usa del cliente del Hub. Un subconjunto, para poder probarlo con un doble. */
export type FuenteAvisos = Pick<
  HubClient,
  'notifications' | 'contadores' | 'markRead' | 'markAllRead'
> & {
  streamNotifications: () => Promise<FlujoAvisos>;
};

/** Lo que se usa de un `EventSource`. */
export interface FlujoAvisos {
  addEventListener: (tipo: string, fn: (e: { data?: unknown }) => void) => void;
  close: () => void;
}

export interface EstadoAvisos {
  items: Notification[];
  /** Lo que dice el Hub contando en la base, no los `!read` de la página cargada. */
  sinLeer: number;
  /**
   * Correos sin leer en Sytue Mail, `null` si no se sabe (sin Mail o sin respuesta).
   * Es ESTADO y no aviso: el correo que ya esperaba antes de que Mail avisara no
   * dejó ninguno, y la campana decía «Todo al día» con correos delante.
   */
  correoSinLeer: number | null;
  cargando: boolean;
  fallo: boolean;
  /** Quedan avisos más antiguos que los de la lista (`masAntiguos` los trae). */
  hayMas: boolean;
}

export interface OpcionesAvisos {
  limite: number;
  /** Un aviso NUEVO del flujo (no uno que ya estaba): es el que merece salir al SO. */
  alLlegar?: (n: Notification) => void;
  /**
   * La persona cambió o quitó su foto en Sytue ID (`event: avatar` del flujo, que no
   * es un aviso y no entra en la lista). `version` es la nueva; vacía si ya no tiene.
   */
  alCambiarFoto?: ((version: string) => void) | undefined;
  /** Para los tests: cuándo reintentar. */
  programar?: (fn: () => void, ms: number) => () => void;
}

/** `a` va detrás de `b` en la lista: más antiguo, o del mismo instante y menor id. */
function esAnterior(a: Notification, b: Notification): boolean {
  // Date.parse con los milisegundos solos: con microsegundos, algún motor da NaN.
  const ta = Date.parse(a.created_at.replace(/(\.\d{3})\d+/, '$1'));
  const tb = Date.parse(b.created_at.replace(/(\.\d{3})\d+/, '$1'));
  if (ta !== tb) return ta < tb;
  // El Hub guarda microsegundos y Date se queda en milésimas: el resto, de la cadena.
  const fa = fraccion(a.created_at);
  const fb = fraccion(b.created_at);
  return fa !== fb ? fa < fb : a.id < b.id;
}

const fraccion = (s: string): string => (/\.(\d+)/.exec(s)?.[1] ?? '').padEnd(9, '0');

function ordenarAvisos(items: Notification[]): Notification[] {
  return items.sort((a, b) => (esAnterior(a, b) ? 1 : esAnterior(b, a) ? -1 : 0));
}

function avisoDeFlujo(value: unknown): value is Notification {
  if (value === null || typeof value !== 'object' || Array.isArray(value)) return false;
  const n = value as Record<string, unknown>;
  for (const key of ['type', 'body', 'url', 'organization', 'organization_name']) {
    if (n[key] !== undefined && typeof n[key] !== 'string') return false;
  }
  if (n.historic !== undefined && typeof n.historic !== 'boolean') return false;
  if (n.params !== undefined) {
    if (n.params === null || typeof n.params !== 'object' || Array.isArray(n.params)) return false;
    if (!Object.values(n.params).every((v) => typeof v === 'string')) return false;
  }
  return (
    typeof n.id === 'string' &&
    n.id.length > 0 &&
    typeof n.source === 'string' &&
    typeof n.title === 'string' &&
    typeof n.created_at === 'string' &&
    Number.isFinite(Date.parse(n.created_at.replace(/(\.\d{3})\d+/, '$1'))) &&
    typeof n.read === 'boolean'
  );
}

/** Techo de la espera entre reconexiones: un Hub caído no recibe una por segundo de cada pestaña. */
export const ESPERA_MAX_MS = 30_000;

export interface Avisos {
  leer: (id: string) => Promise<void>;
  leerTodos: () => Promise<void>;
  reintentar: () => void;
  /** Trae la página siguiente, la de detrás del último de la lista. */
  masAntiguos: () => Promise<void>;
  cerrar: () => void;
}

export function conectarAvisos(
  hub: FuenteAvisos,
  alCambiar: (e: EstadoAvisos) => void,
  { limite, alLlegar, alCambiarFoto, programar = programarConReloj }: OpcionesAvisos,
): Avisos {
  let estado: EstadoAvisos = {
    items: [],
    sinLeer: 0,
    correoSinLeer: null,
    cargando: true,
    fallo: false,
    hayMas: false,
  };
  let flujo: FlujoAvisos | null = null;
  let cancelarEspera: (() => void) | null = null;
  let cerrado = false;
  let seguidos = 0;
  // Los ya contados: un evento repetido del flujo no infla el contador.
  const vistos = new Set<string>();

  const poner = (cambio: Partial<EstadoAvisos>) => {
    if (cerrado) return;
    estado = { ...estado, ...cambio };
    alCambiar(estado);
  };

  // Sube en cada carga: una página de «más antiguos» pedida antes se descarta.
  let generacion = 0;
  let cargandoMas = false;
  // Hay en la lista algo traído con «más antiguos»: solo entonces recargar lo conserva.
  let conAntiguos = false;
  let revisionEventos = 0;
  let revisionContadores = 0;
  let recibidosSinLeer = 0;
  const revisionDeAviso = new Map<string, number>();
  let escribiendo = false;
  let escrituraAnterior: Promise<void> = Promise.resolve();
  let intentoFlujo = 0;
  let intentoConteo = 0;
  let cancelarConteo: (() => void) | null = null;
  let primeraCargaTerminada = false;
  let tengoBase = false;
  let reconciliarAlFinal = false;

  const contar = (gen: number) => {
    cancelarConteo?.();
    cancelarConteo = null;
    const intento = ++intentoConteo;
    const revision = revisionContadores;
    void hub
      .contadores()
      .then(({ unread, mailUnread }) => {
        if (cerrado || gen !== generacion || intento !== intentoConteo || escribiendo) return;
        if (revision !== revisionContadores) {
          if (!cancelarConteo) {
            cancelarConteo = programar(() => {
              cancelarConteo = null;
              if (!cerrado && gen === generacion && !escribiendo) contar(gen);
            }, 500);
          }
          return;
        }
        poner({ sinLeer: unread, correoSinLeer: mailUnread });
      })
      .catch(() => undefined);
  };

  const cargar = () => {
    if (cerrado || escribiendo) return;
    const gen = ++generacion;
    const revision = revisionEventos;
    cancelarConteo?.();
    cancelarConteo = null;
    void hub
      .notifications({ limit: limite })
      .then((lista) => {
        if (cerrado || gen !== generacion || escribiendo) return;
        const previos = estado.items;
        // La página pudo consultarse antes de un evento ya pintado: conservar
        // esa versión y no duplicarla ni relegarla detrás de una respuesta vieja.
        const recientes = previos.filter((n) => (revisionDeAviso.get(n.id) ?? 0) > revision);
        if (tengoBase) {
          const anterior = previos[0];
          // Tras reconectar, anunciar avisos más nuevos que el extremo conocido
          // también actualiza consumidores de concesiones sin anunciar páginas viejas.
          const nuevos = lista.filter(
            (n) => !vistos.has(n.id) && (!anterior || esAnterior(anterior, n)),
          );
          for (const n of nuevos.reverse()) {
            llega(n, false, true);
            if (cerrado || gen !== generacion || escribiendo) return;
          }
          if (nuevos.length) contar(gen);
        }
        for (const n of lista) vistos.add(n.id);
        // Recargar (al reconectar el flujo, cada ~15 min) trae la primera página: lo que
        // ya se había traído con «más antiguos» se conserva detrás, sin repetir.
        // Solo si enlaza con la anterior (comparten algún aviso): si con el flujo caído
        // llegaron más de una página, pegar lo viejo detrás dejaría un hueco que no
        // trae nadie. Y la campana, que nunca carga más, no crece en cada reconexión.
        const ids = new Set(lista.map((n) => n.id));
        const ultimo = lista.at(-1);
        const enlaza = previos.some((n) => ids.has(n.id));
        const detras =
          conAntiguos && enlaza && ultimo
            ? previos.filter((n) => !ids.has(n.id) && esAnterior(n, ultimo))
            : [];
        conAntiguos = detras.length > 0;
        const idsRecientes = new Set(recientes.map((n) => n.id));
        const items = [
          ...recientes,
          ...lista.filter((n) => !idsRecientes.has(n.id)),
          ...detras.filter((n) => !idsRecientes.has(n.id)),
        ];
        if (recientes.length) ordenarAvisos(items);
        for (const [id, version] of revisionDeAviso) {
          if (version <= revision) revisionDeAviso.delete(id);
        }
        poner({
          items: conAntiguos ? items : items.slice(0, limite),
          cargando: false,
          fallo: false,
          hayMas: detras.length > 0 ? estado.hayMas : lista.length >= limite,
        });
        tengoBase = true;
        primeraCargaTerminada = true;
        if (reconciliarAlFinal) {
          reconciliarAlFinal = false;
          cargar();
        }
      })
      .catch(() => {
        if (!cerrado && gen === generacion && !escribiendo) {
          primeraCargaTerminada = true;
          poner({ cargando: false, fallo: true });
          if (reconciliarAlFinal) {
            reconciliarAlFinal = false;
            cargar();
          }
        }
      });
    // Si el contador falla, la lista sigue sirviendo: se queda sin número, no sin avisos.
    contar(gen);
  };

  const llega = (n: Notification, sumarContador = true, recuperado = false) => {
    const previo = estado.items.find((item) => item.id === n.id);
    // El servicio solo marca leídos: un evento duplicado o atrasado no debe
    // deshacer una lectura ya conocida ni el gesto aún pendiente.
    if (previo?.read && !n.read) n = { ...n, read: true };
    if (!recuperado) revisionDeAviso.set(n.id, ++revisionEventos);
    if (!vistos.has(n.id)) {
      vistos.add(n.id);
      if (!n.read) {
        recibidosSinLeer++;
        revisionContadores++;
        if (sumarContador) poner({ sinLeer: estado.sinLeer + 1 });
        if (cerrado) return;
      }
      // ponytail: el correo nuevo suma aquí; el que se lee en otra pestaña no resta
      // hasta la próxima carga (reconexión o recarga). Sondear costaría un viaje a
      // Mail por pestaña y minuto.
      if (n.type === 'mail.message.received' && estado.correoSinLeer !== null) {
        revisionContadores++;
        if (sumarContador) poner({ correoSinLeer: estado.correoSinLeer + 1 });
        if (cerrado) return;
      }
      try {
        if (!recuperado || !n.read) alLlegar?.(n);
      } catch {
        // Un fallo del aviso al sistema no debe impedir actualizar la página.
      }
      if (cerrado) return;
      try {
        avisarALaPagina(n);
      } catch {
        // Una integración de la página no debe impedir pintar la campana.
      }
      if (cerrado) return;
    }
    poner({
      // Lo que se haya traído con «más antiguos» no se recorta.
      items: ordenarAvisos([n, ...estado.items.filter((v) => v.id !== n.id)]).slice(
        0,
        Math.max(limite, estado.items.length),
      ),
      cargando: false,
    });
  };

  // RECONECTAR CON UN TOKEN NUEVO, y no dejar que lo haga `EventSource`: el token
  // viaja en la query y vive quince minutos, y la reconexión del navegador repite
  // LA MISMA URL, así que en cuanto caduca cada reintento es un 401. Y al volver se
  // recarga la lista: lo que llegó con el flujo caído no lo trae nadie más.
  const escuchar = () => {
    const intento = ++intentoFlujo;
    void hub
      .streamNotifications()
      .then((es) => {
        if (cerrado || intento !== intentoFlujo) {
          es.close();
          return;
        }
        cancelarEspera?.();
        cancelarEspera = null;
        flujo?.close();
        flujo = es;
        let listo = false;
        es.addEventListener('ready', () => {
          if (cerrado || flujo !== es) return;
          if (listo) return;
          listo = true;
          seguidos = 0;
          // Al suscribirnos puede haberse creado un aviso después del GET
          // anterior; conciliar cuando haya una base con la que compararlo.
          if (primeraCargaTerminada) cargar();
          else reconciliarAlFinal = true;
        });
        es.addEventListener('notification', (e) => {
          if (cerrado || flujo !== es) return;
          seguidos = 0;
          try {
            const n: unknown = JSON.parse(String(e.data));
            if (avisoDeFlujo(n)) llega(n);
          } catch {
            /* un evento malformado no tumba la campana */
          }
        });
        es.addEventListener('avatar', (e) => {
          if (cerrado || flujo !== es) return;
          try {
            const { version } = JSON.parse(String(e.data)) as { version?: unknown };
            alCambiarFoto?.(typeof version === 'string' ? version : '');
          } catch {
            /* un evento malformado no tumba la campana */
          }
        });
        es.addEventListener('error', () => {
          if (cerrado || flujo !== es) return;
          es.close();
          if (flujo === es) flujo = null;
          reconectar();
        });
      })
      .catch(() => {
        if (!cerrado && intento === intentoFlujo) reconectar();
      });
  };

  const reconectar = () => {
    if (cerrado || cancelarEspera) return;
    const espera = Math.min(ESPERA_MAX_MS, 1000 * 2 ** seguidos);
    seguidos += 1;
    cancelarEspera = programar(() => {
      cancelarEspera = null;
      if (cerrado) return;
      cargar();
      escuchar();
    }, espera);
  };

  cargar();
  escuchar();

  // Los gestos se serializan para que un fallo anterior no deshaga otro gesto.
  // Ninguna carga empezada antes o durante una escritura se aplica al terminar.
  const escribir = (operacion: () => Promise<void>): Promise<void> => {
    const siguiente = escrituraAnterior.then(async () => {
      if (cerrado) return;
      escribiendo = true;
      generacion++;
      revisionContadores++;
      try {
        await operacion();
      } finally {
        escribiendo = false;
        if (!cerrado) cargar();
      }
    });
    escrituraAnterior = siguiente.catch(() => undefined);
    return siguiente;
  };

  return {
    leer(id) {
      return escribir(async () => {
        const antes = estado.items.find((n) => n.id === id);
        if (!antes || antes.read) return;
        poner({
          items: estado.items.map((n) => (n.id === id ? { ...n, read: true } : n)),
          sinLeer: Math.max(0, estado.sinLeer - 1),
        });
        try {
          await hub.markRead(id);
        } catch {
          if (cerrado) return;
          poner({
            items: estado.items.map((n) => (n.id === id ? { ...n, read: false } : n)),
            sinLeer: estado.sinLeer + 1,
          });
        }
      });
    },
    leerTodos() {
      return escribir(async () => {
        const antes = estado;
        const antesPorId = new Map(antes.items.map((n) => [n.id, n.read]));
        const recibidosAntes = recibidosSinLeer;
        poner({ items: estado.items.map((n) => ({ ...n, read: true })), sinLeer: 0 });
        try {
          await hub.markAllRead();
        } catch {
          if (cerrado) return;
          poner({
            items: estado.items.map((n) =>
              antesPorId.has(n.id) ? { ...n, read: antesPorId.get(n.id)! } : n,
            ),
            sinLeer: antes.sinLeer + recibidosSinLeer - recibidosAntes,
          });
        }
      });
    },
    async masAntiguos() {
      const ultimo = estado.items.at(-1);
      if (!ultimo || !estado.hayMas || cargandoMas || escribiendo) return;
      const gen = generacion;
      cargandoMas = true;
      let lista: Notification[];
      try {
        lista = await hub.notifications({ limit: limite, antes: ultimo });
      } catch {
        return; // el botón sigue ahí para volver a probar
      } finally {
        cargandoMas = false;
      }
      // Si entretanto se recargó la lista, esta página se pidió respecto a otra: se
      // tira, y la persona vuelve a pulsar sobre la lista de ahora.
      if (cerrado || escribiendo || gen !== generacion || estado.items.at(-1)?.id !== ultimo.id)
        return;
      for (const n of lista) vistos.add(n.id);
      const ya = new Set(estado.items.map((n) => n.id));
      const nuevos = lista.filter((n) => !ya.has(n.id));
      if (nuevos.length > 0) conAntiguos = true;
      poner({
        items: [...estado.items, ...nuevos],
        hayMas: lista.length >= limite,
      });
    },
    reintentar() {
      poner({ cargando: true, fallo: false });
      cargar();
      if (!flujo) escuchar();
    },
    cerrar() {
      cerrado = true;
      intentoFlujo++;
      cancelarEspera?.();
      cancelarConteo?.();
      flujo?.close();
    },
  };
}

/**
 * La URL de la foto con la versión nueva, para que el navegador no pinte la de su
 * caché: `/drive/auth/avatar?v=vieja` pasa a `?v=nueva`. Sin versión, sin foto (las
 * iniciales). `respaldo` es la ruta `/auth/avatar` de esta app, para quien no tenía
 * foto al cargar la página y no trae URL que reescribir.
 */
export function fotoConVersion(
  actual: string | undefined,
  version: string,
  respaldo: string,
): string | undefined {
  if (!version) return undefined;
  const base = (actual || respaldo).split('?')[0];
  return `${base}?v=${encodeURIComponent(version)}`;
}

function programarConReloj(fn: () => void, ms: number): () => void {
  const id = setTimeout(fn, ms);
  return () => clearTimeout(id);
}

/** Familia de productos bajo la que se agrupan las reglas de silencio. */
export type GrupoDeAvisos = 'communication' | 'files' | 'business' | 'publishing' | 'suite';

/**
 * Los orígenes que se pueden silenciar, agrupados por familia y en el orden en que
 * se enseñan. Es la lista de `domain.NotificationSources` del Hub menos dos:
 *
 * - `id`: los avisos de seguridad (un inicio de sesión nuevo, una contraseña
 *   cambiada) no se ofrecen; callar el que dice «alguien ha entrado en tu cuenta»
 *   es justo lo que no tiene que poder hacerse de un clic.
 * - `chat`: es el nombre con el que Rooms se deja BUSCAR; sus avisos llegan como
 *   `rooms`, y dos interruptores con el mismo nombre de Rooms confundirían.
 *
 * Una sola lista para la bandeja de One (y la paridad de Fyne): la que tenía
 * escrita a mano solo seis productos, y las tareas del ERP, los pedidos de Sytue Web
 * o los directos de Cast no se podían callar. `avisos.test.ts` la compara con el
 * `domain.go` del Hub: un origen nuevo allí la pone en rojo.
 */
export const ORIGENES_DE_AVISOS: readonly {
  grupo: GrupoDeAvisos;
  origenes: readonly NotificationSource[];
}[] = [
  { grupo: 'communication', origenes: ['mail', 'rooms', 'calendar', 'contacts'] },
  {
    grupo: 'files',
    origenes: ['drive', 'docs', 'sheets', 'slides', 'pdf', 'notes', 'photos'],
  },
  { grupo: 'business', origenes: ['erp', 'bases'] },
  { grupo: 'publishing', origenes: ['pages', 'cast', 'cloud'] },
  { grupo: 'suite', origenes: ['one', 'integrations'] },
];

/** Los rótulos de las filas de «por tipo» (`inbox.prefsKind.<id>` del portal). */
export type IdTipoDeAviso =
  | 'messages'
  | 'directMessages'
  | 'mentions'
  | 'invitations'
  | 'reports'
  | 'reminders'
  | 'accessRequests'
  | 'accessGranted'
  | 'shared'
  | 'comments'
  | 'quota'
  | 'signatureRequests'
  | 'signatureUpdates'
  | 'albumChanges'
  | 'tasks'
  | 'approvals'
  | 'transfers'
  | 'orders'
  | 'forms'
  | 'domains'
  | 'exports'
  | 'settingsChanges'
  | 'storeIncidents'
  | 'withdrawals'
  | 'live'
  | 'deploys'
  | 'updates';

/**
 * Una fila de «por tipo» dentro de un origen: su rótulo (`inbox.prefsKind.<id>` del
 * portal, con nombre propio: las claves `notifications.kind.*` son títulos de aviso
 * con parámetros, «{actor} te ha mencionado en «{name}»», y no sirven de rótulo) y
 * los tipos del Hub que reúne. Varios tipos van juntos cuando para quien lee son lo
 * mismo: «Invitaciones» de Rooms son la de canal y la de conversación directa.
 */
export interface TipoDeAviso {
  id: IdTipoDeAviso;
  /** Los `type` de los avisos del Hub que agrupa la fila. */
  tipos: readonly string[];
  /**
   * Pide actuar y no se puede callar (`domain.TipoCritico` del Hub): se enseña fijo y
   * encendido, y llega aunque se calle el origen entero.
   */
  critico?: true;
}

const peticiones = (app: string): TipoDeAviso[] => [
  { id: 'accessRequests', tipos: [`${app}.access.requested`] },
  { id: 'accessGranted', tipos: [`${app}.access.granted`] },
];
// Docus, Calcus y Diapos avisan con los mismos tipos `docs.*`: el origen es el de la
// app que abre el documento (`origenDe` del servicio de documentos).
const documentos: readonly TipoDeAviso[] = [
  { id: 'shared', tipos: ['docs.shared'] },
  { id: 'comments', tipos: ['docs.comment'] },
  { id: 'mentions', tipos: ['docs.mention'] },
  ...peticiones('docs'),
];

/**
 * Los tipos de aviso de cada origen, para callar solo una parte («las menciones sí,
 * cada nuevo correo no»). Cubre todos los `notifications.kind.*` del catálogo común
 * salvo los de seguridad, que son de Sytue ID y van fijos; `avisos.test.ts` lo vigila,
 * y también que los críticos sean los del Hub. Un origen sin entrada solo tiene su
 * interruptor general.
 */
export const TIPOS_DE_AVISOS: Readonly<
  Partial<Record<NotificationSource, readonly TipoDeAviso[]>>
> = {
  mail: [{ id: 'messages', tipos: ['mail.message.received'] }, ...peticiones('mail')],
  rooms: [
    { id: 'directMessages', tipos: ['rooms.message'] },
    { id: 'mentions', tipos: ['rooms.mention'] },
    { id: 'invitations', tipos: ['rooms.invitation', 'rooms.invitation.direct'] },
    { id: 'reports', tipos: ['rooms.report'], critico: true },
  ],
  calendar: [
    { id: 'invitations', tipos: ['calendar.invitation'] },
    { id: 'reminders', tipos: ['calendar.reminder.due'] },
    ...peticiones('calendar'),
  ],
  drive: [
    { id: 'shared', tipos: ['drive.share.created'] },
    { id: 'quota', tipos: ['drive.quota.warning'] },
    ...peticiones('drive'),
  ],
  docs: documentos,
  sheets: documentos,
  slides: documentos,
  pdf: [
    { id: 'signatureRequests', tipos: ['docs.signature.requested', 'docs.signature.reminder'] },
    {
      id: 'signatureUpdates',
      tipos: ['docs.signature.completed', 'docs.signature.declined', 'docs.signature.expired'],
    },
  ],
  notes: [
    { id: 'shared', tipos: ['notes.shared'] },
    { id: 'mentions', tipos: ['notes.mention'] },
    { id: 'reminders', tipos: ['notes.reminder.due', 'notes.reminder.untitled'] },
    ...peticiones('notes'),
  ],
  photos: [
    { id: 'shared', tipos: ['photos.shared'] },
    { id: 'albumChanges', tipos: ['photos.album.added', 'photos.album.renamed'] },
  ],
  erp: [
    { id: 'tasks', tipos: ['erp.task.assigned'] },
    { id: 'approvals', tipos: ['erp.approval.requested'] },
    ...peticiones('erp'),
  ],
  bases: [
    { id: 'comments', tipos: ['bases.comment'] },
    { id: 'mentions', tipos: ['bases.mention'] },
    { id: 'transfers', tipos: ['bases.transferred'] },
  ],
  pages: [
    { id: 'orders', tipos: ['pages.order.paid'] },
    { id: 'forms', tipos: ['pages.form.received'] },
    { id: 'domains', tipos: ['pages.domain.active', 'pages.domain.failed'] },
    { id: 'exports', tipos: ['pages.export.ready'] },
    { id: 'settingsChanges', tipos: ['pages.settings.changed'] },
    {
      id: 'storeIncidents',
      tipos: [
        'pages.order.oversold',
        'pages.payment.duplicated',
        'pages.erp.failed',
        'pages.store.busy',
      ],
      critico: true,
    },
    { id: 'withdrawals', tipos: ['pages.withdrawal.requested'], critico: true },
  ],
  cast: [
    { id: 'live', tipos: ['cast.live'] },
    { id: 'reports', tipos: ['cast.report'], critico: true },
  ],
  cloud: [{ id: 'deploys', tipos: ['cloud.deploy'] }],
  one: [{ id: 'updates', tipos: ['one.update.available'] }],
};

/** Los tipos que no se pueden callar: la lista `tiposCriticos` del Hub. */
export const TIPOS_CRITICOS: ReadonlySet<string> = new Set(
  Object.values(TIPOS_DE_AVISOS).flatMap((filas) =>
    (filas ?? []).filter((f) => f.critico).flatMap((f) => f.tipos),
  ),
);

const HUECO = /\{(\w+)\}/g;

/**
 * El título de un aviso en el idioma de quien lo mira. El emisor manda `type` y
 * `params` y el catálogo común tiene `notifications.kind.<type>`; si el tipo no está
 * —un emisor más nuevo que este cliente— o le falta una variable, sale el `title`
 * que mandó el emisor, que es su respaldo.
 */
export function tituloAviso(locale: Locale, n: Notification): string {
  const clave = `notifications.kind.${n.type ?? ''}`;
  const plantilla = (catalogs[locale] as Record<string, unknown>)[clave];
  if (typeof plantilla !== 'string') return n.title;
  const vars = n.params ?? {};
  for (const [, nombre] of plantilla.matchAll(HUECO)) {
    if (!vars[nombre ?? '']) return n.title;
  }
  return translate(locale, clave as MessageKey, vars);
}

/**
 * El cuerpo de un aviso: lo que mandó el emisor (el asunto de un correo, el texto de
 * un comentario) o, si no trae, la hora de lo que avisa (`params.start`: un
 * recordatorio, una invitación) en la zona de quien lo mira. El servicio solo la
 * tiene en UTC, y escrita allí salía con la hora de otra zona.
 */
export function cuerpoAviso(locale: Locale, n: Notification): string | undefined {
  if (n.body) return n.body;
  const inicio = n.params?.['start'];
  return inicio ? formatDateTime(locale, inicio) : undefined;
}

/**
 * Cómo se enseña una organización de fuera: su nombre visible si Sytue ID lo da
 * («Acme Ingeniería S.L.») y, si no, su identificador. La campana y las bandejas de
 * invitaciones de Rooms y Calendar lo comparten (`api.OrgVisible` en Fyne).
 */
export function nombreDeOrganizacion(x: {
  organization?: string;
  organization_name?: string;
}): string {
  return x.organization_name || x.organization || '';
}

/**
 * Lo que dice de dónde viene un aviso, antes de cuándo llegó: la organización que lo
 * mandó si no es la de la sesión (ADR 0053), por su nombre visible si el Hub lo da, y, si se reconstruyó de lo que ya había,
 * que es anterior al registro de avisos. Vacío en un aviso normal de la propia.
 */
export function procedenciaAviso(locale: Locale, n: Notification): string[] {
  const out: string[] = [];
  const org = nombreDeOrganizacion(n);
  if (org) out.push(org);
  if (n.historic) out.push(translate(locale, 'notifications.historic'));
  return out;
}

/**
 * Pulsar un aviso: se da por leído y se va a su elemento. Espera al «leído» un
 * momento, porque navegar antes cancelaba la petición y el aviso volvía sin leer.
 */
export async function abrirAviso(
  avisos: Pick<Avisos, 'leer'>,
  n: Notification,
  navegar: (url: string) => void,
): Promise<void> {
  const destino = destinoAviso(n);
  await Promise.race([avisos.leer(n.id), new Promise((r) => setTimeout(r, 800))]);
  if (destino) navegar(destino);
}

/**
 * Adónde lleva un aviso. Solo http(s): el Hub ya lo valida al crearlo, y esto es la
 * segunda verja en el sitio donde se navega.
 */
export function destinoAviso(n: Notification): string | null {
  if (!n.url) return null;
  try {
    const u = new URL(n.url);
    return u.protocol === 'https:' || u.protocol === 'http:' ? u.toString() : null;
  } catch {
    return null;
  }
}

/**
 * EL FLUJO DEL HUB, PARA LA PÁGINA. La campana ya tiene abierto el flujo de avisos; en
 * vez de que cada app abra el suyo (o sondee), cada aviso NUEVO se reemite en la
 * ventana como `sytue:aviso`. Es lo que desbloquea en vivo la franja de solo lectura
 * de las apps sin canal propio: al conceder, llega `<producto>.access.granted` con
 * `params.resource`.
 */
export const EVENTO_AVISO = 'sytue:aviso';

function avisarALaPagina(n: Notification): void {
  if (typeof window === 'undefined' || typeof CustomEvent === 'undefined') return;
  window.dispatchEvent(new CustomEvent<Notification>(EVENTO_AVISO, { detail: n }));
}

/** Escucha los avisos nuevos del Hub en esta página. Devuelve con qué dejar de escuchar. */
export function alLlegarAviso(fn: (n: Notification) => void): () => void {
  const oir = (e: Event) => fn((e as CustomEvent<Notification>).detail);
  window.addEventListener(EVENTO_AVISO, oir);
  return () => window.removeEventListener(EVENTO_AVISO, oir);
}

/** ¿Es `n` el aviso de que se ha concedido editar `recurso`? (`<producto>.access.granted`) */
export function esConcesion(n: Notification, tipo: string, recurso?: string): boolean {
  return n.type === tipo && (!recurso || n.params?.['resource'] === recurso);
}
