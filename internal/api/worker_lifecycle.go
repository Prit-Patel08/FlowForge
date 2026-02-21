package api

import (
	"errors"
	"flowforge/internal/state"
	"flowforge/internal/supervisor"
	"fmt"
	"os/exec"
	"sync"
	"syscall"
	"time"
)

const (
	lifecycleStarting = "STARTING"
	lifecycleRunning  = "RUNNING"
	lifecycleStopping = "STOPPING"
	lifecycleStopped  = "STOPPED"
	lifecycleFailed   = "FAILED"
)

const (
	opNone    = ""
	opKill    = "kill"
	opRestart = "restart"
)

type WorkerController interface {
	PID() int
	Exited() bool
	Stop(grace time.Duration) error
	Wait() error
}

type workerSpec struct {
	Command string
	Args    []string
	Dir     string
}

func (s workerSpec) valid() bool {
	return s.Command != "" && len(s.Args) > 0
}

type lifecycleAction struct {
	Status      string
	Lifecycle   string
	PID         int
	AcceptedNew bool
}

type lifecycleError struct {
	code int
	msg  string
}

func (e *lifecycleError) Error() string { return e.msg }

func newLifecycleError(code int, msg string) error {
	return &lifecycleError{code: code, msg: msg}
}

type workerLifecycle struct {
	mu sync.Mutex

	phase     string
	operation string

	spec workerSpec

	controller WorkerController
	managed    bool
	pid        int

	lastErr string
	watchID uint64

	lastAction    string
	lastActionAt  time.Time
	lastActionPID int
}

func newWorkerLifecycle() *workerLifecycle {
	return &workerLifecycle{phase: lifecycleStopped}
}

func (w *workerLifecycle) registerExternal(command string, args []string, dir string, controller WorkerController) {
	if controller == nil {
		return
	}
	w.mu.Lock()
	w.spec = workerSpec{
		Command: command,
		Args:    append([]string(nil), args...),
		Dir:     dir,
	}
	w.controller = controller
	w.managed = false
	w.pid = controller.PID()
	w.phase = lifecycleRunning
	w.operation = opNone
	w.lastErr = ""
	w.startWatcherLocked(controller, false)
	w.mu.Unlock()

	state.UpdateLifecycle(lifecycleRunning, "RUNNING", w.pid)
}

func (w *workerLifecycle) requestKill() (lifecycleAction, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.syncFromStateLocked()

	if w.operation == opRestart || w.phase == lifecycleStarting {
		return lifecycleAction{}, newLifecycleError(409, "restart already in progress")
	}
	if w.operation == opKill || w.phase == lifecycleStopping {
		return lifecycleAction{
			Status:      "stop_requested",
			Lifecycle:   lifecycleStopping,
			PID:         w.pid,
			AcceptedNew: false,
		}, nil
	}
	if !w.hasActiveWorkerLocked() {
		if w.isRecentActionLocked(opKill, 2*time.Second) {
			return lifecycleAction{
				Status:      "stop_requested",
				Lifecycle:   lifecycleStopped,
				PID:         w.lastActionPID,
				AcceptedNew: false,
			}, nil
		}
		return lifecycleAction{}, newLifecycleError(400, "no active process to kill")
	}

	pid := w.activePIDLocked()
	controller := w.controller
	w.phase = lifecycleStopping
	w.operation = opKill
	w.lastErr = ""
	w.lastAction = opKill
	w.lastActionAt = time.Now()
	w.lastActionPID = pid
	state.UpdateLifecycle(lifecycleStopping, "STOPPING", pid)

	go w.stopAsync(controller, pid, false)

	return lifecycleAction{
		Status:      "stop_requested",
		Lifecycle:   lifecycleStopping,
		PID:         pid,
		AcceptedNew: true,
	}, nil
}

