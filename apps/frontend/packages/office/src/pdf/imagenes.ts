/**
 * Exportar páginas de un PDF como imágenes (PNG o JPEG).
 *
 * SE HACE EN EL CLIENTE, y no es un atajo: el navegador YA tiene la página
 * dibujada en SVG, con sus fuentes y sus imágenes dentro. Rasterizarla es
 * pintarla en un `canvas`. Hacerlo en el servidor obligaría a escribir el
 * rasterizador que [ADR-0033] decidió no escribir —el motor de la casa traduce
 * a SVG precisamente para que rasterice quien sabe—, y encima subiría y bajaría
 * megas por la red para producir lo que ya está en pantalla.
 *
 * El único requisito real es que el SVG sea AUTOSUFICIENTE: nada de referencias
 * externas. Lo es por construcción —las fuentes van como `@font-face` con un
 * `data:` y las imágenes también—, y por eso esto funciona sin pedirle nada al
 * servidor.
 */

/** Formatos que ofrece la exportación. */
export type FormatoImagen = 'png' | 'jpeg';

/** Una página exportada, lista para descargar o para meter en un ZIP. */
export type PaginaExportada = {
  numero: number;
  blob: Blob;
  nombre: string;
};

/**
 * Escala por defecto. 2 equivale a 144 ppp, que es lo que hace falta para que
 * una captura de una página no se vea borrosa en una pantalla moderna ni al
 * imprimirla en un documento. Subirla a 4 cuadruplica la memoria del `canvas`
 * y un A4 se planta en 100 megapíxeles.
 */
export const ESCALA_IMAGEN_POR_DEFECTO = 2;

/** Tope de píxeles por página. Por encima, los navegadores devuelven un canvas
 *  en blanco SIN error, que es peor que negarse. */
const MAX_PIXELES = 40e6;

/**
 * Convierte el SVG de una página en una imagen.
 *
 * `fondo` importa: un PDF no declara color de papel y un PNG con transparencia
 * se ve negro en cualquier visor con tema oscuro. En JPEG no hay alfa siquiera,
 * así que sin fondo el papel saldría negro.
 */
export async function paginaComoImagen(
  svg: string,
  anchoPt: number,
  altoPt: number,
  opciones: {
    formato?: FormatoImagen;
    escala?: number;
    calidad?: number;
    fondo?: string;
    /** Zona a quedarse, en coordenadas del SVG: `[x, y, ancho, alto]`. */
    recorte?: [number, number, number, number];
  } = {},
): Promise<Blob> {
  const formato = opciones.formato ?? 'png';
  const fondo = opciones.fondo ?? '#ffffff';
  let escala = opciones.escala ?? ESCALA_IMAGEN_POR_DEFECTO;

  if (anchoPt <= 0 || altoPt <= 0) throw new Error('pagina_sin_tamano');
  // Se BAJA la escala en vez de fallar: quien exporta un plano A0 quiere su
  // imagen, no un error sobre píxeles.
  const maxEscala = Math.sqrt(MAX_PIXELES / (anchoPt * altoPt));
  if (escala > maxEscala) escala = Math.max(1, maxEscala);

  const ancho = Math.max(1, Math.round(anchoPt * escala));
  const alto = Math.max(1, Math.round(altoPt * escala));
  // El recorte se aplica DESPLAZANDO la página dentro de un lienzo más pequeño,
  // no recortando el resultado: la página se dibuja una sola vez y a la escala
  // que toca, así que un trozo de un plano sale con el mismo detalle que si se
  // hubiera exportado entero.
  const [rx, ry, rw, rh] = opciones.recorte ?? [0, 0, anchoPt, altoPt];
  const anchoSalida = Math.max(1, Math.round(rw * escala));
  const altoSalida = Math.max(1, Math.round(rh * escala));

  const imagen = await cargarSVG(svg);
  const canvas = document.createElement('canvas');
  canvas.width = anchoSalida;
  canvas.height = altoSalida;
  const ctx = canvas.getContext('2d');
  if (!ctx) throw new Error('sin_canvas');
  ctx.fillStyle = fondo;
  ctx.fillRect(0, 0, anchoSalida, altoSalida);
  ctx.drawImage(imagen, -rx * escala, -ry * escala, ancho, alto);

  return await new Promise<Blob>((resolver, rechazar) => {
    canvas.toBlob(
      (b) => (b ? resolver(b) : rechazar(new Error('canvas_vacio'))),
      formato === 'jpeg' ? 'image/jpeg' : 'image/png',
      formato === 'jpeg' ? (opciones.calidad ?? 0.92) : undefined,
    );
  });
}

/**
 * Carga el SVG como imagen decodificada.
 *
 * Va por `data:` con base64 y NO por `URL.createObjectURL`: un blob URL de tipo
 * `image/svg+xml` lo tratan algunos navegadores como documento externo y
 * bloquean el `data:` de las fuentes incrustadas, con lo que el texto sale con
 * la tipografía equivocada. El `data:` no tiene ese problema porque el SVG
 * entero es el propio recurso.
 */
function cargarSVG(svg: string): Promise<HTMLImageElement> {
  return new Promise((resolver, rechazar) => {
    const img = new Image();
    img.onload = () => resolver(img);
    img.onerror = () => rechazar(new Error('svg_ilegible'));
    img.src = 'data:image/svg+xml;base64,' + aBase64(asegurarNamespace(svg));
  });
}

/**
 * `btoa` solo acepta bytes latin-1, así que una página con un solo acento lo
 * hace lanzar. Se codifica a UTF-8 primero, que es lo que el SVG declara.
 */
function aBase64(texto: string): string {
  const bytes = new TextEncoder().encode(texto);
  let binario = '';
  // A trozos: `String.fromCharCode(...bytes)` con una página entera desborda la
  // pila de argumentos y falla justo con los documentos grandes.
  const paso = 0x8000;
  for (let i = 0; i < bytes.length; i += paso) {
    binario += String.fromCharCode(...bytes.subarray(i, i + paso));
  }
  return btoa(binario);
}

/**
 * Un SVG suelto necesita su `xmlns` para que el navegador lo cargue como
 * imagen. El del motor ya lo trae; esto cubre el caso de que alguien pase un
 * fragmento, que fallaría con un «svg_ilegible» sin pista de por qué.
 */
function asegurarNamespace(svg: string): string {
  if (svg.includes('xmlns=')) return svg;
  return svg.replace('<svg', '<svg xmlns="http://www.w3.org/2000/svg"');
}

/** Nombre de fichero de una página, con el número rellenado a la izquierda para
 *  que el explorador las ordene bien más allá de la novena. */
export function nombreDePagina(base: string, numero: number, total: number, ext: string): string {
  const ancho = String(total).length;
  return `${base}-${String(numero).padStart(ancho, '0')}.${ext}`;
}

/** Lanza la descarga de un blob con el nombre dado. */
export function descargarBlob(blob: Blob, nombre: string): void {
  const url = URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = url;
  a.download = nombre;
  document.body.appendChild(a);
  a.click();
  a.remove();
  // El objeto se libera en el siguiente turno: revocarlo en el mismo cancela la
  // descarga en Safari.
  setTimeout(() => URL.revokeObjectURL(url), 0);
}
