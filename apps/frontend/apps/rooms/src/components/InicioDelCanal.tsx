/**
 * InicioDelCanal — el bloque «Este es el principio de #canal» que Slack y Teams pintan
 * al llegar al principio del historial (no solo con el canal vacío): quién creó el
 * canal y cuándo, de qué va (propósito) y su tema, con las dos acciones que tienen
 * sentido justo ahí —añadir personas y editar el tema— para quien puede usarlas.
 * En un directo es «Conversación con Ana» con su presencia.
 *
 * Lo monta ChatApp cuando ya no queda historial anterior (`hasOlder=false`) y
 * MessageView lo pone encima de la primera fila; con el canal vacío sustituye al
 * estado vacío de siempre y añade la invitación a escribir el primer mensaje.
 */
import type { JSX } from 'preact';
import { Avatar, Button, Icon, Illustration } from '@sytue/ui';
import { formatDate, type Locale } from '@sytue/i18n';
import type { Channel } from '../lib/chat.js';
import type { T } from '../i18n/index.js';
import { EditGlyph } from './glyphs.js';

/** Ancho de la ilustración: pequeña, es un encabezado y no un estado vacío a pantalla. */
const LADO_ILUSTRACION = 72;

export interface InicioDelCanalProps {
  canal: Pick<Channel, 'kind' | 'name' | 'topic' | 'purpose' | 'peers' | 'createdBy' | 'createdAt'>;
  /** Sub de quien mira: si creó él el canal, la línea dice «Creaste…». */
  yo: string;
  /** Nombre del creador (roster o equipo). Sin él se dice «alguien». */
  nombreCreador?: string | undefined;
  /** Directo 1:1: etiqueta de la presencia del otro («En línea», «Ausente · …»). */
  presencia?: string | undefined;
  /** Abre el panel de personas para añadir. Ausente = quien mira no puede. */
  onAnadirPersonas?: (() => void) | undefined;
  /** Abre la edición del tema. Ausente = quien mira no puede. */
  onEditarTema?: (() => void) | undefined;
  /** El canal no tiene mensajes: añade «Escribe el primer mensaje…». */
  sinMensajes?: boolean;
  locale: Locale;
  t: T;
}

/** La línea «Ana creó este canal el 3 de mayo.» (o «Creaste…»). Vacía si el canal
 *  no trae fecha de creación: mejor no decir nada que inventarse una. */
export function lineaDeCreacion(
  t: T,
  locale: Locale,
  canal: Pick<Channel, 'createdBy' | 'createdAt'>,
  yo: string,
  nombreCreador?: string,
): string {
  if (!canal.createdAt) return '';
  const fecha = formatDate(locale, canal.createdAt, { dateStyle: 'long' });
  if (canal.createdBy && canal.createdBy === yo)
    return t('message.introCreatedByYou', { date: fecha });
  return t('message.introCreatedBy', {
    name: nombreCreador?.trim() || t('message.introSomeone'),
    date: fecha,
  });
}

export function InicioDelCanal({
  canal,
  yo,
  nombreCreador,
  presencia,
  onAnadirPersonas,
  onEditarTema,
  sinMensajes = false,
  locale,
  t,
}: InicioDelCanalProps): JSX.Element {
  const directo = canal.kind === 'direct';
  const otros = canal.peers?.length ? canal.peers.join(', ') : canal.name;
  const titulo = directo
    ? t('message.introDm', { name: otros })
    : t('message.channelStart', { channel: canal.name });
  const creacion = directo ? '' : lineaDeCreacion(t, locale, canal, yo, nombreCreador);
  const unoAUno = directo && (canal.peers?.length ?? 1) <= 1;
  const acciones = !directo && (onAnadirPersonas || onEditarTema);

  return (
    <section class="px-2 pt-6 pb-4" aria-label={titulo} data-inicio-canal="">
      {directo ? (
        <Avatar name={otros} size="xl" class="mb-3" />
      ) : (
        <div class="mb-2">
          <Illustration name="emptyChat" size={LADO_ILUSTRACION} />
        </div>
      )}
      <h2 class="text-text text-xl leading-tight font-semibold break-words">{titulo}</h2>
      {unoAUno && presencia ? <p class="text-text-muted mt-1 text-sm">{presencia}</p> : null}
      {directo ? (
        <p class="text-text-muted mt-2 text-sm">{t('message.introDmStart', { name: otros })}</p>
      ) : null}
      {creacion ? <p class="text-text-muted mt-2 text-sm">{creacion}</p> : null}
      {!directo && canal.purpose ? (
        <p class="text-text mt-2 text-sm break-words whitespace-pre-wrap">{canal.purpose}</p>
      ) : null}
      {!directo && canal.topic ? (
        <p class="text-text-muted mt-1 text-sm break-words">
          <span class="font-semibold">{t('channel.topic')}:</span> {canal.topic}
        </p>
      ) : null}
      {sinMensajes ? <p class="text-text-muted mt-2 text-sm">{t('message.writeFirst')}</p> : null}
      {acciones ? (
        <div class="mt-3 flex flex-wrap gap-2">
          {onAnadirPersonas ? (
            <Button
              size="sm"
              variant="secondary"
              leadingIcon={<Icon name="plus" size={14} />}
              onClick={onAnadirPersonas}
            >
              {t('members.addPeople')}
            </Button>
          ) : null}
          {onEditarTema ? (
            <Button
              size="sm"
              variant="secondary"
              leadingIcon={<EditGlyph size={14} />}
              onClick={onEditarTema}
            >
              {canal.topic ? t('channel.editTopic') : t('channel.addTopic')}
            </Button>
          ) : null}
        </div>
      ) : null}
    </section>
  );
}
