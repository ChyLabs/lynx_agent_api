package middleware

import (
	"fmt"
	"io/ioutil"
	"lynx_agent_api/src/lib"
	"net/http"
	"strings"
)

func NodeApiKeyMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Printf("[NodeApiKeyMiddleware] Request: %s %s\n", r.Method, r.URL.Path)

		encryptedApiKey := r.Header.Get("X-Node-Api-Key")

		if encryptedApiKey == "" {
			lib.HandleError(w, lib.Unauthorized("Missing Node API Key"))
			return
		}

		decryptedMachineID, err := lib.DecryptNodeKey(encryptedApiKey)
		if err != nil {
			lib.HandleError(w, lib.Unauthorized("Invalid or malformed Node API Key"))
			return
		}

		machineIDBytes, err := ioutil.ReadFile("/etc/machine-id")
		if err != nil {
			lib.HandleError(w, lib.InternalServerError("Failed to read machine ID", err))
			return
		}

		localMachineID := strings.TrimSpace(string(machineIDBytes))

		if decryptedMachineID != localMachineID {
			lib.HandleError(w, lib.Unauthorized("Invalid Node API Key - machine ID mismatch"))
			return
		}

		next.ServeHTTP(w, r)
	})
}
