/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * ShareModal — COMPARTIR un documento de Sytue Docs.
 *
 * El servicio Go tenía el modelo entero desde el principio: cuatro roles, permisos por
 * persona, por grupo y para toda la organización, y enlaces públicos con caducidad. No
 * existía ninguna forma de usarlo — ni endpoint en el BFF ni botón en la interfaz—, así
 * que un documento colaborativo solo lo veía quien lo había creado.
 *
 * Dos maneras de dar acceso, que es como funciona de verdad: nombrar a alguien, o
 * generar un enlace. El enlace se enseña UNA vez —el servicio devuelve su token solo al
 * crearlo— y por eso se copia al portapapeles en el mismo momento.
 */
import type { JSX } from 'preact';
import { useEffect, useState } from 'preact/hooks';
import {
  Modal,
  Button,
  Input,
  Select,
  Spinner,
  Badge,
  ChoiceCard,
  FieldHint,
  InlineHelp,
  EmptyState,
  useToast,
  withBase,
} from '@sytue/ui';
import type { DocsT } from '../i18n';

/** Los cuatro roles del servicio, de menos a más poder. */
type Role = 'viewer' | 'commenter' | 'editor' | 'owner';

/** A quién se le da acceso. `public` no se elige aquí: eso es un enlace. */
type PrincipalKind = 'user' | 'group' | 'tenant';

interface Permission {
  principal_kind: PrincipalKind | 'public';
  principal_id: string;
  /** Correo y nombre de la persona, que pone el BFF (el servicio solo guarda el `sub`). */
  principal_email?: string;
  principal_name?: string;
  role: Role;
}

interface ShareLink {
  id: string;
  role: Role;
  /** Token EN CLARO: solo viene al crearlo. En la lista llega vacío. */
  token?: string;
  expires_at?: string;
  revoked_at?: string;
  /** Cuándo el techo de la organización acortó su caducidad. */
  policy_adjusted_at?: string;
  /** Pide contraseña. La contraseña y su hash no salen nunca del servicio. */
  has_password?: boolean;
}

/** El techo de caducidad de los enlaces de la organización (0 = sin techo). */
interface LinkPolicy {
  max_days: number;
  can_manage: boolean;
}

/** Una fecha local `AAAA-MM-DD` a `n` días de hoy, para los límites del selector. */
function diaLocal(n: number): string {
  const d = new Date();
  d.setDate(d.getDate() + n);
  const dos = (v: number): string => String(v).padStart(2, '0');
  return `${d.getFullYear()}-${dos(d.getMonth() + 1)}-${dos(d.getDate())}`;
}

export interface ShareModalProps {
  open: boolean;
  onClose: () => void;
  documentId: string;
  documentTitle: string;
  t: DocsT;
}

/**
 * Dirección pública de un enlace, con el token que solo se ve al crearlo.
 *
 * Con el PREFIJO de Docs (`/docs/s/…`) —la raíz del host no enruta nada— y SIN
 * org a propósito (ADR 0056): es un enlace PARA ANÓNIMOS, que viaja copiado a
 * quien sea y no tiene por qué anunciar la organización de quien lo creó. La
 * vista pública pasa igual sin ella.
 */
function shareUrl(token: string): string {
  return `${window.location.origin}${withBase(
    import.meta.env.BASE_URL,
    `/s/${encodeURIComponent(token)}`,
  )}`;
}

