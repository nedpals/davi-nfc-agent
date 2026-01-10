# Contributing

Thanks for your interest in contributing to the NFC Agent!

## Development Setup

### Requirements

- Go 1.21 or later
- libnfc development libraries
- libfreefare development libraries
- libusb

### Installing Dependencies

**Linux (Debian/Ubuntu):**
```bash
sudo apt install -y libnfc-dev libfreefare-dev libusb-1.0-0-dev
```

**macOS:**
```bash
brew install libnfc libfreefare libusb
```

### Building

```bash
git clone https://github.com/nedpals/davi-nfc-agent.git
cd davi-nfc-agent
go build .
```

### Running Tests

```bash
go test ./...
```

## Advanced Building

For cross-compilation and production builds, use the automated build scripts instead of `go build`.

### Quick Start

```bash
# Build for your current platform (auto-detected)
./scripts/build-unix.sh

# Cross-compile for specific platforms
./scripts/build-unix.sh linux amd64
./scripts/build-unix.sh linux arm64
./scripts/build-unix.sh darwin amd64
./scripts/build-unix.sh darwin arm64

# Cross-compile for Windows (from Linux)
./scripts/build-windows.sh amd64
```

### Prerequisites

**Required Tools:**
- Go 1.24.2+
- autotools: autoconf, automake, libtool
- pkg-config
- wget

**For Cross-Compilation:**
- Zig 0.11.0 (used as C cross-compiler)

**Install Zig:**

```bash
# macOS
brew install zig

# Linux
wget https://ziglang.org/download/0.11.0/zig-linux-x86_64-0.11.0.tar.xz
tar xf zig-linux-x86_64-0.11.0.tar.xz
sudo mv zig-linux-x86_64-0.11.0 /usr/local/zig
export PATH="/usr/local/zig:$PATH"
```

### What the Scripts Do

The build scripts automatically:

1. Download and compile all C dependencies (libusb, libnfc, libfreefare, OpenSSL)
2. Apply platform-specific patches
3. Configure cross-compilation toolchains
4. Build the Go binary with proper CGO flags
5. Install dependencies to `~/cross-build/[os]-[arch]/`

### Platform Support

| Platform | Script | Architectures |
|----------|--------|---------------|
| Linux | `build-unix.sh` | amd64, arm64 |
| macOS | `build-unix.sh` | amd64, arm64 |
| Windows | `build-windows.sh` | amd64 |

### Examples

**Build for Current Platform:**

```bash
./scripts/build-unix.sh
# Output: davi-nfc-agent-darwin-arm64 (or linux-amd64, etc.)
```

**Cross-Compile from macOS to Linux ARM64:**

```bash
./scripts/build-unix.sh linux arm64
# Takes ~10-15 minutes (first time, then cached)
# Output: davi-nfc-agent-linux-arm64
```

**Build All Platforms:**

```bash
# macOS/Linux builds
for os in linux darwin; do
  for arch in amd64 arm64; do
    ./scripts/build-unix.sh $os $arch
  done
done

# Windows (from Linux)
./scripts/build-windows.sh amd64
```

**Build with Version Info:**

```bash
BUILD_VERSION=1.0.0 ./scripts/build-unix.sh darwin arm64
```

### Build Artifacts

Binaries are created in the current directory:

- `davi-nfc-agent-linux-amd64`
- `davi-nfc-agent-linux-arm64`
- `davi-nfc-agent-darwin-amd64`
- `davi-nfc-agent-darwin-arm64`
- `davi-nfc-agent-windows-amd64.exe`

Dependencies are cached in `~/cross-build/` and reused on subsequent builds.

### CI/CD

The GitHub Actions workflow (`.github/workflows/build.yml`) automatically:

1. Builds for all platforms on push to master
2. Creates releases with binaries
3. Sets version info from git tags

**Manual Release:**

```bash
# Tag a release
git tag v1.0.0
git push origin v1.0.0

# CI will build with BUILD_VERSION=v1.0.0
```

### Build Troubleshooting

**Build Fails with CGO Errors:**

Ensure all dependencies are installed:

```bash
# Linux
sudo apt install autoconf automake libtool pkg-config

# macOS
brew install autoconf automake libtool pkg-config
```

**Zig Not Found:**

Add Zig to your PATH:

```bash
export PATH="/usr/local/zig:$PATH"
```

**Clean Build:**

Remove cached dependencies and rebuild:

```bash
rm -rf ~/cross-build/
./scripts/build-unix.sh
```

## Project Architecture

```
davi-nfc-agent/
├── main.go              # Entry point, CLI flags
├── agent.go             # Core agent logic
├── systray.go           # System tray UI
├── buildinfo/           # Version and build metadata
├── nfc/                 # NFC abstraction layer
│   ├── manager.go       # Device manager interface
│   ├── device.go        # Device interface
│   ├── tag.go           # Tag interface
│   ├── reader.go        # NFC reader implementation
│   ├── remotenfc/       # Smartphone NFC support
│   └── multimanager/    # Multiple manager aggregation
├── server/              # WebSocket servers
│   ├── deviceserver/    # Device server (port 9470)
│   └── clientserver/    # Client server (port 9471)
├── tls/                 # Auto-TLS certificate management
├── protocol/            # Protocol definitions
├── client/              # JavaScript client library
├── scripts/             # Build scripts
└── docs/                # Documentation
```

### Key Components

**NFC Layer** (`nfc/`)
- Abstraction over libnfc/libfreefare
- Modular design supporting hardware and smartphone readers
- See [nfc/README.md](nfc/README.md) for details

**Server Layer** (`server/`)
- Two-server architecture:
  - **DeviceServer**: Handles NFC devices and hardware readers
  - **ClientServer**: Handles client applications
- Bridge component for inter-server communication

**TLS Layer** (`tls/`)
- Automatic certificate generation
- Network change detection
- CA bootstrap server for device setup

## Code Style

- Follow standard Go conventions
- Run `go fmt` before committing
- Use meaningful variable and function names
- Add comments for exported functions

## Submitting Changes

### Pull Request Process

1. Fork the repository
2. Create a feature branch: `git checkout -b feature/my-feature`
3. Make your changes
4. Run tests: `go test ./...`
5. Commit with clear message: `git commit -m "Add feature X"`
6. Push to your fork: `git push origin feature/my-feature`
7. Open a Pull Request

### Commit Messages

Use clear, descriptive commit messages:

```
feat: add support for DESFire EV3 cards
fix: handle connection timeout gracefully
docs: update API documentation
refactor: simplify device manager interface
```

### What to Include

- Clear description of changes
- Tests for new functionality
- Documentation updates if needed
- Update CHANGELOG if applicable

## Adding NFC Support

To add support for new NFC readers or tag types, see [docs/extending-nfc-support.md](docs/extending-nfc-support.md).

## Reporting Issues

When reporting bugs, please include:

- Operating system and version
- NFC reader model
- Steps to reproduce
- Expected vs actual behavior
- Relevant log output

## Questions?

Open an issue for questions or discussions.
