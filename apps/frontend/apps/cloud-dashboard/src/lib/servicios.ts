/**
 * Lo que la isla del Resumen necesita de los servicios, sin nada de servidor.
 *
 * Vivía en `cloud.ts`, que importa el BFF (`auth.ts`) al cargarse: la isla
 * `client:load` lo arrastraba al bundle del navegador, el BFF pedía
 * SYTUE_OIDC_ISSUER al arrancar y la isla no llegaba a hidratarse
 * (Redesplegar y Reiniciar no hacían nada).
 */
import type { Deployment, ManagedService } from './types.js';

/**
 * Servicios que el despliegue EN MARCHA no puede usar todavía.
 *
 * Las credenciales de un servicio gestionado (DATABASE_URL, S3_*…) se resuelven
 * en el momento de desplegar y se inyectan en el pod. Así que crear una base de
 * datos con la app ya corriendo la deja aprovisionada y a la vez inalcanzable: el
 * panel dice «lista», el proyecto la tiene, y la app sigue sin la variable hasta
 * el siguiente despliegue. Hasta ahora eso no se contaba en ninguna parte, y es
 * de los fallos más desconcertantes que hay —todo parece correcto y nada funciona.
 *
 * Se compara contra `updatedAt` y no contra `createdAt` del despliegue: promover
 * o reaplicar la producción vuelve a resolver credenciales, así que usar la fecha
 * de creación avisaría de más justo después de un rollback.
 *
 * Función pura y exportada para poder probar el criterio sin montar la pantalla.
 */
export function servicesNotYetLive(
  services: ManagedService[],
  current: Deployment | undefined,
): ManagedService[] {
  // Sin despliegue vivo no hay nada que avisar: el aviso es «redespliega», y no
  // hay nada que redesplegar.
  if (!current) return [];
  const seen = Date.parse(current.updatedAt ?? current.createdAt);
  if (Number.isNaN(seen)) return [];
  return services.filter((s) => {
    // Un servicio aún aprovisionándose no es el caso: todavía no tiene
    // credenciales que inyectar, así que avisar sería adelantarse.
    if (s.status !== 'running' || !s.createdAt) return false;
    const born = Date.parse(s.createdAt);
    return !Number.isNaN(born) && born > seen;
  });
}
