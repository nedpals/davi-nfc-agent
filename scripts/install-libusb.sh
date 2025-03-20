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
        sudo apt-get install -y libusb-1.0-0-dev
    
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

else
    echo "Unsupported operating system: $OSTYPE"
    exit 1
fi

echo ""
echo "libusb installation complete"
echo "You can now run the configure script again"
