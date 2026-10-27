// Copyright 2026 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package agentengine

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"iter"
	"strings"

	"google.golang.org/genai"
)

// Event is one line of a stream-query response.
type Event struct {
	Author string
	// Content is nil for lines without a content object, which some agent
	// frameworks emit.
	Content *genai.Content
	// Partial marks a streamed chunk of a reply. Partial chunks are followed
	// by one aggregated non-partial event carrying the full content, so
	// consumers that want each reply once skip partial events.
	Partial bool
	// Raw is the whole decoded line, for fields this type does not model.
	Raw map[string]any
}

// decodeJSONL reassembles JSONL across chunk boundaries and decodes each line.
// The server writes one JSON object per line and reports failure as a line
// {"error": "..."}, which becomes the iterator's error and ends the stream.
// A final line without a trailing newline is still decoded.
func decodeJSONL(chunks iter.Seq2[[]byte, error]) iter.Seq2[*Event, error] {
	return func(yield func(*Event, error) bool) {
		var buf []byte
		emit := func(line []byte) bool {
			line = bytes.TrimSpace(line)
			if len(line) == 0 {
				return true
			}
			ev, err := decodeLine(line)
			if err != nil {
				yield(nil, err)
				return false
			}
			return yield(ev, nil)
		}
		for chunk, err := range chunks {
			if err != nil {
				yield(nil, err)
				return
			}
			buf = append(buf, chunk...)
			for {
				i := bytes.IndexByte(buf, '\n')
				if i < 0 {
					break
				}
				line := buf[:i]
				buf = buf[i+1:]
				if !emit(line) {
					return
				}
			}
		}
		emit(buf)
	}
}

func decodeLine(line []byte) (*Event, error) {
	var raw map[string]any
	if err := json.Unmarshal(line, &raw); err != nil {
		return nil, fmt.Errorf("agentengine: decoding stream line: %w", err)
	}
	if msg, ok := raw["error"]; ok {
		return nil, errors.New("agentengine: remote error: " + fmt.Sprint(msg))
	}
	ev := &Event{Raw: raw}
	ev.Author, _ = raw["author"].(string)
	ev.Partial, _ = raw["partial"].(bool)
	if c, ok := raw["content"]; ok && c != nil {
		// genai's JSON tags are camelCase, but servers emit snake_case keys,
		// so normalize before decoding.
		c = camelizeKeys(c)
		stdBase64Parts(c)
		b, err := json.Marshal(c)
		if err != nil {
			return nil, fmt.Errorf("agentengine: re-encoding content: %w", err)
		}
		var content genai.Content
		if err := json.Unmarshal(b, &content); err != nil {
			return nil, fmt.Errorf("agentengine: decoding content: %w", err)
		}
		ev.Content = &content
	}
	return ev, nil
}

// userKeyFields hold map-valued genai fields whose keys are user data (tool
// arguments and results, part metadata) and must reach the caller unchanged.
var userKeyFields = map[string]bool{"args": true, "response": true, "partMetadata": true}

// camelizeKeys renames snake_case object keys to the lowerCamelCase names in
// genai's JSON tags. Both the adk-go server (helper.ConvertSnake) and Python
// agents (model_dump_json without by_alias) emit snake_case keys. camelCase
// input passes through unchanged. Values of userKeyFields are copied without
// descending into them.
func camelizeKeys(v any) any {
	switch v := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(v))
		for k, val := range v {
			k = snakeToCamel(k)
			if userKeyFields[k] {
				out[k] = val
				continue
			}
			out[k] = camelizeKeys(val)
		}
		return out
	case []any:
		out := make([]any, len(v))
		for i, val := range v {
			out[i] = camelizeKeys(val)
		}
		return out
	default:
		return v
	}
}

func snakeToCamel(s string) string {
	if !strings.Contains(s, "_") {
		return s
	}
	var b strings.Builder
	upper := false
	for _, r := range s {
		if r == '_' {
			upper = b.Len() > 0
			continue
		}
		if upper {
			r = []rune(strings.ToUpper(string(r)))[0]
			upper = false
		}
		b.WriteRune(r)
	}
	return b.String()
}

// stdBase64Parts rewrites the bytes fields of each part (inlineData.data and
// thoughtSignature) to standard base64 in place. Pydantic emits bytes as
// URL-safe base64, while encoding/json decodes []byte only from standard
// base64, so a Python agent's value would otherwise fail the whole line.
// content must be a value returned by camelizeKeys, which the caller owns.
func stdBase64Parts(content any) {
	c, _ := content.(map[string]any)
	parts, _ := c["parts"].([]any)
	for _, p := range parts {
		part, _ := p.(map[string]any)
		if s, ok := part["thoughtSignature"].(string); ok {
			part["thoughtSignature"] = stdBase64(s)
		}
		if blob, ok := part["inlineData"].(map[string]any); ok {
			if s, ok := blob["data"].(string); ok {
				blob["data"] = stdBase64(s)
			}
		}
	}
}

// stdBase64 converts URL-safe base64 to the standard alphabet and restores
// padding. Strings without '-' or '_' are returned unchanged.
func stdBase64(s string) string {
	if !strings.ContainsAny(s, "-_") {
		return s
	}
	s = strings.NewReplacer("-", "+", "_", "/").Replace(s)
	if r := len(s) % 4; r != 0 {
		s += strings.Repeat("=", 4-r)
	}
	return s
}
