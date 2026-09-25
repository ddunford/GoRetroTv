package media

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Source describes one schedule-owned file or folder beneath the operator's media root.
type Source struct {
	Path string
	Loop bool
}

// Item is one probed file on a playlist timeline.
type Item struct {
	Path     string
	Duration time.Duration
}

// Playlist is an immutable, duration-probed media timeline.
type Playlist struct {
	Items []Item
	Total time.Duration
	Loop  bool
}

// Position identifies the file and offset which are live at a programme-relative elapsed time.
type Position struct {
	Item   Item
	Offset time.Duration
}

// ProbePlaylist resolves a file or a lexically ordered folder, confines every result to root, and
// asks ffprobe for the exact duration used by the live-offset calculation.
func ProbePlaylist(ctx context.Context, ffprobe, root, kind string, source Source,
	programmeDuration time.Duration) (Playlist, error) {
	resolvedProbe, err := exec.LookPath(ffprobe)
	if err != nil {
		return Playlist{}, fmt.Errorf("media: find ffprobe: %w", err)
	}
	root, err = filepath.Abs(root)
	if err != nil {
		return Playlist{}, fmt.Errorf("media: resolve root: %w", err)
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return Playlist{}, fmt.Errorf("media: resolve root: %w", err)
	}
	target, err := confinedPath(root, source.Path)
	if err != nil {
		return Playlist{}, err
	}
	var paths []string
	switch kind {
	case "file":
		paths = []string{target}
	case "folder":
		entries, readErr := os.ReadDir(target)
		if readErr != nil {
			return Playlist{}, fmt.Errorf("media: read folder %q: %w", source.Path, readErr)
		}
		for _, entry := range entries {
			if entry.Type().IsRegular() && supportedMedia(entry.Name()) {
				path, pathErr := confinedPath(root, filepath.Join(source.Path, entry.Name()))
				if pathErr != nil {
					return Playlist{}, pathErr
				}
				paths = append(paths, path)
			}
		}
		sort.Strings(paths)
	default:
		return Playlist{}, fmt.Errorf("media: unsupported playlist kind %q", kind)
	}
	if len(paths) == 0 {
		return Playlist{}, fmt.Errorf("media: %s %q contains no supported files", kind, source.Path)
	}
	playlist := Playlist{Loop: source.Loop}
	for _, path := range paths {
		duration, probeErr := probeDuration(ctx, resolvedProbe, path)
		if probeErr != nil {
			return Playlist{}, probeErr
		}
		playlist.Items = append(playlist.Items, Item{Path: path, Duration: duration})
		playlist.Total += duration
	}
	if !playlist.Loop && playlist.Total < programmeDuration {
		return Playlist{}, fmt.Errorf("media: playlist duration %s does not cover programme duration %s without loop",
			playlist.Total, programmeDuration)
	}
	return playlist, nil
}

func confinedPath(root, relative string) (string, error) {
	if relative == "" || filepath.IsAbs(relative) {
		return "", fmt.Errorf("media: path %q must be relative to the media root", relative)
	}
	joined := filepath.Join(root, relative)
	resolved, err := filepath.EvalSymlinks(joined)
	if err != nil {
		return "", fmt.Errorf("media: resolve %q: %w", relative, err)
	}
	rel, err := filepath.Rel(root, resolved)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("media: path %q escapes the media root", relative)
	}
	return resolved, nil
}

func supportedMedia(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".mp4", ".m4v", ".mov", ".mkv", ".webm", ".avi", ".mpg", ".mpeg", ".ts":
		return true
	default:
		return false
	}
}

func probeDuration(ctx context.Context, ffprobe, path string) (time.Duration, error) {
	output, err := exec.CommandContext(ctx, ffprobe, "-v", "error", "-show_entries", // #nosec G204 -- resolved executable and confined operator path
		"format=duration", "-of", "json", path).Output()
	if err != nil {
		return 0, fmt.Errorf("media: probe %q: %w", filepath.Base(path), err)
	}
	var result struct {
		Format struct {
			Duration string `json:"duration"`
		} `json:"format"`
	}
	if err := json.Unmarshal(output, &result); err != nil {
		return 0, fmt.Errorf("media: decode probe for %q: %w", filepath.Base(path), err)
	}
	duration, err := time.ParseDuration(result.Format.Duration + "s")
	if err != nil || duration <= 0 {
		return 0, fmt.Errorf("media: invalid duration %q for %q", result.Format.Duration, filepath.Base(path))
	}
	return duration, nil
}

// At maps the in-world programme offset to a file and offset within that file.
func (p Playlist) At(elapsed time.Duration) (Position, error) {
	if elapsed < 0 || p.Total <= 0 || len(p.Items) == 0 {
		return Position{}, fmt.Errorf("media: invalid playlist position %s", elapsed)
	}
	if p.Loop {
		elapsed %= p.Total
	} else if elapsed >= p.Total {
		return Position{}, fmt.Errorf("media: programme offset %s is beyond playlist duration %s", elapsed, p.Total)
	}
	for _, item := range p.Items {
		if elapsed < item.Duration {
			return Position{Item: item, Offset: elapsed}, nil
		}
		elapsed -= item.Duration
	}
	return Position{}, fmt.Errorf("media: playlist position fell outside its probed duration")
}
