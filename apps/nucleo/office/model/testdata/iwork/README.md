<a id="ficheros-de-iwork-de-prueba"></a>
# iWork test files

Third-party files redistributed under their license, unmodified. They are used by
`import_iwork*_test.go`. Before being added, each one was checked to make sure it carried no
personal data: metadata (`buildVersionHistory.plist`, `Metadata/*`, `sf:authors`), comment
authors and email addresses. Anything that contained any of these was left out.

| File                          | What it is                                                                                     | Source      | License    |
| ----------------------------- | ---------------------------------------------------------------------------------------------- | ----------- | ---------- |
| `testPages.pages`             | Pages '09 (XML) with `QuickLook/Thumbnail.jpg` and `Preview.pdf`                               | Apache Tika | Apache-2.0 |
| `testPagesPwdProtected.pages` | Password-protected Pages '09                                                                   | Apache Tika | Apache-2.0 |
| `testPages2013.pages`         | Pages 2013+ (IWA) with `preview.jpg`                                                           | Apache Tika | Apache-2.0 |
| `testKeynote2018.key`         | Keynote 2018: the ZIP wraps the package folder, with `Index.zip` inside and no preview image   | Apache Tika | Apache-2.0 |

**Apache Tika**: `tika-parsers/tika-parsers-standard/tika-parsers-standard-modules/tika-parser-apple-module/src/test/resources/test-documents/`
from <https://github.com/apache/tika>, commit `49e40e6ddaf4`. Apache 2.0 license
(<https://www.apache.org/licenses/LICENSE-2.0>); its notice is in `NOTICE` in this folder.

The provenance of what is known about these formats is in `docs/security.md`, §4.4,
“Formatos de iWork” (iWork formats).
