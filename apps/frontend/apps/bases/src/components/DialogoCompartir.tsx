/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Compartir una base (solo el propietario): con toda la organización, con un grupo de
 * Sytue ID, con una persona —por su dirección, que el servicio resuelve en el
 * directorio— o con alguien de OTRA organización —por su @usuario o su dirección: la
 * base sigue en esta y esa persona no ve nada más—; ver quién tiene acceso y retirarlo; y
 * transferir la propiedad a otra persona de la organización. Todo lo decide el servicio: aquí solo se pide.
 */
import type { JSX } from 'preact';
import { useCallback, useEffect, useState } from 'preact/hooks';
import { Button, FieldGroup, Input, Modal, Select, useConfirm } from '@sytue/ui';
import type { BasesPermission } from '@sytue/api-clients/bases';
import type { T } from '../i18n/index.js';
import { cliente, mensajeDeError } from '../lib/api.js';

type RolConcedible = BasesPermission['role'];
type Clase = BasesPermission['principal_kind'];

/** Cómo se enseña una entrada de la ACL. */
export function nombreDePermiso(x: BasesPermission, t: T): string {
  if (x.principal_kind === 'tenant') return t('share.kind.tenant');
  if (x.principal_kind === 'group') return t('share.groupLabel', { name: x.principal_id });
  if (x.principal_kind === 'external')
    return t('share.externalLabel', { name: x.principal_label || x.principal_id });
  return x.principal_label || x.principal_id;
}

