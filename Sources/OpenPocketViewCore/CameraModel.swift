import Foundation

/// GATT UUIDs and DJI BLE identifiers. Strings here so the core stays free of CoreBluetooth;
/// the app wraps them in `CBUUID`.
public enum BleConstants {
    public static let serviceFFF0 = "0000FFF0-0000-1000-8000-00805F9B34FB"
    public static let charFFF4 = "0000FFF4-0000-1000-8000-00805F9B34FB"  // notify + arm-pairing
    public static let charFFF5 = "0000FFF5-0000-1000-8000-00805F9B34FB"  // command writes
    public static let cccd = "00002902-0000-1000-8000-00805F9B34FB"

    // DJI BLE company ids (Android SparseArray key form; on the wire little-endian: AA 08 / AA F7).
    public static let djiCompanyIds: Set<Int> = [0x08AA, 0xF7AA, 0xE5C0]
    public static func isDjiCompanyId(_ cid: Int) -> Bool { djiCompanyIds.contains(cid) }
}

public struct CameraModel: Equatable, Sendable {
    public let name: String
    public let datalinkPort: Int
    public let tcpPoke: Bool
    public let wpa3: Bool
    public let verified: Bool
    public let isDrone: Bool

    public init(
        name: String, datalinkPort: Int = 9004, tcpPoke: Bool = true,
        wpa3: Bool = false, verified: Bool = false, isDrone: Bool = false
    ) {
        self.name = name
        self.datalinkPort = datalinkPort
        self.tcpPoke = tcpPoke
        self.wpa3 = wpa3
        self.verified = verified
        self.isDrone = isDrone
    }

    public var pairingToken: String { "osmo" }
    public var usesCapturedLiveEnable: Bool { family == .nano }
    public var liveViewEnableReceiver: UInt8 { 0x41 }
    public var usesNanoLiveViewGate: Bool { family == .nano }
    public var needsFirstPictureFormatPoke: Bool { false }
    public var isoAutoRangeFloor: Int { 100 }
    public var hasGimbal: Bool { false }
    public var supportsTapFocus: Bool { false }
    public var supportsFocusMode: Bool { false }
    public var family: CameraBodyFamily {
        CameraBodyFamily.resolve(modelId: nil, name: name)
    }
    public var zoomStops: [Double] { [1] }
    public var zoomMax: Double { 1 }

    public func activeZoomStops(resolution: VideoResolution?, shootingMode: Int) -> [Double] {
        [1]
    }

    public func alternate() -> CameraModel { self }

    public static let `default` = CameraModel(name: "Osmo Nano", verified: true)

    public static func resolve(
        modelId: Int?, name: String?, brand: CameraBrand = .unknown
    ) -> CameraModel {
        let resolvedBrand = brand == .unknown ? CameraBrand.of(address: nil, name: name) : brand
        guard resolvedBrand != .xtra,
            CameraBodyFamily.resolve(modelId: modelId, name: name) == .nano
        else {
            return CameraModel(name: "Unsupported camera")
        }
        return .default
    }
}

public enum CameraBodyFamily: Equatable, Sendable {
    case nano
    case other

    public static func resolve(modelId: Int?, name: String?) -> CameraBodyFamily {
        if let modelId { return modelId == 0x0019 ? .nano : .other }
        let compact = (name ?? "").lowercased().replacingOccurrences(of: " ", with: "")
        return compact.hasPrefix("osmonano") ? .nano : .other
    }

    public static func ofSSID(_ ssid: String) -> CameraBodyFamily {
        resolve(modelId: nil, name: ssid)
    }

    public static func ssidConflictsWithBody(
        ssid: String, modelId: Int?, advertisedName: String
    ) -> Bool {
        guard resolve(modelId: modelId, name: advertisedName) == .nano else { return true }
        let compact = ssid.lowercased().replacingOccurrences(of: " ", with: "")
        return ["osmopocket", "osmoaction", "osmo360", "xtra", "mavic", "djineo"]
            .contains { compact.hasPrefix($0) }
    }
}

public enum ModelNames {
    public static let byId: [Int: String] = [0x0019: "OsmoNano"]
}

public enum CameraBrand: Equatable, Sendable {
    case dji
    case xtra
    case unknown

    public static let xtraOUI = "EC:9E:EA"

    public static func of(address: String?, name: String?, djiCid: Bool = false) -> CameraBrand {
        let compact = (name ?? "").lowercased().replacingOccurrences(of: " ", with: "")
        if (address ?? "").uppercased().hasPrefix(xtraOUI) || compact.hasPrefix("xtra") {
            return .xtra
        }
        return djiCid || compact.hasPrefix("osmonano") ? .dji : .unknown
    }
}
