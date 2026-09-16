# Nano Monitor

A standalone Go viewer for Osmo Nano on the same IPv4 LAN. It opens a video-only
FFplay window. It does not depend on the Swift package, iOS app or Android app
at build time or runtime. The complete folder can be copied into another repo.

## Requirements

- Go 1.26 or newer to build; this module has no third-party Go dependencies.
- FFplay on `PATH`, or an executable path supplied with `-ffplay`.
- Nano on the same WPA2 router as the computer after setup.
- A private IPv4 network that allows direct communication between clients.
- Nano in normal Video mode, with other camera monitoring apps disconnected.

On macOS, `setup` provisions the Nano over Bluetooth using native CoreBluetooth.
This build requires Xcode Command Line Tools and cgo; no Apple development
account or third-party Go package is required. Other platforms can build the
viewer with `CGO_ENABLED=0`, but cannot run Bluetooth setup.

## Connect Nano to a router

Attach Nano to its powered vision dock, finish first activation in DJI Mimo,
then disconnect Mimo and other camera apps. Connect the computer to the target
WPA2 router and run:

```sh
just build
./bin/nano-monitor setup -list
./bin/nano-monitor setup -interface en0 -monitor
```

Select the router-connected interface for this computer. If several cameras
appear, add `-device` with the exact name or Bluetooth ID from the list. Enter
the router name and password in the native dialogs. Approve a connection request
on Nano if shown. Password input is hidden and is not written to arguments,
logs or files. Temporary Go credential buffers are cleared after use; this is
not a guarantee that every OS-managed memory copy has been erased.

Setup pairs, requests station mode, supplies the router credentials and checks
the same camera name over the LAN before reporting success. A join response
alone does not prove connectivity. `-monitor` opens the viewer only after that
check. A missing join response still allows bounded LAN verification.

If the router already lists the camera, add `setup -camera 192.168.10.42` to
skip subnet discovery. A failed LAN check may leave Nano in station mode. Check
client isolation, the selected interface and macOS Local Network permission
before repeating setup. All-host route or permission failures are reported
separately from an empty candidate list. If necessary, request its own Wi-Fi:

```sh
./bin/nano-monitor setup -restore-ap
```

Verify restoration on the camera and in the computer's Wi-Fi list. The command
confirms acceptance of the request, not successful AP restoration.

Only 1 monitoring client should use a Nano at a time. Starting another client
may take over the camera's unicast preview. If using Multiview to provision the
camera, remove its tile to release the monitor while leaving the camera on the
shared network. Closing Multiview with the camera still assigned may return it
to its own Wi-Fi. Check the router's client list before starting this viewer.

## Build and run

From this directory:

```sh
just check
just build
./bin/nano-monitor -camera 192.168.10.42
```

On macOS, if the terminal executable is blocked from the LAN or never appears
in Local Network settings, build and launch the app bundle:

```sh
just mac-run -interface en0 -camera 192.168.10.42
```

The bundle has a stable identifier and the Bluetooth/Local Network usage
strings. Approve macOS's access prompt if shown. Logs are in `bin/monitor.log`.
If the first attempt reports a route/permission error, it stays alive for
20 seconds so macOS can present its prompt; allow access and run it again.
That error can also indicate a real routing fault, so it is not proof of a
permission denial. Initial TCP errors now stop startup rather than being
hidden by a later UDP error.

