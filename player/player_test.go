package player

import (
	"os"
	"os/exec"
	"runtime"
	"slices"
	"testing"
	"time"
)

func TestStreamArgsProtocolWhitelist(t *testing.T) {
	args := streamArgs("https://media.test/audio", 0)
	index := slices.Index(args, "-protocol_whitelist")
	if index < 0 || index+1 == len(args) || args[index+1] != "http,https,tcp,tls,crypto,file" {
		t.Fatalf("protocol whitelist args = %v", args)
	}
}

func TestStreamArgsKeepsLocalFilesAllowed(t *testing.T) {
	args := streamArgs("/tmp/audio.wav", 0)
	index := slices.Index(args, "-protocol_whitelist")
	if index < 0 || index+1 == len(args) || args[index+1] != "http,https,tcp,tls,crypto,file" {
		t.Fatalf("local-file protocol whitelist args = %v", args)
	}
}

func TestStreamArgsDoesNotAddHTTPOptionsToLocalFiles(t *testing.T) {
	args := streamArgs("/tmp/audio.wav", 0)
	if slices.Contains(args, "-reconnect") || slices.Contains(args, "-user_agent") {
		t.Fatalf("local file got HTTP-only options: %v", args)
	}
}

func TestReapObservesExitWhileReaderIsPaused(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test command uses sh")
	}
	p := &Player{}
	cmd := exec.Command("sh", "-c", "exit 7")
	pipe, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	cmd.Stdout = writer
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	_ = writer.Close()
	s := &session{cmd: cmd, done: make(chan struct{}), source: newCountingReader(pipe)}
	go p.reap(s)
	select {
	case <-s.done:
	case <-time.After(time.Second):
		t.Fatal("process exit was not observed while the reader was idle")
	}
	_ = pipe.Close()
}
