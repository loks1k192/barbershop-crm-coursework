// Команда migrate — управление схемой БД.
//
//	migrate up [-to N]     применить миграции (до версии N)
//	migrate down [-steps N] откатить N последних миграций (по умолчанию 1)
//	migrate status         показать состояние
//
// Подключение: CORE_MIGRATE_DB_URL (роль-владелец схемы crm_owner).
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/RomanKovalev007/barber_crm/services/core/internal/migrate"
	"github.com/RomanKovalev007/barber_crm/services/core/migrations"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: migrate up|down|status [flags]")
		os.Exit(2)
	}
	cmd := os.Args[1]
	fl := flag.NewFlagSet(cmd, flag.ExitOnError)
	to := fl.Int("to", 0, "target version for up (0 = latest)")
	steps := fl.Int("steps", 1, "number of migrations to roll back")
	_ = fl.Parse(os.Args[2:])

	url := os.Getenv("CORE_MIGRATE_DB_URL")
	if url == "" {
		log.Fatal("CORE_MIGRATE_DB_URL is required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	var conn *pgx.Conn
	var err error
	for i := 0; i < 30; i++ { // ожидание готовности БД при старте docker compose
		if conn, err = pgx.Connect(ctx, url); err == nil {
			break
		}
		time.Sleep(time.Second)
	}
	if err != nil {
		log.Fatalf("connect: %v", err)
	}
	defer conn.Close(ctx)

	ms, err := migrate.Load(migrations.FS)
	if err != nil {
		log.Fatalf("load migrations: %v", err)
	}
	m := migrate.New(conn, ms)
	m.Log = func(f string, a ...any) { log.Printf(f, a...) }

	switch cmd {
	case "up":
		err = m.Up(ctx, *to)
	case "down":
		err = m.Down(ctx, *steps)
	case "status":
		var applied []migrate.Applied
		if applied, err = m.AppliedList(ctx); err == nil {
			done := map[int]bool{}
			for _, a := range applied {
				done[a.Version] = true
			}
			for _, mg := range ms {
				state := "pending"
				if done[mg.Version] {
					state = "applied"
				}
				fmt.Printf("%04d  %-28s %s\n", mg.Version, mg.Name, state)
			}
		}
	default:
		err = fmt.Errorf("unknown command %q", cmd)
	}
	if err != nil {
		log.Fatal(err)
	}
}
