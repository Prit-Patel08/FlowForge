package supervisor

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

const defaultGrace = 5 * time.Second

// Supervisor starts one command in a dedicated process group and guarantees
// bounded teardown for the whole group.
type Supervisor struct {
	cmd *exec.Cmd

	mu      sync.Mutex
	waitErr error
	pid     int

	waitCh   chan struct{}
	started  bool
	stopOnce sync.Once
	stopErr  error
}

func New(cmd *exec.Cmd) *Supervisor {
	return &Supervisor{
		cmd:    cmd,
		waitCh: make(chan struct{}),
	}
}

func (s *Supervisor) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.started {
		return errors.New("supervisor: command already started")
	}

	if s.cmd == nil {
		return errors.New("supervisor: nil command")
	}

	if s.cmd.SysProcAttr == nil {
		s.cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	s.cmd.SysProcAttr.Setpgid = true

	if err := s.cmd.Start(); err != nil {
		return err
	}

	s.pid = s.cmd.Process.Pid
	s.started = true

	go func() {
		err := s.cmd.Wait()
		s.mu.Lock()
		s.waitErr = err
		s.mu.Unlock()
		close(s.waitCh)
	}()

	return nil
}

func (s *Supervisor) PID() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.pid
}

func (s *Supervisor) Exited() bool {
	select {
	case <-s.waitCh:
		return true
	default:
		return false
	}
}

func (s *Supervisor) Wait() error {
	<-s.waitCh
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.waitErr
}

// Stop sends SIGTERM to the process group, waits for grace duration, then
// escalates to SIGKILL for the process group if needed.
func (s *Supervisor) Stop(grace time.Duration) error {
	s.stopOnce.Do(func() {
		s.stopErr = s.stopInternal(grace)
	})
	return s.stopErr
}

// TrapSignals binds OS signals and invokes Stop when one is received.
// The returned function must be called to stop signal notification.
func (s *Supervisor) TrapSignals(grace time.Duration, onSignal func(os.Signal), sigs ...os.Signal) func() {
	if len(sigs) == 0 {
		sigs = []os.Signal{syscall.SIGINT, syscall.SIGTERM}
	}

	sigCh := make(chan os.Signal, 1)
	done := make(chan struct{})
	signal.Notify(sigCh, sigs...)

	go func() {
		select {
		case <-done:
			return
		case sig := <-sigCh:
			if onSignal != nil {
				onSignal(sig)
			}
			_ = s.Stop(grace)
		}
	}()

	return func() {
		close(done)
		signal.Stop(sigCh)
	}
}

func (s *Supervisor) stopInternal(grace time.Duration) error {
	pid := s.PID()
	if pid <= 0 {
		return nil
	}

	if grace <= 0 {
		grace = defaultGrace
	}

	termErr := signalGroup(pid, syscall.SIGTERM)
	if waitForGroupExit(pid, grace) {
		return nil
	}

	killErr := signalGroup(pid, syscall.SIGKILL)
	if !waitForGroupExit(pid, 2*time.Second) {
		return fmt.Errorf("supervisor: process group %d did not exit after SIGKILL", pid)
	}

	if termErr != nil && killErr != nil {
		return fmt.Errorf("supervisor: SIGTERM failed: %v; SIGKILL failed: %v", termErr, killErr)
	}
	if killErr != nil {
		return fmt.Errorf("supervisor: SIGKILL failed: %v", killErr)
	}
	return nil
}

func signalGroup(pid int, sig syscall.Signal) error {
	if pid <= 0 {
		return nil
	}

	// Negative PID targets the process group created by Setpgid=true.
	groupErr := syscall.Kill(-pid, sig)
	if groupErr == nil {
		return nil
	}

	// Fallback: signal the direct child if group signaling fails.
	procErr := syscall.Kill(pid, sig)
	if procErr == nil {
		return nil
	}

	// ESRCH means process already gone; treat as success.
	if errors.Is(groupErr, syscall.ESRCH) || errors.Is(procErr, syscall.ESRCH) {
		return nil
	}

	return procErr
}

func waitForGroupExit(pid int, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if !processGroupExists(pid) {
			return true
		}
		time.Sleep(25 * time.Millisecond)
	}
	return !processGroupExists(pid)
}

func processGroupExists(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(-pid, 0)
	if err == nil {
		return true
	}
	// EPERM means the process group exists but is not signalable.
	return errors.Is(err, syscall.EPERM)
}
