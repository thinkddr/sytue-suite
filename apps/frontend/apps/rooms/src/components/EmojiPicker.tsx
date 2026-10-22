/**
 * EmojiPicker — selector de emoji buscable y navegable por teclado. Se usa en dos
 * sitios: el Composer (inserta el glyph en el texto) y la barra de reacciones de un
 * mensaje (elige con qué emoji reaccionar, cualquiera del catálogo, no solo el set
 * rápido). Reusa el mismo mapa EMOJI de la Ola 1 vía lib/emoji.ts.
 *
 * Patrón combobox + grid (WAI-ARIA): el foco permanece en el buscador y la opción
 * activa se marca con aria-activedescendant; las flechas mueven por la rejilla,
 * Enter elige, Esc cierra. Se pinta en un portal con posición fija anclada al
 * disparador (mismo motivo que MessageMenu: el histórico va en un scroller
 * virtualizado que recortaría un popover absolute).
 */
import type { JSX, TargetedEvent } from 'preact';
import { createPortal } from 'preact/compat';
import { useEffect, useRef, useState } from 'preact/hooks';
import { cx, Icon, Button, Modal, limitesViewport, useEnHoja, useFitInViewport } from '@sytue/ui';
import { EmojiGlyph } from './glyphs.js';
import { searchEmoji } from '../lib/emoji.js';
import {
  customEmojiList,
  customEmojiSrc,
  setCustomEmoji,
  subscribeCustomEmoji,
} from '../lib/customEmoji.js';
import { uploadAttachment, createCustomEmoji } from '../client/chatApi.js';
import { hayPunteroFino, useCierreDePopover } from '../lib/popover.js';
import type { T } from '../i18n/index.js';
import { contiene } from '@sytue/i18n';

const COLS = 8;
const POP_W = 300;
const POP_H = 332;

/** Nombre de emoji personalizado (igual invariante que el backend: minúsculas,
 *  dígitos, «_» o «-», 1..64). */
const EMOJI_NAME_RE = /^[a-z0-9_-]{1,64}$/;

/** Tope de subida de un emoji. ponytail: cap propio de emoji (no 25 MB como un
 *  adjunto cualquiera); Drive aplica además su ceiling real (413) server-side. */
const MAX_EMOJI_BYTES = 512 * 1024;
const MAX_EMOJI_KB = Math.round(MAX_EMOJI_BYTES / 1024);

const DEFAULT_TRIGGER =
  'flex h-8 w-8 items-center justify-center rounded-control text-text-muted transition-colors duration-150 hover:bg-bg-subtle hover:text-text sytue-focus-ring';

