package main

import (
	"fmt"
	"os"
	"time"
)

// tracef writes to /tmp/repomon-trace.log when REPOMON_TRACE is set.
// used during development to follow execution across goroutines.
func tracef(format string, args ...any) {
	if os.Getenv("REPOMON_TRACE") == "" {
		return
	}
	f, err := os.OpenFile("/tmp/repomon-trace.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintf(f, "%s "+format+"\n", append([]any{time.Now().Format("15:04:05.000")}, args...)...)
}
