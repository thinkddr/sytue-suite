/**
 * CreateChannelModal — «Crear canal» (paridad Slack «+ Añadir canales» / Mattermost
 * «Crear canal nuevo» / Teams «Añadir canal»). Nombre + visibilidad (público/privado)
 * + tema opcional; POST /api/channels y, al volver, el padre selecciona el canal.
 *
 * El nombre se guarda tal cual se escribe, pero el servicio Go deriva el SLUG de la URL
 * con domain.Slugify: se muestra como pista para que nadie se lleve sorpresas con los
 * espacios ni con los acentos (que el backend descarta).
 *
 * ponytail: sin miembros iniciales. El backend acepta `members[]` solo en la creación y
 * todavía no hay panel de miembros de canal, así que un canal privado nacería congelado
 * con esa lista; se añade cuando exista ese panel (docs/roadmap.md, «Miembros del
 * canal»). Mientras tanto, un canal privado empieza con su creador.
 */
import type { JSX } from 'preact';
import { useEffect, useId, useState } from 'preact/hooks';
import { Modal, Button, Input, FieldGroup, Radio, describedBy } from '@sytue/ui';
import { createChannel, ChatHttpError, type Channel } from '../client/chatApi.js';
import type { T } from '../i18n/index.js';

/** Tope de `domain.ValidName` en el servicio Go (100 runas). */
const MAX_NAME = 100;

/**
 * Réplica del `domain.Slugify` del servicio Go (services/chat/internal/domain/domain.go):
 * minúsculas; solo [a-z0-9] sobreviven; espacio, `_`, `-` y `.` se colapsan en un guion;
 * lo demás (acentos, emoji, signos) se descarta; sin guiones en los extremos; 64 máx.
 * Es solo la PISTA visible: el slug de verdad lo calcula el backend.
 */
