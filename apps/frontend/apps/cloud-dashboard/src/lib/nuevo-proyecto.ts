/**
 * Lógica del asistente «Nuevo proyecto» (origen → configurar → variables →
 * revisar → desplegar), separada de la isla para poder probarla sin navegador.
 *
 * Lo delicado no es pintar los pasos sino el ÚLTIMO: crear el proyecto, guardar
 * sus variables y lanzar el primer despliegue son tres llamadas distintas al
 * control-plane, y cualquiera puede fallar a mitad. Si falla la tercera variable
 * de cinco, el proyecto ya existe y dos variables ya están guardadas: repetir
 * desde el principio chocaría con un 409 («ese nombre ya es tuyo») y volvería a
 * escribir lo ya escrito. Por eso el alta es una cadena REANUDABLE: el estado dice
 * qué se hizo y «Reintentar» sigue justo donde se paró.
 *
 * Solo tipos y funciones puras: sin `lib/cloud.ts` (es de servidor) ni DOM.
 */
import type { DeployStatus, Project } from './types.js';

/**
 * Nombres propios de framework: no se traducen (solo «estático» y «automático»,
 * que son palabras, salen del catálogo). Incluye los que devuelve la detección
 * del repositorio aunque no tengan opción en el desplegable: sin nombre saldrían
 * en minúscula y con pinta de erratas. Lo comparten la lista de proyectos y los
 * dos asistentes de alta.
 */
export const FRAMEWORK_NAME: Readonly<Record<string, string>> = {
  astro: 'Astro',
  next: 'Next.js',
  preact: 'Preact',
  node: 'Node',
  sveltekit: 'SvelteKit',
  go: 'Go',
  docker: 'Docker',
};

/** Etiqueta DNS: es el subdominio público, así que el formato no es negociable. */
export const SLUG_RE = /^[a-z0-9]([a-z0-9-]{0,38}[a-z0-9])?$/;

/** Misma regla que el BFF de variables (`/api/env`) y que el alta de una sola. */
export const ENV_KEY_RE = /^[A-Za-z_][A-Za-z0-9_]{0,127}$/;

/**
 * Propone la dirección a partir del nombre. Se quitan los acentos en vez de
 * rechazar el nombre: quien escribe «Mi Tienda Rápida» no tiene por qué saber que
 * un subdominio no admite tildes.
 */
export function toSlug(name: string): string {
  return name
    .normalize('NFD')
    .replace(/\p{Diacritic}/gu, '')
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, '-')
    .slice(0, 40)
    .replace(/^-+|-+$/g, '');
}

/**
 * El alta a mano (`POST /api/projects`) solo acepta https://: el webhook y la
 * detección leen el repositorio por HTTPS. Exigir algo más («que acabe en .git»)
 * rechazaría direcciones que las forjas sirven sin problema.
 */
export function validRepoUrl(url: string): boolean {
  const u = url.trim();
  return /^https:\/\/[^\s/]+\/\S+/.test(u);
}

/**
 * «owner/repo» a partir de la URL, para proponer el nombre del proyecto: quien
 * pega `https://git.sytue.eu/acme/tienda` casi siempre quiere llamarlo «tienda».
 */
