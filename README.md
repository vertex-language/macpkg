# macpkg

[![Go Reference](https://pkg.go.dev/badge/github.com/vertex-language/macpkg.svg)](https://pkg.go.dev/github.com/vertex-language/macpkg)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Platform: Cross-Platform](https://img.shields.io/badge/Platform-macOS%20%7C%20Linux%20%7C%20Windows-lightgrey.svg)]()
[![Cgo: Zero](https://img.shields.io/badge/Cgo-0%25-brightgreen.svg)]()

**A 100% pure Go unified toolchain and library for assembling, signing, packaging, and notarizing Apple macOS software (`.app` bundles, `.dmg` disk images, `.pkg` installer packages, and Apple Notarization).**

Zero Cgo. Zero Xcode. Zero mandatory external tools (`xcodebuild`, `pkgbuild`, `productbuild`, `hdiutil`, `codesign`, `notarytool` not required). Runs with byte-for-byte reproducibility on Linux, Windows, and macOS.

---

## Table of Contents

- [Overview](#overview)
- [End-to-End Packaging Flow](#end-to-end-packaging-flow)
- [Real macOS Native Host Verification](#real-macos-native-host-verification)
- [The 5 Core Engines](#the-5-core-engines)
  - [1. Application Bundle Engine (`app/`)](#1-application-bundle-engine-app)
  - [2. Code Signing Engine (`sign/`)](#2-code-signing-engine-sign)
  - [3. Apple Disk Image Engine (`dmg/`)](#3-apple-disk-image-engine-dmg)
  - [4. Flat Package Engine (`pkg/`)](#4-flat-package-engine-pkg)
  - [5. Apple Notarization & Stapler (`notary/`)](#5-apple-notarization--stapler-notary)
- [Command-Line Interface (CLI)](#command-line-interface-cli)
  - [Installation](#installation)
  - [CLI Commands & Examples](#cli-commands--examples)
- [Go Programmatic API](#go-programmatic-api)
  - [1. Assembling a `.app` Bundle](#1-assembling-a-app-bundle)
  - [2. Code Signing with Hardened Runtime](#2-code-signing-with-hardened-runtime)
  - [3. Building a Branded `.dmg` Disk Image](#3-building-a-branded-dmg-disk-image)
  - [4. Compiling a Flat `.pkg` Installer](#4-compiling-a-flat-pkg-installer)
  - [5. Unified Pipeline with `macpkg.Build`](#5-unified-pipeline-with-macpkgbuild)
- [Hermetic & In-Memory Builds (`vfs.MemFS`)](#hermetic--in-memory-builds-vfsmemfs)
- [Shared Architecture with `winpkg`](#shared-architecture-with-winpkg)
- [License](#license)

---

## Overview

Shipping macOS applications traditionally demands a physical Mac or expensive macOS cloud runners to invoke Apple developer tools (`codesign`, `hdiutil`, `pkgbuild`, `notarytool`).

`github.com/vertex-language/macpkg` replaces these external dependencies with **pure-Go implementations** of every binary container, filesystem structure, cryptographic envelope, and API required in macOS software packaging:

* **`.app` Bundles**: Standard macOS bundle directory layout, `Info.plist` (XML & Apple binary `bplist00`), `PkgInfo`, and multi-resolution Apple Icon (`.icns`) encoding.
* **Code Signing**: Embeds Mach-O `LC_CODE_SIGNATURE` `SuperBlob` structures, `CodeDirectory` (version 0x20400) page hashing, special slot binding (`Info.plist` at slot -1, `CodeResources` at slot -3), Apple canonical rule sets, and Hardened Runtime (`CS_RUNTIME`).
* **`.dmg` Apple Disk Images**: Universal Disk Image Format (UDIF) with 512-byte `koly` trailer at EOF-512, UDZO zlib Deflate compression chunks, pure-Go HFS+ filesystem, and `.DS_Store` binary Buddy Allocator records (`Iloc`, `bwsp`, `icvp`).
* **`.pkg` Flat Packages**: Apple Extensible Archive (XAR) container with SHA-1 TOC checksum at heap offset 0, pure-Go Apple `BOMStore` generator with B-Tree leaf layout & POSIX CRC-32 checksums, and standard POSIX odc (`070707`) streaming cpio.gz payload.
* **Apple Notarization & Stapling**: App Store Connect API v2 client using ECDSA ES256 JWT tokens, direct parallel AWS S3 multipart upload, polling status, and CloudKit ticket extraction and offline stapling.

---

## End-to-End Packaging Flow

```
                [ Raw Mach-O Executable (arm64 / x86_64) ]
                                    │
                                    ▼
       1. Assemble Application Bundle (.app) [app.Assemble / macpkg app build]
          - Generates Contents/MacOS/<exec>
          - Generates Contents/Info.plist (XML Plist)
          - Generates Contents/PkgInfo (APPL????)
          - Encodes Contents/Resources/<icon>.icns
                                    │
                                    ▼
       2. Cryptographic Code Signing [sign.Sign / macpkg sign]
          - Hashes bundle resources -> Contents/_CodeSignature/CodeResources
          - Binds Info.plist (slot -1) and CodeResources (slot -3) in CodeDirectory
          - Injects LC_CODE_SIGNATURE SuperBlob with Hardened Runtime (CS_RUNTIME)
          - Ad-hoc signature ("-") or Developer ID Application certificate
                                    │
                  ┌─────────────────┴─────────────────┐
                  │                                   │
                  ▼                                   ▼
       3a. Disk Image (.dmg)               3b. Flat Package (.pkg)
           [dmg.Build / macpkg dmg build]       [pkg.Build / macpkg pkg build]
           - HFS+ Volume filesystem             - POSIX odc 070707 cpio.gz Payload
           - .DS_Store Finder layout            - Apple BOMStore Bill of Materials
             (Icon positions, window bounds)    - PackageInfo XML metadata
           - Applications symlink               - XAR archive container
           - UDIF compressed chunks (UDZO)        (SHA-1 TOC checksum in heap)
           - 512-byte koly trailer
                  │                                   │
                  └─────────────────┬─────────────────┘
                                    │
                                    ▼
       4. Apple Notarization & Stapling [notary.Submit / staple.Staple]
          - Mint App Store Connect API JWT token (ES256 ECDSA P-256)
          - Submit artifact to Apple Notary API v2
          - Stream upload chunks to presigned AWS S3 storage
          - Poll status until "Accepted"
          - Staple CloudKit ticket directly into .app, .dmg, or .pkg
```

---

## Real macOS Native Host Verification

Every format produced by `macpkg` is tested and verified directly on macOS against Apple's native host tools:

```
======================================================================
  macpkg Real macOS Native Host Verification Results
======================================================================
[✓] Clang / Cocoa arm64 Mach-O: Compiled & tested headless execution
[✓] macpkg app build:           Assembled valid macOS application bundle
[✓] macpkg sign:                Signed with ad-hoc identity & hardened runtime
[✓] codesign --verify:          "valid on disk", "satisfies Designated Requirement"
[✓] macpkg dmg build:           Generated compressed UDZO disk image
[✓] hdiutil attach / detach:    Mounted volume, verified .DS_Store, executed app, detached cleanly
[✓] macpkg pkg build:           Built flat installer package
[✓] /usr/bin/xar:               Listed & extracted PackageInfo, Bom, and Payload
[✓] /usr/bin/lsbom:             Parsed permissions, uid/gid 0/80, sizes, and CRC-32 checksums
[✓] pkgutil --expand-full:      Unpacked full directory hierarchy & executables
[✓] /usr/sbin/installer:        LIVE INSTALL COMPLETED: "The upgrade was successful"
[✓] Installed App Run:          /Users/galaxy/Applications/SampleApp.app executed successfully
======================================================================
```

---

## The 5 Core Engines

### 1. Application Bundle Engine (`app/`)

An Application Bundle is a structured directory hierarchy recognized by macOS LaunchServices.

* **`plist/` (Property Lists)**:
  * Pure Go serializer and deserializer for both **XML property lists** (`<!DOCTYPE plist ...>`) and **Apple Binary Property Lists** (`bplist00`).
  * Enforces mandatory `Info.plist` keys: `CFBundleIdentifier`, `CFBundleExecutable`, `CFBundlePackageType` (`APPL`), `CFBundleShortVersionString`, `CFBundleVersion`, `LSMinimumSystemVersion`, and `NSHighResolutionCapable`.
* **`icns/` (Apple Icon Images)**:
  * Encodes multi-resolution icon containers (`.icns`) from raw PNG or image sources.
  * Supports standard Apple OSType markers: `ic07` (128x128), `ic08` (256x256), `ic09` (512x512), and `ic10` (1024x1024 / 512x512@2x Retina).
* **`structure/` (Bundle Hierarchy)**:
  * Creates `Contents/MacOS`, `Contents/Resources`, `Contents/_CodeSignature`, and generates `Contents/PkgInfo` (`APPL????`).

---

### 2. Code Signing Engine (`sign/`)

macOS Gatekeeper mandates cryptographic signatures on all executables and application bundles.

* **`macho/` (Mach-O Code Directory & SuperBlob)**:
  * Serializes the `SuperBlob` structure (`CSMAGIC_EMBEDDED_SIGNATURE = 0xfade0cc0`) into the binary's `LC_CODE_SIGNATURE` load command.
  * Emits `CS_CodeDirectory` (version `0x20400`) with SHA-256 code slot hashes computed for every 4096-byte page of the executable.
  * Correctly computes and binds negative special slots:
    * **Slot -1**: `Info.plist` SHA-256 digest
    * **Slot -2**: Requirements blob (`CSMAGIC_REQUIREMENTS = 0xfade0c01`)
    * **Slot -3**: `CodeResources` SHA-256 digest
    * **Slot -5**: Entitlements XML blob (`CSMAGIC_EMBEDDED_ENTITLEMENTS = 0xfade0c05`)
    * **Slot -7**: DER-encoded entitlements
* **`coderesources/` (Resource Directory Tree Hashing)**:
  * Generates `Contents/_CodeSignature/CodeResources` with Apple's canonical rules (13 rules) and SHA-1 / SHA-256 hashes of all resources and nested binaries.
* **Hardened Runtime**:
  * Sets the `CS_RUNTIME` bitflag (`0x10000`) in the `CodeDirectory`, required for Apple Notarization.

---

### 3. Apple Disk Image Engine (`dmg/`)

Apple Disk Images are mountable virtual block devices wrapped in a Universal Disk Image Format (UDIF) container.

* **`udif/` (Universal Disk Image Format)**:
  * **512-Byte `koly` Trailer**: `.dmg` files are recognized by a 512-byte `koly` trailer at offset `EOF - 512`.
  * Encodes header version 4, sector counts, XML partition plist (`<title> (Apple_HFS : 0)`), and chunk tables.
  * **Chunk Descriptors (`blkx`)**: Generates compressed `UDZO` zlib Deflate chunks (`0x80000005`), raw data chunks (`0x00000001`), and ignored/sparse chunks (`0x00000002`).
* **`hfs/` (HFS+ Filesystem)**:
  * Constructs the `HFSPlusVolumeHeader` (`0x482B`), Allocation File bitmap, Extents Overflow B-Tree, and Catalog B-Tree containing the `.app` bundle and `/Applications` symlink.
* **`dsstore/` (Finder Layout Presentation)**:
  * Reverse-engineered implementation of Apple's `.DS_Store` binary Buddy Allocator and B-Tree.
  * Encodes `Iloc` (icon coordinates), `bwsp` (window bounds), and `icvp` (icon size, background image reference).

---

### 4. Flat Package Engine (`pkg/`)

macOS Flat Packages are the official enterprise installer format, supported by `installer`, Jamf Pro, Munki, and Microsoft Intune.

* **`xar/` (Extensible Archive Container)**:
  * Serializes the 28-byte XAR header (`0x78617221`), zlib-compressed XML Table of Contents (TOC), and heap.
  * Implements Apple's standard checksum layout: algorithm `1` (`SHA-1`), with the 20-byte SHA-1 digest of the compressed TOC placed at heap offset 0.
* **`bom/` (Apple Bill of Materials)**:
  * Pure-Go generator for Apple's `BOMStore` format (magic `BOMStore\0`).
  * Emits block tables, variable pointers (`BomInfo`, `Paths`, `HLIndex`, `VIndex`, `Size64`), and B-Tree leaf structures encoding file modes (`0755`, `0644`), UID/GID (`0/80`), sizes, and POSIX CRC-32 checksums. Verified against `/usr/bin/lsbom`.
* **`cpio/` (Archive Payload Streaming)**:
  * Implements POSIX odc (`070707`) 76-byte ASCII header format required by macOS `/usr/sbin/installer` and `pkgutil --expand-full`.
  * Streams gzip-compressed `Payload` and `Scripts`.
* **`packageinfo/` (PackageInfo XML)**:
  * Generates `<pkg-info>` XML declarations with `format-version="2"`, install location, file counts, and installed size.

---

### 5. Apple Notarization & Stapler (`notary/`)

Distributing software outside the Mac App Store requires notarization by Apple's automated service.

* **`jwt/` (App Store Connect Authentication)**:
  * Automatically mints ES256 JSON Web Tokens using ECDSA P-256 private keys (`AuthKey_<KeyID>.p8`).
* **`client/` (Notary REST API v2)**:
  * Connects to `https://appstoreconnect.apple.com/notary/v2`.
  * Manages package submission, parallel AWS S3 chunk uploads, polling status, and fetching audit logs.
* **`staple/` (CloudKit Ticket Stapling)**:
  * Queries CloudKit for the issued notarization ticket.
  * Injects the ticket directly into `.pkg`, `.dmg`, or `.app` for offline Gatekeeper validation.

---

## Command-Line Interface (CLI)

### Installation

```bash
go install github.com/vertex-language/macpkg/cmd/macpkg@latest
```

### CLI Commands & Examples

```bash
macpkg v1.0.0 - Pure-Go macOS Packaging & Signing Toolchain

Usage:
  macpkg <command> [subcommand] [flags]

Commands:
  app       Assemble macOS Application Bundles (.app)
  dmg       Build Apple Disk Images (.dmg)
  pkg       Compile macOS Flat Packages (.pkg)
  sign      Code-sign Mach-O binaries and bundles
  notary    Apple Notarization REST API v2 and stapler
  staple    Staple notarization ticket to .app, .dmg, or .pkg
  version   Print macpkg version
  help      Display help information
```

#### 1. Assemble an Application Bundle
```bash
macpkg app build \
  --name "MyApp" \
  --id "com.example.myapp" \
  --version "1.0.0" \
  --bin ./bin/myapp \
  --icon ./assets/AppIcon.icns \
  --out dist/MyApp.app
```

#### 2. Sign Bundle with Hardened Runtime
```bash
# Ad-hoc signing (for local testing / development)
macpkg sign --target dist/MyApp.app --identity "-" --hardened

# Developer ID signing (for distribution)
macpkg sign \
  --target dist/MyApp.app \
  --cert cert.pem \
  --key key.pem \
  --hardened \
  --deep
```

#### 3. Build a Branded Disk Image (.dmg)
```bash
macpkg dmg build \
  --title "MyApp Installer" \
  --app dist/MyApp.app \
  --out dist/MyApp.dmg \
  --bg assets/dmg-bg.png \
  --icon-size 128
```

#### 4. Compile a Flat Package (.pkg)
```bash
macpkg pkg build \
  --id "com.example.myapp.pkg" \
  --version "1.0.0" \
  --location "/Applications" \
  --payload dist/MyApp.app \
  --out dist/MyApp.pkg
```

#### 5. Submit to Apple Notarization & Staple
```bash
# Submit and wait for completion
macpkg notary submit dist/MyApp.dmg \
  --issuer "57246542-96fe-1a63-e053-0824d011072a" \
  --key-id "2X9R4NN74K" \
  --key "AuthKey_2X9R4NN74K.p8" \
  --wait

# Staple ticket to artifact
macpkg staple dist/MyApp.dmg
```

---

## Go Programmatic API

`macpkg` follows Go standard library conventions, utilizing declarative **Config structs** rather than fluent builders.

### 1. Assembling a `.app` Bundle

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
        Name:         "SuperApp",
        Identifier:   "com.example.superapp",
        Version:      "1.0.0",
        Build:        "1",
        SourceBinary: "bin/superapp",
        SourceIcon:   "assets/AppIcon.icns",
        Category:     "public.app-category.developer-tools",
        MinOS:        "11.0",
        OutDir:       "dist/SuperApp.app",
        FS:           vfs.RealFS(""),
    })
    if err != nil {
        log.Fatalf("Assemble failed: %v", err)
    }

    log.Printf("Successfully assembled %s (%d files, %d bytes)",
        bundle.Path, len(bundle.Files), bundle.TotalSize)
}
```

### 2. Code Signing with Hardened Runtime

```go
package main

import (
    "context"
    "log"

    "github.com/vertex-language/macpkg/sign"
    "github.com/vertex-language/macpkg/vfs"
)

func main() {
    ctx := context.Background()

    res, err := sign.Sign(ctx, sign.Config{
        Target:   "dist/SuperApp.app",
        Identity: "-", // Ad-hoc signing (or pass CertPath/KeyPath)
        Hardened: true,
        Deep:     true,
        Force:    true,
        FS:       vfs.RealFS(""),
    })
    if err != nil {
        log.Fatalf("Signing failed: %v", err)
    }

    log.Printf("Signed %s (Identifier: %s, Format: %s)",
        res.Target, res.Identifier, res.Format)
}
```

### 3. Building a Branded `.dmg` Disk Image

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
        OutFile:              "dist/SuperApp.dmg",
        Background:           "assets/dmg-bg.png",
        IconSize:             128,
        AddApplicationsLink:  true,
        WindowSize:           dmg.Size{Width: 640, Height: 480},
        AppPosition:          dmg.Point{X: 180, Y: 240},
        ApplicationsPosition: dmg.Point{X: 460, Y: 240},
        FS:                   vfs.RealFS(""),
    })
    if err != nil {
        log.Fatalf("DMG build failed: %v", err)
    }

    log.Printf("Built DMG: %s (%d bytes)", res.OutputFile, res.TotalSize)
}
```

### 4. Compiling a Flat `.pkg` Installer

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
        Version:         "1.0.0",
        InstallLocation: "/Applications",
        SourcePayload:   "dist/SuperApp.app",
        OutFile:         "dist/SuperApp.pkg",
        FS:              vfs.RealFS(""),
    })
    if err != nil {
        log.Fatalf("PKG build failed: %v", err)
    }

    log.Printf("Built PKG: %s (%d files, %d bytes)",
        res.OutputFile, res.FilesCount, res.TotalSize)
}
```

### 5. Unified Pipeline with `macpkg.Build`

Orchestrate the entire pipeline using the top-level facade:

```go
package main

import (
    "context"
    "log"

    "github.com/vertex-language/macpkg"
    "github.com/vertex-language/macpkg/app"
    "github.com/vertex-language/macpkg/sign"
)

func main() {
    ctx := context.Background()

    res, err := macpkg.Build(ctx, macpkg.Config{
        Format: macpkg.FormatApp,
        App: &app.Config{
            Name:         "SuperApp",
            Identifier:   "com.example.superapp",
            SourceBinary: "bin/superapp",
            OutDir:       "dist/SuperApp.app",
        },
        Sign: &sign.Config{
            Identity: "-",
            Hardened: true,
        },
    })
    if err != nil {
        log.Fatalf("Pipeline failed: %v", err)
    }

    log.Printf("Completed %s build: %s", res.Format, res.Artifact)
}
```

---

## Hermetic & In-Memory Builds (`vfs.MemFS`)

Every packaging engine in `macpkg` accepts a pluggable virtual filesystem (`vfs.FS`). This allows complete pipelines to execute in memory without disk I/O, perfect for unit tests and deterministic CI builds:

```go
mem := vfs.NewMemFS()

// Assemble bundle entirely in memory
bundle, err := app.Assemble(ctx, app.Config{
    Name:           "InMemoryApp",
    Identifier:     "com.example.inmemory",
    Executable:     "app",
    ExecutableData: []byte("RAW MACH-O DATA"),
    FS:             mem,
})

// Read the generated Info.plist directly from memory
plistData, _ := mem.ReadFile("InMemoryApp.app/Contents/Info.plist")
```

---

## Shared Architecture with `winpkg`

`macpkg` shares design principles and architectural patterns with [`github.com/vertex-language/winpkg`](https://github.com/vertex-language/winpkg):

| Capability | `winpkg` (Windows) | `macpkg` (macOS) |
|---|---|---|
| **Zero Cgo / Host Tools** | No `msi.dll`, `signtool.exe`, `wix.exe` | No `codesign`, `hdiutil`, `pkgbuild` |
| **Pluggable Virtual FS** | `vfs.FS` (`RealFS` & `MemFS`) | `vfs.FS` (`RealFS` & `MemFS`) |
| **API Pattern** | `Config` structs with zero-defaults | `Config` structs with zero-defaults |
| **App Bundle Format** | MSIX Package (`msix.Build`) | macOS App Bundle (`app.Assemble`) |
| **Installer Format** | Windows Installer MSI (`msi.Build`) | Flat Package PKG (`pkg.Build`) |
| **Disk Image Format** | - | Apple UDZO DMG (`dmg.Build`) |
| **Code Signing** | Authenticode PKCS#7 (`sign.Sign`) | Mach-O LC_CODE_SIGNATURE (`sign.Sign`) |
| **Cloud Service** | Windows Store / Azure | Apple Notarization REST API v2 (`notary`) |

---

## License

MIT License. See [LICENSE](LICENSE) for details.
