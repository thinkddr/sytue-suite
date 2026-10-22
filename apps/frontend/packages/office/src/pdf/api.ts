/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Cliente de Sytue PDF contra su BFF same-origin (`/api/pdf` y
 * `/api/pdf-aplicar`). Aquí viven los tipos que cruzan la frontera y la
 * conversión de coordenadas, que es lo único con miga de todo el fichero.
 */
import type { InformeVerificacion } from './verificacion';

/** Página tal y como la describe el motor. */
export interface Pagina {
  numero: number;
  ancho: number;
  alto: number;
  rotacion: number;
  /** MediaBox ordenado [x0 y0 x1 y1]. */
  caja: [number, number, number, number];
  /** Las primeras palabras, para reconocerla. Vacío: la página no tiene texto.
   *  Ausente: el motor no llegó a leerla (abrir no espera a leerlas todas). */
  extracto?: string;
}

export interface AnotacionLeida {
  pagina: number;
  tipo: string;
  rect: [number, number, number, number];
  texto?: string;
  autor?: string;
  fecha?: string;
  color?: [number, number, number];
  /** Índice, en esta misma lista, del comentario al que contesta (/IRT). */
  respuesta_a?: number;
  /** Estado de revisión (/State), en inglés porque es lo que lee Acrobat. */
  estado?: 'Accepted' | 'Rejected' | 'Cancelled' | 'Completed' | 'None';
}

export interface Campo {
  nombre: string;
  etiqueta?: string;
  tipo: 'texto' | 'casilla' | 'opcion' | 'lista' | 'desplegable' | 'boton' | 'firma';
  valor?: string;
  opciones?: string[];
  pagina: number;
  rect: [number, number, number, number];
  obligatorio: boolean;
  solo_lectura: boolean;
  varias_lineas?: boolean;
  contrasena?: boolean;
}

export interface FirmaLeida {
  nombre?: string;
  motivo?: string;
  lugar?: string;
  fecha?: string;
  campo?: string;
  cubre_todo: boolean;
}

export interface Permisos {
  Imprimir: boolean;
  Modificar: boolean;
  Copiar: boolean;
  Anotar: boolean;
  RellenarCampos: boolean;
  Accesibilidad: boolean;
  Ensamblar: boolean;
  ImprimirEnAlta: boolean;
  SoloConClaveDeDueno: boolean;
}

export interface Huella {
  Autor: string;
  Creador: string;
  Productor: string;
  Titulo: string;
  Asunto: string;
  Palabras: string;
  Creado: string;
  Modificado: string;
  ConXMP: boolean;
  ConAdjuntos: boolean;
  ConJS: boolean;
}

/** Una entrada del índice del documento (marcadores de /Outlines). */
export interface Marcador {
  titulo: string;
  /** 0-based; -1 si el destino no se pudo resolver. */
  pagina: number;
  nivel: number;
  abierto?: boolean;
}

/** Zona pulsable de una página: salto interno o dirección externa. */
export interface Enlace {
  pagina: number;
  rect: [number, number, number, number];
  /** Externo, YA filtrado en el servidor (solo http/https/mailto). */
  url?: string;
  /** Página de destino 0-based; -1 si el enlace es externo. */
  destino: number;
}

/** Un fragmento que casa con la búsqueda, con lo que pasaría al cambiarlo. */
export interface Coincidencia {
  pagina: number;
  id: number;
  antes: string;
  despues: string;
  /** Falso: el texto nuevo va a quedar apretado. Un PDF no tiene reflujo. */
  cabe: boolean;
}

/** Campo que el documento parece pedir, aún sin crear. */
export interface PropuestaCampo {
  tipo: string;
  pagina: number;
  rect: [number, number, number, number];
  etiqueta?: string;
}

/** Fichero incrustado en el PDF (una factura electrónica lleva su XML así). */
export interface Adjunto {
  nombre: string;
  descripcion?: string;
  tamano_bytes: number;
  /** MIME que DECLARA el documento: informativo, no decide cómo se sirve. */
  tipo?: string;
  /** Página donde está pegado, o -1 si cuelga del documento entero. */
  pagina: number;
}

