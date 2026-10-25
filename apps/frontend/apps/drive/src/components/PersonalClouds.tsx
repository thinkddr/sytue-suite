/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Nubes personales: el Google Drive, OneDrive o Dropbox de la propia persona como
 * una ubicación más de Sytue Drive. Se conecta con solo el consentimiento del
 * proveedor; Admin únicamente puede apagar un proveedor para toda la organización.
 * Lo sirve el servicio de integraciones (/api/personal-storage/…), con la credencial
 * de quien la conectó. Se añaden desde «Añadir almacenamiento», la misma puerta que
 * el almacén de la organización.
 */
import { useCallback, useEffect, useRef, useState } from 'preact/hooks';
import {
  Button,
  EmptyState,
  InlineHelp,
  SidebarItem,
  SidebarSection,
  formatBytes,
  useToast,
  withBase,
} from '@sytue/ui';
import { useI18n } from '@sytue/i18n/preact';
import { useT, type T } from '../i18n';
import { Glyph } from './glyphs';
import { PARTNER, StorageDialog, type ExternalStorageState } from './ExternalStorage';

export type CloudProvider = 'google-drive' | 'onedrive' | 'dropbox';

export const CLOUD_NAMES: Record<CloudProvider, string> = {
  'google-drive': 'Google Drive',
  onedrive: 'OneDrive',
  dropbox: 'Dropbox',
};

export interface Cloud {
  instance_id: string;
  provider: CloudProvider;
  connected: boolean;
}

interface Accounts {
  items: Cloud[];
  providers: { provider: CloudProvider; available: boolean }[];
}

interface Item {
  id: string;
  name: string;
  folder: boolean;
  size?: number;
}

const BASE = '/api/personal-storage';
const storageUrl = (path: string) => withBase(import.meta.env.BASE_URL ?? '/drive', path);

async function connect(
  provider: CloudProvider,
  t: T,
  notify: ReturnType<typeof useToast>['notify'],
) {
  const res = await fetch(`${BASE}/connect`, {
    method: 'POST',
    headers: { 'content-type': 'application/json' },
    body: JSON.stringify({ provider, return_to: 'drive' }),
  }).catch(() => null);
  const body = res?.ok ? ((await res.json()) as { authorize_url?: string }) : null;
  if (body?.authorize_url?.startsWith('https://')) window.location.assign(body.authorize_url);
  else notify({ tone: 'danger', title: t('cloud.failed') });
}

/**
 * «Tus nubes» en la barra lateral: las nubes personales conectadas, el almacén de la
 * organización si lo hay, y UNA sola entrada «Añadir almacenamiento» que abre el
 * selector (ExternalStorage.tsx). Antes eran un «Conectar X» por proveedor aquí y
 * otro asistente distinto en el pie; dos puertas para lo mismo.
 */
