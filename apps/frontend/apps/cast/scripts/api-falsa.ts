/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * API FALSA de Sytue Cast, para desarrollar la web y sacar capturas sin el servicio
 * Go, Postgres, S3 ni LiveKit. Contesta el MISMO cable que
 * `services/cast/internal/http` (snake_case, sobre de error `{error:{code,…}}`) con
 * datos verosímiles e inventados, y NO forma parte del build: solo se arranca a mano.
 *
 *   bun apps/cast/scripts/api-falsa.ts            # :8099 (como el servicio)
 *   PUERTO=8199 bun apps/cast/scripts/api-falsa.ts
 *
 * y la app contra ella:
 *
 *   SYTUE_CAST_API_URL=http://localhost:8099 SYTUE_CAST_WS_URL=ws://localhost:8099 \
 *     bun --bun astro dev --port 4335
 *
 * Con ffmpeg en el PATH genera una vez (en el directorio temporal) pósteres JPEG y
 * un clip HLS de 12 s para que el reproductor tenga algo que reproducir; sin
 * ffmpeg, los pósteres salen en SVG y el reproductor dice que no puede reproducir.
 *
 * Cualquier `Authorization: Bearer …` vale: quien llama es siempre Ana Pérez, de la
 * organización `corporate`. Lo que se crea (subidas, directos, comentarios) vive en
 * memoria hasta que se para el proceso.
 */
