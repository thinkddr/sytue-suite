/**
 * Shell — isla raíz del portal. Envuelve <SytueShell> inyectando el proveedor
 * de access token del cliente (getter en memoria contra /auth/token), ya que
 * las funciones no son serializables como props de isla Astro. Recibe solo
 * props serializables (usuario, Hub) desde el layout SSR.
 */
import type { ComponentChildren } from 'preact';
import { useRef } from 'preact/hooks';
import { SytueShell } from '@sytue/shell';
import { IndicadorRefrescoTactil, usePullToRefresh } from '@sytue/ui';
import type { SytueUser } from '@sytue/auth';
import type { Locale } from '@sytue/i18n';
import { getAccessToken } from '@sytue/auth/client';

interface ShellProps {
  user: SytueUser;
  hubBaseUrl: string;
  /** Idioma negociado en el servidor. Sin él, el cromo (buscador, cuenta,
   *  notificaciones) se quedaba en inglés aunque el resto del portal estuviera
   *  en alemán: era la única app de la suite que no se lo pasaba al shell. */
  locale: Locale;
  children?: ComponentChildren;
}

export function Shell({ user, hubBaseUrl, locale, children }: ShellProps) {
  const mainRef = useRef<HTMLElement>(null);
  const refresco = usePullToRefresh(mainRef);
  return (
    <SytueShell
      currentApp="one"
      user={user}
      hubBaseUrl={hubBaseUrl}
      locale={locale}
      accessToken={getAccessToken}
    >
      {/* Landmark main + destino del skip-link de la shell. En la forma «Carril» los dos
          envoltorios son display:contents y no alteran el layout del lanzador; en
          «Marco» son el marco de color y la hoja que flota dentro
          (tokens/appearance.css). */}
      <main
        ref={mainRef}
        id="contenido"
        tabIndex={-1}
        class="sytue-frame sytue-frame-page sytue-safe-workspace focus-visible:outline-none"
      >
        <IndicadorRefrescoTactil estado={refresco} fijo />
        <div class="sytue-sheet sytue-sheet-page">{children}</div>
      </main>
    </SytueShell>
  );
}
