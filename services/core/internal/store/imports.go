package store

import (
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/RomanKovalev007/barber_crm/services/core/internal/db"
	"github.com/RomanKovalev007/barber_crm/services/core/internal/model"
)

// Ожидаемые столбцы CSV (заголовок обязателен, порядок любой).
var importColumns = []string{"phone", "last_name", "first_name", "middle_name", "email",
	"birth_date", "notes", "marketing_consent"}

// ImportClientsCSV — пакетная загрузка клиентов:
//  1. разбор CSV и проверка заголовка (структурные ошибки → 400 до обращения к БД);
//  2. в одной транзакции: создание пакета, COPY сырых строк в staging,
//     вызов staging.process_client_import (валидация, журнал ошибок, upsert).
//
// Ошибочные строки фиксируются в staging.import_errors и не прерывают загрузку;
// сбой на уровне БД откатывает пакет целиком.
func (s *Store) ImportClientsCSV(ctx context.Context, source string, r io.Reader) (*model.ImportResult, error) {
	cr := csv.NewReader(r)
	cr.FieldsPerRecord = -1
	cr.TrimLeadingSpace = true
	header, err := cr.Read()
	if err != nil {
		return nil, fmt.Errorf("%w: не удалось прочитать заголовок CSV: %v", ErrBadCSV, err)
	}
	idx := map[string]int{}
	for i, h := range header {
		idx[strings.ToLower(strings.TrimSpace(strings.TrimPrefix(h, "\ufeff")))] = i
	}
	for _, req := range []string{"phone", "first_name"} {
		if _, ok := idx[req]; !ok {
			return nil, fmt.Errorf("%w: в заголовке нет обязательного столбца %q", ErrBadCSV, req)
		}
	}

	var rowsData [][]any
	line := 1
	for {
		rec, err := cr.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		line++
		if err != nil {
			return nil, fmt.Errorf("%w: строка %d: %v", ErrBadCSV, line, err)
		}
		row := make([]any, 0, len(importColumns)+1)
		row = append(row, line)
		for _, col := range importColumns {
			var v any
			if i, ok := idx[col]; ok && i < len(rec) && strings.TrimSpace(rec[i]) != "" {
				v = rec[i]
			}
			row = append(row, v)
		}
		rowsData = append(rowsData, row)
	}

	res := &model.ImportResult{}
	err = db.InTx(ctx, s.Pool, pgx.ReadCommitted, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `INSERT INTO staging.import_batches (source_name) VALUES ($1) RETURNING batch_id`,
			source).Scan(&res.BatchID); err != nil {
			return err
		}
		src := make([][]any, len(rowsData))
		for i, r := range rowsData {
			src[i] = append([]any{res.BatchID}, r...)
		}
		cols := append([]string{"batch_id", "line_no"}, importColumns...)
		if _, err := tx.CopyFrom(ctx, pgx.Identifier{"staging", "client_import_rows"}, cols, pgx.CopyFromRows(src)); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `SELECT total, inserted, updated, rejected FROM staging.process_client_import($1)`,
			res.BatchID).Scan(&res.Total, &res.Inserted, &res.Updated, &res.Rejected); err != nil {
			return err
		}
		var err error
		res.Errors, err = getAll[model.ImportError](ctx, tx, `
			SELECT line_no, field, message FROM staging.import_errors
			WHERE batch_id = $1 ORDER BY line_no, error_id`, res.BatchID)
		return err
	})
	if err != nil {
		return nil, err
	}
	return res, nil
}

// ErrBadCSV — структурная ошибка файла (не данных).
var ErrBadCSV = errors.New("некорректный CSV")
