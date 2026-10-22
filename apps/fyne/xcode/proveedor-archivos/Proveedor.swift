// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
//
// SYTUE DRIVE EN EL FINDER (macOS 13) Y EN ARCHIVOS (iOS 16): la extensión File Provider
// de la app de Drive. La arranca el sistema cuando hace falta, con la app CERRADA, y habla
// WebDAV con Drive (`/dav`, services/drive/internal/webdav) con la contraseña de aplicación
// que la app dejó en el llavero compartido (apps/nucleo/sistema/archivos.go). Los ficheros
// se bajan cuando se abren y se suben cuando se guardan: el sistema lleva la copia local.
//
// La identidad de cada elemento es un identificador propio, no la ruta: el sistema exige
// que no cambie al renombrar o mover, y en WebDAV la ruta es lo único que hay. `Indice`
// guarda la correspondencia en el contenedor de la extensión.
//
// ponytail: sin aviso de cambios remotos. Lo que se cambie en Drive desde otro sitio se
// ve cuando el sistema vuelve a listar la carpeta; para verlo al momento hace falta que el
// servidor empuje (PushKit de File Provider) o un `sync-collection` (RFC 6578) en /dav.
// ponytail: sin If-Match al subir: si dos equipos guardan el mismo fichero, gana el último.
//
// El proyecto lo genera apps/fyne/xcode/generar.py (target «archivos-drive»).

import FileProvider
import Foundation
import UniformTypeIdentifiers

// MARK: - La credencial

/// Lo que la app de Drive deja en el llavero: `sistema.CredencialArchivos`.
struct Credencial: Decodable {
    let url: String
    let usuario: String
    let clave: String
}

enum Llavero {
    // Los de apps/nucleo/sistema/archivos.go; lo vigila su archivos_test.go.
    static let grupo = "VHC5QW7BZ3.eu.sytue.drive.shared"
    static let servicio = "eu.sytue.drive.archivos"
    static let cuenta = "webdav"

    static func credencial() -> Credencial? {
        let consulta: [String: Any] = [
            kSecClass as String: kSecClassGenericPassword,
            kSecAttrService as String: servicio,
            kSecAttrAccount as String: cuenta,
            kSecAttrAccessGroup as String: grupo,
            kSecUseDataProtectionKeychain as String: true,
            kSecReturnData as String: true,
            kSecMatchLimit as String: kSecMatchLimitOne,
        ]
        var salida: CFTypeRef?
        guard SecItemCopyMatching(consulta as CFDictionary, &salida) == errSecSuccess,
              let datos = salida as? Data else { return nil }
        return try? JSONDecoder().decode(Credencial.self, from: datos)
    }
}

// MARK: - WebDAV

/// Una entrada de un PROPFIND. `ruta` va sin barra final, y la raíz es "/".
struct Entrada {
    var ruta = ""
    var carpeta = false
    var tamano: Int64?
    var fecha: Date?
    var etag = ""
}

final class DAV {
    let base: String
    let autorizacion: String
    let sesion: URLSession

    init(_ c: Credencial) throws {
        guard let u = URL(string: c.url), u.scheme == "https" else {
            throw NSFileProviderError(.notAuthenticated)
        }
        base = u.absoluteString.hasSuffix("/") ? String(u.absoluteString.dropLast()) : u.absoluteString
        autorizacion = "Basic " + Data("\(c.usuario):\(c.clave)".utf8).base64EncodedString()
        // Sin cookies ni caché: cada petición va con su credencial y nada se queda en disco.
        sesion = URLSession(configuration: .ephemeral)
    }

    /// La URL de `ruta`; las carpetas con barra final, que es como las nombra WebDAV.
    func url(_ ruta: String, carpeta: Bool) -> URL {
        var r = ruta.addingPercentEncoding(withAllowedCharacters: .urlPathAllowed) ?? ruta
        if carpeta && !r.hasSuffix("/") { r += "/" }
        return URL(string: base + r)!
    }

