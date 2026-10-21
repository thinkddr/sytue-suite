/**
 * useAppsDesplegadas — qué productos levanta ESTA instalación.
 *
 * El conmutador de aplicaciones y el Launcher del portal presentan el catálogo
 * entero de la suite, y no toda instalación despliega todo: el perfil piloto se
 * queda sin Sytue DevX, y su tesela llevaba a un `cloud.<dominio>` que ahí no
 * existe. Un acceso que solo puede acabar en error parece una avería del
 * producto, no una decisión de despliegue.
 *
 * # POR QUÉ AL HUB Y NO SONDEANDO
 *
 * Quién está levantado lo sabe el DESPLIEGUE, y el Hub lo publica en
 * `GET /api/v1/apps` leyéndolo de su configuración. Sondear los dominios desde
 * el navegador diría «no responde» ante un corte de red o un bloqueador y
 * apagaría accesos que sí existen; además serían diez peticiones cruzadas por
 * apertura.
 *
 * # MISMO TRATO QUE EL MAPA DE PRODUCTOS
 *
 * Se pide al ABRIR el conmutador, no al montar: la shell la monta cada página de
 * cada aplicación, y esto solo se usa al desplegarlo. Y fallar es seguro: sin
 * respuesta se presenta la suite entera, que es como se ha comportado siempre.
 */
import { useCallback, useRef, useState } from 'preact/hooks';

/**
 * Dónde se recuerda qué productos levanta esta instalación.
 *
 * POR QUÉ HACE FALTA RECORDARLO. La lista se pide al pulsar el conmutador, y
 * hasta que contesta `appsPresentables` devuelve el catálogo ENTERO —es su
 * degradación correcta: mejor ofrecer algo que no está a esconder media suite
 * porque una petición falló—. El efecto visible era que Sytue DevX, apagado en
 * el `.env` de la instalación, aparecía en el selector y desaparecía un segundo
 * después. Un producto que parpadea y se va se lee como un fallo, no como una
 * decisión.
 *
 * Con la respuesta anterior guardada, la segunda vez —y todas las siguientes—
 * el selector ya se pinta bien de primeras. La petición sigue saliendo y
 * corrige sola si la instalación cambia.
 */
const RECUERDO = 'sytue:apps-desplegadas';

/** Lo último que dijo el Hub, si es que lo dijo alguna vez. */
export function appsRecordadas(): string[] | undefined {
  if (typeof localStorage === 'undefined') return undefined;
  try {
    const crudo = localStorage.getItem(RECUERDO);
    if (!crudo) return undefined;
    const v: unknown = JSON.parse(crudo);
    // Se valida antes de creérselo: esto viene del disco del navegador, que
    // cualquiera puede editar, y una lista con basura escondería productos.
    if (!Array.isArray(v) || v.length === 0) return undefined;
    const apps = v.filter((a): a is string => typeof a === 'string');
    return apps.length > 0 ? apps : undefined;
  } catch {
    return undefined; // sin almacenamiento (modo privado) o JSON roto
  }
}

/** Guarda lo que acaba de decir el Hub. Si no se puede, no pasa nada. */
export function recordarApps(apps: string[]): void {
  if (typeof localStorage === 'undefined') return;
  try {
    localStorage.setItem(RECUERDO, JSON.stringify(apps));
  } catch {
    // Cuota llena o almacenamiento bloqueado: se sigue sin recuerdo.
  }
}

export function useAppsDesplegadas(
  hubBaseUrl: string,
  accessToken: string | (() => string | Promise<string>),
): { desplegadas: string[] | undefined; load: () => void } {
  // `undefined` = todavía no se sabe, y se distingue de `[]` a propósito: la
  // lista vacía también significa «no se sabe» por contrato del Hub, y las dos
  // llevan a presentar todo.
  const [desplegadas, setDesplegadas] = useState<string[] | undefined>(appsRecordadas);
  const pedidoRef = useRef(false);

  const load = useCallback(() => {
    if (pedidoRef.current || !hubBaseUrl) return;
    pedidoRef.current = true;
    void (async () => {
      try {
        const token = typeof accessToken === 'function' ? await accessToken() : accessToken;
        if (!token) return;
        const res = await fetch(`${hubBaseUrl.replace(/\/$/, '')}/api/v1/apps`, {
          headers: { authorization: `Bearer ${token}` },
        });
        if (!res.ok) return;
        const cuerpo = (await res.json()) as { apps?: unknown };
        if (Array.isArray(cuerpo.apps) && cuerpo.apps.length > 0) {
          const apps = cuerpo.apps.filter((a): a is string => typeof a === 'string');
          setDesplegadas(apps);
          recordarApps(apps);
        }
      } catch {
        // Sin respuesta, el conmutador es el de siempre. No se avisa: no hay nada
        // que la persona que lo abre pueda arreglar.
      }
    })();
  }, [hubBaseUrl, accessToken]);

  return { desplegadas, load };
}
