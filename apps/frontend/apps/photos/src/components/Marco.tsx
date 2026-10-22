/**
 * El marco de Sytue Photos: la Sytue Shell (barra superior con el conmutador de
 * aplicaciones, la búsqueda global y la cuenta) y, debajo, el AppShell con la
 * vista. El editor NO lo usa: es un espacio de trabajo a pantalla completa.
 */
import type { ComponentChildren, JSX } from 'preact';
import { AppShell } from '@sytue/ui';
import { SytueShell } from '@sytue/shell';
import type { SytueUser } from '@sytue/auth';
import type { Locale } from '@sytue/i18n';
import { getAccessToken } from '@sytue/auth/client';

export interface MarcoProps {
  user: SytueUser;
  hubBaseUrl: string;
  locale: Locale;
  title: string;
  children: ComponentChildren;
}

export function Marco({ user, hubBaseUrl, locale, title, children }: MarcoProps): JSX.Element {
  return (
    <SytueShell
      currentApp="photos"
      user={user}
      hubBaseUrl={hubBaseUrl}
      accessToken={getAccessToken}
      locale={locale}
    >
      {/* La topbar global la pinta la Sytue Shell: el AppShell ocupa el resto. */}
      <AppShell topbar={null} class="h-[calc(100dvh-var(--spacing-topbar))]!" title={title}>
        {/* `relative`: el que desplaza es este panel, no el `main` del AppShell; lo que
            lleva dentro (un `sr-only`) se ancla a él y no un piso más arriba. */}
        <div class="relative h-full min-h-0 overflow-y-auto">{children}</div>
      </AppShell>
    </SytueShell>
  );
}
