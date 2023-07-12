package sqlez

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"time"
)

type DBObject interface {
	Init(interface{}, *DB) error
	GetAddOn() *DBObjectAddOn
	CreateTable() error
	GetExisting(params Params) error
	SaveNew(ignore bool) (int, error)
	SaveExisting() (int, error)
	Refresh() error
	Delete() (int, error)
}

type DBObjectAddOn struct {
	db     *DB
	parent interface{}
	meta   *DBObjectMetadata
}

func (d *DBObjectAddOn) Init(parent interface{}, db *DB) error {
	val := reflect.ValueOf(parent)
	if val.Kind() != reflect.Ptr {
		return fmt.Errorf("expected pointer, got %s", val.Kind())
	} else if val.Elem().Kind() != reflect.Struct {
		return fmt.Errorf("expected pointer to struct, got pointer to %s", val.Elem().Kind())
	}
	val = val.Elem()

	d.parent = parent
	d.db = db

	typ := reflect.TypeOf(val)

	// Check if we already have the metadata for this type
	if t, e := db.objects[typ]; e {
		d.meta = &t
	} else {
		md := DBObjectMetadata{
			pkey:    -1,
			fkey:    -1,
			created: -1,
			updated: -1,
		}
		for i := 0; i < val.NumField(); i++ {
			tag := val.Type().Field(i).Tag.Get("db")
			if tag == "" {
				continue
			}

			split := strings.Split(tag, ",")
			c := DBColumn{label: split[0]}

			for _, v := range split[1:] {
				vv := strings.Split(v, ":")
				if len(vv) > 1 {
					switch vv[0] {
					case "default":
						c.def = vv[1]
					case "table":
						if md.table == "" {
							md.table = vv[1]
						}
					case "type":
						c.sqlType = vv[1]
					case "refresh":
						md.refreshOrderBy = vv[1]
					}

				} else {

					switch {
					case v == "primary" && md.pkey < 0:
						c.primary = true
						md.pkey = i
					case v == "foreign" && md.fkey < 0:
						// must be a pointer to a struct
						if val.Field(i).Kind() != reflect.Ptr {
							return fmt.Errorf("foreign key must be a pointer to a struct")
						}
						// get the type of the struct
						fkType := val.Field(i).Type()
						// see if we have that type in our objects map
						if t, e := db.objects[fkType]; !e {
							return fmt.Errorf("foreign table not yet defined")
						} else {
							c.foreignTable = t.table
							c.foreignKey = t.cols[t.pkey].label
							md.fkey = i
						}
					case v == "unique":
						c.unique = true
					case v == "json":
						c.json = true
					case v == "autoinc":
						c.autoinc = true
					case v == "created":
						c.created = true
						md.created = i
					case v == "updated":
						c.updated = true
						md.updated = i
					default:
						c.colProp = v
					}
				}

				if c.sqlType == "" {
					c.sqlType = d.db.driver.GetDataType(val.Field(i).Type())
				}

				// check to see if the field is a time struct
				if md.created >= 0 && val.Field(md.created).Type() != reflect.TypeOf(time.Time{}) {
					return fmt.Errorf("created field must be a time struct")
				}

				if md.updated >= 0 && val.Field(md.updated).Type() != reflect.TypeOf(time.Time{}) {
					return fmt.Errorf("updated field must be a time struct")
				}

				// find go type

				switch val.Field(i).Kind() {
				case reflect.Struct:
					if val.Field(i).Type() == d.db.timeType {
						c.goType = GoTime
					} else {
						c.goType = GoStruct
					}
				case reflect.Float32, reflect.Float64:
					c.goType = GoFloat
				case reflect.Int:
					c.goType = GoInt
				case reflect.String:
					c.goType = GoString
				case reflect.Bool:
					c.goType = GoBool
				default:
					return fmt.Errorf("unsupported type %s", typ.Kind())
				}

			}
			c.field = i
			md.cols = append(md.cols, c)
		}

		// add to db objects
		db.objects[typ] = md
		d.meta = &md
	}

	return nil
}

func validateMetadata(md *DBObjectMetadata) error {
	if md == nil {
		return fmt.Errorf("database object not initialized")
	}

	if md.validated {
		return nil
	}

	if md.table == "" {
		return fmt.Errorf("table name not specified")
	}

	if md.pkey < 0 {
		return fmt.Errorf("no primary key specified")
	}

	if md.fkey == md.pkey {
		return fmt.Errorf("foreign key cannot be the same as primary key")
	}

	md.validated = true
	return nil
}
func (d *DBObjectAddOn) GetAddOn() *DBObjectAddOn {
	return d
}

