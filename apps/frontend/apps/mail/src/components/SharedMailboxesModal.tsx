/**
 * SharedMailboxesModal — gestión de buzones compartidos y delegación.
 *
 *  - Delegar MI buzón: conceder a otra persona lectura o enviar-en-mi-nombre sobre
 *    mi propia cuenta (asistente ↔ directivo), y retirarlo.
 *  - Buzones compartidos (soporte@, ventas@): crear uno nuevo (quedo como `manage`)
 *    y, en los que gestiono, administrar su ACL (miembros y permisos).
 *
 * Toda la autorización la impone el servicio (deny-by-default): solo se ven/gestionan
 * buzones con ACL vigente; conceder acceso exige `manage`. Aquí solo el CRUD vía BFF.
 * docs/CONVENTIONS.md §5.
 */
import type { JSX } from 'preact';
import { useEffect, useState } from 'preact/hooks';
import {
  Button,
  ChoiceCard,
  EmptyState,
  FieldGroup,
  FieldHint,
  InlineHelp,
  Input,
  Modal,
  Icon,
  SettingsSection,
  Spinner,
  useToast,
} from '@sytue/ui';
import type { IconName } from '@sytue/ui';
import {
  listSharedMailboxes,
  createSharedMailbox,
  listGrants,
  grantAccess,
  revokeAccess,
  listDelegation,
  delegateAccess,
  revokeDelegation,
  type SharedMailbox,
  type MailGrant,
  type MailPermission,
} from '../client/mailApi.js';
import type { Locale } from '@sytue/i18n';
import { useT, type MailMessageKey, type T } from '../i18n/index.js';

interface Props {
  /** Idioma ya negociado en el servidor; llega por props desde la isla raíz. */
  locale: Locale;
  open: boolean;
  /** Cambió algún acceso/buzón: el padre refresca el sidebar y el selector «De». */
  onChanged?: () => void;
  onClose: () => void;
}

const EMAIL_RE = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;

/** Etiqueta del permiso según el contexto (delegación vs buzón compartido). */
function permLabel(t: T, p: MailPermission, delegated: boolean): string {
  if (p === 'read') return t('shared.permRead');
  if (p === 'manage') return t('shared.permManage');
  return delegated ? t('shared.permSendOnBehalf') : t('shared.permSendAs');
}

/**
 * Cada permiso contado por su CONSECUENCIA, no por su nombre técnico: quien
 * concede acceso tiene que saber exactamente qué podrá hacer la otra persona.
 * Solo CLAVES: los mapas son de módulo y el texto lo resuelve la `t` del padre.
 */
interface PermChoice {
  value: MailPermission;
  title: MailMessageKey;
  description: MailMessageKey;
  icon: IconName;
}

const DELEGATE_PERMS: PermChoice[] = [
  {
    value: 'read',
    title: 'shared.delegateReadTitle',
    description: 'shared.delegateReadDescription',
    icon: 'user',
  },
  {
    value: 'send',
    title: 'shared.delegateSendTitle',
    description: 'shared.delegateSendDescription',
    icon: 'users',
  },
];

const MAILBOX_PERMS: PermChoice[] = [
  {
    value: 'read',
    title: 'shared.mailboxReadTitle',
    description: 'shared.mailboxReadDescription',
    icon: 'user',
  },
  {
    value: 'send',
    title: 'shared.mailboxSendTitle',
    description: 'shared.mailboxSendDescription',
    icon: 'users',
  },
  {
    value: 'manage',
    title: 'shared.mailboxManageTitle',
    description: 'shared.mailboxManageDescription',
    icon: 'lock',
  },
];

