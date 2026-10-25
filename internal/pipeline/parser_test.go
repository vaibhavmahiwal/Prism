package pipeline

import "testing"

func TestParse(t *testing.T) {
	const data = `
name: build-and-test
image: golang:1.23
steps:
  - name: test
    run: go test ./...
`

	p, err := Parse([]byte(data))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if p.Name != "build-and-test" || p.Image != "golang:1.23" || len(p.Steps) != 1 {
		t.Fatalf("Parse() = %#v, want parsed pipeline", p)
	}
}

func TestParseRejectsUnknownFields(t *testing.T) {
	_, err := Parse([]byte(`
image: golang:1.23
steps:
  - name: test
    run: go test ./...
unexpected: true
`))
	if err == nil {
		t.Fatal("Parse() error = nil, want unknown-field error")
	}
}

func TestPipelineValidate(t *testing.T) {
	tests := []struct {
		name     string
		pipeline Pipeline
	}{
		{
			name:     "missing image",
			pipeline: Pipeline{Steps: []Step{{Name: "test"}}},
		},
		{
			name:     "empty steps",
			pipeline: Pipeline{Image: "golang:1.23"},
		},
		{
			name:     "empty step name",
			pipeline: Pipeline{Image: "golang:1.23", Steps: []Step{{}}},
		},
		{
			name:     "duplicate step names",
			pipeline: Pipeline{Image: "golang:1.23", Steps: []Step{{Name: "test"}, {Name: "test"}}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.pipeline.Validate(); err == nil {
				t.Fatal("Validate() error = nil, want validation error")
			}
		})
	}
}
