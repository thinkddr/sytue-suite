// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package exportar

import (
	"encoding/json"
	"fmt"

	"git.sytue.eu/sytue/one/services/pages/internal/exportar/apiweb"
	"git.sytue.eu/sytue/one/services/pages/internal/ir"
)

// GenerarCompartidosNextjs produce los archivos deterministas compartidos de un proyecto Next.js (§9.4).
func GenerarCompartidosNextjs(irSitio *ir.Sitio, baseURL string) (map[string][]byte, error) {
	archivos := make(map[string][]byte)

	// 1. package.json con versiones exactas de versiones.go
	vNext, _ := ObtenerVersion("nextjs_version")
	vReact, _ := ObtenerVersion("react_version")
	vReactDom, _ := ObtenerVersion("react_dom_version")
	vTS, _ := ObtenerVersion("typescript_version")
	vTypesReact, _ := ObtenerVersion("types_react_version")
	vTypesReactDom, _ := ObtenerVersion("types_react_dom_version")
	vTypesNode, _ := ObtenerVersion("types_node_version")

	pkg := map[string]any{
		"name":    "sytue-pages-nextjs",
		"private": true,
		"version": "1.0.0",
		"scripts": map[string]string{
			"dev":   "next dev",
			"build": "next build",
			"start": "next start",
		},
		"dependencies": map[string]string{
			"next":      vNext,
			"react":     vReact,
			"react-dom": vReactDom,
		},
		"devDependencies": map[string]string{
			"@types/node":      vTypesNode,
			"@types/react":     vTypesReact,
			"@types/react-dom": vTypesReactDom,
			"typescript":       vTS,
		},
	}
	pkgBytes, err := json.MarshalIndent(pkg, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("nextjs: error serializando package.json: %w", err)
	}
	archivos["package.json"] = append(pkgBytes, '\n')

	// 2. tsconfig.json
	archivos["tsconfig.json"] = []byte(`{
  "compilerOptions": {
    "target": "ES2022",
    "lib": ["dom", "dom.iterable", "esnext"],
    "allowJs": true,
    "skipLibCheck": true,
    "strict": true,
    "noEmit": true,
    "esModuleInterop": true,
    "module": "esnext",
    "moduleResolution": "bundler",
    "resolveJsonModule": true,
    "isolatedModules": true,
    "jsx": "preserve",
    "incremental": true,
    "plugins": [
      {
        "name": "next"
      }
    ],
    "paths": {
      "@/*": ["./*"]
    }
  },
  "include": ["next-env.d.ts", "**/*.ts", "**/*.tsx", ".next/types/**/*.ts"],
  "exclude": ["node_modules"]
}
`)

	// 3. next.config.ts
	archivos["next.config.ts"] = []byte(`import type { NextConfig } from 'next';

const nextConfig: NextConfig = {
  reactStrictMode: true,
};

export default nextConfig;
`)

	// 4. app/globals.css
	archivos["app/globals.css"] = []byte(`:root {
  --font-sans: system-ui, -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, Oxygen, Ubuntu, Cantarell, sans-serif;
}

body {
  font-family: var(--font-sans);
  color: #18181b;
  background-color: #ffffff;
  margin: 0;
  padding: 0;
}
`)

	// 5. app/layout.tsx
	nombreSitio := "Sytue Site"
	if irSitio != nil && irSitio.SEO.SiteName != nil {
		idioma := irSitio.Languages.Default
		if idioma == "" {
			idioma = "es"
		}
		if n := obtenerTextoIdioma(irSitio.SEO.SiteName, idioma); n != "" {
			nombreSitio = n
		}
	}

	archivos["app/layout.tsx"] = []byte(fmt.Sprintf(`import './globals.css';
import type { Metadata } from 'next';

export const metadata: Metadata = {
  title: '%s',
  description: 'Sitio exportado desde Sytue Web',
};

export default function RootLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return (
    <html lang="es">
      <body>{children}</body>
    </html>
  );
}
`, nombreSitio))

	// 6. app/page.tsx
	archivos["app/page.tsx"] = []byte(fmt.Sprintf(`import React from 'react';

export default function HomePage() {
  return (
    <main style={{ padding: '2rem', maxWidth: '800px', margin: '0 auto' }}>
      <h1>%s</h1>
      <p>Sitio exportado a Next.js (App Router) conectado a la API pública de Sytue Web.</p>
    </main>
  );
}
`, nombreSitio))

	// 7. lib/pages-api.ts
	clienteTS, err := apiweb.GenerarClienteTS(baseURL)
	if err != nil {
		return nil, fmt.Errorf("nextjs: error generando pages-api.ts: %w", err)
	}
	archivos["lib/pages-api.ts"] = clienteTS

	// 8. components/Formulario.tsx
	archivos["components/Formulario.tsx"] = []byte(`'use client';

import React, { useState } from 'react';
import { pagesApi, resolverAltcha, type FormSubmission } from '@/lib/pages-api';

interface FormularioProps {
  formId: string;
  language?: string;
}

export function Formulario({ formId, language = 'es' }: FormularioProps) {
  const [enviado, setEnviado] = useState(false);
  const [cargando, setCargando] = useState(false);
  const [error, setError] = useState<string | null>(null);

  async function handleSubmit(e: React.FormEvent<HTMLFormElement>) {
    e.preventDefault();
    setCargando(true);
    setError(null);

    try {
      const formData = new FormData(e.currentTarget);
      const fields: Record<string, unknown> = {};
      formData.forEach((value, key) => {
        fields[key] = value;
      });

      const reto = await pagesApi.getAltcha();
      const altcha = await resolverAltcha(reto);

      const submission: FormSubmission = {
        fields,
        consent: true,
        language,
        altcha,
      };

      await pagesApi.submitForm(formId, submission);
      setEnviado(true);
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : 'Error al enviar');
    } finally {
      setCargando(false);
    }
  }

  if (enviado) {
    return <div className="sytue-form-success">Mensaje enviado correctamente.</div>;
  }

  return (
    <form onSubmit={handleSubmit} className="sytue-form">
      {error && <div className="sytue-form-error">{error}</div>}
      <label>
        Nombre:
        <input type="text" name="nombre" required />
      </label>
      <label>
        Correo:
        <input type="email" name="email" required />
      </label>
      <label>
        Mensaje:
        <textarea name="mensaje" required />
      </label>
      <button type="submit" disabled={cargando}>
        {cargando ? 'Enviando...' : 'Enviar'}
      </button>
    </form>
  );
}
`)

	// 9. components/BotonComprar.tsx
	archivos["components/BotonComprar.tsx"] = []byte(`'use client';

import React, { useState } from 'react';
import { pagesApi, resolverAltcha, type CheckoutRequest } from '@/lib/pages-api';

interface BotonComprarProps {
  sku: string;
  qty?: number;
}

export function BotonComprar({ sku, qty = 1 }: BotonComprarProps) {
  const [cargando, setCargando] = useState(false);
  const [error, setError] = useState<string | null>(null);

  async function handleComprar() {
    setCargando(true);
    setError(null);

    try {
      const reto = await pagesApi.getAltcha();
      const altcha = await resolverAltcha(reto);

      const req: CheckoutRequest = {
        checkout_token: crypto.randomUUID(),
        items: [{ sku, qty }],
        buyer: { email: 'comprador@ejemplo.es', name: 'Cliente Sytue' },
        shipping: { method_id: 'standard' },
        payment: { gateway: 'stripe' },
        terms_version: 1,
        language: 'es',
        altcha,
      };

      const res = await pagesApi.checkout(req);
      if (res.pay.url) {
        window.location.href = res.pay.url;
      }
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : 'Error al tramitar compra');
    } finally {
      setCargando(false);
    }
  }

  return (
    <div>
      <button onClick={handleComprar} disabled={cargando}>
        {cargando ? 'Procesando...' : 'Comprar ahora'}
      </button>
      {error && <p className="error">{error}</p>}
    </div>
  );
}
`)

	// 10. README.md
	archivos["README.md"] = []byte(fmt.Sprintf(`# %s (Exportación Next.js)

Este proyecto ha sido generado deterministamente desde **Sytue Web**.

## Conexión con la API pública

La aplicación utiliza el cliente normativo en `+"`lib/pages-api.ts`"+` que apunta a:
`+"```"+`
%s/_pages/*
`+"```"+`

## Puesta en marcha

1. Instalar dependencias:
   `+"```bash"+`
   npm install
   `+"```"+`
2. Iniciar en desarrollo:
   `+"```bash"+`
   npm run dev
   `+"```"+`
3. Construir para producción:
   `+"```bash"+`
   npm run build
   `+"```"+`
`, nombreSitio, baseURL))

	return archivos, nil
}
