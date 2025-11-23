#!/bin/bash
set -e

# Unified build script for macOS and Linux
# Usage: ./scripts/build-unix.sh [os] [arch]
#   os: linux, darwin (defaults to current OS)
#   arch: amd64, arm64 (defaults to current arch)

# Library versions
LIBNFC_VERSION="${LIBNFC_VERSION:-1.8.0}"
LIBFREEFARE_VERSION="${LIBFREEFARE_VERSION:-0.4.0}"
LIBUSB_VERSION="${LIBUSB_VERSION:-1.0.27}"
LIBUSB_COMPAT_VERSION="${LIBUSB_COMPAT_VERSION:-0.1.8}"
OPENSSL_VERSION="${OPENSSL_VERSION:-1.1.1w}"

# Detect current OS and architecture
detect_platform() {
    local os=$(uname -s | tr '[:upper:]' '[:lower:]')
    local arch=$(uname -m)

    case "$arch" in
        x86_64) arch="amd64" ;;
        aarch64) arch="arm64" ;;
    esac

    echo "$os $arch"
}

# Parse arguments
if [ $# -eq 0 ]; then
    read TARGET_OS TARGET_ARCH <<< $(detect_platform)
else
    TARGET_OS="${1}"
    TARGET_ARCH="${2:-amd64}"
fi

echo "=== Building for $TARGET_OS-$TARGET_ARCH ==="

# Setup build directory
BUILD_ROOT="$HOME/cross-build/$TARGET_OS-$TARGET_ARCH"
mkdir -p "$BUILD_ROOT"
export PKG_CONFIG_PATH="$BUILD_ROOT/lib/pkgconfig"

echo "Build root: $BUILD_ROOT"

# Setup compiler and build tools
setup_compiler() {
    case "$TARGET_OS" in
        linux)
            # Use Zig for cross-compilation
            if [ "$TARGET_ARCH" = "amd64" ]; then
                ZIG_TARGET="x86_64-linux-gnu"
            else
                ZIG_TARGET="aarch64-linux-gnu"
            fi

            export CC="zig cc -target $ZIG_TARGET"
            export CXX="zig c++ -target $ZIG_TARGET"
            export AR="zig ar"
            export RANLIB="zig ranlib"
            HOST_FLAG="--host=$ZIG_TARGET"
            MAKE_JOBS="-j$(nproc 2>/dev/null || sysctl -n hw.ncpu 2>/dev/null || echo 4)"
            ;;

        darwin)
            # Use native clang with architecture flags
            SDK_PATH=$(xcrun --show-sdk-path 2>/dev/null || echo "")

            if [ "$TARGET_ARCH" = "amd64" ]; then
                ARCH_FLAG="x86_64"
            else
                ARCH_FLAG="arm64"
            fi

            export CFLAGS="-arch $ARCH_FLAG ${SDK_PATH:+-isysroot $SDK_PATH}"
            export LDFLAGS="-arch $ARCH_FLAG"
            HOST_FLAG=""
            MAKE_JOBS="-j$(sysctl -n hw.ncpu 2>/dev/null || nproc 2>/dev/null || echo 4)"
            ;;

        *)
            echo "Error: Unsupported OS: $TARGET_OS"
            exit 1
            ;;
    esac
}

# Build libusb
build_libusb() {
    echo "=== Building libusb $LIBUSB_VERSION ==="

    wget -q "https://github.com/libusb/libusb/releases/download/v${LIBUSB_VERSION}/libusb-${LIBUSB_VERSION}.tar.bz2"
    tar xjf "libusb-${LIBUSB_VERSION}.tar.bz2"
    cd "libusb-${LIBUSB_VERSION}"

    local configure_flags="--prefix=$BUILD_ROOT --enable-static --disable-shared"

    # Linux: disable udev for cross-compilation
    if [ "$TARGET_OS" = "linux" ]; then
        configure_flags="$configure_flags --disable-udev"
    fi

    ./configure $HOST_FLAG $configure_flags
    make $MAKE_JOBS
    make install
    cd ..

    echo "✓ libusb installed"
}

# Build libusb-compat
build_libusb_compat() {
    echo "=== Building libusb-compat $LIBUSB_COMPAT_VERSION ==="

    wget -q "https://github.com/libusb/libusb-compat-0.1/releases/download/v${LIBUSB_COMPAT_VERSION}/libusb-compat-${LIBUSB_COMPAT_VERSION}.tar.bz2"
    tar xjf "libusb-compat-${LIBUSB_COMPAT_VERSION}.tar.bz2"
    cd "libusb-compat-${LIBUSB_COMPAT_VERSION}"

    ./configure $HOST_FLAG --prefix="$BUILD_ROOT" --enable-static --disable-shared
    make $MAKE_JOBS
    make install
    cd ..

    echo "✓ libusb-compat installed"
}

