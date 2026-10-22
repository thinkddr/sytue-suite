/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * El marco de Sytue Bases (SytueShell + AppShell).
 */
import type { ComponentChildren, JSX } from 'preact';
import { AppShell } from '@sytue/ui';
import { SytueShell } from '@sytue/shell';
import type { SytueUser } from '@sytue/auth';
import { getAccessToken } from '@sytue/auth/client';
import type { Locale } from '@sytue/i18n';
import { useT } from '../i18n/index.js';
import { Nav, type Seccion } from './Nav.js';

export interface MarcoProps {
  user: SytueUser;
  hubBaseUrl: string;
  locale: Locale;
  activa: Seccion | null;
  children: ComponentChildren;
}

export function Marco({ user, hubBaseUrl, locale, activa, children }: MarcoProps): JSX.Element {
  const t = useT(locale);
  return (
    <SytueShell
      currentApp="bases"
      user={user}
      hubBaseUrl={hubBaseUrl}
      accessToken={getAccessToken}
      locale={locale}
    >
      <AppShell
        topbar={null}
        class="h-[calc(100dvh-var(--spacing-topbar))]!"
        sidebar={<Nav t={t} activa={activa} />}
        title={t('nav.title')}
      >
        {/* `relative`: el que desplaza es este panel, no el `main` del AppShell; lo que
            lleva dentro (el título oculto de la lista) se ancla a él y no un piso más arriba. */}
        <div class="relative h-full min-h-0 overflow-y-auto">{children}</div>
      </AppShell>
    </SytueShell>
  );
}
