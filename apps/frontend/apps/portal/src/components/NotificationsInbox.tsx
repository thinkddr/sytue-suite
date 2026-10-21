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
import type { NotificationPref } from '@sytue/api-clients';
import type { ProductKey } from '@sytue/config';
import { appForKey, Button, EmptyState, ProductIcon, Skeleton, Switch } from '@sytue/ui';
import { formatTimeAgo, translate } from '@sytue/i18n';
import type { Locale } from '@sytue/i18n';
import { getAccessToken } from '@sytue/auth/client';
import {
  abrirAviso,
  conectarAvisos,
  cuerpoAviso,
  destinoAviso,
  procedenciaAviso,
  tituloAviso,
  type Avisos,
  type EstadoAvisos,
} from '@sytue/shell';
import { useT } from '../i18n/index.js';

interface Props {
  locale: Locale;
  hubBaseUrl: string;
}

type Filtro = 'all' | 'unread';

export function NotificationsInbox({ locale, hubBaseUrl }: Props): JSX.Element {
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
  const [prefs, setPrefs] = useState<NotificationPref[]>([]);
  const [intento, setIntento] = useState(0);
  const [filtro, setFiltro] = useState<Filtro>('all');
  const [producto, setProducto] = useState<ProductKey | null>(null);

  useEffect(() => {
    const a = conectarAvisos(hub, setEstado, { limite: 100 });
    setAvisos(a);
    return () => a.cerrar();
  }, [hub]);

  // Las preferencias van aparte a propósito: si el Hub no las sirve, la bandeja sigue
  // siendo útil y solo se cae esa sección.
  useEffect(() => {
    let cancelado = false;
    void hub
      .notificationPrefs()
      .then((p) => {
        if (!cancelado) setPrefs(p);
      })
      .catch(() => undefined);
    return () => {
      cancelado = true;
    };
  }, [hub, intento]);

  const items = estado && !estado.cargando ? estado.items : null;
  const error = estado?.fallo === true && (estado.items.length ?? 0) === 0;
  const marcarTodas = () => void avisos?.leerTodos();

  const silenciar = async (source: ProductKey, muted: boolean) => {
    setPrefs((prev) => [
      ...prev.filter((p) => !(p.source === source && !p.kind)),
      { source, muted },
    ]);
    await hub.setNotificationPref({ source, muted }).catch(() => undefined);
  };

  const silenciado = (source: ProductKey): boolean =>
    prefs.some((p) => p.source === source && !p.kind && p.muted);

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
        {sinLeer > 0 ? (
          <Button variant="secondary" onClick={marcarTodas}>
            {t('inbox.markAll')}
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
          title={t('inbox.errorTitle')}
          description={t('inbox.errorBody')}
          action={
            <Button
              variant="secondary"
              onClick={() => {
                avisos?.reintentar();
                setIntento((n) => n + 1);
              }}
            >
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
        <ul class="divide-border divide-y">
          {visibles.map((n) => {
            const app = appForKey(n.source);
            const contenido = (
              <>
                <span class="mt-0.5 shrink-0">
                  <ProductIcon appKey={n.source} size={18} />
                </span>
                <span class="min-w-0 flex-1">
                  <span
                    class={'text-text block text-sm ' + (n.read ? 'font-medium' : 'font-semibold')}
                  >
                    {tituloAviso(locale, n)}
                  </span>
                  {cuerpoAviso(locale, n) ? (
                    <span class="text-text-muted mt-0.5 block text-sm">
                      {cuerpoAviso(locale, n)}
                    </span>
                  ) : null}
                  <span class="text-text-subtle mt-1 block text-xs">
                    {/* De otra organización, o anterior al registro de avisos. */}
                    {procedenciaAviso(locale, n).map((p) => `${p} · `)}
                    {app ? app.name.replace('Sytue ', '') : n.source} ·{' '}
                    {formatTimeAgo(locale, new Date(n.created_at))}
                  </span>
                </span>
                {!n.read ? (
                  <span
                    class="bg-primary mt-2 h-2 w-2 shrink-0 rounded-full"
                    aria-label={t('inbox.unreadDot')}
                  />
                ) : null}
              </>
            );
            return (
              <li key={n.id}>
                {destinoAviso(n) ? (
                  <a
                    href={destinoAviso(n) ?? undefined}
                    onClick={(e) => {
                      // Se navega DESPUÉS del «leído»: antes la navegación cancelaba la
                      // petición y el aviso volvía sin leer.
                      e.preventDefault();
                      if (avisos) {
                        void abrirAviso(avisos, n, (u) => window.location.assign(u));
                      }
                    }}
                    class="hover:bg-chrome-hover sytue-focus-ring flex items-start gap-3 rounded-md px-1 py-3 transition-colors"
                  >
                    {contenido}
                  </a>
                ) : (
                  <button
                    type="button"
                    onClick={() => void avisos?.leer(n.id)}
                    class="hover:bg-chrome-hover sytue-focus-ring flex w-full items-start gap-3 rounded-md px-1 py-3 text-left transition-colors"
                  >
                    {contenido}
                  </button>
                )}
              </li>
            );
          })}
        </ul>
      )}
      {estado?.hayMas && items && items.length > 0 ? (
        <div>
          <Button variant="secondary" size="sm" onClick={() => void avisos?.masAntiguos()}>
            {translate(locale, 'notifications.loadMore')}
          </Button>
        </div>
      ) : null}

      {/* Reglas de silencio. Al final y no en otra página: quien viene a mirar
          sus avisos es quien decide cuáles quiere, y son dos clics desde aquí. */}
      <section aria-labelledby="prefs-title" class="border-border mt-2 border-t pt-6">
        <h2 id="prefs-title" class="text-text text-base font-semibold">
          {t('inbox.prefsTitle')}
        </h2>
        <p class="text-text-muted mt-1 mb-4 max-w-2xl text-sm">{t('inbox.prefsBody')}</p>
        <div class="flex flex-col gap-3">
          {(['mail', 'drive', 'docs', 'calendar', 'contacts', 'rooms'] as ProductKey[]).map(
            (key) => {
              const app = appForKey(key);
              return (
                <Switch
                  key={key}
                  label={app ? app.name : key}
                  checked={!silenciado(key)}
                  onChange={(e) => {
                    // El interruptor dice «recibir»; el Hub guarda «silenciado».
                    void silenciar(key, !e.currentTarget.checked);
                  }}
                />
              );
            },
          )}
        </div>
      </section>
    </div>
  );
}
