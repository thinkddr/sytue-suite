/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * NotebookSettings — todo lo que define un cuaderno, en UNA pantalla: nombre,
 * icono, color, disposición, quién puede verlo, si entra en la búsqueda de la
 * suite y si va cifrado.
 *
 * Las cuatro visibilidades se enseñan con su consecuencia escrita al lado, no
 * con un desplegable de cuatro palabras: «privado» y «personal» suenan igual y
 * la diferencia —una está en la búsqueda y la otra no— es justo lo que hay que
 * saber antes de elegir.
 *
 * La casilla del índice se preselecciona según la visibilidad y se BLOQUEA cuando
 * el cuaderno va cifrado: no hay texto en claro que indexar, y dejar la casilla
 * activable prometería algo que el servicio rechaza.
 */
import type { JSX } from 'preact';
import { useId, useState } from 'preact/hooks';
import { Button, Checkbox, FieldGroup, Input, Modal, Switch, cx, useToast } from '@sytue/ui';
import type { NoteVisibility, Notebook, NotebookInput, NotebookLayout } from '@sytue/api-clients';
import { appUrl } from '@sytue/config';
import type { T } from '../i18n/index.js';
import type { NotesCrypto } from '../client/crypto.js';
import { Glyph, type GlyphName } from './glyphs.js';
import { PassphraseDialog } from './PassphraseDialog.js';

interface Props {
  t: T;
  /** Cuaderno a editar, o null para crear uno nuevo. */
  notebook: Notebook | null;
  /** Cuaderno padre al crear (anidamiento). */
  parentId?: string | null;
  onClose: () => void;
  onSave: (input: NotebookInput) => Promise<void>;
  onDelete?: () => void;
  /** Archivar o devolver al árbol. Solo su dueño, y nunca un diario. */
  onArchive?: () => void;
  archivado?: boolean;
  /** Solo quien creó el cuaderno o quien administra la organización puede cambiar su visibilidad o cifrado. */
  puedeCambiarAcceso?: boolean;
}

const VISIBILIDADES: { key: NoteVisibility; glyph: GlyphName }[] = [
  { key: 'private', glyph: 'notebook' },
  { key: 'personal', glyph: 'note' },
  { key: 'organization', glyph: 'link' },
  { key: 'public', glyph: 'link' },
];

const LAYOUTS: { key: NotebookLayout; glyph: GlyphName }[] = [
  // El árbol va primero porque es el modo por defecto; el resto son «funciones
  // añadidas», como pide el producto.
  { key: 'tree', glyph: 'layoutTree' },
  { key: 'free', glyph: 'layoutFree' },
  { key: 'columns', glyph: 'layoutColumns' },
  { key: 'list', glyph: 'layoutList' },
];

/** Paleta de cuadernos. Doce tonos medidos para llevar texto blanco encima. */
const COLORES = [
  '#86198f',
  '#be123c',
  '#c2410c',
  '#a16207',
  '#4d7c0f',
  '#146a47',
  '#0f766e',
  '#0e7490',
  '#1c58d9',
  '#4c1d95',
  '#7e22ce',
  '#3a4a5e',
];

