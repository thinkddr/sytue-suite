/**
 * Centro de notificaciones de la shell — notificaciones unificadas del Hub,
 * en tiempo real vía SSE. Popover con carga (skeleton), error con reintento,
 * lista y "marcar como leída". docs/CONVENTIONS.md §11.
 */
import type { JSX } from 'preact';
import { useEffect, useRef, useState } from 'preact/hooks';
import {
  Badge,
  Button,
  Dropdown,
  EmptyState,
  Icon,
  IconButton,
  List,
  ListItem,
  Skeleton,
  TopBarButton,
  Illustration,
} from '@sytue/ui';
import type { Notification } from '@sytue/api-clients';
import { formatDateTime, formatTimeAgo } from '@sytue/i18n';
import { useI18n } from '@sytue/i18n/preact';
import { appUrl } from '@sytue/config';
import { useShell } from './context.js';
import { inNativeShell } from './openInApp.js';
import {
  abrirAviso,
  conectarAvisos,
  cuerpoAviso,
  procedenciaAviso,
  tituloAviso,
  type Avisos,
  type EstadoAvisos,
} from './avisos.js';

/**
 * Espejo de una notificación de la suite en el CENTRO DE NOTIFICACIONES DEL SO
 * de móvil. Se accede por `window.Capacitor.Plugins` y NO por import: el plugin
 * (@capacitor/local-notifications) vive en apps/mobile/suite, que está fuera de
 * este workspace — un import aquí lo haría depencia del frontend web, que no lo
 * tiene. En el navegador `window.Capacitor` no existe y esto es un no-op; en la
 * app nativa el runtime ya registra el plugin si el binario lo lleva.
 */
function notificarEnSO(title: string, body: string | undefined): void {
  if (!inNativeShell()) return;
  const cap = (
    window as unknown as {
      Capacitor?: { Plugins?: { LocalNotifications?: { schedule?: (o: unknown) => void } } };
    }
  ).Capacitor;
  cap?.Plugins?.LocalNotifications?.schedule?.({
    notifications: [
      {
        id: Math.floor(Date.now() % 2147483647),
        title,
        body: body || '',
      },
    ],
  });
}

