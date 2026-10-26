//go:build !windows

package main

import (
	"errors"
	"os/exec"
)

// Job Objects are Windows-only; this stub lets the spike build (go vet)
// elsewhere. Nothing here runs.

var errWindows = errors.New("job objects are Windows-only")

type job struct{}

func newJob() (*job, error)                 { return nil, errWindows }
func suspended(*exec.Cmd)                   {}
func (j *job) adopt(int) error              { return errWindows }
func (j *job) pids() ([]int, error)         { return nil, errWindows }
func (j *job) watch(int) func()             { return func() {} }
func (j *job) closeOnly(int) ([]int, error) { return nil, errWindows }
func (j *job) release()                     {}
func (j *job) close() error                 { return nil }
func processes() (map[int]proc, error)      { return nil, errWindows }
func tree(int) []proc                       { return nil }
func still([]proc) []proc                   { return nil }
func killProc(proc, string) error           { return errWindows }
