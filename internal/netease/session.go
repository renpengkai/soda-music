package netease

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

// Session is what the application remembers about a NetEase login. The cookie
// is the credential; the rest is only there so the sidebar can greet the user
// without a round trip.
type Session struct {
	Cookie   string `json:"cookie"`
	UID      int64  `json:"uid"`
	Nickname string `json:"nickname"`
	Avatar   string `json:"avatar"`
}

// SignedIn reports whether the session carries a credential.
func (s Session) SignedIn() bool { return s.Cookie != "" }

// FileName is where the session lives inside the application's data directory.
const FileName = "online.json"

// LoadSession reads the saved session. A missing or unreadable file is not an
// error: it simply means nobody is signed in.
func LoadSession(dir string) Session {
	var session Session
	raw, err := os.ReadFile(filepath.Join(dir, FileName))
	if err != nil {
		return Session{}
	}
	if err := json.Unmarshal(raw, &session); err != nil {
		return Session{}
	}
	return session
}

// SaveSession writes the session with owner-only permissions, since the cookie
// is a credential.
func SaveSession(dir string, session Session) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(session, "", "  ")
	if err != nil {
		return err
	}
	path := filepath.Join(dir, FileName)
	temp := path + ".tmp"
	if err := os.WriteFile(temp, raw, 0o600); err != nil {
		return err
	}
	return os.Rename(temp, path)
}

// ClearSession removes the saved session.
func ClearSession(dir string) error {
	err := os.Remove(filepath.Join(dir, FileName))
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}
