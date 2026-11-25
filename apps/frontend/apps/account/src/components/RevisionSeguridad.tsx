/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * «Revisión de seguridad»: lo primero de Mi cuenta. Resume en una tarjeta lo que
 * conviene arreglar (sin segundo factor, sin forma de recuperar la cuenta,
 * sesiones en otros equipos, credenciales olvidadas) y lleva con un enlace a la
 * sección que lo resuelve. Si todo está bien, una sola línea que lo dice.
 *
 * El criterio es `revisionDeCuenta` de `@sytue/shell` —el mismo que usarán el
 * portal y el Hub—; aquí solo se trae y se pinta. Las lecturas son las PRIMERAS
 * de cada recurso, compartidas con las tarjetas de abajo (`primeraLectura`): la
 * revisión no añade ni una petición a la página.
 */
import type { JSX } from 'preact';
import { useEffect, useState } from 'preact/hooks';
import { Button, Card, Icon, marcarAppLista } from '@sytue/ui';
import { revisionDeCuenta, type ClaveRevision, type PuntoRevision } from '@sytue/shell';
import type { AccountMessageKey, T } from '../i18n/index.js';
import { primeraLectura } from '../lib/primeraLectura.js';
import { FilasCargando } from './FilasCargando.js';

/** El cuerpo JSON de una lectura, o `null` si no llegó: la revisión lo omite. */
async function leer<V>(url: string, campo?: string): Promise<V | null> {
  try {
    const res = await primeraLectura(url);
    if (!res.ok) return null;
    const cuerpo = (await res.json()) as Record<string, unknown>;
    return (campo ? (cuerpo[campo] ?? []) : cuerpo) as V;
  } catch {
    return null;
  }
}

const ACCION: Record<ClaveRevision, AccountMessageKey> = {
  dos_pasos: 'checkup.turn_on',
  recuperacion: 'checkup.add',
  sesiones: 'checkup.review',
  contrasenas_app: 'checkup.review',
  dispositivos: 'checkup.review',
};

export function RevisionSeguridad({ t }: { t: T }): JSX.Element | null {
  const [puntos, setPuntos] = useState<PuntoRevision[] | null>(null);

  useEffect(() => {
    let vivo = true;
    void (async () => {
      const [me, passkeys, sesiones, dispositivos, contrasenasApp] = await Promise.all([
        leer<{ mfa_enabled?: boolean; recovery_email?: string; phone?: string }>('/api/me'),
        leer<unknown[]>('/api/passkeys', 'credentials'),
        leer<{ current?: boolean }[]>('/api/sessions', 'sessions'),
        leer<{ trusted?: boolean; current?: boolean; last_seen?: string }[]>(
          '/api/devices',
          'devices',
        ),
        leer<{ created_at?: string; last_used_at?: string | null }[]>(
          '/api/app-passwords',
          'app_passwords',
        ),
      ]);
      if (vivo) {
        setPuntos(
          revisionDeCuenta({
            me,
            passkeys,
            sesiones,
            dispositivos,
            contrasenasApp,
            ahora: new Date(),
          }),
        );
      }
    })();
    return () => {
      vivo = false;
    };
  }, []);

  // `sytue:app-ready`: la revisión, lo primero de la página y lo que lee de cinco
  // sitios, ya está pintada. Lo que se presupuesta en vez del LCP.
  useEffect(() => {
    if (puntos) marcarAppLista();
  }, [puntos]);

  // Si no ha llegado nada no hay revisión que hacer: una tarjeta vacía, o «todo
  // bien» sin haber mirado, sería peor que no enseñarla.
  if (puntos && puntos.length === 0) return null;
  const avisos = puntos?.filter((p) => p.estado === 'aviso') ?? [];

  return (
    <Card id="revision" aria-labelledby="revision-titulo">
      <div class="flex flex-col gap-3 p-4 sm:p-5">
        <h2 id="revision-titulo" class="text-lg font-semibold">
          {t('checkup.title')}
        </h2>
        {!puntos ? (
          <FilasCargando etiqueta={t('common.loading')} filas={1} accion="boton" />
        ) : avisos.length === 0 ? (
          <p class="text-success-text flex items-center gap-2 text-sm" data-revision="ok">
            <Icon name="shield" size={16} />
            {t('checkup.all_good')}
          </p>
        ) : (
          <ul class="flex flex-col gap-3">
            {avisos.map((p) => (
              <li
                key={p.clave}
                class="flex items-center justify-between gap-3"
                data-revision={p.clave}
              >
                <span class="flex min-w-0 items-start gap-2 text-sm">
                  <Icon name="warning" size={16} class="text-warning-text mt-0.5 shrink-0" />
                  <span>{t(`checkup.${p.clave}` as AccountMessageKey, { count: p.cuantos })}</span>
                </span>
                <Button variant="secondary" size="sm" href={p.ancla} class="shrink-0">
                  {t(ACCION[p.clave])}
                </Button>
              </li>
            ))}
          </ul>
        )}
      </div>
    </Card>
  );
}

export default RevisionSeguridad;
