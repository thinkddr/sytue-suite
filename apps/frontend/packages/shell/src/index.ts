/**
 * @sytue/shell — la Sytue Shell: topbar global, app switcher (9 puntos),
 * busqueda global (Sytue Hub), centro de notificaciones (SSE), presencia y menu
 * de cuenta (logout SSO). Se monta en CADA app de la suite para una experiencia
 * unica. docs/CONVENTIONS.md §11.
 */
export { SytueShell } from './SytueShell.js';
export type { SytueShellProps } from './SytueShell.js';
export { GlobalSearch, detalleDe } from './GlobalSearch.js';
export { AppSwitcher } from './AppSwitcher.js';
export { NotificationCenter } from './NotificationCenter.js';
// Los avisos del Hub sin Preact: la campana y la bandeja de One los comparten.
export {
  abrirAviso,
  alLlegarAviso,
  conectarAvisos,
  cuerpoAviso,
  destinoAviso,
  EVENTO_AVISO,
  nombreDeOrganizacion,
  procedenciaAviso,
  tituloAviso,
} from './avisos.js';
// La franja de solo lectura con su «Pedir acceso de edición» (platform/pkg/peticiones).
export { FranjaPeticion } from './FranjaPeticion.js';
export type { FranjaPeticionProps } from './FranjaPeticion.js';
// «Compartir con una persona» (el BFF es `rutasDeCompartir` de @sytue/auth/server).
export { CompartirConPersonas } from './CompartirConPersonas.js';
export type { CompartirConPersonasProps, NivelCompartido } from './CompartirConPersonas.js';
export type { Avisos, EstadoAvisos, FlujoAvisos, FuenteAvisos } from './avisos.js';
export { AccountMenu } from './AccountMenu.js';
export { ShellContext, useShell } from './context.js';
export { useAppsDesplegadas, appsRecordadas, recordarApps } from './useAppsDesplegadas.js';
export type { ShellConfig } from './context.js';

export { openInApp, deepLinkFor, inNativeShell } from './openInApp.js';

// «Traer mis datos»: el asistente de importacion DENTRO de cada producto, no solo en
// la consola de administracion.
export { ImportData } from './ImportData.js';
export type { ImportDataProps } from './ImportData.js';
export { LinkedServices } from './LinkedServices.js';
export type { LinkedServicesProps } from './LinkedServices.js';

export { useProductMap } from './useProductMap.js';
export type { ProductMap, SubstituteTarget } from './useProductMap.js';

export { SelectorDeFicheros } from './SelectorDeFicheros.js';
export type {
  SelectorDeFicherosProps,
  SelectorDeFicherosTextos,
  PickerItem,
} from './SelectorDeFicheros.js';
