// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
// Compila con Proveedor.swift y el SDK real. No solicita grupos ni lee llaveros.
import Foundation
import FileProvider

/// La prueba cambia cuenta/autoridad mientras conserva proveedores y enumeradores.
/// Los callbacks reales releen esta entrada; ninguna prueba toca el llavero del equipo.
private final class SesionPrueba {
    private let cerrojo = NSLock()
    private var credencial: Credencial?
    private var datos = Data()

    func poner(_ c: Credencial, _ estado: [String: Any]) throws {
        let nuevos = try JSONSerialization.data(withJSONObject: estado)
        cerrojo.lock()
        defer { cerrojo.unlock() }
        credencial = c
        datos = nuevos
    }
    func leerCredencial() -> Credencial? {
        cerrojo.lock()
        defer { cerrojo.unlock() }
        return credencial
    }
    func leerAcceso(_ identidad: String, _ revision: String?) -> AccesoDrive.Estado? {
        cerrojo.lock()
        defer { cerrojo.unlock() }
        return AccesoDrive.decodificar(datos, identidad: identidad, revision: revision)
    }
}

@main
struct PruebaAccesoDrive {
    static func main() throws {
        let identidad = String(repeating: "a", count: 64)
        let revision = String(repeating: "b", count: 32)
        let base: [String: Any] = ["version": 1, "identity_hash": identidad, "revision": revision, "active": true]
        var controles = 0
        func comprobar(_ resultado: Bool, _ motivo: String) throws {
            controles += 1
            if !resultado { throw NSError(domain: "SytueDriveAccesoPrueba", code: controles, userInfo: [NSLocalizedDescriptionKey: motivo]) }
        }
        func datos(_ valor: [String: Any]) throws -> Data { try JSONSerialization.data(withJSONObject: valor) }
        let original = try datos(base)
        try comprobar(AccesoDrive.decodificar(original, identidad: identidad, revision: revision) != nil, "la sesión válida debe permitir la operación")
        try comprobar(AccesoDrive.decodificar(original, identidad: identidad) != nil, "una operación nueva captura la revisión actual")
        try comprobar(AccesoDrive.decodificar(original, identidad: String(repeating: "c", count: 64)) == nil, "otra identidad no puede leer contenido")
        try comprobar(AccesoDrive.decodificar(original, identidad: identidad, revision: String(repeating: "c", count: 32)) == nil, "la transferencia anterior a salir o cambiar cuenta debe fallar")
        // Una renovación normal conserva ambos campos; no invalida operaciones en curso.
        try comprobar(AccesoDrive.decodificar(try datos(base), identidad: identidad, revision: revision) != nil, "renovar no debe cerrar una operación válida")
        for (campo, valor) in [("active", false), ("active", 1), ("version", 2), ("identity_hash", "persona@ejemplo.eu"), ("revision", "") ] as [(String, Any)] {
            var alterado = base
            alterado[campo] = valor
            try comprobar(AccesoDrive.decodificar(try datos(alterado), identidad: identidad) == nil, "marca inválida: \(campo)")
        }
        for campo in ["version", "active", "identity_hash", "revision"] {
            var incompleto = base
            incompleto.removeValue(forKey: campo)
            try comprobar(AccesoDrive.decodificar(try datos(incompleto), identidad: identidad) == nil, "marca incompleta: \(campo)")
        }
        var revisionInvalida = base
        revisionInvalida["revision"] = String(repeating: "z", count: 32)
        try comprobar(AccesoDrive.decodificar(try datos(revisionInvalida), identidad: identidad) == nil, "la revisión debe ser hexadecimal")
        try comprobar(AccesoDrive.decodificar(Data("{}".utf8), identidad: identidad) == nil, "sin autoridad no se entrega contenido")
        try comprobar(AccesoDrive.decodificar(Data(repeating: 32, count: 1025), identidad: identidad) == nil, "el lector tiene límite de tamaño")
        try comprobar(AccesoDrive.decodificar(original, identidad: String(repeating: "z", count: 64)) == nil, "una identidad opaca debe tener formato válido")
        let directorio = FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString)
        defer { try? FileManager.default.removeItem(at: directorio) }
        let sesion = SesionPrueba()
        func dominio(_ id: String, _ rev: String) -> NSFileProviderDomain {
            NSFileProviderDomain(identifier: NSFileProviderDomainIdentifier(rawValue: "sytue-drive." + id + "." + rev), displayName: "Sytue Drive")
        }
        func proveedor(_ id: String, _ rev: String) -> Proveedor {
            Proveedor(domain: dominio(id, rev), credencial: sesion.leerCredencial, acceso: sesion.leerAcceso, directorio: directorio)
        }
        func falla<T>(_ operacion: () throws -> T) -> Bool {
            do { _ = try operacion(); return false } catch { return true }
        }
        func credencial(_ id: String) -> Credencial {
            Credencial(url: "https://drive.ejemplo.eu/dav", usuario: "prueba", clave: "no-es-una-clave-real", identidad: id)
        }
        try sesion.poner(credencial(identidad), base)
        let a = proveedor(identidad, revision)
        let antigua = a.indice.id("/misma-ruta.txt")
        a.indice.guardar()
        let retenido = Carpeta(proveedor: a, ruta: "/")
        let transferencia = try a.dav()
        try comprobar(!falla(retenido.abrirDAV), "el enumerador inicial debe usar su propia autorización")
        let bID = String(repeating: "c", count: 64)
        var estadoB = base
        estadoB["identity_hash"] = bID
        try sesion.poner(credencial(bID), estadoB)
        let b = proveedor(bID, revision)
        let nueva = b.indice.id("/misma-ruta.txt")
        try comprobar(antigua != nueva && b.indice.ruta(antigua) == nil, "la misma ruta de B no puede adoptar el identificador ni índice de A")
        try comprobar(falla(a.dav), "un proveedor de A no debe adoptar la clave actual de B")
        try comprobar(falla(retenido.abrirDAV), "un enumerador de A retenido no debe adoptar B")
        try comprobar(falla(transferencia.validar), "una transferencia de A debe cerrarse al cambiar a B")
        try comprobar(!falla(b.dav), "el dominio nuevo de B debe poder acceder con su propia clave")
        var reentrada = base
        let revisionNueva = String(repeating: "d", count: 32)
        reentrada["revision"] = revisionNueva
        try sesion.poner(credencial(identidad), reentrada)
        let aNueva = proveedor(identidad, revisionNueva)
        try comprobar(falla(a.dav) && falla(retenido.abrirDAV), "A→B→A no reactiva proveedores ni enumeradores anteriores")
        try comprobar(aNueva.indice.ruta(antigua) == nil && !falla(aNueva.dav), "la sesión nueva de A tiene índice nuevo y autorización propia")
        let legado = Proveedor(domain: NSFileProviderDomain(identifier: NSFileProviderDomainIdentifier(rawValue: "sytue-drive"), displayName: "Sytue Drive"), credencial: sesion.leerCredencial, acceso: sesion.leerAcceso, directorio: directorio)
        try comprobar(ContextoDrive(dominio: "sytue-drive") == nil && falla(legado.dav), "el dominio legado sin identidad nunca adopta una credencial actual")
        for id in ["sytue-drive." + identidad, "sytue-drive." + identidad + ".../", "otro." + identidad + "." + revision] {
            try comprobar(ContextoDrive(dominio: id) == nil, "el dominio no puede elegir una ruta fuera del índice autorizado")
        }
        print("ok - File Provider: \(controles) controles de identidad, renovación y cierre local")
    }
}
