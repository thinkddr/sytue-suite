/**
 * Toast — notificacion efimera (success/info/warning/danger) con accion
 * opcional. role=status/alert + aria-live; no comunica solo por color.
 * Provider + hook useToast. docs/design.md §9.
 *
 * Tres reglas de la spec (design.md §3.11 y §5.5):
 * - El auto-cierre se PAUSA mientras el puntero o el foco están en el aviso y se reanuda
 *   con el tiempo que le quedaba (WCAG 2.2.1): un aviso que se va mientras lo lees, o
 *   mientras tabulas hacia su «Deshacer», no se ha podido usar.
 * - Como mucho MAX_AVISOS_VISIBLES a la vez; el resto espera en cola, por orden, y su
 *   reloj no corre hasta que entra.
 * - Lo que se anuncia va a regiones vivas que existen ANTES del aviso: los lectores de
 *   pantalla no leen un `role=status` recién insertado, sí un cambio dentro de uno que ya
 *   estaba. Con un diálogo abierto van dentro de él, porque `aria-modal` deja fuera del
 *   árbol accesible lo que hay en `body`.
 */
import { createContext, type ComponentChildren, type JSX } from 'preact';
import { createPortal } from 'preact/compat';
import { useT } from '@sytue/i18n/preact';
import { useCallback, useContext, useEffect, useMemo, useRef, useState } from 'preact/hooks';
import { cx } from '../lib/cx.js';
import { Icon, type IconName } from './Icon.js';

export type ToastTone = 'success' | 'info' | 'warning' | 'danger';

/**
 * Lo que dura un aviso con «Deshacer» (la papelera, una importación): diez segundos y no
 * los cinco de serie, porque deshacer hay que leerlo, decidirlo y alcanzarlo. Uno para
 * toda la suite; espejo de `ui.DuracionDeshacer` en Fyne.
 */
export const DURACION_DESHACER = 10_000;

/**
 * Cuántos avisos se ven a la vez; los demás esperan en cola y entran cuando se va uno.
 * Espejo de `ui.MaxAvisosVisibles` en Fyne.
 */
export const MAX_AVISOS_VISIBLES = 3;

export interface ToastInput {
  tone?: ToastTone;
  title: string;
  description?: string;
  /** Duración en ms; 0 = persistente. Por defecto 5000 (warning/danger: persistente). */
  duration?: number;
  action?: { label: string; onClick: () => void };
}

interface ToastEntry extends ToastInput {
  id: string;
  /**
   * Dónde se anunció al entrar en pantalla: el hueco del diálogo de encima, `null` para
   * `body` y `undefined` mientras espera en cola. Se fija UNA vez: si después se abre un
   * diálogo, el aviso se pinta en él pero no se vuelve a anunciar.
   */
  anuncio?: HTMLElement | null;
}

/** El reloj de un aviso que se va solo: lo que le queda y quién lo está reteniendo. */
interface Reloj {
  restante: number;
  desde: number;
  timer?: ReturnType<typeof setTimeout> | undefined;
  encima: boolean;
  foco: boolean;
}

interface ToastContextValue {
  notify: (toast: ToastInput) => string;
  dismiss: (id: string) => void;
  registerDialog?: (outlet: HTMLElement) => () => void;
}

const ToastContext = createContext<ToastContextValue | null>(null);

/** Keep feedback in the modal's layout, so it cannot cover its fields or actions. */
export function useDialogToasts(): (outlet: HTMLDivElement | null) => void {
  const register = useContext(ToastContext)?.registerDialog;
  // A portal can commit after its parent's layout effects. Register when its
  // actual DOM node attaches, and release it when that node detaches.
  return useMemo(() => {
    let cleanup: (() => void) | undefined;
    return (outlet: HTMLDivElement | null) => {
      cleanup?.();
      cleanup = outlet && register ? register(outlet) : undefined;
    };
  }, [register]);
}

const toneConfig: Record<
  ToastTone,
  { icon: IconName; bar: string; fg: string; live: 'polite' | 'assertive' }
> = {
  success: { icon: 'check', bar: 'bg-success', fg: 'text-success-text', live: 'polite' },
  info: { icon: 'info', bar: 'bg-info', fg: 'text-info-text', live: 'polite' },
  warning: { icon: 'warning', bar: 'bg-warning', fg: 'text-warning-text', live: 'assertive' },
  danger: { icon: 'danger', bar: 'bg-danger', fg: 'text-danger-text', live: 'assertive' },
};

export function ToastProvider({
  children,
  isolated = false,
}: {
  children: ComponentChildren;
  /** Private account presenters own their queue and discard it when detached. */
  isolated?: boolean;
}): JSX.Element {
  const parent = useContext(ToastContext);
  // App controllers can live above the shell. A nested shell must keep their
  // notifications and dialog outlets in the same queue.
  return parent && !isolated ? <>{children}</> : <RootToastProvider>{children}</RootToastProvider>;
}