`mac-app` signs locally with an ad hoc signature by default. For reliable
identity tracking across builds, set `NANO_CODESIGN_IDENTITY` to an installed
Apple-issued signing identity. No identity is stored in the repository.
See [Apple's local network privacy guidance](https://developer.apple.com/documentation/technotes/tn3179-understanding-local-network-privacy).
`mac-run` requires FFplay on the terminal's PATH and passes its absolute path
to the app. Close the existing viewer before starting another instance.

To select a network interface and pin the camera's identity:

```sh
./bin/nano-monitor -interface en0 -camera 192.168.10.42 -name OsmoNano-TEST
```

The example address and name are placeholders. `-name` is the Nano's own
camera Wi-Fi name, not the router SSID. The reply must have an `OsmoNano` prefix
even when `-name` is supplied. Arbitrarily renamed cameras are not supported.
This is a protocol/name check, not cryptographic device authentication.

Find candidates without requesting video:

```sh
./bin/nano-monitor -discover -interface en0
```

Discovery checks TCP 7001 on the selected local subnet, with at most 24 parallel
connections. It supports prefixes `/22` through `/30`, excludes the local,
network and broadcast addresses, and stops on cancellation. Results are service
candidates, not verified cameras. There is no public-address or whole-internet
scan. For larger LANs, supply the camera IP from the router instead.

Without `-camera`, the program discovers candidates on the selected interface.
It connects automatically only when exactly 1 candidate exists, and checks its
DUML identity reply before registration or preview. Multiple interfaces or
candidates require an explicit selection.

Press Esc or close the FFplay window to stop. Ctrl+C in the terminal also stops
the session. FFplay's F key toggles fullscreen. The program does not record to
disk, change exposure, capture photos, or start/stop camera recording.

## Clean architecture

| Layer | Directory | Responsibility |
| --- | --- | --- |
| Domain | `internal/domain` | Access-unit bounds and pure recovery decisions |
| Application | `internal/application` | Camera/display ports, bounded handoff and cancellation |
| Camera adapter | `internal/adapters/nano` | DUML, UDP windows, session setup, Nano AVC assembly |
| Network adapter | `internal/adapters/network` | Interface selection and bounded LAN discovery |
| Bluetooth and secret adapters | `internal/adapters/bluetooth`, `internal/adapters/secret` | Native macOS transport and secure input |
| Display adapter | `internal/adapters/ffplay` | FFplay process and its input pipe |
| Composition root | `cmd/nano-monitor` | Flags, signals and dependency wiring |

The domain imports no I/O or platform code. The application imports the domain
and knows only the camera/display interfaces. Adapters do not control the
application. Tests substitute the ports and use a loopback camera without
hardware or a media player.

## Protocol and failure behavior

The port follows the parent project's DUML framing, initial station window,
three ACK windows and Nano access-unit framing:

- Keep the TCP 7001 initialization connection open.
- Open UDP 9004 from an ephemeral local port bound to the selected interface.
- Wait for both handshake and initial telemetry before selecting command sequence.
- Check the camera-name reply before device registration and subscriptions.
- Send Nano gate `0x02/0x09`, wait up to 3 seconds for its matching success
  reply, then send enable `0x09/0xa8` to receiver `0x41` once. Recovery enables
  use the same gate acknowledgement barrier.
- ACK at 40 Hz, with independent video, command-reply and extra cursors.
- Assemble complete declared-length frames across transport group boundaries.
- Remove Nano private AVC metadata by length, preserving normal AVC NAL units.
- Wait for SPS/PPS plus IDR initially and after a packet gap.

The watchdog allows 8 seconds after each enable and observes picture-input
stalls. It makes at most 2 recovery enables within a session. Healthy input for
30 seconds resets that budget. If recovery is exhausted, it keeps the display
process and opens a fresh TCP/UDP session after 1 second, rechecking the pinned
camera identity before preview. At most 2 fresh sessions are attempted per run;
cancellation, identity failures and unrelated errors are not blindly retried.
Exhausting that bound exits with an explicit error. It never sends periodic
keyframe requests merely because the previous keyframe is old and does not
re-provision Wi-Fi during recovery.

Late or duplicate video packets are ignored without discarding the current
picture. Video and reply ACK cursors cannot move backward on retransmissions,
including across 16-bit sequence wrap. Genuine forward gaps still require IDR
resynchronization. Recovery messages include loss and ignored-packet counts.
A reported window closure was traced to the old recovery-exhaustion exit, not
an observed OS crash. Late-packet handling defects are covered by regression
tests; the earlier log cannot prove which packet triggered that incident.

Every access unit is capped at 4 MiB. The application handoff holds at most
8 units for short scheduling bursts; receiver and diagnostic queues are also bounded. Overload stops the
session rather than silently dropping dependent AVC pictures. FFplay decodes
and displays the video; received pictures do not prove presentation timing.

The preview uses plaintext camera traffic on the local network. There is no
HTTP server, cloud upload, password cache or saved footage in this program.

## Validation

`just check` runs formatting, `go vet`, race-enabled tests and a build. Tests
cover captured byte vectors, malformed/truncated packets, independent ACK
cursors, sequence wrap, large frames, lost fragments, private metadata,
initial IDR gating, recovery limits, subprocess exit and loopback sessions.

Fuzz the wire and AVC parsers separately:

```sh
go test ./internal/adapters/nano -run '^$' -fuzz FuzzWire -fuzztime 10s
go test ./internal/adapters/nano -run '^$' -fuzz FuzzAVC -fuzztime 10s
```

An optional FFmpeg/FFplay integration check feeds a 10-second synthetic burst
and requires it to drain within 5 seconds instead of replaying an old timeline:

```sh
NANO_MONITOR_REAL_PLAYER=1 go test ./internal/adapters/ffplay -run '^TestActualFFplay$' -count=1 -v
```

Physical Bluetooth provisioning reached an accepted join response. The macOS
app bundle verified Nano identity over the router LAN and displayed real AVC
video in FFplay. Waiting for the gate reply removed a reproducible `0xd6`
rejection despite non-playback camera status. Earlier runs with file-style
playback timing accumulated latency and eventually overflowed the input queue.

The live player now uses packet-arrival timestamps rather than generated
recording timestamps, skips stream-info probing, disables AVIO read-ahead and
uses 1 decoder thread. The compressed handoff is reduced from 32 to 8 units;
frames needed for H.264 prediction are not discarded. Late decoded frames may
be dropped by FFplay. A 2-unit trial was not retained because short scheduling
bursts could terminate the session. Overload still stops explicitly rather than
silently accumulating unbounded latency.

On this host, the 10-second synthetic burst test took about 10.45 seconds with
the previous settings and 1.28 seconds with the live settings, including test
setup. This is a backlog-drain comparison, not camera-to-screen latency.
Physical video display is verified, and the operator reported near-immediate
response to camera movement after the live-player changes. This is subjective
confirmation, not a millisecond measurement. Long-run stability, measured
glass-to-glass latency and AP restoration remain unqualified.
