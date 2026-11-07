/**
 * ReminderCenter — entrega IN-APP de recordatorios (VALARM). Una campana con
 * contador que abre un panel sobrio con los recordatorios vencidos del usuario:
 * cada uno con su hora, «descartar» y deep-link a su día. Sondea el backend con
 * intervalo razonable + backoff exponencial ante error, y se pausa con la pestaña
 * oculta (escala: sin sondeo en segundo plano). Deduplica por id (no repite avisos
 * ni notificaciones nativas) y el descarte es persistente (servidor). Si el
 * navegador concedió permiso, además emite una Notification nativa.
 *
 * Sólo @sytue/ui + tokens --sytue-*; panel accesible como DISCLOSURE (no modal):
 * la campana es el disparador (aria-expanded) y el foco entra tabulando; Escape y
 * clic fuera lo cierran devolviendo el foco a la campana. Anti AI-slop: hairlines,
 * sin degradados ni emojis.
 *
 * DOS PIEZAS. El estado —sondeo, avisos, descartados, permiso— es `useRecordatorios`,
 * que la app monta UNA vez; `ReminderCenter` solo presenta. Así, en el mes del teléfono
 * la campana pasa de la cabecera al cajón (`disparador="fila"`, una rama del menú con su
 * contador y el panel desplegado debajo) sin volver a sondear ni olvidar lo descartado.
 */
import type { JSX } from 'preact';
import type { Locale } from '@sytue/i18n';
import { useEffect, useRef, useState } from 'preact/hooks';
import {
  Badge,
  Button,
  Icon,
  SidebarItem,
  Spinner,
  cx,
  useFitInViewport,
  useToast,
} from '@sytue/ui';
import { listDueReminders, dismissReminder, type Reminder } from '../client/calendarApi.js';
import { timeLabel, isToday, dayMonthLabel } from './dates.js';
import type { T } from '../i18n/index.js';

interface Opciones {
  /** Traductor ya atado al idioma de la petición; baja del padre. */
  t: T;
  locale: Locale;
  /** Deep-link: llevar el calendario al día del evento del recordatorio. */
  onDeepLink: (start: Date) => void;
}

/** Lo que `useRecordatorios` mantiene y `ReminderCenter` pinta. */
export interface EstadoRecordatorios {
  lista: Reminder[];
  cargando: boolean;
  error: boolean;
  /** El recordatorio que se está descartando (su botón, inerte). */
  ocupado: string | null;
  permiso: NotificationPermission | 'unsupported';
  descartar: (id: string) => Promise<void>;
  /** Lleva el calendario al día del recordatorio. */
  irA: (r: Reminder) => void;
  pedirPermiso: () => Promise<void>;
}

interface Props {
  t: T;
  locale: Locale;
  estado: EstadoRecordatorios;
  /**
   * `campana` (por defecto): el botón de la cabecera con su panel flotante.
   * `fila`: una rama del menú lateral con su contador y el panel desplegado debajo,
   * en línea (el mes del teléfono, donde la cabecera no tiene sitio).
   */
  disparador?: 'campana' | 'fila';
  class?: string | undefined;
}

const BASE_INTERVAL = 60_000; // 1 min
const MAX_INTERVAL = 10 * 60_000; // techo del backoff
const NOTIFIED_KEY = 'sytue-cal-reminders-notified';
const NOTIFIED_CAP = 300;

/** Ids ya notificados (nativo/toast) — persistente para no repetir tras recargar. */
function loadNotified(): Set<string> {
  try {
    const raw = localStorage.getItem(NOTIFIED_KEY);
    return new Set(raw ? (JSON.parse(raw) as string[]) : []);
  } catch {
    return new Set();
  }
}
function saveNotified(ids: Set<string>): void {
  try {
    localStorage.setItem(NOTIFIED_KEY, JSON.stringify([...ids].slice(-NOTIFIED_CAP)));
  } catch {
    /* almacenamiento no disponible: se degrada a dedupe en memoria */
  }
}

