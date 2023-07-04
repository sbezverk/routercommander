package types

import (
	"encoding/json"
	"io"
	"os"
	"testing"
)

func TestParseModel(t *testing.T) {
	f, err := os.Open("./model.yaml")
	if err != nil {
		t.Fatalf("failed to open model file with error: %+v", err)
	}
	defer f.Close()

	b, err := io.ReadAll(f)
	if err != nil {
		t.Fatalf("failed to read model file with error: %+v", err)
	}

	c, err := parseCommandFile(b)
	if err != nil {
		t.Fatalf("failed to parse model file with error: %+v", err)
	}

	b, _ = json.MarshalIndent(c, "", "   ")
	t.Logf("Resulting commander structure: %s", string(b))
}
