# macpkg

[![Go Reference](https://pkg.go.dev/badge/github.com/vertex-language/macpkg.svg)](https://pkg.go.dev/github.com/vertex-language/macpkg)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Platform: Cross-Platform](https://img.shields.io/badge/Platform-macOS%20%7C%20Linux%20%7C%20Windows-lightgrey.svg)]()

**A 100% pure Go unified toolchain and library for assembling, inspecting, signing, packaging, and notarizing Apple macOS software (`.app` bundles, `.dmg` disk images, `.pkg` installer packages, and Sparkle-compatible `.zip` archives).**

Zero Cgo. Zero Xcode (`xcodebuild` / `pkgbuild` / `productbuild` / `hdiutil` / `codesign` / `notarytool`). Zero macOS host requirements. Works with byte-for-byte reproducibility across Linux AMD64/ARM64, macOS Apple Silicon/Intel, and Windows runners.

---

## Table of Contents

- [Overview](#overview)
- [Why `macpkg`?](#why-macpkg)
- [Architecture & Internal Design](#architecture--internal-design)
- [The Apple Packaging Pipeline](#the-apple-packaging-pipeline)
- [Format Specifications](#format-specifications)
  - [1. Application Bundles (`.app`)](#1-application-bundles-app)
  - [2. Apple Disk Images (`.dmg` / UDIF)](#2-apple-disk-images-dmg--udif)
  - [3. Flat & Distribution Packages (`.pkg` / XAR)](#3-flat--distribution-packages-pkg--xar)
  - [4. Pure Go Code Signing (`sign`)](#4-pure-go-code-signing-sign)
  - [5. Apple Notarization & Stapling (`notary`)](#5-apple-notarization--stapling-notary)
- [Declarative Manifest (`macpkg.yaml`)](#declarative-manifest-macpkgyaml)
- [Command-Line Interface (CLI)](#command-line-interface-cli)
- [Go Programmatic API](#go-programmatic-api)
- [Shared Infrastructure](#shared-infrastructure)
- [License](#license)

---

## Overview

Shipping software on macOS is notoriously multi-staged and historically tethered to Apple hardware:

1. **Inner Application Bundle (`.app`)**: Assembling Mach-O binaries, universal fat binaries (`lipo`), icons (`.icns`), asset catalogs, and `Info.plist`.
2. **Code Signing & Hardened Runtime**: Hashing Mach-O load commands (`LC_CODE_SIGNATURE`), building `_CodeSignature/CodeResources`, injecting entitlements plists, and generating CMS / PKCS#7 signatures with Developer ID Application certificates.
3. **Delivery Containers**:
   - **Consumer (`.dmg`)**: Serializing Universal Disk Image Format (UDIF) containers, constructing raw HFS+/APFS partitions, encoding binary `.DS_Store` Finder layouts (window positions, icon grid placement), and embedding Software License Agreements (SLA).
   - **Enterprise & App Store (`.pkg`)**: Serializing Extensible Archive (XAR) headers, streaming compressed `cpio` payloads, generating Apple Bill of Materials (`.bom`) binary indexes, and structuring `Distribution` XML definitions.
4. **Apple Notarization & Stapling**: Authenticating to App Store Connect REST APIs via JWT (ES256), uploading packages, polling status, fetching CloudKit notarization tickets, and stapling tickets directly into package headers/trailers for offline Gatekeeper validation.

`github.com/vertex-language/macpkg` replaces the entire proprietary Apple toolchain with a **single, pure Go cross-platform engine**.

---

## Why `macpkg`?

- **100% Pure Go**: Zero Cgo. Compiles on any OS (`GOOS=linux`, `GOOS=darwin`, `GOOS=windows`). Build and sign production-ready macOS installers directly on cheap Linux GitHub Actions runners or Docker containers without paying for macOS cloud instances.
- **In-Memory & Hermetic**: Build `.app` bundles, `.dmg` images, and `.pkg` installers entirely in-memory using virtual filesystems (`vfs.MemFS`) with zero disk I/O and deterministic timestamps.
- **Unified Standard**: Replaces fragmented single-purpose scripts (`dmgbuild`, `bomutils`, `xar`, `libdmg-hfsplus`, `rcodesign`) with a single cohesive architecture.
- **Gatekeeper Compliant**: Produces hardened runtime signatures, code resources, and notarization stapling fully compliant with modern macOS Gatekeeper requirements (macOS 10.15 Catalina through macOS 15+ Sequoia).

---

## Architecture & Internal Design

```
macpkg/
├── macpkg.go                 # Unified top-level facade & auto-detection
├── vfs/                      # [Shared] Virtual filesystem (MemFS, DiskFS/RealFS)
├── run/                      # [Shared] Process runner abstraction & test seams
│
├── app/                      # Application Bundle (.app) assembler
│   ├── bundle.go             # Bundle hierarchy (Contents/MacOS, Resources, Frameworks)
│   ├── plist.go              # Info.plist XML and binary property list serializer
│   ├── icns.go               # Apple Icon Image (.icns) encoder & generator
│   └── lipo.go               # Universal Mach-O fat binary packager
│
├── dmg/                      # Apple Disk Image (.dmg) engine
│   ├── udif.go               # Universal Disk Image Format serializer & block chunker
│   ├── hfs.go                # HFS+ filesystem partition & volume generator
│   ├── dsstore.go            # Apple .DS_Store binary B-tree parser & serializer
│   └── sla.go                # Software License Agreement resource dictionary injector
│
├── pkg/                      # Flat & Distribution Installer (.pkg) engine
│   ├── xar.go                # Extensible Archive (XAR) container & XML TOC serializer
│   ├── bom.go                # Apple Bill of Materials (.bom) binary index generator
│   ├── cpio.go               # Compressed cpio payload & script stream generator
│   ├── packageinfo.go        # PackageInfo XML definition builder
│   └── distribution.go       # Distribution XML multi-package installer generator
│
├── sign/                     # Pure Go Apple Code Signing Engine
│   ├── codesign.go           # Mach-O LC_CODE_SIGNATURE builder & CMS signer
│   ├── coderesources.go      # _CodeSignature/CodeResources SHA-256 tree hasher
│   ├── entitlements.go       # Hardened runtime flags & entitlements XML embedder
│   ├── xar_sign.go           # XAR container envelope signature (Developer ID Installer)
│   └── cert.go               # Apple Developer ID certificate & private key loader
│
├── notary/                   # Apple Notarization & Stapling Engine
│   ├── client.go             # App Store Connect Notary REST API client (JWT ES256)
│   ├── submit.go             # S3 presigned asset uploader & submission poller
│   └── staple.go             # CloudKit ticket injector (XAR header & UDIF trailer)
│
├── internal/
│   └── cli/                  # Unified CLI engine
└── cmd/
    └── macpkg/
        └── main.go           # CLI binary entrypoint
```

---

## The Apple Packaging Pipeline

```
[ Mach-O Binary / Assets ]
            │
            ▼
   ┌─────────────────┐
   │   app.Builder   │  Assemble .app (Info.plist, icon, dylibs)
   └────────┬────────┘
            │
            ▼
   ┌─────────────────┐
   │   sign.Sign()   │  Sign Mach-O & seal _CodeSignature/CodeResources
   └────────┬────────┘
            │
    ┌───────┴────────────────────────┐
    ▼                                ▼
┌─────────────────┐          ┌─────────────────┐
│   dmg.Builder   │          │   pkg.Builder   │
│   (Consumer)    │          │  (Enterprise)   │
│  UDIF + HFS+    │          │  XAR + BOM cpio │
│  + .DS_Store    │          │  + Distribution │
└───────┬─────────┘          └───────┬─────────┘
        │                            │
        └─────────────┬──────────────┘
                      ▼
             ┌─────────────────┐
             │  notary.Submit  │  Upload to Apple Notary REST API
             └────────┬────────┘
                      ▼
             ┌─────────────────┐
             │  notary.Staple  │  Staple CloudKit ticket into container
             └─────────────────┘
```

---

## Format Specifications

### 1. Application Bundles (`.app`)
An Apple bundle is a standardized directory structure recognized by LaunchServices:
* `Contents/Info.plist`: Key/value property list declaring `CFBundleIdentifier`, `CFBundleExecutable`, `CFBundleVersion`, `LSMinimumSystemVersion`, and document types.
* `Contents/MacOS/`: Executable binaries (single-architecture or multi-arch Universal Mach-O fat headers).
* `Contents/Resources/`: Branded `.icns` icons, asset catalogs (`Assets.car`), and localized `.lproj` directories.
* `Contents/Frameworks/`: Shared dylibs with `@rpath` runtime linker configurations.
* `Contents/_CodeSignature/CodeResources`: XML plist storing individual SHA-1 and SHA-256 digests of every resource file.

### 2. Apple Disk Images (`.dmg` / UDIF)
Universal Disk Image Format (UDIF) block containers:
* **Container Header**: 512-byte `koly` trailer structure defining sector counts, XML property list offsets, and chunk table checksums.
* **Compression**: Bit-accurate `UDZO` (zlib Deflate) and `ULFO` (LZFSE) chunk streams.
* **HFS+ Filesystem**: In-process generation of Allocation Files, Extents Overflow B-Trees, and Catalog B-Trees containing the `.app` bundle and a symlink to `/Applications`.
* **Finder Layout (`.DS_Store`)**: Binary serialization of Apple Buddy Allocator pages containing `Iloc`, `BWSP`, and `icvp` records to set Finder window coordinates, custom background images, and icon dimensions.

### 3. Flat & Distribution Packages (`.pkg` / XAR)
The enterprise installer format utilized by Jamf, Munki, Microsoft Intune, and the Mac App Store:
* **XAR Container**: 28-byte header, zlib-compressed XML Table of Contents (TOC) with SHA-256 checksums, and trailing heap data.
* **Apple BOM (`.bom`)**: Binary Bill of Materials index encoding file paths, Unix permissions (`0755`, `0644`), UID/GID, modification times, and checksums.
* **Payload Stream**: Streaming `cpio` archive compressed with gzip (`.cpio.gz`) or XZ (`.cpio.xz`).
* **Distribution XML**: Orchestrates welcome screens, license acceptance (HTML/RTF), custom target volume requirements, and pre/postinstall shell scripts.

### 4. Pure Go Code Signing (`sign`)
* **Mach-O Signing**: Serializes the `SuperBlob` structure into the Mach-O `LC_CODE_SIGNATURE` load command, containing the `CodeDirectory` (version `0x20400`), `Requirements` blob, and `Entitlements` XML blob.
* **Hardened Runtime**: Sets the `CS_RUNTIME` flag (`0x10000`) enabling macOS hardened memory protection.
* **CMS / PKCS#7**: Generates RFC 5652 cryptographic envelopes using Apple Developer ID Application certificates with SHA-256 digests and RFC 3161 secure timestamps.

### 5. Apple Notarization & Stapling (`notary`)
* **REST API Authentication**: Mints JSON Web Tokens (JWT) using Elliptic Curve ES256 keys generated from App Store Connect API keys (`AuthKey_<KeyID>.p8`).
* **Asset Staging**: Fetches presigned S3 upload URLs and streams packages in parallel.
* **Ticket Stapling**: Fetches base64-encoded CloudKit notarization records and staples them directly:
  * For `.pkg`: Injected as an embedded XAR leaf.
  * For `.dmg`: Injected into the UDIF trailer (`kUDIFSignatureResource`).

---

## Declarative Manifest (`macpkg.yaml`)

Configure your entire macOS distribution pipeline in a single, human-readable manifest:

```yaml
bundle:
  name: "MyApp"
  identifier: "com.example.myapp"
  version: "1.0.0"
  build: "100"
  executable: "bin/myapp-darwin-universal"
  icon: "assets/icon.png"
  category: "public.app-category.developer-tools"
  min_os: "11.0"
  entitlements: "build/entitlements.plist"

sign:
  identity: "Developer ID Application: Example Corp (TEAMID1234)"
  key: "certs/dev_id_app.p12"
  password: "${MACOS_CERT_PASSWORD}"
  hardened_runtime: true
  timestamp: true

targets:
  - dmg:
      output: "dist/MyApp-1.0.0.dmg"
      title: "MyApp Installer"
      background: "assets/dmg-background.png"
      window_size: [600, 400]
      icon_size: 128
      app_position: [150, 200]
      applications_symlink_position: [450, 200]
      license: "LICENSE.txt"

  - pkg:
      output: "dist/MyApp-1.0.0.pkg"
      identifier: "com.example.myapp.pkg"
      install_location: "/Applications"
      scripts: "scripts/" # preinstall, postinstall
      signing_identity: "Developer ID Installer: Example Corp (TEAMID1234)"

notary:
  key_id: "2X9R4NN74K"
  issuer_id: "57246542-96fe-1a63-e053-0824d011072a"
  private_key: "certs/AuthKey_2X9R4NN74K.p8"
  staple: true
```

---

## Command-Line Interface (CLI)

### Build Packages

```bash
# Auto-detects macpkg.yaml and executes pipeline (.app -> .dmg -> .pkg)
macpkg build

# Build a drag-and-drop DMG directly from an existing .app bundle
macpkg dmg pack dist/MyApp.app -o dist/MyApp.dmg --background bg.png

# Build a silent enterprise .pkg installer with scripts
macpkg pkg pack dist/MyApp.app -o dist/MyApp.pkg --install-location /Applications
```

### Sign & Notarize

```bash
# Sign Mach-O binary or .app bundle with Hardened Runtime
macpkg sign dist/MyApp.app --cert cert.p12 --entitlements entitlements.plist

# Submit package to Apple Notary Service and staple result
macpkg notary submit dist/MyApp.dmg --key-id KEYID --issuer-id ISSUER --key key.p8 --staple
```

### Inspect & Verify

```bash
# Inspect .pkg (lists XAR TOC, BOM files, permissions)
macpkg pkg inspect dist/MyApp.pkg

# Verify signature and Gatekeeper acceptance
macpkg verify dist/MyApp.dmg
```

---

## Go Programmatic API

### Assemble `.app` Bundle

```go
import (
    "context"
    "github.com/vertex-language/macpkg/app"
)

bundle, err := app.NewBuilder().
    WithName("MyApp").
    WithIdentifier("com.example.myapp").
    WithVersion("1.0.0").
    WithExecutable("bin/app", binaryBytes).
    WithIcon("assets/icon.png", iconBytes).
    Build()
```

### Build `.dmg` Disk Image

```go
import (
    "github.com/vertex-language/macpkg/dmg"
)

d := dmg.NewBuilder().
    WithTitle("MyApp Installer").
    WithAppBundle(bundle).
    WithApplicationsSymlink().
    WithWindowSize(600, 400).
    WithBackground(bgPNGBytes)

dmgBytes, err := d.Build()
```

### Build Enterprise `.pkg` Installer

```go
import (
    "github.com/vertex-language/macpkg/pkg"
)

p := pkg.NewBuilder().
    WithIdentifier("com.example.myapp.pkg").
    WithVersion("1.0.0").
    WithInstallLocation("/Applications").
    WithPayload(bundle).
    WithPostInstallScript(scriptBytes)

pkgBytes, err := p.Build()
```

---

## Shared Infrastructure

`macpkg` shares core architecture patterns with [`winpkg`](https://github.com/vertex-language/winpkg):
* **`vfs.FS`**: Pluggable filesystem interfaces (`RealFS`, `MemFS`) for pure in-memory compilation without temp directory pollution.
* **`run.Runner`**: Process execution seaming and hermetic mocking for external hooks.

---

## License

MIT
