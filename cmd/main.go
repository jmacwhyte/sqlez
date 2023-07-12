package main

import (
	"fmt"
	"time"

	"github.com/jmacwhyte/sqlez"
)

type Test struct {
	ID   int    `db:"id,primary,table:tests"`
	Name string `db:"name"`
}

// create table
type User struct {
	ID      int       `db:"id,primary,table:users"`
	Name    string    `db:"name"`
	Created time.Time `db:"created,created"`
	Updated time.Time `db:"updated,updated"`
	sqlez.DBObjectAddOn
}

func main() {

	// t := Test{}

	// var in []interface{}

	// v := reflect.ValueOf(t)
	// for i := 0; i < v.NumField(); i++ {
	// 	in = append(in, v.Field(i).P)
	// }

	// fmt.Printf("%#v\n", in)
	// fmt.Printf("%#v\n", t)

	// t.Name = "test"

	// fmt.Printf("%#v\n", in)
	// fmt.Printf("%#v\n", t)

	// return

	// create sqlez db
	db, err := sqlez.Open(sqlez.Sqlite, "test.db")
	if err != nil {
		panic(err)
	}

	u := User{}
	db.Attach(&u)

	u.GetExisting(sqlez.Params{Where: "id = 3"})

	fmt.Printf("%d - %s - %s - %s", u.ID, u.Name, u.Created, u.Updated)

	// return

	// user := make([]User, 0)

	// err = db.GetMany(sqlez.Params{}, &user)
	// fmt.Println(db.LastQuery)
	// if err != nil {
	// 	panic(err)
	// }

	// for _, u := range user {
	// 	fmt.Printf("%d - %s\n", u.ID, u.Name)
	// }

	// if e := user.CreateTable(); e != nil {
	// 	panic(e)
	// }
	// fmt.Println(db.LastQuery)

	// if n, e := user.SaveNew(false); e != nil {
	// 	panic(e)
	// } else {
	// 	fmt.Printf("inserted %d rows\n", n)
	// }

	// n, e := user.Delete()
	// if e != nil {
	// 	panic(e)
	// } else {
	// 	fmt.Printf("deleted %d rows\n", n)
	// }

}
