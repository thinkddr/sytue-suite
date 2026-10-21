// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
/**
 * «Tareas»: las tareas de todos los cuadernos en una lista, como en Twos. Se
 * marcan, se cancelan y se posponen sin abrir su cuaderno; pulsar el título lo abre.
 */
import type { JSX } from 'preact';
import { useState } from 'preact/hooks';
import {
  Button,
  Checkbox,
  Dropdown,
  FieldGroup,
  Icon,
  IconButton,
  Input,
  MenuItem,
  Modal,
  Spinner,
  cx,
} from '@sytue/ui';
import { intlTag, type Locale } from '@sytue/i18n';
import type { Task, TaskState } from '../client/notesApi.js';
import type { T } from '../i18n/index.js';
import { diaLocal, terminada, vecesTrasladada } from '../client/diario.js';

const ESTADOS = [
  ['open', 'filter.unchecked'],
  ['done', 'filter.checked'],
  ['cancelled', 'tasks.cancelled'],
] as const satisfies readonly (readonly [TaskState, string])[];

export function PanelTareas({
  t,
  locale,
  estado,
  tareas,
  onEstado,
  onAbrir,
  onCambio,
  onPosponer,
  onManana,
}: {
  t: T;
  locale: Locale;
  estado: TaskState;
  /** null mientras carga. */
  tareas: Task[] | null;
  onEstado: (e: TaskState) => void;
  onAbrir: (t: Task) => void;
  onCambio: (t: Task, patch: { checked?: boolean; cancelled?: boolean }) => void;
  onPosponer: (t: Task) => void;
  onManana: (t: Task) => void;
}): JSX.Element {
  const hoy = diaLocal();
  // Dónde vive cada tarea: su día («Hoy», o la fecha) o su cuaderno. Las de un mismo
  // sitio van bajo un solo rótulo, en vez de repetirlo debajo de cada una.
  const dondeEsta = (x: Task) =>
    x.daily_date
      ? x.daily_date === hoy
        ? t('daily.today')
        : new Date(`${x.daily_date}T12:00:00`).toLocaleDateString(intlTag(locale), {
            weekday: 'long',
            day: 'numeric',
            month: 'long',
          })
      : x.notebook || t('notebook.untitled');
  const grupos: { donde: string; tareas: Task[] }[] = [];
  for (const x of tareas ?? []) {
    const donde = dondeEsta(x);
    const ultimo = grupos[grupos.length - 1];
    if (ultimo && ultimo.donde === donde) ultimo.tareas.push(x);
    else grupos.push({ donde, tareas: [x] });
  }
  return (
    <div class="notes-col notes-page">
      <div class="notes-page-head">
        <h2 class="notes-page-title">{t('tasks.title')}</h2>
        <div class="notes-seg -mr-2" role="group" aria-label={t('tasks.title')}>
          {ESTADOS.map(([e, rotulo]) => (
            <button
              type="button"
              key={e}
              class="notes-seg-item sytue-focus-ring"
              aria-pressed={estado === e}
              onClick={() => onEstado(e)}
            >
              {t(rotulo)}
            </button>
          ))}
        </div>
      </div>
      {tareas === null ? (
        <div class="grid place-items-center p-6">
          <Spinner />
        </div>
      ) : tareas.length === 0 ? (
        <p class="text-text-subtle text-base">{t(`tasks.empty.${estado}`)}</p>
      ) : (
        grupos.map((g) => (
          <section
            key={`${g.donde}:${g.tareas[0]!.id}`}
            class="notes-group-block"
            aria-label={g.donde}
          >
            <h3 class="notes-group">{g.donde}</h3>
            <ul class="notes-rows">
              {g.tareas.map((x) => (
                <li key={x.id} class="notes-row">
                  <span class="notes-stream-glyph">
                    <Checkbox
                      checked={x.checked}
                      aria-label={x.title || t('note.untitled')}
                      onChange={(e) =>
                        onCambio(x, { checked: (e.target as HTMLInputElement).checked })
                      }
                      class="border-border h-4 w-4 shrink-0 rounded accent-[var(--sytue-primary)]"
                    />
                  </span>
                  <button
                    type="button"
                    onClick={() => onAbrir(x)}
                    class={cx(
                      'sytue-focus-ring text-text min-w-0 flex-1 truncate rounded-sm text-left text-base',
                      terminada(x) && 'text-text-muted line-through',
                    )}
                  >
                    {x.title || t('note.untitled')}
                  </button>
                  {vecesTrasladada(x) > 0 && !terminada(x) ? (
                    <span class="notes-carry">
                      {t('task.carried', { count: vecesTrasladada(x) })}
                    </span>
                  ) : null}
                  <span class="notes-row-actions">
                    {estado === 'open' ? (
                      <Button variant="ghost" size="xs" onClick={() => onManana(x)}>
                        {t('task.tomorrow')}
                      </Button>
                    ) : null}
                    <Dropdown
                      label={t('common.more')}
                      align="end"
                      trigger={(props) => (
                        <IconButton {...props} label={t('common.more')} size="sm">
                          <Icon name="ellipsis" size={16} />
                        </IconButton>
                      )}
                    >
                      <MenuItem onClick={() => onAbrir(x)}>{t('tasks.openNotebook')}</MenuItem>
                      {estado === 'open' ? (
                        <MenuItem onClick={() => onPosponer(x)}>{t('task.postpone')}</MenuItem>
                      ) : null}
                      <MenuItem onClick={() => onCambio(x, { cancelled: !x.cancelled_at })}>
                        {t(x.cancelled_at ? 'task.reopen' : 'task.cancel')}
                      </MenuItem>
                    </Dropdown>
                  </span>
                </li>
              ))}
            </ul>
          </section>
        ))
      )}
    </div>
  );
}

/** «Posponer»: mañana con un clic, o el día que se elija. */
export function PosponerModal({
  t,
  abierto,
  manana,
  onClose,
  onPick,
}: {
  t: T;
  abierto: boolean;
  /** `YYYY-MM-DD` de mañana, que es también el valor inicial del campo. */
  manana: string;
  onClose: () => void;
  onPick: (date: string) => void;
}): JSX.Element {
  const [dia, setDia] = useState(manana);
  return (
    <Modal
      open={abierto}
      onClose={onClose}
      title={t('task.postpone')}
      size="sm"
      footer={
        <>
          <Button variant="secondary" onClick={onClose}>
            {t('common.cancel')}
          </Button>
          <Button disabled={!/^\d{4}-\d{2}-\d{2}$/.test(dia)} onClick={() => onPick(dia)}>
            {t('task.postponeAction')}
          </Button>
        </>
      }
    >
      <div class="grid gap-3">
        <Button variant="secondary" onClick={() => onPick(manana)}>
          {t('task.tomorrow')}
        </Button>
        <FieldGroup label={t('task.postponeDate')} htmlFor="notes-posponer-dia">
          <Input
            id="notes-posponer-dia"
            type="date"
            value={dia}
            min={manana}
            onInput={(e) => setDia(e.currentTarget.value)}
          />
        </FieldGroup>
      </div>
    </Modal>
  );
}
