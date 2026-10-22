/*
 * Carga en servidor de las credenciales con las que Sytue se presenta ante cada
 * proveedor de terceros.
 *
 * Habla con la superficie de OPERADOR de Sytue Integrations, que va con un token
 * propio del despliegue y nunca baja al navegador. Quién puede verlo lo decide
 * antes la página, preguntándole a Sytue ID si es admin de plataforma; aquí solo
 * se trae el dato.
 */
import { serverEnv } from '@sytue/auth/server';
import type { ConnectorTemplate, PlatformOAuthClient } from '../components/ConnectionsPanel';

export interface ConnectionsData {
  clients: PlatformOAuthClient[];
  templates: ConnectorTemplate[];
  /** Estado HTTP con el que falló el CATÁLOGO, si falló. La pantalla lo dice en
   *  vez de dejar «añadir proveedor» desactivado sin motivo. */
  catalogoRoto?: number;
  /** Motivo por el que no hay datos (5xx del servicio, 403 sin permiso…). */
  problema?: number;
}

/**
 * Entorno del servidor, tipado como cadenas. `import.meta.env` de Astro es
 * `unknown` para TypeScript, y dejarlo así obliga a un `String(...)` por lectura
 * que además convertiría un objeto en «[object Object]» sin avisar.
 */
export type Entorno = Record<string, string | undefined>;

/**
 * `serverEnv` SIEMPRE, y aquí dentro en vez de en quien llama.
 *
 * En el servidor SSR COMPILADO —o sea, en producción— `import.meta.env` quedó
 * congelado en el build: no contiene NADA de lo que el contenedor recibe por
 * entorno. La única fuente viva es `process.env`, y `serverEnv` funde las dos.
 *
 * Esto costó una pantalla inalcanzable: el token de operador estaba bien puesto
 * en el chart y bien montado en el pod, pero aquí se leía vacío, así que la
 * pantalla concluía que esta superficie no gestionaba las conexiones y ofrecía un
 * enlace… a sí misma. Se hace en el helper y no en la página para que ningún
 * llamador futuro pueda volver a equivocarse.
 */
function vivo(env: Entorno): Entorno {
  return serverEnv(env);
}

/** Origen público del servicio de integraciones (para componer el redirect_uri). */
export function integrationsPublicUrl(buildEnv: Entorno): string {
  const env = vivo(buildEnv);
  const apex = env.SYTUE_SUITE_APEX ?? 'sytue.eu';
  return env.SYTUE_INTEGRATIONS_PUBLIC_URL ?? `https://integrations.${apex}`;
}

function upstream(buildEnv: Entorno): { base: string; token: string } {
  const env = vivo(buildEnv);
  return {
    base: env.SYTUE_INTEGRATIONS_INTERNAL_URL ?? 'http://sytue-one-integrations',
    token: env.SYTUE_INTEGRATIONS_OPERATOR_TOKEN ?? '',
  };
}

/**
 * Trae credenciales y catálogo. Las dos a la vez porque la pantalla necesita
 * ambas para poder ofrecer «añadir proveedor»: sin el catálogo solo se podrían
 * editar las que ya existen.
 *
 * Un fallo NO se propaga como excepción: devuelve `problema` para que la vista
 * explique qué pasa. Una consola de plataforma que revienta con un 500 cuando el
 * servicio de integraciones está caído deja al operador sin saber si el problema
 * es suyo o del despliegue.
 */
export async function loadConnections(env: Entorno): Promise<ConnectionsData> {
  const { base, token } = upstream(env);
  // SIN TOKEN SÍ ES UNA AVERÍA, y hay que decirlo así.
  //
  // Hubo una versión que lo trataba como «esta superficie no gestiona esto» y
  // ofrecía un enlace a la superficie de plataforma. Era falso: esta página
  // devuelve 404 en cualquier superficie que no sea la de plataforma
  // (`isPlatformSurface`), así que si se está leyendo esto YA se está en el sitio
  // correcto, y aquel enlace apuntaba a la propia página. Un bucle del que no se
  // salía nunca.
  //
  // 503 y no 403: el despliegue no le ha dado la credencial, no es que a quien
  // mira le falte permiso. El texto ya dice exactamente eso.
  if (!token) return { clients: [], templates: [], problema: 503 };

  const pedir = async (ruta: string): Promise<{ ok: boolean; status: number; data: unknown }> => {
    try {
      const res = await fetch(`${base}/operator/${ruta}`, {
        headers: { authorization: `Bearer ${token}` },
      });
      const data: unknown = await res.json().catch(() => ({}));
      return { ok: res.ok, status: res.status, data };
    } catch {
      // El servicio no responde. 503 y no 500: es indisponibilidad, y la vista
      // dice algo distinto para cada caso.
      return { ok: false, status: 503, data: {} };
    }
  };

  const [cred, cat] = await Promise.all([pedir('oauth-clients'), pedir('templates')]);
  if (!cred.ok) return { clients: [], templates: [], problema: cred.status };

  const clients = ((cred.data as { clients?: PlatformOAuthClient[] }).clients ?? []).slice();
  // El catálogo es opcional PARA LA PANTALLA —con él caído se pueden seguir
  // editando las credenciales existentes—, pero su fallo tiene que VERSE: sin
  // plantillas, «añadir proveedor» sale desactivado, y un botón gris no dice si
  // es que ya están todos configurados o que el servicio devolvió un 500.
  //
  // Eso es lo que pasó en producción: `/operator/templates` respondía 500
  // («no hay tenant en el contexto») y la pantalla no lo contaba en ninguna
  // parte. Se distingue del error de credenciales porque ese sí tumba la
  // pantalla entera; este solo desactiva el alta.
  const templates = cat.ok
    ? (
        (cat.data as { templates?: { key: string; name?: string; enabled?: boolean }[] })
          .templates ?? []
      ).map((tp) => ({
        key: tp.key,
        ...(tp.name ? { name: tp.name } : {}),
        enabled: tp.enabled ?? true,
      }))
    : [];
  const catalogoRoto = cat.ok ? undefined : cat.status;
  return { clients, templates, ...(catalogoRoto ? { catalogoRoto } : {}) };
}
