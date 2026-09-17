# macpkg

[![Go Reference](https://pkg.go.dev/badge/github.com/vertex-language/macpkg.svg)](https://pkg.go.dev/github.com/vertex-language/macpkg)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Platform: Cross-Platform](https://img.shields.io/badge/Platform-macOS%20%7C%20Linux%20%7C%20Windows-lightgrey.svg)]()

**A 100% pure Go unified toolchain and library for assembling, signing, packaging, and notarizing Apple macOS software (`.app` bundles, `.dmg` disk images, `.pkg` installer packages, and Sparkle-compatible `.zip` archives).**

Zero Cgo. Zero Xcode (`xcodebuild` / `pkgbuild` / `productbuild` / `hdiutil` / `codesign` / `notarytool`). Zero Apple hardware or macOS host requirements. Works with bit-for-bit reproducibility on Linux AMD64/ARM64, macOS Apple Silicon/Intel, and Windows CI runners.

---

## Table of Contents

- [Overview](#overview)
- [Why `macpkg`?](#why-macpkg)
- [API Design Philosophy: Config Structs vs Builders](#api-design-philosophy-config-structs-vs-builders)
- [Repository & Package Architecture](#repository--package-architecture)
- [Format Deep Dives & Specifications](#format-deep-dives--specifications)
  - [1. Application Bundle Engine (`app/`)](#1-application-bundle-engine-app)
  - [2. Apple Disk Image Engine (`dmg/`)](#2-apple-disk-image-engine-dmg)
  - [3. Flat & Distribution Installer Engine (`pkg/`)](#3-flat--distribution-installer-engine-pkg)
  - [4. Pure Go Code Signing Engine (`sign/`)](#4-pure-go-code-signing-engine-sign)
  - [5. Apple Notarization & Stapling Engine (`notary/`)](#5-apple-notarization--stapling-engine-notary)
- [Declarative Manifest Specification (`macpkg.yaml`)](#declarative-manifest-specification-macpkgyaml)
- [Go Programmatic API](#go-programmatic-api)
- [Command-Line Interface (CLI)](#command-line-interface-cli)
- [Shared Infrastructure with `winpkg`](#shared-infrastructure-with-winpkg)
- [License](#license)

---

## Overview

Historically, shipping software on Apple platforms has required a macOS machine running proprietary Apple toolchains. Even cross-platform toolchains (GoReleaser, Tauri, Electron Builder, Flutter) either require paying for expensive macOS CI runners or executing fragile, reverse-engineered shell scripts and wrappers (`dmgbuild`, `libdmg-hfsplus`, `bomutils`, `xar`, `rcodesign`).

`github.com/vertex-language/macpkg` solves this by delivering **native, pure-Go implementations** of every binary format, filesystem, container, cryptographic signature, and cloud API required in the macOS software delivery lifecycle:

```
[ Mach-O Binaries & Assets ]
              │
              ▼
   ┌──────────────────────┐
   │     app.Assemble     │   Directory structure, Info.plist, .icns, universal lipo
   └──────────┬───────────┘
              │
              ▼
   ┌──────────────────────┐
   │      sign.Sign       │   Mach-O LC_CODE_SIGNATURE, CodeResources, Hardened Runtime
   └──────────┬───────────┘
              │
      ┌───────┴──────────────────────────────┐
      ▼                                      ▼
┌──────────────────────┐           ┌──────────────────────┐
│      dmg.Build       │           │      pkg.Build       │
│  UDIF block chunks   │           │  XAR XML container   │
│  HFS+ filesystem     │           │  Apple BOM catalog   │
│  .DS_Store B-Tree    │           │  cpio.gz stream      │
│  SLA license dict    │           │  Distribution XML    │
└──────────┬───────────┘           └──────────┬───────────┘
           │                                  │
           └─────────────────┬────────────────┘
                             ▼
                   ┌──────────────────────┐
                   │    notary.Submit     │   App Store Connect REST API (JWT ES256)
                   └─────────┬────────────┘
                             ▼
                   ┌──────────────────────┐
                   │    notary.Staple     │   Inject CloudKit ticket into container
                   └──────────────────────┘
```

---

## Why `macpkg`?

* **100% Pure Go & Zero Cgo**: Compiles anywhere (`GOOS=linux`, `GOOS=darwin`, `GOOS=windows`). Generate fully signed, notarized DMGs and PKGs directly inside standard Ubuntu Linux Docker containers.
* **Hermetic & In-Memory (`vfs.MemFS`)**: Entire pipelines can run without touching the host disk. Perfect for unit testing, reproducible CI builds, and byte-for-byte deterministic package hashes.
* **Zero Host Toolchains**: No `hdiutil`, no `pkgbuild`, no `productbuild`, no `codesign`, and no `altool`/`notarytool`.
* **Full Gatekeeper Compliance**: Creates valid `SuperBlob` code signatures with Hardened Runtime flags (`CS_RUNTIME = 0x10000`), SHA-256 resource trees, entitlements, and offline ticket stapling compatible with macOS 10.15 (Catalina) through macOS 15+ (Sequoia).

---

## API Design Philosophy: Config Structs vs Builders

In Go, heavily chained "fluent" builders (`builder.WithName().WithVersion().Build()`) are widely considered an un-idiomatic anti-pattern borrowed from Java/C#. Fluent builders introduce two major problems in Go:
1. **Error propagation**: Every chained method must either return an error (which destroys chaining) or defer error reporting until `Build()`, hiding where invalid state was introduced.
2. **Impedance mismatch with serialization**: A fluent builder cannot be deserialized directly from YAML or JSON.

Following standard Go library practices (`http.Server`, `exec.Cmd`, `tls.Config`, and `winpkg/msi`):

> **`macpkg` uses declarative `Config` structs with zero-value defaults.**

* **Declarative & Typed**: Struct literals provide self-documenting named fields.
* **Direct Serialization**: `macpkg.yaml` maps directly to Go config structs via `yaml.Unmarshal`.
* **Clean Error Handling**: Constructors and execution functions take a `context.Context` and a `Config` struct, returning `(Result, error)` synchronously and reliably.

---

## Repository & Package Architecture

Every macOS format is an intricate binary specification. Rather than monolithic files, each format is organized into dedicated, cohesive subpackages:

```
macpkg/
├── macpkg.go                      # Top-level unified facade (auto-detect manifest & build)
├── macpkg_test.go                 # End-to-end integration tests (in-memory pipeline)
├── go.mod                         # module github.com/vertex-language/macpkg
├── README.md                      # Documentation & format specifications
├── LICENSE                        # MIT License
│
├── vfs/                           # [SHARED] Virtual Filesystem (MemFS, DiskFS/RealFS)
├── run/                           # [SHARED] Process runner abstraction & test seams
│
├── app/                           # [FORMAT] Application Bundle (.app) Engine
│   ├── app.go                     # app.Assemble, app.Config, app.Bundle
│   ├── app_test.go
│   ├── plist/                     # Property List XML & Apple binary bplist00 parser/serializer
│   ├── icns/                      # Apple Icon Image (.icns) multi-resolution encoder
│   ├── lipo/                      # Universal Mach-O fat binary packager (arm64 + x86_64)
│   ├── rpath/                     # Mach-O load command dynamic linker path editor (@rpath)
│   └── structure/                 # Standard bundle layout generator & PkgInfo (APPL????)
│
├── dmg/                           # [FORMAT] Apple Disk Image (.dmg) Engine
│   ├── dmg.go                     # dmg.Build, dmg.Config, dmg.Inspect
│   ├── dmg_test.go
│   ├── udif/                      # Universal Disk Image Format (512-byte koly trailer & blkx)
│   ├── hfs/                       # In-memory HFS+ volume, catalog B-Tree & allocation bitmap
│   ├── dsstore/                   # .DS_Store binary Buddy Allocator & Finder layout (Iloc, bwsp, icvp)
│   └── sla/                       # Software License Agreement multilingual LPic/XML resource dictionary
│
├── pkg/                           # [FORMAT] Flat & Distribution Installer (.pkg) Engine
│   ├── pkg.go                     # pkg.Build, pkg.Config, pkg.Inspect
│   ├── pkg_test.go
│   ├── xar/                       # Extensible Archive (XAR) XML TOC, header & compressed heap
│   ├── bom/                       # Apple Bill of Materials (BOMStore) binary VTree indexer
│   ├── cpio/                      # Streaming SVR4 portable cpio (070701) archive encoder
│   ├── packageinfo/               # PackageInfo XML manifest generator & validator
│   ├── distribution/              # Distribution XML product definition (choices, requirements)
│   └── scripts/                   # preinstall, postinstall, and upgrade hook script manager
│
├── sign/                          # [SECURITY] Pure-Go Code Signing Engine
│   ├── sign.go                    # sign.SignFile, sign.SignBundle, sign.Config
│   ├── sign_test.go
│   ├── macho/                     # Mach-O LC_CODE_SIGNATURE SuperBlob & CodeDirectory builder
│   ├── coderesources/             # _CodeSignature/CodeResources SHA-256 tree hasher
│   ├── entitlements/              # Hardened runtime flags & entitlements XML/DER embedding
│   ├── cms/                       # RFC 5652 PKCS#7 / CMS cryptographic envelope & RFC 3161 timestamping
│   └── cert/                      # Developer ID cert loader (PKCS#12 .p12, PEM, Apple Root CA)
│
├── notary/                        # [SECURITY] Apple Notarization & Stapling Engine
│   ├── notary.go                  # notary.Submit, notary.Staple, notary.Config
│   ├── notary_test.go
│   ├── jwt/                       # App Store Connect API token minter (ECDSA ES256)
│   ├── client/                    # Notary REST API v2 HTTP client & AWS S3 presigned uploader
│   └── staple/                    # CloudKit ticket extractor & injector (XAR header & UDIF trailer)
│
├── internal/
│   └── cli/                       # [INTERNAL] Unified CLI Engine
│       ├── cli.go                 # Root dispatcher & auto-detect
│       ├── cmd_app.go             # macpkg app commands
│       ├── cmd_dmg.go             # macpkg dmg commands
│       ├── cmd_pkg.go             # macpkg pkg commands
│       ├── cmd_sign.go            # macpkg sign commands
│       ├── cmd_notary.go          # macpkg notary commands
│       └── cli_test.go            # Full CLI integration test suite
│
└── cmd/
    └── macpkg/
        └── main.go                # CLI binary entrypoint
```

---

## Format Deep Dives & Specifications

### 1. Application Bundle Engine (`app/`)

An Application Bundle is a structured directory hierarchy recognized by macOS LaunchServices.

* **`plist/` (Property Lists)**:
  * Pure Go serializer and deserializer for both **XML property lists** (`<!DOCTYPE plist ...>`) and **Apple Binary Property Lists** (`bplist00`).
  * Enforces mandatory `Info.plist` keys: `CFBundleIdentifier`, `CFBundleExecutable`, `CFBundlePackageType` (`APPL`), `CFBundleShortVersionString`, `CFBundleVersion`, `LSMinimumSystemVersion`, and `NSHighResolutionCapable`.
* **`icns/` (Apple Icon Images)**:
  * Encodes multi-resolution icon containers (`.icns`) from a single high-resolution source image (e.g. 1024x1024 PNG).
  * Automatically packs all required Apple OSType chunk markers:
    * Standard: `ic07` (128x128), `ic08` (256x256), `ic09` (512x512), `ic10` (1024x1024 / 512x512@2x Retina).
    * High-DPI: `ic11` (16x16@2x), `ic12` (32x32@2x), `ic13` (128x128@2x), `ic14` (256x256@2x).
* **`lipo/` (Universal Fat Binaries)**:
  * Reads multiple single-architecture Mach-O executables (e.g. `darwin/arm64` and `darwin/amd64`) and produces a universal Mach-O binary containing the `FatHeader` (`0xCAFEBABE`) and aligned `FatArch` offsets.
* **`rpath/` (Dynamic Linker Paths)**:
  * Inspects and rewrites Mach-O load commands (`LC_LOAD_DYLIB`, `LC_RPATH`, `LC_ID_DYLIB`) to ensure bundled frameworks and dynamic libraries resolve relative to `@executable_path/../Frameworks` or `@rpath`.

---

### 2. Apple Disk Image Engine (`dmg/`)

Apple Disk Images are mountable virtual block devices wrapped in a Universal Disk Image Format (UDIF) container.

* **`udif/` (Universal Disk Image Format)**:
  * **Trailer Architecture (`koly`)**: Modern `.dmg` files do not have a magic header at byte 0; they are identified by a **512-byte `koly` trailer block** located at offset `EOF - 512`.
  * Serializes the `koly` block (`0x6B6F6C79`): header version (`4`), sector counts, XML metadata descriptor offsets, and chunk table checksums.
  * **Chunk Descriptors (`blkx`)**: Generates compressed chunk streams:
    * `0x00000000`: Zero-fill / empty blocks
    * `0x00000001`: Raw uncompressed data
    * `0x00000002`: Ignored / sparse blocks
    * `0x80000005`: `UDZO` zlib Deflate compression
    * `0x80000007`: `ULFO` LZFSE Apple-proprietary compression
    * `0xFFFFFFFF`: Stream terminator
* **`hfs/` (In-Process HFS+ Filesystem)**:
  * Serializes a valid, clean **HFS+ (Hierarchical File System Plus)** volume directly in memory without calling `hdiutil` or mounting loopback devices.
  * Constructs the `HFSPlusVolumeHeader` (`0x482B`), Allocation File bitmap, Extents Overflow B-Tree, and Catalog B-Tree containing the `.app` bundle hierarchy and the `/Applications` symlink.
* **`dsstore/` (Finder Layout & Presentation)**:
  * Reverse-engineers Apple's proprietary `.DS_Store` binary database.
  * Implements the internal **Buddy Allocator** (allocating $2^N$ byte pages) and **B-Tree** index.
  * Writes exact Finder layout records:
    * `Iloc`: (Icon Location) exact pixel coordinates `[x, y]` for the `.app` icon and the `/Applications` folder icon.
    * `bwsp`: (Browser Window Settings Property list) window geometry bounds `[top, left, bottom, right]`, sidebar visibility, and toolbar status.
    * `icvp`: (Icon View Properties) binary plist configuring icon size (e.g. 128px), text label size, label position (bottom vs right), and background image reference.
    * `vstl`: (View Style) set to `icnv` (Icon View).
* **`sla/` (Software License Agreements)**:
  * Embeds multilingual license agreements into the disk image trailer (`LPic` resource dictionary). Displays an unavoidable agreement dialog to the user upon mounting.

---

### 3. Flat & Distribution Installer Engine (`pkg/`)

macOS Flat Packages are the official enterprise distribution format accepted by MDM systems (Jamf Pro, Munki, Microsoft Intune) and mandatory for Mac App Store submissions.

* **`xar/` (Extensible Archive Container)**:
  * Serializes the XAR container: 28-byte header (`magic = 0x78617221` / `xar!`), zlib-compressed XML Table of Contents (TOC) with SHA-256 hashes, and binary data heap.
  * Embeds container-level RSA/ECDSA digital signatures into `<signature style="RSA">` in the TOC header.
* **`bom/` (Apple Bill of Materials)**:
  * Native binary serializer for Apple's proprietary `BOMStore` format (magic `BOMStore\0`).
  * Emits the BOM block table, `Paths` index, `File` records, and B-Tree nodes encoding file modes (`0755`, `0644`), UID/GID (0/80), sizes, checksums, and hardlink maps without relying on `mkbom`.
* **`cpio/` (Archive Payload Streaming)**:
  * High-speed streaming SVR4 portable format archive (`070701` / `070702` with CRC) compressed with `gzip` or `xz`.
  * Generates both `Payload` (installed files) and `Scripts` (`preinstall`, `postinstall`).
* **`packageinfo/` & `distribution/` (XML Manifests)**:
  * Generates component package `PackageInfo` manifests.
  * Generates `Distribution` XML product definitions (`productbuild` equivalent) containing localization, welcome/license screens (HTML/RTF), hardware/OS requirements (`<os-version min="11.0"/>`), and volume checks.

---

### 4. Pure Go Code Signing Engine (`sign/`)

macOS Gatekeeper requires all binaries and bundles to be cryptographically signed with a trusted Apple Developer ID certificate.

* **`macho/` (Mach-O Code Directory & SuperBlob)**:
  * Serializes the `SuperBlob` structure (`CSMAGIC_EMBEDDED_SIGNATURE = 0xfade0cc0`) into the Mach-O binary's `LC_CODE_SIGNATURE` load command.
  * Emits the `CS_CodeDirectory` blob (`version 0x20400` / `0x20500`) with SHA-256 code slot hashes computed for every 4096-byte page of the executable.
  * Computes negative special slots:
    * Slot -1: `Info.plist` SHA-256 hash
    * Slot -2: Requirements blob (`CSMAGIC_REQUIREMENTS = 0xfade0c01`)
    * Slot -3: `CodeResources` SHA-256 hash
    * Slot -5: Entitlements XML blob (`CSMAGIC_EMBEDDED_ENTITLEMENTS = 0xfade0c05`)
    * Slot -7: DER-encoded entitlements
* **`coderesources/` (Resource Directory Tree Hashing)**:
  * Generates `Contents/_CodeSignature/CodeResources` XML plist containing recursive SHA-1 and SHA-256 hashes of all resources, nested dynamic libraries, frameworks, and plugins.
* **Hardened Runtime**:
  * Sets the `CS_RUNTIME` bitflag (`0x10000`) in the CodeDirectory, enforcing Apple runtime integrity protections required for notarization.
* **`cms/` (Cryptographic Message Syntax)**:
  * Pure-Go RFC 5652 CMS / PKCS#7 detached signature generator. Signs code directory hashes using Apple Developer ID Application or Installer certificates.
  * Embeds secure RFC 3161 trusted timestamp tokens from Apple's timestamp authority (`http://timestamp.apple.com/ts01`).

---

### 5. Apple Notarization & Stapling Engine (`notary/`)

All software distributed outside the Mac App Store must be notarized by Apple's automated notary service.

* **`jwt/` (App Store Connect Authentication)**:
  * Automatically signs and mints JSON Web Tokens using ECDSA ES256 private keys (`AuthKey_<KeyID>.p8`), Key IDs, and Issuer GUIDs.
* **`client/` (Notary REST API v2)**:
  * Interacts with Apple's `https://appstoreconnect.apple.com/notary/v2` endpoints.
  * Handles package submission, parallel S3 chunk uploads, polling submission status, and retrieving audit logs.
* **`staple/` (CloudKit Ticket Stapling)**:
  * Fetches the base64-encoded notarization ticket issued by Apple.
  * Injects the ticket directly into packages so Gatekeeper validates them offline:
    * **In `.pkg`**: Inserted as an embedded XAR TOC leaf.
    * **In `.dmg`**: Appended to the UDIF trailer as a signature resource (`kUDIFSignatureResource`).
    * **In `.app`**: Written to `Contents/CodeResources` or the `com.apple.notary.ticket` extended attribute.

---

## Declarative Manifest Specification (`macpkg.yaml`)

Define your entire macOS distribution pipeline in a single declarative manifest:

```yaml
# macpkg.yaml
bundle:
  name: "SuperApp"
  identifier: "com.example.superapp"
  version: "1.2.0"
  build: "120"
  executable: "bin/superapp-darwin-universal"
  icon: "assets/icon.png"
  category: "public.app-category.developer-tools"
  min_os: "11.0"
  entitlements: "build/entitlements.plist"

sign:
  cert_file: "certs/DeveloperIDApp.p12"
  password: "${MACOS_CERT_PASSWORD}"
  hardened_runtime: true
  timestamp: true

targets:
  - dmg:
      output: "dist/SuperApp-1.2.0.dmg"
      title: "SuperApp Installer"
      background: "assets/dmg-background.png"
      window_size: [640, 420]
      icon_size: 128
      app_position: [160, 210]
      applications_symlink_position: [480, 210]
      license: "LICENSE.txt"

  - pkg:
      output: "dist/SuperApp-1.2.0.pkg"
      identifier: "com.example.superapp.pkg"
      install_location: "/Applications"
      scripts: "scripts/" # preinstall, postinstall
      signing_cert_file: "certs/DeveloperIDInstaller.p12"
      signing_password: "${MACOS_INSTALLER_CERT_PASSWORD}"

notary:
  key_id: "2X9R4NN74K"
  issuer_id: "57246542-96fe-1a63-e053-0824d011072a"
  private_key: "certs/AuthKey_2X9R4NN74K.p8"
  staple: true
```

---

## Go Programmatic API

`macpkg` uses **Config structs** throughout its programmatic API for type safety and clarity:

### 1. Assemble an `.app` Bundle

```go
package main

import (
    "context"
    "log"

    "github.com/vertex-language/macpkg/app"
    "github.com/vertex-language/macpkg/vfs"
)

func main() {
    ctx := context.Background()

    bundle, err := app.Assemble(ctx, app.Config{
        Name:       "SuperApp",
        Identifier: "com.example.superapp",
        Version:    "1.2.0",
        Executable: "bin/superapp",
        Icon:       "assets/icon.png",
        MinOS:      "11.0",
        OutDir:     "dist/SuperApp.app",
        FS:         vfs.RealFS(""),
    })
    if err != nil {
        log.Fatal(err)
    }
    log.Printf("Built bundle at %s (%d bytes)", bundle.Path, bundle.TotalSize)
}
```

### 2. Build a Branded `.dmg` Disk Image

```go
package main

import (
    "context"
    "log"

    "github.com/vertex-language/macpkg/dmg"
    "github.com/vertex-language/macpkg/vfs"
)

func main() {
    ctx := context.Background()

    res, err := dmg.Build(ctx, dmg.Config{
        Title:                "SuperApp Installer",
        SourceApp:            "dist/SuperApp.app",
        OutFile:              "dist/SuperApp-1.2.0.dmg",
        Background:           "assets/dmg-background.png",
        WindowSize:           dmg.Size{Width: 640, Height: 420},
        IconSize:             128,
        AppPosition:          dmg.Point{X: 160, Y: 210},
        ApplicationsPosition: dmg.Point{X: 480, Y: 210},
        AddApplicationsLink:  true,
        FS:                   vfs.RealFS(""),
    })
    if err != nil {
        log.Fatal(err)
    }
    log.Printf("Built DMG: %s (%d bytes)", res.OutputFile, res.TotalSize)
}
```

### 3. Build an Enterprise `.pkg` Installer

```go
package main

import (
    "context"
    "log"

    "github.com/vertex-language/macpkg/pkg"
    "github.com/vertex-language/macpkg/vfs"
)

func main() {
    ctx := context.Background()

    res, err := pkg.Build(ctx, pkg.Config{
        Identifier:      "com.example.superapp.pkg",
        Version:         "1.2.0",
        InstallLocation: "/Applications",
        SourcePayload:   "dist/SuperApp.app",
        OutFile:         "dist/SuperApp-1.2.0.pkg",
        ScriptsDir:      "scripts",
        FS:              vfs.RealFS(""),
    })
    if err != nil {
        log.Fatal(err)
    }
    log.Printf("Built PKG: %s (%d bytes)", res.OutputFile, res.TotalSize)
}
```

---

## Command-Line Interface (CLI)

```bash
# Build all targets defined in macpkg.yaml
macpkg build [macpkg.yaml]

# Format-specific commands
macpkg app assemble -c app.yaml -o dist/MyApp.app
macpkg dmg pack dist/MyApp.app -o dist/MyApp.dmg --background bg.png
macpkg pkg pack dist/MyApp.app -o dist/MyApp.pkg --install-location /Applications

# Security & Verification
macpkg sign dist/MyApp.app --cert cert.p12 --entitlements entitlements.plist
macpkg notary submit dist/MyApp.dmg --key-id $KEYID --issuer-id $ISSUER --key auth.p8 --staple
macpkg inspect dist/MyApp.dmg
macpkg verify dist/MyApp.pkg
```

---

## Shared Infrastructure with `winpkg`

`macpkg` shares core engineering primitives with [`github.com/vertex-language/winpkg`](https://github.com/vertex-language/winpkg):
* **`vfs.FS`**: Pluggable filesystem interface (`RealFS` for OS operations, `MemFS` for zero-disk-I/O in-memory testing).
* **`run.Runner`**: Process execution seaming and recording mocks for testing.
* **Declarative Workflow**: Unified manifest patterns across Windows (`winpkg`) and macOS (`macpkg`).

---

## License

MIT
