import OpenPocketViewCore
import XCTest

@testable import OpenPocketCine

@MainActor final class MultiviewStageTests: XCTestCase {
    func testTimecodeRequiresCameraReportAndIsHiddenOnNano() {
        let tile = MultiviewSession.Tile()
        for name in ["OsmoPocket3-Test", "OsmoPocket4P-Test", "OsmoNano-Test"] {
            tile.camera = FoundCamera(
                id: UUID(), name: name,
                model: .resolve(modelId: nil, name: name), modelId: nil)
            tile.settings.timecode = nil
            XCTAssertNil(tile.timecodeReadout)
            tile.settings.timecode = "01:02:03:04"
            XCTAssertEqual(tile.timecodeReadout, name.contains("Nano") ? nil : "01:02:03:04")
        }
    }

    func testMissingRoleQueryIsRestrictedToPhysicallyVerifiedModelsAndReply() {
        for (name, accepted) in [
            ("OsmoPocket3-Test", false), ("OsmoNano-Test", true),
            ("OsmoPocket4P-Test", false), ("OsmoAction4-Test", false),
        ] {
            let camera = FoundCamera(
                id: UUID(), name: name,
                model: .resolve(modelId: nil, name: name), modelId: nil)
            XCTAssertEqual(camera.acceptsMissingMultiviewRoleQuery([0xe0]), accepted)
            for reply: [UInt8] in [[], [0], [0xff], [0xe0, 0], [0, 0]] {
                XCTAssertFalse(camera.acceptsMissingMultiviewRoleQuery(reply))
            }
        }
    }
    func testMultiviewDiscoversOsmoCatalogWithoutGuessingUnknownPreviewCommands() {
        for (id, name) in [
            (0x10, "Osmo Action 2"), (0x12, "Osmo Action 3"), (0x14, "Osmo Action 4"),
            (0x15, "Osmo Action 5 Pro"), (0x17, "Osmo 360"), (0x18, "Osmo Action 6"),
            (0x19, "Osmo Nano"), (0x20, "Osmo Pocket 3"), (0x21, "Osmo Pocket 4"),
            (0x22, "Osmo Pocket 4 Pro"),
        ] {
            let camera = FoundCamera(
                id: UUID(), name: name, model: .resolve(modelId: id, name: name), modelId: id)
            XCTAssertEqual(camera.appearsInMultiview, id == 0x19, name)
            XCTAssertEqual(camera.hasMultiviewPreview, id == 0x19, name)
        }
        let pocket = FoundCamera(
            id: UUID(), name: "OsmoPocket3-Test",
            model: .resolve(modelId: nil, name: "OsmoPocket3-Test"), modelId: nil)
        XCTAssertFalse(pocket.hasMultiviewPreview)
        let drone = FoundCamera(
            id: UUID(), name: "DJI Neo", model: .resolve(modelId: 0x7E, name: "DJI Neo"),
            modelId: 0x7E)
        XCTAssertFalse(drone.appearsInMultiview)
        let oldPocket = FoundCamera(
            id: UUID(), name: "Osmo Pocket 2", model: .resolve(modelId: nil, name: "Osmo Pocket 2"),
            modelId: nil)
        XCTAssertFalse(oldPocket.appearsInMultiview)
        XCTAssertFalse(oldPocket.hasMultiviewPreview)
    }
    func testStageUsesSharedPresentationAndKeepsOneSlotPerCamera() {
        for size in [
            CGSize(width: 390, height: 844), CGSize(width: 852, height: 393),
            CGSize(width: 1194, height: 834),
        ] {
            for arrangement in MultiviewLayout.allCases {
                for selected in 0..<4 {
                    let layout = arrangement.presentation(in: size, selected: selected)
                    XCTAssertEqual(layout.tiles.count, 4)
                    XCTAssertEqual(layout.portrait, size.height > size.width)
                    XCTAssertGreaterThan(layout.tiles[selected].width, 0)
                }
            }
        }
    }

    private func assign(_ tile: MultiviewSession.Tile, recording: Bool, available: Bool) {
        tile.camera = FoundCamera(
            id: UUID(), name: "Test camera", model: .resolve(modelId: 0x19, name: "OsmoNano-Test"),
            modelId: 0x19)
        tile.recordingObservation = (recording, Date())
        tile.recordingAvailable = available
    }
    func testUnavailableRecordingCameraKeepsStopIntentAndBlocksGroupCommand() {
        let session = MultiviewSession()
        assign(session.tiles[0], recording: true, available: false)
        assign(session.tiles[1], recording: false, available: true)
        XCTAssertTrue(session.anyRecording)
        XCTAssertFalse(session.canRecordTogether)
        session.tiles[0].recordingAvailable = true
        XCTAssertTrue(session.canRecordTogether)
        session.tiles[1].recordingBusy = true
        XCTAssertFalse(session.canRecordTogether)
    }
    func testChangingNetworkClearsUnrelatedPassword() {
        let session = MultiviewSession()
        session.ssid = "Old test network"
        session.password = "old-test-password"
        session.selectNetwork("New test network \(UUID().uuidString)")
        XCTAssertEqual(session.password, "")
    }

