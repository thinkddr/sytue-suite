/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Comments — los comentarios bajo un vídeo, en hilos de un nivel.
 *
 * Sin sesión se leen (los de un vídeo público o no listado) y se invita a entrar
 * para escribir. Borrar lo pueden el autor, el dueño del vídeo y quien administra
 * la organización: lo dice el servicio en `can.delete` de cada comentario, y aquí
 * solo se enseña el botón a quien lo tiene (la verja está en el servicio).
 *
 * Lo que no es tuyo se puede denunciar (también sin sesión) y, con sesión, bloquear a
 * quien lo escribió: sus comentarios dejan de verse en el acto.
 */
import type { JSX } from 'preact';
import { useEffect, useId, useState } from 'preact/hooks';
import { Avatar, Button, Textarea, useConfirm, useToast } from '@sytue/ui';
import { formatTimeAgo, translate, type Locale } from '@sytue/i18n';
import type { T } from '../i18n/index.js';
import { borrarComentario, comentar, comentarios } from '../client/castApi.js';
import { enHilos } from '../lib/comentarios.js';
import { FalloDelServicio } from '../lib/fallo.js';
import type { Comentario } from '../lib/tipos.js';
import type { Moderacion } from './Moderacion.js';

const MAX_CUERPO = 2000;

export interface CommentsProps {
  t: T;
  locale: Locale;
  videoId: string;
  conSesion: boolean;
  activos: boolean;
  entrar: () => void;
  miSub: string | null;
  moderacion: Moderacion;
}

export function Comments({
  t,
  locale,
  videoId,
  conSesion,
  activos,
  entrar,
  miSub,
  moderacion,
}: CommentsProps): JSX.Element {
  const [filas, setFilas] = useState<Comentario[] | null>(null);
  const [cursor, setCursor] = useState<string | undefined>(undefined);
  const [fallo, setFallo] = useState(false);
  const [respondiendo, setRespondiendo] = useState<string | null>(null);
  const toast = useToast();
  const { confirmar, dialogo } = useConfirm();

  const cargar = async (desde?: string): Promise<void> => {
    setFallo(false);
    try {
      const r = await comentarios(videoId, conSesion, desde);
      setFilas((prev) => (desde && prev ? [...prev, ...r.comments] : r.comments));
      setCursor(r.next_cursor || undefined);
    } catch {
      setFallo(true);
    }
  };
  useEffect(() => {
    void cargar();
    // oxlint-disable-next-line react-hooks/exhaustive-deps
  }, [videoId, conSesion]);

  const publicar = async (cuerpo: string, replyTo?: string): Promise<boolean> => {
    try {
      const nuevo = await comentar(videoId, cuerpo, replyTo);
      setFilas((prev) => [...(prev ?? []), nuevo]);
      setRespondiendo(null);
      return true;
    } catch (e) {
      const bloqueado = e instanceof FalloDelServicio && e.code === 'blocked';
      toast.notify({
        tone: 'danger',
        title: bloqueado ? t('block.blockedYou') : t('comments.sendError'),
      });
      return false;
    }
  };

  const retirar = async (c: Comentario): Promise<void> => {
    const ok = await confirmar({
      titulo: t('comments.deleteTitle'),
      mensaje: t('comments.deleteConfirm'),
      confirmLabel: t('comments.delete'),
      tone: 'danger',
    });
    if (!ok) return;
    try {
      await borrarComentario(videoId, c.id);
      // El servicio conserva el hueco si tiene respuestas; aquí igual.
      setFilas((prev) =>
        (prev ?? []).map((x) =>
          x.id === c.id ? { ...x, deleted: true, body: '', can: { delete: false } } : x,
        ),
      );
    } catch {
      toast.notify({ tone: 'danger', title: t('errors.title') });
    }
  };

  // Sin lo de quien se acaba de bloquear ni las respuestas a sus comentarios: lo
  // mismo que devolverá el servicio en la próxima carga.
  const visibles = filas
    ? enHilos(filas)
        .filter((h) => !moderacion.bloqueados.has(h.raiz.author.sub))
        .map((h) => ({
          ...h,
          respuestas: h.respuestas.filter((r) => !moderacion.bloqueados.has(r.author.sub)),
        }))
    : [];
  const hilos = visibles;
  const total = hilos.reduce(
    (n, h) => n + (h.raiz.deleted ? 0 : 1) + h.respuestas.filter((r) => !r.deleted).length,
    0,
  );
  const acciones = (c: Comentario): AccionesDeFila => ({
    retirar,
    denunciar:
      c.author.sub === miSub
        ? undefined
        : () => moderacion.denunciar({ kind: 'comment', videoId, target: c.id }),
    bloquear:
      !conSesion || c.author.sub === miSub
        ? undefined
        : () => void moderacion.bloquear(videoId, c.author.sub, c.author.name),
  });

  return (
    <section aria-labelledby="comentarios-titulo" class="flex flex-col gap-4">
      <h2 id="comentarios-titulo" class="text-text text-base font-semibold">
        {filas && total > 0 ? t('comments.count', { count: total }) : t('comments.title')}
      </h2>

      {!activos ? (
        <p class="text-text-muted text-sm">{t('comments.disabled')}</p>
      ) : conSesion ? (
        <Redactor
          t={t}
          locale={locale}
          placeholder={t('comments.placeholder')}
          enviar={(c) => publicar(c)}
        />
      ) : (
        <div class="border-border flex flex-wrap items-center justify-between gap-3 rounded-md border px-4 py-3">
          <p class="text-text-muted text-sm">{t('comments.signIn')}</p>
          <Button size="sm" variant="secondary" onClick={entrar}>
            {t('nav.signIn')}
          </Button>
        </div>
      )}

      {fallo ? (
        <div class="flex items-center gap-3">
          <p class="text-danger-text text-sm" role="alert">
            {t('comments.loadError')}
          </p>
          <Button size="sm" variant="ghost" onClick={() => void cargar()}>
            {t('errors.retry')}
          </Button>
        </div>
      ) : filas === null ? null : hilos.length === 0 && activos ? (
        <p class="text-text-subtle text-sm">{t('comments.empty')}</p>
      ) : (
        <ol class="flex flex-col gap-5">
          {hilos.map((h) => (
            <li key={h.raiz.id} class="flex flex-col gap-3">
              <Fila
                t={t}
                locale={locale}
                c={h.raiz}
                responder={conSesion && activos ? () => setRespondiendo(h.raiz.id) : undefined}
                {...acciones(h.raiz)}
              />
              {h.respuestas.length || respondiendo === h.raiz.id ? (
                <ol class="border-border ml-5 flex flex-col gap-3 border-l pl-4">
                  {h.respuestas.map((r) => (
                    <li key={r.id}>
                      <Fila t={t} locale={locale} c={r} {...acciones(r)} />
                    </li>
                  ))}
                  {respondiendo === h.raiz.id ? (
                    <li>
                      <Redactor
                        t={t}
                        locale={locale}
                        enfocar
                        placeholder={t('comments.replyPlaceholder', { name: h.raiz.author.name })}
                        enviar={(c) => publicar(c, h.raiz.id)}
                        cancelar={() => setRespondiendo(null)}
                      />
                    </li>
                  ) : null}
                </ol>
              ) : null}
            </li>
          ))}
        </ol>
      )}

      {cursor ? (
        <Button variant="ghost" size="sm" onClick={() => void cargar(cursor)} class="self-start">
          {t('comments.more')}
        </Button>
      ) : null}
      {dialogo}
    </section>
  );
}

