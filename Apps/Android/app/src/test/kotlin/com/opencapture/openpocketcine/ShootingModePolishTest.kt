package com.opencapture.openpocketcine

import com.opencapture.openpocketcine.session.CameraCommands
import com.opencapture.openpocketcine.session.CameraModel
import com.opencapture.openpocketcine.session.CameraStatus
import com.opencapture.openpocketcine.session.VideoFormat
import com.opencapture.openpocketcine.session.VideoFrameRate
import com.opencapture.openpocketcine.session.VideoResolution
import kotlin.test.Test
import kotlin.test.assertContentEquals
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertNull
import kotlin.test.assertTrue

class ShootingModePolishTest {
    private val pocket3 = CameraModel(name = "Osmo Pocket 3")
    private val leftoverVideo = listOf(
        VideoFormat(VideoResolution.P4K, VideoFrameRate.FPS24),
        VideoFormat(VideoResolution.P4K, VideoFrameRate.FPS60),
    )

    @Test
    fun superNightIsVideo() {
        assertFalse(CameraCommands.isPhotoMode(CameraCommands.SHOOT_SUPER_NIGHT))
        assertTrue(CameraCommands.isPhotoMode(CameraCommands.SHOOT_PHOTO))
        assertEquals("SuperNight", CameraCommands.shootingModeLabel(CameraCommands.SHOOT_SUPER_NIGHT))
        assertEquals(CameraCommands.SHOOT_SUPER_NIGHT, CaptureLists.shootingModeRaw("SuperNight", "Osmo Nano"))
        assertNull(CaptureLists.shootingModeRaw("Low-Light", "Osmo Nano"))
        assertTrue("SuperNight" in CaptureLists.shootingModeLabels("Osmo Nano"))
        assertTrue("Live Photo" !in CaptureLists.shootingModeLabels("Osmo Nano"))
    }

    @Test
    fun photoSkipsRecordConfirmationAndSuperNightDoesNot() {
        assertFalse(
            CaptureShutterPolicy.requiresRecordConfirmation(true, CameraCommands.SHOOT_PHOTO),
        )
        assertFalse(
            CaptureShutterPolicy.requiresRecordConfirmation(true, CameraCommands.SHOOT_PHOTO),
        )
        assertTrue(
            CaptureShutterPolicy.requiresRecordConfirmation(true, CameraCommands.SHOOT_SUPER_NIGHT),
        )
        assertTrue(
            CaptureShutterPolicy.requiresRecordConfirmation(true, CameraCommands.SHOOT_VIDEO),
        )
        assertFalse(
            CaptureShutterPolicy.requiresRecordConfirmation(false, CameraCommands.SHOOT_VIDEO),
        )
        val live = CaptureShutterPolicy.request(
            CameraCommands.SHOOT_VIDEO, recording = false, locked = false, busy = false,
            phase = com.opencapture.openpocketcine.core.ConnectionPhase.LIVE,
        )
        val slowMo = live.copy(shootingMode = CameraCommands.SHOOT_SLOWMO)
        val photo = live.copy(shootingMode = CameraCommands.SHOOT_PHOTO)
        val rec = live.copy(recording = true)
        val locked = live.copy(locked = true)
        assertTrue(CaptureShutterPolicy.shouldDismiss(live, slowMo))
        assertTrue(CaptureShutterPolicy.shouldDismiss(live, photo))
        assertTrue(CaptureShutterPolicy.shouldDismiss(live, rec))
        assertFalse(CaptureShutterPolicy.canCommit(live, photo))
        assertFalse(CaptureShutterPolicy.canCommit(live, locked))
        assertTrue(CaptureShutterPolicy.canCommit(live, live))
        assertFalse(photo.canConfirm)
        assertTrue(live.canConfirm)
    }

    @Test
    fun photoFormatIsReadOnlyAndDoesNotKeepVideoFps() {
        val photo = CameraStatus(
            shootingMode = CameraCommands.SHOOT_PHOTO,
            resolutionCode = CameraCommands.RES_4K,
            fpsIndex = 6,
            fps = 60,
            availableVideoFormats = leftoverVideo,
        )
        assertEquals("Photo", CaptureLists.recFormatChipLabel(photo))
        assertFalse(CaptureLists.formatPickerEditable(photo))
        assertEquals(listOf("Photo"), CaptureLists.formatDrumLabels(photo, tab = 0))
        assertEquals(emptyList(), CaptureLists.modeTabs(LiveSheet.FORMAT, photo, offersIsoAuto = false))
        assertNull(CaptureLists.nextVideoFormat(photo, tab = 0, drum = "60p", fromDrum = true))
        assertEquals("FORMAT", CaptureLists.headerTitle(LiveSheet.FORMAT, -1, photo.shootingMode))
        assertEquals("Photo", CaptureLists.headerSubtitle(LiveSheet.FORMAT, -1, 0, false, photo.shootingMode))
        val hold = recordingCategoryQuickControl(LiveSheet.FORMAT, photo, "Osmo Pocket 3")
        assertEquals(listOf("Photo"), hold?.options)
        assertFalse(hold!!.enabled)
    }

