/**
 * TenantsPanel — administración de organizaciones (tenants) de Sytue ID. Tabla con
 * plan, usuarios, dominio de correo y namespace; buscador, alta real (POST al BFF),
 * edición de ajustes (nombre/estado), MULTI-PERTENENCIA (añadir a alguien que ya
 * existe a otra organización) y paginación keyset («Cargar más»). Cada tenant ⇄
 * grupo org/<slug> ⇄ namespace Kapsule ⇄ servicios Sytue. El slug es la identidad
 * inmutable del tenant. docs/CONVENTIONS.md §9.
 *
 * La acción de añadir vive AQUÍ, en la fila de la organización de destino, porque
 * es la organización la que decide si se puede: la lista solo trae las que quien
 * mira administra (owner/admin), así que toda fila visible es una acción que el
 * servicio va a aceptar. Y el rol owner solo se ofrece donde `org.role` es owner
 * —lo dice el servicio, org por org—; donde no, el formulario explica por qué no
 * está en vez de dejar que el backend responda 403 después de rellenarlo.
 */
import { useMemo, useState } from 'preact/hooks';
import {
  Table,
  Badge,
  Button,
  EmptyState,
  Icon,
  Input,
  Select,
  Modal,
  FieldGroup,
  SectionHeader,
  ToastProvider,
  useToast,
} from '@sytue/ui';
import type { TableColumn } from '@sytue/ui';
import { contiene, formatNumber, type Locale } from '@sytue/i18n';
import { useT, type T } from '../i18n/index.js';
import { r } from '../lib/rutas.js';
import type { DirectoryUser, SytueRole, Tenant } from '../lib/types.js';
import { useUserSearch } from '../lib/useUserSearch.js';

// «info» y no «primary»: mismo par claro, pero con reasignación en modo oscuro.
const planTone = { starter: 'neutral', business: 'info', enterprise: 'info' } as const;

// Roles ofrecibles al añadir a alguien, de menos a más privilegio. `owner` NO
// está: se añade solo cuando quien mira es owner de ESA organización, porque es
// el único que puede concederlo (anti auto-escalada, y el servicio lo comprueba).
const ROLES_ASIGNABLES: readonly SytueRole[] = ['member', 'billing', 'admin'];

// El buscador de personas no precarga el directorio: esta página va de
// organizaciones, y traerse la plantilla entera por si se abre un diálogo es
// trabajo que casi nunca se usa. Constantes estables para no rehacerlas en cada
// render (useUserSearch las lee al pintar, no en un efecto).
const SIN_PRECARGA: DirectoryUser[] = [];
const SIN_EXCLUIDOS = new Set<string>();

interface TenantsPanelProps {
  tenants: Tenant[];
  /** Cursor keyset de la página siguiente ('' = lista completa cargada). */
  nextCursor?: string;
  /** Idioma ya negociado en el servidor; llega por props. */
  locale: Locale;
}

