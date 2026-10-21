/** Utilidad de clases condicionales para los componentes de @sytue/ui. */
import clsx, { type ClassValue } from 'clsx';

export function cx(...inputs: ClassValue[]): string {
  return clsx(...inputs);
}

export type { ClassValue };
