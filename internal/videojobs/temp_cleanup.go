package videojobs

import (
	"os"
	"path/filepath"
	"time"
)

type TempCleanupReport struct {
	Scanned int
	Removed int
	Failed  int
}

func CleanupStaleTempDirs(prefix string, olderThan time.Duration) TempCleanupReport {
	report := TempCleanupReport{}
	if olderThan <= 0 {
		return report
	}
	if prefix == "" {
		prefix = "video-job-"
	}

	pattern := filepath.Join(os.TempDir(), prefix+"*")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return report
	}

	cutoff := time.Now().Add(-olderThan)
	for _, candidate := range matches {
		info, statErr := os.Stat(candidate)
		if statErr != nil || !info.IsDir() {
			continue
		}
		report.Scanned++
		if info.ModTime().After(cutoff) {
			continue
		}
		if removeErr := os.RemoveAll(candidate); removeErr != nil {
			report.Failed++
			continue
		}
		report.Removed++
	}
	return report
}
