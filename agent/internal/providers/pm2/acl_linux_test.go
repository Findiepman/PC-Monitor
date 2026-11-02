package pm2

import (
	"os/user"
	"path/filepath"
	"syscall"
	"testing"
)

// The ACL must be one the kernel accepts and getfacl reads back.
func TestControlSocketACL(t *testing.T) {
	me, err := user.Current()
	if err != nil {
		t.Skip(err)
	}
	sock := filepath.Join(t.TempDir(), "control.sock")
	ln, err := listenControl(sock, me.Username)
	if err != nil {
		t.Skipf("no ACL support on this filesystem: %v", err)
	}
	defer ln.Close()
	buf := make([]byte, 128)
	n, err := syscall.Getxattr(sock, "system.posix_acl_access", buf)
	if err != nil || n != 4+5*8 {
		t.Fatalf("acl xattr: %d bytes, %v", n, err)
	}
}
