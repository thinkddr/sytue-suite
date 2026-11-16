/**
 * Lógica pura de la paleta de órdenes (⌘K) de Sytue Admin: qué se ofrece y adónde
 * lleva cada cosa. Aislada del componente para probarla sin DOM.
 *
 * Tres grupos: SECCIONES (las del menú, ya filtradas por `allowedSections`, así un
 * admin delegado no ve puertas que le van a dar 403), ACCIONES (las cuatro que más
 * se buscan, solo si su sección está permitida) y PERSONAS (la búsqueda del
 * directorio, la misma de los selectores de grupos y roles).
 *
 * Las acciones no abren el diálogo desde aquí: navegan a su sección con
 * `?accion=<nombre>` y el panel lo abre al montar (`accionPedida`). Así funcionan
 * igual desde cualquier página, y la URL no se queda con la marca.
 */
import type { IconName } from '@sytue/ui';

export type AccionDePaleta = 'invitar' | 'crear-grupo' | 'conectar-app' | 'conectar-sso';

/** Una entrada en el formato de `filterCommands` de @sytue/ui. */
export interface ComandoDeConsola {
  id: string;
  group: string;
  label: string;
  icon: IconName;
  keywords?: string;
  hint?: string;
  /** Ruta DENTRO de la app (sin prefijo ni org: los pone quien navega). */
  href: string;
}

export interface SeccionDeMenu {
  id: string;
  label: string;
  icon: IconName;
  href: string;
}

/** Qué sección necesita cada acción, adónde lleva y con qué icono. */
export const ACCIONES: readonly {
  accion: AccionDePaleta;
  seccion: string;
  /** Ruta interna; se prefija al navegar (`ruta()` de ConsoleShell), como NAV. */
  destino: string;
  icon: IconName;
}[] = [
  { accion: 'invitar', seccion: 'users', destino: '/users', icon: 'plus' },
  { accion: 'crear-grupo', seccion: 'groups', destino: '/groups', icon: 'users' },
  { accion: 'conectar-app', seccion: 'apps', destino: '/apps', icon: 'globe' },
  { accion: 'conectar-sso', seccion: 'sso', destino: '/sso', icon: 'key' },
];

export interface Textos {
  grupoSecciones: string;
  grupoAcciones: string;
  grupoPersonas: string;
  accion: (a: AccionDePaleta) => { label: string; keywords: string };
}

/**
 * Los comandos que ve ESTE usuario. `allowed` ausente = todas (owner/admin), el
 * mismo criterio que el menú lateral.
 */
export function comandosDeConsola(
  nav: readonly SeccionDeMenu[],
  allowed: readonly string[] | undefined,
  textos: Textos,
  personas: readonly { id: string; fullName: string; email: string }[] = [],
): ComandoDeConsola[] {
  const puede = (id: string) => allowed === undefined || allowed.includes(id);
  const secciones = nav
    .filter((n) => puede(n.id))
    .map((n) => ({
      id: `seccion:${n.id}`,
      group: textos.grupoSecciones,
      label: n.label,
      icon: n.icon,
      href: n.href,
    }));
  const acciones = ACCIONES.filter((a) => puede(a.seccion)).map((a) => ({
    id: `accion:${a.accion}`,
    group: textos.grupoAcciones,
    icon: a.icon,
    ...textos.accion(a.accion),
    href: `${a.destino}?accion=${a.accion}`,
  }));
  const gente = puede('users')
    ? personas.map((p) => ({
        id: `persona:${p.id}`,
        group: textos.grupoPersonas,
        label: p.fullName || p.email,
        icon: 'user' as IconName,
        hint: p.email,
        keywords: p.email,
        href: `/users?q=${encodeURIComponent(p.email)}`,
      }))
    : [];
  return [...secciones, ...acciones, ...gente];
}

/**
 * ¿Ha llegado esta página pidiendo `accion`? Si sí, quita la marca de la barra
 * (recargar no debe volver a abrir el diálogo) y devuelve true.
 */
export function accionPedida(accion: AccionDePaleta): boolean {
  if (typeof window === 'undefined') return false;
  const url = new URL(window.location.href);
  if (url.searchParams.get('accion') !== accion) return false;
  url.searchParams.delete('accion');
  window.history.replaceState(null, '', url.pathname + url.search + url.hash);
  return true;
}

/** Búsqueda inicial pedida por la paleta (`?q=`), para el directorio. */
export function consultaPedida(): string {
  if (typeof window === 'undefined') return '';
  return new URLSearchParams(window.location.search).get('q') ?? '';
}
