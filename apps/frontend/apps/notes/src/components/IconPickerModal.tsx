/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * IconPickerModal — selector de icono predefinido para una nota.
 */
import type { JSX } from 'preact';
import { Modal, cx } from '@sytue/ui';
import type { Note } from '@sytue/api-clients';
import type { NotesMessageKey, T } from '../i18n/index.js';
import { Glyph, type GlyphName } from './glyphs.js';

export interface PredefinedIcon {
  id: string;
  glyph?: GlyphName;
}

/**
 * Los iconos que se ofrecen, EN SU ORDEN. El rótulo de cada uno es `icon.<id>` del
 * catálogo (`icon.none` para quitarlo): estaban escritos a mano en español y la app
 * nativa, que solo pinta claves de catálogo, no podía ofrecer este selector.
 */
export const ICONOS_PREDEFINIDOS: PredefinedIcon[] = [
  { id: '' },
  { id: 'note', glyph: 'note' },
  { id: 'todo', glyph: 'todo' },
  { id: 'file', glyph: 'file' },
  { id: 'link', glyph: 'link' },
  { id: 'palette', glyph: 'palette' },
  { id: 'pin', glyph: 'pin' },
  { id: 'quote', glyph: 'quote' },
  { id: 'sparkles', glyph: 'sparkles' },
  { id: 'send', glyph: 'send' },
  { id: 'table', glyph: 'table' },
  { id: 'heading', glyph: 'heading' },
  { id: 'unlock', glyph: 'unlock' },
  { id: 'filter', glyph: 'filter' },
];

/** El rótulo de un icono del selector, del catálogo. */
export function rotuloIcono(t: T, id: string): string {
  return t(`icon.${id || 'none'}` as NotesMessageKey);
}

export function IconPickerModal({
  nota,
  onClose,
  onPick,
  t,
}: {
  nota: Note | null;
  onClose: () => void;
  onPick: (icono: string) => void;
  t: T;
}): JSX.Element {
  const actual = (nota?.data?.icon as string) || '';

  return (
    <Modal open={nota !== null} onClose={onClose} title={t('note.setIcon')} size="sm">
      <div class="grid grid-cols-3 gap-2 p-1 sm:grid-cols-4">
        {ICONOS_PREDEFINIDOS.map((item) => {
          const seleccionado = actual === item.id;
          return (
            <button
              key={item.id}
              type="button"
              onClick={() => onPick(item.id)}
              class={cx(
                'sytue-focus-ring flex flex-col items-center justify-center gap-1.5 rounded-lg border p-3 text-sm transition-colors',
                seleccionado
                  ? 'border-primary bg-primary/10 text-primary-text ring-primary font-medium ring-1'
                  : 'border-border bg-surface text-text hover:bg-bg-subtle',
              )}
            >
              {item.glyph ? (
                <Glyph name={item.glyph} size={22} class="shrink-0" />
              ) : (
                <span class="text-text-muted text-xs">—</span>
              )}
              <span class="truncate">{rotuloIcono(t, item.id)}</span>
            </button>
          );
        })}
      </div>
    </Modal>
  );
}
