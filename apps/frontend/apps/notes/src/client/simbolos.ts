/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Los símbolos y emoji que se insertan desde la barra, desde «/» y con «:».
 *
 * Las categorías y sus símbolos son las del editor de Fyne (mismo orden): flechas,
 * matemáticas, moneda, tipografía y marcas, y unos emoji con palabras para buscarlos en
 * castellano y en inglés. Los nombres que oye un lector de pantalla NO se traducen
 * aquí: el botón lleva el propio carácter y el lector lo nombra en su idioma.
 *
 * Los últimos usados (16) se guardan en este navegador (`localStorage`), y si no se
 * puede —modo privado, almacenamiento bloqueado— el selector funciona igual sin ellos.
 */
import { plegar } from '@sytue/i18n';
import type { NotesMessageKey } from '../i18n/index.js';

export interface Simbolo {
  s: string;
  /** Palabras con las que se encuentra, en castellano y en inglés. */
  claves: string;
}

export type IdCategoria = 'flechas' | 'matematicas' | 'moneda' | 'tipografia' | 'marcas' | 'emoji';

export interface Categoria {
  id: IdCategoria;
  clave: NotesMessageKey;
  simbolos: readonly Simbolo[];
}

const s = (simbolo: string, claves: string): Simbolo => ({ s: simbolo, claves });

