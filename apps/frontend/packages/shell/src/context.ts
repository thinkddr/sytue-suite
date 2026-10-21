/**
 * Contexto de la Sytue Shell: configuracion (que app es la actual, URL del Hub)
 * y cliente del Hub compartido por busqueda/notificaciones/presencia.
 */
import { createContext } from 'preact';
import { useContext } from 'preact/hooks';
import type { HubClient } from '@sytue/api-clients';
import type { SytueUser } from '@sytue/auth';
import type { ProductKey } from '@sytue/config';

export interface ShellConfig {
  /** Clave de la app actual (resalta su icono en el app switcher). */
  currentApp: ProductKey;
  /** Cliente del Hub (busqueda/notificaciones/presencia). */
  hub: HubClient;
  /** Base del Hub. Va aparte del cliente porque el conmutador pregunta por
   *  `fetch` directo qué productos levanta la instalación, y el cliente no
   *  expone su baseUrl. */
  hubBaseUrl: string;
  /** Usuario autenticado (de /auth/session). */
  user: SytueUser;
  /** Token de acceso (o getter). Lo usa el conmutador para preguntar a la pasarela
   *  quién sirve cada producto en esta organización. */
  accessToken: string | (() => string | Promise<string>);
  /** Pinta la foto nueva en la barra sin recargar (ver `fotoConVersion`). */
  cambiarFoto?: (version: string) => void;
}

export const ShellContext = createContext<ShellConfig | null>(null);

export function useShell(): ShellConfig {
  const ctx = useContext(ShellContext);
  if (ctx === null) {
    throw new Error('Los componentes de @sytue/shell deben montarse dentro de <SytueShell>.');
  }
  return ctx;
}
