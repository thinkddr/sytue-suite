/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Los estados que comparten las páginas de Cast (CAST-ENCARGO §10): cargando (la
 * rejilla en esqueleto con la forma de lo que va a llegar), sin conexión, sin
 * permiso y error. «Vacía» la pinta cada página, porque lo que se dice depende de
 * dónde se está; «lista» es la página.
 */
import type { JSX } from 'preact';
import { useEffect, useState } from 'preact/hooks';
import { Button, EmptyState, Icon, Illustration, Skeleton } from '@sytue/ui';
import type { T } from '../i18n/index.js';
import type { TipoDeFallo } from '../lib/fallo.js';

/** La rejilla de tarjetas 16:9, de 1 a 4 columnas según el ancho. */
export const REJILLA =
  'grid grid-cols-1 gap-x-4 gap-y-6 sm:grid-cols-2 lg:grid-cols-3 2xl:grid-cols-4';

export function CargandoRejilla({
  n = 8,
  etiqueta,
}: {
  n?: number;
  etiqueta: string;
}): JSX.Element {
  return (
    <div class={REJILLA} role="status" aria-busy="true" aria-label={etiqueta}>
      {Array.from({ length: n }, (_, i) => (
        <div key={i} class="flex flex-col gap-3">
          <Skeleton height="auto" class="rounded-card! aspect-video w-full" />
          <Skeleton height="1rem" width="80%" />
          <Skeleton height="0.75rem" width="50%" />
        </div>
      ))}
    </div>
  );
}

export function Fallo({
  t,
  tipo,
  reintentar,
}: {
  t: T;
  tipo: TipoDeFallo;
  reintentar?: (() => void) | undefined;
}): JSX.Element {
  if (tipo === 'offline') {
    return (
      <EmptyState
        illustration={<Icon name="cloud" size={48} />}
        title={t('errors.offline.title')}
        description={t('errors.offline.description')}
        {...(reintentar
          ? {
              action: (
                <Button variant="secondary" onClick={reintentar}>
                  {t('errors.retry')}
                </Button>
              ),
            }
          : {})}
      />
    );
  }
  if (tipo === 'sinPermiso') {
    return (
      <EmptyState
        illustration={<Icon name="lock" size={48} />}
        title={t('errors.forbidden.title')}
        description={t('errors.forbidden.description')}
      />
    );
  }
  return (
    <EmptyState
      illustration={<Illustration name="errorRetry" size={160} />}
      title={t('errors.title')}
      description={t('errors.description')}
      {...(reintentar
        ? {
            action: (
              <Button variant="secondary" onClick={reintentar}>
                {t('errors.retry')}
              </Button>
            ),
          }
        : {})}
    />
  );
}

/** `navigator.onLine`, al día con los eventos del navegador. */
export function useEnLinea(): boolean {
  const [enLinea, setEnLinea] = useState(true);
  useEffect(() => {
    const act = (): void => setEnLinea(navigator.onLine);
    act();
    window.addEventListener('online', act);
    window.addEventListener('offline', act);
    return () => {
      window.removeEventListener('online', act);
      window.removeEventListener('offline', act);
    };
  }, []);
  return enLinea;
}