import { existsSync, mkdirSync, readFileSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { spawnSync } from 'node:child_process';

const PUERTO = Number(process.env['PUERTO'] ?? 8099);
const DIR = process.env['CAST_FALSA_DIR'] ?? join(tmpdir(), 'sytue-cast-api-falsa');
mkdirSync(join(DIR, 'hls'), { recursive: true });

// ── Pósteres: composiciones planas, sin degradados, una paleta por vídeo ──────

type Motivo = 'montes' | 'mar' | 'ciudad' | 'sala' | 'bosque';
const PALETAS: Record<string, string[]> = {
  arena: ['#e9dcc4', '#c9a877', '#8a6a44', '#5b4630', '#2f261c'],
  pizarra: ['#d7dde3', '#9fb0bf', '#627589', '#3b4b5c', '#1f2a36'],
  vino: ['#efdcdc', '#c98a92', '#9f1239', '#5c1024', '#2a0a12'],
  salvia: ['#e3e8dc', '#adbf9f', '#6f8a63', '#46603f', '#243322'],
  noche: ['#1b2130', '#2c3650', '#46557a', '#e8c872', '#0e121b'],
  terracota: ['#f1e2d3', '#e0a47a', '#b8643a', '#7a3b22', '#3a1d12'],
};

function poster(motivo: Motivo, paleta: string): string {
  const [c0, c1, c2, c3, c4] = PALETAS[paleta]!;
  const w = 1280;
  const h = 720;
  let cuerpo = '';
  if (motivo === 'montes') {
    cuerpo = `<rect width="${w}" height="${h}" fill="${c0}"/><circle cx="930" cy="230" r="90" fill="${c1}"/>
<polygon points="0,520 220,300 420,470 640,250 900,500 1100,340 1280,460 1280,720 0,720" fill="${c2}"/>
<polygon points="0,600 300,430 520,580 760,420 1020,600 1280,520 1280,720 0,720" fill="${c3}"/>
<rect y="650" width="${w}" height="70" fill="${c4}"/>`;
  } else if (motivo === 'mar') {
    cuerpo = `<rect width="${w}" height="${h}" fill="${c0}"/><circle cx="640" cy="380" r="120" fill="${c1}"/>
<rect y="420" width="${w}" height="300" fill="${c2}"/>
<rect x="340" y="470" width="600" height="10" rx="5" fill="${c1}"/><rect x="440" y="510" width="400" height="8" rx="4" fill="${c1}"/>
<rect x="540" y="545" width="200" height="6" rx="3" fill="${c1}"/><rect y="640" width="${w}" height="80" fill="${c3}"/>`;
  } else if (motivo === 'ciudad') {
    const edificios = [
      [60, 300, 140],
      [220, 220, 120],
      [360, 340, 160],
      [540, 180, 110],
      [670, 280, 150],
      [840, 240, 120],
      [980, 320, 140],
      [1140, 200, 110],
    ]
      .map(([x, y, a]) => `<rect x="${x}" y="${y}" width="${a}" height="${h - y!}" fill="${c3}"/>`)
      .join('');
    const ventanas = Array.from({ length: 36 }, (_, i) => {
      const x = 80 + (i % 12) * 100;
      const y = 360 + Math.floor(i / 12) * 90;
      return i % 3 === 0 ? `<rect x="${x}" y="${y}" width="18" height="26" fill="${c1}"/>` : '';
    }).join('');
    cuerpo = `<rect width="${w}" height="${h}" fill="${c0}"/><circle cx="1050" cy="150" r="60" fill="${c1}"/>
<g>${edificios}</g>${ventanas}<rect y="660" width="${w}" height="60" fill="${c4}"/>`;
  } else if (motivo === 'sala') {
    cuerpo = `<rect width="${w}" height="${h}" fill="${c0}"/>
<rect x="240" y="110" width="800" height="420" rx="12" fill="${c4}"/>
<rect x="300" y="170" width="300" height="24" rx="6" fill="${c1}"/>
<rect x="300" y="230" width="520" height="14" rx="7" fill="${c2}"/><rect x="300" y="262" width="460" height="14" rx="7" fill="${c2}"/>
<rect x="680" y="330" width="60" height="140" fill="${c1}"/><rect x="760" y="280" width="60" height="190" fill="${c2}"/><rect x="840" y="370" width="60" height="100" fill="${c1}"/>
<rect x="120" y="600" width="1040" height="120" fill="${c3}"/>
<circle cx="300" cy="600" r="46" fill="${c2}"/><circle cx="520" cy="610" r="46" fill="${c2}"/><circle cx="760" cy="600" r="46" fill="${c2}"/><circle cx="980" cy="610" r="46" fill="${c2}"/>`;
  } else {
    const arboles = Array.from({ length: 11 }, (_, i) => {
      const x = 40 + i * 120;
      const alto = 260 + ((i * 97) % 180);
      return `<polygon points="${x},${h - 90} ${x + 60},${h - 90 - alto} ${x + 120},${h - 90}" fill="${i % 2 ? c3 : c2}"/>`;
    }).join('');
    cuerpo = `<rect width="${w}" height="${h}" fill="${c0}"/><circle cx="240" cy="190" r="70" fill="${c1}"/>${arboles}<rect y="${h - 90}" width="${w}" height="90" fill="${c4}"/>`;
  }
  return `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 ${w} ${h}" width="${w}" height="${h}">${cuerpo}</svg>`;
}

const hayFfmpeg = spawnSync('ffmpeg', ['-version']).status === 0;

/** Póster en JPEG (con ffmpeg) o en SVG; devuelve [ruta, tipo]. */
function prepararPoster(clave: string, motivo: Motivo, paleta: string): [string, string] {
  const svg = join(DIR, `${clave}.svg`);
  const jpg = join(DIR, `${clave}.jpg`);
  if (!existsSync(svg)) writeFileSync(svg, poster(motivo, paleta));
  if (hayFfmpeg && !existsSync(jpg)) {
    spawnSync('ffmpeg', ['-y', '-loglevel', 'error', '-i', svg, '-q:v', '4', jpg]);
  }
  return existsSync(jpg) ? [jpg, 'image/jpeg'] : [svg, 'image/svg+xml'];
}

/** Un clip HLS de 12 s: un póster con un barrido lento y un tono suave. */
function prepararHls(): boolean {
  const master = join(DIR, 'hls', 'master.m3u8');
  if (existsSync(master)) return true;
  if (!hayFfmpeg) return false;
  const [img] = prepararPoster('clip', 'montes', 'arena');
  const r = spawnSync('ffmpeg', [
    '-y',
    '-loglevel',
    'error',
    '-loop',
    '1',
    '-i',
    img,
    '-f',
    'lavfi',
    '-i',
    'sine=frequency=220:sample_rate=48000',
    '-filter_complex',
    "[0:v]scale=1600:900,zoompan=z='1.0+0.0015*on':d=300:s=1280x720:fps=25[v]",
    '-map',
    '[v]',
    '-map',
    '1:a',
    '-t',
    '12',
    '-c:v',
    'libx264',
    '-preset',
    'veryfast',
    '-pix_fmt',
    'yuv420p',
    '-c:a',
    'aac',
    '-b:a',
    '96k',
    '-f',
    'hls',
    '-hls_time',
    '4',
    '-hls_playlist_type',
    'vod',
    '-hls_segment_type',
    'fmp4',
    '-master_pl_name',
    'master.m3u8',
    join(DIR, 'hls', 'stream.m3u8'),
  ]);
  return r.status === 0 && existsSync(master);
}

// ── Datos ────────────────────────────────────────────────────────────────────

// Los títulos y descripciones de los vídeos de ejemplo van aparte, como datos: son
// contenido de quien sube, no texto de la interfaz.
const TEXTOS = JSON.parse(
  readFileSync(new URL('./datos-falsos.json', import.meta.url), 'utf8'),
) as { title: string; description?: string }[];

const YO = { sub: 'u-ana', name: 'Ana Pérez' };
const PERSONAS = {
  jonas: { sub: 'u-jonas', name: 'Jonas Weber' },
  marta: { sub: 'u-marta', name: 'Marta Kowalska' },
  luca: { sub: 'u-luca', name: 'Luca Bianchi' },
  ines: { sub: 'u-ines', name: 'Inès Laurent' },
  sofia: { sub: 'u-sofia', name: 'Sofia Andersson' },
};

interface Video {
  id: string;
  tenant: string;
  owner: { sub: string; name: string };
  kind: 'upload' | 'live';
  status: string;
  visibility: string;
  title: string;
  description: string;
  duration_ms: number | null;
  width: number | null;
  height: number | null;
  poster_url: string | null;
  comments_enabled: boolean;
  chat_enabled: boolean;
  download_enabled: boolean;
  live: null | {
    source: string;
    started_at: string | null;
    ended_at: string | null;
    viewers_now: number;
    viewers_peak: number;
  };
  views: number;
  failure: string;
  published_at: string | null;
  created_at: string;
  updated_at: string;
  _motivo: Motivo;
  _paleta: string;
}

const hace = (min: number): string => new Date(Date.now() - min * 60_000).toISOString();
let serie = 0;
const nuevoId = (): string => {
  serie++;
  const h = serie.toString(16).padStart(12, '0');
  return `7b0c${h.slice(0, 4)}-0000-4000-8000-${h}`;
};

function v(p: Partial<Video> & Pick<Video, 'title' | 'owner'>): Video {
  const id = nuevoId();
  const creado = p.created_at ?? hace(60 * 24 * (serie % 9) + serie * 17);
  const base: Video = {
    id,
    tenant: 'corporate',
    kind: 'upload',
    status: 'ready',
    visibility: 'org',
    description: '',
    duration_ms: 60_000 * (3 + (serie % 17)) + serie * 7_000,
    width: 1920,
    height: 1080,
    poster_url: null,
    comments_enabled: true,
    chat_enabled: true,
    download_enabled: false,
    live: null,
    views: 40 + ((serie * 131) % 900),
    failure: '',
    published_at: creado,
    created_at: creado,
    updated_at: creado,
    _motivo: (['montes', 'mar', 'ciudad', 'sala', 'bosque'] as Motivo[])[serie % 5]!,
    _paleta: Object.keys(PALETAS)[serie % 6]!,
    ...p,
  };
  if (base.status === 'ready' || base.status === 'live' || base.status === 'ended')
    base.poster_url = `/media/${id}/tok/poster.jpg`;
  return base;
}

const VIDEOS: Video[] = [
  v({
    ...TEXTOS[0]!,
    owner: PERSONAS.jonas,
    kind: 'live',
    status: 'live',
    visibility: 'public',
    duration_ms: null,
    live: {
      source: 'rtmp',
      started_at: hace(23),
      ended_at: null,
      viewers_now: 148,
      viewers_peak: 173,
    },
    _motivo: 'sala',
    _paleta: 'pizarra',
  }),
  v({
    ...TEXTOS[1]!,
    owner: YO,
    visibility: 'org',
    _motivo: 'sala',
    _paleta: 'vino',
    views: 312,
    created_at: hace(60 * 26),
  }),
  v({
    ...TEXTOS[2]!,
    owner: YO,
    visibility: 'public',
    _motivo: 'ciudad',
    _paleta: 'terracota',
    views: 1204,
    download_enabled: true,
    created_at: hace(60 * 50),
  }),
  v({
    ...TEXTOS[3]!,
    owner: YO,
    visibility: 'people',
    _motivo: 'sala',
    _paleta: 'salvia',
    views: 58,
    created_at: hace(60 * 75),
  }),
  v({
    ...TEXTOS[4]!,
    owner: YO,
    status: 'processing',
    visibility: 'unlisted',
    duration_ms: null,
    views: 0,
    created_at: hace(4),
  }),
  v({
    ...TEXTOS[5]!,
    owner: PERSONAS.marta,
    visibility: 'org',
    _motivo: 'bosque',
    _paleta: 'salvia',
    created_at: hace(60 * 5),
  }),
  v({
    ...TEXTOS[6]!,
    owner: PERSONAS.luca,
    visibility: 'public',
    _motivo: 'mar',
    _paleta: 'pizarra',
    views: 2381,
    created_at: hace(60 * 30),
  }),
  v({
    ...TEXTOS[7]!,
    owner: PERSONAS.ines,
    visibility: 'public',
    tenant: 'nordlicht',
    _motivo: 'montes',
    _paleta: 'noche',
    views: 5120,
    created_at: hace(60 * 24 * 6),
  }),
  v({
    ...TEXTOS[8]!,
    owner: PERSONAS.sofia,
    visibility: 'public',
    tenant: 'nordlicht',
    _motivo: 'ciudad',
    _paleta: 'noche',
    views: 887,
    created_at: hace(60 * 24 * 3),
  }),
  v({
    ...TEXTOS[9]!,
    owner: PERSONAS.jonas,
    visibility: 'public',
    _motivo: 'montes',
    _paleta: 'salvia',
    views: 3410,
    created_at: hace(60 * 24 * 12),
  }),
  v({
    ...TEXTOS[10]!,
    owner: PERSONAS.sofia,
    visibility: 'public',
    tenant: 'nordlicht',
    _motivo: 'bosque',
    _paleta: 'arena',
    views: 460,
    created_at: hace(60 * 24 * 9),
  }),
  v({
    ...TEXTOS[11]!,
    owner: YO,
    kind: 'live',
    status: 'scheduled',
    visibility: 'org',
    duration_ms: null,
    live: { source: 'webrtc', started_at: null, ended_at: null, viewers_now: 0, viewers_peak: 0 },
    views: 0,
    created_at: hace(90),
  }),
  v({
    ...TEXTOS[12]!,
    owner: YO,
    status: 'failed',
    visibility: 'org',
    duration_ms: null,
    views: 0,
    failure: 'The original file has no video stream.',
    created_at: hace(60 * 3),
  }),
];

const COMENTARIOS: Record<string, unknown[]> = {};
const comentario = (
  vid: string,
  autor: { sub: string; name: string },
  body: string,
  min: number,
  reply_to?: string,
) => {
  const c = {
    id: crypto.randomUUID(),
    author: { ...autor, username: autor.name.split(' ')[0]!.toLowerCase() },
    body,
    created_at: hace(min),
    can: { delete: autor.sub === YO.sub },
    ...(reply_to ? { reply_to } : {}),
  };
  (COMENTARIOS[vid] ??= []).push(c);
  return c.id;
};
{
  const sede = VIDEOS[2]!.id;
  const a = comentario(
    sede,
    PERSONAS.luca,
    'La terraza es una maravilla. ¿Se puede reservar para reuniones de equipo?',
    600,
  );
  comentario(sede, YO, 'Sí: desde la semana que viene aparece en el calendario de salas.', 540, a);
  comentario(
    sede,
    PERSONAS.marta,
    'Qué luz tiene la biblioteca. Muy buen trabajo con la grabación.',
    300,
  );
}

const CHAT = [
  {
    id: crypto.randomUUID(),
    author: PERSONAS.marta,
    body: '¿Habrá versión para tabletas este año?',
    created_at: hace(12),
  },
  {
    id: crypto.randomUUID(),
    author: PERSONAS.luca,
    body: 'Buenísima la parte de la hoja de ruta.',
    created_at: hace(9),
  },
  {
    id: crypto.randomUUID(),
    author: PERSONAS.sofia,
    body: 'Desde Estocolmo se ve y se oye perfecto.',
    created_at: hace(6),
  },
  {
    id: crypto.randomUUID(),
    author: PERSONAS.ines,
    body: '¿Se podrá ver la grabación luego?',
    created_at: hace(2),
  },
];

let clave = {
  rtmp_url: 'rtmp://cast-ingest.sytue.eu/live',
  stream_key: 'sk_7Fq2mW9xLp4TzR8cVb3N',
  ingress_id: 'IN_a1b2c3',
};
const PERSONAS_CON_ACCESO: Record<string, unknown[]> = {
  [VIDEOS[3]!.id]: [
    { user_id: 'u-jonas', name: 'Jonas Weber', username: 'jonas', added_at: hace(3000) },
    { user_id: 'u-sofia', name: 'Sofia Andersson', username: 'sofia', added_at: hace(2900) },
  ],
};

// ── Respuestas ───────────────────────────────────────────────────────────────

const dto = (x: Video, conSesion: boolean) => {
  const { _motivo: _m, _paleta: _p, ...resto } = x;
  const edita = conSesion && x.owner.sub === YO.sub;
  return {
    ...resto,
    failure: edita ? x.failure : '',
    can: { edit: edita, delete: edita, moderate: edita },
  };
};
const json = (cuerpo: unknown, status = 200) =>
  new Response(JSON.stringify(cuerpo), { status, headers: { 'content-type': 'application/json' } });
const falla = (status: number, code: string, message: string, details?: unknown) =>
  json({ error: { code, message, ...(details ? { details } : {}) } }, status);
const buscar = (id: string) => VIDEOS.find((x) => x.id === id);
const abierto = (x: Video) => x.visibility === 'public' || x.visibility === 'unlisted';
const visible = (x: Video) => x.status === 'ready' || x.status === 'live';

function play(x: Video, conSesion: boolean) {
  const hls = prepararHls();
  return {
    kind: x.kind,
    hls_url: x.status === 'ready' && hls ? `/media/${x.id}/tok/hls/master.m3u8` : null,
    webm_url: null,
    poster_url: x.poster_url,
    ...(x.download_enabled || (conSesion && x.owner.sub === YO.sub)
      ? { download_url: `/media/${x.id}/tok/original` }
      : {}),
    duration_ms: x.duration_ms,
    ...(x.status === 'live'
      ? {
          live: {
            server_url: 'ws://127.0.0.1:7880',
            token: 'lk-falso',
            room: `cast-${x.tenant}-${x.id}`,
            identity: 'anon-0f3a',
          },
        }
      : {}),
  };
}

function publico(ruta: string, url: URL): Response {
  if (ruta === '/explore') {
    const pub = VIDEOS.filter((x) => x.visibility === 'public' && visible(x));
    return json({
      live: pub.filter((x) => x.status === 'live').map((x) => dto(x, false)),
      videos: pub.filter((x) => x.status === 'ready').map((x) => dto(x, false)),
      next_cursor: '',
    });
  }
  let m = ruta.match(/^\/c\/([a-z0-9-]+)$/);
  if (m) {
    const lista = VIDEOS.filter(
      (x) => x.tenant === m![1] && x.visibility === 'public' && visible(x),
    );
    if (!lista.length && !VIDEOS.some((x) => x.tenant === m![1]))
      return falla(404, 'not_found', 'Not found.');
    const nombres: Record<string, string> = {
      corporate: 'Corporate',
      nordlicht: 'Nordlicht Studio',
    };
    return json({
      org: m[1],
      name: nombres[m[1]!] ?? m[1],
      videos: lista.map((x) => dto(x, false)),
      next_cursor: '',
    });
  }
  m = ruta.match(/^\/videos\/([^/]+)(\/play|\/comments)?$/);
  if (m) {
    const x = buscar(m[1]!);
    if (!x || !abierto(x) || !visible(x)) return falla(404, 'not_found', 'Not found.');
    if (m[2] === '/play') return json(play(x, false));
    if (m[2] === '/comments')
      return json({
        comments: (COMENTARIOS[x.id] ?? []).map((c) => ({
          ...(c as object),
          can: { delete: false },
        })),
        next_cursor: '',
      });
    return json(dto(x, false));
  }
  if (ruta === '/oembed') {
    const id = (url.searchParams.get('url') ?? '').split('/').pop() ?? '';
    const x = buscar(id);
    if (!x || !abierto(x)) return falla(404, 'not_found', 'That URL is not a Sytue Cast video.');
    return json({
      version: '1.0',
      type: 'video',
      provider_name: 'Sytue Cast',
      provider_url: 'https://cast.sytue.eu',
      title: x.title,
      author_name: x.owner.name,
      width: 640,
      height: 360,
      html: `<iframe src="https://cast.sytue.eu/cast/embed/${x.id}" width="640" height="360" frameborder="0" allowfullscreen title="${x.title}"></iframe>`,
    });
  }
  return falla(404, 'not_found', 'Not found.');
}

async function conSesion(req: Request, ruta: string, url: URL): Promise<Response> {
  const metodo = req.method;
  if (ruta === '/videos' && metodo === 'GET') {
    const scope = url.searchParams.get('scope');
    const status = url.searchParams.get('status');
    const q = (url.searchParams.get('q') ?? '').toLowerCase();
    let lista = VIDEOS.filter(
      (x) => x.tenant === 'corporate' && (x.owner.sub === YO.sub || x.visibility !== 'people'),
    );
    if (scope === 'mine') lista = lista.filter((x) => x.owner.sub === YO.sub);
    if (status) lista = lista.filter((x) => x.status === status);
    if (q) lista = lista.filter((x) => x.title.toLowerCase().includes(q));
    return json({ videos: lista.map((x) => dto(x, true)) });
  }
  if (ruta === '/videos/shared') return json({ videos: [dto(VIDEOS[8]!, true)] });
  if (ruta === '/videos' && metodo === 'POST') {
    const b = (await req.json()) as { title: string; visibility?: string; description?: string };
    const x = v({
      title: b.title,
      owner: YO,
      status: 'uploading',
      visibility: b.visibility ?? 'org',
      description: b.description ?? '',
      duration_ms: null,
      views: 0,
      created_at: hace(0),
    });
    VIDEOS.unshift(x);
    return json(dto(x, true), 201);
  }
  if (ruta === '/quota')
    return json({
      limit_bytes: 200 * 1024 ** 3,
      used_bytes: 61.4 * 1024 ** 3,
      available_bytes: 138.6 * 1024 ** 3,
      cast_bytes: 18.2 * 1024 ** 3,
    });
  if (ruta === '/studio/stream-key') return json(clave);
  if (ruta === '/studio/stream-key:rotate' && metodo === 'POST') {
    clave = { ...clave, stream_key: `sk_${crypto.randomUUID().replace(/-/g, '').slice(0, 20)}` };
    return json(clave);
  }
  if (ruta === '/lives' && metodo === 'POST') {
    const b = (await req.json()) as { title: string; visibility?: string; source: string };
    const x = v({
      title: b.title,
      owner: YO,
      kind: 'live',
      status: 'scheduled',
      visibility: b.visibility ?? 'org',
      duration_ms: null,
      views: 0,
      live: { source: b.source, started_at: null, ended_at: null, viewers_now: 0, viewers_peak: 0 },
      created_at: hace(0),
    });
    VIDEOS.unshift(x);
    return json(
      {
        video: dto(x, true),
        ...(b.source === 'webrtc'
          ? {
              publish: {
                server_url: 'ws://127.0.0.1:7880',
                token: 'lk-emisor',
                room: `cast-corporate-${x.id}`,
                identity: 'emisor-u-ana',
              },
            }
          : {}),
      },
      201,
    );
  }
  let m = ruta.match(/^\/lives\/([^/]+)\/(publish|end)$/);
  if (m) {
    const x = buscar(m[1]!);
    if (!x) return falla(404, 'not_found', 'Not found.');
    if (m[2] === 'end') {
      x.status = 'ended';
      return json(dto(x, true));
    }
    return json({
      server_url: 'ws://127.0.0.1:7880',
      token: 'lk-emisor',
      room: `cast-corporate-${x.id}`,
      identity: 'emisor-u-ana',
    });
  }
  m = ruta.match(/^\/videos\/([^/]+)(\/[a-z_:]+)?(?:\/([^/]+))?$/);
  if (m) {
    const x = buscar(m[1]!);
    if (!x || (x.visibility === 'people' && x.owner.sub !== YO.sub))
      return falla(404, 'not_found', 'Not found.');
    const sub = m[2] ?? '';
    if (sub === '' && metodo === 'GET') return json(dto(x, true));
    if (sub === '' && metodo === 'PATCH') {
      Object.assign(x, await req.json(), { updated_at: hace(0) });
      return json(dto(x, true));
    }
    if (sub === '' && metodo === 'DELETE') {
      VIDEOS.splice(VIDEOS.indexOf(x), 1);
      return new Response(null, { status: 204 });
    }
    if (sub === '/original' && metodo === 'PUT') {
      const largo = Number(req.headers.get('content-length') ?? 0);
      if (!largo) return falla(411, 'length_required', 'Content-Length is required.');
      await req.arrayBuffer();
      x.status = 'processing';
      return json(dto(x, true));
    }
    if (sub === '/play') return json(play(x, true));
    if (sub === '/poster' && metodo === 'PUT') {
      await req.arrayBuffer();
      return json(dto(x, true));
    }
    if (sub === '/poster:frame') return json(dto(x, true), 202);
    if (sub === '/people' && metodo === 'GET')
      return json({ people: PERSONAS_CON_ACCESO[x.id] ?? [] });
    if (sub === '/people' && metodo === 'POST') {
      const { query } = (await req.json()) as { query: string };
      const u = query.replace(/^@/, '').toLowerCase();
      const p = Object.values(PERSONAS).find((y) => y.name.toLowerCase().startsWith(u));
      if (!p) return falla(404, 'not_found', 'Nobody found.');
      const fila = {
        user_id: p.sub,
        name: p.name,
        username: p.name.split(' ')[0]!.toLowerCase(),
        added_at: hace(0),
      };
      (PERSONAS_CON_ACCESO[x.id] ??= []).push(fila);
      return json(fila, 201);
    }
    if (sub === '/people' && metodo === 'DELETE') {
      PERSONAS_CON_ACCESO[x.id] = (PERSONAS_CON_ACCESO[x.id] ?? []).filter(
        (p) => (p as { user_id: string }).user_id !== m[3],
      );
      return new Response(null, { status: 204 });
    }
    if (sub === '/comments' && metodo === 'GET')
      return json({
        comments: (COMENTARIOS[x.id] ?? []).map((c) => ({
          ...(c as object),
          can: { delete: true },
        })),
        next_cursor: '',
      });
    if (sub === '/comments' && metodo === 'POST') {
      const b = (await req.json()) as { body: string; reply_to?: string };
      const id = comentario(x.id, YO, b.body, 0, b.reply_to);
      return json(
        (COMENTARIOS[x.id] ?? []).find((c) => (c as { id: string }).id === id),
        201,
      );
    }
    if (sub === '/comments' && metodo === 'DELETE') return new Response(null, { status: 204 });
    if (sub === '/chat' && metodo === 'GET') return json({ messages: CHAT });
    if (sub === '/chat' && metodo === 'DELETE') return new Response(null, { status: 204 });
  }
  return falla(404, 'not_found', 'Not found.');
}

function media(ruta: string, req: Request): Response {
  const m = ruta.match(/^\/media\/([^/]+)\/[^/]+\/(.+)$/);
  if (!m) return falla(404, 'not_found', 'Not found.');
  const x = buscar(m[1]!);
  if (m[2] === 'poster.jpg' && x) {
    const [f, tipo] = prepararPoster(x.id, x._motivo, x._paleta);
    return new Response(readFileSync(f), {
      headers: { 'content-type': tipo, 'cache-control': 'private, max-age=3600' },
    });
  }
  if (m[2]!.startsWith('hls/') && prepararHls()) {
    const f = join(DIR, 'hls', m[2]!.slice(4).replace(/\.\./g, ''));
    if (!existsSync(f)) return falla(404, 'not_found', 'Not found.');
    const tipo = f.endsWith('.m3u8')
      ? 'application/vnd.apple.mpegurl'
      : f.endsWith('.mp4')
        ? 'video/mp4'
        : 'video/iso.segment';
    return new Response(req.method === 'HEAD' ? null : readFileSync(f), {
      headers: { 'content-type': tipo },
    });
  }
  return falla(404, 'not_found', 'Not found.');
}

Bun.serve({
  port: PUERTO,
  async fetch(req, server) {
    const url = new URL(req.url);
    const ruta = url.pathname;
    if (ruta.startsWith('/ws/videos/')) {
      if (!url.searchParams.get('access_token'))
        return falla(401, 'unauthorized', 'Missing token.');
      return server.upgrade(req) ? undefined : falla(400, 'bad_request', 'Upgrade failed.');
    }
    if (ruta.startsWith('/media/')) return media(ruta, req);
    if (ruta.startsWith('/api/v1/public/'))
      return publico(ruta.slice('/api/v1/public'.length), url);
    if (ruta.startsWith('/public/')) return publico(ruta.slice('/public'.length), url);
    if (ruta.startsWith('/api/v1/')) {
      if (!req.headers.get('authorization')?.startsWith('Bearer '))
        return falla(401, 'unauthorized', 'Missing token.');
      return conSesion(req, ruta.slice('/api/v1'.length), url);
    }
    return falla(404, 'not_found', 'Not found.');
  },
  websocket: {
    open(ws) {
      ws.send(JSON.stringify({ type: 'viewers', count: 148 }));
    },
    message(ws, datos) {
      try {
        const o = JSON.parse(String(datos)) as { type: string; body?: string };
        if (o.type === 'send' && o.body) {
          const m = {
            id: crypto.randomUUID(),
            author: YO,
            body: o.body,
            created_at: new Date().toISOString(),
          };
          CHAT.push(m);
          ws.send(JSON.stringify({ type: 'message', ...m }));
        }
      } catch {
        ws.send(JSON.stringify({ type: 'error', code: 'bad_request', message: 'Unknown order.' }));
      }
    },
  },
});

process.stdout.write(
  `api falsa de Sytue Cast en :${PUERTO} (medios en ${DIR}; ffmpeg: ${hayFfmpeg ? 'sí' : 'no'})\n`,
);
