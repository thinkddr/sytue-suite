/**
 * AvatarPicker — avatar con control de foto: predefinidos (iniciales sobre un
 * color de marca) o subir un archivo propio (PNG/JPEG/WebP/GIF), más «quitar»
 * si ya hay foto. Compartido por Contactos (foto de un contacto) y Mi cuenta
 * (avatar propio de Sytue ID): mismo widget, cada app decide qué hacer con el
 * data URI resultante (su propio endpoint de subida).
 *
 * No sube nada por su cuenta: entrega el data URI ya recortado/validado en el
 * navegador (onPick) y deja la llamada de red, el toast y el «busy» a quien lo
 * usa, que es quien conoce su propia API.
 */
import type { JSX } from 'preact';
import { useRef, useState } from 'preact/hooks';
import { Avatar } from './Avatar.js';
import type { AvatarSize } from './Avatar.js';
import { Dropdown } from './Dropdown.js';
import { Icon } from './Icon.js';
import {
  fileToAvatarDataUri,
  presetAvatarDataUri,
  ImageError,
  PRESET_AVATAR_COLORS,
  type ImageErrorCode,
} from '../lib/avatarImage.js';

export interface AvatarPickerLabels {
  /** Etiqueta accesible cuando ya hay foto («Cambiar foto»). */
  change: string;
  /** Etiqueta accesible cuando no hay foto todavía («Añadir foto»). */
  add: string;
  /** Título de la sección de colores predefinidos. */
  presets: string;
  /** Acción «Subir foto…». */
  upload: string;
  /** Acción «Quitar foto». */
  remove: string;
  /** Traduce el código de error del navegador a un texto. */
  error: (code: ImageErrorCode) => string;
}

export interface AvatarPickerProps {
  /** Nombre para iniciales, aria-label y avatares predefinidos. */
  name: string;
  src?: string | undefined;
  hasPhoto: boolean;
  size?: AvatarSize;
  /** Deshabilita toda interacción mientras la app sube/quita la foto. */
  busy?: boolean;
  labels: AvatarPickerLabels;
  onPick: (dataUri: string) => void | Promise<void>;
  onRemove?: (() => void | Promise<void>) | undefined;
  onError?: ((message: string) => void) | undefined;
}

export function AvatarPicker({
  name,
  src,
  hasPhoto,
  size = 'xl',
  busy,
  labels,
  onPick,
  onRemove,
  onError,
}: AvatarPickerProps): JSX.Element {
  const fileRef = useRef<HTMLInputElement>(null);
  const [working, setWorking] = useState(false);
  const isBusy = Boolean(busy) || working;

  async function fromFile(input: HTMLInputElement, close: () => void): Promise<void> {
    const file = input.files?.[0];
    input.value = ''; // permite re-subir el mismo fichero
    if (!file) return;
    setWorking(true);
    try {
      const dataUri = await fileToAvatarDataUri(file);
      close();
      await onPick(dataUri);
    } catch (err) {
      onError?.(err instanceof ImageError ? labels.error(err.code) : labels.error('process'));
    } finally {
      setWorking(false);
    }
  }

  async function fromPreset(colorVar: string, close: () => void): Promise<void> {
    setWorking(true);
    try {
      const dataUri = presetAvatarDataUri(name, colorVar);
      close();
      await onPick(dataUri);
    } catch {
      onError?.(labels.error('process'));
    } finally {
      setWorking(false);
    }
  }

  return (
    <div class="relative inline-flex">
      <Avatar name={name} size={size} src={src} />
      <Dropdown
        label={hasPhoto ? labels.change : labels.add}
        align="start"
        trigger={(props) => (
          <button
            type="button"
            disabled={isBusy}
            aria-label={hasPhoto ? labels.change : labels.add}
            class="border-surface bg-surface text-text-muted hover:text-text sytue-focus-ring rounded-control absolute -right-1 -bottom-1 grid h-7 w-7 place-items-center border-2 shadow-[var(--shadow-xs)] transition-colors duration-150 disabled:pointer-events-none disabled:opacity-50 pointer-coarse:after:absolute pointer-coarse:after:-inset-2 pointer-coarse:after:content-['']"
            {...props}
          >
            <Icon name="camera" size={14} />
          </button>
        )}
      >
        {(close) => (
          <div class="flex w-56 flex-col gap-2 p-2.5">
            <p class="text-text-muted px-0.5 text-xs font-medium">{labels.presets}</p>
            <div class="grid grid-cols-6 gap-1.5">
              {PRESET_AVATAR_COLORS.map((color) => (
                <button
                  key={color}
                  type="button"
                  disabled={isBusy}
                  aria-label={labels.presets}
                  onClick={() => void fromPreset(color, close)}
                  class="sytue-focus-ring h-7 w-7 rounded-full ring-1 ring-[var(--sytue-border)] ring-inset disabled:opacity-50"
                  style={{ backgroundColor: color }}
                />
              ))}
            </div>
            <hr class="border-border my-0.5" />
            <button
              type="button"
              disabled={isBusy}
              onClick={() => fileRef.current?.click()}
              class="hover:bg-bg-subtle sytue-focus-ring rounded-control px-2 py-1.5 text-left text-sm disabled:opacity-50"
            >
              {labels.upload}
            </button>
            {hasPhoto && onRemove ? (
              <button
                type="button"
                disabled={isBusy}
                onClick={() => {
                  close();
                  void onRemove();
                }}
                class="hover:bg-danger-bg sytue-focus-ring text-danger-text rounded-control px-2 py-1.5 text-left text-sm disabled:opacity-50"
              >
                {labels.remove}
              </button>
            ) : null}
            <input
              ref={fileRef}
              type="file"
              accept="image/jpeg,image/png,image/webp,image/gif"
              class="hidden"
              aria-hidden="true"
              tabindex={-1}
              onChange={(e) => void fromFile(e.currentTarget, close)}
            />
          </div>
        )}
      </Dropdown>
    </div>
  );
}

export default AvatarPicker;
