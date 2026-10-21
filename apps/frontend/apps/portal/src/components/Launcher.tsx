/**
 * Launcher — cara del portal Sytue One. Saludo personalizado, rejilla de
 * lanzadores hacia TODOS los productos de la suite (tarjetas grandes con icono y
 * color de marca), accesos rápidos cross-app y actividad reciente del Hub
 * (búsqueda federada). Lenguaje visual Sytue, claro/oscuro.
 *
 * Es una isla autónoma: la actividad reciente se obtiene del BFF del portal
 * (/api/recent) autorizado por cookie; el access token JAMÁS se maneja en el
 * cliente. No gestiona credenciales (identidad federada por Sytue ID).
 */
import type { JSX } from 'preact';
import type { Locale } from '@sytue/i18n';
import { createTranslate, formatDate, formatTime, formatTimeAgo } from '@sytue/i18n';
import { useT } from '../i18n/index.js';
import { r } from '../lib/rutas.js';
import { haTerminado, limitesDelDia, ordenarEventos, type EventoDeHoy } from '../lib/hoy.js';
import { recientesVisibles } from '../lib/reciente.js';
import { useEffect, useMemo, useState } from 'preact/hooks';
import {
  Button,
  Dropdown,
  MenuItem,
  Card,
  CardBody,
  CardIcon,
  type CardTone,
  cardTone,
  SectionHeader,
  Skeleton,
  EmptyState,
  Icon,
  appsPresentables,
  suiteApps,
  CLAVE_ADMIN,
  appForKey,
  hrefDe,
  type SuiteApp,
  type ThemeForma,
  ProductIcon,
  Illustration,
  cx,
  useAhora,
  Glifo,
} from '@sytue/ui';
import { appsRecordadas, detalleDe, recordarApps } from '@sytue/shell';
import { appUrl, esSoloDeOrganizaciones, orgActual, suiteApex } from '@sytue/config';
import type { SearchResult } from '@sytue/api-clients';
/** App conectada (integración) tal como la sirve el gateway vía /api/my-apps: misma
 *  forma que un SuiteApp + estado. key es el id de instancia (no un ProductKey). */
interface ConnectedApp {
  key: string;
  name: string;
  description: string;
  href: string;
  iconUrl: string;
  color: string;
  status: string;
}

/**
 * El Hub no se lanza desde el Hub: es la única exclusión de esta galería.
 *
 * ANTES ERA UNA LISTA A MANO de trece claves, y por eso NOTEPADS no aparecía:
 * al añadirse el producto a `APP_SWITCHER_ORDER` nadie tocó esta copia, así que
 * el conmutador lo ofrecía y la primera pantalla de la suite no. Un producto
 * entero invisible, sin que nada fallara.
 *
 * Derivado y no duplicado: lo que entre en el catálogo entra aquí solo, y lo que
 * salga desaparece. La lista a mano tampoco decía la verdad —su comentario
 * prometía «sin duplicar sheets/slides» y los incluía—.
 */
const CLAVE_DEL_HUB: SuiteApp['key'] = 'one';

/** Índice de productos por clave: color y nombre del origen en la actividad reciente. */
const APP_BY_KEY = new Map<SuiteApp['key'], SuiteApp>(suiteApps.map((a) => [a.key, a]));

/** URL base del producto en el entorno actual (dev/prod), como los lanzadores. */
const appHref = (key: SuiteApp['key']): string => APP_BY_KEY.get(key)?.href ?? '';

/**
 * Glifos de los accesos rápidos que aún no existen en el set de cromo de
 * @sytue/ui (mismo lenguaje visual: rejilla 24, envoltorio `Glifo`, currentColor,
 * decorativos). Mismo patrón local que DocList (docs) y glyphs (mail/drive).
 */
const GLYPHS = {
  mail: 'M4 5h16a1 1 0 0 1 1 1v12a1 1 0 0 1-1 1H4a1 1 0 0 1-1-1V6a1 1 0 0 1 1-1Zm-1 2 9 6 9-6',
  file: 'M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8zM14 2v6h6',
  upload: 'M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4M7 8l5-5 5 5M12 3v12',
  calendar:
    'M8 2v4M16 2v4M3 10h18M5 4h14a2 2 0 0 1 2 2v14a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V6a2 2 0 0 1 2-2Z',
  download: 'M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4M7 10l5 5 5-5M12 15V3',
  phone: 'M7 2h10a1 1 0 0 1 1 1v18a1 1 0 0 1-1 1H7a1 1 0 0 1-1-1V3a1 1 0 0 1 1-1Zm4 16h2',
  archive: 'M21 8v12a1 1 0 0 1-1 1H4a1 1 0 0 1-1-1V8M2 3h20v5H2zM10 12h4',
} as const;

function Glyph({ name, size = 16 }: { name: keyof typeof GLYPHS; size?: number }): JSX.Element {
  return <Glifo d={GLYPHS[name]} size={size} />;
}

