import Foundation

/// Operator LUT choice. DJI Auto follows color + body; Creative looks are
/// generated; locked rows stay put until the operator picks something else.
public enum LUTSelection: String, CaseIterable, Sendable, Codable {
    case auto
    case officialDLog
    case officialDLog2
    case djiAuto
    case djiDLog
    case djiDLog2
    case djiDLogM
    case creativeMono
    case creativeContrast
    case creativeWarm
    case creativeCool
    case customRec709
    case customDLog
    case customDLog2
    case customFile

    public var title: String {
        switch self {
        case .auto, .djiAuto: "Auto"
        case .officialDLog: "Unavailable"
        case .officialDLog2: "Unavailable"
        case .djiDLog: "Unavailable"
        case .djiDLog2: "Unavailable"
        case .djiDLogM: OfficialDJILUT.nanoDLogM.title
        case .creativeMono: BuiltInLook.mono.rawValue
        case .creativeContrast: BuiltInLook.contrast.rawValue
        case .creativeWarm: BuiltInLook.warm.rawValue
        case .creativeCool: BuiltInLook.cool.rawValue
        case .customRec709: CustomLUTSlot.rec709.title
        case .customDLog: CustomLUTSlot.dLog.title
        case .customDLog2: CustomLUTSlot.dLog2.title
        case .customFile: "Custom"
        }
    }

    public var isBuiltIn: Bool {
        self == .auto || self == .officialDLog || self == .officialDLog2
    }

    public var isDJI: Bool {
        self == .djiAuto || self == .djiDLog || self == .djiDLog2 || self == .djiDLogM
    }

    public var isCreative: Bool { creativeLook != nil }

    public var creativeLook: BuiltInLook? {
        switch self {
        case .creativeMono: .mono
        case .creativeContrast: .contrast
        case .creativeWarm: .warm
        case .creativeCool: .cool
        default: nil
        }
    }

    public var isCustom: Bool {
        customSlot != nil || self == .customFile
    }

    /// Built-in Rec.709 conversions are gone; Auto lives on the DJI tab.
    public var migratedToDJICatalog: LUTSelection {
        switch self {
        case .auto: .djiAuto
        case .officialDLog: .djiDLog
        case .officialDLog2: .djiDLog2
        default: self
        }
    }

    public var customSlot: CustomLUTSlot? {
        switch self {
        case .customRec709: .rec709
        case .customDLog: .dLog
        case .customDLog2: .dLog2
        default: nil
        }
    }

    public static let djiCases: [LUTSelection] = [.djiAuto, .djiDLogM]
    public static let creativeCases: [LUTSelection] = [
        .creativeMono, .creativeContrast, .creativeWarm, .creativeCool,
    ]
    public static let customCases: [LUTSelection] = [.customRec709, .customDLog, .customDLog2]
}

public enum OfficialDJILUT: String, CaseIterable, Sendable, Hashable {
    case nanoDLogM

    public var fileName: String { "DJI_Official_Nano_DLogM_Rec709_33.cube" }
    public var resourceName: String {
        URL(fileURLWithPath: fileName).deletingPathExtension().lastPathComponent
    }
    public var title: String { "D-Log M → Rec.709" }
    public var selection: LUTSelection { .djiDLogM }

    public static func dLogM(cameraName: String?) -> OfficialDJILUT { .nanoDLogM }

    public static func auto(
        colorMode: ColorMode?, family: CameraBodyFamily, cameraName: String? = nil
    ) -> OfficialDJILUT? {
        family == .nano && colorMode == .dLogM ? .nanoDLogM : nil
    }
}

/// Independent on-device custom cubes: Normal/HDR, D-Log, and D-Log2.
public enum CustomLUTSlot: String, CaseIterable, Sendable {
    case rec709
    case dLog
    case dLog2

    public var title: String {
        switch self {
        case .rec709: "Custom"
        case .dLog: "Custom D-Log"
        case .dLog2: "Custom D-Log2"
        }
    }

    public var selection: LUTSelection {
        switch self {
        case .rec709: .customRec709
        case .dLog: .customDLog
        case .dLog2: .customDLog2
        }
    }

    public var others: [CustomLUTSlot] { Self.allCases.filter { $0 != self } }
}

