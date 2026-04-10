package lib

import (
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"lynx_agent_api/src/config"
)

type EncryptedPayload struct {
	IV   string `json:"iv"`
	Data string `json:"data"`
}

func DecryptNodeKey(encryptedToken string) (string, error) {
	decoded, err := base64.StdEncoding.DecodeString(encryptedToken)
	if err != nil {
		return "", errors.New("failed to decode base64")
	}

	var payload EncryptedPayload
	if err := json.Unmarshal(decoded, &payload); err != nil {
		return "", errors.New("failed to parse encrypted payload")
	}

	keyHex := config.AppConfig.Security.APIKeySecret
	if keyHex == "" {
		return "", errors.New("API_KEY_SECRET not configured")
	}

	key, err := hex.DecodeString(keyHex)
	if err != nil {
		return "", errors.New("failed to decode encryption key")
	}

	if len(key) != 32 {
		return "", errors.New("encryption key must be 32 bytes")
	}

	iv, err := base64.StdEncoding.DecodeString(payload.IV)
	if err != nil {
		return "", errors.New("failed to decode IV")
	}

	encryptedData, err := base64.StdEncoding.DecodeString(payload.Data)
	if err != nil {
		return "", errors.New("failed to decode encrypted data")
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}

	mode := cipher.NewCBCDecrypter(block, iv)

	decrypted := make([]byte, len(encryptedData))
	mode.CryptBlocks(decrypted, encryptedData)

	decrypted, err = removePKCS7Padding(decrypted)
	if err != nil {
		return "", err
	}

	return string(decrypted), nil
}

func removePKCS7Padding(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return nil, errors.New("empty data")
	}

	paddingLen := int(data[len(data)-1])
	if paddingLen > len(data) || paddingLen == 0 {
		return nil, errors.New("invalid padding")
	}

	for i := len(data) - paddingLen; i < len(data); i++ {
		if data[i] != byte(paddingLen) {
			return nil, errors.New("invalid padding bytes")
		}
	}

	return data[:len(data)-paddingLen], nil
}
