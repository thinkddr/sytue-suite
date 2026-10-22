/**
 * MeetingsPanel — panel lateral (RHS) de REUNIONES: las salas que no cuelgan de un
 * canal. Cubre lo que la llamada por canal no puede: convocar a gente que no está en
 * un canal, y entrar en la reunión de otro pegando su enlace.
 *
 * Con invitados —escritos por su correo o su @usuario, también de OTRA organización
 * (ADR 0040)— la reunión no es una sala suelta: se abre la conversación con esas
 * personas y se llama en ella. Así les suena, la ven en su barra y pueden volver a
 * entrar, sin depender de que alguien les pase un enlace.
 *
 * El servicio Go servía `GET /rooms`, `/recordings` y `RemoveParticipant` desde el
 * principio y NADA en el frontend los consumía: se podía llamar dentro de un canal y
 * punto. Este panel es su puerta.
 *
 * Mismo patrón visual que ScheduledPanel (cabecera sólida + cuerpo desplazable) para
 * que no haya una tercera forma de panel en la app.
 *
 * ponytail: sin sondeo automático — la lista es una foto al abrir, con «actualizar»
 * a mano. Un WS para reuniones sería un canal nuevo por una vista que se mira poco;
 * si se demuestra que hace falta, el evento `meet.room.started` ya viaja por el outbox.
 */
import { useState } from 'preact/hooks';
import type { JSX } from 'preact';
import {
  EmptyState,
  formatBytes,
  Illustration,
  Button,
  Dropdown,
  Icon,
  MenuItem,
  Skeleton,
  IconButton,
  withBase,
} from '@sytue/ui';
import { enlaceReunion } from '@sytue/config';
import { formatDateTime, type Locale } from '@sytue/i18n';
import type { T } from '../i18n/index.js';
import type { Persona } from '../client/chatApi.js';
import type { Meeting, MeetingParticipant, MeetingRecording } from '../lib/meetingTypes.js';
import { BuscarPersona } from './BuscarPersona.js';
import { RemovableChip } from './RemovableChip.js';
import {
  enlaceParaCompartir,
  nombreDeProveedor,
  proveedorDeReunion,
  type DestinoCompartir,
} from '../lib/enlacesExternos.js';

// Los tipos vienen del cliente (que los reexporta de lib/meet.ts): una sola forma
// para el servidor, el navegador y la vista. `import type` no arrastra el módulo.
export type { Meeting, MeetingParticipant, MeetingRecording };

interface Props {
  meetings: Meeting[];
  loading: boolean;
  error?: boolean;
  /** Detalle abierto (participantes + grabaciones) de la reunión `detailFor`. */
  detailFor?: string | undefined;
  participants?: MeetingParticipant[];
  recordings?: MeetingRecording[];
  detailLoading?: boolean;
  /** true si el usuario puede moderar (host de la sala o admin del equipo). */
  canModerate?: boolean;
  /** Convoca; con `people`, la llamada se abre en la conversación con ellos. */
  onCreate: (title: string, people: Persona[]) => Promise<void>;
  onJoin: (id: string, clave: string) => Promise<void>;
  onOpenDetail: (id: string) => void;
  onClose: (id: string) => void;
  onRemoveParticipant: (id: string, identity: string, name: string) => void;
  onRefresh: () => void;
  onDismiss: () => void;
  locale: Locale;
  t: T;
}

/**
 * Saca el id de reunión de lo que el usuario pegue: la URL completa
 * (`https://rooms…/?rooms=<id>`, o la antigua, con `meet`) o el id suelto. Devuelve "" si no hay nada usable —
 * es lo que distingue «enlace inválido» de «campo vacío» en la UI.
 */
export function meetingIdFromInput(raw: string): string {
  const s = raw.trim();
  if (!s) return '';
  const uuid = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;
  if (uuid.test(s)) return s;
  try {
    const q = new URL(s).searchParams;
    const fromParam = (q.get('rooms') ?? q.get('meet'))?.trim() ?? '';
    return uuid.test(fromParam) ? fromParam : '';
  } catch {
    return '';
  }
}

