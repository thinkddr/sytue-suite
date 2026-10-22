/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * NotebookTree — el árbol de cuadernos de la barra lateral.
 *
 * Es un `tree` de verdad (roles ARIA `tree`/`treeitem`, `aria-level`,
 * `aria-expanded`), no una lista con sangría: un lector de pantalla tiene que
 * poder decir «nivel 2 de 3, contraído», que es lo único que orienta cuando no
 * se ve la sangría.
 *
 * Los cuadernos que la organización comparte y los publicados llevan su distintivo
 * a la vista, junto al nombre: saber quién puede leer lo que se está escribiendo no
 * es un detalle que deba costar un clic.
 */
import type { JSX } from 'preact';
import { useState } from 'preact/hooks';
import { Icon, SIDEBAR_ACTION_CLASS, cx } from '@sytue/ui';
import type { Note, Notebook } from '@sytue/api-clients';
import type { T } from '../i18n/index.js';
import { Glyph, type GlyphName } from './glyphs.js';
import { resumenNota, terminada } from '../client/diario.js';

/**
 * El anillo del CROMO, no el global: este árbol vive en el menú de tinta, y el azul
 * de `sytue-focus-ring` mide 2,67:1 contra el cuaderno activo (`sidebar-active`),
 * por debajo del 3:1 de WCAG 1.4.11. Es el mismo que llevan `SidebarItem` y
 * `SidebarAction`.
 */
const ANILLO_CROMO = 'sytue-focus-ring-chrome focus-visible:outline-none';

/**
 * «Nuevo cuaderno dentro» y «ajustes»: la acción del carril (24 px, anillo del
 * cromo). Eran `p-1` alrededor de un icono de 13: 20×20, medido en producción.
 * `-my-[2px]` devuelve los 4 px que crece, así que la fila mide lo mismo que antes.
 */
const ACCION_DE_CUADERNO = cx(SIDEBAR_ACTION_CLASS, '-my-[2px]');

interface Props {
  t: T;
  notebooks: Notebook[];
  activo: string | null;
  /**
   * Cuántas notas hay CARGADAS del cuaderno abierto. Sin ella, el contador de
   * ese cuaderno se queda con la cifra que trajo el servidor al abrirlo.
   */
  notasDelActivo?: number;
  bloqueado?: boolean;
  notasActivas?: Note[];
  notaSeleccionada?: string | null;
  onSelect: (nb: Notebook) => void;
  onSettings: (nb: Notebook) => void;
  onNewChild: (nb: Notebook) => void;
  onUnlock?: (nb: Notebook) => void;
  onSelectNota?: (id: string) => void;
  onMoveNoteToNotebook?: (noteId: string, targetNotebookId: string) => void;
  onReorderNotebook?: (draggedId: string, targetId: string) => void;
}

/** Distintivo de visibilidad. `personal` no lleva: es el caso normal. */
const MARCA: Partial<Record<Notebook['visibility'], GlyphName>> = {
  private: 'notebook',
  organization: 'link',
  public: 'link',
};