function RootToastProvider({ children }: { children: ComponentChildren }): JSX.Element {
  // `t` ya nombra cada toast en el map de abajo: la traducción va como `tr`.
  const tr = useT();
  const [toasts, setToasts] = useState<ToastEntry[]>([]);
  const relojes = useRef(new Map<string, Reloj>());
  const outlets = useRef<HTMLElement[]>([]);
  const [outlet, setOutlet] = useState<HTMLElement | null>(null);
  useEffect(
    () => () => {
      for (const reloj of relojes.current.values()) if (reloj.timer) clearTimeout(reloj.timer);
      relojes.current.clear();
      outlets.current = [];
    },
    [],
  );
  const registerDialog = useCallback((target: HTMLElement) => {
    const update = () => {
      // Portals share the same z-layer: the last dialog in document order is
      // uppermost, regardless of parent/child layout-effect registration order.
      const top = outlets.current.reduce<HTMLElement | null>(
        (last, next) =>
          !last || last.compareDocumentPosition(next) & Node.DOCUMENT_POSITION_FOLLOWING
            ? next
            : last,
        null,
      );
      setOutlet(top);
    };
    outlets.current.push(target);
    update();
    return () => {
      outlets.current = outlets.current.filter((entry) => entry !== target);
      update();
    };
  }, []);

  const dismiss = useCallback((id: string) => {
    setToasts((list) => list.filter((t) => t.id !== id));
    const reloj = relojes.current.get(id);
    if (reloj?.timer) clearTimeout(reloj.timer);
    relojes.current.delete(id);
  }, []);

  // Pone en marcha el reloj con lo que le queda, salvo que alguien lo esté reteniendo.
  const arrancar = useCallback(
    (id: string) => {
      const reloj = relojes.current.get(id);
      if (!reloj || reloj.timer || reloj.encima || reloj.foco) return;
      reloj.desde = Date.now();
      reloj.timer = setTimeout(() => dismiss(id), reloj.restante);
    },
    [dismiss],
  );

  const retener = useCallback(
    (id: string, motivo: 'encima' | 'foco', si: boolean) => {
      const reloj = relojes.current.get(id);
      if (!reloj) return;
      reloj[motivo] = si;
      if (si && reloj.timer) {
        clearTimeout(reloj.timer);
        reloj.timer = undefined;
        reloj.restante = Math.max(reloj.restante - (Date.now() - reloj.desde), 0);
      } else if (!si) {
        arrancar(id);
      }
    },
    [arrancar],
  );

  const notify = useCallback((toast: ToastInput): string => {
    const id = `toast-${Date.now()}-${Math.random().toString(36).slice(2, 7)}`;
    setToasts((list) => [...list, { ...toast, id }]);
    // warning/danger persisten hasta que el usuario los cierra (spec §Toasts);
    // un error que se autodescarta puede perderse sin ser leído.
    const persistent = toast.tone === 'warning' || toast.tone === 'danger';
    const duration = toast.duration ?? (persistent ? 0 : 5000);
    // El reloj se crea parado: arranca cuando el aviso sale de la cola (efecto de abajo).
    if (duration > 0) {
      relojes.current.set(id, { restante: duration, desde: 0, encima: false, foco: false });
    }
    return id;
  }, []);

  const visibles = toasts.slice(0, MAX_AVISOS_VISIBLES);

  // Lo que acaba de entrar en pantalla: arranca su reloj y se anuncia donde toca ahora.
  useEffect(() => {
    const nuevos = new Set<string>();
    for (const t of toasts.slice(0, MAX_AVISOS_VISIBLES)) {
      arrancar(t.id);
      if (t.anuncio === undefined) nuevos.add(t.id);
    }
    if (nuevos.size > 0) {
      setToasts((list) => list.map((t) => (nuevos.has(t.id) ? { ...t, anuncio: outlet } : t)));
    }
  }, [toasts, outlet, arrancar]);

  const value = useMemo<ToastContextValue>(
    () => ({ notify, dismiss, registerDialog }),
    [notify, dismiss, registerDialog],
  );

  // El hueco del diálogo de encima, si hay uno, o la esquina de la ventana. Con el «+»
  // flotante de la app en la misma esquina (BotonFlotante.tsx), los avisos suben por
  // encima de él: los dos a la vez se pisaban, y el «Deshacer» de un aviso quedaba
  // justo donde el dedo va a crear.
  const pila =
    visibles.length > 0 ? (
      <div
        class={cx(
          'pointer-events-none flex flex-col gap-2',
          outlet
            ? 'px-5 pb-3'
            : 'fixed right-[max(1rem,var(--sytue-safe-right,0px))] bottom-[max(1rem,var(--sytue-safe-bottom,0px))] w-[calc(100%_-_max(1rem,var(--sytue-safe-left,0px))_-_max(1rem,var(--sytue-safe-right,0px)))] max-w-sm',
          !outlet &&
            '[body:has([data-sytue-fab])_&]:bottom-[calc(max(1rem,var(--sytue-safe-bottom,0px))+4.5rem)]',
        )}
        style={outlet ? undefined : { zIndex: 'var(--z-toast)' }}
      >
        {visibles.map((t) => {
          const cfg = toneConfig[t.tone ?? 'info'];
          return (
            // Sin `role` ni `aria-live`: lo que se lee lo dicen las regiones vivas, que
            // ya estaban montadas. Aquí solo se retiene el reloj mientras alguien lo mira
            // con el puntero o está dentro con el foco.
            <div
              key={t.id}
              data-toast={t.tone ?? 'info'}
              // Eventos de puntero y no de ratón: en una pantalla táctil el `mouseenter`
              // que emula el toque no tiene `mouseleave` hasta que se toca otra cosa y el
              // aviso se quedaría parado; el `pointerleave` llega al levantar el dedo.
              onPointerEnter={() => retener(t.id, 'encima', true)}
              onPointerLeave={() => retener(t.id, 'encima', false)}
              onFocusIn={() => retener(t.id, 'foco', true)}
              onFocusOut={(e) => {
                const dentro =
                  e.relatedTarget instanceof Node && e.currentTarget.contains(e.relatedTarget);
                if (!dentro) retener(t.id, 'foco', false);
              }}
              class={cx(
                'sytue-toast-in border-border pointer-events-auto flex items-start gap-3 overflow-hidden rounded-lg border',
                'bg-surface-raised pr-3 shadow-[var(--shadow-lg)]',
              )}
            >
              <span class={cx('w-1 self-stretch', cfg.bar)} aria-hidden="true" />
              <span class={cx('py-3', cfg.fg)}>
                <Icon name={cfg.icon} size={18} />
              </span>
              <div class="flex-1 py-3">
                <p class="text-text text-base font-semibold">{t.title}</p>
                {t.description ? (
                  <p class="text-text-muted mt-0.5 text-sm">{t.description}</p>
                ) : null}
                {t.action ? (
                  <button
                    type="button"
                    onClick={() => {
                      t.action?.onClick();
                      dismiss(t.id);
                    }}
                    class="text-primary-text sytue-focus-ring mt-1.5 rounded-sm text-sm font-semibold hover:underline"
                  >
                    {t.action.label}
                  </button>
                ) : null}
              </div>
              <button
                type="button"
                onClick={() => dismiss(t.id)}
                aria-label={tr('ui.dismissNotification')}
                class="text-text-subtle hover:text-text sytue-focus-ring mt-1 inline-flex h-7 w-7 shrink-0 items-center justify-center self-start rounded-sm"
              >
                <Icon name="close" size={16} />
              </button>
            </div>
          );
        })}
      </div>
    ) : null;

  return (
    <ToastContext.Provider value={value}>
      {children}
      {typeof document === 'undefined' ? null : (
        <>
          {createPortal(
            <RegionesVivas avisos={visibles.filter((t) => t.anuncio === null)} />,
            document.body,
          )}
          {outlet
            ? createPortal(
                <>
                  <RegionesVivas avisos={visibles.filter((t) => t.anuncio === outlet)} />
                  {pila}
                </>,
                outlet,
              )
            : visibles.length > 0
              ? createPortal(pila, document.body)
              : null}
        </>
      )}
    </ToastContext.Provider>
  );
}

