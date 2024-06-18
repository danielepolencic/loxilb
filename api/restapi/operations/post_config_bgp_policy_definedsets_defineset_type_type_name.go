// Code generated by go-swagger; DO NOT EDIT.

package operations

// This file was generated by the swagger tool.
// Editing this file might prove futile when you re-run the generate command

import (
	"net/http"

	"github.com/go-openapi/runtime/middleware"
)

// PostConfigBgpPolicyDefinedsetsDefinesetTypeTypeNameHandlerFunc turns a function with the right signature into a post config bgp policy definedsets defineset type type name handler
type PostConfigBgpPolicyDefinedsetsDefinesetTypeTypeNameHandlerFunc func(PostConfigBgpPolicyDefinedsetsDefinesetTypeTypeNameParams) middleware.Responder

// Handle executing the request and returning a response
func (fn PostConfigBgpPolicyDefinedsetsDefinesetTypeTypeNameHandlerFunc) Handle(params PostConfigBgpPolicyDefinedsetsDefinesetTypeTypeNameParams) middleware.Responder {
	return fn(params)
}

// PostConfigBgpPolicyDefinedsetsDefinesetTypeTypeNameHandler interface for that can handle valid post config bgp policy definedsets defineset type type name params
type PostConfigBgpPolicyDefinedsetsDefinesetTypeTypeNameHandler interface {
	Handle(PostConfigBgpPolicyDefinedsetsDefinesetTypeTypeNameParams) middleware.Responder
}

// NewPostConfigBgpPolicyDefinedsetsDefinesetTypeTypeName creates a new http.Handler for the post config bgp policy definedsets defineset type type name operation
func NewPostConfigBgpPolicyDefinedsetsDefinesetTypeTypeName(ctx *middleware.Context, handler PostConfigBgpPolicyDefinedsetsDefinesetTypeTypeNameHandler) *PostConfigBgpPolicyDefinedsetsDefinesetTypeTypeName {
	return &PostConfigBgpPolicyDefinedsetsDefinesetTypeTypeName{Context: ctx, Handler: handler}
}

/*
	PostConfigBgpPolicyDefinedsetsDefinesetTypeTypeName swagger:route POST /config/bgp/policy/definedsets/{defineset_type}/{type_name} postConfigBgpPolicyDefinedsetsDefinesetTypeTypeName

# Adds a BGP BGP definedsets for making Policy

Adds a BGP definedsets for making Policy
*/
type PostConfigBgpPolicyDefinedsetsDefinesetTypeTypeName struct {
	Context *middleware.Context
	Handler PostConfigBgpPolicyDefinedsetsDefinesetTypeTypeNameHandler
}

func (o *PostConfigBgpPolicyDefinedsetsDefinesetTypeTypeName) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	route, rCtx, _ := o.Context.RouteInfo(r)
	if rCtx != nil {
		*r = *rCtx
	}
	var Params = NewPostConfigBgpPolicyDefinedsetsDefinesetTypeTypeNameParams()
	if err := o.Context.BindValidRequest(r, route, &Params); err != nil { // bind params
		o.Context.Respond(rw, r, route.Produces, route, err)
		return
	}

	res := o.Handler.Handle(Params) // actually handle the request
	o.Context.Respond(rw, r, route.Produces, route, res)

}