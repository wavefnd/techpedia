package techdata

import (
	"errors"
	"fmt"
	"strconv"
)

var (
	ErrInvalidTID = errors.New("invalid TID")
	ErrInvalidPID = errors.New("invalid PID")
)

type TID string
type PID string

func ParseTID(value string) (TID, error) {
	number, err := parseNumericID(value, 'T')
	if err != nil {
		return "", fmt.Errorf("%w: %q", ErrInvalidTID, value)
	}
	return NewTID(number)
}

func NewTID(number uint64) (TID, error) {
	if number == 0 {
		return "", fmt.Errorf("%w: number must be positive", ErrInvalidTID)
	}
	return TID("T" + strconv.FormatUint(number, 10)), nil
}

func (id TID) Validate() error {
	_, err := ParseTID(string(id))
	return err
}

func (id TID) String() string {
	return string(id)
}

func ParsePID(value string) (PID, error) {
	number, err := parseNumericID(value, 'P')
	if err != nil {
		return "", fmt.Errorf("%w: %q", ErrInvalidPID, value)
	}
	return NewPID(number)
}

func NewPID(number uint64) (PID, error) {
	if number == 0 {
		return "", fmt.Errorf("%w: number must be positive", ErrInvalidPID)
	}
	return PID("P" + strconv.FormatUint(number, 10)), nil
}

func (id PID) Validate() error {
	_, err := ParsePID(string(id))
	return err
}

func (id PID) String() string {
	return string(id)
}

func parseNumericID(value string, prefix byte) (uint64, error) {
	if len(value) < 2 || value[0] != prefix || value[1] == '0' {
		return 0, errors.New("invalid numeric identifier")
	}
	for i := 1; i < len(value); i++ {
		if value[i] < '0' || value[i] > '9' {
			return 0, errors.New("invalid numeric identifier")
		}
	}
	return strconv.ParseUint(value[1:], 10, 64)
}
