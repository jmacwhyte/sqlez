package sqlez

import (
	"database/sql"
	"errors"
	"reflect"
	"strconv"
	"strings"
)

// Params contains the parameters for the query
type Params struct {
	Where     string
	OrderBy   string
	Limit     int
	SkipEmpty bool
}

// DB represents the sqlez database wrapper
type DB struct {
	DB        *sql.DB
	LastQuery string
}

// Open initiates the connection to the database. It takes the same parameters as the database/sql package, and returns a sqlEZ DB struct. The contained *sql.DB is exported so you can make use of it directly.
func Open(driverName, dataSourceName string) *DB {
	var ez DB
	var err error

	ez.DB, err = sql.Open(driverName, dataSourceName)
	if err != nil {
		panic(err.Error())
	}
	return &ez
}

// Close closes the connection to the database
func (s DB) Close() error {
	return s.DB.Close()
}

// scanStruct recursively scans a provided struct and returns pointers/interfaces and labels for all items that
// have a "db" tag. Set pointers to true if you want pointers, otherwise interfaces will be returned. Set skipEmpty
// to true if you want to ignore fields that have unset/zero values.
func (s DB) scanStruct(v reflect.Value, pointers bool, skipEmpty bool) ([]string, []interface{}) {

	var data []interface{}
	var labels []string

	for i := 0; i < v.NumField(); i++ {
		field := v.Field(i)
		if field.Kind() == reflect.Struct {
			l, d := s.scanStruct(field, pointers, skipEmpty)
			labels = append(labels, l...)
			data = append(data, d...)
		} else {
			if label, exists := v.Type().Field(i).Tag.Lookup("db"); exists {
				if skipEmpty && (field.Interface() == reflect.Zero(field.Type()).Interface()) {
					continue
				}

				if pointers {
					data = append(data, field.Addr().Interface())
				} else {
					data = append(data, field.Interface())
				}
				labels = append(labels, label)
			}
		}
	}
	return labels, data
}

// SelectFrom performs a "SELECT y FROM x" query on the database and returns a []interface{} of the results.
// Pass in a struct representing the database rows to specify what data you get back.
func (s *DB) SelectFrom(database string, structure interface{}, params ...interface{}) (out []interface{}, err error) {

	t := reflect.TypeOf(structure)
	if t.Kind() != reflect.Struct {
		return nil, errors.New(`'structure' must be a struct describing the database rows`)
	}

	copy := reflect.New(t).Elem()
	labels, pointers := s.scanStruct(copy, true, false)

	query := "SELECT "
	for i, v := range labels {
		query += v
		if i < len(labels)-1 {
			query += ", "
		}
	}
	query += " FROM " + database

	if len(params) > 0 {

		if reflect.TypeOf(params[0]).String() != "sqlez.Params" {
			return nil, errors.New(`first parameter passed wasn't a sqlez.Params`)
		}

		p := params[0].(Params)
		params = params[1:]

		if p.Where != "" {
			where := strings.Trim(p.Where, " ,")

			if strings.ToLower(where[:5]) == "where" {
				return nil, errors.New(`Params.Where starts with WHERE`)
			}
			query += " WHERE " + where
		}

		if p.OrderBy != "" {
			order := strings.Trim(p.OrderBy, " ,")

			if strings.ToLower(order[:5]) == "order" {
				return nil, errors.New(`Params.OrderBy starts with ORDER`)
			}
			query += " ORDER BY " + order
		}

		if p.Limit > 0 {
			query += " LIMIT " + strconv.Itoa(p.Limit)
		}
	}

	s.LastQuery = query

	var result *sql.Rows
	result, err = s.DB.Query(query, params...)
	if err != nil {
		return
	}

	for result.Next() {
		err = result.Scan(pointers...)
		if err != nil {
			return
		}
		out = append(out, copy.Interface())
	}

	err = result.Close()
	if err != nil {
		return
	}

	return
}

// InsertInto performs a "INSERT INTO x (y) VALUES (z)" query on the database and returns the results.
// Pass a struct representing the data you want to insert. Set skipEmpty to true if you want to ignore
// fields in the struct that are unset/zero. Otherwise the zeroed values will be inserted.
func (s *DB) InsertInto(database string, data interface{}, skipEmpty bool) (res sql.Result, err error) {

	v := reflect.ValueOf(data)
	if v.Kind() != reflect.Struct {
		return nil, errors.New(`'structure' must be a struct describing the database rows`)
	}

	labels, interfaces := s.scanStruct(v, false, skipEmpty)

	var columns string
	var values string

	for i, v := range labels {
		columns += v
		values += "?"
		if i < len(labels)-1 {
			columns += ", "
			values += ", "
		}
	}

	query := "INSERT INTO " + database + " (" + columns + ") VALUES (" + values + ")"
	s.LastQuery = query
	return s.DB.Exec(query, interfaces...)
}

// Update performs an "UPDATE x SET y = z" query on the database and returns the results.
// Pass a struct representing the data you want to update and Params to specify what to update.
func (s *DB) Update(database string, data interface{}, params ...interface{}) (res sql.Result, err error) {

	v := reflect.ValueOf(data)
	if v.Kind() != reflect.Struct {
		return nil, errors.New(`'data' must be a struct describing the database rows`)
	}

	var options string
	var p Params
	if len(params) > 0 {
		if reflect.TypeOf(params[0]).String() != "sqlez.Params" {
			return nil, errors.New(`first parameter passed wasn't a sqlez.Params`)
		}

		p = params[0].(Params)
		params = params[1:]

		if p.Where != "" {
			where := strings.Trim(p.Where, " ,")

			if strings.ToLower(where[:5]) == "where" {
				return nil, errors.New(`Params.Where starts with WHERE`)
			}
			options += " WHERE " + where
		}

		if p.OrderBy != "" {
			order := strings.Trim(p.OrderBy, " ,")

			if strings.ToLower(order[:5]) == "order" {
				return nil, errors.New(`Params.OrderBy starts with ORDER`)
			}
			options += " ORDER BY " + order
		}

		if p.Limit > 0 {
			options += " LIMIT " + strconv.Itoa(p.Limit)
		}
	}

	labels, interfaces := s.scanStruct(v, false, p.SkipEmpty)

	query := "UPDATE " + database + " SET "
	for i, v := range labels {
		query += v + " = ?"
		if i < len(labels)-1 {
			query += ", "
		}
	}
	query += options

	s.LastQuery = query
	return s.DB.Exec(query, interfaces...)
}
