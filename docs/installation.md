# Installation

This guide covers installing the NFC Agent and its dependencies.

## Requirements

- libnfc (v1.8.0 or later)
- libfreefare (v0.4.0 or later)
- libusb (v1.0 or later)

## Quick Install

### Pre-built Binaries

Pre-built binaries for various platforms are available in the [releases](https://github.com/dotside-studios/davi-nfc-agent/releases) section.

### Building from Source

```bash
git clone https://github.com/dotside-studios/davi-nfc-agent.git
cd davi-nfc-agent
go build -o davi-nfc-agent .
```

## Installing Dependencies

### Linux (Debian/Ubuntu)

For Debian-based distributions, install dependencies from the package manager:

```bash
sudo apt update
sudo apt install -y libnfc-dev libfreefare-dev libusb-1.0-0-dev
```

This is the preferred method as it ensures compatibility with your system's libraries.

### Building Dependencies from Source

If your package manager has outdated versions or you need specific features:

```bash
# Download and build libnfc
wget https://github.com/nfc-tools/libnfc/releases/download/libnfc-1.8.0/libnfc-1.8.0.tar.bz2
tar xjf libnfc-1.8.0.tar.bz2
cd libnfc-1.8.0
autoreconf -vis
./configure --prefix=/usr --sysconfdir=/etc
make
sudo make install
cd ..

# Download and build libfreefare
wget https://github.com/nfc-tools/libfreefare/releases/download/libfreefare-0.4.0/libfreefare-0.4.0.tar.bz2
tar xjf libfreefare-0.4.0.tar.bz2
cd libfreefare-0.4.0
autoreconf -vis
./configure --prefix=/usr --sysconfdir=/etc
make
sudo make install
cd ..
```

### macOS

```bash
brew install libnfc libfreefare libusb
```

### Windows

Install the dependencies using the following steps:

```bash
# Install required tools using Chocolatey
choco install mingw
choco install msys2

# Launch MSYS2 and install dependencies
pacman -S mingw-w64-x86_64-toolchain autoconf automake libtool mingw-w64-x86_64-libusb
```

### Other Systems

The project includes a helper script to install libusb:

```bash
chmod +x scripts/install-libusb.sh
./scripts/install-libusb.sh
```

This script automatically detects your operating system and installs libusb using the appropriate package manager.

## Troubleshooting

### "No NFC devices found"

- Ensure your NFC reader is properly connected
- Check that libnfc is installed and configured correctly
- Verify user permissions for accessing USB/serial devices

On Linux, you may need to add udev rules:

```bash
# Create udev rule for NFC readers
sudo tee /etc/udev/rules.d/99-nfc.rules << 'EOF'
SUBSYSTEM=="usb", ATTR{idVendor}=="072f", MODE="0666"
SUBSYSTEM=="usb", ATTR{idVendor}=="04e6", MODE="0666"
EOF

sudo udevadm control --reload-rules
sudo udevadm trigger
```

### "Failed to connect to device"

- Try unplugging and reconnecting your NFC reader
- Restart the application
- Check for conflicting applications using the NFC reader

### Permission Denied

On Linux, add your user to the appropriate group:

```bash
sudo usermod -aG plugdev $USER
# Log out and back in for changes to take effect
```
