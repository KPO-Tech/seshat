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

// sessionWorkspaceDir returns the per-session working directory where the agent
// organises files and produces deliverables. Lives inside the session dir so
// the entire session (workspace + artifacts + logs) can be cleaned up together.
func sessionWorkspaceDir(sessionID sdk.SessionID) string {
	return filepath.Join(runtimepath.SessionDir("", string(sessionID)), "workspace")
}

// uploadTurnArtifacts uploads files produced during the current turn to the
// Slack thread. Only user-relevant output is uploaded:
//   - session workspace (agent-created files, reports, code, etc.)
//   - artifacts/images (AI-generated images)
//   - artifacts/audio  (TTS output)
//
// Browser screenshots, web-scraping cache, plan files, pastes and logs are
// intentionally excluded — they are operational artefacts, not deliverables.
func (b *bot) uploadTurnArtifacts(
	ctx context.Context,
	channel, replyTS string,
	sessionID sdk.SessionID,
	startTime time.Time,
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

	scanDir := func(dir string) {
		_ = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
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
	}

	sid := string(sessionID)

	// Agent workspace — primary deliverable area.
	scanDir(sessionWorkspaceDir(sessionID))

	// Generated images (DALL-E, Stable Diffusion, etc.).
	scanDir(runtimepath.SessionArtifactsImagesDir("", sid))

	// TTS audio output.
	scanDir(runtimepath.SessionArtifactsAudioDir("", sid))

	// screenshots, web cache, plans, pastes, session.log → excluded

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
