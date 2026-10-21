/**
 * PrivilegedAccessPanel — PIM (Privileged Identity Management, paridad Entra PIM).
 * Elevación de privilegios just-in-time sobre los roles personalizados: un usuario
 * ELEGIBLE para un rol lo ACTIVA por una ventana acotada (con justificación) y, si la
 * elegibilidad lo exige, previa aprobación. Solo mientras hay una activación ACTIVA sus
 * permisos entran en el token (el enforcement vive en Sytue ID); esto es su plano de
 * administración + autoservicio. Dos vistas: «Mi acceso» (activar/desactivar lo elegible)
 * y «Administración» (definir elegibilidades, aprobar solicitudes, ver vigentes).
 *
 * Todo pasa por el BFF (/api/me/pim*, /api/admin/pim/*, cookie httpOnly; el token nunca
 * está aquí). Solo @sytue/ui, Graphite & Brass, denso, oscuro, WCAG AA.
 */
import { useMemo, useState } from 'preact/hooks';
import {
  Badge,
  Button,
  Card,
  CardBody,
  Checkbox,
  EmptyState,
  FieldGroup,
  Icon,
  Input,
  Modal,
  SectionHeader,
  Select,
  Tabs,
  Textarea,
  ToastProvider,
  useToast,
} from '@sytue/ui';
import type { BadgeTone } from '@sytue/ui';
import { formatNumber, type Locale } from '@sytue/i18n';
import { useT, type T } from '../i18n/index.js';
import type { ConsoleMessageKey } from '../i18n/index.js';
import { formatWhen } from '../lib/format.js';
import type { PimActivation, PimEligibility } from '../lib/types.js';

interface DirectoryRef {
  id: string;
  email: string;
  name: string;
}

interface RoleRef {
  id: string;
  name: string;
}

interface PrivilegedAccessPanelProps {
  /** Idioma negociado en el servidor; llega por props desde el `.astro`. */
  locale: Locale;
  /** Vista de usuario: para qué roles soy elegible y mis activaciones. */
  myEligibilities: PimEligibility[];
  myActivations: PimActivation[];
  /** Vista admin: elegibilidades definidas y activaciones (todas). */
  eligibilities: PimEligibility[];
  activations: PimActivation[];
  /** Directorio (para el picker de usuario) y roles personalizados (para el de rol). */
  users: DirectoryRef[];
  roles: RoleRef[];
  /** false ⇒ sin backend vivo (no se puede cargar ni operar). */
  live?: boolean;
}

const STATUS_KEY: Record<string, ConsoleMessageKey> = {
  pending: 'pim.status.pending',
  active: 'pim.status.active',
  revoked: 'pim.status.revoked',
  denied: 'pim.status.denied',
  expired: 'pim.status.expired',
};

function statusTone(s: string): BadgeTone {
  if (s === 'active') return 'success';
  if (s === 'pending') return 'warning';
  if (s === 'denied' || s === 'revoked') return 'danger';
  return 'neutral';
}

function statusLabel(s: string, t: T): string {
  const key = STATUS_KEY[s];
  return key ? t(key) : s;
}

function errorCode(err: unknown): string {
  return err instanceof Error ? err.message : '';
}

/** Minutos restantes hasta expiresAt (0 si ya pasó o no aplica). Estático al render. */
function minutesLeft(expiresAt?: string): number {
  if (!expiresAt) return 0;
  return Math.max(0, Math.round((new Date(expiresAt).getTime() - Date.now()) / 60000));
}

/** Valor del picker de rol: 'builtin:<role>' para built-in, o el uuid del personalizado. */
function roleRefFromValue(value: string): { role_id?: string; builtin_role?: string } {
  return value.startsWith('builtin:')
    ? { builtin_role: value.slice('builtin:'.length) }
    : { role_id: value };
}

