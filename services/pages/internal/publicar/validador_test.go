// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package publicar

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidarSalidaCorrecta(t *testing.T) {
	p, err := CargarPermitido()
	if err != nil {
		t.Fatal(err)
	}

	RegistrarIsla("0123456789abcdef", []byte("console.log('islas');"))

	ficheros := []FicheroSalida{
		{
			Ruta: "_astro/0123456789abcdef.css",
			Contenido: []byte(`
				@font-face { font-family: 'Inter'; src: url('/_astro/inter.woff2'); }
				@media (min-width: 768px) { body { background: #fff; } }
				.hero { background: url('data:image/png;base64,iVBORw0KGgo='); }
			`),
		},
		{
			Ruta: "es/index.html",
			Contenido: []byte(`<!DOCTYPE html>
<html lang="es">
<head>
	<meta charset="utf-8">
	<title>Test</title>
	<link rel="stylesheet" href="/_astro/0123456789abcdef.css">
	<link rel="canonical" href="https://example.com/es/">
	<script defer src="/_sytue/s.js"></script>
	<script type="module" src="/_astro/islas-0123456789abcdef.js"></script>
	<script type="application/ld+json">{"@context":"https://schema.org","@type":"Organization","name":"Test"}</script>
</head>
<body>
	<main id="contenido">
		<h1>Hola</h1>
		<p>Bienvenido</p>
		<a href="/es/contacto/">Contacto</a>
		<img src="/_astro/img.jpg" alt="Foto" loading="lazy">
		<svg width="24" height="24" viewBox="0 0 24 24"><path d="M0 0h24v24H0z"/></svg>
		<form method="post" action="/_pages/form"><button type="submit">Enviar</button></form>
		<iframe src="https://www.openstreetmap.org/export/embed.html" loading="lazy" referrerpolicy="no-referrer"></iframe>
	</main>
</body>
</html>`),
		},
		{
			Ruta: "sitemap.xml",
			Contenido: []byte(`<?xml version="1.0" encoding="UTF-8"?>
<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
	<url>
		<loc>https://example.com/es/</loc>
		<lastmod>2026-09-30</lastmod>
	</url>
</urlset>`),
		},
		{
			Ruta: "robots.txt",
			Contenido: []byte(`User-agent: *
Allow: /
Sitemap: https://example.com/sitemap.xml`),
		},
	}

	opts := OpcionesValidacion{
		Kind:            "live",
		CastOrigin:      "https://cast.example.com",
		HostsPermitidos: []string{"example.com"},
		Idiomas:         []string{"es"},
		RecursosValidos: map[string]bool{
			"/_astro/inter.woff2": true,
			"/_astro/img.jpg":     true,
		},
	}

	if err := ValidarSalida(p, ficheros, opts); err != nil {
		t.Fatalf("ValidarSalida debió pasar pero falló: %v", err)
	}
}

