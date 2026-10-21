import type { JSX } from 'preact';
import { Icon, Sidebar, SidebarItem, SidebarSection, type IconName } from '@sytue/ui';
import { ruta } from '../lib/rutas.js';

import type { ClaveMensaje } from '../i18n/index.js';

export type Seccion = 'sites' | 'new' | 'settings';

interface Props {
  t: (key: ClaveMensaje, params?: Record<string, string | number>) => string;
  activa: Seccion | null;
}

export const SECCIONES: readonly { clave: Seccion; camino: string; icono: IconName }[] = [
  { clave: 'sites', camino: '/', icono: 'globe' },
  { clave: 'new', camino: '/new', icono: 'plus' },
  { clave: 'settings', camino: '/settings', icono: 'settings' },
];

export function Nav({ t, activa }: Props): JSX.Element {
  return (
    <Sidebar>
      <SidebarSection title={t('nav.title')}>
        {SECCIONES.map((s) => (
          <SidebarItem
            key={s.clave}
            href={ruta(s.camino)}
            icon={<Icon name={s.icono} size={16} />}
            label={t(`nav.${s.clave}`)}
            active={activa === s.clave}
          />
        ))}
      </SidebarSection>
    </Sidebar>
  );
}
