package api

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

type openAPIOperation struct {
	Path   string
	Method string
	Alias  string
	Block  string
}

func TestOpenAPIRouteCoverageAndContractStatus(t *testing.T) {
	registered := registeredRoutes(t)
	spec, err := os.ReadFile("../../../spec/openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	operations, routeOnlyAnchors := parseOpenAPIOperations(t, string(spec))

	documented := make(map[string]bool, len(operations))
	var unclassified []string
	for _, operation := range operations {
		key := operation.Method + " " + operation.Path
		documented[key] = true
		routeOnly := strings.Contains(operation.Block, "x-contract-status: route-only")
		if operation.Alias != "" {
			routeOnly = routeOnlyAnchors[operation.Alias]
		}
		if !routeOnly && !hasConcreteSuccessResponse(operation.Block) {
			unclassified = append(unclassified, key)
		}
	}

	var missingDocumentation []string
	for route := range registered {
		if !documented[route] {
			missingDocumentation = append(missingDocumentation, route)
		}
	}
	sort.Strings(missingDocumentation)
	if len(missingDocumentation) > 0 {
		t.Errorf("registered routes missing from OpenAPI:\n%s", strings.Join(missingDocumentation, "\n"))
	}

	probes := map[string]bool{"/health": true, "/healthz": true, "/readyz": true}
	var missingRegistration []string
	for route := range documented {
		path := strings.SplitN(route, " ", 2)[1]
		if !probes[path] && !registered[route] {
			missingRegistration = append(missingRegistration, route)
		}
	}
	sort.Strings(missingRegistration)
	if len(missingRegistration) > 0 {
		t.Errorf("documented non-probe API operations missing from router:\n%s", strings.Join(missingRegistration, "\n"))
	}

	sort.Strings(unclassified)
	if len(unclassified) > 0 {
		t.Errorf("OpenAPI operations need x-contract-status: route-only or a concrete success response:\n%s", strings.Join(unclassified, "\n"))
	}
}

func TestOpenAPIProbeOperationsArePublic(t *testing.T) {
	spec, err := os.ReadFile("../../../spec/openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	operations, _ := parseOpenAPIOperations(t, string(spec))

	byRoute := make(map[string]openAPIOperation, len(operations))
	for _, operation := range operations {
		byRoute[operation.Method+" "+operation.Path] = operation
	}
	for _, path := range []string{"/health", "/healthz", "/readyz"} {
		operation, ok := byRoute[http.MethodGet+" "+path]
		if !ok {
			t.Errorf("GET %s is not documented", path)
			continue
		}
		if !hasIndentedLine(operation.Block, 6, "security: []") {
			t.Errorf("GET %s must override global security with security: []", path)
		}
		if !strings.Contains(operation.Block, `schema: { $ref: "#/components/schemas/HealthStatus" }`) {
			t.Errorf("GET %s 200 response must use HealthStatus", path)
		}
	}
}

func TestProbeRoutesArePublic(t *testing.T) {
	router := (&Server{}).Routes()
	for _, test := range []struct {
		path       string
		wantStatus int
	}{
		{path: "/health", wantStatus: http.StatusOK},
		{path: "/healthz", wantStatus: http.StatusOK},
		{path: "/readyz", wantStatus: http.StatusServiceUnavailable},
	} {
		t.Run(test.path, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, test.path, nil))
			if recorder.Code != test.wantStatus {
				t.Fatalf("status=%d want %d", recorder.Code, test.wantStatus)
			}
		})
	}
}

func registeredRoutes(t *testing.T) map[string]bool {
	t.Helper()
	registered := map[string]bool{}
	err := chi.Walk((&Server{}).Routes(), func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		if strings.HasPrefix(route, "/api/") {
			route = strings.TrimPrefix(route, "/api")
		}
		registered[strings.ToUpper(method)+" "+route] = true
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return registered
}

func parseOpenAPIOperations(t *testing.T, spec string) ([]openAPIOperation, map[string]bool) {
	t.Helper()
	lines := strings.Split(spec, "\n")
	routeOnlyAnchors := map[string]bool{}
	currentAnchor := ""
	pathsLine := -1
	for i, line := range lines {
		if line == "paths:" {
			pathsLine = i
			break
		}
		if leadingSpaces(line) == 0 {
			currentAnchor = ""
			if _, anchor, ok := strings.Cut(line, " &"); ok {
				currentAnchor = strings.TrimSpace(anchor)
			}
			continue
		}
		if currentAnchor != "" && strings.TrimSpace(line) == "x-contract-status: route-only" {
			routeOnlyAnchors[currentAnchor] = true
		}
	}
	if pathsLine < 0 {
		t.Fatal("OpenAPI paths section not found")
	}

	methods := map[string]bool{"get": true, "post": true, "patch": true, "delete": true, "put": true}
	currentPath := ""
	var operations []openAPIOperation
	for i := pathsLine + 1; i < len(lines); i++ {
		line := lines[i]
		if leadingSpaces(line) == 0 && strings.TrimSpace(line) != "" {
			break
		}
		if leadingSpaces(line) == 2 && strings.HasPrefix(strings.TrimSpace(line), "/") && strings.HasSuffix(strings.TrimSpace(line), ":") {
			currentPath = strings.TrimSuffix(strings.TrimSpace(line), ":")
			continue
		}
		if currentPath == "" || leadingSpaces(line) != 4 {
			continue
		}
		key, value, ok := strings.Cut(strings.TrimSpace(line), ":")
		if !ok || !methods[key] {
			continue
		}
		end := i + 1
		for end < len(lines) {
			if strings.TrimSpace(lines[end]) != "" && leadingSpaces(lines[end]) <= 4 {
				break
			}
			end++
		}
		alias := ""
		value = strings.TrimSpace(value)
		if strings.HasPrefix(value, "*") {
			alias = strings.TrimPrefix(value, "*")
		}
		operations = append(operations, openAPIOperation{
			Path:   currentPath,
			Method: strings.ToUpper(key),
			Alias:  alias,
			Block:  strings.Join(lines[i:end], "\n"),
		})
		i = end - 1
	}
	if len(operations) == 0 {
		t.Fatal("no OpenAPI operations parsed")
	}
	for _, operation := range operations {
		if operation.Alias != "" && !routeOnlyAnchors[operation.Alias] {
			t.Errorf("%s %s uses anchor %q without x-contract-status: route-only", operation.Method, operation.Path, operation.Alias)
		}
	}
	return operations, routeOnlyAnchors
}

func hasConcreteSuccessResponse(block string) bool {
	for _, line := range strings.Split(block, "\n") {
		if leadingSpaces(line) != 8 {
			continue
		}
		key, _, ok := strings.Cut(strings.TrimSpace(line), ":")
		key = strings.Trim(key, `"'`)
		if ok && len(key) == 3 && key[0] == '2' && key[1] >= '0' && key[1] <= '9' && key[2] >= '0' && key[2] <= '9' {
			return true
		}
	}
	return false
}

func hasIndentedLine(block string, spaces int, content string) bool {
	want := fmt.Sprintf("%s%s", strings.Repeat(" ", spaces), content)
	for _, line := range strings.Split(block, "\n") {
		if line == want {
			return true
		}
	}
	return false
}

func leadingSpaces(line string) int {
	return len(line) - len(strings.TrimLeft(line, " "))
}
