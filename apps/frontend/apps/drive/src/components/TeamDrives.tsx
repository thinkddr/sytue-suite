/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * TeamDrivesView — «Unidades de equipo» de Sytue Drive: unidades propiedad de la
 * ORGANIZACIÓN (no de un usuario), con acceso por MEMBRESÍA y por ROL (Lector <
 * Editor < Gestor). Lista mis unidades, crea nuevas, abre una para navegar su
 * contenido (crear carpeta / subir / descargar / eliminar según el rol) y —solo un
 * Gestor— renombra/archiva la unidad y administra sus miembros.
 *
 * La raíz es una página de AJUSTES (SettingsShell/SettingsSection): explica en una
 * frase la diferencia con «Mi unidad» (la propiedad es de la organización) y cuenta
 * cada rol por lo que PERMITE HACER, no por su nombre. El detalle de una unidad
 * sigue siendo un explorador de archivos.
 *
 * Isla Preact: habla SOLO con el BFF de esta app (/api/team-drives,
 * /api/team-drive-content, /api/team-drive-members) por cookie de sesión httpOnly;
 * la verja deny-by-default y por rol la impone el servicio Go (no-miembro → 404,
 * rol insuficiente → 403). El access token NUNCA llega al cliente. Componentes
 * canónicos @sytue/ui; sobrio, claro/oscuro, sin UI ad-hoc.
 *
 * Idioma: lo publica <SytueShell> en el contexto de i18n a partir del locale que negoció
 * el SERVIDOR (`src/middleware.ts`). Aquí solo se LEE (`useI18n`): detectarlo en el
 * cliente desajustaría la hidratación. Los tamaños se formatean con ese locale
 * (docs/CONVENTIONS.md §13.1).
 */
import { useCallback, useEffect, useRef, useState } from 'preact/hooks';
import {
  Button,
  Icon,
  EmptyState,
  FieldHint,
  Modal,
  Input,
  Select,
  FieldGroup,
  SettingsShell,
  SettingsSection,
  Skeleton,
  useToast,
  cx,
  Illustration,
  withBase,
} from '@sytue/ui';
import type { Locale } from '@sytue/i18n';
import { useI18n } from '@sytue/i18n/preact';
import { useT, type T, type DriveMessageKey } from '../i18n';
import { Glyph } from './glyphs';
import { formatBytes } from '@sytue/ui';
import { PreviewDialog, previewKind } from './FileBrowser';
import type { DriveResource } from '@sytue/api-clients';

// --- tipos ---------------------------------------------------------------

type Role = 'viewer' | 'editor' | 'manager';

interface TeamDrive {
  id: string;
  name: string;
  role: Role;
  archived_at?: string | null;
}

interface TeamNode {
  id: string;
  name: string;
  isDirectory: boolean;
  size?: number;
  contentType?: string;
  modifiedAt?: string;
}

interface Member {
  subject: string;
  role: Role;
}

/** Error con código HTTP del BFF, para distinguir 401 (sesión) / 403 / 404 / 409. */
class BffError extends Error {
  constructor(public readonly status: number) {
    super(`bff_${status}`);
  }
}

async function bffJson<T>(input: RequestInfo, init?: RequestInit): Promise<T> {
  const res = await fetch(input, { credentials: 'include', ...init });
  if (!res.ok) throw new BffError(res.status);
  return (await res.json()) as T;
}

const api = {
  list: () => bffJson<{ items: TeamDrive[] }>('/api/team-drives').then((r) => r.items),
  create: (name: string) =>
    bffJson<TeamDrive>('/api/team-drives', {
      method: 'POST',
      headers: { 'content-type': 'application/json' },
      body: JSON.stringify({ name }),
    }),
  rename: (id: string, name: string) =>
    bffJson<TeamDrive>('/api/team-drives', {
      method: 'POST',
      headers: { 'content-type': 'application/json' },
      body: JSON.stringify({ id, name }),
    }),
  setArchived: (id: string, archived: boolean) =>
    bffJson<TeamDrive>('/api/team-drives', {
      method: 'POST',
      headers: { 'content-type': 'application/json' },
      body: JSON.stringify({ id, archived }),
    }),
  children: (id: string, folder?: string, cursor?: string) => {
    const qs = new URLSearchParams({ id });
    if (folder) qs.set('folder', folder);
    if (cursor) qs.set('cursor', cursor);
    return bffJson<{ role: Role; canWrite: boolean; items: TeamNode[]; nextCursor?: string }>(
      `/api/team-drive-content?${qs.toString()}`,
    );
  },
  createFolder: (id: string, name: string, parentId?: string) =>
    bffJson<{ resource: TeamNode }>(`/api/team-drive-content?id=${encodeURIComponent(id)}`, {
      method: 'POST',
      headers: { 'content-type': 'application/json' },
      body: JSON.stringify(parentId ? { name, parent_id: parentId } : { name }),
    }),
  upload: (id: string, folder: string | undefined, file: File) => {
    const qs = new URLSearchParams({ id, name: file.name, type: file.type || '' });
    if (folder) qs.set('folder', folder);
    return bffJson<{ resource: TeamNode }>(`/api/team-drive-content?${qs.toString()}`, {
      method: 'PUT',
      headers: { 'content-type': 'application/octet-stream' },
      body: file,
    });
  },
  trash: (id: string, node: TeamNode) => {
    const key = node.isDirectory ? 'folder' : 'file';
    return bffJson<{ ok: true }>(
      `/api/team-drive-content?id=${encodeURIComponent(id)}&${key}=${encodeURIComponent(node.id)}`,
      { method: 'DELETE', headers: { 'content-type': 'application/octet-stream' } },
    );
  },
  members: (id: string) =>
    bffJson<{ members: Member[] }>(`/api/team-drive-members?id=${encodeURIComponent(id)}`).then(
      (r) => r.members,
    ),
  putMember: (id: string, subject: string, role: Role) =>
    bffJson<Member>(`/api/team-drive-members?id=${encodeURIComponent(id)}`, {
      method: 'POST',
      headers: { 'content-type': 'application/json' },
      body: JSON.stringify({ subject, role }),
    }),
  removeMember: (id: string, subject: string) =>
    bffJson<{ ok: true }>(
      `/api/team-drive-members?id=${encodeURIComponent(id)}&subject=${encodeURIComponent(subject)}`,
      { method: 'DELETE', headers: { 'content-type': 'application/octet-stream' } },
    ),
};