    func peticion(_ metodo: String, _ ruta: String, carpeta: Bool = false) -> URLRequest {
        var r = URLRequest(url: url(ruta, carpeta: carpeta))
        r.httpMethod = metodo
        r.setValue(autorizacion, forHTTPHeaderField: "Authorization")
        return r
    }

    /// Lo que el sistema entiende de cada respuesta: 401 es «vuelve a entrar en la app».
    static func comprobar(_ respuesta: URLResponse, admite: Set<Int> = []) throws {
        guard let h = respuesta as? HTTPURLResponse else { throw URLError(.badServerResponse) }
        if (200..<300).contains(h.statusCode) || admite.contains(h.statusCode) { return }
        switch h.statusCode {
        case 401: throw NSFileProviderError(.notAuthenticated)
        case 403: throw CocoaError(.fileWriteNoPermission)
        case 404: throw NSFileProviderError(.noSuchItem)
        case 412: throw NSFileProviderError(.filenameCollision)
        case 413, 507: throw NSFileProviderError(.insufficientQuota)
        default: throw URLError(.badServerResponse)
        }
    }

    /// Sin red el sistema reintenta solo si se lo dice así.
    static func traducir(_ e: Error) -> Error {
        guard let u = e as? URLError else { return e }
        switch u.code {
        case .notConnectedToInternet, .timedOut, .cannotConnectToHost, .cannotFindHost,
             .networkConnectionLost, .dnsLookupFailed:
            return NSFileProviderError(.serverUnreachable)
        default:
            return e
        }
    }

    func hacer(_ r: URLRequest, admite: Set<Int> = []) async throws -> Data {
        let (datos, respuesta) = try await sesion.data(for: r)
        try DAV.comprobar(respuesta, admite: admite)
        return datos
    }

    static let propfind = Data("""
    <?xml version="1.0" encoding="utf-8"?>
    <d:propfind xmlns:d="DAV:"><d:prop><d:resourcetype/><d:getcontentlength/><d:getlastmodified/><d:getetag/></d:prop></d:propfind>
    """.utf8)

    /// PROPFIND con `profundidad` "0" (la propia) o "1" (y sus hijos).
    func listar(_ ruta: String, profundidad: String) async throws -> [Entrada] {
        var r = peticion("PROPFIND", ruta, carpeta: profundidad == "1" || ruta == "/")
        r.setValue(profundidad, forHTTPHeaderField: "Depth")
        r.setValue("application/xml; charset=utf-8", forHTTPHeaderField: "Content-Type")
        r.httpBody = DAV.propfind
        let datos = try await hacer(r)
        let lector = Multiestado(raiz: URL(string: base + "/")!)
        let xml = XMLParser(data: datos)
        xml.shouldProcessNamespaces = true
        xml.delegate = lector
        guard xml.parse() else { throw URLError(.cannotParseResponse) }
        return lector.entradas
    }

    /// La entrada de `ruta`, o nil si no existe.
    func existe(_ ruta: String) async throws -> Entrada? {
        do {
            return try await listar(ruta, profundidad: "0").first
        } catch NSFileProviderError.noSuchItem {
            return nil
        }
    }

    func bajar(_ ruta: String, a destino: URL) async throws {
        let (temporal, respuesta) = try await sesion.download(for: peticion("GET", ruta))
        try DAV.comprobar(respuesta)
        try? FileManager.default.removeItem(at: destino)
        try FileManager.default.moveItem(at: temporal, to: destino)
    }

    func subir(_ ruta: String, desde origen: URL?) async throws {
        var r = peticion("PUT", ruta)
        r.setValue("application/octet-stream", forHTTPHeaderField: "Content-Type")
        let respuesta: URLResponse
        if let origen {
            (_, respuesta) = try await sesion.upload(for: r, fromFile: origen)
        } else {
            (_, respuesta) = try await sesion.upload(for: r, from: Data())
        }
        try DAV.comprobar(respuesta)
    }

