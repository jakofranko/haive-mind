package inbox_test

import (
	"testing"
	"time"

	"jakofranko/haive-mind/tui/inbox"
)

const wait = 2 * time.Second

func popAsync[T any](q *inbox.Queue[T]) <-chan T {
	ch := make(chan T, 1)
	go func() { ch <- q.Pop() }()
	return ch
}

func TestPopBlocksUntilPush(t *testing.T) {
	q := inbox.New[int]()
	got := popAsync(q)

	select {
	case v := <-got:
		t.Fatalf("Pop returned %d before any push", v)
	case <-time.After(50 * time.Millisecond):
	}

	q.Push(7)
	select {
	case v := <-got:
		if v != 7 {
			t.Fatalf("Pop = %d, want 7", v)
		}
	case <-time.After(wait):
		t.Fatal("Pop did not return after push")
	}
}

func TestPopIsFirstInFirstOut(t *testing.T) {
	q := inbox.New[int]()
	for i := 0; i < 5; i++ {
		q.Push(i)
	}
	for i := 0; i < 5; i++ {
		if v := q.Pop(); v != i {
			t.Fatalf("Pop #%d = %d", i, v)
		}
	}
}

func TestPushNeverBlocksOrDrops(t *testing.T) {
	q := inbox.New[int]()
	const n = 10000
	done := make(chan struct{})
	go func() {
		for i := 0; i < n; i++ {
			q.Push(i)
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(wait):
		t.Fatal("Push blocked with no consumer")
	}
	for i := 0; i < n; i++ {
		if v := q.Pop(); v != i {
			t.Fatalf("Pop #%d = %d (item dropped or reordered)", i, v)
		}
	}
}

func TestNextWaitingIsReleasedByClose(t *testing.T) {
	q := inbox.New[int]()
	type res struct {
		v  int
		ok bool
	}
	got := make(chan res, 1)
	go func() { v, ok := q.Next(); got <- res{v, ok} }()

	time.Sleep(50 * time.Millisecond)
	q.Close()
	select {
	case r := <-got:
		if r.ok {
			t.Fatalf("Next = (%d, true) after close, want ok=false", r.v)
		}
	case <-time.After(wait):
		t.Fatal("waiting Next was not released by Close")
	}
}

func TestNextDeliversItemsPushedBeforeCloseThenReportsClosed(t *testing.T) {
	q := inbox.New[int]()
	q.Push(1)
	q.Push(2)
	q.Close()
	for _, want := range []int{1, 2} {
		v, ok := q.Next()
		if !ok || v != want {
			t.Fatalf("Next = (%d, %v), want (%d, true)", v, ok, want)
		}
	}
	if _, ok := q.Next(); ok {
		t.Fatal("Next reported ok after drain of a closed queue")
	}
	if _, ok := q.Next(); ok {
		t.Fatal("Next on a drained closed queue must keep reporting closed")
	}
}

func TestNextBehavesLikePopWhileOpen(t *testing.T) {
	q := inbox.New[int]()
	q.Push(3)
	if v, ok := q.Next(); !ok || v != 3 {
		t.Fatalf("Next = (%d, %v), want (3, true)", v, ok)
	}
}

func TestCloseTwiceAndPushAfterCloseAreHarmless(t *testing.T) {
	q := inbox.New[int]()
	q.Close()
	q.Close()
	done := make(chan struct{})
	go func() {
		for i := 0; i < 1000; i++ {
			q.Push(i)
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(wait):
		t.Fatal("Push blocked after Close")
	}
}

func TestMultipleWaitersAllReleasedByClose(t *testing.T) {
	q := inbox.New[int]()
	const n = 3
	done := make(chan bool, n)
	for i := 0; i < n; i++ {
		go func() { _, ok := q.Next(); done <- ok }()
	}
	time.Sleep(50 * time.Millisecond)
	q.Close()
	for i := 0; i < n; i++ {
		select {
		case ok := <-done:
			if ok {
				t.Fatal("waiter got ok=true")
			}
		case <-time.After(wait):
			t.Fatal("a waiter was left blocked after Close")
		}
	}
}
