// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package exportar

import (
	"fmt"

	"git.sytue.eu/sytue/one/services/pages/internal/exportar/apiweb"
	"git.sytue.eu/sytue/one/services/pages/internal/ir"
)

// generarAstro crea un proyecto Astro estático. El único contrato de red queda
// centralizado en pages-api.ts, generado desde apiweb.RutasPublicas.
func generarAstro(sitio *ir.Sitio) (map[string][]byte, error) {
	baseURL := obtenerBaseURLSitio(sitio)
	cliente, err := apiweb.GenerarClienteTS(baseURL)
	if err != nil {
		return nil, fmt.Errorf("astro: generando pages-api.ts: %w", err)
	}
	nombre := obtenerTextoIdioma(sitio.SEO.SiteName, sitio.Languages.Default)
	if nombre == "" {
		nombre = "Sytue Web"
	}
	astro, err := ObtenerVersion("astro_version")
	if err != nil {
		return nil, fmt.Errorf("astro: versión de Astro: %w", err)
	}
	adaptador, err := ObtenerVersion("astro_preact_version")
	if err != nil {
		return nil, fmt.Errorf("astro: versión del adaptador Preact: %w", err)
	}
	preact, err := ObtenerVersion("preact_version")
	if err != nil {
		return nil, fmt.Errorf("astro: versión de Preact: %w", err)
	}
	paquete := fmt.Sprintf(`{"name":"sytue-pages-export","private":true,"type":"module","scripts":{"dev":"astro dev","build":"astro build","preview":"astro preview"},"dependencies":{"@astrojs/preact":"%s","astro":"%s","preact":"%s"},"devDependencies":{}}`+"\n", adaptador, astro, preact)
	return map[string][]byte{
		"package.json":         []byte(paquete),
		"astro.config.mjs":     []byte(`import { defineConfig } from 'astro/config'; import preact from '@astrojs/preact'; export default defineConfig({ integrations: [preact()] });` + "\n"),
		"src/lib/pages-api.ts": cliente,
		"src/pages/index.astro": []byte(fmt.Sprintf(`---
import Cart from '../islands/Cart.tsx';
import Form from '../islands/Form.tsx';
---
<!doctype html><html lang="es"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width"><title>%s</title></head><body><main><h1>%s</h1><Form client:load formId="contacto" /><Cart client:visible /></main></body></html>
`, nombre, nombre)),
		"src/islands/Form.tsx": []byte(`import { useState } from 'preact/hooks'; import { pagesApi, resolverAltcha } from '../lib/pages-api'; export default function Form({ formId }: { formId: string }) { const [sent, setSent] = useState(false); return <form onSubmit={async (event) => { event.preventDefault(); const fields = Object.fromEntries(new FormData(event.currentTarget)); const altcha = await resolverAltcha(await pagesApi.getAltcha()); await pagesApi.submitForm(formId, { fields, consent: true, language: document.documentElement.lang || 'es', altcha }); setSent(true); }}><label>Nombre <input required name="name" /></label><button type="submit">Enviar</button>{sent && <p role="status">Enviado</p>}</form>; }` + "\n"),
		"src/islands/Cart.tsx": []byte(`import { useState } from 'preact/hooks'; import { pagesApi, resolverAltcha } from '../lib/pages-api'; export default function Cart() { const [state, setState] = useState(''); return <button onClick={async () => { const altcha = await resolverAltcha(await pagesApi.getAltcha()); const result = await pagesApi.checkout({ checkout_token: crypto.randomUUID(), items: [], buyer: { email: '', name: '' }, shipping: { method_id: 'standard' }, payment: { gateway: 'mollie' }, terms_version: 1, language: document.documentElement.lang || 'es', altcha }); setState(result.order_id); }}>{state ? 'Pedido creado' : 'Finalizar compra'}</button>; }` + "\n"),
		"README.md":            []byte("# Exportación Astro de Sytue Web\n\n`src/lib/pages-api.ts` se genera desde el catálogo público de rutas; no lo edites manualmente. Las islas Preact se hidratan solo donde hay formulario o carrito.\n"),
	}, nil
}
