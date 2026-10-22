/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * ChatPanel — el chat del directo.
 *
 * El historial llega por REST (lo que se dijo antes de entrar) y lo nuevo por el
 * WebSocket del servicio, que el navegador abre DIRECTO contra cast.<dominio>/ws
 * con el token de acceso en la query (`getAccessToken`, que lo pide al BFF): un
 * WebSocket no lleva cabeceras propias y el proxy del BFF no sirve para uno.
 *
 * El socket se reabre solo tras un corte, con espera creciente (1, 2, 4… 30 s) y un
 * token nuevo en cada intento. El desplazamiento baja solo si se estaba abajo: quien
 * sube a leer algo no quiere que cada mensaje nuevo se lo arranque de la pantalla.
 */
import type { JSX } from 'preact';
import { useEffect, useLayoutEffect, useRef, useState } from 'preact/hooks';
import { Avatar, Button, Dropdown, Icon, IconButton, Input, MenuItem, useToast } from '@sytue/ui';
import { formatTime, type Locale } from '@sytue/i18n';
import { getAccessToken } from '@sytue/auth/client';
import type { T } from '../i18n/index.js';
import { borrarMensaje, historialChat } from '../client/castApi.js';
import { aplicar, leerTrama, unir, urlDelChat } from '../lib/chat.js';
import type { MensajeChat } from '../lib/tipos.js';
import type { Moderacion } from './Moderacion.js';

export interface ChatPanelProps {
  t: T;
  locale: Locale;
  videoId: string;
  wsBase: string;
  conSesion: boolean;
  activo: boolean;
  puedeModerar: boolean;
  miSub: string | null;
  alEspectadores: (n: number) => void;
  entrar: () => void;
  moderacion: Moderacion;
}

const MAX_CUERPO = 500;

export function ChatPanel(p: ChatPanelProps): JSX.Element {
  const { t } = p;
  return (
    <section
      aria-label={t('chat.title')}
      class="border-border bg-surface flex min-h-0 flex-col rounded-md border lg:h-full"
    >
      <h2 class="border-border text-text border-b px-4 py-3 text-sm font-semibold">
        {t('chat.title')}
      </h2>
      {!p.activo ? (
        <p class="text-text-muted px-4 py-6 text-sm">{t('chat.disabled')}</p>
      ) : !p.conSesion ? (
        <div class="flex flex-col items-start gap-3 px-4 py-6">
          <p class="text-text-muted text-sm">{t('chat.signIn')}</p>
          <Button size="sm" variant="primary" onClick={p.entrar}>
            {t('nav.signIn')}
          </Button>
        </div>
      ) : (
        <Conversacion {...p} />
      )}
    </section>
  );
}

