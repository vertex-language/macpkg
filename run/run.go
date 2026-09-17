package run

import (
	"context"
	"errors"
	"io"
	"os/exec"
	"sync"
)

// Runner represents a process execution abstraction for external hooks, signing, and msiexec.
type Runner interface {
	Run(ctx context.Context, cmd string, args []string, dir string, env []string, stdout, stderr io.Writer) error
}

type osRunner struct{}

// OS returns a Runner backed by the host operating system's exec.CommandContext.
func OS() Runner {
	return &osRunner{}
}

func (r *osRunner) Run(ctx context.Context, cmd string, args []string, dir string, env []string, stdout, stderr io.Writer) error {
	c := exec.CommandContext(ctx, cmd, args...)
	if dir != "" {
		c.Dir = dir
	}
	if len(env) > 0 {
		c.Env = env
	}
	c.Stdout = stdout
	c.Stderr = stderr
	return c.Run()
}

// MockRunner records invocations and can return preset responses.
type MockRunner struct {
	mu          sync.Mutex
	Invocations []Invocation
	Handlers    map[string]func(args []string, stdout, stderr io.Writer) error
}

type Invocation struct {
	Cmd  string
	Args []string
	Dir  string
	Env  []string
}

// NewMock creates a new MockRunner for hermetic testing.
func NewMock() *MockRunner {
	return &MockRunner{
		Handlers: make(map[string]func(args []string, stdout, stderr io.Writer) error),
	}
}

func (m *MockRunner) Run(ctx context.Context, cmd string, args []string, dir string, env []string, stdout, stderr io.Writer) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.Invocations = append(m.Invocations, Invocation{
		Cmd:  cmd,
		Args: args,
		Dir:  dir,
		Env:  env,
	})

	if handler, ok := m.Handlers[cmd]; ok {
		return handler(args, stdout, stderr)
	}
	return nil
}

func (m *MockRunner) SetHandler(cmd string, fn func(args []string, stdout, stderr io.Writer) error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Handlers[cmd] = fn
}

var ErrProcessFailed = errors.New("process failed")
