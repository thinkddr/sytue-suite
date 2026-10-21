/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * La navegación lateral de Sytue Cast: Inicio, Explorar, Emitir y Subir
 * (CAST-ENCARGO §10). En el móvil es el cajón del AppShell.
 */
import type { JSX } from 'preact';
import { useEffect, useState } from 'preact/hooks';
import { Icon, Sidebar, SidebarItem, SidebarSection, type IconName } from '@sytue/ui';
import { resumenDeModeracion } from '../client/castApi.js';
import type { T } from '../i18n/index.js';
import { ruta, rutaPublica } from '../lib/rutas.js';

export type Seccion = 'home' | 'explore' | 'studio' | 'upload' | 'moderation';

const SECCIONES: readonly { clave: Seccion; camino: string; icono: IconName }[] = [
  { clave: 'home', camino: '/', icono: 'home' },
  { clave: 'explore', camino: '/explore', icono: 'explorar' },
  { clave: 'studio', camino: '/studio', icono: 'emitir' },
  { clave: 'upload', camino: '/upload', icono: 'upload' },
  { clave: 'moderation', camino: '/moderation', icono: 'shield' },
];

export function Nav({
  t,
  activa,
  administra,
}: {
  t: T;
  activa: Seccion | null;
  /** Quien administra la organización ve cuántas denuncias tiene sin resolver. */
  administra: boolean;
}): JSX.Element {
  const [abiertas, setAbiertas] = useState(0);
  useEffect(() => {
    if (!administra) return;
    // Además de contar, esta llamada apunta a quien pregunta como moderador: es a
    // quien el servicio avisa de las denuncias nuevas.
    resumenDeModeracion()
      .then((r) => setAbiertas(r.open))
      .catch(() => undefined);
  }, [administra]);
  return (
    <Sidebar>
      <SidebarSection>
        {SECCIONES.map((s) => (
          <SidebarItem
            key={s.clave}
            // «Explorar» es pública y sin org: el enlace sale sin ella.
            href={s.clave === 'explore' ? rutaPublica(s.camino) : ruta(s.camino)}
            icon={<Icon name={s.icono} size={16} />}
            label={t(`nav.${s.clave}`)}
            active={activa === s.clave}
            {...(s.clave === 'moderation' && abiertas > 0 ? { count: abiertas } : {})}
          />
        ))}
      </SidebarSection>
    </Sidebar>
  );
}
