/**
 * Avatar: redimensionado de fotos en el NAVEGADOR y avatares predefinidos.
 *
 * La foto se recorta a un cuadrado pequeño y se recomprime antes de subir, así
 * el data URI que viaja (y se incrusta en la PHOTO de la vCard o el perfil de
 * Sytue ID, según quien llame) es liviano. El servidor revalida tipo y tamaño
 * como frontera de confianza; esto es solo la primera barrera + UX.
 *
 * Compartido por Contactos (foto de un contacto) y Mi cuenta (avatar propio):
 * las dos rutas necesitan el mismo recorte/tope y los mismos seis colores de
 * avatar de marca, así que vive una vez en @sytue/ui en vez de en cada app.
 */

/** Tipos raster admitidos (sin SVG: vectorial, puede portar script). */
const ALLOWED = new Set(['image/jpeg', 'image/png', 'image/webp', 'image/gif']);

/** Lado del avatar tras recortar (px). Suficiente para @2x sin inflar el peso. */
const AVATAR_SIDE = 256;

/** Tope duro del data URI DECODIFICADO (coincide con el del servidor: 256 KiB). */
export const MAX_PHOTO_BYTES = 256 * 1024;

/** Los seis colores de persona de la marca (mismo origen que Avatar.bgFor). */
export const PRESET_AVATAR_COLORS = [
  'var(--sytue-avatar-1)',
  'var(--sytue-avatar-2)',
  'var(--sytue-avatar-3)',
  'var(--sytue-avatar-4)',
  'var(--sytue-avatar-5)',
  'var(--sytue-avatar-6)',
] as const;

/**
 * Motivo del fallo. Es un CÓDIGO, no un texto: este módulo no sabe en qué
 * idioma está la interfaz (se ejecuta en el navegador, sin locale a mano), así
 * que la ficha traduce el código con su catálogo (docs/CONVENTIONS.md §13.1).
 */
export type ImageErrorCode = 'format' | 'process' | 'tooLarge' | 'read';

export class ImageError extends Error {
  constructor(
    readonly code: ImageErrorCode,
    mensaje?: string,
  ) {
    super(mensaje ?? code);
    this.name = 'ImageError';
  }
}

/**
 * Carga un File de imagen, lo recorta a un cuadrado de AVATAR_SIDE (cover) y lo
 * exporta como JPEG bajando la calidad hasta caber en MAX_PHOTO_BYTES. Devuelve
 * un data URI listo para subir. Lanza ImageError si el tipo no es raster
 * admitido o no se puede procesar.
 */
export async function fileToAvatarDataUri(file: File): Promise<string> {
  if (!ALLOWED.has(file.type)) {
    throw new ImageError('format');
  }
  const bitmap = await loadBitmap(file);
  try {
    const canvas = document.createElement('canvas');
    canvas.width = AVATAR_SIDE;
    canvas.height = AVATAR_SIDE;
    const ctx = canvas.getContext('2d');
    if (!ctx) throw new ImageError('process');

    // Recorte «cover»: la dimensión menor llena el cuadrado, centrado.
    const side = Math.min(bitmap.width, bitmap.height);
    const sx = (bitmap.width - side) / 2;
    const sy = (bitmap.height - side) / 2;
    ctx.drawImage(bitmap, sx, sy, side, side, 0, 0, AVATAR_SIDE, AVATAR_SIDE);

    // JPEG con calidad decreciente hasta caber; el avatar no necesita alfa.
    for (const quality of [0.85, 0.7, 0.55, 0.4]) {
      const dataUri = canvas.toDataURL('image/jpeg', quality);
      if (decodedBytes(dataUri) <= MAX_PHOTO_BYTES) return dataUri;
    }
    throw new ImageError('tooLarge');
  } finally {
    // ImageBitmap conviene liberarlo explícitamente.
    if ('close' in bitmap && typeof bitmap.close === 'function') bitmap.close();
  }
}

