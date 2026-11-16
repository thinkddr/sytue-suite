/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Cómo se ENSEÑA un fallo en las pantallas de Sytue Fotos. Qué se dice lo decide
 * `lib/fallo.ts`; aquí, dónde:
 *
 * - un fallo de CARGA deja la pantalla sin nada que enseñar, así que ocupa su sitio:
 *   el estado vacío de la suite con su icono y su salida («Reintentar», o «Entrar»
 *   si es la sesión). Es el `CuerpoDeEstado` de Fyne;
 * - un fallo de ACCIÓN (restaurar, renombrar, añadir) no se lleva lo que ya se ve:
 *   es un aviso `danger` de la suite, que se queda hasta que la persona lo cierra.
 */
import type { ComponentType, JSX } from 'preact';
import { useCallback } from 'preact/hooks';
import { Button, EmptyState, ToastProvider, useToast, withBase } from '@sytue/ui';
import { I18nProvider } from '@sytue/i18n/preact';
import type { Locale } from '@sytue/i18n';
import { useT } from '../i18n/index.js';
import { pantallaDeFallo, textoDeFallo, type EstadoDeFallo } from '../lib/fallo.js';

const ICONO: Record<Exclude<EstadoDeFallo, 'sinSesion'>, 'danger' | 'cloud' | 'lock'> = {
  error: 'danger',
  sinConexion: 'cloud',
  sinPermiso: 'lock',
};

function entrar(): void {
  // `withBase(` junto a `/auth/login`, sin alias: lo busca el gate de packages/auth
  // para saber que el login lleva el prefijo de la app.
  window.location.assign(
    withBase(
      import.meta.env.BASE_URL,
      `/auth/login?returnTo=${encodeURIComponent(location.pathname + location.search)}`,
    ),
  );
}

export function FalloDeCarga({
  locale,
  fallo,
  alReintentar,
  class: cls = 'flex-1',
}: {
  locale: Locale;
  fallo: unknown;
  alReintentar: () => void;
  class?: string;
}): JSX.Element {
  const t = useT(locale);
  const p = pantallaDeFallo(locale, fallo, t('app.title'));
  // En el gris del estado vacío, como en Fyne: el fallo se dice con el texto y la
  // salida, no tiñendo la pantalla de rojo. Por eso `icon` y no `tone="error"`, que
  // pintaría el glifo en `danger-text`; el `role="alert"` lo pone el envoltorio.
  const marca =
    p.estado === 'sinSesion' ? { spot: 'emptyPhotos' as const } : { icon: ICONO[p.estado] };
  const salida = p.salida ? (
    <Button
      variant={p.salida === 'entrar' ? 'primary' : 'secondary'}
      size="sm"
      onClick={p.salida === 'entrar' ? entrar : alReintentar}
    >
      {p.rotulo}
    </Button>
  ) : null;
  return (
    <div role="alert" class={cls}>
      <EmptyState
        class="h-full"
        title={p.titulo}
        {...marca}
        {...(p.descripcion ? { description: p.descripcion } : {})}
        {...(salida ? { action: salida } : {})}
      />
    </div>
  );
}

/** Avisa de un fallo de acción con un toast `danger` en el idioma de la persona. */
export function useAvisoDeFallo(locale: Locale): (e: unknown) => void {
  const { notify } = useToast();
  return useCallback(
    (e: unknown) => {
      notify({ tone: 'danger', title: textoDeFallo(locale, e) });
    },
    [notify, locale],
  );
}

/**
 * Envuelve una pantalla en el proveedor de avisos. Hace falta POR ENCIMA de la
 * pantalla, no dentro: la pantalla llama a `useAvisoDeFallo` en su propio cuerpo, y
 * el `ToastProvider` de la Sytue Shell queda por debajo (lo monta `Marco`). La
 * Shell anidada reutiliza esta misma cola, así que no hay dos pilas de avisos.
 */
export function conAvisos<P extends { locale: Locale }>(
  Pantalla: ComponentType<P>,
): (props: P) => JSX.Element {
  return function ConAvisos(props: P): JSX.Element {
    return (
      <I18nProvider locale={props.locale}>
        <ToastProvider>
          <Pantalla {...props} />
        </ToastProvider>
      </I18nProvider>
    );
  };
}
