/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * La lista viva de requisitos bajo «Contraseña nueva».
 *
 * Se ve desde el principio (con todo pendiente), no solo al fallar: la persona
 * sabe qué se le pide antes de inventarse nada, y no se entera por un 400.
 *
 * El estado de cada punto no va solo en el color: el icono cambia de forma
 * (círculo → marca) y un texto para lectores de pantalla dice si se cumple. La
 * lista NO es una región viva: anunciar cada tecla sería ruido; el campo la
 * enlaza con `aria-describedby`, que la lee al entrar en él.
 */
import type { JSX } from 'preact';
import { Glifo, Icon } from '@sytue/ui';
import type { T } from '../i18n/index.js';
import type { Requisito } from '../lib/requisitosPassword.js';

export function RequisitosPassword({
  t,
  id,
  requisitos,
  minimo,
}: {
  t: T;
  id: string;
  requisitos: Requisito[];
  minimo: number;
}): JSX.Element {
  return (
    <div id={id} class="flex flex-col gap-1.5">
      <p class="text-text-muted text-xs">{t('security.password.requirements')}</p>
      <ul class="flex flex-col gap-1">
        {requisitos.map((r) => (
          <li
            key={r.clave}
            class="flex items-start gap-2 text-xs"
            data-requisito={r.clave}
            data-cumplido={r.cumplido ? 'si' : 'no'}
          >
            {r.cumplido ? (
              <Icon name="check" size={14} class="text-success-text mt-0.5 shrink-0" />
            ) : (
              // Un círculo de verdad y no `circle` del catálogo, que es la elipse
              // de las herramientas de dibujo.
              <Glifo size={14} class="text-text-muted mt-0.5 shrink-0">
                <circle cx="12" cy="12" r="6" />
              </Glifo>
            )}
            <span>
              {r.clave === 'length'
                ? t('security.password.req_length', { count: minimo })
                : t(`security.password.req_${r.clave}`)}
              <span class="sr-only">
                {' '}
                {t(r.cumplido ? 'security.password.req_met' : 'security.password.req_pending')}
              </span>
            </span>
          </li>
        ))}
      </ul>
    </div>
  );
}

export default RequisitosPassword;
