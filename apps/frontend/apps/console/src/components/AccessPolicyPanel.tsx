/**
 * AccessPolicyPanel — Acceso Condicional (paridad Entra Conditional Access) como
 * página de AJUSTES.
 *
 * Crear o cambiar una regla ya no es un muro de formulario: abre el constructor
 * guiado (AccessPolicyWizard), que pregunta a quién afecta, cuándo se aplica y qué
 * ocurre, y antes de guardar traduce la regla a una frase en llano y avisa de a quién
 * dejaría fuera.
 *
 * Aquí quedan la lista y su gestión: el orden en el que se evalúan (con flechas para
 * moverlas), encender/apagar, editar y borrar. El enforcement vive en el login de
 * Sytue ID; esto es solo su plano de administración. CRUD vía el BFF (cookie httpOnly).
 */
import { useState } from 'preact/hooks';
import {
  Badge,
  Button,
  Card,
  EmptyState,
  Icon,
  InlineHelp,
  SettingsSection,
  SettingsShell,
  Switch,
  ToastProvider,
  useToast,
  ConfirmDialog,
} from '@sytue/ui';
import { formatList, type Locale } from '@sytue/i18n';
import AccessPolicyWizard from './AccessPolicyWizard.js';
import type { PickUser } from './AccessPolicyWizard.js';
import { useT, type T } from '../i18n/index.js';
import type { AccessAction, AccessPolicy, AdminGroup } from '../lib/types.js';

interface AccessPolicyPanelProps {
  /** Idioma negociado en el servidor; llega por props desde el `.astro`. */
  locale: Locale;
  policies: AccessPolicy[];
  groups: AdminGroup[];
  users: PickUser[];
  /** false = Sytue ID no accesible; el panel avisa y no edita. */
  live: boolean;
}

function actionBadge(a: AccessAction, t: T) {
  if (a === 'block') return <Badge tone="danger">{t('access.action.block')}</Badge>;
  if (a === 'require_mfa') return <Badge tone="warning">{t('access.action.mfa')}</Badge>;
  return <Badge tone="success">{t('access.action.allow')}</Badge>;
}

/** «A 2 grupos y 3 personas»: la conjunción la pone `formatList` con el locale. */
function appliesSummary(p: AccessPolicy, t: T, locale: Locale): string {
  if (p.includeUsers.length === 0 && p.includeGroups.length === 0)
    return t('access.applies.everyone');
  const parts: string[] = [];
  if (p.includeGroups.length)
    parts.push(t('access.applies.groups', { count: p.includeGroups.length }));
  if (p.includeUsers.length)
    parts.push(t('access.applies.users', { count: p.includeUsers.length }));
  return t('access.applies.to', { what: formatList(locale, parts) });
}

function conditionSummary(p: AccessPolicy, t: T): string {
  const parts: string[] = [];
  if (p.ipRanges.length)
    parts.push(p.ipNegate ? t('access.cond.outsideNetworks') : t('access.cond.fromNetworks'));
  if (p.requireTrustedDevice) parts.push(t('access.cond.untrustedDevices'));
  if (p.clientIds.length) parts.push(t('access.cond.apps', { count: p.clientIds.length }));
  if (p.daysMask !== 0 || p.startMinute >= 0) parts.push(t('access.cond.schedule'));
  const exempt = p.excludeUsers.length + p.excludeGroups.length;
  const when = parts.length ? parts.join(' · ') : t('access.cond.always');
  return exempt ? `${when} · ${t('access.cond.exempt', { count: exempt })}` : when;
}

