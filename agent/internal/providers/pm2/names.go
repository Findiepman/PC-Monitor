package pm2

import (
	"bytes"
	"io"
	"os"
	"regexp"

	"github.com/findiepman/dashd/internal/provider"
)

// Bots print who they logged in as when they connect. These are the two
// shapes seen on the server; a bot that prints neither just has no Discord
// name on the page.
var (
	// discord.js examples: "Ready! Logged in as AftermathCoordinator#3579".
	// Bot usernames still use the old system, so they can contain spaces when
	// a #1234 discriminator ends them; try that form first.
	// No commas, quotes or an earlier '#' inside, and the tag must end the
	// name, so "Logged in as bot, watching #1234 channels" isn't one name.
	loggedInTagRE = regexp.MustCompile(`[Ll]ogged in as ([^\s'",#][^'",#]{0,62}?#\d{4})(?:$|[\s.!,;:)])`)
	loggedInRE    = regexp.MustCompile(`[Ll]ogged in as ([^\s'",]+)`)
	// Fin & Co (presence.py): "Manager is signed in as 'Sigmabot1' and can see 1 server(s)"
	signedInRE = regexp.MustCompile(`(\S+) is signed in as '([^']+)'`)
)

// logTail is how much of the end of a log is searched. Bots log their login
// once per start, so it stays near the end unless the bot is very chatty.
const logTail = 256 << 10

// DiscordNames finds the latest login lines in a log. self is the name a
// single-bot process logged in as; logins maps account to Discord name for
// processes that sign in several bots.
func DiscordNames(log []byte) (self string, logins map[string]string) {
	for _, line := range bytes.Split(log, []byte("\n")) {
		if !bytes.Contains(line, []byte(" in as ")) {
			continue
		}
		text := provider.CleanLine(string(line))
		if m := signedInRE.FindStringSubmatch(text); m != nil {
			acct, name := clean(m[1], 64), clean(m[2], 64)
			if acct != "" && name != "" {
				if logins == nil {
					logins = map[string]string{}
				}
				if _, seen := logins[acct]; seen || len(logins) < maxAccounts {
					logins[acct] = name
				}
			}
			continue
		}
		if m := loggedInTagRE.FindStringSubmatch(text); m != nil {
			self = clean(m[1], 64)
		} else if m := loggedInRE.FindStringSubmatch(text); m != nil {
			self = clean(m[1], 64)
		}
	}
	return self, logins
}

// readTail returns up to n bytes from the end of a regular file, starting at
// a line boundary.
func readTail(path string, n int64) ([]byte, os.FileInfo, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return nil, nil, err
	}
	if !st.Mode().IsRegular() {
		return nil, st, nil
	}
	off := max(0, st.Size()-n)
	buf := make([]byte, st.Size()-off)
	if _, err := f.ReadAt(buf, off); err != nil && err != io.EOF {
		return nil, st, err
	}
	if off > 0 {
		if i := bytes.IndexByte(buf, '\n'); i >= 0 {
			buf = buf[i+1:]
		}
	}
	return buf, st, nil
}
