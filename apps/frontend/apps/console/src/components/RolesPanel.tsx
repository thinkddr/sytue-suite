/**
 * RolesPanel — roles personalizados / admin delegada (paridad Entra roles / Workspace
 * admin roles) de la organización. Cada rol concede un subconjunto de PERMISOS
 * granulares (p. ej. «Help Desk» = gestionar usuarios, pero NO facturación) y se asigna
 * a usuarios del tenant. owner/admin siguen siendo superusuarios (no dependen de estos
 * permisos); la gestión de roles NO se delega (reservada a owner/admin, anti
 * auto-escalada). El enforcement vive en Sytue ID (requirePermission); esto es su plano
 * de administración. Todo pasa por el BFF (/api/admin/roles*, cookie httpOnly, el token
 * nunca está aquí). Solo @sytue/ui, Graphite & Brass, denso, oscuro, WCAG AA.
 */
import { useMemo, useState } from 'preact/hooks';
import {
  Badge,
  Button,
  Card,
  CardBody,
  EmptyState,
  Icon,
  InlineHelp,
  Input,
  Modal,
  SettingsSection,
  SettingsShell,
  Spinner,
  ToastProvider,
  useToast,
} from '@sytue/ui';
import { contiene, formatNumber, type Locale } from '@sytue/i18n';
import { useT, type T } from '../i18n/index.js';
import { permissionLabelKey, togglePerm } from '../lib/roles.js';
import type { CustomRole, CustomRoleMember, DirectoryUser } from '../lib/types.js';
import { useUserSearch } from '../lib/useUserSearch.js';
import { PermissionChecklist } from './PermissionChecklist.js';
import RoleWizard from './RoleWizard.js';
import { useTGestion } from '../i18n/gestion/index.js';

interface RolesPanelProps {
  /** Idioma negociado en el servidor; llega por props desde el `.astro`. */
  locale: Locale;
  roles: CustomRole[];
  /** Directorio (primera página) para el picker de asignación. */
  users: DirectoryUser[];
  /** false ⇒ sin backend vivo (no se puede cargar ni editar). */
  live?: boolean;
}

function permLabel(key: string, t: T): string {
  const label = permissionLabelKey(key);
  return label ? t(label) : key;
}

function errorCode(err: unknown): string {
  return err instanceof Error ? err.message : '';
}

function sameSet(a: string[], b: string[]): boolean {
  if (a.length !== b.length) return false;
  const s = new Set(a);
  return b.every((x) => s.has(x));
}

