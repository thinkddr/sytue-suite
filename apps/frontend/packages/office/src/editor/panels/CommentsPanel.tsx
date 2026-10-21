/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Panel «Comentarios» (BLUEPRINT.md §6.4): hilos del contrato del auditor
 * (autor + fecha local + cuerpo), con responder, resolver/reabrir y eliminar.
 * Los comentarios anclados guardan `{ mark, text }`: el rango vive en el HTML
 * del folio como `<mark data-comment-id>` y al pasar el ratón (o enfocar) la
 * tarjeta se resalta en el lienzo (localizado por id de marca; texto citado de
 * respaldo). «Resolver» retira el `<mark>` (quita el resaltado, autosava).
 * Las respuestas solo llevan borrado (resolver es un estado del hilo raíz).
 */
import type { JSX } from 'preact';
import { useEffect, useMemo, useState } from 'preact/hooks';
import { fetchSession } from '@sytue/auth/client';
import { formatDateTime, type Locale } from '@sytue/i18n';
import { Button, IconButton, Textarea, useToast } from '@sytue/ui';
import { useMenciones } from './Menciones.js';
import { getEditorRoot, removeCommentMark } from '../canvas/exec';
import { EditorIcon } from '../icons';
import { AnclaDibujo } from '../dibujo/AnclaDibujo';
import type { CommentItem, EditorContext } from '../types';
import { findTextRanges, setEditorHighlight } from './SearchPanel';

const HL_ANCHOR = 'sytue-comment-anchor';

/** Texto citado del ancla, si el comentario trae una viable (cadena o `{ text }`). */
function anchorText(anchor: unknown): string | null {
  if (typeof anchor === 'string' && anchor.trim() !== '') return anchor;
  if (anchor !== null && typeof anchor === 'object') {
    const text = (anchor as { text?: unknown }).text;
    if (typeof text === 'string' && text.trim() !== '') return text;
  }
  return null;
}

/** Id de marca del ancla (`<mark data-comment-id>`), si lo trae. */
function anchorMark(anchor: unknown): string | null {
  if (anchor !== null && typeof anchor === 'object') {
    const mark = (anchor as { mark?: unknown }).mark;
    if (typeof mark === 'string' && mark !== '') return mark;
  }
  return null;
}

/** Fecha legible en el idioma activo; cadena vacía si el ISO no parsea. */
/** Quién lo escribió: su nombre del directorio, o el principio del `sub` si no se resolvió. */
export function autorDe(c: Pick<CommentItem, 'author_sub' | 'author_name'>): string {
  return c.author_name || c.author_sub.slice(0, 8);
}

function commentDate(locale: Locale, iso: string): string {
  const date = new Date(iso);
  return Number.isNaN(date.getTime()) ? '' : formatDateTime(locale, date);
}

/** Acción secundaria de una tarjeta de comentario. La geometría (24px, texto xs) la pone
 *  ya `Button size="xs"`; lo único propio es el color atenuado, que el `ghost` del DS no
 *  trae: aquí el texto compite con el del comentario y debe pesar menos. */
const TEXT_BTN = 'text-text-muted hover:text-text';
/** «Responder» abre la fila de acciones: con `-ml-2` el texto (no el botón) queda a la
 *  altura del nombre y del cuerpo del comentario, en vez de sangrado por el relleno del botón. */
const TEXT_BTN_INICIO = `${TEXT_BTN} -ml-2`;

