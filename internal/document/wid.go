package document

import (
	"errors"
	"fmt"
	"strconv"
)

var ErrInvalidWID = errors.New("invalid WID")

// WID is the immutable identity of one locale-specific wiki document.
type WID string

func ParseWID(value string) (WID, error) {
	number, err := parseNumericID(value, 'W')
	if err != nil {
		return "", fmt.Errorf("%w: %q", ErrInvalidWID, value)
	}
	return WID("W" + strconv.FormatUint(number, 10)), nil
}

func NewWID(number uint64) (WID, error) {
	if number == 0 {
		return "", fmt.Errorf("%w: number must be positive", ErrInvalidWID)
	}
	return WID("W" + strconv.FormatUint(number, 10)), nil
}

func (id WID) Validate() error {
	_, err := ParseWID(string(id))
	return err
}

func (id WID) String() string {
	return string(id)
}

func parseNumericID(value string, prefix byte) (uint64, error) {
	if len(value) < 2 || value[0] != prefix || value[1] == '0' {
		return 0, ErrInvalidWID
	}
	for i := 1; i < len(value); i++ {
		if value[i] < '0' || value[i] > '9' {
			return 0, ErrInvalidWID
		}
	}
	number, err := strconv.ParseUint(value[1:], 10, 64)
	if err != nil || number == 0 {
		return 0, ErrInvalidWID
	}
	return number, nil
}
