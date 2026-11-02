<!--
SPDX-License-Identifier: Apache-2.0
Copyright (c) 2026 Daniel Delgado Ramos (thinkddr)
-->

# Documentación — Sytue Suite

**Un tema, un documento.** Toda la documentación vive en los nueve ficheros de abajo. Si dos se
contradicen, manda `CONVENTIONS.md`; corrige el otro en el mismo cambio.

| Documento                              | Qué contiene                                                                                                                                                                                                                                                                                         | Cuándo lo abres                                                                  |
| -------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------- |
| ★ **[CONVENTIONS.md](CONVENTIONS.md)** | **Contrato canónico**: principios nucleares, catálogo de productos, versiones del stack, puertos, layout del monorepo, identidad, tenancy, licencias y, en el anexo §16, naming, dominios y DNS.                                                                                                     | Antes de nombrar, versionar o exponer cualquier cosa. Manda sobre todo lo demás. |
| **[architecture.md](architecture.md)** | Arquitectura de la suite (C4, flujos SSO/intents/deploy, multi-tenancy, topología), integración Shell+Hub, Sytue Cloud, Integration Gateway (saliente), capa de compatibilidad (entrante), identidad enterprise y el hogar de los documentos en Drive.                                               | Para entender cómo encajan las piezas antes de tocar una.                        |
| **[decisions.md](decisions.md)**       | Las decisiones de arquitectura (ADR) con contexto, alternativas descartadas y consecuencias, incluido el pivote «Sytue Native» y el [0037](decisions.md#0037), que fija cómo se comparte el código.                                                                                                  | Cuando te preguntes _por qué_ algo es así, o vayas a cambiarlo.                  |
| **[security.md](security.md)**         | Seguridad y RGPD, modelo de amenazas STRIDE, checklist OWASP ASVS L2, política clean-room y licencias.                                                                                                                                                                                               | Al tocar auth, datos personales, dependencias o el gate de licencias.            |
| **[operations.md](operations.md)**     | Guía de despliegue y operación: CLI interactivo (`./sytue-deploy`), Docker Compose, Kubernetes/Helm, variables de entorno, migraciones, identidad, tenants, correo, backups, secretos, CI/CD, pruebas e2e y rendimiento/SEO.                                                                         | Para levantar, desplegar, configurar, verificar o recuperar algo.                |
| **[design.md](design.md)**             | Dirección de arte, sistema de diseño (`@sytue/ui`, tokens `--sytue-*`, AppShell, componentes, WCAG AA, configuración guiada) y assets.                                                                                                                                                               | Antes de escribir una línea de UI.                                               |
| **[roadmap.md](roadmap.md)**           | Estado y madurez por producto, arquitectura entregada y hoja de ruta pública de la suite.                                                                                                                                                                                                            | Para conocer las capacidades actuales y la evolución de la suite.                |
| **[paridad-pdf.md](paridad-pdf.md)**   | Sytue CertiPDF función a función con la referencia de implementación de cada herramienta.                                                                                                                                                                                                            | Para consultar las capacidades de edición, firma eIDAS y OCR de PDF.             |
| **[business.md](business.md)**         | Visión de plataforma (posicionamiento, interoperabilidad, Platform OS) y modelo de dimensionamiento.                                                                                                                                                                                                 | Para entender el enfoque de producto e interoperabilidad.                        |

**Fuera de `docs/`:** `branding/` es la fuente de verdad de logos, iconos, ilustraciones y tokens.

<a id="reglas"></a>

## Reglas

- **No crear ficheros nuevos en `docs/`.** Lo nuevo entra como sección del documento que le
  corresponde, con su `<a id="...">` para poder enlazarlo. Si de verdad no encaja en ninguno, es
  señal de que falta una decisión, no un fichero.
- **Antes de escribir código**, [`CONVENTIONS.md` §13.2](CONVENTIONS.md#13-2-lo-que-ya-existe):
  el catálogo de lo que ya existe en `platform/pkg/` y en `apps/frontend/packages/`, qué NO se
  unifica y por qué, y los tests que se ponen en rojo cuando alguien vuelve a copiar plomería.
- **Cada afirmación, anclada al código.** Los documentos citan `fichero:línea` o el comando que lo
  verifica; lo que no se puede verificar se marca como pendiente, no se pinta como hecho.
- **Al entregar algo, actualízalo donde estaba anotado** (estado en `roadmap.md`, decisión en
  `decisions.md`), no en un documento nuevo.
