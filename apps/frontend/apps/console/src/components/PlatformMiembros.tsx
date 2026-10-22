/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Las personas de UNA organización, vistas desde la consola de plataforma:
 * ponerle propietario —una cuenta que ya existe, o una persona nueva por
 * invitación— y atender una cuenta: restablecer su contraseña, bloquearla o
 * eliminarla.
 *
 * Lo decide todo Sytue ID (`platform_usuarios.go`): quién es la única
 * propietaria, el step-up, la auditoría. Aquí se pide el motivo, se hace repetir
 * el correo antes de eliminar y se enseña UNA vez la contraseña temporal.
 */
import { useCallback, useEffect, useState } from 'preact/hooks';
import {
  Badge,
  Button,
  ConfirmDialog,
  FieldGroup,
  InlineHelp,
  Input,
  Modal,
  Table,
  type useToast,
} from '@sytue/ui';
import type { ConsoleMessageKey as MessageKey, T } from '../i18n/index.js';
import type { PlatformMember, PlatformOrg } from '../lib/platform.js';
import { pedirReautenticacion } from '../lib/reautenticar.js';

type Toast = ReturnType<typeof useToast>;

/**
 * Al BFF de plataforma. Un 403 NO recarga, como en CrearOrganizacion: el motivo
 * escrito se conserva y el error se enseña.
 */
function pedir(path: string, body?: unknown): Promise<Response> {
  return fetch(
    `/api/platform${path}`,
    body === undefined
      ? {}
      : {
          method: 'POST',
          headers: { 'content-type': 'application/json' },
          body: JSON.stringify(body),
        },
  );
}

/** Qué decir cuando Sytue ID dice que no (sus mensajes vienen en inglés). */
async function motivoDelRechazo(res: Response, t: T): Promise<string> {
  const c = (await res.json().catch(() => ({}))) as {
    code?: string;
    message?: string;
    params?: { orgs?: string };
  };
  switch (c.code) {
    case 'insufficient_user_authentication':
      return t('platform.members.error.stepUp');
    case 'sole_owner':
      return t('platform.members.error.soleOwner', { orgs: c.params?.orgs ?? '' });
    case 'no_such_user':
      return t('platform.members.error.noSuchUser');
    case 'account_exists':
      return t('platform.members.error.accountExists');
    case 'account_inactive':
      return t('platform.members.error.inactive');
    case 'platform_admin':
      return t('platform.members.error.platformAdmin');
    default:
      return c.message || t('platform.toast.failed');
  }
}

const ROL: Record<PlatformMember['role'], MessageKey> = {
  owner: 'role.owner',
  admin: 'role.admin',
  member: 'role.member',
  billing: 'role.billing',
};

// Los de la organización no valen: concuerdan con «organización» y aquí se habla
// de una persona, y «suspendida» es aquí «bloqueada», que es el botón de al lado.
const ESTADO: Record<PlatformMember['status'], MessageKey> = {
  active: 'platform.members.status.active',
  suspended: 'platform.members.status.suspended',
  deleted: 'platform.members.status.deleted',
};

const TONO: Record<PlatformMember['status'], 'success' | 'warning' | 'neutral'> = {
  active: 'success',
  suspended: 'warning',
  deleted: 'neutral',
};

type Accion =
  | { tipo: 'assign'; email: string }
  | { tipo: 'invite' }
  | { tipo: 'password' | 'block' | 'unblock' | 'delete'; m: PlatformMember };

