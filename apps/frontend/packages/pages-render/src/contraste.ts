/* SPDX-License-Identifier: Apache-2.0 */

/** Relación de contraste WCAG 2.x para dos colores hexadecimales (#rgb, #rrggbb, alfa ignorado). */
export function contraste(a: string, b: string): number {
  const luminancia = (color: string): number => {
    const limpio = color.replace('#', '');
    const hex =
      limpio.length === 3
        ? limpio
            .split('')
            .map((c) => c + c)
            .join('')
        : limpio.slice(0, 6);
    if (!/^[0-9a-f]{6}$/i.test(hex)) throw new Error(`color hex inválido: ${color}`);
    const canales = [0, 2, 4].map((i) => Number.parseInt(hex.slice(i, i + 2), 16) / 255);
    const lineales = canales.map((v) => (v <= 0.04045 ? v / 12.92 : ((v + 0.055) / 1.055) ** 2.4));
    return 0.2126 * lineales[0]! + 0.7152 * lineales[1]! + 0.0722 * lineales[2]!;
  };
  const [claro, oscuro] = [luminancia(a), luminancia(b)].sort((x, y) => y - x);
  return (claro! + 0.05) / (oscuro! + 0.05);
}
