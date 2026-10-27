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

import "testing"

func TestResourceName(t *testing.T) {
	tests := []struct {
		name         string
		cfg          Config
		wantName     string
		wantLocation string
		wantErr      bool
	}{
		{
			name:         "triple form",
			cfg:          Config{Project: "p", Location: "us-central1", ReasoningEngine: "123"},
			wantName:     "projects/p/locations/us-central1/reasoningEngines/123",
			wantLocation: "us-central1",
		},
		{
			name:         "full name form",
			cfg:          Config{Name: "projects/p/locations/europe-west1/reasoningEngines/456"},
			wantName:     "projects/p/locations/europe-west1/reasoningEngines/456",
			wantLocation: "europe-west1",
		},
		{
			name:         "full name wins over triple",
			cfg:          Config{Name: "projects/p/locations/europe-west1/reasoningEngines/456", Project: "x", Location: "y", ReasoningEngine: "z"},
			wantName:     "projects/p/locations/europe-west1/reasoningEngines/456",
			wantLocation: "europe-west1",
		},
		{name: "missing project", cfg: Config{Location: "l", ReasoningEngine: "r"}, wantErr: true},
		{name: "missing location", cfg: Config{Project: "p", ReasoningEngine: "r"}, wantErr: true},
		{name: "missing engine", cfg: Config{Project: "p", Location: "l"}, wantErr: true},
		{name: "malformed full name", cfg: Config{Name: "reasoningEngines/456"}, wantErr: true},
		{name: "empty", cfg: Config{}, wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gotName, gotLocation, err := resourceName(tc.cfg)
			if (err != nil) != tc.wantErr {
				t.Fatalf("resourceName(%+v) error = %v, wantErr %v", tc.cfg, err, tc.wantErr)
			}
			if gotName != tc.wantName || gotLocation != tc.wantLocation {
				t.Errorf("resourceName(%+v) = (%q, %q), want (%q, %q)", tc.cfg, gotName, gotLocation, tc.wantName, tc.wantLocation)
			}
		})
	}
}

func TestNewRejectsInvalidConfig(t *testing.T) {
	if _, err := New(t.Context(), Config{}); err == nil {
		t.Fatal("New(empty Config) succeeded, want error")
	}
}
