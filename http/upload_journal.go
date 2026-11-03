package fbhttp

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/filebrowser/filebrowser/v2/files"
	"github.com/filebrowser/filebrowser/v2/runner"
	"github.com/filebrowser/filebrowser/v2/settings"
	"github.com/filebrowser/filebrowser/v2/storage"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/spf13/afero"
	bolt "go.etcd.io/bbolt"
	"io"
	"log"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type UploadInfo struct {
	ID        string     `json:"id"`
	Path      string     `json:"path"`
	Name      string     `json:"name"`
	Length    int64      `json:"length"`
	Offset    int64      `json:"offset"`
	Status    string     `json:"status"`
	Scan      ScanResult `json:"scan"`
	Message   string     `json:"message,omitempty"`
	SHA256    string     `json:"sha256,omitempty"`
	Modified  string     `json:"modified,omitempty"`
	CreatedAt time.Time  `json:"createdAt"`
	UpdatedAt time.Time  `json:"updatedAt"`
	ExpiresAt time.Time  `json:"expiresAt"`
	CanRetry  bool       `json:"canRetry"`
}
type uploadRecord struct {
	FollowExternal bool
	UploadInfo
	OwnerID                             uint
	Scope, Stage, Token, ExpectedSHA256 string
	Before                              files.FileVersion
	HashState                           []byte
	FileMode                            os.FileMode
}
type uploadEntry struct {
	mu     sync.Mutex
	record uploadRecord
	cancel context.CancelFunc
}
type UploadManager struct {
	mu, admit            sync.Mutex
	entries              map[string]*uploadEntry
	journal              *bolt.DB
	instance             string
	store                *storage.Storage
	server               *settings.Server
	ctx                  context.Context
	cancel               context.CancelFunc
	workers              sync.WaitGroup
	wake                 chan struct{}
	closeOnce, startOnce sync.Once
}

var uploadBucket = []byte("uploads-v1")
var ErrUploadExpired = errors.New("upload has expired")
var ErrUploadState = errors.New("upload is not accepting this operation")
var ErrUploadLength = errors.New("upload exceeds its declared length")
var ErrUploadStorage = errors.New("insufficient space or unfinished-upload quota exceeded")

