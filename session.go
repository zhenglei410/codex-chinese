package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type injectionStatus struct {
	Installed bool
	Lang      string
	I18n      string
	HTMLLang  string
}

type verifyPayload struct {
	Installed bool   `json:"installed"`
	Lang      string `json:"lang"`
	I18n      string `json:"i18n"`
	HTMLLang  string `json:"htmlLang"`
}

func supervisorLogPath() string {
	return filepath.Join(os.TempDir(), "codex-chinese-supervisor.log")
}

func supervisorLog(format string, args ...any) {
	f, err := os.OpenFile(supervisorLogPath(), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintf(f, "%s %s\n", time.Now().Format("2006-01-02 15:04:05"), fmt.Sprintf(format, args...))
}

// startSupervisor starts a detached copy of this executable. The child owns a
// long-lived CDP connection so page navigation does not silently remove the
// prelude after the launcher exits.
func startSupervisor(exe string, port int, noReload bool) error {
	self, err := os.Executable()
	if err != nil {
		return err
	}
	args := []string{
		"-supervise",
		"-port", strconv.Itoa(port),
		"-codex", exe,
	}
	if noReload {
		args = append(args, "-no-reload")
	}
	if *flagWait > 0 {
		args = append(args, "-wait", strconv.Itoa(*flagWait))
	}
	if guardPID, err := strconv.Atoi(os.Getenv("CODEX_CHINESE_GUARD_PID")); err == nil && guardPID > 0 {
		args = append(args, "-guard-pid", strconv.Itoa(guardPID))
	}

	logFile, err := os.OpenFile(supervisorLogPath(), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	cmd := exec.Command(self, args...)
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	applyDetach(cmd)
	if err := cmd.Start(); err != nil {
		_ = logFile.Close()
		return err
	}
	_ = logFile.Close()
	return cmd.Process.Release()
}

func runSupervisor(port int, exe string, guardPID int) {
	release, acquired := acquireSupervisorLock(port)
	if !acquired {
		supervisorLog("supervisor already running port=%d", port)
		return
	}
	defer release()
	supervisorLog("supervisor start port=%d codex=%s", port, exe)

	missingSince := time.Time{}
	unreadySince := time.Time{}
	restartAttempts := 0
	for {
		if guardPID > 0 && !processAlive(guardPID) {
			supervisorLog("guard process %d exited; supervisor stop", guardPID)
			return
		}
		ids := codexProcessIDs(exe)
		if len(ids) == 0 && !endpointReady(port) {
			if missingSince.IsZero() {
				missingSince = time.Now()
			}
			unreadySince = time.Time{}
			if guardPID == 0 && time.Since(missingSince) > 20*time.Second {
				supervisorLog("Codex exited; supervisor stop")
				return
			}
			time.Sleep(time.Second)
			continue
		}
		missingSince = time.Time{}
		if !endpointReady(port) {
			if unreadySince.IsZero() {
				unreadySince = time.Now()
			}
			if guardPID == 0 && time.Since(unreadySince) > 20*time.Second {
				supervisorLog("CDP endpoint unavailable without guard; supervisor stop")
				return
			}
			if guardPID > 0 && time.Since(unreadySince) > 12*time.Second {
				if restartAttempts >= 3 {
					supervisorLog("automatic restart failed 3 times; supervisor stop")
					return
				}
				restartAttempts++
				supervisorLog("Codex running without CDP; restarting pids=%v", ids)
				if terminated, err := terminateCodexProcesses(exe); err != nil {
					supervisorLog("automatic restart failed: %v", err)
				} else {
					supervisorLog("terminated Codex pids=%v", terminated)
					time.Sleep(2 * time.Second)
					if err := launchCodex(exe, port); err != nil {
						supervisorLog("automatic launch failed: %v", err)
					}
				}
				unreadySince = time.Time{}
			}
			time.Sleep(time.Second)
			continue
		}
		unreadySince = time.Time{}
		restartAttempts = 0

		t, err := pickTarget(port)
		if err != nil {
			supervisorLog("wait target: %v", err)
			time.Sleep(750 * time.Millisecond)
			continue
		}
		ws, err := dialWS(t.WSURL)
		if err != nil {
			supervisorLog("connect %s: %v", t.URL, err)
			time.Sleep(750 * time.Millisecond)
			continue
		}
		supervisorLog("attached target=%s", t.URL)

		err = keepInjectionSession(ws, port, guardPID)
		ws.Close()
		if err != nil {
			supervisorLog("session ended: %v", err)
		}
		time.Sleep(time.Second)
	}
}

func keepInjectionSession(ws *wsConn, port int, guardPID int) error {
	if _, err := ws.call(1, "Page.enable", map[string]any{}); err != nil {
		return fmt.Errorf("Page.enable: %w", err)
	}
	if _, err := ws.call(2, "Runtime.enable", map[string]any{}); err != nil {
		return fmt.Errorf("Runtime.enable: %w", err)
	}
	if _, err := ws.call(3, "Page.addScriptToEvaluateOnNewDocument", map[string]any{"source": prelude}); err != nil {
		return fmt.Errorf("register prelude: %w", err)
	}
	// Patch the current document as well as future documents. Runtime.evaluate
	// runs after the app has bootstrapped, so locale-setting retries can find the
	// application API that did not exist at document-start time.
	if _, err := ws.call(4, "Runtime.evaluate", map[string]any{
		"expression":    prelude,
		"returnByValue": false,
	}); err != nil {
		return fmt.Errorf("evaluate prelude: %w", err)
	}
	// Do not reload a freshly started Electron app. The prelude is evaluated in
	// the current document immediately and is also registered for all future
	// documents. Reloading during Codex bootstrap can abort its main app phase.
	if !waitInjected(ws, 8*time.Second) {
		return fmt.Errorf("prelude marker not observed in current document")
	}

	badChecks := 0
	for {
		if guardPID > 0 && !processAlive(guardPID) {
			return fmt.Errorf("guard process exited")
		}
		if !endpointReady(port) {
			return fmt.Errorf("CDP endpoint closed")
		}
		status, err := evaluateInjection(ws)
		if err != nil {
			badChecks++
		} else if injectionOK(status) {
			badChecks = 0
		} else {
			badChecks++
		}
		if badChecks >= 3 {
			return fmt.Errorf("injection health check failed %d times", badChecks)
		}
		time.Sleep(2 * time.Second)
	}
}

func evaluateInjection(ws *wsConn) (injectionStatus, error) {
	res, err := ws.call(901, "Runtime.evaluate", map[string]any{
		"expression":    verifyExpr,
		"returnByValue": true,
	})
	if err != nil {
		return injectionStatus{}, err
	}
	var wrap struct {
		Result struct {
			Value string `json:"value"`
		} `json:"result"`
	}
	if err := json.Unmarshal(res, &wrap); err != nil {
		return injectionStatus{}, err
	}
	var payload verifyPayload
	if err := json.Unmarshal([]byte(wrap.Result.Value), &payload); err != nil {
		return injectionStatus{}, err
	}
	return injectionStatus{
		Installed: payload.Installed,
		Lang:      payload.Lang,
		I18n:      payload.I18n,
		HTMLLang:  payload.HTMLLang,
	}, nil
}

func injectionOK(status injectionStatus) bool {
	return status.Installed &&
		strings.HasPrefix(strings.ToLower(status.Lang), "zh") &&
		status.I18n != "false"
}

func inspectInjection(port int) (injectionStatus, error) {
	if !endpointReady(port) {
		return injectionStatus{}, fmt.Errorf("cdp not-ready (127.0.0.1:%d)", port)
	}
	t, err := pickTarget(port)
	if err != nil {
		return injectionStatus{}, err
	}
	ws, err := dialWS(t.WSURL)
	if err != nil {
		return injectionStatus{}, err
	}
	defer ws.Close()
	return evaluateInjection(ws)
}

func waitForInjection(port int, timeout time.Duration) (injectionStatus, error) {
	deadline := time.Now().Add(timeout)
	var last injectionStatus
	var lastErr error
	for {
		last, lastErr = inspectInjection(port)
		if lastErr == nil && injectionOK(last) {
			return last, nil
		}
		if time.Now().After(deadline) {
			if lastErr != nil {
				return last, lastErr
			}
			return last, fmt.Errorf("注入已建立但中文状态未通过校验")
		}
		time.Sleep(400 * time.Millisecond)
	}
}