export function NotebookTree({
  t,
  notebooks,
  activo,
  notasDelActivo,
  bloqueado,
  notasActivas,
  notaSeleccionada,
  onSelect,
  onSettings,
  onNewChild,
  onUnlock,
  onSelectNota,
  onMoveNoteToNotebook,
  onReorderNotebook,
}: Props): JSX.Element {
  // Contraídos y no expandidos: un árbol recién abierto se ve entero, que es lo
  // que espera quien acaba de crear un cuaderno dentro de otro.
  const [cerrados, setCerrados] = useState<Set<string>>(new Set());

  const hijosDe = (padre: string | null) =>
    notebooks.filter((n) => (n.parent_id ?? null) === padre);

  /**
   * Cuántas notas tiene un cuaderno.
   *
   * EN EL ABIERTO SE CUENTAN LAS QUE HAY, no las que decía el servidor al cargar.
   * `note_count` es un número aparte que había que acordarse de subir al crear,
   * bajar al borrar, y otra vez al deshacer y al rehacer cada una de las dos —y
   * solo se tocaba al MOVER notas entre cuadernos—. Medido el 2026-09-11 con
   * siete notas: el árbol decía 1 y la lista de al lado, 7.
   *
   * Derivarlo de lo que ya está cargado es lo que hace que no pueda volver a
   * desincronizarse: no hay dos verdades que mantener iguales, hay una. Los
   * cuadernos que NO están abiertos siguen con la cifra del servidor, que es la
   * buena mientras no se tocan.
   */
  const cuantas = (nb: Notebook): number =>
    nb.id === activo && notasDelActivo !== undefined ? notasDelActivo : nb.note_count;

  const alternar = (id: string) =>
    setCerrados((prev) => {
      const s = new Set(prev);
      if (!s.delete(id)) s.add(id);
      return s;
    });

  const rama = (nb: Notebook, nivel: number): JSX.Element => {
    const hijos = hijosDe(nb.id);
    const cerrado = cerrados.has(nb.id);
    return (
      // `min-w-0`: este `li` es hijo de un `ul` con `display:grid`, y un elemento de
      // rejilla trae `min-width: auto` —se niega a encoger por debajo de su
      // contenido—. Con un título de cuaderno largo eso saca la fila de la barra
      // lateral en vez de dejar que el `truncate` del span haga su trabajo: el
      // barrido de escritorio lo midió en +43px en las cuatro ventanas. Mismo
      // motivo que en `RibbonSelect` y en la fila de pestañas de Fotos.
      <li key={nb.id} role="none" class="min-w-0">
        <div
          role="treeitem"
          aria-level={nivel}
          aria-selected={activo === nb.id}
          aria-expanded={hijos.length > 0 ? !cerrado : undefined}
          tabIndex={activo === nb.id ? 0 : -1}
          onClick={() => onSelect(nb)}
          // El asa de una nota (arrastre de puntero, NoteCanvas) la suelta aquí.
          {...(onMoveNoteToNotebook ? { 'data-drop-notebook': nb.id } : {})}
          draggable
          onDragStart={(e) => {
            e.dataTransfer?.setData('text/x-sytue-notebook-id', nb.id);
            e.dataTransfer?.setData('text/plain', nb.title);
          }}
          onDragOver={(e) => {
            e.preventDefault();
            if (e.dataTransfer) e.dataTransfer.dropEffect = 'move';
          }}
          onDrop={(e) => {
            e.preventDefault();
            const noteId = e.dataTransfer?.getData('text/x-sytue-note-id');
            const draggedNbId = e.dataTransfer?.getData('text/x-sytue-notebook-id');
            if (noteId && onMoveNoteToNotebook) {
              onMoveNoteToNotebook(noteId, nb.id);
            } else if (draggedNbId && draggedNbId !== nb.id && onReorderNotebook) {
              onReorderNotebook(draggedNbId, nb.id);
            }
          }}
          onKeyDown={(e) => {
            if (e.key === 'Enter' || e.key === ' ') {
              e.preventDefault();
              onSelect(nb);
            } else if (e.key === 'ArrowRight' && cerrado) {
              alternar(nb.id);
            } else if (e.key === 'ArrowLeft' && !cerrado && hijos.length > 0) {
              alternar(nb.id);
            }
          }}
          class={cx(
            'group flex cursor-pointer items-center gap-1.5 rounded-md py-1.5 pr-1 text-base transition-colors duration-150 pointer-coarse:min-h-[max(44px,2.75rem)]',
            ANILLO_CROMO,
            activo === nb.id
              ? 'bg-sidebar-active text-sidebar-active-fg font-medium'
              : 'text-sidebar-fg hover:bg-sidebar-hover',
          )}
          style={{ paddingLeft: `${0.5 + (nivel - 1) * 0.85}rem` }}
        >
          {hijos.length > 0 ? (
            <button
              type="button"
              // Expandir una rama NO es haber terminado con el menú: en móvil el
              // drawer se cierra al activar algo dentro, y sin esto desplegar un
              // cuaderno para ver sus hijos lo cerraba antes de poder elegirlos.
              data-mantiene-nav
              aria-label={cerrado ? t('note.expand') : t('note.collapse')}
              onClick={(e) => {
                e.stopPropagation();
                alternar(nb.id);
              }}
              class={cx(ANILLO_CROMO, '-m-1 shrink-0 rounded p-1')}
            >
              <Icon
                name="chevronRight"
                size={13}
                class={cx('transition-transform duration-150', !cerrado && 'rotate-90')}
              />
            </button>
          ) : (
            <span class="w-[13px] shrink-0" aria-hidden="true" />
          )}

          {nb.emoji ? (
            <span aria-hidden="true" class="w-5 shrink-0 text-center">
              {nb.emoji}
            </span>
          ) : (
            <span
              aria-hidden="true"
              class="h-2.5 w-2.5 shrink-0 rounded-full"
              style={{ background: nb.color }}
            />
          )}

          <span class="min-w-0 flex-1 truncate" title={nb.title}>
            {nb.title}
          </span>

          {nb.encrypted ? (
            <button
              type="button"
              aria-label={bloqueado && activo === nb.id ? t('crypto.unlock') : t('crypto.lockNow')}
              title={bloqueado && activo === nb.id ? t('crypto.unlock') : t('crypto.lockNow')}
              onClick={(e) => {
                e.stopPropagation();
                if (onUnlock) {
                  onUnlock(nb);
                } else {
                  onSelect(nb);
                }
              }}
              class={cx(ANILLO_CROMO, 'hover:text-sidebar-fg -m-1 rounded p-1')}
            >
              {bloqueado && activo === nb.id ? (
                // `on-chrome-warning` y no `warning-text`: aquel es el ámbar para PAPEL y sobre
                // la tinta del menú daba 2,07:1 (1,62 en la fila activa), por debajo del 3:1
                // que pide un icono que dice algo. Este es el ámbar del cromo: 7,24:1.
                <Icon name="lock" size={13} class="text-on-chrome-warning shrink-0" />
              ) : (
                <Glyph name="unlock" size={13} class="text-sidebar-fg-muted shrink-0" />
              )}
            </button>
          ) : MARCA[nb.visibility] ? (
            <Glyph
              name={MARCA[nb.visibility]!}
              size={13}
              class="text-sidebar-fg-muted shrink-0"
              aria-label={t(`visibility.${nb.visibility}` as never)}
            />
          ) : null}

          {cuantas(nb) > 0 ? (
            <span class="text-sidebar-fg-muted shrink-0 text-sm tabular-nums">{cuantas(nb)}</span>
          ) : null}

          {/* Acciones del cuaderno. Aparecen al pasar por encima o al enfocar; en
              táctil están siempre, que es donde no hay `hover`. */}
          <span class="flex shrink-0 gap-0.5 opacity-0 transition-opacity duration-150 group-focus-within:opacity-100 group-hover:opacity-100 pointer-coarse:opacity-100">
            <button
              type="button"
              aria-label={t('notebook.newChild')}
              onClick={(e) => {
                e.stopPropagation();
                onNewChild(nb);
              }}
              class={ACCION_DE_CUADERNO}
            >
              <Icon name="plus" size={13} />
            </button>
            <button
              type="button"
              aria-label={t('notebook.settings')}
              onClick={(e) => {
                e.stopPropagation();
                onSettings(nb);
              }}
              class={ACCION_DE_CUADERNO}
            >
              <Icon name="settings" size={13} />
            </button>
          </span>
        </div>

        {/* Cerrado con clave, ni una fila: los títulos son justo lo que la clave protege. */}
        {activo === nb.id && !bloqueado && notasActivas && notasActivas.length > 0 && !cerrado ? (
          <ul role="group" class="grid gap-0.5 py-0.5">
            {notasActivas.map((n) => (
              <li key={n.id} role="none" class="min-w-0">
                <div
                  role="button"
                  tabIndex={0}
                  onClick={() => onSelectNota?.(n.id)}
                  onKeyDown={(e) => {
                    if (e.key === 'Enter' || e.key === ' ') {
                      e.preventDefault();
                      onSelectNota?.(n.id);
                    }
                  }}
                  draggable
                  onDragStart={(e) => {
                    e.dataTransfer?.setData('text/x-sytue-note-id', n.id);
                    e.dataTransfer?.setData('text/plain', n.title || 'nota');
                  }}
                  class={cx(
                    'group/nota flex w-full cursor-pointer items-center gap-1.5 rounded-md py-1 pr-1 text-left text-xs transition-colors duration-150 pointer-coarse:min-h-[36px]',
                    ANILLO_CROMO,
                    notaSeleccionada === n.id
                      ? 'bg-sidebar-active/60 text-sidebar-active-fg font-medium'
                      : 'text-sidebar-fg-muted hover:bg-sidebar-hover hover:text-sidebar-fg',
                  )}
                  style={{ paddingLeft: `${0.75 + nivel * 0.85}rem` }}
                >
                  {n.data?.icon && typeof n.data.icon === 'string' ? (
                    <Glyph
                      name={n.data.icon as GlyphName}
                      size={12}
                      class="text-primary-text shrink-0"
                    />
                  ) : n.kind === 'todo' ? (
                    <Glyph name="todo" size={12} class="text-sidebar-fg-muted shrink-0" />
                  ) : (
                    <Glyph name="note" size={12} class="text-sidebar-fg-muted shrink-0" />
                  )}
                  <span class={cx('min-w-0 flex-1 truncate', terminada(n) && 'line-through')}>
                    {n.title || resumenNota(n, 60) || t('note.untitled')}
                  </span>
                  {n.pinned ? (
                    <Glyph name="pin" size={10} class="text-primary-text shrink-0" />
                  ) : null}
                </div>
              </li>
            ))}
          </ul>
        ) : null}

        {hijos.length > 0 && !cerrado ? (
          <ul role="group">{hijos.map((h) => rama(h, nivel + 1))}</ul>
        ) : null}
      </li>
    );
  };

  return (
    <ul role="tree" aria-label={t('sidebar.notebooks')} class="grid gap-0.5">
      {hijosDe(null).map((nb) => rama(nb, 1))}
    </ul>
  );
}