func randomUploadID() (string, error) {
	b := make([]byte, 16)
	_, e := rand.Read(b)
	return hex.EncodeToString(b), e
}
func validUploadID(id string) bool {
	if len(id) != 32 {
		return false
	}
	_, e := hex.DecodeString(id)
	return e == nil && id == strings.ToLower(id)
}
func sessionHasStaging(status string) bool {
	return status != "ready" && status != "cancelled" && status != "expired"
}
func safeStagingRecord(r uploadRecord) bool {
	return !files.IsPrivatePath(r.Path) && r.Path == filepath.Clean(r.Path) && strings.HasPrefix(r.Path, "/") && r.Stage == filepath.Join(filepath.Dir(r.Path), files.StagingDirectory, r.Token+".part") && !strings.ContainsAny(r.Token, "/\\.")
}
func recoveryFs(r uploadRecord) afero.Fs {
	return files.InternalFs(files.NewFs(afero.NewOsFs(), r.Scope, r.FollowExternal))
}
func (m *UploadManager) staged(r uploadRecord, fs afero.Fs) *files.StagedFile {
	return &files.StagedFile{Fs: files.InternalFs(fs), Path: r.Stage, Destination: r.Path, Token: r.Token, Before: r.Before, FileMode: r.FileMode}
}
func NewUploadManager(path string, store *storage.Storage, server *settings.Server, start bool) (*UploadManager, error) {
	db, e := bolt.Open(path, 0600, &bolt.Options{Timeout: time.Second})
	if e != nil {
		return nil, e
	}
	if e = os.Chmod(path, 0600); e != nil {
		db.Close()
		return nil, e
	}
	ctx, cancel := context.WithCancel(context.Background())
	m := &UploadManager{entries: map[string]*uploadEntry{}, journal: db, store: store, server: server, ctx: ctx, cancel: cancel, wake: make(chan struct{}, 8)}
	e = db.Update(func(tx *bolt.Tx) error {
		b, e := tx.CreateBucketIfNotExists([]byte("metadata"))
		if e != nil {
			return e
		}
		m.instance = string(b.Get([]byte("instance")))
		if m.instance == "" {
			m.instance, e = randomUploadID()
			if e != nil {
				return e
			}
			if e = b.Put([]byte("instance"), []byte(m.instance)); e != nil {
				return e
			}
		}
		_, e = tx.CreateBucketIfNotExists(uploadBucket)
		return e
	})
	if e == nil {
		e = db.View(func(tx *bolt.Tx) error {
			return tx.Bucket(uploadBucket).ForEach(func(k, v []byte) error {
				var r uploadRecord
				if e := json.Unmarshal(v, &r); e != nil {
					return e
				}
				if !validUploadID(r.ID) || r.ID != string(k) || r.Token != m.instance+"-"+r.ID || !filepath.IsAbs(r.Scope) || !safeStagingRecord(r) || r.Offset < 0 || r.Length < 0 || r.Offset > r.Length {
					return fmt.Errorf("invalid upload journal record")
				}
				m.entries[r.ID] = &uploadEntry{record: r}
				return nil
			})
		})
	}
	if e != nil {
		cancel()
		db.Close()
		return nil, e
	}
	for _, entry := range m.snapshot() {
		entry.mu.Lock()
		if e = m.recoverEntry(entry); e != nil {
			r := entry.record
			r.Status = "failed"
			r.CanRetry = false
			r.Message = "Upload recovery needs attention; existing destination was preserved"
			log.Printf("upload recovery %s: %v", r.ID, e)
			e = m.save(entry, r)
		}
		entry.mu.Unlock()
		if e != nil {
			m.Close()
			return nil, e
		}
	}
	if start {
		m.Start()
	}
	return m, nil
}
func (m *UploadManager) snapshot() []*uploadEntry {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]*uploadEntry, 0, len(m.entries))
	for _, e := range m.entries {
		out = append(out, e)
	}
	return out
}
func (m *UploadManager) entry(id string) (*uploadEntry, error) {
	if !validUploadID(id) {
		return nil, os.ErrNotExist
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	e := m.entries[id]
	if e == nil {
		return nil, os.ErrNotExist
	}
	return e, nil
}
func (m *UploadManager) save(entry *uploadEntry, r uploadRecord) error {
	v, e := json.Marshal(r)
	if e != nil {
		return e
	}
	if e = m.journal.Update(func(tx *bolt.Tx) error { return tx.Bucket(uploadBucket).Put([]byte(r.ID), v) }); e != nil {
		return e
	}
	entry.record = r
	return nil
}
func (m *UploadManager) remove(entry *uploadEntry) error {
	if e := m.journal.Update(func(tx *bolt.Tx) error { return tx.Bucket(uploadBucket).Delete([]byte(entry.record.ID)) }); e != nil {
		return e
	}
	m.mu.Lock()
	delete(m.entries, entry.record.ID)
	m.mu.Unlock()
	entry.record.Status = "expired"
	return nil
}
func (m *UploadManager) notify() {
	select {
	case m.wake <- struct{}{}:
	default:
	}
}
func (m *UploadManager) Close() {
	m.closeOnce.Do(func() {
		m.cancel()
		m.workers.Wait()
		for _, e := range m.snapshot() {
			e.mu.Lock()
			files.ReleaseWrite(recoveryFs(e.record), e.record.Path, e.record.Token)
			e.mu.Unlock()
		}
		_ = m.journal.Close()
	})
}
func (m *UploadManager) dataFor(r uploadRecord) (*data, error) {
	set, e := m.store.Settings.Get()
	if e != nil {
		return nil, e
	}
	u, e := m.store.Users.Get(m.server.Root, m.server.FollowExternalSymlinks, r.OwnerID)
	if e != nil {
		return nil, e
	}
	base := files.BasePath(u.Fs)
	if base == nil || filepath.Clean(afero.FullBaseFsPath(base, "/")) != r.Scope {
		return nil, fmt.Errorf("upload owner's scope changed")
	}
	return &data{Runner: &runner.Runner{Enabled: m.server.EnableExec, Settings: set}, settings: set, server: m.server, store: m.store, user: u}, nil
}
func (m *UploadManager) access(e *uploadEntry, d *data, write bool) error {
	if e.record.OwnerID != d.user.ID && (write || !d.user.Perm.Admin) {
		return os.ErrNotExist
	}
	if !write {
		return nil
	}
	base := files.BasePath(d.user.Fs)
	if !d.user.Perm.Create || !d.Check(e.record.Path) || base == nil || filepath.Clean(afero.FullBaseFsPath(base, "/")) != e.record.Scope || (e.record.Before.Exists && !d.user.Perm.Modify) {
		return os.ErrPermission
	}
	return nil
}
func (m *UploadManager) recoverCommit(e *uploadEntry, fs afero.Fs) (bool, error) {
	r := e.record
	if _, err := fs.Stat(r.Stage); !os.IsNotExist(err) {
		return false, err
	}
	f, err := fs.Open(r.Path)
	if err != nil {
		return false, err
	}
	defer f.Close()
	i, err := f.Stat()
	if err != nil {
		return false, err
	}
	if i.Size() != r.Length {
		return false, nil
	}
	hash := sha256.New()
	if _, err = io.Copy(hash, f); err != nil {
		return false, err
	}
	if hex.EncodeToString(hash.Sum(nil)) != r.SHA256 {
		return false, nil
	}
	r.Status = "ready"
	r.CanRetry = false
	r.Message = "Publication recovered after restart"
	r.UpdatedAt = time.Now()
	if err = m.save(e, r); err != nil {
		return false, err
	}
	files.ReleaseWrite(fs, r.Path, r.Token)
	return true, nil
}
func (m *UploadManager) recoverEntry(e *uploadEntry) error {
	r := e.record
	fs := recoveryFs(r)
	if r.Status == "cancelled" {
		err := fs.Remove(r.Stage)
		if os.IsNotExist(err) {
			err = nil
		}
		return err
	}
	if !sessionHasStaging(r.Status) {
		return nil
	}
	if err := files.ReserveWrite(fs, r.Path, r.Token); err != nil {
		return err
	}
	if r.Status == "committing" {
		if ok, err := m.recoverCommit(e, fs); ok || err != nil {
			return err
		}
		r = e.record
	}
	f, err := fs.OpenFile(r.Stage, os.O_RDWR, 0600)
	if err != nil {
		files.ReleaseWrite(fs, r.Path, r.Token)
		return err
	}
	defer f.Close()
	i, err := f.Stat()
	if err != nil {
		return err
	}
	if i.Size() < r.Offset {
		return fmt.Errorf("stage is shorter than acknowledged offset")
	}
	if i.Size() > r.Offset {
		if err = f.Truncate(r.Offset); err != nil {
			return err
		}
		if err = f.Sync(); err != nil {
			return err
		}
	}
	hash := sha256.New()
	if err = hash.(encoding.BinaryUnmarshaler).UnmarshalBinary(r.HashState); err != nil {
		return err
	}
	if r.Status == "scanning" || r.Status == "committing" {
		r.Status = "received"
		r.Scan = ScanResult{Status: "pending", Message: "Processing resumes after restart"}
		return m.save(e, r)
	}
	return nil
}
func (m *UploadManager) capacity(fs afero.Fs, dest string, required int64, reserve uint64) error {
	if required < 0 {
		return ErrUploadStorage
	}
	base := files.BasePath(fs)
	if base == nil {
		return nil
	}
	usage, e := disk.Usage(afero.FullBaseFsPath(base, filepath.Dir(dest)))
	if e != nil {
		return e
	}
	if uint64(required) > usage.Free || reserve > usage.Free-uint64(required) {
		return ErrUploadStorage
	}
	return nil
}
func (m *UploadManager) Create(d *data, path string, length int64, overwrite bool, modified, expectedSHA string) (UploadInfo, error) {
	if length < 0 || length > math.MaxInt64-1 {
		return UploadInfo{}, ErrUploadLength
	}
	expectedSHA = strings.ToLower(expectedSHA)
	if expectedSHA != "" {
		b, e := hex.DecodeString(expectedSHA)
		if e != nil || len(b) != 32 {
			return UploadInfo{}, fmt.Errorf("invalid expected SHA-256")
		}
	}
	path = filepath.Clean("/" + path)
	if path == "/" || !d.user.Perm.Create || !d.Check(path) {
		return UploadInfo{}, os.ErrPermission
	}
	before, e := files.Version(d.user.Fs, path)
	if e != nil {
		return UploadInfo{}, e
	}
	if before.Exists && (!overwrite || !d.user.Perm.Modify) {
		return UploadInfo{}, files.ErrWriteConflict
	}
	base := files.BasePath(d.user.Fs)
	if base == nil {
		return UploadInfo{}, fmt.Errorf("durable upload requires filesystem scope")
	}
	m.admit.Lock()
	defer m.admit.Unlock()
	settings.NormalizeTransfers(&d.settings.Tus, &d.settings.ClamAV)
	count := 0
	var pending, volumeRemaining int64
	parent := filepath.Dir(path)
	target, err := d.user.Fs.Stat(parent)
	for os.IsNotExist(err) && parent != "/" {
		parent = filepath.Dir(parent)
		target, err = d.user.Fs.Stat(parent)
	}
	entries := m.snapshot()
	if len(entries) >= 10000 {
		return UploadInfo{}, fmt.Errorf("clear completed upload history before creating more sessions")
	}
	for _, entry := range entries {
		entry.mu.Lock()
		r := entry.record
		if sessionHasStaging(r.Status) {
			count++
			if r.Length > math.MaxInt64-pending {
				entry.mu.Unlock()
				return UploadInfo{}, ErrUploadStorage
			}
			pending += r.Length
			if i, err := recoveryFs(r).Stat(filepath.Dir(r.Stage)); err == nil && target != nil && files.DeviceID(i) == files.DeviceID(target) {
				if r.Length-r.Offset > math.MaxInt64-volumeRemaining {
					entry.mu.Unlock()
					return UploadInfo{}, ErrUploadStorage
				}
				volumeRemaining += r.Length - r.Offset
			}
		}
		entry.mu.Unlock()
	}
	if count >= d.settings.Tus.MaxActiveUploads || (d.settings.Tus.MaxPendingBytes > 0 && (length > d.settings.Tus.MaxPendingBytes || pending > d.settings.Tus.MaxPendingBytes-length)) || volumeRemaining > math.MaxInt64-length {
		return UploadInfo{}, ErrUploadStorage
	}
	id, e := randomUploadID()
	if e != nil {
		return UploadInfo{}, e
	}
	token := m.instance + "-" + id
	s, f, e := files.NewStagedFile(d.user.Fs, path, token, d.settings.FileMode, d.settings.DirMode)
	if e != nil {
		return UploadInfo{}, e
	}
	abort := func(err error) (UploadInfo, error) { _ = f.Close(); _ = s.Abort(); return UploadInfo{}, err }
	if s.Before != before {
		return abort(files.ErrWriteConflict)
	}
	if e = m.capacity(d.user.Fs, path, length+volumeRemaining, d.settings.Tus.ReserveBytes); e != nil {
		return abort(e)
	}
	if e = f.Sync(); e != nil {
		return abort(e)
	}
	if e = f.Close(); e != nil {
		return abort(e)
	}
	now := time.Now()
	r := uploadRecord{UploadInfo: UploadInfo{ID: id, Path: path, Name: filepath.Base(path), Length: length, Status: "uploading", Scan: ScanResult{Status: "pending"}, Modified: modified, CreatedAt: now, UpdatedAt: now, ExpiresAt: now.Add(time.Duration(d.settings.Tus.RetentionHours) * time.Hour)}, FollowExternal: d.server.FollowExternalSymlinks, OwnerID: d.user.ID, Scope: filepath.Clean(afero.FullBaseFsPath(base, "/")), Stage: s.Path, Token: token, Before: before, FileMode: d.settings.FileMode, ExpectedSHA256: expectedSHA}
	hash := sha256.New()
	r.HashState, _ = hash.(encoding.BinaryMarshaler).MarshalBinary()
	if length == 0 {
		r.Status = "received"
		r.SHA256 = hex.EncodeToString(hash.Sum(nil))
		if expectedSHA != "" && expectedSHA != r.SHA256 {
			return abort(fmt.Errorf("upload SHA-256 mismatch"))
		}
	}
	entry := &uploadEntry{}
	if e = m.save(entry, r); e != nil {
		return abort(e)
	}
	m.mu.Lock()
	m.entries[id] = entry
	m.mu.Unlock()
	m.notify()
	return r.UploadInfo, nil
}
func (m *UploadManager) Info(id string, d *data) (UploadInfo, error) {
	e, err := m.entry(id)
	if err != nil {
		return UploadInfo{}, err
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if err = m.access(e, d, false); err != nil {
		return UploadInfo{}, err
	}
	if e.record.Status == "committing" {
		_, _ = m.recoverCommit(e, recoveryFs(e.record))
	}
	return e.record.UploadInfo, nil
}
func (m *UploadManager) List(d *data) []UploadInfo {
	out := []UploadInfo{}
	for _, e := range m.snapshot() {
		e.mu.Lock()
		if e.record.OwnerID == d.user.ID {
			out = append(out, e.record.UploadInfo)
		}
		e.mu.Unlock()
	}
	return out
}
func (m *UploadManager) Patch(id string, d *data, offset int64, body io.Reader, bodyLength int64) (UploadInfo, error) {
	e, err := m.entry(id)
	if err != nil {
		return UploadInfo{}, err
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if err = m.access(e, d, true); err != nil {
		return UploadInfo{}, err
	}
	r := e.record
	if time.Now().After(r.ExpiresAt) {
		return r.UploadInfo, ErrUploadExpired
	}
	if r.Status != "uploading" && r.Status != "paused" {
		return r.UploadInfo, ErrUploadState
	}
	if offset != r.Offset {
		return r.UploadInfo, files.ErrWriteConflict
	}
	remaining := r.Length - offset
	if remaining < 0 || bodyLength > remaining {
		return r.UploadInfo, ErrUploadLength
	}
	needed := remaining
	if bodyLength >= 0 {
		needed = bodyLength
	}
	if err = m.capacity(d.user.Fs, r.Path, needed, d.settings.Tus.ReserveBytes); err != nil {
		return r.UploadInfo, err
	}
	f, err := files.InternalFs(d.user.Fs).OpenFile(r.Stage, os.O_RDWR, 0600)
	if err != nil {
		return r.UploadInfo, err
	}
	defer f.Close()
	i, err := f.Stat()
	if err != nil {
		return r.UploadInfo, err
	}
	if i.Size() < offset {
		return r.UploadInfo, io.ErrUnexpectedEOF
	}
	if i.Size() > offset {
		if err = f.Truncate(offset); err != nil {
			return r.UploadInfo, err
		}
	}
	if _, err = f.Seek(offset, io.SeekStart); err != nil {
		return r.UploadInfo, err
	}
	hash := sha256.New()
	if err = hash.(encoding.BinaryUnmarshaler).UnmarshalBinary(r.HashState); err != nil {
		return r.UploadInfo, err
	}
	rollback := func(err error) (UploadInfo, error) { _ = f.Truncate(offset); _ = f.Sync(); return r.UploadInfo, err }
	n, err := io.Copy(io.MultiWriter(f, hash), io.LimitReader(body, remaining+1))
	if err != nil {
		return rollback(err)
	}
	if n > remaining {
		return rollback(ErrUploadLength)
	}
	if bodyLength >= 0 && n != bodyLength {
		return rollback(io.ErrUnexpectedEOF)
	}
	if err = f.Sync(); err != nil {
		return rollback(err)
	}
	if err = f.Close(); err != nil {
		return r.UploadInfo, err
	}
	r.Offset += n
	r.UpdatedAt = time.Now()
	r.ExpiresAt = r.UpdatedAt.Add(time.Duration(d.settings.Tus.RetentionHours) * time.Hour)
	r.HashState, err = hash.(encoding.BinaryMarshaler).MarshalBinary()
	if err != nil {
		return e.record.UploadInfo, err
	}
	r.Status = "uploading"
	if r.Offset == r.Length {
		r.SHA256 = hex.EncodeToString(hash.Sum(nil))
		r.Status = "received"
		r.Scan = ScanResult{Status: "pending"}
		if r.ExpectedSHA256 != "" && r.SHA256 != r.ExpectedSHA256 {
			r.Status = "failed"
			r.CanRetry = false
			r.Message = "SHA-256 mismatch; destination was preserved"
		}
	}
	if err = m.save(e, r); err != nil {
		return e.record.UploadInfo, err
	}
	m.notify()
	if r.Status == "failed" {
		return r.UploadInfo, fmt.Errorf("upload SHA-256 mismatch")
	}
	return r.UploadInfo, nil
}
func (m *UploadManager) Pause(id string, d *data, resume bool) (UploadInfo, error) {
	e, err := m.entry(id)
	if err != nil {
		return UploadInfo{}, err
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if err = m.access(e, d, true); err != nil {
		return UploadInfo{}, err
	}
	r := e.record
	if time.Now().After(r.ExpiresAt) {
		return r.UploadInfo, ErrUploadExpired
	}
	if r.Status != "uploading" && r.Status != "paused" {
		return r.UploadInfo, ErrUploadState
	}
	r.Status = "paused"
	if resume {
		r.Status = "uploading"
	}
	r.UpdatedAt = time.Now()
	r.ExpiresAt = r.UpdatedAt.Add(time.Duration(d.settings.Tus.RetentionHours) * time.Hour)
	return r.UploadInfo, m.save(e, r)
}
func (m *UploadManager) Retry(id string, d *data) (UploadInfo, error) {
	e, err := m.entry(id)
	if err != nil {
		return UploadInfo{}, err
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if err = m.access(e, d, true); err != nil {
		return UploadInfo{}, err
	}
	r := e.record
	if !r.CanRetry || r.Status != "failed" || r.Offset != r.Length {
		return r.UploadInfo, ErrUploadState
	}
	if time.Now().After(r.ExpiresAt) {
		return r.UploadInfo, ErrUploadExpired
	}
	r.Status = "received"
	r.Scan = ScanResult{Status: "pending"}
	r.CanRetry = false
	r.Message = ""
	r.UpdatedAt = time.Now()
	err = m.save(e, r)
	m.notify()
	return r.UploadInfo, err
}
func (m *UploadManager) Cancel(id string, d *data) error {
	e, err := m.entry(id)
	if err != nil {
		return err
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if err = m.access(e, d, false); err != nil {
		return err
	}
	if e.record.Status == "committing" {
		_, err = m.recoverCommit(e, recoveryFs(e.record))
		if err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	r := e.record
	if r.Status == "ready" {
		return ErrUploadState
	}
	if !safeStagingRecord(r) {
		return os.ErrPermission
	}
	if e.cancel != nil {
		e.cancel()
	}
	r.Status = "cancelled"
	r.CanRetry = false
	r.Message = "Upload cancelled; existing destination was preserved"
	if err = m.save(e, r); err != nil {
		return err
	}
	fs := recoveryFs(r)
	defer files.ReleaseWrite(fs, r.Path, r.Token)
	err = fs.Remove(r.Stage)
	if os.IsNotExist(err) {
		err = nil
	}
	return err
}
func (m *UploadManager) Dismiss(id string, d *data) error {
	e, err := m.entry(id)
	if err != nil {
		return err
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if err = m.access(e, d, false); err != nil {
		return err
	}
	if sessionHasStaging(e.record.Status) {
		return ErrUploadState
	}
	if e.record.Status == "cancelled" {
		if err = recoveryFs(e.record).Remove(e.record.Stage); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return m.remove(e)
}
