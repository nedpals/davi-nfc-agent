#!/bin/bash
set -e

# Build script for Windows (cross-compile from Linux)
# Usage: ./scripts/build-windows.sh [arch]
#   arch: amd64 (arm64 not supported yet)

# Library versions
LIBNFC_VERSION="${LIBNFC_VERSION:-1.8.0}"
LIBFREEFARE_VERSION="${LIBFREEFARE_VERSION:-0.4.0}"
LIBUSB_VERSION="${LIBUSB_VERSION:-1.0.27}"
LIBUSB_COMPAT_VERSION="${LIBUSB_COMPAT_VERSION:-0.1.8}"
OPENSSL_VERSION="${OPENSSL_VERSION:-1.1.1w}"

TARGET_ARCH="${1:-amd64}"

if [ "$TARGET_ARCH" != "amd64" ]; then
    echo "Error: Only amd64 is supported for Windows builds"
    exit 1
fi

echo "=== Building for windows-$TARGET_ARCH ==="

# Setup build directory
BUILD_ROOT="$HOME/cross-build/windows-$TARGET_ARCH"
mkdir -p "$BUILD_ROOT"
export PKG_CONFIG_PATH="$BUILD_ROOT/lib/pkgconfig"

echo "Build root: $BUILD_ROOT"

# Setup compiler
ZIG_TARGET="x86_64-windows-gnu"
export CC="zig cc -target $ZIG_TARGET"
export CXX="zig c++ -target $ZIG_TARGET"
export AR="zig ar"
export RANLIB="zig ranlib"
HOST_FLAG="--host=x86_64-w64-mingw32"
MAKE_JOBS="-j$(nproc 2>/dev/null || sysctl -n hw.ncpu 2>/dev/null || echo 4)"

# Download prebuilt libusb for Windows
build_libusb() {
    echo "=== Setting up libusb $LIBUSB_VERSION ==="

    wget -q "https://github.com/libusb/libusb/releases/download/v${LIBUSB_VERSION}/libusb-${LIBUSB_VERSION}.7z"
    7z x -y "libusb-${LIBUSB_VERSION}.7z" > /dev/null

    mkdir -p "$BUILD_ROOT/include/libusb-1.0"
    mkdir -p "$BUILD_ROOT/lib"

    # Copy header
    cp include/libusb.h "$BUILD_ROOT/include/libusb-1.0/"

    # Copy static library
    if [ -f "MinGW64/static/libusb-1.0.a" ]; then
        cp MinGW64/static/libusb-1.0.a "$BUILD_ROOT/lib/"
    elif [ -f "MS64/static/libusb-1.0.a" ]; then
        cp MS64/static/libusb-1.0.a "$BUILD_ROOT/lib/"
    else
        echo "Error: libusb-1.0.a not found"
        exit 1
    fi

    # Create pkg-config file
    mkdir -p "$BUILD_ROOT/lib/pkgconfig"
    cat > "$BUILD_ROOT/lib/pkgconfig/libusb-1.0.pc" << EOF
prefix=$BUILD_ROOT
exec_prefix=\${prefix}
libdir=\${exec_prefix}/lib
includedir=\${prefix}/include

Name: libusb-1.0
Description: C API for USB device access
Version: $LIBUSB_VERSION
Libs: -L\${libdir} -lusb-1.0
Cflags: -I\${includedir}/libusb-1.0
EOF

    echo "✓ libusb installed"
}

# Build libusb-compat
build_libusb_compat() {
    echo "=== Building libusb-compat $LIBUSB_COMPAT_VERSION ==="

    wget -q "https://github.com/libusb/libusb-compat-0.1/releases/download/v${LIBUSB_COMPAT_VERSION}/libusb-compat-${LIBUSB_COMPAT_VERSION}.tar.bz2"
    tar xjf "libusb-compat-${LIBUSB_COMPAT_VERSION}.tar.bz2"
    cd "libusb-compat-${LIBUSB_COMPAT_VERSION}"

    ./configure $HOST_FLAG \
        --prefix="$BUILD_ROOT" \
        --enable-static \
        --disable-shared

    make $MAKE_JOBS
    make install
    cd ..

    # Create lusb0_usb.h symlink (Windows libusb-compat compatibility)
    ln -sf "$BUILD_ROOT/include/usb.h" "$BUILD_ROOT/include/lusb0_usb.h" 2>/dev/null || true

    echo "✓ libusb-compat installed"
}

