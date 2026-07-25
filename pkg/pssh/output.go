package pssh

import (
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
	"sync/atomic"
)

const outputChunkSize int64 = 32 * 1024

// nolint:gochecknoglobals
var copyBufferPool = sync.Pool{
	New: func() any {
		buffer := make([]byte, outputChunkSize)
		return &buffer
	},
}

type memoryBudget struct {
	limit int64
	used  atomic.Int64
}

func newMemoryBudget(limit int64) *memoryBudget {
	return &memoryBudget{limit: limit}
}

func (b *memoryBudget) TryReserve(size int64) bool {
	if size < 0 {
		return false
	}
	for {
		used := b.used.Load()
		if size > b.limit || used > b.limit-size {
			return false
		}
		if b.used.CompareAndSwap(used, used+size) {
			return true
		}
	}
}

func (b *memoryBudget) Release(size int64) {
	if size > 0 {
		b.used.Add(-size)
	}
}

func (b *memoryBudget) Used() int64 {
	return b.used.Load()
}

type resultOutput interface {
	io.Writer
	WriteTo(io.Writer) (int64, error)
	Finalize() error
	Close() error
	Err() error
	Size() int64
}

type outputChunk struct {
	data []byte
	used int
}

type spillBuffer struct {
	memoryBudget *memoryBudget
	spoolBudget  *memoryBudget
	createFile   func() (*os.File, error)

	chunks         []*outputChunk
	memoryReserved int64
	spoolReserved  int64
	filePath       string
	file           *os.File
	size           int64
	err            error
	finalized      bool
}

func newSpillBuffer(
	memoryBudget *memoryBudget,
	spoolBudget *memoryBudget,
	createFile func() (*os.File, error),
) *spillBuffer {
	return &spillBuffer{
		memoryBudget: memoryBudget,
		spoolBudget:  spoolBudget,
		createFile:   createFile,
	}
}

func (b *spillBuffer) Write(data []byte) (int, error) {
	originalLen := len(data)
	if originalLen == 0 {
		return 0, nil
	}
	if b.finalized {
		b.setError(errors.New("cannot write finalized output"))
		return originalLen, nil
	}
	if b.err != nil {
		return originalLen, nil
	}
	if b.file != nil {
		b.writeToFile(data)
		return originalLen, nil
	}

	for len(data) > 0 {
		if len(b.chunks) > 0 {
			last := b.chunks[len(b.chunks)-1]
			if last.used < len(last.data) {
				n := copy(last.data[last.used:], data)
				last.used += n
				b.size += int64(n)
				data = data[n:]
				continue
			}
		}
		if !b.memoryBudget.TryReserve(outputChunkSize) {
			if err := b.spillToDisk(); err != nil {
				b.setError(err)
				return originalLen, nil
			}
			b.writeToFile(data)
			return originalLen, nil
		}
		b.memoryReserved += outputChunkSize
		b.chunks = append(b.chunks, &outputChunk{data: make([]byte, outputChunkSize)})
	}
	return originalLen, nil
}

func (b *spillBuffer) spillToDisk() error {
	file, err := b.createFile()
	if err != nil {
		return fmt.Errorf("create output spool file: %w", err)
	}

	var reserved int64
	for _, chunk := range b.chunks {
		size := int64(chunk.used)
		if !b.spoolBudget.TryReserve(size) {
			_ = file.Close()
			_ = os.Remove(file.Name())
			b.spoolBudget.Release(reserved)
			return fmt.Errorf("maximum spool size of %d bytes exceeded", b.spoolBudget.limit)
		}
		reserved += size
		n, writeErr := file.Write(chunk.data[:chunk.used])
		if int64(n) != size || writeErr != nil {
			_ = file.Close()
			_ = os.Remove(file.Name())
			b.spoolBudget.Release(reserved)
			if writeErr == nil {
				writeErr = io.ErrShortWrite
			}
			return fmt.Errorf("write output spool file: %w", writeErr)
		}
	}

	b.file = file
	b.filePath = file.Name()
	b.spoolReserved = reserved
	b.releaseMemory()
	return nil
}

func (b *spillBuffer) writeToFile(data []byte) {
	size := int64(len(data))
	if !b.spoolBudget.TryReserve(size) {
		b.setError(fmt.Errorf("maximum spool size of %d bytes exceeded", b.spoolBudget.limit))
		_ = b.closeFile()
		return
	}
	n, err := b.file.Write(data)
	b.spoolReserved += int64(n)
	b.size += int64(n)
	b.spoolBudget.Release(size - int64(n))
	if err == nil && n != len(data) {
		err = io.ErrShortWrite
	}
	if err != nil {
		b.setError(fmt.Errorf("write output spool file: %w", err))
		_ = b.closeFile()
	}
}

func (b *spillBuffer) Finalize() error {
	if b.finalized {
		return b.err
	}
	b.finalized = true
	if err := b.closeFile(); err != nil {
		b.setError(fmt.Errorf("close output spool file: %w", err))
	}
	return b.err
}

func (b *spillBuffer) WriteTo(dst io.Writer) (int64, error) {
	if err := b.Finalize(); err != nil && b.size == 0 {
		return 0, err
	}
	if b.filePath != "" {
		file, err := os.Open(b.filePath)
		if err != nil {
			return 0, fmt.Errorf("open output spool file: %w", err)
		}
		written, copyErr := io.Copy(dst, file)
		closeErr := file.Close()
		return written, errors.Join(copyErr, closeErr)
	}

	var total int64
	for _, chunk := range b.chunks {
		n, err := dst.Write(chunk.data[:chunk.used])
		total += int64(n)
		if err != nil {
			return total, err
		}
		if n != chunk.used {
			return total, io.ErrShortWrite
		}
	}
	return total, nil
}

func (b *spillBuffer) Close() error {
	b.finalized = true
	closeErr := b.closeFile()
	var removeErr error
	if b.filePath != "" {
		if err := os.Remove(b.filePath); err != nil && !errors.Is(err, os.ErrNotExist) {
			removeErr = err
		}
		b.filePath = ""
	}
	b.releaseMemory()
	b.spoolBudget.Release(b.spoolReserved)
	b.spoolReserved = 0
	b.chunks = nil
	b.size = 0
	return errors.Join(closeErr, removeErr)
}

func (b *spillBuffer) Err() error {
	return b.err
}

func (b *spillBuffer) Size() int64 {
	return b.size
}

func (b *spillBuffer) setError(err error) {
	if b.err == nil {
		b.err = err
	}
}

func (b *spillBuffer) closeFile() error {
	if b.file == nil {
		return nil
	}
	err := b.file.Close()
	b.file = nil
	return err
}

func (b *spillBuffer) releaseMemory() {
	b.memoryBudget.Release(b.memoryReserved)
	b.memoryReserved = 0
	b.chunks = nil
}