func TestValidarSalidaRechazaPeligros(t *testing.T) {
	p, err := CargarPermitido()
	if err != nil {
		t.Fatal(err)
	}

	casos := []struct {
		nombre string
		f      FicheroSalida
		kind   string
	}{
		{
			nombre: "subida de fichero js",
			f:      FicheroSalida{Ruta: "app.js", Contenido: []byte("alert(1)")},
		},
		{
			nombre: "html con script arbitrario",
			f: FicheroSalida{
				Ruta:      "es/index.html",
				Contenido: []byte(`<html><head><script>alert(1)</script></head><body><h1>X</h1></body></html>`),
			},
		},
		{
			nombre: "html con onmouseover",
			f: FicheroSalida{
				Ruta:      "es/index.html",
				Contenido: []byte(`<html><body><div onmouseover="alert(1)">X</div></body></html>`),
			},
		},
		{
			nombre: "html con style inline",
			f: FicheroSalida{
				Ruta:      "es/index.html",
				Contenido: []byte(`<html><body><div style="color:red">X</div></body></html>`),
			},
		},
		{
			nombre: "html con base",
			f: FicheroSalida{
				Ruta:      "es/index.html",
				Contenido: []byte(`<html><head><base href="https://evil.com"></head><body><h1>X</h1></body></html>`),
			},
		},
		{
			nombre: "svg con use/href",
			f: FicheroSalida{
				Ruta:      "es/index.html",
				Contenido: []byte(`<html><body><svg><use href="https://evil.com/x.svg"/></svg></body></html>`),
			},
		},
		{
			nombre: "iframe con origen no permitido",
			f: FicheroSalida{
				Ruta:      "es/index.html",
				Contenido: []byte(`<html><body><iframe src="https://evil.com/embed"></iframe></body></html>`),
			},
		},
		{
			nombre: "css con @import",
			f: FicheroSalida{
				Ruta:      "_astro/0123456789abcdef.css",
				Contenido: []byte(`@import url("https://evil.com/style.css");`),
			},
		},
		{
			nombre: "css con javascript:",
			f: FicheroSalida{
				Ruta:      "_astro/0123456789abcdef.css",
				Contenido: []byte(`.box { background: url('javascript:alert(1)'); }`),
			},
		},
		{
			nombre: "vista previa con formulario",
			kind:   "preview",
			f: FicheroSalida{
				Ruta: "es/index.html",
				Contenido: []byte(`<html><body><div class="sp-vista" role="status">Vista previa</div>
					<form method="post" action="/_pages/f"><button type="submit">Enviar</button></form>
				</body></html>`),
			},
		},
		{
			nombre: "vista previa sin franja sp-vista",
			kind:   "preview",
			f: FicheroSalida{
				Ruta:      "es/index.html",
				Contenido: []byte(`<html><body><main id="contenido"><h1>Hola</h1></main></body></html>`),
			},
		},
		{
			nombre: "json-ld con secuencia de escape </script>",
			f: FicheroSalida{
				Ruta: "es/index.html",
				Contenido: []byte(`<html><head>
					<script type="application/ld+json">{"name":"</script><script>alert(1)</script>"}</script>
				</head><body><h1>Hola</h1></body></html>`),
			},
		},
	}

	for _, tc := range casos {
		t.Run(tc.nombre, func(t *testing.T) {
			opts := OpcionesValidacion{
				Kind:            tc.kind,
				CastOrigin:      "https://cast.example.com",
				HostsPermitidos: []string{"example.com"},
				Idiomas:         []string{"es"},
			}
			if opts.Kind == "" {
				opts.Kind = "live"
			}
			err := ValidarSalida(p, []FicheroSalida{tc.f}, opts)
			if err == nil {
				t.Fatalf("ValidarSalida debió fallar en caso %q pero pasó", tc.nombre)
			}
		})
	}
}

// TestLaSalidaDelRenderizadorPasa (PAGES-ENCARGO §7.4):
// pasa el validador sobre los ficheros de oro de REN (ruta relativa; se salta diciéndolo si aún no existen);
// verlo en rojo quitando svg de la lista.
func TestLaSalidaDelRenderizadorPasa(t *testing.T) {
	p, err := CargarPermitido()
	if err != nil {
		t.Fatal(err)
	}

	// Buscar ficheros de oro en ../../../../apps/frontend/packages/pages-render/test/oro
	rutaOro := filepath.Join("..", "..", "..", "..", "apps", "frontend", "packages", "pages-render", "test", "oro")
	info, err := os.Stat(rutaOro)
	if err != nil || !info.IsDir() {
		t.Skip("saltando: no existen los ficheros de oro de REN (" + rutaOro + ")")
		return
	}

	var ficheros []FicheroSalida
	err = filepath.Walk(rutaOro, func(path string, fi os.FileInfo, err error) error {
		if err != nil || fi.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(rutaOro, path)
		contenido, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		ficheros = append(ficheros, FicheroSalida{
			Ruta:      filepath.ToSlash(rel),
			Contenido: contenido,
		})
		return nil
	})
	if err != nil {
		t.Fatalf("error leyendo ficheros de oro: %v", err)
	}
	if len(ficheros) == 0 {
		t.Skip("saltando: directorio de oro vacío")
		return
	}

	opts := OpcionesValidacion{
		Kind:            "live",
		HostsPermitidos: []string{"demo.example.test"},
		Idiomas:         []string{"es", "en"},
	}

	if err := ValidarSalida(p, ficheros, opts); err != nil {
		t.Fatalf("la salida de oro de REN no pasó el validador: %v", err)
	}
}