function Conversacion({
  t,
  locale,
  videoId,
  wsBase,
  puedeModerar,
  miSub,
  alEspectadores,
  moderacion,
}: ChatPanelProps): JSX.Element {
  const [mensajes, setMensajes] = useState<readonly MensajeChat[]>([]);
  const [conectado, setConectado] = useState(false);
  const [texto, setTexto] = useState('');
  const [aviso, setAviso] = useState<string | null>(null);
  const socket = useRef<WebSocket | null>(null);
  const lista = useRef<HTMLOListElement>(null);
  const abajo = useRef(true);
  const toast = useToast();

  useEffect(() => {
    let vivo = true;
    let intento = 0;
    let espera: ReturnType<typeof setTimeout> | null = null;

    void historialChat(videoId)
      .then((h) => vivo && setMensajes((vivos) => unir(h, vivos)))
      .catch(() => undefined);

    const abrir = async (): Promise<void> => {
      let token: string;
      try {
        token = await getAccessToken();
      } catch {
        return;
      }
      if (!vivo) return;
      const ws = new WebSocket(urlDelChat(wsBase, videoId, token));
      socket.current = ws;
      ws.onopen = () => {
        intento = 0;
        setConectado(true);
      };
      ws.onmessage = (e) => {
        const trama = leerTrama(e.data);
        if (!trama) return;
        if (trama.type === 'viewers') alEspectadores(trama.count);
        else if (trama.type === 'error') {
          setAviso(
            trama.code === 'too_many_requests'
              ? t('chat.tooFast')
              : trama.code === 'blocked'
                ? t('block.blockedYou')
                : t('chat.sendError'),
          );
        } else setMensajes((l) => aplicar(l, trama));
      };
      ws.onclose = () => {
        setConectado(false);
        if (!vivo) return;
        const ms = Math.min(30_000, 1000 * 2 ** intento++);
        espera = setTimeout(() => void abrir(), ms);
      };
    };
    void abrir();
    return () => {
      vivo = false;
      if (espera) clearTimeout(espera);
      socket.current?.close();
      socket.current = null;
    };
    // `alEspectadores` y `t` no reabren el socket.
    // oxlint-disable-next-line react-hooks/exhaustive-deps
  }, [videoId, wsBase]);

  // Abajo del todo si se estaba abajo.
  useLayoutEffect(() => {
    const el = lista.current;
    if (el && abajo.current) el.scrollTop = el.scrollHeight;
  }, [mensajes]);

  const enviar = (e: Event): void => {
    e.preventDefault();
    const cuerpo = texto.trim();
    if (!cuerpo || socket.current?.readyState !== WebSocket.OPEN) return;
    socket.current.send(JSON.stringify({ type: 'send', body: cuerpo }));
    setTexto('');
    setAviso(null);
    abajo.current = true;
  };

  const retirar = async (id: string): Promise<void> => {
    try {
      await borrarMensaje(videoId, id);
      setMensajes((l) => aplicar(l, { type: 'deleted', id }));
    } catch {
      toast.notify({ tone: 'danger', title: t('errors.title') });
    }
  };

  // El servicio deja de mandar lo de quien se bloquea; lo que ya estaba en pantalla
  // se quita aquí.
  const visibles = mensajes.filter((m) => !moderacion.bloqueados.has(m.author.sub));

  return (
    <>
      {conectado ? null : (
        <p class="bg-warning-bg text-warning-text px-4 py-2 text-xs" role="status">
          {t('chat.reconnecting')}
        </p>
      )}
      <ol
        ref={lista}
        aria-live="polite"
        aria-relevant="additions"
        onScroll={(e) => {
          const el = e.currentTarget;
          abajo.current = el.scrollHeight - el.scrollTop - el.clientHeight < 48;
        }}
        class="flex max-h-96 min-h-48 flex-1 flex-col gap-3 overflow-y-auto px-4 py-3 lg:max-h-none"
      >
        {visibles.length === 0 ? (
          <li class="text-text-subtle my-auto text-center text-sm">{t('chat.empty')}</li>
        ) : (
          visibles.map((m) => (
            <li key={m.id} class="group flex items-start gap-2">
              <Avatar name={m.author.name} size="xs" />
              <div class="min-w-0 flex-1 text-sm leading-snug">
                <span class="text-text font-semibold">{m.author.name}</span>{' '}
                <time class="text-text-subtle text-xs" dateTime={m.created_at}>
                  {formatTime(locale, m.created_at)}
                </time>
                <p class="text-text break-words whitespace-pre-wrap">{m.body}</p>
              </div>
              {puedeModerar || m.author.sub === miSub ? (
                <IconButton
                  label={t('chat.delete')}
                  size="sm"
                  class="opacity-0 group-focus-within:opacity-100 group-hover:opacity-100"
                  onClick={() => void retirar(m.id)}
                >
                  <Icon name="trash" size={14} />
                </IconButton>
              ) : null}
              {m.author.sub === miSub ? null : (
                <Dropdown
                  align="end"
                  label={t('chat.more', { name: m.author.name })}
                  class="opacity-0 group-focus-within:opacity-100 group-hover:opacity-100 has-[[aria-expanded=true]]:opacity-100"
                  trigger={(props) => (
                    <IconButton
                      label={t('chat.more', { name: m.author.name })}
                      size="sm"
                      {...props}
                    >
                      <Icon name="ellipsis" size={14} />
                    </IconButton>
                  )}
                >
                  {(cerrar) => (
                    <>
                      <MenuItem
                        size="sm"
                        onClick={() => {
                          cerrar();
                          moderacion.denunciar({ kind: 'chat', videoId, target: m.id });
                        }}
                      >
                        {t('report.action')}
                      </MenuItem>
                      <MenuItem
                        size="sm"
                        danger
                        onClick={() => {
                          cerrar();
                          void moderacion.bloquear(videoId, m.author.sub, m.author.name);
                        }}
                      >
                        {t('block.action', { name: m.author.name })}
                      </MenuItem>
                    </>
                  )}
                </Dropdown>
              )}
            </li>
          ))
        )}
      </ol>
      <form onSubmit={enviar} class="border-border flex flex-col gap-1 border-t p-3">
        {aviso ? (
          <p class="text-danger-text text-xs" role="alert">
            {aviso}
          </p>
        ) : null}
        <div class="flex gap-2">
          <Input
            value={texto}
            maxLength={MAX_CUERPO}
            aria-label={t('chat.placeholder')}
            placeholder={t('chat.placeholder')}
            onInput={(e) => setTexto(e.currentTarget.value)}
            class="min-w-0 flex-1"
          />
          <Button type="submit" variant="primary" disabled={!texto.trim() || !conectado}>
            {t('chat.send')}
          </Button>
        </div>
      </form>
    </>
  );
}
