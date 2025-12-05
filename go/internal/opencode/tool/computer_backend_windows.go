//go:build windows

package tool

func detectBackend() computerBackend {
	return &windowsBackend{}
}