export function NotificationCenter(): JSX.Element {
  const { hub, cambiarFoto } = useShell();
  const { locale, t } = useI18n();
  // La carga, el contador (el de la base, no los `!read` de la página), el flujo con
  // su reconexión y el «leído» los lleva `conectarAvisos`, que comparte con la bandeja
  // de One.
  const [estado, setEstado] = useState<EstadoAvisos>({
    items: [],
    sinLeer: 0,
    correoSinLeer: null,
    cargando: true,
    fallo: false,
    hayMas: false,
  });
  const avisosRef = useRef<Avisos | null>(null);
  useEffect(() => {
    const a = conectarAvisos(hub, setEstado, {
      limite: 30,
      // En la app nativa el aviso también llega por ENCIMA: bandeja del SO, sin
      // abrir la suite. En navegador esto no hace nada.
      alLlegar: (n) => notificarEnSO(tituloAviso(locale, n), cuerpoAviso(locale, n)),
      // El mismo flujo trae el «cambió la foto» de Sytue ID: la barra la repinta.
      alCambiarFoto: cambiarFoto,
    });
    avisosRef.current = a;
    return () => a.cerrar();
  }, [hub, locale, cambiarFoto]);

  // El correo sin leer lo cuenta el Hub (`mail_unread`), así que la fila sale en las
  // trece apps y no solo en Mail, que era la única que podía preguntar a su BFF.
  const { items, sinLeer: unread, correoSinLeer, cargando: loading, fallo: error } = estado;
  const cuerpo = (n: Notification) => {
    const c = cuerpoAviso(locale, n);
    return c ? <span title={c}>{c}</span> : undefined;
  };

  return (
    <Dropdown
      label={t('notifications.title')}
      trigger={({ onClick, ...aria }) => (
        <TopBarButton
          label={
            unread > 0
              ? t('notifications.buttonLabel', { count: unread })
              : t('notifications.title')
          }
          indicator={unread > 0}
          onClick={onClick}
          class={aria['aria-expanded'] ? 'bg-chrome-hover' : undefined}
          {...aria}
        >
          <Icon name="bell" size={20} />
        </TopBarButton>
      )}
    >
      {/* 280 px (`w-80` con el rem de 14) y nunca más que el panel, que ya se queda
          a 8 px de cada borde de la ventana (`max-w-[calc(100vw-16px)]` de Dropdown).
          Con el ancho fijo a secas desbordaba en los móviles estrechos —la auditoría lo cazó
          42 veces, «+10px scroll-lateral en :root»—; con `w-full max-w-80`, en cambio,
          el panel medía lo que su contenido (de 220 a 290 según el título más largo),
          y ni la web era siempre igual ni había con qué comparar la app nativa. */}
      <div class="w-80 max-w-full">
        {/* Estado vivo del buzón, POR ENCIMA del historial del Hub: los
            correos sin leer son estado, no eventos, y el usuario los espera
            aquí. Enlace directo a Sytue Mail. */}
        {correoSinLeer !== null && correoSinLeer > 0 ? (
          <a
            href={appUrl('mail', '')}
            class="bg-selected text-text hover:bg-bg-subtle sytue-focus-ring flex items-center gap-2 border-t border-b px-3 py-2 text-sm font-medium"
          >
            <Icon name="mail" size={16} />
            <span class="min-w-0 flex-1 truncate">
              {t('notifications.mailUnread', { count: correoSinLeer })}
            </span>
            <span aria-hidden="true">→</span>
          </a>
        ) : null}
        <div class="flex items-center justify-between gap-2 px-3 pt-1.5 pb-2">
          <span class="flex min-w-0 items-center gap-2">
            <span class="text-text text-base font-semibold">{t('notifications.title')}</span>
            {unread > 0 ? (
              <Badge tone="primary">{t('notifications.unread', { count: unread })}</Badge>
            ) : null}
          </span>
          {/* Glifo y no texto: «Marcar todas como leídas» no cabe junto al título y
              la insignia en los 280 px del panel. El rótulo va en aria-label y title. */}
          {unread > 0 ? (
            <IconButton
              label={t('notifications.markAllRead')}
              onClick={() => void avisosRef.current?.leerTodos()}
            >
              <Icon name="check" size={16} />
            </IconButton>
          ) : null}
        </div>
        {loading ? (
          <div class="border-border flex flex-col gap-4 border-t px-3 py-4" aria-busy="true">
            {[0, 1, 2].map((i) => (
              <div key={i} class="flex flex-col gap-1.5">
                <Skeleton width="65%" height="0.75rem" />
                <Skeleton width="90%" height="0.625rem" />
              </div>
            ))}
            <span class="sr-only">{t('notifications.loading')}</span>
          </div>
        ) : error && items.length === 0 ? (
          <EmptyState
            title={t('notifications.failed')}
            description={t('error.connection')}
            illustration={<Icon name="danger" size={28} />}
            action={
              <Button variant="secondary" size="sm" onClick={() => avisosRef.current?.reintentar()}>
                {t('action.retry')}
              </Button>
            }
            class="border-border border-t px-6 py-10"
          />
        ) : items.length === 0 ? (
          <EmptyState
            title={t('notifications.emptyTitle')}
            description={t('notifications.emptyDescription')}
            // «Todo al día» con la ilustración de marca, no una campana gris: el
            // vacío aquí es la buena noticia, y se lee como tal.
            illustration={<Illustration name="allDone" size={112} />}
            class="border-border border-t px-6 py-10"
          />
        ) : (
          <List
            label={t('notifications.title')}
            class="border-border max-h-96 overflow-y-auto border-t"
          >
            {items.map((n) => (
              <ListItem
                key={n.id}
                title={<span title={tituloAviso(locale, n)}>{tituloAviso(locale, n)}</span>}
                subtitle={cuerpo(n)}
                unread={!n.read}
                meta={
                  <>
                    {/* Un aviso de OTRA organización lo dice (ADR 0053), y uno
                        reconstruido, que es anterior al registro de avisos. */}
                    {procedenciaAviso(locale, n).map((p) => (
                      <span key={p} class="block truncate" title={p}>
                        {p}
                      </span>
                    ))}
                    <time dateTime={n.created_at} title={formatDateTime(locale, n.created_at)}>
                      {formatTimeAgo(locale, n.created_at)}
                    </time>
                  </>
                }
                onClick={() => {
                  if (avisosRef.current) {
                    void abrirAviso(avisosRef.current, n, (u) => window.location.assign(u));
                  }
                }}
              />
            ))}
          </List>
        )}
        {/* Salida a la bandeja completa: el popover enseña las últimas y no
            tiene filtros, «marcar todas» ni las reglas de silencio. Absoluta y a
            One porque este cromo se monta en las once superficies. */}
        <div class="border-border border-t px-3 py-2">
          <a
            href={appUrl('one', 'notificaciones')}
            class="text-primary-text sytue-focus-ring inline-flex items-center gap-1 rounded-sm text-sm font-semibold hover:underline"
          >
            {t('notifications.viewAll')}
            <span aria-hidden="true">→</span>
          </a>
        </div>
      </div>
    </Dropdown>
  );
}
