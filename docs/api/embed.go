package apidocs

import _ "embed"

// OpenAPI is the public LoopForge OpenAPI 3.1 contract.
//
//go:embed openapi.yaml
var OpenAPI []byte
