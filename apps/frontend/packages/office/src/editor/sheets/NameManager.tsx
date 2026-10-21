/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Administrador de nombres (pestaña Datos): la lista de nombres definidos del
 * libro con su ámbito y su definición, y un formulario para crear o editar uno.
 * Valida mientras se escribe con las mismas reglas que el lienzo
 * (sheets/nombres.ts); el lienzo vuelve a validar al recibir el comando, con el
 * libro entero delante (tablas incluidas), y avisa si algo no cuadra.
 */
import type { JSX } from 'preact';
import { useState } from 'preact/hooks';
import { BOTON_DE_BARRA, Button, Input, Select } from '@sytue/ui';
import { useT } from '../context';
import { EditorIcon } from '../icons';
import { POPOVER_SHELL, usePopover } from '../ribbon/primitives';
import type { DefinedName } from '../types';
import { nameError, normalizeRef } from './nombres';
import { dispatchSheetCommand, readSheetState } from './SheetCanvas';

/** Lo que se está escribiendo; `prev` = el nombre que se edita (null: uno nuevo). */
interface Draft {
  name: string;
  ref: string;
  sheet: string; // '' = libro
  comment: string;
  prev: DefinedName | null;
}

export function NameManagerMenu(): JSX.Element {
  const t = useT();
  const { open, setOpen, rootRef } = usePopover();
  const st = readSheetState();
  const names = st?.names ?? [];
  const sheets = st?.sheetNames ?? [];
  const [d, setD] = useState<Draft>({ name: '', ref: '', sheet: '', comment: '', prev: null });

  /** Formulario vacío con la selección como referencia («Hoja1!$A$1:$B$5»). */
  const fresh = (): Draft => {
    const s = readSheetState();
    const ref = normalizeRef(s?.selection ?? '', s?.activeSheet ?? '');
    return { name: '', ref, sheet: '', comment: '', prev: null };
  };
  const draft: DefinedName = {
    name: d.name,
    ref: d.ref,
    ...(d.sheet !== '' ? { sheet: d.sheet } : {}),
  };
  // Un nombre vacío todavía no es un error que enseñar: es que no se ha empezado.
  const err =
    d.name.trim() === '' ? null : nameError(draft, { names, sheets }, d.prev ?? undefined);
  const canSave = d.name.trim() !== '' && err === null;

  const save = (): void => {
    if (!canSave) return;
    dispatchSheetCommand(
      'nameset',
      JSON.stringify({
        name: d.name.trim(),
        ref: d.ref,
        sheet: d.sheet,
        comment: d.comment,
        ...(d.prev ? { prev: { name: d.prev.name, sheet: d.prev.sheet ?? '' } } : {}),
      }),
    );
    setD(fresh());
  };

  const field = (label: string, input: JSX.Element): JSX.Element => (
    <label class="block pb-1.5">
      <span class="sytue-meta-dense mb-0.5 block">{label}</span>
      {input}
    </label>
  );

  return (
    <div ref={rootRef} class="relative">
      <button
        type="button"
        class={BOTON_DE_BARRA}
        aria-label={t('names.title')}
        title={t('names.hint')}
        aria-haspopup="true"
        aria-expanded={open}
        onMouseDown={(e) => e.preventDefault()}
        onClick={() => {
          if (!open) setD(fresh());
          setOpen(!open);
        }}
      >
        <EditorIcon name="definedName" />
      </button>
      {open && (
        <div
          role="group"
          aria-label={t('names.title')}
          class={`${POPOVER_SHELL} left-0 w-80 max-w-[calc(100vw-16px)] p-2`}
        >
          <div class="text-text pb-1 text-xs font-medium">{t('names.title')}</div>

          {names.length > 0 ? (
            <ul class="border-border mb-2 max-h-48 overflow-auto border-b pb-2">
              {names.map((n) => (
                <li
                  key={`${n.sheet ?? ''}:${n.name}`}
                  class="text-text flex items-center gap-1 py-0.5 text-xs"
                >
                  <span class="min-w-0 flex-1" title={n.comment ?? n.ref}>
                    <span class="flex items-baseline gap-1.5">
                      <span class="truncate font-medium">{n.name}</span>
                      <span class="text-text-subtle text-2xs shrink-0">
                        {n.sheet ?? t('names.scope.book')}
                      </span>
                    </span>
                    <span class="text-text-muted block truncate tabular-nums">={n.ref}</span>
                  </span>
                  <Button
                    variant="ghost"
                    size="xs"
                    iconOnly
                    aria-label={t('names.editNamed', { name: n.name })}
                    title={t('action.edit')}
                    onClick={() =>
                      setD({
                        name: n.name,
                        ref: n.ref,
                        sheet: n.sheet ?? '',
                        comment: n.comment ?? '',
                        prev: n,
                      })
                    }
                  >
                    <EditorIcon name="pencil" size="sm" />
                  </Button>
                  <Button
                    variant="ghost"
                    size="xs"
                    iconOnly
                    aria-label={t('names.deleteNamed', { name: n.name })}
                    title={t('action.removeItem')}
                    onClick={() => {
                      dispatchSheetCommand(
                        'namedel',
                        JSON.stringify({ name: n.name, sheet: n.sheet ?? '' }),
                      );
                      if (d.prev === n) setD(fresh());
                    }}
                  >
                    <EditorIcon name="trash" size="sm" />
                  </Button>
                </li>
              ))}
            </ul>
          ) : (
            <div class="sytue-meta-dense pb-2">{t('names.empty')}</div>
          )}

          <div class="sytue-overline text-text-subtle pb-1">
            {d.prev ? t('names.editing', { name: d.prev.name }) : t('common.newName')}
          </div>
          {field(
            t('common.name'),
            <Input
              size="sm"
              value={d.name}
              invalid={err !== null && err !== 'badRef'}
              onInput={(e) => setD({ ...d, name: e.currentTarget.value })}
              onKeyDown={(e) => {
                if (e.key === 'Enter') save();
              }}
            />,
          )}
          {field(
            t('names.ref'),
            <Input
              size="sm"
              value={d.ref}
              invalid={err === 'badRef'}
              placeholder="Hoja1!$A$1:$A$10"
              onInput={(e) => setD({ ...d, ref: e.currentTarget.value })}
              onKeyDown={(e) => {
                if (e.key === 'Enter') save();
              }}
            />,
          )}
          {field(
            t('names.scope'),
            <Select
              size="sm"
              value={d.sheet}
              onChange={(e) => setD({ ...d, sheet: e.currentTarget.value })}
            >
              <option value="">{t('names.scope.book')}</option>
              {sheets.map((s) => (
                <option key={s} value={s}>
                  {s}
                </option>
              ))}
            </Select>,
          )}
          {field(
            t('names.comment'),
            <Input
              size="sm"
              value={d.comment}
              onInput={(e) => setD({ ...d, comment: e.currentTarget.value })}
            />,
          )}
          {err ? (
            <div class="text-danger-text text-2xs pb-1.5" role="alert">
              {t(`names.error.${err}`)}
            </div>
          ) : null}
          <div class="flex gap-1.5">
            <Button variant="secondary" size="xs" class="flex-1" disabled={!canSave} onClick={save}>
              {d.prev ? t('action.save') : t('action.add')}
            </Button>
            {d.prev ? (
              <Button variant="ghost" size="xs" onClick={() => setD(fresh())}>
                {t('action.cancel')}
              </Button>
            ) : null}
          </div>
        </div>
      )}
    </div>
  );
}