/** Selector de permiso con la consecuencia de cada nivel a la vista. */
function PermissionChoice({
  t,
  name,
  options,
  value,
  onChange,
}: {
  t: T;
  name: string;
  options: PermChoice[];
  value: MailPermission;
  onChange: (p: MailPermission) => void;
}): JSX.Element {
  return (
    <fieldset class="m-0 flex flex-col gap-2 border-0 p-0">
      <legend class="text-text mb-1 p-0 text-sm font-semibold">{t('shared.permGroup')}</legend>
      {options.map((o) => (
        <ChoiceCard
          key={o.value}
          dense
          name={name}
          value={o.value}
          title={t(o.title)}
          description={t(o.description)}
          icon={o.icon}
          selected={value === o.value}
          onSelect={() => onChange(o.value)}
        />
      ))}
    </fieldset>
  );
}

export function SharedMailboxesModal({
  locale,
  open,
  onChanged,
  onClose,
}: Props): JSX.Element | null {
  const t = useT(locale);
  const [shared, setShared] = useState<SharedMailbox[]>([]);
  const [delegates, setDelegates] = useState<MailGrant[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState(false);
  const [reload, setReload] = useState(0);

  // Buzón compartido cuyo ACL se está editando (de los que gestiono).
  const [editing, setEditing] = useState<SharedMailbox | null>(null);
  const managed = shared.filter((m) => m.kind === 'shared' && m.permission === 'manage');

  useEffect(() => {
    if (!open) return;
    let active = true;
    setLoading(true);
    setError(false);
    setEditing(null);
    Promise.all([listSharedMailboxes(), listDelegation()])
      .then(([sm, del]) => {
        if (!active) return;
        setShared(sm.sharedMailboxes);
        setDelegates(del.grants);
      })
      .catch(() => active && setError(true))
      .finally(() => active && setLoading(false));
    return () => {
      active = false;
    };
  }, [open, reload]);

  function refreshAll(): void {
    setReload((n) => n + 1);
    onChanged?.();
  }

  return (
    <Modal
      open={open}
      onClose={onClose}
      title={t('shared.title')}
      size="lg"
      footer={
        <>
          <Button variant="secondary" onClick={onClose}>
            {t('common.close')}
          </Button>
        </>
      }
    >
      {loading ? (
        <div class="text-text-subtle flex items-center gap-2 py-8">
          <Spinner size={16} label={t('common.loading')} />
          <span class="text-sm" aria-hidden="true">
            {t('common.loading')}
          </span>
        </div>
      ) : error ? (
        <div class="flex flex-col items-start gap-2 py-4" role="alert">
          <p class="text-text flex items-start gap-2 text-sm">
            <Icon name="warning" size={16} class="mt-0.5 shrink-0" />
            {t('shared.loadError')}
          </p>
          <Button size="sm" variant="secondary" onClick={() => setReload((n) => n + 1)}>
            {t('common.retry')}
          </Button>
        </div>
      ) : editing ? (
        <AclEditor t={t} mailbox={editing} onBack={() => setEditing(null)} onChanged={refreshAll} />
      ) : (
        <div class="flex flex-col gap-8">
          <DelegationSection
            t={t}
            delegates={delegates}
            onChanged={(list) => {
              setDelegates(list);
              onChanged?.();
            }}
          />
          <SharedSection t={t} managed={managed} onEdit={setEditing} onCreated={refreshAll} />
        </div>
      )}
    </Modal>
  );
}

/** Delegación de MI buzón: personas con acceso a mi cuenta. */
function DelegationSection({
  t,
  delegates,
  onChanged,
}: {
  t: T;
  delegates: MailGrant[];
  onChanged: (list: MailGrant[]) => void;
}): JSX.Element {
  const toast = useToast();
  const [email, setEmail] = useState('');
  const [perm, setPerm] = useState<MailPermission>('read');
  const [saving, setSaving] = useState(false);
  const canAdd = EMAIL_RE.test(email.trim());
  // Solo se señala el error cuando ya se ha escrito algo (no se regaña en vacío).
  const showError = email.trim() !== '' && !canAdd;

  async function handleAdd(): Promise<void> {
    const grantee = email.trim();
    if (!canAdd || saving) return;
    setSaving(true);
    try {
      await delegateAccess(grantee, perm);
      const next = [
        ...delegates.filter((g) => g.grantee !== grantee),
        { grantee, permission: perm },
      ];
      onChanged(next);
      setEmail('');
      toast.notify({ tone: 'success', title: t('shared.delegated') });
    } catch {
      toast.notify({
        tone: 'danger',
        title: t('shared.delegateFailed'),
        description: t('shared.delegateFailedDescription'),
      });
    } finally {
      setSaving(false);
    }
  }

  function handleRemove(g: MailGrant): void {
    const prev = delegates;
    onChanged(delegates.filter((x) => x.grantee !== g.grantee));
    void revokeDelegation(g.grantee).catch(() => {
      onChanged(prev);
      toast.notify({ tone: 'danger', title: t('shared.revokeDelegateFailed') });
    });
  }

  return (
    <SettingsSection
      plain
      title={t('shared.delegateTitle')}
      description={t('shared.delegateDescription')}
    >
      {delegates.length > 0 ? (
        <ul class="divide-border border-border flex flex-col divide-y rounded-md border">
          {delegates.map((g) => (
            <li key={g.grantee} class="flex items-center justify-between gap-3 px-3 py-2.5">
              <span class="text-text truncate text-sm">{g.grantee}</span>
              <div class="flex shrink-0 items-center gap-2">
                <span class="sytue-meta">{permLabel(t, g.permission, true)}</span>
                <Button
                  variant="ghost"
                  size="sm"
                  iconOnly
                  class="text-text-subtle hover:text-danger-text"
                  aria-label={t('shared.revokeDelegate', { person: g.grantee })}
                  onClick={() => handleRemove(g)}
                >
                  <Icon name="close" size={16} />
                </Button>
              </div>
            </li>
          ))}
        </ul>
      ) : (
        <EmptyState
          title={t('shared.delegateEmptyTitle')}
          description={t('shared.delegateEmptyDescription')}
          illustration={<Icon name="lock" size={32} />}
        />
      )}
      <div class="flex flex-col gap-3">
        <FieldGroup label={t('shared.person')} htmlFor="delegate-email">
          <Input
            id="delegate-email"
            type="email"
            value={email}
            placeholder={t('shared.personPlaceholder')}
            invalid={showError}
            aria-describedby="delegate-email-hint"
            onInput={(e) => setEmail(e.currentTarget.value)}
            onKeyDown={(e) => {
              if (e.key === 'Enter' && canAdd) void handleAdd();
            }}
          />
          <FieldHint id="delegate-email-hint" tone={showError ? 'danger' : 'neutral'}>
            {showError ? t('shared.personHintError') : t('shared.personHint')}
          </FieldHint>
        </FieldGroup>
        <PermissionChoice
          t={t}
          name="delegate-perm"
          options={DELEGATE_PERMS}
          value={perm}
          onChange={setPerm}
        />
        <div class="flex justify-end">
          <Button
            variant="primary"
            leadingIcon={<Icon name="plus" />}
            loading={saving}
            disabled={!canAdd}
            onClick={() => void handleAdd()}
          >
            {t('shared.grantAccess')}
          </Button>
        </div>
      </div>
    </SettingsSection>
  );
}

/** Buzones compartidos de la organización: crear + entrar a gestionar el ACL. */
function SharedSection({
  t,
  managed,
  onEdit,
  onCreated,
}: {
  t: T;
  managed: SharedMailbox[];
  onEdit: (m: SharedMailbox) => void;
  onCreated: () => void;
}): JSX.Element {
  const toast = useToast();
  const [address, setAddress] = useState('');
  const [name, setName] = useState('');
  const [saving, setSaving] = useState(false);
  const canCreate = EMAIL_RE.test(address.trim());
  const showError = address.trim() !== '' && !canCreate;

  async function handleCreate(): Promise<void> {
    if (!canCreate || saving) return;
    setSaving(true);
    try {
      await createSharedMailbox(address.trim(), name.trim());
      setAddress('');
      setName('');
      onCreated();
      toast.notify({ tone: 'success', title: t('shared.created') });
    } catch {
      toast.notify({
        tone: 'danger',
        title: t('shared.createFailed'),
        description: t('shared.createFailedDescription'),
      });
    } finally {
      setSaving(false);
    }
  }

  return (
    <SettingsSection
      plain
      title={t('shared.managedTitle')}
      description={t('shared.managedDescription')}
      class="border-border border-t pt-6"
    >
      {managed.length > 0 ? (
        <ul class="divide-border border-border flex flex-col divide-y rounded-md border">
          {managed.map((m) => (
            <li key={m.accountId} class="flex items-center justify-between gap-3 px-3 py-2.5">
              <span class="text-text truncate text-sm font-medium">{m.address}</span>
              <Button variant="secondary" size="sm" class="shrink-0" onClick={() => onEdit(m)}>
                {t('shared.manageAccess')}
              </Button>
            </li>
          ))}
        </ul>
      ) : (
        <EmptyState
          title={t('shared.managedEmptyTitle')}
          description={t('shared.managedEmptyDescription')}
          illustration={<Icon name="users" size={32} />}
        />
      )}
      <div class="flex flex-col gap-3">
        <div class="grid gap-3 sm:grid-cols-2">
          <FieldGroup label={t('common.address')} htmlFor="shared-address">
            <Input
              id="shared-address"
              type="email"
              value={address}
              placeholder={t('shared.addressPlaceholder')}
              invalid={showError}
              aria-describedby="shared-address-hint"
              onInput={(e) => setAddress(e.currentTarget.value)}
            />
            <FieldHint id="shared-address-hint" tone={showError ? 'danger' : 'neutral'}>
              {showError ? t('shared.addressHintError') : t('shared.addressHint')}
            </FieldHint>
          </FieldGroup>
          <FieldGroup label={t('common.name')} htmlFor="shared-name">
            <Input
              id="shared-name"
              type="text"
              value={name}
              placeholder={t('shared.namePlaceholder')}
              aria-describedby="shared-name-hint"
              onInput={(e) => setName(e.currentTarget.value)}
            />
            <FieldHint id="shared-name-hint">{t('shared.nameHint')}</FieldHint>
          </FieldGroup>
        </div>
        <div class="flex justify-end">
          <Button
            variant="primary"
            leadingIcon={<Icon name="plus" />}
            loading={saving}
            disabled={!canCreate}
            onClick={() => void handleCreate()}
          >
            {t('shared.createMailbox')}
          </Button>
        </div>
      </div>
    </SettingsSection>
  );
}

/** Editor del ACL de un buzón compartido concreto (miembros + permisos). */
function AclEditor({
  t,
  mailbox,
  onBack,
  onChanged,
}: {
  t: T;
  mailbox: SharedMailbox;
  onBack: () => void;
  onChanged: () => void;
}): JSX.Element {
  const toast = useToast();
  const [grants, setGrants] = useState<MailGrant[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState(false);
  const [email, setEmail] = useState('');
  const [perm, setPerm] = useState<MailPermission>('read');
  const [saving, setSaving] = useState(false);
  const [reload, setReload] = useState(0);
  const canAdd = EMAIL_RE.test(email.trim());
  const showError = email.trim() !== '' && !canAdd;

  useEffect(() => {
    let active = true;
    setLoading(true);
    setError(false);
    listGrants(mailbox.accountId)
      .then(({ grants: list }) => active && setGrants(list))
      .catch(() => active && setError(true))
      .finally(() => active && setLoading(false));
    return () => {
      active = false;
    };
  }, [mailbox.accountId, reload]);

  async function handleAdd(): Promise<void> {
    const grantee = email.trim();
    if (!canAdd || saving) return;
    setSaving(true);
    try {
      await grantAccess(mailbox.accountId, grantee, perm);
      setEmail('');
      setReload((n) => n + 1);
      onChanged();
      toast.notify({ tone: 'success', title: t('shared.accessGranted') });
    } catch {
      toast.notify({
        tone: 'danger',
        title: t('shared.grantFailed'),
        description: t('shared.grantFailedDescription'),
      });
    } finally {
      setSaving(false);
    }
  }

  function handleRemove(g: MailGrant): void {
    const prev = grants;
    setGrants((list) => list.filter((x) => x.grantee !== g.grantee));
    void revokeAccess(mailbox.accountId, g.grantee)
      .then(() => onChanged())
      .catch(() => {
        setGrants(prev);
        toast.notify({ tone: 'danger', title: t('shared.revokeFailed') });
      });
  }

  return (
    <div class="flex flex-col gap-4">
      <div class="flex items-center gap-2">
        <Button variant="ghost" size="sm" iconOnly aria-label={t('common.back')} onClick={onBack}>
          <Icon name="chevronLeft" size={18} />
        </Button>
        <div class="min-w-0">
          <p class="sytue-overline text-text-subtle">{t('shared.aclHeading')}</p>
          <p class="text-text truncate text-sm font-medium">{mailbox.address}</p>
        </div>
      </div>

      {loading ? (
        <div class="text-text-subtle flex items-center gap-2 py-4">
          <Spinner size={16} label={t('shared.aclLoading')} />
          <span class="text-sm" aria-hidden="true">
            {t('common.loading')}
          </span>
        </div>
      ) : error ? (
        <div class="flex flex-col items-start gap-2 py-2" role="alert">
          <p class="text-text flex items-start gap-2 text-sm">
            <Icon name="warning" size={16} class="mt-0.5 shrink-0" />
            {t('shared.aclLoadError')}
          </p>
          <Button size="sm" variant="secondary" onClick={() => setReload((n) => n + 1)}>
            {t('common.retry')}
          </Button>
        </div>
      ) : grants.length === 0 ? (
        <EmptyState
          title={t('shared.aclEmptyTitle')}
          description={t('shared.aclEmptyDescription')}
          illustration={<Icon name="users" size={32} />}
        />
      ) : (
        <ul class="divide-border border-border flex flex-col divide-y rounded-md border">
          {grants.map((g) => (
            <li key={g.grantee} class="flex items-center justify-between gap-3 px-3 py-2.5">
              <span class="text-text truncate text-sm">{g.grantee}</span>
              <div class="flex shrink-0 items-center gap-2">
                <span class="sytue-meta">{permLabel(t, g.permission, false)}</span>
                <Button
                  variant="ghost"
                  size="sm"
                  iconOnly
                  class="text-text-subtle hover:text-danger-text"
                  aria-label={t('shared.revokeAccess', { person: g.grantee })}
                  onClick={() => handleRemove(g)}
                >
                  <Icon name="close" size={16} />
                </Button>
              </div>
            </li>
          ))}
        </ul>
      )}

      <div class="border-border flex flex-col gap-3 border-t pt-4">
        <FieldGroup label={t('shared.addPerson')} htmlFor="acl-email">
          <Input
            id="acl-email"
            type="email"
            value={email}
            placeholder={t('shared.aclPlaceholder')}
            invalid={showError}
            aria-describedby="acl-email-hint"
            onInput={(e) => setEmail(e.currentTarget.value)}
            onKeyDown={(e) => {
              if (e.key === 'Enter' && canAdd) void handleAdd();
            }}
          />
          <FieldHint id="acl-email-hint" tone={showError ? 'danger' : 'neutral'}>
            {showError ? t('shared.aclHintError') : t('shared.personHint')}
          </FieldHint>
        </FieldGroup>
        <PermissionChoice
          t={t}
          name="acl-perm"
          options={MAILBOX_PERMS}
          value={perm}
          onChange={setPerm}
        />
        {perm === 'manage' ? (
          <InlineHelp tone="warning" title={t('shared.manageWarningTitle')}>
            {t('shared.manageWarningBody')}
          </InlineHelp>
        ) : null}
        <div class="flex justify-end">
          <Button
            variant="primary"
            leadingIcon={<Icon name="plus" />}
            loading={saving}
            disabled={!canAdd}
            onClick={() => void handleAdd()}
          >
            {t('shared.grantAccess')}
          </Button>
        </div>
      </div>
    </div>
  );
}
