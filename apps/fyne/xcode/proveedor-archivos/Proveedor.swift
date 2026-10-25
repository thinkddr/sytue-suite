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
import Security
import UniformTypeIdentifiers

// MARK: - La credencial

/// Lo que la app de Drive deja en el llavero: `sistema.CredencialArchivos`.
struct Credencial: Decodable {
    let url: String
    let usuario: String
    let clave: String
    let identidad: String
}

/// Autoridad local no secreta. El llavero de la sesión NO está en esta extensión.
/// Cada renovación conserva identity_hash/revision; logout o cambio de cuenta los invalida.
enum AccesoDrive {
    struct Estado: Decodable {
        let version: Int
        let identity_hash: String
        let revision: String
        let active: Bool
    }
    static func hexadecimal(_ valor: String, longitud: Int) -> Bool {
        valor.utf8.count == longitud && valor.utf8.allSatisfy { (48...57).contains($0) || (97...102).contains($0) }
    }
    static func decodificar(_ datos: Data, identidad: String, revision: String? = nil) -> Estado? {
        guard hexadecimal(identidad, longitud: 64), datos.count <= 1024,
              let estado = try? JSONDecoder().decode(Estado.self, from: datos),
              estado.version == 1, estado.active, estado.identity_hash == identidad,
              hexadecimal(estado.revision, longitud: 32),
              revision == nil || revision == estado.revision else { return nil }
        return estado
    }
    static func estado(_ identidad: String, revision: String? = nil) -> Estado? {
        guard let contenedor = FileManager.default.containerURL(forSecurityApplicationGroupIdentifier: "group.eu.sytue.suite"),
              let datos = try? Data(contentsOf: contenedor.appendingPathComponent("sytue/drive-acceso.json")) else { return nil }
        return decodificar(datos, identidad: identidad, revision: revision)
    }
}


/// El dominio y sus índices pertenecen a una autorización concreta, nunca a la
/// credencial que casualmente esté en el llavero cuando llegue una petición.
struct ContextoDrive {
    let identidad: String
    let revision: String
    static let prefijo = "sytue-drive."

    init?(dominio: String) {
        guard dominio.hasPrefix(Self.prefijo) else { return nil }
        let partes = dominio.dropFirst(Self.prefijo.count).split(separator: ".", omittingEmptySubsequences: false)
        guard partes.count == 2,
              AccesoDrive.hexadecimal(String(partes[0]), longitud: 64),
              AccesoDrive.hexadecimal(String(partes[1]), longitud: 32) else { return nil }
        identidad = String(partes[0])
        revision = String(partes[1])
    }

    var dominio: String { Self.prefijo + identidad + "." + revision }
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
    let identidad: String
    let revision: String
    private let leerAcceso: (String, String?) -> AccesoDrive.Estado?
    private var vigilante: DispatchSourceTimer?

    init(_ c: Credencial, contexto: ContextoDrive,
         acceso: @escaping (String, String?) -> AccesoDrive.Estado? = AccesoDrive.estado) throws {
        guard let u = URL(string: c.url), u.scheme == "https", u.host != nil,
              c.identidad == contexto.identidad,
              acceso(contexto.identidad, contexto.revision) != nil else {
            throw NSFileProviderError(.notAuthenticated)
        }
        base = u.absoluteString.hasSuffix("/") ? String(u.absoluteString.dropLast()) : u.absoluteString
        autorizacion = "Basic " + Data("\(c.usuario):\(c.clave)".utf8).base64EncodedString()
        identidad = contexto.identidad
        revision = contexto.revision
        leerAcceso = acceso
        // Sin cookies ni caché: cada petición va con su credencial y nada se queda en disco.
        sesion = URLSession(configuration: .ephemeral)
        let reloj = DispatchSource.makeTimerSource(queue: DispatchQueue.global(qos: .utility))
        reloj.schedule(deadline: .now() + .milliseconds(250), repeating: .milliseconds(250))
        reloj.setEventHandler { [weak self] in
            guard let self else { return }
            do { try self.validar() } catch {
                self.sesion.invalidateAndCancel()
                self.vigilante?.cancel()
            }
        }
        vigilante = reloj
        reloj.resume()
    }

    deinit {
        vigilante?.cancel()
        sesion.invalidateAndCancel()
    }