export function DialogoCompartir(p: {
  t: T;
  baseId: string;
  nombreBase: string;
  /** La organización de quien comparte (el principal de «toda la organización»). */
  tenant: string;
  onCerrar: () => void;
  /** Tras transferir la propiedad: quien llamaba ya no es propietario. */
  onTransferida: () => void;
}): JSX.Element {
  const { t } = p;
  const { confirmar, dialogo } = useConfirm();
  const [permisos, setPermisos] = useState<BasesPermission[]>([]);
  const [grupos, setGrupos] = useState<string[]>([]);
  const [clase, setClase] = useState<Clase>('tenant');
  const [quien, setQuien] = useState('');
  const [rol, setRol] = useState<RolConcedible>('viewer');
  const [destino, setDestino] = useState('');
  const [ocupado, setOcupado] = useState(false);
  const [error, setError] = useState('');

  const cargar = useCallback(async () => {
    try {
      setPermisos(await cliente().listPermissions(p.baseId));
    } catch (e) {
      setError(mensajeDeError(e, t));
    }
  }, [p.baseId, t]);
  useEffect(() => {
    void cargar();
    cliente()
      .myGroups()
      .then(setGrupos)
      .catch(() => setGrupos([]));
  }, [cargar]);

  async function conceder(e: Event): Promise<void> {
    e.preventDefault();
    const valor = quien.trim();
    if (clase !== 'tenant' && !valor) return;
    setOcupado(true);
    setError('');
    try {
      await cliente().grantPermission(
        p.baseId,
        clase === 'tenant'
          ? { principal_kind: 'tenant', principal_id: p.tenant, role: rol }
          : clase === 'group'
            ? { principal_kind: 'group', principal_id: valor, role: rol }
            : clase === 'external'
              ? { principal_kind: 'external', query: valor, role: rol }
              : { principal_kind: 'user', email: valor, role: rol },
      );
      setQuien('');
      await cargar();
    } catch (err) {
      setError(mensajeDeError(err, t));
    } finally {
      setOcupado(false);
    }
  }

  async function revocar(id: string): Promise<void> {
    try {
      await cliente().revokePermission(p.baseId, id);
      setPermisos((ps) => ps.filter((x) => x.id !== id));
    } catch (err) {
      setError(mensajeDeError(err, t));
    }
  }

  async function transferir(e: Event): Promise<void> {
    e.preventDefault();
    const email = destino.trim();
    if (!email) return;
    if (
      !(await confirmar({
        mensaje: t('transfer.confirm', { name: p.nombreBase, email }),
        confirmLabel: t('transfer.run'),
        tone: 'danger',
      }))
    )
      return;
    setOcupado(true);
    setError('');
    try {
      await cliente().transferBase(p.baseId, email);
      p.onTransferida();
    } catch (err) {
      setError(mensajeDeError(err, t));
    } finally {
      setOcupado(false);
    }
  }

  return (
    <Modal open onClose={p.onCerrar} title={t('share.title')}>
      <div class="flex flex-col gap-5">
        <p class="text-text-muted text-sm">{t('share.intro')}</p>
        <form class="flex flex-col gap-3" onSubmit={(e) => void conceder(e)}>
          <div class="grid gap-3 sm:grid-cols-2">
            <FieldGroup label={t('share.who')} htmlFor="compartir-clase">
              <Select
                id="compartir-clase"
                value={clase}
                onChange={(e) => {
                  setClase(e.currentTarget.value as Clase);
                  setQuien('');
                }}
              >
                {(['tenant', 'group', 'user', 'external'] as const).map((c) => (
                  <option key={c} value={c}>
                    {t(`share.kind.${c}`)}
                  </option>
                ))}
              </Select>
            </FieldGroup>
            <FieldGroup label={t('share.role')} htmlFor="compartir-rol">
              <Select
                id="compartir-rol"
                value={rol}
                onChange={(e) => setRol(e.currentTarget.value as RolConcedible)}
              >
                {(['viewer', 'commenter', 'editor'] as const).map((r) => (
                  <option key={r} value={r}>
                    {t(`role.${r}`)}
                  </option>
                ))}
              </Select>
            </FieldGroup>
          </div>
          {clase === 'user' ? (
            <FieldGroup label={t('share.email')} htmlFor="compartir-quien" required>
              <Input
                id="compartir-quien"
                type="email"
                autoComplete="off"
                maxLength={254}
                value={quien}
                onInput={(e) => setQuien(e.currentTarget.value)}
              />
            </FieldGroup>
          ) : null}
          {clase === 'external' ? (
            <FieldGroup
              label={t('share.external')}
              htmlFor="compartir-quien"
              help={t('share.externalHelp')}
              required
            >
              <Input
                id="compartir-quien"
                aria-describedby="compartir-quien-help"
                autoComplete="off"
                maxLength={254}
                value={quien}
                onInput={(e) => setQuien(e.currentTarget.value)}
              />
            </FieldGroup>
          ) : null}
          {clase === 'group' ? (
            <FieldGroup
              label={t('share.group')}
              htmlFor="compartir-quien"
              help={t('share.groupHelp')}
              required
            >
              <Input
                id="compartir-quien"
                list="compartir-grupos"
                aria-describedby="compartir-quien-help"
                autoComplete="off"
                maxLength={200}
                value={quien}
                onInput={(e) => setQuien(e.currentTarget.value)}
              />
              <datalist id="compartir-grupos">
                {grupos.map((g) => (
                  <option key={g} value={g} />
                ))}
              </datalist>
            </FieldGroup>
          ) : null}
          <div>
            <Button type="submit" disabled={ocupado || (clase !== 'tenant' && !quien.trim())}>
              {t('share.grant')}
            </Button>
          </div>
        </form>
        {error ? (
          <p role="alert" class="text-danger-text text-sm">
            {error}
          </p>
        ) : null}
        {permisos.length === 0 ? (
          <p class="text-text-muted text-sm">{t('share.none')}</p>
        ) : (
          <ul aria-label={t('share.listLabel')} class="flex flex-col gap-2">
            {permisos.map((x) => (
              <li key={x.id} class="flex items-center justify-between gap-2 text-sm">
                <span class="text-text min-w-0 truncate">
                  {nombreDePermiso(x, t)}
                  {' · '}
                  {t(`role.${x.role}`)}
                  {x.table_id ? ` · ${t('share.oneTable')}` : ''}
                </span>
                <Button variant="ghost" size="sm" onClick={() => void revocar(x.id)}>
                  {t('share.revoke')}
                </Button>
              </li>
            ))}
          </ul>
        )}
        <form
          class="border-border flex flex-col gap-3 border-t pt-4"
          onSubmit={(e) => void transferir(e)}
        >
          <h3 class="text-text text-sm font-semibold">{t('transfer.title')}</h3>
          <p class="text-text-muted text-sm" id="transferir-ayuda">
            {t('transfer.help')}
          </p>
          <FieldGroup label={t('transfer.email')} htmlFor="transferir-destino">
            <Input
              id="transferir-destino"
              type="email"
              autoComplete="off"
              maxLength={254}
              aria-describedby="transferir-ayuda"
              value={destino}
              onInput={(e) => setDestino(e.currentTarget.value)}
            />
          </FieldGroup>
          <div>
            <Button type="submit" variant="secondary" disabled={ocupado || !destino.trim()}>
              {t('transfer.run')}
            </Button>
          </div>
        </form>
      </div>
      {dialogo}
    </Modal>
  );
}
