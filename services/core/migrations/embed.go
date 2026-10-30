// Package migrations встраивает SQL-миграции схемы БД в бинарники приложения.
package migrations

import "embed"

// FS содержит файлы вида NNNN_name.up.sql / NNNN_name.down.sql.
//
//go:embed *.sql
var FS embed.FS
