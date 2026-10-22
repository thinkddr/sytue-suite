/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * NoteCanvas — donde viven las notas de un cuaderno, en la disposición que ese
 * cuaderno tenga: LIBRE (se colocan donde se sueltan, como en un corcho),
 * COLUMNAS o LISTA.
 *
 * Las tres comparten componente porque comparten todo lo que cuesta: cargar,
 * filtrar, descifrar, seleccionar y reordenar. Lo único que cambia es dónde se
 * pinta cada tarjeta, y eso son quince líneas.
 *
 * EL ARRASTRE VA CON PUNTEROS, no con la API de arrastrar y soltar de HTML.
 * `dragstart`/`drop` no existen en táctil sin un `draggable` que en móvil pelea
 * con el desplazamiento de la página; los eventos de puntero son los mismos para
 * ratón, dedo y lápiz. Y hay alternativa de TECLADO —Alt+flechas— porque una
 * disposición que solo se puede cambiar arrastrando no la puede cambiar todo el
 * mundo (WCAG 2.1.1).
 */
import type { JSX } from 'preact';
import { useCallback, useEffect, useRef, useState } from 'preact/hooks';
import { Button, Illustration, EmptyState, Spinner, cx, usePointerDrag } from '@sytue/ui';
import type { Note, Notebook, Reminder } from '@sytue/api-clients';
import type { T } from '../i18n/index.js';
import { conSangria } from '../client/arbol.js';
import { interpretarCaptura, type Captura } from '../client/captura.js';
import type { DestinoInterno } from '../client/enlaces.js';
import type { FuentesDeEnlace } from './LinkTool.js';
import { NoteCard } from './NoteCard.js';
import type { ClaroDeNota } from '../client/crypto.js';

/** Cuántas columnas tiene la disposición en columnas. */
const COLUMNAS = 3;

export interface ClaroPorNota {
  [noteId: string]: ClaroDeNota;
}

interface Props {
  t: T;
  notebook: Notebook;
  notes: Note[];
  cargando: boolean;
  /** Contenidos ya descifrados, si el cuaderno está desbloqueado. */
  claro: ClaroPorNota;
  bloqueado: boolean;
  soloLectura?: boolean;
  /** La nota llena el panel: es el caso de la vista de árbol, donde se muestra
   *  UNA sola y el espacio de al lado no es de nadie más. */
  llenar?: boolean;
  seleccionada: string | null;
  onSelect: (id: string | null) => void;
  onChange: (
    id: string,
    patch: {
      title?: string;
      content?: string;
      checked?: boolean;
      cancelled?: boolean;
      data?: Record<string, unknown>;
    },
    opts?: { fusion?: string },
  ) => void;
  onMove: (
    id: string,
    pos: { x: number; y: number; col: number; position: number; group_id?: string | null },
  ) => void;
  onDelete: (id: string) => void;
  onDuplicate: (note: Note) => void;
  onMoveTo?: ((note: Note) => void) | undefined;
  onPostpone?: ((note: Note) => void) | undefined;
  onExport?: ((note: Note) => void) | undefined;
  onTogglePin: (note: Note) => void;
  onShare: (note: Note) => void;
  onTags: (note: Note) => void;
  onBacklinks: (note: Note) => void;
  onNuevaDentro?: ((padre: Note) => void) | undefined;
  onSacar?: ((note: Note) => void) | undefined;
  onIcon?: ((note: Note) => void) | undefined;
  /** Mis recordatorios, por nota. */
  recordatorios?: Record<string, Reminder>;
  onReminder?: ((note: Note) => void) | undefined;
  /** «Compartir con personas» de una nota; `null` para las que no se ofrece. */
  compartir?: ((note: Note) => { rotulo: string; onClick: () => void } | null) | undefined;
  enlaces?: FuentesDeEnlace;
  onNavigate?: (d: DestinoInterno) => void;
  /** Acción del estado vacío: crear la primera nota. */
  onCreateFirst?: () => void;
  /** Lo que se suelta o se pega sobre el cuaderno nace como nota. */
  onCaptura?: (captura: Captura, posicion?: { x: number; y: number }) => void;
  /** «Añadir al calendario» de una nota (fuera de cuadernos cifrados). */
  onCrearEvento?: ((note: Note) => void) | undefined;
  /** Soltar una nota sobre un día de la tira de la semana (`data-drop-day`) la lleva allí. */
  onSoltarEnDia?: ((noteId: string, dia: string) => void) | undefined;
  /** Y sobre un cuaderno del árbol (`data-drop-notebook`), a ese cuaderno. */
  onSoltarEnCuaderno?: ((noteId: string, notebookId: string) => void) | undefined;
}

