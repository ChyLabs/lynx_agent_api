package lib

import "golang.org/x/crypto/bcrypt"

const (
	DefaultCost = bcrypt.DefaultCost
	MinCost     = bcrypt.MinCost
	MaxCost     = bcrypt.MaxCost
)

func Hash(password string) (string, error) {
	return HashWithCost(password, DefaultCost)
}

func HashWithCost(password string, cost int) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), cost)
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}

func Compare(password, hash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}

func GenerateSalt() (string, error) {
	return GenerateSaltWithCost(DefaultCost)
}

func GenerateSaltWithCost(cost int) (string, error) {
	salt, err := bcrypt.GenerateFromPassword([]byte(""), cost)
	if err != nil {
		return "", err
	}
	return string(salt), nil
}