export interface Contenido {
  paginas: Pagina[];
  total: number;
  indice: Marcador[] | null;
  enlaces: Enlace[] | null;
  adjuntos: Adjunto[] | null;
  anotaciones: AnotacionLeida[] | null;
  campos: Campo[] | null;
  firmas: FirmaLeida[] | null;
  cifrado: boolean;
  permisos: Permisos;
  huella: Huella;
}

/** Fragmento de texto con su posición, en coordenadas del PDF. */
export interface Fragmento {
  texto: string;
  x: number;
  y: number;
  tamano: number;
}

export interface DiferenciaPagina {
  pagina: number;
  anadido: string[] | null;
  quitado: string[] | null;
  solo_en_uno: boolean;
}

/** Una caja de diferencia, en PUNTOS de página (origen arriba-izquierda). */
export interface RegionVisual {
  x: number;
  y: number;
  ancho: number;
  alto: number;
}

/** Una página en la comparación VISUAL (ver `domain.DiferenciaVisual`). */
export interface DiferenciaVisual {
  pagina: number;
  /** `a` o `b`: la página solo existe en uno de los dos documentos. */
  solo_en?: 'a' | 'b';
  omitida?: boolean;
  motivo?: 'demasiado_grande' | 'ilegible';
  tamanos_distintos?: boolean;
  /** El lienzo comparado, en puntos: la referencia de las regiones. */
  ancho: number;
  alto: number;
  /** A qué escala se comparó de verdad (puede ser menor que la pedida). */
  escala: number;
  pixeles_diferentes: number;
  pixeles_totales: number;
  porcentaje: number;
  regiones: RegionVisual[] | null;
  /** PNG del resaltado en base64; solo cuando se pide UNA página. */
  imagen?: string;
}

export interface ComparacionVisual {
  paginas_a: number;
  paginas_b: number;
  comparadas: number;
  truncado: boolean;
  paginas: DiferenciaVisual[] | null;
  iguales: boolean;
}

/** Anotación que se ENVÍA. Coordenadas del PDF, origen abajo-izquierda. */
export interface AnotacionNueva {
  pagina: number;
  tipo: string;
  rect?: [number, number, number, number];
  cuadros?: [number, number, number, number][];
  trazos?: [number, number][][];
  puntos?: [number, number][];
  color: [number, number, number];
  color_interior?: [number, number, number];
  opacidad: number;
  grosor: number;
  texto?: string;
  autor?: string;
  tamano?: number;
  etiqueta?: string;
}

/** Error del BFF con el código estable del motor, cuando lo trae. */
export class ErrorPDF extends Error {
  constructor(
    message: string,
    readonly codigo?: string,
    readonly estado?: number,
    /** Los `details` del error del servicio, cuando trae datos para la pantalla. */
    readonly detalles?: Record<string, string>,
  ) {
    super(message);
    this.name = 'ErrorPDF';
  }
}

/**
 * `true` si guardar el cambio invalidaría las firmas del documento. El motor ya
 * no reescribe un PDF firmado por su cuenta: lo anexa, y si no puede, lo dice
 * con este código para que se pregunte antes de perderlas.
 */
export function invalidaFirmas(e: unknown): boolean {
  return e instanceof ErrorPDF && e.codigo === 'pdf_invalidates_signatures';
}

/** `true` si el documento pide una contraseña que aún no tenemos. */
export function pideContrasena(e: unknown): boolean {
  return e instanceof ErrorPDF && e.codigo === 'pdf_password_required';
}

async function pedir<T>(url: string, cuerpo: unknown): Promise<T> {
  const res = await fetch(url, {
    method: 'POST',
    headers: { 'content-type': 'application/json' },
    body: JSON.stringify(cuerpo),
  });
  const tipo = res.headers.get('content-type') ?? '';
  if (!res.ok) {
    let mensaje = `HTTP ${res.status}`;
    let codigo: string | undefined;
    let detalles: Record<string, string> | undefined;
    try {
      const j = (await res.json()) as {
        message?: string;
        code?: string;
        error?: string;
        details?: Record<string, string>;
      };
      mensaje = j.message ?? j.error ?? mensaje;
      detalles = j.details;
      // `error` del BFF («download_failed», «too_large») también es un CÓDIGO, no
      // un texto: sin recogerlo aquí acababa impreso tal cual en la pantalla del
      // usuario, que es lo que hacía antes.
      codigo = j.code ?? j.error;
    } catch {
      // respuesta sin JSON: se queda el mensaje genérico
    }
    throw new ErrorPDF(mensaje, codigo, res.status, detalles);
  }
  if (tipo.includes('image/svg')) return (await res.text()) as T;
  return (await res.json()) as T;
}

