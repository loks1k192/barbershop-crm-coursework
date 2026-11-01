package store_test

import (
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/RomanKovalev007/barber_crm/services/core/internal/migrate"
	"github.com/RomanKovalev007/barber_crm/services/core/internal/testdb"
	"github.com/RomanKovalev007/barber_crm/services/core/migrations"
)

// Миграция 0005 изменяет структуру clients и сохраняет данные; откат возвращает исходный вид.
func TestMigrationPreservesData(t *testing.T) {
	c := ctx(t)
	d, err := testdb.Create(c, 4, false)
	must(t, err)
	defer d.Close()

	_, err = d.Admin.Exec(c, `INSERT INTO crm.clients (phone, full_name) VALUES
		('+79161000001', 'Иван'), ('+79161000002', 'Артём Волков'), ('+79161000003', 'Петров Пётр Петрович')`)
	must(t, err)

	conn, err := pgx.Connect(c, d.AdminURL)
	must(t, err)
	defer conn.Close(c)
	ms, err := migrate.Load(migrations.FS)
	must(t, err)
	m := migrate.New(conn, ms)
	must(t, m.Up(c, 5))

	rows, err := d.Admin.Query(c, `SELECT coalesce(last_name, '-'), first_name, coalesce(middle_name, '-'), full_name
		FROM crm.clients ORDER BY phone`)
	must(t, err)
	got, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) ([4]string, error) {
		var v [4]string
		return v, r.Scan(&v[0], &v[1], &v[2], &v[3])
	})
	must(t, err)
	want := [][4]string{
		{"-", "Иван", "-", "Иван"},
		{"Волков", "Артём", "-", "Волков Артём"},
		{"Петров", "Пётр", "Петрович", "Петров Пётр Петрович"},
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("строка %d: %v, ожидалось %v", i, got[i], want[i])
		}
	}
	// бонусные счета (созданы триггером до миграции) не потеряны
	var n int
	must(t, d.Admin.QueryRow(c, `SELECT count(*) FROM crm.loyalty_accounts`).Scan(&n))
	if n != 3 {
		t.Fatalf("бонусные счета: %d", n)
	}
	// откат и повторное применение
	must(t, m.Down(c, 1))
	var full string
	must(t, d.Admin.QueryRow(c, `SELECT full_name FROM crm.clients WHERE phone = '+79161000003'`).Scan(&full))
	if full != "Петров Пётр Петрович" {
		t.Fatalf("после отката: %s", full)
	}
	must(t, m.Up(c, 0))
}