    func carpeta(_ ruta: String) async throws {
        _ = try await hacer(peticion("MKCOL", ruta, carpeta: true))
    }

    func mover(_ de: String, a: String, carpeta: Bool) async throws {
        var r = peticion("MOVE", de, carpeta: carpeta)
        r.setValue(url(a, carpeta: carpeta).absoluteString, forHTTPHeaderField: "Destination")
        r.setValue("F", forHTTPHeaderField: "Overwrite")
        _ = try await hacer(r)
    }

    func borrar(_ ruta: String, carpeta: Bool) async throws {
        // Lo que ya no está, borrado está.
        _ = try await hacer(peticion("DELETE", ruta, carpeta: carpeta), admite: [404])
    }
}

/// Lee el 207 Multi-Status de un PROPFIND.
final class Multiestado: NSObject, XMLParserDelegate {
    let raiz: URL
    var entradas: [Entrada] = []
    private var actual: Entrada?
    private var texto = ""

    static let fechas: DateFormatter = {
        let f = DateFormatter()
        f.locale = Locale(identifier: "en_US_POSIX")
        f.timeZone = TimeZone(identifier: "GMT")
        f.dateFormat = "EEE, dd MMM yyyy HH:mm:ss zzz"
        return f
    }()

    init(raiz: URL) { self.raiz = raiz }

    func parser(_ parser: XMLParser, didStartElement elemento: String, namespaceURI: String?,
                qualifiedName: String?, attributes: [String: String] = [:]) {
        texto = ""
        if elemento == "response" { actual = Entrada() }
        if elemento == "collection" { actual?.carpeta = true }
    }

    func parser(_ parser: XMLParser, foundCharacters string: String) { texto += string }

    func parser(_ parser: XMLParser, didEndElement elemento: String, namespaceURI: String?,
                qualifiedName: String?) {
        let t = texto.trimmingCharacters(in: .whitespacesAndNewlines)
        switch elemento {
        case "href":
            // La ruta, sin el prefijo de la raíz WebDAV (/dav) y decodificada.
            let camino = URL(string: t, relativeTo: raiz)?.path ?? t
            var r = String(camino.dropFirst(raiz.path.count))
            if !r.hasPrefix("/") { r = "/" + r }
            while r.count > 1 && r.hasSuffix("/") { r.removeLast() }
            actual?.ruta = r
        case "getcontentlength": actual?.tamano = Int64(t)
        case "getlastmodified": actual?.fecha = Multiestado.fechas.date(from: t)
        case "getetag": actual?.etag = t
        case "response":
            if let e = actual { entradas.append(e) }
            actual = nil
        default: break
        }
        texto = ""
    }
}

// MARK: - Identificadores estables

/// Ruta ↔ identificador, guardado en el contenedor de la extensión.
///
/// ponytail: un JSON entero que se reescribe tras cada operación; SQLite si las unidades
/// crecen hasta notarse.
final class Indice {
    private var deRuta: [String: String] = [:]
    private var deId: [String: String] = [:]
    private var sucio = false
    private let fichero: URL
    private let cerrojo = NSLock()

    init(dominio: NSFileProviderDomain) {
        let dir = (FileManager.default.urls(for: .applicationSupportDirectory, in: .userDomainMask).first
            ?? FileManager.default.temporaryDirectory)
        try? FileManager.default.createDirectory(at: dir, withIntermediateDirectories: true)
        fichero = dir.appendingPathComponent("indice-\(dominio.identifier.rawValue).json")
        if let datos = try? Data(contentsOf: fichero),
           let leido = try? JSONDecoder().decode([String: String].self, from: datos) {
            deRuta = leido
            for (r, i) in leido { deId[i] = r }
        }
    }

