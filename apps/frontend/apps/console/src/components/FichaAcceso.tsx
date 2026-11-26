/**
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * FichaAcceso — dos preguntas que la ficha de una persona no sabía contestar:
 *
 *   - «¿Qué puede hacer, y por qué?»: el acceso EFECTIVO —rol integrado, roles
 *     personalizados y elevaciones PIM vivas— con la fuente de cada permiso. Lo
 *     calcula Sytue ID con la regla con la que autoriza; la consola lo enseñaba por
 *     piezas en cuatro pantallas y la suma se hacía a ojo, que es como se cree
 *     retirado un permiso que sigue llegando por otro sitio.
 *   - «¿Dónde tiene la sesión abierta?»: dispositivo, IP, cuándo entró y si lo hizo
 *     con segundo factor. Antes solo se podía cerrar todas sin ver ninguna.
 *
 * Las dos se piden al desplegarlas, no al abrir la ficha: son lecturas que no hacen
 * falta para la mayoría de visitas, y la ficha ya pide el impacto al abrirse.
 */
import { useState } from 'preact/hooks';
import { Badge, Button, InlineHelp, Spinner } from '@sytue/ui';
import { formatDateTime, type Locale } from '@sytue/i18n';
import type { T } from '../i18n/index.js';
import { useTGestion, type TGestion } from '../i18n/gestion/index.js';
import { permissionLabelKey } from '../lib/roles.js';
import { formatWhen } from '../lib/format.js';
import {
  conSegundoFactor,
  type AccesoEfectivo,
  type Concesion,
  type SesionAdmin,
} from '../lib/gestion.js';

type Carga<D> =
  { estado: 'cerrado' } | { estado: 'cargando' } | { estado: 'error' } | { estado: 'ok'; datos: D };

function permLabel(key: string, t: T): string {
  const label = permissionLabelKey(key);
  return label ? t(label) : key;
}

/** El porqué de un permiso, dicho para quien tiene que ir a quitarlo. */
function fuente(c: Concesion, tg: TGestion, locale: Locale): string {
  switch (c.source) {
    case 'rol_integrado':
      return tg('gestion.acceso.fuente.integrado', { role: c.detail ?? '' });
    case 'rol_personalizado':
      return tg('gestion.acceso.fuente.rol', { name: c.detail ?? '' });
    case 'elevacion_temporal':
      return tg('gestion.acceso.fuente.elevacion', {
        name: c.detail ?? '',
        when: c.expires_at ? formatDateTime(locale, c.expires_at) : '—',
      });
    default:
      return c.source;
  }
}

async function pedir<D>(url: string): Promise<D> {
  const res = await fetch(url, { headers: { accept: 'application/json' } });
  if (!res.ok) throw new Error(String(res.status));
  return (await res.json()) as D;
}

