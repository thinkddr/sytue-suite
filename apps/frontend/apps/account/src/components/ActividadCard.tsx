/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * «Actividad reciente»: los inicios de sesión y los cambios de seguridad de la
 * cuenta, con su fecha y la dirección desde la que se hicieron.
 *
 * Es la pantalla a la que lleva un aviso de seguridad («se ha cambiado la
 * contraseña de tu cuenta»): quien lo recibe quiere saber CUÁNDO y DESDE DÓNDE, y
 * hasta ahora la cuenta no se lo podía decir. Lo sirve Sytue ID desde su log de
 * auditoría (`GET /api/v1/me/activity`), ya filtrado a lo que es de la cuenta y con
 * una clase pública por suceso; aquí solo se traduce y se pinta.
 *
 * Diez a la vista y el resto tras «Ver todo»: lo que se viene a mirar es lo
 * último, y cincuenta filas empujarían el resto de Mi cuenta muy abajo. Y debajo,
 * qué hacer si algo no cuadra, con enlaces a las dos secciones que lo arreglan.
 */
import type { JSX } from 'preact';
import { useEffect, useState } from 'preact/hooks';
import { Button, Card, EmptyState } from '@sytue/ui';
import { formatDateTime, formatTimeAgo, type Locale } from '@sytue/i18n';
import type { AccountMessageKey, T } from '../i18n/index.js';
import { FilasCargando } from './FilasCargando.js';

export interface Suceso {
  id: number;
  kind: string;
  at: string;
  ip?: string;
}

/** La clase pública de Sytue ID → su rótulo. Una clase nueva que aún no esté aquí
 *  sale como «Cambio en la cuenta» en vez de desaparecer. */
const ROTULO: Record<string, AccountMessageKey> = {
  signin: 'activity.kind.signin',
  signin_locked: 'activity.kind.signin_locked',
  password_changed: 'activity.kind.password_changed',
  mfa_enabled: 'activity.kind.mfa_enabled',
  mfa_disabled: 'activity.kind.mfa_disabled',
  recovery_codes: 'activity.kind.recovery_codes',
  passkey_added: 'activity.kind.passkey_added',
  passkey_removed: 'activity.kind.passkey_removed',
  passkey_renamed: 'activity.kind.passkey_renamed',
  app_password_created: 'activity.kind.app_password_created',
  app_password_revoked: 'activity.kind.app_password_revoked',
  sessions_closed: 'activity.kind.sessions_closed',
  device_trusted: 'activity.kind.device_trusted',
  device_removed: 'activity.kind.device_removed',
  recovery_changed: 'activity.kind.recovery_changed',
  username_changed: 'activity.kind.username_changed',
};

export function rotuloDeSuceso(kind: string): AccountMessageKey {
  return ROTULO[kind] ?? 'activity.kind.other';
}

/** Lo que se ve antes de «Ver todo». */
export const A_LA_VISTA = 10;
/** La ventana del servicio, por si una respuesta antigua no la trae. */
const DIAS_POR_DEFECTO = 90;

export function ActividadCard({ t, locale }: { t: T; locale: Locale }): JSX.Element {
  const [sucesos, setSucesos] = useState<Suceso[] | null>(null);
  const [dias, setDias] = useState(DIAS_POR_DEFECTO);
  const [error, setError] = useState(false);
  const [todo, setTodo] = useState(false);

  async function cargar(): Promise<void> {
    setError(false);
    try {
      const res = await fetch('/api/activity');
      if (!res.ok) throw new Error(String(res.status));
      const datos = (await res.json()) as { events?: Suceso[]; days?: number };
      setSucesos(datos.events ?? []);
      if (typeof datos.days === 'number' && datos.days > 0) setDias(datos.days);
    } catch {
      setError(true);
    }
  }

  useEffect(() => {
    void cargar();
  }, []);

  const visibles = (sucesos ?? []).slice(0, todo ? undefined : A_LA_VISTA);
  const hayMas = !todo && (sucesos?.length ?? 0) > A_LA_VISTA;

  return (
    <Card id="actividad">
      <div class="flex flex-col gap-4 p-4 sm:p-5">
        <h2 class="text-lg font-semibold">{t('activity.title')}</h2>
        <p class="text-text-muted -mt-2 text-xs">{t('activity.hint', { days: dias })}</p>

        {error && (
          <div role="alert" class="flex items-center justify-between gap-3">
            <p class="text-danger-text text-sm">{t('activity.failed')}</p>
            <Button variant="ghost" onClick={() => void cargar()}>
              {t('common.retry')}
            </Button>
          </div>
        )}
        {!error && sucesos === null && <FilasCargando etiqueta={t('common.loading')} />}
        {!error && sucesos?.length === 0 && (
          <EmptyState title={t('activity.empty', { days: dias })} />
        )}

        {visibles.length > 0 && (
          <ul class="flex flex-col gap-3">
            {visibles.map((s) => {
              const cuando = new Date(s.at);
              const valida = !Number.isNaN(cuando.getTime());
              return (
                <li key={s.id} class="flex flex-col">
                  <p class="text-sm font-medium">{t(rotuloDeSuceso(s.kind))}</p>
                  {/* La fecha exacta en el `title`: «hace 3 días» basta para
                      reconocerlo, y la exacta es la que se compara con un aviso. */}
                  <p
                    class="text-text-muted text-xs"
                    title={
                      valida
                        ? formatDateTime(locale, cuando, { dateStyle: 'full', timeStyle: 'short' })
                        : undefined
                    }
                  >
                    {[valida ? formatTimeAgo(locale, cuando) : '', s.ip ?? '']
                      .filter(Boolean)
                      .join(' · ')}
                  </p>
                </li>
              );
            })}
          </ul>
        )}
        {hayMas && (
          <div>
            <Button variant="ghost" onClick={() => setTodo(true)}>
              {t('activity.more')}
            </Button>
          </div>
        )}

        {!error && (sucesos?.length ?? 0) > 0 && (
          <div class="border-border flex flex-col gap-2 border-t pt-3">
            <p class="text-text-muted text-xs">{t('activity.not_you')}</p>
            {/* El mismo botón-enlace que la revisión de seguridad: lleva al ancla
                pública de la sección, que el índice resuelve y enfoca. */}
            <div class="flex flex-wrap gap-2">
              <Button variant="secondary" size="sm" href="#seguridad">
                {t('activity.change_password')}
              </Button>
              <Button variant="secondary" size="sm" href="#sesiones">
                {t('activity.see_sessions')}
              </Button>
            </div>
          </div>
        )}
      </div>
    </Card>
  );
}

export default ActividadCard;