func (w *workerLifecycle) requestRestart() (lifecycleAction, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.syncFromStateLocked()

	if w.operation == opKill || w.phase == lifecycleStopping {
		return lifecycleAction{}, newLifecycleError(409, "worker is stopping; retry restart after stop completes")
	}
	if w.operation == opRestart || w.phase == lifecycleStarting {
		return lifecycleAction{
			Status:      "restart_requested",
			Lifecycle:   lifecycleStarting,
			PID:         0,
			AcceptedNew: false,
		}, nil
	}
	if w.hasActiveWorkerLocked() {
		return lifecycleAction{}, newLifecycleError(409, "process is still running; stop/kill it before restart")
	}

	spec := w.spec
	if !spec.valid() {
		st := state.GetState()
		if st.Command != "" && len(st.Args) > 0 {
			spec = workerSpec{
				Command: st.Command,
				Args:    append([]string(nil), st.Args...),
				Dir:     st.Dir,
			}
			w.spec = spec
		}
	}
	if !spec.valid() {
		return lifecycleAction{}, newLifecycleError(400, "no command available to restart")
	}

	w.phase = lifecycleStarting
	w.operation = opRestart
	w.pid = 0
	w.lastErr = ""
	w.lastAction = opRestart
	w.lastActionAt = time.Now()
	w.lastActionPID = 0
	state.UpdateLifecycle(lifecycleStarting, "STARTING", 0)

	go w.startAsync(spec)

	return lifecycleAction{
		Status:      "restart_requested",
		Lifecycle:   lifecycleStarting,
		PID:         0,
		AcceptedNew: true,
	}, nil
}

func (w *workerLifecycle) stopAsync(controller WorkerController, pid int, fromWatcher bool) {
	var err error
	if controller != nil {
		err = controller.Stop(2 * time.Second)
	} else {
		// Keep a small deterministic STOPPING window so immediate repeated
		// mutation requests observe conflict/idempotent semantics.
		time.Sleep(75 * time.Millisecond)
		err = killProcessTree(pid)
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	// Ignore stale completions from previous watchers/controllers.
	if fromWatcher && w.operation != opNone && w.operation != opKill {
		return
	}

	if err != nil {
		w.phase = lifecycleFailed
		w.operation = opNone
		w.lastErr = err.Error()
		state.UpdateLifecycle(lifecycleFailed, "FAILED", w.pid)
		return
	}

	w.watchID++
	w.controller = nil
	w.managed = false
	w.pid = 0
	w.phase = lifecycleStopped
	w.operation = opNone
	w.lastErr = ""
	state.UpdateLifecycle(lifecycleStopped, "STOPPED", 0)
}

func (w *workerLifecycle) startAsync(spec workerSpec) {
	cmd := exec.Command(spec.Args[0], spec.Args[1:]...)
	if spec.Dir != "" {
		cmd.Dir = spec.Dir
	}

	sup := supervisor.New(cmd)
	if err := sup.Start(); err != nil {
		w.mu.Lock()
		w.phase = lifecycleFailed
		w.operation = opNone
		w.lastErr = err.Error()
		w.pid = 0
		w.controller = nil
		w.managed = false
		w.mu.Unlock()
		state.UpdateLifecycle(lifecycleFailed, "FAILED", 0)
		return
	}

	pid := sup.PID()
	w.mu.Lock()
	w.controller = sup
	w.managed = true
	w.pid = pid
	w.phase = lifecycleRunning
	w.operation = opNone
	w.lastErr = ""
	w.spec = spec
	w.startWatcherLocked(sup, true)
	w.mu.Unlock()

	state.UpdateState(0, "", "RUNNING", spec.Command, spec.Args, spec.Dir, pid)
	state.UpdateLifecycle(lifecycleRunning, "RUNNING", pid)
}

func (w *workerLifecycle) startWatcherLocked(controller WorkerController, managed bool) uint64 {
	w.watchID++
	id := w.watchID
	go w.waitForController(id, controller, managed)
	return id
}

func (w *workerLifecycle) waitForController(id uint64, controller WorkerController, managed bool) {
	err := controller.Wait()

	w.mu.Lock()
	defer w.mu.Unlock()
	if id != w.watchID {
		return
	}
	// If a stop operation is already in progress, the explicit stop finalizer owns state transition.
	if w.operation == opKill || w.phase == lifecycleStopping {
		return
	}

	w.controller = nil
	w.managed = false
	w.pid = 0
	w.operation = opNone

	if err != nil {
		w.phase = lifecycleFailed
		w.lastErr = err.Error()
		state.UpdateLifecycle(lifecycleFailed, "FAILED", 0)
		return
	}
	w.phase = lifecycleStopped
	w.lastErr = ""
	state.UpdateLifecycle(lifecycleStopped, "STOPPED", 0)
	_ = managed
}

func (w *workerLifecycle) hasActiveWorkerLocked() bool {
	if w.controller != nil {
		if !w.controller.Exited() {
			pid := w.controller.PID()
			if pid > 0 {
				w.pid = pid
			}
			return true
		}
	}
	if w.pid > 0 && processLikelyAlive(w.pid) {
		return true
	}
	return false
}

func (w *workerLifecycle) activePIDLocked() int {
	if w.controller != nil {
		if pid := w.controller.PID(); pid > 0 {
			return pid
		}
	}
	return w.pid
}

func (w *workerLifecycle) registerSpecFromStateIfMissing() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.syncFromStateLocked()
}