export function CloudsSidebar({
  t,
  active,
  onSelect,
  onStorageChange,
}: {
  t: T;
  active: string | null;
  onSelect: (cloud: Cloud) => void;
  onStorageChange?: () => void;
}) {
  const toast = useToast();
  const [accounts, setAccounts] = useState<Accounts | null>(null);
  // `undefined` mientras carga; `null` si quien mira no administra (403) o falló.
  const [estado, setEstado] = useState<ExternalStorageState | null | undefined>(undefined);
  const [dialog, setDialog] = useState<'add' | 'manage' | null>(null);
  const deepLinkRef = useRef(false);
  const notify = toast.notify;
  const loadOrg = useCallback(() => {
    void fetch(storageUrl('/api/storage-external'), { credentials: 'include' })
      .then((r) => (r.ok ? (r.json() as Promise<ExternalStorageState>) : null))
      .catch(() => null)
      .then(setEstado);
  }, []);
  useEffect(loadOrg, [loadOrg]);
  useEffect(() => {
    // La cuota se relee aunque el inventario falle tras conectar o desconectar.
    onStorageChange?.();
    void fetch(`${BASE}/accounts`)
      .then((r) => (r.ok ? (r.json() as Promise<Accounts>) : null))
      .then((a) => {
        setAccounts(a);
        // Al volver del consentimiento (`?connect-cloud=1&instance=…`) se abre esa nube.
        const q = new URLSearchParams(window.location.search);
        const nueva = q.get('connect-cloud')
          ? a?.items.find((c) => c.instance_id === q.get('instance'))
          : undefined;
        if (nueva?.connected) onSelect(nueva);
        else if (q.get('connect-cloud') && q.get('oauth_error'))
          notify({ tone: 'danger', title: t('cloud.failed') });
        if (q.get('connect-cloud')) {
          ['connect-cloud', 'instance', 'oauth_error'].forEach((k) => q.delete(k));
          window.history.replaceState(
            null,
            '',
            `${window.location.pathname}${q.size ? `?${q}` : ''}`,
          );
        }
      })
      .catch(() => setAccounts(null));
    // Se relee al cambiar de ubicación: así una nube desconectada sale de la lista.
  }, [active, onSelect, t, notify, onStorageChange]);
  // `?connect-storage=1` (portal, Hub nativo, vuelta del consentimiento de
  // organización) abre el selector cuando ya se sabe qué se puede ofrecer.
  useEffect(() => {
    if (deepLinkRef.current || estado === undefined) return;
    if (new URLSearchParams(window.location.search).get('connect-storage') !== '1') return;
    deepLinkRef.current = true;
    setDialog('add');
  }, [estado]);
  const close = () => {
    setDialog(null);
    const url = new URL(window.location.href);
    if (!url.searchParams.has('connect-storage')) return;
    for (const key of ['connect-storage', 'instance', 'oauth_error']) url.searchParams.delete(key);
    window.history.replaceState(window.history.state, '', url);
  };
  const conectadas = accounts?.items.filter((c) => c.connected) ?? [];
  const personal: Partial<Record<CloudProvider, boolean>> = {};
  for (const p of accounts?.providers ?? []) personal[p.provider] = p.available;
  const org = estado ?? null;
  const canAdd = Boolean(org) || Object.values(personal).some(Boolean);
  if (conectadas.length === 0 && !org?.connected && !canAdd) return null;
  return (
    <SidebarSection title={t('nav.clouds')}>
      {conectadas.map((c) => (
        <SidebarItem
          key={c.instance_id}
          label={CLOUD_NAMES[c.provider]}
          icon={<Glyph name="folder" size={18} />}
          active={active === c.instance_id}
          onClick={() => onSelect(c)}
        />
      ))}
      {org?.connected ? (
        <SidebarItem
          // Su nombre, o el bucket, y solo si no hay ninguno la marca: «S3» a secas no
          // dice cuál es. El mismo orden que `RotuloExterno` en la app nativa.
          label={org.label || org.bucket || PARTNER[org.kind as keyof typeof PARTNER] || ''}
          icon={<Glyph name="building" size={18} />}
          onClick={() => setDialog('manage')}
        />
      ) : null}
      {canAdd ? (
        <SidebarItem
          label={t('storage.add')}
          icon={<Glyph name="link" size={18} />}
          onClick={() => setDialog('add')}
        />
      ) : null}
      {dialog ? (
        <StorageDialog
          t={t}
          estado={org}
          personal={personal}
          manage={dialog === 'manage'}
          onPersonal={(provider) => {
            const abierta = conectadas.find((c) => c.provider === provider);
            if (!abierta) {
              void connect(provider, t, notify);
              return;
            }
            close();
            onSelect(abierta);
          }}
          onClose={close}
          onChange={() => {
            loadOrg();
            onStorageChange?.();
          }}
        />
      ) : null}
    </SidebarSection>
  );
}