function Inner({ tenants, nextCursor = '', locale, t }: TenantsPanelProps & { t: T }) {
  const toast = useToast();
  const [items, setItems] = useState<Tenant[]>(tenants);
  const [query, setQuery] = useState('');
  const [cursor, setCursor] = useState(nextCursor);
  const [loadingMore, setLoadingMore] = useState(false);

  // Alta.
  const [open, setOpen] = useState(false);
  const [name, setName] = useState('');
  const [slug, setSlug] = useState('');
  const [creating, setCreating] = useState(false);

  // Edición de ajustes.
  const [editing, setEditing] = useState<Tenant | null>(null);
  const [editName, setEditName] = useState('');
  const [editStatus, setEditStatus] = useState<'active' | 'suspended'>('active');
  const [savingEdit, setSavingEdit] = useState(false);

  // Multi-pertenencia: añadir a alguien que YA existe a esta organización.
  const [addingTo, setAddingTo] = useState<Tenant | null>(null);
  const [memberRole, setMemberRole] = useState<SytueRole>('member');
  const [addingMember, setAddingMember] = useState(false);
  // El término del buscador ES el correo que se envía: quien no aparezca en el
  // directorio de la organización activa (está en una tercera) se añade
  // escribiendo su correo entero, y elegir un candidato solo lo rellena.
  const {
    query: person,
    setQuery: setPerson,
    candidates,
    buscando,
  } = useUserSearch(SIN_PRECARGA, SIN_EXCLUIDOS);

  const filtered = useMemo(() => {
    const q = query.trim().toLowerCase();
    if (!q) return items;
    return items.filter((org) => contiene(org.name, q) || contiene(org.slug, q));
  }, [items, query]);

  // Página siguiente por cursor keyset vía el BFF (el token nunca está aquí).
  async function loadMore() {
    if (!cursor || loadingMore) return;
    setLoadingMore(true);
    try {
      const res = await fetch(`/api/admin/tenants?cursor=${encodeURIComponent(cursor)}`, {
        headers: { accept: 'application/json' },
      });
      if (!res.ok) throw new Error(String(res.status));
      const page = (await res.json()) as { items?: Tenant[]; nextCursor?: string };
      setItems((prev) => [...prev, ...(page.items ?? [])]);
      setCursor(typeof page.nextCursor === 'string' ? page.nextCursor : '');
    } catch {
      toast.notify({
        tone: 'danger',
        title: t('users.toast.loadMoreFailed'),
        description: t('tenants.toast.loadMoreFailedHint'),
      });
    } finally {
      setLoadingMore(false);
    }
  }

  // Alta real: Sytue ID crea la org y hace owner al solicitante. Prepende la fila
  // devuelta (con user_count y namespace reales).
  async function create(e: Event) {
    e.preventDefault();
    if (!slug.trim() || !name.trim() || creating) return;
    setCreating(true);
    try {
      const res = await fetch('/api/admin/tenants', {
        method: 'POST',
        headers: { 'content-type': 'application/json' },
        body: JSON.stringify({ slug: slug.trim(), name: name.trim() }),
      });
      if (!res.ok) throw new Error(String(res.status));
      const created = (await res.json()) as Tenant;
      setItems((prev) => [created, ...prev]);
      setOpen(false);
      setName('');
      setSlug('');
      toast.notify({
        tone: 'success',
        title: t('tenants.toast.created'),
        description: t('tenants.toast.createdHint', {
          slug: created.slug,
          namespace: created.namespace,
        }),
      });
    } catch (err) {
      const code = err instanceof Error ? err.message : '';
      toast.notify({
        tone: 'danger',
        title: t('tenants.toast.createFailed'),
        description:
          code === '409'
            ? t('tenants.toast.slugTaken')
            : code === '400'
              ? t('tenants.toast.invalidSlug')
              : t('common.tryAgain'),
      });
    } finally {
      setCreating(false);
    }
  }

  function openEdit(org: Tenant) {
    setEditing(org);
    setEditName(org.name);
    setEditStatus(org.status);
  }

  function openAdd(org: Tenant) {
    setAddingTo(org);
    setPerson('');
    setMemberRole('member');
  }

  // Añade a una persona QUE YA EXISTE a esta organización: una identidad, dos
  // membresías (no toca las que ya tiene en otras organizaciones).
  //
  // Los errores NO se aplanan. En un 403 el motivo del servicio es lo único que
  // distingue «te han quitado la administración de esta org» de «solo un owner
  // nombra owners» —los dos son `code: forbidden`—, así que se muestra tal cual.
  // El 404 es al revés: el servicio dice «Resource not found» y quien lee
  // necesita saber que no hay cuenta con ese correo y que lo suyo es invitar.
  async function addPerson(e: Event) {
    e.preventDefault();
    const org = addingTo;
    const email = person.trim();
    if (!org || !email || addingMember) return;
    setAddingMember(true);
    try {
      const res = await fetch(`/api/admin/tenants/${encodeURIComponent(org.slug)}/members`, {
        method: 'POST',
        headers: { 'content-type': 'application/json' },
        body: JSON.stringify({ email, role: memberRole }),
      });
      const cuerpo = (await res.json().catch(() => ({}))) as { message?: string };
      if (!res.ok) {
        toast.notify({
          tone: 'danger',
          title: t('tenants.member.failed'),
          description:
            res.status === 404
              ? t('tenants.member.noAccount', { email })
              : (cuerpo.message ?? t('common.tryAgain')),
        });
        return;
      }
      setAddingTo(null);
      // ponytail: el recuento de la fila no se incrementa. La alta es un upsert —
      // añadir a quien ya estaba solo le cambia el rol— así que sumar uno mentiría
      // en ese caso; el número se pone al día al recargar. Si hiciera falta al
      // instante, que la respuesta traiga el user_count en vez de adivinarlo.
      toast.notify({
        tone: 'success',
        title: t('tenants.member.added'),
        description: t('tenants.member.addedHint', { email, org: org.name }),
      });
    } catch {
      toast.notify({
        tone: 'danger',
        title: t('tenants.member.failed'),
        description: t('common.tryAgain'),
      });
    } finally {
      setAddingMember(false);
    }
  }

  // Editar ajustes (nombre/estado). Sytue ID exige owner/admin de ESA org y aplica
  // el aislamiento cross-tenant. Optimista con revert si el servicio lo rechaza.
  async function saveEdit(e: Event) {
    e.preventDefault();
    if (!editing || savingEdit || !editName.trim()) return;
    const target = editing;
    const previous = items;
    setSavingEdit(true);
    setItems((prev) =>
      prev.map((it) =>
        it.slug === target.slug ? { ...it, name: editName.trim(), status: editStatus } : it,
      ),
    );
    try {
      const res = await fetch(`/api/admin/tenants/${encodeURIComponent(target.slug)}`, {
        method: 'PATCH',
        headers: { 'content-type': 'application/json' },
        body: JSON.stringify({ name: editName.trim(), status: editStatus }),
      });
      if (!res.ok) throw new Error(String(res.status));
      setEditing(null);
      toast.notify({
        tone: 'success',
        title: t('tenants.toast.settingsSaved'),
        description: target.slug,
      });
    } catch (err) {
      setItems(previous);
      const code = err instanceof Error ? err.message : '';
      toast.notify({
        tone: 'danger',
        title: t('tenants.toast.settingsFailed'),
        description: code === '404' ? t('tenants.toast.noAccess') : t('common.tryAgain'),
      });
    } finally {
      setSavingEdit(false);
    }
  }

  const columns: TableColumn<Tenant>[] = [
    {
      key: 'name',
      header: t('users.col.tenant'),
      cell: (org) => (
        <a
          href={r(`/tenants/${org.slug}`)}
          class="group text-text sytue-focus-ring -mx-1 flex items-center gap-2 rounded-md px-1 py-0.5 font-medium"
        >
          <Icon name="building" size={14} class="text-text-subtle shrink-0" />
          <span class="min-w-0">
            <span class="block truncate group-hover:underline" title={org.name}>
              {org.name}
            </span>
            <span class="sytue-meta block truncate font-mono font-normal">org/{org.slug}</span>
          </span>
        </a>
      ),
    },
    {
      key: 'plan',
      header: t('tenants.col.plan'),
      cell: (org) => <Badge tone={planTone[org.plan]}>{org.plan}</Badge>,
    },
    {
      key: 'users',
      header: t('nav.users'),
      align: 'end',
      cell: (org) => formatNumber(locale, org.userCount),
    },
    {
      key: 'mail',
      header: t('tenants.mailDomain'),
      cell: (org) => <span class="text-text-muted font-mono text-sm">{org.mailDomain}</span>,
    },
    {
      key: 'ns',
      header: t('tenants.col.namespace'),
      cell: (org) => <span class="text-text-muted font-mono text-sm">{org.namespace}</span>,
    },
    {
      key: 'status',
      header: t('users.col.status'),
      cell: (org) => (
        <Badge tone={org.status === 'active' ? 'success' : 'warning'}>
          {org.status === 'active' ? t('tenants.statusActive') : t('tenants.statusSuspended')}
        </Badge>
      ),
    },
    {
      key: 'actions',
      header: '',
      align: 'end',
      cell: (org) => (
        <div class="flex items-center justify-end gap-1">
          <Button
            variant="ghost"
            size="sm"
            leadingIcon={<Icon name="users" size={14} />}
            aria-label={t('tenants.member.actionFor', { name: org.name })}
            onClick={() => openAdd(org)}
          >
            {t('tenants.member.action')}
          </Button>
          <Button
            variant="ghost"
            size="sm"
            leadingIcon={<Icon name="settings" size={14} />}
            aria-label={t('tenants.settingsFor', { name: org.name })}
            onClick={() => openEdit(org)}
          >
            {t('tenants.settings')}
          </Button>
        </div>
      ),
    },
  ];

  return (
    <div>
      <SectionHeader level={1} title={t('page.tenants')} subtitle={t('tenants.subtitle')} />
      <div class="border-border bg-surface flex flex-wrap items-center gap-2 border-b px-4 py-2.5">
        <Input
          type="search"
          value={query}
          placeholder={t('tenants.searchPlaceholder')}
          aria-label={t('tenants.searchPlaceholder')}
          leadingIcon={<Icon name="search" size={16} />}
          class="w-60"
          onInput={(e) => setQuery(e.currentTarget.value)}
        />
        <Button
          variant="primary"
          class="ml-auto"
          leadingIcon={<Icon name="plus" size={16} />}
          onClick={() => setOpen(true)}
        >
          {t('dashboard.tenants.newTenant')}
        </Button>
      </div>

      <div class="px-4 py-4">
        <Table
          columns={columns}
          rows={filtered}
          rowKey={(org) => org.slug}
          caption={t('page.tenants')}
          emptyContent={
            items.length === 0 ? (
              <EmptyState
                title={t('tenants.empty.title')}
                description={t('tenants.empty.description')}
                illustration={<Icon name="grid" size={28} />}
                action={
                  <Button
                    variant="primary"
                    size="sm"
                    leadingIcon={<Icon name="plus" size={14} />}
                    onClick={() => setOpen(true)}
                  >
                    {t('dashboard.tenants.newTenant')}
                  </Button>
                }
              />
            ) : (
              <EmptyState
                title={t('tenants.noResults', { query: query.trim() })}
                description={t('tenants.noResultsHint')}
                illustration={<Icon name="search" size={28} />}
                action={
                  <Button variant="secondary" size="sm" onClick={() => setQuery('')}>
                    {t('tenants.clearSearch')}
                  </Button>
                }
              />
            )
          }
        />
        {cursor ? (
          <div class="mt-3 flex justify-center">
            <Button
              variant="secondary"
              size="sm"
              disabled={loadingMore}
              onClick={() => void loadMore()}
            >
              {loadingMore ? t('common.loading') : t('common.loadMore')}
            </Button>
          </div>
        ) : null}
      </div>

      <Modal open={open} onClose={() => setOpen(false)} title={t('dashboard.tenants.newTenant')}>
        <form class="flex flex-col gap-4" onSubmit={(e) => void create(e)}>
          <FieldGroup label={t('tenants.form.name')} htmlFor="t-name" required>
            <Input
              id="t-name"
              value={name}
              placeholder={t('tenants.form.namePlaceholder')}
              required
              onInput={(e) => setName(e.currentTarget.value)}
            />
          </FieldGroup>
          <FieldGroup
            label={t('tenants.form.slug')}
            htmlFor="t-slug"
            required
            help={t('tenants.form.slugHelp')}
          >
            <Input
              id="t-slug"
              value={slug}
              placeholder="acme"
              class="font-mono"
              required
              onInput={(e) => setSlug(e.currentTarget.value)}
            />
          </FieldGroup>
          <div class="flex justify-end gap-2">
            <Button variant="secondary" type="button" onClick={() => setOpen(false)}>
              {t('common.cancel')}
            </Button>
            <Button
              variant="primary"
              type="submit"
              loading={creating}
              disabled={!slug.trim() || !name.trim()}
            >
              {t('common.create')}
            </Button>
          </div>
        </form>
      </Modal>

      <Modal
        open={Boolean(editing)}
        onClose={() => setEditing(null)}
        title={editing ? t('tenants.settingsOf', { name: editing.name }) : t('tenants.settings')}
        size="sm"
      >
        {editing ? (
          <form class="flex flex-col gap-4" onSubmit={(e) => void saveEdit(e)}>
            <p class="text-text-muted text-sm">
              {t('tenants.form.slugImmutable', { slug: `org/${editing.slug}` })}
            </p>
            <FieldGroup label={t('tenants.form.name')} htmlFor="e-name" required>
              <Input
                id="e-name"
                value={editName}
                required
                onInput={(e) => setEditName(e.currentTarget.value)}
              />
            </FieldGroup>
            <FieldGroup label={t('users.col.status')} htmlFor="e-status">
              <Select
                id="e-status"
                value={editStatus}
                onChange={(e) => setEditStatus(e.currentTarget.value as 'active' | 'suspended')}
              >
                <option value="active">{t('tenants.statusActive')}</option>
                <option value="suspended">{t('tenants.statusSuspended')}</option>
              </Select>
            </FieldGroup>
            <div class="flex justify-end gap-2">
              <Button variant="secondary" type="button" onClick={() => setEditing(null)}>
                {t('common.cancel')}
              </Button>
              <Button
                variant="primary"
                type="submit"
                loading={savingEdit}
                disabled={!editName.trim()}
              >
                {t('common.save')}
              </Button>
            </div>
          </form>
        ) : null}
      </Modal>

      <Modal
        open={Boolean(addingTo)}
        onClose={() => setAddingTo(null)}
        title={addingTo ? t('tenants.member.title', { name: addingTo.name }) : ''}
        size="sm"
      >
        {addingTo ? (
          <form class="flex flex-col gap-4" onSubmit={(e) => void addPerson(e)}>
            <FieldGroup
              label={t('tenants.member.person')}
              htmlFor="m-person"
              required
              help={t('tenants.member.personHelp')}
            >
              <Input
                id="m-person"
                type="search"
                value={person}
                placeholder={t('tenants.member.personPlaceholder')}
                leadingIcon={<Icon name="search" size={16} />}
                required
                onInput={(e) => setPerson(e.currentTarget.value)}
              />
            </FieldGroup>
            {person.trim() ? (
              candidates.length === 0 ? (
                <p class="text-text-subtle -mt-2 text-sm">
                  {buscando ? t('common.loading') : t('tenants.member.noCandidates')}
                </p>
              ) : (
                <ul class="-mt-2 flex flex-col gap-1">
                  {candidates.map((u) => (
                    <li key={u.id}>
                      <button
                        type="button"
                        class="hover:bg-bg-subtle sytue-focus-ring flex w-full items-center gap-2 rounded-md px-2 py-1.5 text-left"
                        onClick={() => setPerson(u.email)}
                      >
                        <Icon
                          name="user"
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
            ) : null}
            <FieldGroup
              label={t('tenants.member.role')}
              htmlFor="m-role"
              help={
                addingTo.role === 'owner'
                  ? t('tenants.member.roleHelp')
                  : t('tenants.member.roleHelpAdmin', { name: addingTo.name })
              }
            >
              <Select
                id="m-role"
                value={memberRole}
                onChange={(e) => setMemberRole(e.currentTarget.value as SytueRole)}
              >
                {ROLES_ASIGNABLES.map((r) => (
                  <option key={r} value={r}>
                    {t(`role.${r}` as const)}
                  </option>
                ))}
                {/* Solo un owner puede nombrar a otro owner: donde quien mira no lo
                    es, la opción NO existe y el `help` de arriba dice por qué. */}
                {addingTo.role === 'owner' ? (
                  <option value="owner">{t('role.owner')}</option>
                ) : null}
              </Select>
            </FieldGroup>
            <div class="flex justify-end gap-2">
              <Button variant="secondary" type="button" onClick={() => setAddingTo(null)}>
                {t('common.cancel')}
              </Button>
              <Button
                variant="primary"
                type="submit"
                loading={addingMember}
                disabled={!person.trim()}
              >
                {t('tenants.member.submit')}
              </Button>
            </div>
          </form>
        ) : null}
      </Modal>
    </div>
  );
}

export default function TenantsPanel({ tenants, nextCursor, locale }: TenantsPanelProps) {
  return (
    <ToastProvider>
      <Inner tenants={tenants} nextCursor={nextCursor ?? ''} locale={locale} t={useT(locale)} />
    </ToastProvider>
  );
}
