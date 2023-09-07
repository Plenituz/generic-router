package generic_router

import (
	"fmt"
	"github.com/pkg/errors"
	"path"
	"reflect"
	"regexp"
	"strings"
)

type Middleware func(handler Handler) (Handler, error)
type Handler interface {
	SetPathParams(pathParams map[string]string) Handler
	PreExecution(event RequestEngine) (Handler, error)
	Execution(event RequestEngine) (interface{}, error)
	//input is the output of Execution
	PostExecution(event RequestEngine, executionOutput interface{}) (interface{}, error)
}
type RequestEngine interface {
	GetPath() string
	GetHttpVerb() string
	GetBody() []byte
	GetHeaders() map[string]string
	GetQueryStringParams() map[string]string
	//input value is the return value of Handler.PostExecution, or just an error from the overall execution
	//check if e is a RouterExecutionError to handle system errors
	FormatOutput(output interface{}, e error) interface{}
}

type Route struct {
	Path string
	Middlewares []Middleware
	SubRoutes []Route
	Get Handler `httpVerb:"GET"`
	Post Handler `httpVerb:"POST"`
	Put Handler `httpVerb:"PUT"`
	Delete Handler `httpVerb:"DELETE"`
	Patch Handler `httpVerb:"PATCH"`
	Head Handler `httpVerb:"HEAD"`
}

func (r *Route) AddRoute(path string, f func(root *Route)) Route {
	newRoute := Route{Path: path}
	f(&newRoute)
	r.SubRoutes = append(r.SubRoutes, newRoute)
	return *r
}
func (r *Route) Use(middleware Middleware) {
	r.Middlewares = append(r.Middlewares, middleware)
}

func (r *Route) AddGet(path string, handler Handler) {
	r.addVerb(path, "GET", handler)
}
func (r *Route) AddPost(path string, handler Handler) {
	r.addVerb(path, "POST", handler)
}
func (r *Route) AddDelete(path string, handler Handler) {
	r.addVerb(path, "DELETE", handler)
}
func (r *Route) AddPut(path string, handler Handler) {
	r.addVerb(path, "PUT", handler)
}
func (r *Route) AddPatch(path string, handler Handler) {
	r.addVerb(path, "PATCH", handler)
}
func (r *Route) AddHead(path string, handler Handler) {
	r.addVerb(path, "HEAD", handler)
}

func (r *Route) addVerb(path string, verb string, handler Handler) {
	var existingSubRoute *Route
	if path == "/" {
		existingSubRoute = r
	} else {
		for _, subRoute := range r.SubRoutes {
			if subRoute.Path == path {
				existingSubRoute = &subRoute
				break
			}
		}
	}

	if existingSubRoute != nil {
		SetRouteVerb(existingSubRoute, verb, handler)
	} else {
		newRoute := Route{Path: path}
		SetRouteVerb(&newRoute, verb, handler)
		r.SubRoutes = append(r.SubRoutes, newRoute)
	}
}

func MakeRoot(f func(root *Route)) Route {
	root := Route{}
	f(&root)
	return root
}

//can return nil if the path doesnt exist
//if a parameterisedPath pointer is given, it will be populated with the value of the
//the final path of the handler found, in a parameterised shape.
//For example trying to route "/abc/123/def" could return a parameterisedPath of "/abc/{myVar}/def"
//this also returns the list of all the middlewares found on the way to the final handler
func FindRoute(path string, verb string, root Route, parameterisedPath *string, middlewares []Middleware) (Handler, []Middleware) {
	split := strings.FieldsFunc(path, func(r rune) bool {
		return r == '/'
	})

	//fmt.Println("looking at ", root.Path, "comparing to", "/" + split[0], "split is", split, len(split))
	if !(len(split) == 0 || root.Path == "" || matchPathElement(root.Path, "/" + split[0])) {
		return nil, nil
	}

	//fmt.Println("match", root.Path, "/" + split[0])

	if root.Path != "" && len(split) == 1 {
		//exact match
		if root.Middlewares != nil {
			middlewares = append(middlewares, root.Middlewares...)
		}
		if parameterisedPath != nil {
			*parameterisedPath += root.Path
		}
		return getVerb(root, verb), middlewares
	}
	//not exact match, look in subroutes
	subpath := strings.TrimPrefix(path, "/" + split[0])
	if root.Path == "" {
		subpath = path
	}
	for _, subroute := range root.SubRoutes {
		handler, middlewares := FindRoute(subpath, verb, subroute, parameterisedPath, middlewares)
		if handler != nil {
			if root.Middlewares != nil {
				middlewares = append(middlewares, root.Middlewares...)
			}
			if parameterisedPath != nil {
				*parameterisedPath = root.Path + *parameterisedPath
			}
			return handler, middlewares
		}
	}
	return nil, nil
}

var pathParamRegex = regexp.MustCompile(`/{[^/]*?}`)
func matchPathElement(inputPath string, element string) bool {
	split := strings.FieldsFunc(inputPath, func(r rune) bool {
		return r == '/'
	})
	nodePath := "/" + split[0]

	if pathParamRegex.MatchString(nodePath) {
		nodePath = pathParamRegex.ReplaceAllString(nodePath, "/*")
		// fmt.Println("matching", nodePath)
		match, err := path.Match(nodePath, element)
		if err != nil {
			fmt.Println("match error", err.Error())
		} else {
			return match
		}
	}
	return nodePath == element
}

func getVerb(route Route, verb string) Handler {
	routeType := reflect.TypeOf(route)
	for i := 0; i < routeType.NumField(); i++ {
		field := routeType.Field(i)
		tagVal, ok := field.Tag.Lookup("httpVerb")
		if !ok {
			continue
		}
		if tagVal == verb {
			handlerValue := reflect.ValueOf(route).Field(i)
			if handlerValue.IsNil() {
				return nil
			}
			tmp := handlerValue.Interface().(Handler)
			return tmp
		}
	}
	return nil
}

func SetRouteVerb(route *Route, verb string, handler Handler) {
	routeType := reflect.TypeOf(*route)
	for i := 0; i < routeType.NumField(); i++ {
		field := routeType.Field(i)
		tagVal, ok := field.Tag.Lookup("httpVerb")
		if !ok {
			continue
		}
		if tagVal == verb {
			handlerValue := reflect.ValueOf(route).Elem().Field(i)
			handlerValue.Set(reflect.ValueOf(handler))
			return
		}
	}
}

//var pathParamRegexExtract = regexp.MustCompile(`({[^/]*?})`)
func ExtractPathParameters(path string, parameterisedPath string) (map[string]string, error) {
	dynamicRegex := pathParamRegex.ReplaceAllFunc([]byte(parameterisedPath), func(bytes []byte) []byte {
		name := string(bytes)
		name = strings.TrimPrefix(name, "/{")
		name = strings.TrimSuffix(name, "}")
		return []byte("\\/(?P<" + name + ">.*?)")
	})
	// fmt.Println(string(dynamicRegex))
	reg, err := regexp.Compile(string(dynamicRegex) + "$")
	if err != nil {
		return nil, errors.Wrap(err, "error building path params regex")
	}

	matches := reg.FindStringSubmatch(path)
	if matches == nil {
		return map[string]string{}, nil
	}
	subMatchMap := map[string]string{}
	for i, name := range reg.SubexpNames() {
		if i != 0 {
			subMatchMap[name] = matches[i]
		}
	}
	return subMatchMap, nil
}