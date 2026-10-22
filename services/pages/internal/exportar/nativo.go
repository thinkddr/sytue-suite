// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package exportar

import (
	"fmt"

	"git.sytue.eu/sytue/one/services/pages/internal/exportar/apiweb"
	"git.sytue.eu/sytue/one/services/pages/internal/ir"
)

// generarNativo entrega un proyecto mínimo abierto por la herramienta nativa,
// además del cliente Pages generado. No incluye wrappers ni carpetas que deben
// regenerar Flutter, Gradle o Xcode en la máquina de destino.
func generarNativo(destino Destino, sitio *ir.Sitio) (map[string][]byte, error) {
	baseURL := obtenerBaseURLSitio(sitio)
	nombre := obtenerTextoIdioma(sitio.SEO.SiteName, sitio.Languages.Default)
	if nombre == "" {
		nombre = "Sytue Web"
	}
	archivos := map[string][]byte{
		"README.md": []byte("# Sytue Web native export\n\nEl catálogo, los formularios y el checkout siguen en la web publicada de Sytue Web. No despubliques el sitio mientras este cliente esté activo.\n\nRegla de layout: fill vertical en una página desplazable es hug; fill en fila usa weight/Expanded/maxWidth; wrap usa el flujo de cada plataforma.\n"),
	}
	switch destino {
	case DestinoFlutter:
		archivos["pubspec.yaml"] = []byte("name: sytue_pages_export\ndescription: Sytue Web export\nenvironment:\n  sdk: '>=3.0.0 <4.0.0'\n")
		archivos["lib/pages_api.dart"] = apiweb.GenerarClienteDart(baseURL)
		archivos["lib/main.dart"] = []byte(fmt.Sprintf("import 'package:flutter/material.dart';\n\nvoid main() => runApp(const PagesApp());\nclass PagesApp extends StatelessWidget { const PagesApp({super.key}); @override Widget build(BuildContext context) => const MaterialApp(home: Scaffold(body: Center(child: Text(%q)))); }\n", nombre))
	case DestinoCompose:
		archivos["settings.gradle.kts"] = []byte("pluginManagement { repositories { google(); mavenCentral(); gradlePluginPortal() } }\nrootProject.name = \"SytuePagesExport\"\ninclude(\":app\")\n")
		archivos["build.gradle.kts"] = []byte("plugins {\n    id(\"com.android.application\") version \"8.8.2\" apply false\n    id(\"org.jetbrains.kotlin.android\") version \"2.1.10\" apply false\n}\n")
		archivos["app/build.gradle.kts"] = []byte("plugins { id(\"com.android.application\"); id(\"org.jetbrains.kotlin.android\"); id(\"org.jetbrains.kotlin.plugin.compose\") }\n\nandroid { namespace = \"pages\"; compileSdk = 35; defaultConfig { applicationId = \"pages.export\"; minSdk = 26; targetSdk = 35; versionCode = 1; versionName = \"1.0\" } }\n")
		archivos["app/src/main/java/pagesapi/PagesApi.kt"] = apiweb.GenerarClienteKotlin(baseURL)
		archivos["app/src/main/java/pages/MainActivity.kt"] = []byte(fmt.Sprintf("package pages\n\nimport androidx.activity.ComponentActivity\nimport androidx.activity.compose.setContent\nimport androidx.compose.material3.Text\n\nclass MainActivity : ComponentActivity() { override fun onCreate(state: android.os.Bundle?) { super.onCreate(state); setContent { Text(%q) } } }\n", nombre))
	case DestinoSwiftUI:
		archivos["Package.swift"] = []byte("// swift-tools-version: 6.0\nimport PackageDescription\nlet package = Package(name: \"SytuePagesExport\", platforms: [.iOS(.v17)], products: [.library(name: \"PagesAPI\", targets: [\"PagesAPI\"])], targets: [.target(name: \"PagesAPI\")])\n")
		archivos["Sources/PagesAPI/PagesApi.swift"] = apiweb.GenerarClienteSwift(baseURL)
		archivos["Sources/PagesAPI/PagesView.swift"] = []byte(fmt.Sprintf("import SwiftUI\n\npublic struct PagesView: View {\n    public init() {}\n    public var body: some View {\n        ScrollView { Text(%q).frame(maxWidth: .infinity) }\n    }\n}\n", nombre))
		archivos["Flujo.swift"] = []byte("import SwiftUI\n\npublic struct Flujo: Layout {\n    public init() {}\n    public func sizeThatFits(proposal: ProposedViewSize, subviews: Subviews, cache: inout ()) -> CGSize {\n        CGSize(width: proposal.width ?? 0, height: subviews.map { $0.sizeThatFits(.unspecified).height }.max() ?? 0)\n    }\n    public func placeSubviews(in bounds: CGRect, proposal: ProposedViewSize, subviews: Subviews, cache: inout ()) {\n        var x = bounds.minX\n        for view in subviews {\n            let size = view.sizeThatFits(.unspecified)\n            if x + size.width > bounds.maxX { x = bounds.minX }\n            view.place(at: CGPoint(x: x, y: bounds.minY), proposal: .unspecified)\n            x += size.width\n        }\n    }\n}\n")
	case DestinoFyne:
		v, _ := ObtenerVersion("fyne_version")
		archivos["go.mod"] = []byte(fmt.Sprintf("module sytue-pages-export\n\ngo 1.26\n\nrequire fyne.io/fyne/v2 v%s\n", v))
		archivos["pagesapi/pagesapi.go"] = apiweb.GenerarClienteGo(baseURL)
		archivos["main.go"] = []byte(fmt.Sprintf("package main\n\nimport (\n  \"fyne.io/fyne/v2\"\n  \"fyne.io/fyne/v2/app\"\n  \"fyne.io/fyne/v2/container\"\n  \"fyne.io/fyne/v2/widget\"\n)\n\nfunc main() { a := app.New(); w := a.NewWindow(%q); w.SetContent(container.NewVBox(widget.NewLabel(%q))); w.Resize(fyne.NewSize(480, 320)); w.ShowAndRun() }\n", nombre, nombre))
		archivos["flex.go"] = []byte("package main\n\nimport (\n  \"fyne.io/fyne/v2\"\n  \"fyne.io/fyne/v2/container\"\n  \"fyne.io/fyne/v2/layout\"\n)\n\nfunc fila(objetos ...fyne.CanvasObject) *fyne.Container { return container.NewHBox(objetos...) }\nfunc filaConRelleno(izquierda, derecha fyne.CanvasObject) *fyne.Container { return container.NewHBox(izquierda, layout.NewSpacer(), derecha) }\nfunc rejilla(ancho float32, objetos ...fyne.CanvasObject) *fyne.Container { return container.New(layout.NewGridWrapLayout(fyne.NewSize(ancho, ancho)), objetos...) }\n")
	default:
		return nil, fmt.Errorf("nativo: destino inválido %q", destino)
	}
	return archivos, nil
}
