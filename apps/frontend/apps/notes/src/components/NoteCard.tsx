/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * NoteCard — UNA nota del lienzo, sea del tipo que sea.
 *
 * Un solo componente para los doce tipos y no doce componentes: lo que cambia
 * entre una nota de texto y un evento vinculado es el CUERPO; el marco, el menú,
 * las etiquetas, el arrastre y el foco son idénticos, y separarlos habría dejado
 * doce copias de la parte que de verdad importa.
 *
 * Editar es pulsar la nota, como una ficha de papel: no hay «modo edición» ni
 * botón de lápiz. Se guarda AL SALIR, se salga como se salga: Escape, Ctrl/Cmd+Enter
 * o llevando el foco a otra parte. Antes Escape descartaba en silencio y un párrafo
 * entero se perdía por pulsarlo por costumbre (medido el 2026-09-09); una ficha de
 * papel no se borra sola, y aquí tampoco.
 */
import type { JSX } from 'preact';
import { useEffect, useRef, useState } from 'preact/hooks';
import {
  Badge,
  Dropdown,
  IconButton,
  Icon,
  MenuItem,
  MenuSeparator,
  cx,
  withBase,
  Checkbox,
  useUltimo,
  formatBytes,
} from '@sytue/ui';
import { documentLocale } from '@sytue/i18n';
import type { Note, NoteLink, Reminder } from '@sytue/api-clients';
import type { T } from '../i18n/index.js';
import { Glyph, type GlyphName } from './glyphs.js';
import { conAnclas, destinoInterno, type DestinoInterno } from '../client/enlaces.js';
import { conHashtags, conMenciones } from '../client/hashtags.js';
import type { FuentesDeEnlace } from './LinkTool.js';
import { RichText } from './RichText.js';
import { terminada, vecesTrasladada } from '../client/diario.js';
import { rotuloRecordatorio, yaPaso } from '../client/recordatorios.js';
import { textoDeNota } from '../client/extracto.js';
import { useVistaPreviaEnlaces, type VistaPrevia } from './VistaPreviaEnlace.js';
import { alternarPlegado, liPlegable } from '../client/en-nota.js';

/**
 * Lo que se espera a que pare el teclado antes de guardar solo.
 *
 * 400 ms y no un segundo largo: escribiendo a 60 ppm hay unos 200 ms entre
 * tecla y tecla, asi que esto guarda en cuanto se levanta la vista del teclado
 * —al terminar una palabra, al pensar la siguiente frase—, que es cuando la
 * gente da por hecho que ya esta guardado.
 */
const PAUSA_DE_AUTOGUARDADO_MS = 400;

/**
 * Y un techo, porque la pausa sola no basta: quien escribe seguido puede no
 * dejar NUNCA un hueco de 400 ms, y con solo el debounce se pasaria media nota
 * sin que saliera un byte. Pasados dos segundos desde el primer cambio sin
 * guardar, se guarda aunque las teclas sigan cayendo.
 */
const ESPERA_MAXIMA_MS = 2_000;

/**
 * Cuánto falta para el próximo autoguardado: la pausa normal, o lo que quede
 * del techo si ya se acerca. Se recalcula en cada tecla, así que la cuenta
 * corre desde el primer cambio sin guardar y NO se reinicia al seguir
 * escribiendo, que es lo que hace que el techo sirva de algo.
 */
export function esperaDeAutoguardado(sucioDesde: number, ahora: number): number {
  return Math.min(PAUSA_DE_AUTOGUARDADO_MS, Math.max(0, sucioDesde + ESPERA_MAXIMA_MS - ahora));
}

export interface NoteAttachment {
  id: string;
  name: string;
  size: number;
  type: string;
  dataUrl: string;
}

