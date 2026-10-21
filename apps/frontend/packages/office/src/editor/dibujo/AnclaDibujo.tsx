/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Lo que el panel «Comentarios» enseña de un comentario anclado a un dibujo: a qué
 * señala (forma o punto) y un botón para llevar el foco al dibujo. Si el dibujo o la
 * forma ya no existen, el comentario se CONSERVA y se marca como huérfano.
 */
import type { JSX } from 'preact';
import { Button } from '@sytue/ui';
import { useT } from '../context';
import { getEditorRoot } from '../canvas/exec';
import { estadoAnclaDibujo, leerAnclaDibujo } from '../dibujo';

export function AnclaDibujo({ anchor }: { anchor: unknown }): JSX.Element | null {
  const t = useT();
  const a = leerAnclaDibujo(anchor);
  if (!a) return null;
  const root = getEditorRoot();
  const { figura, huerfano } = root ? estadoAnclaDibujo(root, a) : { figura: null, huerfano: true };
  const donde = a.nodo !== undefined ? t('drawing.comment.onShape') : t('drawing.comment.onPoint');
  return (
    <div class="mt-1 flex flex-wrap items-center gap-1.5 text-xs">
      <span class="text-text-muted">{donde}</span>
      {huerfano && (
        <span class="border-border text-text-muted rounded-sm border px-1">
          {t('drawing.comment.orphan')}
        </span>
      )}
      {figura && (
        <Button
          size="xs"
          variant="ghost"
          onClick={() => {
            figura.scrollIntoView({
              block: 'center',
              behavior: matchMedia('(prefers-reduced-motion: reduce)').matches ? 'auto' : 'smooth',
            });
            figura.focus({ preventScroll: true });
          }}
        >
          {t('drawing.comment.goTo')}
        </Button>
      )}
    </div>
  );
}
