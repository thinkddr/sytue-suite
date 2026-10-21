/**
 * GroupsPanel — grupos gestionables de Sytue ID (contrato /admin/groups). Lista de
 * grupos con nº de miembros, alta, edición (nombre/descripción), gestión de
 * miembros (picker sobre el directorio del tenant) y borrado con confirmación.
 * Todo pasa por el BFF (/api/admin/groups*), que proxya a Sytue ID con el access
 * token httpOnly bajo RLS del tenant. Estos grupos alimentan el share por-grupo de
 * Sytue Drive (claim `groups` del JWT). docs/CONVENTIONS.md §8, §9.
 */
import { useMemo, useState } from 'preact/hooks';
import {
  Badge,
  Button,
  Card,
  CardBody,
  EmptyState,
  Icon,
  Input,
  Modal,
  SectionHeader,
  Select,
  Spinner,
  Switch,
  ToastProvider,
  useToast,
} from '@sytue/ui';
import { contiene, formatNumber, type Locale } from '@sytue/i18n';
import { useT, type T } from '../i18n/index.js';
import type {
  AdminGroup,
  AdminGroupMember,
  DirectoryUser,
  RuleAttribute,
  RuleOp,
} from '../lib/types.js';
import {
  builderToRule,
  emptyBuilderRule,
  ruleToBuilder,
  RULE_ATTRIBUTES,
  RULE_OPS,
  type BuilderRule,
} from '../lib/rules.js';
import { useUserSearch } from '../lib/useUserSearch.js';

interface GroupsPanelProps {
  /** Idioma negociado en el servidor; llega por props desde el `.astro`. */
  locale: Locale;
  groups: AdminGroup[];
  /** Directorio (primera página) para el picker de miembros. */
  users: DirectoryUser[];
  /** false ⇒ datos de ejemplo (sin backend vivo). */
  live?: boolean;
}

function errorCode(err: unknown): string {
  return err instanceof Error ? err.message : '';
}

/** Resultado de la previsualización de una regla (contrato del BFF /groups/preview). */
interface PreviewResult {
  count: number;
  members: AdminGroupMember[];
}

/**
 * Constructor de reglas de membresía dinámica: selector AND/OR + lista de condiciones
 * (atributo · operador · valor). El operador `in` acepta varios valores separados por
 * comas. Accesible: cada control tiene etiqueta asociada. La validación fuerte la aplica
 * Sytue ID al guardar/previsualizar.
 */
function RuleBuilder({
  rule,
  onChange,
  idPrefix,
  t,
}: {
  rule: BuilderRule;
  onChange: (r: BuilderRule) => void;
  idPrefix: string;
  t: T;
}) {
  function setCond(i: number, patch: Partial<BuilderRule['conditions'][number]>) {
    onChange({
      ...rule,
      conditions: rule.conditions.map((c, idx) => (idx === i ? { ...c, ...patch } : c)),
    });
  }
  return (
    <div class="flex flex-col gap-3">
      <div class="flex items-center gap-2">
        <label class="text-text text-sm font-medium" for={`${idPrefix}-match`}>
          {t('groups.rule.include')}
        </label>
        <Select
          id={`${idPrefix}-match`}
          value={rule.match}
          class="w-40"
          onChange={(e) =>
            onChange({ ...rule, match: e.currentTarget.value === 'any' ? 'any' : 'all' })
          }
        >
          <option value="all">{t('groups.rule.matchAll')}</option>
          <option value="any">{t('groups.rule.matchAny')}</option>
        </Select>
      </div>

      <ul class="flex flex-col gap-2">
        {rule.conditions.map((c, i) => (
          <li
            key={i}
            class="border-border bg-surface flex flex-wrap items-center gap-2 rounded-md border p-2"
          >
            <Select
              value={c.attribute}
              aria-label={t('groups.rule.attribute')}
              class="w-40"
              onChange={(e) => setCond(i, { attribute: e.currentTarget.value as RuleAttribute })}
            >
              {RULE_ATTRIBUTES.map((a) => (
                <option key={a.value} value={a.value}>
                  {t(a.label)}
                </option>
              ))}
            </Select>
            <Select
              value={c.op}
              aria-label={t('groups.rule.operator')}
              class="w-40"
              onChange={(e) => setCond(i, { op: e.currentTarget.value as RuleOp })}
            >
              {RULE_OPS.map((o) => (
                <option key={o.value} value={o.value}>
                  {t(o.label)}
                </option>
              ))}
            </Select>
            <Input
              class="min-w-40 flex-1"
              value={c.value}
              aria-label={t('groups.rule.value')}
              placeholder={t(c.op === 'in' ? 'groups.rule.valueListHint' : 'groups.rule.value')}
              onInput={(e) => setCond(i, { value: e.currentTarget.value })}
            />
            <Button
              variant="ghost"
              size="sm"
              aria-label={t('groups.rule.removeCondition')}
              disabled={rule.conditions.length <= 1}
              onClick={() =>
                onChange({ ...rule, conditions: rule.conditions.filter((_, idx) => idx !== i) })
              }
            >
              <Icon name="close" size={14} />
            </Button>
          </li>
        ))}
      </ul>

      <div>
        <Button
          variant="secondary"
          size="sm"
          leadingIcon={<Icon name="plus" size={14} />}
          onClick={() =>
            onChange({
              ...rule,
              conditions: [...rule.conditions, { attribute: 'department', op: 'eq', value: '' }],
            })
          }
        >
          {t('groups.rule.addCondition')}
        </Button>
      </div>
    </div>
  );
}

