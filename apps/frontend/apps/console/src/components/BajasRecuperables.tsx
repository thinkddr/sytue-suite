/**
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * BajasRecuperables — las cuentas cerradas que todavía se pueden restaurar.
 *
 * Sytue ID guarda una cuenta cerrada durante la retención de la organización y
 * sabía restaurarla desde el principio, pero la consola no enseñaba ninguna: el
 * directorio no lista las bajas, así que deshacer un cierre equivocado exigía
 * saberse un uuid de memoria. Es la papelera de Entra («usuarios eliminados»).
 *
 * Restaurar devuelve la cuenta SUSPENDIDA: los datos vuelven, el acceso no hasta
 * que alguien la reactive en el directorio. Pide identidad reciente (step-up): si
 * Sytue ID la exige, se lleva al login y se vuelve aquí a repetirla.
 *
 * Solo se pinta si hay alguna, como las invitaciones pendientes: en una
 * organización asentada lo normal es que no haya ninguna. Vive dentro del
 * directorio y usa SU proveedor de avisos (las islas no comparten contexto).
 */
import { useEffect, useMemo, useState } from 'preact/hooks';
import { Badge, Button, ConfirmDialog, useToast } from '@sytue/ui';
import { formatDateTime, type Locale } from '@sytue/i18n';
import { useTGestion } from '../i18n/gestion/index.js';
import { pedirReautenticacion } from '../lib/reautenticar.js';
import { diasParaRestaurar, type CuentaCerrada } from '../lib/gestion.js';

interface Props {
  locale: Locale;
}

export default function BajasRecuperables({ locale }: Props) {
  const tg = useTGestion(locale);
  const toast = useToast();
  const [cuentas, setCuentas] = useState<CuentaCerrada[]>([]);
  const [retencion, setRetencion] = useState(30);
  const [aRestaurar, setARestaurar] = useState<CuentaCerrada | null>(null);
  const [restaurando, setRestaurando] = useState(false);

  useEffect(() => {
    let vigente = true;
    void fetch('/api/admin/users/closed', { headers: { accept: 'application/json' } })
      .then((r) => (r.ok ? r.json() : null))
      .then((d: { users?: CuentaCerrada[]; retention_days?: number } | null) => {
        if (!vigente || !d) return;
        setCuentas(Array.isArray(d.users) ? d.users : []);
        if (typeof d.retention_days === 'number') setRetencion(d.retention_days);
      })
      // Sin la lista la sección no se pinta: es una ayuda, no la pantalla.
      .catch(() => undefined);
    return () => {
      vigente = false;
    };
  }, []);

  async function restaurar(c: CuentaCerrada) {
    setRestaurando(true);
    try {
      const res = await fetch(`/api/admin/users/${encodeURIComponent(c.id)}/restore`, {
        method: 'POST',
      });
      if (await pedirReautenticacion(res)) return;
      if (!res.ok) throw new Error(String(res.status));
      setCuentas((prev) => prev.filter((x) => x.id !== c.id));
      setARestaurar(null);
      toast.notify({
        tone: 'success',
        title: tg('gestion.bajas.restaurada', { name: c.name || c.email }),
        description: tg('gestion.bajas.reactivar'),
      });
    } catch {
      toast.notify({ tone: 'danger', title: tg('gestion.bajas.fallo') });
    } finally {
      setRestaurando(false);
    }
  }

  // Una vez por montaje: los días que quedan no necesitan reloj vivo.
  const ahora = useMemo(() => Date.now(), []);
  if (cuentas.length === 0) return null;

  return (
    <section
      aria-labelledby="bajas-titulo"
      class="border-border bg-surface mx-4 mb-4 rounded-lg border"
    >
      <div class="border-border flex flex-wrap items-baseline gap-x-2 gap-y-1 border-b px-3 py-2.5">
        <h2 id="bajas-titulo" class="text-text text-sm font-semibold">
          {tg('gestion.bajas.titulo')}
        </h2>
        <span class="text-text-muted text-sm">({cuentas.length})</span>
        <p class="text-text-muted w-full text-sm">
          {tg('gestion.bajas.ayuda', { days: retencion })}
        </p>
      </div>
      <ul class="divide-border divide-y">
        {cuentas.map((c) => {
          const dias = diasParaRestaurar(c.restorable_until, ahora);
          return (
            <li key={c.id} class="flex flex-wrap items-center gap-x-3 gap-y-1 px-3 py-2">
              <span class="min-w-0 flex-1">
                <span class="text-text block truncate text-sm font-medium">
                  {c.name || c.email}
                </span>
                <span class="sytue-meta block truncate">
                  {c.email} ·{' '}
                  {tg('gestion.bajas.cerrada', { when: formatDateTime(locale, c.closed_at) })}
                  {c.reason ? ` · ${tg('gestion.bajas.motivo', { reason: c.reason })}` : ''}
                </span>
              </span>
              <Badge tone={dias <= 3 ? 'warning' : 'neutral'}>
                {tg('gestion.bajas.quedan', { count: dias })}
              </Badge>
              <Button
                size="sm"
                variant="secondary"
                disabled={dias === 0 || restaurando}
                onClick={() => setARestaurar(c)}
              >
                {tg('gestion.bajas.restaurar')}
              </Button>
            </li>
          );
        })}
      </ul>
      <ConfirmDialog
        open={aRestaurar !== null}
        onClose={() => {
          if (!restaurando) setARestaurar(null);
        }}
        onConfirm={() => {
          if (aRestaurar) void restaurar(aRestaurar);
        }}
        title={tg('gestion.bajas.confirmarTitulo', {
          name: aRestaurar?.name || aRestaurar?.email || '',
        })}
        confirmLabel={tg('gestion.bajas.restaurar')}
        busy={restaurando}
      >
        <p class="text-text-muted text-sm">{tg('gestion.bajas.confirmar')}</p>
      </ConfirmDialog>
    </section>
  );
}