export function NotebookSettings({
  t,
  notebook,
  parentId,
  onClose,
  onSave,
  onDelete,
  onArchive,
  archivado = false,
  puedeCambiarAcceso = notebook === null,
}: Props): JSX.Element {
  const toast = useToast();
  const base = useId();
  const idEmoji = `${base}-emoji`;
  const idTitulo = `${base}-titulo`;
  const [title, setTitle] = useState(notebook?.title ?? '');
  const [emoji, setEmoji] = useState(notebook?.emoji ?? '');
  const [color, setColor] = useState(notebook?.color ?? COLORES[0]!);
  const [layout, setLayout] = useState<NotebookLayout>(notebook?.layout ?? 'tree');
  const [visibility, setVisibility] = useState<NoteVisibility>(notebook?.visibility ?? 'personal');
  const [indexable, setIndexable] = useState(notebook?.indexable ?? true);
  const [encrypted, setEncrypted] = useState(notebook?.encrypted ?? false);
  const [crypto, setCrypto] = useState<NotesCrypto | undefined>(notebook?.crypto);
  const [pidiendoClave, setPidiendoClave] = useState(false);
  const [guardando, setGuardando] = useState(false);

  // Cambiar de visibilidad reajusta la casilla del índice al valor por defecto de
  // esa visibilidad. Privado NUNCA se indexa; el resto sí salvo que se diga.
  const cambiarVisibilidad = (v: NoteVisibility) => {
    setVisibility(v);
    setIndexable(v !== 'private' && !encrypted);
  };

  const activarCifrado = (activar: boolean) => {
    if (!activar) {
      setEncrypted(false);
      setCrypto(undefined);
      return;
    }
    setPidiendoClave(true);
  };

  const guardar = async () => {
    setGuardando(true);
    try {
      await onSave({
        parent_id: notebook?.parent_id ?? parentId ?? null,
        title: title.trim(),
        emoji,
        color,
        layout,
        visibility,
        indexable: encrypted ? false : indexable,
        encrypted,
        crypto: encrypted && crypto ? crypto : null,
        room_id: notebook?.room_id ?? '',
      });
    } finally {
      setGuardando(false);
    }
  };

  // SIN org a propósito (ADR 0056): el enlace de un cuaderno publicado es para
  // quien tenga el token, que puede no ser de la organización —y puede no tener
  // cuenta—. Va fuera del `publicBase` de las notas, que sí lo lleva.
  const enlacePublico =
    notebook?.public_token && visibility === 'public'
      ? `${appUrl('notes')}/p/${notebook.public_token}`
      : null;

  return (
    <>
      <Modal
        open
        onClose={onClose}
        title={notebook ? t('notebook.settings') : t('sidebar.newNotebook')}
        footer={
          <>
            {/* Eliminar y archivar a la izquierda en escritorio: el último de los dos lleva
                el `md:mr-auto` que empuja Cancelar | Guardar a la derecha. En el teléfono,
                de dos en dos (el pie del Modal). */}
            {onDelete ? (
              <Button
                variant="danger"
                class={onArchive ? undefined : 'md:mr-auto'}
                onClick={onDelete}
              >
                {t('notebook.delete')}
              </Button>
            ) : null}
            {onArchive ? (
              <Button variant="secondary" class="md:mr-auto" onClick={onArchive}>
                {t(archivado ? 'archive.unarchive' : 'archive.archive')}
              </Button>
            ) : null}
            <Button variant="ghost" onClick={onClose}>
              {t('common.cancel')}
            </Button>
            <Button
              loading={guardando}
              disabled={title.trim() === ''}
              onClick={() => void guardar()}
            >
              {t('common.save')}
            </Button>
          </>
        }
      >
        {/* `grid-cols-1` (minmax(0, 1fr)) y no la columna `auto`: el enlace público, que
            no parte, ensanchaba la columna a 505 px en una hoja de 342 y el diálogo
            entero se desplazaba de lado. */}
        <div class="grid grid-cols-1 gap-4">
          <div class="grid grid-cols-[5rem_1fr] gap-3">
            <FieldGroup label={t('notebook.emoji')} htmlFor={idEmoji}>
              <Input
                id={idEmoji}
                value={emoji}
                maxLength={4}
                class="text-center text-xl"
                placeholder="📓"
                onInput={(e) => setEmoji((e.target as HTMLInputElement).value)}
              />
            </FieldGroup>
            <FieldGroup label={t('notebook.title')} htmlFor={idTitulo}>
              <Input
                id={idTitulo}
                value={title}
                autofocus
                onInput={(e) => setTitle((e.target as HTMLInputElement).value)}
              />
            </FieldGroup>
          </div>

          <fieldset>
            <legend class="text-text mb-1.5 text-base font-medium">{t('notebook.color')}</legend>
            <div class="flex flex-wrap gap-1.5 pointer-coarse:gap-4">
              {COLORES.map((c) => (
                <button
                  key={c}
                  type="button"
                  aria-label={c}
                  aria-pressed={color === c}
                  onClick={() => setColor(c)}
                  style={{ background: c }}
                  class={cx(
                    // Con el dedo, el blanco es de 44 (`after:-inset-2`) y el paso también.
                    "sytue-focus-ring relative h-7 w-7 rounded-full transition-transform duration-150 pointer-coarse:after:absolute pointer-coarse:after:-inset-2 pointer-coarse:after:content-['']",
                    color === c && 'ring-text ring-2 ring-offset-2 ring-offset-[var(--sytue-bg)]',
                  )}
                />
              ))}
            </div>
          </fieldset>

          <fieldset>
            <legend class="text-text mb-1.5 text-base font-medium">{t('layout.label')}</legend>
            <div class="grid grid-cols-2 gap-2 sm:grid-cols-3">
              {LAYOUTS.map((l) => (
                <button
                  key={l.key}
                  type="button"
                  aria-pressed={layout === l.key}
                  onClick={() => setLayout(l.key)}
                  class={cx(
                    'border-border sytue-focus-ring grid gap-1 rounded-md border p-2.5 text-left transition-colors duration-150',
                    layout === l.key ? 'border-primary bg-primary/5' : 'hover:bg-bg-subtle',
                  )}
                >
                  <Glyph name={l.glyph} size={20} class="text-text-muted" />
                  <span class="text-text text-base font-medium">
                    {t(`layout.${l.key}` as never)}
                  </span>
                  <span class="sytue-meta">{t(`layout.${l.key}.hint` as never)}</span>
                </button>
              ))}
            </div>
          </fieldset>

          {puedeCambiarAcceso ? (
            <fieldset>
              <legend class="text-text mb-1.5 text-base font-medium">
                {t('visibility.label')}
              </legend>
              <div class="grid gap-1.5">
                {VISIBILIDADES.map((v) => (
                  <button
                    key={v.key}
                    type="button"
                    aria-pressed={visibility === v.key}
                    onClick={() => cambiarVisibilidad(v.key)}
                    class={cx(
                      'border-border sytue-focus-ring flex items-start gap-2.5 rounded-md border p-2.5 text-left transition-colors duration-150',
                      visibility === v.key ? 'border-primary bg-primary/5' : 'hover:bg-bg-subtle',
                    )}
                  >
                    <Glyph name={v.glyph} size={18} class="text-text-muted mt-0.5 shrink-0" />
                    <span class="min-w-0">
                      <span class="text-text block text-base font-medium">
                        {t(`visibility.${v.key}` as never)}
                      </span>
                      <span class="sytue-meta block">{t(`visibility.${v.key}.hint` as never)}</span>
                    </span>
                  </button>
                ))}
              </div>
            </fieldset>
          ) : null}

          {enlacePublico ? (
            <div class="border-border bg-bg-subtle flex items-center gap-2 rounded-md border p-2.5">
              <code class="text-text-muted min-w-0 flex-1 truncate text-sm">{enlacePublico}</code>
              <Button
                size="sm"
                variant="secondary"
                onClick={() => {
                  void navigator.clipboard.writeText(enlacePublico);
                  toast.notify({ tone: 'success', title: t('public.copied') });
                }}
              >
                {t('public.copyLink')}
              </Button>
            </div>
          ) : null}

          <div class="border-border grid gap-3 rounded-md border p-3">
            {/* La consecuencia va DEBAJO de cada interruptor, no en un tooltip:
                decidir si algo entra en el índice o se cifra sin poder leer qué
                implica es decidir a ciegas. */}
            <div class="grid gap-1">
              <Checkbox
                checked={encrypted ? false : indexable}
                disabled={encrypted}
                label={t('index.label')}
                onChange={(e) => setIndexable((e.target as HTMLInputElement).checked)}
              />
              <p class="sytue-meta pl-6">
                {encrypted ? t('index.lockedByEncryption') : t('index.hint')}
              </p>
            </div>
            {puedeCambiarAcceso ? (
              <>
                <div class="grid gap-1">
                  <Switch
                    checked={encrypted}
                    label={t('crypto.title')}
                    onChange={(e) => activarCifrado((e.target as HTMLInputElement).checked)}
                  />
                  <p class="sytue-meta pl-12">{t('crypto.hint')}</p>
                </div>
                {notebook?.encrypted && !encrypted ? (
                  <p class="text-warning-text text-base" role="alert">
                    {t('crypto.disableWarning')}
                  </p>
                ) : null}
              </>
            ) : null}
          </div>
        </div>
      </Modal>

      {pidiendoClave ? (
        <PassphraseDialog
          t={t}
          modo="crear"
          onClose={() => setPidiendoClave(false)}
          onKey={(_key, params) => {
            setCrypto(params);
            setEncrypted(true);
            setIndexable(false);
            setPidiendoClave(false);
          }}
        />
      ) : null}
    </>
  );
}
