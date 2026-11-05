package main

import (
	"os"
	"os/exec"
	"time"
)

// The spike only drives windows it opened itself: the test editor (its own process), Calculator
// and Chrome with a fresh profile in the scratch folder. close() posts WM_CLOSE to that exact
// window, and a process is killed only when the spike started it (by PID).

type App struct {
	Name  string
	Win   Window
	close func()
}

func (a *App) Close() {
	if a != nil && a.close != nil {
		a.close()
		a.close = nil
	}
}

func ids(ws []Window) map[string]bool {
	m := map[string]bool{}
	for _, w := range ws {
		m[w.ID] = true
	}
	return m
}

func launchEditor(cli *CLI, title string) (*App, error) {
	self, err := os.Executable()
	if err != nil {
		return nil, err
	}
	cmd := exec.Command(self, "editor", "-title", title)
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	pid := cmd.Process.Pid
	go cmd.Wait()
	w, err := cli.WaitWindow("spike040", 15*time.Second, func(w Window) bool { return w.PID == pid })
	if err != nil {
		killTree(pid)
		return nil, err
	}
	return &App{Name: "editor", Win: w, close: func() {
		if !closeWindow(hwndOf(w.ID), 3*time.Second) || processAlive(pid) {
			killTree(pid)
		}
	}}, nil
}

// launchCalculator opens a new Calculator window. Calculator keeps its mode between starts, so
// a test that changes the mode puts Standard back before it closes the window.
func launchCalculator(cli *CLI) (*App, error) {
	before, _ := cli.ListWindows("CalculatorApp")
	seen := ids(before)
	wasRunning := len(before) > 0
	if err := exec.Command("calc.exe").Start(); err != nil {
		return nil, err
	}
	w, err := cli.WaitWindow("CalculatorApp", 20*time.Second, func(w Window) bool { return !seen[w.ID] })
	if err != nil {
		return nil, err
	}
	return &App{Name: "calculator", Win: w, close: func() {
		closeWindow(hwndOf(w.ID), 5*time.Second)
		if !wasRunning && processAlive(w.PID) {
			killTree(w.PID)
		}
	}}, nil
}
