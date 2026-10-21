package runs

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const maxSnapshotBytes = 4 << 20

var runIDPattern = regexp.MustCompile(`^[0-9a-f-]{36}$`)

type Docker struct {
	ArtifactRoot string
	Image        string
	Binary       string
}

type boundedWriter struct {
	buf       bytes.Buffer
	limit     int
	truncated bool
}

func (w *boundedWriter) Write(p []byte) (int, error) {
	n := len(p)
	if room := w.limit - w.buf.Len(); room > 0 {
		if room < len(p) {
			p = p[:room]
		}
		_, _ = w.buf.Write(p)
	}
	if n > len(p) {
		w.truncated = true
	}
	return n, nil
}

func (w *boundedWriter) String() string {
	if w.truncated {
		return w.buf.String() + "\n[output truncated]"
	}
	return w.buf.String()
}

func safeRelative(v string) bool {
	return v != "" && !strings.Contains(v, "\\") && !strings.HasPrefix(v, "/") && path.Clean(v) == v &&
		v != "." && !strings.HasPrefix(v, "../") && v != ".."
}

func validateLimits(l Limits) error {
	if l.CPUMillis < 100 || l.CPUMillis > 2000 || l.MemoryMiB < 64 || l.MemoryMiB > 2048 ||
		l.TimeoutSeconds < 1 || l.TimeoutSeconds > 180 || l.OutputKiB < 1 || l.OutputKiB > 256 {
		return ErrInvalid
	}
	return nil
}

func commandFor(s Snapshot) ([]string, error) {
	if err := validateLimits(s.Manifest.Limits); err != nil {
		return nil, err
	}
	var argv []string
	if s.Kind == Baseline {
		argv = s.Manifest.BaselineCommand
	} else if s.Kind == Visible {
		argv = s.Manifest.VisibleTestCommand
	} else {
		return nil, ErrInvalid
	}
	if len(argv) < 1 || len(argv) > 16 || argv[0] != "go" {
		return nil, ErrInvalid
	}
	for _, arg := range argv {
		if arg == "" || len(arg) > 256 || strings.ContainsAny(arg, "\x00\r\n") {
			return nil, ErrInvalid
		}
	}
	return argv, nil
}

func (d Docker) Execute(ctx context.Context, s Snapshot) (Result, error) {
	argv, err := commandFor(s)
	if err != nil {
		return Result{}, err
	}
	if !safeRelative(s.ArtifactURI) || !safeRelative(s.Manifest.SourceRoot) || d.ArtifactRoot == "" || d.Image == "" {
		return Result{}, ErrInvalid
	}
	if !runIDPattern.MatchString(s.RunID) {
		return Result{}, ErrInvalid
	}
	root, err := filepath.EvalSymlinks(d.ArtifactRoot)
	if err != nil {
		return Result{}, err
	}
	source := filepath.Join(root, filepath.FromSlash(s.ArtifactURI), filepath.FromSlash(s.Manifest.SourceRoot))
	resolved, err := filepath.EvalSymlinks(source)
	if err != nil {
		return Result{}, err
	}
	if !strings.HasPrefix(resolved, root+string(os.PathSeparator)) {
		return Result{}, ErrInvalid
	}
	allowed := make(map[string]bool, len(s.Manifest.AllowedPatchPaths))
	for _, name := range s.Manifest.AllowedPatchPaths {
		if !safeRelative(name) {
			return Result{}, ErrInvalid
		}
		allowed[name] = true
	}
	for name := range s.Overlay {
		if !allowed[name] {
			return Result{}, ErrInvalid
		}
	}
	tmp, err := os.MkdirTemp("", "repotrial-run-")
	if err != nil {
		return Result{}, err
	}
	defer os.RemoveAll(tmp)
	if err := copySnapshot(source, tmp, s.Overlay); err != nil {
		return Result{}, err
	}
	if err := os.Chmod(tmp, 0755); err != nil {
		return Result{}, err
	}
	limit := s.Manifest.Limits
	maxOutput := limit.OutputKiB * 1024
	out := &boundedWriter{limit: maxOutput / 2}
	errOut := &boundedWriter{limit: maxOutput - maxOutput/2}
	deadline, cancel := context.WithTimeout(ctx, time.Duration(limit.TimeoutSeconds+10)*time.Second)
	defer cancel()
	binary := d.Binary
	if binary == "" {
		binary = "docker"
	}
	containerName := "repotrial-" + s.RunID
	// The CLI can be killed by a deadline while the container continues running.
	// Remove the named container independently before returning in every case.
	defer func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		_ = exec.CommandContext(cleanupCtx, binary, "rm", "-f", containerName).Run()
	}()
	args := []string{"run", "--rm", "--name=" + containerName, "--network=none", "--read-only", "--cap-drop=ALL", "--security-opt=no-new-privileges",
		"--pids-limit=64", "--cpus=" + strconv.FormatFloat(float64(limit.CPUMillis)/1000, 'f', 3, 64),
		"--memory=" + strconv.Itoa(limit.MemoryMiB) + "m", "--memory-swap=" + strconv.Itoa(limit.MemoryMiB) + "m",
		"--user=65532:65532", "--tmpfs=/tmp:rw,nosuid,nodev,size=128m", "--workdir=/workspace",
		"--env=GOCACHE=/tmp/gocache", "--env=GOPATH=/tmp/gopath", "--env=GOPROXY=off", "--env=GOSUMDB=off",
		"--volume=" + tmp + ":/workspace:ro", d.Image}
	args = append(args, argv...)
	cmd := exec.CommandContext(deadline, binary, args...)
	cmd.Stdout, cmd.Stderr = out, errOut
	err = cmd.Run()
	result := Result{Stdout: out.String(), Stderr: errOut.String()}
	if errors.Is(deadline.Err(), context.DeadlineExceeded) {
		result.Status = TimedOut
		return result, nil
	}
	if err == nil {
		code := 0
		result.ExitCode = &code
		result.Status = Passed
		return result, nil
	}
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		code := exit.ExitCode()
		result.ExitCode = &code
		result.Status = Failed
		if code == 125 || code == 126 || code == 127 {
			result.Status = InfraFailed
		}
		return result, nil
	}
	return Result{}, fmt.Errorf("start container: %w", err)
}

func copySnapshot(source, dest string, overlay map[string]string) error {
	total := 0
	seen := map[string]bool{}
	err := filepath.Walk(source, func(full string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info.Mode()&os.ModeSymlink != 0 || (!info.IsDir() && !info.Mode().IsRegular()) {
			return ErrInvalid
		}
		rel, err := filepath.Rel(source, full)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		slash := filepath.ToSlash(rel)
		if info.IsDir() {
			return os.MkdirAll(filepath.Join(dest, rel), 0755)
		}
		if info.Size() > maxSnapshotBytes {
			return ErrInvalid
		}
		var content []byte
		if replacement, ok := overlay[slash]; ok {
			content = []byte(replacement)
			seen[slash] = true
		} else {
			content, err = os.ReadFile(full)
			if err != nil {
				return err
			}
		}
		total += len(content)
		if total > maxSnapshotBytes {
			return ErrInvalid
		}
		return os.WriteFile(filepath.Join(dest, rel), content, 0644)
	})
	if err != nil {
		return err
	}
	for name := range overlay {
		if !safeRelative(name) || !seen[name] {
			return ErrInvalid
		}
	}
	return nil
}

var _ io.Writer = (*boundedWriter)(nil)
