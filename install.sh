#!/bin/bash

# URP Installation Script
# This script installs URP in the system with proper configuration

set -e  # Exit on any error

echo "URP System Installation"
echo "======================="

# Check if running as root or with sudo
if [ "$EUID" -eq 0 ]; then
    echo "⚠️  Running as root. This is not recommended for URP."
    echo "It's better to install in user space. Continue? (y/N)"
    read -r response
    if [[ ! "$response" =~ ^([yY][eE][sS]|[yY])$ ]]; then
        echo "Installation cancelled."
        exit 1
    fi
    INSTALL_PREFIX="/usr/local"
    BIN_DIR="/usr/local/bin"
    ETC_DIR="/usr/local/etc"
else
    echo "Installing URP in user space..."
    INSTALL_PREFIX="$HOME/.local"
    BIN_DIR="$HOME/.local/bin"
    ETC_DIR="$HOME/.config"
fi

# Create directories
echo "Creating directories..."
mkdir -p "$BIN_DIR"
mkdir -p "$ETC_DIR/urp"

# Copy the binary to the system
echo "Installing URP binary..."
cp urp "$BIN_DIR/urp"
chmod +x "$BIN_DIR/urp"

# Verify the installation
if [ -f "$BIN_DIR/urp" ]; then
    echo "✅ URP binary installed successfully in $BIN_DIR"
else
    echo "❌ Failed to install URP binary"
    exit 1
fi

# Check if we need to add to PATH
if [[ ":$PATH:" != *":$BIN_DIR:"* ]]; then
    echo "⚠️  $BIN_DIR is not in your PATH. Adding it to shell configuration..."

    # Detect shell
    if [ -n "$BASH_VERSION" ]; then
        SHELL_CONFIG="$HOME/.bashrc"
    elif [ -n "$ZSH_VERSION" ]; then
        SHELL_CONFIG="$HOME/.zshrc"
    else
        SHELL_CONFIG="$HOME/.bashrc"  # Default fallback
    fi

    # Add the PATH export to shell config if not already there
    if ! grep -q "export PATH.*$BIN_DIR" "$SHELL_CONFIG" 2>/dev/null; then
        echo "" >> "$SHELL_CONFIG"
        echo "# Added by URP installer" >> "$SHELL_CONFIG"
        echo "export PATH=\"\$PATH:$BIN_DIR\"" >> "$SHELL_CONFIG"
        echo "✅ Added $BIN_DIR to PATH in $SHELL_CONFIG"
        echo "Run 'source $SHELL_CONFIG' or restart your terminal to use URP immediately"
    else
        echo "✓ PATH already contains $BIN_DIR"
    fi
fi

# Copy configuration files if they exist
if [ -d "$HOME/.urp" ]; then
    echo "Copying URP configuration..."
    cp -r "$HOME/.urp" "$ETC_DIR/"
    echo "✅ URP configuration copied to $ETC_DIR/urp"
fi

if [ -d "$HOME/.urp-go" ]; then
    echo "Copying URP-Go configuration..."
    cp -r "$HOME/.urp-go" "$ETC_DIR/"
    echo "✅ URP-Go configuration copied to $ETC_DIR/urp-go"
fi

# Create a systemd service for infrastructure if running as root (optional)
if [ "$EUID" -eq 0 ]; then
    echo "Creating systemd service for URP infrastructure (optional)..."
    
    cat > /etc/systemd/system/urp-infra.service << EOF
[Unit]
Description=URP Infrastructure Services
After=docker.service
Requires=docker.service

[Service]
Type=oneshot
RemainAfterExit=yes
ExecStart=/usr/local/bin/urp infra start
ExecStop=/usr/local/bin/urp infra stop

[Install]
WantedBy=multi-user.target
EOF
    
    systemctl daemon-reload
    echo "✅ URP infrastructure systemd service created"
fi

# Verify installation
echo ""
echo "Verifying installation..."
"$BIN_DIR/urp" doctor || echo "Note: Doctor command may fail if infrastructure is not running"

echo ""
echo "🎉 URP successfully installed!"
echo ""
echo "Installation details:"
echo "  Binary location: $BIN_DIR/urp"
echo "  Configuration: $ETC_DIR/urp and $ETC_DIR/urp-go"
echo ""
echo "To start using URP:"
echo "  1. Restart your terminal or run 'source $SHELL_CONFIG'"
echo "  2. Start infrastructure: urp infra start"
echo "  3. Test: urp ask \"Hello, are you working?\""
echo ""
echo "For more information: urp --help"