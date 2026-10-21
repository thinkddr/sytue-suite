// Checks de reducción de imágenes y avatares en @sytue/ui. Ejecutar: `bun test`.
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { describe, expect, test } from 'bun:test';
import { reducirImagen, presetAvatarDataUri, type ImageError } from './avatarImage';

if (typeof document === 'undefined') GlobalRegistrator.register();

// En entorno de prueba (happy-dom), simulamos el contexto 2d del canvas si no existe.
if (typeof HTMLCanvasElement !== 'undefined') {
  const origGetContext = HTMLCanvasElement.prototype.getContext;
  HTMLCanvasElement.prototype.getContext = function (contextId: string, ...args: unknown[]) {
    if (contextId === '2d') {
      return {
        fillStyle: '',
        font: '',
        textAlign: '',
        textBaseline: '',
        fillRect: () => {},
        fillText: () => {},
        drawImage: () => {},
        getImageData: () => ({ data: new Uint8ClampedArray(4) }),
      } as unknown as CanvasRenderingContext2D;
    }
    return origGetContext ? origGetContext.call(this, contextId, ...args) : null;
  } as typeof HTMLCanvasElement.prototype.getContext;

  HTMLCanvasElement.prototype.toDataURL = function () {
    return 'data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg==';
  };
}

describe('avatarImage y reducirImagen', () => {
  test('presetAvatarDataUri produce un data URI de PNG con iniciales', () => {
    const uri = presetAvatarDataUri('Daniel Delgado', '--sytue-avatar-1');
    expect(uri.startsWith('data:image/png')).toBe(true);
  });

  test('reducirImagen rechaza formato HEIC indicando «Guárdala como JPEG»', async () => {
    const heicBlob = new Blob(['fake heic'], { type: 'image/heic' });
    let error: ImageError | null = null;
    try {
      await reducirImagen(heicBlob, { maxLado: 1000 });
    } catch (err) {
      error = err as ImageError;
    }
    expect(error).not.toBeNull();
    expect(error?.message).toContain('Guárdala como JPEG');
  });

  test('reducirImagen rechaza fichero .heic por extensión indicando «Guárdala como JPEG»', async () => {
    const heicFile = new File(['fake heic'], 'foto.heic', { type: '' });
    let error: ImageError | null = null;
    try {
      await reducirImagen(heicFile, { maxLado: 1000 });
    } catch (err) {
      error = err as ImageError;
    }
    expect(error).not.toBeNull();
    expect(error?.message).toContain('Guárdala como JPEG');
  });
});
