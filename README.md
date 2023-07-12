Notes: Nothing will be null
Private key must exist, will be int and autoincrement
Time will be stored as int (unix)

TODO: maps to json

# sqlEZ - A simpler way to use SQL in Go
### Why this exists
I had written multiple functions to SELECT, INSERT, and UPDATE data in my database, but with 30+ columns of data being referenced each time, making small changes turned tedious. If I wanted to change or add a column in/to my database, I now had to update the SQL code in each function, update the structs I was adding the data to, and make sure the data from the column made it into the correct part of the struct. There must be a better way!

I decided it would be much better if I could simply define a struct that represents the data in my database and let a library do all the other work. I took a look at [sqlx](https://github.com/jmoiron/sqlx) which adds many improvements to the standard sql library, but I realized it wouldn't help much with my long lists of column names. I decided to write this library, sqleEZ, to make it even easier. Now when I change or add a column, all I need to do is change the struct(s) that reference that data and I'm ready to use it in my program!

### Compared to sqlx
Here are some differences between sqlEZ and sqlx. If any of these are wrong, please open an issue and let me know!

* It's possible to use sqlEZ without writing any SQL code (although you will need to write some if you want to use WHERE statements). With sqlx you still write raw SQL code, which makes it much more flexible but also doesn't help cut down on code (which sqlEZ was specifically designed to do).
* SqlEZ lets you UPDATE by simple passing in the updated struct--no need for lengthy "SET x=y" sql code.
* SqlEZ handles NULL strings behind the scenes, so you don't need to use the sql.NullString type in your structs. If it's NULL in your database, it will be unset in your struct. Sqlx requires you to use the NullString type in your struct if your database allows NULL values.
* SqlEZ can store non-sql datatypes (such as entire structs and maps) as JSON string values, and automatically does the conversion to-from JSON for you.

## Getting started
To use sqlEZ, first define a struct to represent the data you want to get from your database. You can then use that struct as a vehicle for moving data in and out of the database.

For example, let's say you had a database of Sumo wrestlers, named `wrestlers`, that looked like the following:
```
+--------------+--------------+
| Field        | Type         |
+--------------+--------------+
| name         | varchar(32)  |
| age          | int(11)      |
| callsign     | varchar(32)  |
| superMove    | varchar(128) |
| weakness     | varchar(128) |
+--------------+--------------+
```
You want to get the vital stats on your wrestlers, so you make a struct like so:
```
type WrestlerBio struct {
	Name     string
	Nickname string
	Age      int
}
```
As you can see, our database has a column titled "callsign", but in Go we're calling it "Nickname". To link fields in our struct to the columns in our database, we need to give each field a "db" tag that matches the name of the column. Any field that doesn't have a "db" tag will be ignored, so you can add some go-only metadata if you want:
```
type WrestlerBio struct {
	Name         string `db:"name"`
	Nickname     string `db:"callsign"`
	Age          int    `db:"age"`
	NumberOfWins int
}
```

### SELECTing data
Once, you've initialized sqlEZ (in the same way as you would call sql.Open()), you can get a list of all your Sumo wrestlers from the database like so:
```
db := sqlez.Open("mysql", dataSource)
res, err := db.SelectFrom("wrestlers", WrestlerBio{})
```
`res` will now contain a slice of interfaces (`[]interface{}`) which you can cast back into the `WrestlerBio` type:
```
for _, i := range res {
	wrestler := i.(WrestlerBio)
	fmt.Printf("Name: %s, Nickname: %s\n", wrestler.Name, wrestler.Nickname)
}
```
The contents of embedded structs will be checked even if the embedded struct itself doesn't have a "db" tag. If you want to prevent this, give the struct a tag of "dbskip" (and any value) and we'll ignore it.


### WHERE, ORDER BY, LIMIT
If you want to be more selective about the kind of data you get back, you can also pass a sqlez.Params struct with extra options:
```
res, err := db.SelectFrom("wrestlers", WrestlerBio{}, sqlez.Params{
	Where: `weakness = "chilidogs" AND superMove != ?`,
	OrderBy: "name ASC",
	Limit: 5,
}, moveToIgnore)
```
These extra parameters can be included in any order/combination. Note they take the same syntax as SQL `WHERE`, `ORDER BY`, and `LIMIT` commands but omit those keywords.

As you can see, if you want to use variables in your `Where` statement, you can include wildcard `?`s just like with SQL, and pass those variables along at the end. In this case, the contents of the `moveToIgnore` variable replace the `?` in `superMove != ?`.

### Complicated Go structs
SqlEZ works recursively, so you can use embedded structs to arrange your data however you want, as long as the data all comes from the same table. You could do the following, for example:
```
type Awesomeness struct {
	Cons string `db:"weakness"`
	Pros string `db:"superMove"`
}
type WrestlerBio struct {
	Name         string `db:"name"`
	Nickname     string `db:"callsign"`
	Age          int    `db:"age"`
	NumberOfWins int
	Awesomeness  Awesomeness
}

res, err := db.SelectFrom("wrestlers", WrestlerBio{})
firstWreslter := res[0].(WreslterBio)
```
Now you can manipulate `firstWrestler.Awesomeness` independently to make your determination about how awesome each wrestler is.

### INSERTing data
Inserting new data to the database follows the same concept. Create your struct and populate it:
```
frank := WrestlerBio{
	Name: "Frank",
	Nickname: "Shinagawa Slammer",
	Age: 29,
	Awesomeness: Awesomeness{
		Cons: "Wet Willies",
		Pros: "Roundhouse Slap"
	}
}
```
And simply pass it to sqlEZ using `InsertInto`:
```
res, err := db.InsertInto("wrestlers", frank, Params{})
```
You can also pass a `Params` struct with some bool flags:

SkipEmpty - See below ([Ignoring unset values](#ignoring-unset-values))
OrIgnore - Will "INSERT OR IGNORE INTO" if you want to silently ignore duplicate keys

### UPDAT[E]ing data
Updating data is very similar to inserting, but it usually helps to add a `Where` statement to make sure you only update what you want to:
```
res, err := db.Update("wrestlers", updatedWrestler, sqlez.Params{Where: `name = "Frank"`})
```

### Ignoring unset values
The only downside to this system is that a partially-set struct is full of many empty values. If you wanted to update only one item, say, change Frank's nickame to "Shinagawa Slender", you would need to populate the rest of the struct with the existing values so Frank's name, age, and other info isn't overwritten by the empty struct's values.

Luckily we've thought of this! By setting `sqlez.Params.SkipEmpty` to true when calling `Update`, you can tell sqlEZ to ignore unset values. That means you can do the following, and only Frank's nickname will be changed:
```
res, err := db.Update("wrestlers", WrestlerBio{Nickname: "Shinagawa Slender"}, sqlez.Params{
	Where: `name = "Frank"`,
	SkipEmpty: true})
```

`InsertInto` also has this feature, which will allow your SQL database to populate columns with default values if you don't want to set them.

### Storing Go types that don't have a database counterpart
Sometimes you may want to store a type of data that exists in Go but doesn't have a related database type--for example, a map, slice, or populated struct. SqlEZ makes this possible by converting those datatypes to JSON and storing them as a string in your database. Simply give an item in your struct a field label of "dbjson" (followed by the column name) and sqlEZ will automatically convert your data type to and from JSON when moving data into/out of the database. Maps will automatically be converted to JSON strings, even if you don't specify the "dbjson" tag.
```
type WrestlerBio struct {
	Name           string      `db:"name"`
	Nickname       string      `db:"callsign"`
	Age            int         `db:"age"`
	PointsPerRound map[int]int `dbjson:"points"`
}
```
With a struct like the above, the PointsPerRound map will be saved in a column titled "points" in your database. Make sure the type of the database column is large enough ("text" is probably a better choice than "varchar").

### Changing struct field tags
If you don't want to use the "db", "dbjson", and "dbskip" tags and would rather call them something else (to avoid conflicts, for example), you can call sqlez.SetDBTag(string), sqlez.SetJSONTag(string), and sqlez.SetSkipTag(string) right after creating the sqlez.DB object to change the text it searches for.

### Troubleshooting
If you want to examine the SQL command that sqlEZ has generated for you, the `sqlez.DB` object that `sqlez.Open()` returns includes a `LastQuery` variable which will contain the string that was last generated. This usually gets updated even if your SQL database returns an error, so printing out this string can be a quick way to see why things aren't working.

The following code with a typo in it...
```
res, err := db.SelectFrom("operators", WrestlerBio{}, sqlez.Params{Where: `"name = "Frank"`})

if err != nil {
	fmt.Println(err)
	fmt.Printf("The query was: %s\n", db.LastQuery)
}

```

Returns:
```
Error 1064: You have an error in your SQL syntax; check the manual that corresponds to your MySQL server version for the right syntax to use near 'Frank"' at line 1
The query was: SELECT displayName, name, type, version, phone, email, url FROM operators WHERE "name = "Frank"
```