    func validar() throws {
        guard leerAcceso(identidad, revision) != nil else {
            throw NSFileProviderError(.notAuthenticated)
        }
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
        try validar()
        let (datos, respuesta) = try await sesion.data(for: r)
        try validar()
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
        try validar()
        let (temporal, respuesta) = try await sesion.download(for: peticion("GET", ruta))
        defer { try? FileManager.default.removeItem(at: temporal) }
        try validar()
        try DAV.comprobar(respuesta)
        try? FileManager.default.removeItem(at: destino)
        try FileManager.default.moveItem(at: temporal, to: destino)
    }

    func subir(_ ruta: String, desde origen: URL?) async throws {
        try validar()
        var r = peticion("PUT", ruta)
        r.setValue("application/octet-stream", forHTTPHeaderField: "Content-Type")
        let respuesta: URLResponse
        if let origen {
            (_, respuesta) = try await sesion.upload(for: r, fromFile: origen)
        } else {
            (_, respuesta) = try await sesion.upload(for: r, from: Data())
        }
        try validar()
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

    init(dominio: NSFileProviderDomain, directorio: URL? = nil) {
        let dir = directorio ?? (FileManager.default.urls(for: .applicationSupportDirectory, in: .userDomainMask).first
            ?? FileManager.default.temporaryDirectory)
        try? FileManager.default.createDirectory(at: dir, withIntermediateDirectories: true)
        // Un dominio antiguo sin identidad nunca adopta el índice de otra sesión.
        let espacio = ContextoDrive(dominio: dominio.identifier.rawValue)?.dominio ?? UUID().uuidString
        fichero = dir.appendingPathComponent("indice-\(espacio).json")
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
    let contexto: ContextoDrive?
    private let leerCredencial: () -> Credencial?
    private let leerAcceso: (String, String?) -> AccesoDrive.Estado?

    required convenience init(domain: NSFileProviderDomain) {
        self.init(domain: domain, credencial: Llavero.credencial, acceso: AccesoDrive.estado)
    }

    init(domain: NSFileProviderDomain, credencial: @escaping () -> Credencial?,
         acceso: @escaping (String, String?) -> AccesoDrive.Estado?, directorio: URL? = nil) {
        dominio = domain
        contexto = ContextoDrive(dominio: domain.identifier.rawValue)
        indice = Indice(dominio: domain, directorio: directorio)
        leerCredencial = credencial
        leerAcceso = acceso
        super.init()
    }

    func invalidate() { indice.guardar() }

    /// Se relee la credencial para detectar su retirada, pero jamás se adopta la de
    /// otra cuenta o una sesión posterior del mismo usuario. El dominio fija ambas.
    func dav() throws -> DAV {
        guard let contexto, let c = leerCredencial() else { throw NSFileProviderError(.notAuthenticated) }
        return try DAV(c, contexto: contexto, acceso: leerAcceso)
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
            let d = try self.dav()
            let item = try await self.elemento(d, self.ruta(identifier))
            try d.validar()
            completionHandler(item, nil)
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
            do { try d.validar() } catch { try? FileManager.default.removeItem(at: destino); throw error }
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
                try d.validar()
                completionHandler(Item(ya, indice: self.indice), [], false, nil)
                return
            }
            if carpeta {
                try await d.carpeta(ruta)
            } else {
                try await d.subir(ruta, desde: url)
            }
            let resultado = try await self.elemento(d, ruta)
            try d.validar()
            completionHandler(resultado, [], false, nil)
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
            let resultado = try await self.elemento(d, ruta)
            try d.validar()
            completionHandler(resultado, [], false, nil)
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
            try d.validar()
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
    let abrirDAV: () throws -> DAV

    init(proveedor: Proveedor, ruta: String) {
        self.proveedor = proveedor
        self.ruta = ruta
        // Retener un enumerador no permite adoptar una sesión nueva del proveedor.
        abrirDAV = proveedor.dav
    }

    func invalidate() {}

    func enumerateItems(for observer: NSFileProviderEnumerationObserver, startingAt page: NSFileProviderPage) {
        Task {
            do {
                let d = try abrirDAV()
                let hijos = try await d.listar(ruta, profundidad: "1").filter { $0.ruta != ruta }
                try d.validar()
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
