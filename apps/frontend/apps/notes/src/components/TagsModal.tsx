/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * TagsModal — el editor de etiquetas CON ESTADO.
 *
 * Una etiqueta aquí no es una palabra suelta: es un eje («Prioridad») con estados
 * excluyentes («Alta», «Media», «Baja»). Marcar uno sustituye al anterior de la
 * MISMA etiqueta, que es lo que la distingue de una lista de palabras y lo que
 * permite filtrar por «prioridad alta» sin que nadie pueda estar en dos a la vez.
 *
 * Cada estado puede llevar un atajo de un carácter: con una nota seleccionada,
 * pulsarlo aplica el estado. Es lo que hace que etiquetar cincuenta notas sea
 * rápido en vez de cincuenta veces tres clics.
 */
import type { JSX } from 'preact';
import { useState } from 'preact/hooks';
import { Button, Icon, Input, Modal, cx, useConfirm } from '@sytue/ui';
import type { Tag, TagState } from '@sytue/api-clients';
import type { T } from '../i18n/index.js';

interface Props {
  t: T;
  tags: Tag[];
  onClose: () => void;
  onSave: (input: { name: string; states: TagState[] }, id?: string) => Promise<void>;
  onDelete: (id: string) => Promise<void>;
}

const COLORES = ['#b0413a', '#8a6a1e', '#16794a', '#0e7490', '#1c58d9', '#86198f', '#565b62'];

/** Una etiqueta vacía, para empezar. Nace con un estado: una etiqueta sin
 *  estados no se puede aplicar a nada, así que no tiene sentido guardarla. */
const NUEVA = (): { name: string; states: TagState[] } => ({
  name: '',
  states: [{ name: '', color: COLORES[0]!, emoji: '', shortcut: '' }],
});

