package stream

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
)

// ParseSSE 解析 SSE 流
func ParseSSE(r io.Reader, handler func(event string, data []byte)) error {
	scanner := bufio.NewScanner(r)
	var buf bytes.Buffer

	for scanner.Scan() {
		line := scanner.Text()

		if line == "" {
			// 空行表示事件结束
			if buf.Len() > 0 {
				handler("", buf.Bytes())
				buf.Reset()
			}
			continue
		}

		if len(line) > 6 && line[:6] == "data: " {
			buf.WriteString(line[6:])
		}
	}

	return scanner.Err()
}

// WriteSSEEvent 写入 SSE 事件
func WriteSSEEvent(w io.Writer, event string, data []byte) error {
	if event != "" {
		if _, err := fmt.Fprintf(w, "event: %s\n", event); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintf(w, "data: %s\n\n", data); err != nil {
		return err
	}
	return nil
}