export function channelSlug(name: string): string {
  let out = '';
  let prevDash = false;
  for (const ch of name.trim().toLowerCase()) {
    if ((ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9')) {
      out += ch;
      prevDash = false;
    } else if (ch === ' ' || ch === '_' || ch === '-' || ch === '.') {
      if (!prevDash && out.length > 0) {
        out += '-';
        prevDash = true;
      }
    }
  }
  out = trimDashes(out);
  return out.length > 64 ? trimDashes(out.slice(0, 64)) : out;
}

function trimDashes(s: string): string {
  return s.replace(/^-+|-+$/g, '');
}

interface Props {
  open: boolean;
  onClose: () => void;
  /** Llamado con el canal recién creado (para refrescar el sidebar y entrar en él). */
  onCreated: (channel: Channel) => void;
  /** Traductor ya atado al idioma. */
  t: T;
}

export function CreateChannelModal({ open, onClose, onCreated, t }: Props): JSX.Element | null {
  const [name, setName] = useState('');
  const [kind, setKind] = useState<'public' | 'private'>('public');
  const [topic, setTopic] = useState('');
  const [busy, setBusy] = useState(false);
  /** Error del campo Nombre (duplicado / inválido), junto al campo. */
  const [nameError, setNameError] = useState('');
  /** Fallo genérico (red, 5xx), como en el resto de modales de la app. */
  const [failed, setFailed] = useState(false);

  const nameId = useId();
  const topicId = useId();
  const kindPublicId = useId();
  const kindPrivateId = useId();

  // El formulario se limpia al CERRAR, no al abrir: el efecto pasivo de abrir correría
  // DESPUÉS del primer render del modal y podría pisar lo primero que se teclee. Al abrir
  // solo se coloca el foco en el primer campo (el Modal lo pone en su «X» de cerrar, y su
  // efecto es de layout: corre antes que este).
  useEffect(() => {
    if (open) {
      document.getElementById(nameId)?.focus();
      return;
    }
    setName('');
    setKind('public');
    setTopic('');
    setBusy(false);
    setNameError('');
    setFailed(false);
  }, [open, nameId]);

  const slug = channelSlug(name);
  const nameHelp = slug ? t('createChannel.slugHelp', { slug }) : t('createChannel.nameHelp');
  const topicHelp = t('createChannel.topicHelp');

  function submit(): void {
    if (!slug || busy) return;
    setBusy(true);
    setNameError('');
    setFailed(false);
    createChannel({ name: name.trim(), kind, ...(topic.trim() ? { topic: topic.trim() } : {}) })
      .then(({ channel }) => onCreated(channel))
      .catch((err: unknown) => {
        if (err instanceof ChatHttpError && err.status === 409) {
          setNameError(t('createChannel.duplicate'));
        } else if (err instanceof ChatHttpError && err.status === 400) {
          setNameError(t('createChannel.invalidName'));
        } else if (err instanceof ChatHttpError && err.status === 403) {
          setNameError(t('createChannel.forbidden'));
        } else {
          setFailed(true);
        }
      })
      .finally(() => setBusy(false));
  }

  if (!open) return null;

  return (
    <Modal
      open={open}
      onClose={onClose}
      title={t('createChannel.title')}
      size="md"
      footer={
        <>
          <Button variant="ghost" onClick={onClose} disabled={busy}>
            {t('action.cancel')}
          </Button>
          <Button variant="primary" onClick={submit} disabled={!slug || busy} loading={busy}>
            {t('createChannel.title')}
          </Button>
        </>
      }
    >
      <div class="flex flex-col gap-4">
        {/* FieldGroup oculta la ayuda cuando hay error: el aviso de nombre duplicado sale
            en el sitio de la pista del slug, junto al campo (no en un toast). */}
        <FieldGroup
          label={t('createChannel.name')}
          htmlFor={nameId}
          required
          help={nameHelp}
          {...(nameError ? { error: nameError } : {})}
        >
          <Input
            id={nameId}
            value={name}
            maxLength={MAX_NAME}
            placeholder={t('createChannel.namePlaceholder')}
            invalid={Boolean(nameError)}
            aria-describedby={describedBy(nameId, nameHelp, nameError || undefined)}
            onInput={(e) => {
              setName(e.currentTarget.value);
              setNameError('');
            }}
            onKeyDown={(e) => {
              if (e.key === 'Enter') {
                e.preventDefault();
                submit();
              }
            }}
          />
        </FieldGroup>

        {/* Grupo de radio: fieldset/legend (semántica del grupo) con el control canónico
            Radio y la explicación de una línea asociada por aria-describedby. */}
        <fieldset class="flex flex-col gap-2">
          <legend class="text-text mb-1.5 text-sm font-semibold">
            {t('createChannel.visibility')}
          </legend>
          <div>
            <Radio
              id={kindPublicId}
              name="channel-kind"
              label={t('createChannel.public')}
              checked={kind === 'public'}
              aria-describedby={`${kindPublicId}-desc`}
              onChange={() => setKind('public')}
            />
            <p id={`${kindPublicId}-desc`} class="text-text-muted pl-6 text-xs">
              {t('createChannel.publicHelp')}
            </p>
          </div>
          <div>
            <Radio
              id={kindPrivateId}
              name="channel-kind"
              label={t('createChannel.private')}
              checked={kind === 'private'}
              aria-describedby={`${kindPrivateId}-desc`}
              onChange={() => setKind('private')}
            />
            <p id={`${kindPrivateId}-desc`} class="text-text-muted pl-6 text-xs">
              {t('createChannel.privateHelp')}
            </p>
          </div>
        </fieldset>

        <FieldGroup label={t('channel.topic')} htmlFor={topicId} help={topicHelp}>
          <Input
            id={topicId}
            value={topic}
            placeholder={t('createChannel.topicPlaceholder')}
            aria-describedby={describedBy(topicId, topicHelp)}
            onInput={(e) => setTopic(e.currentTarget.value)}
          />
        </FieldGroup>

        {failed ? (
          <p class="text-danger-text text-sm" role="alert">
            {t('createChannel.failed')}
          </p>
        ) : null}
      </div>
    </Modal>
  );
}