/** Accesos rápidos cross-app (intents típicos estilo Google Workspace).
 *  Chip neutro: la página tiene UN acento (el del portal), no un arcoíris.
 *
 *  `product` dice a qué producto pertenece la acción y `path` la ruta de Sytue que la
 *  materializa. Se guardan por separado porque si la organización ha elegido que ese
 *  producto lo sirva otra aplicación, la ruta de Sytue NO se puede pegar detrás: Google
 *  el sustituto no tiene esa ruta, y concatenarla daría un enlace roto. En ese caso la
 *  acción lleva a la aplicación elegida, que es lo peor que puede pasar sin mentir.
 *
 *  El icono se pinta llamando a `icon()` y no se guarda ya construido: así cada render
 *  crea el suyo, sin que dos filas compartan el mismo nodo. Y es una función y no un
 *  nombre de glifo porque la lupa YA existe en el set de cromo (`Icon name="search"`):
 *  copiar su trazado aquí sería la segunda copia del mismo dibujo, y las copias
 *  divergen. */
/** Los colores de grupo de los accesos rápidos, por turno: los cuatro de la tarjeta de
 *  recurso (azul, violeta, verde y ámbar). No dicen nada del acceso; lo separan
 *  del de al lado. */
const TONOS_ACCESO: CardTone[] = ['blue', 'violet', 'mint', 'amber'];

const QUICK_ACTIONS: {
  /** Clave del catálogo, no el texto: el rótulo se traduce al pintarlo. */
  labelKey:
    | 'quick.search'
    | 'quick.compose'
    | 'quick.newDoc'
    | 'quick.upload'
    | 'quick.newEvent'
    | 'quick.download'
    | 'quick.devices'
    | 'quick.data';
  icon: () => JSX.Element;
  product?: SuiteApp['key'];
  path?: string;
  href?: string;
}[] = [
  // La búsqueda federada tenía su página entera (/buscar: todos los resultados,
  // agrupados por producto, con URL compartible) y NADIE enlazaba a ella: solo se
  // llegaba escribiendo la ruta a mano. El popover de la topbar no ofrece salida
  // a la vista completa, así que en la práctica no existía.
  { labelKey: 'quick.search', href: r('/buscar'), icon: () => <Icon name="search" size={16} /> },
  {
    // Sin página propia (`MailApp` la abre leyendo `?action=compose` en la raíz,
    // el mismo intent que usa Drive con «Adjuntar a un correo»): un `/compose`
    // literal no es una ruta de Mail y era 404.
    labelKey: 'quick.compose',
    product: 'mail',
    path: '/?action=compose',
    icon: () => <Glyph name="mail" />,
  },
  // Sin ruta: Sytue Docs NO tiene un `/new/document` —solo `/` y `/edit`—, así que
  // apuntaba a un 404. Su portada es la lista de documentos, con el botón de crear.
  { labelKey: 'quick.newDoc', product: 'docs', path: '', icon: () => <Glyph name="file" /> },
  {
    labelKey: 'quick.upload',
    product: 'drive',
    path: '/?upload=1',
    icon: () => <Glyph name="upload" />,
  },
  {
    // La página existe en `/events/new`, no en `/new`, y solo abre el modal de
    // creación con `?action=create` (mismo intent que «crear evento desde
    // correo»); sin los dos, aterrizaba en la agenda sin nada abierto.
    labelKey: 'quick.newEvent',
    product: 'calendar',
    path: '/events/new?action=create',
    icon: () => <Glyph name="calendar" />,
  },
  {
    labelKey: 'quick.download',
    href: `https://www.${suiteApex}/downloads`,
    icon: () => <Glyph name="download" />,
  },
  { labelKey: 'quick.devices', href: r('/dispositivos'), icon: () => <Glyph name="phone" /> },
  // Llevarse una copia de todo (RGPD art. 20): sin pedírselo a nadie y a un clic.
  { labelKey: 'quick.data', href: r('/datos'), icon: () => <Glyph name="archive" /> },
];

/** Fecha larga del subtítulo (p. ej. «martes, 1 de julio» / «Tuesday, 1 July»). */
function todayLabel(locale: Locale): string {
  return formatDate(locale, new Date(), { weekday: 'long', day: 'numeric', month: 'long' });
}

/** Antigüedad de la actividad reciente, en el idioma de la sesión. */
function relativeTime(locale: Locale, ts?: number): string | null {
  return ts ? formatTimeAgo(locale, ts) : null;
}

/** Nombre corto del producto (sin el prefijo «Sytue »). */
function shortName(app: SuiteApp | undefined, fallback: string): string {
  return app ? app.name.replace('Sytue ', '') : fallback;
}

/**
 * La forma en vigor, leída de donde se APLICA: `data-forma` en `<html>` (ausente es
 * «Marco»). El lanzador es una isla aparte de la `Shell`, así que el `ThemeProvider` de
 * esta no lo envuelve y `useTheme()` aquí no existe; el atributo sí, y cambia en el
 * momento en que alguien elige otra forma en el menú de la cuenta.
 */
function useForma(): ThemeForma {
  const [forma, setForma] = useState<ThemeForma>('marco');
  useEffect(() => {
    const raiz = document.documentElement;
    const leer = (): void => setForma(raiz.dataset.forma === 'carril' ? 'carril' : 'marco');
    leer();
    const vigia = new MutationObserver(leer);
    vigia.observe(raiz, { attributes: true, attributeFilter: ['data-forma'] });
    return () => vigia.disconnect();
  }, []);
  return forma;
}

