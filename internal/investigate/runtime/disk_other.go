//go:build !unix

package runtime

import "errors"

// freeDisk is not implemented here: the validator refuses to create a
// cluster when it cannot measure free disk.
func freeDisk(string) (uint64, error) {
	return 0, errors.New("free disk measurement is not supported on this platform")
}
