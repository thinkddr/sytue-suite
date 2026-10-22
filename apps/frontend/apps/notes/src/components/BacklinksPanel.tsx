/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * BacklinksPanel — QUÉ NOTAS APUNTAN A ESTA.
 *
 * La referencia entre notas ya existía, pero solo se veía desde el lado que la
 * creó: al abrir la nota apuntada no había forma de saber quién la señalaba. Un
 * enlace que solo se ve desde un extremo es una nota suelta con una flecha, no
 * una red — y la red es justo lo que Sytue Notepads hereda de BasKet Note Pads.
 *
 * Se abre desde el menú de la nota y NO al pintar el lienzo: una pared de
 * cuarenta notas serían cuarenta peticiones para enseñar, casi siempre, nada.
 */
import type { JSX } from 'preact';
import { useEffect, useState } from 'preact/hooks';
import { Modal, Spinner } from '@sytue/ui';
import type { Note } from '@sytue/api-clients';
import type { T } from '../i18n/index.js';
import * as api from '../client/notesApi.js';
import { Glyph } from './glyphs.js';

export function BacklinksPanel({
  t,
  note,
  onClose,
}: {
  t: T;
  note: Note;
  onClose: () => void;
}): JSX.Element {
  const [estado, setEstado] = useState<'cargando' | 'listo' | 'error'>('cargando');
  const [notas, setNotas] = useState<Note[]>([]);

  useEffect(() => {
    // `vivo` corta el `setState` de una respuesta que llega después de cerrar:
    // sin él, abrir y cerrar rápido escribe en un componente desmontado.
    let vivo = true;
    setEstado('cargando');
    api
      .retroenlaces(note.id)
      .then((r) => {
        if (!vivo) return;
        setNotas(r.notes ?? []);
        setEstado('listo');
      })
      .catch(() => vivo && setEstado('error'));
    return () => {
      vivo = false;
    };
  }, [note.id]);

  return (
    <Modal open onClose={onClose} title={t('backlinks.title')} size="sm">
      {/* De QUÉ nota, escrito: el panel se abre desde el menú de una tarjeta y,
          una vez abierto, el modal tapa justo la tarjeta de la que salió. */}
      <p class="sytue-meta mb-2">{t('backlinks.of', { title: note.title || t('note.new') })}</p>
      {estado === 'cargando' ? (
        <div class="grid place-items-center py-8">
          <Spinner label={t('backlinks.title')} />
        </div>
      ) : estado === 'error' ? (
        <p class="text-danger-text text-base" role="alert">
          {t('error.loadFailed')}
        </p>
      ) : notas.length === 0 ? (
        // El vacío se EXPLICA. «Ninguna» a secas se lee como un fallo; lo que
        // hace falta saber es que esto se llena solo, enlazando desde otra nota.
        <div class="grid gap-1.5 py-4">
          <p class="text-text text-base">{t('backlinks.empty')}</p>
          <p class="sytue-meta">{t('backlinks.emptyHint')}</p>
        </div>
      ) : (
        <ul class="grid gap-1.5">
          {notas.map((n) => (
            <li key={n.id}>
              {/* Un `<a>` de verdad y no un `onClick`: es la MISMA navegación que
                  usan los resultados de búsqueda, así que se abre en otra pestaña
                  con el botón central y se copia con el derecho, gratis. */}
              <a
                href={`?notebook=${encodeURIComponent(n.notebook_id)}&note=${encodeURIComponent(n.id)}`}
                class="border-border hover:bg-bg-subtle sytue-focus-ring flex items-start gap-2 rounded-md border px-3 py-2"
              >
                <Glyph name="link" size={15} class="text-text-subtle mt-0.5 shrink-0" />
                <span class="min-w-0">
                  <span class="text-text block text-base font-medium break-words">
                    {n.title || t('note.new')}
                  </span>
                  {/* Se enseña la nota que APUNTA, no el texto del enlace: quien
                      mira quiere saber desde dónde le están señalando. */}
                  <span class="sytue-meta block">
                    {n.content_cipher ? t('crypto.locked') : t('backlinks.from')}
                  </span>
                </span>
              </a>
            </li>
          ))}
        </ul>
      )}
    </Modal>
  );
}
