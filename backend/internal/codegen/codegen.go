package codegen

import (
	"math/rand"
	"strings"
)

const (
	letters = "ABCDEFGHJKLMNPQRSTUVWXYZ" // no I or O to avoid confusion with 1 and 0
	digits  = "23456789"                 // no 0 or 1 to avoid confusion with O and I
)

// New generates a random code in the format AAA111 —
// 3 uppercase letters followed by 3 digits.
func New() string {
	var b strings.Builder
	b.Grow(6)
	for i := 0; i < 3; i++ {
		b.WriteByte(letters[rand.Intn(len(letters))])
	}
	for i := 0; i < 3; i++ {
		b.WriteByte(digits[rand.Intn(len(digits))])
	}
	return b.String()
}

// NewUnique generates a code that is not already in the provided set.
// The set is checked by the caller (the Hub), which owns the rooms map.
func NewUnique(exists func(code string) bool) string {
	for {
		code := New()
		if !exists(code) {
			return code
		}
	}
}