interface Props {
  t: T;
  note: Note;
  diario?: boolean;
  /** Profundidad de la nota dentro de su sublista (0 en la raíz); solo en filas. */
  nivel?: number;
  /** Contenido en claro de una nota cifrada, ya descifrado por el lienzo. */
  claro?: { title: string; content: string; data?: Record<string, unknown> } | null;
  /** El cuaderno está cifrado y todavía bloqueado. */
  bloqueado?: boolean;
  seleccionada: boolean;
  soloLectura?: boolean;
  /** La tarjeta llena su hueco (vista de árbol): el cuerpo crece con ella, para
   *  que se pueda pulsar en cualquier punto y no solo en las primeras líneas. */
  llenar?: boolean;
  onSelect: () => void;
  onChange: (
    patch: {
      title?: string;
      content?: string;
      checked?: boolean;
      cancelled?: boolean;
      data?: Record<string, unknown>;
    },
    opts?: { fusion?: string },
  ) => void;
  onDelete: () => void;
  onDuplicate: () => void;
  /** «Mover a otro cuaderno…»; sin él no se ofrece (cuaderno cifrado, o único). */
  onMoveTo?: (() => void) | undefined;
  /** «Posponer…» de una tarea: la lleva a otro día con lo que cuelga de ella. */
  onPostpone?: (() => void) | undefined;
  /** «Exportar…»: descarga esta nota sola. */
  onExport?: (() => void) | undefined;
  onTogglePin: () => void;
  onShare: () => void;
  onTags: () => void;
  /** Abre «qué apunta aquí». Es la vuelta de la referencia entre notas. */
  onBacklinks: () => void;
  /** Nueva nota dentro de esta (anidada). */
  onNuevaDentro?: (() => void) | undefined;
  /** Sacar nota de su grupo / nota padre. */
  onSacar?: (() => void) | undefined;
  /** Configurar icono predefinido. */
  onIcon?: (() => void) | undefined;
  /** Mi recordatorio en esta nota, si lo tiene: el indicador de la cabecera. */
  recordatorio?: Reminder | undefined;
  /** «Recordatorio…»: ponerlo, cambiarlo o quitarlo. */
  onReminder?: (() => void) | undefined;
  /**
   * «Compartir con personas»: solo en las notas de quien la escribió. Lleva su rótulo
   * porque es del catálogo común (@sytue/i18n), no del de Notes.
   */
  compartir?: { rotulo: string; onClick: () => void } | undefined;
  /**
   * Sin el menú «Más»: una nota que alguien me ha compartido con edición. Se cambia su
   * título y su contenido, y nada más —fijar, mover, borrar… son de su dueña—.
   */
  sinMenu?: boolean;
  /** Empieza a mover o reordenar desde el asa, con ratón, dedo o lápiz. */
  arrastre?: {
    onPointerDown: (e: PointerEvent) => void;
    onPointerMove: (e: PointerEvent) => void;
    onPointerUp: (e: PointerEvent) => void;
    onPointerCancel: (e: PointerEvent) => void;
    onLostPointerCapture: (e: PointerEvent) => void;
  };
  /** Notas y títulos que la herramienta de enlaces del editor puede ofrecer. */
  enlaces?: FuentesDeEnlace;
  /** Un enlace a otra nota, pulsado en el cuerpo: navegar sin recargar. */
  onNavigate?: (d: DestinoInterno) => void;
  /** «Añadir al calendario»: un evento de Sytue Calendar enlazado a la nota. */
  onCrearEvento?: (() => void) | undefined;
}

/** Icono de cada tipo de nota, para el rótulo de las que no son texto. */
const GLIFO: Partial<Record<Note['kind'], GlyphName>> = {
  link: 'link',
  image: 'image',
  file: 'file',
  drive: 'file',
  doc: 'file',
  event: 'note',
  room: 'link',
  crossref: 'notebook',
  color: 'palette',
  launcher: 'link',
};

/** Un dato de `data` como texto, sin confiar en su forma. */
function texto(data: Note['data'], clave: string): string {
  const v = data?.[clave];
  return typeof v === 'string' ? v : '';
}

