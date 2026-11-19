package opencode

import (
	"bufio"
	"bytes"
)

func startsChatEvents(reader *bufio.Reader) bool {
	for offset := 0; offset < reader.Size(); offset++ {
		head, err := reader.Peek(offset + 1)
		if err != nil {
			return false
		}
		var prefix string
		switch head[offset] {
		case ' ', '\t', '\r', '\n':
			continue
		case ':':
			return true
		case 'd':
			prefix = "data:"
		case 'e':
			prefix = "event:"
		default:
			return false
		}
		head, err = reader.Peek(offset + len(prefix))
		return err == nil && string(head[offset:]) == prefix
	}
	return false
}

// chatTerminal observes only OpenAI chat's terminal SSE data event. It never
// buffers a conversation and a "[DONE]" inside model text is not completion.
type chatTerminal struct {
	line      []byte
	long      bool
	data      []byte
	hasData   bool
	eventLong bool
	done      bool
}

func (terminal *chatTerminal) write(body []byte) {
	for _, b := range body {
		if b != '\n' {
			if len(terminal.line) < 64 {
				terminal.line = append(terminal.line, b)
			} else {
				terminal.long = true
			}
			continue
		}
		line := bytes.TrimSuffix(terminal.line, []byte("\r"))
		if len(line) == 0 && !terminal.long {
			if terminal.hasData && !terminal.eventLong && bytes.Equal(terminal.data, []byte("[DONE]")) {
				terminal.done = true
			}
			terminal.data, terminal.eventLong, terminal.hasData = nil, false, false
		} else if terminal.long {
			if bytes.HasPrefix(line, []byte("data:")) {
				terminal.eventLong = true
			}
		} else if value, data := bytes.CutPrefix(line, []byte("data:")); data && !terminal.eventLong {
			value = bytes.TrimPrefix(value, []byte(" "))
			length := len(terminal.data) + len(value)
			if terminal.hasData {
				length++
			}
			if length > 32 {
				terminal.eventLong = true
			} else {
				if terminal.hasData {
					terminal.data = append(terminal.data, '\n')
				}
				terminal.data = append(terminal.data, value...)
				terminal.hasData = true
			}
		}
		terminal.line, terminal.long = terminal.line[:0], false
	}
}
