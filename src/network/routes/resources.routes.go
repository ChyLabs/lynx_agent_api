package routes

import (
	controller "lynx_agent_api/src/network/controllers"

	"github.com/gorilla/mux"
)

func SetupResourceRoutes(router *mux.Router) {
	router.HandleFunc("/{uuid}/ws", controller.ServerResourcesWebSocket).Methods("GET")
}
