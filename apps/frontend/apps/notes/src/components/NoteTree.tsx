/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * NoteTree — el panel izquierdo de la vista por defecto: las notas del cuaderno
 * como árbol, y a la derecha la que se elige. Es la disposición de Mail con sus
 * hilos, y la del BasKet clásico: las notas se anidan unas dentro de otras
 * (`group_id`), y las carpetas —que aquí son los cuadernos— en la barra lateral.
 *
 * Son listas anidadas con botones, no un widget `role="tree"` con navegación
 * propia: cada nota es un botón normal al que se llega con Tab, y el pliegue
 * tiene su propio botón con `aria-expanded`. Menos ARIA que cumplir y nada que
 * un lector de pantalla no sepa ya leer.
 */
import type { JSX } from 'preact';
import { useEffect, useRef, useState } from 'preact/hooks';
import { Button, Dropdown, Icon, IconButton, MenuItem, MenuSeparator, cx } from '@sytue/ui';
import type { Note, Reminder } from '@sytue/api-clients';
import { formatDate, type Locale } from '@sytue/i18n';
import type { T } from '../i18n/index.js';
import { construirArbol, type Nodo } from '../client/arbol.js';
import { terminada } from '../client/diario.js';
import { extracto } from '../client/extracto.js';
import type { ClaroPorNota } from './NoteCanvas.js';
import { Glyph, type GlyphName } from './glyphs.js';
import { RecordatorioEnFila } from './NoteCard.js';

export { extracto } from '../client/extracto.js';

interface Props {
  t: T;
  locale: Locale;
  notes: Note[];
  claro: ClaroPorNota;
  bloqueado: boolean;
  soloLectura: boolean;
  seleccionada: string | null;
  onSelect: (id: string) => void;
  onNueva: () => void;
  onNuevaDentro: (padre: Note) => void;
  onSacar: (note: Note) => void;
  onToggle: (note: Note, collapsed: boolean) => void;
  /** Cambia el título de una nota ya creada. Vacío = se deja el que tenía. */
  onRenombrar: (note: Note, titulo: string) => void;
  onDuplicate?: (note: Note) => void;
  onMoveTo?: (note: Note) => void;
  onExport?: (note: Note) => void;
  onTogglePin?: (note: Note) => void;
  onShare?: (note: Note) => void;
  onTags?: (note: Note) => void;
  onBacklinks?: (note: Note) => void;
  onDelete?: (id: string) => void;
  onIcon?: (note: Note) => void;
  /** Mis recordatorios, por nota. */
  recordatorios?: Record<string, Reminder>;
  onReminder?: (note: Note) => void;
  /** «Compartir con personas» de una nota; `null` para las que no se ofrece. */
  compartir?: (note: Note) => { rotulo: string; onClick: () => void } | null;
}

export function NoteTree(p: Props): JSX.Element {
  const arbol = construirArbol(p.notes);
  return (
    <div class="flex h-full min-h-0 flex-col">
      {/* Misma altura y mismo relleno que la barra del cuaderno de al lado
          (BarraDeCuaderno en NotesApp): las dos son la misma franja partida en
          dos columnas, y con 4px de diferencia sus pelos no casaban. */}
      <div class="border-border flex min-h-12 shrink-0 items-center justify-between gap-2 border-b px-4 py-2">
        {/* El contador CEDE y el botón NO. En un `justify-between` sin estas dos
            reglas manda quien tenga el texto más largo: el rótulo se quedaba
            entero y «Nueva nota» partía en dos líneas. Se vio con el plural sin
            resolver, pero pasa igual con un contador grande o en alemán. */}
        <span class="text-text-muted min-w-0 truncate text-sm font-medium">
          {p.t('notebook.notes', { count: p.notes.length })}
        </span>
        {/* Solo por debajo de `md`: ahí la barra del cuaderno no se ve y este es
            el único «Nueva nota». En escritorio las dos franjas van juntas y
            había dos botones iguales a un palmo de distancia. */}
        {!p.soloLectura && !p.bloqueado ? (
          <Button
            size="sm"
            variant="secondary"
            class="shrink-0 whitespace-nowrap md:hidden"
            leadingIcon={<Icon name="plus" size={14} />}
            onClick={p.onNueva}
          >
            {p.t('note.new')}
          </Button>
        ) : null}
      </div>
      {arbol.length === 0 ? (
        <p class="text-text-muted p-4 text-base">{p.t('empty.noNotes.title')}</p>
      ) : (
        <ul class="min-h-0 flex-1 overflow-auto p-2">
          {arbol.map((n) => (
            <Rama key={n.note.id} nodo={n} nivel={0} {...p} />
          ))}
        </ul>
      )}
    </div>
  );
}

