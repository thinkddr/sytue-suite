/**
 * useNavDrawer — mecánica compartida del drawer de navegación en móvil (<768px):
 * foco inicial dentro, trampa de foco (Tab/Shift+Tab), Esc para cerrar, retorno
 * de foco al botón y cierre automático al cruzar a ≥768px, donde la sidebar
 * vuelve al flujo y un drawer abierto reaparecería como «fantasma».
 * docs/design.md §2 / §10. La escribían igual AdminShell, AppShell y Chat.
 */
import { useEffect } from 'preact/hooks';
import { useMobileBack } from './useMobileBack.js';

/* Mismo selector que Modal (docs/design.md §10). */
export const DRAWER_FOCUSABLE =
  'a[href],button:not([disabled]),textarea:not([disabled]),input:not([disabled]),select:not([disabled]),[tabindex]:not([tabindex="-1"])';

export function useNavDrawer(
  open: boolean,
  drawerId: string,
  toggleId: string,
  cerrar: () => void,
  /** Mantener el cajón modal en escritorio cuando la navegación es una rail. */
  options: { desktopDrawer?: boolean } = {},
): void {
  const desktopDrawer = options.desktopDrawer ?? false;
  useMobileBack(open, cerrar, { estrecho: !desktopDrawer, nivel: 1 });
  useEffect(() => {
    if (!open) return;
    const drawer = document.getElementById(drawerId);
    (drawer?.querySelector<HTMLElement>(DRAWER_FOCUSABLE) ?? drawer)?.focus();

    const onKeyDown = (e: KeyboardEvent): void => {
      if (e.key === 'Escape') {
        e.stopPropagation();
        cerrar();
        return;
      }
      if (e.key === 'Tab' && drawer) {
        const nodes = Array.from(drawer.querySelectorAll<HTMLElement>(DRAWER_FOCUSABLE));
        if (nodes.length === 0) {
          e.preventDefault();
          return;
        }
        const first = nodes[0]!;
        const last = nodes[nodes.length - 1]!;
        if (e.shiftKey && document.activeElement === first) {
          e.preventDefault();
          last.focus();
        } else if (!e.shiftKey && document.activeElement === last) {
          e.preventDefault();
          first.focus();
        }
      }
    };
    document.addEventListener('keydown', onKeyDown, true);

    const mq = window.matchMedia('(min-width: 768px)');
    const onChange = (): void => {
      if (mq.matches && !desktopDrawer) cerrar();
    };
    mq.addEventListener('change', onChange);

    return () => {
      document.removeEventListener('keydown', onKeyDown, true);
      mq.removeEventListener('change', onChange);
      document.getElementById(toggleId)?.focus();
    };
    // `cerrar` se omite a propósito: las tres llamadas pasan un setter estable de
    // useState y añadirlo obligaría a cada una a envolverlo en useCallback.
    // oxlint-disable-next-line react-hooks/exhaustive-deps
  }, [open, drawerId, toggleId, desktopDrawer]);
}
