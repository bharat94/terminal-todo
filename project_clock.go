package main

import (
	"time"

	"github.com/bharat94/terminal-todo/internal/projectclock"
)

func projectNow() time.Time {
	return projectclock.Now()
}
