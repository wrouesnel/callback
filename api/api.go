// Defines the user-facing api
package api

import (
	"github.com/julienschmidt/httprouter"
	"github.com/wrouesnel/callback/api/apisettings"
	"github.com/wrouesnel/callback/api/callback"
	"github.com/wrouesnel/callback/api/connect"
	"net/http"
)

// Appends a new goboot-callback API to the supplied router.
func NewAPI_v1(settings apisettings.APISettings) http.Handler {
	router := httprouter.New()

	// Event APIs
	//router.GET("/events/connect", boot.Subscribe(settings))
	//router.GET("/events/callback", plan.Subscribe(settings))

	// Callback (reverse proxy) setup
	router.GET("/callback/:callbackId", callback.CallbackPost(settings))
	//router.GET("/callback/:identifier", plan.GetPlan(settings))
	//router.PUT("/callback/:identifier", plan.SetPlan(settings))
	//router.DELETE("/callback/:identifier", plan.DeletePlan(settings))

	//router.GET("/callback", plan.ListPlan(settings))
	//router.DELETE("/callback", plan.ClearPlans(settings))

	// Connect setup
	router.POST("/connect/:callbackId", connect.ConnectPost(settings))

	return router
}