/**
 * Un nodo de unidad de equipo visto como recurso de Drive.
 *
 * Solo para poder REUTILIZAR el diálogo de vista previa, que habla ese tipo. El
 * `path` va vacío a propósito: en una unidad de equipo el fichero se pide por
 * identificador, no por ruta, y dejar aquí una ruta inventada haría que el
 * diálogo ofreciera enlaces que llevan a un 404.
 */
function comoRecurso(node: TeamNode): DriveResource {
  return {
    id: node.id,
    name: node.name,
    path: '',
    isDirectory: node.isDirectory,
    ...(node.size !== undefined ? { size: node.size } : {}),
    ...(node.contentType !== undefined ? { contentType: node.contentType } : {}),
    ...(node.modifiedAt !== undefined ? { modifiedAt: node.modifiedAt } : {}),
  };
}

function downloadHref(id: string, fileId: string): string {
  return withBase(
    import.meta.env.BASE_URL,
    `/api/team-drive-content?id=${encodeURIComponent(id)}&file=${encodeURIComponent(fileId)}`,
  );
}

/** `t` + locale de la isla; el proveedor lo pone <SytueShell> desde el SSR. */
function useTeamT(): { t: T; locale: Locale } {
  const { locale } = useI18n();
  return { t: useT(locale), locale };
}

/** Nombre del rol. */
const ROLE_LABEL: Record<Role, DriveMessageKey> = {
  manager: 'team.role.manager',
  editor: 'team.role.editor',
  viewer: 'team.role.viewer',
};

/** El rol contado por lo que PERMITE HACER, no por su nombre: en segunda persona
 *  para el rol propio («puedes…») y en tercera al elegírselo a otra persona
 *  («podrá…»). Dos claves y no un `replace` sobre el texto: cambiar la persona de
 *  un verbo con una sustitución de cadena solo funciona en español. */
const ROLE_CAN: Record<Role, DriveMessageKey> = {
  viewer: 'team.roleCan.viewer',
  editor: 'team.roleCan.editor',
  manager: 'team.roleCan.manager',
};
const ROLE_WILL: Record<Role, DriveMessageKey> = {
  viewer: 'team.roleWill.viewer',
  editor: 'team.roleWill.editor',
  manager: 'team.roleWill.manager',
};

function canWrite(role: Role): boolean {
  return role === 'editor' || role === 'manager';
}

/** Tamaño legible; el separador decimal lo pone el idioma («1,5 MB» / «1.5 MB»).
 *  Las unidades (B, KB, MB…) son símbolos y NO se traducen. */
function formatSize(locale: Locale, bytes?: number): string {
  return bytes === undefined ? '—' : formatBytes(locale, bytes);
}

// --- distintivos ---------------------------------------------------------

/* Un solo juego de tokens, los de superficie clara. Hubo un `chrome` para la
 * variante sobre la cabecera grafito —ahí `text-muted` y `primary-text` no
 * llegaban a 3:1—; con la cabecera en crema esa rama se quedó sin llamadas. */
function RoleBadge({ role, t }: { role: Role; t: T }) {
  const label = t(ROLE_LABEL[role]);
  return (
    <span
      class={cx(
        'inline-flex shrink-0 items-center gap-1 rounded-full border px-2 py-0.5 text-xs font-medium',
        role === 'manager'
          ? 'border-primary/40 text-primary-text'
          : role === 'editor'
            ? 'border-border text-text-muted'
            : 'border-border text-text-subtle',
      )}
      title={t('team.roleBadgeTitle', { role: label })}
    >
      {label}
    </span>
  );
}

function ArchivedBadge({ t }: { t: T }) {
  return (
    <span
      class={cx(
        'inline-flex shrink-0 items-center gap-1 rounded-full border px-2 py-0.5 text-xs font-medium',
        'border-border text-text-subtle',
      )}
      title={t('team.archivedBadgeTitle')}
    >
      <Glyph name="lock" size={12} />
      {t('team.archivedBadge')}
    </span>
  );
}

