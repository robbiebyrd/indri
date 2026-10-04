package env

import (
	"bufio"
	"os"
	"regexp"
	"slices"
	"testing"
)

// composeOnly are read by docker-compose.yml for the local stack, not by the
// server; testOnly are read by the contract tests. Both belong in .env.example,
// which documents every INDRI_ variable anything in the repo reads.
var (
	composeOnly = []string{"INDRI_MONGO_PORT", "INDRI_POSTGRES_USER", "INDRI_POSTGRES_PASSWORD", "INDRI_POSTGRES_DB", "INDRI_POSTGRES_PORT"}
	testOnly    = []string{"INDRI_TEST_MONGO_URI", "INDRI_TEST_POSTGRES_URI"}
)

func documented(t *testing.T) []string {
	t.Helper()

	f, err := os.Open("../../../.env.example")
	if err != nil {
		t.Fatalf("opening .env.example: %v", err)
	}
	defer f.Close()

	key := regexp.MustCompile(`^#?\s*(INDRI_[A-Z0-9_]+)=`)

	var keys []string

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for scanner.Scan() {
		if m := key.FindStringSubmatch(scanner.Text()); m != nil {
			keys = append(keys, m[1])
		}
	}

	if err := scanner.Err(); err != nil {
		t.Fatalf("reading .env.example: %v", err)
	}

	return keys
}

// .env.example is the reference for configuring a server, so a setting it
// leaves out is one an operator cannot find, and one it lists that nothing
// reads is a knob that does nothing.
func TestEnvExample_DocumentsExactlyWhatIsRead(t *testing.T) {
	keys := documented(t)

	read := slices.Concat(composeOnly, testOnly)
	for _, s := range Settings() {
		read = append(read, s.EnvVar)
	}

	for _, want := range read {
		if !slices.Contains(keys, want) {
			t.Errorf(".env.example does not document %s", want)
		}
	}

	for _, key := range keys {
		if !slices.Contains(read, key) {
			t.Errorf(".env.example documents %s, which nothing reads", key)
		}
	}
}