function Inner({
  locale,
  myEligibilities,
  myActivations,
  eligibilities,
  activations,
  users,
  roles,
  live = true,
}: PrivilegedAccessPanelProps) {
  const t = useT(locale);
  const toast = useToast();

  // Estado self.
  const [myActs, setMyActs] = useState<PimActivation[]>(myActivations);
  const [activateFor, setActivateFor] = useState<PimEligibility | null>(null);
  const [justification, setJustification] = useState('');
  const [duration, setDuration] = useState(30);
  const [activating, setActivating] = useState(false);

  // Estado admin.
  const [eligs, setEligs] = useState<PimEligibility[]>(eligibilities);
  const [acts, setActs] = useState<PimActivation[]>(activations);
  const [busyId, setBusyId] = useState('');
  const [newOpen, setNewOpen] = useState(false);
  const [newUserId, setNewUserId] = useState('');
  const [newRoleValue, setNewRoleValue] = useState('');
  const [newRequiresApproval, setNewRequiresApproval] = useState(false);
  const [newMaxDuration, setNewMaxDuration] = useState(60);
  const [creating, setCreating] = useState(false);

  const pending = useMemo(() => acts.filter((a) => a.status === 'pending'), [acts]);
  const activeNow = useMemo(() => acts.filter((a) => a.effectiveStatus === 'active'), [acts]);

  function openActivate(e: PimEligibility) {
    setActivateFor(e);
    setJustification('');
    setDuration(Math.min(30, e.maxDurationMinutes));
  }

  async function submitActivate(ev: Event) {
    ev.preventDefault();
    if (!activateFor || activating) return;
    const reason = justification.trim();
    if (!reason) return;
    setActivating(true);
    try {
      const res = await fetch('/api/me/pim', {
        method: 'POST',
        headers: { 'content-type': 'application/json' },
        body: JSON.stringify({
          action: 'activate',
          ...roleRefFromValue(
            activateFor.roleId ? activateFor.roleId : `builtin:${activateFor.builtinRole}`,
          ),
          justification: reason,
          duration_minutes: duration,
        }),
      });
      if (!res.ok) throw new Error(String(res.status));
      const act = (await res.json()) as PimActivation;
      setMyActs((prev) => [act, ...prev]);
      toast.notify({
        tone: 'success',
        title: t(act.status === 'pending' ? 'pim.toast.requested' : 'pim.toast.activated'),
        description:
          act.status === 'pending'
            ? t('pim.toast.requestedHint')
            : t('pim.toast.activatedHint', { minutes: formatNumber(locale, act.durationMinutes) }),
      });
      setActivateFor(null);
    } catch (err) {
      toast.notify({
        tone: 'danger',
        title: t('pim.toast.activateFailed'),
        description:
          errorCode(err) === '403'
            ? t('pim.toast.notEligible')
            : errorCode(err) === '400'
              ? t('pim.toast.checkDuration')
              : t('common.tryAgain'),
      });
    } finally {
      setActivating(false);
    }
  }

  async function deactivate(a: PimActivation) {
    if (busyId) return;
    setBusyId(a.id);
    try {
      const res = await fetch('/api/me/pim', {
        method: 'POST',
        headers: { 'content-type': 'application/json' },
        body: JSON.stringify({ action: 'deactivate', activation_id: a.id }),
      });
      if (!res.ok) throw new Error(String(res.status));
      setMyActs((prev) =>
        prev.map((it) =>
          it.id === a.id ? { ...it, status: 'revoked', effectiveStatus: 'revoked' } : it,
        ),
      );
      toast.notify({ tone: 'success', title: t('pim.toast.deactivated') });
    } catch {
      toast.notify({
        tone: 'danger',
        title: t('pim.toast.deactivateFailed'),
        description: t('common.tryAgain'),
      });
    } finally {
      setBusyId('');
    }
  }

  async function decide(a: PimActivation, action: 'approve' | 'deny') {
    if (busyId) return;
    setBusyId(a.id);
    try {
      const res = await fetch('/api/admin/pim/activations', {
        method: 'POST',
        headers: { 'content-type': 'application/json' },
        body: JSON.stringify({ activation_id: a.id, action }),
      });
      if (!res.ok) throw new Error(String(res.status));
      if (action === 'approve') {
        const updated = (await res.json()) as PimActivation;
        setActs((prev) => prev.map((it) => (it.id === a.id ? updated : it)));
        toast.notify({ tone: 'success', title: t('pim.toast.approved') });
      } else {
        setActs((prev) =>
          prev.map((it) =>
            it.id === a.id ? { ...it, status: 'denied', effectiveStatus: 'denied' } : it,
          ),
        );
        toast.notify({ tone: 'success', title: t('pim.toast.denied') });
      }
    } catch {
      toast.notify({
        tone: 'danger',
        title: t('pim.toast.decideFailed'),
        description: t('common.tryAgain'),
      });
    } finally {
      setBusyId('');
    }
  }

  async function createEligibility(ev: Event) {
    ev.preventDefault();
    if (creating || !newUserId || !newRoleValue) return;
    setCreating(true);
    try {
      const res = await fetch('/api/admin/pim/eligibilities', {
        method: 'POST',
        headers: { 'content-type': 'application/json' },
        body: JSON.stringify({
          user_id: newUserId,
          ...roleRefFromValue(newRoleValue),
          requires_approval: newRequiresApproval,
          max_duration_minutes: newMaxDuration,
        }),
      });
      if (!res.ok) throw new Error(String(res.status));
      const elig = (await res.json()) as PimEligibility;
      setEligs((prev) => [elig, ...prev]);
      toast.notify({ tone: 'success', title: t('pim.toast.eligCreated') });
      setNewOpen(false);
      setNewUserId('');
      setNewRoleValue('');
      setNewRequiresApproval(false);
      setNewMaxDuration(60);
    } catch (err) {
      toast.notify({
        tone: 'danger',
        title: t('pim.toast.eligCreateFailed'),
        description:
          errorCode(err) === '409'
            ? t('pim.toast.eligDuplicate')
            : errorCode(err) === '400'
              ? t('pim.toast.eligCheckFields')
              : t('common.tryAgain'),
      });
    } finally {
      setCreating(false);
    }
  }

  async function deleteEligibility(e: PimEligibility) {
    if (busyId) return;
    setBusyId(e.id);
    try {
      const res = await fetch(`/api/admin/pim/eligibilities/${encodeURIComponent(e.id)}`, {
        method: 'DELETE',
      });
      if (!res.ok) throw new Error(String(res.status));
      setEligs((prev) => prev.filter((it) => it.id !== e.id));
      toast.notify({ tone: 'success', title: t('pim.toast.eligRemoved') });
    } catch {
      toast.notify({
        tone: 'danger',
        title: t('pim.toast.eligRemoveFailed'),
        description: t('common.tryAgain'),
      });
    } finally {
      setBusyId('');
    }
  }

  // ---- Render de una fila de activación (compartida self/admin) ----
  function ActivationRow({ a, mine }: { a: PimActivation; mine: boolean }) {
    const left = minutesLeft(a.expiresAt);
    return (
      <li class="border-border bg-surface flex flex-wrap items-center gap-x-3 gap-y-1 rounded-md border px-3 py-2">
        <Icon name="lock" size={16} class="text-text-subtle shrink-0" aria-hidden="true" />
        <span class="text-text min-w-0 flex-1 truncate font-medium" title={a.roleName}>
          {a.roleName}
        </span>
        {!mine ? (
          <span class="text-text-muted min-w-0 truncate text-xs" title={a.userEmail}>
            {a.userEmail}
          </span>
        ) : null}
        <Badge tone={statusTone(a.effectiveStatus)}>{statusLabel(a.effectiveStatus, t)}</Badge>
        {a.effectiveStatus === 'active' ? (
          <span
            class="sytue-meta"
            title={t('pim.expiresAt', { when: formatWhen(locale, a.expiresAt) })}
          >
            {t('pim.minutesLeft', { minutes: formatNumber(locale, left) })}
          </span>
        ) : null}
        {mine && a.effectiveStatus === 'active' ? (
          <Button
            variant="ghost"
            size="sm"
            loading={busyId === a.id}
            onClick={() => void deactivate(a)}
          >
            {t('pim.deactivate')}
          </Button>
        ) : null}
      </li>
    );
  }

  const meTab = (
    <div class="flex flex-col gap-5">
      <section>
        <h3 class="text-text mb-2 text-sm font-semibold">{t('pim.me.eligibleTitle')}</h3>
        {myEligibilities.length === 0 ? (
          <Card>
            <CardBody class="p-0">
              <EmptyState
                title={t('pim.me.emptyTitle')}
                description={t('pim.me.emptyDescription')}
                illustration={<Icon name="lock" size={28} />}
              />
            </CardBody>
          </Card>
        ) : (
          <div class="grid gap-3 md:grid-cols-2 xl:grid-cols-3">
            {myEligibilities.map((e) => (
              <Card key={e.id}>
                <CardBody class="flex flex-col gap-2 p-4">
                  <span class="flex items-center gap-2">
                    <Icon
                      name="lock"
                      size={16}
                      class="text-text-subtle shrink-0"
                      aria-hidden="true"
                    />
                    <span class="text-text min-w-0 flex-1 truncate font-medium" title={e.roleName}>
                      {e.roleName}
                    </span>
                  </span>
                  <span class="flex flex-wrap gap-1">
                    <Badge tone={e.requiresApproval ? 'warning' : 'accent'}>
                      {t(e.requiresApproval ? 'pim.requiresApproval' : 'pim.directActivation')}
                    </Badge>
                    <Badge tone="neutral">
                      {t('pim.maxDuration', {
                        minutes: formatNumber(locale, e.maxDurationMinutes),
                      })}
                    </Badge>
                  </span>
                  <Button
                    variant="primary"
                    size="sm"
                    class="mt-1 self-start"
                    disabled={!live}
                    leadingIcon={<Icon name="plus" size={14} />}
                    onClick={() => openActivate(e)}
                  >
                    {t('pim.activate')}
                  </Button>
                </CardBody>
              </Card>
            ))}
          </div>
        )}
      </section>

      <section>
        <h3 class="text-text mb-2 text-sm font-semibold">{t('pim.me.activationsTitle')}</h3>
        {myActs.length === 0 ? (
          <p class="text-text-subtle text-sm">{t('pim.me.noActivations')}</p>
        ) : (
          <ul class="flex flex-col gap-1.5">
            {myActs.map((a) => (
              <ActivationRow key={a.id} a={a} mine />
            ))}
          </ul>
        )}
      </section>
    </div>
  );

  const adminTab = (
    <div class="flex flex-col gap-5">
      <section>
        <div class="mb-2 flex items-center gap-2">
          <h3 class="text-text text-sm font-semibold">{t('pim.admin.pendingTitle')}</h3>
          {pending.length ? (
            <Badge tone="warning">{formatNumber(locale, pending.length)}</Badge>
          ) : null}
        </div>
        {pending.length === 0 ? (
          <p class="text-text-subtle text-sm">{t('pim.admin.noPending')}</p>
        ) : (
          <ul class="flex flex-col gap-1.5">
            {pending.map((a) => (
              <li
                key={a.id}
                class="border-border bg-surface flex flex-wrap items-center gap-x-3 gap-y-1 rounded-md border px-3 py-2"
              >
                <span class="min-w-0 flex-1">
                  <span class="text-text block truncate text-sm font-medium">
                    {a.roleName} · {a.userEmail}
                  </span>
                  <span class="sytue-meta block truncate" title={a.justification}>
                    {t('pim.minutes', { minutes: formatNumber(locale, a.durationMinutes) })} ·{' '}
                    {a.justification || t('pim.noJustification')}
                  </span>
                </span>
                <Button
                  variant="primary"
                  size="sm"
                  loading={busyId === a.id}
                  onClick={() => void decide(a, 'approve')}
                >
                  {t('pim.approve')}
                </Button>
                <Button
                  variant="ghost"
                  size="sm"
                  class="text-danger-text"
                  loading={busyId === a.id}
                  onClick={() => void decide(a, 'deny')}
                >
                  {t('pim.deny')}
                </Button>
              </li>
            ))}
          </ul>
        )}
      </section>

      <section>
        <h3 class="text-text mb-2 text-sm font-semibold">{t('pim.admin.activeTitle')}</h3>
        {activeNow.length === 0 ? (
          <p class="text-text-subtle text-sm">{t('pim.admin.noActive')}</p>
        ) : (
          <ul class="flex flex-col gap-1.5">
            {activeNow.map((a) => (
              <ActivationRow key={a.id} a={a} mine={false} />
            ))}
          </ul>
        )}
      </section>

      <section>
        <div class="mb-2 flex items-center gap-2">
          <h3 class="text-text text-sm font-semibold">{t('pim.admin.eligibilitiesTitle')}</h3>
          <Button
            variant="secondary"
            size="sm"
            class="ml-auto"
            disabled={!live}
            leadingIcon={<Icon name="plus" size={14} />}
            onClick={() => setNewOpen(true)}
          >
            {t('pim.admin.newEligibility')}
          </Button>
        </div>
        {eligs.length === 0 ? (
          <Card>
            <CardBody class="p-0">
              <EmptyState
                title={t('pim.admin.emptyTitle')}
                description={t('pim.admin.emptyDescription')}
                illustration={<Icon name="lock" size={28} />}
                action={
                  <Button
                    variant="primary"
                    size="sm"
                    disabled={!live}
                    leadingIcon={<Icon name="plus" size={14} />}
                    onClick={() => setNewOpen(true)}
                  >
                    {t('pim.admin.newEligibility')}
                  </Button>
                }
              />
            </CardBody>
          </Card>
        ) : (
          <ul class="flex flex-col gap-1.5">
            {eligs.map((e) => (
              <li
                key={e.id}
                class="border-border bg-surface flex flex-wrap items-center gap-x-3 gap-y-1 rounded-md border px-3 py-2"
              >
                <span class="min-w-0 flex-1">
                  <span class="text-text block truncate text-sm font-medium">
                    {e.roleName} · {e.userEmail}
                  </span>
                  <span class="sytue-meta block">
                    {t(e.requiresApproval ? 'pim.withApproval' : 'pim.directActivationShort')} ·{' '}
                    {t('pim.maxDuration', {
                      minutes: formatNumber(locale, e.maxDurationMinutes),
                    })}
                  </span>
                </span>
                <Button
                  variant="ghost"
                  size="sm"
                  class="text-danger-text"
                  aria-label={t('pim.removeEligibilityAria', {
                    user: e.userEmail,
                    role: e.roleName,
                  })}
                  loading={busyId === e.id}
                  onClick={() => void deleteEligibility(e)}
                >
                  <Icon name="close" size={14} />
                </Button>
              </li>
            ))}
          </ul>
        )}
      </section>
    </div>
  );

  return (
    <div class="flex h-full min-h-0 flex-col">
      <SectionHeader
        level={1}
        class="shrink-0"
        title={t('page.pim')}
        subtitle={t('pim.subtitle')}
      />
      {!live ? (
        <p class="border-border bg-warning-bg text-warning-text mx-4 mt-3 rounded-md border px-3 py-2 text-sm">
          {t('pim.offline')}
        </p>
      ) : null}

      <div class="min-h-0 flex-1 overflow-auto p-4">
        <Tabs
          items={[
            {
              id: 'me',
              label: t('pim.tab.me'),
              icon: <Icon name="user" size={16} />,
              content: meTab,
            },
            {
              id: 'admin',
              label: t('pim.tab.admin'),
              icon: <Icon name="settings" size={16} />,
              content: adminTab,
            },
          ]}
        />
      </div>

      {/* Activar un rol elegible */}
      <Modal
        open={Boolean(activateFor)}
        onClose={() => setActivateFor(null)}
        title={
          activateFor
            ? t('pim.activateTitle', { name: activateFor.roleName })
            : t('pim.activateFallbackTitle')
        }
        size="sm"
      >
        {activateFor ? (
          <form class="flex flex-col gap-3" onSubmit={(e) => void submitActivate(e)}>
            {activateFor.requiresApproval ? (
              <p class="border-border bg-warning-bg text-warning-text rounded-md border px-3 py-2 text-xs">
                {t('pim.approvalNotice')}
              </p>
            ) : null}
            <FieldGroup label={t('pim.justification')} htmlFor="pim-justification">
              <Textarea
                id="pim-justification"
                rows={3}
                value={justification}
                placeholder={t('pim.justificationPlaceholder')}
                invalid={justification.trim().length === 0}
                onInput={(e) => setJustification(e.currentTarget.value)}
                required
              />
            </FieldGroup>
            <FieldGroup
              label={t('pim.durationLabel')}
              htmlFor="pim-duration"
              help={t('pim.durationHelp', {
                minutes: formatNumber(locale, activateFor.maxDurationMinutes),
              })}
            >
              <Input
                id="pim-duration"
                type="number"
                min={1}
                max={activateFor.maxDurationMinutes}
                value={String(duration)}
                onInput={(e) => setDuration(Number(e.currentTarget.value) || 0)}
              />
            </FieldGroup>
            <div class="mt-1 flex justify-end gap-2">
              <Button variant="ghost" size="sm" type="button" onClick={() => setActivateFor(null)}>
                {t('common.cancel')}
              </Button>
              <Button
                variant="primary"
                size="sm"
                type="submit"
                loading={activating}
                disabled={
                  justification.trim().length === 0 ||
                  duration < 1 ||
                  duration > activateFor.maxDurationMinutes
                }
              >
                {t(activateFor.requiresApproval ? 'pim.request' : 'pim.activate')}
              </Button>
            </div>
          </form>
        ) : null}
      </Modal>

      {/* Nueva elegibilidad (admin) */}
      <Modal
        open={newOpen}
        onClose={() => setNewOpen(false)}
        title={t('pim.admin.newEligibility')}
        size="sm"
      >
        <form class="flex flex-col gap-3" onSubmit={(e) => void createEligibility(e)}>
          <FieldGroup label={t('pim.newElig.user')} htmlFor="pim-user">
            <Select
              id="pim-user"
              value={newUserId}
              invalid={newUserId === ''}
              onChange={(e) => setNewUserId(e.currentTarget.value)}
            >
              <option value="">{t('pim.newElig.selectUser')}</option>
              {users.map((u) => (
                <option key={u.id} value={u.id}>
                  {u.name} · {u.email}
                </option>
              ))}
            </Select>
          </FieldGroup>
          <FieldGroup label={t('pim.newElig.role')} htmlFor="pim-role">
            <Select
              id="pim-role"
              value={newRoleValue}
              invalid={newRoleValue === ''}
              onChange={(e) => setNewRoleValue(e.currentTarget.value)}
            >
              <option value="">{t('pim.newElig.selectRole')}</option>
              <optgroup label={t('pim.newElig.customRoles')}>
                {roles.map((r) => (
                  <option key={r.id} value={r.id}>
                    {r.name}
                  </option>
                ))}
              </optgroup>
              <optgroup label={t('pim.newElig.builtinRoles')}>
                <option value="builtin:admin">{t('pim.newElig.builtinAdmin')}</option>
                <option value="builtin:owner">{t('pim.newElig.builtinOwner')}</option>
              </optgroup>
            </Select>
          </FieldGroup>
          <FieldGroup
            label={t('pim.newElig.maxLabel')}
            htmlFor="pim-max"
            help={t('pim.newElig.maxHelp')}
          >
            <Input
              id="pim-max"
              type="number"
              min={1}
              max={1440}
              value={String(newMaxDuration)}
              onInput={(e) => setNewMaxDuration(Number(e.currentTarget.value) || 0)}
            />
          </FieldGroup>
          <Checkbox
            label={t('pim.newElig.requiresApproval')}
            checked={newRequiresApproval}
            onChange={(e) => setNewRequiresApproval(e.currentTarget.checked)}
          />
          <div class="mt-1 flex justify-end gap-2">
            <Button variant="ghost" size="sm" type="button" onClick={() => setNewOpen(false)}>
              {t('common.cancel')}
            </Button>
            <Button
              variant="primary"
              size="sm"
              type="submit"
              loading={creating}
              disabled={!newUserId || !newRoleValue || newMaxDuration < 1 || newMaxDuration > 1440}
            >
              {t('pim.newElig.submit')}
            </Button>
          </div>
        </form>
      </Modal>
    </div>
  );
}

export default function PrivilegedAccessPanel(props: PrivilegedAccessPanelProps) {
  return (
    <ToastProvider>
      <Inner {...props} />
    </ToastProvider>
  );
}
