/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Entradas del menú Archivo para la biblioteca de plantillas de la organización:
 * marcar este documento como plantilla, publicar una versión nueva y quitar la
 * marca. Quién puede cada cosa lo decide el servicio (marcar y quitar, quien administra
 * la organización; publicar, quien edita el documento); aquí solo se ofrece lo que
 * esta persona puede hacer, y un 403 se cuenta en un aviso.
 *
 * Se monta solo con el menú abierto, así que pregunta el estado una vez por apertura.
 */
import type { JSX } from 'preact';
import { useEffect, useState } from 'preact/hooks';
import { MenuItem, MenuSeparator, useToast } from '@sytue/ui';
import type { DocTemplate } from '@sytue/api-clients';
import type { DocsKey } from '../../i18n';
import type { EditorContext } from '../types';

interface Ficha {
  template: DocTemplate | null;
  canManage: boolean;
}

/** Sin plantilla todavía el servicio contesta 404 y no dice si administras: se
 *  pregunta a la galería, que sí lo dice. */
async function puedeAdministrar(): Promise<boolean> {
  try {
    const res = await fetch('/api/templates', { credentials: 'include' });
    return res.ok && ((await res.json()) as { can_manage?: boolean }).can_manage === true;
  } catch {
    return false;
  }
}

export function PlantillaOrgItems({
  ctx,
  cerrar,
}: {
  ctx: EditorContext;
  cerrar: () => void;
}): JSX.Element | null {
  const toast = useToast();
  const [ficha, setFicha] = useState<Ficha | null>(null);
  const id = ctx.doc.id;

  useEffect(() => {
    let vigente = true;
    fetch(`/api/org-templates?id=${encodeURIComponent(id)}`, { credentials: 'include' })
      .then(async (res) => {
        if (res.status === 404) return { template: null, canManage: await puedeAdministrar() };
        if (!res.ok) return null;
        const b = (await res.json()) as { template?: DocTemplate; can_manage?: boolean };
        return { template: b.template ?? null, canManage: b.can_manage === true };
      })
      .then((f) => vigente && setFicha(f))
      .catch(() => undefined);
    return () => {
      vigente = false;
    };
  }, [id]);

  const llamar = (metodo: 'PUT' | 'POST' | 'DELETE', ok: DocsKey, cuerpo = '{}') => {
    cerrar();
    void (async () => {
      try {
        const res = await fetch(`/api/org-templates?id=${encodeURIComponent(id)}`, {
          method: metodo,
          credentials: 'include',
          headers: { 'content-type': 'application/json' },
          body: metodo === 'DELETE' ? null : cuerpo,
        });
        if (!res.ok) throw new Error(String(res.status));
        toast.notify({ tone: 'success', title: ctx.t(ok) });
      } catch {
        toast.notify({ tone: 'danger', title: ctx.t('orgTemplate.msg.failed') });
      }
    })();
  };

  if (!ficha) return null;
  const sinRed = ctx.connection === 'offline';
  const puedeMarcar = !ficha.template && ficha.canManage && !ctx.soloLectura;
  const puedePublicar = !!ficha.template && !ctx.soloLectura;
  const puedeQuitar = !!ficha.template && ficha.canManage;
  if (!puedeMarcar && !puedePublicar && !puedeQuitar) return null;
  return (
    <>
      <MenuSeparator />
      {puedeMarcar && (
        <MenuItem
          size="sm"
          disabled={sinRed}
          onClick={() => llamar('PUT', 'orgTemplate.msg.marked')}
        >
          {ctx.t('orgTemplate.mark')}
        </MenuItem>
      )}
      {/* Un bloque es la misma marca con otro uso: se inserta desde la cinta en vez de
          ofrecerse en «Crear nuevo». Mismas versiones, mismo registro. */}
      {puedeMarcar && (
        <MenuItem
          size="sm"
          disabled={sinRed}
          onClick={() => llamar('PUT', 'orgBlock.msg.marked', '{"block":true}')}
        >
          {ctx.t('orgBlock.mark')}
        </MenuItem>
      )}
      {puedePublicar && (
        <MenuItem
          size="sm"
          disabled={sinRed}
          onClick={() => llamar('POST', 'orgTemplate.msg.published')}
        >
          {ctx.t('orgTemplate.publish')}
        </MenuItem>
      )}
      {puedeQuitar && (
        <MenuItem
          size="sm"
          disabled={sinRed}
          onClick={() => llamar('DELETE', 'orgTemplate.msg.removed')}
        >
          {ctx.t('orgTemplate.unmark')}
        </MenuItem>
      )}
    </>
  );
}