export default function FichaAcceso({
  userId,
  locale,
  t,
}: {
  userId: string;
  locale: Locale;
  /** La `t` de la consola (rótulos de permisos), que ya tiene la ficha. */
  t: T;
}) {
  const tg = useTGestion(locale);
  const [acceso, setAcceso] = useState<Carga<AccesoEfectivo>>({ estado: 'cerrado' });
  const [sesiones, setSesiones] = useState<Carga<SesionAdmin[]>>({ estado: 'cerrado' });
  const base = `/api/admin/users/${encodeURIComponent(userId)}`;

  async function verAcceso() {
    setAcceso({ estado: 'cargando' });
    try {
      setAcceso({ estado: 'ok', datos: await pedir<AccesoEfectivo>(`${base}/effective-access`) });
    } catch {
      setAcceso({ estado: 'error' });
    }
  }

  async function verSesiones() {
    setSesiones({ estado: 'cargando' });
    try {
      const r = await pedir<{ sessions?: SesionAdmin[] }>(`${base}/sessions`);
      setSesiones({ estado: 'ok', datos: r.sessions ?? [] });
    } catch {
      setSesiones({ estado: 'error' });
    }
  }

  return (
    <>
      <section aria-labelledby={`acceso-${userId}`} class="flex flex-col gap-2">
        <div class="flex items-center justify-between gap-2">
          <h3 id={`acceso-${userId}`} class="text-text text-sm font-semibold">
            {tg('gestion.acceso.titulo')}
          </h3>
          {acceso.estado === 'cerrado' || acceso.estado === 'error' ? (
            <Button size="sm" variant="secondary" onClick={() => void verAcceso()}>
              {acceso.estado === 'error' ? t('common.tryAgain') : tg('gestion.acceso.ver')}
            </Button>
          ) : null}
        </div>
        {acceso.estado === 'cargando' ? (
          <p class="text-text-muted flex items-center gap-2 text-sm" role="status">
            <Spinner size={16} /> {t('common.loading')}
          </p>
        ) : acceso.estado === 'error' ? (
          <p class="text-danger-text text-sm" role="alert">
            {tg('gestion.acceso.error')}
          </p>
        ) : acceso.estado === 'ok' ? (
          <AccesoDetalle datos={acceso.datos} t={t} tg={tg} locale={locale} />
        ) : null}
      </section>

      <section aria-labelledby={`sesiones-${userId}`} class="flex flex-col gap-2">
        <div class="flex items-center justify-between gap-2">
          <h3 id={`sesiones-${userId}`} class="text-text text-sm font-semibold">
            {sesiones.estado === 'ok'
              ? tg('gestion.sesiones.total', { count: sesiones.datos.length })
              : tg('gestion.sesiones.titulo')}
          </h3>
          {sesiones.estado === 'cerrado' || sesiones.estado === 'error' ? (
            <Button size="sm" variant="secondary" onClick={() => void verSesiones()}>
              {sesiones.estado === 'error' ? t('common.tryAgain') : tg('gestion.sesiones.ver')}
            </Button>
          ) : null}
        </div>
        {sesiones.estado === 'cargando' ? (
          <p class="text-text-muted flex items-center gap-2 text-sm" role="status">
            <Spinner size={16} /> {t('common.loading')}
          </p>
        ) : sesiones.estado === 'error' ? (
          <p class="text-danger-text text-sm" role="alert">
            {tg('gestion.sesiones.error')}
          </p>
        ) : sesiones.estado === 'ok' ? (
          sesiones.datos.length === 0 ? (
            <p class="text-text-muted text-sm">{tg('gestion.sesiones.vacia')}</p>
          ) : (
            <ul class="border-border divide-border divide-y rounded-md border">
              {sesiones.datos.map((s) => (
                <li key={s.id} class="flex flex-wrap items-center gap-x-3 gap-y-1 px-3 py-2">
                  <span class="min-w-0 flex-1">
                    <span class="text-text block truncate text-sm font-medium" title={s.user_agent}>
                      {s.device || s.user_agent}
                    </span>
                    <span class="sytue-meta block truncate">
                      {s.ip ? `IP ${s.ip} · ` : ''}
                      {tg('gestion.sesiones.inicio', { when: formatWhen(locale, s.auth_time) })}
                      {' · '}
                      {tg('gestion.sesiones.caduca', {
                        when: formatDateTime(locale, s.expires_at),
                      })}
                    </span>
                  </span>
                  {conSegundoFactor(s.amr) ? (
                    <Badge tone="success">{tg('gestion.sesiones.mfa')}</Badge>
                  ) : (
                    <Badge tone="warning">{tg('gestion.sesiones.sinMfa')}</Badge>
                  )}
                </li>
              ))}
            </ul>
          )
        ) : null}
      </section>
    </>
  );
}

function AccesoDetalle({
  datos,
  t,
  tg,
  locale,
}: {
  datos: AccesoEfectivo;
  t: T;
  tg: TGestion;
  locale: Locale;
}) {
  // Una fila por permiso, con la fuente más estable que lo da (Sytue ID elige:
  // rol integrado, luego rol personalizado, luego elevación temporal).
  const filas = datos.permissions;
  return (
    <div class="flex flex-col gap-3">
      <InlineHelp>{tg('gestion.acceso.ayuda')}</InlineHelp>
      {filas.length === 0 ? (
        <p class="text-text-muted text-sm">{tg('gestion.acceso.sinPermisos')}</p>
      ) : (
        <ul class="border-border divide-border divide-y rounded-md border">
          {filas.map((c) => (
            <li key={c.permission} class="flex flex-wrap items-center gap-2 px-3 py-2">
              <span class="text-text min-w-0 flex-1 text-sm font-medium">
                {permLabel(c.permission, t)}
              </span>
              <Badge tone={c.source === 'elevacion_temporal' ? 'warning' : 'neutral'}>
                {fuente(c, tg, locale)}
              </Badge>
            </li>
          ))}
        </ul>
      )}
      {datos.pending_elevations.length > 0 ? (
        <div>
          <p class="text-text mb-1 text-sm font-medium">{tg('gestion.acceso.pendientes')}</p>
          <ul class="flex flex-col gap-1">
            {datos.pending_elevations.map((p) => (
              <li key={p.id} class="text-text-muted text-sm">
                {tg('gestion.acceso.pendiente', {
                  role: p.role,
                  when: formatWhen(locale, p.requested_at),
                })}
                {p.justification ? ` — ${p.justification}` : ''}
              </li>
            ))}
          </ul>
        </div>
      ) : null}
      <div>
        <p class="text-text mb-1 text-sm font-medium">{tg('gestion.acceso.grupos')}</p>
        {datos.groups.length === 0 ? (
          <p class="text-text-muted text-sm">{tg('gestion.acceso.sinGrupos')}</p>
        ) : (
          <div class="flex flex-wrap gap-1">
            {datos.groups.map((g) => (
              <Badge key={g} tone="neutral">
                {g}
              </Badge>
            ))}
          </div>
        )}
      </div>
      <p class="sytue-meta">
        {tg('gestion.acceso.calculado', { when: formatDateTime(locale, datos.as_of) })}
      </p>
    </div>
  );
}
