package routes

import (
	controller "lynx_agent_api/src/network/controllers"

	"github.com/gorilla/mux"
)

func SetupServerRoutes(router *mux.Router) {
	router.HandleFunc("/list", controller.GetAllServers).Methods("GET")
	// Used
	router.HandleFunc("/node/{node_uuid}", controller.GetAllServersByNodeId).Methods("GET")
	// Used
	router.HandleFunc("/{uuid}", controller.GetServerByID).Methods("GET")
	// Used
	router.HandleFunc("/user/{user_uuid}", controller.GetAllServersByUserId).Methods("GET")
	// Used
	router.HandleFunc("/allocation/{allocation_uuid}", controller.GetServerByAllocationID).Methods("GET")
	// Used
	router.HandleFunc("/provision", controller.CreateServer).Methods("POST")
	// Used
	router.HandleFunc("/update/{uuid}", controller.UpdateServer).Methods("PATCH")
	// Used
	router.HandleFunc("/start/{uuid}", controller.StartServer).Methods("PATCH")
	// Used
	router.HandleFunc("/stop/{uuid}", controller.StopServer).Methods("PATCH")
	// Used
	router.HandleFunc("/restart/{uuid}", controller.RestartServer).Methods("PATCH")
	// Used
	router.HandleFunc("/delete/{uuid}", controller.DeleteServer).Methods("DELETE")
}
