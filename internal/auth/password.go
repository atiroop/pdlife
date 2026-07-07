package auth

import (
	"fmt"
	"unicode/utf8"

	"golang.org/x/crypto/bcrypt"
)

const bcryptCost = 12

const minPasswordLength = 10

// ValidatePasswordStrength returns a user-facing error if the password
// doesn't meet the minimum bar, or nil if it's acceptable.
func ValidatePasswordStrength(password string) error {
	if utf8.RuneCountInString(password) < minPasswordLength {
		return fmt.Errorf("รหัสผ่านต้องมีอย่างน้อย %d ตัวอักษร", minPasswordLength)
	}
	var hasLetter, hasDigit bool
	for _, r := range password {
		switch {
		case r >= '0' && r <= '9':
			hasDigit = true
		case (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z'):
			hasLetter = true
		}
	}
	if !hasLetter || !hasDigit {
		return fmt.Errorf("รหัสผ่านต้องมีทั้งตัวอักษรและตัวเลขอย่างน้อยอย่างละ 1 ตัว")
	}
	return nil
}

func HashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcryptCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

func CheckPassword(hash, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}
