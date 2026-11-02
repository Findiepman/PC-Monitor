package pm2

import (
	"encoding/binary"
	"syscall"
)

// allowUser gives one more user read/write on path with a POSIX ACL, the
// same as `setfacl -m u:<uid>:rw,g::-,o::- path`. The owner can do this
// without being in any group, so the helper needs no extra privileges.
func allowUser(path string, uid int) error {
	const (
		userObj  = 0x01
		user     = 0x02
		groupObj = 0x04
		mask     = 0x10
		other    = 0x20
		rw       = 6
		noID     = 0xffffffff
	)
	// linux/posix_acl_xattr.h: le32 version 2, then sorted {le16 tag, le16 perm, le32 id}.
	entries := []struct {
		tag, perm uint16
		id        uint32
	}{
		{userObj, rw, noID},
		{user, rw, uint32(uid)},
		{groupObj, 0, noID},
		{mask, rw, noID},
		{other, 0, noID},
	}
	buf := binary.LittleEndian.AppendUint32(nil, 2)
	for _, e := range entries {
		buf = binary.LittleEndian.AppendUint16(buf, e.tag)
		buf = binary.LittleEndian.AppendUint16(buf, e.perm)
		buf = binary.LittleEndian.AppendUint32(buf, e.id)
	}
	return syscall.Setxattr(path, "system.posix_acl_access", buf, 0)
}
