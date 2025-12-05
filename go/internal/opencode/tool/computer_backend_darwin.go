//go:build darwin

package tool

import "os/exec"

func detectBackend() computerBackend {
	if _, err := exec.LookPath("cliclick"); err == nil {
		return &cliclickBackend{}
	}
	return nil
}
