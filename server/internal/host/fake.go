package host

import (
	"io"
	"os"
	"path/filepath"
	"sync"
)

// FakeHost is an injectable Host for tests. Its filesystem is backed by a real
// temporary directory so rename/move/delete and out-of-root rejection run
// against real syscalls, while metrics and docker return canned data the test
// controls.
type FakeHost struct {
	mu sync.Mutex

	// Vitals returned by Sample(); tests set this directly.
	Vitals    Vitals
	SampleErr error

	// DockerStatus returned by Status().
	Docker DockerStatus

	// Canned drill-in details keyed by pid / container id. A missing key is
	// ErrNotFound; container lookups return ErrDockerUnavailable while
	// Docker.Enabled is false.
	Processes  map[int32]ProcessDetail
	Containers map[string]ContainerDetail
}

// NewFakeHost builds a FakeHost with Docker disabled by default.
func NewFakeHost() *FakeHost {
	return &FakeHost{
		Docker:     DockerStatus{Enabled: false},
		Processes:  map[int32]ProcessDetail{},
		Containers: map[string]ContainerDetail{},
	}
}

// --- FS capability (real temp-dir backed) ---

func (f *FakeHost) Stat(path string) (FileInfo, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return FileInfo{}, err
	}
	return toFileInfo(path, fi), nil
}

func (f *FakeHost) List(dir string) ([]FileInfo, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	out := make([]FileInfo, 0, len(entries))
	for _, e := range entries {
		fi, err := e.Info()
		if err != nil {
			continue
		}
		out = append(out, toFileInfo(filepath.Join(dir, e.Name()), fi))
	}
	return out, nil
}

func (f *FakeHost) Walk(root string) ([]FileInfo, error) {
	var out []FileInfo
	err := filepath.Walk(root, func(path string, fi os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if fi.IsDir() {
			return nil
		}
		out = append(out, toFileInfo(path, fi))
		return nil
	})
	return out, err
}

func (f *FakeHost) Open(path string) (io.ReadCloser, error) {
	return os.Open(path)
}

func (f *FakeHost) Rename(oldPath, newPath string) error {
	return os.Rename(oldPath, newPath)
}

func (f *FakeHost) Remove(path string) error {
	return os.Remove(path)
}

func (f *FakeHost) MkdirAll(path string) error {
	return os.MkdirAll(path, 0o755)
}

// --- Metrics capability ---

func (f *FakeHost) Sample() (Vitals, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.SampleErr != nil {
		return Vitals{}, f.SampleErr
	}
	return f.Vitals, nil
}

// SetVitals updates the canned vitals in a goroutine-safe way.
func (f *FakeHost) SetVitals(v Vitals) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Vitals = v
}

// SetProcessDetail pins the detail returned for a pid.
func (f *FakeHost) SetProcessDetail(d ProcessDetail) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Processes[d.PID] = d
}

func (f *FakeHost) ProcessDetail(pid int32) (ProcessDetail, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	d, ok := f.Processes[pid]
	if !ok {
		return ProcessDetail{}, ErrNotFound
	}
	return d, nil
}

// --- Docker capability ---

func (f *FakeHost) Status() DockerStatus {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.Docker
}

// SetDocker updates the canned docker status.
func (f *FakeHost) SetDocker(d DockerStatus) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Docker = d
}

// SetContainerDetail pins the detail returned for a container id.
func (f *FakeHost) SetContainerDetail(d ContainerDetail) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Containers[d.ID] = d
}

func (f *FakeHost) ContainerDetail(id string) (ContainerDetail, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.Docker.Enabled {
		return ContainerDetail{}, ErrDockerUnavailable
	}
	d, ok := f.Containers[id]
	if !ok {
		return ContainerDetail{}, ErrNotFound
	}
	return d, nil
}

var _ Host = (*FakeHost)(nil)
