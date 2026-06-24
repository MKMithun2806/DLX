package ytdlp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"

	"videodl/internal/models"
)

// Runner wraps invocations of the yt-dlp binary.
type Runner struct {
	BinPath string
}

func New(binPath string) *Runner {
	if binPath == "" {
		binPath = "yt-dlp"
	}
	return &Runner{BinPath: binPath}
}

// rawFormat mirrors the subset of yt-dlp's --dump-json "formats" array that
// we care about.
type rawFormat struct {
	FormatID   string  `json:"format_id"`
	Ext        string  `json:"ext"`
	Height     int     `json:"height"`
	Width      int     `json:"width"`
	Filesize   float64 `json:"filesize"`
	FilesizeAp float64 `json:"filesize_approx"`
	FormatNote string  `json:"format_note"`
	VCodec     string  `json:"vcodec"`
	ACodec     string  `json:"acodec"`
}

// rawInfo mirrors the subset of yt-dlp's --dump-json top-level video info
// that we care about.
type rawInfo struct {
	Title      string      `json:"title"`
	Thumbnail  string      `json:"thumbnail"`
	Duration   float64     `json:"duration"`
	Uploader   string      `json:"uploader"`
	Formats    []rawFormat `json:"formats"`
	Filesize   float64     `json:"filesize"`
	WebpageURL string      `json:"webpage_url"`
}

// buildArgs appends global/per-request proxy flags to a yt-dlp argument list.
func proxyArgs(proxy string) []string {
	if proxy == "" {
		return nil
	}
	return []string{"--proxy", proxy}
}

// Scan runs `yt-dlp --dump-json` (with -J for flat playlist expansion via
// --yes-playlist) against a single URL and returns a normalized ScanResult.
// A non-fatal per-URL error is captured in ScanResult.Error rather than
// returned, so callers can scan a batch and report partial failures.
func (r *Runner) Scan(ctx context.Context, url, proxy string) models.ScanResult {
	args := []string{"--dump-json", "--no-warnings", "--skip-download", "--no-playlist"}
	args = append(args, proxyArgs(proxy)...)
	args = append(args, url)

	cmd := exec.CommandContext(ctx, r.BinPath, args...)
	out, err := cmd.Output()
	if err != nil {
		msg := err.Error()
		if ee, ok := err.(*exec.ExitError); ok {
			msg = string(ee.Stderr)
			if msg == "" {
				msg = err.Error()
			}
		}
		return models.ScanResult{URL: url, Error: fmt.Sprintf("yt-dlp scan failed: %s", strings.TrimSpace(msg))}
	}

	// yt-dlp can print one JSON object per line for playlists; take the
	// first line for single-video scans, but also handle the common case
	// of exactly one JSON document.
	line := out
	if idx := strings.IndexByte(string(out), '\n'); idx >= 0 {
		line = out[:idx]
	}

	var info rawInfo
	if err := json.Unmarshal(line, &info); err != nil {
		return models.ScanResult{URL: url, Error: fmt.Sprintf("failed to parse yt-dlp output: %v", err)}
	}

	formats := make([]models.Format, 0, len(info.Formats))
	var bestRes string
	var bestSize int64
	for _, f := range info.Formats {
		size := f.Filesize
		if size == 0 {
			size = f.FilesizeAp
		}
		res := f.FormatNote
		if f.Height > 0 {
			res = fmt.Sprintf("%dx%d", f.Width, f.Height)
		}
		formats = append(formats, models.Format{
			FormatID:   f.FormatID,
			Resolution: res,
			Ext:        f.Ext,
			Filesize:   int64(size),
			Note:       f.FormatNote,
		})
		if f.Height > 0 {
			bestRes = res
			if int64(size) > bestSize {
				bestSize = int64(size)
			}
		}
	}
	if bestSize == 0 {
		bestSize = int64(info.Filesize)
	}

	return models.ScanResult{
		URL:        url,
		Title:      info.Title,
		Thumbnail:  info.Thumbnail,
		Duration:   int(info.Duration),
		Uploader:   info.Uploader,
		Formats:    formats,
		Resolution: bestRes,
		Filesize:   bestSize,
	}
}

// ProgressFunc receives incremental progress updates (0-100) and a status
// message while a download runs.
type ProgressFunc func(percent float64, message string)

var progressRe = regexp.MustCompile(`\[download\]\s+([0-9.]+)%`)

// Download invokes yt-dlp to fetch a single URL/format into outputTemplate
// (a yt-dlp -o template, e.g. "/downloads/%(title)s.%(ext)s"), streaming
// progress lines to onProgress as they arrive. It returns the resolved
// output file path reported by yt-dlp, if it can be determined.
func (r *Runner) Download(ctx context.Context, url, formatID, proxy, outputTemplate string, onProgress ProgressFunc) (string, error) {
	args := []string{"--newline", "--no-warnings", "--no-playlist", "-o", outputTemplate}
	if formatID != "" {
		args = append(args, "-f", formatID)
	}
	args = append(args, proxyArgs(proxy)...)
	args = append(args, "--print", "after_move:filepath")
	args = append(args, url)

	cmd := exec.CommandContext(ctx, r.BinPath, args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return "", err
	}

	if err := cmd.Start(); err != nil {
		return "", err
	}

	var resolvedPath string
	done := make(chan struct{})

	go func() {
		defer close(done)
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			line := scanner.Text()
			if m := progressRe.FindStringSubmatch(line); m != nil {
				if pct, perr := strconv.ParseFloat(m[1], 64); perr == nil && onProgress != nil {
					onProgress(pct, line)
				}
				continue
			}
			// any non-progress, non-empty line printed last is treated as
			// the resolved output path (from --print after_move:filepath)
			trimmed := strings.TrimSpace(line)
			if trimmed != "" && !strings.HasPrefix(trimmed, "[") {
				resolvedPath = trimmed
			}
			if onProgress != nil {
				onProgress(-1, line)
			}
		}
	}()

	errBuf := &strings.Builder{}
	go func() {
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			errBuf.WriteString(scanner.Text())
			errBuf.WriteString("\n")
		}
	}()

	<-done
	waitErr := cmd.Wait()
	if waitErr != nil {
		return "", fmt.Errorf("yt-dlp download failed: %s", strings.TrimSpace(errBuf.String()))
	}
	return resolvedPath, nil
}
