// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package exportar

import (
	"encoding/json"
	"fmt"

	"git.sytue.eu/sytue/one/services/pages/internal/exportar/apiweb"
	"git.sytue.eu/sytue/one/services/pages/internal/ir"
)

// GenerarCompartidosReact produce los archivos deterministas compartidos de un proyecto React (§9.4).
func GenerarCompartidosReact(irSitio *ir.Sitio, baseURL string) (map[string][]byte, error) {
	archivos := make(map[string][]byte)

	// 1. package.json con versiones exactas de versiones.go
	vReact, _ := ObtenerVersion("react_version")
	vReactDom, _ := ObtenerVersion("react_dom_version")
	vVite, _ := ObtenerVersion("vite_version")
	vVitePluginReact, _ := ObtenerVersion("vite_plugin_react_version")
	vTS, _ := ObtenerVersion("typescript_version")
	vTypesReact, _ := ObtenerVersion("types_react_version")
	vTypesReactDom, _ := ObtenerVersion("types_react_dom_version")
	vTypesNode, _ := ObtenerVersion("types_node_version")

	pkg := map[string]any{
		"name":    "sytue-pages-react",
		"private": true,
		"version": "1.0.0",
		"type":    "module",
		"scripts": map[string]string{
			"dev":     "vite",
			"build":   "tsc -b && vite build",
			"preview": "vite preview",
		},
		"dependencies": map[string]string{
			"react":     vReact,
			"react-dom": vReactDom,
		},
		"devDependencies": map[string]string{
			"@types/node":          vTypesNode,
			"@types/react":         vTypesReact,
			"@types/react-dom":     vTypesReactDom,
			"@vitejs/plugin-react": vVitePluginReact,
			"typescript":           vTS,
			"vite":                 vVite,
		},
	}
	pkgBytes, err := json.MarshalIndent(pkg, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("react: error serializando package.json: %w", err)
	}
	archivos["package.json"] = append(pkgBytes, '\n')

	// 2. tsconfig.json
	archivos["tsconfig.json"] = []byte(`{
  "compilerOptions": {
    "target": "ES2022",
    "useDefineForClassFields": true,
    "lib": ["ES2022", "DOM", "DOM.Iterable"],
    "module": "ESNext",
    "skipLibCheck": true,
    "moduleResolution": "bundler",
    "allowImportingTsExtensions": true,
    "resolveJsonModule": true,
    "isolatedModules": true,
    "noEmit": true,
    "jsx": "react-jsx",
    "strict": true,
    "noUnusedLocals": true,
    "noUnusedParameters": true,
    "noFallthroughCasesInSwitch": true
  },
  "include": ["src"]
}
`)

	// 3. tsconfig.node.json
	archivos["tsconfig.node.json"] = []byte(`{
  "compilerOptions": {
    "composite": true,
    "skipLibCheck": true,
    "module": "ESNext",
    "moduleResolution": "bundler",
    "allowSyntheticDefaultImports": true,
    "strict": true
  },
  "include": ["vite.config.ts"]
}
`)

	// 4. vite.config.ts
	archivos["vite.config.ts"] = []byte(`import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';

export default defineConfig({
  plugins: [react()],
});
`)

	// 5. index.html
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

	archivos["index.html"] = []byte(fmt.Sprintf(`<!DOCTYPE html>
<html lang="es">
  <head>
    <meta charset="UTF-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1.0" />
    <title>%s</title>
  </head>
  <body>
    <div id="root"></div>
    <script type="module" src="/src/main.tsx"></script>
  </body>
</html>
`, nombreSitio))

	// 6. src/index.css
	archivos["src/index.css"] = []byte(`:root {
  font-family: system-ui, -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, Oxygen, Ubuntu, Cantarell, sans-serif;
  line-height: 1.5;
  font-weight: 400;
  color: #18181b;
  background-color: #ffffff;
}

body {
  margin: 0;
  min-width: 320px;
  min-height: 100dvh;
}
`)

	// 7. src/main.tsx
	archivos["src/main.tsx"] = []byte(`import React from 'react';
import ReactDOM from 'react-dom/client';
import App from './App';
import './index.css';

ReactDOM.createRoot(document.getElementById('root')!).render(
  <React.StrictMode>
    <App />
  </React.StrictMode>
);
`)

	// 8. src/App.tsx
	archivos["src/App.tsx"] = []byte(fmt.Sprintf(`import React from 'react';

export default function App() {
  return (
    <main style={{ padding: '2rem', maxWidth: '800px', margin: '0 auto' }}>
      <h1>%s</h1>
      <p>Sitio exportado desde Sytue Web conectado a la API pública.</p>
    </main>
  );
}
`, nombreSitio))

	// 9. src/api/pages-api.ts generado deterministamente desde apiweb
	clienteTS, err := apiweb.GenerarClienteTS(baseURL)
	if err != nil {
		return nil, fmt.Errorf("react: error generando pages-api.ts: %w", err)
	}
	archivos["src/api/pages-api.ts"] = clienteTS

	// 10. src/components/Formulario.tsx
	archivos["src/components/Formulario.tsx"] = []byte(`import React, { useState } from 'react';
import { pagesApi, resolverAltcha, type FormSubmission } from '../api/pages-api';

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
      const altchaSolution = await resolverAltcha(reto);

      const submission: FormSubmission = {
        fields,
        consent: true,
        language,
        altcha: altchaSolution,
      };

      await pagesApi.submitForm(formId, submission);
      setEnviado(true);
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : 'Error al enviar el formulario');
    } finally {
      setCargando(false);
    }
  }

  if (enviado) {
    return <div className="sytue-form-success">Mensaje recibido correctamente.</div>;
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

	// 11. src/components/BotonComprar.tsx
	archivos["src/components/BotonComprar.tsx"] = []byte(`import React, { useState } from 'react';
import { pagesApi, resolverAltcha, type CheckoutRequest } from '../api/pages-api';

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
      setError(err instanceof Error ? err.message : 'Error al procesar el pago');
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

	// 12. README.md
	archivos["README.md"] = []byte(fmt.Sprintf(`# %s (Exportación React SPA)

Este proyecto ha sido generado deterministamente desde **Sytue Web**.

## Conexión con la API pública

La aplicación se comunica con el backend de tu tienda en Sytue Web a través de:
`+"```"+`
%s/_pages/*
`+"```"+`

## Puesta en marcha

1. Instalar dependencias exactas:
   `+"```bash"+`
   npm install
   `+"```"+`
2. Iniciar servidor de desarrollo:
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
