/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Panel de Sitio del editor (PAGES-PLAN §14.1).
 * Ajustes globales: Tema (colores con contraste WCAG en vivo, fuentes, radios, contenedor),
 * Idiomas del sitio (conforme a IDIOMAS_VISITANTE) y SEO/Negocio.
 */

import { useMemo, useState } from 'preact/hooks';
import type { JSX } from 'preact';
import type { DatosRender, Idioma, Sitio, Tema } from '@sytue/pages-render';
import { IDIOMAS_VISITANTE, contraste } from '@sytue/pages-render';
import { Checkbox } from '@sytue/ui';

interface PanelSitioProps {
  ir: Sitio;
  datosRender: DatosRender;
  onActualizarTema: (tema: Partial<Tema>) => void;
  onActualizarIdiomas: (languages: Sitio['languages']) => void;
  onActualizarSeo: (seo: Partial<Sitio['seo']>) => void;
}

type SubPestana = 'tema' | 'idiomas' | 'seo';

export function PanelSitio({
  ir,
  datosRender,
  onActualizarTema,
  onActualizarIdiomas,
  onActualizarSeo,
}: PanelSitioProps): JSX.Element {
  const [subPestana, setSubPestana] = useState<SubPestana>('tema');

  const colors = ir.theme.colors;
  const defLang = ir.languages.default;

  // Cálculo de contraste WCAG 2.x en tiempo real (§14.1, P17)
  const contrastes = useMemo(() => {
    const calc = (c1?: string, c2?: string): number | null => {
      if (!c1 || !c2 || !c1.startsWith('#') || !c2.startsWith('#')) return null;
      try {
        return Math.round(contraste(c1, c2) * 10) / 10;
      } catch {
        return null;
      }
    };

    return {
      textoFondo: calc(colors.text, colors.background),
      botonPrimary: calc(colors.on_primary, colors.primary),
      textoMuted: calc(colors.muted, colors.background),
    };
  }, [colors]);

  const cambiarColor = (clave: string, valor: string) => {
    onActualizarTema({
      colors: { ...colors, [clave]: valor },
    });
  };

  const handleCopiarDatosLegales = () => {
    if (!datosRender?.legal) return;
    const l = datosRender.legal;
    const currentBiz = ir.seo.business;
    const biz: NonNullable<Sitio['seo']['business']> = {
      type: currentBiz?.type || 'Organization',
    };
    const email = l.email || currentBiz?.email;
    if (email) biz.email = email;
    const phone = l.phone || currentBiz?.phone;
    if (phone) biz.phone = phone;
    if (l.address) {
      biz.address = {
        street: l.address.street || '',
        city: l.address.city || '',
        postal_code: l.address.postal_code || '',
        country: l.country || 'ES',
      };
      if (l.address.region) biz.address.region = l.address.region;
    } else if (currentBiz?.address) {
      biz.address = currentBiz.address;
    }
    onActualizarSeo({ business: biz });
  };

  return (
    <div class="flex h-full flex-col overflow-y-auto p-3">
      {/* Selector de subpestañas */}
      <div class="border-border bg-bg-subtle mb-3 flex rounded-lg border p-0.5">
        <button
          type="button"
          onClick={() => setSubPestana('tema')}
          class={`flex-1 rounded py-1 text-center text-xs font-medium transition-colors ${
            subPestana === 'tema'
              ? 'bg-surface text-text font-semibold shadow-sm'
              : 'text-text-muted hover:text-text'
          }`}
        >
          Tema
        </button>
        <button
          type="button"
          onClick={() => setSubPestana('idiomas')}
          class={`flex-1 rounded py-1 text-center text-xs font-medium transition-colors ${
            subPestana === 'idiomas'
              ? 'bg-surface text-text font-semibold shadow-sm'
              : 'text-text-muted hover:text-text'
          }`}
        >
          Idiomas
        </button>
        <button
          type="button"
          onClick={() => setSubPestana('seo')}
          class={`flex-1 rounded py-1 text-center text-xs font-medium transition-colors ${
            subPestana === 'seo'
              ? 'bg-surface text-text font-semibold shadow-sm'
              : 'text-text-muted hover:text-text'
          }`}
        >
          SEO y Negocio
        </button>
      </div>

      {/* Pestaña TEMA */}
      {subPestana === 'tema' && (
        <div class="flex flex-col gap-4">
          {/* Colores principales */}
          <div>
            <h4 class="text-text-muted mb-2 text-[11px] font-semibold tracking-wider uppercase">
              Paleta de colores
            </h4>
            <div class="flex flex-col gap-2.5">
              {[
                { k: 'primary', label: 'Color primario' },
                { k: 'on_primary', label: 'Texto en primario' },
                { k: 'background', label: 'Fondo general' },
                { k: 'text', label: 'Texto principal' },
                { k: 'muted', label: 'Texto secundario' },
                { k: 'border', label: 'Líneas y bordes' },
              ].map(({ k, label }) => (
                <div key={k} class="flex items-center justify-between">
                  <span class="text-text text-xs">{label}</span>
                  <div class="flex items-center gap-2">
                    <input
                      type="color"
                      value={colors[k] || '#000000'}
                      onChange={(e) => cambiarColor(k, (e.target as HTMLInputElement).value)}
                      class="h-6 w-7 cursor-pointer rounded border-0 bg-transparent p-0"
                    />
                    <input
                      type="text"
                      value={colors[k] || ''}
                      onInput={(e) => cambiarColor(k, (e.target as HTMLInputElement).value)}
                      class="border-border bg-bg text-text h-6 w-20 rounded border px-1 font-mono text-[11px]"
                    />
                  </div>
                </div>
              ))}
            </div>
          </div>

          {/* Comprobación de accesibilidad WCAG en vivo */}
          <div class="border-border bg-surface rounded-lg border p-2.5">
            <h5 class="text-text mb-2 text-xs font-semibold">Contraste de color (WCAG 2.1)</h5>
            <div class="flex flex-col gap-1.5 text-[11px]">
              <div class="flex items-center justify-between">
                <span class="text-text-muted">Texto / Fondo:</span>
                <span
                  class={`font-mono font-bold ${
                    contrastes.textoFondo && contrastes.textoFondo >= 4.5
                      ? 'text-emerald-600'
                      : 'text-amber-600'
                  }`}
                >
                  {contrastes.textoFondo ? `${contrastes.textoFondo}:1` : '—'}
                  {contrastes.textoFondo && contrastes.textoFondo >= 4.5 ? ' (AA ✓)' : ' (Bajo)'}
                </span>
              </div>

              <div class="flex items-center justify-between">
                <span class="text-text-muted">Botón / Primario:</span>
                <span
                  class={`font-mono font-bold ${
                    contrastes.botonPrimary && contrastes.botonPrimary >= 4.5
                      ? 'text-emerald-600'
                      : 'text-amber-600'
                  }`}
                >
                  {contrastes.botonPrimary ? `${contrastes.botonPrimary}:1` : '—'}
                  {contrastes.botonPrimary && contrastes.botonPrimary >= 4.5
                    ? ' (AA ✓)'
                    : ' (Bajo)'}
                </span>
              </div>

              <div class="flex items-center justify-between">
                <span class="text-text-muted">Secundario / Fondo:</span>
                <span
                  class={`font-mono font-bold ${
                    contrastes.textoMuted && contrastes.textoMuted >= 4.5
                      ? 'text-emerald-600'
                      : 'text-amber-600'
                  }`}
                >
                  {contrastes.textoMuted ? `${contrastes.textoMuted}:1` : '—'}
                  {contrastes.textoMuted && contrastes.textoMuted >= 4.5 ? ' (AA ✓)' : ' (Bajo)'}
                </span>
              </div>
            </div>
          </div>

          {/* Ancho del contenedor */}
          <div>
            <div class="mb-1 flex items-center justify-between">
              <span class="text-text text-xs">Ancho máximo del contenedor</span>
              <span class="text-text-muted font-mono text-xs">{ir.theme.container} px</span>
            </div>
            <input
              type="range"
              min={800}
              max={1600}
              step={20}
              value={ir.theme.container}
              onInput={(e) =>
                onActualizarTema({
                  container: Number((e.target as HTMLInputElement).value),
                })
              }
              class="w-full"
            />
          </div>

          {/* Radios de borde */}
          <div>
            <h4 class="text-text-muted mb-2 text-[11px] font-semibold tracking-wider uppercase">
              Radios de esquina (px)
            </h4>
            <div class="grid grid-cols-3 gap-2">
              {(['sm', 'md', 'lg'] as const).map((r) => (
                <div key={r}>
                  <label class="text-text-muted mb-0.5 block text-[10px] uppercase">{r}</label>
                  <input
                    type="number"
                    min={0}
                    max={64}
                    value={ir.theme.radius[r]}
                    onInput={(e) =>
                      onActualizarTema({
                        radius: {
                          ...ir.theme.radius,
                          [r]: Number((e.target as HTMLInputElement).value),
                        },
                      })
                    }
                    class="border-border bg-bg text-text h-7 w-full rounded border px-2 text-xs"
                  />
                </div>
              ))}
            </div>
          </div>
        </div>
      )}

      {/* Pestaña IDIOMAS */}
      {subPestana === 'idiomas' && (
        <div class="flex flex-col gap-3">
          <p class="text-text-muted text-xs">
            Configura los idiomas de la web. Solo los idiomas oficiales de visitante están
            disponibles en el piloto.
          </p>

          <div class="border-border bg-surface rounded-lg border p-3">
            <label class="text-text mb-1 flex flex-col gap-1 text-xs font-semibold">
              <span>Idioma principal</span>
              <select
                value={ir.languages.default}
                onChange={(e) => {
                  const nuevoDef = (e.target as HTMLSelectElement).value as Idioma;
                  const otros = ir.languages.others.filter((l) => l !== nuevoDef);
                  if (ir.languages.default !== nuevoDef && !otros.includes(ir.languages.default)) {
                    otros.push(ir.languages.default);
                  }
                  onActualizarIdiomas({ default: nuevoDef, others: otros });
                }}
                class="border-border bg-bg text-text h-8 w-full rounded border px-2 text-xs font-normal"
              >
                {IDIOMAS_VISITANTE.map((l) => (
                  <option key={l} value={l}>
                    {l === 'es' ? 'Español (es)' : 'English (en)'}
                  </option>
                ))}
              </select>
            </label>
          </div>

          <div class="border-border bg-surface rounded-lg border p-3">
            <span class="text-text mb-2 block text-xs font-semibold">Idiomas adicionales</span>
            <div class="flex flex-col gap-2">
              {IDIOMAS_VISITANTE.filter((l) => l !== ir.languages.default).map((lang) => {
                const activo = ir.languages.others.includes(lang);
                return (
                  <label
                    key={lang}
                    class="text-text flex cursor-pointer items-center gap-2 text-xs"
                  >
                    <Checkbox
                      checked={activo}
                      onChange={(e) => {
                        const checked = (e.target as HTMLInputElement).checked;
                        let nuevosOtros = [...ir.languages.others];
                        if (checked) {
                          if (!nuevosOtros.includes(lang)) nuevosOtros.push(lang);
                        } else {
                          nuevosOtros = nuevosOtros.filter((l) => l !== lang);
                        }
                        onActualizarIdiomas({
                          default: ir.languages.default,
                          others: nuevosOtros,
                        });
                      }}
                    />
                    <span>{lang === 'es' ? 'Español (es)' : 'English (en)'}</span>
                  </label>
                );
              })}
            </div>
          </div>
        </div>
      )}

      {/* Pestaña SEO Y NEGOCIO */}
      {subPestana === 'seo' && (
        <div class="flex flex-col gap-3">
          <div class="flex items-center justify-between">
            <h4 class="text-text-muted text-[11px] font-semibold tracking-wider uppercase">
              Datos comerciales y SEO
            </h4>
            <button
              type="button"
              onClick={handleCopiarDatosLegales}
              class="text-primary-text text-[11px] font-medium hover:text-[var(--sytue-primary-hover)]"
            >
              Usar datos legales
            </button>
          </div>

          <label class="text-text-muted flex flex-col gap-1 text-[11px]">
            <span>Nombre del sitio ({defLang})</span>
            <input
              type="text"
              value={ir.seo.site_name?.[defLang] || ''}
              onInput={(e) => {
                const val = (e.target as HTMLInputElement).value;
                onActualizarSeo({
                  site_name: { ...ir.seo.site_name, [defLang]: val },
                });
              }}
              class="border-border bg-bg text-text h-7 w-full rounded border px-2 text-xs"
            />
          </label>

          <label class="text-text-muted flex flex-col gap-1 text-[11px]">
            <span>Tipo de organización (Schema.org)</span>
            <select
              value={ir.seo.business?.type || 'Organization'}
              onChange={(e) => {
                const val = (e.target as HTMLSelectElement).value as NonNullable<
                  Sitio['seo']['business']
                >['type'];
                const biz: NonNullable<Sitio['seo']['business']> = {
                  type: val,
                };
                if (ir.seo.business?.email) biz.email = ir.seo.business.email;
                if (ir.seo.business?.phone) biz.phone = ir.seo.business.phone;
                if (ir.seo.business?.address) biz.address = ir.seo.business.address;
                onActualizarSeo({ business: biz });
              }}
              class="border-border bg-bg text-text h-7 w-full rounded border px-2 text-xs"
            >
              <option value="Organization">Organización general</option>
              <option value="LocalBusiness">Negocio local</option>
              <option value="Store">Tienda / Comercio</option>
              <option value="Restaurant">Restaurante / Hostelería</option>
              <option value="MedicalClinic">Clínica médica / Salud</option>
              <option value="ProfessionalService">Servicios profesionales</option>
              <option value="HomeAndConstructionBusiness">Reformas y construcción</option>
            </select>
          </label>

          <div class="grid grid-cols-2 gap-2">
            <label class="text-text-muted flex flex-col gap-1 text-[11px]">
              <span>Teléfono</span>
              <input
                type="text"
                value={ir.seo.business?.phone || ''}
                onInput={(e) => {
                  const val = (e.target as HTMLInputElement).value;
                  const biz: NonNullable<Sitio['seo']['business']> = {
                    type: ir.seo.business?.type || 'Organization',
                  };
                  if (ir.seo.business?.email) biz.email = ir.seo.business.email;
                  if (ir.seo.business?.address) biz.address = ir.seo.business.address;
                  if (val.trim()) biz.phone = val.trim();
                  onActualizarSeo({ business: biz });
                }}
                class="border-border bg-bg text-text h-7 w-full rounded border px-2 text-xs"
              />
            </label>
            <label class="text-text-muted flex flex-col gap-1 text-[11px]">
              <span>Correo electrónico</span>
              <input
                type="email"
                value={ir.seo.business?.email || ''}
                onInput={(e) => {
                  const val = (e.target as HTMLInputElement).value;
                  const biz: NonNullable<Sitio['seo']['business']> = {
                    type: ir.seo.business?.type || 'Organization',
                  };
                  if (ir.seo.business?.phone) biz.phone = ir.seo.business.phone;
                  if (ir.seo.business?.address) biz.address = ir.seo.business.address;
                  if (val.trim()) biz.email = val.trim();
                  onActualizarSeo({ business: biz });
                }}
                class="border-border bg-bg text-text h-7 w-full rounded border px-2 text-xs"
              />
            </label>
          </div>

          <label class="text-text-muted flex flex-col gap-1 text-[11px]">
            <span>Calle y número</span>
            <input
              type="text"
              value={ir.seo.business?.address?.street || ''}
              onInput={(e) => {
                const val = (e.target as HTMLInputElement).value;
                const prevAddr = ir.seo.business?.address || {
                  street: '',
                  city: '',
                  postal_code: '',
                  country: 'ES',
                };
                const biz: NonNullable<Sitio['seo']['business']> = {
                  type: ir.seo.business?.type || 'Organization',
                  address: { ...prevAddr, street: val },
                };
                if (ir.seo.business?.email) biz.email = ir.seo.business.email;
                if (ir.seo.business?.phone) biz.phone = ir.seo.business.phone;
                onActualizarSeo({ business: biz });
              }}
              class="border-border bg-bg text-text h-7 w-full rounded border px-2 text-xs"
            />
          </label>

          <div class="grid grid-cols-2 gap-2">
            <label class="text-text-muted flex flex-col gap-1 text-[11px]">
              <span>Ciudad</span>
              <input
                type="text"
                value={ir.seo.business?.address?.city || ''}
                onInput={(e) => {
                  const val = (e.target as HTMLInputElement).value;
                  const prevAddr = ir.seo.business?.address || {
                    street: '',
                    city: '',
                    postal_code: '',
                    country: 'ES',
                  };
                  const biz: NonNullable<Sitio['seo']['business']> = {
                    type: ir.seo.business?.type || 'Organization',
                    address: { ...prevAddr, city: val },
                  };
                  if (ir.seo.business?.email) biz.email = ir.seo.business.email;
                  if (ir.seo.business?.phone) biz.phone = ir.seo.business.phone;
                  onActualizarSeo({ business: biz });
                }}
                class="border-border bg-bg text-text h-7 w-full rounded border px-2 text-xs"
              />
            </label>
            <label class="text-text-muted flex flex-col gap-1 text-[11px]">
              <span>Código Postal</span>
              <input
                type="text"
                value={ir.seo.business?.address?.postal_code || ''}
                onInput={(e) => {
                  const val = (e.target as HTMLInputElement).value;
                  const prevAddr = ir.seo.business?.address || {
                    street: '',
                    city: '',
                    postal_code: '',
                    country: 'ES',
                  };
                  const biz: NonNullable<Sitio['seo']['business']> = {
                    type: ir.seo.business?.type || 'Organization',
                    address: { ...prevAddr, postal_code: val },
                  };
                  if (ir.seo.business?.email) biz.email = ir.seo.business.email;
                  if (ir.seo.business?.phone) biz.phone = ir.seo.business.phone;
                  onActualizarSeo({ business: biz });
                }}
                class="border-border bg-bg text-text h-7 w-full rounded border px-2 font-mono text-xs"
              />
            </label>
          </div>
        </div>
      )}
    </div>
  );
}
