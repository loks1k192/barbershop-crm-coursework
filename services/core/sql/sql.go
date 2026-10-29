// Package sqlfiles встраивает SQL-запросы отчётов в бинарник приложения.
// Файлы в sql/queries — единственный источник текста запросов: они же
// приводятся в документации и выполняются через psql.
package sqlfiles

import "embed"

//go:embed queries/*.sql
var Queries embed.FS

// MustQuery возвращает текст запроса по имени файла (без каталога).
func MustQuery(name string) string {
	b, err := Queries.ReadFile("queries/" + name)
	if err != nil {
		panic(err)
	}
	return string(b)
}
