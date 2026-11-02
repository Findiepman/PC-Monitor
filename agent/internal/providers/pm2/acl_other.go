//go:build !linux

package pm2

import "errors"

func allowUser(string, int) error {
	return errors.New("access lists for the control socket need Linux")
}
