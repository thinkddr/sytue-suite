/**
 * Bandeja de notificaciones de Sytue One — la vista COMPLETA de lo que el
 * popover de la shell solo asoma: filtros, marcar todo y las reglas de silencio,
 * que el Hub servía desde el principio y no tenían una sola pantalla.
 *
 * Isla propia y no parte de la shell: los slots de una isla Astro se hidratan
 * por separado, así que aquí no llega su contexto. Construye su HubClient con el
 * mismo getter de token que ella (`/auth/token`, en memoria), que es lo que
 * evita que el access token se persista en ningún sitio.
 */
import type { JSX } from 'preact';
import { useEffect, useMemo, useState } from 'preact/hooks';
import { HubClient, bearer } from '@sytue/api-clients';
import type { ProductKey } from '@sytue/config';
import type { Notification } from '@sytue/api-clients';
import {
  appForKey,
  Button,
  Dropdown,
  EmptyState,
  Icon,
  IconButton,
  MenuItem,
  ProductIcon,
  Skeleton,
  useAhora,
} from '@sytue/ui';
import { formatDate, formatTimeAgo, translate } from '@sytue/i18n';
import type { Locale } from '@sytue/i18n';
import { getAccessToken } from '@sytue/auth/client';
import {
  abrirAviso,
  conectarAvisos,
  MarcarLeido,
  cuerpoAviso,
  destinoAviso,
  procedenciaAviso,
  tituloAviso,
  type Avisos,
  type EstadoAvisos,
} from '@sytue/shell';
import { useT } from '../i18n/index.js';
import { PreferenciasAvisos } from './PreferenciasAvisos.js';
import { agruparPorDia, dayLabelKey } from '../lib/activity.js';
import { clicAparte, silenciableDesdeElAviso } from '../lib/bandeja.js';

interface Props {
  locale: Locale;
  hubBaseUrl: string;
  /** Organización activa: las reglas de silencio son de ella (ver `PreferenciasAvisos`). */
  organizacion?: string | undefined;
  personal?: boolean;
}

type Filtro = 'all' | 'unread';