// CreateTable creates a table in the database according to the DBObjectMetadata
func (d *DBObjectAddOn) CreateTable() error {
	if err := validateMetadata(d.meta); err != nil {
		return err
	}

	query := d.db.driver.CreateTable(d.meta)
	d.db.LastQuery = query

	_, err := d.db.DB.Exec(query)
	return err
}

func (d *DBObjectAddOn) GetExisting(params Params) error {
	if err := validateMetadata(d.meta); err != nil {
		return err
	}
	params.Limit = 1

	query := d.db.driver.Select(d, params)
	d.db.LastQuery = query

	rows, err := d.db.DB.Query(query)
	if err != nil {
		return err
	}
	defer rows.Close()

	if !rows.Next() {
		return fmt.Errorf("no rows returned matching criteria")
	}

	_, err = d.populate(rows)
	return err
}

func (d *DBObjectAddOn) SaveNew(ignore bool) (n int, err error) {
	if err = validateMetadata(d.meta); err != nil {
		return
	}

	query, vals := d.db.driver.InsertIgnore(d, ignore)
	d.db.LastQuery = query

	var res sql.Result
	res, err = d.db.DB.Exec(query, vals...)
	if err != nil {
		return
	}
	if nr, e := res.RowsAffected(); e == nil {
		n = int(nr)
	}
	return
}

func (d *DBObjectAddOn) SaveExisting() (n int, err error) {
	if err = validateMetadata(d.meta); err != nil {
		return
	}

	query, vals := d.db.driver.Update(d)
	d.db.LastQuery = query

	var res sql.Result
	res, err = d.db.DB.Exec(query, vals...)
	if err != nil {
		return
	}
	if nr, e := res.RowsAffected(); e == nil {
		n = int(nr)
	}
	return
}

func (d *DBObjectAddOn) Refresh() error {

	if err := validateMetadata(d.meta); err != nil {
		return err
	}

	p := Params{
		Where:   fmt.Sprintf("%s = ?", d.meta.cols[d.meta.pkey].label),
		Limit:   1,
		OrderBy: d.meta.refreshOrderBy,
	}

	query := d.db.driver.Select(d, p)
	d.db.LastQuery = query

	pk := reflect.ValueOf(d.parent).Elem().Field(d.meta.pkey).Interface()
	rows, err := d.db.DB.Query(query, pk)
	if err != nil {
		return err
	}
	defer rows.Close()

	if !rows.Next() {
		return fmt.Errorf("no rows returned")
	}

	if _, e := d.populate(rows); e != nil {
		return e
	}
	return nil
}

// populate is a way to get entries from the database and return populated structs
func (d *DBObjectAddOn) populate(rows *sql.Rows) (n int, err error) {

	dest := reflect.ValueOf(d.parent).Elem()
	values := make([]interface{}, len(d.meta.cols))
	pointers := make([]interface{}, len(values))

	for i := range values {
		pointers[i] = &values[i]
	}

	if err = rows.Scan(pointers...); err != nil {
		fmt.Printf("error scanning row: %s\n", err)
		return
	}

	for i, c := range d.meta.cols {
		if c.json {
			// unmarshal
			if err = json.Unmarshal(values[i].([]byte), dest.Field(i).Addr().Interface()); err != nil {
				err = fmt.Errorf("error unmarshalling column flagged as json: %s", err)
				return
			}
			continue
		}

		switch c.goType {
		case GoInt:
			dest.Field(c.field).SetInt(values[i].(int64))
		case GoFloat:
			dest.Field(c.field).SetFloat(values[i].(float64))
		case GoString:
			dest.Field(c.field).SetString(values[i].(string))
		case GoBool:
			dest.Field(c.field).SetBool(values[i].(bool))
		case GoTime:
			var v int64
			switch values[i].(type) {
			case string:
				if v, err = strconv.ParseInt(values[i].(string), 10, 64); err != nil {
					err = fmt.Errorf("error parsing time: %s", err)
					return
				}
			case int64:
				v = values[i].(int64)
			default:
				err = fmt.Errorf("invalid time type")
				return
			}

			dest.Field(c.field).Set(reflect.ValueOf(time.Unix(v, 0)))
		case GoStruct:
			err = fmt.Errorf("structs must be marked as json or ignored")
		}
	}

	if err == nil {
		n = 1
	}
	return
}

func (d *DBObjectAddOn) Delete() (n int, err error) {
	if err = validateMetadata(d.meta); err != nil {
		return
	}

	query, val := d.db.driver.Delete(d)
	d.db.LastQuery = query

	var res sql.Result
	res, err = d.db.DB.Exec(query, val)
	if err != nil {
		return
	}
	if nr, e := res.RowsAffected(); e == nil {
		n = int(nr)
	}
	return
}