// --- vista raíz: lista de unidades + crear -------------------------------

export function TeamDrivesView() {
  const toast = useToast();
  const { t } = useTeamT();
  const [drives, setDrives] = useState<TeamDrive[] | null>(null);
  const [error, setError] = useState<'session' | 'network' | null>(null);
  const [open, setOpen] = useState<TeamDrive | null>(null);
  const [createOpen, setCreateOpen] = useState(false);
  const [newName, setNewName] = useState('');
  const [creating, setCreating] = useState(false);

  const load = useCallback(async () => {
    setDrives(null);
    setError(null);
    try {
      setDrives(await api.list());
    } catch (e) {
      setError(e instanceof BffError && e.status === 401 ? 'session' : 'network');
    }
  }, []);

  useEffect(() => {
    void load();
  }, [load]);

  const onCreate = async () => {
    const name = newName.trim();
    if (!name || creating) return;
    setCreating(true);
    try {
      const created = await api.create(name);
      toast.notify({ tone: 'success', title: t('team.created', { name }) });
      setCreateOpen(false);
      setNewName('');
      await load();
      setOpen(created);
    } catch {
      toast.notify({ tone: 'danger', title: t('team.createFailed') });
    } finally {
      setCreating(false);
    }
  };

  if (open) {
    return (
      <TeamDriveDetail
        key={open.id}
        drive={open}
        onBack={() => {
          setOpen(null);
          void load();
        }}
      />
    );
  }

  return (
    <div class="h-full overflow-y-auto">
      <SettingsShell
        title={t('team.title')}
        // La frase que separa esto de «Mi unidad»: aquí la propiedad es de la
        // ORGANIZACIÓN. Es la única diferencia que cambia decisiones reales.
        description={t('team.description')}
        actions={
          <Button leadingIcon={<Icon name="plus" size={16} />} onClick={() => setCreateOpen(true)}>
            {t('team.new')}
          </Button>
        }
      >
        <SettingsSection title={t('team.listTitle')} description={t('team.listDescription')}>
          {drives === null && !error ? (
            <TeamDriveListSkeleton />
          ) : error ? (
            <EmptyState
              title={t(error === 'session' ? 'team.error.sessionTitle' : 'team.error.listTitle')}
              description={t(error === 'session' ? 'team.error.sessionBody' : 'team.error.network')}
              illustration={<Icon name="warning" size={40} class="text-danger-text" />}
              action={
                error === 'session' ? (
                  <Button size="sm" onClick={() => location.reload()}>
                    {t('team.error.sessionCta')}
                  </Button>
                ) : (
                  <Button variant="secondary" size="sm" onClick={() => void load()}>
                    {t('team.retry')}
                  </Button>
                )
              }
            />
          ) : drives && drives.length === 0 ? (
            <EmptyState
              title={t('team.empty.title')}
              description={t('team.empty.body')}
              illustration={<Illustration name="emptyDrive" />}
              action={
                <Button
                  size="sm"
                  leadingIcon={<Icon name="plus" size={16} />}
                  onClick={() => setCreateOpen(true)}
                >
                  {t('team.empty.cta')}
                </Button>
              }
            />
          ) : (
            <ul class="divide-border/60 border-border divide-y rounded-md border">
              {drives!.map((d) => (
                <li
                  key={d.id}
                  class="group hover:bg-bg-subtle flex items-center gap-3 px-3 py-2.5 transition-colors duration-150"
                >
                  <span class="text-text-muted grid h-9 w-9 shrink-0 place-items-center">
                    <Glyph name="users" size={22} />
                  </span>
                  <span class="min-w-0 flex-1">
                    <button
                      type="button"
                      onClick={() => setOpen(d)}
                      title={t('team.openTitle', { name: d.name })}
                      class="text-text sytue-focus-ring block max-w-full truncate rounded text-left font-medium hover:underline"
                    >
                      {d.name}
                    </button>
                    <span class="sytue-meta mt-0.5 block">{t(ROLE_CAN[d.role])}</span>
                  </span>
                  {d.archived_at ? <ArchivedBadge t={t} /> : null}
                  <RoleBadge role={d.role} t={t} />
                  <Button variant="ghost" size="sm" onClick={() => setOpen(d)}>
                    {t('team.open')}
                  </Button>
                </li>
              ))}
            </ul>
          )}
        </SettingsSection>
      </SettingsShell>

      <Modal
        open={createOpen}
        onClose={() => setCreateOpen(false)}
        title={t('team.create.title')}
        footer={
          <>
            <Button variant="ghost" onClick={() => setCreateOpen(false)}>
              {t('team.create.cancel')}
            </Button>
            <Button onClick={() => void onCreate()} loading={creating} disabled={!newName.trim()}>
              {t('team.create.submit')}
            </Button>
          </>
        }
      >
        <FieldGroup label={t('team.create.nameLabel')} htmlFor="team-drive-name">
          <Input
            id="team-drive-name"
            value={newName}
            placeholder={t('team.create.namePlaceholder')}
            aria-describedby="team-drive-name-hint"
            onInput={(e) => setNewName(e.currentTarget.value)}
            onKeyDown={(e) => {
              if (e.key === 'Enter') void onCreate();
            }}
          />
          <FieldHint id="team-drive-name-hint">{t('team.create.nameHint')}</FieldHint>
        </FieldGroup>
        <p class="text-text-muted mt-3 text-sm leading-relaxed">{t('team.create.note')}</p>
      </Modal>
    </div>
  );
}