/**
 * Renombrar una nota ya creada: doble clic en el rótulo, o «Renombrar» en su
 * menú. No había forma de hacerlo — el título solo se fijaba al crear— y era lo
 * primero que se busca en un árbol de notas.
 *
 * En LÍNEA y no en un diálogo: el rótulo ya está donde hay que escribir, y un
 * modal para una palabra es tres clics más. Enter guarda, Escape descarta, y
 * salir del campo guarda también —quien pulsa fuera da por hecho que se queda—.
 *
 * El doble clic no pisa la selección: el primer clic ya selecciona, y el segundo
 * abre la edición sobre la nota que ya estaba abierta.
 */
function Rama({ nodo, nivel, ...p }: Props & { nodo: Nodo; nivel: number }): JSX.Element {
  const n = nodo.note;
  const titulo = p.bloqueado
    ? p.t('crypto.locked')
    : (p.claro[n.id]?.title ?? n.title ?? '') || p.t('note.new');
  // Fecha y primera línea, como en cualquier lista de notas: un árbol de solo
  // títulos no dice cuál es la de ayer ni de qué va. En un cuaderno cifrado y
  // cerrado no hay texto que enseñar.
  const meta = [formatDate(p.locale, n.updated_at, { day: 'numeric', month: 'short' })];
  const cuerpo = p.bloqueado ? '' : extracto(p.claro[n.id]?.content ?? n.content ?? '');
  if (cuerpo) meta.push(cuerpo);
  const tieneHijos = nodo.hijos.length > 0;
  const activa = p.seleccionada === n.id;
  const [renombrando, setRenombrando] = useState(false);
  const campo = useRef<HTMLInputElement>(null);
  const puedeRenombrar = !p.soloLectura && !p.bloqueado;

  // Al abrir, el texto entero seleccionado: renombrar suele ser sustituir, no
  // añadir al final.
  useEffect(() => {
    if (renombrando) campo.current?.select();
  }, [renombrando]);

  const confirmar = (): void => {
    const nuevo = campo.current?.value.trim() ?? '';
    setRenombrando(false);
    // Vacío o sin cambios: no se llama al servicio. Un título en blanco dejaría
    // la fila con el texto de «nota nueva» y parecería que se ha perdido.
    if (nuevo && nuevo !== titulo) p.onRenombrar(n, nuevo);
  };
  return (
    <li>
      <div
        class={cx(
          'group/fila flex items-center gap-1 rounded-md pr-1',
          activa ? 'bg-primary/10' : 'hover:bg-bg-subtle',
        )}
        style={{ paddingLeft: `${nivel * 14}px` }}
      >
        {/* El pliegue es un botón APARTE del de seleccionar: plegar no debe
            cambiar qué nota se lee, ni al revés. */}
        {tieneHijos ? (
          <IconButton
            label={n.collapsed ? p.t('tree.expand') : p.t('tree.collapse')}
            size="sm"
            aria-expanded={!n.collapsed}
            onClick={() => p.onToggle(n, !n.collapsed)}
          >
            <Icon name={n.collapsed ? 'chevronRight' : 'chevronDown'} size={14} />
          </IconButton>
        ) : (
          <span class="inline-block w-7 shrink-0" aria-hidden="true" />
        )}
        {renombrando ? (
          <input
            ref={campo}
            type="text"
            defaultValue={titulo}
            aria-label={p.t('tree.rename')}
            class="border-border bg-bg text-text sytue-focus-ring min-w-0 flex-1 rounded-md border px-1.5 py-1 text-base"
            onBlur={confirmar}
            onKeyDown={(e) => {
              if (e.key === 'Enter') {
                e.preventDefault();
                confirmar();
              } else if (e.key === 'Escape') {
                e.preventDefault();
                setRenombrando(false);
              }
            }}
          />
        ) : (
          <button
            type="button"
            aria-current={activa ? 'true' : undefined}
            onClick={() => p.onSelect(n.id)}
            onDblClick={puedeRenombrar ? () => setRenombrando(true) : undefined}
            class="sytue-focus-ring rounded-control flex min-w-0 flex-1 items-center gap-1.5 py-1.5 text-left text-base pointer-coarse:min-h-[max(44px,2.75rem)]"
          >
            {!p.bloqueado &&
            (p.claro[n.id]?.data ?? n.data)?.icon &&
            typeof (p.claro[n.id]?.data ?? n.data)?.icon === 'string' ? (
              <Glyph
                name={(p.claro[n.id]?.data ?? n.data)?.icon as GlyphName}
                size={14}
                class="text-primary-text shrink-0"
              />
            ) : n.kind === 'todo' ? (
              <Glyph name="todo" size={14} class="text-text-subtle shrink-0" />
            ) : n.kind !== 'text' ? (
              <Glyph name="link" size={14} class="text-text-subtle shrink-0" />
            ) : null}
            <span class="flex min-w-0 flex-1 flex-col">
              <span
                class={cx('text-text truncate', terminada(n) && 'text-text-muted line-through')}
              >
                {titulo}
              </span>
              <span class="text-text-subtle truncate text-xs">{meta.join(' · ')}</span>
            </span>
            {n.pinned ? <Glyph name="pin" size={12} class="text-primary-text shrink-0" /> : null}
            {p.recordatorios?.[n.id] ? (
              <RecordatorioEnFila t={p.t} r={p.recordatorios[n.id]!} />
            ) : null}
            {n.states?.map((s) => (
              <span
                key={s.state_id}
                aria-label={`${s.tag_name}: ${s.name}`}
                class="inline-block h-2 w-2 shrink-0 rounded-full"
                style={{ background: s.color }}
              />
            ))}
          </button>
        )}
        {!p.soloLectura && !p.bloqueado ? (
          <Dropdown
            label={p.t('common.more')}
            align="end"
            trigger={(props) => (
              <IconButton
                {...props}
                label={p.t('common.more')}
                size="sm"
                class="opacity-0 transition-opacity duration-150 group-focus-within/fila:opacity-100 group-hover/fila:opacity-100 pointer-coarse:opacity-100"
              >
                <Icon name="ellipsis" size={14} />
              </IconButton>
            )}
          >
            {p.onIcon ? (
              <MenuItem onClick={() => p.onIcon?.(n)}>{p.t('note.setIcon')}</MenuItem>
            ) : null}
            {p.onTags ? (
              <MenuItem onClick={() => p.onTags?.(n)}>{p.t('tags.apply')}</MenuItem>
            ) : null}
            {p.onReminder ? (
              <MenuItem onClick={() => p.onReminder?.(n)}>{p.t('reminder.set')}</MenuItem>
            ) : null}
            {p.onTogglePin ? (
              <MenuItem onClick={() => p.onTogglePin?.(n)}>
                {n.pinned ? p.t('note.unpin') : p.t('note.pin')}
              </MenuItem>
            ) : null}
            <MenuItem onClick={() => setRenombrando(true)}>{p.t('tree.rename')}</MenuItem>
            {p.onDuplicate ? (
              <MenuItem onClick={() => p.onDuplicate?.(n)}>{p.t('note.duplicate')}</MenuItem>
            ) : null}
            <MenuItem onClick={() => p.onNuevaDentro(n)}>{p.t('tree.newInside')}</MenuItem>
            {n.group_id ? (
              <MenuItem onClick={() => p.onSacar(n)}>{p.t('tree.unnest')}</MenuItem>
            ) : null}
            {p.onMoveTo ? (
              <MenuItem onClick={() => p.onMoveTo?.(n)}>{p.t('note.moveTo')}</MenuItem>
            ) : null}
            {p.onExport ? (
              <MenuItem onClick={() => p.onExport?.(n)}>{p.t('note.export')}</MenuItem>
            ) : null}
            {p.compartir?.(n) ? (
              <MenuItem onClick={() => p.compartir?.(n)?.onClick()}>
                {p.compartir(n)?.rotulo}
              </MenuItem>
            ) : null}
            {p.onShare ? (
              <MenuItem onClick={() => p.onShare?.(n)}>{p.t('share.toRoom')}</MenuItem>
            ) : null}
            {p.onBacklinks ? (
              <MenuItem onClick={() => p.onBacklinks?.(n)}>{p.t('backlinks.title')}</MenuItem>
            ) : null}
            {p.onDelete ? (
              <>
                <MenuSeparator />
                <MenuItem danger onClick={() => p.onDelete?.(n.id)}>
                  {p.t('note.delete')}
                </MenuItem>
              </>
            ) : null}
          </Dropdown>
        ) : null}
      </div>
      {tieneHijos && !n.collapsed ? (
        <ul>
          {nodo.hijos.map((h) => (
            <Rama key={h.note.id} nodo={h} nivel={nivel + 1} {...p} />
          ))}
        </ul>
      ) : null}
    </li>
  );
}
