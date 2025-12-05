//go:build !linux && !darwin && !windows

package tool

func detectBackend() computerBackend {
	return nil
}
