// Команда importer — пакетная загрузка клиентской базы из CSV.
//
//	importer -file data/import/clients_legacy.csv
//
// Подключение: CORE_DB_URL (роль crm_app). Строки с ошибками не загружаются и
// выводятся в отчёте; код выхода 2, если были отклонённые строки.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/RomanKovalev007/barber_crm/services/core/internal/db"
	"github.com/RomanKovalev007/barber_crm/services/core/internal/store"
)

func main() {
	file := flag.String("file", "", "CSV-файл (UTF-8, с заголовком)")
	flag.Parse()
	if *file == "" {
		log.Fatal("укажите -file")
	}
	url := os.Getenv("CORE_DB_URL")
	if url == "" {
		log.Fatal("CORE_DB_URL is required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	pool, err := db.Connect(ctx, url, 2)
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()

	f, err := os.Open(*file)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()

	res, err := store.New(pool).ImportClientsCSV(db.WithAppUser(ctx, "importer"), filepath.Base(*file), f)
	if err != nil {
		log.Fatalf("импорт не выполнен (изменения откатены): %v", err)
	}
	fmt.Printf("Пакет #%d: строк %d, добавлено %d, обновлено %d, отклонено %d\n",
		res.BatchID, res.Total, res.Inserted, res.Updated, res.Rejected)
	for _, e := range res.Errors {
		fmt.Printf("  строка %4d  %-11s %s\n", e.LineNo, e.Field, e.Message)
	}
	if res.Rejected > 0 {
		os.Exit(2)
	}
}
