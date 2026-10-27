package classes

import (
	"os"
	"testing"

	"github.com/MitiaKorotkov/drop-home-works/internal/testdb"
)

func TestMain(m *testing.M) {
	os.Exit(testdb.Main(m))
}
