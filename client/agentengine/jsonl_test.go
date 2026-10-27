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
	"errors"
	"iter"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"google.golang.org/genai"
)

// chunksOf yields the given byte slices in order, then stops.
func chunksOf(parts ...string) iter.Seq2[[]byte, error] {
	return func(yield func([]byte, error) bool) {
		for _, p := range parts {
			if !yield([]byte(p), nil) {
				return
			}
		}
	}
}

func collect(t *testing.T, events iter.Seq2[*Event, error]) ([]*Event, error) {
	t.Helper()
	var got []*Event
	for ev, err := range events {
		if err != nil {
			return got, err
		}
		got = append(got, ev)
	}
	return got, nil
}

const (
	lineA = `{"author":"root","content":{"role":"model","parts":[{"text":"A"}]},"partial":false}` + "\n"
	lineB = `{"author":"root","content":{"role":"model","parts":[{"text":"B"}]}}` + "\n"
	lineC = `{"author":"root","content":{"role":"model","parts":[{"text":"C"}]},"partial":true}` + "\n"
)

func texts(events []*Event) []string {
	var out []string
	for _, ev := range events {
		if ev.Content == nil {
			out = append(out, "<nil>")
			continue
		}
		out = append(out, ev.Content.Parts[0].Text)
	}
	return out
}