export const CATEGORIAS: readonly Categoria[] = [
  {
    id: 'flechas',
    clave: 'editor.symbolsArrows',
    simbolos: [
      s('←', 'flecha izquierda arrow left'),
      s('→', 'flecha derecha arrow right'),
      s('↑', 'flecha arriba arrow up'),
      s('↓', 'flecha abajo arrow down'),
      s('⇒', 'implica doble flecha implies double arrow'),
      s('↔', 'flecha doble ida vuelta arrow both'),
    ],
  },
  {
    id: 'matematicas',
    clave: 'editor.symbolsMath',
    simbolos: [
      s('±', 'mas menos plus minus'),
      s('×', 'por multiplicar times multiply'),
      s('÷', 'entre dividir divide'),
      s('≠', 'distinto no igual not equal'),
      s('≤', 'menor o igual less equal'),
      s('≥', 'mayor o igual greater equal'),
      s('∞', 'infinito infinity'),
      s('√', 'raiz root square'),
      s('∑', 'suma sumatorio sum sigma'),
      s('π', 'pi'),
      s('°', 'grado degree'),
    ],
  },
  {
    id: 'moneda',
    clave: 'editor.symbolsCurrency',
    simbolos: [
      s('€', 'euro'),
      s('$', 'dolar dollar'),
      s('£', 'libra pound'),
      s('¥', 'yen yuan'),
      s('₿', 'bitcoin'),
    ],
  },
  {
    id: 'tipografia',
    clave: 'editor.symbolsTypography',
    simbolos: [
      s('«', 'comillas angulares abrir quote guillemet'),
      s('»', 'comillas angulares cerrar quote guillemet'),
      s('“', 'comillas abrir quote left'),
      s('”', 'comillas cerrar quote right'),
      s('—', 'raya guion largo em dash'),
      s('–', 'semirraya guion medio en dash'),
      s('…', 'puntos suspensivos ellipsis'),
      s('•', 'vineta punto bullet'),
      s('·', 'punto medio middle dot'),
      s('§', 'seccion section'),
      s('¶', 'parrafo calderon pilcrow paragraph'),
      s('©', 'copyright'),
      s('®', 'registrado registered'),
      s('™', 'marca trademark'),
    ],
  },
  {
    id: 'marcas',
    clave: 'editor.symbolsMarks',
    simbolos: [
      s('✓', 'visto hecho check tick'),
      s('✗', 'cruz no cross'),
      s('★', 'estrella llena star'),
      s('☆', 'estrella vacia star outline'),
      s('♥', 'corazon heart'),
      s('⚠', 'aviso cuidado warning'),
      s('☐', 'casilla vacia checkbox'),
      s('☑', 'casilla marcada checkbox checked'),
    ],
  },
  {
    id: 'emoji',
    clave: 'editor.emoji',
    simbolos: [
      s('😀', 'sonrisa feliz grin smile happy'),
      s('😂', 'risa lagrimas joy laugh'),
      s('🙂', 'sonrisa leve slight smile'),
      s('😉', 'guino wink'),
      s('😍', 'enamorado corazones heart eyes love'),
      s('🤔', 'pensando thinking'),
      s('😅', 'sudor alivio sweat smile'),
      s('😎', 'gafas sol cool sunglasses'),
      s('😢', 'triste llorar cry sad'),
      s('😡', 'enfadado angry rage'),
      s('😴', 'dormir sueno sleep'),
      s('🤯', 'explota cabeza mind blown'),
      s('🥳', 'fiesta celebrar party'),
      s('🙏', 'gracias por favor pray thanks please'),
      s('👍', 'bien pulgar arriba ok thumbs up like'),
      s('👎', 'mal pulgar abajo thumbs down dislike'),
      s('👏', 'aplauso clap'),
      s('🙌', 'manos arriba celebrar raised hands'),
      s('👋', 'hola adios saludo wave hello'),
      s('💪', 'fuerza musculo strong muscle'),
      s('👀', 'ojos mirar eyes look'),
      s('🤝', 'acuerdo apreton handshake deal'),
      s('✅', 'hecho completado check done'),
      s('❌', 'no cruz error cross'),
      s('❓', 'pregunta duda question'),
      s('❗', 'importante exclamacion exclamation'),
      s('⭐', 'estrella star'),
      s('🔥', 'fuego fire hot'),
      s('✨', 'brillos chispas sparkles'),
      s('💡', 'idea bombilla bulb'),
      s('🎉', 'fiesta confeti tada party'),
      s('🎯', 'objetivo diana target goal'),
      s('🚀', 'cohete lanzamiento rocket launch'),
      s('📌', 'chincheta fijar pin'),
      s('📎', 'clip adjunto paperclip attach'),
      s('📝', 'nota apunte memo note'),
      s('📅', 'calendario fecha calendar date'),
      s('📈', 'grafico sube chart up'),
      s('📉', 'grafico baja chart down'),
      s('📊', 'grafico barras bar chart'),
      s('📚', 'libros books'),
      s('📞', 'telefono llamada phone call'),
      s('📧', 'correo email mail'),
      s('🔒', 'candado cerrado lock'),
      s('🔑', 'llave key'),
      s('🔗', 'enlace link'),
      s('🔍', 'buscar lupa search'),
      s('⏰', 'alarma despertador alarm clock'),
      s('⏳', 'reloj arena espera hourglass wait'),
      s('🕒', 'reloj hora clock time'),
      s('💬', 'comentario mensaje comment speech'),
      s('💭', 'pensamiento thought'),
      s('⚡', 'rayo rapido lightning fast'),
      s('🐛', 'bicho fallo bug'),
      s('🛠', 'herramientas tools'),
      s('⚙', 'ajustes engranaje settings gear'),
      s('💻', 'portatil ordenador laptop computer'),
      s('📱', 'movil telefono phone mobile'),
      s('🏠', 'casa hogar home house'),
      s('🏢', 'oficina empresa office building'),
      s('✈', 'avion viaje plane travel'),
      s('🚗', 'coche car'),
      s('☕', 'cafe coffee'),
      s('🍕', 'pizza'),
      s('🎂', 'tarta cumpleanos cake birthday'),
      s('🎁', 'regalo gift present'),
      s('❤', 'corazon amor heart love'),
      s('💔', 'corazon roto broken heart'),
      s('☀', 'sol sun'),
      s('🌧', 'lluvia rain'),
      s('❄', 'nieve frio snow cold'),
      s('🌱', 'planta brote plant sprout'),
      s('🌍', 'mundo tierra world earth'),
      s('🇪🇺', 'europa union europea eu europe flag'),
      s('🏁', 'meta bandera fin finish flag'),
      s('🚧', 'obras en construccion construction wip'),
      s('🛑', 'parar stop'),
      s('🟢', 'verde circulo green circle'),
      s('🟡', 'amarillo circulo yellow circle'),
      s('🔴', 'rojo circulo red circle'),
      s('🔵', 'azul circulo blue circle'),
      s('➕', 'mas sumar plus add'),
      s('➖', 'menos restar minus'),
      s('💯', 'cien perfecto hundred'),
      s('🤖', 'robot bot'),
      s('🧠', 'cerebro brain'),
      s('🎵', 'musica nota music'),
      s('📷', 'camara foto camera photo'),
      s('🎨', 'arte paleta art palette'),
      s('🏆', 'trofeo premio trophy award'),
      s('🤷', 'no se encogerse shrug'),
      s('🙈', 'no mirar mono see no evil'),
    ],
  },
];