/**
 * Renderiza un avatar predefinido (iniciales sobre un color de marca) como el
 * mismo tipo de PNG que produciría una foto subida, para que el resto del
 * circuito (validación de servidor, incrustado en PHOTO, propagación) no tenga
 * que distinguir «predefinido» de «foto»: ambos son, a partir de aquí, un
 * data URI raster.
 */
export function presetAvatarDataUri(name: string, colorVar: string): string {
  const canvas = document.createElement('canvas');
  canvas.width = AVATAR_SIDE;
  canvas.height = AVATAR_SIDE;
  const ctx = canvas.getContext('2d');
  if (!ctx) throw new ImageError('process');
  const color = getComputedStyle(document.documentElement).getPropertyValue(
    colorVar.replace('var(', '').replace(')', ''),
  );
  ctx.fillStyle = color.trim() || '#0e7c74';
  ctx.fillRect(0, 0, AVATAR_SIDE, AVATAR_SIDE);
  ctx.fillStyle = '#fff';
  ctx.font = `600 ${Math.round(AVATAR_SIDE * 0.4)}px system-ui, sans-serif`;
  ctx.textAlign = 'center';
  ctx.textBaseline = 'middle';
  ctx.fillText(initialsOf(name), AVATAR_SIDE / 2, AVATAR_SIDE / 2 + 2);
  return canvas.toDataURL('image/png');
}

function initialsOf(name: string): string {
  const parts = name.trim().split(/\s+/).filter(Boolean);
  if (parts.length === 0) return '?';
  if (parts.length === 1) return (parts[0] ?? '?').slice(0, 2).toUpperCase();
  return ((parts[0]?.[0] ?? '') + (parts[parts.length - 1]?.[0] ?? '')).toUpperCase();
}

export interface ReducirImagenOpciones {
  maxLado: number;
  tipo?: string;
  maxBytes?: number;
  alfa?: 'png' | 'jpeg';
}

export interface ImagenReducida {
  dataUrl: string;
  ancho: number;
  alto: number;
  bytes: number;
}

/** ¿Algún píxel translúcido? Decide PNG (conserva alfa) frente a JPEG. */
function canvasHasAlpha(cx: CanvasRenderingContext2D, w: number, h: number): boolean {
  try {
    const data = cx.getImageData(0, 0, w, h).data;
    for (let i = 3; i < data.length; i += 4) {
      if (data[i]! < 255) return true;
    }
  } catch {
    /* ignore */
  }
  return false;
}

/**
 * Reescala y reduce una imagen en el cliente:
 * - Respeta orientación EXIF con createImageBitmap.
 * - Si es SVG, se rasteriza a PNG al doble de su tamaño de uso.
 * - Si createImageBitmap falla con un HEIC, el mensaje avisa «Guárdala como JPEG».
 * - Reduce tamaño y calidad por pasos hasta caber en maxBytes (si se indica).
 * - Devuelve la imagen reducida o null si ni al mínimo cabe.
 */