export function NotificationsInbox({
  locale,
  hubBaseUrl,
  organizacion,
  personal = false,
}: Props): JSX.Element {
  const t = useT(locale);
  const hub = useMemo(
    () => new HubClient({ baseUrl: hubBaseUrl, auth: bearer(getAccessToken) }),
    [hubBaseUrl],
  );

  // Lista, contador, flujo con reconexión y «leído»: lo mismo que la campana
  // (`conectarAvisos`). Esta bandeja lo tenía escrito aparte y sin escuchar `error`:
  // al cortarse el flujo se quedaba sorda hasta recargar.
  const [estado, setEstado] = useState<EstadoAvisos | null>(null);
  // En estado y no en un ref: mutar `.current` dentro del efecto hacía que el React
  // Compiler se saltara el componente entero (react/immutability).
  const [avisos, setAvisos] = useState<Avisos | null>(null);
  const [filtro, setFiltro] = useState<Filtro>('all');
  const [producto, setProducto] = useState<ProductKey | null>(null);

  useEffect(() => {
    const a = conectarAvisos(hub, setEstado, { limite: 100 });
    setAvisos(a);
    return () => a.cerrar();
  }, [hub]);

  const items = estado && !estado.cargando ? estado.items : null;
  const error = estado?.fallo === true && (estado.items.length ?? 0) === 0;
  // Con un producto elegido, «Marcar todas» marca las de ESE producto: es lo que se
  // está viendo. Antes se llevaba también los avisos de los demás.
  const marcarTodas = () => void avisos?.leerTodos(producto ?? undefined);
  // Lo que acaba de pasar con un aviso desde su menú: se dice en una región `status`.
  const [hecho, setHecho] = useState<'' | 'silenciado' | 'fallo'>('');
  // Cambiarla vuelve a montar «Qué quieres recibir», que relee las reglas: lo que se
  // acaba de silenciar desde un aviso aparece ahí ya apagado.
  const [versionReglas, setVersionReglas] = useState(0);
  // «Hoy» y «Ayer» contra un reloj que avanza (cada minuto): con la bandeja abierta
  // de un día para otro, lo de ayer deja de llamarse «Hoy» solo.
  const ahora = useAhora();

  async function silenciarComoEste(n: Notification): Promise<void> {
    setHecho('');
    try {
      await hub.setNotificationPref({ source: n.source, kind: n.type ?? '', muted: true });
      setHecho('silenciado');
      setVersionReglas((v) => v + 1);
    } catch {
      setHecho('fallo');
    }
  }

  const todas = items ?? [];
  // El número de la base, no el de los cien cargados.
  const sinLeer = estado?.sinLeer ?? 0;
  // Los productos que HAN emitido algo: ofrecer un filtro vacío es ruido.
  const productos = [...new Set(todas.map((n) => n.source))];
  const visibles = todas.filter(
    (n) => (filtro === 'all' || !n.read) && (producto === null || n.source === producto),
  );

  const chip = (activo: boolean): string =>
    'sytue-focus-ring inline-flex items-center gap-1.5 rounded-control border px-3 py-1.5 text-sm font-medium transition-colors ' +
    (activo
      ? 'border-primary bg-primary text-[var(--sytue-on-primary)]'
      : 'border-border text-text-muted hover:border-text-subtle hover:text-text');

  return (
    <div class="flex flex-col gap-6">
      <div class="flex flex-wrap items-center justify-between gap-3">
        {/* `aria-pressed` en todos: el único indicio de qué filtro está puesto era el
            relleno primario, así que con lector de pantalla —o sin distinguir el
            color— la bandeja no decía por qué faltaban avisos. La versión enlazada de
            esta misma barra (actividad) ya lo dice con `aria-current`. */}
        <nav aria-label={t('inbox.filterBy')} class="flex flex-wrap gap-2">
          <button
            type="button"
            aria-pressed={filtro === 'all'}
            class={chip(filtro === 'all')}
            onClick={() => setFiltro('all')}
          >
            {t('inbox.all')}
          </button>
          <button
            type="button"
            aria-pressed={filtro === 'unread'}
            class={chip(filtro === 'unread')}
            onClick={() => setFiltro('unread')}
          >
            {t('inbox.unreadFilter', { count: sinLeer })}
          </button>
          {productos.map((key) => {
            const app = appForKey(key);
            return (
              <button
                key={key}
                type="button"
                aria-pressed={producto === key}
                class={chip(producto === key)}
                onClick={() => setProducto(producto === key ? null : key)}
              >
                <ProductIcon appKey={key} size={14} />
                {app ? app.name.replace('Sytue ', '') : key}
              </button>
            );
          })}
        </nav>
        {(producto === null ? sinLeer > 0 : visibles.some((n) => !n.read)) ? (
          <Button variant="secondary" onClick={marcarTodas}>
            {producto === null
              ? t('inbox.markAll')
              : t('inbox.markAllOf', {
                  app: appForKey(producto)?.name.replace('Sytue ', '') ?? producto,
                })}
          </Button>
        ) : null}
      </div>

      {items === null && !error ? (
        <div class="divide-border divide-y" aria-busy="true">
          <p class="sr-only" role="status">
            {t('inbox.loading')}
          </p>
          {[0, 1, 2, 3].map((i) => (
            <div key={i} class="flex items-center gap-3 px-1 py-3">
              <Skeleton circle width="18px" height="18px" />
              <div class="flex min-w-0 flex-1 flex-col gap-2">
                <Skeleton width="45%" height="14px" />
                <Skeleton width="70%" height="12px" />
              </div>
            </div>
          ))}
        </div>
      ) : error ? (
        <EmptyState
          tone="error"
          title={t('inbox.errorTitle')}
          description={t('inbox.errorBody')}
          action={
            <Button variant="secondary" onClick={() => avisos?.reintentar()}>
              {t('inbox.retry')}
            </Button>
          }
        />
      ) : visibles.length === 0 ? (
        estado?.hayMas ? (
          // Con más detrás, «nada que leer» o «estás al día» serían falsos: solo no hay
          // entre lo cargado, y el botón de abajo trae lo siguiente.
          <EmptyState title={translate(locale, 'notifications.noneLoaded')} />
        ) : (
          <EmptyState
            title={filtro === 'unread' ? t('inbox.emptyUnreadTitle') : t('inbox.emptyTitle')}
            description={filtro === 'unread' ? t('inbox.emptyUnreadBody') : t('inbox.emptyBody')}
          />
        )
      ) : (
        // Cortada por día, como la actividad: «Hoy», «Ayer» y luego fechas. Con cien
        // avisos seguidos no se sabía dónde acababa lo de esta mañana.
        <div class="flex flex-col gap-6">
          {agruparPorDia(visibles, (n) => n.created_at).map((grupo) => {
            const clave = dayLabelKey(grupo.day, ahora);
            const rotulo =
              clave === 'today'
                ? t('activity.today')
                : clave === 'yesterday'
                  ? t('activity.yesterday')
                  : formatDate(locale, grupo.at, {
                      day: 'numeric',
                      month: 'long',
                      year: 'numeric',
                    });
            return (
              <section key={grupo.day} aria-label={rotulo}>
                <h2 class="sytue-overline text-text-subtle border-border mb-1 border-b pb-2">
                  {rotulo}
                </h2>
                <ul class="divide-border divide-y">
                  {grupo.items.map((n) => (
                    <FilaAviso
                      key={n.id}
                      n={n}
                      t={t}
                      locale={locale}
                      avisos={avisos}
                      alSilenciar={(x) => void silenciarComoEste(x)}
                    />
                  ))}
                </ul>
              </section>
            );
          })}
        </div>
      )}
      {/* Siempre montada: lo que pasó con el último «Silenciar avisos como este». */}
      <p
        role="status"
        class={hecho === 'fallo' ? 'text-danger-text text-sm' : 'text-text-muted text-sm'}
      >
        {hecho === 'silenciado'
          ? t('inbox.mutedLikeThis')
          : hecho === 'fallo'
            ? t('inbox.muteFailed')
            : ''}
      </p>
      {estado?.hayMas && items && items.length > 0 ? (
        <div>
          <Button variant="secondary" size="sm" onClick={() => void avisos?.masAntiguos()}>
            {translate(locale, 'notifications.loadMore')}
          </Button>
        </div>
      ) : null}

      {/* Reglas de silencio. Al final y no en otra página: quien viene a mirar
          sus avisos es quien decide cuáles quiere, y son dos clics desde aquí. Van
          aparte a propósito: si el Hub no las sirve, la bandeja sigue siendo útil y
          solo se cae esa sección. */}
      <PreferenciasAvisos
        key={versionReglas}
        locale={locale}
        hub={hub}
        organizacion={organizacion}
        personal={personal}
      />
    </div>
  );
}

