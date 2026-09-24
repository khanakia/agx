//go:build unix

package sessions

import (
	"errors"
	"os"
	"syscall"
	"testing"
)

// A real cross-disk rename needs two filesystems, so the classification is
// tested with the exact error the OS returns (EXDEV inside a *LinkError).
func TestRenameError_CrossDevice(t *testing.T) {
	t.Parallel()
	exdev := &os.LinkError{Op: "rename", Old: "/a", New: "/b", Err: syscall.EXDEV}
	if err := renameError("/a", "/b", exdev); !errors.Is(err, ErrCrossDevice) {
		t.Errorf("EXDEV not classified: %v", err)
	}
	other := &os.LinkError{Op: "rename", Old: "/a", New: "/b", Err: syscall.EACCES}
	if err := renameError("/a", "/b", other); errors.Is(err, ErrCrossDevice) || !errors.Is(err, syscall.EACCES) {
		t.Errorf("EACCES misclassified or unwrapped: %v", err)
	}
}