/** Navegar, descargar y subir en una nube personal. */
export function CloudView({ cloud, onDisconnected }: { cloud: Cloud; onDisconnected: () => void }) {
  const { locale } = useI18n();
  const t = useT(locale);
  const toast = useToast();
  const name = CLOUD_NAMES[cloud.provider];
  const base = `${BASE}/${cloud.instance_id}`;
  // La ruta de carpetas abiertas: [] es la raíz de la nube.
  const [path, setPath] = useState<Item[]>([]);
  const [items, setItems] = useState<Item[] | null>(null);
  const [cursor, setCursor] = useState('');
  const [failed, setFailed] = useState(false);
  const [busy, setBusy] = useState(false);
  const input = useRef<HTMLInputElement>(null);
  const folder = path.at(-1)?.id ?? '';

  const load = useCallback(
    async (more: string) => {
      setFailed(false);
      const q = new URLSearchParams({
        ...(folder ? { folder } : {}),
        ...(more ? { cursor: more } : {}),
      });
      const res = await fetch(`${base}/items?${q}`).catch(() => null);
      if (!res?.ok) {
        setFailed(true);
        if (!more) setItems([]);
        return;
      }
      const page = (await res.json()) as { items: Item[]; next_cursor?: string };
      setItems((prev) => (more && prev ? [...prev, ...page.items] : page.items));
      setCursor(page.next_cursor ?? '');
    },
    [base, folder],
  );
  useEffect(() => {
    setItems(null);
    void load('');
  }, [load]);

  async function upload(files: FileList | null) {
    if (!files?.length) return;
    setBusy(true);
    for (const file of Array.from(files)) {
      if (file.size > 100 * 1024 * 1024) {
        toast.notify({ tone: 'danger', title: t('cloud.tooLarge'), description: file.name });
        continue;
      }
      const q = new URLSearchParams({ name: file.name, ...(folder ? { folder } : {}) });
      const res = await fetch(`${base}/content?${q}`, { method: 'POST', body: file }).catch(
        () => null,
      );
      if (!res?.ok)
        toast.notify({ tone: 'danger', title: t('cloud.failed'), description: file.name });
    }
    setBusy(false);
    void load('');
  }

  async function disconnect() {
    const res = await fetch(base, { method: 'DELETE' }).catch(() => null);
    if (res?.ok) onDisconnected();
    else toast.notify({ tone: 'danger', title: t('cloud.failed') });
  }

  return (
    <div class="flex h-full flex-col">
      <header class="flex flex-wrap items-center gap-2 px-6 pt-5 pb-3">
        <nav class="flex min-w-0 flex-1 flex-wrap items-center gap-1 text-sm" aria-label={name}>
          <button
            type="button"
            class="text-text sytue-focus-ring rounded font-semibold hover:underline"
            onClick={() => setPath([])}
          >
            {name}
          </button>
          {path.map((p, i) => (
            <span key={p.id} class="flex items-center gap-1">
              <span class="text-text-muted">/</span>
              <button
                type="button"
                class="text-text sytue-focus-ring max-w-48 truncate rounded hover:underline"
                onClick={() => setPath(path.slice(0, i + 1))}
              >
                {p.name}
              </button>
            </span>
          ))}
        </nav>
        <input
          ref={input}
          type="file"
          multiple
          hidden
          onChange={(e) => void upload((e.target as HTMLInputElement).files)}
        />
        <Button size="sm" disabled={busy} onClick={() => input.current?.click()}>
          {t('files.upload')}
        </Button>
        <Button size="sm" variant="ghost" onClick={() => void disconnect()}>
          {t('cloud.disconnect')}
        </Button>
      </header>
      {cloud.provider === 'google-drive' ? (
        <div class="px-6 pb-2">
          <InlineHelp>{t('cloud.googleNote')}</InlineHelp>
        </div>
      ) : null}
      {failed && !items?.length ? (
        <div class="grid flex-1 place-items-center p-6">
          <EmptyState
            title={t('cloud.failed')}
            action={
              <Button size="sm" variant="secondary" onClick={() => void load('')}>
                {t('files.retry')}
              </Button>
            }
          />
        </div>
      ) : items && items.length === 0 ? (
        <div class="grid flex-1 place-items-center p-6">
          <EmptyState title={t('team.folder.emptyTitle')} />
        </div>
      ) : (
        <ul class="px-3 pb-2">
          {(items ?? []).map((it) => (
            <li
              key={it.id}
              class="border-border/60 hover:bg-bg-subtle flex items-center gap-3 border-b px-3 py-2"
            >
              <span class={it.folder ? 'text-primary-text' : 'text-text-muted'}>
                <Glyph name={it.folder ? 'folder' : 'file'} size={22} />
              </span>
              <span class="min-w-0 flex-1">
                {it.folder ? (
                  <button
                    type="button"
                    class="text-text sytue-focus-ring block max-w-full truncate rounded text-left font-medium hover:underline"
                    onClick={() => setPath([...path, it])}
                  >
                    {it.name}
                  </button>
                ) : (
                  <a
                    class="text-text sytue-focus-ring block max-w-full truncate rounded font-medium hover:underline"
                    href={`${base}/content?${new URLSearchParams({ item: it.id })}`}
                    download={it.name}
                  >
                    {it.name}
                  </a>
                )}
                {!it.folder && it.size ? (
                  <span class="sytue-meta mt-0.5 block tabular-nums">
                    {formatBytes(locale, it.size)}
                  </span>
                ) : null}
              </span>
            </li>
          ))}
          {cursor ? (
            <li class="p-3">
              <Button size="sm" variant="secondary" onClick={() => void load(cursor)}>
                {t('files.loadMore')}
              </Button>
            </li>
          ) : null}
        </ul>
      )}
    </div>
  );
}
