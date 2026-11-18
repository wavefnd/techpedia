package document

import (
	"errors"
	"testing"
)

func TestParseWID(t *testing.T) {
	t.Parallel()

	valid := []string{"W1", "W42", "W18446744073709551615"}
	for _, value := range valid {
		value := value
		t.Run(value, func(t *testing.T) {
			t.Parallel()
			id, err := ParseWID(value)
			if err != nil {
				t.Fatalf("parse %q: %v", value, err)
			}
			if id.String() != value {
				t.Fatalf("expected %q, got %q", value, id)
			}
		})
	}

	invalid := []string{"", "W", "W0", "W01", "w1", "T1", " W1", "W1 "}
	for _, value := range invalid {
		value := value
		t.Run("invalid_"+value, func(t *testing.T) {
			t.Parallel()
			if _, err := ParseWID(value); !errors.Is(err, ErrInvalidWID) {
				t.Fatalf("expected invalid WID for %q, got %v", value, err)
			}
		})
	}
}
