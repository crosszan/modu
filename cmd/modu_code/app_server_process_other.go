//go:build !darwin && !dragonfly && !freebsd && !linux && !netbsd && !openbsd

package main

import "os/exec"

func configureDetachedProcess(*exec.Cmd) {}
