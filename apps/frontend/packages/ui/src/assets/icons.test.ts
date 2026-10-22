/**
 * El fallback del SVG es lo que SE PINTA de verdad.
 *
 * Los iconos de producto se consumen con `<img src={url}>` (AppGrid, Launcher,
 * GlobalSearch, ProductGrid…). Un SVG cargado así NO resuelve las custom
 * properties del documento anfitrión: `var(--sytue-product-x, #hex)` cae SIEMPRE
 * al literal. Así que cambiar el token sin cambiar el fallback no cambia nada en
 * pantalla — y si solo se cambia en algunos ficheros, el mismo producto acaba
 * con dos colores según el tamaño (le pasó a `id`: pizarra a 40 px, latón a 15).
 *
 * Este test ata las tres fuentes: token CSS, variante normal y variante -sm.
 */
import { describe, expect, it } from 'bun:test';
import { readFileSync, readdirSync } from 'node:fs';
import { fileURLToPath } from 'node:url';

const ICONS = fileURLToPath(new URL('./icons/', import.meta.url));
const TOKENS = fileURLToPath(new URL('../../../config/src/tokens/colors.css', import.meta.url));

/** Fallback declarado dentro de un SVG: var(--sytue-product-<clave>, #hex). */
function fallback(archivo: string): { clave: string; hex: string } | null {
  const svg = readFileSync(`${ICONS}${archivo}`, 'utf8');
  const m = /var\(--sytue-product-([a-z]+),\s*(#[0-9a-fA-F]{6})\)/.exec(svg);
  return m ? { clave: m[1]!, hex: m[2]!.toLowerCase() } : null;
}

const tokens = new Map<string, string>();
for (const linea of readFileSync(TOKENS, 'utf8').split('\n')) {
  const m = /--sytue-product-([a-z]+):\s*(#[0-9a-fA-F]{6})/.exec(linea);
  if (m) tokens.set(m[1]!, m[2]!.toLowerCase());
}

const svgs = readdirSync(ICONS).filter((f) => f.endsWith('.svg'));

describe('iconos de producto', () => {
  it('encuentra los tokens de color de producto', () => {
    expect(tokens.size).toBeGreaterThan(10);
  });

  it('el fallback de cada SVG coincide con su token', () => {
    const divergen: string[] = [];
    for (const f of svgs) {
      const fb = fallback(f);
      if (!fb) continue;
      const token = tokens.get(fb.clave);
      if (token && token !== fb.hex) divergen.push(`${f}: ${fb.hex} ≠ token ${token}`);
    }
    expect(divergen).toEqual([]);
  });

  it('cada icono y su variante -sm pintan el mismo color', () => {
    const distintos: string[] = [];
    for (const f of svgs.filter((x) => !x.endsWith('-sm.svg'))) {
      const sm = f.replace('.svg', '-sm.svg');
      if (!svgs.includes(sm)) continue;
      const a = fallback(f);
      const b = fallback(sm);
      if (a && b && a.hex !== b.hex) distintos.push(`${f} ${a.hex} vs ${sm} ${b.hex}`);
    }
    expect(distintos).toEqual([]);
  });

  it('todo icono con variante normal tiene su -sm (y al revés)', () => {
    const base = svgs.filter((f) => !f.endsWith('-sm.svg')).map((f) => f.replace('.svg', ''));
    const sm = svgs.filter((f) => f.endsWith('-sm.svg')).map((f) => f.replace('-sm.svg', ''));
    // `meet` quedó retirado de la UI (unificado en Rooms) y no tiene -sm propia.
    const sinPareja = base.filter((k) => k !== 'meet' && !sm.includes(k));
    expect(sinPareja).toEqual([]);
  });
});
