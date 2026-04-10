package routes

import (
	controller "lynx_agent_api/src/network/controllers"

	"github.com/gorilla/mux"
)

func SetupSSHRoutes(router *mux.Router) {
	router.HandleFunc("/{uuid}/ws", controller.SSHTerminalWebSocket).Methods("GET")
}