/**
 * El estado de los recordatorios: sondeo con backoff, avisos (toast + Notification),
 * descartados y permiso. Se monta UNA vez por app: cambiar de disparador no lo rehace.
 */
export function useRecordatorios({ t, locale, onDeepLink }: Opciones): EstadoRecordatorios {
  const toast = useToast();
  const [reminders, setReminders] = useState<Reminder[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(false);
  const [busyId, setBusyId] = useState<string | null>(null);
  const [permission, setPermission] = useState<NotificationPermission | 'unsupported'>(
    typeof Notification === 'undefined' ? 'unsupported' : Notification.permission,
  );

  const notified = useRef<Set<string>>(loadNotified());
  const dismissed = useRef<Set<string>>(new Set());
  const backoff = useRef<number>(BASE_INTERVAL);
  const timer = useRef<ReturnType<typeof setTimeout> | null>(null);

  // --- Sondeo con backoff + pausa por visibilidad -----------------------------
  useEffect(() => {
    let cancelled = false;

    async function poll(): Promise<void> {
      if (document.hidden) {
        schedule(BASE_INTERVAL);
        return;
      }
      try {
        const { reminders: due } = await listDueReminders();
        if (cancelled) return;
        const fresh = due.filter((r) => !dismissed.current.has(r.id));
        setReminders(fresh);
        setError(false);
        backoff.current = BASE_INTERVAL;
        announce(fresh);
      } catch {
        if (cancelled) return;
        setError(true);
        backoff.current = Math.min(backoff.current * 2, MAX_INTERVAL);
      } finally {
        if (!cancelled) {
          setLoading(false);
          schedule(backoff.current);
        }
      }
    }

    function schedule(ms: number): void {
      if (timer.current) clearTimeout(timer.current);
      timer.current = setTimeout(() => void poll(), ms);
    }

    /** Avisa (toast + Notification nativa) de los recordatorios nuevos, una vez. */
    function announce(list: Reminder[]): void {
      for (const r of list) {
        if (notified.current.has(r.id)) continue;
        notified.current.add(r.id);
        const when = new Date(r.dtstart);
        // El cuerpo es una plantilla entera («Evento · 09:30»), no un `Evento` +
        // la hora: el separador y el orden no son universales.
        const body = Number.isNaN(when.getTime())
          ? t('reminders.body')
          : t('reminders.bodyAt', { time: timeLabel(locale, when) });
        toast.notify({
          tone: 'info',
          title: r.summary || t('reminders.fallbackTitle'),
          description: body,
          action: { label: t('reminders.view'), onClick: () => deepLink(r) },
        });
        if (typeof Notification !== 'undefined' && Notification.permission === 'granted') {
          try {
            new Notification(r.summary || t('reminders.fallbackTitle'), {
              body,
              tag: r.id,
            });
          } catch {
            /* algunos navegadores exigen SW para Notification: se ignora */
          }
        }
      }
      saveNotified(notified.current);
    }

    void poll();
    const onVisible = (): void => {
      if (!document.hidden) void poll();
    };
    document.addEventListener('visibilitychange', onVisible);
    return () => {
      cancelled = true;
      if (timer.current) clearTimeout(timer.current);
      document.removeEventListener('visibilitychange', onVisible);
    };
    // Montaje único: el sondeo se reprograma solo.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  function deepLink(r: Reminder): void {
    const d = new Date(r.dtstart);
    if (!Number.isNaN(d.getTime())) onDeepLink(d);
  }

  async function dismiss(id: string): Promise<void> {
    setBusyId(id);
    dismissed.current.add(id);
    setReminders((list) => list.filter((r) => r.id !== id));
    try {
      await dismissReminder(id);
    } catch {
      toast.notify({ tone: 'danger', title: t('toast.reminderDismissFailed') });
    } finally {
      setBusyId(null);
    }
  }

  async function requestPermission(): Promise<void> {
    if (typeof Notification === 'undefined') return;
    try {
      setPermission(await Notification.requestPermission());
    } catch {
      /* el usuario canceló o el navegador lo bloqueó */
    }
  }

  return {
    lista: reminders,
    cargando: loading,
    error,
    ocupado: busyId,
    permiso: permission,
    descartar: dismiss,
    irA: deepLink,
    pedirPermiso: requestPermission,
  };
}

export function ReminderCenter({
  t,
  locale,
  estado,
  disparador = 'campana',
  class: className,
}: Props): JSX.Element {
  const [open, setOpen] = useState(false);
  const rootRef = useRef<HTMLDivElement | null>(null);
  const bellRef = useRef<HTMLButtonElement | null>(null);
  const panelRef = useRef<HTMLDivElement | null>(null);
  const fila = disparador === 'fila';

  // El panel se ancla al borde derecho de la campana, que en la barra de
  // herramientas de Calendar está lejos del borde de la pantalla: a 360 px
  // arrancaba en −112 px (un tercio fuera, título incluido) y a lo recortado por
  // la izquierda no se llega desplazando. Mismo acotado que Dropdown/Popover.
  useFitInViewport(panelRef, open && !fila);

  // --- Cierre del panel flotante: Escape y clic fuera -----------------------------
  // La FILA no lleva ninguno de los dos. Vive en el cajón de AppShell, cuyo Escape se
  // escucha antes (en captura, sobre `document`) y cierra el cajón devolviendo el foco
  // al ☰: un Escape aquí no llegaría nunca. Y la rama conserva su estado al cerrar el
  // cajón, como las demás ramas del lateral y como `recEnCajon` en Fyne.
  useEffect(() => {
    if (!open || fila) return;
    const onKey = (e: KeyboardEvent): void => {
      if (e.key === 'Escape') {
        setOpen(false);
        bellRef.current?.focus();
      }
    };
    const onDown = (e: MouseEvent): void => {
      if (rootRef.current && !rootRef.current.contains(e.target as Node)) setOpen(false);
    };
    document.addEventListener('keydown', onKey);
    document.addEventListener('mousedown', onDown);
    return () => {
      document.removeEventListener('keydown', onKey);
      document.removeEventListener('mousedown', onDown);
    };
  }, [open, fila]);

  const { lista: reminders, cargando: loading, error, ocupado: busyId, permiso } = estado;
  function deepLink(r: Reminder): void {
    estado.irA(r);
    setOpen(false);
  }
  const count = reminders.length;
  // `data-mantiene-nav` en lo que actúa en el sitio (descartar, activar avisos): en el
  // cajón del teléfono no debe cerrarlo. El enlace a un recordatorio NO lo lleva: lleva
  // a su día y cierra el cajón como cualquier destino del menú.
  const enSitio = fila ? { 'data-mantiene-nav': true } : {};
  const cuerpo = (
    <>
      <div class="max-h-96 overflow-y-auto">
        {loading ? (
          <div
            class="flex items-center justify-center py-8"
            role="status"
            aria-label={t('reminders.loading')}
          >
            <Spinner />
          </div>
        ) : error && count === 0 ? (
          <div class="flex flex-col items-center gap-2 px-4 py-8 text-center">
            <p class="text-text-muted text-sm">{t('reminders.loadError')}</p>
          </div>
        ) : count === 0 ? (
          <div class="flex flex-col items-center gap-1 px-4 py-8 text-center">
            <Icon name="bell" size={24} class="text-text-subtle" aria-hidden="true" />
            <p class="text-text-muted text-sm">{t('reminders.empty')}</p>
          </div>
        ) : (
          <ul class="divide-border divide-y">
            {reminders.map((r) => (
              <li key={r.id} class="hover:bg-bg-subtle flex items-start gap-2 px-3 py-2.5">
                <button
                  type="button"
                  onClick={() => deepLink(r)}
                  class="sytue-focus-ring min-w-0 flex-1 text-left"
                >
                  <span class="text-text block truncate text-sm font-medium">
                    {r.summary || t('event.untitled')}
                  </span>
                  <span class="sytue-meta block tabular-nums">
                    {reminderWhen(t, locale, r.dtstart)}
                  </span>
                </button>
                <Button
                  variant="ghost"
                  size="sm"
                  iconOnly
                  aria-label={t('reminders.dismiss', {
                    title: r.summary || t('reminders.eventFallback'),
                  })}
                  onClick={() => void estado.descartar(r.id)}
                  disabled={busyId === r.id}
                  class="mt-0.5 shrink-0"
                  leadingIcon={<Icon name="close" size={16} />}
                  {...enSitio}
                />
              </li>
            ))}
          </ul>
        )}
      </div>

      {permiso === 'default' ? (
        <div class="border-border border-t px-3 py-2">
          <Button
            variant="ghost"
            size="sm"
            fullWidth
            onClick={() => void estado.pedirPermiso()}
            {...enSitio}
          >
            {t('reminders.enableNotifications')}
          </Button>
        </div>
      ) : null}
    </>
  );

  if (fila) {
    // Una rama del menú (es un `<li>`: va dentro de la lista de una sección). El
    // contador es el de la campana, y lo de dentro va en una hoja de superficie porque
    // el cuerpo usa los tokens de superficie y el carril del menú es oscuro.
    return (
      <SidebarItem
        icon={<Icon name="bell" size={18} />}
        label={t('reminders.title')}
        count={count}
        expanded={open}
        onClick={() => setOpen((v) => !v)}
      >
        <li class={cx('list-none', className)}>
          <div class="bg-surface text-text border-border overflow-hidden rounded-md border">
            {cuerpo}
          </div>
        </li>
      </SidebarItem>
    );
  }

  return (
    <div ref={rootRef} class={cx('relative', className)}>
      <button
        ref={bellRef}
        type="button"
        aria-label={count > 0 ? t('reminders.bellCount', { count }) : t('reminders.bell')}
        aria-expanded={open}
        title={t('reminders.title')}
        onClick={() => setOpen((v) => !v)}
        class={cx(
          'border-chrome-border text-header-fg rounded-control relative inline-flex h-8 w-8 items-center justify-center border',
          // La campana vive en la banda de cabecera: hover/active son los velos del
          // cromo (bg-chrome-*), como el resto de controles de esa banda. Un velo de
          // superficie (bg-bg-subtle) dejaba el icono casi ilegible sobre la de tinta.
          'hover:bg-chrome-hover active:bg-chrome-active transition-colors duration-150',
          'sytue-focus-ring',
        )}
      >
        <Icon name="bell" size={16} />
        {count > 0 ? (
          <Badge count aria-hidden="true" class="absolute -top-1 -right-1">
            {count > 99 ? '99+' : count}
          </Badge>
        ) : null}
      </button>

      {open ? (
        <div
          ref={panelRef}
          class="sytue-menu-surface border-border absolute right-0 z-[var(--z-popover)] mt-2 w-80 max-w-[calc(100vw-16px)] rounded-lg border shadow-[var(--shadow-md)]"
        >
          <div class="border-border flex items-center justify-between border-b px-3 py-2">
            <span class="text-text text-sm font-semibold">{t('reminders.title')}</span>
            <Button
              variant="ghost"
              size="sm"
              iconOnly
              aria-label={t('common.close')}
              onClick={() => {
                setOpen(false);
                bellRef.current?.focus();
              }}
              leadingIcon={<Icon name="close" size={16} />}
            />
          </div>

          {cuerpo}
        </div>
      ) : null}
    </div>
  );
}

/** Etiqueta de cuándo: «Hoy · 09:30» o «3 de julio · 09:30». Día y hora se unen
 *  con una plantilla del catálogo, nunca con un `+`. */
function reminderWhen(t: T, locale: Locale, dtstart: string): string {
  const d = new Date(dtstart);
  if (Number.isNaN(d.getTime())) return '';
  const day = isToday(d) ? t('reminders.today') : dayMonthLabel(locale, d);
  return t('reminders.when', { day, time: timeLabel(locale, d) });
}
