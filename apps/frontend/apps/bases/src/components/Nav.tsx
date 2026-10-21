import type { JSX } from 'preact';
import { Icon, Sidebar, SidebarItem, SidebarSection } from '@sytue/ui';
import { ruta } from '../lib/rutas.js';
import type { T } from '../i18n/index.js';

export type Seccion = 'bases';

interface Props {
  t: T;
  activa: Seccion | null;
}

export function Nav({ t, activa }: Props): JSX.Element {
  return (
    <Sidebar>
      <SidebarSection title={t('nav.title')}>
        <SidebarItem
          href={ruta('/')}
          icon={<Icon name="table" size={16} />}
          label={t('nav.bases')}
          active={activa === 'bases'}
        />
      </SidebarSection>
    </Sidebar>
  );
}
