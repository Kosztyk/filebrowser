package fbhttp

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	bolt "go.etcd.io/bbolt"
	"net/http"
	"sort"
	"strings"
	"time"
)

type sessionRecord struct {
	ID         string    `json:"id"`
	Owner      uint      `json:"owner"`
	Created    time.Time `json:"created"`
	Seen       time.Time `json:"lastSeen"`
	Expires    time.Time `json:"expires"`
	Agent      string    `json:"agent"`
	Revoked    bool      `json:"revoked"`
	Credential string    `json:"-"`
}
type storedSession struct {
	sessionRecord
	Credential string
}

func credentialHash(p string) string { b := sha256.Sum256([]byte(p)); return hex.EncodeToString(b[:]) }
func sessionKey(tk *authToken, r *http.Request) string {
	if tk.ID != "" {
		return tk.ID
	}
	raw, _ := (extractor{}).ExtractToken(r)
	return "legacy-" + credentialHash(raw)
}
func sessionValid(r *http.Request, d *data, tk *authToken) bool {
	if d.features == nil {
		return true
	}
	id := sessionKey(tk, r)
	valid := false
	e := d.features.m.journal.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("sessions-v1"))
		v := b.Get([]byte(id))
		var s storedSession
		if v == nil {
			if tk.ID != "" {
				return nil
			}
			s.sessionRecord = sessionRecord{ID: id, Owner: d.user.ID, Created: time.Now(), Seen: time.Now(), Expires: tk.ExpiresAt.Time, Agent: boundedAgent(r.UserAgent())}
			s.Credential = credentialHash(d.user.Password)
		} else if e := json.Unmarshal(v, &s); e != nil {
			return e
		}
		if s.Revoked || s.Owner != d.user.ID || time.Now().After(s.Expires) || s.Credential != credentialHash(d.user.Password) {
			return nil
		}
		valid = true
		d.sessionID = id
		if time.Since(s.Seen) > time.Minute || v == nil {
			s.Seen = time.Now()
			v, e := json.Marshal(s)
			if e != nil {
				return e
			}
			return b.Put([]byte(id), v)
		}
		return nil
	})
	return e == nil && valid
}
func boundedAgent(s string) string {
	if len(s) > 200 {
		return s[:200]
	}
	return s
}
func mintSession(r *http.Request, d *data, owner uint, password string) (string, error) {
	if d.features == nil {
		return "", nil
	}
	if d.sessionID != "" {
		return d.sessionID, nil
	}
	id, e := randomUploadID()
	if e != nil {
		return "", e
	}
	s := storedSession{sessionRecord: sessionRecord{ID: id, Owner: owner, Created: time.Now(), Seen: time.Now(), Expires: time.Now().Add(30 * 24 * time.Hour), Agent: boundedAgent(r.UserAgent())}, Credential: credentialHash(password)}
	e = d.features.m.journal.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("sessions-v1"))
		var live []storedSession
		c := b.Cursor()
		count := 0
		for k, v := c.First(); k != nil; k, v = c.Next() {
			var old storedSession
			if e := json.Unmarshal(v, &old); e != nil {
				return e
			}
			if time.Now().After(old.Expires) || (old.Revoked && !strings.HasPrefix(old.ID, "legacy-")) {
				if e := c.Delete(); e != nil {
					return e
				}
				continue
			}
			count++
			if old.Owner == owner && !old.Revoked {
				live = append(live, old)
			}
		}
		if count >= 10000 {
			return errors.New("session capacity reached; revoke unused sessions")
		}
		sort.Slice(live, func(i, j int) bool { return live[i].Seen.Before(live[j].Seen) })
		for len(live) >= 50 {
			old := live[0]
			live = live[1:]
			old.Revoked = true
			v, e := json.Marshal(old)
			if e != nil {
				return e
			}
			if e = b.Put([]byte(old.ID), v); e != nil {
				return e
			}
		}

		v, e := json.Marshal(s)
		if e != nil {
			return e
		}
		return b.Put([]byte(id), v)
	})
	return id, e
}

var sessionsHandler = withUser(func(w http.ResponseWriter, r *http.Request, d *data) (int, error) {
	if d.features == nil {
		return http.StatusServiceUnavailable, nil
	}
	out := []map[string]any{}
	target := r.URL.Query().Get("id")
	if target == "current" {
		target = d.sessionID
	}
	e := d.features.m.journal.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("sessions-v1"))
		return b.ForEach(func(k, v []byte) error {
			var s storedSession
			if e := json.Unmarshal(v, &s); e != nil {
				return e
			}
			if s.Owner != d.user.ID && !d.user.Perm.Admin {
				return nil
			}
			if r.Method == http.MethodDelete && (target == string(k) || target == "all" && s.Owner == d.user.ID) {
				s.Revoked = true
				v, e := json.Marshal(s)
				if e != nil {
					return e
				}
				return b.Put(k, v)
			}
			if !s.Revoked && time.Now().Before(s.Expires) {
				out = append(out, map[string]any{"id": s.ID, "owner": s.Owner, "created": s.Created, "lastSeen": s.Seen, "expires": s.Expires, "agent": s.Agent, "current": s.ID == d.sessionID})
			}
			return nil
		})
	})
	if e != nil {
		return http.StatusInternalServerError, e
	}
	return renderJSON(w, r, out)
})

// Session expiration bounds renewal even if a client remains open indefinitely.
func sessionExpiry(d *data, fallback time.Time) time.Time {
	if d.features == nil || d.sessionID == "" {
		return fallback
	}
	_ = d.features.m.journal.View(func(tx *bolt.Tx) error {
		var s storedSession
		if json.Unmarshal(tx.Bucket([]byte("sessions-v1")).Get([]byte(d.sessionID)), &s) == nil && s.Expires.Before(fallback) {
			fallback = s.Expires
		}
		return nil
	})
	return fallback
}