export function PlatformMiembros({ org, t, toast }: { org: PlatformOrg; t: T; toast: Toast }) {
  const [q, setQ] = useState('');
  const [items, setItems] = useState<PlatformMember[] | null>(null);
  const [next, setNext] = useState('');
  const [fallo, setFallo] = useState(false);
  const [accion, setAccion] = useState<Accion | null>(null);
  const [temporal, setTemporal] = useState<{ email: string; password: string } | null>(null);

  const cargar = useCallback(
    async (termino: string, cursor = '') => {
      const qs = new URLSearchParams();
      if (termino.trim()) qs.set('q', termino.trim());
      if (cursor) qs.set('cursor', cursor);
      const res = await pedir(`/orgs/${org.id}/members?${qs.toString()}`);
      if (!res.ok) {
        setFallo(true);
        return;
      }
      const data = (await res.json()) as { items?: PlatformMember[]; nextCursor?: string };
      setFallo(false);
      setItems((prev) => (cursor ? [...(prev ?? []), ...(data.items ?? [])] : (data.items ?? [])));
      setNext(data.nextCursor ?? '');
    },
    [org.id],
  );

  useEffect(() => {
    void cargar('');
  }, [cargar]);

  // Una fusionada ya no tiene a quién poner: sus miembros están en el destino.
  const admitePropietario = org.status !== 'merged';

  return (
    <>
      <h4 class="sy-stat__label">{t('platform.members.title')}</h4>
      {admitePropietario && (
        <div class="sy-row-actions">
          <Button size="sm" onClick={() => setAccion({ tipo: 'assign', email: '' })}>
            {t('platform.members.assignOwner')}
          </Button>
          <Button size="sm" variant="secondary" onClick={() => setAccion({ tipo: 'invite' })}>
            {t('platform.members.inviteOwner')}
          </Button>
        </div>
      )}
      <p class="sy-hint">{t('platform.members.ownersHint')}</p>
      <form
        onSubmit={(e) => {
          e.preventDefault();
          void cargar(q);
        }}
      >
        <Input
          type="search"
          value={q}
          placeholder={t('platform.members.search')}
          aria-label={t('platform.members.search')}
          onInput={(e) => setQ((e.target as HTMLInputElement).value)}
        />
      </form>
      {fallo ? (
        <p class="sy-hint">{t('platform.error.unavailable')}</p>
      ) : items === null ? (
        <p class="sy-hint">{t('common.loading')}</p>
      ) : items.length === 0 ? (
        <p class="sy-hint">{t('platform.members.empty')}</p>
      ) : (
        <Table<PlatformMember>
          rowKey={(m) => m.id}
          rows={items}
          caption={t('platform.members.title')}
          columns={[
            {
              key: 'person',
              header: t('platform.members.col.person'),
              cell: (m) => (
                <>
                  <strong>{m.name || m.email}</strong>
                  <br />
                  <span class="text-text-subtle text-xs">{m.email}</span>
                </>
              ),
            },
            {
              key: 'role',
              header: t('platform.members.col.role'),
              cell: (m) => t(ROL[m.role]),
            },
            {
              key: 'status',
              header: t('platform.col.status'),
              cell: (m) => <Badge tone={TONO[m.status]}>{t(ESTADO[m.status])}</Badge>,
            },
            {
              key: 'actions',
              header: t('platform.col.actions'),
              align: 'end',
              // Una baja no se toca desde aquí: la restaura la organización.
              cell: (m) =>
                m.status === 'deleted' ? null : (
                  <div class="sy-row-actions">
                    {admitePropietario && m.role !== 'owner' && m.status === 'active' && (
                      <Button
                        size="sm"
                        variant="secondary"
                        onClick={() => setAccion({ tipo: 'assign', email: m.email })}
                      >
                        {t('platform.members.makeOwner')}
                      </Button>
                    )}
                    <Button
                      size="sm"
                      variant="secondary"
                      onClick={() => setAccion({ tipo: 'password', m })}
                    >
                      {t('platform.members.resetPassword')}
                    </Button>
                    <Button
                      size="sm"
                      variant="secondary"
                      onClick={() =>
                        setAccion({ tipo: m.status === 'active' ? 'block' : 'unblock', m })
                      }
                    >
                      {m.status === 'active'
                        ? t('platform.members.block')
                        : t('platform.members.unblock')}
                    </Button>
                    <Button
                      size="sm"
                      variant="danger"
                      onClick={() => setAccion({ tipo: 'delete', m })}
                    >
                      {t('platform.action.delete')}
                    </Button>
                  </div>
                ),
            },
          ]}
        />
      )}
      {next && (
        <Button size="sm" variant="secondary" onClick={() => void cargar(q, next)}>
          {t('common.loadMore')}
        </Button>
      )}

      {accion && (
        <AccionDialog
          org={org}
          accion={accion}
          t={t}
          onClose={() => setAccion(null)}
          onHecho={(pass) => {
            if (pass && 'm' in accion) setTemporal({ email: accion.m.email, password: pass });
            setAccion(null);
            void cargar(q);
          }}
          toast={toast}
        />
      )}

      {temporal && (
        <Modal open title={t('platform.members.temporaryTitle')} onClose={() => setTemporal(null)}>
          <div class="sy-form">
            <InlineHelp tone="warning">
              {t('platform.members.temporaryOnce', { email: temporal.email })}
            </InlineHelp>
            <Input
              readOnly
              value={temporal.password}
              class="font-mono"
              aria-label={t('platform.members.temporaryTitle')}
            />
            <div class="sy-form__actions">
              <Button
                variant="secondary"
                onClick={() => {
                  void navigator.clipboard
                    ?.writeText(temporal.password)
                    .then(() => toast.notify({ tone: 'success', title: t('common.copied') }));
                }}
              >
                {t('common.copy')}
              </Button>
              <Button onClick={() => setTemporal(null)}>{t('common.close')}</Button>
            </div>
          </div>
        </Modal>
      )}
    </>
  );
}

