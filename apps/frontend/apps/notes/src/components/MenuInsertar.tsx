/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * MenuInsertar — el «+» de la barra: TODO lo que se puede insertar, con su dibujo, su
 * nombre y una línea que dice qué hace, y una búsqueda arriba. Es el mismo catálogo que
 * «/» (`client/comandos.ts`), como en Confluence: lo que se encuentra en uno se
 * encuentra en el otro.
 *
 * Patrón combobox con lista (WAI-ARIA): el foco se queda en la búsqueda, ↑ ↓ mueven la
 * opción activa (`aria-activedescendant`), Intro la inserta y Escape lo cierra el
 * `Popover` que lo contiene, devolviendo el foco al «+».
 */
import type { JSX } from 'preact';
import { useEffect, useState } from 'preact/hooks';
import { Icon, Input, cx, type IconName } from '@sytue/ui';
import type { T } from '../i18n/index.js';
import { filtrarComandos, type Comando, type IdComando } from '../client/comandos.js';
import { Glyph, type GlyphName } from './glyphs.js';

/** El dibujo de cada elemento: el mismo en «/», en «+» y en su botón de la barra. */
const DIBUJO: Record<IdComando, { glyph?: GlyphName; icon?: IconName }> = {
  p: { icon: 'text' },
  h1: { glyph: 'heading' },
  h2: { glyph: 'heading' },
  h3: { glyph: 'heading' },
  vinetas: { glyph: 'bulletList' },
  numeros: { glyph: 'numberedList' },
  tarea: { glyph: 'todo' },
  decision: { glyph: 'decision' },
  cita: { glyph: 'quote' },
  codigo: { icon: 'code' },
  panelInfo: { glyph: 'panel' },
  panelNota: { glyph: 'panel' },
  panelExito: { glyph: 'panel' },
  panelAviso: { glyph: 'panel' },
  panelError: { glyph: 'panel' },
  expandir: { glyph: 'expandir' },
  tabla: { glyph: 'table' },
  columnas2: { glyph: 'layoutColumns' },
  columnas3: { glyph: 'layoutColumns' },
  imagen: { icon: 'image' },
  archivo: { glyph: 'file' },
  regla: { icon: 'minus' },
  estado: { icon: 'tag' },
  fecha: { icon: 'calendar' },
  hoy: { icon: 'calendar' },
  enlace: { glyph: 'link' },
  mencion: { glyph: 'arroba' },
  simbolo: { glyph: 'omega' },
  emoji: { glyph: 'carita' },
};

/** El tinte de cada panel en el menú, para distinguirlos de un vistazo. */
const TINTE: Partial<Record<IdComando, string>> = {
  panelInfo: 'text-info-text',
  panelNota: 'text-violet-text',
  panelExito: 'text-success-text',
  panelAviso: 'text-warning-text',
  panelError: 'text-danger-text',
};

export function DibujoDeComando({ id, size = 16 }: { id: IdComando; size?: number }): JSX.Element {
  const d = DIBUJO[id];
  return (
    <span class={cx('inline-grid place-items-center', TINTE[id])}>
      {d.icon ? <Icon name={d.icon} size={size} /> : <Glyph name={d.glyph ?? 'note'} size={size} />}
    </span>
  );
}

export function MenuInsertar({
  t,
  disponible,
  onElegir,
}: {
  t: T;
  /** Lo que aquí se puede insertar (sin cuaderno no hay notas que enlazar). */
  disponible: (c: Comando) => boolean;
  onElegir: (id: IdComando) => void;
}): JSX.Element {
  const [q, setQ] = useState('');
  const [activa, setActiva] = useState(0);
  const lista = filtrarComandos(
    q,
    (c) => t(c.clave),
    (c) => t(c.descripcion),
    disponible,
  );
  const i = Math.min(activa, Math.max(lista.length - 1, 0));
  useEffect(() => {
    document.getElementById(`notes-insertar-${i}`)?.scrollIntoView({ block: 'nearest' });
  }, [i]);

  return (
    <div class="flex w-80 max-w-full flex-col gap-2">
      <Input
        type="search"
        size="sm"
        value={q}
        autocomplete="off"
        role="combobox"
        aria-expanded="true"
        aria-controls="notes-insertar-lista"
        aria-autocomplete="list"
        {...(lista.length ? { 'aria-activedescendant': `notes-insertar-${i}` } : {})}
        aria-label={t('editor.insert')}
        placeholder={t('editor.insertSearch')}
        onInput={(e) => {
          setQ(e.currentTarget.value);
          setActiva(0);
        }}
        onKeyDown={(e) => {
          if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
            e.preventDefault();
            const n = lista.length;
            if (n) setActiva((i + (e.key === 'ArrowDown' ? 1 : -1) + n) % n);
          } else if (e.key === 'Enter') {
            e.preventDefault();
            const c = lista[i];
            if (c) onElegir(c.id);
          }
        }}
      />
      <div
        id="notes-insertar-lista"
        role="listbox"
        aria-label={t('editor.insert')}
        class="-mx-1 max-h-80 overflow-y-auto"
      >
        {lista.map((c, n) => (
          <div
            key={c.id}
            id={`notes-insertar-${n}`}
            role="option"
            aria-selected={n === i}
            class={cx(
              'flex cursor-pointer items-start gap-2.5 rounded-md px-2 py-1.5',
              n === i ? 'bg-selected' : 'hover:bg-bg-subtle',
            )}
            // El teclado lo lleva la búsqueda (`aria-activedescendant`); con el ratón se
            // elige al apoyar, sin quitarle el foco.
            tabIndex={-1}
            onMouseDown={(e) => {
              e.preventDefault();
              onElegir(c.id);
            }}
            onMouseEnter={() => setActiva(n)}
          >
            <span class="border-border bg-surface text-text-muted grid h-8 w-8 shrink-0 place-items-center rounded-md border">
              <DibujoDeComando id={c.id} />
            </span>
            <span class="min-w-0 flex-1">
              <span class="text-text block truncate text-sm font-medium">{t(c.clave)}</span>
              <span class="text-text-muted block truncate text-xs">{t(c.descripcion)}</span>
            </span>
            {c.pista ? <span class="sytue-meta shrink-0 pt-0.5">{c.pista}</span> : null}
          </div>
        ))}
        {lista.length === 0 ? (
          <p class="text-text-muted px-2 py-1.5 text-sm" role="status">
            {t('editor.slashNoResults')}
          </p>
        ) : null}
      </div>
    </div>
  );
}