    func id(_ ruta: String) -> NSFileProviderItemIdentifier {
        if ruta == "/" { return .rootContainer }
        cerrojo.lock()
        defer { cerrojo.unlock() }
        if let i = deRuta[ruta] { return NSFileProviderItemIdentifier(i) }
        let i = UUID().uuidString
        deRuta[ruta] = i
        deId[i] = ruta
        sucio = true
        return NSFileProviderItemIdentifier(i)
    }

    func ruta(_ id: NSFileProviderItemIdentifier) -> String? {
        if id == .rootContainer { return "/" }
        cerrojo.lock()
        defer { cerrojo.unlock() }
        return deId[id.rawValue]
    }

    /// Lo que estaba en `de` o debajo pasa a `a`, con los mismos identificadores.
    func mover(_ de: String, a: String) {
        cerrojo.lock()
        defer { cerrojo.unlock() }
        for (r, i) in deRuta where r == de || r.hasPrefix(de + "/") {
            let nueva = a + r.dropFirst(de.count)
            deRuta[r] = nil
            deRuta[nueva] = i
            deId[i] = nueva
        }
        sucio = true
    }

    func olvidar(_ ruta: String) {
        cerrojo.lock()
        defer { cerrojo.unlock() }
        for (r, i) in deRuta where r == ruta || r.hasPrefix(ruta + "/") {
            deRuta[r] = nil
            deId[i] = nil
        }
        sucio = true
    }

    func guardar() {
        cerrojo.lock()
        defer { cerrojo.unlock() }
        guard sucio, let datos = try? JSONEncoder().encode(deRuta) else { return }
        if (try? datos.write(to: fichero, options: .atomic)) != nil { sucio = false }
    }
}

// MARK: - Los elementos

final class Item: NSObject, NSFileProviderItem {
    let itemIdentifier: NSFileProviderItemIdentifier
    let parentItemIdentifier: NSFileProviderItemIdentifier
    let filename: String
    let contentType: UTType
    let documentSize: NSNumber?
    let contentModificationDate: Date?
    let itemVersion: NSFileProviderItemVersion

    init(_ e: Entrada, indice: Indice) {
        itemIdentifier = indice.id(e.ruta)
        let padre = (e.ruta as NSString).deletingLastPathComponent
        parentItemIdentifier = indice.id(padre.isEmpty ? "/" : padre)
        filename = e.ruta == "/" ? "Sytue Drive" : (e.ruta as NSString).lastPathComponent
        contentType = e.carpeta ? .folder
            : (UTType(filenameExtension: (filename as NSString).pathExtension) ?? .data)
        documentSize = e.carpeta ? nil : e.tamano.map { NSNumber(value: $0) }
        contentModificationDate = e.fecha
        // La ETag de WebDAV es la versión: cambia con el contenido y la fecha.
        let version = Data((e.etag.isEmpty ? "\(e.fecha?.timeIntervalSince1970 ?? 0)" : e.etag).utf8)
        itemVersion = NSFileProviderItemVersion(contentVersion: version, metadataVersion: version)
    }

    var capabilities: NSFileProviderItemCapabilities {
        var c: NSFileProviderItemCapabilities = [.allowsReading, .allowsWriting, .allowsRenaming,
                                                 .allowsReparenting, .allowsDeleting]
        if contentType == .folder { c.formUnion([.allowsContentEnumerating, .allowsAddingSubItems]) }
        return c
    }
}

// MARK: - La extensión

final class Proveedor: NSObject, NSFileProviderReplicatedExtension {
    let dominio: NSFileProviderDomain
    let indice: Indice

    required init(domain: NSFileProviderDomain) {
        dominio = domain
        indice = Indice(dominio: domain)
        super.init()
    }

    func invalidate() { indice.guardar() }

    /// La credencial se lee en cada operación: si la app la cambia o la borra, vale ya.
    func dav() throws -> DAV {
        guard let c = Llavero.credencial() else { throw NSFileProviderError(.notAuthenticated) }
        return try DAV(c)
    }

