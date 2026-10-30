package pg

import (
	"reflect"
	"testing"
)

func TestEraseStatements(t *testing.T) {
	got := EraseStatements([]string{"public", `we"ird`}, []string{"pgcrypto"})
	want := []string{
		`DROP SCHEMA IF EXISTS "public" CASCADE`,
		`DROP SCHEMA IF EXISTS "we""ird" CASCADE`,
		`DROP EXTENSION IF EXISTS "pgcrypto" CASCADE`,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q\nwant %q", got, want)
	}
}

func TestEndpointURLEscapesPassword(t *testing.T) {
	e := Endpoint{Host: "h", Port: 5432, Database: "d", User: "u", Password: "p@ss/w:rd", SSLMode: "disable"}
	u := e.URL()
	want := "postgres://u:p%40ss%2Fw%3Ard@h:5432/d?application_name=db-copier&connect_timeout=15&sslmode=disable"
	if u != want {
		t.Fatalf("got %s\nwant %s", u, want)
	}
}
