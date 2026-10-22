// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

import Foundation
#if canImport(Glibc)
import Glibc
#else
import Darwin
#endif

@main
struct WidgetProjectTests {
    static func main() throws {
        let root = URL(fileURLWithPath: CommandLine.arguments[1])
        let xcode = root.appendingPathComponent("apps/fyne/xcode")
        func plist(_ url: URL) throws -> [String: Any] {
            guard let value = try PropertyListSerialization.propertyList(from: Data(contentsOf: url), options: [], format: nil) as? [String: Any] else {
                throw CocoaError(.propertyListReadCorrupt)
            }
            return value
        }
        var checks = 0
        func require(_ condition: Bool, _ message: String) {
            checks += 1
            if !condition {
                FileHandle.standardError.write(Data(("not ok - " + message + "\n").utf8))
                exit(1)
            }
        }
        func quoted(_ key: String, _ source: String) -> String {
            let expression = try! NSRegularExpression(pattern: "(?m)^\\s*" + NSRegularExpression.escapedPattern(for: key) + "\\s*=\\s*\"([^\"]*)\"")
            guard let match = expression.firstMatch(in: source, range: NSRange(source.startIndex..., in: source)),
                  let range = Range(match.range(at: 1), in: source) else { return "" }
            return String(source[range])
        }

        let project = try plist(xcode.appendingPathComponent("Sytue.xcodeproj/project.pbxproj"))
        let objects = project["objects"] as! [String: [String: Any]]
        let targets = objects.filter { $0.value["isa"] as? String == "PBXNativeTarget" }
        var parents = [String: String]()
        for (id, object) in objects {
            if let children = object["children"] as? [String] {
                for child in children { parents[child] = id }
            }
        }
        func sourcePath(_ id: String, seen: Set<String> = []) -> URL? {
            guard !seen.contains(id), let object = objects[id] else { return nil }
            let tree = object["sourceTree"] as? String ?? "<group>"
            if tree == "BUILT_PRODUCTS_DIR" { return nil }
            guard tree == "<group>" || tree == "SOURCE_ROOT" else { return nil }
            var base = xcode
            if tree == "<group>", let parent = parents[id] {
                guard let directory = sourcePath(parent, seen: seen.union([id])) else { return nil }
                base = directory
            }
            if let path = object["path"] as? String { return base.appendingPathComponent(path).standardizedFileURL }
            return base
        }
        for (id, object) in objects where object["isa"] as? String == "PBXFileReference" && object["sourceTree"] as? String != "BUILT_PRODUCTS_DIR" {
            let path = sourcePath(id)
            require(path != nil && FileManager.default.fileExists(atPath: path!.path), "project source exists: " + (object["path"] as? String ?? id))
        }
        func target(_ name: String) -> (String, [String: Any]) {
            let found = targets.filter { $0.value["name"] as? String == name }
            require(found.count == 1, "exactly one target " + name)
            return found.first!
        }
        func settings(_ target: [String: Any]) -> [[String: Any]] {
            let list = objects[target["buildConfigurationList"] as! String]!
            return (list["buildConfigurations"] as! [String]).map { objects[$0]!["buildSettings"] as! [String: Any] }
        }
        func embeds(_ host: [String: Any], _ name: String, _ product: String) -> Bool {
            let phases = (host["buildPhases"] as! [String]).compactMap { objects[$0] }
            guard let phase = phases.first(where: { $0["name"] as? String == name }) else { return false }
            return (phase["files"] as! [String]).contains { id in
                let file = objects[id]!
                let attributes = (file["settings"] as? [String: Any])?["ATTRIBUTES"] as? [String] ?? []
                return file["fileRef"] as? String == product && attributes.contains("CodeSignOnCopy")
            }
        }

        // El interruptor único (Sytue.xcconfig): apagado, los targets existen pero ningún
        // host los lleva dentro ni pide el App Group.
        let xcconfig = try String(contentsOf: xcode.appendingPathComponent("Sytue.xcconfig"), encoding: .utf8)
        let switches = xcconfig.components(separatedBy: "\n").filter { $0.hasPrefix("SYTUE_WIDGETS_APPLE = ") }
        require(switches.count == 1 && ["SYTUE_WIDGETS_APPLE = YES", "SYTUE_WIDGETS_APPLE = NO"].contains(switches[0]),
                "Sytue.xcconfig declares one SYTUE_WIDGETS_APPLE switch")
        let enabled = switches.first == "SYTUE_WIDGETS_APPLE = YES"
        func resources(_ target: [String: Any]) -> [String] {
            (target["buildPhases"] as! [String]).compactMap { objects[$0] }
                .filter { $0["isa"] as? String == "PBXResourcesBuildPhase" }
                .flatMap { ($0["files"] as! [String]).compactMap { objects[$0]?["fileRef"] as? String } }
                .compactMap { objects[$0]?["path"] as? String }
        }

        let groupSource = try String(contentsOf: root.appendingPathComponent("apps/nucleo/ajustes/grupo_apple.go"), encoding: .utf8)
        let group = quoted("const GrupoContenedorSuite", groupSource)
        let team = String(quoted("const GrupoSuite", groupSource).prefix(10))
        let keychain = quoted("const GrupoSuite", groupSource)
        require(group.hasPrefix("group.") && team.count == 10, "canonical Apple group is declared")
        let (bridgeID, bridge) = target("SytueWidgetBridge")
        require(bridge["productType"] as? String == "com.apple.product-type.framework", "public Swift bridge is a framework")
        require(settings(bridge).allSatisfy { $0["LD_DYLIB_INSTALL_NAME"] as? String == "@rpath/$(EXECUTABLE_PATH)" }, "bridge install name resolves inside the host bundle")
        var widgetApps = 0
        var products = Set<String>()
        let commands = root.appendingPathComponent("apps/fyne/cmd")
        for directory in try FileManager.default.contentsOfDirectory(at: commands, includingPropertiesForKeys: nil) {
            if directory.lastPathComponent == "demo" { continue }
            guard let metadata = try? String(contentsOf: directory.appendingPathComponent("FyneApp.toml"), encoding: .utf8) else { continue }
            let actions = quoted("widget_acciones", metadata)
            if actions.isEmpty { continue }
            require(actions.components(separatedBy: ",").contains("search"), "global search declared for " + directory.lastPathComponent)
            widgetApps += 1
            let app = directory.lastPathComponent
            let bundle = quoted("ID", metadata)
            let scheme = quoted("esquema", metadata)
            let (widgetID, widget) = target(app + "-widgets")
            let (_, host) = target(app)
            require(settings(host).allSatisfy { $0["CODE_SIGN_ENTITLEMENTS[sdk=iphoneos*]"] as? String == app + "/ios.entitlements" },
                    "every iOS host signs its shared session rights " + app)
            let hostRights = try plist(xcode.appendingPathComponent(app + "/ios.entitlements"))
            if enabled {
                require(settings(host).allSatisfy { $0["ALWAYS_EMBED_SWIFT_STANDARD_LIBRARIES"] as? String == "YES" },
                        "host embeds the runtime used by its Swift widget and public bridge " + app)
                require((hostRights["com.apple.security.application-groups"] as? [String])?.contains(group) == true,
                        "host and widget use the same snapshot container " + app)
            } else {
                require(settings(host).allSatisfy { $0["ALWAYS_EMBED_SWIFT_STANDARD_LIBRARIES"] == nil },
                        "switched-off host signs exactly as before widgets " + app)
                require(hostRights["com.apple.security.application-groups"] == nil,
                        "switched-off host requests no unregistered App Group " + app)
            }
            let access = hostRights["keychain-access-groups"] as? [String] ?? []
            require(access.first == team + "." + bundle && access.contains(keychain),
                    "host keeps original and suite session Keychain access " + app)
            let product = widget["productReference"] as! String
            require(products.insert(objects[product]!["path"] as! String).inserted, "unique appex output for " + app)
            require(widget["productType"] as? String == "com.apple.product-type.app-extension", "widget product type " + app)
            require(settings(widget).allSatisfy { $0["PRODUCT_BUNDLE_IDENTIFIER"] as? String == bundle + ".widgets" }, "widget identifier from TOML " + app)
            require(settings(widget).allSatisfy { $0["APPLICATION_EXTENSION_API_ONLY"] as? String == "YES" }, "extension safe API mode " + app)
            let dependencies = (host["dependencies"] as! [String]).compactMap { objects[$0]?["target"] as? String }
            require((dependencies.contains(widgetID) && dependencies.contains(bridgeID)) == enabled, "host builds widget and bridge only when enabled " + app)
            require(embeds(host, "Embed WidgetKit", product) == enabled, "host embeds signed widget only when enabled " + app)
            require(embeds(host, "Embed WidgetKit Bridge", bridge["productReference"] as! String) == enabled, "host embeds signed public bridge only when enabled " + app)
            require(resources(widget).contains("PrivacyInfo.xcprivacy"), "widget ships its privacy manifest " + app)
            let info = try plist(xcode.appendingPathComponent(app + "/widgets/Info.plist"))
            require(info["SytueWidgetBundle"] as? String == bundle && info["SytueWidgetScheme"] as? String == scheme,
                    "widget metadata matches containing app " + app)
            require(info["SytueWidgetActions"] as? [String] == actions.components(separatedBy: ","), "actions come from TOML " + app)
            require(info["SytueWidgetGroup"] as? String == group, "widget reads canonical group " + app)
            require((info["NSExtension"] as? [String: String])?["NSExtensionPointIdentifier"] == "com.apple.widgetkit-extension", "WidgetKit extension point " + app)
            for platform in ["ios", "macos"] {
                let entitlements = try plist(xcode.appendingPathComponent(app + "/widgets/" + platform + ".entitlements"))
                require(entitlements["com.apple.security.application-groups"] as? [String] == [group], "only shared snapshot group " + app + " " + platform)
                require(entitlements["keychain-access-groups"] == nil && entitlements["com.apple.security.network.client"] == nil && entitlements["com.apple.security.network.server"] == nil,
                        "widget has no keychain or network permission " + app + " " + platform)
                let key = platform == "ios" ? "application-identifier" : "com.apple.application-identifier"
                require(entitlements[key] as? String == team + "." + bundle + ".widgets", "exact extension signing identifier " + app + " " + platform)
                if platform == "ios" {
                    require(entitlements["com.apple.developer.default-data-protection"] as? String == "NSFileProtectionComplete", "private while locked " + app)
                } else {
                    require(entitlements["com.apple.security.app-sandbox"] as? Bool == true, "widget sandbox " + app)
                }
            }
        }
        require(widgetApps > 0, "widget catalogue is not empty")
        require(targets.filter { ($0.value["name"] as? String)?.hasSuffix("-widgets") == true }.count == widgetApps,
                "one widget target per declaring TOML, no second application table")
        require(checks >= widgetApps * 20, "project contract executes every app")
        print("ok - \(checks) WidgetKit project checks across \(widgetApps) apps")
    }
}