# Build OpenSSL
build_openssl() {
    echo "=== Building OpenSSL $OPENSSL_VERSION ==="

    wget -q "https://www.openssl.org/source/openssl-${OPENSSL_VERSION}.tar.gz"
    tar xzf "openssl-${OPENSSL_VERSION}.tar.gz"
    cd "openssl-${OPENSSL_VERSION}"

    ./Configure mingw64 \
        --prefix="$BUILD_ROOT" \
        --openssldir="$BUILD_ROOT/ssl" \
        no-shared \
        no-tests

    # Build only libraries (skip tools that need resource compiler)
    make $MAKE_JOBS build_libs
    make install_dev
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
Libs: -L\${libdir} -lcrypto -lws2_32 -lgdi32 -ladvapi32 -lcrypt32 -luser32
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

    # Get libusb flags
    if pkg-config --exists libusb-1.0; then
        LIBUSB_CFLAGS=$(pkg-config --cflags libusb-1.0)
        LIBUSB_LIBS=$(pkg-config --libs libusb-1.0)
    else
        LIBUSB_CFLAGS="-I$BUILD_ROOT/include/libusb-1.0"
        LIBUSB_LIBS="-L$BUILD_ROOT/lib -lusb-1.0"
    fi

    # Patch for Windows compatibility (setenv/unsetenv)
    sed -i '1i#ifdef _WIN32\n#include "contrib/windows.h"\n#endif' libnfc/nfc.c
    sed -i '1i#ifdef _WIN32\n#include "contrib/windows.h"\n#endif' libnfc/log.c

    CFLAGS="-I$BUILD_ROOT/include -I$(pwd)/contrib/win32 -DHAVE_WINSCARD_H -DLIBNFC_STATIC $LIBUSB_CFLAGS" \
    CPPFLAGS="-DLIBNFC_STATIC" \
    LDFLAGS="-L$BUILD_ROOT/lib" \
    LIBS="-lusb $LIBUSB_LIBS -lws2_32" \
    libusb_CFLAGS="$LIBUSB_CFLAGS" \
    libusb_LIBS="$LIBUSB_LIBS" \
        ./configure $HOST_FLAG \
        --prefix="$BUILD_ROOT" \
        --enable-static \
        --disable-shared \
        --with-drivers=pn53x_usb,acr122_usb

    # Build only the library (skip examples/utils that may have issues)
    make $MAKE_JOBS -C libnfc
    make -C libnfc install
    make -C include/nfc install
    make install-pkgconfigDATA

    # Patch installed nfc.h for static linking
    sed -i '/^[[:space:]]*#[[:space:]]*define[[:space:]]*NFC_EXPORT/d' "$BUILD_ROOT/include/nfc/nfc.h"
    echo -e '#ifndef NFC_EXPORT\n#define NFC_EXPORT\n#endif\n' | cat - "$BUILD_ROOT/include/nfc/nfc.h" > /tmp/nfc.h
    mv /tmp/nfc.h "$BUILD_ROOT/include/nfc/nfc.h"

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

    # Get crypto flags
    if pkg-config --exists libcrypto; then
        CRYPTO_CFLAGS=$(pkg-config --cflags libcrypto)
        CRYPTO_LIBS=$(pkg-config --libs libcrypto)
    else
        CRYPTO_CFLAGS="-I$BUILD_ROOT/include"
        CRYPTO_LIBS="-L$BUILD_ROOT/lib -lcrypto -lws2_32 -lgdi32 -ladvapi32 -lcrypt32 -luser32"
    fi

    # Create Windows endian.h compatibility header
    cat > "$BUILD_ROOT/include/endian.h" << 'EOF'
#ifndef _ENDIAN_H_COMPAT
#define _ENDIAN_H_COMPAT
#include <stdint.h>
// Windows x86/x64 is always little-endian
#define htole16(x) (x)
#define htole32(x) (x)
#define le16toh(x) (x)
#define le32toh(x) (x)
static inline uint16_t htobe16(uint16_t x) {
    return ((x & 0x00FF) << 8) | ((x & 0xFF00) >> 8);
}
static inline uint16_t be16toh(uint16_t x) {
    return htobe16(x);
}
#endif
EOF

    # Create Windows err.h compatibility header
    cat > "$BUILD_ROOT/include/err.h" << 'EOF'
#ifndef _ERR_H_COMPAT
#define _ERR_H_COMPAT
#include <stdio.h>
#include <stdlib.h>
#include <stdarg.h>
#include <errno.h>
#include <string.h>

static inline void err(int eval, const char *fmt, ...) {
    va_list ap;
    va_start(ap, fmt);
    if (fmt) {
        vfprintf(stderr, fmt, ap);
        fprintf(stderr, ": %s\n", strerror(errno));
    }
    va_end(ap);
    exit(eval);
}

static inline void errx(int eval, const char *fmt, ...) {
    va_list ap;
    va_start(ap, fmt);
    if (fmt) vfprintf(stderr, fmt, ap);
    fprintf(stderr, "\n");
    va_end(ap);
    exit(eval);
}

static inline void warn(const char *fmt, ...) {
    va_list ap;
    va_start(ap, fmt);
    if (fmt) {
        vfprintf(stderr, fmt, ap);
        fprintf(stderr, ": %s\n", strerror(errno));
    }
    va_end(ap);
}

static inline void warnx(const char *fmt, ...) {
    va_list ap;
    va_start(ap, fmt);
    if (fmt) vfprintf(stderr, fmt, ap);
    fprintf(stderr, "\n");
    va_end(ap);
}
#endif
EOF

    # Get libnfc flags
    if pkg-config --exists libnfc; then
        LIBNFC_CFLAGS=$(pkg-config --cflags libnfc)
        LIBNFC_LIBS=$(pkg-config --libs libnfc)
    else
        LIBNFC_CFLAGS="-I$BUILD_ROOT/include/nfc"
        LIBNFC_LIBS="-L$BUILD_ROOT/lib -lnfc"
    fi

    CFLAGS="-I$BUILD_ROOT/include -I$BUILD_ROOT/include/nfc -DLIBNFC_STATIC" \
    CPPFLAGS="-DLIBNFC_STATIC" \
    LDFLAGS="-L$BUILD_ROOT/lib -static" \
    CRYPTO_CFLAGS="$CRYPTO_CFLAGS" \
    CRYPTO_LIBS="-L$BUILD_ROOT/lib $BUILD_ROOT/lib/libcrypto.a -lws2_32 -lgdi32 -ladvapi32 -lcrypt32 -luser32" \
    libnfc_CFLAGS="$LIBNFC_CFLAGS" \
    libnfc_LIBS="$LIBNFC_LIBS" \
    ac_cv_lib_crypto_DES_ecb_encrypt=yes \
        ./configure $HOST_FLAG \
        --prefix="$BUILD_ROOT" \
        --enable-static \
        --disable-shared

    # Build only the library
    make $MAKE_JOBS -C libfreefare CFLAGS="-I$BUILD_ROOT/include -I$BUILD_ROOT/include/nfc -DLIBNFC_STATIC"
    make -C libfreefare install
    make install-pkgconfigDATA
    cd ..

    echo "✓ libfreefare installed"
}

