package fbhttp

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/filebrowser/filebrowser/v2/files"
	"github.com/filebrowser/filebrowser/v2/settings"
	"github.com/filebrowser/filebrowser/v2/users"
	"github.com/spf13/afero"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"
)

func uploadFixture(t *testing.T) (*UploadManager, *data, string, string) {
	t.Helper()
	scope := t.TempDir()
	st := scopedUserStorage(t, scope, users.Permissions{Create: true, Modify: true}, []byte("key"))
	set, e := st.Settings.Get()
	if e != nil {
		t.Fatal(e)
	}
	set.FileMode = 0644
	set.DirMode = 0755
	if e = st.Settings.Save(set); e != nil {
		t.Fatal(e)
	}
	u, e := st.Users.Get("", false, uint(1))
	if e != nil {
		t.Fatal(e)
	}
	journal := filepath.Join(t.TempDir(), "uploads.db")
	server := &settings.Server{}
	m, e := NewUploadManager(journal, st, server, false)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(m.Close)
	return m, &data{settings: set, server: server, store: st, user: u}, scope, journal
}
func startUpload(t *testing.T, m *UploadManager, d *data, name string, n int64, overwrite bool) UploadInfo {
	t.Helper()
	i, e := m.Create(d, name, n, overwrite, "123", "")
	if e != nil {
		t.Fatal(e)
	}
	return i
}
func patchUpload(t *testing.T, m *UploadManager, d *data, i UploadInfo, offset int64, body string) UploadInfo {
	t.Helper()
	v, e := m.Patch(i.ID, d, offset, strings.NewReader(body), int64(len(body)))
	if e != nil {
		t.Fatal(e)
	}
	return v
}
func fixtureFile(t *testing.T, scope, name, content string) {
	t.Helper()
	if e := os.WriteFile(filepath.Join(scope, name), []byte(content), 0640); e != nil {
		t.Fatal(e)
	}
}
func expectContent(t *testing.T, scope, name, want string) {
	t.Helper()
	b, e := os.ReadFile(filepath.Join(scope, name))
	if e != nil || string(b) != want {
		t.Fatal(name, string(b), e)
	}
}

type interruptedChunk struct{ sent bool }