export function ShareModal({
  open,
  onClose,
  documentId,
  documentTitle,
  t,
}: ShareModalProps): JSX.Element | null {
  const toast = useToast();
  const [permissions, setPermissions] = useState<Permission[]>([]);
  const [shares, setShares] = useState<ShareLink[]>([]);
  const [loading, setLoading] = useState(false);
  const [busy, setBusy] = useState(false);
  const [denied, setDenied] = useState(false);

  // Alta de una persona.
  const [who, setWho] = useState('');
  const [role, setRole] = useState<Role>('viewer');
  // Enlace recién creado: su token no se vuelve a poder consultar.
  const [fresh, setFresh] = useState<ShareLink | null>(null);
  // Caducidad del enlace nuevo (`AAAA-MM-DD`; vacío = el máximo de la organización) y
  // el techo que la organización impone. El servicio es quien lo hace cumplir; aquí se
  // enseña y se limita el selector para no dejar pedir lo que va a rechazar.
  const [expira, setExpira] = useState('');
  const [politica, setPolitica] = useState<LinkPolicy | null>(null);
  // Contraseña opcional del enlace nuevo, y la que se está poniendo a uno que ya existe.
  const [clave, setClave] = useState('');
  const [editando, setEditando] = useState<string | null>(null);
  const [claveNueva, setClaveNueva] = useState('');
  const [techo, setTecho] = useState('');

  // ¿Lo ve toda la organización? El servicio lo soporta con el principal `tenant` desde
  // el principio y no había forma de pedirlo: quien quería «que lo vea toda la empresa»
  // tenía que ir nombrando a la gente de una en una.
  const wholeOrg = permissions.find((p) => p.principal_kind === 'tenant') ?? null;

  async function alcance(todos: boolean): Promise<void> {
    if (busy) return;
    setBusy(true);
    try {
      const res = todos
        ? await fetch(`/api/sharing?id=${encodeURIComponent(documentId)}`, {
            method: 'PUT',
            credentials: 'include',
            headers: { 'content-type': 'application/json' },
            body: JSON.stringify({ principal_kind: 'tenant', principal_id: '', role: 'viewer' }),
          })
        : await fetch(
            `/api/sharing?${new URLSearchParams({ id: documentId, principalKind: 'tenant', principalId: '' }).toString()}`,
            { method: 'DELETE', credentials: 'include' },
          );
      if (!res.ok) throw new Error(String(res.status));
      await recargar();
    } catch {
      toast.notify({ tone: 'danger', title: t('share.grantFailed') });
    } finally {
      setBusy(false);
    }
  }

  useEffect(() => {
    if (!open) return;
    setFresh(null);
    setDenied(false);
    setLoading(true);
    void fetch(`/api/sharing?id=${encodeURIComponent(documentId)}`, {
      credentials: 'include',
      headers: { accept: 'application/json' },
    })
      .then(async (r) => {
        // 403 = no es tu documento. Se dice, en vez de enseñar una lista vacía que
        // parecería «no lo has compartido con nadie».
        if (r.status === 403) {
          setDenied(true);
          return { permissions: [], shares: [] };
        }
        if (!r.ok) throw new Error(String(r.status));
        return (await r.json()) as { permissions: Permission[]; shares: ShareLink[] };
      })
      .then((d) => {
        setPermissions(d.permissions ?? []);
        setShares(d.shares ?? []);
      })
      .catch(() => toast.notify({ tone: 'danger', title: t('share.loadFailed') }))
      .finally(() => setLoading(false));
    setExpira('');
    void fetch('/api/link-policy', {
      credentials: 'include',
      headers: { accept: 'application/json' },
    })
      .then(async (r) => (r.ok ? ((await r.json()) as LinkPolicy) : null))
      .then((p) => {
        setPolitica(p);
        setTecho(p ? String(p.max_days) : '');
      })
      .catch(() => setPolitica(null));
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, documentId]);

  async function recargar(): Promise<void> {
    const r = await fetch(`/api/sharing?id=${encodeURIComponent(documentId)}`, {
      credentials: 'include',
      headers: { accept: 'application/json' },
    });
    if (!r.ok) return;
    const d = (await r.json()) as { permissions: Permission[]; shares: ShareLink[] };
    setPermissions(d.permissions ?? []);
    setShares(d.shares ?? []);
  }

  async function conceder(): Promise<void> {
    const id = who.trim();
    if (!id || busy) return;
    setBusy(true);
    try {
      const res = await fetch(`/api/sharing?id=${encodeURIComponent(documentId)}`, {
        method: 'PUT',
        credentials: 'include',
        headers: { 'content-type': 'application/json' },
        body: JSON.stringify({ principal_kind: 'user', principal_id: id, role }),
      });
      if (!res.ok) throw new Error(String(res.status));
      setWho('');
      await recargar();
    } catch (e) {
      // 404: la dirección no es de nadie de la organización (lo resuelve el BFF).
      const nadie = e instanceof Error && e.message === '404';
      toast.notify({
        tone: 'danger',
        title: t(nadie ? 'share.unknownPerson' : 'share.grantFailed'),
      });
    } finally {
      setBusy(false);
    }
  }

  async function revocar(p: Permission): Promise<void> {
    if (busy) return;
    setBusy(true);
    try {
      const qs = new URLSearchParams({
        id: documentId,
        principalKind: p.principal_kind,
        principalId: p.principal_id,
      });
      const res = await fetch(`/api/sharing?${qs.toString()}`, {
        method: 'DELETE',
        credentials: 'include',
      });
      if (!res.ok) throw new Error(String(res.status));
      await recargar();
    } catch {
      toast.notify({ tone: 'danger', title: t('share.revokeFailed') });
    } finally {
      setBusy(false);
    }
  }

  async function crearEnlace(): Promise<void> {
    if (busy) return;
    setBusy(true);
    try {
      const res = await fetch(`/api/sharing?id=${encodeURIComponent(documentId)}`, {
        method: 'POST',
        credentials: 'include',
        headers: { 'content-type': 'application/json' },
        // Hasta el final del día elegido, en la hora de quien lo crea.
        body: JSON.stringify({
          role,
          ...(expira ? { expires_at: new Date(`${expira}T23:59:59`).toISOString() } : {}),
          ...(clave ? { password: clave } : {}),
        }),
      });
      if (res.status === 400) {
        const { error } = (await res.json().catch(() => ({}))) as { error?: { code?: string } };
        if (error?.code === 'share_password_invalid') {
          toast.notify({ tone: 'danger', title: t('share.password.failed') });
          return;
        }
        if (error?.code === 'share_expiry_too_far' || error?.code === 'share_expiry_past') {
          toast.notify({
            tone: 'danger',
            title: t('share.expiry.rejected', { days: politica?.max_days ?? 0 }),
          });
          return;
        }
      }
      if (!res.ok) throw new Error(String(res.status));
      const { share } = (await res.json()) as { share: ShareLink };
      setFresh(share);
      setClave('');
      // Al portapapeles en el acto: el token no se vuelve a poder consultar, así que
      // pedirle a la persona que lo copie «luego» es perderlo.
      if (share.token) {
        await navigator.clipboard?.writeText(shareUrl(share.token)).catch(() => undefined);
        toast.notify({ tone: 'success', title: t('share.linkCopied') });
      }
      await recargar();
    } catch {
      toast.notify({ tone: 'danger', title: t('share.linkFailed') });
    } finally {
      setBusy(false);
    }
  }

  /** Pone, cambia o quita (`''`) la contraseña de un enlace que ya existe. */
  async function guardarClave(linkId: string, password: string): Promise<void> {
    if (busy) return;
    setBusy(true);
    try {
      const qs = new URLSearchParams({ id: documentId, linkId });
      const res = await fetch(`/api/sharing?${qs.toString()}`, {
        method: 'PATCH',
        credentials: 'include',
        headers: { 'content-type': 'application/json' },
        body: JSON.stringify({ password }),
      });
      if (!res.ok) throw new Error(String(res.status));
      setEditando(null);
      setClaveNueva('');
      toast.notify({
        tone: 'success',
        title: t(password ? 'share.password.saved' : 'share.password.removed'),
      });
      await recargar();
    } catch {
      toast.notify({ tone: 'danger', title: t('share.password.failed') });
    } finally {
      setBusy(false);
    }
  }

  async function guardarTecho(): Promise<void> {
    const dias = Number(techo);
    if (busy || !Number.isInteger(dias) || dias < 0 || dias > 3650) return;
    setBusy(true);
    try {
      const res = await fetch('/api/link-policy', {
        method: 'PUT',
        credentials: 'include',
        headers: { 'content-type': 'application/json' },
        body: JSON.stringify({ max_days: dias }),
      });
      if (!res.ok) throw new Error(String(res.status));
      const p = (await res.json()) as LinkPolicy & { adjusted?: number };
      setPolitica({ max_days: p.max_days, can_manage: true });
      setExpira('');
      toast.notify({
        tone: 'success',
        title: t('share.policy.saved', { count: p.adjusted ?? 0 }),
      });
      await recargar();
    } catch {
      toast.notify({ tone: 'danger', title: t('share.policy.failed') });
    } finally {
      setBusy(false);
    }
  }

  async function revocarEnlace(linkId: string): Promise<void> {
    if (busy) return;
    setBusy(true);
    try {
      const qs = new URLSearchParams({ id: documentId, linkId });
      const res = await fetch(`/api/sharing?${qs.toString()}`, {
        method: 'DELETE',
        credentials: 'include',
      });
      if (!res.ok) throw new Error(String(res.status));
      if (fresh?.id === linkId) setFresh(null);
      await recargar();
    } catch {
      toast.notify({ tone: 'danger', title: t('share.revokeFailed') });
    } finally {
      setBusy(false);
    }
  }

  if (!open) return null;

  return (
    <Modal
      open={open}
      onClose={onClose}
      title={t('share.title', { name: documentTitle })}
      size="md"
    >
      {loading ? (
        <div class="flex items-center justify-center py-8" aria-busy="true">
          <Spinner size={22} />
        </div>
      ) : denied ? (
        <p class="text-text-muted text-sm">{t('share.onlyOwner')}</p>
      ) : (
        <div class="flex flex-col gap-5">
          {/* Lo primero es la pregunta grande —¿quién lo ve?— y no un formulario: es la
              decisión que la persona viene a tomar, y las dos opciones dicen lo que
              implican en vez de dejarlo a la imaginación. */}
          <div class="flex flex-col gap-2">
            <h3 class="text-text text-sm font-semibold">{t('share.scope')}</h3>
            <div class="grid gap-2 sm:grid-cols-2">
              <ChoiceCard
                name="share-scope"
                value="named"
                title={t('share.scope.named')}
                description={t('share.scope.named.desc')}
                icon="users"
                selected={wholeOrg === null}
                disabled={busy}
                onSelect={() => void alcance(false)}
              />
              <ChoiceCard
                name="share-scope"
                value="org"
                title={t('share.scope.org')}
                description={t('share.scope.org.desc')}
                icon="building"
                selected={wholeOrg !== null}
                disabled={busy}
                onSelect={() => void alcance(true)}
              />
            </div>
          </div>

          {/* Dar acceso a alguien por su nombre. */}
          <div class="flex flex-col gap-2">
            <label class="sytue-label font-medium" for="share-who">
              {t('share.addPerson')}
            </label>
            <div class="flex flex-wrap items-end gap-2">
              <Input
                id="share-who"
                type="email"
                class="min-w-[14rem] flex-1"
                value={who}
                placeholder={t('share.personPlaceholder')}
                onInput={(e) => setWho(e.currentTarget.value)}
              />
              <Select
                value={role}
                aria-label={t('share.role')}
                onChange={(e) => setRole(e.currentTarget.value as Role)}
              >
                <option value="viewer">{t('share.role.viewer')}</option>
                <option value="commenter">{t('share.role.commenter')}</option>
                <option value="editor">{t('share.role.editor')}</option>
              </Select>
              <Button
                variant="primary"
                size="sm"
                disabled={busy || !who.trim()}
                onClick={() => void conceder()}
              >
                {t('share.grant')}
              </Button>
            </div>
            {/* Qué escribir y qué implica lo elegido, sin salir a buscarlo: es la
                diferencia entre configurar esto de un vistazo o adivinarlo. */}
            <FieldHint>{t('share.personHint')}</FieldHint>
            <FieldHint>{t(`share.role.${role}.desc`)}</FieldHint>
          </div>

          {/* Quién tiene acceso ahora. */}
          <div class="flex flex-col gap-1.5">
            <h3 class="text-text text-sm font-semibold">{t('share.whoHasAccess')}</h3>
            {permissions.length === 0 ? (
              <EmptyState title={t('share.nobody')} description={t('share.nobodyHelp')} />
            ) : (
              <ul class="flex flex-col gap-1">
                {permissions.map((p) => (
                  <li
                    key={`${p.principal_kind}:${p.principal_id}`}
                    class="border-border bg-surface flex items-center justify-between gap-3 rounded-md border px-3 py-2"
                  >
                    <span class="min-w-0 truncate text-sm">
                      {p.principal_kind === 'tenant'
                        ? t('share.wholeOrg')
                        : (p.principal_name ?? p.principal_email ?? p.principal_id)}
                    </span>
                    <span class="flex shrink-0 items-center gap-2">
                      <Badge tone="neutral">{t(`share.role.${p.role}`)}</Badge>
                      {/* Al propietario no se le quita el acceso: quedaría un documento
                          sin dueño y nadie podría volver a compartirlo. */}
                      {p.role === 'owner' ? null : (
                        <Button
                          size="sm"
                          variant="ghost"
                          disabled={busy}
                          onClick={() => void revocar(p)}
                        >
                          {t('share.revoke')}
                        </Button>
                      )}
                    </span>
                  </li>
                ))}
              </ul>
            )}
          </div>

          {/* Enlaces públicos. */}
          <div class="flex flex-col gap-1.5">
            <h3 class="text-text text-sm font-semibold">{t('share.links')}</h3>
            {/* Un enlace público lo abre cualquiera que lo reciba, también fuera de la
                organización. Eso es un aviso, no una nota al pie. */}
            <InlineHelp tone="warning" title={t('share.linksHelp')}>
              {t('share.linksWarn')}
            </InlineHelp>
            {fresh?.token ? (
              <div class="border-warning bg-warning-bg mt-1 rounded-md border p-2.5" role="alert">
                <p class="text-warning-text text-xs font-semibold">{t('share.linkOnce')}</p>
                <code class="border-border bg-surface text-text mt-1.5 block truncate rounded border px-2 py-1 text-xs">
                  {shareUrl(fresh.token)}
                </code>
              </div>
            ) : null}
            {shares.some((s) => !s.revoked_at) ? (
              <ul class="mt-1 flex flex-col gap-1">
                {shares
                  .filter((s) => !s.revoked_at)
                  .map((s) => (
                    <li
                      key={s.id}
                      class="border-border bg-surface flex flex-col gap-2 rounded-md border px-3 py-2"
                    >
                      <div class="flex flex-wrap items-center justify-between gap-x-3 gap-y-1">
                        <span class="sytue-meta min-w-0 truncate">
                          {t(`share.role.${s.role}`)}
                          {s.expires_at
                            ? ` · ${t('share.expires', { date: s.expires_at.slice(0, 10) })}`
                            : ` · ${t('share.noExpiry')}`}
                          {s.policy_adjusted_at ? ` · ${t('share.adjusted')}` : ''}
                          {s.has_password ? ` · ${t('share.password.on')}` : ''}
                        </span>
                        <span class="flex items-center gap-1">
                          <Button
                            size="sm"
                            variant="ghost"
                            disabled={busy}
                            onClick={() => {
                              setClaveNueva('');
                              setEditando(editando === s.id ? null : s.id);
                            }}
                          >
                            {t(s.has_password ? 'share.password.change' : 'share.password.set')}
                          </Button>
                          <Button
                            size="sm"
                            variant="ghost"
                            disabled={busy}
                            onClick={() => void revocarEnlace(s.id)}
                          >
                            {t('share.revoke')}
                          </Button>
                        </span>
                      </div>
                      {editando === s.id ? (
                        <div class="flex flex-wrap items-center gap-2">
                          <Input
                            type="password"
                            class="min-w-[12rem] flex-1"
                            autoComplete="new-password"
                            maxLength={128}
                            aria-label={t('share.password.new')}
                            placeholder={t('share.password.new')}
                            value={claveNueva}
                            onInput={(e) => setClaveNueva(e.currentTarget.value)}
                          />
                          <Button
                            size="sm"
                            variant="secondary"
                            disabled={busy || claveNueva.length < 8}
                            onClick={() => void guardarClave(s.id, claveNueva)}
                          >
                            {t('share.policy.save')}
                          </Button>
                          {s.has_password ? (
                            <Button
                              size="sm"
                              variant="ghost"
                              disabled={busy}
                              onClick={() => void guardarClave(s.id, '')}
                            >
                              {t('share.password.remove')}
                            </Button>
                          ) : null}
                        </div>
                      ) : null}
                    </li>
                  ))}
              </ul>
            ) : null}
            <div class="mt-1 flex flex-wrap items-end gap-2">
              <label class="flex flex-col gap-1" for="share-expiry">
                <span class="sytue-meta">{t('share.expiry.label')}</span>
                <Input
                  id="share-expiry"
                  type="date"
                  value={expira}
                  min={diaLocal(1)}
                  max={politica && politica.max_days > 0 ? diaLocal(politica.max_days) : undefined}
                  onInput={(e) => setExpira(e.currentTarget.value)}
                />
              </label>
              <label class="flex flex-col gap-1" for="share-password">
                <span class="sytue-meta">{t('share.password.label')}</span>
                <Input
                  id="share-password"
                  type="password"
                  autoComplete="new-password"
                  maxLength={128}
                  value={clave}
                  onInput={(e) => setClave(e.currentTarget.value)}
                />
              </label>
              <Button
                size="sm"
                variant="secondary"
                disabled={busy || (clave !== '' && clave.length < 8)}
                onClick={() => void crearEnlace()}
              >
                {t('share.createLink')}
              </Button>
            </div>
            <FieldHint>{t('share.password.hint')}</FieldHint>
            {politica ? (
              <FieldHint>
                {politica.max_days > 0
                  ? t('share.expiry.hint', { days: politica.max_days })
                  : t('share.expiry.noLimit')}
              </FieldHint>
            ) : null}
            {/* El techo lo cambia quien administra la organización; el servicio lo exige. */}
            {politica?.can_manage ? (
              <div class="border-border mt-2 flex flex-col gap-1.5 border-t pt-3">
                <label class="text-text text-sm font-semibold" for="share-policy-days">
                  {t('share.policy.title')}
                </label>
                <div class="flex flex-wrap items-center gap-2">
                  <Input
                    id="share-policy-days"
                    type="number"
                    class="w-28"
                    min={0}
                    max={3650}
                    step={1}
                    value={techo}
                    onInput={(e) => setTecho(e.currentTarget.value)}
                  />
                  <Button
                    size="sm"
                    variant="secondary"
                    disabled={busy || techo === '' || Number(techo) === politica.max_days}
                    onClick={() => void guardarTecho()}
                  >
                    {t('share.policy.save')}
                  </Button>
                </div>
                <FieldHint>{t('share.policy.hint')}</FieldHint>
              </div>
            ) : null}
          </div>
        </div>
      )}
    </Modal>
  );
}
