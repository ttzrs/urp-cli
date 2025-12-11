#!/bin/bash

# URP Enhanced Installation Script
# This script installs URP in the system with proper configuration

set -e  # Exit on any error

echo "URP Enhanced System Installation"
echo "================================="
echo ""

# Function to print colored output
print_success() {
    echo -e "\033[0;32m✓ $1\033[0m"
}

print_warning() {
    echo -e "\033[1;33m⚠️  $1\033[0m"
}

print_error() {
    echo -e "\033[0;31m❌ $1\033[0m"
}

print_info() {
    echo -e "\033[1;34mINFO: $1\033[0m"
}

# Check for required dependencies
check_dependencies() {
    echo "Checking dependencies..."
    
    if ! command -v docker &> /dev/null; then
        print_error "Docker is required but not installed. Please install Docker first."
        exit 1
    fi
    
    if ! command -v go &> /dev/null; then
        print_error "Go is required but not installed. Please install Go 1.24+ first."
        exit 1
    fi
    
    print_success "Dependencies check passed"
}

# Check dependencies
check_dependencies

# Determine installation prefix
if [ "$EUID" -eq 0 ]; then
    print_warning "Running as root. This is not recommended for URP."
    echo "It's better to install in user space. Continue? (y/N)"
    read -r response
    if [[ ! "$response" =~ ^([yY][eE][sS]|[yY])$ ]]; then
        echo "Installation cancelled."
        exit 1
    fi
    INSTALL_PREFIX="/usr/local"
    BIN_DIR="/usr/local/bin"
    ETC_DIR="/usr/local/etc"
    URPCONFIG_DIR="/etc/urp"
else
    print_info "Installing URP in user space..."
    INSTALL_PREFIX="$HOME/.local"
    BIN_DIR="$HOME/.local/bin"
    ETC_DIR="$HOME/.config"
    URPCONFIG_DIR="$HOME/.urp-go"
fi

# Create directories
echo ""
echo "Creating directories..."
mkdir -p "$BIN_DIR"
mkdir -p "$URPCONFIG_DIR"
mkdir -p "$ETC_DIR/urp"

# Build the binary if it doesn't exist
if [ ! -f "urp" ]; then
    print_info "Building URP binary from source..."
    if [ -d "go" ]; then
        cd go
        go build -o ../urp ./cmd/urp
        cd ..
        print_success "URP binary built successfully"
    else
        print_error "Go source not found and urp binary doesn't exist. Cannot build."
        exit 1
    fi
fi

# Copy the binary to the system
echo ""
echo "Installing URP binary..."
cp urp "$BIN_DIR/urp"
chmod +x "$BIN_DIR/urp"

# Verify the installation
if [ -f "$BIN_DIR/urp" ]; then
    print_success "URP binary installed successfully in $BIN_DIR"
else
    print_error "Failed to install URP binary"
    exit 1
fi

# Check if we need to add to PATH
if [[ ":$PATH:" != *":$BIN_DIR:"* ]]; then
    print_warning "$BIN_DIR is not in your PATH. Adding it to shell configuration..."

    # Detect shell and determine config file
    if [ -n "$BASH_VERSION" ] || [ -f "$HOME/.bashrc" ]; then
        SHELL_CONFIG="$HOME/.bashrc"
    elif [ -n "$ZSH_VERSION" ] || [ -f "$HOME/.zshrc" ]; then
        SHELL_CONFIG="$HOME/.zshrc"
    else
        SHELL_CONFIG="$HOME/.bashrc"  # Default fallback
    fi

    # Add the PATH export to shell config if not already there
    if ! grep -q "export PATH.*$BIN_DIR" "$SHELL_CONFIG" 2>/dev/null; then
        echo "" >> "$SHELL_CONFIG"
        echo "# Added by URP installer" >> "$SHELL_CONFIG"
        echo "export PATH=\"\$PATH:$BIN_DIR\"" >> "$SHELL_CONFIG"
        print_success "Added $BIN_DIR to PATH in $SHELL_CONFIG"
        print_info "Run 'source $SHELL_CONFIG' or restart your terminal to use URP immediately"
    else
        print_success "PATH already contains $BIN_DIR"
    fi
fi

# Create default configuration if it doesn't exist
if [ ! -f "$URPCONFIG_DIR/.env" ]; then
    echo ""
    echo "Creating default configuration..."
    
    # Create the enhanced .env configuration with new model variables
    cat > "$URPCONFIG_DIR/.env" << EOF
# URP Default Configuration
# This file contains default URP settings
# Copy this to ~/.urp-go/.env and customize for your needs

# --- API Keys ---
# ANTHROPIC_API_KEY="your-anthropic-key-here"
# OPENAI_API_KEY="your-openai-key-here"
# DEEPSEEK_API_KEY="your-deepseek-key-here"
# HF_TOKEN="your-huggingface-token-here"

