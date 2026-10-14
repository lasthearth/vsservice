//go:build !unix

package service

// freeBytes is unknown off unix (a developer's Windows machine): the build is
// then guarded by the size cap alone.
func freeBytes(string) (int64, bool) {
	return 0, false
}