# Build OpenSSL
build_openssl() {
    echo "=== Building OpenSSL $OPENSSL_VERSION ==="

    wget -q "https://www.openssl.org/source/openssl-${OPENSSL_VERSION}.tar.gz"
    tar xzf "openssl-${OPENSSL_VERSION}.tar.gz"
    cd "openssl-${OPENSSL_VERSION}"

    # Determine OpenSSL target
    case "$TARGET_OS" in
        linux)
            if [ "$TARGET_ARCH" = "amd64" ]; then
                OPENSSL_TARGET="linux-x86_64"
            else
                OPENSSL_TARGET="linux-aarch64"
            fi
            ;;
        darwin)
            if [ "$TARGET_ARCH" = "amd64" ]; then
                OPENSSL_TARGET="darwin64-x86_64-cc"
            else
                OPENSSL_TARGET="darwin64-arm64-cc"
            fi
            ;;
    esac

    ./Configure $OPENSSL_TARGET \
        --prefix="$BUILD_ROOT" \
        --openssldir="$BUILD_ROOT/ssl" \
        no-shared \
        no-tests

    make $MAKE_JOBS
    make install_sw
    cd ..

    # Create pkg-config file
    cat > "$BUILD_ROOT/lib/pkgconfig/libcrypto.pc" << EOF
prefix=$BUILD_ROOT
exec_prefix=\${prefix}
libdir=\${exec_prefix}/lib
includedir=\${prefix}/include

Name: OpenSSL-libcrypto
Description: OpenSSL cryptography library
Version: $OPENSSL_VERSION
Libs: -L\${libdir} -lcrypto
Cflags: -I\${includedir}
EOF

    echo "✓ OpenSSL installed"
}

# Build libnfc
build_libnfc() {
    echo "=== Building libnfc $LIBNFC_VERSION ==="

    wget -q "https://github.com/nfc-tools/libnfc/releases/download/libnfc-${LIBNFC_VERSION}/libnfc-${LIBNFC_VERSION}.tar.bz2"
    tar xjf "libnfc-${LIBNFC_VERSION}.tar.bz2"
    cd "libnfc-${LIBNFC_VERSION}"
    autoreconf -vis

    # Get libusb flags from pkg-config
    if pkg-config --exists libusb-1.0; then
        LIBUSB_CFLAGS=$(pkg-config --cflags libusb-1.0)
        LIBUSB_LIBS=$(pkg-config --libs libusb-1.0)
    else
        LIBUSB_CFLAGS="-I$BUILD_ROOT/include/libusb-1.0"
        LIBUSB_LIBS="-L$BUILD_ROOT/lib -lusb-1.0"
    fi

    local extra_cflags="-I$BUILD_ROOT/include $LIBUSB_CFLAGS"
    local extra_ldflags="-L$BUILD_ROOT/lib"
    local extra_libs="-lusb $LIBUSB_LIBS"

    # macOS: Add framework flags
    if [ "$TARGET_OS" = "darwin" ]; then
        extra_ldflags="$extra_ldflags -framework CoreFoundation -framework IOKit -framework Security"
    fi

    CFLAGS="$CFLAGS $extra_cflags" \
    LDFLAGS="$LDFLAGS $extra_ldflags" \
    LIBS="$extra_libs" \
    libusb_CFLAGS="$LIBUSB_CFLAGS" \
    libusb_LIBS="$LIBUSB_LIBS" \
        ./configure $HOST_FLAG \
        --prefix="$BUILD_ROOT" \
        --enable-static \
        --disable-shared \
        --with-drivers=pn53x_usb,acr122_usb

    make $MAKE_JOBS
    make install
    cd ..

    echo "✓ libnfc installed"
}

