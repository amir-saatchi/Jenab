package scenario

import (
	"os"
	"testing"

	"github.com/amir-saatchi/jenab/internal/sqlworker"
)

// TestMain lets the test binary run as the query worker, for the real
// query tool.
func TestMain(m *testing.M) {
	sqlworker.IfWorker()
	os.Exit(m.Run())
}
