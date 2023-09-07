package generic_router

const (
	RouterExecutionMiddlewareExecutionError = "Middleware execution error"
	RouterExecutionPathParamsError = "Error extracting path params"
	RouterExecutionPathNotFound = "Not found"
)
type RouterExecutionError struct {
	Msg     string
	Details *string
}
func NewRouterExecutionError(msg string, details *string) error {
	return RouterExecutionError{Msg: msg, Details: details}
}
func (r RouterExecutionError) Error() string {
	return r.Msg
}

// returned value is the returned value of RequestEngine.FormatOutput
func ExecutePath(event RequestEngine, root Route) interface{} {
	parameterisedPath := ""
	handlerP, middlewares := FindRoute(event.GetPath(), event.GetHttpVerb(), root, &parameterisedPath, []Middleware{})
	if handlerP == nil {
		return event.FormatOutput(nil, NewRouterExecutionError(RouterExecutionPathNotFound, nil))
	}
	handler := handlerP
	pathParams, err := ExtractPathParameters(event.GetPath(), parameterisedPath)
	if err != nil {
		tmp := err.Error()
		return event.FormatOutput(nil, NewRouterExecutionError(RouterExecutionPathParamsError, &tmp))
	}
	handler = handler.SetPathParams(pathParams)

	if middlewares != nil && len(middlewares) != 0 {
		h := handler
		var err error
		for _, middleware := range middlewares {
			h, err = middleware(h)
			if err != nil {
				tmp := err.Error()
				return event.FormatOutput(nil, NewRouterExecutionError(RouterExecutionMiddlewareExecutionError, &tmp))
			}
		}
		handler = h
	}

	return ExecuteHandler(event, handler)
}

func ExecuteHandler(event RequestEngine, handler Handler) interface{} {
	handler, err := handler.PreExecution(event)
	if err != nil {
		return event.FormatOutput(nil, err)
	}

	output, err := handler.Execution(event)
	if err != nil {
		return event.FormatOutput(nil, err)
	}

	output, err = handler.PostExecution(event, output)
	if err != nil {
		return event.FormatOutput(nil, err)
	}

	return event.FormatOutput(output, nil)
}