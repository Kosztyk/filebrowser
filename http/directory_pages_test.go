package fbhttp

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/filebrowser/filebrowser/v2/files"
	"github.com/maruel/natural"
)

// Pages must be one consistent folders-first sequence in either direction,
// including the page that straddles the folder/file boundary.
func TestDirectoryPagesKeepSectionsInEverySort(t *testing.T) {
	f, d, scope, _ := featureFixture(t)
	for i := 0; i < 251; i++ {
		name := filepath.Join(scope, fmt.Sprintf("dir-%d", i))
		if err := os.Mkdir(name, 0700); err != nil {
			t.Fatal(err)
		}
		stamp := time.Unix(1700000000+int64(i), 0)
		if err := os.Chtimes(name, stamp, stamp); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 257; i++ {
		name := fmt.Sprintf("file-%d.txt", i)
		fixtureFile(t, scope, name, strings.Repeat("x", i+1))
		stamp := time.Unix(1700000000+int64(i), 0)
		if err := os.Chtimes(filepath.Join(scope, name), stamp, stamp); err != nil {
			t.Fatal(err)
		}
	}
	for _, by := range []string{"name", "size", "modified"} {
		for _, asc := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/%t", by, asc), func(t *testing.T) {
				query := fmt.Sprintf("/?paged=true&sort=%s&asc=%t&limit=73", by, asc)
				w := httptest.NewRecorder()
				status, err := f.directoryPage(w, httptest.NewRequest("GET", query, nil), d)
				if err != nil || status != 0 || w.Code != 202 {
					t.Fatal(status, w.Code, err)
				}
				var pending struct {
					JobID string `json:"jobId"`
				}
				if err = json.Unmarshal(w.Body.Bytes(), &pending); err != nil {
					t.Fatal(err)
				}
				j, err := f.get(pending.JobID)
				if err != nil {
					t.Fatal(err)
				}
				if done := runJob(t, f, j); done.State != "succeeded" {
					t.Fatal(done.Message)
				}
				var all []files.FileInfo
				for offset := 0; offset < 508; offset += 73 {
					w = httptest.NewRecorder()
					status, err = f.directoryPage(w, httptest.NewRequest("GET", query+fmt.Sprintf("&snapshot=%s&offset=%d", j.ID, offset), nil), d)
					if err != nil || status != 0 {
						t.Fatal(status, err)
					}
					var page struct {
						Items []files.FileInfo
						Page  struct{ Total, NumDirs, NumFiles int }
					}
					if err = json.Unmarshal(w.Body.Bytes(), &page); err != nil {
						t.Fatal(err)
					}
					if page.Page.Total != 508 || page.Page.NumDirs != 251 || page.Page.NumFiles != 257 {
						t.Fatal(page.Page)
					}
					all = append(all, page.Items...)
				}
				if len(all) != 508 {
					t.Fatal(len(all))
				}
				seen := map[string]bool{}
				for i, item := range all {
					if seen[item.Name] {
						t.Fatal("duplicate", item.Name)
					}
					seen[item.Name] = true
					if item.IsDir != (i < 251) {
						t.Fatalf("section order at %d: %s", i, item.Name)
					}
					if i == 0 || all[i-1].IsDir != item.IsDir {
						continue
					}
					prev := all[i-1]
					switch by {
					case "name":
						increasing := natural.Less(strings.ToLower(prev.Name), strings.ToLower(item.Name))
						if increasing == asc {
							t.Fatalf("name order: %s, %s", prev.Name, item.Name)
						}
					case "size":
						if (asc && prev.Size > item.Size) || (!asc && prev.Size < item.Size) {
							t.Fatalf("size order: %d, %d", prev.Size, item.Size)
						}
					case "modified":
						if (asc && prev.ModTime.After(item.ModTime)) || (!asc && prev.ModTime.Before(item.ModTime)) {
							t.Fatal("modified order", prev.Name, item.Name)
						}
					}
				}
			})
		}
	}
}