# --- Primary Models (new configuration system) ---
URP_MASTER_MODEL_ID="anthropic/claude-sonnet-4-5-20250929"  # Primary reasoning model
URP_GATE_MODEL_ID="gpt-4o-mini"                           # Noise filtering model  
URP_WORKER_MODEL_ID="deepseek-chat"                       # Task execution model
URP_FALLBACK_MODEL_ID="gpt-4o-mini"                       # Fallback for primary model failures

# --- Specialized Models ---
URP_CODING_MODEL_ID="deepseek-coder"                      # For coding tasks
URP_REASONING_MODEL_ID="o1"                               # For complex reasoning
URP_FAST_MODEL_ID="gpt-4o-mini"                           # For quick responses
URP_VISION_MODEL_ID="gpt-4o"                              # For vision tasks
URP_LONG_CONTEXT_MODEL_ID="claude-opus-4-20250929"        # For long context tasks

# --- Provider URLs (optional - for custom endpoints) ---
# URP_MASTER_MODEL_URL="https://my-custom-provider.com/v1"
# URP_GATE_MODEL_URL="https://my-custom-provider.com/v1"
# URP_WORKER_MODEL_URL="https://my-custom-provider.com/v1"

# --- API Keys for Specific Models (optional) ---
# URP_MASTER_MODEL_API_KEY="sk-model-specific-key"
# URP_GATE_MODEL_API_KEY="sk-model-specific-key"
# URP_WORKER_MODEL_API_KEY="sk-model-specific-key"

# --- Default Fallback Models ---
URP_DEFAULT_MASTER_MODEL="anthropic/claude-sonnet-4-5-20250929"
URP_DEFAULT_GATE_MODEL="gpt-4o-mini"

# --- Infrastructure Settings ---
# NEO4J_URI="bolt://localhost:7687"
# URP_DEFAULT_OPENAI_BASE_URL="https://api.openai.com/v1"
EOF

    print_success "Default configuration created at $URPCONFIG_DIR/.env"
    print_info "To customize, edit: $URPCONFIG_DIR/.env"
fi

# Create data directories if they don't exist
mkdir -p "$URPCONFIG_DIR/data"
mkdir -p "$URPCONFIG_DIR/vectors"
mkdir -p "$URPCONFIG_DIR/backups"
mkdir -p "$URPCONFIG_DIR/alerts"
mkdir -p "$URPCONFIG_DIR/skills"

print_success "Configuration directories created in $URPCONFIG_DIR"

# Copy documentation if it exists
if [ -f "MODEL_CONFIGURATION.md" ]; then
    cp MODEL_CONFIGURATION.md "$URPCONFIG_DIR/MODEL_CONFIGURATION.md"
    print_success "Model configuration documentation copied"
fi

# Create a script to start infrastructure (for convenience)
cat > "$BIN_DIR/urp-start" << EOF
#!/bin/bash
# Convenience script to start URP infrastructure

# Load configuration if exists
if [ -f "$URPCONFIG_DIR/.env" ]; then
    export \$(grep -v '^#' "$URPCONFIG_DIR/.env" | xargs)
fi

echo "Starting URP infrastructure..."
urp infra start
echo "URP infrastructure started. You can now use URP commands."
EOF

chmod +x "$BIN_DIR/urp-start"
print_success "Convenience script created: $BIN_DIR/urp-start"

# Verify installation
echo ""
echo "Verifying installation..."
if "$BIN_DIR/urp" version &> /dev/null; then
    print_success "URP installation verified successfully"
    "$BIN_DIR/urp" version
else
    print_warning "URP binary exists but version check failed - this may be expected before infrastructure setup"
fi

# Check if docker is running and warn if not
if ! docker info &> /dev/null; then
    print_warning "Docker is installed but may not be running. Start Docker before using URP."
fi

echo ""
echo "🎉 URP Enhanced System Installation Complete!"
echo "============================================="
echo ""
echo "Installation details:"
echo "  Binary location: $BIN_DIR/urp"
echo "  Configuration: $URPCONFIG_DIR"
echo "  Convenience script: $BIN_DIR/urp-start"
echo ""
echo "To start using URP:"
echo "  1. Restart your terminal or run 'source $SHELL_CONFIG'"
echo "  2. Add your API keys to $URPCONFIG_DIR/.env"
echo "  3. Start infrastructure: urp infra start"
echo "  4. Or use convenience script: urp-start"
echo "  5. Test: urp doctor"
echo ""
echo "Model Configuration:"
echo "  - All new model configuration variables are supported"
echo "  - Specialized models for different tasks (coding, reasoning, etc.)"
echo "  - Custom provider URLs and API keys per model"
echo "  - Robust fallback mechanisms"
echo ""
echo "For more information: urp --help or see $URPCONFIG_DIR/MODEL_CONFIGURATION.md"
echo ""
print_info "For security reasons, ensure your API keys in $URPCONFIG_DIR/.env have restricted permissions:"
print_info "chmod 600 $URPCONFIG_DIR/.env"