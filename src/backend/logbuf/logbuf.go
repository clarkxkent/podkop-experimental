// Package logbuf keeps the last lines of the backend's log in memory, the backend's own
// and those of the sing-box inside it. The system log of a router is one small ring for
// everything on it; here the lines of this run are kept apart from the rest, and a reader
// asks for what is new since it last asked.
package logbuf

import (
	"strconv"
	"sync"
	"time"
)

// Line is one line of the log.
type Line struct {
	// Number counts the lines of this run from one.
	Number uint64
	Time   time.Time
	// Source is "backend" or "sing-box".
	Source string
	// Level is the level as the source names it: info, warn, error and so on.
	Level   string
	Message string
}

// Buffer holds lines up to a size; the oldest go first.
type Buffer struct {
	access sync.Mutex
	lines  []Line
	// bytes is the size of the messages held
	bytes int
	limit int
	last  uint64
	now   func() time.Time
	run   string
}

// New makes a buffer that holds about limit bytes of messages.
func New(limit int) *Buffer {
	return &Buffer{limit: limit, now: time.Now, run: strconv.FormatInt(time.Now().UnixNano(), 36)}
}

// Run names this buffer among those of other runs of the backend: the numbers of the lines
// start again at one in each, so a reader that keeps a number keeps the run with it.
func (b *Buffer) Run() string {
	return b.run
}

// Write adds a line.
func (b *Buffer) Write(source string, level string, message string) {
	b.access.Lock()
	defer b.access.Unlock()
	b.last++
	b.lines = append(b.lines, Line{Number: b.last, Time: b.now(), Source: source, Level: level, Message: message})
	b.bytes += len(message)
	drop := 0
	for b.bytes > b.limit && drop < len(b.lines)-1 {
		b.bytes -= len(b.lines[drop].Message)
		drop++
	}
	if drop > 0 {
		// The dropped lines let their messages go and the slice moves past them. append
		// copies the lines held into a new array only when the old one is full: a copy of
		// the buffer each time a part of it has been written over, not one for every line
		// (a full buffer holds a few thousand lines, and a proxy that is down writes one a
		// connection).
		clear(b.lines[:drop])
		b.lines = b.lines[drop:]
	}
}

// Since returns the lines after the one with the given number, all that are held for
// zero, and the number to ask with next time.
func (b *Buffer) Since(number uint64) ([]Line, uint64) {
	b.access.Lock()
	defer b.access.Unlock()
	for index := range b.lines {
		if b.lines[index].Number > number {
			return append([]Line(nil), b.lines[index:]...), b.last
		}
	}
	return nil, b.last
}

// Text writes a line the way the log of the shell backend reads in the user interface:
// the time, where the line is from, and "[level] message". The user interface looks for
// "[error]" and "[fatal]" in the text to raise a notice, so a line of sing-box, whose
// errors are about connections and not about the backend, has its level written without
// the brackets.
func (l Line) Text() string {
	stamp := l.Time.Format("2006-01-02 15:04:05")
	if l.Source == "sing-box" {
		return stamp + " sing-box: " + l.Level + ": " + l.Message
	}
	return stamp + " podkop: [" + l.Level + "] " + l.Message
}