/**
 * Las dos regiones vivas, `polite` y `assertive`, montadas aunque estén vacías: lo que
 * hace hablar al lector de pantalla es que les ENTRE un párrafo, no que aparezcan. Se
 * quedan con los avisos en pantalla que se anunciaron aquí; al irse uno, su párrafo sale
 * sin que se lea nada (`aria-relevant` de serie: altas y texto).
 */
function RegionesVivas({ avisos }: { avisos: ToastEntry[] }): JSX.Element {
  return (
    <>
      {(['polite', 'assertive'] as const).map((live) => (
        <div
          key={live}
          role={live === 'assertive' ? 'alert' : 'status'}
          aria-live={live}
          class="sr-only"
        >
          {avisos
            .filter((t) => toneConfig[t.tone ?? 'info'].live === live)
            .map((t) => (
              <p key={t.id}>
                {t.title}
                {t.description ? <> {t.description}</> : null}
              </p>
            ))}
        </div>
      ))}
    </>
  );
}

// Fallback sin provider: no-op (avisa en dev). Un island que use useToast fuera
// de un ToastProvider NO debe tumbar el render SSR de toda la app; degrada a
// "sin toasts" en vez de lanzar. Lo ideal es montar el island bajo la Sytue Shell.
const noopToast: ToastContextValue = {
  notify: () => {
    // import.meta.env no está tipado en este paquete de librería (sin Vite); cast seguro.
    if ((import.meta as { env?: { DEV?: boolean } }).env?.DEV) {
      console.warn('useToast usado fuera de <ToastProvider>: toast ignorado.');
    }
    return '';
  },
  dismiss: () => {},
};

export function useToast(): ToastContextValue {
  return useContext(ToastContext) ?? noopToast;
}
