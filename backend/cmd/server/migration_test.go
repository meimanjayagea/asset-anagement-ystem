package main

import (
	"bytes"
	"os"
	"testing"
)

func TestEmbeddedMigrationMatchesSource(t *testing.T) {
	for _, name := range []string{"001_init.sql", "002_branches_activity.sql"} {
		embedded, e := migrations.ReadFile("migrations/" + name)
		if e != nil {
			t.Fatal(e)
		}
		source, e := os.ReadFile("../../migrations/" + name)
		if e != nil {
			t.Fatal(e)
		}
		if !bytes.Equal(embedded, source) {
			t.Fatal("embedded migration differs: " + name)
		}
	}
}