export function CommentsPanel({ ctx }: { ctx: EditorContext }): JSX.Element {
  const t = ctx.t;
  const offline = ctx.connection === 'offline';
  const { notify } = useToast();
  const [draft, setDraft] = useState('');
  const [replyFor, setReplyFor] = useState<string | null>(null);
  const [replyDraft, setReplyDraft] = useState('');
  const mencionNueva = useMenciones(draft, setDraft, t('comments.mention.label'));
  const mencionRespuesta = useMenciones(replyDraft, setReplyDraft, t('comments.mention.label'));
  const [pending, setPending] = useState(false);
  /** Quién soy (`sub`): borrar es del autor y de quien edita; el servidor lo hace cumplir. */
  const [yo, setYo] = useState<string | null>(null);
  useEffect(() => {
    let vivo = true;
    void fetchSession().then((u) => {
      if (vivo && u) setYo(u.id);
    });
    return () => {
      vivo = false;
    };
  }, []);
  const puedeBorrar = (c: CommentItem): boolean => !ctx.soloLectura || c.author_sub === yo;
  /** Respuestas creadas aquí que aún no llegaron por ctx.comments (se depuran por id). */
  const [extras, setExtras] = useState<CommentItem[]>([]);

  const threads = useMemo(() => {
    const known = new Set(ctx.comments.map((c) => c.id));
    const all = [...ctx.comments, ...extras.filter((c) => !known.has(c.id))];
    const ids = new Set(all.map((c) => c.id));
    const roots: CommentItem[] = [];
    const replies = new Map<string, CommentItem[]>();
    for (const comment of all) {
      const parentId = comment.parent_id ?? null;
      if (parentId !== null && ids.has(parentId)) {
        const list = replies.get(parentId) ?? [];
        list.push(comment);
        replies.set(parentId, list);
      } else {
        roots.push(comment); // los huérfanos (padre borrado) se muestran como raíz
      }
    }
    const byDate = (a: CommentItem, b: CommentItem): number =>
      a.created_at.localeCompare(b.created_at);
    roots.sort(byDate);
    replies.forEach((list) => list.sort(byDate));
    return { roots, replies };
  }, [ctx.comments, extras]);

  /** Ejecuta una mutación con candado anti doble clic y toast de error (403 del contrato incluido). */
  const run = async (action: () => Promise<void>, errorTitle: string): Promise<void> => {
    setPending(true);
    try {
      await action();
    } catch {
      notify({ tone: 'danger', title: errorTitle });
    } finally {
      setPending(false);
    }
  };

  const submitComment = (): void => {
    const body = draft.trim();
    if (body === '') return;
    void run(async () => {
      await ctx.addComment(body);
      setDraft('');
    }, t('comments.addError'));
  };

  const submitReply = (parent: CommentItem): void => {
    const body = replyDraft.trim();
    if (body === '') return;
    // ponytail: EditorContext.addComment aún no acepta parent_id, así que la
    // respuesta entra por el proxy BFF same-origin de §4.2 y se fusiona en
    // `extras` hasta que ctx.comments la traiga; cuando el contrato crezca,
    // borrar este fetch y el estado `extras`.
    void run(async () => {
      const res = await fetch(`/api/comments?id=${encodeURIComponent(ctx.doc.id)}`, {
        method: 'POST',
        headers: { 'content-type': 'application/json' },
        body: JSON.stringify({ body, parent_id: parent.id, anchor: null }),
      });
      if (!res.ok) throw new Error(`HTTP ${res.status}`);
      const data = (await res.json()) as { comment?: CommentItem };
      const created = data.comment;
      if (created !== undefined) setExtras((prev) => [...prev, created]);
      setReplyDraft('');
      setReplyFor(null);
    }, t('comments.reply.error'));
  };

  const toggleResolved = (comment: CommentItem): void => {
    const next = !comment.resolved;
    void run(async () => {
      await ctx.resolveComment(comment.id, next);
      // Resolver quita el resaltado: se retira el <mark> del folio (autosava).
      // ponytail: reabrir no restaura el <mark> (el rango se perdió); re-anclar
      //           al reabrir pediría guardar el offset — de momento no compensa.
      const markId = anchorMark(comment.anchor);
      if (next && markId !== null) {
        hideAnchor();
        removeCommentMark(markId);
      }
    }, t('comments.resolveError'));
  };

  const remove = (comment: CommentItem): void => {
    const markId = anchorMark(comment.anchor);
    void run(async () => {
      await ctx.removeComment(comment.id);
      if (markId !== null) {
        hideAnchor();
        removeCommentMark(markId); // borrar el hilo también retira su resaltado
      }
    }, t('comments.deleteError'));
  };

  /** Rango del comentario en el lienzo: por id de marca; texto citado de respaldo. */
  const showAnchor = (comment: CommentItem): void => {
    const root = getEditorRoot();
    if (root === null) return;
    let range: Range | null = null;
    const markId = anchorMark(comment.anchor);
    if (markId !== null) {
      const el = root.querySelector<HTMLElement>(`mark[data-comment-id="${markId}"]`);
      if (el !== null) {
        range = document.createRange();
        range.selectNodeContents(el);
      }
    }
    if (range === null) {
      const text = anchorText(comment.anchor);
      if (text !== null) range = findTextRanges(root, text, 1)[0] ?? null;
    }
    setEditorHighlight(HL_ANCHOR, range === null ? [] : [range]);
    range?.startContainer.parentElement?.scrollIntoView({
      block: 'center',
      behavior: matchMedia('(prefers-reduced-motion: reduce)').matches ? 'auto' : 'smooth',
    });
  };
  const hideAnchor = (): void => {
    setEditorHighlight(HL_ANCHOR, []);
  };
  // Al desmontar el panel no queda ancla pintada en el lienzo.
  useEffect(() => hideAnchor, []);

  return (
    <div class="flex min-h-full flex-col gap-2 p-3">
      {/* Azul #BFDBFE de HIGHLIGHT_PALETTE y negro #0A0A0A de CONTENT_PALETTE (§5):
          colores de CONTENIDO sobre el papel, no tokens de UI. */}
      <style>{`::highlight(${HL_ANCHOR}) { background-color: #bfdbfe; color: #0a0a0a; }`}</style>

      {threads.roots.length === 0 ? (
        <p class="text-text-muted text-sm">{t('comments.empty')}</p>
      ) : (
        <ul class="flex flex-col gap-2" aria-label={t('comments.threads')}>
          {threads.roots.map((comment) => (
            <li key={comment.id}>
              {/* Hover que resalta el ancla del comentario en el documento: es un
                  refuerzo visual, no la vía de acceso —el hilo se abre con el botón
                  de dentro—. oxlint cuenta el hover como interacción; jsx-a11y no. */}
              {/* oxlint-disable-next-line jsx-a11y/no-noninteractive-element-interactions */}
              <article
                class={`border-border rounded-md border p-2 ${comment.resolved ? 'opacity-60' : ''}`}
                onMouseEnter={() => showAnchor(comment)}
                onMouseLeave={hideAnchor}
                onFocusIn={() => showAnchor(comment)}
                onFocusOut={hideAnchor}
              >
                <div class="flex items-baseline gap-2">
                  <span class="text-text text-xs font-semibold">{autorDe(comment)}</span>
                  <time class="sytue-meta-dense" dateTime={comment.created_at}>
                    {commentDate(ctx.locale, comment.created_at)}
                  </time>
                </div>
                {anchorText(comment.anchor) !== null && (
                  <p class="border-primary/50 text-text-muted mt-1 border-l-2 pl-2 text-xs italic">
                    «{anchorText(comment.anchor)}»
                  </p>
                )}
                <AnclaDibujo anchor={comment.anchor} />
                <p class="text-text mt-1 text-sm break-words whitespace-pre-wrap">{comment.body}</p>

                <div class="mt-1.5 flex items-center gap-1">
                  <Button
                    size="xs"
                    variant="ghost"
                    class={TEXT_BTN_INICIO}
                    disabled={pending || offline}
                    onClick={() => {
                      setReplyDraft('');
                      setReplyFor(replyFor === comment.id ? null : comment.id);
                    }}
                  >
                    {t('comments.reply')}
                  </Button>
                  <span class="flex-1" />
                  <Button
                    size="xs"
                    variant="ghost"
                    class={TEXT_BTN}
                    disabled={pending || offline}
                    title={
                      comment.resolved ? t('comments.reopen.title') : t('comments.resolve.title')
                    }
                    onClick={() => toggleResolved(comment)}
                  >
                    <EditorIcon name="check" size="sm" />
                    {comment.resolved ? t('comments.reopen') : t('comments.resolve')}
                  </Button>
                  {puedeBorrar(comment) ? (
                    <IconButton
                      size="sm"
                      tone="danger"
                      label={t('comments.delete')}
                      disabled={pending || offline}
                      onClick={() => remove(comment)}
                    >
                      <EditorIcon name="trash" size="sm" />
                    </IconButton>
                  ) : null}
                </div>

                {(threads.replies.get(comment.id) ?? []).map((reply) => (
                  <div key={reply.id} class="border-border mt-2 border-l-2 pl-2">
                    <div class="flex items-baseline gap-2">
                      <span class="text-text text-xs font-semibold">{autorDe(reply)}</span>
                      <time class="sytue-meta-dense" dateTime={reply.created_at}>
                        {commentDate(ctx.locale, reply.created_at)}
                      </time>
                      <span class="flex-1" />
                      {puedeBorrar(reply) ? (
                        <IconButton
                          size="sm"
                          tone="danger"
                          label={t('comments.deleteReply')}
                          disabled={pending || offline}
                          onClick={() => remove(reply)}
                        >
                          <EditorIcon name="trash" size="sm" />
                        </IconButton>
                      ) : null}
                    </div>
                    <p class="text-text mt-0.5 text-sm break-words whitespace-pre-wrap">
                      {reply.body}
                    </p>
                  </div>
                ))}

                {replyFor === comment.id && (
                  <form
                    class="mt-2 flex flex-col gap-1.5"
                    onSubmit={(event) => {
                      event.preventDefault();
                      submitReply(comment);
                    }}
                  >
                    <Textarea
                      rows={2}
                      placeholder={t('comments.reply.placeholder')}
                      aria-label={t('comments.reply.label')}
                      value={replyDraft}
                      disabled={offline}
                      onInput={(event) => setReplyDraft(event.currentTarget.value)}
                      {...mencionRespuesta.campo}
                    />
                    {mencionRespuesta.lista}
                    <div class="flex justify-end gap-1.5">
                      <Button
                        type="button"
                        size="sm"
                        variant="ghost"
                        onClick={() => {
                          setReplyFor(null);
                          setReplyDraft('');
                        }}
                      >
                        {t('action.cancel')}
                      </Button>
                      <Button
                        type="submit"
                        size="sm"
                        disabled={pending || offline || replyDraft.trim() === ''}
                      >
                        {t('comments.reply.send')}
                      </Button>
                    </div>
                  </form>
                )}
              </article>
            </li>
          ))}
        </ul>
      )}

      <form
        class="mt-auto flex flex-col gap-1.5 pt-1"
        onSubmit={(event) => {
          event.preventDefault();
          submitComment();
        }}
      >
        <Textarea
          rows={2}
          placeholder={t('comments.new.placeholder')}
          aria-label={t('comments.new.label')}
          value={draft}
          disabled={offline}
          onInput={(event) => setDraft(event.currentTarget.value)}
          {...mencionNueva.campo}
        />
        {mencionNueva.lista}
        <Button
          type="submit"
          size="sm"
          class="self-end"
          disabled={pending || offline || draft.trim() === ''}
        >
          {t('comments.new.send')}
        </Button>
      </form>
    </div>
  );
}