export async function reducirImagen(
  blob: Blob | File,
  opciones: ReducirImagenOpciones,
): Promise<ImagenReducida | null> {
  const { maxLado, tipo, maxBytes, alfa } = opciones;

  const esHeic =
    blob.type === 'image/heic' ||
    blob.type === 'image/heif' ||
    (blob instanceof File && /\.(heic|heif)$/i.test(blob.name));

  if (esHeic) {
    throw new ImageError('format', 'Guárdala como JPEG');
  }

  const esSvg =
    blob.type === 'image/svg+xml' || (blob instanceof File && /\.svg$/i.test(blob.name));

  if (esSvg) {
    return rasterizarSvg(blob, maxLado, maxBytes);
  }

  let bitmap: ImageBitmap;
  try {
    bitmap = await loadBitmap(blob);
  } catch (err) {
    if (esHeic || (err instanceof Error && /heic|heif/i.test(err.message))) {
      throw new ImageError('format', 'Guárdala como JPEG');
    }
    throw err;
  }

  try {
    let side = Math.min(maxLado, Math.max(bitmap.width, bitmap.height));
    let quality = 0.82;
    let png: boolean | null = null;

    for (let paso = 0; paso < 7; paso++) {
      const k = side / Math.max(1, Math.max(bitmap.width, bitmap.height));
      const canvas = document.createElement('canvas');
      canvas.width = Math.max(1, Math.round(bitmap.width * k));
      canvas.height = Math.max(1, Math.round(bitmap.height * k));
      const cx = canvas.getContext('2d');
      if (!cx) return null;
      cx.drawImage(bitmap, 0, 0, canvas.width, canvas.height);

      if (alfa === 'png') {
        png = true;
      } else if (alfa === 'jpeg') {
        png = false;
      } else {
        png ??= tipo !== 'image/jpeg' && canvasHasAlpha(cx, canvas.width, canvas.height);
      }

      const mime = png ? 'image/png' : tipo || 'image/jpeg';
      const dataUrl = canvas.toDataURL(mime, quality);
      const bytes = decodedBytes(dataUrl);

      if (!maxBytes || bytes <= maxBytes || dataUrl.length <= maxBytes) {
        return {
          dataUrl,
          ancho: canvas.width,
          alto: canvas.height,
          bytes,
        };
      }

      side = Math.round(side * 0.75);
      quality = Math.max(0.5, quality - 0.07);
    }
    return null;
  } finally {
    if ('close' in bitmap && typeof bitmap.close === 'function') {
      bitmap.close();
    }
  }
}

async function rasterizarSvg(
  blob: Blob | File,
  maxLado: number,
  maxBytes?: number,
): Promise<ImagenReducida | null> {
  const url = URL.createObjectURL(blob);
  try {
    const img = new Image();
    await new Promise<void>((resolve, reject) => {
      img.onload = () => resolve();
      img.onerror = () => reject(new ImageError('read'));
      img.src = url;
    });

    const natW = img.naturalWidth || 100;
    const natH = img.naturalHeight || 100;
    const escalaDoble = 2;
    let targetW = natW * escalaDoble;
    let targetH = natH * escalaDoble;
    if (Math.max(targetW, targetH) > maxLado) {
      const k = maxLado / Math.max(targetW, targetH);
      targetW = Math.max(1, Math.round(targetW * k));
      targetH = Math.max(1, Math.round(targetH * k));
    }

    const canvas = document.createElement('canvas');
    canvas.width = targetW;
    canvas.height = targetH;
    const cx = canvas.getContext('2d');
    if (!cx) return null;
    cx.drawImage(img, 0, 0, targetW, targetH);
    const dataUrl = canvas.toDataURL('image/png');
    const bytes = decodedBytes(dataUrl);

    if (maxBytes && bytes > maxBytes) {
      return null;
    }

    return {
      dataUrl,
      ancho: targetW,
      alto: targetH,
      bytes,
    };
  } finally {
    URL.revokeObjectURL(url);
  }
}

/** Decodifica un File o Blob a ImageBitmap (o HTMLImageElement de reserva). */
async function loadBitmap(file: Blob | File): Promise<ImageBitmap> {
  if ('createImageBitmap' in globalThis) {
    try {
      return await createImageBitmap(file);
    } catch {
      /* algunos navegadores no soportan todos los tipos: cae al <img> */
    }
  }
  const url = URL.createObjectURL(file);
  try {
    const img = new Image();
    await new Promise<void>((resolve, reject) => {
      img.onload = () => resolve();
      img.onerror = () => reject(new ImageError('read'));
      img.src = url;
    });
    return await createImageBitmap(img);
  } finally {
    URL.revokeObjectURL(url);
  }
}

/** Tamaño en bytes del contenido decodificado de un data URI base64. */
function decodedBytes(dataUri: string): number {
  const comma = dataUri.indexOf(',');
  if (comma < 0) return 0;
  const b64 = dataUri.slice(comma + 1);
  const padding = b64.endsWith('==') ? 2 : b64.endsWith('=') ? 1 : 0;
  return Math.floor((b64.length * 3) / 4) - padding;
}
