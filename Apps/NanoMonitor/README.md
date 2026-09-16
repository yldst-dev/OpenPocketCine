# Nano Monitor

A standalone Go viewer for Osmo Nano on the same IPv4 LAN. It opens a video-only
FFplay window. It does not depend on the Swift package, iOS app or Android app
at build time or runtime. The complete folder can be copied into another repo.

## Requirements

- Go 1.26 or newer to build; this module has no third-party Go dependencies.
- FFplay on `PATH`, or an executable path supplied with `-ffplay`.
- Nano already provisioned onto the same WPA2 router as the computer.
- A private IPv4 network that allows direct communication between clients.
- Nano in normal Video mode, with other camera monitoring apps disconnected.

Joining the computer to a router does not put the Nano on that router. Initial
camera network provisioning is a separate Bluetooth operation, available in
the parent project's experimental iPhone Multiview workflow. This viewer does
not perform that operation or store a Wi-Fi password. The existing Mac
single-camera app's direct Nano connection is not evidence that station mode
has been configured.

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
- Send Nano gate `0x02/0x09` and enable `0x09/0xa8` to receiver `0x41` once.
- ACK at 40 Hz, with independent video, command-reply and extra cursors.
- Assemble complete declared-length frames across transport group boundaries.
- Remove Nano private AVC metadata by length, preserving normal AVC NAL units.
- Wait for SPS/PPS plus IDR initially and after a packet gap.

The watchdog allows 8 seconds after each enable and observes picture-input
stalls. It makes at most 2 recovery enables, then exits with an error. Healthy
input for 30 seconds resets this budget. It never sends periodic keyframe
requests merely because the previous keyframe is old. Unlike the full mobile
session, it does not automatically rebuild the UDP endpoint or re-provision
Wi-Fi after recovery exhaustion.

Every access unit is capped at 4 MiB. The application handoff holds at most
8 units; receiver and diagnostic queues are also bounded. Overload stops the
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

An optional FFmpeg/FFplay integration check opens a 10-second synthetic video:

```sh
NANO_MONITOR_REAL_PLAYER=1 go test ./internal/adapters/ffplay -run '^TestActualFFplay$' -count=1 -v
```

Physical Nano-on-router validation is still required. The earlier Mac iPad-app
connection confirmation does not qualify this separate Go implementation.