# Build libfreefare
build_libfreefare() {
    echo "=== Building libfreefare $LIBFREEFARE_VERSION ==="

    wget -q "https://github.com/nfc-tools/libfreefare/releases/download/libfreefare-${LIBFREEFARE_VERSION}/libfreefare-${LIBFREEFARE_VERSION}.tar.bz2"
    tar xjf "libfreefare-${LIBFREEFARE_VERSION}.tar.bz2"
    cd "libfreefare-${LIBFREEFARE_VERSION}"
    autoreconf -vis

    # Get crypto flags from pkg-config
    if pkg-config --exists libcrypto; then
        CRYPTO_CFLAGS=$(pkg-config --cflags libcrypto)
        CRYPTO_LIBS=$(pkg-config --libs libcrypto)
    else
        CRYPTO_CFLAGS="-I$BUILD_ROOT/include"
        CRYPTO_LIBS="-L$BUILD_ROOT/lib -lcrypto"
    fi

    # Platform-specific patches
    if [ "$TARGET_OS" = "darwin" ]; then
        # macOS: Create endian compatibility header
        cat > libfreefare/freefare_endian.h << 'EOF'
#ifndef FREEFARE_ENDIAN_H
#define FREEFARE_ENDIAN_H
#ifdef __APPLE__
#include <libkern/OSByteOrder.h>
#define htole16(x) OSSwapHostToLittleInt16(x)
#define htole32(x) OSSwapHostToLittleInt32(x)
#define le16toh(x) OSSwapLittleToHostInt16(x)
#define le32toh(x) OSSwapLittleToHostInt32(x)
#define htobe16(x) OSSwapHostToBigInt16(x)
#define be16toh(x) OSSwapBigToHostInt16(x)
#endif
#endif
EOF

        # Patch source files to include the compatibility header
        for file in libfreefare/mifare_classic.c libfreefare/mifare_desfire.c libfreefare/mifare_desfire_aid.c libfreefare/mifare_desfire_crypto.c libfreefare/tlv.c; do
            sed -i.bak '1i\
#include "freefare_endian.h"' "$file"
        done
    elif [ "$TARGET_OS" = "linux" ]; then
        # Linux: Add stdlib.h include
        sed -i '1i#include <stdlib.h>' libfreefare/mifare_desfire_crypto.c
    fi

    CFLAGS="$CFLAGS -I$BUILD_ROOT/include" \
    LDFLAGS="$LDFLAGS -L$BUILD_ROOT/lib" \
    CRYPTO_CFLAGS="$CRYPTO_CFLAGS" \
    CRYPTO_LIBS="$CRYPTO_LIBS" \
        ./configure $HOST_FLAG \
        --prefix="$BUILD_ROOT" \
        --enable-static \
        --disable-shared

    make $MAKE_JOBS
    make install
    cd ..

    echo "✓ libfreefare installed"
}

# Build Go binary
build_go_binary() {
    echo "=== Building Go binary ==="

    export CGO_ENABLED=1
    export GOOS="$TARGET_OS"
    export GOARCH="$TARGET_ARCH"
    export CGO_CFLAGS="-I$BUILD_ROOT/include"

    case "$TARGET_OS" in
        linux)
            export CGO_LDFLAGS="-L$BUILD_ROOT/lib -lcrypto -lusb-1.0 -static"
            GO_LDFLAGS="-linkmode external -extldflags '-static'"
            ;;
        darwin)
            export CGO_LDFLAGS="-L$BUILD_ROOT/lib -lcrypto -lusb-1.0 -framework IOKit -framework CoreFoundation -framework Security"
            GO_LDFLAGS=""
            ;;
    esac

    BINARY_NAME="davi-nfc-agent-$TARGET_OS-$TARGET_ARCH"

    echo "Building $BINARY_NAME..."
    if [ -n "$GO_LDFLAGS" ]; then
        go build -v -ldflags="$GO_LDFLAGS" -o "$BINARY_NAME" .
    else
        go build -v -o "$BINARY_NAME" .
    fi

    echo "✓ Binary built: $BINARY_NAME"
    ls -lh "$BINARY_NAME"
}

# Main build sequence
main() {
    # Save original directory
    ORIGINAL_DIR=$(pwd)

    # Create temp directory for builds
    WORK_DIR=$(mktemp -d)
    echo "Working in: $WORK_DIR"
    cd "$WORK_DIR"

    trap "cd '$ORIGINAL_DIR'; rm -rf $WORK_DIR" EXIT

    setup_compiler
    build_libusb
    build_libusb_compat
    build_openssl
    build_libnfc
    build_libfreefare

    # Return to original directory to build Go binary
    cd "$ORIGINAL_DIR"
    build_go_binary

    echo ""
    echo "=== Build complete ==="
    echo "Binary: $BINARY_NAME"
    echo "Dependencies: $BUILD_ROOT"
}

main
