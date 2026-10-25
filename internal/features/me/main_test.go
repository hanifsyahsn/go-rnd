package me

import (
	"database/sql"
	"testing"

	"github.com/hanifsyahsn/go-rnd/internal/shared/testhelper"
)

var testDB *sql.DB

func TestMain(m *testing.M) {
	testhelper.RunWithDB(m, func(db *sql.DB) { testDB = db })
}
