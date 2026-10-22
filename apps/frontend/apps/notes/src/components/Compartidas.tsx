/**
 * «Compartido conmigo»: las notas sueltas que otras personas han compartido conmigo.
 * Los cuadernos compartidos no van aquí: salen en el árbol como cualquier otro.
 *
 * La lista con la misma fila que la papelera; la nota abierta, con su tarjeta de
 * siempre. Con «puede ver», en solo lectura; con «puede editar», se cambia su título y
 * su contenido y nada más —sin el menú «Más», porque fijarla, moverla o borrarla es
 * de su dueña (el servicio tampoco lo deja)—.
 */
import type { JSX } from 'preact';
import { Badge, Button, Icon, Spinner } from '@sytue/ui';
import { translate, type Locale } from '@sytue/i18n';
import { resumenNota } from '../client/diario.js';
import type { SharedNote } from '../client/notesApi.js';
import type { useT } from '../i18n/index.js';
import { Glyph } from './glyphs.js';
import { NoteCard } from './NoteCard.js';

const nada = (): void => undefined;

export function PanelCompartidas({
  t,
  locale,
  notas,
  abierta,
  onAbrir,
  onCambio,
}: {
  t: ReturnType<typeof useT>;
  locale: Locale;
  /** null mientras carga. */
  notas: SharedNote[] | null;
  abierta: string | null;
  onAbrir: (id: string | null) => void;
  onCambio: (
    nota: SharedNote,
    patch: { title?: string; content?: string; checked?: boolean },
  ) => void;
}): JSX.Element {
  const tc = (k: Parameters<typeof translate>[1], v?: Record<string, string>) =>
    translate(locale, k, v);
  const quien = (n: SharedNote) =>
    tc('sharedWithMe.by', { name: n.shared_by_name ?? tc('sharePeople.someone') });
  const nivel = (n: SharedNote) =>
    tc(n.permission === 'write' ? 'sharePeople.write' : 'sharePeople.read');

  if (notas === null) {
    return (
      <div class="grid h-full place-items-center p-6">
        <Spinner />
      </div>
    );
  }
  const nota = notas.find((n) => n.id === abierta);
  if (nota) {
    return (
      <div class="notes-col notes-page">
        <div class="mb-3 -ml-2 flex flex-wrap items-center gap-2">
          <Button
            size="sm"
            variant="ghost"
            leadingIcon={<Icon name="chevronLeft" size={16} />}
            onClick={() => onAbrir(null)}
          >
            {tc('sharedWithMe.title')}
          </Button>
          <span class="text-text-muted ml-auto text-sm">{quien(nota)}</span>
          {nota.permission === 'write' ? null : <Badge>{tc('sharedWithMe.readOnly')}</Badge>}
        </div>
        <NoteCard
          t={t}
          note={nota}
          seleccionada
          llenar
          {...(nota.permission === 'write' ? { sinMenu: true } : { soloLectura: true })}
          onSelect={nada}
          onChange={(patch) => onCambio(nota, patch)}
          onDelete={nada}
          onDuplicate={nada}
          onTogglePin={nada}
          onShare={nada}
          onTags={nada}
          onBacklinks={nada}
        />
      </div>
    );
  }
  return (
    <div class="notes-col notes-page">
      <div class="notes-page-head">
        <h2 class="notes-page-title">{tc('sharedWithMe.title')}</h2>
      </div>
      {notas.length === 0 ? (
        <p class="text-text-subtle text-base">{tc('sharedWithMe.empty')}</p>
      ) : (
        <ul class="notes-rows">
          {notas.map((n) => (
            <li key={n.id}>
              <button
                type="button"
                onClick={() => onAbrir(n.id)}
                class="notes-row sytue-focus-ring w-full text-left"
              >
                <span class="notes-stream-glyph">
                  {n.kind === 'todo' ? (
                    <Glyph name="todo" size={14} />
                  ) : (
                    <span class="notes-stream-bullet" aria-hidden="true" />
                  )}
                </span>
                <span class="flex min-w-0 flex-1 flex-col">
                  <span class="text-text truncate text-base">
                    {n.title || resumenNota(n) || t('note.untitled')}
                  </span>
                  <span class="text-text-subtle truncate text-xs">
                    {quien(n)} · {nivel(n)}
                  </span>
                </span>
              </button>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}
