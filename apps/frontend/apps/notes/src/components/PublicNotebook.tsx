/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * PublicNotebook — el cuaderno publicado, tal como lo ve quien llega por el
 * enlace. Sin sesión y sin shell: es una página de lectura; si el cuaderno
 * está cifrado, permite desbloquearlo en memoria del navegador con su frase de
 * paso sin enviar nada por la red.
 *
 * No reutiliza `NoteCanvas` porque ahí todo son manejadores —arrastrar, editar,
 * etiquetar, compartir— y aquí no hay edición. Traer ese componente con diez
 * props apagadas sería más código, no menos.
 */
import type { JSX } from 'preact';
import { useEffect, useRef, useState } from 'preact/hooks';
import type { Locale } from '@sytue/i18n';
import { Button, Icon } from '@sytue/ui';
import type { Note, Notebook } from '@sytue/api-clients';
import { useT, type T } from '../i18n/index.js';
import { conAnclas, enlacesAAnclas } from '../client/enlaces.js';
import { textoDeNota } from '../client/extracto.js';
import { abrirContenidoNota } from '../client/notesEncrypted.js';
import { PassphraseDialog } from './PassphraseDialog.js';

interface Props {
  locale: Locale;
  notebook: Notebook;
  notes: Note[];
}

export function PublicNotebook({ locale, notebook, notes }: Props): JSX.Element {
  const t = useT(locale);
  const [pidiendoClave, setPidiendoClave] = useState(false);
  const [notasAbiertas, setNotasAbiertas] = useState<Note[] | null>(null);
  const generacionRef = useRef(0);

  const tieneCifrado = Boolean(
    notebook.crypto && (notebook.encrypted || notes.some((n) => Boolean(n.content_cipher))),
  );
  const bloqueado = notasAbiertas === null;

  const bloquear = () => {
    generacionRef.current += 1;
    setPidiendoClave(false);
    setNotasAbiertas(null);
  };

  const previoRef = useRef({ id: notebook.id, check: notebook.crypto?.check });

  useEffect(() => {
    if (
      previoRef.current.id === notebook.id &&
      previoRef.current.check === notebook.crypto?.check
    ) {
      return;
    }
    previoRef.current = { id: notebook.id, check: notebook.crypto?.check };
    generacionRef.current += 1;
    setPidiendoClave(false);
    setNotasAbiertas(null);
  }, [notebook.id, notebook.crypto?.check]);

  useEffect(() => {
    if (typeof window === 'undefined') return undefined;
    const alOcultar = () => {
      generacionRef.current += 1;
      setNotasAbiertas(null);
    };
    window.addEventListener('pagehide', alOcultar);
    return () => window.removeEventListener('pagehide', alOcultar);
  }, []);

  const desbloquear = async (key: CryptoKey) => {
    const gen = ++generacionRef.current;
    const abiertas = await Promise.all(
      notes.map(async (n): Promise<Note> => {
        if (!n.content_cipher) return n;
        try {
          const { claro } = await abrirContenidoNota(key, n, true);
          const limpia: Note = {
            ...n,
            title: claro.title,
            content: claro.content,
            data: claro.data ?? {},
          };
          delete limpia.content_cipher;
          return limpia;
        } catch {
          return n;
        }
      }),
    );
    if (generacionRef.current !== gen) return;
    setNotasAbiertas(abiertas);
    setPidiendoClave(false);
  };

  return (
    <main class="mx-auto grid max-w-3xl gap-4 px-4 py-8">
      <header class="flex flex-wrap items-start justify-between gap-3">
        <div class="grid min-w-0 gap-1">
          <h1 class="text-text flex items-center gap-2 text-2xl font-semibold">
            {notebook.emoji ? <span aria-hidden="true">{notebook.emoji}</span> : null}
            <span class="break-words">{notebook.title}</span>
          </h1>
          <p class="sytue-meta">{t('public.readOnly')}</p>
        </div>

        {tieneCifrado ? (
          <Button
            size="sm"
            variant="ghost"
            onClick={bloqueado ? () => setPidiendoClave(true) : bloquear}
            leadingIcon={<Icon name="lock" size={14} />}
          >
            {bloqueado ? t('crypto.unlock') : t('crypto.lockNow')}
          </Button>
        ) : null}
      </header>

      <NotasSoloLectura t={t} notes={notasAbiertas ?? notes} />

      {pidiendoClave && notebook.crypto ? (
        <PassphraseDialog
          t={t}
          modo="abrir"
          params={notebook.crypto}
          onClose={() => setPidiendoClave(false)}
          onKey={(key) => {
            void desbloquear(key);
          }}
        />
      ) : null}
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
      {notes.map((n) => {
        const textoCuerpo =
          n.kind === 'text' || n.kind === 'todo' ? '' : textoDeNota(n.content ?? '');
        return (
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
            ) : textoCuerpo ? (
              <p class="text-text-muted text-base break-words">{textoCuerpo}</p>
            ) : null}
          </li>
        );
      })}
    </ul>
  );
}