/** Fragmento de texto que se puede sustituir, con lo que decide si se puede. */
export interface FragmentoEditable {
  id: number;
  pagina: number;
  texto: string;
  x: number;
  y: number;
  tamano: number;
  ancho_disponible: number;
  fuente_incrustada: boolean;
  subconjunto: boolean;
  /** La fuente no dice qué letras usa: se puede borrar, no reescribir. */
  letras_desconocidas?: boolean;
}

export interface ImagenEditable {
  id: string;
  pagina: number;
  ancho: number;
  alto: number;
  es_mascara: boolean;
  formato: string;
  tamano_bytes: number;
}

export interface CapaDocumento {
  id: number;
  nombre: string;
  visible: boolean;
}

export interface AnalisisEdicion {
  fragmentos: FragmentoEditable[] | null;
  imagenes: ImagenEditable[] | null;
  capas: CapaDocumento[] | null;
  avisos: string[] | null;
}

export interface IncumplimientoArchivo {
  codigo: string;
  detalle?: string;
  bloqueante: boolean;
}

export interface InformeArchivo {
  cumple_pdfa: boolean;
  cumple_pdfua: boolean;
  problemas: IncumplimientoArchivo[] | null;
}

/**
 * Lo que hace falta para decidir si se ofrece «reconocer texto».
 *
 * `disponible` es del SERVIDOR y no del documento: dice si hay servicio de
 * reconocimiento detrás (ADR-0034). `paginas_sin_texto` es del documento. Se
 * necesitan las dos, y son cosas distintas: un servidor sin OCR y un documento
 * que ya tiene texto llevan al mismo sitio —no enseñar el botón— por motivos
 * que no se parecen en nada.
 */
export type EstadoOCR = {
  paginas_sin_texto: number[] | null;
  total: number;
  disponible: boolean;
  idiomas?: string[];
};

/** Una petición de firma (`pdfpedirfirma.go`), con la otra persona ya resuelta. */
export interface PeticionFirma {
  id: string;
  requester: string;
  requester_name?: string;
  recipient: string;
  recipient_email?: string;
  file_id: string;
  path: string;
  message?: string;
  status: 'pendiente' | 'firmada' | 'rechazada' | 'cancelada';
  created_at: string;
  resolved_at?: string;
  signed_path?: string;
  /**
   * Quien firma es de OTRA organización (ADR 0057). En las recibidas, `path` es la
   * copia en mi unidad —vacía hasta traerla con `firmas.abrir`— y `name` el nombre
   * del fichero. En las pedidas, una firmada sin `signed_path` se trae con
   * `firmas.traer`.
   */
  external?: boolean;
  name?: string;
  /** Quien pide (en las recibidas) o a quien se pidió (en las pedidas). */
  persona?: { email: string; name?: string };
}

export interface PeticionesFirma {
  received: PeticionFirma[];
  sent: PeticionFirma[];
  /** A quién he bloqueado (ADR 0059): sus peticiones de fuera no me llegan. */
  blocked?: { sub: string; name?: string; created_at: string }[];
}

/** Cómo se enseña a la otra persona: su nombre, su correo o lo que haya. */
export function quienFirma(p: PeticionFirma, lado: 'received' | 'sent'): string {
  return (
    p.persona?.name ||
    p.persona?.email ||
    (lado === 'received' ? (p.requester_name ?? '') : (p.recipient_email ?? ''))
  );
}

/** La pendiente que alguien me ha pedido firmar sobre ESTE fichero, si la hay. */
export function pendienteDe(ps: PeticionesFirma | null, path: string): PeticionFirma | undefined {
  return ps?.received.find((p) => p.status === 'pendiente' && p.path !== '' && p.path === path);
}

