package sqlez

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"time"
)

//TODO: complete this

type MySQLDriver struct{}

func (d MySQLDriver) GetDataType(t reflect.Type) string {
	switch t.Kind() {
	case reflect.String:
		return "VARCHAR(255)"
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64, reflect.Bool:
		return "INT"
	case reflect.Float32, reflect.Float64:
		return "FLOAT"
	case reflect.Struct:
		if t == reflect.TypeOf(time.Time{}) {
			return "DATETIME"
		}
	}
	return "TEXT"
}

func (d MySQLDriver) GetName() string {
	return "mysql"
}

func (d MySQLDriver) CreateTable(data *DBObjectMetadata) string {

	var columns []string
	for _, col := range data.cols {
		pk := ""
		if col.primary {
			pk = " NOT NULL PRIMARY KEY"
		}

		auto := ""
		if col.autoinc || col.primary {
			auto = " AUTOINCREMENT"
		}

		def := ""
		if col.def != "" {
			def = " DEFAULT " + col.def
		}

		prop := ""
		if col.colProp != "" {
			prop = " " + col.colProp
		}

		columns = append(columns, fmt.Sprintf("%s %s%s%s%s%s", col.label, col.sqlType, pk, auto, def, prop))
	}

	if data.fkey >= 0 {
		fk := data.cols[data.fkey]
		columns = append(columns, fmt.Sprintf("FOREIGN KEY (%s) REFERENCES %s(%s)", fk.label, fk.foreignTable, fk.foreignKey))
	}

	return fmt.Sprintf("CREATE TABLE %s (%s)", data.table, strings.Join(columns, ", "))
}

// InsertIgnore returns a query string and a slice of values to be used with it
func (d MySQLDriver) InsertIgnore(data *DBObjectAddOn, ignore bool) (query string, vals []interface{}) {

	val := reflect.ValueOf(data.parent).Elem()

	var columns []string
	for i, col := range data.meta.cols {
		if col.primary {
			continue
		}

		if col.created {
			val.Field(i).Set(reflect.ValueOf(time.Now()))
		}
		if col.updated {
			val.Field(i).Set(reflect.ValueOf(time.Now()))
		}

		columns = append(columns, col.label)
		if col.json {
			if j, err := json.Marshal(val.Field(i).Interface()); err != nil {
				fmt.Printf("err marshalling json: %s\n", err)
			} else {
				vals = append(vals, string(j))
				continue
			}
		}

		if col.goType == GoTime {
			vals = append(vals, val.Field(i).Interface().(time.Time).Unix())
			continue
		}

		vals = append(vals, val.Field(i).Interface())
	}

	ig := ""
	if ignore {
		ig = " OR IGNORE"
	}

	query = fmt.Sprintf("INSERT%s INTO %s (%s) VALUES (%s)", ig, data.meta.table, strings.Join(columns, ", "), strings.Repeat("?, ", len(columns)-1)+"?")
	return
}

// Update returns a query string and a slice of values to be used with it
func (d MySQLDriver) Update(data *DBObjectAddOn) (query string, vals []interface{}) {
	val := reflect.ValueOf(data.parent).Elem()

	var where string
	var whereval interface{}

	var columns []string
	for i, col := range data.meta.cols {
		if col.primary {
			where = col.label
			whereval = val.Field(i).Interface()
			continue
		}

		if col.updated {
			val.Field(i).Set(reflect.ValueOf(time.Now()))
		}

		columns = append(columns, col.label)

		if col.goType == GoTime {
			vals = append(vals, val.Field(i).Interface().(time.Time).Unix())
			continue
		}
		vals = append(vals, val.Field(i).Interface())
	}

	vals = append(vals, whereval)
	query = fmt.Sprintf("UPDATE %s SET %s WHERE %s = ?", data.meta.table, strings.Join(columns, "= ?, ")+"= ?", where)
	return
}

// Select
func (d MySQLDriver) Select(data *DBObjectAddOn, params Params) (query string) {
	var where, order, limit string
	if params.Where != "" {
		where = " WHERE " + params.Where
	}
	if params.OrderBy != "" {
		order = " ORDER BY " + params.OrderBy
	}
	if params.Limit > 0 {
		limit = fmt.Sprintf(" LIMIT %d", params.Limit)
	}

	query = fmt.Sprintf("SELECT * FROM %s%s%s%s", data.meta.table, where, order, limit)
	return
}

// Delete
func (d MySQLDriver) Delete(data *DBObjectAddOn) (query string, vals interface{}) {
	query = fmt.Sprintf("DELETE FROM %s WHERE %s = ?", data.meta.table, data.meta.cols[data.meta.pkey].label)
	vals = reflect.ValueOf(data.parent).Elem().Field(data.meta.pkey).Interface()
	return
}
