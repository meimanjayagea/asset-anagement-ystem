package main

import (
	"bytes"
	"os"
	"testing"
)

func TestEmbeddedMigrationMatchesSource(t *testing.T) {
	for _, name := range []string{"001_init.sql", "002_branches_activity.sql", "003_roles_scope_archive.sql", "004_finance_lifecycle.sql", "005_organization_codes.sql", "006_employee_login.sql", "007_field_lifecycle.sql", "008_record_metadata.sql"} {
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