/** Botón de previsualización + recuento y muestra de a quién incluiría la regla. */
function PreviewBlock({
  preview,
  loading,
  onPreview,
  t,
}: {
  preview: PreviewResult | null;
  loading: boolean;
  onPreview: () => void;
  t: T;
}) {
  return (
    <div class="flex flex-col gap-2">
      <div class="flex flex-wrap items-center gap-2">
        <Button variant="secondary" size="sm" loading={loading} onClick={onPreview}>
          {t('groups.rule.preview')}
        </Button>
        {preview ? (
          <span class="text-text-muted text-sm">
            {preview.count === 0
              ? t('groups.rule.previewNone')
              : t('groups.rule.previewCount', { count: preview.count })}
          </span>
        ) : null}
      </div>
      {preview && preview.members.length > 0 ? (
        <ul class="border-border max-h-32 overflow-auto rounded-md border">
          {preview.members.map((m) => (
            <li key={m.userId} class="border-border flex flex-col border-b px-2 py-1 last:border-0">
              <span class="text-text truncate text-sm font-medium">{m.name}</span>
              <span class="text-text-muted truncate text-xs">{m.email}</span>
            </li>
          ))}
        </ul>
      ) : null}
    </div>
  );
}

function Inner({ locale, groups, users, live = true }: GroupsPanelProps) {
  const t = useT(locale);
  const toast = useToast();
  const [items, setItems] = useState<AdminGroup[]>(groups);
  const [query, setQuery] = useState('');

  // Crear grupo.
  const [createOpen, setCreateOpen] = useState(false);
  const [newName, setNewName] = useState('');
  const [newDesc, setNewDesc] = useState('');
  const [creating, setCreating] = useState(false);
  const [newDynamic, setNewDynamic] = useState(false);
  const [newRule, setNewRule] = useState<BuilderRule>(emptyBuilderRule);
  const [newPreview, setNewPreview] = useState<PreviewResult | null>(null);
  const [previewingNew, setPreviewingNew] = useState(false);

  // Detalle del grupo seleccionado.
  const [selected, setSelected] = useState<AdminGroup | null>(null);
  const [members, setMembers] = useState<AdminGroupMember[]>([]);
  const [loadingDetail, setLoadingDetail] = useState(false);
  const [editName, setEditName] = useState('');
  const [editDesc, setEditDesc] = useState('');
  const [savingEdit, setSavingEdit] = useState(false);
  const [busyUser, setBusyUser] = useState('');
  const [confirmingDelete, setConfirmingDelete] = useState(false);
  const [deleting, setDeleting] = useState(false);
  // Regla dinámica del grupo seleccionado (editar / convertir a dinámico).
  const [editDynamic, setEditDynamic] = useState(false);
  const [editRule, setEditRule] = useState<BuilderRule>(emptyBuilderRule);
  const [editPreview, setEditPreview] = useState<PreviewResult | null>(null);
  const [previewingEdit, setPreviewingEdit] = useState(false);
  const [savingRule, setSavingRule] = useState(false);
  const [recomputing, setRecomputing] = useState(false);
  const [recomputingAll, setRecomputingAll] = useState(false);

  const filtered = useMemo(() => {
    const q = query.trim().toLowerCase();
    if (!q) return items;
    return items.filter((g) => contiene(g.name, q) || contiene(g.description, q));
  }, [items, query]);

  const editDirty = selected
    ? editName.trim() !== selected.name || editDesc.trim() !== selected.description
    : false;

  function bump(id: string, delta: number) {
    setItems((prev) =>
      prev.map((g) =>
        g.id === id ? { ...g, memberCount: Math.max(0, g.memberCount + delta) } : g,
      ),
    );
    setSelected((s) =>
      s && s.id === id ? { ...s, memberCount: Math.max(0, s.memberCount + delta) } : s,
    );
  }

  function syncDynamicState(g: AdminGroup) {
    setEditDynamic(g.dynamic);
    setEditRule(g.membershipRule ? ruleToBuilder(g.membershipRule) : emptyBuilderRule());
    setEditPreview(null);
  }

  async function openGroup(g: AdminGroup) {
    setSelected(g);
    setEditName(g.name);
    setEditDesc(g.description);
    setMemberQuery('');
    setConfirmingDelete(false);
    setMembers([]);
    syncDynamicState(g);
    setLoadingDetail(true);
    try {
      const res = await fetch(`/api/admin/groups/${encodeURIComponent(g.id)}`, {
        headers: { accept: 'application/json' },
      });
      if (!res.ok) throw new Error(String(res.status));
      const data = (await res.json()) as { group?: AdminGroup; members?: AdminGroupMember[] };
      setMembers(data.members ?? []);
      if (data.group) {
        setSelected(data.group);
        setEditName(data.group.name);
        setEditDesc(data.group.description);
        syncDynamicState(data.group);
        setItems((prev) => prev.map((it) => (it.id === g.id ? { ...it, ...data.group } : it)));
      }
    } catch {
      toast.notify({
        tone: 'danger',
        title: t('groups.toast.membersFailed'),
        description: t(live ? 'common.tryAgain' : 'groups.toast.sampleData'),
      });
    } finally {
      setLoadingDetail(false);
    }
  }

  function closeModal() {
    setSelected(null);
    setConfirmingDelete(false);
  }

  /** Previsualiza una regla contra el tenant (BFF); devuelve null si falla. */
  async function fetchPreview(rule: BuilderRule): Promise<PreviewResult | null> {
    const res = await fetch('/api/admin/groups/preview', {
      method: 'POST',
      headers: { 'content-type': 'application/json' },
      body: JSON.stringify({ membership_rule: builderToRule(rule) }),
    });
    if (!res.ok) throw new Error(String(res.status));
    const data = (await res.json()) as PreviewResult;
    return { count: data.count ?? 0, members: data.members ?? [] };
  }

  async function previewNew() {
    if (previewingNew) return;
    setPreviewingNew(true);
    try {
      setNewPreview(await fetchPreview(newRule));
    } catch {
      toast.notify({
        tone: 'danger',
        title: t('groups.toast.previewFailed'),
        description: t('groups.toast.previewFailedHint'),
      });
    } finally {
      setPreviewingNew(false);
    }
  }

  function resetCreate() {
    setCreateOpen(false);
    setNewName('');
    setNewDesc('');
    setNewDynamic(false);
    setNewRule(emptyBuilderRule());
    setNewPreview(null);
  }

  async function createGroup(e: Event) {
    e.preventDefault();
    const name = newName.trim();
    if (!name || creating) return;
    setCreating(true);
    try {
      const res = await fetch('/api/admin/groups', {
        method: 'POST',
        headers: { 'content-type': 'application/json' },
        body: JSON.stringify({
          name,
          description: newDesc.trim(),
          ...(newDynamic ? { membership_rule: builderToRule(newRule) } : {}),
        }),
      });
      if (!res.ok) throw new Error(String(res.status));
      const group = (await res.json()) as AdminGroup;
      setItems((prev) => [{ ...group, memberCount: group.memberCount ?? 0 }, ...prev]);
      toast.notify({
        tone: 'success',
        title: t(newDynamic ? 'groups.toast.dynamicCreated' : 'groups.toast.created'),
        description: t('groups.toast.createdHint', { name: group.name }),
      });
      resetCreate();
    } catch (err) {
      toast.notify({
        tone: 'danger',
        title: t('groups.toast.createFailed'),
        description:
          errorCode(err) === '409'
            ? t('groups.toast.nameTaken')
            : t(newDynamic ? 'groups.toast.checkNameAndRule' : 'groups.toast.checkName'),
      });
    } finally {
      setCreating(false);
    }
  }

  async function saveEdit() {
    if (!selected || !editDirty || savingEdit) return;
    const name = editName.trim();
    if (!name) return;
    setSavingEdit(true);
    try {
      const res = await fetch(`/api/admin/groups/${encodeURIComponent(selected.id)}`, {
        method: 'PATCH',
        headers: { 'content-type': 'application/json' },
        body: JSON.stringify({ name, description: editDesc.trim() }),
      });
      if (!res.ok) throw new Error(String(res.status));
      const group = (await res.json()) as AdminGroup;
      setItems((prev) => prev.map((it) => (it.id === group.id ? { ...it, ...group } : it)));
      setSelected((s) => (s ? { ...s, ...group } : s));
      toast.notify({ tone: 'success', title: t('groups.toast.updated') });
    } catch (err) {
      toast.notify({
        tone: 'danger',
        title: t('common.saveFailed'),
        description: errorCode(err) === '409' ? t('groups.toast.nameTaken') : t('common.tryAgain'),
      });
    } finally {
      setSavingEdit(false);
    }
  }

  async function previewEdit() {
    if (previewingEdit) return;
    setPreviewingEdit(true);
    try {
      setEditPreview(await fetchPreview(editRule));
    } catch {
      toast.notify({
        tone: 'danger',
        title: t('groups.toast.previewFailed'),
        description: t('groups.toast.previewFailedHint'),
      });
    } finally {
      setPreviewingEdit(false);
    }
  }

  /**
   * Recalcula AHORA la membresía del grupo dinámico contra el estado actual de los
   * usuarios y enseña el efecto real (cuántos entraron y cuántos salieron). Sytue ID
   * también lo barre cada pocos minutos; esto es para no esperar.
   */
  async function recomputeNow() {
    if (!selected || recomputing) return;
    setRecomputing(true);
    try {
      const res = await fetch(`/api/admin/groups/${encodeURIComponent(selected.id)}/recompute`, {
        method: 'POST',
        headers: { 'content-type': 'application/json' },
      });
      if (!res.ok) throw new Error(String(res.status));
      const out = (await res.json()) as { added: number; removed: number; member_count: number };
      const count = out.member_count;
      setItems((prev) =>
        prev.map((it) => (it.id === selected.id ? { ...it, memberCount: count } : it)),
      );
      setSelected((s) => (s ? { ...s, memberCount: count } : s));
      const dres = await fetch(`/api/admin/groups/${encodeURIComponent(selected.id)}`, {
        headers: { accept: 'application/json' },
      });
      if (dres.ok) {
        const data = (await dres.json()) as { members?: AdminGroupMember[] };
        setMembers(data.members ?? []);
      }
      const moved = out.added + out.removed;
      toast.notify({
        tone: 'success',
        title: t(moved === 0 ? 'groups.toast.alreadyUpToDate' : 'groups.toast.recomputed'),
        description:
          moved === 0
            ? t('groups.toast.recomputedSame', { count })
            : `${t('groups.toast.recomputedAdded', { count: out.added })} · ${t(
                'groups.toast.recomputedRemoved',
                { count: out.removed },
              )} · ${t('groups.memberCount', { count })}`,
      });
    } catch {
      toast.notify({
        tone: 'danger',
        title: t('groups.toast.recomputeFailed'),
        description: t('common.tryAgain'),
      });
    } finally {
      setRecomputing(false);
    }
  }

  /**
   * Lo mismo, pero para TODOS los grupos dinámicos de la org. Tras una importación
   * o una reorganización la deriva no es de un grupo: ir uno a uno no es opción.
   * Si algo se movió recarga, porque los recuentos de la lista vienen del servidor.
   */
  async function recomputeAllNow() {
    if (recomputingAll) return;
    setRecomputingAll(true);
    try {
      const res = await fetch('/api/admin/groups/recompute', {
        method: 'POST',
        headers: { 'content-type': 'application/json' },
      });
      if (!res.ok) throw new Error(String(res.status));
      const out = (await res.json()) as { groups: number; added: number; removed: number };
      const moved = out.added + out.removed;
      toast.notify({
        tone: 'success',
        title: t(moved === 0 ? 'groups.toast.alreadyUpToDate' : 'groups.toast.recomputed'),
        description: `${t('groups.toast.recomputedAllGroups', { count: out.groups })} · ${t(
          'groups.toast.recomputedAdded',
          { count: out.added },
        )} · ${t('groups.toast.recomputedRemoved', { count: out.removed })}`,
      });
      if (moved > 0) window.location.reload();
    } catch {
      toast.notify({
        tone: 'danger',
        title: t('groups.toast.recomputeFailed'),
        description: t('common.tryAgain'),
      });
    } finally {
      setRecomputingAll(false);
    }
  }

  /** Guarda la regla del grupo (lo convierte en dinámico si no lo era) y recomputa. */
  async function saveRule() {
    if (!selected || savingRule) return;
    setSavingRule(true);
    try {
      const res = await fetch(`/api/admin/groups/${encodeURIComponent(selected.id)}`, {
        method: 'PATCH',
        headers: { 'content-type': 'application/json' },
        body: JSON.stringify({ membership_rule: builderToRule(editRule) }),
      });
      if (!res.ok) throw new Error(String(res.status));
      const group = (await res.json()) as AdminGroup;
      setItems((prev) => prev.map((it) => (it.id === group.id ? { ...it, ...group } : it)));
      setSelected((s) => (s ? { ...s, ...group } : s));
      syncDynamicState(group);
      // La membresía la recalculó Sytue ID: recarga los miembros materializados.
      try {
        const dres = await fetch(`/api/admin/groups/${encodeURIComponent(selected.id)}`, {
          headers: { accept: 'application/json' },
        });
        if (dres.ok) {
          const data = (await dres.json()) as { members?: AdminGroupMember[] };
          setMembers(data.members ?? []);
        }
      } catch {
        /* la recarga es best-effort; la regla ya se guardó */
      }
      toast.notify({
        tone: 'success',
        title: t('groups.toast.ruleSaved'),
        description: t('groups.toast.ruleSavedHint'),
      });
    } catch (err) {
      toast.notify({
        tone: 'danger',
        title: t('groups.toast.ruleSaveFailed'),
        description:
          errorCode(err) === '400' ? t('groups.toast.ruleInvalid') : t('common.tryAgain'),
      });
    } finally {
      setSavingRule(false);
    }
  }

  async function addMember(u: DirectoryUser) {
    if (!selected || busyUser) return;
    setBusyUser(u.id);
    try {
      const res = await fetch(`/api/admin/groups/${encodeURIComponent(selected.id)}/members`, {
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
        title: t('groups.toast.addFailed'),
        description:
          errorCode(err) === '409' ? t('groups.toast.alreadyMember') : t('common.tryAgain'),
      });
    } finally {
      setBusyUser('');
    }
  }

  async function removeMember(m: AdminGroupMember) {
    if (!selected || busyUser) return;
    setBusyUser(m.userId);
    try {
      const res = await fetch(
        `/api/admin/groups/${encodeURIComponent(selected.id)}/members/${encodeURIComponent(m.userId)}`,
        { method: 'DELETE' },
      );
      if (!res.ok) throw new Error(String(res.status));
      setMembers((prev) => prev.filter((it) => it.userId !== m.userId));
      bump(selected.id, -1);
    } catch {
      toast.notify({
        tone: 'danger',
        title: t('groups.toast.removeMemberFailed'),
        description: t('common.tryAgain'),
      });
    } finally {
      setBusyUser('');
    }
  }

  async function deleteGroup() {
    if (!selected || deleting) return;
    setDeleting(true);
    try {
      const res = await fetch(`/api/admin/groups/${encodeURIComponent(selected.id)}`, {
        method: 'DELETE',
      });
      if (!res.ok) throw new Error(String(res.status));
      setItems((prev) => prev.filter((it) => it.id !== selected.id));
      toast.notify({
        tone: 'success',
        title: t('groups.toast.deleted'),
        description: t('groups.toast.deletedHint', { name: selected.name }),
      });
      closeModal();
    } catch {
      toast.notify({
        tone: 'danger',
        title: t('groups.toast.deleteFailed'),
        description: t('common.tryAgain'),
      });
      setConfirmingDelete(false);
    } finally {
      setDeleting(false);
    }
  }

  // Usuarios que aún no son miembros. La búsqueda la resuelve el servidor: los
  // `users` de la página son solo las primeras filas del directorio.
  const inGroup = useMemo(() => new Set(members.map((m) => m.userId)), [members]);
  const {
    query: memberQuery,
    setQuery: setMemberQuery,
    candidates,
    buscando: buscandoMiembro,
  } = useUserSearch(users, inGroup);

  return (
    <div class="flex h-full min-h-0 flex-col">
      <SectionHeader
        level={1}
        class="shrink-0"
        title={t('page.groups')}
        subtitle={t('groups.subtitle')}
      />
      {!live ? (
        <p class="border-border bg-warning-bg text-warning-text mx-4 mt-3 rounded-md border px-3 py-2 text-sm">
          {t('groups.offline')}
        </p>
      ) : null}

      <div class="border-border bg-surface mt-3 flex shrink-0 flex-wrap items-center gap-2 border-y px-4 py-2.5">
        <Input
          type="search"
          value={query}
          placeholder={t('groups.searchPlaceholder')}
          aria-label={t('groups.searchPlaceholder')}
          leadingIcon={<Icon name="search" size={16} />}
          class="w-60"
          onInput={(e) => setQuery(e.currentTarget.value)}
        />
        <Button
          variant="secondary"
          class="ml-auto"
          loading={recomputingAll}
          onClick={() => void recomputeAllNow()}
        >
          {t('groups.recomputeAll')}
        </Button>
        <Button
          variant="primary"
          leadingIcon={<Icon name="plus" size={16} />}
          onClick={() => setCreateOpen(true)}
        >
          {t('groups.create')}
        </Button>
      </div>

      <div class="min-h-0 flex-1 overflow-auto p-4">
        {filtered.length === 0 ? (
          <Card>
            <CardBody class="p-0">
              <EmptyState
                title={t(items.length === 0 ? 'groups.empty.title' : 'groups.noResults')}
                description={t(
                  items.length === 0 ? 'groups.empty.description' : 'roles.noResultsHint',
                )}
                illustration={<Icon name="users" size={28} />}
                action={
                  items.length === 0 ? (
                    <Button
                      variant="primary"
                      size="sm"
                      leadingIcon={<Icon name="plus" size={14} />}
                      onClick={() => setCreateOpen(true)}
                    >
                      {t('groups.create')}
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
            {filtered.map((g) => (
              <Card key={g.id}>
                <CardBody class="p-0">
                  <button
                    type="button"
                    class="sytue-focus-ring flex w-full flex-col gap-2 rounded-lg p-4 text-left"
                    onClick={() => void openGroup(g)}
                    aria-haspopup="dialog"
                    aria-label={t('groups.openAria', { name: g.name })}
                  >
                    <span class="flex min-w-0 items-center gap-2">
                      <Icon
                        name="users"
                        size={16}
                        class="text-text-subtle shrink-0"
                        aria-hidden="true"
                      />
                      <span class="text-text min-w-0 flex-1 truncate font-medium" title={g.name}>
                        {g.name}
                      </span>
                      {g.dynamic ? <Badge tone="accent">{t('groups.dynamic')}</Badge> : null}
                      <Badge tone="neutral">{formatNumber(locale, g.memberCount)}</Badge>
                    </span>
                    {g.description ? (
                      <span class="text-text-muted line-clamp-2 text-sm">{g.description}</span>
                    ) : (
                      <span class="text-text-subtle text-sm italic">
                        {t('groups.noDescription')}
                      </span>
                    )}
                  </button>
                </CardBody>
              </Card>
            ))}
          </div>
        )}
      </div>

      {/* Crear grupo */}
      <Modal
        open={createOpen}
        onClose={resetCreate}
        title={t('groups.create')}
        size={newDynamic ? 'md' : 'sm'}
      >
        <form class="flex flex-col gap-3" onSubmit={(e) => void createGroup(e)}>
          <div>
            <label class="text-text mb-1 block text-sm font-medium" for="new-group-name">
              {t('tenants.form.name')}
            </label>
            <Input
              id="new-group-name"
              value={newName}
              placeholder={t('groups.namePlaceholder')}
              onInput={(e) => setNewName(e.currentTarget.value)}
              required
            />
          </div>
          <div>
            <label class="text-text mb-1 block text-sm font-medium" for="new-group-desc">
              {t('groups.description')}{' '}
              <span class="text-text-subtle font-normal">({t('common.optional')})</span>
            </label>
            <Input
              id="new-group-desc"
              maxLength={256}
              value={newDesc}
              onInput={(e) => setNewDesc(e.currentTarget.value)}
            />
          </div>
          <div class="border-border border-t pt-3">
            <Switch
              label={t('groups.dynamicSwitch')}
              checked={newDynamic}
              onChange={(e) => setNewDynamic(e.currentTarget.checked)}
            />
            {newDynamic ? (
              <div class="mt-3 flex flex-col gap-3">
                <p class="text-text-muted text-xs">{t('groups.dynamicHelp')}</p>
                <RuleBuilder rule={newRule} onChange={setNewRule} idPrefix="new-rule" t={t} />
                <PreviewBlock
                  preview={newPreview}
                  loading={previewingNew}
                  onPreview={() => void previewNew()}
                  t={t}
                />
              </div>
            ) : null}
          </div>
          <div class="mt-1 flex justify-end gap-2">
            <Button variant="ghost" size="sm" type="button" onClick={resetCreate}>
              {t('common.cancel')}
            </Button>
            <Button
              variant="primary"
              size="sm"
              type="submit"
              loading={creating}
              disabled={!newName.trim()}
            >
              {t('groups.create')}
            </Button>
          </div>
        </form>
      </Modal>

      {/* Detalle / edición del grupo */}
      <Modal
        open={Boolean(selected)}
        onClose={closeModal}
        title={selected?.name ?? t('groups.fallbackTitle')}
        size="md"
      >
        {selected ? (
          <div class="flex flex-col gap-4">
            <div class="flex flex-col gap-3">
              <div>
                <label class="text-text mb-1 block text-sm font-medium" for="edit-group-name">
                  {t('tenants.form.name')}
                </label>
                <Input
                  id="edit-group-name"
                  value={editName}
                  invalid={editName.trim().length === 0}
                  onInput={(e) => setEditName(e.currentTarget.value)}
                />
              </div>
              <div>
                <label class="text-text mb-1 block text-sm font-medium" for="edit-group-desc">
                  {t('groups.description')}
                </label>
                <Input
                  id="edit-group-desc"
                  maxLength={256}
                  value={editDesc}
                  onInput={(e) => setEditDesc(e.currentTarget.value)}
                />
              </div>
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
                      setEditDesc(selected.description);
                    }}
                  >
                    {t('roles.discard')}
                  </Button>
                </div>
              ) : null}
            </div>

            {/* Membresía dinámica: marcar/editar la regla + preview. */}
            <div class="border-border border-t pt-3">
              <div class="mb-2 flex items-center gap-2">
                <p class="text-text text-sm font-semibold">{t('groups.ruleMembership')}</p>
                {selected.dynamic ? <Badge tone="accent">{t('groups.dynamic')}</Badge> : null}
              </div>
              {!selected.dynamic ? (
                <Switch
                  label={t('groups.convertSwitch')}
                  checked={editDynamic}
                  onChange={(e) => {
                    setEditDynamic(e.currentTarget.checked);
                    setEditPreview(null);
                  }}
                />
              ) : null}
              {editDynamic ? (
                <div class="mt-3 flex flex-col gap-3">
                  {selected.dynamic ? (
                    <p class="text-text-muted text-xs">{t('groups.ruleMembershipHelp')}</p>
                  ) : (
                    <p class="text-warning-text text-xs">{t('groups.convertWarning')}</p>
                  )}
                  <RuleBuilder rule={editRule} onChange={setEditRule} idPrefix="edit-rule" t={t} />
                  <PreviewBlock
                    preview={editPreview}
                    loading={previewingEdit}
                    onPreview={() => void previewEdit()}
                    t={t}
                  />
                  <div class="flex flex-wrap items-center gap-2">
                    <Button
                      variant="primary"
                      size="sm"
                      loading={savingRule}
                      onClick={() => void saveRule()}
                    >
                      {t(selected.dynamic ? 'groups.saveRule' : 'groups.convert')}
                    </Button>
                    {selected.dynamic ? (
                      <Button
                        variant="secondary"
                        size="sm"
                        loading={recomputing}
                        onClick={() => void recomputeNow()}
                      >
                        {t('groups.recompute')}
                      </Button>
                    ) : null}
                  </div>
                  {selected.dynamic ? <p class="sytue-meta">{t('groups.recomputeHelp')}</p> : null}
                </div>
              ) : null}
            </div>

            <div class="border-border border-t pt-3">
              <p class="text-text mb-2 text-sm font-semibold">
                {t(selected.dynamic ? 'groups.computedMembers' : 'groups.members')}{' '}
                {loadingDetail ? '' : `(${formatNumber(locale, members.length)})`}
              </p>
              {loadingDetail ? (
                <div class="text-text-muted flex items-center gap-2 py-3 text-sm">
                  <Spinner size={16} /> {t('common.loading')}
                </div>
              ) : members.length === 0 ? (
                <p class="text-text-subtle py-2 text-sm">
                  {t(selected.dynamic ? 'groups.noRuleMatches' : 'groups.noMembers')}
                </p>
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
                      {selected.dynamic ? null : (
                        <Button
                          variant="ghost"
                          size="sm"
                          loading={busyUser === m.userId}
                          aria-label={t('groups.removeMemberAria', { name: m.name })}
                          onClick={() => void removeMember(m)}
                        >
                          <Icon name="close" size={14} />
                        </Button>
                      )}
                    </li>
                  ))}
                </ul>
              )}
            </div>

            {!loadingDetail && !selected.dynamic ? (
              <div class="border-border border-t pt-3">
                <label class="text-text mb-1 block text-sm font-medium" for="member-picker">
                  {t('groups.addMember')}
                </label>
                <Input
                  id="member-picker"
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

            <div class="border-border border-t pt-3">
              {confirmingDelete ? (
                <div class="flex flex-col gap-2">
                  <p class="text-text-muted text-sm">
                    {t('groups.deleteConfirm', { name: selected.name })}
                  </p>
                  <div class="flex gap-2">
                    <Button
                      variant="danger"
                      size="sm"
                      id="delete-group-confirm"
                      loading={deleting}
                      onClick={() => void deleteGroup()}
                    >
                      {t('groups.delete')}
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
                      document.getElementById('delete-group-confirm')?.focus(),
                    );
                  }}
                >
                  {t('groups.delete')}
                </Button>
              )}
            </div>
          </div>
        ) : null}
      </Modal>
    </div>
  );
}

export default function GroupsPanel({ locale, groups, users, live }: GroupsPanelProps) {
  return (
    <ToastProvider>
      <Inner locale={locale} groups={groups} users={users} live={live ?? true} />
    </ToastProvider>
  );
}
