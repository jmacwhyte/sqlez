package sqlez

import (
	"database/sql"
	"fmt"
	"reflect"
	"time"
)

var (
	MySQL  = MySQLDriver{}
	Sqlite = SqliteDriver{}
)

type GoType int

const (
	GoString GoType = iota
	GoInt
	GoFloat
	GoBool
	GoTime
	GoStruct
)

type DBDriver interface {
	GetDataType(reflect.Type) string
	GetName() string
	CreateTable(data *DBObjectMetadata) string
	InsertIgnore(data *DBObjectAddOn, ignore bool) (string, []interface{})
	Update(data *DBObjectAddOn) (string, []interface{})
	Select(data *DBObjectAddOn, params Params) string
	Delete(data *DBObjectAddOn) (string, interface{})
}

// Params contains the parameters for the query
type Params struct {
	Where   string
	OrderBy string
	Limit   int
}

// DB represents the sqlez database wrapper
type DB struct {
	DB        *sql.DB
	driver    DBDriver
	objects   map[reflect.Type]DBObjectMetadata
	LastQuery string
	dbTag     string
	timeType  reflect.Type
	// mutex     sync.Mutex
}

type DBObjectMetadata struct {
	table          string
	pkey           int
	fkey           int
	created        int
	updated        int
	refreshOrderBy string
	cols           []DBColumn
	validated      bool
}

type DBColumn struct {
	label        string
	field        int
	sqlType      string
	goType       GoType
	primary      bool
	foreignTable string
	foreignKey   string
	unique       bool
	autoinc      bool
	created      bool
	updated      bool
	def          string
	json         bool
	colProp      string
}

// Open initiates the connection to the database. It takes the same parameters as the database/sql package, and returns a sqlEZ DB struct. The contained *sql.DB is exported so you can make use of it directly.
func Open(driver DBDriver, dataSourceName string) (d *DB, err error) {
	ez := DB{
		driver:   driver,
		objects:  make(map[reflect.Type]DBObjectMetadata),
		dbTag:    "db",
		timeType: reflect.TypeOf(time.Time{}),
	}

	ez.DB, err = sql.Open(driver.GetName(), dataSourceName)
	if err != nil {
		return
	}

	d = &ez
	return
}

// Close closes the connection to the database
func (d DB) Close() error {
	return d.DB.Close()
}

// SetDBTag changes the struct field tag to look for when searching for database column names.
func (d *DB) SetDBTag(tag string) {
	d.dbTag = tag
}

// Attach connects an object to the database and makes it ready to be accessed
func (d *DB) Attach(ptr DBObject) error {
	ptr.Init(ptr, d)
	return nil
}

func (d *DB) GetMany(params Params, ptr interface{}) error {
	// make sure it's a Pointer
	if reflect.ValueOf(ptr).Kind() != reflect.Ptr {
		return fmt.Errorf("expected pointer, got %s", reflect.ValueOf(ptr).Kind().String())
	}

	// make sure the pointer points to a slice
	if k := reflect.ValueOf(ptr).Elem().Kind(); k != reflect.Slice {
		return fmt.Errorf("expected slice, got %s", k.String())
	}

	var query string
	// make a new one for metadata and get the query
	if obj, ok := reflect.New(reflect.TypeOf(ptr).Elem().Elem()).Interface().(DBObject); !ok {
		return fmt.Errorf("expected pointer to slice of DBObjects, got pointer to slice of %s", reflect.TypeOf(ptr).Elem().Elem().Kind().String())
	} else {
		obj.Init(obj, d)
		query = d.driver.Select(obj.GetAddOn(), params)
	}
	d.LastQuery = query

	rows, err := d.DB.Query(query)
	if err != nil {
		return err
	}
	defer rows.Close()

	slc := reflect.MakeSlice(reflect.TypeOf(ptr).Elem(), 0, 0)

	count := 0
	for rows.Next() {
		no := reflect.New(reflect.TypeOf(ptr).Elem().Elem()).Interface().(DBObject)
		no.Init(no, d)

		if n, e := no.GetAddOn().populate(rows); e != nil {
			break
		} else if n == 0 {
			continue
		}

		slc = reflect.Append(slc, reflect.ValueOf(no).Elem())
		count++
	}

	reflect.ValueOf(ptr).Elem().Set(slc)
	return err
}
