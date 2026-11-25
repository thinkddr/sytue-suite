/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Buscar y ordenar webs, respuestas y pedidos: sin acentos ni mayúsculas, sin
 * reordenar la lista que se recibe y con los pedidos de prueba fuera cuando se pide.
 */
import { expect, test } from 'bun:test';
import {
  correoPlausible,
  existencias,
  filtrarEnvios,
  filtrarPedidos,
  filtrarProductos,
  filtrarYOrdenarSitios,
} from './listados.js';

test('webs: busca en nombre, slug y dirección, y ordena por nombre o por edición', () => {
  const sitios = [
    {
      name: 'Zapatería Íñigo',
      slug: 'zapateria',
      canonical_host: 'zapatos.example',
      updated_at: '2026-11-01T00:00:00Z',
    },
    {
      name: 'árbol verde',
      slug: 'arbol',
      canonical_host: null,
      updated_at: '2026-11-20T00:00:00Z',
    },
    { name: 'Bodega', slug: 'bodega', updated_at: '2026-11-10T00:00:00Z' },
  ];
  expect(filtrarYOrdenarSitios(sitios, 'inigo', 'recent', 'es').map((s) => s.slug)).toEqual([
    'zapateria',
  ]);
  expect(
    filtrarYOrdenarSitios(sitios, 'zapatos.example', 'recent', 'es').map((s) => s.slug),
  ).toEqual(['zapateria']);
  expect(filtrarYOrdenarSitios(sitios, '', 'name', 'es').map((s) => s.slug)).toEqual([
    'arbol',
    'bodega',
    'zapateria',
  ]);
  expect(filtrarYOrdenarSitios(sitios, '', 'recent', 'es').map((s) => s.slug)).toEqual([
    'arbol',
    'bodega',
    'zapateria',
  ]);
  // La lista recibida no cambia de orden.
  expect(sitios[0]!.slug).toBe('zapateria');
});

test('respuestas: se busca en cualquier valor, también números y casillas', () => {
  const envios = [
    { id: '1', data: { nombre: 'José', mensaje: 'Quiero una TARTA' } },
    { id: '2', data: { nombre: 'Eva', personas: 12, acepta: true } },
  ];
  expect(filtrarEnvios(envios, 'jose').map((e) => e.id)).toEqual(['1']);
  expect(filtrarEnvios(envios, 'tarta').map((e) => e.id)).toEqual(['1']);
  expect(filtrarEnvios(envios, '12').map((e) => e.id)).toEqual(['2']);
  expect(filtrarEnvios(envios, '  ').length).toBe(2);
});

test('pedidos: número, comprador, estado y sin los de prueba', () => {
  const pedidos = [
    {
      number: 'P-000001',
      test: false,
      status: 'paid',
      buyer_email: 'ana@x.test',
      buyer_name: 'Ana Núñez',
    },
    {
      number: 'P-000002',
      test: true,
      status: 'paid',
      buyer_email: 'qa@x.test',
      buyer_name: 'Prueba',
    },
    {
      number: 'P-000003',
      test: false,
      status: 'fulfilled',
      buyer_email: 'luis@x.test',
      buyer_name: 'Luis',
    },
  ];
  expect(
    filtrarPedidos(pedidos, { q: 'nunez', estado: '', sinPruebas: false }).map((p) => p.number),
  ).toEqual(['P-000001']);
  expect(
    filtrarPedidos(pedidos, { q: '', estado: 'paid', sinPruebas: true }).map((p) => p.number),
  ).toEqual(['P-000001']);
  expect(
    filtrarPedidos(pedidos, { q: '000003', estado: '', sinPruebas: false }).map((p) => p.number),
  ).toEqual(['P-000003']);
  expect(filtrarPedidos(pedidos, { q: '', estado: '', sinPruebas: false }).length).toBe(3);
});

test('correo plausible para la lista de avisos', () => {
  expect(correoPlausible('ventas@tienda.es')).toBe(true);
  expect(correoPlausible('ventas@tienda')).toBe(false);
  expect(correoPlausible('ventas tienda.es')).toBe(false);
});

test('catálogo: las existencias suman TODAS las variantes, y una sin límite manda', () => {
  const p = (
    stocks: Array<number | null>,
    extra: Partial<{ active: boolean; category: string }> = {},
  ) => ({
    name: { es: 'Camiseta', en: 'T-shirt' },
    category: extra.category ?? 'ropa',
    active: extra.active ?? true,
    variants: stocks.map((stock, i) => ({ sku: `CAM-${i}`, stock })),
  });
  // Talla M agotada y L con 8: el producto NO está agotado (antes se miraba la primera).
  expect(existencias(p([0, 8]))).toEqual({ total: 8, agotado: false, bajo: false });
  expect(existencias(p([0, 0]))).toEqual({ total: 0, agotado: true, bajo: false });
  expect(existencias(p([2, 1])).bajo).toBe(true);
  expect(existencias(p([0, null])).total).toBeNull();

  const agotado = p([0, 0]);
  const pocas = p([3]);
  const inactivo = p([20], { active: false, category: 'tazas' });
  const lista = [agotado, pocas, inactivo];
  expect(filtrarProductos(lista, '', 'agotados')).toEqual([agotado]);
  expect(filtrarProductos(lista, '', 'bajos')).toEqual([pocas]);
  expect(filtrarProductos(lista, '', 'inactivos')).toEqual([inactivo]);
  expect(filtrarProductos(lista, 't-shirt', '').length).toBe(3);
  expect(filtrarProductos(lista, 'tazas', '')).toEqual([inactivo]);
  expect(filtrarProductos(lista, 'cam-0', '').length).toBe(3);
});