/** Lo que sale en «Recientes» mientras no se ha usado nada. */
export const FRECUENTES: readonly string[] = ['→', '✓', '…', '—', '€', '«', '»', '•'];

/** Cuántos recientes se recuerdan. */
export const MAX_RECIENTES = 16;

/** Clave en `localStorage`, con el prefijo de las demás de Notes. */
export const CLAVE_RECIENTES = 'sytue.notes.simbolos';

function almacen(): Storage | null {
  try {
    return typeof localStorage === 'undefined' ? null : localStorage;
  } catch {
    return null;
  }
}

/** Los usados hace poco, el último primero. Lo que no sea una lista de símbolos se ignora. */
export function leerRecientes(a: Storage | null = almacen()): string[] {
  try {
    const crudo: unknown = JSON.parse(a?.getItem(CLAVE_RECIENTES) ?? '[]');
    if (!Array.isArray(crudo)) return [];
    return crudo
      .filter((x): x is string => typeof x === 'string' && x.length > 0 && x.length <= 16)
      .slice(0, MAX_RECIENTES);
  } catch {
    return [];
  }
}

/** Apunta `simbolo` como el último usado y devuelve la lista nueva. */
export function apuntarReciente(simbolo: string, a: Storage | null = almacen()): string[] {
  const lista = [simbolo, ...leerRecientes(a).filter((x) => x !== simbolo)].slice(0, MAX_RECIENTES);
  try {
    a?.setItem(CLAVE_RECIENTES, JSON.stringify(lista));
  } catch {
    // Sin sitio o sin permiso: los recientes duran lo que dure esta página.
  }
  return lista;
}

/** Los símbolos y emoji cuyas palabras empiezan por lo buscado (sin acentos ni mayúsculas). */
export function buscarSimbolos(q: string, solo?: IdCategoria): Simbolo[] {
  const terminos = plegar(q).split(/\s+/).filter(Boolean);
  if (terminos.length === 0) return [];
  return CATEGORIAS.filter((c) => !solo || c.id === solo)
    .flatMap((c) => c.simbolos)
    .filter((x) => {
      const palabras = plegar(x.claves).split(' ');
      return x.s === q.trim() || terminos.every((t) => palabras.some((p) => p.startsWith(t)));
    });
}

/**
 * Si el texto antes del cursor acaba en `:palabra` (dos letras o más, al principio o tras
 * un blanco), la palabra; si no, null. Es el `:` de los emoji, como en Confluence o Slack:
 * «a las 10:30» o una URL no abren nada.
 */
export function consultaDeEmoji(antes: string): string | null {
  const m = /(?:^|\s):([\p{L}\p{N}_]{2,20})$/u.exec(antes.replace(/ /g, ' '));
  return m ? m[1]! : null;
}