    func testAutoLUTFollowsEachTilesCameraColorIndependently() {
        let session = MultiviewSession()
        let nano = session.tiles[0]
        assign(nano, recording: false, available: true)
        nano.settings.colorMode = .dLogM
        nano.toggleLUT()
        XCTAssertGreaterThan(nano.effects.lutDimension, 0)
        XCTAssertEqual(session.tiles[1].effects.lutDimension, 0)
        nano.settings.colorMode = .normal
        nano.updateLUT()
        XCTAssertEqual(nano.effects.lutDimension, 0)
        XCTAssertTrue(nano.lutEnabled)
    }

    func testRecoveryDoesNotResetHealthyVideoForStalePresentation() {
        var recovery = MultiviewRecovery()
        let snapshot = FeedWatchdog.Snapshot(
            now: 100, lastDecodedFrameAge: 20, lastVideoPacketAge: 0.01,
            lastStatusAge: 0.1, flowHealthy: true, pathReady: true, hasFormat: true,
            decoderFailed: false, live: true, sawPicture: true,
            secondsSinceLastEnable: 50)
        XCTAssertEqual(recovery.action(snapshot), .none)
    }
    func testRecoveryStopsAfterTwoFullRejoinsUntilOperatorRetries() {
        var recovery = MultiviewRecovery()
        XCTAssertTrue(recovery.beginRejoin())
        XCTAssertTrue(recovery.beginRejoin())
        XCTAssertFalse(recovery.beginRejoin())
        XCTAssertTrue(recovery.failed)
        let snapshot = FeedWatchdog.Snapshot(
            now: 100, lastDecodedFrameAge: 20, lastVideoPacketAge: 20,
            lastStatusAge: 20, flowHealthy: false, pathReady: true, hasFormat: true,
            decoderFailed: false, live: true, sawPicture: true,
            secondsSinceLastEnable: 50)
        XCTAssertEqual(recovery.action(snapshot), .none)
    }
    func testFirstPictureFailureEscalatesBeyondBaseCooldown() {
        var recovery = MultiviewRecovery()
        func snapshot(_ time: Double) -> FeedWatchdog.Snapshot {
            FeedWatchdog.Snapshot(
                now: time, lastDecodedFrameAge: nil, lastVideoPacketAge: nil,
                lastStatusAge: nil, flowHealthy: true, pathReady: true, hasFormat: false,
                decoderFailed: false, live: true, sawPicture: false, hadVideo: false,
                secondsSinceLastEnable: 50)
        }
        XCTAssertEqual(recovery.action(snapshot(100)), .resendLiveViewEnable)
        XCTAssertEqual(recovery.action(snapshot(110)), .reopenDatalink)
        XCTAssertEqual(recovery.action(snapshot(120)), .none)
        var noNetwork = snapshot(130)
        noNetwork.pathReady = false
        XCTAssertEqual(recovery.action(noNetwork), .none)
        var recentEnable = snapshot(130)
        recentEnable.secondsSinceLastEnable = 1
        XCTAssertEqual(recovery.action(recentEnable), .none)
        XCTAssertEqual(recovery.action(snapshot(130)), .fullSessionRejoin)
    }

    func testBorrowedLiveViewDoesNotOwnOrCloseTileTransport() {
        let tile = MultiviewSession.Tile()
        assign(tile, recording: false, available: true)
        let driver = DatalinkDriver(
            port: 9004, tcpPoke: false, pairingToken: "test", stationHost: "192.168.1.10")
        let borrowed = CameraSession(borrowing: tile.decoder)
        borrowed.updateMultiview(camera: tile.camera!, driver: driver, status: tile.settings)
        XCTAssertTrue(borrowed.decoder === tile.decoder)
        XCTAssertTrue(borrowed.datalink === driver)
        borrowed.disconnect()
        XCTAssertNil(borrowed.datalink)
        XCTAssertFalse(driver.isClosed)
        driver.close()
    }
    func testBorrowedLiveViewCannotStartSharingOrChangeItsPreference() {
        let model = AppModel()
        model.session = CameraSession(borrowing: HevcDecoder())
        model.session.updateMultiview(
            camera: FoundCamera(
                id: UUID(), name: "OsmoPocket4P-Test",
                model: .resolve(modelId: 0x22, name: "OsmoPocket4P-Test"), modelId: 0x22),
            driver: nil, status: CameraStatus())
        let savedPreference = OperatorPrefs.shareThisFeed
        model.shareThisFeed = false
        model.setShareThisFeed(true)
        XCTAssertFalse(model.shareThisFeed)
        XCTAssertEqual(OperatorPrefs.shareThisFeed, savedPreference)

        // Automatic startup must also reject a previously saved sharing preference.
        model.shareThisFeed = true
        model.startRelayHost()
        XCTAssertNil(model.session.decoder.onIdentityFrame)
        XCTAssertNil(model.session.decoder.onIdentityOrientation)
    }

    func testReplacingTileDriverDetachesBorrowedControlsUntilVerified() {
        let tile = MultiviewSession.Tile()
        let model = AppModel()
        model.session = CameraSession(borrowing: tile.decoder)
        tile.liveModel = model
        let first = DatalinkDriver(
            port: 9004, tcpPoke: false, pairingToken: "test", stationHost: "192.168.1.10")
        let next = DatalinkDriver(
            port: 9004, tcpPoke: false, pairingToken: "test", stationHost: "192.168.1.11")
        model.session.datalink = first
        tile.driver = next
        XCTAssertNil(model.session.datalink)
        first.close()
        next.close()
    }

}
