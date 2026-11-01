package store_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/RomanKovalev007/barber_crm/services/core/internal/store"
)

func TestImportClients(t *testing.T) {
	csv := "\ufeffphone,last_name,first_name,email,birth_date,marketing_consent\n" +
		"8 (916) 444-00-01,Импортов,Иван,ivan@imp.ru,01.02.1990,да\n" + // новый
		"+79160000001,Клиент,Первый,first@imp.ru,,нет\n" + // существующий → обновление
		"12345,Плохой,Телефон,,,\n" + // ошибка телефона
		"+79164440001,Дубль,Иван,,,\n" + // дубль строки 2
		"+79164440002,Без,,bad@,31.02.1990,\n" // три ошибки
	res, err := st.ImportClientsCSV(ctx(t), "test.csv", strings.NewReader(csv))
	must(t, err)
	if res.Total != 5 || res.Inserted != 1 || res.Updated != 1 || res.Rejected != 3 || len(res.Errors) != 5 {
		t.Fatalf("итоги импорта: %+v", res)
	}
	if e := scalar[string](t, `SELECT email FROM crm.clients WHERE phone = '+79160000001'`); e != "first@imp.ru" {
		t.Fatalf("обновление существующего клиента: %s", e)
	}
	if n := scalar[string](t, `SELECT birth_date::text FROM crm.clients WHERE phone = '+79164440001'`); n != "1990-02-01" {
		t.Fatalf("нормализация: %s", n)
	}
	// структурная ошибка файла — до обращения к БД
	_, err = st.ImportClientsCSV(ctx(t), "bad.csv", strings.NewReader("name;phone\nx;y\n"))
	if !errors.Is(err, store.ErrBadCSV) {
		t.Fatalf("ожидалась ошибка формата CSV, получено %v", err)
	}
	// повторная обработка пакета запрещена
	_, err = tdb.App.Exec(ctx(t), `SELECT * FROM staging.process_client_import($1)`, res.BatchID)
	wantState(t, err, "CR010")
}