/**
 * Un aviso de la bandeja: la fila que lo abre (o lo marca, si no lleva a ningún
 * sitio), el punto de «sin leer» y su menú «…» con lo que se hace con UN aviso sin
 * abrirlo: devolverlo a «sin leer» y silenciar los que son como él.
 */
function FilaAviso({
  n,
  t,
  locale,
  avisos,
  alSilenciar,
}: {
  n: Notification;
  t: ReturnType<typeof useT>;
  locale: Locale;
  avisos: Avisos | null;
  alSilenciar: (n: Notification) => void;
}): JSX.Element {
  const app = appForKey(n.source);
  const destino = destinoAviso(n);
  const contenido = (
    <>
      <span class="mt-0.5 shrink-0">
        <ProductIcon appKey={n.source} size={18} />
      </span>
      <span class="min-w-0 flex-1">
        <span class={'text-text block text-sm ' + (n.read ? 'font-medium' : 'font-semibold')}>
          {tituloAviso(locale, n)}
        </span>
        {cuerpoAviso(locale, n) ? (
          <span class="text-text-muted mt-0.5 block text-sm">{cuerpoAviso(locale, n)}</span>
        ) : null}
        <span class="text-text-subtle mt-1 block text-xs">
          {/* De otra organización, o anterior al registro de avisos. */}
          {procedenciaAviso(locale, n).map((p) => `${p} · `)}
          {app ? app.name.replace('Sytue ', '') : n.source} ·{' '}
          {formatTimeAgo(locale, new Date(n.created_at))}
        </span>
      </span>
      {/* El punto se ve fuera, en su botón; aquí se dice para quien no lo ve. */}
      {!n.read ? <span class="sr-only">{t('inbox.unreadDot')}</span> : null}
    </>
  );
  const conMenu = n.read || silenciableDesdeElAviso(n);
  return (
    <li class="flex items-start">
      {destino ? (
        <a
          href={destino}
          onClick={(e) => {
            // Ctrl/⌘/Mayús/rueda: el navegador lo abre aparte y la bandeja se queda;
            // aquí solo se marca leído. Lo demás navega DESPUÉS del «leído»: antes la
            // navegación cancelaba la petición y el aviso volvía sin leer.
            if (clicAparte(e)) {
              void avisos?.leer(n.id);
              return;
            }
            e.preventDefault();
            if (avisos) void abrirAviso(avisos, n, (u) => window.location.assign(u));
          }}
          onAuxClick={(e) => {
            if (e.button === 1) void avisos?.leer(n.id);
          }}
          class="hover:bg-chrome-hover sytue-focus-ring flex min-w-0 flex-1 items-start gap-3 rounded-md px-1 py-3 transition-colors"
        >
          {contenido}
        </a>
      ) : (
        <button
          type="button"
          onClick={() => void avisos?.leer(n.id)}
          class="hover:bg-chrome-hover sytue-focus-ring flex min-w-0 flex-1 items-start gap-3 rounded-md px-1 py-3 text-left transition-colors"
        >
          {contenido}
        </button>
      )}
      {/* El punto azul es un botón: marca ESTE aviso como leído sin abrirlo. Va
          fuera de la fila (un botón dentro de un enlace no es HTML válido). */}
      {!n.read ? (
        <MarcarLeido
          class="mt-3"
          rotulo={t('inbox.markRead')}
          alMarcar={() => void avisos?.leer(n.id)}
        />
      ) : null}
      {conMenu ? (
        <Dropdown
          align="end"
          label={t('inbox.actionsOf', { title: tituloAviso(locale, n) })}
          trigger={(p) => (
            <span class="mt-2">
              <IconButton
                {...p}
                size="sm"
                label={t('inbox.actionsOf', { title: tituloAviso(locale, n) })}
              >
                <Icon name="ellipsis" />
              </IconButton>
            </span>
          )}
        >
          {(cerrar) => (
            <>
              {n.read ? (
                <MenuItem
                  onClick={() => {
                    cerrar();
                    void avisos?.desleer(n.id);
                  }}
                >
                  {t('inbox.markUnread')}
                </MenuItem>
              ) : null}
              {silenciableDesdeElAviso(n) ? (
                <MenuItem
                  onClick={() => {
                    cerrar();
                    alSilenciar(n);
                  }}
                >
                  {t('inbox.muteLikeThis')}
                </MenuItem>
              ) : null}
            </>
          )}
        </Dropdown>
      ) : null}
    </li>
  );
}
