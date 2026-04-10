package network

import (
	"lynx_agent_api/src/middleware"
	"lynx_agent_api/src/network/routes"

	"github.com/gorilla/mux"
)

func SetupRouter() *mux.Router {
	router := mux.NewRouter()

	apiRouter := router.PathPrefix("/api/v1").Subrouter()
	apiRouter.Use(middleware.NodeApiKeyMiddleware)
	serverRouter := apiRouter.PathPrefix("/servers").Subrouter()
	routes.SetupServerRoutes(serverRouter)

	resourceRouter := router.PathPrefix("/resources").Subrouter()
	routes.SetupResourceRoutes(resourceRouter)

	nodeRouter := router.PathPrefix("/node").Subrouter()
	routes.SetupNodeRoutes(nodeRouter)

	sshRouter := router.PathPrefix("/ssh").Subrouter()
	routes.SetupSSHRoutes(sshRouter)

	return router
}