interface AccionesDeFila {
  retirar: (c: Comentario) => Promise<void>;
  denunciar: (() => void) | undefined;
  bloquear: (() => void) | undefined;
}

function Fila({
  t,
  locale,
  c,
  responder,
  retirar,
  denunciar,
  bloquear,
}: AccionesDeFila & {
  t: T;
  locale: Locale;
  c: Comentario;
  responder?: (() => void) | undefined;
}): JSX.Element {
  if (c.deleted) {
    return <p class="text-text-subtle text-sm italic">{t('comments.deleted')}</p>;
  }
  return (
    <article class="flex gap-3">
      <Avatar name={c.author.name} size="sm" />
      <div class="min-w-0 flex-1">
        <p class="text-xs">
          <span class="text-text font-semibold">{c.author.name}</span>
          {c.author.username ? <span class="text-text-subtle"> @{c.author.username}</span> : null}
          <span class="text-text-subtle"> · </span>
          <time class="text-text-subtle" dateTime={c.created_at}>
            {formatTimeAgo(locale, c.created_at)}
          </time>
        </p>
        <p class="text-text mt-0.5 text-sm break-words whitespace-pre-wrap">{c.body}</p>
        <div class="mt-1 flex gap-1">
          {responder ? (
            <Button size="xs" variant="ghost" onClick={responder}>
              {t('comments.reply')}
            </Button>
          ) : null}
          {c.can.delete ? (
            <Button size="xs" variant="ghost" onClick={() => void retirar(c)}>
              {t('comments.delete')}
            </Button>
          ) : null}
          {denunciar ? (
            <Button size="xs" variant="ghost" onClick={denunciar}>
              {t('report.action')}
            </Button>
          ) : null}
          {bloquear ? (
            <Button size="xs" variant="ghost" onClick={bloquear}>
              {t('block.action', { name: c.author.name })}
            </Button>
          ) : null}
        </div>
      </div>
    </article>
  );
}

function Redactor({
  t,
  locale,
  placeholder,
  enviar,
  cancelar,
  enfocar = false,
}: {
  t: T;
  locale: Locale;
  placeholder: string;
  enviar: (cuerpo: string) => Promise<boolean>;
  cancelar?: () => void;
  /** Se abre al pulsar «Responder»: el foco va al campo, que es lo que se pidió. */
  enfocar?: boolean;
}): JSX.Element {
  const id = useId();
  // Por id y no por ref: `Textarea` de @sytue/ui no reenvía la ref.
  useEffect(() => {
    if (enfocar) document.getElementById(id)?.focus();
  }, [enfocar, id]);
  const [texto, setTexto] = useState('');
  const [enviando, setEnviando] = useState(false);
  const alEnviar = async (e: Event): Promise<void> => {
    e.preventDefault();
    const cuerpo = texto.trim();
    if (!cuerpo) return;
    setEnviando(true);
    if (await enviar(cuerpo)) setTexto('');
    setEnviando(false);
  };
  return (
    <form onSubmit={(e) => void alEnviar(e)} class="flex flex-col gap-2">
      <label for={id} class="sr-only">
        {placeholder}
      </label>
      <Textarea
        id={id}
        rows={2}
        value={texto}
        maxLength={MAX_CUERPO}
        placeholder={placeholder}
        onInput={(e) => setTexto(e.currentTarget.value)}
      />
      <div class="flex justify-end gap-2">
        {cancelar ? (
          <Button type="button" variant="ghost" size="sm" onClick={cancelar}>
            {translate(locale, 'ui.cancel')}
          </Button>
        ) : null}
        <Button
          type="submit"
          variant="primary"
          size="sm"
          loading={enviando}
          disabled={!texto.trim()}
        >
          {t('comments.send')}
        </Button>
      </div>
    </form>
  );
}
