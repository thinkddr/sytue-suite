/**
 * La RECETA de edición, en el navegador — el mismo contrato que
 * services/syphotos/internal/domain/receta.go, calcado a propósito.
 *
 * DOS IMPLEMENTACIONES, UN CONTRATO. El navegador previsualiza sobre una copia
 * reducida para responder al instante; el servidor exporta aplicando la misma
 * receta al original. Si las dos interpretan un paso de forma distinta, lo que
 * se descarga no es lo que se vio: el peor fallo posible en un editor. Por eso
 * aquí no hay «lo que haga la librería»: son los mismos tipos, los mismos
 * rangos y la misma huella, y hay tests de paridad con vectores generados por
 * el propio Go.
 *
 * LA HUELLA NOMBRA LAS DERIVADAS. `huella()` tiene que dar exactamente lo que da
 * `Receta.Huella()` en Go —misma serialización canónica, mismo redondeo, mismo
 * hash, mismos doce hexadecimales— o el cliente pediría un fichero que el
 * servidor guardó con otro nombre.
 */

export const VERSION_RECETA = 1;

export type TipoPaso =
  'recorte' | 'giro' | 'espejo' | 'exposicion' | 'contraste' | 'saturacion' | 'temperatura';

export interface Paso {
  tipo: TipoPaso;
  params?: Record<string, number>;
  /** Solo el espejo: «h» o «v». */
  eje?: 'h' | 'v';
}

export interface Receta {
  version: number;
  pasos: Paso[];
}

/** Rangos admitidos por parámetro, idénticos a `rangos` en Go. */
export const RANGOS: Record<TipoPaso, Record<string, readonly [number, number]>> = {
  recorte: { x: [0, 1], y: [0, 1], ancho: [0, 1], alto: [0, 1] },
  giro: { grados: [0, 270] },
  espejo: {},
  exposicion: { valor: [-5, 5] },
  contraste: { valor: [-1, 1] },
  saturacion: { valor: [-1, 1] },
  temperatura: { valor: [-1, 1] },
};

export const MAX_PASOS = 256;

export function recetaVacia(): Receta {
  return { version: VERSION_RECETA, pasos: [] };
}

/**
 * Valida un paso: existe, sus parámetros están en rango y no falta ninguno.
 * Devuelve el motivo o `null` si es válido. Las mismas reglas que Go, incluidos
 * NaN e infinito, que no son «fuera de rango» —toda comparación con NaN es
 * falsa— y hay que rechazarlos aparte.
 */
export function validaPaso(p: Paso): string | null {
  const esperados = RANGOS[p.tipo];
  if (!esperados) return `paso desconocido «${p.tipo}»`;
  if (p.tipo === 'espejo' && p.eje !== 'h' && p.eje !== 'v') {
    return `el espejo necesita eje «h» o «v»`;
  }
  const params = p.params ?? {};
  for (const [nombre, v] of Object.entries(params)) {
    const lim = esperados[nombre];
    if (!lim) return `${p.tipo} no admite el parámetro «${nombre}»`;
    if (!Number.isFinite(v)) return `${p.tipo}.${nombre} no es un número`;
    if (v < lim[0] || v > lim[1]) {
      return `${p.tipo}.${nombre} = ${v}, fuera de [${lim[0]}, ${lim[1]}]`;
    }
  }
  for (const nombre of Object.keys(esperados)) {
    if (!(nombre in params)) return `a ${p.tipo} le falta el parámetro «${nombre}»`;
  }
  if (p.tipo === 'recorte') {
    const { x = 0, y = 0, ancho = 0, alto = 0 } = params;
    if (x + ancho > 1.0000001 || y + alto > 1.0000001) return 'el recorte se sale del lienzo';
    if (ancho <= 0 || alto <= 0) return 'el recorte no puede tener lado cero';
  }
  return null;
}

export function validaReceta(r: Receta): string | null {
  if (!(r.version > 0 && r.version <= VERSION_RECETA)) {
    return `versión de receta ${r.version} (esta compilación entiende hasta la ${VERSION_RECETA})`;
  }
  if (r.pasos.length > MAX_PASOS) return `${r.pasos.length} pasos; el máximo es ${MAX_PASOS}`;
  for (let i = 0; i < r.pasos.length; i += 1) {
    const m = validaPaso(r.pasos[i]!);
    if (m) return `paso ${i}: ${m}`;
  }
  return null;
}

/**
 * Seis decimales como CADENA, igual que `fmt.Sprintf("%.6f")`.
 *
 * `toFixed` y `%.6f` redondean el mismo valor binario, así que coinciden en
 * todo lo que sale de un deslizador. La única diferencia conocida es el cero
 * negativo: Go imprime «-0.000000» y JS «0.000000». Se conserva el signo aquí
 * para que la huella no cambie por un `0 * -1`.
 */
function fijo6(v: number): string {
  const s = v.toFixed(6);
  return Object.is(v, -0) || (v < 0 && s === '0.000000') ? '-0.000000' : s;
}

/**
 * Serialización canónica, byte a byte la de Go: `json.Marshal` de un mapa
 * ordena las claves alfabéticamente —`eje`, `params`, `tipo`; y arriba `pasos`,
 * `version`—, los parámetros van ordenados por nombre y como cadenas de seis
 * decimales, sin espacios. Cualquier desviación es una huella distinta y una
 * derivada duplicada.
 */
export function canonica(r: Receta): string {
  const pasos = r.pasos.map((p) => {
    const partes: string[] = [];
    if (p.eje) partes.push(`"eje":${JSON.stringify(p.eje)}`);
    const nombres = Object.keys(p.params ?? {}).sort();
    if (nombres.length) {
      const params = nombres.map(
        (n) => `${JSON.stringify(n)}:${JSON.stringify(fijo6(p.params![n]!))}`,
      );
      partes.push(`"params":{${params.join(',')}}`);
    }
    partes.push(`"tipo":${JSON.stringify(p.tipo)}`);
    return `{${partes.join(',')}}`;
  });
  return `{"pasos":[${pasos.join(',')}],"version":${r.version}}`;
}

/** SHA-256 de la forma canónica, primeros doce hexadecimales: igual que Go. */
export async function huella(r: Receta): Promise<string> {
  const datos = new TextEncoder().encode(canonica(r));
  const suma = await crypto.subtle.digest('SHA-256', datos);
  return [...new Uint8Array(suma)]
    .map((b) => b.toString(16).padStart(2, '0'))
    .join('')
    .slice(0, 12);
}
