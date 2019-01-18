package gantry // import "github.com/ad-freiburg/gantry"

import (
	"bytes"
	"fmt"
	"io"
)

type PrefixedWriter struct {
	prefix string
	target io.Writer
	buf    *bytes.Buffer
}

func NewPrefixedWriter(prefix string, target io.Writer) *PrefixedWriter {
	return &PrefixedWriter{
		prefix: prefix,
		target: target,
		buf:    bytes.NewBuffer([]byte("")),
	}
}

func (l *PrefixedWriter) Write(p []byte) (int, error) {
	n, err := l.buf.Write(p)
	if err != nil {
		return n, err
	}
	err = l.Output()
	return n, err
}

func (l *PrefixedWriter) Output() error {
	const format string = "\u001b[1m%s\u001b[0m %s\u001b[0m"
	for {
		line, err := l.buf.ReadString('\n')
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		fmt.Fprintf(l.target, format, l.prefix, line)
	}
	return nil
}