function Inner({ locale, roles, users, live = true }: RolesPanelProps) {
  const t = useT(locale);
  const toast = useToast();
  const [items, setItems] = useState<CustomRole[]>(roles);
  const [query, setQuery] = useState('');

  // Alta guiada: sustituye la página entera, como el resto de asistentes de la consola.
  const [creatingRole, setCreatingRole] = useState(false);
  // Al duplicar, el asistente arranca con el nombre y los permisos del original.
  const [plantilla, setPlantilla] = useState<{ name: string; permissions: string[] } | undefined>(
    undefined,
  );
  const tg = useTGestion(locale);

  // Detalle del rol seleccionado.
  const [selected, setSelected] = useState<CustomRole | null>(null);
  const [members, setMembers] = useState<CustomRoleMember[]>([]);
  const [loadingDetail, setLoadingDetail] = useState(false);
  const [editName, setEditName] = useState('');
  const [editPerms, setEditPerms] = useState<string[]>([]);
  const [savingEdit, setSavingEdit] = useState(false);
  const [busyUser, setBusyUser] = useState('');
  const [confirmingDelete, setConfirmingDelete] = useState(false);
  const [deleting, setDeleting] = useState(false);

  const filtered = useMemo(() => {
    const q = query.trim().toLowerCase();
    if (!q) return items;
    return items.filter((r) => contiene(r.name, q));
  }, [items, query]);

  const editDirty = selected
    ? editName.trim() !== selected.name || !sameSet(editPerms, selected.permissions)
    : false;

  function bump(id: string, delta: number) {
    setItems((prev) =>
      prev.map((r) =>
        r.id === id ? { ...r, memberCount: Math.max(0, r.memberCount + delta) } : r,
      ),
    );
    setSelected((s) =>
      s && s.id === id ? { ...s, memberCount: Math.max(0, s.memberCount + delta) } : s,
    );
  }

  async function openRole(r: CustomRole) {
    setSelected(r);
    setEditName(r.name);
    setEditPerms(r.permissions);
    setMemberQuery('');
    setConfirmingDelete(false);
    setMembers([]);
    setLoadingDetail(true);
    try {
      const res = await fetch(`/api/admin/roles/${encodeURIComponent(r.id)}`, {
        headers: { accept: 'application/json' },
      });
      if (!res.ok) throw new Error(String(res.status));
      const data = (await res.json()) as { role?: CustomRole; members?: CustomRoleMember[] };
      setMembers(data.members ?? []);
      if (data.role) {
        setSelected(data.role);
        setEditName(data.role.name);
        setEditPerms(data.role.permissions);
        setItems((prev) => prev.map((it) => (it.id === r.id ? { ...it, ...data.role } : it)));
      }
    } catch {
      toast.notify({
        tone: 'danger',
        title: t('roles.toast.membersFailed'),
        description: t('common.tryAgain'),
      });
    } finally {
      setLoadingDetail(false);
    }
  }

  function closeModal() {
    setSelected(null);
    setConfirmingDelete(false);
  }

  function roleCreated(role: CustomRole) {
    setItems((prev) => [role, ...prev.filter((r) => r.id !== role.id)]);
    toast.notify({
      tone: 'success',
      title: t('roles.toast.created'),
      description: t('roles.toast.createdHint', { name: role.name }),
    });
  }

  async function saveEdit() {
    if (!selected || !editDirty || savingEdit) return;
    const name = editName.trim();
    if (!name) return;
    setSavingEdit(true);
    try {
      const res = await fetch(`/api/admin/roles/${encodeURIComponent(selected.id)}`, {
        method: 'PATCH',
        headers: { 'content-type': 'application/json' },
        body: JSON.stringify({ name, permissions: editPerms }),
      });
      if (!res.ok) throw new Error(String(res.status));
      const role = (await res.json()) as CustomRole;
      setItems((prev) => prev.map((it) => (it.id === role.id ? { ...it, ...role } : it)));
      setSelected((s) => (s ? { ...s, ...role } : s));
      toast.notify({ tone: 'success', title: t('roles.toast.updated') });
    } catch (err) {
      toast.notify({
        tone: 'danger',
        title: t('common.saveFailed'),
        description:
          errorCode(err) === '409'
            ? t('roles.toast.nameTaken')
            : errorCode(err) === '400'
              ? t('roles.toast.checkPerms')
              : t('common.tryAgain'),
      });
    } finally {
      setSavingEdit(false);
    }
  }

  async function addMember(u: DirectoryUser) {
    if (!selected || busyUser) return;
    setBusyUser(u.id);
    try {
      const res = await fetch(`/api/admin/roles/${encodeURIComponent(selected.id)}/members`, {
        method: 'POST',
        headers: { 'content-type': 'application/json' },
        body: JSON.stringify({ user_id: u.id }),
      });
      if (!res.ok) throw new Error(String(res.status));
      setMembers((prev) => [...prev, { userId: u.id, email: u.email, name: u.fullName }]);
      bump(selected.id, 1);
    } catch (err) {
      toast.notify({
        tone: 'danger',
        title: t('roles.toast.assignFailed'),
        description:
          errorCode(err) === '409' ? t('roles.toast.alreadyAssigned') : t('common.tryAgain'),
      });
    } finally {
      setBusyUser('');
    }
  }

  async function removeMember(m: CustomRoleMember) {
    if (!selected || busyUser) return;
    setBusyUser(m.userId);
    try {
      const res = await fetch(
        `/api/admin/roles/${encodeURIComponent(selected.id)}/members/${encodeURIComponent(m.userId)}`,
        { method: 'DELETE' },
      );
      if (!res.ok) throw new Error(String(res.status));
      setMembers((prev) => prev.filter((it) => it.userId !== m.userId));
      bump(selected.id, -1);
    } catch {
      toast.notify({
        tone: 'danger',
        title: t('roles.toast.unassignFailed'),
        description: t('common.tryAgain'),
      });
    } finally {
      setBusyUser('');
    }
  }

  async function deleteRole() {
    if (!selected || deleting) return;
    setDeleting(true);
    try {
      const res = await fetch(`/api/admin/roles/${encodeURIComponent(selected.id)}`, {
        method: 'DELETE',
      });
      if (!res.ok) throw new Error(String(res.status));
      setItems((prev) => prev.filter((it) => it.id !== selected.id));
      toast.notify({
        tone: 'success',
        title: t('roles.toast.deleted'),
        description: t('roles.toast.deletedHint', { name: selected.name }),
      });
      closeModal();
    } catch {
      toast.notify({
        tone: 'danger',
        title: t('roles.toast.deleteFailed'),
        description: t('common.tryAgain'),
      });
      setConfirmingDelete(false);
    } finally {
      setDeleting(false);
    }
  }

  // Usuarios que aún no tienen el rol. La búsqueda la resuelve el servidor: los
  // `users` de la página son solo las primeras filas del directorio.
  const assigned = useMemo(() => new Set(members.map((m) => m.userId)), [members]);
  const {
    query: memberQuery,
    setQuery: setMemberQuery,
    candidates,
    buscando: buscandoMiembro,
  } = useUserSearch(users, assigned);

  if (creatingRole) {
    return (
      <SettingsShell title={t('roles.create')} description={t('roles.wizard.description')}>
        <RoleWizard
          t={t}
          initial={plantilla}
          onCreated={roleCreated}
          onAssign={(role) => {
            setCreatingRole(false);
            void openRole(role);
          }}
          onCancel={() => setCreatingRole(false)}
          onDone={() => setCreatingRole(false)}
        />
      </SettingsShell>
    );
  }

  return (
    <SettingsShell
      title={t('page.roles')}
      description={t('roles.subtitle')}
      actions={
        <Button
          variant="primary"
          leadingIcon={<Icon name="plus" size={16} />}
          disabled={!live}
          onClick={() => {
            setPlantilla(undefined);
            setCreatingRole(true);
          }}
        >
          {t('roles.create')}
        </Button>
      }
      banner={
        live ? undefined : (
          <InlineHelp tone="warning" title={t('common.offlineIdTitle')}>
            {t('roles.offline')}
          </InlineHelp>
        )
      }
    >
      <SettingsSection
        plain
        title={t('roles.listTitle')}
        description={t('roles.listDescription')}
        actions={
          <Input
            type="search"
            value={query}
            placeholder={t('roles.searchPlaceholder')}
            aria-label={t('roles.searchPlaceholder')}
            leadingIcon={<Icon name="search" size={16} />}
            class="w-60 max-w-full"
            onInput={(e) => setQuery(e.currentTarget.value)}
          />
        }
      >
        {filtered.length === 0 ? (
          <Card>
            <CardBody class="p-0">
              <EmptyState
                title={t(items.length === 0 ? 'roles.empty.title' : 'roles.noResults')}
                description={t(
                  items.length === 0 ? 'roles.empty.description' : 'roles.noResultsHint',
                )}
                icon="lock"
                action={
                  items.length === 0 ? (
                    <Button
                      variant="primary"
                      size="sm"
                      leadingIcon={<Icon name="plus" size={14} />}
                      disabled={!live}
                      onClick={() => {
                        setPlantilla(undefined);
                        setCreatingRole(true);
                      }}
                    >
                      {t('roles.create')}
                    </Button>
                  ) : (
                    <Button variant="secondary" size="sm" onClick={() => setQuery('')}>
                      {t('tenants.clearSearch')}
                    </Button>
                  )
                }
              />
            </CardBody>
          </Card>
        ) : (
          <div class="grid gap-3 md:grid-cols-2 xl:grid-cols-3">
            {filtered.map((r) => (
              <Card key={r.id}>
                <CardBody class="p-0">
                  <button
                    type="button"
                    class="sytue-focus-ring flex w-full flex-col gap-2 rounded-lg p-4 text-left"
                    onClick={() => void openRole(r)}
                    aria-haspopup="dialog"
                    aria-label={t('roles.openAria', { name: r.name })}
                  >
                    <span class="flex min-w-0 items-center gap-2">
                      <Icon
                        name="lock"
                        size={16}
                        class="text-text-subtle shrink-0"
                        aria-hidden="true"
                      />
                      <span class="text-text min-w-0 flex-1 truncate font-medium" title={r.name}>
                        {r.name}
                      </span>
                      <Badge tone="neutral">{formatNumber(locale, r.memberCount)}</Badge>
                    </span>
                    {r.permissions.length ? (
                      <span class="flex flex-wrap gap-1">
                        {r.permissions.slice(0, 4).map((p) => (
                          <Badge key={p} tone="accent">
                            {permLabel(p, t)}
                          </Badge>
                        ))}
                        {r.permissions.length > 4 ? (
                          <Badge tone="neutral">+{r.permissions.length - 4}</Badge>
                        ) : null}
                      </span>
                    ) : (
                      <span class="text-text-subtle text-sm italic">
                        {t('roles.noPermissions')}
                      </span>
                    )}
                  </button>
                </CardBody>
              </Card>
            ))}
          </div>
        )}
      </SettingsSection>

      {/* Detalle / edición del rol */}
      <Modal
        open={Boolean(selected)}
        onClose={closeModal}
        title={selected?.name ?? t('roles.fallbackTitle')}
        size="md"
      >
        {selected ? (
          <div class="flex flex-col gap-4">
            <div class="flex flex-col gap-3">
              <div>
                <label class="text-text mb-1 block text-sm font-medium" for="edit-role-name">
                  {t('tenants.form.name')}
                </label>
                <Input
                  id="edit-role-name"
                  value={editName}
                  invalid={editName.trim().length === 0}
                  onInput={(e) => setEditName(e.currentTarget.value)}
                />
              </div>
              <PermissionChecklist
                selected={editPerms}
                onToggle={(key) => setEditPerms((prev) => togglePerm(prev, key))}
                t={t}
              />
              {editDirty ? (
                <div class="flex gap-2">
                  <Button
                    variant="primary"
                    size="sm"
                    loading={savingEdit}
                    disabled={editName.trim().length === 0}
                    onClick={() => void saveEdit()}
                  >
                    {t('roles.saveChanges')}
                  </Button>
                  <Button
                    variant="ghost"
                    size="sm"
                    onClick={() => {
                      setEditName(selected.name);
                      setEditPerms(selected.permissions);
                    }}
                  >
                    {t('roles.discard')}
                  </Button>
                </div>
              ) : null}
            </div>

            <div class="border-border border-t pt-3">
              <p class="text-text mb-2 text-sm font-semibold">
                {t('roles.assigned')}{' '}
                {loadingDetail ? '' : `(${formatNumber(locale, members.length)})`}
              </p>
              {loadingDetail ? (
                <div class="text-text-muted flex items-center gap-2 py-3 text-sm">
                  <Spinner size={16} /> {t('common.loading')}
                </div>
              ) : members.length === 0 ? (
                <p class="text-text-subtle py-2 text-sm">{t('roles.noAssignees')}</p>
              ) : (
                <ul class="flex max-h-48 flex-col gap-1 overflow-auto">
                  {members.map((m) => (
                    <li
                      key={m.userId}
                      class="hover:bg-bg-subtle flex items-center gap-2 rounded-md px-2 py-1.5"
                    >
                      <span class="min-w-0 flex-1">
                        <span class="text-text block truncate text-sm font-medium" title={m.name}>
                          {m.name}
                        </span>
                        <span class="text-text-muted block truncate text-xs" title={m.email}>
                          {m.email}
                        </span>
                      </span>
                      <Button
                        variant="ghost"
                        size="sm"
                        loading={busyUser === m.userId}
                        aria-label={t('roles.unassignAria', { name: m.name })}
                        onClick={() => void removeMember(m)}
                      >
                        <Icon name="close" size={14} />
                      </Button>
                    </li>
                  ))}
                </ul>
              )}
            </div>

            {!loadingDetail ? (
              <div class="border-border border-t pt-3">
                <label class="text-text mb-1 block text-sm font-medium" for="assign-picker">
                  {t('roles.assignTo')}
                </label>
                <Input
                  id="assign-picker"
                  type="search"
                  value={memberQuery}
                  placeholder={t('roles.assignPlaceholder')}
                  leadingIcon={<Icon name="search" size={16} />}
                  onInput={(e) => setMemberQuery(e.currentTarget.value)}
                />
                {memberQuery.trim() ? (
                  candidates.length === 0 ? (
                    <p class="text-text-subtle mt-2 text-sm">
                      {buscandoMiembro ? t('common.loading') : t('roles.noCandidates')}
                    </p>
                  ) : (
                    <ul class="mt-2 flex flex-col gap-1">
                      {candidates.map((u) => (
                        <li key={u.id}>
                          <button
                            type="button"
                            class="hover:bg-bg-subtle sytue-focus-ring flex w-full items-center gap-2 rounded-md px-2 py-1.5 text-left disabled:opacity-50"
                            disabled={busyUser === u.id}
                            onClick={() => void addMember(u)}
                          >
                            <Icon
                              name="plus"
                              size={14}
                              class="text-text-subtle shrink-0"
                              aria-hidden="true"
                            />
                            <span class="min-w-0 flex-1">
                              <span class="text-text block truncate text-sm font-medium">
                                {u.fullName}
                              </span>
                              <span class="text-text-muted block truncate text-xs">{u.email}</span>
                            </span>
                          </button>
                        </li>
                      ))}
                    </ul>
                  )
                ) : (
                  <p class="sytue-meta mt-2">{t('roles.assignHint')}</p>
                )}
              </div>
            ) : null}

            {/* Duplicar: un rol «como Soporte pero sin facturación» empezaba marcando
                a mano las mismas casillas, que es donde se cuela la que falta. */}
            <div class="border-border flex flex-wrap items-center gap-2 border-t pt-3">
              <Button
                variant="secondary"
                size="sm"
                disabled={!live}
                onClick={() => {
                  setPlantilla({
                    name: tg('gestion.roles.copia', { name: selected.name }),
                    permissions: selected.permissions,
                  });
                  closeModal();
                  setCreatingRole(true);
                }}
              >
                {tg('gestion.roles.duplicar')}
              </Button>
            </div>

            <div class="border-border border-t pt-3">
              {confirmingDelete ? (
                <div class="flex flex-col gap-2">
                  <p class="text-text-muted text-sm">
                    {t('roles.deleteConfirm', { name: selected.name })}
                  </p>
                  <div class="flex gap-2">
                    <Button
                      variant="danger"
                      size="sm"
                      id="delete-role-confirm"
                      loading={deleting}
                      onClick={() => void deleteRole()}
                    >
                      {t('roles.delete')}
                    </Button>
                    <Button variant="ghost" size="sm" onClick={() => setConfirmingDelete(false)}>
                      {t('common.cancel')}
                    </Button>
                  </div>
                </div>
              ) : (
                <Button
                  variant="ghost"
                  size="sm"
                  class="text-danger-text"
                  aria-expanded={confirmingDelete}
                  onClick={() => {
                    setConfirmingDelete(true);
                    requestAnimationFrame(() =>
                      document.getElementById('delete-role-confirm')?.focus(),
                    );
                  }}
                >
                  {t('roles.delete')}
                </Button>
              )}
            </div>
          </div>
        ) : null}
      </Modal>
    </SettingsShell>
  );
}

export default function RolesPanel({ locale, roles, users, live }: RolesPanelProps) {
  return (
    <ToastProvider>
      <Inner locale={locale} roles={roles} users={users} live={live ?? true} />
    </ToastProvider>
  );
}
