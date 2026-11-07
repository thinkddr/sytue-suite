/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * El desplegable del historial de versiones: pide la lista al abrirse (§6.3) y, al
 * elegir una, confirma antes de restaurarla. Lo abren dos disparadores —«Historial» de
 * la pestaña Colaboración y el chip «Guardado hace…» de la cabecera— y por eso vive
 * aquí y no dentro de ninguno de los dos: con dos copias, una confirmaría y la otra no.
 *
 * El gemelo de Fyne es `Contexto.abrirHistorialEn` (apps/fyne/editor/colaboracion.go).
 */
import type { ComponentChildren, JSX } from 'preact';
import { Dropdown, MenuItem, useConfirm, useToast } from '@sytue/ui';
import { formatDateTime } from '@sytue/i18n';
import type { EditorContext } from './types';

/** Props del disparador: las de Dropdown, con el `onClick` que ya pide la lista. */
export interface DisparadorHistorial {
  onClick: () => void;
  'aria-expanded': boolean;
  'aria-haspopup': 'menu';
}

export function MenuHistorial({
  ctx,
  trigger,
  align = 'start',
}: {
  ctx: EditorContext;
  trigger: (props: DisparadorHistorial) => ComponentChildren;
  align?: 'start' | 'end';
}): JSX.Element {
  const t = ctx.t;
  const { confirmar, dialogo } = useConfirm();
  const toast = useToast();

  const restaurar = async (id: string): Promise<void> => {
    const ok = await confirmar({
      titulo: t('versions.restoreTitle'),
      mensaje: t('versions.restoreConfirm'),
      confirmLabel: t('versions.restoreAction'),
    });
    if (!ok) return;
    void ctx
      .restoreVersion(id)
      .then(() => toast.notify({ tone: 'success', title: t('versions.restored') }))
      .catch(() => toast.notify({ tone: 'danger', title: t('versions.restoreError') }));
  };

  return (
    <>
      <Dropdown
        label={t('versions.history')}
        align={align}
        trigger={(props) =>
          trigger({
            ...props,
            onClick: () => {
              // Al abrir, refrescar la lista (§6.3); reabrir también refresca.
              if (!props['aria-expanded']) void ctx.loadVersions();
              props.onClick();
            },
          })
        }
      >
        {ctx.versions.length === 0 ? (
          /* Texto estático (sin role=menuitem): no debe recibir foco del menú. */
          <p class="text-text-muted px-2.5 py-2 text-sm">{t('versions.empty')}</p>
        ) : (
          ctx.versions.map((v) => (
            <MenuItem key={v.id} onClick={() => void restaurar(v.id)}>
              {`${v.label || t('versions.untitled')} — ${formatDateTime(ctx.locale, v.created_at, {
                dateStyle: 'short',
                timeStyle: 'short',
              })}`}
            </MenuItem>
          ))
        )}
      </Dropdown>
      {dialogo}
    </>
  );
}
