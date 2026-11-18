package techdata

import (
	"errors"
	"testing"
)

func TestTechdataIdentifiers(t *testing.T) {
	t.Parallel()

	if id, err := ParseTID("T42"); err != nil || id != "T42" {
		t.Fatalf("parse TID: id=%q err=%v", id, err)
	}
	if id, err := ParsePID("P31"); err != nil || id != "P31" {
		t.Fatalf("parse PID: id=%q err=%v", id, err)
	}

	for _, value := range []string{"", "T", "T0", "T01", "t1", "W1"} {
		if _, err := ParseTID(value); !errors.Is(err, ErrInvalidTID) {
			t.Fatalf("expected invalid TID for %q, got %v", value, err)
		}
	}
	for _, value := range []string{"", "P", "P0", "P01", "p1", "T1"} {
		if _, err := ParsePID(value); !errors.Is(err, ErrInvalidPID) {
			t.Fatalf("expected invalid PID for %q, got %v", value, err)
		}
	}
}

func TestStatementValueValidation(t *testing.T) {
	t.Parallel()

	statement := Statement{
		Property: "P31",
		Value: Value{
			Type: DataTypeItem,
			Item: "T7",
		},
		Rank: RankNormal,
	}
	if err := statement.Validate(); err != nil {
		t.Fatalf("validate statement: %v", err)
	}

	statement.Value = Value{Type: DataTypeString}
	if err := statement.Validate(); !errors.Is(err, ErrInvalidValue) {
		t.Fatalf("expected invalid value, got %v", err)
	}
}