/** El nombre del PDF de una petición: de fuera, el suyo aunque no haya ruta. */
export function nombreDePeticion(p: PeticionFirma): string {
  return p.name ?? p.path.slice(p.path.lastIndexOf('/') + 1);
}

export const firmas = {
  listar: async (): Promise<PeticionesFirma> => {
    const r = await fetch('/api/pdf-firmas');
    if (!r.ok) throw new ErrorPDF(`HTTP ${r.status}`, undefined, r.status);
    return (await r.json()) as PeticionesFirma;
  },
  /**
   * `confirm`: la confirmación de sacar el PDF por un enlace de Drive hacia esa persona
   * de otra organización (ADR 0057): el `confirm` de los detalles del 409.
   */
  pedir: (path: string, email: string, mensaje: string, confirm = '') =>
    pedir<PeticionFirma>('/api/pdf-firmas', { op: 'pedir', path, email, mensaje, confirm }),
  /** De fuera: trae el PDF a mi unidad y devuelve la petición con su `path`. */
  abrir: (id: string) => pedir<PeticionFirma>('/api/pdf-firmas', { op: 'abrir', id }),
  /** De fuera: trae la copia firmada a mi unidad. */
  traer: (id: string) => pedir<PeticionFirma>('/api/pdf-firmas', { op: 'traer', id }),
  entregar: (id: string, path: string) =>
    pedir<PeticionFirma>('/api/pdf-firmas', { op: 'entregar', id, path }),
  rechazar: (id: string) => pedir<PeticionFirma>('/api/pdf-firmas', { op: 'rechazar', id }),
  cancelar: (id: string) => pedir<{ ok: true }>('/api/pdf-firmas', { op: 'cancelar', id }),
  /** De fuera: bloquea a quien me la pidió (ADR 0059). No se le dice. */
  bloquear: (id: string) => pedir<{ ok: true }>('/api/pdf-firmas', { op: 'bloquear', id }),
  desbloquear: (sub: string) => pedir<{ ok: true }>('/api/pdf-firmas', { op: 'desbloquear', sub }),
};

