/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * El marco de Sytue Web (SytueShell + AppShell).
 */
import type { ComponentChildren, JSX } from 'preact';
import { AppShell } from '@sytue/ui';
import { SytueShell } from '@sytue/shell';
import type { SytueUser } from '@sytue/auth';
import { getAccessToken } from '@sytue/auth/client';
import type { Locale } from '@sytue/i18n';
import { t as translate, type ClaveMensaje } from '../i18n/index.js';
import { Nav, type Seccion } from './Nav.js';

export interface MarcoProps {
  user: SytueUser;
  hubBaseUrl: string;
  locale: Locale;
  activa: Seccion | null;
  children: ComponentChildren;
}

export function Marco({ user, hubBaseUrl, locale, activa, children }: MarcoProps): JSX.Element {
  const t = (k: ClaveMensaje, params?: Record<string, string | number>) =>
    translate(locale, k, params);
  // `relative`: el que desplaza es este panel, no el `main` del AppShell; lo que lleva
  // dentro (los `sr-only` de «Nuevo sitio») se ancla a él y no un piso más arriba.
  const contenido = <div class="relative h-full min-h-0 overflow-y-auto">{children}</div>;

  return (
    <SytueShell
      currentApp="pages"
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
        {contenido}
      </AppShell>
    </SytueShell>
  );
}