func (w *workerLifecycle) syncFromStateLocked() {
	st := state.GetState()
	if !w.spec.valid() && st.Command != "" && len(st.Args) > 0 {
		w.spec = workerSpec{
			Command: st.Command,
			Args:    append([]string(nil), st.Args...),
			Dir:     st.Dir,
		}
	}
	if w.controller == nil && st.PID > 0 && processLikelyAlive(st.PID) {
		w.pid = st.PID
		if w.phase == lifecycleStopped && w.operation == opNone {
			w.phase = lifecycleRunning
		}
	}
}

func (w *workerLifecycle) snapshot() map[string]interface{} {
	w.mu.Lock()
	defer w.mu.Unlock()
	return map[string]interface{}{
		"phase":     w.phase,
		"operation": w.operation,
		"pid":       w.pid,
		"managed":   w.managed,
		"last_err":  w.lastErr,
	}
}

func (w *workerLifecycle) setSpec(command string, args []string, dir string) {
	if command == "" || len(args) == 0 {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	w.spec = workerSpec{
		Command: command,
		Args:    append([]string(nil), args...),
		Dir:     dir,
	}
}

func (w *workerLifecycle) isRecentActionLocked(action string, window time.Duration) bool {
	if w.lastAction != action || w.lastActionAt.IsZero() {
		return false
	}
	return time.Since(w.lastActionAt) <= window
}

func processLikelyAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, 0)
	if err == nil {
		return true
	}
	return !errors.Is(err, syscall.ESRCH)
}

var workerControl = newWorkerLifecycle()

func RegisterExternalWorker(command string, args []string, dir string, controller WorkerController) {
	workerControl.registerExternal(command, args, dir, controller)
}

func SetWorkerSpec(command string, args []string, dir string) {
	workerControl.setSpec(command, args, dir)
}

func WorkerLifecycleSnapshot() map[string]interface{} {
	return workerControl.snapshot()
}

// ResetWorkerControlForTests resets global lifecycle state.
// Intended only for test isolation.
func ResetWorkerControlForTests() {
	workerControl = newWorkerLifecycle()
	state.UpdateLifecycle(lifecycleStopped, "STOPPED", 0)
}

func requestLifecycleKill() (lifecycleAction, error) {
	return workerControl.requestKill()
}

func requestLifecycleRestart() (lifecycleAction, error) {
	return workerControl.requestRestart()
}

func lifecycleHTTPCode(err error, fallback int) int {
	if err == nil {
		return fallback
	}
	var lerr *lifecycleError
	if errors.As(err, &lerr) {
		return lerr.code
	}
	return 500
}

func lifecycleErrorMessage(err error, fallback string) string {
	if err == nil {
		return fallback
	}
	var lerr *lifecycleError
	if errors.As(err, &lerr) {
		return lerr.msg
	}
	return fmt.Sprintf("internal lifecycle error: %v", err)
}
