/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * REVISIÓN DE SEGURIDAD DE UNA CUENTA, COMO FUNCIÓN PURA.
 *
 * Mi cuenta la pinta arriba del todo, y el portal y el Hub la van a enseñar
 * también: por eso vive en la Shell y no en la app, y por eso no pide nada a la
 * red ni lee el reloj. Recibe lo que ya han traído `/me`, las passkeys, las
 * sesiones, los dispositivos y las contraseñas de aplicación, y dice punto por
 * punto si está bien o merece un vistazo, con el ancla de la sección que lo
 * arregla.
 *
 * Un dato que no ha llegado (`null`/ausente) NO es un aviso: si `/api/devices`
 * ha fallado no se sabe nada de los dispositivos, y avisar de algo que no se ha
 * mirado asusta sin motivo. Ese punto simplemente no sale.
 */

/** Pasado este tiempo sin uso, una credencial se da por olvidada. */
export const DIAS_SIN_USO = 90;

export type ClaveRevision =
  'dos_pasos' | 'recuperacion' | 'sesiones' | 'contrasenas_app' | 'dispositivos';

export interface PuntoRevision {
  clave: ClaveRevision;
  estado: 'ok' | 'aviso';
  /** Sección de Mi cuenta que lo resuelve, con su `#`. */
  ancla: string;
  /** Cuántas cosas lo provocan (sesiones ajenas, contraseñas viejas…). 0 si está bien. */
  cuantos: number;
}

export interface DatosRevision {
  /** Lo que importa de `/me`. */
  me?: { mfa_enabled?: boolean; recovery_email?: string; phone?: string } | null;
  passkeys?: readonly unknown[] | null;
  sesiones?: readonly { current?: boolean }[] | null;
  dispositivos?: readonly { trusted?: boolean; current?: boolean; last_seen?: string }[] | null;
  contrasenasApp?: readonly { created_at?: string; last_used_at?: string | null }[] | null;
  /** El «ahora» se recibe para que el resultado no dependa del reloj (y se pueda probar). */
  ahora: Date;
}

export const ANCLAS: Record<ClaveRevision, string> = {
  dos_pasos: '#seguridad',
  recuperacion: '#recuperacion',
  sesiones: '#sesiones',
  contrasenas_app: '#contrasenas-app',
  dispositivos: '#dispositivos',
};

function punto(clave: ClaveRevision, cuantos: number): PuntoRevision {
  return { clave, estado: cuantos > 0 ? 'aviso' : 'ok', ancla: ANCLAS[clave], cuantos };
}

/** `true` si la fecha es de hace más de `DIAS_SIN_USO`; si falta o no se entiende, `false` (no se inventan avisos). */
function olvidada(iso: string | null | undefined, ahora: Date): boolean {
  const ms = iso ? Date.parse(iso) : NaN;
  if (Number.isNaN(ms)) return false;
  return ahora.getTime() - ms > DIAS_SIN_USO * 86_400_000;
}

/** Puntos de la revisión, en el orden en que se enseñan (de más a menos grave). */
export function revisionDeCuenta(d: DatosRevision): PuntoRevision[] {
  const out: PuntoRevision[] = [];
  // Una passkey ya es un segundo factor (posesión + biometría o PIN): con ella
  // no hace falta pedir además un código de aplicación.
  // Si la verificación está apagada y las passkeys no han llegado, no se sabe si
  // hay segundo factor: avisar de «sin verificación» a quien tiene una llave sería
  // justo el aviso inventado que se evita arriba, así que ese punto no sale.
  if (d.me) {
    const conLlave = d.passkeys ? d.passkeys.length > 0 : null;
    if (d.me.mfa_enabled || conLlave !== null) {
      const protegida = Boolean(d.me.mfa_enabled) || conLlave === true;
      out.push(punto('dos_pasos', protegida ? 0 : 1));
    }
    const recuperable = Boolean(d.me.recovery_email?.trim() || d.me.phone?.trim());
    out.push(punto('recuperacion', recuperable ? 0 : 1));
  }
  if (d.sesiones) {
    out.push(punto('sesiones', d.sesiones.filter((s) => !s.current).length));
  }
  if (d.contrasenasApp) {
    // Nunca usada cuenta desde el alta: una contraseña creada hace un año y que no
    // ha servido nunca es justo la que conviene revocar.
    const viejas = d.contrasenasApp.filter((p) =>
      olvidada(p.last_used_at || p.created_at, d.ahora),
    );
    out.push(punto('contrasenas_app', viejas.length));
  }
  if (d.dispositivos) {
    // De confianza y sin verse en 90 días: se saltan el segundo factor desde un
    // equipo que quizá ya ni es de la persona. El actual nunca cuenta.
    const dormidos = d.dispositivos.filter(
      (e) => e.trusted && !e.current && olvidada(e.last_seen, d.ahora),
    );
    out.push(punto('dispositivos', dormidos.length));
  }
  return out;
}

