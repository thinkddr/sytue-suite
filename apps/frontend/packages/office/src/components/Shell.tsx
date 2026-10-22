/**
 * Shell — isla raiz de Sytue Docs. Envuelve <SytueShell> inyectando el proveedor
 * de access token del cliente. Recibe solo props serializables desde el SSR.
 */
import type { ComponentChildren } from 'preact';
import { SytueShell } from '@sytue/shell';
import type { SytueUser } from '@sytue/auth';
import type { Locale } from '@sytue/i18n';
import { getAccessToken } from '@sytue/auth/client';
import type { ProductoDocs } from '../lib/producto';

interface ShellProps {
  user: SytueUser;
  hubBaseUrl: string;
  /** Idioma negociado en el servidor (middleware). Sin él la shell hidrataría en inglés. */
  locale: Locale;
  /** Docus, Calcus, Diapos o CertiPDF: nombre e icono del selector de apps. */
  producto?: ProductoDocs;
  children?: ComponentChildren;
}

export function Shell({ user, hubBaseUrl, locale, producto = 'docs', children }: ShellProps) {
  return (
    <SytueShell
      currentApp={producto}
      user={user}
      hubBaseUrl={hubBaseUrl}
      accessToken={getAccessToken}
      locale={locale}
    >
      {/* Landmark main + destino del skip-link de la shell. En la forma «Carril» los dos
          envoltorios son display:contents y no alteran el layout de la página (lista o
          editor a viewport). En «Marco» son el marco y la hoja, aquí A SANGRE: un editor
          no cede ni un píxel de lienzo, y la hoja solo conserva las esquinas de arriba
          (tokens/appearance.css). */}
      <main
        id="contenido"
        tabIndex={-1}
        class="sytue-frame sytue-frame-page sytue-safe-workspace focus-visible:outline-none"
      >
        {/* El main ya reserva la zona segura inferior: la hoja no debe volver a
            sumarla a su mínimo y crear un segundo scroll en el documento. */}
        <div
          class="sytue-sheet sytue-sheet-page sytue-sheet-bleed"
          style={{
            minHeight: 'calc(100dvh - var(--spacing-topbar) - var(--sytue-safe-bottom, 0px))',
          }}
        >
          {children}
        </div>
      </main>
    </SytueShell>
  );
}
