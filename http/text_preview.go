package fbhttp

import (
	"bytes"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"unicode/utf8"
)

var textPreviewHandler = withUser(func(w http.ResponseWriter, r *http.Request, d *data) (int, error) {
	if !d.user.Perm.Download || !d.Check(r.URL.Path) {
		return http.StatusForbidden, nil
	}
	f, e := d.user.Fs.Open(r.URL.Path)
	if e != nil {
		return errToStatus(e), e
	}
	defer f.Close()
	info, e := f.Stat()
	if e != nil {
		return errToStatus(e), e
	}
	if !info.Mode().IsRegular() {
		return http.StatusBadRequest, nil
	}
	version := fmt.Sprintf("%x-%x", info.ModTime().UnixNano(), info.Size())
	if expected := r.URL.Query().Get("version"); expected != "" && expected != version {
		return http.StatusConflict, nil
	}
	offset, _ := strconv.ParseInt(r.URL.Query().Get("offset"), 10, 64)
	if offset < 0 || offset > info.Size() {
		return http.StatusBadRequest, nil
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("bytes"))
	if limit < 1 || limit > 1<<20 {
		limit = 256 << 10
	}
	if _, e = f.Seek(offset, io.SeekStart); e != nil {
		return errToStatus(e), e
	}
	if r.URL.Query().Get("csv") == "true" {
		buffer, e := io.ReadAll(io.LimitReader(f, int64(limit)))
		if e != nil {
			return http.StatusInternalServerError, e
		}
		rows := [][]string{}
		reader := csv.NewReader(bytes.NewReader(buffer))
		reader.FieldsPerRecord = -1
		delim := []rune(r.URL.Query().Get("delimiter"))
		if len(delim) == 1 && (delim[0] == ',' || delim[0] == ';' || delim[0] == '\t') {
			reader.Comma = delim[0]
		}
		used := int64(0)
		for len(rows) < 2000 {
			row, err := reader.Read()
			if err != nil {
				if !errors.Is(err, io.EOF) && used == 0 {
					return http.StatusRequestEntityTooLarge, fmt.Errorf("CSV record exceeds preview budget or is invalid: %w", err)
				}
				break
			}
			end := reader.InputOffset()
			if end == int64(len(buffer)) && offset+end < info.Size() && len(buffer) > 0 && buffer[len(buffer)-1] != '\n' {
				break
			}
			used = end
			rows = append(rows, row)
		}
		if used == 0 && offset < info.Size() {
			return http.StatusRequestEntityTooLarge, fmt.Errorf("CSV record exceeds preview budget")
		}
		after, e := f.Stat()
		if e != nil || after.Size() != info.Size() || !after.ModTime().Equal(info.ModTime()) {
			return http.StatusConflict, nil
		}
		return renderJSON(w, r, map[string]any{"rows": rows, "offset": offset, "next": offset + used, "size": info.Size(), "version": version, "hasMore": offset+used < info.Size()})
	}
	b, e := io.ReadAll(io.LimitReader(f, int64(limit)))
	if e != nil {
		return http.StatusInternalServerError, e
	}
	if bytes.IndexByte(b, 0) >= 0 {
		return http.StatusUnsupportedMediaType, nil
	}
	n := len(b)
	if offset+int64(n) < info.Size() {
		if last := bytes.LastIndexByte(b, '\n'); last >= 0 {
			n = last + 1
		} else {
			for n > 0 && !utf8.Valid(b[:n]) {
				n--
			}
		}
	}
	if n == 0 && len(b) > 0 {
		return http.StatusUnsupportedMediaType, nil
	}
	b = b[:n]
	lines := bytes.SplitAfter(b, []byte{'\n'})
	if len(lines) > 2000 {
		b = bytes.Join(lines[:2000], nil)
		n = len(b)
	}
	after, e := f.Stat()
	if e != nil || after.Size() != info.Size() || !after.ModTime().Equal(info.ModTime()) {
		return http.StatusConflict, nil
	}
	return renderJSON(w, r, map[string]any{"text": string(b), "offset": offset, "next": offset + int64(n), "size": info.Size(), "version": version, "hasMore": offset+int64(n) < info.Size()})
})
