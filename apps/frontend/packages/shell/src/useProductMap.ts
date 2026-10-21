/**
 * useProductMap — resuelve QUIÉN sirve cada producto de la suite en esta organización.
 *
 * La organización puede decidir que un producto lo sirva una aplicación conectada:
 * Google Sheets en vez de Sytue Sheets, Teams en vez de Rooms, Azure en vez de Cloud.
 * Ese mapa vive en la pasarela de integraciones; aquí se lee para que el conmutador de
 * aplicaciones abra lo que la organización ha elegido y no siempre el producto nativo.
 *
 * # POR QUÉ SE PIDE AL ABRIR Y NO AL MONTAR
 *
 * La shell la monta CADA página de CADA app de la suite. Pedir el mapa en el montaje
 * sería una petición de red en cada carga de cada aplicación para un dato que solo se
 * usa al desplegar el conmutador —y que en la mayoría de organizaciones está vacío—.
 * Se pide la primera vez que alguien lo abre y se queda en memoria.
 *
 * # FALLAR ES SEGURO
 *
 * Si la pasarela no contesta, el mapa queda vacío y todo apunta al producto nativo de
 * Sytue. Es la degradación correcta: como mucho el usuario aterriza en el producto de la
 * suite en vez de en su sustituto, en lugar de quedarse con un conmutador roto.
 */
import { useCallback, useRef, useState } from 'preact/hooks';
import { integrationsDomain } from '@sytue/config';
import type { ProductKey } from '@sytue/config';

/** A dónde abrir un producto que sirve una aplicación conectada. */
export interface SubstituteTarget {
  /** Nombre de la aplicación que lo sirve (se enseña junto al del producto). */
  name: string;
  /** URL a la que lleva la tesela. */
  href: string;
}

export type ProductMap = Partial<Record<ProductKey, SubstituteTarget>>;

interface ResolvedWire {
  products?: Record<string, { name?: unknown; href?: unknown }>;
}

/**
 * useProductMap devuelve el mapa y una función para asegurarse de que está cargado.
 * `load` es idempotente: varias aperturas del conmutador no disparan varias peticiones.
 */
export function useProductMap(accessToken: string | (() => string | Promise<string>)): {
  map: ProductMap;
  load: () => void;
} {
  const [map, setMap] = useState<ProductMap>({});
  // Ref y no estado: que ya se haya pedido no debe provocar un render.
  const requestedRef = useRef(false);

  const load = useCallback(() => {
    if (requestedRef.current) return;
    requestedRef.current = true;
    void (async () => {
      try {
        const token = typeof accessToken === 'function' ? await accessToken() : accessToken;
        if (!token) return;
        const res = await fetch(`${integrationsDomain}/api/v1/substitutions/resolved`, {
          headers: { authorization: `Bearer ${token}` },
          credentials: 'omit',
        });
        if (!res.ok) return;
        const body = (await res.json()) as ResolvedWire;
        const out: ProductMap = {};
        for (const [product, entry] of Object.entries(body.products ?? {})) {
          if (typeof entry?.href !== 'string' || !entry.href) continue;
          out[product as ProductKey] = {
            name: typeof entry.name === 'string' ? entry.name : product,
            href: entry.href,
          };
        }
        setMap(out);
      } catch {
        // Silencio deliberado: sin mapa se abre el producto nativo, que siempre existe.
      }
    })();
  }, [accessToken]);

  return { map, load };
}
