/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Los diseños dentro del patrón: en «Editar patrón», qué se edita —el patrón o uno de los
 * diseños— y cómo se gestionan (nuevo, duplicar, quitar, renombrar). La lógica pura está en
 * disenos.ts; aquí solo el pintado.
 */
import type { JSX } from 'preact';
import { EditorIcon } from '../icons';
import { useT } from '../context';
import type { SlideLayout } from '../types';
import { MAX_LAYOUT_NAME } from './disenos';

const FOCUS_RING = 'sytue-focus-ring';
const FIELD = `border-border bg-surface text-text h-6 min-h-[24px] rounded-sm border px-1 text-xs ${FOCUS_RING}`;
const BTN = `border-border bg-surface text-text hover:bg-bg-subtle inline-flex h-6 min-h-[24px] shrink-0 items-center gap-1 rounded-sm border px-2 text-xs disabled:opacity-40 ${FOCUS_RING}`;

export function DisenosBarra(props: {
  layouts: SlideLayout[];
  /** Id del diseño que se edita; null = el patrón. */
  editing: string | null;
  canAdd: boolean;
  onSelect: (id: string | null) => void;
  onAdd: () => void;
  onDuplicate: () => void;
  onDelete: () => void;
  onRename: (name: string) => void;
}): JSX.Element {
  const t = useT();
  const current = props.layouts.find((l) => l.id === props.editing);
  return (
    <div class="flex flex-wrap items-center gap-2">
      <label class="text-text-muted flex items-center gap-1 text-xs">
        {t('slides.layout.editing')}
        <select
          class={`${FIELD} w-44`}
          value={props.editing ?? ''}
          onChange={(e) =>
            props.onSelect(e.currentTarget.value === '' ? null : e.currentTarget.value)
          }
        >
          <option value="">{t('slides.layout.master')}</option>
          {props.layouts.map((l) => (
            <option key={l.id} value={l.id}>
              {l.name || l.id}
            </option>
          ))}
        </select>
      </label>
      {current && (
        <input
          key={current.id}
          class={`${FIELD} w-40`}
          value={current.name}
          maxLength={MAX_LAYOUT_NAME}
          aria-label={t('slides.layout.name')}
          onChange={(e) => props.onRename(e.currentTarget.value)}
        />
      )}
      <button type="button" class={BTN} disabled={!props.canAdd} onClick={props.onAdd}>
        <EditorIcon name="plus" size="sm" /> {t('slides.layout.new')}
      </button>
      {current && (
        <>
          <button type="button" class={BTN} disabled={!props.canAdd} onClick={props.onDuplicate}>
            <EditorIcon name="slideAdd" size="sm" /> {t('slides.layout.duplicate')}
          </button>
          <button type="button" class={BTN} onClick={props.onDelete}>
            <EditorIcon name="trash" size="sm" /> {t('slides.layout.delete')}
          </button>
        </>
      )}
    </div>
  );
}
