package store

import "time"

// ring is a fixed-capacity FIFO buffer of entries. Once full, pushing
// overwrites the oldest entry. Iteration order is oldest to newest.
type ring struct {
	entries []Entry
	head    int // index of the oldest entry
	count   int
}

func newRing(capacity int) *ring {
	if capacity < 1 {
		capacity = 1
	}
	return &ring{entries: make([]Entry, capacity)}
}

// push appends an entry, overwriting the oldest one when the buffer is full.
func (r *ring) push(e Entry) {
	if r.count < len(r.entries) {
		r.entries[(r.head+r.count)%len(r.entries)] = e
		r.count++
		return
	}
	r.entries[r.head] = e
	r.head = (r.head + 1) % len(r.entries)
}

// slice returns the entries in chronological order.
func (r *ring) slice() []Entry {
	out := make([]Entry, r.count)
	for i := 0; i < r.count; i++ {
		out[i] = r.entries[(r.head+i)%len(r.entries)]
	}
	return out
}

// each iterates entries in chronological order until fn returns false.
func (r *ring) each(fn func(Entry) bool) {
	for i := 0; i < r.count; i++ {
		if !fn(r.entries[(r.head+i)%len(r.entries)]) {
			return
		}
	}
}

// uptime returns the share of successful entries within the window ending
// at now, or nil if there are no entries in the window.
func (r *ring) uptime(window time.Duration, now time.Time) *float64 {
	cutoff := now.Add(-window)
	total, success := 0, 0
	r.each(func(e Entry) bool {
		if !e.Timestamp.Before(cutoff) {
			total++
			if e.Success {
				success++
			}
		}
		return true
	})
	if total == 0 {
		return nil
	}
	ratio := float64(success) / float64(total) * 100
	return &ratio
}