    /// Corre `cuerpo` aparte y devuelve el Progress que pide el sistema. El fallo, traducido.
    func trabajo(_ cuerpo: @escaping () async throws -> Void, fallo: @escaping (Error) -> Void) -> Progress {
        let p = Progress(totalUnitCount: 1)
        let t = Task {
            do {
                try await cuerpo()
            } catch {
                fallo(DAV.traducir(error))
            }
            self.indice.guardar()
            p.completedUnitCount = 1
        }
        p.cancellationHandler = { t.cancel() }
        return p
    }

    func ruta(_ id: NSFileProviderItemIdentifier) throws -> String {
        guard let r = indice.ruta(id) else { throw NSFileProviderError(.noSuchItem) }
        return r
    }

    func elemento(_ d: DAV, _ ruta: String) async throws -> Item {
        guard let e = try await d.existe(ruta) else { throw NSFileProviderError(.noSuchItem) }
        return Item(e, indice: indice)
    }

    static func unir(_ padre: String, _ nombre: String) -> String {
        padre == "/" ? "/" + nombre : padre + "/" + nombre
    }

    func item(for identifier: NSFileProviderItemIdentifier, request: NSFileProviderRequest,
              completionHandler: @escaping (NSFileProviderItem?, Error?) -> Void) -> Progress {
        trabajo({
            completionHandler(try await self.elemento(self.dav(), self.ruta(identifier)), nil)
        }, fallo: { completionHandler(nil, $0) })
    }

    func fetchContents(for itemIdentifier: NSFileProviderItemIdentifier, version requestedVersion: NSFileProviderItemVersion?,
                       request: NSFileProviderRequest,
                       completionHandler: @escaping (URL?, NSFileProviderItem?, Error?) -> Void) -> Progress {
        trabajo({
            let d = try self.dav()
            let ruta = try self.ruta(itemIdentifier)
            let item = try await self.elemento(d, ruta)
            guard let dir = try NSFileProviderManager(for: self.dominio)?.temporaryDirectoryURL() else {
                throw NSFileProviderError(.noSuchItem)
            }
            let destino = dir.appendingPathComponent(UUID().uuidString)
            try await d.bajar(ruta, a: destino)
            completionHandler(destino, item, nil)
        }, fallo: { completionHandler(nil, nil, $0) })
    }

    func createItem(basedOn itemTemplate: NSFileProviderItem, fields: NSFileProviderItemFields, contents url: URL?,
                    options: NSFileProviderCreateItemOptions = [], request: NSFileProviderRequest,
                    completionHandler: @escaping (NSFileProviderItem?, NSFileProviderItemFields, Bool, Error?) -> Void) -> Progress {
        trabajo({
            let d = try self.dav()
            let ruta = Proveedor.unir(try self.ruta(itemTemplate.parentItemIdentifier), itemTemplate.filename)
            let carpeta = itemTemplate.contentType == .folder
            if itemTemplate.contentType == .symbolicLink || itemTemplate.contentType == .aliasFile {
                throw CocoaError(.featureUnsupported)
            }
            if let ya = try await d.existe(ruta) {
                // Al reimportar, el sistema vuelve a crear lo que ya estaba: es el mismo.
                guard options.contains(.mayAlreadyExist) else { throw NSFileProviderError(.filenameCollision) }
                completionHandler(Item(ya, indice: self.indice), [], false, nil)
                return
            }
            if carpeta {
                try await d.carpeta(ruta)
            } else {
                try await d.subir(ruta, desde: url)
            }
            completionHandler(try await self.elemento(d, ruta), [], false, nil)
        }, fallo: { completionHandler(nil, [], false, $0) })
    }

