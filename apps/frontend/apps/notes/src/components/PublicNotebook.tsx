/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * PublicNotebook — el cuaderno publicado, tal como lo ve quien llega por el
 * enlace. Sin sesión, sin shell, sin acciones: es una página de lectura.
 *
 * No reutiliza `NoteCanvas` porque ahí todo son manejadores —arrastrar, editar,
 * etiquetar, compartir— y aquí no hay ninguno. Traer ese componente con diez
 * props apagadas sería más código, no menos.
 */
import type { JSX } from 'preact';
import type { Locale } from '@sytue/i18n';
import { Icon } from '@sytue/ui';
import type { Note, Notebook } from '@sytue/api-clients';
import { useT, type T } from '../i18n/index.js';
import { conAnclas, enlacesAAnclas } from '../client/enlaces.js';

interface Props {
  locale: Locale;
  notebook: Notebook;
  notes: Note[];
}

export function PublicNotebook({ locale, notebook, notes }: Props): JSX.Element {
  const t = useT(locale);
  return (
    <main class="mx-auto grid max-w-3xl gap-4 px-4 py-8">
      <header class="grid gap-1">
        <h1 class="text-text flex items-center gap-2 text-2xl font-semibold">
          {notebook.emoji ? <span aria-hidden="true">{notebook.emoji}</span> : null}
          {notebook.title}
        </h1>
        <p class="sytue-meta">{t('public.readOnly')}</p>
      </header>

      <NotasSoloLectura t={t} notes={notes} />
    </main>
  );
}

/**
 * Las notas como LECTURA, sin manejadores. Lo comparten la página pública y la
 * vista previa de un `.onex` antes de importarlo: es el mismo «enséñame el
 * cuaderno sin dejarme tocarlo».
 *
 * SOLO `text` Y `todo` SE PINTAN COMO HTML. Son los dos tipos cuyo contenido el
 * servicio sanea al guardarlo (allowlist en services/notes/internal/domain/html.go)
 * y al previsualizarlo; el resto de tipos lleva texto plano, y texto plano que se
 * inyecta como HTML es la definición de un agujero. Antes se inyectaba todo.
 */
export function NotasSoloLectura({ t, notes }: { t: T; notes: Note[] }): JSX.Element {
  if (notes.length === 0) {
    return <p class="text-text-muted text-base">{t('empty.noNotes.title')}</p>;
  }
  // Los enlaces a notas de ESTA página se vuelven anclajes: así el cuaderno
  // publicado se lee como un wiki, sin sesión y sin salir de la página.
  const enPagina = new Set(notes.map((n) => n.id));
  return (
    <ul class="grid gap-3">
      {notes.map((n) => (
        <li
          key={n.id}
          id={`nota-${n.id}`}
          class="bg-surface border-border rounded-lg border p-4 shadow-sm"
        >
          {n.title ? <h2 class="text-text mb-1 text-base font-medium">{n.title}</h2> : null}
          {n.content_cipher ? (
            // Una nota cifrada se DICE, no se omite: quien mira tiene que ver que
            // ahí hay algo que no puede leer sin su clave.
            <p class="text-text-subtle inline-flex items-center gap-1 text-base">
              <Icon name="lock" size={14} /> {t('crypto.locked')}
            </p>
          ) : n.kind === 'text' || n.kind === 'todo' ? (
            <div
              class="sytue-prose text-text text-base leading-relaxed break-words"
              // eslint-disable-next-line react/no-danger
              dangerouslySetInnerHTML={{
                __html: enlacesAAnclas(conAnclas(n.content ?? ''), enPagina),
              }}
            />
          ) : n.content ? (
            <p class="text-text-muted text-base break-words">{n.content}</p>
          ) : null}
        </li>
      ))}
    </ul>
  );
}