export function NoteCard({
  t,
  note: rawNote,
  diario = false,
  nivel = 0,
  claro,
  bloqueado,
  seleccionada,
  soloLectura,
  llenar = false,
  onSelect,
  onChange,
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
  recordatorio,
  onReminder,
  compartir,
  sinMenu = false,
  arrastre,
  enlaces,
  onNavigate,
  onCrearEvento,
}: Props): JSX.Element {
  const note = bloqueado
    ? { ...rawNote, title: '', content: '', data: {} }
    : claro
      ? { ...rawNote, data: claro.data ?? {} }
      : rawNote;
  const [editando, setEditando] = useState(false);
  const [ancha, setAncha] = useState(false);
  const fileInputRef = useRef<HTMLInputElement>(null);
  const temporizadorRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  const estirada = llenar || (editando && ancha);
  const [renombrando, setRenombrando] = useState(false);
  const [tituloBorrador, setTituloBorrador] = useState('');
  const [borrador, setBorrador] = useState('');

  const titulo = claro ? claro.title : (note.title ?? '');
  const contenido = claro ? claro.content : (note.content ?? '');
  const enlace: NoteLink | undefined = note.links?.[0];
  const editable = !soloLectura && !bloqueado && (note.kind === 'text' || note.kind === 'todo');

  const adjuntos: NoteAttachment[] = Array.isArray(note.data?.attachments)
    ? (note.data.attachments as NoteAttachment[])
    : [];

  const alSubirArchivos = (e: Event) => {
    const files = (e.target as HTMLInputElement).files;
    if (!files || files.length === 0) return;
    const nuevasPromesas = Array.from(files).map((f) => {
      return new Promise<NoteAttachment>((resolve) => {
        const reader = new FileReader();
        reader.onload = () => {
          resolve({
            id: `${Date.now()}-${Math.random().toString(36).slice(2, 8)}`,
            name: f.name,
            size: f.size,
            type: f.type,
            dataUrl: (reader.result as string) || '',
          });
        };
        reader.readAsDataURL(f);
      });
    });
    void Promise.all(nuevasPromesas).then((nuevos) => {
      const dataActual = note.data ?? {};
      onChange({
        data: {
          ...dataActual,
          attachments: [...adjuntos, ...nuevos],
        },
      });
    });
    (e.target as HTMLInputElement).value = '';
  };

  const eliminarAdjunto = (id: string) => {
    const dataActual = note.data ?? {};
    onChange({
      data: {
        ...dataActual,
        attachments: adjuntos.filter((a) => a.id !== id),
      },
    });
  };

  const alCambiarRef = useUltimo(onChange);
  const ultimoRef = useUltimo({ borrador, contenido, editando });
  const intentadoRef = useRef<string | null>(null);
  const sesionRef = useRef('');
  const sucioDesdeRef = useRef(0);

  useEffect(() => {
    if (!editando || borrador === contenido) {
      sucioDesdeRef.current = 0;
      return;
    }
    if (borrador === intentadoRef.current) return;
    if (sucioDesdeRef.current === 0) sucioDesdeRef.current = Date.now();
    const espera = esperaDeAutoguardado(sucioDesdeRef.current, Date.now());
    const temporizador = setTimeout(() => {
      sucioDesdeRef.current = 0;
      intentadoRef.current = borrador;
      alCambiarRef.current({ content: borrador }, { fusion: sesionRef.current });
    }, espera);
    temporizadorRef.current = temporizador;
    return () => {
      clearTimeout(temporizador);
      temporizadorRef.current = null;
    };
  }, [borrador, contenido, editando, alCambiarRef]);

  useEffect(
    () => () => {
      const { borrador: b, contenido: c, editando: e } = ultimoRef.current;
      if (e && b !== c) alCambiarRef.current({ content: b }, { fusion: sesionRef.current });
    },
    [alCambiarRef, ultimoRef],
  );

  const empezarARenombrar = () => {
    setTituloBorrador(titulo);
    setRenombrando(true);
  };

  const guardarTitulo = () => {
    setRenombrando(false);
    const nuevo = tituloBorrador.trim();
    if (nuevo !== titulo) onChange({ title: nuevo });
  };

  const guardar = () => {
    if (temporizadorRef.current) {
      clearTimeout(temporizadorRef.current);
      temporizadorRef.current = null;
    }
    setEditando(false);
    setAncha(false);
    if (borrador !== contenido) onChange({ content: borrador }, { fusion: sesionRef.current });
  };

  const glifo =
    note.data?.icon && typeof note.data.icon === 'string' ? (
      <Glyph
        name={note.data.icon as GlyphName}
        size={15}
        class="text-primary-text mt-0.5 shrink-0"
      />
    ) : note.kind === 'todo' ? (
      <Checkbox
        ringClass="mt-0.5"
        checked={note.checked}
        disabled={soloLectura || bloqueado}
        aria-label={titulo || t('note.newTodo')}
        onChange={(e) => onChange({ checked: (e.target as HTMLInputElement).checked })}
        class="border-border h-4 w-4 shrink-0 rounded accent-[var(--sytue-primary)]"
      />
    ) : GLIFO[note.kind] ? (
      <Glyph name={GLIFO[note.kind]!} size={15} class="text-text-subtle mt-0.5 shrink-0" />
    ) : null;
  // En el diario un texto sin título es la fila entera: su cuerpo va en la línea del
  // glifo, sin el «Sin título» que se repetiría en cada captura.
  const enLinea = diario && !titulo && note.kind === 'text' && !bloqueado && !renombrando;
  // Un separador (`---`) es solo su raya: sin viñeta y sin editor que abrir.
  const separador = enLinea && !editando && /^\s*<hr\s*\/?>\s*$/i.test(contenido);
  const cuerpo = bloqueado ? (
    <p class="text-text-subtle text-base">{t('crypto.unlockHint')}</p>
  ) : editando ? (
    <RichText
      t={t}
      initialHtml={contenido}
      ariaLabel={titulo || t('note.new')}
      placeholder={t('note.placeholder')}
      onChange={setBorrador}
      onDone={() => guardar()}
      estirado={estirada}
      {...(enlaces ? { enlaces } : {})}
    />
  ) : separador ? (
    <hr class="notes-stream-rule" />
  ) : note.kind === 'todo' && !contenido && !seleccionada ? null : (
    <Cuerpo
      t={t}
      note={note}
      contenido={contenido}
      llenar={estirada}
      {...(enlace ? { enlace } : {})}
      {...(enlaces ? { publicBase: enlaces.publicBase } : {})}
      {...(enlaces?.previa ? { previa: enlaces.previa } : {})}
      {...(onNavigate ? { onNavigate } : {})}
      {...(editable
        ? {
            onPlegar: (html: string) => onChange({ content: html }),
            onEdit: () => {
              setBorrador(contenido);
              sesionRef.current = `${note.id}:${Date.now()}`;
              setEditando(true);
            },
          }
        : {})}
    />
  );

  return (
    <article
      data-note-id={note.id}
      // `article` y no `div`: cada nota es una unidad de contenido independiente y
      // así los lectores de pantalla ofrecen saltar de una a la siguiente.
      //
      // NO es un objetivo de foco: un `article` con `tabIndex` mete una parada de
      // tabulación que no lleva a ninguna acción. La selección —la que usan los
      // atajos de etiqueta— sigue al foco de lo que hay DENTRO: el cuerpo
      // editable, la casilla de la tarea o el menú «…».
      aria-label={titulo || t('note.new')}
      onFocusCapture={onSelect}
      {...(diario && nivel > 0 ? { style: { marginLeft: `${Math.min(nivel, 4) * 1.75}rem` } } : {})}
      class={cx(
        // `min-w-0`: la tarjeta es hija de una rejilla (la hoja de un árbol, una columna
        // del tablero) y el título no parte (`truncate`), así que su ancho mínimo era el
        // del título entero y la columna crecía con él en vez de recortarlo: la nota se
        // salía 73 px de su panel en una tableta y 34 de su columna a 320.
        'group relative flex min-w-0 flex-col gap-1.5 transition-colors duration-150',
        'focus-visible:outline-none',
        // Tres formas: fila (lista y diario), hoja (la nota de un árbol, que llena su
        // panel) y tarjeta (columnas y lienzo libre). Solo la tarjeta lleva borde, y
        // ninguna sombra: lo que separa es el aire.
        diario
          ? 'notes-stream-note'
          : llenar
            ? 'notes-sheet'
            : 'bg-surface border-border-subtle hover:border-border rounded-lg border p-3',
        // La elegida: un fondo en la fila y un anillo fino en la tarjeta. Editando
        // manda el anillo del propio editor, y la hoja es lo único que hay en su panel.
        seleccionada && !editando && diario && 'notes-stream-note--selected',
        seleccionada && !editando && !diario && !llenar && 'ring-primary/50 ring-1',
        // `col-span-full` y no una posición absoluta: así la nota se ensancha
        // dentro de la rejilla, sin taparlo todo ni pelearse con el scroll.
        editando && ancha ? 'col-span-full min-h-[70dvh]' : '',
      )}
      onBlurCapture={(e: FocusEvent) => {
        // Salir de la tarjeta (otra nota, tabulación fuera) guarda. El foco que va a
        // un hijo —la barra de formato— no cuenta, y tampoco el clic en el lienzo
        // vacío (relatedTarget null; en Safari los botones tampoco toman el foco).
        const destino = e.relatedTarget as Node | null;
        if (!editando || !destino) return;
        const tarjeta = e.currentTarget as HTMLElement;
        // Y NO BASTA CON `contains`: un menú de la barra se cierra al elegir una
        // opción, así que para cuando llega este evento su elemento ya no está en
        // el documento y `contains` dice que no. Eso se leía como «me he ido de
        // la nota» y cerraba el editor a mitad de dar formato. Un destino que ya
        // no existe no es un sitio al que se haya ido nadie.
        if (!destino.isConnected) return;
        if (!tarjeta.contains(destino)) guardar();
      }}
    >
      <header class={cx('flex items-start', diario ? 'gap-2' : 'gap-1.5')}>
        {/* UNA asa por nota: reordena, mueve en el lienzo libre y, soltada sobre un día
            de la semana o un cuaderno del árbol, lleva la nota allí. En una fila vive
            en el margen, fuera de la columna, y solo se ve al pasar por encima o con
            el foco —una lista con un asa azul por línea no se leía—; con el dedo (o en
            una ventana sin margen) va en la fila, que es donde no hay «encima». */}
        {arrastre ? (
          <button
            type="button"
            aria-label={`${t('history.move')}: ${titulo || t('note.new')}`}
            title={t('history.move')}
            {...arrastre}
            class={cx(
              'text-text-subtle hover:text-primary-text sytue-focus-ring grid shrink-0 cursor-grab touch-none place-items-center rounded-md select-none active:cursor-grabbing pointer-coarse:size-11',
              diario
                ? 'size-6 opacity-60 transition-opacity duration-150 sm:pointer-fine:absolute sm:pointer-fine:top-1.5 sm:pointer-fine:-left-5 sm:pointer-fine:opacity-0 sm:pointer-fine:group-focus-within:opacity-100 sm:pointer-fine:group-hover:opacity-100'
                : '-mt-0.5 -ml-1 size-6',
            )}
          >
            <Glyph name="drag" size={14} />
          </button>
        ) : null}
        {diario ? (
          // La columna fija de Twos: cada fila empieza por su glifo en el mismo
          // sitio, y un texto suelto lleva la viñeta.
          <span class="notes-stream-glyph">
            {separador ? null : (glifo ?? <span class="notes-stream-bullet" aria-hidden="true" />)}
          </span>
        ) : (
          glifo
        )}

        {enLinea ? (
          <div class={cx('min-w-0 flex-1', separador && 'flex min-h-5')}>{cuerpo}</div>
        ) : (
          <h3
            class={cx(
              'text-text min-w-0 flex-1 text-base break-words',
              // En una fila el título es el texto de la nota, no un encabezado.
              diario ? 'font-normal' : 'font-medium',
              llenar && !diario && 'notes-sheet-title',
              terminada(note) && 'text-text-muted line-through',
            )}
          >
            {bloqueado ? (
              <span class="text-text-subtle inline-flex items-center gap-1">
                <Icon name="lock" size={14} /> {t('crypto.locked')}
              </span>
            ) : renombrando ? (
              <input
                type="text"
                value={tituloBorrador}
                // eslint-disable-next-line jsx-a11y/no-autofocus -- al renombrar, el campo toma el foco
                autoFocus
                maxLength={200}
                placeholder={t('note.titlePlaceholder')}
                aria-label={t('note.rename')}
                onInput={(e) => setTituloBorrador((e.target as HTMLInputElement).value)}
                onBlur={guardarTitulo}
                onKeyDown={(e) => {
                  if (e.key === 'Enter') {
                    e.preventDefault();
                    guardarTitulo();
                  } else if (e.key === 'Escape') {
                    e.preventDefault();
                    setRenombrando(false);
                  }
                }}
                class="border-border bg-surface text-text sytue-focus-ring w-full rounded-md border px-1.5 py-0.5 text-base font-medium"
              />
            ) : soloLectura ? (
              titulo || (enlace?.label ?? '')
            ) : (
              // Pulsar el título lo edita, que es donde todo el mundo lo intenta
              // primero. `button` y no un `div` con onClick: así llega con el
              // teclado y se anuncia como lo que es.
              <button
                type="button"
                onClick={empezarARenombrar}
                title={t('note.rename')}
                class={cx(
                  'sytue-focus-ring -mx-1 max-w-full truncate rounded-sm px-1 text-left hover:bg-[var(--sytue-bg-subtle)]',
                  // El `line-through` del `<h3>` no llega a un botón (inline-block).
                  terminada(note) && 'text-text-muted line-through',
                )}
              >
                {titulo || (enlace?.label ?? t('note.untitled'))}
              </button>
            )}
          </h3>
        )}

        {vecesTrasladada(note) > 0 && !terminada(note) ? (
          <span
            class="notes-carry"
            title={
              typeof note.data?.['carried_from'] === 'string'
                ? t('task.carriedFrom', { date: note.data['carried_from'] })
                : undefined
            }
          >
            {t('task.carried', { count: vecesTrasladada(note) })}
          </span>
        ) : null}

        {note.pinned ? (
          <Glyph
            name="pin"
            size={14}
            class="text-primary-text mt-1 shrink-0"
            aria-label={t('note.pin')}
          />
        ) : null}

        {recordatorio ? (
          <RecordatorioEnFila
            t={t}
            r={recordatorio}
            {...(onReminder ? { onClick: onReminder } : {})}
          />
        ) : null}

        {editando ? (
          <button
            type="button"
            // Igual que la barra de formato: si este botón se lleva el foco, la
            // tarjeta cree que te has ido de la nota y cierra el editor.
            onMouseDown={(e: MouseEvent) => e.preventDefault()}
            onClick={() => setAncha((v) => !v)}
            aria-pressed={ancha}
            title={ancha ? t('note.narrow') : t('note.widen')}
            aria-label={ancha ? t('note.narrow') : t('note.widen')}
            class="text-text-muted hover:text-text hover:bg-bg-subtle sytue-focus-ring rounded-control flex h-7 w-7 shrink-0 items-center justify-center"
          >
            <Glyph name={ancha ? 'outdent' : 'indent'} size={14} />
          </button>
        ) : null}

        {!soloLectura && !sinMenu ? (
          <Dropdown
            label={t('common.more')}
            align="end"
            trigger={(props) => (
              <IconButton
                {...props}
                label={t('common.more')}
                size="sm"
                class="opacity-0 transition-opacity duration-150 group-focus-within:opacity-100 group-hover:opacity-100 pointer-coarse:opacity-100"
              >
                <Icon name="ellipsis" size={16} />
              </IconButton>
            )}
          >
            {/* Lo de cada día arriba, organizar en medio y lo que sale de la nota
                (calendario, compartir, exportar) al final: diecisiete entradas seguidas
                no se leían. */}
            <MenuItem onClick={onTogglePin}>
              {note.pinned ? t('note.unpin') : t('note.pin')}
            </MenuItem>
            {onReminder ? <MenuItem onClick={onReminder}>{t('reminder.set')}</MenuItem> : null}
            {note.kind === 'todo' && onPostpone && !terminada(note) ? (
              <MenuItem onClick={onPostpone}>{t('task.postpone')}</MenuItem>
            ) : null}
            {note.kind === 'todo' ? (
              <MenuItem onClick={() => onChange({ cancelled: !note.cancelled_at })}>
                {t(note.cancelled_at ? 'task.reopen' : 'task.cancel')}
              </MenuItem>
            ) : null}
            <MenuItem onClick={onTags}>{t('tags.apply')}</MenuItem>
            <MenuSeparator />
            <MenuItem onClick={empezarARenombrar}>{t('note.rename')}</MenuItem>
            <MenuItem onClick={onDuplicate}>{t('note.duplicate')}</MenuItem>
            {onNuevaDentro ? (
              <MenuItem onClick={onNuevaDentro}>{t('tree.newInside')}</MenuItem>
            ) : null}
            {note.group_id && onSacar ? (
              <MenuItem onClick={onSacar}>{t('tree.unnest')}</MenuItem>
            ) : null}
            {onMoveTo ? <MenuItem onClick={onMoveTo}>{t('note.moveTo')}</MenuItem> : null}
            {onIcon ? <MenuItem onClick={onIcon}>{t('note.setIcon')}</MenuItem> : null}
            <MenuSeparator />
            {onCrearEvento ? (
              <MenuItem onClick={onCrearEvento}>{t('event.create')}</MenuItem>
            ) : null}
            {compartir ? <MenuItem onClick={compartir.onClick}>{compartir.rotulo}</MenuItem> : null}
            <MenuItem onClick={onShare}>{t('share.toRoom')}</MenuItem>
            {onExport ? <MenuItem onClick={onExport}>{t('note.export')}</MenuItem> : null}
            {!soloLectura ? (
              <MenuItem onClick={() => fileInputRef.current?.click()}>{t('note.attach')}</MenuItem>
            ) : null}
            <MenuItem onClick={onBacklinks}>{t('backlinks.title')}</MenuItem>
            <MenuSeparator />
            <MenuItem danger onClick={onDelete}>
              {t('note.delete')}
            </MenuItem>
          </Dropdown>
        ) : null}
      </header>

      {/* CUERPO — lo único que cambia entre tipos. */}
      {enLinea ? null : diario && cuerpo ? <div class="pl-7">{cuerpo}</div> : cuerpo}
      <input
        type="file"
        multiple
        ref={fileInputRef}
        onChange={alSubirArchivos}
        class="sr-only"
        tabIndex={-1}
        aria-hidden="true"
      />

      {adjuntos.length > 0 ? (
        <div class="border-border mt-2 grid gap-1.5 border-t pt-2">
          <span class="text-text-muted text-xs font-semibold tracking-wider uppercase">
            {t('note.attachments')} ({adjuntos.length})
          </span>
          <div class="grid gap-1">
            {adjuntos.map((adj) => (
              <div
                key={adj.id}
                class="border-border bg-bg-subtle/50 flex items-center justify-between gap-2 rounded-md border px-2 py-1 text-xs"
              >
                <div class="flex min-w-0 items-center gap-1.5">
                  <Glyph name="file" size={13} class="text-text-subtle shrink-0" />
                  <span
                    class="text-text max-w-44 truncate font-medium sm:max-w-64"
                    title={adj.name}
                  >
                    {adj.name}
                  </span>
                  <span class="text-text-muted shrink-0 text-[10px]">
                    {formatBytes(documentLocale(), adj.size)}
                  </span>
                </div>
                <div class="flex shrink-0 items-center gap-1">
                  <a
                    href={
                      typeof adj.dataUrl === 'string' &&
                      /^data:[^,;]*;base64,[A-Za-z0-9+/\r\n]*={0,2}$/.test(adj.dataUrl)
                        ? adj.dataUrl
                        : undefined
                    }
                    download={adj.name}
                    class="text-primary-text hover:text-primary sytue-focus-ring flex h-6 w-6 items-center justify-center rounded"
                    title={t('note.downloadAttachment')}
                    aria-label={t('note.downloadAttachment')}
                  >
                    <Glyph name="download" size={13} />
                  </a>
                  {!soloLectura ? (
                    <button
                      type="button"
                      onClick={() => eliminarAdjunto(adj.id)}
                      class="text-text-muted hover:text-danger-text sytue-focus-ring flex h-6 w-6 items-center justify-center rounded"
                      title={t('note.deleteAttachment')}
                      aria-label={t('note.deleteAttachment')}
                    >
                      <Glyph name="trash" size={13} />
                    </button>
                  ) : null}
                </div>
              </div>
            ))}
          </div>
        </div>
      ) : null}

      {note.states && note.states.length > 0 ? (
        // En una fila, las etiquetas van bajo el texto, no bajo el glifo.
        <footer class={cx('flex flex-wrap gap-1', diario && 'pl-7')}>
          {note.states.map((s) => (
            <Badge key={s.state_id} tone="neutral" class="gap-1">
              <span
                aria-hidden="true"
                class="inline-block h-2 w-2 rounded-full"
                style={{ background: s.color }}
              />
              {s.emoji ? <span aria-hidden="true">{s.emoji}</span> : null}
              <span class="sr-only">{s.tag_name}: </span>
              {s.name}
            </Badge>
          ))}
        </footer>
      ) : null}
    </article>
  );
}