// --- detalle de una unidad: navegación + miembros ------------------------

function TeamDriveDetail({ drive, onBack }: { drive: TeamDrive; onBack: () => void }) {
  const toast = useToast();
  const { t, locale } = useTeamT();
  // Pila de carpetas navegadas (sin la raíz): breadcrumb + carpeta actual.
  const [stack, setStack] = useState<{ id: string; name: string }[]>([]);
  const [items, setItems] = useState<TeamNode[] | null>(null);
  const [nextCursor, setNextCursor] = useState<string | null>(null);
  const [loadingMore, setLoadingMore] = useState(false);
  const [error, setError] = useState<'denied' | 'network' | null>(null);
  const [busy, setBusy] = useState(false);
  const [name, setName] = useState(drive.name);
  const [archivedAt, setArchivedAt] = useState<string | null>(drive.archived_at ?? null);

  const [newFolderOpen, setNewFolderOpen] = useState(false);
  const [folderName, setFolderName] = useState('');
  const [pendingDelete, setPendingDelete] = useState<TeamNode | null>(null);
  const [renameOpen, setRenameOpen] = useState(false);
  const [renameValue, setRenameValue] = useState(drive.name);
  const [membersOpen, setMembersOpen] = useState(false);
  const [uploading, setUploading] = useState(false);
  const fileInput = useRef<HTMLInputElement>(null);

  const role = drive.role;
  const writable = canWrite(role);
  const isManager = role === 'manager';
  const currentFolder = stack.length > 0 ? stack[stack.length - 1]!.id : undefined;
  const epochRef = useRef(0);

  const load = useCallback(
    async (folder?: string) => {
      const e = ++epochRef.current;
      setItems(null);
      setNextCursor(null);
      setError(null);
      try {
        const page = await api.children(drive.id, folder);
        if (epochRef.current !== e) return;
        setItems(page.items);
        setNextCursor(page.nextCursor ?? null);
      } catch (err) {
        if (epochRef.current !== e) return;
        // 403/404: se revocó tu acceso o la unidad dejó de existir (deny-by-default).
        setError(
          err instanceof BffError && (err.status === 403 || err.status === 404)
            ? 'denied'
            : 'network',
        );
      }
    },
    [drive.id],
  );

  useEffect(() => {
    void load(currentFolder);
  }, [load, currentFolder]);

  const loadMore = useCallback(async () => {
    if (!nextCursor || loadingMore) return;
    setLoadingMore(true);
    const e = epochRef.current;
    try {
      const page = await api.children(drive.id, currentFolder, nextCursor);
      if (epochRef.current !== e) return;
      setItems((prev) => [...(prev ?? []), ...page.items]);
      setNextCursor(page.nextCursor ?? null);
    } catch {
      toast.notify({ tone: 'danger', title: t('team.loadMoreFailed') });
    } finally {
      setLoadingMore(false);
    }
  }, [drive.id, currentFolder, nextCursor, loadingMore, toast, t]);

  const openFolder = (node: TeamNode) => setStack((s) => [...s, { id: node.id, name: node.name }]);
  const goToDepth = (depth: number) => setStack((s) => s.slice(0, depth));

  // Vista previa: la misma que en el resto de Drive. Antes pulsar un fichero
  // aquí lo DESCARGABA, así que la misma acción hacía cosas distintas según la
  // superficie en la que estuvieras.
  const [preview, setPreview] = useState<TeamNode | null>(null);

  const onOpen = (node: TeamNode) => {
    // Sin vista previa para ese tipo, se conserva lo de siempre: descargar. Es
    // mejor que abrir un diálogo vacío para decir que no se puede enseñar.
    if (previewKind(comoRecurso(node))) setPreview(node);
    else onDownload(node);
  };

  const onDownload = (node: TeamNode) => {
    const a = document.createElement('a');
    a.href = downloadHref(drive.id, node.id);
    a.download = node.name;
    a.rel = 'noopener';
    a.click();
  };

  const onCreateFolder = async () => {
    const n = folderName.trim();
    if (!n || busy) return;
    setBusy(true);
    try {
      await api.createFolder(drive.id, n, currentFolder);
      toast.notify({ tone: 'success', title: t('team.newFolder.created', { name: n }) });
      setNewFolderOpen(false);
      setFolderName('');
      await load(currentFolder);
    } catch (e) {
      toast.notify({
        tone: 'danger',
        title: t(
          e instanceof BffError && e.status === 403
            ? 'team.newFolder.denied'
            : 'team.newFolder.failed',
        ),
      });
    } finally {
      setBusy(false);
    }
  };

  const onUpload = async (e: Event) => {
    const input = e.currentTarget as HTMLInputElement;
    const files = Array.from(input.files ?? []);
    input.value = '';
    if (files.length === 0) return;
    setUploading(true);
    let failed = 0;
    for (const file of files) {
      try {
        await api.upload(drive.id, currentFolder, file);
      } catch {
        failed += 1;
      }
    }
    setUploading(false);
    // Plurales por `Intl.PluralRules` (formas del catálogo), no por `length === 1`.
    if (failed === 0) {
      toast.notify({ tone: 'success', title: t('team.upload.done', { count: files.length }) });
    } else {
      toast.notify({
        tone: 'danger',
        title: t('team.upload.partial', {
          uploaded: files.length - failed,
          count: files.length,
        }),
      });
    }
    await load(currentFolder);
  };

  const onDelete = async (node: TeamNode) => {
    setBusy(true);
    try {
      await api.trash(drive.id, node);
      toast.notify({ tone: 'success', title: t('team.delete.done', { name: node.name }) });
      await load(currentFolder);
    } catch {
      toast.notify({ tone: 'danger', title: t('team.delete.failed') });
    } finally {
      setBusy(false);
      setPendingDelete(null);
    }
  };

  const onRename = async () => {
    const n = renameValue.trim();
    if (!n || busy) return;
    if (n === name) {
      setRenameOpen(false);
      return;
    }
    setBusy(true);
    try {
      const updated = await api.rename(drive.id, n);
      setName(updated.name);
      toast.notify({ tone: 'success', title: t('team.rename.done', { name: updated.name }) });
      setRenameOpen(false);
    } catch (e) {
      toast.notify({
        tone: 'danger',
        title: t(
          e instanceof BffError && e.status === 403 ? 'team.rename.denied' : 'team.rename.failed',
        ),
      });
    } finally {
      setBusy(false);
    }
  };

  const onToggleArchive = async () => {
    const next = !archivedAt;
    setBusy(true);
    try {
      const updated = await api.setArchived(drive.id, next);
      setArchivedAt(updated.archived_at ?? null);
      toast.notify({
        tone: 'success',
        title: t(next ? 'team.archive.done' : 'team.unarchive.done'),
      });
    } catch {
      toast.notify({ tone: 'danger', title: t('team.archive.failed') });
    } finally {
      setBusy(false);
    }
  };

  return (
    <div class="flex h-full flex-col">
      {/* Cabecera de la unidad: volver + nombre + rol/estado. */}
      <div class="bg-surface text-text border-border flex flex-wrap items-center gap-x-3 gap-y-2 border-b px-4 py-2.5 sm:px-6">
        <button
          type="button"
          onClick={onBack}
          class="text-text/80 hover:bg-surface-sunken hover:text-text sytue-focus-ring rounded-control inline-flex items-center gap-1 px-2 py-1 text-sm font-medium transition-colors duration-150"
        >
          <Icon name="chevronLeft" size={14} />
          {t('team.back')}
        </button>
        <nav
          aria-label={t('team.breadcrumb')}
          class="flex min-w-0 grow flex-wrap items-center gap-0.5 text-base"
        >
          <Icon name="chevronRight" size={14} class="text-text/50 shrink-0" />
          <button
            type="button"
            onClick={() => goToDepth(0)}
            aria-current={stack.length === 0 ? 'page' : undefined}
            title={name}
            class={cx(
              'hover:bg-surface-sunken sytue-focus-ring rounded-control max-w-[40vw] truncate px-2 py-1 transition-colors duration-150',
              stack.length === 0
                ? 'text-text font-semibold'
                : 'text-text-muted hover:text-text font-medium',
            )}
          >
            {name}
          </button>
          {stack.map((s, i) => {
            const last = i === stack.length - 1;
            return (
              <span key={s.id} class="flex min-w-0 items-center gap-0.5">
                <Icon name="chevronRight" size={14} class="text-text/50 shrink-0" />
                <button
                  type="button"
                  onClick={() => goToDepth(i + 1)}
                  aria-current={last ? 'page' : undefined}
                  title={s.name}
                  class={cx(
                    'hover:bg-surface-sunken sytue-focus-ring rounded-control max-w-[40vw] truncate px-2 py-1 transition-colors duration-150',
                    last
                      ? 'text-text font-semibold'
                      : 'text-text-muted hover:text-text font-medium',
                  )}
                >
                  {s.name}
                </button>
              </span>
            );
          })}
        </nav>

        <div class="ml-auto flex shrink-0 flex-wrap items-center gap-2">
          {archivedAt ? <ArchivedBadge t={t} /> : null}
          <RoleBadge role={role} t={t} />
          {writable ? (
            <>
              <button
                type="button"
                onClick={() => setNewFolderOpen(true)}
                disabled={busy}
                class="text-text hover:bg-surface-sunken active:bg-surface-sunken border-border-strong sytue-focus-ring rounded-control inline-flex h-8 items-center border px-3 text-sm font-semibold transition-colors duration-150 disabled:pointer-events-none disabled:opacity-50"
              >
                {t('team.newFolder')}
              </button>
              <Button
                variant="primary"
                size="sm"
                onClick={() => fileInput.current?.click()}
                disabled={busy}
                loading={uploading}
                leadingIcon={<Icon name="plus" size={16} />}
              >
                {t('team.upload')}
              </Button>
              <input
                ref={fileInput}
                type="file"
                multiple
                class="hidden"
                onChange={(e) => void onUpload(e)}
                aria-hidden="true"
                tabIndex={-1}
              />
            </>
          ) : null}
          {isManager ? (
            <>
              <Button
                variant="secondary"
                size="sm"
                onClick={() => setMembersOpen(true)}
                disabled={busy}
              >
                {t('team.members')}
              </Button>
              <Button
                variant="ghost"
                size="sm"
                onClick={() => {
                  setRenameValue(name);
                  setRenameOpen(true);
                }}
                disabled={busy}
              >
                {t('team.rename')}
              </Button>
              <Button
                variant="ghost"
                size="sm"
                onClick={() => void onToggleArchive()}
                disabled={busy}
              >
                {t(archivedAt ? 'team.unarchive' : 'team.archive')}
              </Button>
            </>
          ) : null}
        </div>
      </div>

      {/* Contenido de la carpeta actual. */}
      <div class="min-h-0 flex-1 overflow-y-auto">
        {items === null && !error ? (
          <TeamDriveListSkeleton />
        ) : error ? (
          <div class="grid h-full place-items-center p-6">
            <EmptyState
              title={t(error === 'denied' ? 'team.detail.deniedTitle' : 'team.detail.loadTitle')}
              description={t(error === 'denied' ? 'team.detail.deniedBody' : 'team.error.network')}
              illustration={<Icon name="warning" size={40} class="text-danger-text" />}
              action={
                error === 'denied' ? (
                  <Button variant="secondary" size="sm" onClick={onBack}>
                    {t('team.detail.backCta')}
                  </Button>
                ) : (
                  <Button variant="secondary" size="sm" onClick={() => void load(currentFolder)}>
                    {t('team.retry')}
                  </Button>
                )
              }
            />
          </div>
        ) : items && items.length === 0 ? (
          <div class="grid h-full place-items-center p-6">
            <EmptyState
              title={t('team.folder.emptyTitle')}
              description={t(writable ? 'team.folder.emptyWritable' : 'team.folder.emptyReadonly')}
              illustration={<Illustration name="emptyDrive" />}
              action={
                writable ? (
                  <Button
                    size="sm"
                    leadingIcon={<Icon name="plus" size={16} />}
                    onClick={() => fileInput.current?.click()}
                  >
                    {t('team.folder.uploadCta')}
                  </Button>
                ) : undefined
              }
            />
          </div>
        ) : (
          <>
            <ul class="px-3 pb-2">
              {items!.map((node) => (
                <li
                  key={node.id}
                  class="group border-border/60 hover:bg-bg-subtle flex items-center gap-3 border-b px-3 py-2 transition-colors duration-150"
                >
                  <span
                    class={cx(
                      'grid h-9 w-9 shrink-0 place-items-center',
                      node.isDirectory ? 'text-primary-text' : 'text-text-muted',
                    )}
                  >
                    <Glyph name={node.isDirectory ? 'folder' : 'file'} size={22} />
                  </span>
                  <span class="min-w-0 flex-1">
                    <button
                      type="button"
                      onClick={() => (node.isDirectory ? openFolder(node) : onOpen(node))}
                      title={t(node.isDirectory ? 'team.item.open' : 'team.item.preview', {
                        name: node.name,
                      })}
                      class="text-text sytue-focus-ring block max-w-full truncate rounded text-left font-medium hover:underline"
                    >
                      {node.name}
                    </button>
                    <span class="sytue-meta mt-0.5 block truncate tabular-nums">
                      {node.isDirectory ? t('team.folder.label') : formatSize(locale, node.size)}
                    </span>
                  </span>
                  {!node.isDirectory ? (
                    <button
                      type="button"
                      onClick={() => onDownload(node)}
                      title={t('team.item.download', { name: node.name })}
                      aria-label={t('team.item.download', { name: node.name })}
                      class="text-text-muted hover:bg-bg hover:text-text sytue-focus-ring rounded-control p-1.5 opacity-0 transition-[opacity,color] duration-150 group-hover:opacity-100 focus-visible:opacity-100 pointer-coarse:opacity-100"
                    >
                      <Glyph name="download" size={18} />
                    </button>
                  ) : null}
                  {writable ? (
                    <button
                      type="button"
                      onClick={() => setPendingDelete(node)}
                      disabled={busy}
                      title={t('team.item.delete', { name: node.name })}
                      aria-label={t('team.item.delete', { name: node.name })}
                      class="text-text-muted hover:bg-danger-bg hover:text-danger-text sytue-focus-ring rounded-control p-1.5 opacity-0 transition-[opacity,color] duration-150 group-hover:opacity-100 focus-visible:opacity-100 disabled:pointer-events-none disabled:opacity-40 pointer-coarse:enabled:opacity-100"
                    >
                      <Glyph name="trash" size={18} />
                    </button>
                  ) : null}
                </li>
              ))}
            </ul>
            {nextCursor !== null ? (
              <div class="flex justify-center py-4">
                <Button
                  variant="secondary"
                  size="sm"
                  loading={loadingMore}
                  onClick={() => void loadMore()}
                >
                  {t('team.loadMore')}
                </Button>
              </div>
            ) : null}
          </>
        )}
      </div>

      {/* Nueva carpeta */}
      <Modal
        open={newFolderOpen}
        onClose={() => setNewFolderOpen(false)}
        title={t('team.newFolder.title')}
        footer={
          <>
            <Button variant="ghost" onClick={() => setNewFolderOpen(false)}>
              {t('team.create.cancel')}
            </Button>
            <Button
              onClick={() => void onCreateFolder()}
              loading={busy}
              disabled={!folderName.trim()}
            >
              {t('team.create.submit')}
            </Button>
          </>
        }
      >
        <FieldGroup label={t('team.newFolder.label')} htmlFor="team-folder-name">
          <Input
            id="team-folder-name"
            value={folderName}
            placeholder={t('team.newFolder.placeholder')}
            onInput={(e) => setFolderName(e.currentTarget.value)}
            onKeyDown={(e) => {
              if (e.key === 'Enter') void onCreateFolder();
            }}
          />
        </FieldGroup>
      </Modal>

      {/* Renombrar unidad */}
      <Modal
        open={renameOpen}
        onClose={() => setRenameOpen(false)}
        title={t('team.rename.title')}
        footer={
          <>
            <Button variant="ghost" onClick={() => setRenameOpen(false)}>
              {t('team.create.cancel')}
            </Button>
            <Button onClick={() => void onRename()} loading={busy} disabled={!renameValue.trim()}>
              {t('team.rename')}
            </Button>
          </>
        }
      >
        <FieldGroup label={t('team.rename.label')} htmlFor="team-rename">
          <Input
            id="team-rename"
            value={renameValue}
            onInput={(e) => setRenameValue(e.currentTarget.value)}
            onKeyDown={(e) => {
              if (e.key === 'Enter') void onRename();
            }}
          />
        </FieldGroup>
      </Modal>

      {/* Confirmar borrado */}
      <Modal
        open={pendingDelete !== null}
        onClose={() => setPendingDelete(null)}
        title={t(pendingDelete?.isDirectory ? 'team.delete.folderTitle' : 'team.delete.fileTitle')}
        size="sm"
        footer={
          <>
            <Button variant="ghost" onClick={() => setPendingDelete(null)} disabled={busy}>
              {t('team.create.cancel')}
            </Button>
            <Button
              variant="danger"
              loading={busy}
              onClick={() => pendingDelete && void onDelete(pendingDelete)}
            >
              {t('team.delete.cta')}
            </Button>
          </>
        }
      >
        <p class="text-text-muted text-base">
          {t('team.delete.body', { name: pendingDelete?.name ?? '' })}
        </p>
      </Modal>

      {/* Administración de miembros (solo gestor) */}
      {membersOpen ? (
        <MembersDialog driveId={drive.id} driveName={name} onClose={() => setMembersOpen(false)} />
      ) : null}

      {/*
        La MISMA vista previa que en «Mi unidad» y en «Compartido conmigo»: el
        PDF lo dibuja Sytue PDF y la ofimática la pinta el visor de Docs. Se le
        pasan `drive`+`file` porque el contenido de una unidad de equipo está
        aislado por unidad (RLS) y no se alcanza por la ruta de la personal.
      */}
      {preview ? (
        <PreviewDialog
          item={comoRecurso(preview)}
          drive={drive.id}
          file={preview.id}
          inlineSrc={`${downloadHref(drive.id, preview.id)}&disposition=inline`}
          onClose={() => setPreview(null)}
          onDownload={() => onDownload(preview)}
        />
      ) : null}
    </div>
  );
}