/** Las píldoras de la portada en «Marco», en su orden: la primera es la principal. */
const ACCIONES_DEL_DIA = ['quick.compose', 'quick.newDoc', 'quick.upload', 'quick.newEvent'];
/** Lo que queda bajo «Más». */
const ACCIONES_EN_MAS = ['quick.download', 'quick.devices', 'quick.data'];
/** La píldora secundaria: superficie blanca con borde, del radio de control. */
const PILDORA =
  'rounded-control border-border bg-surface text-text hover:border-border-strong sytue-focus-ring inline-flex h-8 items-center gap-1 border px-3.5 text-sm font-semibold shadow-[var(--shadow-xs)] transition-colors pointer-coarse:min-h-[max(44px,2.75rem)]';

type Momento = 'morning' | 'afternoon' | 'evening';
/** De 5 a 12, mañana; de 12 a 20, tarde; lo demás, noche. */
export const momentoDelDia = (hora: number): Momento =>
  hora >= 5 && hora < 12 ? 'morning' : hora >= 12 && hora < 20 ? 'afternoon' : 'evening';

export function Launcher({
  locale,
  puedeAdmin,
  puedeAlmacenamientoExterno = false,
  desplegadasIniciales,
  cuentaPersonal = false,
  org = null,
  nombre,
}: {
  locale: Locale;
  /** El nombre de pila, para el saludo de la forma «Marco». Sin él, el saludo solo. */
  nombre?: string | undefined;
  puedeAdmin: boolean;
  /** Resuelto en SSR contra Drive, para cuentas individuales y owner/admin. */
  puedeAlmacenamientoExterno?: boolean | undefined;
  desplegadasIniciales?: string[] | undefined;
  /**
   * Una cuenta personal —una persona cuya organización es su correo— no tiene
   * los productos solo de organizaciones (SyERP, Sytue Web) ni Sytue Admin. Lo decide el SERVIDOR (claim `org_kind`) y llega como
   * prop, igual que `puedeAdmin`: aquí no hay token que leer.
   */
  cuentaPersonal?: boolean | undefined;
  /**
   * La organización activa de la sesión (claim `tenant`), si la hay: es la que
   * llevan los enlaces a los productos compuestos aquí (ADR 0056). La página la
   * pasa desde la sesión —aquí no hay token que leer— y puede ser null en la
   * cuenta sin organización, para la que los enlaces siguen saliendo sin org.
   */
  org?: string | null | undefined;
}) {
  const t = useT(locale);
  // El saludo depende de la hora LOCAL de quien mira, que el servidor no conoce: se
  // decide al montar. Hasta entonces el título reserva su línea, sin texto que cambie.
  const [momento, setMomento] = useState<Momento | null>(null);
  useEffect(() => setMomento(momentoDelDia(new Date().getHours())), []);
  const saludo = momento
    ? nombre
      ? `${t(`launcher.greeting.${momento}`)}, ${nombre}`
      : t(`launcher.greeting.${momento}`)
    : '\u00a0';
  const ahora = useAhora();
  // Qué productos levanta esta instalación (BFF → Hub). Si SSR ya los resolvió,
  // se usan directamente (cero desfase de hidratación). Si no, arranca con lo que
  // el Hub dijo la última vez (localStorage) o undefined (suite entera).
  const [desplegadas, setDesplegadas] = useState<string[] | undefined>(
    desplegadasIniciales ?? appsRecordadas,
  );
  const baseApps = useMemo(
    () =>
      appsPresentables(suiteApps, desplegadas).filter(
        // Fuera el Hub —es donde ya estás— y fuera Sytue Admin si esta persona
        // no puede entrar. Lo segundo lo decide el SERVIDOR y llega como prop:
        // aquí no hay token que leer, y adivinarlo por el rol dejaría sin puerta
        // a los administradores delegados, que entran sin tenerlo.
        (a) => a.key !== CLAVE_DEL_HUB && (a.key !== CLAVE_ADMIN || puedeAdmin),
      ),
    [desplegadas, puedeAdmin],
  );

  const forma = useForma();
  const [recent, setRecent] = useState<SearchResult[] | null>(null);
  const [recentError, setRecentError] = useState(false);
  const [retry, setRetry] = useState(0);
  const [connected, setConnected] = useState<ConnectedApp[]>([]);
  // Quién sirve cada producto en esta organización. Vacío ⇒ todo lo sirve Sytue, que es
  // la degradación correcta si la pasarela no contesta.
  const [productMap, setProductMap] = useState<Record<string, { name: string; href: string }>>({});
  // «Hoy»: la agenda del día y el correo sin leer. `undefined` = cargando;
  // cada parte a `null` cuando su fuente no contesta.
  const [hoy, setHoy] = useState<
    { eventos: EventoDeHoy[] | null; correo: { unread: number } | null } | undefined
  >(undefined);

  // La organización que llevan las teselas (ADR 0056): la de la URL ACTUAL —este
  // lanzador ES el Hub, así que `orgActual` mira el base de esta app— y, si la
  // URL no la lleva (forma transicional, o el HTML previo a la hidratación), la
  // org de la sesión que pasa la página: es a la que el middleware del destino
  // mandaría el enlace sin org con su 302, solo que así aterriza directo. Es la
  // misma con la que ya componen las quick actions de abajo.
  const orgTeselas = orgActual(import.meta.env.BASE_URL) ?? org;

  // Las teselas sustituidas llevan el nombre de la aplicación en su descripción: quien
  // pulsa «Hojas de cálculo» debe saber ANTES del clic que sale a otro producto, no
  // descubrirlo al aterrizar en otro dominio.
  // La descripción viene del catálogo COMÚN: `suiteApps` ya no lleva ninguna
  // (era una copia en castellano que nadie mostraba). Este lanzador es la
  // primera pantalla de la suite.
  const apps = useMemo(() => {
    const tc = createTranslate(locale);
    return baseApps.map((app) => {
      const description = tc(`app.${app.key}.description`);
      const sub = productMap[app.key];
      const disabled = cuentaPersonal && esSoloDeOrganizaciones(app.key);
      // La sustituida manda —es otra aplicación y ahí no hay org de Sytue—; la
      // nativa sale con la org y aterriza sin el 302 (ADR 0056).
      return sub
        ? { ...app, href: sub.href, description: `${description} · ${sub.name}`, disabled }
        : { ...app, href: hrefDe(app.key, orgTeselas), description, disabled };
    });
  }, [baseApps, productMap, locale, cuentaPersonal, orgTeselas]);

  // Un acceso rápido sobre un producto sustituido NO puede concatenar la ruta de Sytue
  // (Google Docs no tiene «/new/document»): lleva a la aplicación elegida sin la ruta.
  const quickActions = useMemo(
    () =>
      QUICK_ACTIONS.map((qa) => {
        if (qa.href)
          return { clave: qa.labelKey, label: t(qa.labelKey), icon: qa.icon, href: qa.href };
        const sub = qa.product ? productMap[qa.product] : undefined;
        if (sub)
          return { clave: qa.labelKey, label: t(qa.labelKey), icon: qa.icon, href: sub.href };
        // `appHref` (el del catálogo, sin org) es el dominio DESNUDO: sirve para
        // la raíz, que Traefik redirige a su prefijo, pero pegarle una ruta
        // profunda detrás se saltaba ese redirect y daba el 404 de Astro.
        // `appUrl` compone con el prefijo real de la app (docs/CONVENTIONS.md §1).
        // Y con la organización (ADR 0056): el destino aterriza ya en la
        // canónica, sin el 302.
        return {
          clave: qa.labelKey,
          label: t(qa.labelKey),
          icon: qa.icon,
          href: appUrl(qa.product!, qa.path ?? '', org ?? undefined),
        };
      }),
    [productMap, t, org],
  );
  // En «Marco», las acciones del día: cuatro píldoras y un «Más». Buscar no está: ya lo
  // tiene la barra de la suite a cualquier ancho.
  const delDia = ACCIONES_DEL_DIA.flatMap((c) => quickActions.filter((qa) => qa.clave === c));
  const enMas = ACCIONES_EN_MAS.flatMap((c) => quickActions.filter((qa) => qa.clave === c));
  // Con las apps en dos filas o menos (tres columnas), «Reciente» baja a la columna de
  // las apps y al lado queda solo «Hoy». Lo sabe el servidor: no se mueve al hidratar.
  const pocas = apps.length <= 6;

  useEffect(() => {
    const controller = new AbortController();
    fetch('/api/product-map', { credentials: 'include', signal: controller.signal })
      .then((res) =>
        res.ok
          ? (res.json() as Promise<{ products?: Record<string, { name: string; href: string }> }>)
          : { products: {} },
      )
      .then((data) => setProductMap(data.products ?? {}))
      .catch(() => {
        /* silencio: sin mapa se abre el producto nativo, que siempre existe. */
      });
    return () => controller.abort();
  }, []);

  useEffect(() => {
    const controller = new AbortController();
    const { from, to } = limitesDelDia(new Date());
    fetch(`/api/hoy?from=${encodeURIComponent(from)}&to=${encodeURIComponent(to)}`, {
      credentials: 'include',
      signal: controller.signal,
    })
      .then((res) =>
        res.ok
          ? (res.json() as Promise<{
              eventos: EventoDeHoy[] | null;
              correo: { unread: number } | null;
            }>)
          : { eventos: null, correo: null },
      )
      .then((data) =>
        setHoy({
          eventos: data.eventos ? ordenarEventos(data.eventos) : null,
          correo: data.correo,
        }),
      )
      .catch(() => {
        if (!controller.signal.aborted) setHoy({ eventos: null, correo: null });
      });
    return () => controller.abort();
  }, []);

  useEffect(() => {
    const controller = new AbortController();
    // Qué hay levantado, via BFF: el token no sale al cliente. Sin respuesta o
    // con lista vacía se presenta todo, que es el comportamiento de siempre.
    fetch('/api/apps', { credentials: 'include', signal: controller.signal })
      .then((res) => (res.ok ? (res.json() as Promise<{ apps?: string[] }>) : { apps: [] }))
      .then((data) => {
        if (Array.isArray(data.apps) && data.apps.length > 0) {
          setDesplegadas(data.apps);
          // Se recuerda también desde aquí: quien entra por el portal deja el
          // conmutador de la topbar ya enterado, y al revés.
          recordarApps(data.apps);
        }
      })
      .catch(() => {
        /* silencio: sin esto el lanzador es el de siempre. */
      });
    return () => controller.abort();
  }, []);

  useEffect(() => {
    const controller = new AbortController();
    // Apps conectadas (integraciones) via BFF: el token nunca sale al cliente. Si el
    // gateway no responde, /api/my-apps degrada a [] y no mostramos la sección.
    fetch('/api/my-apps', { credentials: 'include', signal: controller.signal })
      .then((res) => (res.ok ? (res.json() as Promise<{ apps: ConnectedApp[] }>) : { apps: [] }))
      .then((data) => setConnected(data.apps ?? []))
      .catch(() => {
        /* silencio: sección opcional. */
      });
    return () => controller.abort();
  }, []);

  useEffect(() => {
    const controller = new AbortController();
    setRecent(null);
    setRecentError(false);
    // Actividad reciente via BFF (/api/recent): el token nunca sale al cliente.
    fetch('/api/recent', { credentials: 'include', signal: controller.signal })
      .then((res) => {
        if (!res.ok) throw new Error(`recent ${res.status}`);
        return res.json() as Promise<{ items: SearchResult[] }>;
      })
      .then((data) => setRecent(data.items ?? []))
      .catch(() => {
        if (controller.signal.aborted) return;
        setRecentError(true);
      });
    return () => controller.abort();
  }, [retry]);

  return (
    <div class="marco:py-5 mx-auto w-full max-w-[var(--container-content,1280px)] px-6 py-8">
      {/* Saludo (sobrio, sin hero recargado) */}
      {/* Un solo marcado para las dos formas (variantes `marco:` y `carril:` de
          theme.css). En «Carril», la portada de siempre. En «Marco», «Bienvenida y
          tablero»: saludo, las acciones del día en píldoras, las apps en teselas
          compactas a dos tercios del ancho y, al lado, «Hoy» y «Reciente». */}
      <header class="marco:mb-3 mb-8">
        <p class="sytue-overline text-text-subtle">{todayLabel(locale)}</p>
        {/* El texto más grande de la portada ORIENTA: dice dónde estás. Antes saludaba
            por hora del día («Buenas tardes, Ana»), que es cordial pero no informa de
            nada y ocupa el sitio de más peso visual de la página. El nombre ya está en
            el menú de cuenta. */}
        <h1 class="sytue-h1 text-text mt-1.5">
          <span class="carril:hidden">{saludo}</span>
          <span class="marco:hidden">{t('launcher.heading')}</span>
        </h1>
        {/* Las acciones del día: los accesos rápidos de siempre, en su orden, con la
            primera como principal. En «Carril» siguen en su tarjeta, más abajo. */}
        <nav
          aria-label={t('launcher.quickActions')}
          class="carril:hidden mt-4 flex flex-wrap gap-2"
        >
          {delDia.map((qa, i) =>
            i === 0 ? (
              <Button key={qa.clave} href={qa.href} size="sm" leadingIcon={qa.icon()}>
                {qa.label}
              </Button>
            ) : (
              <a key={qa.clave} href={qa.href} class={PILDORA}>
                {qa.label}
              </a>
            ),
          )}
          {enMas.length > 0 ? (
            <Dropdown
              label={t('launcher.more')}
              align="start"
              trigger={(p) => (
                <button type="button" {...p} class={PILDORA}>
                  {t('launcher.more')}
                  <Icon name="chevronDown" size={14} />
                </button>
              )}
            >
              {(cerrar) =>
                enMas.map((qa) => (
                  <MenuItem key={qa.clave} href={qa.href} icon={qa.icon()} onClick={cerrar}>
                    {qa.label}
                  </MenuItem>
                ))
              }
            </Dropdown>
          ) : null}
        </nav>
      </header>

      {puedeAlmacenamientoExterno ? (
        <Card class="mb-4">
          <a
            href={appUrl('drive', '/?connect-storage=1', orgTeselas ?? undefined)}
            aria-labelledby="connect-storage-title"
            aria-describedby="connect-storage-description"
            class="sytue-focus-ring hover:bg-bg-subtle focus-visible:bg-bg-subtle flex items-center gap-3 rounded-[inherit] p-4 transition-colors"
          >
            <Icon name="link" size={24} class="text-text-muted shrink-0" />
            <span class="min-w-0 flex-1">
              <span id="connect-storage-title" class="text-text block text-sm font-semibold">
                {t('storage.connectTitle')}
              </span>
              <span id="connect-storage-description" class="text-text-muted mt-1 block text-sm">
                {t('storage.connectDescription')}
              </span>
            </span>
            <Icon name="chevronRight" size={16} class="text-text-muted shrink-0" />
          </a>
        </Card>
      ) : null}

      {/* En «Marco», dos columnas desde 1024 px: lo principal y el lado. */}
      <div class="marco:lg:grid marco:lg:grid-cols-[minmax(0,1.9fr)_minmax(260px,1fr)] marco:lg:items-start marco:lg:gap-4">
        <div class={cx('min-w-0', pocas && 'marco:lg:col-start-1 marco:lg:row-start-1')}>
          {/* Lanzadores de producto */}
          <section aria-labelledby="apps-heading" class="marco:lg:last:mb-0 mb-8">
            <h2 id="apps-heading" class="sytue-overline text-text-muted mb-4">
              {t('launcher.apps')}
            </h2>
            <ul class="marco:grid-cols-[repeat(auto-fill,minmax(160px,1fr))] marco:gap-2.5 marco:lg:grid-cols-3 grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-5">
              {apps.map((app) => {
                const disabled = app.disabled;
                const caption = disabled
                  ? `${app.name} — ${app.description} (${t('launcher.disabled')})`
                  : t('launcher.appAria', { name: app.name, description: app.description });
                return (
                  <li key={app.key}>
                    <a
                      href={disabled ? undefined : app.href}
                      aria-disabled={disabled ? 'true' : undefined}
                      aria-label={caption}
                      title={caption}
                      style={{
                        '--hub-product-color': app.color,
                      }}
                      onClick={disabled ? (e) => e.preventDefault() : undefined}
                      class={cx(
                        // En «Marco», tesela compacta: el icono a la izquierda (42 px) y el
                        // nombre y la descripción, en una línea cada uno, a la derecha.
                        'hub-product-card group border-border-subtle rounded-card flex h-full flex-col items-start gap-3 border p-4 transition-colors',
                        'marco:flex-row marco:items-center marco:px-3 marco:py-2.5 marco:[&>span:first-child]:scale-105',
                        disabled
                          ? 'cursor-not-allowed opacity-40 grayscale'
                          : 'hover:border-border-strong',
                      )}
                    >
                      {/* El icono va SOLO. El SVG del producto ya ES la tesela —lleva su
                      `rect` redondeado con el color de la app y el glifo en blanco— y
                      ProductIcon le añade encima su anillo. Envolverlo en otra caja
                      teñida del mismo tono apilaba halo sobre badge sobre tarjeta sobre
                      página; y el selector de aplicaciones, a un clic de aquí, pinta
                      estas mismas marcas desnudas (AppGrid). Dos tratamientos de lo
                      mismo tan cerca se leen como un descuido. */}
                      <ProductIcon key={app.key} appKey={app.key} size={40} />
                      <span class="flex w-full min-w-0 flex-col">
                        <span class="flex items-center justify-between gap-1">
                          <span
                            class={cx(
                              'truncate font-semibold',
                              disabled ? 'text-text-muted' : 'text-text',
                            )}
                          >
                            {app.name.replace('Sytue ', '')}
                          </span>
                          {disabled ? (
                            <span class="text-text-muted bg-bg-subtle border-border shrink-0 rounded border px-1.5 py-0.5 text-[10px] font-medium tracking-wider uppercase">
                              {t('launcher.disabled')}
                            </span>
                          ) : null}
                        </span>
                        <span class="text-text-muted marco:line-clamp-1 line-clamp-2 text-sm leading-snug">
                          {app.description}
                        </span>
                      </span>
                    </a>
                  </li>
                );
              })}
            </ul>
          </section>

          {/* Apps conectadas (integraciones): productos de terceros dados de alta por el
          admin, bajo las apps propias. Solo se muestra si hay alguna. */}
          {connected.length > 0 ? (
            <section aria-labelledby="connected-heading" class="marco:lg:last:mb-0 mb-8">
              <h2 id="connected-heading" class="sytue-overline text-text-muted mb-4">
                {t('launcher.connected')}
              </h2>
              <ul class="marco:grid-cols-[repeat(auto-fill,minmax(160px,1fr))] marco:gap-2.5 marco:lg:grid-cols-3 grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-5">
                {connected.map((app) => (
                  <li key={app.key}>
                    <a
                      href={app.href}
                      target="_blank"
                      rel="noopener noreferrer"
                      aria-label={t('launcher.appAria', {
                        name: app.name,
                        description: app.description,
                      })}
                      title={t('launcher.appAria', {
                        name: app.name,
                        description: app.description,
                      })}
                      class="group border-border bg-surface hover:border-border-strong flex h-full flex-col items-start gap-3 rounded-lg border p-4 transition-colors"
                    >
                      {/* Aquí la caja SÍ vale: el icono lo pone un tercero y su forma es
                      impredecible, así que darle geometría estable evita que cada
                      tarjeta mida distinto. Lo que sobra es el TINTE con el color de
                      la app: teñir el logotipo de otro con nuestro acento no es
                      nuestro. Fondo neutro del sistema. */}
                      <span class="bg-bg-subtle flex h-12 w-12 shrink-0 items-center justify-center rounded-lg">
                        {app.iconUrl ? (
                          <img src={app.iconUrl} alt="" width={28} height={28} aria-hidden="true" />
                        ) : null}
                      </span>
                      <span class="flex w-full min-w-0 flex-col">
                        <span class="text-text truncate font-semibold">{app.name}</span>
                        <span class="text-text-muted line-clamp-2 text-sm leading-snug">
                          {app.description}
                        </span>
                      </span>
                    </a>
                  </li>
                ))}
              </ul>
            </section>
          ) : null}
        </div>
        <div
          class={cx(
            'marco:gap-2.5 marco:lg:grid-cols-1 grid items-start gap-6 lg:grid-cols-3',
            // Con pocas apps el lado se deshace: «Hoy» ocupa la columna derecha entera
            // y «Reciente» baja a la de las apps. Solo en dos columnas (≥1024 px).
            pocas && 'marco:lg:contents',
          )}
        >
          {/* Hoy y Accesos rápidos, APILADAS en la primera columna, y lo reciente en las
            otras dos. Iban en fila, una columna cada una, con lo reciente debajo a lo
            ancho: la tercera columna quedaba vacía —un hueco de 420×433 px a 1280×800,
            medido contra producción el 2026-09-21— y «Reciente en toda la suite»
            empezaba en y=1045, fuera de la primera pantalla. Así era antes de que
            llegara «Hoy» (Accesos 1 + Reciente 2), y al añadirla se partió la fila.
            El orden del DOM no cambia: Hoy, Accesos, Reciente. */}
          <div
            class={cx(
              'flex min-w-0 flex-col gap-6',
              pocas && 'marco:lg:col-start-2 marco:lg:row-span-2 marco:lg:row-start-1',
            )}
          >
            {/* Hoy: la agenda del día y el correo sin leer (docs/design.md §2.4: el Hub es
            la rejilla de apps MÁS las tarjetas de resumen). Cada parte falla por su
            lado, y la tarjeta lo dice sin dejar la otra a oscuras. */}
            <section aria-label={t('today.title')} class="min-w-0">
              <Card class="overflow-hidden">
                <SectionHeader
                  title={t('today.title')}
                  actions={
                    <a
                      href={apps.find((a) => a.key === 'calendar')?.href ?? appHref('calendar')}
                      class="text-primary-text sytue-focus-ring -my-1 rounded-sm py-1 text-sm font-medium hover:underline"
                    >
                      {t('today.openCalendar')}
                    </a>
                  }
                />
                <CardBody class="p-0">
                  {hoy === undefined ? (
                    <div class="flex flex-col gap-2 p-4">
                      <Skeleton width="70%" height="0.875rem" />
                      <Skeleton width="55%" height="0.875rem" />
                    </div>
                  ) : hoy.eventos === null ? (
                    <p class="text-text-muted px-4 py-3 text-sm">{t('today.unavailable')}</p>
                  ) : hoy.eventos.length === 0 ? (
                    <p class="text-text-muted px-4 py-3 text-sm">{t('today.noEvents')}</p>
                  ) : (
                    <ul class="divide-border divide-y">
                      {hoy.eventos.map((e) => {
                        const pasado = haTerminado(e, ahora);
                        return (
                          <li
                            key={e.id}
                            class={`marco:py-1 flex items-baseline gap-3 px-4 py-2 ${pasado ? 'opacity-60' : ''}`}
                          >
                            <span class="text-text-muted w-12 shrink-0 text-xs tabular-nums">
                              {e.allDay ? t('today.allDay') : formatTime(locale, e.start)}
                            </span>
                            <span
                              class="text-text min-w-0 flex-1 truncate text-sm"
                              title={e.summary}
                            >
                              {e.summary || t('today.untitled')}
                            </span>
                          </li>
                        );
                      })}
                    </ul>
                  )}
                  {/* El correo, en una línea al pie: el número es lo que se mira. */}
                  {hoy && hoy.correo !== null ? (
                    <a
                      href={apps.find((a) => a.key === 'mail')?.href ?? appHref('mail')}
                      class="border-border text-text hover:bg-bg-subtle focus-visible:bg-bg-subtle marco:py-2 flex items-center gap-3 border-t px-4 py-2.5 text-sm transition-colors duration-150"
                    >
                      <ProductIcon appKey="mail" size={20} class="shrink-0" />
                      <span class="min-w-0 flex-1 truncate">
                        {hoy.correo.unread > 0
                          ? t('today.unread', { count: hoy.correo.unread })
                          : t('today.inboxZero')}
                      </span>
                    </a>
                  ) : null}
                </CardBody>
              </Card>
            </section>

            {/* Accesos rápidos */}
            <section aria-label={t('launcher.quickActions')} class="marco:hidden min-w-0">
              <Card class="overflow-hidden">
                <SectionHeader title={t('launcher.quickActions')} />
                {/* `sytue-tiles` y `cardTone`: filas con el color en el icono en la forma
                  «Carril»; en «Marco», teselas de color pleno (tokens/appearance.css). */}
                <CardBody class="sytue-tiles flex flex-col gap-1 p-2">
                  {quickActions.map((qa, i) => (
                    <a
                      // Por rótulo y no por destino: si la organización sirve dos productos
                      // con la MISMA aplicación de terceros, dos accesos comparten href.
                      key={qa.label}
                      href={qa.href}
                      class={cx(
                        'text-text hover:bg-bg-subtle focus-visible:bg-bg-subtle active:bg-surface-sunken flex items-center gap-3 rounded-md px-3 py-2 text-base font-medium transition-colors',
                        cardTone(TONOS_ACCESO[i % TONOS_ACCESO.length]!).class,
                      )}
                      style={cardTone(TONOS_ACCESO[i % TONOS_ACCESO.length]!).style}
                    >
                      {/* Icono sobre relleno sólido, un color de grupo por acceso: es lo
                        que los distingue de un vistazo, sin teñir la fila. */}
                      <CardIcon tone={TONOS_ACCESO[i % TONOS_ACCESO.length]!}>{qa.icon()}</CardIcon>
                      <span class="min-w-0 truncate">{qa.label}</span>
                    </a>
                  ))}
                </CardBody>
              </Card>
            </section>
          </div>

          {/* Actividad reciente (Hub) */}
          <section
            aria-label={t('launcher.recentAll')}
            class={cx(
              'marco:lg:col-span-1 min-w-0 lg:col-span-2',
              pocas && 'marco:lg:col-start-1 marco:lg:row-start-2',
            )}
          >
            {/* overflow-hidden: la cabecera sólida y las filas llegan al borde sin pisar las esquinas. */}
            <Card class="overflow-hidden">
              <SectionHeader
                title={t('launcher.recentAll')}
                actions={
                  <a
                    href={r('/actividad')}
                    // `py-1 -my-1`: 26 px de diana (WCAG 2.5.8 pide 24) sin mover un
                    // píxel la cabecera. Era el ÚNICO objetivo por debajo del suelo en
                    // las nueve apps —medía 65x18— y no le vale la excepción de enlace
                    // en línea porque va suelto, no dentro de un párrafo. Mismo remedio
                    // que el botón de quitar etiqueta de Sytue Mail.
                    class="text-primary-text sytue-focus-ring -my-1 inline-flex items-center gap-1 rounded-sm py-1 text-sm font-semibold hover:underline"
                  >
                    {t('recent.viewAll')}
                    <span aria-hidden="true">→</span>
                  </a>
                }
              />
              <CardBody class="p-0">
                {recent === null && !recentError ? (
                  /* Carga: filas Skeleton con la geometría real de la lista (sin salto de layout). */
                  <div class="divide-border divide-y" aria-busy="true">
                    <p class="sr-only" role="status">
                      {t('recent.loading')}
                    </p>
                    {[0, 1, 2, 3].map((i) => (
                      <div key={i} class="flex items-center gap-3 px-4 py-3">
                        <Skeleton circle width="10px" height="10px" />
                        <div class="flex min-w-0 flex-1 flex-col gap-2">
                          <Skeleton width={i % 2 ? '45%' : '60%'} height="0.875rem" />
                          <Skeleton width={i % 2 ? '70%' : '35%'} height="0.75rem" />
                        </div>
                        <Skeleton width="3rem" height="0.75rem" />
                      </div>
                    ))}
                  </div>
                ) : recentError ? (
                  <EmptyState
                    title={t('launcher.activityError')}
                    description={t('recent.hubDown')}
                    illustration={<Icon name="warning" size={32} class="text-danger-text" />}
                    action={
                      <Button variant="secondary" size="sm" onClick={() => setRetry((n) => n + 1)}>
                        {t('recent.retry')}
                      </Button>
                    }
                  />
                ) : recent && recent.length === 0 ? (
                  <EmptyState
                    title={t('launcher.noActivity')}
                    description={t('recent.emptyBody')}
                    illustration={<Illustration name="emptySearch" size={120} />}
                    action={
                      // Enlace de verdad (`href`), no un botón que navega: se abre en otra
                      // pestaña con Cmd/Ctrl, se copia con el botón derecho y el lector de
                      // pantalla lo anuncia como lo que es. Y respeta la aplicación elegida
                      // por la organización: mandar a Sytue Drive a quien usa otra sería
                      // mandarlo a una carpeta vacía.
                      <Button
                        variant="secondary"
                        size="sm"
                        href={productMap['drive']?.href ?? appHref('drive')}
                      >
                        {t('recent.emptyCta')}
                      </Button>
                    }
                  />
                ) : (
                  <ul class="divide-border divide-y">
                    {/* En «Marco», solo las tres últimas: el resto está en «Ver todo». */}
                    {recientesVisibles(recent!, forma).map((item) => {
                      const app = appForKey(item.source);
                      const when = relativeTime(locale, item.timestamp);
                      const detalle = detalleDe(locale, item);
                      return (
                        <li key={`${item.source}-${item.id}`}>
                          <a
                            href={item.url}
                            class="hover:bg-bg-subtle focus-visible:bg-bg-subtle active:bg-surface-sunken marco:py-2 flex items-start gap-3 px-4 py-3 transition-colors"
                          >
                            {/* La tesela del producto y no un punto de su color: el punto no
                              decía nada —cuatro verdes seguidos— y la tesela es la misma
                              del conmutador, así que se reconoce de un vistazo. */}
                            {app ? (
                              <ProductIcon appKey={app.key} size={20} class="mt-0.5 shrink-0" />
                            ) : (
                              <span
                                class="bg-primary mt-1.5 inline-block h-2.5 w-2.5 shrink-0 rounded-full"
                                aria-hidden="true"
                              />
                            )}
                            <span class="flex min-w-0 flex-1 flex-col">
                              <span class="text-text truncate font-medium" title={item.title}>
                                {item.title}
                              </span>
                              {detalle ? (
                                <span class="text-text-muted truncate text-sm" title={detalle}>
                                  {detalle}
                                </span>
                              ) : null}
                            </span>
                            <span class="flex shrink-0 flex-col items-end gap-0.5 text-xs">
                              <span class="text-text-muted font-medium capitalize">
                                {shortName(app, item.source)}
                              </span>
                              {when ? (
                                <span class="text-text-subtle tabular-nums">{when}</span>
                              ) : null}
                            </span>
                          </a>
                        </li>
                      );
                    })}
                  </ul>
                )}
              </CardBody>
            </Card>
          </section>
        </div>
      </div>
    </div>
  );
}
