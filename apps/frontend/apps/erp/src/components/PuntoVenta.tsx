/* SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos */
import { useContext, useEffect, useRef, useState } from 'preact/hooks';
import type { JSX, TargetedSubmitEvent } from 'preact';
import { Button, Icon, Spinner, withBase, CARD_TONES, Radio, Popover } from '@sytue/ui';
import type { Locale } from '@sytue/i18n';
import { mutateOfflineValue, readOfflineValue } from '@sytue/offline';
import type { T } from '../i18n/index.js';
import { api, baseDeLaApp, consulta, ErrorApi, mensajeDeError } from '../lib/api.js';
import { aCentimos, desglosar, formatearDinero, nombreDe } from './formato.js';
import type { Pagina, Registro } from './tipos.js';
import { SelectorRelacion } from './SelectorRelacion.js';
import { AltasContext } from './AltasContext.js';
import { hoyISO, subirAdjunto } from './Seguimiento.js';

interface Linea {
  producto: Registro;
  cantidad: number;
}
interface Venta {
  factura: Registro;
  cambio: number;
}
interface CobroPendiente {
  cuerpo: {
    operacion: string;
    sesion: string;
    cliente: string;
    medio: 'efectivo' | 'tarjeta';
    recibido: number;
    lineas: { producto: string; cantidad: number; precio: number; tipo_iva: string }[];
  };
  lineas: Linea[];
  nombreCliente: string;
}

/** La cesta conserva el precio que se vio. El servidor contrasta ese precio,
 * calcula por grupos y confirma toda la operación en una transacción. */
