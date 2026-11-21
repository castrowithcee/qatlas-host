package service

import (
	"context"
	"testing"
	"time"
)

// A call for a stack whose check is running waits for that check and starts none of its own: the token of the
// running pass is held, so a check of its own would wait until its context ends, and this service has no
// stacks to read.
func TestCheckStackJoinsTheRunningCheckOfTheStack(t *testing.T) {
	p := &pollService{sem: make(chan struct{}, 1), pending: map[string]chan struct{}{}}
	running := make(chan struct{})
	p.pending["demo-stack"] = running
	p.sem <- struct{}{}
	// the running check has just ended; its entry goes once it has told everyone waiting
	close(running)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	p.CheckStack(ctx, "demo-stack")
	if ctx.Err() != nil || p.pending["demo-stack"] != running || len(p.sem) != 1 {
		t.Fatalf("the call did not join the running check: ctx %v, pending %v", ctx.Err(), p.pending)
	}
}
