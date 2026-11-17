/**
 * Filtros de estado del directorio de Usuarios.
 *
 * Son los mismos tres recuentos que la portada enseña como avisos («N personas no
 * tienen segundo factor»…), con el MISMO nombre con el que Sytue ID los cuenta en
 * /admin/insights y los filtra en /admin/users?filter=. El aviso enlaza a la lista
 * con su filtro puesto, y la lista tiene que enseñar exactamente a quien el aviso
 * cuenta: por eso el criterio (activas sin MFA, suspendidas, activas sin verificar)
 * es el del servicio y aquí solo se repite para el seed de demostración.
 *
 * El filtro lo resuelve el SERVIDOR: «quién no tiene segundo factor» mirado sobre
 * las cincuenta filas cargadas no es la respuesta, es una muestra.
 */
import type { DirectoryUser } from './types.js';

export const FILTROS_USUARIOS = ['without_mfa', 'suspended', 'unverified_email'] as const;

/** Un filtro de estado; '' es «todas las personas». */
export type FiltroUsuarios = (typeof FILTROS_USUARIOS)[number];

/** ¿Es `v` un filtro que el servicio entiende? Lo de fuera de la lista no se reenvía. */
export function esFiltroUsuarios(v: unknown): v is FiltroUsuarios {
  return typeof v === 'string' && (FILTROS_USUARIOS as readonly string[]).includes(v);
}

/** Lee el filtro de un parámetro de URL; lo que no se reconoce es «sin filtro». */
export function filtroDeParametro(v: string | null | undefined): FiltroUsuarios | '' {
  const f = v?.trim() ?? '';
  return esFiltroUsuarios(f) ? f : '';
}

/**
 * El criterio de cada filtro sobre una fila ya cargada. Solo para el seed (sin
 * Sytue ID): con backend vivo filtra el servidor, que es quien ve a todo el mundo.
 */
export function cumpleFiltro(u: DirectoryUser, f: FiltroUsuarios | ''): boolean {
  switch (f) {
    case 'without_mfa':
      return u.enabled && !u.mfaEnabled;
    case 'suspended':
      return !u.enabled;
    case 'unverified_email':
      return u.enabled && !u.emailVerified;
    default:
      return true;
  }
}
