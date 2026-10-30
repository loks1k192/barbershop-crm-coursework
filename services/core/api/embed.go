// Package apidocs встраивает спецификацию OpenAPI сервиса core.
package apidocs

import _ "embed"

//go:embed openapi.yaml
var OpenAPI []byte
