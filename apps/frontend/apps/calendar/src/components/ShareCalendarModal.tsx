/**
 * ShareCalendarModal — «Compartir calendario» (ACL). Solo el DUEÑO: se abre desde
 * el kebab de un calendario propio. Comparte por correo y lista con-quién-está-
 * compartido con revocar. El servicio Go verifica la propiedad y casa el
 * destinatario contra el email/sub verificado del JWT al acceder.
 *
 * Configuración guiada: el nivel de acceso se elige por lo que PERMITE HACER
 * (ChoiceCard: ver / ver y editar / llevar tu agenda), no por su nombre técnico, y
 * se avisa de la consecuencia antes de compartir. Accesible: foco/teclado del
 * Modal, envío con Enter, ayuda y error en el mismo FieldHint (aria-describedby);
 * el permiso concedido se muestra con TEXTO, no solo color (WCAG AA).
 */
import type { JSX } from 'preact';
import { useEffect, useState } from 'preact/hooks';
import {
  Badge,
  Button,
  ChoiceCard,
  EmptyState,
  FieldGroup,
  FieldHint,
  Icon,
  InlineHelp,
  Input,
  Modal,
  useToast,
} from '@sytue/ui';
import type { IconName } from '@sytue/ui';
import {
  listCalendarShares,
  revokeCalendarShare,
  shareCalendar,
  type Calendar,
  type CalendarShare,
  type SharePermission,
} from '../client/calendarApi.js';
import type { CalendarMessageKey, T } from '../i18n/index.js';

/** Validación mínima de email (el servicio Go valida de nuevo en el borde). */
const EMAIL_RE = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;

/**
 * Los tres niveles contados por lo que PERMITEN HACER, no por su nombre: quien
 * comparte decide con la consecuencia delante, sin tener que interpretar «writer».
 */
const PERMISSIONS: ReadonlyArray<{
  value: SharePermission;
  label: CalendarMessageKey;
  hint: CalendarMessageKey;
  icon: IconName;
}> = [
  { value: 'reader', label: 'share.readerLabel', hint: 'share.readerHint', icon: 'user' },
  { value: 'writer', label: 'share.writerLabel', hint: 'share.writerHint', icon: 'users' },
  { value: 'manage', label: 'share.manageLabel', hint: 'share.manageHint', icon: 'lock' },
];

/** Etiqueta de texto del permiso (no solo color) para la lista de shares. */
const PERMISSION_BADGE: Record<SharePermission, CalendarMessageKey> = {
  reader: 'share.badgeReader',
  writer: 'share.badgeWriter',
  manage: 'share.badgeManage',
};

interface Props {
  /** Traductor ya atado al idioma de la petición; baja del padre. */
  t: T;
  /** Calendario propio a compartir, o null (cerrado). */
  calendar: Calendar | null;
  onClose: () => void;
}