/** La clave que lleva el enlace pegado (la de una reunión suelta), o "". */
export function claveDeEnlace(raw: string): string {
  try {
    return new URL(raw.trim()).searchParams.get('clave')?.trim() ?? '';
  } catch {
    return '';
  }
}

/** Duración legible a partir de segundos. */
function humanDuration(t: T, sec: number): string {
  if (sec < 60) return t('call.duration.underMinute');
  const mins = Math.round(sec / 60);
  if (mins < 60) return t('call.duration.minutes', { count: mins });
  const hours = Math.floor(mins / 60);
  const rest = mins % 60;
  return rest === 0
    ? t('call.duration.hours', { count: hours })
    : t('call.duration.hoursMinutes', { hours, minutes: rest });
}

export function MeetingsPanel({
  meetings,
  loading,
  error = false,
  detailFor,
  participants = [],
  recordings = [],
  detailLoading = false,
  canModerate = false,
  onCreate,
  onJoin,
  onOpenDetail,
  onClose,
  onRemoveParticipant,
  onRefresh,
  onDismiss,
  locale,
  t,
}: Props): JSX.Element {
  const [title, setTitle] = useState('');
  const [busqueda, setBusqueda] = useState('');
  const [invitados, setInvitados] = useState<Persona[]>([]);
  const [link, setLink] = useState('');
  const [linkError, setLinkError] = useState(false);
  const [busy, setBusy] = useState<'create' | 'join' | null>(null);
  const [copied, setCopied] = useState<string | null>(null);

  async function crear(): Promise<void> {
    if (busy) return;
    setBusy('create');
    try {
      await onCreate(title.trim(), invitados);
      setTitle('');
      setInvitados([]);
    } finally {
      setBusy(null);
    }
  }

  async function entrarPorEnlace(): Promise<void> {
    if (busy) return;
    // Un enlace de Zoom, Meet, Teams o Webex se abre tal cual, en otra pestaña:
    // Rooms es desde donde se entra a cualquier reunión, no solo a las suyas.
    const externo = proveedorDeReunion(link);
    if (externo) {
      window.open(link.trim(), '_blank', 'noopener');
      setLink('');
      return;
    }
    const id = meetingIdFromInput(link);
    if (!id) {
      setLinkError(true);
      return;
    }
    setLinkError(false);
    setBusy('join');
    try {
      await onJoin(id, claveDeEnlace(link));
      setLink('');
    } finally {
      setBusy(null);
    }
  }

  async function copiar(m: Meeting): Promise<void> {
    await navigator.clipboard.writeText(enlaceReunion(m.id, m.linkKey ?? ''));
    setCopied(m.id);
    window.setTimeout(() => setCopied((c) => (c === m.id ? null : c)), 2000);
  }

  /** Compartir por WhatsApp, correo o Teams: la URL de compartir de cada uno, sin API. */
  function compartir(destino: DestinoCompartir, m: Meeting): void {
    const url = enlaceReunion(m.id, m.linkKey ?? '');
    const href = enlaceParaCompartir(
      destino,
      t('meetings.shareText', { title: m.title }),
      url,
      m.title,
    );
    window.open(href, '_blank', 'noopener');
  }

  const proveedorPegado = proveedorDeReunion(link);

  return (
    <aside
      class="sytue-sheet-corners border-border bg-surface marco:overflow-clip flex h-full w-full shrink-0 flex-col border-l md:w-[var(--rooms-rhs-width)] md:[--sytue-sheet-bl:0px] md:[--sytue-sheet-tl:0px]"
      aria-label={t('meetings.title')}
    >
      <header class="bg-header text-header-fg flex h-14 shrink-0 items-center justify-between gap-2 px-4">
        <h2 class="text-md text-header-fg leading-tight font-semibold">{t('meetings.title')}</h2>
        <div class="flex items-center gap-0.5">
          <button
            type="button"
            onClick={onRefresh}
            class="text-header-fg hover:bg-chrome-hover active:bg-chrome-active sytue-focus-ring rounded-control inline-flex h-8 shrink-0 items-center justify-center px-2 text-xs font-medium transition-colors duration-150"
          >
            {t('action.retry')}
          </button>
          <IconButton size="lg" tone="header" label={t('action.close')} onClick={onDismiss}>
            <Icon name="close" size={18} />
          </IconButton>
        </div>
      </header>

      <div class="min-h-0 flex-1 overflow-y-auto">
        {/* --- Convocar. Un campo y un botón: el título es opcional y el BFF pone
            uno reconocible si se deja vacío, así que «Nueva reunión» es 1 clic. --- */}
        <section class="border-border-subtle border-b px-4 py-4">
          <h3 class="text-text mb-1 text-sm font-semibold">{t('meetings.new')}</h3>
          <p class="sytue-meta mb-3">{t('meetings.newHint')}</p>
          <label class="sr-only" for="meeting-title">
            {t('meetings.titleLabel')}
          </label>
          <div class="flex items-center gap-2">
            <input
              id="meeting-title"
              type="text"
              value={title}
              placeholder={t('meetings.titlePlaceholder')}
              onInput={(e) => setTitle((e.target as HTMLInputElement).value)}
              onKeyDown={(e) => {
                if (e.key === 'Enter') void crear();
              }}
              class="border-border bg-surface text-text placeholder:text-text-subtle sytue-focus-ring min-w-0 flex-1 rounded-md border px-2.5 py-1.5 text-sm"
            />
            <Button size="sm" onClick={() => void crear()} disabled={busy !== null}>
              {busy === 'create' ? t('meetings.creating') : t('meetings.create')}
            </Button>
          </div>
          <label class="text-text mt-3 mb-1 block text-xs font-medium" for="meeting-invite">
            {t('meetings.invite')}
          </label>
          <input
            id="meeting-invite"
            type="text"
            value={busqueda}
            placeholder={t('meetings.invitePlaceholder')}
            autocomplete="off"
            spellcheck={false}
            onInput={(e) => setBusqueda((e.target as HTMLInputElement).value)}
            aria-describedby="meeting-invite-hint"
            class="border-border bg-surface text-text placeholder:text-text-subtle sytue-focus-ring w-full rounded-md border px-2.5 py-1.5 text-sm"
          />
          <p id="meeting-invite-hint" class="sytue-meta mt-1">
            {t('meetings.inviteHint')}
          </p>
          <BuscarPersona
            query={busqueda}
            onFound={(p) => {
              setBusqueda('');
              // Un DM de grupo admite hasta ocho personas contando a quien convoca.
              setInvitados((prev) =>
                prev.some((x) => x.sub === p.sub) || prev.length >= 7 ? prev : [...prev, p],
              );
            }}
            disabled={busy !== null || invitados.length >= 7}
            t={t}
          />
          {invitados.length > 0 ? (
            <ul class="mt-2 flex flex-wrap gap-1.5" aria-label={t('meetings.invite')}>
              {invitados.map((p) => (
                <li key={p.sub}>
                  <RemovableChip
                    label={p.username ? `${p.name} · @${p.username}` : p.name}
                    ariaLabel={t('action.removePerson', { name: p.name })}
                    onRemove={() => setInvitados((prev) => prev.filter((x) => x.sub !== p.sub))}
                  />
                </li>
              ))}
            </ul>
          ) : null}
        </section>

        {/* --- Unirse por enlace. Acepta la URL entera o el id suelto: pegar lo que
            te han mandado tiene que bastar. --- */}
        <section class="border-border-subtle border-b px-4 py-4">
          <h3 class="text-text mb-1 text-sm font-semibold">{t('meetings.joinByLink')}</h3>
          <p class="sytue-meta mb-3">{t('meetings.joinByLinkHint')}</p>
          <label class="sr-only" for="meeting-link">
            {t('meetings.joinByLink')}
          </label>
          <div class="flex items-center gap-2">
            <input
              id="meeting-link"
              type="text"
              value={link}
              placeholder={t('meetings.linkPlaceholder')}
              aria-invalid={linkError ? 'true' : undefined}
              aria-describedby={linkError ? 'meeting-link-error' : undefined}
              onInput={(e) => {
                setLink((e.target as HTMLInputElement).value);
                setLinkError(false);
              }}
              onKeyDown={(e) => {
                if (e.key === 'Enter') void entrarPorEnlace();
              }}
              class="border-border bg-surface text-text placeholder:text-text-subtle sytue-focus-ring min-w-0 flex-1 rounded-md border px-2.5 py-1.5 text-sm"
            />
            <Button
              size="sm"
              variant="secondary"
              onClick={() => void entrarPorEnlace()}
              disabled={busy !== null}
            >
              {busy === 'join' ? t('meetings.joining') : t('meetings.join')}
            </Button>
          </div>
          {linkError ? (
            <p id="meeting-link-error" role="alert" class="text-danger-text mt-2 text-xs">
              {t('meetings.linkInvalid')}
            </p>
          ) : proveedorPegado ? (
            <p class="sytue-meta mt-2" role="status">
              {t('meetings.external', { provider: nombreDeProveedor(proveedorPegado) })}
            </p>
          ) : null}
        </section>

        {/* --- Lo que está pasando ahora --- */}
        <section class="px-4 py-4">
          <h3 class="text-text mb-3 text-sm font-semibold">{t('meetings.active')}</h3>

          {error ? (
            <p role="alert" class="text-danger-text flex items-center gap-1.5 text-sm">
              <Icon name="warning" size={14} />
              <span>{t('error.loadFailed')}</span>
              <button
                type="button"
                class="hover:text-text sytue-focus-ring rounded-sm font-semibold underline underline-offset-2"
                onClick={onRefresh}
              >
                {t('action.retry')}
              </button>
            </p>
          ) : loading ? (
            <div class="flex flex-col gap-3" aria-busy="true">
              <span role="status" class="sr-only">
                {t('state.loading')}
              </span>
              {['70%', '55%'].map((w, i) => (
                <Skeleton key={i} width={w} height="1rem" />
              ))}
            </div>
          ) : meetings.length === 0 ? (
            <EmptyState
              class="py-4"
              title={t('meetings.empty')}
              description={t('meetings.emptyHint')}
              illustration={<Illustration name="emptyCalendar" size={96} />}
            />
          ) : (
            <ul class="flex flex-col gap-2">
              {meetings.map((m) => (
                <li
                  key={m.id}
                  class="border-border-subtle rounded-lg border p-3 transition-colors duration-150"
                >
                  <div class="flex items-start justify-between gap-2">
                    <div class="min-w-0">
                      <p class="text-text truncate text-sm font-medium">{m.title}</p>
                      <p class="sytue-meta mt-0.5">
                        {m.startedAt
                          ? t('meetings.startedAt', {
                              time: formatDateTime(locale, m.startedAt, {
                                hour: '2-digit',
                                minute: '2-digit',
                              }),
                            })
                          : ''}
                      </p>
                    </div>
                    <Button size="sm" onClick={() => void onJoin(m.id, m.linkKey ?? '')}>
                      {t('meetings.join')}
                    </Button>
                  </div>

                  <div class="mt-2 flex flex-wrap items-center gap-1">
                    <button
                      type="button"
                      onClick={() => void copiar(m)}
                      class="sytue-meta hover:text-text sytue-focus-ring rounded-sm px-1.5 py-1"
                    >
                      {copied === m.id ? t('meetings.linkCopied') : t('meetings.copyLink')}
                    </button>
                    <Dropdown
                      label={t('meetings.share')}
                      align="start"
                      trigger={(props) => (
                        <button
                          {...props}
                          type="button"
                          class="sytue-meta hover:text-text sytue-focus-ring rounded-sm px-1.5 py-1"
                        >
                          {t('meetings.share')}
                        </button>
                      )}
                    >
                      {(close) => (
                        <>
                          {(['whatsapp', 'email', 'teams'] as const).map((destino) => (
                            <MenuItem
                              key={destino}
                              onClick={() => {
                                close();
                                compartir(destino, m);
                              }}
                            >
                              {t(`meetings.share.${destino}`)}
                            </MenuItem>
                          ))}
                        </>
                      )}
                    </Dropdown>
                    <button
                      type="button"
                      onClick={() => onOpenDetail(m.id)}
                      aria-expanded={detailFor === m.id}
                      class="sytue-meta hover:text-text sytue-focus-ring rounded-sm px-1.5 py-1"
                    >
                      {t('meetings.details')}
                    </button>
                    {canModerate ? (
                      <button
                        type="button"
                        onClick={() => onClose(m.id)}
                        class="text-danger-text hover:text-danger-text sytue-focus-ring rounded-sm px-1.5 py-1 text-xs"
                      >
                        {t('meetings.close')}
                      </button>
                    ) : null}
                  </div>

                  {detailFor === m.id ? (
                    <div class="border-border-subtle mt-3 border-t pt-3">
                      {detailLoading ? (
                        <Skeleton width="60%" height="0.875rem" />
                      ) : (
                        <>
                          <p class="sytue-meta mb-1.5 font-semibold">
                            {t('meetings.who')} ·{' '}
                            {t('meetings.participants', { count: participants.length })}
                          </p>
                          <ul class="mb-3 flex flex-col gap-1">
                            {participants.map((p) => (
                              <li key={p.id} class="flex items-center justify-between gap-2">
                                <span class="text-text truncate text-xs">{p.name}</span>
                                {canModerate ? (
                                  <button
                                    type="button"
                                    aria-label={t('meetings.remove')}
                                    title={t('meetings.remove')}
                                    onClick={() => onRemoveParticipant(m.id, p.identity, p.name)}
                                    class="text-text-subtle hover:text-danger-text sytue-focus-ring shrink-0 rounded-sm p-1"
                                  >
                                    <Icon name="close" size={14} />
                                  </button>
                                ) : null}
                              </li>
                            ))}
                          </ul>

                          <p class="sytue-meta mb-1.5 font-semibold">{t('meetings.recordings')}</p>
                          {recordings.length === 0 ? (
                            <p class="sytue-meta">{t('meetings.noRecordings')}</p>
                          ) : (
                            <ul class="flex flex-col gap-1">
                              {recordings.map((r) => (
                                <li key={r.id} class="text-text text-xs">
                                  {/* Solo las TERMINADAS se enlazan: mientras el egress
                                      graba, el fichero está a medias y descargarlo da un
                                      vídeo truncado. El enlace es un `<a>` de verdad
                                      —el BFF redirige a la URL firmada— para que abra
                                      con un clic y no lo pare el bloqueador. */}
                                  {r.status === 'completed' ? (
                                    <a
                                      class="text-primary-text sytue-focus-ring rounded-sm font-semibold underline underline-offset-2"
                                      href={withBase(
                                        import.meta.env.BASE_URL,
                                        `/api/meeting-recording?id=${encodeURIComponent(m.id)}&rec=${encodeURIComponent(r.id)}`,
                                      )}
                                      target="_blank"
                                      rel="noopener"
                                    >
                                      {t('action.download')} ·{' '}
                                      {t('meetings.recordingSize', {
                                        size:
                                          r.sizeBytes > 0 ? formatBytes(locale, r.sizeBytes) : '—',
                                        duration: humanDuration(t, r.durationSec),
                                      })}
                                    </a>
                                  ) : r.status === 'failed' ? (
                                    t('meetings.recordingFailed')
                                  ) : (
                                    t('meetings.recordingPending')
                                  )}
                                </li>
                              ))}
                            </ul>
                          )}
                        </>
                      )}
                    </div>
                  ) : null}
                </li>
              ))}
            </ul>
          )}
        </section>
      </div>
    </aside>
  );
}
