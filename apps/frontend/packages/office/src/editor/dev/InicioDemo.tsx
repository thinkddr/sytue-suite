/** Inicio real bajo la misma Shell que las vistas autenticadas, sin sesión de servidor. */
import type { DocKind } from '@sytue/api-clients';
import { MatrixStorageFixture } from '@sytue/shell/matrix/fixture';
import type { Locale } from '@sytue/i18n';
import { Shell } from '../../components/Shell';
import { DocList } from '../../components/DocList';
import { PortadaPdf } from '../../pdf/Portada';
import { productoDeKind } from '../../lib/producto';

export function InicioDemo({ kind, locale }: { kind: DocKind | 'pdf'; locale: Locale }) {
  // Sin sesión no hay homeserver: la puerta de almacenamiento privado de la Shell es la
  // real, con un dispositivo ya verificado y sintético (solo bancos de desarrollo).
  return (
    <MatrixStorageFixture>
      <Shell
        user={{
          id: 'demo-user',
          email: 'demo@sytue.test',
          emailVerified: true,
          name: 'Demo',
          preferredUsername: 'demo',
          roles: [],
          groups: [],
          tenant: 'prueba',
        }}
        hubBaseUrl={window.location.origin}
        locale={locale}
        producto={kind === 'pdf' ? 'pdf' : productoDeKind(kind)}
      >
        {kind === 'pdf' ? (
          <div class="h-[calc(100dvh-var(--spacing-topbar)-var(--sytue-sheet-gap,0px)-var(--sytue-safe-bottom,0px))] min-h-0">
            <PortadaPdf locale={locale} />
          </div>
        ) : (
          <DocList locale={locale} userId="demo-user" kind={kind} />
        )}
      </Shell>
    </MatrixStorageFixture>
  );
}