export function EmojiPickerButton({
  onPick,
  t,
  label,
  title,
  triggerClass,
  disabled = false,
  size = 18,
  children,
}: {
  /** Recibe el glyph elegido. El llamante gestiona el foco tras insertar/reaccionar. */
  onPick: (glyph: string) => void;
  /** Traductor ya atado al idioma. */
  t: T;
  label?: string;
  title?: string;
  triggerClass?: string;
  /**
   * Apaga el disparador con el resto del cromo de su barra.
   *
   * Existe porque no existía: el compositor de Rooms se bloquea entero —sin canal, o
   * sin red— y este botón se quedaba vivo, así que se podía abrir el selector e
   * insertar un glyph en un campo que no admitía escritura.
   */
  disabled?: boolean;
  /** Tamaño del glyph por defecto (px). Para igualar la barra de acciones del mensaje. */
  size?: number;
  /** Contenido del disparador (por defecto, la cara sonriente). */
  children?: JSX.Element;
}): JSX.Element {
  const [open, setOpen] = useState(false);
  const [pos, setPos] = useState<{ top: number; left: number } | null>(null);
  const [query, setQuery] = useState('');
  const [active, setActive] = useState(0);
  // Ola 14: subir un emoji personalizado (formulario plegable dentro del popover).
  const [showUpload, setShowUpload] = useState(false);
  const [emojiName, setEmojiName] = useState('');
  const [emojiFile, setEmojiFile] = useState<File | null>(null);
  const [uploadBusy, setUploadBusy] = useState(false);
  const [uploadErr, setUploadErr] = useState<string | null>(null);
  const [, setCustomTick] = useState(0);
  const triggerRef = useRef<HTMLButtonElement>(null);
  const popRef = useRef<HTMLDivElement>(null);
  const inputRef = useRef<HTMLInputElement>(null);
  const listRef = useRef<HTMLDivElement>(null);

  useFitInViewport(popRef, open, { vertical: true });
  // Con qué se pulsó lo último. Preact tipa el `click` como MouseEvent y ahí ya no
  // se puede preguntar el `pointerType`, así que se anota en el pointerdown, que
  // siempre lo precede.
  const ultimoPuntero = useRef<string>('');

  // Sin etiqueta propia, la genérica del catálogo («Emoji»).
  const buttonLabel = label ?? t('emoji.label');

  const results = searchEmoji(query);
  // Emoji personalizados del tenant (Ola 14) que casan con la query (subcadena).
  const q = query.trim().toLowerCase();
  const customMatches = customEmojiList().filter((e) => (q ? contiene(e.name, q) : true));

  // Reacciona a los cambios del catálogo del tenant (carga inicial / subida) mientras
  // el popover está abierto: sin esto, un emoji recién subido no aparecería hasta
  // reabrir. Solo el picker ABIERTO se suscribe (uno a la vez), no cada disparador.
  useEffect(() => {
    if (!open) return;
    return subscribeCustomEmoji(() => setCustomTick((t) => t + 1));
  }, [open]);

  function close(refocus = true): void {
    setOpen(false);
    setQuery('');
    setActive(0);
    setShowUpload(false);
    setUploadErr(null);
    if (refocus) triggerRef.current?.focus();
  }

  /** Sube la imagen a Drive y liga el shortcode (POST /emoji). Valida nombre + tamaño. */
  async function submitUpload(): Promise<void> {
    const name = emojiName.trim().toLowerCase();
    if (!EMOJI_NAME_RE.test(name)) {
      setUploadErr(t('emoji.badName'));
      return;
    }
    if (customEmojiList().some((e) => e.name === name)) {
      setUploadErr(t('emoji.exists', { name }));
      return;
    }
    if (!emojiFile) {
      setUploadErr(t('emoji.pickImage'));
      return;
    }
    if (!emojiFile.type.startsWith('image/')) {
      setUploadErr(t('emoji.mustBeImage'));
      return;
    }
    if (emojiFile.size > MAX_EMOJI_BYTES) {
      setUploadErr(t('emoji.tooLarge', { max: MAX_EMOJI_KB }));
      return;
    }
    setUploadBusy(true);
    setUploadErr(null);
    try {
      const att = await uploadAttachment(emojiFile);
      const { emoji } = await createCustomEmoji({ name, fileId: att.fileId });
      // Actualiza el catálogo de módulo (notifica al picker y a MessageBody).
      setCustomEmoji(
        [...customEmojiList().filter((e) => e.name !== emoji.name), emoji].sort((a, b) =>
          a.name.localeCompare(b.name),
        ),
      );
      setEmojiName('');
      setEmojiFile(null);
      setShowUpload(false);
    } catch {
      setUploadErr(t('emoji.uploadFailed'));
    } finally {
      setUploadBusy(false);
    }
  }

  function openPop(): void {
    const el = triggerRef.current;
    if (!el) return;
    const r = el.getBoundingClientRect();
    const v = limitesViewport(8);
    // Volteo arriba/abajo y sujeción horizontal al viewport (como MessageMenu).
    const alto = Math.min(POP_H, v.height);
    const top =
      r.bottom + 4 + alto <= v.bottom
        ? Math.max(v.top, r.bottom + 4)
        : Math.max(v.top, Math.min(r.top - alto - 4, v.bottom - alto));
    const left = Math.max(v.left, Math.min(r.left, v.right - Math.min(POP_W, v.width)));
    setPos({ top, left });
    setQuery('');
    setActive(0);
    setOpen(true);
  }

  // El foco al buscador SOLO donde hay puntero fino. Con el dedo, enfocar un
  // campo levanta el teclado virtual: se come media pantalla —justo la del
  // catálogo— y, hasta el arreglo de lib/popover.ts, además cerraba el popover.
  // Quien escribe con teclado sigue pudiendo buscar: el campo está el primero.
  useEffect(() => {
    if (open && hayPunteroFino()) inputRef.current?.focus();
  }, [open]);

  // En el teléfono el selector es la hoja de los diálogos (`Modal`): junto al botón no cabe
  // —medido: el panel se salía por abajo en 390×844— y la hoja ya cierra con el velo,
  // el asa, Escape y «atrás».
  const enHoja = useEnHoja();
  useCierreDePopover(open && !enHoja, () => setOpen(false), [popRef, triggerRef]);

  // Mantener el activo dentro del rango al filtrar.
  useEffect(() => {
    setActive((a) => Math.min(a, Math.max(0, results.length - 1)));
  }, [results.length]);

  // Desplazar la opción activa a la vista dentro de la rejilla.
  useEffect(() => {
    if (!open) return;
    listRef.current
      ?.querySelector<HTMLElement>(`#emoji-opt-${active}`)
      ?.scrollIntoView({ block: 'nearest' });
  }, [active, open]);

  function pick(i: number): void {
    const e = results[i];
    if (!e) return;
    onPick(e.glyph);
    close();
  }

  function onKeyDown(e: TargetedEvent<HTMLInputElement, KeyboardEvent>): void {
    const n = results.length;
    switch (e.key) {
      case 'Escape':
        e.preventDefault();
        close();
        break;
      case 'Enter':
        e.preventDefault();
        pick(active);
        break;
      case 'ArrowRight':
        e.preventDefault();
        setActive((a) => Math.min(n - 1, a + 1));
        break;
      case 'ArrowLeft':
        e.preventDefault();
        setActive((a) => Math.max(0, a - 1));
        break;
      case 'ArrowDown':
        e.preventDefault();
        setActive((a) => Math.min(n - 1, a + COLS));
        break;
      case 'ArrowUp':
        e.preventDefault();
        setActive((a) => Math.max(0, a - COLS));
        break;
      case 'Home':
        e.preventDefault();
        setActive(0);
        break;
      case 'End':
        e.preventDefault();
        setActive(n - 1);
        break;
    }
  }

  const activeName = results[active]?.names[0];

  // Lo de dentro, el mismo en el popover del escritorio y en la hoja del teléfono.
  const contenido = (
    <>
      <input
        ref={inputRef}
        type="text"
        role="combobox"
        aria-expanded="true"
        aria-controls="emoji-listbox"
        aria-activedescendant={results.length > 0 ? `emoji-opt-${active}` : undefined}
        autocomplete="off"
        value={query}
        placeholder={t('emoji.searchPlaceholder')}
        aria-label={t('emoji.searchLabel')}
        onInput={(e) => {
          setQuery(e.currentTarget.value);
          setActive(0);
        }}
        onKeyDown={onKeyDown}
        class="border-border bg-bg text-text placeholder:text-text-subtle sytue-focus-ring mb-2 w-full rounded-md border px-2.5 py-1.5 text-sm outline-none"
      />
      {results.length === 0 ? (
        <p class="text-text-muted px-1 py-6 text-center text-sm">{t('search.noResults')}</p>
      ) : (
        <div
          ref={listRef}
          id="emoji-listbox"
          role="listbox"
          aria-label={t('emoji.label')}
          class="grid max-h-56 grid-cols-8 gap-0.5 overflow-y-auto max-md:max-h-none"
        >
          {results.map((e, i) => (
            <button
              key={e.glyph}
              id={`emoji-opt-${i}`}
              role="option"
              aria-selected={i === active}
              type="button"
              tabIndex={-1}
              title={`:${e.names[0]}:`}
              aria-label={e.names[0]}
              onMouseEnter={() => setActive(i)}
              // Con RATÓN se elige en pointerdown y se evita que el botón
              // robe el foco del buscador (importa en el Composer). Con el
              // DEDO no: `preventDefault` aquí cancelaría también el
              // arrastre y la rejilla dejaría de poder desplazarse. Ahí se
              // elige en el click, que el navegador solo emite si el toque
              // fue un toque y no un deslizamiento.
              onPointerDown={(ev) => {
                ultimoPuntero.current = ev.pointerType;
                if (ev.pointerType !== 'mouse') return;
                ev.preventDefault();
                pick(i);
              }}
              onClick={() => {
                if (ultimoPuntero.current === 'mouse') return;
                pick(i);
              }}
              class={cx(
                'flex h-8 w-full min-w-0 items-center justify-center rounded text-lg transition-colors duration-150 pointer-coarse:h-11',
                i === active ? 'bg-selected' : 'hover:bg-bg-subtle',
              )}
            >
              <span aria-hidden="true">{e.glyph}</span>
            </button>
          ))}
        </div>
      )}
      {activeName ? (
        <p class="sytue-meta mt-1 truncate px-1" aria-hidden="true">
          :{activeName}:
        </p>
      ) : null}

      {/* Ola 14: emoji personalizados del tenant (insertar / reaccionar). */}
      {customMatches.length > 0 ? (
        <div class="border-border mt-2 border-t pt-2">
          <p class="sytue-meta mb-1 px-1 font-medium">{t('emoji.custom')}</p>
          <div
            role="listbox"
            aria-label={t('emoji.customList')}
            class="grid max-h-24 grid-cols-8 gap-0.5 overflow-y-auto"
          >
            {customMatches.map((e) => (
              <button
                key={e.name}
                role="option"
                aria-selected={false}
                type="button"
                title={`:${e.name}:`}
                aria-label={e.name}
                // Mismo reparto que en los Unicode: con ratón en pointerdown
                // (para no robar el foco del buscador) y con el dedo en el
                // click (para no cancelar el arrastre de la rejilla).
                onPointerDown={(ev) => {
                  ultimoPuntero.current = ev.pointerType;
                  if (ev.pointerType !== 'mouse') return;
                  ev.preventDefault();
                  onPick(`:${e.name}:`);
                  close();
                }}
                onClick={() => {
                  if (ultimoPuntero.current === 'mouse') return;
                  onPick(`:${e.name}:`);
                  close();
                }}
                class="hover:bg-bg-subtle sytue-focus-ring flex h-8 w-full min-w-0 items-center justify-center rounded transition-colors duration-150"
              >
                <img
                  src={customEmojiSrc(e.fileId)}
                  alt={`:${e.name}:`}
                  class="h-5 w-5 object-contain"
                  loading="lazy"
                  decoding="async"
                />
              </button>
            ))}
          </div>
        </div>
      ) : null}

      {/* Ola 14: subir un emoji personalizado (nombre + imagen → Drive → /emoji). */}
      <div class="border-border mt-2 border-t pt-2">
        {showUpload ? (
          <div class="flex flex-col gap-1.5">
            <input
              type="text"
              value={emojiName}
              placeholder={t('emoji.namePlaceholder')}
              aria-label={t('emoji.nameLabel')}
              maxLength={64}
              autocomplete="off"
              onInput={(e) => {
                setEmojiName(e.currentTarget.value);
                if (uploadErr) setUploadErr(null);
              }}
              class="border-border bg-bg text-text placeholder:text-text-subtle sytue-focus-ring w-full rounded-md border px-2 py-1 text-sm outline-none"
            />
            <input
              type="file"
              accept="image/*"
              aria-label={t('emoji.imageLabel')}
              onChange={(e) => {
                setEmojiFile(e.currentTarget.files?.[0] ?? null);
                if (uploadErr) setUploadErr(null);
              }}
              class="text-text-muted file:bg-bg-subtle file:text-text-muted w-full text-xs file:mr-2 file:rounded file:border-0 file:px-2 file:py-1"
            />
            {uploadErr ? (
              <p class="text-danger-text flex items-center gap-1 text-xs" role="alert">
                <Icon name="warning" size={11} />
                {uploadErr}
              </p>
            ) : (
              <p class="sytue-meta">{t('emoji.imageHint', { max: MAX_EMOJI_KB })}</p>
            )}
            <div class="flex items-center gap-1.5">
              <Button size="sm" disabled={uploadBusy} onClick={() => void submitUpload()}>
                {uploadBusy ? t('state.uploading') : t('action.upload')}
              </Button>
              <Button
                size="sm"
                variant="secondary"
                disabled={uploadBusy}
                onClick={() => {
                  setShowUpload(false);
                  setUploadErr(null);
                }}
              >
                {t('action.cancel')}
              </Button>
            </div>
          </div>
        ) : (
          <button
            type="button"
            onClick={() => setShowUpload(true)}
            class="text-text-muted hover:bg-bg-subtle hover:text-text sytue-focus-ring rounded-control flex w-full items-center gap-1.5 px-1 py-1 text-xs font-medium transition-colors duration-150"
          >
            <span aria-hidden="true" class="text-sm leading-none font-semibold">
              +
            </span>
            {t('emoji.uploadCustom')}
          </button>
        )}
      </div>
    </>
  );

  return (
    <>
      <button
        ref={triggerRef}
        type="button"
        aria-haspopup="dialog"
        aria-expanded={open}
        aria-label={buttonLabel}
        title={title ?? buttonLabel}
        disabled={disabled}
        onClick={() => (open ? close() : openPop())}
        class={triggerClass ?? DEFAULT_TRIGGER}
      >
        {children ?? <EmojiGlyph size={size} />}
      </button>
      {open && enHoja ? (
        <Modal open onClose={() => close()} title={buttonLabel} size="sm">
          {contenido}
        </Modal>
      ) : null}
      {open && pos && !enHoja
        ? createPortal(
            <div
              ref={popRef}
              role="dialog"
              aria-label={buttonLabel}
              style={{
                position: 'fixed',
                top: `${pos.top}px`,
                left: `${pos.left}px`,
                width: `${POP_W}px`,
                zIndex: 'var(--z-dropdown)',
              }}
              class="sytue-pop-in sytue-popover-surface border-border flex flex-col rounded-lg border p-2 shadow-[var(--shadow-md)]"
            >
              {contenido}
            </div>,
            document.body,
          )
        : null}
    </>
  );
}
