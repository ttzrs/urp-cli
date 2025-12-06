package container

import (
	"fmt"
	"os"
	"path/filepath"
)

// VolumeSpec defines a container volume mount with consistent handling
type VolumeSpec struct {
	Source   string // Host path or named volume
	Target   string // Container path
	ReadOnly bool   // Mount as read-only
}

// String returns the docker/podman -v argument format
func (v VolumeSpec) String() string {
	if v.ReadOnly {
		return fmt.Sprintf("%s:%s:ro", v.Source, v.Target)
	}
	return fmt.Sprintf("%s:%s:rw", v.Source, v.Target)
}

// StringWithSELinux returns the volume string with :Z label for Podman/SELinux
func (v VolumeSpec) StringWithSELinux() string {
	mode := "rw"
	if v.ReadOnly {
		mode = "ro"
	}
	return fmt.Sprintf("%s:%s:%s:Z", v.Source, v.Target, mode)
}

// StandardVolumes returns the common volume mounts for URP containers
func StandardVolumes(projectPath, projectName, envFile string, readOnly bool) []VolumeSpec {
	return []VolumeSpec{
		{Source: projectPath, Target: "/workspace", ReadOnly: readOnly},
		{Source: VectorVolume(projectName), Target: "/var/lib/urp/vector", ReadOnly: false},
		{Source: envFile, Target: "/etc/urp/.env", ReadOnly: true},
	}
}

// ContainerEnvPath is where .env is mounted inside URP containers
const ContainerEnvPath = "/etc/urp/.env"

// IsInsideContainer detects if we're running inside a URP container
func IsInsideContainer() bool {
	_, err := os.Stat("/.urp-container")
	return err == nil
}

// ResolveEnvFile finds the .env file path - container path if inside, host path if outside
func ResolveEnvFile() string {
	if IsInsideContainer() {
		return ContainerEnvPath
	}
	return filepath.Join(ResolveHomeDir(), ".urp-go", ".env")
}

// ResolveHostEnvFile returns the HOST path for .env (for docker mounts, not validation)
// Inside container: uses URP_HOST_HOME (set by master)
// Outside container: uses user home
func ResolveHostEnvFile() string {
	hostHome := os.Getenv("URP_HOST_HOME")
	if hostHome == "" {
		hostHome, _ = os.UserHomeDir()
	}
	return filepath.Join(hostHome, ".urp-go", ".env")
}

// ResolveEnvFileReal finds the .env file path with symlink resolution (for Silverblue)
func ResolveEnvFileReal() string {
	return filepath.Join(ResolveHomeDirReal(), ".urp-go", ".env")
}

// ResolveAlertsDir returns the alerts directory path with symlink resolution
func ResolveAlertsDir() string {
	return filepath.Join(ResolveHomeDirReal(), ".urp-go", "alerts")
}

// ResolveHomeDir returns URP_HOST_HOME or user home directory
func ResolveHomeDir() string {
	homeDir := os.Getenv("URP_HOST_HOME")
	if homeDir == "" {
		homeDir, _ = os.UserHomeDir()
	}
	return homeDir
}

// ResolveHomeDirReal returns home directory with symlinks resolved (for Silverblue /var/home)
func ResolveHomeDirReal() string {
	homeDir := ResolveHomeDir()
	if realHome, err := filepath.EvalSymlinks(homeDir); err == nil {
		return realHome
	}
	return homeDir
}
