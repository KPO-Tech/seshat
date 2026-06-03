package browser

import (
	"time"

	"github.com/EngineerProjects/nexus-engine/internal/storage"
	"github.com/EngineerProjects/nexus-engine/internal/types"
)

func screenshotKey(sessionID types.SessionID, pageID string) string {
	return storage.ScreenshotKey(string(sessionID), pageID, time.Now().UTC())
}

func downloadKey(sessionID types.SessionID, pageID string, filename string) string {
	return storage.DownloadKey(string(sessionID), pageID, filename, time.Now().UTC())
}
