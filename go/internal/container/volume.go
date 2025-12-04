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
func StandardVolumes(projectPath, envFile string, readOnly bool) []VolumeSpec {
	return []VolumeSpec{
		{Source: projectPath, Target: "/workspace", ReadOnly: readOnly},
		{Source: VectorVolume, Target: "/var/lib/urp/vector", ReadOnly: false},
		{Source: envFile, Target: "/etc/urp/.env", ReadOnly: true},
	}
}

// ResolveEnvFile finds the .env file path from URP_HOST_HOME or user home
func ResolveEnvFile() string {
	homeDir := os.Getenv("URP_HOST_HOME")
	if homeDir == "" {
		homeDir, _ = os.UserHomeDir()
	}
	return filepath.Join(homeDir, ".urp-go", ".env")
}

// ResolveHomeDir returns URP_HOST_HOME or user home directory
func ResolveHomeDir() string {
	homeDir := os.Getenv("URP_HOST_HOME")
	if homeDir == "" {
		homeDir, _ = os.UserHomeDir()
	}
	return homeDir
}