export function NoteCanvas({
  t,
  notebook,
  notes,
  cargando,
  claro,
  bloqueado,
  soloLectura,
  llenar = false,
  seleccionada,
  onSelect,
  onChange,
  onMove,
  onDelete,
  onDuplicate,
  onMoveTo,
  onPostpone,
  onExport,
  onTogglePin,
  onShare,
  onTags,
  onBacklinks,
  onNuevaDentro,
  onSacar,
  onIcon,
  recordatorios,
  onReminder,
  compartir,
  onCaptura,
  onCreateFirst,
  enlaces,
  onNavigate,
  onCrearEvento,
  onSoltarEnDia,
  onSoltarEnCuaderno,
}: Props): JSX.Element {
  const lienzo = useRef<HTMLDivElement>(null);
  // Nota que se está arrastrando y su posición provisional. Vive en estado para
  // que la tarjeta siga al dedo; al soltar se confirma con una sola escritura.
  const [arrastrando, setArrastrando] = useState<{ id: string; x: number; y: number } | null>(null);
  const [alturaLibre, setAlturaLibre] = useState(0);

  const libre = notebook.layout === 'free';
  // El orden en que se PINTAN: en lista, cada sublista justo debajo de su nota. Es
  // también el orden sobre el que se reordena al soltar, o el hueco elegido con el
  // puntero no sería el que se ve.
  const enOrden =
    notebook.layout === 'list' && !llenar
      ? conSangria(notes)
      : notes.map((note) => ({ note, nivel: 0 }));
  useEffect(() => {
    const root = lienzo.current;
    if (!libre || !root) return;
    // Los hijos absolutos no estiran el lienzo. Medir las tarjetas mantiene
    // alcanzables las notas largas y las que se han movido por debajo del panel.
    const measure = () =>
      setAlturaLibre(
        Math.max(
          0,
          ...Array.from(root.children).map(
            (el) => (el as HTMLElement).offsetTop + (el as HTMLElement).offsetHeight + 24,
          ),
        ),
      );
    const observer = new ResizeObserver(measure);
    Array.from(root.children).forEach((el) => observer.observe(el));
    measure();
    return () => observer.disconnect();
  }, [libre, notes, cargando]);
  // El día o el cuaderno que hay bajo el puntero mientras se arrastra: se le pone
  // `data-drop` para que se vea dónde va a caer la nota, y al soltar la recibe. El
  // marcado se lee del propio DOM: guardarlo en un ref era escribirlo desde los
  // manejadores que se le pasan a `usePointerDrag`, que el compilador de React no admite.
  const marcado = (): Element | null => document.querySelector('[data-drop="true"]');
  const marcarDestino = (x: number, y: number): HTMLElement | null => {
    // Sin ningún destino que reciba la nota (un cuaderno de solo lectura, el lienzo de
    // una prueba) no hay nada que buscar bajo el puntero.
    if (!onSoltarEnDia && !onSoltarEnCuaderno) return null;
    const el =
      document
        .elementsFromPoint(x, y)
        .map((e) => e.closest<HTMLElement>('[data-drop-day], [data-drop-notebook]'))
        .find(
          (e) =>
            e &&
            (e.dataset.dropDay
              ? Boolean(onSoltarEnDia)
              : Boolean(onSoltarEnCuaderno) && e.dataset.dropNotebook !== notebook.id),
        ) ?? null;
    if (el !== marcado()) {
      marcado()?.removeAttribute('data-drop');
      el?.setAttribute('data-drop', 'true');
    }
    return el;
  };
  const arrastre = usePointerDrag<{ note: Note; dx: number; dy: number; x0: number; y0: number }>({
    onMove: ({ data, clientX, clientY, dxRaw, dyRaw }) => {
      if (Math.hypot(dxRaw, dyRaw) < 4) return;
      marcarDestino(clientX, clientY);
      const caja = lienzo.current?.getBoundingClientRect();
      if (!caja) return;
      setArrastrando({
        id: data.note.id,
        x: Math.max(0, clientX - caja.left - data.dx),
        y: Math.max(0, clientY - caja.top - data.dy),
      });
    },
    onCancel: () => {
      setArrastrando(null);
      marcado()?.removeAttribute('data-drop');
    },
    onEnd: ({ note, dx, dy, x0, y0 }, e) => {
      setArrastrando(null);
      if (Math.hypot(e.clientX - x0, e.clientY - y0) < 4) return;
      const sobre = marcarDestino(e.clientX, e.clientY);
      marcado()?.removeAttribute('data-drop');
      if (sobre?.dataset.dropDay) {
        onSoltarEnDia?.(note.id, sobre.dataset.dropDay);
        return;
      }
      if (sobre?.dataset.dropNotebook) {
        onSoltarEnCuaderno?.(note.id, sobre.dataset.dropNotebook);
        return;
      }
      if (libre) {
        const caja = lienzo.current?.getBoundingClientRect();
        if (caja)
          onMove(note.id, {
            ...pd(note),
            x: Math.round(Math.max(0, e.clientX - caja.left - dx)),
            y: Math.round(Math.max(0, e.clientY - caja.top - dy)),
          });
        return;
      }
      const columna = document
        .elementsFromPoint(e.clientX, e.clientY)
        .map((el) => el.closest<HTMLElement>('[data-note-column]'))
        .find((el) => el && lienzo.current?.contains(el));
      const col = notebook.layout === 'list' ? 0 : Number(columna?.dataset.noteColumn);
      if (!Number.isInteger(col)) return;
      const grupo = enOrden
        .map((x) => x.note)
        .filter((n) => n.id !== note.id && (notebook.layout === 'list' || n.col === col));
      const indice = grupo.findIndex((n) => {
        const el = Array.from(
          lienzo.current?.querySelectorAll<HTMLElement>('article[data-note-id]') ?? [],
        ).find((el) => el.dataset.noteId === n.id);
        return el && e.clientY < el.getBoundingClientRect().top + el.offsetHeight / 2;
      });
      grupo.splice(indice < 0 ? grupo.length : indice, 0, note);
      // Posiciones enteras únicas, sin empates al recargar.
      grupo.forEach((n, position) => {
        if (n.position !== position || n.col !== col) onMove(n.id, { ...pd(n), col, position });
      });
    },
  });
  const empezarArrastre = (note: Note) => (e: PointerEvent) => {
    if (soloLectura || e.button !== 0) return;
    const caja = lienzo.current?.getBoundingClientRect();
    if (!caja) return;
    arrastre.iniciarArrastre(e, {
      note,
      dx: e.clientX - caja.left - note.x,
      dy: e.clientY - caja.top - note.y,
      x0: e.clientX,
      y0: e.clientY,
    });
  };

  // Alt+flechas mueve la nota seleccionada. Es la alternativa de teclado al
  // arrastre: en libre desplaza el lienzo, en columnas cambia de columna o de
  // orden. Sin Alt las flechas siguen siendo del navegador (desplazar la página).
  useEffect(() => {
    if (soloLectura) return;
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape' && seleccionada) {
        onSelect(null);
        return;
      }
      if (!e.altKey || !seleccionada) return;
      const note = notes.find((n) => n.id === seleccionada);
      if (!note) return;
      const PASO = 24;
      let pos: {
        x: number;
        y: number;
        col: number;
        position: number;
        group_id?: string | null;
      } | null = null;
      if (libre) {
        if (e.key === 'ArrowLeft') pos = { ...pd(note), x: Math.max(0, note.x - PASO) };
        else if (e.key === 'ArrowRight') pos = { ...pd(note), x: note.x + PASO };
        else if (e.key === 'ArrowUp') pos = { ...pd(note), y: Math.max(0, note.y - PASO) };
        else if (e.key === 'ArrowDown') pos = { ...pd(note), y: note.y + PASO };
      } else {
        if (e.key === 'ArrowLeft') pos = { ...pd(note), col: Math.max(0, note.col - 1) };
        else if (e.key === 'ArrowRight')
          pos = { ...pd(note), col: Math.min(COLUMNAS - 1, note.col + 1) };
        else if (e.key === 'ArrowUp')
          pos = { ...pd(note), position: Math.max(0, note.position - 1) };
        else if (e.key === 'ArrowDown') pos = { ...pd(note), position: note.position + 1 };
      }
      if (pos) {
        e.preventDefault();
        onMove(note.id, pos);
      }
    };
    document.addEventListener('keydown', onKey);
    return () => document.removeEventListener('keydown', onKey);
  }, [seleccionada, notes, libre, soloLectura, onMove, onSelect]);

  // --- Captura: soltar o pegar sobre el cuaderno -------------------------------
  //
  // Es el gesto de BasKet: el cuaderno es donde se tiran las cosas. Solo texto
  // plano (y `text/uri-list`), nunca `text/html` — la misma política que el
  // editor de la nota. La interpretación vive en client/captura.ts, sin DOM.
  const capturaActiva = Boolean(onCaptura) && !soloLectura;

  const arrastrarEncima = useCallback((e: DragEvent) => {
    const tipos = e.dataTransfer?.types ?? [];
    if (!tipos.includes('text/plain') && !tipos.includes('text/uri-list')) return;
    // Sin `preventDefault` el navegador no permite soltar: es lo que enciende
    // el cursor de «copiar» y lo que hace que llegue el `drop`.
    e.preventDefault();
    if (e.dataTransfer) e.dataTransfer.dropEffect = 'copy';
  }, []);

  const soltar = useCallback(
    (e: DragEvent) => {
      const dt = e.dataTransfer;
      if (!dt) return;
      const c = interpretarCaptura(dt.getData('text/plain'), dt.getData('text/uri-list'));
      if (!c) return;
      e.preventDefault();
      // En el lienzo libre la nota nace DONDE se soltó; en columnas y lista no
      // hay «dónde» y se deja la posición por defecto.
      const caja = libre ? lienzo.current?.getBoundingClientRect() : undefined;
      onCaptura?.(
        c,
        caja
          ? {
              x: Math.max(0, Math.round(e.clientX - caja.left)),
              y: Math.max(0, Math.round(e.clientY - caja.top)),
            }
          : undefined,
      );
    },
    [libre, onCaptura],
  );

  useEffect(() => {
    if (!capturaActiva) return;
    const alPegar = (e: ClipboardEvent) => {
      // Pegar DENTRO de algo que se edita —el cuerpo de una nota, el buscador, un
      // diálogo— sigue siendo pegar ahí. Esto solo actúa sobre el cuaderno.
      const t = e.target;
      if (t instanceof HTMLElement) {
        if (
          t.isContentEditable ||
          t.closest('input, textarea, [contenteditable], [role="dialog"], article, .group')
        )
          return;
      }
      const active = document.activeElement;
      if (active instanceof HTMLElement) {
        if (
          active.isContentEditable ||
          active.closest('input, textarea, [contenteditable], [role="dialog"], article, .group')
        )
          return;
      }
      const c = interpretarCaptura(
        e.clipboardData?.getData('text/plain') ?? '',
        e.clipboardData?.getData('text/uri-list') ?? '',
      );
      if (!c) return;
      e.preventDefault();
      onCaptura?.(c);
    };
    window.addEventListener('paste', alPegar);
    return () => window.removeEventListener('paste', alPegar);
  }, [capturaActiva, onCaptura]);

  const zonaDeCaptura = capturaActiva ? { onDragOver: arrastrarEncima, onDrop: soltar } : {};

  if (cargando) {
    return (
      <div class="grid h-full place-items-center" aria-busy="true">
        <Spinner size={24} />
      </div>
    );
  }

  const diario = Boolean(notebook.daily_date) && notebook.layout === 'list';
  // En lista las notas son FILAS —en un día y en cualquier cuaderno—; la nota de un
  // árbol, que llena su panel, es una hoja.
  const filas = notebook.layout === 'list' && !llenar;

  // Un día vacío no es un error ni un hueco que adornar: la captura está justo
  // encima, así que basta una línea en voz baja.
  if (notes.length === 0 && diario) {
    return (
      <div class="notes-col py-4" {...zonaDeCaptura}>
        <p class="text-text-subtle text-base">{t('empty.noNotes.title')}</p>
      </div>
    );
  }

  if (notes.length === 0) {
    return (
      <div class="grid h-full place-items-center p-6" {...zonaDeCaptura}>
        <div class="grid justify-items-center gap-2">
          <EmptyState
            title={t('empty.noNotes.title')}
            description={t('empty.noNotes.body')}
            illustration={<Illustration name="emptyNotes" />}
            action={
              onCreateFirst && !soloLectura ? (
                <Button size="sm" onClick={onCreateFirst}>
                  {t('note.new')}
                </Button>
              ) : undefined
            }
          />
          {capturaActiva ? <p class="sytue-meta text-center">{t('capture.hint')}</p> : null}
        </div>
      </div>
    );
  }

  const tarjeta = (note: Note, nivel = 0) => (
    <NoteCard
      diario={filas}
      nivel={nivel}
      key={note.id}
      t={t}
      note={note}
      llenar={llenar}
      claro={claro[note.id] ?? null}
      bloqueado={bloqueado}
      {...(soloLectura ? { soloLectura } : {})}
      seleccionada={seleccionada === note.id}
      onSelect={() => onSelect(note.id)}
      onChange={(patch, opts) => onChange(note.id, patch, opts)}
      onDelete={() => onDelete(note.id)}
      onDuplicate={() => onDuplicate(note)}
      {...(onMoveTo ? { onMoveTo: () => onMoveTo(note) } : {})}
      {...(onPostpone ? { onPostpone: () => onPostpone(note) } : {})}
      {...(onExport ? { onExport: () => onExport(note) } : {})}
      onTogglePin={() => onTogglePin(note)}
      onShare={() => onShare(note)}
      onTags={() => onTags(note)}
      onBacklinks={() => onBacklinks(note)}
      {...(onNuevaDentro ? { onNuevaDentro: () => onNuevaDentro(note) } : {})}
      {...(onSacar ? { onSacar: () => onSacar(note) } : {})}
      {...(onIcon ? { onIcon: () => onIcon(note) } : {})}
      recordatorio={recordatorios?.[note.id]}
      {...(onReminder ? { onReminder: () => onReminder(note) } : {})}
      compartir={compartir?.(note) ?? undefined}
      {...(enlaces ? { enlaces } : {})}
      {...(onNavigate ? { onNavigate } : {})}
      {...(!soloLectura && !llenar
        ? {
            arrastre: {
              onPointerDown: empezarArrastre(note),
              onPointerMove: arrastre.onPointerMove,
              onPointerUp: arrastre.onPointerUp,
              onPointerCancel: arrastre.onPointerCancel,
              onLostPointerCapture: arrastre.onPointerCancel,
            },
          }
        : {})}
      {...(onCrearEvento ? { onCrearEvento: () => onCrearEvento(note) } : {})}
    />
  );

  if (libre) {
    return (
      <div
        ref={lienzo}
        // `min-h` y no `h`: el lienzo libre crece con lo que se suelte abajo del
        // todo, y una altura fija dejaría notas fuera de alcance.
        //
        // Sin «clic en el vacío para deseleccionar»: sería un manejador en un
        // contenedor sin rol ni equivalente de teclado. Se deselecciona con
        // Escape, que funciona para todo el mundo y ya está cableado abajo.
        class="relative min-h-full p-4"
        style={{ minHeight: `${Math.max(alturaLibre, (arrastrando?.y ?? 0) + 480)}px` }}
        {...zonaDeCaptura}
      >
        {notes.map((note) => {
          const arr = arrastrando?.id === note.id ? arrastrando : null;
          return (
            <div
              key={note.id}
              class={cx('absolute', arr && 'z-10 cursor-grabbing select-none')}
              style={{
                left: `${arr?.x ?? note.x}px`,
                top: `${arr?.y ?? note.y}px`,
                width: `${note.width}px`,
              }}
            >
              {tarjeta(note)}
            </div>
          );
        })}
      </div>
    );
  }

  if (notebook.layout === 'list') {
    if (llenar) {
      // EN ÁRBOL LA NOTA LLENA EL PANEL: es una hoja, sin tarjeta alrededor.
      //
      // `minmax(min-content, 1fr)` y `min-h-full`: la nota corta llena el panel (1fr) y
      // la larga estira la fila hasta su contenido. Con `grid-rows-1` —`minmax(0, 1fr)`—
      // una nota más larga que el panel se salía por debajo de su propio fondo (medido
      // con 60 párrafos en un panel de 700 px: 2297 px de texto fuera).
      return (
        <div class="grid min-h-full grid-rows-[minmax(min-content,1fr)]" {...zonaDeCaptura}>
          {notes.map((note) => tarjeta(note))}
        </div>
      );
    }
    // La columna de lectura, y las filas sangradas 8 px hacia fuera para que el glifo
    // de cada una caiga en el borde de la columna, bajo el título.
    return (
      <div ref={lienzo} class="notes-col" {...zonaDeCaptura}>
        <div class="notes-stream -mx-2">
          {enOrden.map(({ note, nivel }) => tarjeta(note, nivel))}
        </div>
      </div>
    );
  }

  // Columnas: se reparten por `col`, y lo que venga con una columna fuera de
  // rango cae en la última en vez de desaparecer.
  return (
    <div
      ref={lienzo}
      class="grid grid-cols-1 items-start gap-3 p-4 md:grid-cols-2 lg:grid-cols-3"
      {...zonaDeCaptura}
    >
      {Array.from({ length: COLUMNAS }, (_, i) => (
        <div key={i} data-note-column={i} class="grid min-h-32 content-start gap-2.5">
          {notes
            .filter((n) => Math.min(Math.max(n.col, 0), COLUMNAS - 1) === i)
            .map((note) => tarjeta(note))}
        </div>
      ))}
    </div>
  );
}

/** Posición actual de una nota, como punto de partida de un movimiento. */
function pd(n: Note): {
  x: number;
  y: number;
  col: number;
  position: number;
  group_id: string | null;
} {
  return { x: n.x, y: n.y, col: n.col, position: n.position, group_id: n.group_id ?? null };
}