    @Test
    fun photoShutterHasNoAngleLadder() {
        assertEquals(emptyList(), CaptureLists.shutterModeTabs(false, CameraCommands.SHOOT_PHOTO))
        assertEquals(listOf("Speed", "Angle"), CaptureLists.shutterModeTabs(false, CameraCommands.SHOOT_VIDEO))
        assertFalse(CaptureLists.isAngleSheet(LiveSheet.SHUTTER, CameraCommands.EXPO_MANUAL, 1, CameraCommands.SHOOT_PHOTO))
        assertTrue(CaptureLists.isAngleSheet(LiveSheet.SHUTTER, CameraCommands.EXPO_MANUAL, 1, CameraCommands.SHOOT_VIDEO))
        assertEquals(
            0,
            CaptureLists.shutterTabAfterExpoChange(
                CameraCommands.EXPO_MANUAL, shutterUsesAngle = true, CameraCommands.SHOOT_PHOTO,
            ),
        )
        val photo = CameraStatus(
            shootingMode = CameraCommands.SHOOT_PHOTO,
            expoMode = CameraCommands.EXPO_MANUAL,
            shutterDenom = 50,
            fps = 25,
        )
        assertEquals(
            CaptureLists.shutterLabels(photo),
            CaptureLists.shutterWheelOptions(photo, isEvSheet = false, isAngleSheet = false),
        )
    }

    @Test
    fun portraitPhotoOpensModeNotRecSetup() {
        assertEquals("MODE", CaptureShutterPolicy.portraitSetupLabel(CameraCommands.SHOOT_PHOTO))
        assertEquals(LiveSheet.MODE, CaptureShutterPolicy.portraitSetupSheet(CameraCommands.SHOOT_PHOTO))
        assertTrue(CaptureShutterPolicy.portraitSetupOpensMode(CameraCommands.SHOOT_PHOTO))
        assertEquals("REC SETUP", CaptureShutterPolicy.portraitSetupLabel(CameraCommands.SHOOT_VIDEO))
        assertEquals(LiveSheet.FORMAT, CaptureShutterPolicy.portraitSetupSheet(CameraCommands.SHOOT_VIDEO))
        assertEquals("REC SETUP", CaptureShutterPolicy.portraitSetupLabel(CameraCommands.SHOOT_SUPER_NIGHT))
        assertFalse(CaptureShutterPolicy.canRevertFormatFailure(CameraCommands.SHOOT_PHOTO, CameraCommands.SHOOT_VIDEO))
        assertTrue(CaptureShutterPolicy.canRevertFormatFailure(CameraCommands.SHOOT_VIDEO, CameraCommands.SHOOT_VIDEO))
        val fourK30 = VideoFormat(VideoResolution.P4K, VideoFrameRate.FPS30)
        assertFalse(
            VideoFormat.allowsOperatorSet(fourK30, emptyList(), pocket3, CameraCommands.SHOOT_VIDEO),
        )
        assertFalse(
            VideoFormat.allowsOperatorSet(
                fourK30, emptyList(), CameraModel("Osmo Pocket 4 Pro"), CameraCommands.SHOOT_VIDEO,
            ),
        )
        assertFalse(CaptureLists.formatPickerEditable(CameraStatus(shootingMode = CameraCommands.SHOOT_VIDEO)))
    }

    @Test
    fun firstUnknownModeKeepsCapsButLaterModeChangeDropsThem() {
        assertTrue(CameraStatus.shouldPreserveModeDependentCaps(-1, CameraCommands.SHOOT_VIDEO))
        assertTrue(CameraStatus.shouldPreserveModeDependentCaps(CameraCommands.SHOOT_VIDEO, CameraCommands.SHOOT_VIDEO))
        assertFalse(CameraStatus.shouldPreserveModeDependentCaps(CameraCommands.SHOOT_VIDEO, CameraCommands.SHOOT_PHOTO))
        val video = CameraStatus(
            shootingMode = CameraCommands.SHOOT_VIDEO,
            availableVideoFormats = leftoverVideo,
            availableShutterDenoms = listOf(50, 100),
            availableIsoIndices = listOf(3, 4, 5),
            availableColorModes = listOf(CameraCommands.COLOR_NORMAL),
        )
        val seeded = CameraStatus(
            shootingMode = CameraCommands.SHOOT_VIDEO,
            availableShutterDenoms = emptyList(),
        ).mergingModeDependentCaps(
            CameraStatus(shootingMode = -1, availableShutterDenoms = listOf(50)),
        )
        assertEquals(listOf(50), seeded.availableShutterDenoms)
        val photo = video.copy(shootingMode = CameraCommands.SHOOT_PHOTO)
        val resurrected = photo.copy(
            availableShutterDenoms = emptyList(),
            availableIsoIndices = emptyList(),
            availableColorModes = emptyList(),
            availableVideoFormats = emptyList(),
        ).mergingModeDependentCaps(video)
        assertTrue(resurrected.availableShutterDenoms.isEmpty())
        assertTrue(resurrected.availableIsoIndices.isEmpty())
        assertTrue(resurrected.availableColorModes.isEmpty())
        assertTrue(resurrected.availableVideoFormats.isEmpty())
        val dropped = photo.droppingStaleModeDependentCaps(video)
        assertTrue(dropped.availableVideoFormats.isEmpty())
        assertTrue(dropped.availableShutterDenoms.isEmpty())
        assertTrue(dropped.availableIsoIndices.isEmpty())
        assertTrue(dropped.availableColorModes.isEmpty())
        val freshIso = photo.copy(availableIsoIndices = listOf(7, 8))
        assertEquals(listOf(7, 8), freshIso.droppingStaleModeDependentCaps(video).availableIsoIndices)
    }