/// What the monitor should paint for a selection + color mode + body.
public enum LUTSource: Equatable, Sendable {
    case dji(OfficialDJILUT)
    case creative(BuiltInLook)
    case custom(CustomLUTSlot)
    case file(String)
    case off

    public var title: String {
        switch self {
        case .dji(let lut): lut.title
        case .creative(let look): look.rawValue
        case .custom(let slot): slot.title
        case .file(let name): CustomLUTIndex.displayName(fileName: name)
        case .off: "Off"
        }
    }
}

/// Resolves Auto vs locked official/custom cubes. No I/O — the shell loads the bytes.
public enum LUTResolver {
    public static func resolve(
        selection: LUTSelection,
        colorMode: ColorMode?,
        family: CameraBodyFamily = .nano,
        cameraName: String? = nil,
        hasCustomDLog: Bool,
        hasCustomDLog2: Bool,
        hasCustomRec709: Bool = false,
        customFileName: String? = nil
    ) -> LUTSource {
        switch selection {
        case .auto, .djiAuto:
            return OfficialDJILUT.auto(
                colorMode: colorMode, family: family, cameraName: cameraName
            ).map(LUTSource.dji) ?? .off
        case .officialDLog:
            return .off
        case .officialDLog2:
            return .off
        case .creativeMono, .creativeContrast, .creativeWarm, .creativeCool:
            return selection.creativeLook.map(LUTSource.creative) ?? .off
        case .djiDLog:
            return .off
        case .djiDLog2:
            return .off
        case .djiDLogM:
            return .dji(OfficialDJILUT.dLogM(cameraName: cameraName))
        case .customRec709:
            return hasCustomRec709 ? .custom(.rec709) : .off
        case .customDLog:
            return hasCustomDLog ? .custom(.dLog) : .off
        case .customDLog2:
            return hasCustomDLog2 ? .custom(.dLog2) : .off
        case .customFile:
            if let customFileName, CustomLUTIndex.isSafeFileName(customFileName) {
                return .file(customFileName)
            }
            return .off
        }
    }

    /// DJI Auto: official Rec.709 cube for this body + color. Rec.709 / HDR stay off.
    public static func builtInAutoSource(
        colorMode: ColorMode?, family: CameraBodyFamily, cameraName: String? = nil
    ) -> LUTSource {
        OfficialDJILUT.auto(colorMode: colorMode, family: family, cameraName: cameraName)
            .map(LUTSource.dji) ?? .off
    }

    /// Legacy name — same as DJI Auto.
    public static func autoSource(
        colorMode: ColorMode?,
        hasCustomDLog: Bool,
        hasCustomDLog2: Bool,
        hasCustomRec709: Bool = false
    ) -> LUTSource {
        builtInAutoSource(colorMode: colorMode, family: .nano)
    }

    public static func statusLabel(
        enabled: Bool,
        selection: LUTSelection,
        source: LUTSource
    ) -> String {
        if !enabled { return "Off · \(selection.title)" }
        if selection == .auto || selection == .djiAuto { return "Auto · \(source.title)" }
        return selection.title
    }

    public static func autoCaption(source: LUTSource) -> String {
        switch source {
        case .dji(let lut):
            return "Applying official \(lut.title)"
        case .creative(let look):
            return "Applying \(look.rawValue)"
        case .custom(let slot):
            return "Applying \(slot.title)"
        case .file(let name):
            return "Applying \(CustomLUTIndex.displayName(fileName: name))"
        case .off:
            return "No matching look for this color / camera"
        }
    }
}

/// Color for Auto LUT on a clip. `colr` / `nclx` is Rec.709 even for D-Log2;
/// the shot profile is QuickTime Keys `com.dji.camera.ColorGammaSxS` on the
/// **original** take. LRF / XRF proxies are Rec.709 even for log — pass `clip`
/// only from the original (or its `moov` tail). That value wins. Live `@2` is
/// the body's current SET — a Rec.709 live SET must not turn Auto off after
/// you just monitored D-Log2 when the original has no Keys atom.
public enum PlaybackLUTColor: Sendable {
    public static func resolve(
        clip: ColorMode? = nil, live: ColorMode?, last: ColorMode?
    ) -> ColorMode? {
        if let clip { return clip }
        if let last, last.bindsAutoLUT {
            if live == nil || live?.bindsAutoLUT == false { return last }
        }
        return live ?? last
    }
}
