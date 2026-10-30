// Package store — доступ к данным PostgreSQL. Весь SQL приложения — здесь
// и в sql/queries; бизнес-инварианты проверяет СУБД (ограничения, триггеры,
// функции), store лишь вызывает их и возвращает ошибки как есть.
package store

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/RomanKovalev007/barber_crm/services/core/internal/db"
	"github.com/RomanKovalev007/barber_crm/services/core/internal/model"
)

type Store struct {
	Pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Store { return &Store{Pool: pool} }

// querier — общий интерфейс пула и транзакции.
type querier interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// tx выполняет fn в транзакции READ COMMITTED (с crm.app_user для аудита).
func (s *Store) tx(ctx context.Context, fn func(pgx.Tx) error) error {
	return db.InTx(ctx, s.Pool, pgx.ReadCommitted, fn)
}

// ListParams — пагинация и сортировка (Sort — уже проверенный SQL-фрагмент ORDER BY).
type ListParams struct {
	Page     int
	PageSize int
	Sort     string
}

func (p ListParams) offset() int { return (p.Page - 1) * p.PageSize }

// Filter накапливает условия WHERE с позиционными параметрами ($1, $2, ...).
// Пользовательские значения передаются только параметрами — SQL-инъекции исключены.
type Filter struct {
	conds []string
	args  []any
}

// Add добавляет условие; каждый '?' в cond заменяется на номер одного и того же параметра.
func (f *Filter) Add(cond string, v any) {
	f.args = append(f.args, v)
	f.conds = append(f.conds, strings.ReplaceAll(cond, "?", fmt.Sprintf("$%d", len(f.args))))
}

func (f *Filter) where() string {
	if len(f.conds) == 0 {
		return ""
	}
	return " WHERE " + strings.Join(f.conds, " AND ")
}

// list выполняет COUNT и запрос страницы с одинаковыми условиями.
func list[T any](ctx context.Context, q querier, selectCols, from string, f Filter, p ListParams) (model.Page[T], error) {
	res := model.Page[T]{Page: p.Page, PageSize: p.PageSize, Items: []T{}}
	if err := q.QueryRow(ctx, "SELECT count(*) FROM "+from+f.where(), f.args...).Scan(&res.Total); err != nil {
		return res, err
	}
	args := append(append([]any{}, f.args...), p.PageSize, p.offset())
	sql := fmt.Sprintf("SELECT %s FROM %s%s ORDER BY %s LIMIT $%d OFFSET $%d",
		selectCols, from, f.where(), p.Sort, len(args)-1, len(args))
	rows, err := q.Query(ctx, sql, args...)
	if err != nil {
		return res, err
	}
	items, err := pgx.CollectRows(rows, pgx.RowToStructByName[T])
	if err != nil {
		return res, err
	}
	res.Items = items
	return res, nil
}

func getOne[T any](ctx context.Context, q querier, sql string, args ...any) (*T, error) {
	rows, err := q.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	v, err := pgx.CollectExactlyOneRow(rows, pgx.RowToAddrOfStructByName[T])
	if err != nil {
		return nil, err
	}
	return v, nil
}

func getAll[T any](ctx context.Context, q querier, sql string, args ...any) ([]T, error) {
	rows, err := q.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	items, err := pgx.CollectRows(rows, pgx.RowToStructByName[T])
	if items == nil {
		items = []T{}
	}
	return items, err
}

// execOne выполняет изменяющий запрос и возвращает pgx.ErrNoRows, если строка не найдена.
func execOne(ctx context.Context, tx pgx.Tx, sql string, args ...any) error {
	tag, err := tx.Exec(ctx, sql, args...)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}