function Inner({ locale, policies, groups, users, live }: AccessPolicyPanelProps) {
  const t = useT(locale);
  const toast = useToast();
  const [items, setItems] = useState<AccessPolicy[]>(
    [...policies].sort((a, b) => a.priority - b.priority),
  );
  const [wizard, setWizard] = useState<{ open: boolean; policy?: AccessPolicy }>({ open: false });
  const [busyId, setBusyId] = useState<string | null>(null);
  const [confirmDel, setConfirmDel] = useState<AccessPolicy | null>(null);

  function upsert(p: AccessPolicy) {
    setItems((prev) => {
      const i = prev.findIndex((x) => x.id === p.id);
      const next = i < 0 ? [...prev, p] : prev.map((x) => (x.id === p.id ? p : x));
      return next.sort((a, b) => a.priority - b.priority);
    });
  }

  async function patch(id: string, body: Record<string, unknown>): Promise<AccessPolicy> {
    const res = await fetch(`/api/admin/access-policies/${encodeURIComponent(id)}`, {
      method: 'PATCH',
      headers: { 'content-type': 'application/json' },
      body: JSON.stringify(body),
    });
    if (!res.ok) throw new Error(String(res.status));
    return (await res.json()) as AccessPolicy;
  }

  async function toggleEnabled(p: AccessPolicy) {
    setBusyId(p.id);
    try {
      upsert(await patch(p.id, { enabled: !p.enabled }));
    } catch {
      toast.notify({
        tone: 'danger',
        title: t('common.toggleFailed'),
        description: t('common.toggleFailedHint', { name: p.name }),
      });
    } finally {
      setBusyId(null);
    }
  }

  /** Mueve la regla una posición y renumera el orden (10, 20, 30…) donde haga falta. */
  async function move(p: AccessPolicy, dir: -1 | 1) {
    const ordered = items.slice();
    const i = ordered.findIndex((x) => x.id === p.id);
    const j = i + dir;
    if (i < 0 || j < 0 || j >= ordered.length) return;
    const a = ordered[i] as AccessPolicy;
    const b = ordered[j] as AccessPolicy;
    ordered[i] = b;
    ordered[j] = a;
    const changes = ordered
      .map((x, k) => ({ x, priority: (k + 1) * 10 }))
      .filter(({ x, priority }) => x.priority !== priority);
    setBusyId(p.id);
    try {
      const saved = await Promise.all(changes.map((c) => patch(c.x.id, { priority: c.priority })));
      setItems(() => {
        const byId = new Map(saved.map((s) => [s.id, s]));
        return ordered.map((x) => byId.get(x.id) ?? x).sort((m, n) => m.priority - n.priority);
      });
    } catch {
      toast.notify({
        tone: 'danger',
        title: t('access.toast.reorderFailed'),
        description: t('common.tryAgain'),
      });
    } finally {
      setBusyId(null);
    }
  }

  async function remove(p: AccessPolicy) {
    setBusyId(p.id);
    try {
      const res = await fetch(`/api/admin/access-policies/${encodeURIComponent(p.id)}`, {
        method: 'DELETE',
      });
      if (!res.ok) throw new Error(String(res.status));
      setItems((prev) => prev.filter((x) => x.id !== p.id));
      setConfirmDel(null);
      toast.notify({ tone: 'success', title: t('access.toast.deleted'), description: p.name });
    } catch {
      toast.notify({
        tone: 'danger',
        title: t('access.toast.deleteFailed'),
        description: t('common.tryAgain'),
      });
    } finally {
      setBusyId(null);
    }
  }

  if (wizard.open) {
    return (
      <SettingsShell
        title={
          wizard.policy
            ? t('access.wizard.editTitle', { name: wizard.policy.name })
            : t('access.wizard.newTitle')
        }
        description={t('access.wizard.description')}
      >
        <AccessPolicyWizard
          t={t}
          locale={locale}
          editing={wizard.policy}
          groups={groups}
          users={users}
          nextPriority={(items.length + 1) * 10}
          onSaved={upsert}
          onCancel={() => setWizard({ open: false })}
          onDone={() => setWizard({ open: false })}
        />
      </SettingsShell>
    );
  }

  return (
    <SettingsShell
      title={t('access.title')}
      description={t('access.subtitle')}
      actions={
        <Button
          variant="primary"
          disabled={!live}
          leadingIcon={<Icon name="plus" size={16} />}
          onClick={() => setWizard({ open: true })}
        >
          {t('access.new')}
        </Button>
      }
      banner={
        live ? undefined : (
          <InlineHelp tone="warning" title={t('common.offlineIdTitle')}>
            {t('access.offlineBody')}
          </InlineHelp>
        )
      }
    >
      <SettingsSection
        plain
        title={t('access.rules.title')}
        description={t('access.rules.description')}
      >
        {items.length === 0 ? (
          <Card>
            <EmptyState
              title={t('access.empty.title')}
              description={t('access.empty.description')}
              illustration={<Icon name="lock" size={28} />}
              action={
                <Button
                  variant="primary"
                  disabled={!live}
                  onClick={() => setWizard({ open: true })}
                >
                  {t('access.empty.action')}
                </Button>
              }
            />
          </Card>
        ) : (
          <ul class="border-border bg-surface divide-border-subtle divide-y overflow-hidden rounded-lg border">
            {items.map((p, i) => (
              <li key={p.id} class="hover:bg-bg-subtle flex flex-wrap items-center gap-3 px-3 py-3">
                <span
                  aria-hidden="true"
                  class="bg-bg-subtle text-text-muted grid h-6 w-6 shrink-0 place-items-center rounded-full text-xs font-semibold tabular-nums"
                >
                  {i + 1}
                </span>
                <span class="flex w-32 shrink-0 flex-col items-start gap-1">
                  {actionBadge(p.action, t)}
                </span>
                <span class="min-w-0 flex-1 basis-64">
                  <span class="text-text block truncate font-medium">{p.name}</span>
                  <span class="sytue-meta block truncate">
                    {appliesSummary(p, t, locale)} · {conditionSummary(p, t)}
                  </span>
                </span>
                <span class="flex shrink-0 items-center gap-0.5">
                  <Button
                    variant="ghost"
                    size="sm"
                    disabled={!live || busyId !== null || i === 0}
                    aria-label={t('access.moveUp', { name: p.name })}
                    onClick={() => void move(p, -1)}
                  >
                    <Icon name="chevronUp" size={14} />
                  </Button>
                  <Button
                    variant="ghost"
                    size="sm"
                    disabled={!live || busyId !== null || i === items.length - 1}
                    aria-label={t('access.moveDown', { name: p.name })}
                    onClick={() => void move(p, 1)}
                  >
                    <Icon name="chevronDown" size={14} />
                  </Button>
                </span>
                <span class="shrink-0">
                  <Switch
                    label={p.enabled ? t('access.on') : t('access.off')}
                    checked={p.enabled}
                    disabled={!live || busyId === p.id}
                    aria-label={t('common.switchAria', {
                      state: p.enabled ? t('access.on') : t('access.off'),
                      name: p.name,
                    })}
                    onChange={() => void toggleEnabled(p)}
                  />
                </span>
                <span class="flex shrink-0 items-center gap-1">
                  <Button
                    variant="ghost"
                    size="sm"
                    disabled={!live}
                    aria-label={t('common.editName', { name: p.name })}
                    onClick={() => setWizard({ open: true, policy: p })}
                  >
                    {t('common.edit')}
                  </Button>
                  <Button
                    variant="ghost"
                    size="sm"
                    disabled={!live || busyId === p.id}
                    aria-label={t('common.deleteName', { name: p.name })}
                    onClick={() => setConfirmDel(p)}
                  >
                    {t('common.delete')}
                  </Button>
                </span>
              </li>
            ))}
          </ul>
        )}
        {items.length > 0 ? <p class="sytue-meta leading-relaxed">{t('access.tip')}</p> : null}
      </SettingsSection>

      <ConfirmDialog
        open={Boolean(confirmDel)}
        onClose={() => setConfirmDel(null)}
        onConfirm={() => {
          if (confirmDel) void remove(confirmDel);
        }}
        title={t('access.delete.title')}
        confirmLabel={t('common.delete')}
        tone="danger"
        busy={confirmDel ? busyId === confirmDel.id : false}
      >
        {confirmDel ? (
          <p class="text-text-muted text-sm leading-relaxed">
            {t('access.delete.body', { name: confirmDel.name })}
          </p>
        ) : null}
      </ConfirmDialog>
    </SettingsShell>
  );
}

export default function AccessPolicyPanel(props: AccessPolicyPanelProps) {
  return (
    <ToastProvider>
      <Inner {...props} />
    </ToastProvider>
  );
}