export const api = {
  contenido: (path: string, clave?: string) =>
    pedir<Contenido>('/api/pdf', { path, op: 'contenido', clave }),

  /** Dibuja un LOTE de páginas: «1-4». Devuelve número de página → SVG. */
  render: (path: string, paginas: string, escala: number, clave?: string) =>
    pedir<{ paginas: Record<string, string>; total: number }>('/api/pdf', {
      path,
      op: 'render',
      paginas,
      escala,
      clave,
    }),

  texto: (path: string, clave?: string) =>
    pedir<{ paginas: (Fragmento[] | null)[] }>('/api/pdf', { path, op: 'texto', clave }),

  comparar: (path: string, path2: string) =>
    pedir<{ diferencias: DiferenciaPagina[] | null; iguales: boolean }>('/api/pdf', {
      path,
      path2,
      op: 'comparar',
    }),

  /** Compara los PÍXELES de cada página: porcentaje y regiones, sin imágenes. */
  compararVisual: (path: string, path2: string, tolerancia: number) =>
    pedir<ComparacionVisual>('/api/pdf', {
      path,
      path2,
      op: 'comparar',
      modo: 'visual',
      tolerancia,
    }),

  /** Una página de la comparación visual, con su resaltado en PNG. */
  compararVisualPagina: (path: string, path2: string, pagina: number, tolerancia: number) =>
    pedir<{ pagina: DiferenciaVisual }>('/api/pdf', {
      path,
      path2,
      op: 'comparar',
      modo: 'visual',
      pagina,
      tolerancia,
    }),

  /** El informe de cada firma: integridad, cambios posteriores, sello y confianza. */
  verificar: (path: string, clave?: string) =>
    pedir<InformeVerificacion>('/api/pdf', { path, op: 'verificar', clave }),

  analizarEdicion: (path: string, pagina: number, clave?: string) =>
    pedir<AnalisisEdicion>('/api/pdf', { path, op: 'analizar-edicion', pagina, clave }),

  comprobarArchivo: (path: string, clave?: string) =>
    pedir<InformeArchivo>('/api/pdf', { path, op: 'comprobar-archivo', clave }),

  estadoOCR: (path: string, clave?: string) =>
    pedir<EstadoOCR>('/api/pdf', {
      path,
      op: 'estado-ocr',
      clave,
    }),

  /** Qué cambiaría un reemplazo, SIN cambiarlo: la lista se revisa antes. */
  buscarTexto: (path: string, busqueda: Record<string, unknown>, clave?: string) =>
    pedir<{ coincidencias: Coincidencia[] | null }>('/api/pdf', {
      path,
      op: 'buscar-texto',
      busqueda,
      clave,
    }),

  /** Los valores del formulario en XFDF, el formato de intercambio (ISO 19444-1). */
  exportarDatos: async (path: string, clave?: string): Promise<Blob> => {
    const r = await fetch('/api/pdf', {
      method: 'POST',
      headers: { 'content-type': 'application/json' },
      body: JSON.stringify({ path, op: 'exportar-datos', clave }),
    });
    if (!r.ok) throw new ErrorPDF('pdf_failed', 'pdf_failed', r.status);
    return await r.blob();
  },

  /** Campos que el impreso parece pedir. Se PROPONEN: crearlos es otro paso. */
  detectarCampos: (path: string, pagina: number, clave?: string) =>
    pedir<{ propuestas: PropuestaCampo[] | null }>('/api/pdf', {
      path,
      op: 'detectar-campos',
      pagina,
      clave,
    }),

  /** Saca un fichero incrustado. Devuelve el binario tal cual llega. */
  adjunto: async (path: string, nombre: string, clave?: string): Promise<Blob> => {
    const r = await fetch('/api/pdf', {
      method: 'POST',
      headers: { 'content-type': 'application/json' },
      body: JSON.stringify({ path, op: 'adjunto', nombre, clave }),
    });
    if (!r.ok) throw new ErrorPDF('pdf_failed', 'pdf_failed', r.status);
    return await r.blob();
  },

  aplicar: (cuerpo: Record<string, unknown>) =>
    pedir<{
      resource?: unknown;
      path: string;
      copia: boolean;
      ahorro?: number;
      bytes?: number;
      /** Resumen del reconocimiento: páginas leídas y saltadas por no tener imagen. */
      ocrPaginas?: number;
      ocrSinImagen?: number;
      /** Campos que encajaron al importar un XFDF. */
      camposImportados?: number;
      /** Fragmentos cambiados por un reemplazo. */
      reemplazos?: number;
      /** El documento lleva XMP: el cambio de propiedades puede no verse. */
      metadatosXmp?: boolean;
      /** Era PDF/A: «perdido» si el cambio no cumplía y se quitó la marca, «rebajado» si bajó de «a» a «b». */
      pdfa?: 'perdido' | 'rebajado';
    }>('/api/pdf-aplicar', cuerpo),
};

/* --------------------------------------------------------------------------
 * Coordenadas
 *
 * El render entrega la página en coordenadas de SVG —origen arriba-izquierda,
 * Y hacia abajo, y ya con /Rotate aplicado—; el motor de anotaciones espera las
 * del PDF —origen abajo-izquierda, sin rotar—. Convertir mal es el fallo que
 * deja el resaltado media página más arriba, así que la vuelta se escribe UNA
 * vez y se usa en todos los sitios.
 * ------------------------------------------------------------------------ */

/** De coordenadas del SVG dibujado a coordenadas del PDF. */
export function aPDF(p: Pagina, sx: number, sy: number): [number, number] {
  const [x0, y0, x1, y1] = p.caja;
  switch (p.rotacion) {
    case 90:
      return [sy + x0, sx + y0];
    case 180:
      return [x1 - sx, sy + y0];
    case 270:
      return [x1 - sy, y1 - sx];
    default:
      return [sx + x0, y1 - sy];
  }
}