// --- diálogo de miembros -------------------------------------------------

const ROLES: Role[] = ['viewer', 'editor', 'manager'];

function MembersDialog({
  driveId,
  driveName,
  onClose,
}: {
  driveId: string;
  driveName: string;
  onClose: () => void;
}) {
  const toast = useToast();
  const { t } = useTeamT();
  const [members, setMembers] = useState<Member[] | null>(null);
  const [error, setError] = useState(false);
  const [subject, setSubject] = useState('');
  const [role, setRole] = useState<Role>('viewer');
  const [busy, setBusy] = useState(false);

  const load = useCallback(async () => {
    setMembers(null);
    setError(false);
    try {
      setMembers(await api.members(driveId));
    } catch {
      setError(true);
    }
  }, [driveId]);

  useEffect(() => {
    void load();
  }, [load]);

  /** El motivo concreto que devolvió el servicio manda sobre el genérico. */
  const failTitle = (e: unknown, fallback: DriveMessageKey) =>
    t(
      e instanceof BffError && e.status === 409
        ? 'team.member.lastManager'
        : e instanceof BffError && e.status === 403
          ? 'team.member.denied'
          : fallback,
    );

  const onAdd = async () => {
    const s = subject.trim();
    if (!s || busy) return;
    setBusy(true);
    try {
      await api.putMember(driveId, s, role);
      toast.notify({
        tone: 'success',
        title: t('team.member.added', { subject: s, role: t(ROLE_LABEL[role]) }),
      });
      setSubject('');
      await load();
    } catch (e) {
      toast.notify({ tone: 'danger', title: failTitle(e, 'team.member.addFailed') });
    } finally {
      setBusy(false);
    }
  };

  const onChangeRole = async (m: Member, next: Role) => {
    if (next === m.role || busy) return;
    setBusy(true);
    try {
      await api.putMember(driveId, m.subject, next);
      await load();
    } catch (e) {
      toast.notify({ tone: 'danger', title: failTitle(e, 'team.member.roleFailed') });
    } finally {
      setBusy(false);
    }
  };

  const onRemove = async (m: Member) => {
    setBusy(true);
    try {
      await api.removeMember(driveId, m.subject);
      await load();
    } catch (e) {
      toast.notify({ tone: 'danger', title: failTitle(e, 'team.member.removeFailed') });
    } finally {
      setBusy(false);
    }
  };

  return (
    <Modal open onClose={onClose} title={t('team.member.title', { name: driveName })}>
      <div class="space-y-4">
        {/* Añadir miembro */}
        <div class="border-border bg-bg-subtle flex flex-wrap items-end gap-2 rounded-lg border p-3">
          <FieldGroup
            label={t('team.member.subjectLabel')}
            htmlFor="member-subject"
            class="min-w-40 flex-1"
          >
            <Input
              id="member-subject"
              value={subject}
              placeholder={t('team.member.subjectPlaceholder')}
              onInput={(e) => setSubject(e.currentTarget.value)}
              onKeyDown={(e) => {
                if (e.key === 'Enter') void onAdd();
              }}
            />
          </FieldGroup>
          <FieldGroup label={t('team.member.roleLabel')} htmlFor="member-role" class="w-40">
            <Select
              id="member-role"
              value={role}
              aria-describedby="member-role-hint"
              onChange={(e) => setRole(e.currentTarget.value as Role)}
            >
              {ROLES.map((r) => (
                <option key={r} value={r}>
                  {t(ROLE_LABEL[r])}
                </option>
              ))}
            </Select>
          </FieldGroup>
          <Button onClick={() => void onAdd()} loading={busy} disabled={!subject.trim()}>
            {t('team.member.add')}
          </Button>
          <FieldHint id="member-role-hint" class="basis-full">
            {t(ROLE_WILL[role])}
          </FieldHint>
        </div>

        {/* Lista de miembros */}
        {members === null && !error ? (
          <div class="space-y-2 py-2" aria-busy="true">
            {Array.from({ length: 3 }).map((_, i) => (
              <Skeleton key={i} height="2.25rem" />
            ))}
          </div>
        ) : error ? (
          <div class="text-text-muted flex items-center justify-between gap-3 py-4 text-sm">
            <span>{t('team.member.loadFailed')}</span>
            <Button variant="secondary" size="sm" onClick={() => void load()}>
              {t('team.retry')}
            </Button>
          </div>
        ) : (
          <ul class="divide-border border-border divide-y overflow-hidden rounded-lg border">
            {members!.map((m) => (
              <li key={m.subject} class="flex items-center gap-3 px-3 py-2">
                <span
                  class="text-text min-w-0 flex-1 truncate text-sm font-medium"
                  title={m.subject}
                >
                  {m.subject}
                </span>
                <Select
                  aria-label={t('team.member.roleAria', { subject: m.subject })}
                  value={m.role}
                  onChange={(e) => void onChangeRole(m, e.currentTarget.value as Role)}
                  disabled={busy}
                >
                  {ROLES.map((r) => (
                    <option key={r} value={r}>
                      {t(ROLE_LABEL[r])}
                    </option>
                  ))}
                </Select>
                <button
                  type="button"
                  onClick={() => void onRemove(m)}
                  disabled={busy}
                  title={t('team.member.remove', { subject: m.subject })}
                  aria-label={t('team.member.remove', { subject: m.subject })}
                  class="text-text-muted hover:bg-danger-bg hover:text-danger-text sytue-focus-ring rounded-control grid shrink-0 place-items-center p-1.5 transition-colors duration-150 disabled:pointer-events-none disabled:opacity-40 pointer-coarse:min-h-[max(44px,2.75rem)] pointer-coarse:min-w-[max(44px,2.75rem)]"
                >
                  <Glyph name="trash" size={16} />
                </button>
              </li>
            ))}
          </ul>
        )}
        {members !== null && members.length > 0 ? (
          <p class="sytue-meta">{t('team.member.note')}</p>
        ) : null}
      </div>
    </Modal>
  );
}

// --- esqueleto de carga --------------------------------------------------

function TeamDriveListSkeleton() {
  const { t } = useTeamT();
  return (
    <ul class="px-3" aria-busy="true">
      <span class="sr-only" role="status">
        {t('team.loading')}
      </span>
      {Array.from({ length: 6 }).map((_, i) => (
        <li key={i} class="flex items-center gap-3 px-3 py-2.5">
          <Skeleton width="36px" height="36px" />
          <Skeleton width={`${40 + ((i * 13) % 40)}%`} height="0.85rem" />
          <span class="ml-auto">
            <Skeleton width="60px" height="1.1rem" />
          </span>
        </li>
      ))}
    </ul>
  );
}
