package main

import (
	"context"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/EngineerProjects/nexus-engine/pkg/runtimepath"
	"github.com/EngineerProjects/nexus-engine/pkg/sdk"
	slackgo "github.com/slack-go/slack"
)

const maxUploadBytes = 10 * 1024 * 1024 // 10 MB

// uploadTurnArtifacts uploads files created or modified during the current turn
// to the Slack thread. It scans:
//   - the per-channel working directory (agent-produced files)
//   - the session artifacts directory (browser screenshots, etc.)
//   - any paths collected explicitly via RuntimeEventFn (extraPaths)
func (b *bot) uploadTurnArtifacts(
	ctx context.Context,
	channel, replyTS string,
	sessionID sdk.SessionID,
	startTime time.Time,
	extraPaths []string,
) {
	seen := make(map[string]bool)
	var toUpload []string

	collect := func(path string) {
		if seen[path] {
			return
		}
		seen[path] = true
		info, err := os.Stat(path)
		if err != nil || info.IsDir() || info.Size() == 0 || info.Size() > maxUploadBytes {
			return
		}
		if strings.HasPrefix(filepath.Base(path), ".") {
			return
		}
		toUpload = append(toUpload, path)
	}

	// Extra paths collected from RuntimeEventFn (e.g. browser screenshots).
	for _, p := range extraPaths {
		collect(p)
	}

	// Scan channel workdir for files created during this turn.
	workdir := channelWorkdir(channel)
	_ = filepath.WalkDir(workdir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		info, err2 := d.Info()
		if err2 != nil || !info.ModTime().After(startTime) {
			return nil
		}
		collect(path)
		return nil
	})

	// Scan session artifacts directory (screenshots, downloaded files, etc.).
	artifactsDir := runtimepath.SessionArtifactsDir("", string(sessionID))
	_ = filepath.WalkDir(artifactsDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		info, err2 := d.Info()
		if err2 != nil || !info.ModTime().After(startTime) {
			return nil
		}
		collect(path)
		return nil
	})

	for _, path := range toUpload {
		if err := b.uploadFile(ctx, channel, replyTS, path); err != nil {
			log.Printf("[nexus-bot] upload %s: %v", filepath.Base(path), err)
		}
	}
}

func (b *bot) uploadFile(ctx context.Context, channel, replyTS, path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return err
	}

	name := filepath.Base(path)
	log.Printf("[nexus-bot] uploading %s (%d bytes)", name, info.Size())

	_, err = b.api.UploadFileContext(ctx, slackgo.UploadFileParameters{
		Filename:        name,
		Title:           name,
		Reader:          f,
		FileSize:        int(info.Size()),
		Channel:         channel,
		ThreadTimestamp: replyTS,
		InitialComment:  fmt.Sprintf("📎 _%s_", name),
	})
	return err
}
