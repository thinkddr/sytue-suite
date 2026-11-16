# Ficheros de iWork de prueba

Ficheros de terceros que se redistribuyen con su licencia, sin modificar. Los usan
`import_iwork*_test.go`. Antes de entrar, a cada uno se le revisó que no llevara datos
personales: metadatos (`buildVersionHistory.plist`, `Metadata/*`, `sf:authors`), autores
de comentarios y direcciones de correo. Lo que tuviera alguno, se quedó fuera.

| Fichero                       | Qué es                                                                                      | Origen      | Licencia   |
| ----------------------------- | ------------------------------------------------------------------------------------------- | ----------- | ---------- |
| `testPages.pages`             | Pages '09 (XML) con `QuickLook/Thumbnail.jpg` y `Preview.pdf`                               | Apache Tika | Apache-2.0 |
| `testPagesPwdProtected.pages` | Pages '09 protegido con contraseña                                                          | Apache Tika | Apache-2.0 |
| `testPages2013.pages`         | Pages 2013+ (IWA) con `preview.jpg`                                                         | Apache Tika | Apache-2.0 |
| `testKeynote2018.key`         | Keynote 2018: el ZIP envuelve la carpeta del paquete, `Index.zip` dentro y sin vista previa | Apache Tika | Apache-2.0 |

**Apache Tika**: `tika-parsers/tika-parsers-standard/tika-parsers-standard-modules/tika-parser-apple-module/src/test/resources/test-documents/`
de <https://github.com/apache/tika>, commit `49e40e6ddaf4`. Licencia Apache 2.0
(<https://www.apache.org/licenses/LICENSE-2.0>); su aviso está en `NOTICE` en esta carpeta.

La procedencia de lo que se sabe de estos formatos está en `docs/security.md`, §4.4,
«Formatos de iWork».