    @Test
    fun modeChangeDropsLeftoverVideoCapsUnlessANewTableArrived() {
        val video = CameraStatus(
            shootingMode = CameraCommands.SHOOT_VIDEO,
            availableVideoFormats = leftoverVideo,
        )
        val photo = video.copy(shootingMode = CameraCommands.SHOOT_PHOTO)
        assertTrue(photo.droppingStaleVideoFormats(video).availableVideoFormats.isEmpty())
        val slowMoTable = listOf(VideoFormat(VideoResolution.P1080, VideoFrameRate.FPS240))
        val slowMo = video.copy(
            shootingMode = CameraCommands.SHOOT_SLOWMO,
            availableVideoFormats = slowMoTable,
        )
        assertEquals(slowMoTable, slowMo.droppingStaleVideoFormats(video).availableVideoFormats)
        assertEquals(leftoverVideo, video.droppingStaleVideoFormats(video).availableVideoFormats)
        val cleared = video.clearedModeDependentCapabilities()
        assertTrue(cleared.availableVideoFormats.isEmpty())
        assertTrue(cleared.availableShutterDenoms.isEmpty())
        assertTrue(cleared.availableIsoIndices.isEmpty())
        assertTrue(cleared.availableColorModes.isEmpty())
        assertEquals(CameraCommands.SHOOT_VIDEO, cleared.shootingMode)
    }

    @Test
    fun effectiveFormatsIgnoreStaleVideoTableInPhoto() {
        val photo = CameraStatus(
            shootingMode = CameraCommands.SHOOT_PHOTO,
            availableVideoFormats = leftoverVideo,
        )
        assertTrue(CaptureLists.effectiveVideoFormats(photo, pocket3).isEmpty())
        val slowMo = CameraStatus(shootingMode = CameraCommands.SHOOT_SLOWMO)
        assertEquals(
            VideoFormat.pickerFormats(emptyList(), pocket3, CameraCommands.SHOOT_SLOWMO),
            CaptureLists.effectiveVideoFormats(slowMo, pocket3),
        )
    }

    @Test
    fun photoHidesVideoColorAudioAndIsoStars() {
        val leftover = CameraStatus(
            shootingMode = CameraCommands.SHOOT_PHOTO,
            colorMode = CameraCommands.COLOR_DLOG_M,
            availableColorModes = listOf(CameraCommands.COLOR_DLOG_M, CameraCommands.COLOR_NORMAL),
            audioChannel = CameraCommands.AUDIO_STEREO,
            availableVideoFormats = leftoverVideo,
        )
        assertNull(recordingCategoryQuickControl(LiveSheet.COLOR, leftover, family = "pocket"))
        assertTrue(CaptureLists.isoMarkedLabels(leftover).isEmpty())
        assertFalse(CaptureShutterPolicy.showsAudioControls(leftover.shootingMode))
        val photoIso = leftover.copy(
            colorMode = CameraCommands.COLOR_DLOG2,
            availableIsoIndices = listOf(0, 0x03, 0x04, 0x05),
            isoLimit = 0x05,
        )
        assertTrue(CaptureLists.offersIsoAuto(photoIso))
        assertEquals("100\u2013200", CaptureLists.isoAutoLabels(photoIso).first())
        assertEquals("100\u20131600", CaptureLists.isoAutoLabel(photoIso))
        assertEquals(
            IsoLimit.Max800,
            CaptureLists.isoLimit("100\u2013800", photoIso),
        )
        val emptyCaps = leftover.copy(colorMode = CameraCommands.COLOR_DLOG2, isoIndex = 0x05)
        assertTrue(CaptureLists.offersIsoAuto(emptyCaps))
        assertEquals(
            CaptureLists.isoFallback(CameraCommands.COLOR_NORMAL),
            CaptureLists.isoIndices(emptyCaps),
        )
        val video = leftover.copy(shootingMode = CameraCommands.SHOOT_VIDEO)
        assertEquals(setOf("400"), CaptureLists.isoMarkedLabels(video.copy(colorMode = CameraCommands.COLOR_DLOG)))
    }

}
