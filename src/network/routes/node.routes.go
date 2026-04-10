package routes

import (
	controller "lynx_agent_api/src/network/controllers"

	"github.com/gorilla/mux"
)

func SetupNodeRoutes(router *mux.Router) {
	router.HandleFunc("/resources/ws", controller.NodeResourcesWebSocket).Methods("GET")
}
