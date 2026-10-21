/**
 * Toast — notificacion efimera (success/info/warning/danger) con accion
 * opcional. role=status/alert + aria-live; no comunica solo por color.
 * Provider + hook useToast. docs/design.md §9.
 */
import { createContext, type ComponentChildren, type JSX } from 'preact';
import { createPortal } from 'preact/compat';
import { useT } from '@sytue/i18n/preact';
import { useCallback, useContext, useMemo, useRef, useState } from 'preact/hooks';
import { cx } from '../lib/cx.js';
import { Icon, type IconName } from './Icon.js';

export type ToastTone = 'success' | 'info' | 'warning' | 'danger';

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
}

interface ToastContextValue {
  notify: (toast: ToastInput) => string;
  dismiss: (id: string) => void;
}

const ToastContext = createContext<ToastContextValue | null>(null);

const toneConfig: Record<
  ToastTone,
  { icon: IconName; bar: string; fg: string; live: 'polite' | 'assertive' }
> = {
  success: { icon: 'check', bar: 'bg-success', fg: 'text-success-text', live: 'polite' },
  info: { icon: 'info', bar: 'bg-info', fg: 'text-info-text', live: 'polite' },
  warning: { icon: 'warning', bar: 'bg-warning', fg: 'text-warning-text', live: 'assertive' },
  danger: { icon: 'danger', bar: 'bg-danger', fg: 'text-danger-text', live: 'assertive' },
};

export function ToastProvider({ children }: { children: ComponentChildren }): JSX.Element {
  // `t` ya nombra cada toast en el map de abajo: la traducción va como `tr`.
  const tr = useT();
  const [toasts, setToasts] = useState<ToastEntry[]>([]);
  const timers = useRef(new Map<string, ReturnType<typeof setTimeout>>());

  const dismiss = useCallback((id: string) => {
    setToasts((list) => list.filter((t) => t.id !== id));
    const timer = timers.current.get(id);
    if (timer) {
      clearTimeout(timer);
      timers.current.delete(id);
    }
  }, []);

  const notify = useCallback(
    (toast: ToastInput): string => {
      const id = `toast-${Date.now()}-${Math.random().toString(36).slice(2, 7)}`;
      setToasts((list) => [...list, { ...toast, id }]);
      // warning/danger persisten hasta que el usuario los cierra (spec §Toasts);
      // un error que se autodescarta puede perderse sin ser leído.
      const persistent = toast.tone === 'warning' || toast.tone === 'danger';
      const duration = toast.duration ?? (persistent ? 0 : 5000);
      if (duration > 0) {
        timers.current.set(
          id,
          setTimeout(() => dismiss(id), duration),
        );
      }
      return id;
    },
    [dismiss],
  );

  const value = useMemo<ToastContextValue>(() => ({ notify, dismiss }), [notify, dismiss]);

  return (
    <ToastContext.Provider value={value}>
      {children}
      {typeof document !== 'undefined'
        ? createPortal(
            <div
              class="pointer-events-none fixed right-[max(1rem,var(--sytue-safe-right,0px))] bottom-[max(1rem,var(--sytue-safe-bottom,0px))] flex w-[calc(100%_-_max(1rem,var(--sytue-safe-left,0px))_-_max(1rem,var(--sytue-safe-right,0px)))] max-w-sm flex-col gap-2"
              style={{ zIndex: 'var(--z-toast)' }}
            >
              {toasts.map((t) => {
                const cfg = toneConfig[t.tone ?? 'info'];
                return (
                  <div
                    key={t.id}
                    role={cfg.live === 'assertive' ? 'alert' : 'status'}
                    aria-live={cfg.live}
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
            </div>,
            document.body,
          )
        : null}
    </ToastContext.Provider>
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