func (r *interruptedChunk) Read(p []byte) (int, error) {
	if r.sent {
		return 0, io.ErrUnexpectedEOF
	}
	r.sent = true
	return copy(p, "bad"), nil
}
func TestDurableUploadRestartReconcilesTailAndChecksum(t *testing.T) {
	m, d, scope, journal := uploadFixture(t)
	fixtureFile(t, scope, "file", "old")
	i := startUpload(t, m, d, "/file", 11, true)
	i = patchUpload(t, m, d, i, 0, "hello ")
	if _, e := m.Patch(i.ID, d, 6, &interruptedChunk{}, -1); e == nil {
		t.Fatal("interrupted chunk accepted")
	}
	entry, _ := m.entry(i.ID)
	stage := entry.record.Stage
	f, e := files.InternalFs(d.user.Fs).OpenFile(stage, os.O_WRONLY|os.O_APPEND, 0600)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = f.WriteString("unacknowledged tail"); e != nil {
		t.Fatal(e)
	}
	_ = f.Close()
	m.Close()
	next, e := NewUploadManager(journal, d.store, d.server, false)
	if e != nil {
		t.Fatal(e)
	}
	defer next.Close()
	i, e = next.Info(i.ID, d)
	if e != nil || i.Offset != 6 {
		t.Fatal(i, e)
	}
	fi, e := files.InternalFs(d.user.Fs).Stat(stage)
	if e != nil || fi.Size() != 6 {
		t.Fatal("tail not reconciled", e)
	}
	expectContent(t, scope, "file", "old")
	i = patchUpload(t, next, d, i, 6, "world")
	if i.Status != "received" {
		t.Fatal(i)
	}
	expectContent(t, scope, "file", "old")
	next.processOne()
	i, e = next.Info(i.ID, d)
	sum := sha256.Sum256([]byte("hello world"))
	if e != nil || i.Status != "ready" || i.SHA256 != hex.EncodeToString(sum[:]) || i.Scan.Status != "disabled" {
		t.Fatal(i, e)
	}
	expectContent(t, scope, "file", "hello world")
}
func TestUploadOwnershipCancellationAndExpiration(t *testing.T) {
	m, d, scope, _ := uploadFixture(t)
	fixtureFile(t, scope, "file", "old")
	i := startUpload(t, m, d, "/file", 3, true)
	other := *d
	u := *d.user
	u.ID = 2
	other.user = &u
	if _, e := m.Info(i.ID, &other); !os.IsNotExist(e) {
		t.Fatal("foreign status exposed", e)
	}
	if e := m.Cancel(i.ID, &other); !os.IsNotExist(e) {
		t.Fatal("foreign cancellation allowed", e)
	}
	if _, e := m.Patch(i.ID, &other, 0, strings.NewReader("bad"), 3); !os.IsNotExist(e) {
		t.Fatal("foreign append allowed", e)
	}
	u.Perm.Admin = true
	if _, e := m.Info(i.ID, &other); e != nil {
		t.Fatal(e)
	}
	d.user.Perm.Create = false
	d.user.Perm.Delete = false
	if e := m.Cancel(i.ID, d); e != nil {
		t.Fatal(e)
	}
	if e := m.Dismiss(i.ID, d); e != nil {
		t.Fatal(e)
	}
	expectContent(t, scope, "file", "old")
	d.user.Perm.Create = true
	i = startUpload(t, m, d, "/file", 3, true)
	if _, e := writeFile(d.user.Fs, "/file", strings.NewReader("bad"), 0644, 0755); !errors.Is(e, files.ErrWriteConflict) {
		t.Fatal("save bypasses reservation", e)
	}
	entry, _ := m.entry(i.ID)
	r := entry.record
	r.ExpiresAt = time.Now().Add(-time.Hour)
	if e := m.save(entry, r); e != nil {
		t.Fatal(e)
	}
	m.expire()
	expectContent(t, scope, "file", "old")
	if e := files.CheckMutation(d.user.Fs, "/file"); e != nil {
		t.Fatal(e)
	}
}
func TestInvalidUploadCreationDoesNotTruncate(t *testing.T) {
	m, d, scope, _ := uploadFixture(t)
	fixtureFile(t, scope, "file", "old")
	handler := handle(tusPostHandler(m), "", d.store, d.server)
	token := signToken(t, d.user.Perm, []byte("key"))
	for _, n := range []string{"", "-1", "bad", "9223372036854775808"} {
		r := httptest.NewRequest("POST", "/file?override=true", http.NoBody)
		r.Header.Set("X-Auth", token)
		r.Header.Set("Upload-Length", n)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != 400 {
			t.Fatal(n, w.Code)
		}
		expectContent(t, scope, "file", "old")
	}
	if len(m.List(d)) != 0 {
		t.Fatal("invalid creation allocated a session")
	}
}
func TestScannerRequiresExplicitCleanEvidence(t *testing.T) {
	cases := []struct{ body, status string }{{`{"success":true,"data":[{"infected":false,"viruses":[]}]}`, "clean"}, {`{"status":"success","results":[{"status":"OK","errors":[]}]}`, "clean"}, {`{"success":true}`, "unknown"}, {`{}`, "unknown"}, {``, "unknown"}, {`<html>proxy</html>`, "unknown"}, {`{"files":[]}`, "unknown"}, {`{"infected":true,"virus":"Test"}`, "infected"}, {"first: Test FOUND\nsecond: OK", "infected"}, {`{"infected":false,"error":"cannot open"}`, "unknown"}, {`{"success":false,"error":"timeout"}`, "error"}, {`file: OK`, "clean"}}
	for _, tt := range cases {
		t.Run(tt.body, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = io.Copy(io.Discard, r.Body)
				_, _ = io.WriteString(w, tt.body)
			}))
			defer srv.Close()
			m, d, _, _ := uploadFixture(t)
			i := startUpload(t, m, d, "/file", 3, false)
			patchUpload(t, m, d, i, 0, "new")
			entry, _ := m.entry(i.ID)
			scan, e := scanUpload(context.Background(), files.InternalFs(d.user.Fs), entry.record.Stage, settings.ClamAV{Enabled: true, URL: srv.URL})
			if scan.Status != tt.status || (e == nil) != (tt.status == "clean") {
				t.Fatal(scan, e, tt.status)
			}
		})
	}
}
func TestScanFailureCanRetryAfterRestart(t *testing.T) {
	m, d, scope, journal := uploadFixture(t)
	fixtureFile(t, scope, "file", "old")
	reply := `{"success":true}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		_, _ = io.WriteString(w, reply)
	}))
	defer srv.Close()
	d.settings.ClamAV = settings.ClamAV{Enabled: true, URL: srv.URL, ScanTimeout: "1m", ScanWorkers: 1}
	if e := d.store.Settings.Save(d.settings); e != nil {
		t.Fatal(e)
	}
	i := startUpload(t, m, d, "/file", 3, true)
	patchUpload(t, m, d, i, 0, "new")
	m.processOne()
	i, _ = m.Info(i.ID, d)
	if i.Status != "failed" || i.Scan.Status != "unknown" || !i.CanRetry {
		t.Fatal(i)
	}
	expectContent(t, scope, "file", "old")
	m.Close()
	next, e := NewUploadManager(journal, d.store, d.server, false)
	if e != nil {
		t.Fatal(e)
	}
	defer next.Close()
	reply = `{"infected":false}`
	if _, e = next.Retry(i.ID, d); e != nil {
		t.Fatal(e)
	}
	next.processOne()
	i, _ = next.Info(i.ID, d)
	if i.Status != "ready" || i.Scan.Status != "clean" {
		t.Fatal(i)
	}
	expectContent(t, scope, "file", "new")
}
func TestFinalPatchDoesNotWaitForScanner(t *testing.T) {
	m, d, scope, _ := uploadFixture(t)
	entered, release := make(chan struct{}), make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		close(entered)
		<-release
		_, _ = io.WriteString(w, `{"infected":false}`)
	}))
	defer srv.Close()
	defer close(release)
	d.settings.ClamAV = settings.ClamAV{Enabled: true, URL: srv.URL, ScanTimeout: "1m", ScanWorkers: 1}
	if e := d.store.Settings.Save(d.settings); e != nil {
		t.Fatal(e)
	}
	i := startUpload(t, m, d, "/file", 3, false)
	start := time.Now()
	i = patchUpload(t, m, d, i, 0, "new")
	if i.Status != "received" || time.Since(start) > time.Second {
		t.Fatal("PATCH waited for scan", i)
	}
	done := make(chan struct{})
	go func() { m.processOne(); close(done) }()
	<-entered
	if _, e := os.Stat(filepath.Join(scope, "file")); !os.IsNotExist(e) {
		t.Fatal("unscanned file published")
	}
	if e := m.Cancel(i.ID, d); e != nil {
		t.Fatal(e)
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("scan did not cancel")
	}
}
func TestOversizeAndAmbiguousCommitSafety(t *testing.T) {
	m, d, scope, journal := uploadFixture(t)
	fixtureFile(t, scope, "file", "old")
	for _, policy := range []string{"block", "skip"} {
		_, scan, e := writeScannedFile(context.Background(), d.user.Fs, "/file", strings.NewReader("new"), settings.ClamAV{Enabled: true, AutomaticMaxBytes: 1, OversizePolicy: policy}, 0644, 0755)
		if scan.Status != "skipped" || (e == nil) != (policy == "skip") {
			t.Fatal(scan, e)
		}
		if policy == "block" {
			expectContent(t, scope, "file", "old")
		}
	}
	i := startUpload(t, m, d, "/file", 4, true)
	i = patchUpload(t, m, d, i, 0, "next")
	entry, _ := m.entry(i.ID)
	r := entry.record
	r.Status = "committing"
	if e := m.save(entry, r); e != nil {
		t.Fatal(e)
	}
	if _, e := m.staged(r, d.user.Fs).Commit(); e != nil {
		t.Fatal(e)
	}
	m.Close()
	next, e := NewUploadManager(journal, d.store, d.server, false)
	if e != nil {
		t.Fatal(e)
	}
	defer next.Close()
	i, e = next.Info(i.ID, d)
	if e != nil || i.Status != "ready" {
		t.Fatal("commit not reconciled", i, e)
	}
	if e = next.Cancel(i.ID, d); !errors.Is(e, ErrUploadState) {
		t.Fatal("cancel claimed success after publication", e)
	}
	if e = next.Dismiss(i.ID, d); e != nil {
		t.Fatal(e)
	}
	expectContent(t, scope, "file", "next")
}
func TestUploadListRouteReturnsJSONWithoutRedirect(t *testing.T) {
	for _, base := range []string{"", "/fb"} {
		t.Run(base, func(t *testing.T) {
			m, d, scope, _ := uploadFixture(t)
			d.server.Root = scope
			d.server.BaseURL = base
			info := startUpload(t, m, d, "/file", 3, false)
			handler, err := NewHandler(nil, nil, m, d.store, d.server, fstest.MapFS{"index.html": {Data: []byte("static fallback")}})
			if err != nil {
				t.Fatal(err)
			}
			for _, suffix := range []string{"", "/"} {
				req := httptest.NewRequest("GET", base+"/api/uploads"+suffix, http.NoBody)
				req.Header.Set("X-Auth", signToken(t, d.user.Perm, []byte("key")))
				w := httptest.NewRecorder()
				handler.ServeHTTP(w, req)
				var list []UploadInfo
				if w.Code != 200 || w.Header().Get("Location") != "" || json.Unmarshal(w.Body.Bytes(), &list) != nil || len(list) != 1 || list[0].ID != info.ID {
					t.Fatalf("list route: status %d location %q body %q", w.Code, w.Header().Get("Location"), w.Body.String())
				}
			}
		})
	}
}

func TestUploadListKeepsOlderUnfinishedSessions(t *testing.T) {
	m, d, scope, _ := uploadFixture(t)
	for index := 0; index < 210; index++ {
		id, err := randomUploadID()
		if err != nil {
			t.Fatal(err)
		}
		m.entries[id] = &uploadEntry{record: uploadRecord{UploadInfo: UploadInfo{ID: id, Status: "ready", UpdatedAt: time.Now()}, OwnerID: d.user.ID, Scope: scope}}
	}
	active, err := randomUploadID()
	if err != nil {
		t.Fatal(err)
	}
	m.entries[active] = &uploadEntry{record: uploadRecord{UploadInfo: UploadInfo{ID: active, Status: "paused", UpdatedAt: time.Now().Add(-time.Hour)}, OwnerID: d.user.ID, Scope: scope}}
	req := httptest.NewRequest("GET", "/api/uploads", http.NoBody)
	req.Header.Set("X-Auth", signToken(t, d.user.Perm, []byte("key")))
	w := httptest.NewRecorder()
	handle(uploadListHandler(m), "", d.store, d.server).ServeHTTP(w, req)
	var list []UploadInfo
	if err = json.Unmarshal(w.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, info := range list {
		found = found || info.ID == active
	}
	if len(list) != 201 || !found {
		t.Fatal("older unfinished upload hidden by completed history", len(list), found)
	}
}

func TestPermittedExternalSymlinkUploadRecoversAfterRestart(t *testing.T) {
	m, d, scope, journal := uploadFixture(t)
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(scope, "link")); err != nil {
		t.Fatal(err)
	}
	d.server.FollowExternalSymlinks = true
	userStore := d.store.Users.(*customFSUser)
	userStore.followExternal = true
	userStore.fs = files.NewFs(afero.NewOsFs(), scope, true)
	d.user.Fs = userStore.fs
	info := startUpload(t, m, d, "/link/file", 11, false)
	patchUpload(t, m, d, info, 0, "hello ")
	m.Close()
	next, err := NewUploadManager(journal, d.store, d.server, false)
	if err != nil {
		t.Fatal(err)
	}
	defer next.Close()
	info, err = next.Info(info.ID, d)
	if err != nil || info.Offset != 6 || info.Status != "uploading" {
		t.Fatal("permitted external upload did not recover", info, err)
	}
	patchUpload(t, next, d, info, 6, "world")
	next.processOne()
	info, err = next.Info(info.ID, d)
	if err != nil || info.Status != "ready" {
		t.Fatal(info, err)
	}
	expectContent(t, outside, "file", "hello world")
}