export function PuntoVenta({
  locale,
  t,
  vendedor,
  cuenta,
  soloLectura,
  puedeEditarProductos = false,
  revision,
  onFactura,
  onPresupuesto,
  onProductos,
}: {
  revision: number;
  locale: Locale;
  t: T;
  vendedor: string;
  cuenta: string;
  soloLectura: boolean;
  puedeEditarProductos?: boolean;
  onPresupuesto: (r: Registro) => void;
  onFactura: (id: string) => void;
  onProductos: () => void;
}): JSX.Element {
  const altas = useContext(AltasContext);
  const [productos, setProductos] = useState<Registro[]>([]);
  const [siguiente, setSiguiente] = useState('');
  const [imagenes, setImagenes] = useState<Record<string, string>>({});
  const [revisionImagen, setRevisionImagen] = useState(0);
  const [subiendoImagen, setSubiendoImagen] = useState('');
  const [editarImagenes, setEditarImagenes] = useState(false);
  const productoImagen = useRef('');
  const selectorImagen = useRef<HTMLInputElement>(null);
  const [sesiones, setSesiones] = useState<Registro[]>([]);
  const [sesion, setSesion] = useState('');
  const [caja, setCaja] = useState('');
  const [q, setQ] = useState('');
  const [categoria, setCategoria] = useState('');
  const [lineas, setLineas] = useState<Linea[]>([]);
  const [cliente, setCliente] = useState('');
  const [nombreCliente, setNombreCliente] = useState('');
  const [medio, setMedio] = useState<'efectivo' | 'tarjeta'>('tarjeta');
  const [recibido, setRecibido] = useState('');
  const [venta, setVenta] = useState<Venta | null>(null);
  const [busy, setBusy] = useState(false);
  const [cargando, setCargando] = useState(true);
  const [error, setError] = useState('');
  const operacion = useRef<string | null>(null);
  const enviando = useRef(false);
  const ofertaOperacion = useRef<string | null>(null);
  const pendiente = useRef<CobroPendiente | null>(null);
  const [restaurando, setRestaurando] = useState(true);
  // Después de un error de red se mantiene EXACTA la cesta, para reintentar con
  // la misma operación sin emitir otra factura ni duplicar el asiento.
  const [incierto, setIncierto] = useState(false);
  const dinero = (n: number) => formatearDinero(n, locale);

  // El almacén compartido conserva la petición antes de enviarla, acotada por
  // organización y persona. Recargar tras perder la respuesta recupera el mismo
  // UUID; otra pestaña no puede reemplazar un cobro todavía sin confirmar.
  useEffect(() => {
    let vivo = true;
    readOfflineValue<CobroPendiente>(cuenta, 'erp-tpv', 'cobro')
      .then((p) => {
        if (!vivo || !p) return;
        recuperarCobro(p);
      })
      .catch((e: unknown) => {
        if (vivo) setError(mensajeDeError(e, t));
      })
      .finally(() => {
        if (vivo) setRestaurando(false);
      });
    return () => {
      vivo = false;
    };
  }, [cuenta, t]);
  function recuperarCobro(p: CobroPendiente): void {
    pendiente.current = p;
    operacion.current = p.cuerpo.operacion;
    setLineas(p.lineas);
    setSesion(p.cuerpo.sesion);
    setCliente(p.cuerpo.cliente);
    setNombreCliente(p.nombreCliente);
    setMedio(p.cuerpo.medio);
    setRecibido(String(p.cuerpo.recibido / 100));
    setIncierto(true);
  }

  useEffect(() => {
    let vivo = true;
    setCargando(true);
    setError('');
    const timer = setTimeout(
      () => {
        Promise.all([
          api<Pagina>(`producto${consulta({ q, estado: 'activa', limite: 60 })}`),
          api<Pagina>(`sesion_tpv${consulta({ estado: 'abierto', limite: 100 })}`),
        ])
          .then(([p, s]) => {
            if (!vivo) return;
            setProductos(p.registros ?? []);
            setSiguiente(p.siguiente ?? '');
            setSesiones(s.registros ?? []);
            setSesion((actual) => actual || s.registros?.[0]?.id || '');
          })
          .catch((e: unknown) => {
            if (vivo) setError(mensajeDeError(e, t));
          })
          .finally(() => {
            if (vivo) setCargando(false);
          });
      },
      q ? 180 : 0,
    );
    return () => {
      vivo = false;
      clearTimeout(timer);
    };
  }, [q, t, revision]);

  useEffect(() => {
    let vivo = true;
    const abortar = new AbortController();
    const cargar = async () => {
      const resultado: Record<string, string> = {};
      for (let i = 0; i < productos.length; i += 200) {
        const ids = productos
          .slice(i, i + 200)
          .map((p) => p.id)
          .join(',');
        const r = await api<{ imagenes: Record<string, string> }>(
          `productos/imagenes${consulta({ registros: ids })}`,
          { signal: abortar.signal },
        );
        Object.assign(resultado, r.imagenes);
      }
      if (vivo) setImagenes(resultado);
    };
    void cargar().catch((e: unknown) => {
      if (vivo) setError(mensajeDeError(e, t));
    });
    return () => {
      vivo = false;
      abortar.abort();
    };
  }, [productos, revisionImagen, t]);

  async function cargarImagen(fichero: File | undefined): Promise<void> {
    const id = productoImagen.current;
    productoImagen.current = '';
    if (!fichero || !id || !puedeEditarProductos || subiendoImagen) return;
    if (
      !['image/jpeg', 'image/png', 'image/webp'].includes(fichero.type) ||
      fichero.size > 10 * 1024 * 1024
    ) {
      setError(t('pos.imageHint'));
      return;
    }
    setSubiendoImagen(id);
    setError('');
    try {
      const r = await subirAdjunto('producto', id, fichero);
      setImagenes((prev) => ({ ...prev, [id]: r.adjunto.id }));
      setRevisionImagen((n) => n + 1);
    } catch (e) {
      setError(mensajeDeError(e, t));
    } finally {
      setSubiendoImagen('');
    }
  }

  async function mas(): Promise<void> {
    setCargando(true);
    try {
      const p = await api<Pagina>(
        `producto${consulta({ q, estado: 'activa', limite: 60, cursor: siguiente })}`,
      );
      setProductos((prev) => [...prev, ...(p.registros ?? [])]);
      setSiguiente(p.siguiente ?? '');
    } catch (e) {
      setError(mensajeDeError(e, t));
    } finally {
      setCargando(false);
    }
  }
  async function abrirCaja(e: TargetedSubmitEvent<HTMLFormElement>): Promise<void> {
    e.preventDefault();
    if (enviando.current) return;
    enviando.current = true;
    setBusy(true);
    setError('');
    try {
      const s = await api<Registro>('sesion_tpv', {
        method: 'POST',
        body: JSON.stringify({
          data: {
            caja: caja.trim(),
            vendedor,
            fecha_inicio: new Date().toISOString(),
            efectivo: 0,
            tarjeta: 0,
            total: 0,
          },
        }),
      });
      setSesiones((prev) => [...prev, s]);
      setSesion(s.id);
      setCaja('');
    } catch (e) {
      setError(mensajeDeError(e, t));
    } finally {
      enviando.current = false;
      setBusy(false);
    }
  }
  function modificar(producto: Registro, delta: number): void {
    if (incierto || busy) return;
    operacion.current = null;
    ofertaOperacion.current = null;
    setLineas((prev) => {
      const actual = prev.find((l) => l.producto.id === producto.id);
      if (!actual) return delta > 0 ? [...prev, { producto, cantidad: 1 }] : prev;
      return prev
        .map((l) =>
          l.producto.id === producto.id
            ? { producto, cantidad: Math.min(1000, l.cantidad + delta) }
            : l,
        )
        .filter((l) => l.cantidad > 0);
    });
  }
  const grupos = new Map<number, number>();
  for (const l of lineas) {
    const tipo = Number(l.producto.data.tipo_iva);
    grupos.set(tipo, (grupos.get(tipo) ?? 0) + Number(l.producto.data.precio_con_iva) * l.cantidad);
  }
  const total = [...grupos.values()].reduce((a, b) => a + b, 0);
  const cuotas = [...grupos].map(([tipo, importe]) => ({ tipo, ...desglosar(importe, tipo) }));
  const efectivo = aCentimos(recibido) ?? 0;
  const categorias = [
    ...new Set(productos.map((p) => String(p.data.categoria ?? '')).filter(Boolean)),
  ].sort();
  const editable = !busy && !incierto && !restaurando;
  const efectivoExcesivo = medio === 'efectivo' && total >= 100000;

  async function cobrar(e: TargetedSubmitEvent<HTMLFormElement>): Promise<void> {
    e.preventDefault();
    if (enviando.current || soloLectura || restaurando) return;
    if (
      !pendiente.current &&
      (!sesion ||
        lineas.length === 0 ||
        total <= 0 ||
        (!cliente && total > 40000) ||
        (medio === 'efectivo' && efectivo < total))
    ) {
      setError(t('pos.review'));
      return;
    }
    if (efectivoExcesivo) {
      setError(t('pos.cashLimit'));
      return;
    }
    enviando.current = true;
    setBusy(true);
    setError('');
    operacion.current ??= crypto.randomUUID();
    const p = pendiente.current ?? {
      cuerpo: {
        operacion: operacion.current,
        sesion,
        cliente,
        medio,
        recibido: medio === 'efectivo' ? efectivo : total,
        lineas: lineas.map((l) => ({
          producto: l.producto.id,
          cantidad: l.cantidad,
          precio: Number(l.producto.data.precio_con_iva),
          tipo_iva: String(l.producto.data.tipo_iva),
        })),
      },
      lineas,
      nombreCliente,
    };
    // Un rechazo tras una respuesta perdida no demuestra que la venta original
    // haya fallado: conserva la operación hasta recuperar su resultado.
    const recuperado = incierto;
    let enviado = false;
    try {
      await mutateOfflineValue<CobroPendiente>(cuenta, 'erp-tpv', 'cobro', (anterior) => {
        if (anterior && anterior.cuerpo.operacion !== p.cuerpo.operacion)
          throw new Error(t('pos.resume'));
        return p;
      });
      pendiente.current = p;
      setIncierto(true);
      enviado = true;
      const v = await api<Venta>('tpv/cobrar', { method: 'POST', body: JSON.stringify(p.cuerpo) });
      setVenta(v);
      setIncierto(false);
      await borrarPendiente(p.cuerpo.operacion);
      pendiente.current = null;
    } catch (e) {
      setError(mensajeDeError(e, t));
      if (!enviado) {
        const anterior = await readOfflineValue<CobroPendiente>(cuenta, 'erp-tpv', 'cobro').catch(
          () => null,
        );
        if (anterior) recuperarCobro(anterior);
      }
      // Solo un rechazo de validación en el primer envío confirma rollback.
      // 401/403/429 y cualquier rechazo de un replay mantienen el mismo UUID.
      if (enviado && !recuperado && e instanceof ErrorApi && [400, 422].includes(e.estado)) {
        try {
          await borrarPendiente(p.cuerpo.operacion);
          pendiente.current = null;
          setIncierto(false);
        } catch {
          setIncierto(true);
        }
      }
    } finally {
      enviando.current = false;
      setBusy(false);
    }
  }
  async function borrarPendiente(id: string): Promise<void> {
    await mutateOfflineValue<CobroPendiente>(cuenta, 'erp-tpv', 'cobro', (p) =>
      p?.cuerpo.operacion === id ? null : p,
    );
  }
  async function crearPresupuesto(): Promise<void> {
    if (enviando.current || !editable) return;
    if (!cliente) {
      setError(t('form.required') + ': ' + t('pos.customer'));
      return;
    }
    enviando.current = true;
    setBusy(true);
    setError('');
    ofertaOperacion.current ??= crypto.randomUUID();
    try {
      const r = await api<Registro>('tpv/presupuesto', {
        method: 'POST',
        body: JSON.stringify({
          operacion: ofertaOperacion.current,
          cliente,
          lineas: lineas.map((l) => ({
            producto: l.producto.id,
            cantidad: l.cantidad,
            precio: l.producto.data.precio_con_iva,
            tipo_iva: l.producto.data.tipo_iva,
          })),
        }),
      });
      onPresupuesto(r);
    } catch (e) {
      setError(mensajeDeError(e, t));
    } finally {
      enviando.current = false;
      setBusy(false);
    }
  }
  async function nuevaVenta(): Promise<void> {
    if (enviando.current) return;
    enviando.current = true;
    setBusy(true);
    try {
      if (pendiente.current) await borrarPendiente(pendiente.current.cuerpo.operacion);
      pendiente.current = null;
      setVenta(null);
      setLineas([]);
      setCliente('');
      setNombreCliente('');
      setRecibido('');
      operacion.current = null;
      ofertaOperacion.current = null;
      setError('');
    } catch (e) {
      setError(mensajeDeError(e, t));
    } finally {
      enviando.current = false;
      setBusy(false);
    }
  }

  return (
    <div class="erp-tpv">
      <header class="erp-tpv-heading">
        <div>
          <p class="sytue-overline text-text-muted">Sytue ERP</p>
          <h1>{t('pos.title')}</h1>
        </div>
        <div class="flex flex-wrap gap-2">
          {altas?.puedeCrear('producto') ? (
            <Button variant="secondary" onClick={() => altas.crear('producto', () => undefined)}>
              {t('pos.newProduct')}
            </Button>
          ) : null}
          <Popover
            label={t('pos.catalogue')}
            align="end"
            trigger={(props) => (
              <Button variant="ghost" {...props}>
                {t('pos.catalogue')}
                <Icon name="chevronDown" size={16} />
              </Button>
            )}
          >
            {(cerrar) => (
              <div class="grid min-w-48 gap-1">
                <Button
                  variant="ghost"
                  class="justify-start"
                  onClick={() => {
                    cerrar();
                    onProductos();
                  }}
                >
                  <Icon name="grid" size={18} />
                  {t('pos.catalogue')}
                </Button>
                {puedeEditarProductos ? (
                  <Button
                    variant="ghost"
                    class="justify-start"
                    aria-pressed={editarImagenes}
                    onClick={() => {
                      setEditarImagenes((v) => !v);
                      cerrar();
                    }}
                  >
                    <Icon name="camera" size={18} />
                    {t('pos.images')}
                    {editarImagenes ? <Icon name="check" size={16} /> : null}
                  </Button>
                ) : null}
              </div>
            )}
          </Popover>
        </div>
      </header>
      {puedeEditarProductos ? (
        <input
          ref={selectorImagen}
          type="file"
          hidden
          accept="image/jpeg,image/png,image/webp"
          onChange={(e) => {
            const fichero = e.currentTarget.files?.[0];
            e.currentTarget.value = '';
            void cargarImagen(fichero);
          }}
        />
      ) : null}
      {error ? (
        <p role="alert" class="bg-danger-bg text-danger-text rounded-md p-3">
          {error}
        </p>
      ) : null}
      <div class="erp-tpv-layout">
        <section class="sytue-glass-surface erp-tpv-catalogue" aria-label={t('pos.catalogue')}>
          <label class="erp-tpv-search">
            <Icon name="search" size={20} />
            <span class="sr-only">{t('list.searchPlaceholder')}</span>
            <input
              type="search"
              name="q"
              value={q}
              onInput={(e) => setQ(e.currentTarget.value)}
              placeholder={t('list.searchPlaceholder')}
            />
          </label>
          {categorias.length ? (
            <div class="erp-tpv-categories">
              <button
                type="button"
                class="sytue-focus-ring"
                aria-pressed={!categoria}
                onClick={() => setCategoria('')}
              >
                {t('list.all')}
              </button>
              {categorias.map((c) => (
                <button
                  key={c}
                  type="button"
                  class="sytue-focus-ring"
                  aria-pressed={categoria === c}
                  onClick={() => setCategoria(c)}
                >
                  {c}
                </button>
              ))}
            </div>
          ) : null}
          {cargando && !productos.length ? <Spinner /> : null}
          {!cargando && !productos.length ? (
            <p class="text-text-muted py-12">{t('relation.empty')}</p>
          ) : null}
          <div class="erp-tpv-products">
            {productos
              .filter((p) => !categoria || p.data.categoria === categoria)
              .map((p) => {
                const precio = p.data.precio_con_iva;
                const imagen = imagenes[p.id];
                const listo =
                  typeof precio === 'number' &&
                  precio >= 0 &&
                  ['21', '10', '5', '4'].includes(String(p.data.tipo_iva));
                const cantidad = lineas.find((l) => l.producto.id === p.id)?.cantidad ?? 0;
                return (
                  <div key={p.id} class="erp-tpv-product-wrap">
                    <button
                      type="button"
                      data-selected={cantidad > 0}
                      data-tone={
                        CARD_TONES[
                          Math.max(0, categorias.indexOf(String(p.data.categoria ?? ''))) %
                            CARD_TONES.length
                        ]
                      }
                      class={`erp-tpv-product sytue-focus-ring ${imagen ? 'erp-tpv-product-with-image' : ''}`}
                      disabled={!editable || !listo}
                      onClick={() => modificar(p, 1)}
                    >
                      {imagen ? (
                        <img
                          class="erp-tpv-product-photo"
                          src={withBase(
                            `/api/adjuntos/${encodeURIComponent(imagen)}?ver=1`,
                            baseDeLaApp(),
                          )}
                          alt=""
                          loading="lazy"
                          decoding="async"
                          onError={() =>
                            setImagenes((prev) => {
                              const copia = { ...prev };
                              delete copia[p.id];
                              return copia;
                            })
                          }
                        />
                      ) : null}
                      <span class="erp-tpv-product-content">
                        <span class="erp-tpv-product-code">
                          {String(p.data.codigo ?? p.data.categoria ?? '—')}
                        </span>
                        <span class="erp-tpv-product-info">
                          <span class="erp-tpv-product-name">{nombreDe(p)}</span>
                          <span class="erp-tpv-product-bottom">
                            <strong>{listo ? dinero(precio) : t('pos.configure')}</strong>
                            <span aria-hidden="true" class="erp-tpv-product-count">
                              {cantidad || '+'}
                            </span>
                          </span>
                        </span>
                      </span>
                    </button>
                    {puedeEditarProductos && editarImagenes ? (
                      <Button
                        iconOnly
                        size="sm"
                        class="erp-tpv-product-image-action"
                        aria-label={t('pos.imageUpload', { name: nombreDe(p) })}
                        title={t('pos.imageUpload', { name: nombreDe(p) })}
                        loading={subiendoImagen === p.id}
                        disabled={Boolean(subiendoImagen)}
                        onClick={() => {
                          productoImagen.current = p.id;
                          selectorImagen.current?.click();
                        }}
                      >
                        <Icon name="camera" size={16} />
                      </Button>
                    ) : null}
                  </div>
                );
              })}
          </div>
          {siguiente ? (
            <Button variant="secondary" disabled={cargando} onClick={() => void mas()}>
              {t('action.more')}
            </Button>
          ) : null}
          <p class="text-text-muted text-sm">{t('pos.taxIncluded')}</p>
        </section>
        <aside
          id="tpv-ticket"
          tabIndex={-1}
          class="sytue-glass-surface erp-tpv-receipt"
          aria-label={t('pos.sale')}
        >
          {venta ? (
            <div class="erp-tpv-success" role="status">
              <Icon name="check" size={32} />
              <h2>{t('pos.recorded')}</h2>
              <p>{venta.factura.ref}</p>
              <strong class="erp-tpv-grand-total">
                {dinero(Number(venta.factura.data.total))}
              </strong>
              {venta.cambio > 0 ? (
                <p>
                  {t('pos.change')}: {dinero(venta.cambio)}
                </p>
              ) : null}
              <Button
                href={withBase(baseDeLaApp(), `/api/factura_venta/${venta.factura.id}/imprimir`)}
                target="_blank"
                rel="noopener"
              >
                {t('form.print')}
              </Button>
              <Button variant="secondary" onClick={() => onFactura(venta.factura.id)}>
                {t('action.open')}
              </Button>
              <Button variant="secondary" disabled={busy} onClick={() => void nuevaVenta()}>
                {t('pos.newSale')}
              </Button>
            </div>
          ) : (
            <>
              <div class="erp-tpv-ticket-heading">
                <h2>{t('pos.sale')}</h2>
                <time dateTime={hoyISO()}>
                  {new Intl.DateTimeFormat(locale, { dateStyle: 'medium' }).format(new Date())}
                </time>
              </div>
              {!sesiones.length && !soloLectura ? (
                <form method="post" onSubmit={(e) => void abrirCaja(e)} class="erp-tpv-open">
                  <label for="pos-caja">{t('pos.register')}</label>
                  <input
                    id="pos-caja"
                    name="caja"
                    required
                    maxLength={100}
                    value={caja}
                    onInput={(e) => setCaja(e.currentTarget.value)}
                  />
                  <Button type="submit" disabled={busy}>
                    {t('pos.openRegister')}
                  </Button>
                </form>
              ) : (
                <label class="erp-tpv-field">
                  {t('pos.register')}
                  <select
                    name="sesion"
                    value={sesion}
                    disabled={!editable}
                    onChange={(e) => {
                      setSesion(e.currentTarget.value);
                      operacion.current = null;
                    }}
                  >
                    {sesiones.map((s) => (
                      <option key={s.id} value={s.id}>
                        {String(s.data.caja)} · {s.ref}
                      </option>
                    ))}
                  </select>
                </label>
              )}
              <div class="erp-tpv-lines">
                {!lineas.length ? (
                  <p class="erp-tpv-empty">{t('pos.addProducts')}</p>
                ) : (
                  <ul>
                    {lineas.map((l) => (
                      <li key={l.producto.id}>
                        <div>
                          <strong>{nombreDe(l.producto)}</strong>
                          <span>
                            {dinero(Number(l.producto.data.precio_con_iva))} ·{' '}
                            {String(l.producto.data.tipo_iva)}% IVA
                          </span>
                        </div>
                        <div class="erp-tpv-quantity">
                          <Button
                            iconOnly
                            size="lg"
                            disabled={!editable}
                            aria-label={t('pos.removeUnit', { name: nombreDe(l.producto) })}
                            onClick={() => modificar(l.producto, -1)}
                          >
                            −
                          </Button>
                          <span>{l.cantidad}</span>
                          <Button
                            iconOnly
                            size="lg"
                            disabled={!editable}
                            aria-label={t('pos.addUnit', { name: nombreDe(l.producto) })}
                            onClick={() => modificar(l.producto, 1)}
                          >
                            +
                          </Button>
                        </div>
                        <strong>
                          {dinero(Number(l.producto.data.precio_con_iva) * l.cantidad)}
                        </strong>
                      </li>
                    ))}
                  </ul>
                )}
              </div>
              {altas?.puedeCrear('presupuesto') && lineas.length ? (
                <Button
                  type="button"
                  variant="secondary"
                  disabled={!editable}
                  onClick={() => void crearPresupuesto()}
                >
                  {t('pos.quote')}
                </Button>
              ) : null}
              <dl class="erp-tpv-totals">
                {cuotas.map((g) => (
                  <div key={g.tipo}>
                    <dt>IVA {g.tipo}%</dt>
                    <dd>{dinero(g.iva)}</dd>
                  </div>
                ))}
                <div class="erp-tpv-total">
                  <dt>{t('pos.total')}</dt>
                  <dd>{dinero(total)}</dd>
                </div>
              </dl>
              <form method="post" onSubmit={(e) => void cobrar(e)} class="erp-tpv-payment">
                <fieldset disabled={!editable}>
                  <legend>{t('pos.customer')}</legend>
                  <SelectorRelacion
                    modelo="contacto"
                    valor={cliente}
                    etiqueta={nombreCliente}
                    id="pos-cliente"
                    t={t}
                    onCambio={(id, nombre) => {
                      setCliente(id);
                      setNombreCliente(nombre);
                      operacion.current = null;
                      ofertaOperacion.current = null;
                    }}
                  />
                  {cliente ? (
                    <button
                      type="button"
                      class="text-text-muted text-sm underline"
                      onClick={() => {
                        setCliente('');
                        setNombreCliente('');
                        operacion.current = null;
                        ofertaOperacion.current = null;
                      }}
                    >
                      {t('list.clearFilters')}
                    </button>
                  ) : null}
                  <p class="text-text-muted text-xs">{t('pos.customerHint')}</p>
                </fieldset>
                <fieldset disabled={!editable} class="erp-tpv-methods">
                  <legend>{t('pos.payment')}</legend>
                  {(['tarjeta', 'efectivo'] as const).map((m) => (
                    <label key={m}>
                      <Radio
                        name="medio"
                        value={m}
                        checked={medio === m}
                        onChange={() => {
                          setMedio(m);
                          operacion.current = null;
                        }}
                      />
                      <span>{t(m === 'tarjeta' ? 'pos.card' : 'pos.cash')}</span>
                    </label>
                  ))}
                </fieldset>
                {medio === 'efectivo' ? (
                  <label class="erp-tpv-field">
                    {t('pos.received')}
                    <input
                      name="recibido"
                      inputMode="decimal"
                      required
                      value={recibido}
                      disabled={!editable}
                      onInput={(e) => {
                        setRecibido(e.currentTarget.value);
                        operacion.current = null;
                      }}
                    />
                    <span>
                      {t('pos.change')}: {dinero(Math.max(0, efectivo - total))}
                    </span>
                  </label>
                ) : (
                  <p class="text-text-muted text-xs">{t('pos.terminalHint')}</p>
                )}
                {incierto ? (
                  <p role="status" class="text-warning-text text-sm">
                    {t('pos.retryHint')}
                  </p>
                ) : null}
                {efectivoExcesivo ? (
                  <p role="alert" class="text-danger-text text-sm">
                    {t('pos.cashLimit')}
                  </p>
                ) : null}
                <Button
                  type="submit"
                  class="erp-tpv-checkout"
                  disabled={
                    busy || restaurando || soloLectura || !lineas.length || efectivoExcesivo
                  }
                >
                  {busy ? <Spinner size={18} /> : null}
                  {t(incierto ? 'pos.retry' : 'pos.charge', { amount: dinero(total) })}
                </Button>
              </form>
            </>
          )}
        </aside>
      </div>
      {lineas.length && !venta ? (
        <a
          href="#tpv-ticket"
          class="erp-tpv-mobile-ticket sytue-focus-ring"
          onClick={() => document.getElementById('tpv-ticket')?.focus({ preventScroll: true })}
        >
          <span>
            {t('pos.sale')} · {lineas.reduce((n, l) => n + l.cantidad, 0)}
          </span>
          <strong>{dinero(total)}</strong>
          <Icon name="chevronDown" size={18} />
        </a>
      ) : null}
    </div>
  );
}