/** El cuerpo por tipo de nota. Fuera del componente principal por legibilidad. */
function Cuerpo({
  t,
  note,
  contenido,
  enlace,
  onEdit,
  publicBase,
  onNavigate,
  previa,
  onPlegar,
  llenar = false,
}: {
  t: T;
  note: Note;
  contenido: string;
  /** La ficha de un enlace a otra nota (VistaPreviaEnlace.tsx). */
  previa?: (d: DestinoInterno) => Promise<VistaPrevia | null>;
  /** Pulsar la viñeta de una lista con sublista la pliega: el HTML nuevo, para guardarlo. */
  onPlegar?: (html: string) => void;
  /** La tarjeta está estirada: el cuerpo crece con ella. Ver `NoteCard`. */
  llenar?: boolean;
  enlace?: NoteLink;
  publicBase?: string;
  onNavigate?: (d: DestinoInterno) => void;
  onEdit?: () => void;
}): JSX.Element | null {
  switch (note.kind) {
    case 'color':
      return (
        <div
          class="border-border h-12 rounded-md border"
          style={{ background: texto(note.data, 'color') || '#86198f' }}
          aria-label={texto(note.data, 'color')}
        />
      );

    case 'image':
      if (!/^https?:\/\//i.test(texto(note.data, 'url'))) return null;
      return (
        <img
          src={texto(note.data, 'url')}
          alt={note.title ?? ''}
          loading="lazy"
          class="border-border max-h-64 w-full rounded-md border object-contain"
        />
      );

    case 'doc':
      // El documento se pinta a través del BFF de esta app y dentro de un iframe
      // con `sandbox` vacío: origen opaco y sin scripts. Ver api/doc-preview.ts.
      return (
        <div class="grid gap-1.5">
          <iframe
            src={withBase(
              import.meta.env.BASE_URL,
              `/api/doc-preview?id=${encodeURIComponent(enlace?.target_id ?? '')}`,
            )}
            title={enlace?.label ?? t('docs.embedded')}
            sandbox=""
            loading="lazy"
            class="border-border h-64 w-full rounded-md border bg-white"
          />
          <EnlaceExterno
            t={t}
            {...(enlace?.url ? { url: enlace.url } : {})}
            etiqueta={t('docs.openViewer')}
          />
        </div>
      );

    case 'link':
    case 'drive':
    case 'file':
    case 'event':
    case 'room':
    case 'crossref':
    case 'launcher': {
      const textoCuerpo = textoDeNota(contenido);
      return (
        <div class="grid gap-1">
          {textoCuerpo ? <p class="text-text-muted text-base break-words">{textoCuerpo}</p> : null}
          <EnlaceExterno
            t={t}
            url={enlace?.url ?? texto(note.data, 'url')}
            {...(enlace?.label ? { etiqueta: enlace.label } : {})}
          />
        </div>
      );
    }

    default:
      // Texto y tarea: HTML ya saneado por el servicio al guardarlo (allowlist en
      // services/notes/internal/domain/html.go). Es la razón por la que se puede
      // pintar el contenido de una nota escrita por OTRA persona del equipo.
      // Se editable y no editable son DOS elementos y no uno con atributos
      // condicionales: un `role`/`tabIndex` que aparece y desaparece es
      // exactamente lo que ni el linter ni un lector de pantalla saben leer.
      //
      // Nota vacía: se dice qué hacer, en vez de una caja en blanco que no invita.
      // Va ANTES de preparar el cuerpo: sin contenido no hay anclas que calcular
      // ni enlaces que interceptar.
      if (!contenido && onEdit) {
        return (
          <button
            type="button"
            onClick={onEdit}
            class="text-text-subtle min-h-[24px] w-full cursor-text text-left text-base leading-relaxed"
          >
            {t('note.placeholder')}
          </button>
        );
      }

      // Un clic sobre un enlace a OTRA nota navega dentro de la app, sin
      // recargar; sobre cualquier otro enlace deja hacer al navegador (ya abre
      // en otra pestaña); y sobre el resto del cuerpo, edita. Los títulos llevan
      // su `id` para que un enlace con `h=` llegue hasta ellos.
      const alPulsar = (e: MouseEvent): boolean => {
        // La viñeta de un elemento con sublista pliega o despliega la lista. La
        // viñeta se pinta FUERA de la caja del `<li>` y el clic cae en el `<li>`:
        // así se distingue de pulsar su texto, que edita.
        const objetivo = e.target;
        if (
          objetivo instanceof HTMLLIElement &&
          e.clientX < objetivo.getBoundingClientRect().left &&
          liPlegable(objetivo)
        ) {
          e.preventDefault();
          const n = [...(e.currentTarget as HTMLElement).querySelectorAll('li')].indexOf(objetivo);
          if (onPlegar) onPlegar(alternarPlegado(contenido, n));
          else objetivo.classList.toggle('plegada');
          return true;
        }
        const a = (e.target as HTMLElement | null)?.closest('a');
        if (!a) return false;
        const d = publicBase ? destinoInterno(a.href, publicBase) : null;
        if (d && onNavigate) {
          e.preventDefault();
          onNavigate(d);
        }
        return true;
      };
      const html = conMenciones(conHashtags(conAnclas(contenido || '')));
      return onEdit ? (
        <CuerpoEditable
          t={t}
          html={html}
          llenar={llenar}
          onEdit={onEdit}
          alPulsar={alPulsar}
          {...(publicBase ? { publicBase } : {})}
          {...(previa ? { previa } : {})}
        />
      ) : (
        <CuerpoLectura
          t={t}
          html={html}
          alPulsar={alPulsar}
          llenar={llenar}
          {...(publicBase ? { publicBase } : {})}
          {...(previa ? { previa } : {})}
        />
      );
  }
}

/** El cuerpo que se puede pulsar para editar. */
function CuerpoEditable({
  t,
  html,
  llenar,
  onEdit,
  alPulsar,
  publicBase,
  previa,
}: {
  t: T;
  html: string;
  llenar: boolean;
  onEdit: () => void;
  alPulsar: (e: MouseEvent) => boolean;
  publicBase?: string;
  previa?: (d: DestinoInterno) => Promise<VistaPrevia | null>;
}): JSX.Element {
  const ref = useRef<HTMLDivElement>(null);
  const ficha = useVistaPreviaEnlaces(ref, t, publicBase, previa);
  return (
    <>
      <div
        ref={ref}
        // `role="button"` con foco y teclado: pulsar la nota la abre para
        // editar, y quien no usa ratón llega igual con Tab y Enter. Meter el
        // HTML dentro de un `<button>` de verdad no vale: un botón solo admite
        // contenido de frase, y aquí hay párrafos y listas.
        role="button"
        tabIndex={0}
        aria-label={t('note.placeholder')}
        // `min-h-[24px]` dejaba la zona pulsable en las primeras líneas y el resto
        // muerto cuando la tarjeta está estirada; el recorte del extracto y el
        // `flex-1` viven los dos en `claseCuerpo`.
        class={cx(claseCuerpo(llenar), 'cursor-text')}
        onClick={(e) => {
          if (!alPulsar(e)) onEdit();
        }}
        onKeyDown={(e) => {
          if (e.key === 'Enter' || e.key === ' ') {
            e.preventDefault();
            onEdit();
          }
        }}
        // eslint-disable-next-line react/no-danger
        dangerouslySetInnerHTML={{ __html: html }}
      />
      {ficha}
    </>
  );
}

/**
 * El cuerpo de solo lectura. El clic se escucha con `addEventListener` y no con
 * `onClick`: un `div` sin rol con manejador de clic es lo que la accesibilidad
 * prohíbe, y aquí el clic solo sirve para seguir un enlace que ya es un `<a>`
 * de verdad, al que el teclado llega por su cuenta.
 */
/**
 * Las clases del cuerpo de una nota, iguales se pueda editar o no.
 *
 * LA NOTA SE VE ENTERA, POR LARGA QUE SEA: aquí no se recorta nada. Se probó lo
 * contrario —un `max-h` en el extracto— y es justo lo que no se quiere: una
 * nota larga DEBE ocupar lo que ocupe, y quien la abre espera leerla, no ver un
 * resumen. Lo que fallaba era el fondo de la tarjeta, que no acompañaba al
 * texto; eso se arregla en la rejilla de `NoteCanvas`, no aquí.
 *
 * `flex-1` es para el caso contrario: con la tarjeta estirada y una nota corta,
 * el cuerpo crece hasta llenarla para que se pueda escribir en todo el alto.
 */
export function claseCuerpo(llenar: boolean): string {
  return cx(
    'sytue-prose text-text min-h-[24px] text-base leading-relaxed break-words',
    llenar && 'flex-1',
  );
}

function CuerpoLectura({
  t,
  html,
  alPulsar,
  llenar,
  publicBase,
  previa,
}: {
  t: T;
  html: string;
  alPulsar: (e: MouseEvent) => boolean;
  llenar: boolean;
  publicBase?: string;
  previa?: (d: DestinoInterno) => Promise<VistaPrevia | null>;
}): JSX.Element {
  const ref = useRef<HTMLDivElement>(null);
  const ficha = useVistaPreviaEnlaces(ref, t, publicBase, previa);
  useEffect(() => {
    const el = ref.current;
    if (!el) return;
    const h = (e: MouseEvent) => void alPulsar(e);
    el.addEventListener('click', h);
    return () => el.removeEventListener('click', h);
  }, [alPulsar]);
  return (
    <>
      <div
        ref={ref}
        class={claseCuerpo(llenar)}
        // eslint-disable-next-line react/no-danger
        dangerouslySetInnerHTML={{ __html: html }}
      />
      {ficha}
    </>
  );
}

function EnlaceExterno({
  t,
  url,
  etiqueta,
}: {
  t: T;
  url?: string;
  etiqueta?: string;
}): JSX.Element | null {
  // Solo http(s). El servicio ya lo exige al guardar, pero una fila antigua o
  // un `data.url` de otra ruta no debe convertirse aquí en un `javascript:`
  // pulsable por otra persona del cuaderno.
  if (!url || !/^https?:\/\//i.test(url)) return null;
  return (
    <a
      href={url}
      target="_blank"
      rel="noopener noreferrer"
      class="text-primary-text sytue-focus-ring inline-flex items-center gap-1 rounded text-base hover:underline"
    >
      <Icon name="arrowUpRight" size={14} />
      <span class="truncate">{etiqueta || url}</span>
      <span class="sr-only">{t('note.openIn', { app: 'Sytue' })}</span>
    </a>
  );
}

/**
 * El indicador de recordatorio de una nota: la campana con la fecha corta, y la marca
 * de que repite. En rojo suave si ya pasó (una tarea que repite y sonó sin hacerse).
 * Lo comparten la tarjeta y la fila del árbol.
 */
export function RecordatorioEnFila({
  t,
  r,
  onClick,
}: {
  t: T;
  r: Reminder;
  onClick?: () => void;
}): JSX.Element {
  const cuando = rotuloRecordatorio(r, documentLocale());
  const repite = Boolean(r.rrule) || (r.after_days ?? 0) > 0;
  const pasado = yaPaso(r);
  const rotulo = t('reminder.at', { when: cuando }) + (repite ? ` · ${t('reminder.repeats')}` : '');
  const clases = cx(
    'inline-flex shrink-0 items-center gap-1 rounded-sm px-1 text-xs tabular-nums',
    pasado ? 'text-danger-text' : 'text-text-muted',
  );
  const dentro = (
    <>
      <Icon name="bell" size={12} />
      <span>{cuando}</span>
      {repite ? <Icon name="rotate" size={11} /> : null}
    </>
  );
  return onClick ? (
    <button
      type="button"
      onClick={onClick}
      title={rotulo}
      aria-label={rotulo}
      class={cx(clases, 'sytue-focus-ring hover:bg-bg-subtle')}
    >
      {dentro}
    </button>
  ) : (
    <span class={clases} title={rotulo} aria-label={rotulo} role="img">
      {dentro}
    </span>
  );
}
