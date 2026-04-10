package utils

import (
	"crypto/rand"
	"math/big"
)

const (
	alphabet    = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
	DefaultSize = 21
)

func GenID() (string, error) {
	id := make([]byte, DefaultSize)
	alphabetLen := big.NewInt(int64(len(alphabet)))

	for i := 0; i < DefaultSize; i++ {
		num, err := rand.Int(rand.Reader, alphabetLen)
		if err != nil {
			return "", err
		}
		id[i] = alphabet[num.Int64()]
	}

	return string(id), nil
}
