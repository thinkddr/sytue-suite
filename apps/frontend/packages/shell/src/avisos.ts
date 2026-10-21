/**
 * Los avisos del Hub, sin Preact: lo que comparten la campana de la shell y la
 * bandeja de One. Carga, cuenta, flujo SSE con reconexión y «leído».
 *
 * Existe porque las dos pantallas lo tenían escrito cada una a su manera y la de
 * One se quedaba sorda en silencio: sin escuchar `error`, cuando el flujo se cortaba
 * —el Hub lo cortaba a los 30 s hasta el 2026-09-24— no volvía hasta recargar.
 */
import type { HubClient, Notification } from '@sytue/api-clients';
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
    estado = { ...estado, ...cambio };
    alCambiar(estado);
  };

  // Sube en cada carga: una página de «más antiguos» pedida antes se descarta.
  let generacion = 0;
  let cargandoMas = false;
  // Hay en la lista algo traído con «más antiguos»: solo entonces recargar lo conserva.
  let conAntiguos = false;

  const cargar = () => {
    generacion++;
    void hub
      .notifications({ limit: limite })
      .then((lista) => {
        if (cerrado) return;
        for (const n of lista) vistos.add(n.id);
        // Recargar (al reconectar el flujo, cada ~15 min) trae la primera página: lo que
        // ya se había traído con «más antiguos» se conserva detrás, sin repetir.
        // Solo si enlaza con la anterior (comparten algún aviso): si con el flujo caído
        // llegaron más de una página, pegar lo viejo detrás dejaría un hueco que no
        // trae nadie. Y la campana, que nunca carga más, no crece en cada reconexión.
        const ids = new Set(lista.map((n) => n.id));
        const ultimo = lista.at(-1);
        const enlaza = estado.items.some((n) => ids.has(n.id));
        const detras =
          conAntiguos && enlaza && ultimo
            ? estado.items.filter((n) => !ids.has(n.id) && esAnterior(n, ultimo))
            : [];
        conAntiguos = detras.length > 0;
        poner({
          items: [...lista, ...detras],
          cargando: false,
          fallo: false,
          hayMas: detras.length > 0 ? estado.hayMas : lista.length >= limite,
        });
      })
      .catch(() => {
        if (!cerrado) poner({ cargando: false, fallo: true });
      });
    // Si el contador falla, la lista sigue sirviendo: se queda sin número, no sin avisos.
    void hub
      .contadores()
      .then(({ unread, mailUnread }) => {
        if (!cerrado) poner({ sinLeer: unread, correoSinLeer: mailUnread });
      })
      .catch(() => undefined);
  };

  const llega = (n: Notification) => {
    if (!vistos.has(n.id)) {
      vistos.add(n.id);
      if (!n.read) poner({ sinLeer: estado.sinLeer + 1 });
      // ponytail: el correo nuevo suma aquí; el que se lee en otra pestaña no resta
      // hasta la próxima carga (reconexión o recarga). Sondear costaría un viaje a
      // Mail por pestaña y minuto.
      if (n.type === 'mail.message.received' && estado.correoSinLeer !== null) {
        poner({ correoSinLeer: estado.correoSinLeer + 1 });
      }
      alLlegar?.(n);
      avisarALaPagina(n);
    }
    poner({
      // Lo que se haya traído con «más antiguos» no se recorta.
      items: [n, ...estado.items.filter((v) => v.id !== n.id)].slice(
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
    void hub
      .streamNotifications()
      .then((es) => {
        if (cerrado) {
          es.close();
          return;
        }
        flujo = es;
        es.addEventListener('ready', () => {
          seguidos = 0;
        });
        es.addEventListener('notification', (e) => {
          seguidos = 0;
          try {
            llega(JSON.parse(String(e.data)) as Notification);
          } catch {
            /* un evento malformado no tumba la campana */
          }
        });
        es.addEventListener('avatar', (e) => {
          try {
            const { version } = JSON.parse(String(e.data)) as { version?: unknown };
            alCambiarFoto?.(typeof version === 'string' ? version : '');
          } catch {
            /* un evento malformado no tumba la campana */
          }
        });
        es.addEventListener('error', () => {
          es.close();
          if (flujo === es) flujo = null;
          reconectar();
        });
      })
      .catch(() => reconectar());
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

  return {
    async leer(id) {
      const antes = estado.items.find((n) => n.id === id);
      if (!antes || antes.read) return;
      poner({
        items: estado.items.map((n) => (n.id === id ? { ...n, read: true } : n)),
        sinLeer: Math.max(0, estado.sinLeer - 1),
      });
      try {
        await hub.markRead(id);
      } catch {
        // Sin acuse no se miente: vuelve a contar, y la próxima carga lo confirma.
        poner({
          items: estado.items.map((n) => (n.id === id ? { ...n, read: false } : n)),
          sinLeer: estado.sinLeer + 1,
        });
      }
    },
    async leerTodos() {
      const antes = estado;
      poner({ items: estado.items.map((n) => ({ ...n, read: true })), sinLeer: 0 });
      try {
        await hub.markAllRead();
      } catch {
        poner({ items: antes.items, sinLeer: antes.sinLeer });
      }
    },
    async masAntiguos() {
      const ultimo = estado.items.at(-1);
      if (!ultimo || !estado.hayMas || cargandoMas) return;
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
      if (cerrado || gen !== generacion || estado.items.at(-1)?.id !== ultimo.id) return;
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
      cancelarEspera?.();
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
