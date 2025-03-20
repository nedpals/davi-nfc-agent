#!/bin/bash

set -e

echo "Installing and configuring libusb..."

# Detect OS
if [[ "$OSTYPE" == "linux-gnu"* ]]; then
    echo "Detected Linux system"
    
    # Check for apt (Debian/Ubuntu)
    if command -v apt-get &> /dev/null; then
        echo "Installing libusb using apt..."
        sudo apt-get update
        sudo apt-get install -y libusb-1.0-0-dev pkg-config
        
        # Verify installation and pkg-config setup
        echo "Verifying libusb installation..."
        if pkg-config --exists libusb-1.0; then
            echo "libusb-1.0 found by pkg-config"
            echo "CFLAGS: $(pkg-config --cflags libusb-1.0)"
            echo "LIBS: $(pkg-config --libs libusb-1.0)"
        else
            echo "WARNING: pkg-config cannot find libusb-1.0"
            echo "Creating manual pkg-config file..."
            
            # Create a custom .pc file if missing
            if [ ! -f /usr/lib/pkgconfig/libusb-1.0.pc ]; then
                sudo tee /usr/lib/pkgconfig/libusb-1.0.pc > /dev/null << 'EOF'
prefix=/usr
exec_prefix=${prefix}
libdir=${exec_prefix}/lib
includedir=${prefix}/include

Name: libusb-1.0
Description: C API for USB device access
Version: 1.0.24
Libs: -L${libdir} -lusb-1.0
Cflags: -I${includedir}/libusb-1.0
EOF
                echo "Created /usr/lib/pkgconfig/libusb-1.0.pc"
            fi
        fi
    
    # Check for yum (Red Hat/CentOS)
    elif command -v yum &> /dev/null; then
        echo "Installing libusb using yum..."
        sudo yum install -y libusbx-devel
    
    # Check for dnf (Fedora)
    elif command -v dnf &> /dev/null; then
        echo "Installing libusb using dnf..."
        sudo dnf install -y libusbx-devel
    
    # Check for pacman (Arch)
    elif command -v pacman &> /dev/null; then
        echo "Installing libusb using pacman..."
        sudo pacman -S libusb
    
    else
        echo "Could not detect package manager. Please install libusb manually."
        exit 1
    fi

elif [[ "$OSTYPE" == "darwin"* ]]; then
    echo "Detected macOS system"
    
    if ! command -v brew &> /dev/null; then
        echo "Homebrew not found. Please install Homebrew first: https://brew.sh"
        exit 1
    fi
    
    echo "Installing libusb using Homebrew..."
    brew install libusb
    
    # Get libusb path from Homebrew
    LIBUSB_PATH=$(brew --prefix libusb)
    echo "Setting up environment variables for macOS..."
    export PKG_CONFIG_PATH="$LIBUSB_PATH/lib/pkgconfig:$PKG_CONFIG_PATH"
    export CPPFLAGS="-I$LIBUSB_PATH/include $CPPFLAGS"
    export LDFLAGS="-L$LIBUSB_PATH/lib $LDFLAGS"
    
    # Print instructions for the user
    echo ""
    echo "For future builds, add these to your shell profile (~/.bash_profile, ~/.zshrc, etc.):"
    echo "export PKG_CONFIG_PATH=\"$LIBUSB_PATH/lib/pkgconfig:\$PKG_CONFIG_PATH\""
    echo "export CPPFLAGS=\"-I$LIBUSB_PATH/include \$CPPFLAGS\""
    echo "export LDFLAGS=\"-L$LIBUSB_PATH/lib \$LDFLAGS\""

elif [[ "$OSTYPE" == "msys"* ]] || [[ "$OSTYPE" == "cygwin"* ]]; then
    echo "Detected Windows system (MSYS2/Cygwin)"
    
    if command -v pacman &> /dev/null; then
        echo "Installing libusb using MSYS2 pacman..."
        pacman -S --noconfirm mingw-w64-x86_64-libusb
    else
        echo "MSYS2 pacman not found. Please install MSYS2 and run this script from an MSYS2 terminal."
        echo "Install instructions: https://www.msys2.org/"
        exit 1
    fi
    
    echo ""
    echo "For Windows native builds outside MSYS2, install Zadig to set up USB drivers:"
    echo "https://zadig.akeo.ie/"
    echo ""
    echo "After installing Zadig, connect your NFC reader and replace its driver with the WinUSB driver."

else
    echo "Unsupported operating system: $OSTYPE"
    echo "For Windows, please run this script from an MSYS2 terminal or install dependencies manually."
    exit 1
fi

echo ""
echo "libusb installation complete"
echo "You can now run the configure script again"
