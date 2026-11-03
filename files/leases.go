package files

import (
	"github.com/spf13/afero"
	"time"
)

// Office leases share the same mutex as publication reservations. A locked
// ancestor/descendant cannot be deleted, renamed or written through another path.
type Lease struct {
	Value   string
	Expires time.Time
	Before  FileVersion
}

var leases = map[string]Lease{}

func WithLease(fs afero.Fs, value string) afero.Fs {
	if s, ok := fs.(*ScopedFs); ok {
		c := *s
		c.lease = value
		return &c
	}
	return fs
}
func leaseValue(fs afero.Fs) string {
	if s, ok := fs.(*ScopedFs); ok {
		return s.lease
	}
	return ""
}
func checkLease(fs afero.Fs, name string) error {
	key := writeKey(fs, name)
	for p, l := range leases {
		if time.Now().After(l.Expires) {
			delete(leases, p)
			continue
		}
		if overlap(p, key) && l.Value != leaseValue(fs) {
			return ErrWriteConflict
		}
	}
	return nil
}
func SetLease(fs afero.Fs, name string, l Lease) error {
	writes.Lock()
	defer writes.Unlock()
	key := writeKey(fs, name)
	for p := range writes.paths {
		if overlap(p, key) {
			return ErrWriteConflict
		}
	}
	if e := checkLease(WithLease(fs, l.Value), name); e != nil {
		return e
	}
	leases[key] = l
	return nil
}
func ClearLease(fs afero.Fs, name string) {
	writes.Lock()
	defer writes.Unlock()
	delete(leases, writeKey(fs, name))
}
func GetLease(fs afero.Fs, name string) (Lease, bool) {
	writes.Lock()
	defer writes.Unlock()
	key := writeKey(fs, name)
	l, ok := leases[key]
	if ok && time.Now().After(l.Expires) {
		delete(leases, key)
		ok = false
	}
	return l, ok
}
