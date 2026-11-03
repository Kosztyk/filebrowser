package fbhttp

import (
	"context"
	"fmt"
	"github.com/filebrowser/filebrowser/v2/files"
	"log"
	"os"
	"time"
)

func (m *UploadManager) Start() {
	m.startOnce.Do(func() {
		for worker := 0; worker < 8; worker++ {
			m.workers.Add(1)
			go func(index int) {
				defer m.workers.Done()
				for {
					if m.ctx.Err() != nil {
						return
					}
					set, e := m.store.Settings.Get()
					if e == nil && index < set.ClamAV.ScanWorkers && m.processOne() {
						continue
					}
					select {
					case <-m.ctx.Done():
						return
					case <-m.wake:
					case <-time.After(time.Second):
					}
				}
			}(worker)
		}
		m.workers.Add(1)
		go func() {
			defer m.workers.Done()
			m.expire()
			timer := time.NewTicker(time.Minute)
			defer timer.Stop()
			for {
				select {
				case <-m.ctx.Done():
					return
				case <-timer.C:
					m.expire()
				}
			}
		}()
	})
}
func (m *UploadManager) processOne() bool {
	for _, e := range m.snapshot() {
		if !e.mu.TryLock() {
			continue
		}
		r := e.record
		if r.Status != "received" || time.Now().After(r.ExpiresAt) {
			e.mu.Unlock()
			continue
		}
		ctx, cancel := context.WithCancel(m.ctx)
		r.Status = "scanning"
		r.Scan = ScanResult{Status: "pending", Message: "Checking upload before publication"}
		r.UpdatedAt = time.Now()
		if err := m.save(e, r); err != nil {
			cancel()
			e.mu.Unlock()
			log.Printf("upload processing checkpoint: %v", err)
			return false
		}
		e.cancel = cancel
		e.mu.Unlock()
		m.process(ctx, e, r)
		cancel()
		return true
	}
	return false
}
func (m *UploadManager) process(ctx context.Context, e *uploadEntry, r uploadRecord) {
	d, err := m.dataFor(r)
	scan := ScanResult{Status: "error"}
	if err == nil {
		duration, parseErr := time.ParseDuration(d.settings.ClamAV.ScanTimeout)
		if parseErr != nil || duration <= 0 {
			duration = 30 * time.Minute
		}
		scanCtx, cancel := context.WithTimeout(ctx, duration)
		scan, err = scanUpload(scanCtx, files.InternalFs(d.user.Fs), r.Stage, d.settings.ClamAV, r.Path)
		cancel()
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	e.cancel = nil
	if e.record.Status != "scanning" {
		return
	}
	r = e.record
	if m.ctx.Err() != nil {
		r.Status = "received"
		r.Scan = ScanResult{Status: "pending", Message: "Processing resumes after restart"}
		_ = m.save(e, r)
		return
	}
	if err != nil {
		r.Status = "failed"
		r.CanRetry = scan.Status != "infected"
		if scan.Status == "infected" {
			r.Status = "infected"
		}
		r.Scan = scan
		r.Message = err.Error()
		r.UpdatedAt = time.Now()
		if err = m.save(e, r); err != nil {
			log.Printf("upload result checkpoint: %v", err)
		}
		return
	}
	latest, err := m.dataFor(r)
	if err == nil {
		err = m.access(e, latest, true)
	}
	if err != nil {
		r.Status = "failed"
		r.CanRetry = true
		r.Message = "Upload permission or scope changed; destination was preserved"
		_ = m.save(e, r)
		return
	}
	if latest.settings.ClamAV != d.settings.ClamAV {
		r.Status = "received"
		r.Scan = ScanResult{Status: "pending", Message: "Scan policy changed; checking again"}
		_ = m.save(e, r)
		m.notify()
		return
	}
	r.Scan = scan
	r.Status = "committing"
	r.UpdatedAt = time.Now()
	if err = m.save(e, r); err != nil {
		log.Printf("upload commit checkpoint: %v", err)
		return
	}
	err = runPublicationHook(latest, "upload", r.Path, func() error { _, err := m.staged(r, latest.user.Fs).Commit(); return err })
	if err != nil {
		if ok, recoveryErr := m.recoverCommit(e, files.InternalFs(latest.user.Fs)); ok {
			return
		} else if recoveryErr != nil && !os.IsNotExist(recoveryErr) {
			log.Printf("upload reconciliation: %v", recoveryErr)
		}
		r.Status = "failed"
		r.CanRetry = uploadErrorState(err) != "conflict"
		r.Message = fmt.Sprintf("Upload %s: %v", uploadErrorState(err), err)
		_ = m.save(e, r)
		return
	}
	r.Status = "ready"
	r.CanRetry = false
	r.Message = "File published"
	r.UpdatedAt = time.Now()
	r.ExpiresAt = r.UpdatedAt.Add(time.Duration(latest.settings.Tus.RetentionHours) * time.Hour)
	if err = m.save(e, r); err != nil {
		log.Printf("upload published; final checkpoint will be reconciled: %v", err)
	}
}
func (m *UploadManager) expire() {
	for _, e := range m.snapshot() {
		if !e.mu.TryLock() {
			continue
		}
		r := e.record
		if time.Now().Before(r.ExpiresAt) || e.cancel != nil {
			e.mu.Unlock()
			continue
		}
		if r.Status == "committing" {
			if _, err := m.recoverCommit(e, recoveryFs(r)); err != nil && !os.IsNotExist(err) {
				e.mu.Unlock()
				continue
			}
			r = e.record
		}
		if (sessionHasStaging(r.Status) || r.Status == "cancelled") && safeStagingRecord(r) {
			fs := recoveryFs(r)
			if err := fs.Remove(r.Stage); err != nil && !os.IsNotExist(err) {
				e.mu.Unlock()
				continue
			}
			files.ReleaseWrite(fs, r.Path, r.Token)
		}
		if err := m.remove(e); err != nil {
			log.Printf("upload expiration: %v", err)
		}
		e.mu.Unlock()
	}
}