export function ShareCalendarModal({ t, calendar, onClose }: Props): JSX.Element | null {
  const toast = useToast();
  const [shares, setShares] = useState<CalendarShare[]>([]);
  const [loading, setLoading] = useState(false);
  const [email, setEmail] = useState('');
  const [permission, setPermission] = useState<SharePermission>('reader');
  const [busy, setBusy] = useState(false);
  const [emailError, setEmailError] = useState<string | null>(null);

  const calendarId = calendar?.id ?? '';

  // Al (re)abrir, carga con quién está compartido y resetea el formulario.
  useEffect(() => {
    if (!calendarId) return;
    setEmail('');
    setPermission('reader');
    setEmailError(null);
    setLoading(true);
    let stale = false;
    void listCalendarShares(calendarId)
      .then(({ shares: list }) => {
        if (!stale) setShares(list ?? []);
      })
      .catch(() => {
        if (!stale) toast.notify({ tone: 'danger', title: t('toast.sharesLoadFailed') });
      })
      .finally(() => {
        if (!stale) setLoading(false);
      });
    return () => {
      stale = true;
    };
  }, [calendarId, t, toast]);

  async function handleShare(): Promise<void> {
    const trimmed = email.trim();
    if (!EMAIL_RE.test(trimmed)) {
      setEmailError(t('share.emailInvalid'));
      document.getElementById('share-email')?.focus();
      return;
    }
    setBusy(true);
    try {
      await shareCalendar(calendarId, trimmed, permission);
      const { shares: list } = await listCalendarShares(calendarId);
      setShares(list ?? []);
      setEmail('');
      setPermission('reader');
      toast.notify({ tone: 'success', title: t('toast.calendarShared') });
    } catch {
      toast.notify({ tone: 'danger', title: t('toast.shareFailed') });
    } finally {
      setBusy(false);
    }
  }

  async function handleRevoke(shareId: string): Promise<void> {
    try {
      await revokeCalendarShare(calendarId, shareId);
      setShares((prev) => prev.filter((s) => s.id !== shareId));
      toast.notify({ tone: 'success', title: t('toast.shareRevoked') });
    } catch {
      toast.notify({ tone: 'danger', title: t('toast.revokeFailed') });
    }
  }

  return (
    <Modal
      open={Boolean(calendar)}
      onClose={onClose}
      title={calendar ? t('share.title', { name: calendar.displayName }) : t('share.titleFallback')}
      size="md"
      footer={
        <Button variant="secondary" onClick={onClose}>
          {t('common.done')}
        </Button>
      }
    >
      <div class="flex flex-col gap-5">
        <InlineHelp tone="info" title={t('share.whatHappens')}>
          {t('share.helpBody')}
        </InlineHelp>

        <div class="flex flex-col gap-4">
          {/* El error va en el FieldHint (mismo sitio que la ayuda), no duplicado. */}
          <FieldGroup label={t('share.recipientEmail')} htmlFor="share-email">
            <Input
              id="share-email"
              type="email"
              value={email}
              placeholder="persona@sytue.eu"
              autoComplete="off"
              invalid={Boolean(emailError)}
              aria-describedby="share-email-hint"
              onInput={(e) => {
                setEmail(e.currentTarget.value);
                if (emailError) setEmailError(null);
              }}
              onKeyDown={(e) => {
                if (e.key === 'Enter') {
                  e.preventDefault();
                  void handleShare();
                }
              }}
            />
            <FieldHint id="share-email-hint" tone={emailError ? 'danger' : 'neutral'}>
              {emailError ?? t('share.recipient.hint')}
            </FieldHint>
          </FieldGroup>

          <fieldset class="m-0 flex flex-col gap-2 border-0 p-0">
            <legend class="text-text mb-1 p-0 text-sm font-semibold">{t('share.legend')}</legend>
            {PERMISSIONS.map((p) => (
              <ChoiceCard
                key={p.value}
                dense
                name="share-permission"
                value={p.value}
                title={t(p.label)}
                description={t(p.hint)}
                icon={p.icon}
                selected={permission === p.value}
                onSelect={() => setPermission(p.value)}
              />
            ))}
          </fieldset>

          {permission !== 'reader' ? (
            <InlineHelp tone="warning" title={t('share.canEdit')}>
              {permission === 'manage' ? t('share.role.manage.hint') : t('share.role.write.hint')}
            </InlineHelp>
          ) : null}

          <div class="flex justify-end">
            <Button variant="primary" onClick={() => void handleShare()} loading={busy}>
              {t('share.submit')}
            </Button>
          </div>
        </div>

        <div class="border-border flex flex-col gap-2 border-t pt-4">
          <p class="text-text text-sm font-semibold">{t('share.whoSeesNow')}</p>
          {loading ? (
            <p class="text-text-muted text-sm">{t('common.loading')}</p>
          ) : shares.length === 0 ? (
            <EmptyState
              title={t('share.onlyYou')}
              description={t('share.empty.hint')}
              illustration={<Icon name="lock" size={32} />}
            />
          ) : (
            <ul class="flex flex-col gap-1">
              {shares.map((s) => (
                <li
                  key={s.id}
                  class="border-border flex items-center gap-2 rounded-md border px-2.5 py-2"
                >
                  <span class="text-text min-w-0 flex-1 truncate text-sm" title={s.grantee_email}>
                    {s.grantee_email}
                  </span>
                  <Badge tone={s.permission === 'reader' ? 'neutral' : 'primary'} class="shrink-0">
                    {t(PERMISSION_BADGE[s.permission])}
                  </Badge>
                  <Button
                    variant="ghost"
                    size="sm"
                    iconOnly
                    aria-label={t('share.revoke', { email: s.grantee_email })}
                    title={t('share.revokeAction')}
                    onClick={() => void handleRevoke(s.id)}
                    class="shrink-0"
                    leadingIcon={<Icon name="close" size={16} />}
                  />
                </li>
              ))}
            </ul>
          )}
        </div>
      </div>
    </Modal>
  );
}