/** De coordenadas del PDF a las del SVG dibujado. Es la inversa exacta. */
export function aSVG(p: Pagina, px: number, py: number): [number, number] {
  const [x0, y0, x1, y1] = p.caja;
  switch (p.rotacion) {
    case 90:
      return [py - y0, px - x0];
    case 180:
      return [x1 - px, py - y0];
    case 270:
      return [y1 - py, x1 - px];
    default:
      return [px - x0, y1 - py];
  }
}

/** Rectángulo del PDF (x0,y0,x1,y1) a caja del SVG {x,y,ancho,alto}. */
export function rectEnSVG(
  p: Pagina,
  r: [number, number, number, number],
): { x: number; y: number; ancho: number; alto: number } {
  const [ax, ay] = aSVG(p, r[0], r[1]);
  const [bx, by] = aSVG(p, r[2], r[3]);
  return {
    x: Math.min(ax, bx),
    y: Math.min(ay, by),
    ancho: Math.abs(bx - ax),
    alto: Math.abs(by - ay),
  };
}

/** `#rrggbb` → componentes 0-1, que es como los quiere el motor. */
export function colorARGB(hex: string): [number, number, number] {
  const n = parseInt(hex.replace('#', ''), 16);
  return [((n >> 16) & 255) / 255, ((n >> 8) & 255) / 255, (n & 255) / 255];
}

/** Componentes 0-1 → `#rrggbb`, para pintar en el navegador. */
export function rgbAColor(c: [number, number, number] | undefined): string {
  if (!c) return '#f9b923';
  const v = (x: number) =>
    Math.max(0, Math.min(255, Math.round(x * 255)))
      .toString(16)
      .padStart(2, '0');
  return `#${v(c[0])}${v(c[1])}${v(c[2])}`;
}

/**
 * Los tramos en que el índice parte el documento.
 *
 * Cada marcador de primer nivel abre un tramo y el siguiente lo cierra; el
 * último llega al final. Es «dividir por marcadores» de Foxit, y sale entero de
 * dos cosas que ya existen —el índice leído y extraer un rango—, así que no
 * necesita ni endpoint ni un ZIP con N ficheros que el navegador bloquearía.
 */
export function tramosDelIndice(
  indice: Marcador[],
  total: number,
): { titulo: string; rango: string }[] {
  // Los que no dicen a qué página van no pueden abrir un tramo.
  const capitulos = indice.filter((m) => m.nivel === 0 && m.pagina >= 0);
  return capitulos.map((m, i) => {
    const desde = m.pagina + 1;
    const siguiente = capitulos[i + 1];
    const hasta = siguiente ? Math.max(desde, siguiente.pagina) : total;
    return { titulo: m.titulo, rango: desde === hasta ? `${desde}` : `${desde}-${hasta}` };
  });
}

/**
 * Junta los fragmentos de una página en texto seguido, para leerlo en voz alta.
 *
 * Un PDF no guarda párrafos: guarda trozos colocados. «Estimado» y «cliente»
 * pueden ser dos fragmentos de la misma línea, y leerlos por separado suena
 * entrecortado —la voz hace una pausa donde no la hay—. Se agrupan por altura y
 * se ordenan por posición, que es como se leería en papel.
 */
export function textoParaLeer(frags: Fragmento[]): string {
  if (frags.length === 0) return '';
  // Dos fragmentos son de la misma línea si sus bases caen dentro de media
  // altura de letra. Con un umbral fijo, un pie de página de 6 pt y un título de
  // 30 pt no pueden compartir criterio.
  const orden = [...frags].sort((a, b) => b.y - a.y || a.x - b.x);
  const lineas: Fragmento[][] = [];
  for (const f of orden) {
    const ultima = lineas[lineas.length - 1];
    const base = ultima?.[0];
    if (ultima && base && Math.abs(base.y - f.y) <= Math.max(base.tamano, f.tamano) * 0.5) {
      ultima.push(f);
    } else {
      lineas.push([f]);
    }
  }
  return lineas
    .map((l) =>
      [...l]
        .sort((a, b) => a.x - b.x)
        .map((f) => f.texto.trim())
        .filter((t) => t !== '')
        // Sin espacio entre trozos, «Estimado»+«cliente» se leería como una sola
        // palabra inventada.
        .join(' '),
    )
    .filter((l) => l !== '')
    .join('. ');
}