/**
 * Un diálogo para las seis acciones: todas llevan motivo, y cada una sus campos.
 * `onHecho` recibe la contraseña temporal si la hubo, para enseñarla una vez.
 */
function AccionDialog({
  org,
  accion,
  t,
  onClose,
  onHecho,
  toast,
}: {
  org: PlatformOrg;
  accion: Accion;
  t: T;
  onClose: () => void;
  onHecho: (temporal?: string) => void;
  toast: Toast;
}) {
  const [email, setEmail] = useState(accion.tipo === 'assign' ? accion.email : '');
  const [nombre, setNombre] = useState('');
  const [modo, setModo] = useState<'link' | 'temporary'>('link');
  const [confirmacion, setConfirmacion] = useState('');
  const [motivo, setMotivo] = useState('');
  const [enviando, setEnviando] = useState(false);

  const quien = 'm' in accion ? accion.m.email : '';
  const invalido =
    motivo.trim() === '' ||
    ((accion.tipo === 'assign' || accion.tipo === 'invite') && email.trim() === '') ||
    (accion.tipo === 'invite' && nombre.trim() === '') ||
    (accion.tipo === 'delete' && confirmacion.trim().toLowerCase() !== quien.toLowerCase());

  const titulo: Record<Accion['tipo'], string> = {
    assign: t('platform.members.assignTitle'),
    invite: t('platform.members.inviteTitle'),
    password: t('platform.members.passwordTitle', { email: quien }),
    block: t('platform.members.blockTitle', { email: quien }),
    unblock: t('platform.members.unblockTitle', { email: quien }),
    delete: t('platform.members.deleteTitle', { email: quien }),
  };
  const boton: Record<Accion['tipo'], string> = {
    assign: t('platform.members.assignOwner'),
    invite: t('platform.members.inviteOwner'),
    password: t('platform.members.resetPassword'),
    block: t('platform.members.block'),
    unblock: t('platform.members.unblock'),
    delete: t('platform.action.delete'),
  };

  async function enviar() {
    if (invalido || enviando) return;
    setEnviando(true);
    const reason = motivo.trim();
    const base = `/orgs/${org.id}`;
    let ruta: string;
    let cuerpo: Record<string, string> = { reason };
    switch (accion.tipo) {
      case 'assign':
        ruta = `${base}/owners`;
        cuerpo = { email: email.trim(), reason };
        break;
      case 'invite':
        ruta = `${base}/owner-invitations`;
        cuerpo = { email: email.trim(), name: nombre.trim(), reason };
        break;
      case 'password':
        ruta = `${base}/members/${accion.m.id}/password`;
        cuerpo = { mode: modo, reason };
        break;
      case 'delete':
        ruta = `${base}/members/${accion.m.id}/delete`;
        cuerpo = { confirm_email: confirmacion.trim(), reason };
        break;
      default:
        ruta = `${base}/members/${accion.m.id}/${accion.tipo}`;
    }
    const res = await pedir(ruta, cuerpo);
    setEnviando(false);
    // Una acción sensible sin autenticación reciente: a confirmarla, y de vuelta aquí.
    if (await pedirReautenticacion(res)) return;
    if (!res.ok) {
      toast.notify({ title: await motivoDelRechazo(res, t), tone: 'danger' });
      return;
    }
    const data = (await res.json().catch(() => ({}))) as {
      temporary_password?: string;
      sessions_revoked?: number;
      already_owner?: boolean;
    };
    const sesiones =
      typeof data.sessions_revoked === 'number'
        ? t('platform.members.sessionsClosed', { count: data.sessions_revoked })
        : '';
    const hecho: Record<Accion['tipo'], string> = {
      assign: data.already_owner
        ? t('platform.members.toast.alreadyOwner')
        : t('platform.members.toast.ownerAssigned'),
      invite: t('platform.members.toast.ownerInvited', { email: email.trim() }),
      password:
        modo === 'link'
          ? t('platform.members.toast.linkSent')
          : t('platform.members.temporaryTitle'),
      block: t('platform.members.toast.blocked'),
      unblock: t('platform.members.toast.unblocked'),
      delete: t('platform.members.toast.deleted'),
    };
    toast.notify({
      title: hecho[accion.tipo],
      ...(sesiones ? { description: sesiones } : {}),
      tone: 'success',
    });
    onHecho(data.temporary_password);
  }

  return (
    <ConfirmDialog
      open
      onClose={onClose}
      onConfirm={() => void enviar()}
      title={titulo[accion.tipo]}
      confirmLabel={boton[accion.tipo]}
      tone={accion.tipo === 'delete' || accion.tipo === 'block' ? 'danger' : 'primary'}
      confirmDisabled={invalido || enviando}
      busy={enviando}
    >
      {accion.tipo === 'assign' && <p class="sy-hint">{t('platform.members.assignHelp')}</p>}
      {accion.tipo === 'invite' && <p class="sy-hint">{t('platform.members.inviteHelp')}</p>}
      {accion.tipo === 'password' && <p class="sy-hint">{t('platform.members.passwordHint')}</p>}
      {accion.tipo === 'block' && <p class="sy-hint">{t('platform.members.blockHint')}</p>}
      {accion.tipo === 'delete' && <p class="sy-warning">{t('platform.members.deleteHint')}</p>}

      {(accion.tipo === 'assign' || accion.tipo === 'invite') && (
        <FieldGroup label={t('platform.col.email')} htmlFor="miembro-email" required>
          <Input
            id="miembro-email"
            type="email"
            value={email}
            onInput={(e) => setEmail(e.currentTarget.value)}
          />
        </FieldGroup>
      )}
      {accion.tipo === 'invite' && (
        <FieldGroup label={t('platform.members.name')} htmlFor="miembro-nombre" required>
          <Input
            id="miembro-nombre"
            value={nombre}
            onInput={(e) => setNombre(e.currentTarget.value)}
          />
        </FieldGroup>
      )}
      {accion.tipo === 'password' && (
        <FieldGroup label={t('platform.members.passwordMode')} htmlFor="miembro-modo">
          <select
            id="miembro-modo"
            class="sy-select"
            value={modo}
            onChange={(e) => setModo((e.target as HTMLSelectElement).value as typeof modo)}
          >
            <option value="link">{t('platform.members.passwordLink')}</option>
            <option value="temporary">{t('platform.members.passwordTemporary')}</option>
          </select>
        </FieldGroup>
      )}
      {accion.tipo === 'delete' && (
        <FieldGroup
          label={t('platform.members.deleteConfirm', { email: quien })}
          htmlFor="miembro-confirmar"
          required
        >
          <Input
            id="miembro-confirmar"
            value={confirmacion}
            autocomplete="off"
            onInput={(e) => setConfirmacion(e.currentTarget.value)}
          />
        </FieldGroup>
      )}
      <FieldGroup label={t('platform.prompt.statusReason')} htmlFor="miembro-motivo" required>
        <Input
          id="miembro-motivo"
          value={motivo}
          onInput={(e) => setMotivo(e.currentTarget.value)}
        />
      </FieldGroup>
    </ConfirmDialog>
  );
}
