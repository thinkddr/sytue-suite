// SPDX-License-Identifier: Apache-2.0
// Banco sin servidor de la APP de Calendar entera (CalendarApp real) para el mes del
// teléfono. Lo compila test/e2e/compilar-mes-movil.mjs y lo conduce test/e2e/mes-movil.mjs,
// que contesta la API (/api/calendars, /api/events…) desde Playwright y fija el reloj.
// El banco de componentes (mes-movil.tsx) no ve la INTEGRACIÓN —que la cabecera y el
// botón flotante no se pinten, que AppShell reciba la barra y el aviso, que `navigate`
// recorte el mes y dé el paso de la semana plegada—: esto sí.
//
// El idioma sale de `?l=`. Justo después del primer render —antes de los efectos, que es
// lo que ve el usuario mientras `useEstrecha` resuelve— apunta en `__primerPintado` qué
// forma de escritorio había a la vista.
import { render } from 'preact';
import { isLocale } from '@sytue/i18n';
import type { SytueUser } from '@sytue/auth';
import { CalendarApp } from '../src/components/CalendarApp.js';

const pedido = new URLSearchParams(location.search).get('l');
const locale = isLocale(pedido) ? pedido : 'es';
const user: SytueUser = {
  id: 'u1',
  email: 'ana@sytue.test',
  emailVerified: true,
  name: 'Ana',
  preferredUsername: 'ana',
  roles: [],
  groups: [],
};

/** ¿Se ve? `display: none` en él o en un antepasado. */
function aLaVista(el: Element | null): boolean {
  for (let e = el; e; e = e.parentElement) {
    if (getComputedStyle(e).display === 'none') return false;
  }
  return el !== null;
}

const raiz = document.createElement('div');
raiz.setAttribute('data-banco-app', '');
document.body.appendChild(raiz);
render(<CalendarApp locale={locale} hubBaseUrl="/hub" user={user} />, raiz);
(window as unknown as { __primerPintado: Record<string, boolean> }).__primerPintado = {
  cabecera: aLaVista(document.querySelector('header[data-scroll-x]')),
  esqueletoMes: aLaVista(document.querySelector('[role="status"][aria-busy="true"]')),
  fab: aLaVista(document.querySelector('button.fixed')),
};
