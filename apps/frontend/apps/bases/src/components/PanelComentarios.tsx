/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * El hilo de comentarios de un registro, en un panel lateral (el `Modal` del sistema en
 * su colocación `right`: atrapa el foco, Escape lo cierra y devuelve el foco a la fila).
 * Lee quien lee el registro; escribe el comentarista hacia arriba; borra quien lo
 * escribió o quien modera. Lo que se pinta sale de `puedeComentar` y de `can_delete`,
 * pero cada operación la vuelve a autorizar el servicio.
 *
 * El texto de un comentario es TEXTO: se pinta como nodo de texto, nunca como HTML.
 */
import type { JSX } from 'preact';
import { useCallback, useEffect, useState } from 'preact/hooks';
import { Button, FieldGroup, Modal, Skeleton, Textarea, useConfirm } from '@sytue/ui';
import { formatDateTime, type Locale } from '@sytue/i18n';
import type { BasesComment } from '@sytue/api-clients/bases';
import type { T } from '../i18n/index.js';
import { cliente, mensajeDeError } from '../lib/api.js';
import { MAX_COMENTARIO, hilosDe } from '../lib/comentarios.js';

export function PanelComentarios(p: {
  t: T;
  locale: Locale;
  registro: string;
  /** Cómo se llama el registro (su primer campo con valor). */
  nombre: string;
  puedeComentar: boolean;
  onCerrar: () => void;
}): JSX.Element {
  const { t } = p;
  const { confirmar, dialogo } = useConfirm();
  const [comentarios, setComentarios] = useState<BasesComment[] | null>(null);
  const [texto, setTexto] = useState('');
  const [respondiendo, setRespondiendo] = useState<BasesComment | null>(null);
  const [enviando, setEnviando] = useState(false);
  const [error, setError] = useState('');
  const [aviso, setAviso] = useState('');

  const cargar = useCallback(async () => {
    try {
      setComentarios(await cliente().listComments(p.registro));
      setError('');
    } catch (e) {
      setComentarios([]);
      setError(mensajeDeError(e, t));
    }
  }, [p.registro, t]);
  useEffect(() => {
    void cargar();
  }, [cargar]);

  async function enviar(e: Event): Promise<void> {
    e.preventDefault();
    const cuerpo = texto.trim();
    if (!cuerpo || enviando) return;
    setEnviando(true);
    setError('');
    try {
      await cliente().createComment(p.registro, cuerpo, respondiendo?.id);
      setTexto('');
      setRespondiendo(null);
      setAviso(t('comments.sent'));
      await cargar();
    } catch (err) {
      setError(mensajeDeError(err, t));
    } finally {
      setEnviando(false);
    }
  }

  async function borrar(c: BasesComment): Promise<void> {
    if (
      !(await confirmar({
        mensaje: t('comments.deleteConfirm'),
        confirmLabel: t('common.delete'),
        tone: 'danger',
      }))
    )
      return;
    try {
      await cliente().deleteComment(c.id);
      if (respondiendo?.id === c.id) setRespondiendo(null);
      setAviso(t('comments.deleted'));
      await cargar();
    } catch (err) {
      setError(mensajeDeError(err, t));
    }
  }

  const autor = (c: BasesComment) => c.author_name || t('comments.unknownAuthor');

  function comentario(c: BasesComment, esRaiz: boolean): JSX.Element {
    if (c.deleted) {
      return <p class="text-text-muted text-sm italic">{t('comments.deleted')}</p>;
    }
    return (
      <article class="flex flex-col gap-1">
        <header class="flex flex-wrap items-baseline gap-x-2">
          <span class="text-text text-sm font-semibold">{autor(c)}</span>
          <time class="text-text-muted text-xs" dateTime={c.created_at}>
            {formatDateTime(p.locale, c.created_at)}
          </time>
        </header>
        <p class="text-text text-sm break-words whitespace-pre-wrap">{c.body}</p>
        <div class="flex flex-wrap gap-1">
          {esRaiz && p.puedeComentar ? (
            <Button
              variant="ghost"
              size="sm"
              onClick={() => {
                setRespondiendo(c);
                document.getElementById('comentario-texto')?.focus();
              }}
            >
              {t('comments.reply')}
            </Button>
          ) : null}
          {c.can_delete ? (
            <Button variant="ghost" size="sm" onClick={() => void borrar(c)}>
              {t('comments.delete')}
            </Button>
          ) : null}
        </div>
      </article>
    );
  }

  return (
    <Modal
      open
      placement="right"
      onClose={p.onCerrar}
      title={t('comments.open', { name: p.nombre })}
    >
      <div class="flex flex-col gap-4">
        {comentarios === null ? (
          <Skeleton height="6rem" />
        ) : comentarios.length === 0 ? (
          <p class="text-text-muted text-sm">{t('comments.empty')}</p>
        ) : (
          <ul aria-label={t('comments.listLabel')} class="flex flex-col gap-3">
            {hilosDe(comentarios).map((h) => (
              <li key={h.raiz.id} class="border-border flex flex-col gap-2 rounded-md border p-3">
                {comentario(h.raiz, true)}
                {h.respuestas.length > 0 ? (
                  <ul class="border-border flex flex-col gap-2 border-l-2 pl-3">
                    {h.respuestas.map((r) => (
                      <li key={r.id}>{comentario(r, false)}</li>
                    ))}
                  </ul>
                ) : null}
              </li>
            ))}
          </ul>
        )}
        {error ? (
          <p role="alert" class="text-danger-text text-sm">
            {error}
          </p>
        ) : null}
        {p.puedeComentar ? (
          <form class="flex flex-col gap-2" onSubmit={(e) => void enviar(e)}>
            {respondiendo ? (
              <p class="text-text-muted flex flex-wrap items-center gap-2 text-xs">
                {t('comments.replyingTo', { name: autor(respondiendo) })}
                <Button variant="ghost" size="sm" onClick={() => setRespondiendo(null)}>
                  {t('comments.cancelReply')}
                </Button>
              </p>
            ) : null}
            <FieldGroup
              label={t('comments.write')}
              htmlFor="comentario-texto"
              help={t('comments.mentionHelp')}
            >
              <Textarea
                id="comentario-texto"
                aria-describedby="comentario-texto-help"
                rows={3}
                maxLength={MAX_COMENTARIO}
                value={texto}
                onInput={(e) => setTexto(e.currentTarget.value)}
              />
            </FieldGroup>
            <div>
              <Button type="submit" disabled={enviando || texto.trim() === ''}>
                {t('comments.send')}
              </Button>
            </div>
          </form>
        ) : (
          <p class="text-text-muted text-sm">{t('comments.readOnly')}</p>
        )}
        <p class="sr-only" role="status" aria-live="polite">
          {aviso}
        </p>
      </div>
      {dialogo}
    </Modal>
  );
}
