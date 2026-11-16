/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * PersonCard — la tarjeta que se abre al pulsar el avatar o el nombre de alguien
 * (D-ROOM-04): nombre, estado, correo del directorio de la organización y su hora
 * local, con «Mensaje» y «Llamar» a mano. Es el Popover de @sytue/ui (no modal: se
 * cierra con Esc o al pulsar fuera y devuelve el foco), así que no se reinventa ni el
 * posicionamiento ni el atrapado del Tab.
 *
 * La foto: el Avatar ya prefiere `src` a las iniciales; Rooms todavía no recibe la
 * foto de Sytue ID de los demás (solo la propia), así que hoy salen iniciales.
 */
import type { ComponentChildren, JSX } from 'preact';
import { useEffect, useState } from 'preact/hooks';
import { Avatar, Button, Icon, Popover } from '@sytue/ui';
import type { Locale } from '@sytue/i18n';
import type { T } from '../i18n/index.js';
import {
  STATE_KEY,
  queryPresence,
  toPresenceStatus,
  type Presence,
} from '../client/presenceApi.js';
import { directorio, horaLocal, type FichaDirectorio } from '../lib/tarjetaPersona.js';
import { VideoGlyph } from './glyphs.js';

/** Lo que la vista que la usa sabe y comparte entre todas sus tarjetas. */
export interface TarjetaContexto {
  presences: Record<string, Presence>;
  selfSub: string;
  locale: Locale;
  t: T;
  /** Abre (o crea) el mensaje directo con esa persona. */
  onMessage: (sub: string) => void;
  /** Abre el directo y empieza la llamada en él. */
  onCall: (sub: string) => void;
}

interface Props {
  sub: string;
  name: string;
  ctx: TarjetaContexto;
  /** Lo que se pulsa (el avatar, el nombre): va dentro de un botón. */
  children: ComponentChildren;
  /** Clases del botón disparador (el sitio decide su forma). */
  class?: string;
}

// El envoltorio del Popover es el elemento flex de la fila: sin `min-w-0` un nombre
// largo no se trunca y empuja la hora fuera del mensaje (en móvil, fuera de pantalla).

export function PersonCard({ sub, name, ctx, children, class: clase }: Props): JSX.Element {
  const { t } = ctx;
  return (
    <Popover
      label={name}
      class="min-w-0"
      trigger={(p) => (
        <button
          type="button"
          {...p}
          aria-label={t('person.openProfile', { name })}
          // El clic NO sube al mensaje: allí despliega la barra de acciones táctil.
          onClick={(e) => {
            e.stopPropagation();
            p.onClick();
          }}
          class={`cursor-pointer rounded-sm text-left ${clase ?? ''}`}
        >
          {children}
        </button>
      )}
    >
      {(cerrar) => <Contenido sub={sub} name={name} ctx={ctx} cerrar={cerrar} />}
    </Popover>
  );
}

function Contenido({
  sub,
  name,
  ctx,
  cerrar,
}: {
  sub: string;
  name: string;
  ctx: TarjetaContexto;
  cerrar: () => void;
}): JSX.Element {
  const { t, locale } = ctx;
  const [ficha, setFicha] = useState<FichaDirectorio | undefined>(undefined);
  // ChatApp solo sondea la presencia de los peers de los DMs y del roster abierto: el
  // autor de un mensaje en un canal no suele estar ahí, y sin esto su tarjeta saldría
  // «Sin conexión» y sin hora. Se pide la suya al abrir (una GET); si falla, se queda
  // lo que ya se sabía.
  const [fresca, setFresca] = useState<Presence | undefined>(undefined);
  // La hora se mueve con la tarjeta abierta: se repinta cada minuto.
  const [ahora, setAhora] = useState(() => new Date());
  useEffect(() => {
    let vivo = true;
    directorio()
      .then((m) => vivo && setFicha(m.get(sub)))
      // Sin directorio la tarjeta sale igual, sin correo.
      .catch(() => {});
    queryPresence([sub])
      .then((m) => vivo && m[sub] && setFresca(m[sub]))
      .catch(() => {});
    const id = setInterval(() => setAhora(new Date()), 60_000);
    return () => {
      vivo = false;
      clearInterval(id);
    };
  }, [sub]);

  const p = fresca ?? ctx.presences[sub];
  const estado = p?.state ?? 'offline';
  const hora = horaLocal(locale, p?.tz, ahora);
  const propia = sub === ctx.selfSub;

  return (
    <div class="flex w-64 max-w-full flex-col gap-3" data-tarjeta-persona={sub}>
      <div class="flex items-center gap-3">
        <Avatar name={name} size="lg" presence={toPresenceStatus(estado)} />
        <div class="min-w-0">
          <p class="text-text truncate text-base font-semibold" title={name}>
            {name}
          </p>
          {ficha?.title ? <p class="text-text-muted truncate text-sm">{ficha.title}</p> : null}
          <p class="text-text-muted truncate text-sm">
            {t(STATE_KEY[estado])}
            {p?.status ? ` · ${p.status}` : ''}
          </p>
        </div>
      </div>
      {ficha?.email || hora ? (
        <dl class="flex flex-col gap-1 text-sm">
          {ficha?.email ? (
            <div class="flex min-w-0 items-center gap-2">
              <dt class="text-text-muted shrink-0">
                <Icon name="mail" size={14} label={t('person.email')} />
              </dt>
              <dd class="min-w-0 truncate">
                <a class="text-primary-text hover:underline" href={`mailto:${ficha.email}`}>
                  {ficha.email}
                </a>
              </dd>
            </div>
          ) : null}
          {hora ? (
            <div class="flex items-center gap-2">
              <dt class="text-text-muted shrink-0">
                <Icon name="clock" size={14} label={t('person.localTimeLabel')} />
              </dt>
              <dd class="text-text">{t('person.localTime', { time: hora })}</dd>
            </div>
          ) : null}
        </dl>
      ) : null}
      {propia ? null : (
        <div class="flex gap-2">
          <Button
            size="sm"
            class="flex-1"
            leadingIcon={<Icon name="comment" size={14} />}
            onClick={() => {
              cerrar();
              ctx.onMessage(sub);
            }}
          >
            {t('person.message')}
          </Button>
          <Button
            size="sm"
            variant="secondary"
            class="flex-1"
            leadingIcon={<VideoGlyph size={14} />}
            onClick={() => {
              cerrar();
              ctx.onCall(sub);
            }}
          >
            {t('person.call')}
          </Button>
        </div>
      )}
    </div>
  );
}
