package api

import (
	"net/http"

	apidocs "github.com/RomanKovalev007/barber_crm/services/core/api"
)

func serveOpenAPI(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/yaml; charset=utf-8")
	_, _ = w.Write(apidocs.OpenAPI)
}

const swaggerHTML = `<!DOCTYPE html>
<html lang="ru"><head><meta charset="utf-8"><title>Barber CRM Core API</title>
<link rel="stylesheet" href="https://unpkg.com/swagger-ui-dist@5/swagger-ui.css"></head>
<body><div id="ui"></div>
<script src="https://unpkg.com/swagger-ui-dist@5/swagger-ui-bundle.js"></script>
<script>SwaggerUIBundle({url: '/openapi.yaml', dom_id: '#ui'});</script>
</body></html>`

func serveSwaggerUI(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(swaggerHTML))
}
