/* SPDX-License-Identifier: Apache-2.0 */
/**
 * El armazón de Mail mientras se decide dónde está el correo y se abre la bóveda
 * (ADR 0086, «La puerta no bloquea la app»): la barra de la suite —con el candado
 * de la bóveda abriéndose—, el menú lateral y la lista en esqueleto. Antes, nada
 * hasta que contestaba la autoridad y después la pantalla de la puerta.
 *
 * No lee nada del correo: todo lo de aquí se pinta antes de abrir la bóveda.
 */
import type { JSX } from 'preact';
import { SytueShell } from '@sytue/shell';
import { AppShell, Sidebar, Skeleton, SkeletonRows } from '@sytue/ui';
import { getAccessToken } from '@sytue/auth/client';
import type { SytueUser } from '@sytue/auth';
import type { Locale } from '@sytue/i18n';

export function EsqueletoCorreo({
  user,
  hubBaseUrl,
  locale,
  franja,
}: {
  user: SytueUser;
  hubBaseUrl: string;
  locale: Locale;
  /** La franja «Desbloqueando…» de la puerta, encima de la lista. */
  franja?: JSX.Element | undefined;
}): JSX.Element {
  return (
    <SytueShell
      currentApp="mail"
      user={user}
      hubBaseUrl={hubBaseUrl}
      accessToken={getAccessToken}
      locale={locale}
    >
      <AppShell
        topbar={null}
        class="h-[calc(100dvh-var(--spacing-topbar))]!"
        sidebar={
          <Sidebar>
            <div class="flex flex-col gap-3 px-4 py-4" aria-hidden="true">
              <Skeleton width="100%" height="2.25rem" />
              {[70, 55, 62, 48, 58, 44].map((ancho) => (
                <Skeleton key={ancho} width={`${ancho}%`} height="1rem" />
              ))}
            </div>
          </Sidebar>
        }
        secondaryPanel={
          <div class="flex flex-col">
            {franja}
            <SkeletonRows rows={10} lead="avatar" lines={2} meta />
          </div>
        }
      >
        <div />
      </AppShell>
    </SytueShell>
  );
}
