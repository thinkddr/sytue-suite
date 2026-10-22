/**
 * InvitationsModal — bandeja de invitaciones (scheduling inbox iTIP, RFC 6638).
 * Lista las invitaciones pendientes del usuario y permite responder
 * Aceptar/Rechazar/Quizás (RespondInvitation via BFF). Design system Sytue.
 */
import type { JSX } from 'preact';
import { useEffect, useState } from 'preact/hooks';
import { Button, Modal, Spinner, EmptyState, InlineHelp, Illustration, useToast } from '@sytue/ui';
import { formatDateTime, type Locale } from '@sytue/i18n';
import { nombreDeOrganizacion } from '@sytue/shell';
import {
  listInvitations,
  respondInvitation,
  type Invitation,
  type InvitationResponse,
} from '../client/calendarApi.js';
import type { CalendarMessageKey, T } from '../i18n/index.js';

interface Props {
  /** Traductor ya atado al idioma de la petición; baja del padre. */
  t: T;
  /** Idioma negociado en el servidor: la fecha sale en su convención. */
  locale: Locale;
  open: boolean;
  onClose: () => void;
  /** Notifica el nuevo nº de pendientes (para el badge de la cabecera). */
  onCountChange: (pending: number) => void;
}

/**
 * Cada respuesta dice lo que HACE, no el estado técnico: quien organiza ve tu
 * respuesta y el evento entra (o no) en tu agenda.
 */
const ACTIONS: {
  status: InvitationResponse;
  label: CalendarMessageKey;
  hint: CalendarMessageKey;
  variant: 'primary' | 'secondary' | 'danger';
}[] = [
  {
    status: 'accepted',
    label: 'invitations.accept',
    hint: 'invitations.acceptHint',
    variant: 'primary',
  },
  {
    status: 'tentative',
    label: 'invitations.tentative',
    hint: 'invitations.tentativeHint',
    variant: 'secondary',
  },
  {
    status: 'declined',
    label: 'invitations.decline',
    hint: 'invitations.declineHint',
    variant: 'danger',
  },
];

/** Aviso de la respuesta enviada, en la clave que corresponda al estado. */
const RESPONSE_TOAST: Record<InvitationResponse, CalendarMessageKey> = {
  accepted: 'toast.invitationAccepted',
  declined: 'toast.invitationDeclined',
  tentative: 'toast.invitationTentative',
};

export function InvitationsModal({ t, locale, open, onClose, onCountChange }: Props): JSX.Element {
  const toast = useToast();
  const [invitations, setInvitations] = useState<Invitation[]>([]);
  const [loading, setLoading] = useState(true);
  const [busyId, setBusyId] = useState<string | null>(null);

  useEffect(() => {
    if (!open) return;
    setLoading(true);
    void listInvitations(true)
      .then(({ invitations: list }) => {
        setInvitations(list);
        onCountChange(list.length);
      })
      .catch(() => toast.notify({ tone: 'danger', title: t('toast.invitationsLoadFailed') }))
      .finally(() => setLoading(false));
    // onCountChange/toast son estables; recargar solo al abrir.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open]);

  async function respond(inv: Invitation, status: InvitationResponse): Promise<void> {
    setBusyId(inv.id);
    try {
      await respondInvitation(inv.id, status);
      const rest = invitations.filter((i) => i.id !== inv.id);
      setInvitations(rest);
      onCountChange(rest.length);
      toast.notify({ tone: 'success', title: t(RESPONSE_TOAST[status]) });
    } catch {
      toast.notify({ tone: 'danger', title: t('toast.invitationFailed') });
    } finally {
      setBusyId(null);
    }
  }

  return (
    <Modal open={open} onClose={onClose} title={t('invitations.title')} size="md">
      {loading ? (
        <div
          class="flex items-center justify-center py-10"
          role="status"
          aria-label={t('invitations.loading')}
        >
          <Spinner />
        </div>
      ) : invitations.length === 0 ? (
        <EmptyState
          title={t('invitations.emptyTitle')}
          description={t('invitations.emptyDescription')}
          illustration={<Illustration name="emptyCalendar" />}
        />
      ) : (
        <div class="flex flex-col gap-4">
          <InlineHelp tone="info">{t('invitations.help')}</InlineHelp>
          <ul class="flex flex-col gap-3">
            {invitations.map((inv) => {
              // Quién y cuándo se unen con una plantilla, no con un `+`: el
              // separador y el orden son cosa de cada lengua.
              // `organizer` llega tal cual viene del iCalendar, y ahí la dirección
              // va con su esquema: «mailto:ada@sytue.eu». El servicio lo limpia al
              // CREAR un evento (`cleanAddr`), pero lo que entra por CalDAV no pasa
              // por ahí, así que a la persona se le enseñaba el «mailto:» delante.
              const quien = inv.organizer?.replace(/^mailto:/i, '').trim();
              // Una invitación de otra organización dice cuál (COM-15).
              const who = !quien
                ? t('invitations.generic')
                : inv.organization
                  ? t('invitations.invitedByFrom', { name: quien, org: nombreDeOrganizacion(inv) })
                  : t('invitations.invitedBy', { name: quien });
              return (
                <li key={inv.id} class="border-border bg-surface rounded-md border p-3">
                  <div class="flex flex-col gap-0.5">
                    <span class="text-text font-medium">
                      {inv.summary || t('invitations.untitled')}
                    </span>
                    <span class="text-text-muted text-sm">
                      {inv.start
                        ? t('invitations.whoWhen', {
                            who,
                            when: formatDateTime(locale, new Date(inv.start)),
                          })
                        : who}
                    </span>
                  </div>
                  <div class="mt-2.5 flex flex-wrap gap-2">
                    {ACTIONS.map((a) => (
                      <Button
                        key={a.status}
                        size="sm"
                        variant={a.variant}
                        title={t(a.hint)}
                        loading={busyId === inv.id}
                        disabled={busyId !== null}
                        onClick={() => void respond(inv, a.status)}
                      >
                        {t(a.label)}
                      </Button>
                    ))}
                  </div>
                </li>
              );
            })}
          </ul>
        </div>
      )}
    </Modal>
  );
}