export function repoName(url: string): string {
  const path = url
    .trim()
    .replace(/^https:\/\/[^/]+\//, '')
    .replace(/\.git$/, '')
    .replace(/\/+$/, '');
  const last = path.split('/').pop() ?? '';
  return last;
}

// ---- Variables ----

export interface VarRow {
  /** Identidad estable de la fila: la clave cambia mientras se escribe. */
  id: number;
  key: string;
  value: string;
  secret: boolean;
}

export type VarError = 'keyFormat' | 'valueRequired' | 'duplicate';

/** Una fila en blanco no es un error: es la que se acaba de añadir. */
function isBlank(r: VarRow): boolean {
  return r.key.trim() === '' && r.value === '';
}

/**
 * Error de cada fila, por id. Se marca la clave REPETIDA en todas sus filas y no
 * solo en la segunda: con dos valores para `API_KEY` el usuario tiene que decidir
 * cuál vale, y señalar solo uno le haría creer que el otro es el bueno.
 */
export function varErrors(rows: VarRow[]): Map<number, VarError> {
  const out = new Map<number, VarError>();
  const count = new Map<string, number>();
  for (const r of rows) {
    if (isBlank(r)) continue;
    const k = r.key.trim();
    count.set(k, (count.get(k) ?? 0) + 1);
  }
  for (const r of rows) {
    if (isBlank(r)) continue;
    const k = r.key.trim();
    if (!ENV_KEY_RE.test(k)) out.set(r.id, 'keyFormat');
    else if (r.value.trim() === '') out.set(r.id, 'valueRequired');
    else if ((count.get(k) ?? 0) > 1) out.set(r.id, 'duplicate');
  }
  return out;
}

/** Las variables que de verdad se van a guardar (sin las filas en blanco). */
export function filledVars(rows: VarRow[]): Array<{ key: string; value: string; secret: boolean }> {
  return rows
    .filter((r) => !isBlank(r))
    .map((r) => ({ key: r.key.trim(), value: r.value, secret: r.secret }));
}

/**
 * ¿Parece un secreto? Es solo el valor POR DEFECTO de la casilla «Secreto» al
 * pegar un `.env`: equivocarse hacia «secreto» cuesta no poder releer el valor;
 * equivocarse hacia «no secreto» deja una contraseña legible en el panel. Las
 * variables que el propio framework expone al navegador (`PUBLIC_`, `NEXT_PUBLIC_`,
 * `VITE_`) son públicas por definición, se llamen como se llamen.
 */
export function looksSecret(key: string): boolean {
  const k = key.toUpperCase();
  if (/^(PUBLIC_|NEXT_PUBLIC_|VITE_|NUXT_PUBLIC_)/.test(k)) return false;
  return /(SECRET|TOKEN|PASSWORD|PASSWD|PWD|PRIVATE|CREDENTIAL|API_?KEY|_KEY$|DSN|DATABASE_URL)/.test(
    k,
  );
}

/**
 * Añade lo leído de un `.env` a las filas. Una clave que ya estaba se ACTUALIZA
 * en su sitio (la última gana, como en cualquier cargador de `.env`) en vez de
 * duplicarse; las filas en blanco desaparecen para no dejar huecos entre medias.
 */
export function mergeParsed(
  rows: VarRow[],
  parsed: Array<{ key: string; value: string }>,
  nextId: () => number,
): VarRow[] {
  const out = rows.filter((r) => !isBlank(r)).map((r) => ({ ...r }));
  for (const p of parsed) {
    const prev = out.find((r) => r.key.trim() === p.key);
    if (prev) prev.value = p.value;
    else out.push({ id: nextId(), key: p.key, value: p.value, secret: looksSecret(p.key) });
  }
  return out;
}

// ---- Alta reanudable ----

export type TaskState = 'pending' | 'running' | 'done' | 'failed';
export type Phase = 'project' | 'vars' | 'deploy';

export interface CreateBody {
  slug: string;
  name: string;
  repo_url: string;
  framework: string;
  build_strategy: string;
  build_command: string;
  install_command: string;
  output_dir: string;
  production_ref: string;
  root_dir: string;
}

export interface PipelineInput {
  project: CreateBody;
  vars: Array<{ key: string; value: string; secret: boolean }>;
}

export interface PipelineError {
  phase: Phase;
  status: number;
  code: string;
  /** Variable que falló, si el fallo fue al guardar variables. */
  key?: string;
}

export interface PipelineState {
  project: TaskState;
  /** Proyecto ya creado (con el secreto del webhook, que solo llega aquí). */
  created?: Project;
  /** false = sin control-plane: no se ha creado nada de verdad. */
  live: boolean;
  vars: TaskState;
  /** Cuántas variables ya están guardadas: lo que un reintento se salta. */
  varsSaved: number;
  deploy: TaskState;
  error?: PipelineError;
}

export const INITIAL_PIPELINE: PipelineState = {
  project: 'pending',
  live: true,
  vars: 'pending',
  varsSaved: 0,
  deploy: 'pending',
};

/** POST al BFF; lanza un error con `status` y `code` (BffError) si falla. */
export type Post = (url: string, body: Record<string, unknown>) => Promise<unknown>;

function errorInfo(err: unknown): { status: number; code: string } {
  const e = err as { status?: unknown; code?: unknown };
  return {
    status: typeof e?.status === 'number' ? e.status : 0,
    code: typeof e?.code === 'string' ? e.code : 'unexpected',
  };
}

/**
 * Ejecuta (o REANUDA) el alta: proyecto → variables → primer despliegue.
 *
 * Cada fase se salta si ya está hecha, así que llamarla otra vez tras un fallo
 * sigue donde se quedó. Se para en el primer error: desplegar sin la variable que
 * falló sería lanzar una compilación que probablemente romperá por su falta, y el
 * usuario vería un error de build en vez del que de verdad hay que arreglar.
 *
 * Las variables van EN SERIE (como el importador de `.env`): cada alta pasa por la
 * cuota y la auditoría del control-plane, y en paralelo leerían el mismo recuento.
 */
export async function runPipeline(
  input: PipelineInput,
  from: PipelineState,
  post: Post,
  emit: (s: PipelineState) => void,
): Promise<PipelineState> {
  let s: PipelineState = { ...from };
  delete s.error;
  const set = (patch: Partial<PipelineState>) => {
    s = { ...s, ...patch };
    emit(s);
  };

  if (s.project !== 'done') {
    set({ project: 'running' });
    try {
      const res = (await post('/api/projects', { ...input.project })) as {
        live?: boolean;
        project?: Project;
      };
      // Sin control-plane el BFF devuelve un proyecto de demostración: seguir con
      // variables y despliegue sería simular un alta que no existe.
      if (!res.live || !res.project) {
        set({ project: 'pending', live: false });
        return s;
      }
      set({ project: 'done', created: res.project, live: true });
    } catch (err) {
      set({ project: 'failed', error: { phase: 'project', ...errorInfo(err) } });
      return s;
    }
  }

  const slug = s.created?.slug ?? input.project.slug;
  if (s.vars !== 'done') {
    set({ vars: 'running' });
    for (let i = s.varsSaved; i < input.vars.length; i++) {
      const v = input.vars[i]!;
      try {
        await post('/api/env', {
          project: slug,
          key: v.key,
          value: v.value,
          secret: v.secret,
          scope: 'all',
        });
        set({ varsSaved: i + 1 });
      } catch (err) {
        set({ vars: 'failed', error: { phase: 'vars', key: v.key, ...errorInfo(err) } });
        return s;
      }
    }
    set({ vars: 'done' });
  }

  if (s.deploy !== 'done') {
    set({ deploy: 'running' });
    try {
      await post('/api/deploy', { project: slug });
      set({ deploy: 'done' });
    } catch (err) {
      set({ deploy: 'failed', error: { phase: 'deploy', ...errorInfo(err) } });
    }
  }
  return s;
}

/** Estados en los que un despliegue ya no va a cambiar: se deja de preguntar. */
export function isFinalStatus(status: DeployStatus | undefined): boolean {
  return status === 'ready' || status === 'error' || status === 'canceled';
}
