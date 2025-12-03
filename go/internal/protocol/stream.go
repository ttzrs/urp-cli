package protocol

import (
	"bufio"
	"io"
	"os/exec"
)

// StreamWriter wraps TaskReporter to implement io.Writer.
// Use this to capture command output and stream to master.
type StreamWriter struct {
	reporter *TaskReporter
	stream   string // "stdout" or "stderr"
}

// NewStreamWriter creates a writer that sends output to master.
func NewStreamWriter(reporter *TaskReporter, stream string) *StreamWriter {
	return &StreamWriter{
		reporter: reporter,
		stream:   stream,
	}
}

// Write implements io.Writer.
func (w *StreamWriter) Write(p []byte) (n int, err error) {
	w.reporter.Output(w.stream, string(p))
	return len(p), nil
}

// StdoutWriter returns a writer for stdout.
func (r *TaskReporter) StdoutWriter() io.Writer {
	return NewStreamWriter(r, "stdout")
}

// StderrWriter returns a writer for stderr.
func (r *TaskReporter) StderrWriter() io.Writer {
	return NewStreamWriter(r, "stderr")
}

// ─────────────────────────────────────────────────────────────────────────────
// Command execution with streaming
// ─────────────────────────────────────────────────────────────────────────────

// RunCommand executes a command and streams output to master.
func (r *TaskReporter) RunCommand(name string, args ...string) error {
	cmd := exec.Command(name, args...)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}

	if err := cmd.Start(); err != nil {
		return err
	}

	// Stream stdout
	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			r.Stdout(scanner.Text() + "\n")
		}
	}()

	// Stream stderr
	go func() {
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			r.Stderr(scanner.Text() + "\n")
		}
	}()

	return cmd.Wait()
}

// ─────────────────────────────────────────────────────────────────────────────
// Buffered output collector
// ─────────────────────────────────────────────────────────────────────────────

// OutputCollector collects output while optionally streaming.
type OutputCollector struct {
	reporter *TaskReporter
	stream   string
	buffer   []byte
	doStream bool
}

// NewOutputCollector creates a collector that optionally streams.
func NewOutputCollector(reporter *TaskReporter, stream string, doStream bool) *OutputCollector {
	return &OutputCollector{
		reporter: reporter,
		stream:   stream,
		doStream: doStream,
	}
}

// Write implements io.Writer.
func (c *OutputCollector) Write(p []byte) (n int, err error) {
	c.buffer = append(c.buffer, p...)
	if c.doStream && c.reporter != nil {
		c.reporter.Output(c.stream, string(p))
	}
	return len(p), nil
}

// String returns collected output.
func (c *OutputCollector) String() string {
	return string(c.buffer)
}

// Bytes returns collected output as bytes.
func (c *OutputCollector) Bytes() []byte {
	return c.buffer
}