# Build Go binary
build_go_binary() {
    echo "=== Building Go binary ==="

    export CGO_ENABLED=1
    export GOOS=windows
    export GOARCH="$TARGET_ARCH"
    export CGO_CFLAGS="-I$BUILD_ROOT/include -DLIBNFC_STATIC -DNFC_EXPORT="
    export CGO_LDFLAGS="-L$BUILD_ROOT/lib -lnfc -lfreefare -lcrypto -lusb-1.0 -lws2_32 -lgdi32 -ladvapi32 -lcrypt32 -luser32 -static"

    BINARY_NAME="davi-nfc-agent-windows-$TARGET_ARCH.exe"

    echo "Building $BINARY_NAME..."
    go build -v -o "$BINARY_NAME" .

    echo "✓ Binary built: $BINARY_NAME"
    ls -lh "$BINARY_NAME"
}

# Main build sequence
main() {
    # Create temp directory for builds
    WORK_DIR=$(mktemp -d)
    echo "Working in: $WORK_DIR"
    cd "$WORK_DIR"

    trap "cd - > /dev/null; rm -rf $WORK_DIR" EXIT

    build_libusb
    build_libusb_compat
    build_openssl
    build_libnfc
    build_libfreefare

    # Return to original directory to build Go binary
    cd - > /dev/null
    build_go_binary

    echo ""
    echo "=== Build complete ==="
    echo "Binary: $BINARY_NAME"
    echo "Dependencies: $BUILD_ROOT"
}

main
