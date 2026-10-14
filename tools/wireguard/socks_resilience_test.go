package main

import (
	"context"
	"net"
	"os"
	"runtime"
	"syscall"
	"testing"
	"time"
)

// flakyListener fails the first Accept calls with err and then hands out
// connections from conns; once they are used up it behaves as closed.
type flakyListener struct {
	errs  []error
	conns chan net.Conn
}

func (l *flakyListener) Accept() (net.Conn, error) {
	if len(l.errs) > 0 {
		err := l.errs[0]
		l.errs = l.errs[1:]
		return nil, err
	}
	c, ok := <-l.conns
	if !ok {
		return nil, net.ErrClosed
	}
	return c, nil
}

func (l *flakyListener) Close() error   { return nil }
func (l *flakyListener) Addr() net.Addr { return &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1)} }

func TestServeSurvivesAcceptError(t *testing.T) {
	emfile := &net.OpError{Op: "accept", Net: "tcp", Err: os.NewSyscallError("accept", syscall.EMFILE)}
	ln := &flakyListener{
		errs:  []error{emfile, emfile},
		conns: make(chan net.Conn, 1),
	}
	target, targetPeer := net.Pipe()
	defer targetPeer.Close()
	srv := &socksServer{dial: func(ctx context.Context, host string, port uint16) (net.Conn, error) {
		return target, nil
	}}
	done := make(chan error, 1)
	go func() { done <- srv.serve(ln) }()

	cli, srvSide := net.Pipe()
	defer cli.Close()
	ln.conns <- srvSide

	_ = cli.SetDeadline(time.Now().Add(5 * time.Second))
	if _, err := cli.Write([]byte{5, 1, 0}); err != nil {
		t.Fatal(err)
	}
	var rep [2]byte
	if _, err := readFull(cli, rep[:]); err != nil || rep[0] != 5 || rep[1] != 0 {
		t.Fatalf("method reply %v err %v", rep, err)
	}
	if _, err := cli.Write([]byte{5, 1, 0, 1, 10, 0, 0, 1, 0, 80}); err != nil {
		t.Fatal(err)
	}
	var resp [10]byte
	if _, err := readFull(cli, resp[:]); err != nil || resp[1] != 0 {
		t.Fatalf("connect reply %v err %v", resp, err)
	}
	if got := srv.acceptErrors.Load(); got != 2 {
		t.Errorf("acceptErrors = %d, want 2", got)
	}
	if got := srv.active.Load(); got != 1 {
		t.Errorf("active = %d, want 1", got)
	}

	close(ln.conns)
	cli.Close()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("serve returned %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("serve did not stop after the listener closed")
	}
	deadline := time.Now().Add(5 * time.Second)
	for srv.active.Load() != 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if got := srv.active.Load(); got != 0 {
		t.Errorf("active after close = %d, want 0", got)
	}
}

func readFull(c net.Conn, b []byte) (int, error) {
	n := 0
	for n < len(b) {
		m, err := c.Read(b[n:])
		n += m
		if err != nil {
			return n, err
		}
	}
	return n, nil
}

func TestPipeClosesBothSides(t *testing.T) {
	before := runtime.NumGoroutine()

	a1, a2 := net.Pipe()
	b1, b2 := net.Pipe()
	done := make(chan struct{})
	go func() { pipe(a2, b2); close(done) }()

	// One direction works.
	go func() { _, _ = a1.Write([]byte("hi")) }()
	buf := make([]byte, 2)
	_ = b1.SetReadDeadline(time.Now().Add(5 * time.Second))
	if _, err := readFull(b1, buf); err != nil || string(buf) != "hi" {
		t.Fatalf("got %q err %v", buf, err)
	}

	// The client side ends; pipe must wind down and close the other side.
	a1.Close()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("pipe did not return")
	}
	_ = b1.SetReadDeadline(time.Now().Add(5 * time.Second))
	if _, err := b1.Read(buf); err == nil {
		t.Error("remote side still open after pipe returned")
	}
	if _, err := a2.Write([]byte("x")); err == nil {
		t.Error("a2 not closed")
	}
	if _, err := b2.Write([]byte("x")); err == nil {
		t.Error("b2 not closed")
	}
	b1.Close()

	deadline := time.Now().Add(2 * time.Second)
	for runtime.NumGoroutine() > before && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if after := runtime.NumGoroutine(); after > before {
		t.Errorf("goroutines: before %d, after %d", before, after)
	}
}

func TestPipeHalfCloseIdleReaped(t *testing.T) {
	old := pipeIdle
	pipeIdle = 100 * time.Millisecond
	defer func() { pipeIdle = old }()

	a1, a2 := net.Pipe()
	b1, b2 := net.Pipe()
	defer a1.Close()
	defer b1.Close()
	done := make(chan struct{})
	go func() { pipe(a2, b2); close(done) }()
	// a1 ends but b1 stays silent forever: the idle limit must reap it.
	a1.Close()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("pipe stuck after half-close with silent peer")
	}
}
