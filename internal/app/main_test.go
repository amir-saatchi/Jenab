package app

import (
	"os"
	"testing"

	"github.com/amir-saatchi/jenab/internal/sqlworker"
)

// TestMain lets the test binary be its own query worker, for the views'
// queries.
func TestMain(m *testing.M) {
	sqlworker.IfWorker()
	os.Exit(m.Run())
}
