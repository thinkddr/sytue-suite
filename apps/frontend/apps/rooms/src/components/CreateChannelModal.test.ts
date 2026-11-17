// Checks de la pista de slug de «Crear canal»: debe coincidir con el domain.Slugify del
// servicio Go (services/chat/internal/domain/domain.go). Ejecutar: `bun test`.
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterAll, describe, expect, test } from 'bun:test';
import { h, render } from 'preact';
import { act } from 'preact/test-utils';
import { ChatHttpError, type Channel, type ChannelMember } from '../client/chatApi.js';
import { roomsT } from '../i18n/index.js';
import { addInitialMembers, channelSlug, CreateChannelModal } from './CreateChannelModal.js';

test('minúsculas y espacios a guion', () => {
  expect(channelSlug('Lanzamiento Otono')).toBe('lanzamiento-otono');
});

test('colapsa separadores repetidos y recorta los de los extremos', () => {
  expect(channelSlug('  __hola  -- mundo._  ')).toBe('hola-mundo');
});

test('descarta acentos y signos (como validSlugRune del backend)', () => {
  expect(channelSlug('Diseño & Móvil!')).toBe('diseo-mvil');
});

test('los dígitos sobreviven; el guion no abre el slug', () => {
  expect(channelSlug('-2026 Q3-')).toBe('2026-q3');
});

test('sin caracteres válidos → cadena vacía (el botón queda deshabilitado)', () => {
  expect(channelSlug('¿¡ ***')).toBe('');
  expect(channelSlug('   ')).toBe('');
});

test('recorta a 64 y no deja guion final', () => {
  const long = channelSlug(`${'a'.repeat(64)} b`);
  expect(long).toBe('a'.repeat(64));
  // El corte cae justo sobre el guion: se recorta también.
  expect(channelSlug(`${'a'.repeat(63)} bcd`)).toBe('a'.repeat(63));
});

// ---- Personas iniciales (pieza ROOM-12) ----
//
// Un canal nace ya con gente: tras crearlo se añade a cada persona por el mismo camino
// que el panel de Miembros (POST /api/channel-members), nunca con el `members[]` de la
// creación. Si una falla, el canal se queda y el padre sabe a quién le faltó.
// Gemelo en Fyne: TestCrearCanalConPersonas (apps/fyne/apps/rooms).

const ana: ChannelMember = { sub: 'ana', name: 'Ana', role: 'member' } as ChannelMember;
const beto: ChannelMember = { sub: 'beto', name: 'Beto', role: 'member' } as ChannelMember;
const yo: ChannelMember = { sub: 'yo', name: 'Yo', role: 'member' } as ChannelMember;

test('addInitialMembers añade a cada una en orden y devuelve solo las que fallaron', async () => {
  const vistas: string[] = [];
  const faltan = await addInitialMembers('c1', [ana, beto], (canal, sub, nombre) => {
    vistas.push(`${canal}:${sub}:${nombre}`);
    return sub === 'ana'
      ? Promise.reject(new ChatHttpError(403, 'no'))
      : Promise.resolve({ member: beto });
  });
  // Un fallo no corta a las demás.
  expect(vistas).toEqual(['c1:ana:Ana', 'c1:beto:Beto']);
  expect(faltan).toEqual([ana]);
});

test('sin personas no hay ninguna petición', async () => {
  let llamadas = 0;
  const faltan = await addInitialMembers('c1', [], () => {
    llamadas++;
    return Promise.resolve({ member: ana });
  });
  expect(llamadas).toBe(0);
  expect(faltan).toEqual([]);
});

describe('el diálogo', () => {
  const registrado = typeof document === 'undefined';
  if (registrado) GlobalRegistrator.register();
  const fetchOriginal = globalThis.fetch;
  afterAll(async () => {
    globalThis.fetch = fetchOriginal;
    if (registrado) await GlobalRegistrator.unregister();
  });

  async function esperar(): Promise<void> {
    for (let i = 0; i < 5; i++) {
      await act(async () => {
        await new Promise((r) => setTimeout(r, 0));
      });
    }
  }

  test('crea el canal privado con dos personas y dice cuál faltó', async () => {
    const t = roomsT('es');
    const pedidas: { url: string; body: Record<string, unknown> }[] = [];
    globalThis.fetch = (async (url: string, init?: RequestInit) => {
      const body = JSON.parse(String(init?.body)) as Record<string, unknown>;
      pedidas.push({ url: String(url), body });
      if (url === '/api/channels') {
        return Response.json({ channel: { id: 'c9', name: 'ventas', kind: 'private' } });
      }
      if (body.subject === 'beto') return new Response('{}', { status: 500 });
      return Response.json({ member: { sub: body.subject, name: body.name, role: 'member' } });
    }) as typeof fetch;

    let creado: { channel: Channel; missing: ChannelMember[] } | null = null;
    const host = document.createElement('div');
    document.body.appendChild(host);
    try {
      await act(async () => {
        render(
          h(CreateChannelModal, {
            open: true,
            onClose: () => undefined,
            members: [yo, ana, beto],
            selfId: 'yo',
            onCreated: (channel: Channel, missing: ChannelMember[]) => {
              creado = { channel, missing };
            },
            t,
          }),
          host,
        );
      });
      const opciones = () => [...document.querySelectorAll<HTMLButtonElement>('[role="option"]')];
      // Quien crea ya está dentro: no se le ofrece.
      expect(opciones().map((o) => o.textContent?.trim())).toEqual(['Ana', 'Beto']);

      const nombre = document.querySelector<HTMLInputElement>('input[maxlength="100"]')!;
      await act(async () => {
        nombre.value = 'Ventas';
        nombre.dispatchEvent(new Event('input', { bubbles: true }));
      });
      await act(async () => {
        document.querySelectorAll<HTMLInputElement>('input[type="radio"]')[1]!.click();
      });
      for (const o of opciones()) {
        await act(async () => {
          o.click();
        });
      }
      expect(opciones().every((o) => o.getAttribute('aria-selected') === 'true')).toBe(true);

      const crear = [...document.querySelectorAll<HTMLButtonElement>('button')].find(
        (b) => b.textContent?.trim() === t('createChannel.title'),
      )!;
      await act(async () => {
        crear.click();
      });
      await esperar();

      expect(pedidas.map((p) => p.url)).toEqual([
        '/api/channels',
        '/api/channel-members',
        '/api/channel-members',
      ]);
      // La creación va sin `members[]`: las personas entran una a una, validadas.
      expect(pedidas[0]!.body).toEqual({ name: 'Ventas', kind: 'private' });
      expect(pedidas.slice(1).map((p) => p.body)).toEqual([
        { channelId: 'c9', subject: 'ana', name: 'Ana' },
        { channelId: 'c9', subject: 'beto', name: 'Beto' },
      ]);
      // El canal se queda creado aunque Beto no entrase, y se sabe que faltó él.
      expect(creado).not.toBeNull();
      expect(creado!.channel.id).toBe('c9');
      expect(creado!.missing.map((m) => m.name)).toEqual(['Beto']);
    } finally {
      render(null, host);
      host.remove();
    }
  });
});
