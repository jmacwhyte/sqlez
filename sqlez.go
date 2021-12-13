package sqlez

import (
	"database/sql"
	"encoding/json"
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
	OrIgnore  bool
}

// DB represents the sqlez database wrapper
type DB struct {
	DB            *sql.DB
	LastQuery     string
	dbTag         string
	dbjsonTag     string
	dbskipTag     string
	jsonPtr       map[*sql.NullString]interface{}
	nullStringPtr map[*sql.NullString]interface{}
}

// Open initiates the connection to the database. It takes the same parameters as the database/sql package, and returns a sqlEZ DB struct. The contained *sql.DB is exported so you can make use of it directly.
func Open(driverName, dataSourceName string) (*DB, error) {
	var ez DB
	var err error

	ez.DB, err = sql.Open(driverName, dataSourceName)
	if err != nil {
		return nil, err
	}

	ez.dbTag, ez.dbjsonTag, ez.dbskipTag = "db", "dbjson", "dbskip"

	return &ez, nil
}

// Close closes the connection to the database
func (s DB) Close() error {
	return s.DB.Close()
}

// SetDBTag changes the struct field tag to look for when searching for database column names.
func (s *DB) SetDBTag(tag string) {
	s.dbTag = tag
}

// SetJSONTag changes the struct field tag to look for when searching for database column names for datatypes that should be saved as JSON.
func (s *DB) SetJSONTag(tag string) {
	s.dbjsonTag = tag
}

// SetSkipTag changes the struct field tag to look for when ignoring embedded structs.
func (s *DB) SetSkipTag(tag string) {
	s.dbskipTag = tag
}

// scanStruct recursively scans a provided struct and returns pointers/interfaces and labels for all items that
// have a "db" tag. Set pointers to true if you want pointers, otherwise interfaces will be returned. Set skipEmpty
// to true if you want to ignore fields that have unset/zero values.
func (s *DB) scanStruct(v reflect.Value, pointers bool, skipEmpty bool, firstRun bool) ([]string, []interface{}, error) {

	if firstRun {
		s.jsonPtr = make(map[*sql.NullString]interface{})
		s.nullStringPtr = make(map[*sql.NullString]interface{})
	}

	var data []interface{}
	var labels []string

	for i := 0; i < v.NumField(); i++ {
		field := v.Field(i)
		fieldt := v.Type().Field(i)

		// Get all the tags, and check if empty/should be skipped
		label, jsonexists := fieldt.Tag.Lookup(s.dbjsonTag)
		dblabel, dbexists := fieldt.Tag.Lookup(s.dbTag)
		_, skiptagexists := fieldt.Tag.Lookup(s.dbskipTag)
		skip := (skipEmpty && (field.Interface() == reflect.Zero(field.Type()).Interface()))

		if label == "" && dblabel != "" {
			label = dblabel
		}

		// If there's a skip tag, skip it
		if skiptagexists {
			continue
		}

		// If it is a struct, but we aren't supposed to handle it as json, recursively scan it
		if field.Kind() == reflect.Struct && !jsonexists {
			l, d, e := s.scanStruct(field, pointers, skipEmpty, false)
			if e != nil {
				return nil, nil, e
			}
			labels = append(labels, l...)
			data = append(data, d...)
			continue
		}

		// If it's tagged as json, we need to convert it. We do this automatically for maps.
		if jsonexists || (dbexists && fieldt.Type.Kind() == reflect.Map) {

			// If we are requesting pointers, we must be pulling data from the database. Save a pointer to the actual
			// interface in the buffer, and pass a pointer to a string in the other buffer. After pulling data from the
			// database, Unmarshal the string into the pointer saved in the buffer before returning the data. We can do
			// the same thing with converting strings to NullString.
			if pointers {
				str := new(sql.NullString)
				s.jsonPtr[str] = field.Addr().Interface()
				data = append(data, str)

			} else { // Prepare json to insert into database
				payload, err := json.Marshal(field.Interface())
				if err != nil {
					return nil, nil, errors.New(field.Type().Name() + ": " + err.Error())
				}
				data = append(data, payload)
			}

			labels = append(labels, label)
			continue
		}

		// Otherwise, if it has a DB label let's process it
		if dbexists && !skip {

			labels = append(labels, label)
			if pointers {
				if field.Kind() == reflect.String {
					ns := new(sql.NullString)
					s.nullStringPtr[ns] = field.Addr().Interface()
					data = append(data, ns)
					continue
				}
				data = append(data, field.Addr().Interface())
			} else {
				data = append(data, field.Interface())
			}
		}
	}

	if len(labels) == 0 {
		return nil, nil, errors.New(`sqlez: couldn't find any fields labeled ` + s.dbTag + ` or ` + s.dbjsonTag)
	}

	return labels, data, nil
}

