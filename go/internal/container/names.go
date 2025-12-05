// Package container naming conventions.
// Centralizes container name construction to eliminate scattered fmt.Sprintf calls.
package container

import "fmt"

// Prefix is the standard container name prefix.
const Prefix = "urp"

// MasterName returns the master container name for a project.
// Pattern: urp-master-{project}
func MasterName(project string) string {
	return fmt.Sprintf("%s-master-%s", Prefix, project)
}

// WorkerName returns the worker container name for a project.
// Pattern: urp-{project}-w{num}
func WorkerName(project string, num int) string {
	return fmt.Sprintf("%s-%s-w%d", Prefix, project, num)
}

// NeMoName returns the NeMo container name for a project.
// Pattern: urp-nemo-{project}
func NeMoName(project string) string {
	return fmt.Sprintf("%s-nemo-%s", Prefix, project)
}

// ProjectName returns a simple project container name.
// Pattern: urp-{project}
func ProjectName(project string) string {
	return fmt.Sprintf("%s-%s", Prefix, project)
}

// IsMasterContainer checks if a container name is a master container.
func IsMasterContainer(name string) bool {
	return len(name) > 11 && name[:11] == "urp-master-"
}

// IsWorkerContainer checks if a container name is a worker container.
func IsWorkerContainer(name string) bool {
	// Workers have pattern urp-{project}-w{N}
	for i := len(name) - 1; i >= 0; i-- {
		if name[i] == 'w' && i > 0 && name[i-1] == '-' {
			return true
		}
		if name[i] < '0' || name[i] > '9' {
			break
		}
	}
	return false
}

// IsNeMoContainer checks if a container name is a NeMo container.
func IsNeMoContainer(name string) bool {
	return len(name) > 9 && name[:9] == "urp-nemo-"
}
