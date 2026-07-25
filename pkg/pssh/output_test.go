package pssh

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestSpillBufferStaysInMemoryWithinBudget(t *testing.T) {
	memory := newMemoryBudget(2 * outputChunkSize)
	spool := newMemoryBudget(1 << 20)
	output := newSpillBuffer(memory, spool, func() (*os.File, error) {
		return nil, errors.New("spool file must not be created")
	})

	data := []byte("hello from memory")
	if n, err := output.Write(data); err != nil || n != len(data) {
		t.Fatalf("Write() n=%d err=%v", n, err)
	}
	if err := output.Finalize(); err != nil {
		t.Fatal(err)
	}
	if memory.Used() != outputChunkSize {
		t.Errorf("memory used=%d, want %d", memory.Used(), outputChunkSize)
	}
	if spool.Used() != 0 {
		t.Errorf("spool used=%d, want 0", spool.Used())
	}

	var got bytes.Buffer
	if _, err := output.WriteTo(&got); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got.Bytes(), data) {
		t.Errorf("output=%q, want %q", got.Bytes(), data)
	}
	if err := output.Close(); err != nil {
		t.Fatal(err)
	}
	if memory.Used() != 0 {
		t.Errorf("memory used after Close=%d, want 0", memory.Used())
	}
}

func TestSpillBufferUsesSharedBudgetWithoutDataLoss(t *testing.T) {
	const (
		hostCount   = 10
		outputSize  = 512 << 10
		memoryLimit = 1 << 20
	)
	p := &Pssh{Config: &Config{
		MaxBufferMemory: memoryLimit,
		MaxSpoolSize:    10 << 20,
		SpoolDir:        t.TempDir(),
	}}
	p.Init()
	t.Cleanup(func() {
		_ = p.cleanupOutputStorage()
	})

	data := bytes.Repeat([]byte("x"), outputSize)
	outputs := make([]resultOutput, 0, hostCount)
	for range hostCount {
		output := newSpillBuffer(p.outputMemory, p.outputSpool, p.createOutputSpoolFile)
		if n, err := output.Write(data); err != nil || n != len(data) {
			t.Fatalf("Write() n=%d err=%v", n, err)
		}
		if err := output.Finalize(); err != nil {
			t.Fatal(err)
		}
		if output.file != nil {
			t.Fatal("spool file descriptor remains open after Finalize")
		}
		outputs = append(outputs, output)
	}

	if used := p.outputMemory.Used(); used > memoryLimit {
		t.Fatalf("memory used=%d, limit=%d", used, memoryLimit)
	}
	if p.outputSpool.Used() == 0 {
		t.Fatal("expected output to spill to disk")
	}
	info, err := os.Stat(p.outputSpoolDir)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o700 {
		t.Errorf("spool directory mode=%o, want 700", info.Mode().Perm())
	}
	files, err := filepath.Glob(filepath.Join(p.outputSpoolDir, "output-*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatal("no spool files were created")
	}
	for _, path := range files {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o600 {
			t.Errorf("spool file mode=%o, want 600", info.Mode().Perm())
		}
	}

	for _, output := range outputs {
		var got bytes.Buffer
		if _, err := output.WriteTo(&got); err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got.Bytes(), data) {
			t.Fatalf("spilled output differs: got %d bytes, want %d", got.Len(), len(data))
		}
		if err := output.Close(); err != nil {
			t.Fatal(err)
		}
	}
	if p.outputMemory.Used() != 0 || p.outputSpool.Used() != 0 {
		t.Errorf("budgets not released: memory=%d spool=%d", p.outputMemory.Used(), p.outputSpool.Used())
	}
	remaining, err := filepath.Glob(filepath.Join(p.outputSpoolDir, "output-*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(remaining) != 0 {
		t.Errorf("spool files remain after Close: %v", remaining)
	}
	spoolDir := p.outputSpoolDir
	if err := p.cleanupOutputStorage(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(spoolDir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("spool directory still exists: %v", err)
	}
}

func TestSpillBufferReportsSpoolLimitAndContinuesDraining(t *testing.T) {
	memory := newMemoryBudget(outputChunkSize)
	spool := newMemoryBudget(1)
	tempDir := t.TempDir()
	output := newSpillBuffer(memory, spool, func() (*os.File, error) {
		return os.CreateTemp(tempDir, "output-*")
	})
	data := bytes.Repeat([]byte("x"), int(outputChunkSize+1))

	n, err := output.Write(data)
	if err != nil || n != len(data) {
		t.Fatalf("Write() n=%d err=%v, want n=%d err=nil", n, err, len(data))
	}
	if output.Err() == nil {
		t.Fatal("Err()=nil, want spool size error")
	}
	if err := output.Finalize(); err == nil {
		t.Fatal("Finalize() error=nil, want spool size error")
	}
	if err := output.Close(); err != nil {
		t.Fatal(err)
	}
	if memory.Used() != 0 || spool.Used() != 0 {
		t.Errorf("budgets not released: memory=%d spool=%d", memory.Used(), spool.Used())
	}
}

func TestSpillBufferReportsSpoolCreationFailure(t *testing.T) {
	memory := newMemoryBudget(outputChunkSize)
	spool := newMemoryBudget(1 << 20)
	want := errors.New("permission denied")
	output := newSpillBuffer(memory, spool, func() (*os.File, error) {
		return nil, want
	})

	data := bytes.Repeat([]byte("x"), int(outputChunkSize+1))
	if n, err := output.Write(data); err != nil || n != len(data) {
		t.Fatalf("Write() n=%d err=%v", n, err)
	}
	if !errors.Is(output.Err(), want) {
		t.Fatalf("Err()=%v, want %v", output.Err(), want)
	}
	if err := output.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestSortedSpilledOutputPreservesHostOrder(t *testing.T) {
	p := &Pssh{Config: &Config{
		MaxBufferMemory: outputChunkSize,
		MaxSpoolSize:    1 << 20,
		SpoolDir:        t.TempDir(),
		SortPrint:       true,
	}}
	p.Init()
	t.Cleanup(func() {
		_ = p.cleanupOutputStorage()
	})

	var stdout bytes.Buffer
	p.print = newPrint(&stdout, io.Discard, false)
	results := make(chan *result, 3)
	cws := []*conWork{{host: "host0"}, {host: "host1"}, {host: "host2"}}
	wantParts := [][]byte{
		bytes.Repeat([]byte("0"), int(outputChunkSize+1)),
		bytes.Repeat([]byte("1"), int(outputChunkSize+1)),
		bytes.Repeat([]byte("2"), int(outputChunkSize+1)),
	}
	for _, id := range []int{2, 0, 1} {
		res := p.newResult(id, 0)
		if _, err := res.stdout.Write(wantParts[id]); err != nil {
			t.Fatal(err)
		}
		if err := res.stdout.Finalize(); err != nil {
			t.Fatal(err)
		}
		if err := res.stderr.Finalize(); err != nil {
			t.Fatal(err)
		}
		results <- res
	}

	if code := p.printSortResults(context.Background(), results, cws); code != 0 {
		t.Fatalf("printSortResults()=%d, want 0", code)
	}
	want := bytes.Join(wantParts, nil)
	if !bytes.Equal(stdout.Bytes(), want) {
		t.Fatalf("sorted output differs: got %d bytes, want %d", stdout.Len(), len(want))
	}
}

func TestCanceledResultRemovesSpoolFile(t *testing.T) {
	p := &Pssh{Config: &Config{
		MaxBufferMemory: outputChunkSize,
		MaxSpoolSize:    1 << 20,
		SpoolDir:        t.TempDir(),
	}}
	p.Init()
	t.Cleanup(func() {
		_ = p.cleanupOutputStorage()
	})
	res := p.newResult(0, 0)
	if _, err := res.stdout.Write(bytes.Repeat([]byte("x"), int(outputChunkSize+1))); err != nil {
		t.Fatal(err)
	}
	if err := res.stdout.Finalize(); err != nil {
		t.Fatal(err)
	}
	spilled := res.stdout.(*spillBuffer).filePath
	if spilled == "" {
		t.Fatal("output did not spill")
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	s := &sessionWork{
		con:   &conWork{Pssh: p},
		input: &input{results: make(chan *result)},
	}
	s.errResult(ctx, res)

	if _, err := os.Stat(spilled); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("spool file still exists: %v", err)
	}
	if p.outputMemory.Used() != 0 || p.outputSpool.Used() != 0 {
		t.Errorf("budgets not released: memory=%d spool=%d", p.outputMemory.Used(), p.outputSpool.Used())
	}
}