// SelectFrom performs a "SELECT y FROM x" query on the database and returns a []interface{} of the results.
// Pass in a struct representing the database rows to specify what data you get back.
func (s *DB) SelectFrom(table string, structure interface{}, params ...interface{}) (out []interface{}, err error) {

	t := reflect.TypeOf(structure)
	if t.Kind() != reflect.Struct {
		return nil, errors.New(`sqlez.SelectFrom: 'structure' must be struct, got ` + t.Kind().String())
	}

	copy := reflect.New(t).Elem()
	labels, pointers, err := s.scanStruct(copy, true, false, true)
	if err != nil {
		return nil, err
	}

	query := "SELECT "
	for i, v := range labels {
		query += v
		if i < len(labels)-1 {
			query += ", "
		}
	}
	query += " FROM " + table

	if len(params) > 0 {

		if reflect.TypeOf(params[0]).String() != "sqlez.Params" {
			return nil, errors.New(`sqlez.SelectFrom: third parameter passed wasn't a sqlez.Params`)
		}

		p := params[0].(Params)
		params = params[1:]

		if p.Where != "" {
			where := strings.Trim(p.Where, " ,")

			if strings.ToLower(where[:5]) == "where" {
				return nil, errors.New(`sqlez.SelectFrom: Params.Where starts with WHERE`)
			}
			query += " WHERE " + where
		}

		if p.OrderBy != "" {
			order := strings.Trim(p.OrderBy, " ,")

			if strings.ToLower(order[:5]) == "order" {
				return nil, errors.New(`sqlez.SelectFrom: Params.OrderBy starts with ORDER`)
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

		// Unmarshal any strings we have in the buffer
		for i, v := range s.jsonPtr {
			if i.Valid {
				err = json.Unmarshal([]byte(i.String), v)
				if err != nil {
					return nil, errors.New(i.String + ": " + err.Error())
				}

			}
		}

		// Unwrap any sql.NullStrings
		for i, v := range s.nullStringPtr {
			if i.Valid {
				reflect.ValueOf(v).Elem().SetString(i.String)
			}
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
// Pass a struct representing the data you want to insert. Set params.SkipEmpty to true if you want to ignore
// fields in the struct that are unset/zero. Otherwise the zeroed values will be inserted.
func (s *DB) InsertInto(table string, data interface{}, params ...Params) (res sql.Result, err error) {

	p := Params{}
	if params != nil {
		p = params[0]
	}

	v := reflect.ValueOf(data)
	if v.Kind() != reflect.Struct {
		return nil, errors.New(`sqlez.InsertInto: 'structure' must be struct, got ` + v.Kind().String())
	}

	labels, interfaces, err := s.scanStruct(v, false, p.SkipEmpty, true)
	if err != nil {
		return nil, err
	}
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

	query := "INSERT INTO "
	if p.OrIgnore {
		query = "INSERT OR IGNORE INTO "
	}

	query = query + table + " (" + columns + ") VALUES (" + values + ")"
	s.LastQuery = query
	return s.DB.Exec(query, interfaces...)
}

// Update performs an "UPDATE x SET y = z" query on the database and returns the results.
// Pass a struct representing the data you want to update and Params to specify what to update.
func (s *DB) Update(table string, data interface{}, params ...interface{}) (res sql.Result, err error) {

	v := reflect.ValueOf(data)
	if v.Kind() != reflect.Struct {
		return nil, errors.New(`sqlez.Update: 'data' must be struct, got ` + v.Kind().String())
	}

	var options string
	var p Params
	if len(params) > 0 {
		if reflect.TypeOf(params[0]).String() != "sqlez.Params" {
			return nil, errors.New(`sqlez.Update: third parameter passed wasn't a sqlez.Params`)
		}

		p = params[0].(Params)
		params = params[1:]

		if p.Where != "" {
			where := strings.Trim(p.Where, " ,")

			if strings.ToLower(where[:5]) == "where" {
				return nil, errors.New(`sqlez.Update: Params.Where starts with WHERE`)
			}
			options += " WHERE " + where
		}

		if p.OrderBy != "" {
			order := strings.Trim(p.OrderBy, " ,")

			if strings.ToLower(order[:5]) == "order" {
				return nil, errors.New(`sqlez.Update: Params.OrderBy starts with ORDER`)
			}
			options += " ORDER BY " + order
		}

		if p.Limit > 0 {
			options += " LIMIT " + strconv.Itoa(p.Limit)
		}
	}

	labels, interfaces, err := s.scanStruct(v, false, p.SkipEmpty, true)
	if err != nil {
		return nil, err
	}

	query := "UPDATE " + table + " SET "
	for i, v := range labels {
		query += v + " = ?"
		if i < len(labels)-1 {
			query += ", "
		}
	}
	query += options

	s.LastQuery = query
	interfaces = append(interfaces, params...)
	return s.DB.Exec(query, interfaces...)
}
