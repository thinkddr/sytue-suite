/**
 * GET /api/shared — «Compartido conmigo»: las notas que otras personas han compartido
 * con quien pregunta, con el nombre de quien las compartió (el servicio solo sabe su
 * `sub`; aquí se le pone cara con el directorio de la organización).
 */
import type { APIRoute } from 'astro';
import { crearDirectorio } from '@sytue/auth/server';
import { bff } from '../../lib/bff.js';
import { conCliente, fallo, json } from '../../lib/notes.js';

export const prerender = false;

const { resolverPersonas } = crearDirectorio(bff, import.meta.env);

export const GET: APIRoute = async (ctx) => {
  const c = await conCliente(ctx);
  if (c instanceof Response) return c;
  try {
    const notas = await c.client.listShared();
    const caras = await resolverPersonas(
      ctx,
      notas.map((n) => n.shared_by),
    );
    return json({
      notes: notas.map((n) => {
        const p = caras.get(n.shared_by);
        const nombre = p?.name ?? p?.email;
        return nombre ? { ...n, shared_by_name: nombre } : n;
      }),
    });
  } catch (err) {
    return fallo(err);
  }
};
