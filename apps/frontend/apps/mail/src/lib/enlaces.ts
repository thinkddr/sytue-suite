/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 */

/**
 * Un trozo del texto de un correo: texto llano, o un enlace con su destino. Juntando
 * los `texto` de todos los tramos sale el texto de partida, letra a letra.
 */
export interface Tramo {
  texto: string;
  /** Lo que se abre al pulsarlo (`https://…` o `mailto:…`). Sin él, es texto. */
  destino?: string;
}

/*
 * Las reglas son las de `htmltext.Enlaces` en apps/nucleo/htmltext/enlaces.go, y los
 * vectores de apps/nucleo/htmltext/testdata/enlaces.json los recorren las dos: el
 * mismo correo tiene que enlazar lo mismo en la web y en Fyne.
 *
 * Lo que NO puede ir dentro de un enlace es todo blanco de Unicode (los de `\s` y los
 * de `unicode.IsSpace` de Go, por donde Fyne parte las líneas), las comillas dobles,
 * los ángulos —`<https://…>` es como se cita una dirección en texto plano— y los
 * caracteres de control. Va escrito a mano y no con `\s` porque `\s` no es el mismo
 * conjunto en JavaScript que en Go.
 */
const FUERA_DE_ENLACE =
  '\\u0000-\\u0020\\u007f-\\u00a0\\u1680\\u2000-\\u200b\\u2028\\u2029\\u202f\\u205f\\u3000\\ufeff<>"';
const DIRECCION = '[a-z0-9._%+-]+@[a-z0-9-]+(?:\\.[a-z0-9-]+)*\\.[a-z]{2,}';
const RE_URL = new RegExp(`(?:https?://|www\\.)[^${FUERA_DE_ENLACE}]+`, 'gi');
const RE_CORREO = new RegExp(`(?:mailto:)?${DIRECCION}`, 'gi');
const ALFANUMERICO = /[\p{L}\p{Nd}]/u;

function buscar(re: RegExp, s: string, desde: number): RegExpExecArray | null {
  re.lastIndex = desde;
  return re.exec(s);
}

/** Una letra delante de «https://» o «www.» que dice que ahí no empieza una dirección. */
function pegado(c: string): boolean {
  return ALFANUMERICO.test(c) || '_@./-+%'.includes(c);
}

/** Cuánto de `u` es dirección: sin la puntuación final ni los cierres que no abren dentro. */
function recortarCola(u: string): number {
  const pares: Record<string, string> = { ')': '(', ']': '[', '}': '{' };
  let fin = u.length;
  while (fin > 0) {
    const c = u[fin - 1] as string;
    if (".,;:!?'*".includes(c)) {
      fin--;
      continue;
    }
    const abre = pares[c];
    if (abre) {
      const tramo = u.slice(0, fin);
      if (tramo.split(abre).length < tramo.split(c).length) {
        fin--;
        continue;
      }
    }
    break;
  }
  return fin;
}

/**
 * Lo que se abre: la dirección tal cual, o `https://` delante de un «www.». Vacío si no
 * queda un nombre de máquina (un «https://» suelto) o si un «www.» no lleva dominio
 * detrás («www.algo» es una palabra con un punto).
 */
function destinoURL(u: string): string {
  const www = u.slice(0, 4).toLowerCase() === 'www.';
  let resto = u;
  if (!www) {
    const i = resto.indexOf('://');
    if (i >= 0) resto = resto.slice(i + 3);
  }
  let maquina = resto.split(/[/?#]/, 1)[0] ?? '';
  maquina = maquina.slice(maquina.lastIndexOf('@') + 1);
  const dosPuntos = maquina.lastIndexOf(':');
  if (dosPuntos >= 0) maquina = maquina.slice(0, dosPuntos);
  if (!ALFANUMERICO.test(maquina)) return '';
  if (
    www &&
    !maquina
      .slice(4)
      .replace(/^\.+|\.+$/g, '')
      .includes('.')
  )
    return '';
  return www ? `https://${u}` : u;
}

function buscarURL(s: string, desde: number): [number, number, string] | null {
  let pos = desde;
  while (pos < s.length) {
    const m = buscar(RE_URL, s, pos);
    if (!m) return null;
    const ini = m.index;
    const antes = ini > 0 ? Array.from(s.slice(Math.max(0, ini - 2), ini)).pop() : undefined;
    if (antes === undefined || !pegado(antes)) {
      const fin = ini + recortarCola(m[0]);
      const destino = destinoURL(s.slice(ini, fin));
      if (destino) return [ini, fin, destino];
    }
    // «xhttps://» o «correo@www.algo»: no empieza aquí; se sigue una letra después.
    pos = ini + 1;
  }
  return null;
}

function buscarCorreo(s: string, desde: number): [number, number, string] | null {
  let pos = desde;
  while (pos < s.length) {
    const m = buscar(RE_CORREO, s, pos);
    if (!m) return null;
    const ini = m.index;
    const fin = ini + m[0].length;
    const despues = s.slice(fin, fin + 2);
    const sig = despues ? (Array.from(despues)[0] as string) : '';
    // «ana@sytue.eu2» o «ana@sytue.eu_x» no son una dirección que acabe en «.eu».
    if (!sig || !(ALFANUMERICO.test(sig) || sig === '_' || sig === '-')) {
      const dir = m[0].slice(0, 7).toLowerCase() === 'mailto:' ? m[0].slice(7) : m[0];
      return [ini, fin, `mailto:${dir}`];
    }
    pos = fin;
  }
  return null;
}

/**
 * Parte un texto plano en tramos, con las direcciones web (`http://`, `https://`,
 * `www.`) y de correo convertidas en enlaces. Solo salen destinos http(s) y mailto:
 * nada que el navegador pudiera ejecutar.
 *
 * La puntuación con la que acaba una frase no es de la dirección («visita
 * https://sytue.eu.»), y un paréntesis de cierre solo lo es si abre otro dentro
 * («(https://es.wikipedia.org/wiki/Gato_(desambiguación))»).
 */
export function enlazarTexto(texto: string): Tramo[] {
  const out: Tramo[] = [];
  const llano = (s: string): void => {
    if (!s) return;
    const ultimo = out[out.length - 1];
    if (ultimo && !ultimo.destino) ultimo.texto += s;
    else out.push({ texto: s });
  };
  let pos = 0;
  while (pos < texto.length) {
    const u = buscarURL(texto, pos);
    const c = buscarCorreo(texto, pos);
    // El que empieza antes; entre una web y un correo que empiezan igual, la web.
    const m = !u ? c : !c ? u : u[0] <= c[0] ? u : c;
    if (!m) break;
    llano(texto.slice(pos, m[0]));
    out.push({ texto: texto.slice(m[0], m[1]), destino: m[2] });
    pos = m[1];
  }
  llano(texto.slice(pos));
  return out;
}

/** La dirección de un destino `mailto:`, o `null` si el destino es una web. */
export function correoDeDestino(destino: string): string | null {
  return destino.slice(0, 7).toLowerCase() === 'mailto:' ? destino.slice(7) : null;
}