    func modifyItem(_ item: NSFileProviderItem, baseVersion version: NSFileProviderItemVersion,
                    changedFields: NSFileProviderItemFields, contents newContents: URL?,
                    options: NSFileProviderModifyItemOptions = [], request: NSFileProviderRequest,
                    completionHandler: @escaping (NSFileProviderItem?, NSFileProviderItemFields, Bool, Error?) -> Void) -> Progress {
        trabajo({
            let d = try self.dav()
            var ruta = try self.ruta(item.itemIdentifier)
            if changedFields.contains(.filename) || changedFields.contains(.parentItemIdentifier) {
                let nueva = Proveedor.unir(try self.ruta(item.parentItemIdentifier), item.filename)
                if nueva != ruta {
                    try await d.mover(ruta, a: nueva, carpeta: item.contentType == .folder)
                    self.indice.mover(ruta, a: nueva)
                    ruta = nueva
                }
            }
            if changedFields.contains(.contents), let newContents {
                try await d.subir(ruta, desde: newContents)
            }
            // Lo demás (fechas, etiquetas, atributos) no lo guarda WebDAV: queda local.
            completionHandler(try await self.elemento(d, ruta), [], false, nil)
        }, fallo: { completionHandler(nil, [], false, $0) })
    }

    func deleteItem(identifier: NSFileProviderItemIdentifier, baseVersion version: NSFileProviderItemVersion,
                    options: NSFileProviderDeleteItemOptions = [], request: NSFileProviderRequest,
                    completionHandler: @escaping (Error?) -> Void) -> Progress {
        trabajo({
            let ruta = try self.ruta(identifier)
            // Una carpeta con barra final; sin saber qué es, se pregunta.
            let d = try self.dav()
            let carpeta = try await d.existe(ruta)?.carpeta ?? false
            try await d.borrar(ruta, carpeta: carpeta)
            self.indice.olvidar(ruta)
            completionHandler(nil)
        }, fallo: { completionHandler($0) })
    }

    func enumerator(for containerItemIdentifier: NSFileProviderItemIdentifier,
                    request: NSFileProviderRequest) throws -> NSFileProviderEnumerator {
        if containerItemIdentifier == .workingSet || containerItemIdentifier == .trashContainer {
            return Vacio()
        }
        return Carpeta(proveedor: self, ruta: try ruta(containerItemIdentifier))
    }
}

/// Los hijos de una carpeta: un PROPFIND de profundidad 1, de una vez.
final class Carpeta: NSObject, NSFileProviderEnumerator {
    let proveedor: Proveedor
    let ruta: String

    init(proveedor: Proveedor, ruta: String) {
        self.proveedor = proveedor
        self.ruta = ruta
    }

    func invalidate() {}

    func enumerateItems(for observer: NSFileProviderEnumerationObserver, startingAt page: NSFileProviderPage) {
        Task {
            do {
                let hijos = try await proveedor.dav().listar(ruta, profundidad: "1").filter { $0.ruta != ruta }
                observer.didEnumerate(hijos.map { Item($0, indice: proveedor.indice) })
                proveedor.indice.guardar()
                observer.finishEnumerating(upTo: nil)
            } catch {
                observer.finishEnumeratingWithError(DAV.traducir(error))
            }
        }
    }
}

/// El conjunto de trabajo y la papelera: sin cambios que contar (ver la cabecera).
final class Vacio: NSObject, NSFileProviderEnumerator {
    static let ancla = NSFileProviderSyncAnchor(Data("0".utf8))

    func invalidate() {}

    func enumerateItems(for observer: NSFileProviderEnumerationObserver, startingAt page: NSFileProviderPage) {
        observer.finishEnumerating(upTo: nil)
    }

    func enumerateChanges(for observer: NSFileProviderChangeObserver, from anchor: NSFileProviderSyncAnchor) {
        observer.finishEnumeratingChanges(upTo: Vacio.ancla, moreComing: false)
    }

    func currentSyncAnchor(completionHandler: @escaping (NSFileProviderSyncAnchor?) -> Void) {
        completionHandler(Vacio.ancla)
    }
}