export function TagsModal({ t, tags, onClose, onSave, onDelete }: Props): JSX.Element {
  const { confirmar, dialogo } = useConfirm();
  const [editando, setEditando] = useState<{
    id?: string;
    name: string;
    states: TagState[];
  } | null>(null);
  const [guardando, setGuardando] = useState(false);

  const guardar = async () => {
    if (!editando || editando.name.trim() === '') return;
    const states = editando.states.filter((s) => s.name.trim() !== '');
    if (states.length === 0) return;
    setGuardando(true);
    try {
      await onSave({ name: editando.name.trim(), states }, editando.id);
      setEditando(null);
    } finally {
      setGuardando(false);
    }
  };

  const cambiarEstado = (i: number, patch: Partial<TagState>) =>
    setEditando((prev) =>
      prev
        ? { ...prev, states: prev.states.map((s, j) => (j === i ? { ...s, ...patch } : s)) }
        : prev,
    );

  return (
    <Modal
      open
      onClose={onClose}
      title={t('tags.title')}
      // Al editar, «Cancelar» y «Guardar» en el pie del diálogo y no al final del cuerpo.
      footer={
        editando ? (
          <>
            <Button variant="ghost" onClick={() => setEditando(null)}>
              {t('common.cancel')}
            </Button>
            <Button loading={guardando} onClick={() => void guardar()}>
              {t('common.save')}
            </Button>
          </>
        ) : undefined
      }
    >
      {dialogo}
      {editando ? (
        <div class="grid gap-3">
          <Input
            value={editando.name}
            aria-label={t('tags.name')}
            placeholder={t('tags.name')}
            autofocus
            onInput={(e) =>
              setEditando({ ...editando, name: (e.target as HTMLInputElement).value })
            }
          />

          <div class="grid gap-2">
            <p class="text-text text-base font-medium">{t('tags.states')}</p>
            {editando.states.map((s, i) => (
              // El índice como clave es correcto aquí: la lista se reordena solo
              // añadiendo o quitando al final, y los estados nuevos no tienen id.
              // eslint-disable-next-line react/no-array-index-key
              // En el teléfono, los colores bajan a su línea: en una, el nombre del
              // estado se quedaba en 26 px y no se leía ninguno.
              <div key={i} class="flex items-center gap-1.5 max-md:flex-wrap">
                <Input
                  value={s.emoji ?? ''}
                  maxLength={4}
                  aria-label={t('notebook.emoji')}
                  class="w-14 text-center"
                  onInput={(e) => cambiarEstado(i, { emoji: (e.target as HTMLInputElement).value })}
                />
                <Input
                  value={s.name}
                  aria-label={t('tags.stateName')}
                  placeholder={t('tags.stateName')}
                  class="flex-1"
                  onInput={(e) => cambiarEstado(i, { name: (e.target as HTMLInputElement).value })}
                />
                <Input
                  value={s.shortcut ?? ''}
                  maxLength={1}
                  aria-label={t('tags.shortcut')}
                  class="w-12 text-center"
                  onInput={(e) =>
                    cambiarEstado(i, { shortcut: (e.target as HTMLInputElement).value })
                  }
                />
                <span class="flex shrink-0 gap-1 max-md:order-last max-md:basis-full max-md:pl-[3.875rem] pointer-coarse:gap-6">
                  {COLORES.map((c) => (
                    <button
                      key={c}
                      type="button"
                      aria-label={c}
                      aria-pressed={s.color === c}
                      onClick={() => cambiarEstado(i, { color: c })}
                      style={{ background: c }}
                      // El punto se queda en 20; con el dedo el BLANCO es de 44
                      // (`after:-inset-3`) y el paso entre puntos también (`gap-6`).
                      class={cx(
                        "relative h-5 w-5 rounded-full pointer-coarse:after:absolute pointer-coarse:after:-inset-3 pointer-coarse:after:content-['']",
                        s.color === c &&
                          'ring-text ring-2 ring-offset-1 ring-offset-[var(--sytue-bg)]',
                      )}
                    />
                  ))}
                </span>
                {editando.states.length > 1 ? (
                  <button
                    type="button"
                    aria-label={t('common.delete')}
                    onClick={() =>
                      setEditando({
                        ...editando,
                        states: editando.states.filter((_, j) => j !== i),
                      })
                    }
                    class="sytue-focus-ring hover:bg-bg-subtle grid shrink-0 place-items-center rounded p-1 pointer-coarse:min-h-[max(44px,2.75rem)] pointer-coarse:min-w-[max(44px,2.75rem)]"
                  >
                    <Icon name="close" size={14} />
                  </button>
                ) : null}
              </div>
            ))}
            <p class="sytue-meta">{t('tags.shortcutHint')}</p>
            <Button
              size="sm"
              variant="secondary"
              onClick={() =>
                setEditando({
                  ...editando,
                  states: [
                    ...editando.states,
                    {
                      name: '',
                      color: COLORES[editando.states.length % COLORES.length]!,
                      emoji: '',
                      shortcut: '',
                    },
                  ],
                })
              }
            >
              {t('tags.addState')}
            </Button>
          </div>
        </div>
      ) : (
        <div class="grid gap-2">
          {tags.length === 0 ? (
            <p class="text-text-muted text-base">{t('tags.none')}</p>
          ) : (
            <ul class="grid gap-1">
              {tags.map((tag) => (
                <li
                  key={tag.id}
                  // En el teléfono, los estados bajan a su línea, a todo el ancho: la
                  // fila pedía 415 y «Eliminar» se salía del panel.
                  class="border-border flex items-center gap-2 rounded-md border px-2.5 py-2 max-md:flex-wrap"
                >
                  <span class="text-text min-w-0 flex-1 truncate text-base font-medium">
                    {tag.name}
                  </span>
                  <span class="flex flex-wrap gap-1 max-md:order-last max-md:basis-full">
                    {tag.states.map((s) => (
                      <span
                        key={s.id}
                        class="rounded-full px-2 py-0.5 text-sm text-[var(--sytue-on-primary)]"
                        style={{ background: s.color }}
                      >
                        {s.emoji ? `${s.emoji} ` : ''}
                        {s.name}
                      </span>
                    ))}
                  </span>
                  <Button
                    size="xs"
                    variant="ghost"
                    onClick={() => setEditando({ id: tag.id, name: tag.name, states: tag.states })}
                  >
                    {t('notebook.rename')}
                  </Button>
                  <Button
                    size="xs"
                    variant="ghost"
                    onClick={() =>
                      void confirmar({
                        titulo: t('tags.delete'),
                        mensaje: t('tags.deleteConfirm', { name: tag.name }),
                        confirmLabel: t('common.delete'),
                        tone: 'danger',
                      }).then((ok) => {
                        if (ok) void onDelete(tag.id);
                      })
                    }
                  >
                    {t('common.delete')}
                  </Button>
                </li>
              ))}
            </ul>
          )}
          <Button variant="secondary" onClick={() => setEditando(NUEVA())}>
            {t('tags.new')}
          </Button>
        </div>
      )}
    </Modal>
  );
}