func TestDecodeJSONL(t *testing.T) {
	tests := []struct {
		name      string
		chunks    iter.Seq2[[]byte, error]
		wantTexts []string
		wantErr   string // substring; "" means no error
	}{
		{
			name:      "three events in one chunk",
			chunks:    chunksOf(lineA + lineB + lineC),
			wantTexts: []string{"A", "B", "C"},
		},
		{
			name:      "line split across two chunks",
			chunks:    chunksOf(lineA[:20], lineA[20:]+lineB),
			wantTexts: []string{"A", "B"},
		},
		{
			name:      "split exactly at the newline",
			chunks:    chunksOf(lineA, lineB),
			wantTexts: []string{"A", "B"},
		},
		{
			name:      "trailing line without newline",
			chunks:    chunksOf(lineA + strings.TrimSuffix(lineB, "\n")),
			wantTexts: []string{"A", "B"},
		},
		{
			name:      "event without content is kept with nil Content",
			chunks:    chunksOf(`{"author":"root","actions":{"state_delta":{}}}` + "\n" + lineA),
			wantTexts: []string{"<nil>", "A"},
		},
		{
			name:      "blank lines are skipped",
			chunks:    chunksOf("\n" + lineA + "\n\n" + lineB),
			wantTexts: []string{"A", "B"},
		},
		{
			name:      "whitespace-only line is skipped",
			chunks:    chunksOf(lineA + " \t\n" + lineB),
			wantTexts: []string{"A", "B"},
		},
		{
			name:      "error line ends the stream",
			chunks:    chunksOf(lineA + `{"error":"boom"}` + "\n" + lineB),
			wantTexts: []string{"A"},
			wantErr:   "boom",
		},
		{
			name:      "malformed line is an error",
			chunks:    chunksOf(lineA + "{not json\n"),
			wantTexts: []string{"A"},
			wantErr:   "decoding",
		},
		{
			name:      "empty stream",
			chunks:    chunksOf(),
			wantTexts: nil,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := collect(t, decodeJSONL(tc.chunks))
			if tc.wantErr == "" && err != nil {
				t.Fatalf("decodeJSONL() error = %v, want nil", err)
			}
			if tc.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tc.wantErr)) {
				t.Fatalf("decodeJSONL() error = %v, want containing %q", err, tc.wantErr)
			}
			if diff := cmp.Diff(tc.wantTexts, texts(got)); diff != "" {
				t.Errorf("texts mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestDecodeJSONLFields(t *testing.T) {
	got, err := collect(t, decodeJSONL(chunksOf(lineC)))
	if err != nil || len(got) != 1 {
		t.Fatalf("decodeJSONL() = %d events, %v; want 1, nil", len(got), err)
	}
	ev := got[0]
	if ev.Author != "root" || !ev.Partial {
		t.Errorf("Author=%q Partial=%v, want root/true", ev.Author, ev.Partial)
	}
	wantContent := &genai.Content{Role: "model", Parts: []*genai.Part{{Text: "C"}}}
	if diff := cmp.Diff(wantContent, ev.Content); diff != "" {
		t.Errorf("Content mismatch (-want +got):\n%s", diff)
	}
	if ev.Raw["author"] != "root" {
		t.Errorf("Raw[author] = %v, want root", ev.Raw["author"])
	}
}

func TestDecodeJSONLNullContent(t *testing.T) {
	got, err := collect(t, decodeJSONL(chunksOf(`{"author":"root","content":null}`+"\n")))
	if err != nil || len(got) != 1 {
		t.Fatalf("decodeJSONL() = %d events, %v; want 1, nil", len(got), err)
	}
	if got[0].Content != nil {
		t.Errorf("Content = %+v, want nil for \"content\": null", got[0].Content)
	}
}

func TestDecodeJSONLPropagatesChunkError(t *testing.T) {
	boom := errors.New("transport broke")
	chunks := func(yield func([]byte, error) bool) {
		if !yield([]byte(lineA), nil) {
			return
		}
		if !yield(nil, boom) {
			return
		}
		// Reached only if decodeJSONL keeps reading after the error.
		yield([]byte(lineB), nil)
	}
	got, err := collect(t, decodeJSONL(chunks))
	if !errors.Is(err, boom) {
		t.Fatalf("error = %v, want %v", err, boom)
	}
	if len(got) != 1 {
		t.Errorf("got %d events before the error, want 1", len(got))
	}
}

func TestDecodeJSONLStopsWhenConsumerStops(t *testing.T) {
	yielded := 0
	chunks := func(yield func([]byte, error) bool) {
		for {
			yielded++
			if !yield([]byte(lineA), nil) {
				return
			}
			if yielded > 10 {
				t.Fatal("producer was not stopped")
			}
		}
	}
	for range decodeJSONL(chunks) {
		break
	}
	if yielded != 1 {
		t.Errorf("producer yielded %d chunks after consumer stopped, want 1", yielded)
	}
}

func TestDecodeJSONLPartFieldCasing(t *testing.T) {
	// The adk-go server writes snake_case keys and Python agents write
	// camelCase. Both must decode to the same parts, and keys inside args,
	// response and partMetadata are user data that must stay as written.
	tests := []struct {
		name  string
		parts string
	}{
		{
			name: "snake_case from the adk-go server",
			parts: `[{"function_call":{"id":"c1","name":"lookup","args":{"user_id":"U1","nested":{"some_key":1}}}},` +
				`{"function_response":{"name":"lookup","response":{"status_code":200}}},` +
				`{"inline_data":{"mime_type":"image/png","data":"AQID"}},` +
				`{"text":"hi","thought_signature":"AQ==","part_metadata":{"source_file":"x"}}]`,
		},
		{
			name: "camelCase from Python agents",
			parts: `[{"functionCall":{"id":"c1","name":"lookup","args":{"user_id":"U1","nested":{"some_key":1}}}},` +
				`{"functionResponse":{"name":"lookup","response":{"status_code":200}}},` +
				`{"inlineData":{"mimeType":"image/png","data":"AQID"}},` +
				`{"text":"hi","thoughtSignature":"AQ==","partMetadata":{"source_file":"x"}}]`,
		},
	}
	want := []*genai.Part{
		{FunctionCall: &genai.FunctionCall{ID: "c1", Name: "lookup", Args: map[string]any{"user_id": "U1", "nested": map[string]any{"some_key": float64(1)}}}},
		{FunctionResponse: &genai.FunctionResponse{Name: "lookup", Response: map[string]any{"status_code": float64(200)}}},
		{InlineData: &genai.Blob{MIMEType: "image/png", Data: []byte{1, 2, 3}}},
		{Text: "hi", ThoughtSignature: []byte{1}, PartMetadata: map[string]any{"source_file": "x"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			line := `{"author":"root","content":{"role":"model","parts":` + tc.parts + `}}` + "\n"
			got, err := collect(t, decodeJSONL(chunksOf(line)))
			if err != nil || len(got) != 1 {
				t.Fatalf("decodeJSONL() = %d events, %v; want 1, nil", len(got), err)
			}
			if diff := cmp.Diff(want, got[0].Content.Parts); diff != "" {
				t.Errorf("Parts mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
