// Defines the user-facing api
package api

import (
	"github.com/julienschmidt/httprouter"
	"github.com/wrouesnel/callback/api/apisettings"
	"github.com/wrouesnel/callback/api/callback"
	"github.com/wrouesnel/callback/api/connect"
)

// Appends a new goboot-callback API to the supplied router.
func NewAPI_v1(settings apisettings.APISettings, router *httprouter.Router) *httprouter.Router {
	// Event APIs
	router.GET(settings.WrapPath("/api/v1/events/connect"), connect.Subscribe(settings))
	router.GET(settings.WrapPath("/api/v1/events/callback"), callback.Subscribe(settings))

	// Callback (reverse proxy) setup
	router.GET(settings.WrapPath("/api/v1/callback/:callbackId"), callback.CallbackGet(settings))
	router.GET(settings.WrapPath("/api/v1/callback"), callback.SessionsGet(settings))
	//router.PUT("/callback/:identifier", plan.SetPlan(settings))
	//router.DELETE("/callback/:identifier", plan.DeletePlan(settings))

	//router.GET("/callback", plan.ListPlan(settings))
	//router.DELETE("/callback", plan.ClearPlans(settings))

	// Connect setup
	router.GET(settings.WrapPath("/api/v1/connect/:callbackId"), connect.ConnectGet(settings))
	router.GET(settings.WrapPath("/api/v1/connect"), connect.SessionsGet(settings))

	return router
}