/*
 * PRIMEROS PASOS DE UNA CUENTA.
 *
 * La portada (y el Hub nativo) enseña una lista corta de lo que queda por dejar
 * listo, y cada paso desaparece al cumplirse. Los dos primeros SON puntos de la
 * revisión de arriba —no se vuelve a decidir aquí qué cuenta como «protegida» o
 * «recuperable»—; los otros tres son de puesta en marcha, no de seguridad.
 *
 * Los tres primeros son los que sostienen la tarjeta: cumplidos, desaparece
 * aunque quede conectar los dispositivos o instalar.
 *
 * La misma regla que la revisión: un dato que no ha llegado no inventa un paso.
 * Si `/me` falla no se pide la foto; si las contraseñas de aplicación no llegan
 * no se pide conectar los dispositivos.
 */

export type ClavePaso = 'dos_pasos' | 'recuperacion' | 'foto' | 'dispositivos' | 'instalar';

export interface PasoPendiente {
  clave: ClavePaso;
  /**
   * Dónde se arregla: una sección de Mi cuenta (`cuenta`, con su `#`) o una
   * página del propio portal (`portal`, con su ruta sin prefijo).
   */
  destino: 'cuenta' | 'portal';
  ruta: string;
}

export interface DatosPrimerosPasos {
  me?: {
    mfa_enabled?: boolean;
    recovery_email?: string;
    phone?: string;
    has_avatar?: boolean;
  } | null;
  passkeys?: readonly unknown[] | null;
  /** Las contraseñas de aplicación: es lo que crea «Conectar tus dispositivos». */
  contrasenasApp?: readonly unknown[] | null;
  /**
   * Si Sytue One ya está instalada aquí o la persona ya pasó por las descargas.
   * No hay API que lo diga (las apps nativas entran por OIDC como un navegador),
   * así que lo decide quien llama con lo que sabe el dispositivo.
   */
  instalada?: boolean;
}

/** Los pasos que QUEDAN, en el orden en que se enseñan (primero lo que protege la cuenta). */
export function primerosPasos(d: DatosPrimerosPasos): PasoPendiente[] {
  const out: PasoPendiente[] = [];
  // El reloj no interviene en los dos puntos que se usan (solo en las
  // contraseñas viejas y los dispositivos dormidos, que aquí no se piden).
  const revision = revisionDeCuenta({
    me: d.me ?? null,
    passkeys: d.passkeys ?? null,
    ahora: new Date(0),
  });
  for (const p of revision) {
    if (p.estado === 'aviso' && (p.clave === 'dos_pasos' || p.clave === 'recuperacion')) {
      out.push({ clave: p.clave, destino: 'cuenta', ruta: p.ancla });
    }
  }
  if (d.me && d.me.has_avatar === false) {
    out.push({ clave: 'foto', destino: 'cuenta', ruta: '#perfil' });
  }
  if (d.contrasenasApp && d.contrasenasApp.length === 0) {
    out.push({ clave: 'dispositivos', destino: 'portal', ruta: '/dispositivos' });
  }
  if (!d.instalada) {
    out.push({ clave: 'instalar', destino: 'portal', ruta: '/descargas' });
  }
  // Conectar e instalar acompañan, no sostienen la tarjeta: una cuenta con la
  // verificación, la recuperación y la foto ya está lista, y una tarjeta con solo
  // «instala la app» sería un anuncio que no se va nunca (instalar no deja rastro).
  return out.some((p) => p.destino === 'cuenta') ? out : [];
}
