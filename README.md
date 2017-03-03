# sqlEZ
By James MacWhyte (@jmacwhyte)
#### An extremely simply SQL database interface for Go
Are you tired of writing so much code just to pull data from a SQL database into Go? Does your data structure in Go closely mirror the structure of your database? Do you often pull out data, modify it, and then want to simply slap it back in? If you answered yes to any of these questions, this package is for you!

#### The concept
To use sqlEZ, first define a struct to represent the data you want to get from your database. You can then use that struct as a vehicle for moving data in and out of the database. So easy!

#### SELECTing data
For example, let's say you had a database of Sumo wrestlers, named `wreslters`, that looked like the following:
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
As you can see, our database has a column titled "callsign", but in Go we're calling it "Nickname". No problem! To link fields in our struct to the columns in our database, we need to give each field a "db" tag that matches the name of the column. Any field that doesn't have a "db" tag will be ignored, so you can add some go-only metadata if you want:
```
type WrestlerBio struct {
	Name         string `db:"name"`
	Nickname     string `db:"callsign"`
	Age          int    `db:"age"`
	NumberOfWins int
}
```
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

#### WHERE, ORDER BY, LIMIT
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

#### Complicated Go structs
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

#### INSERTing data
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
res, err := db.InsertInto("wrestlers", frank, false)
```
You may be wondering about the `false` being passed into the `InsertInto` method. More on that below ([Ignoring unset values](#ignoring-unset-values)).

#### UPDAT[E]ing data
Updating data is very similar to inserting, but it usually helps to add a `Where` statement to make sure you only update what you want to:
```
res, err := db.Update("wrestlers", updatedWrestler, sqlez.Params{Where: `name = "Frank"`})
```

#### Ignoring unset values
The only downside to this system is that a partially-set struct is full of many empty values. If you wanted to update only one item, say, change Frank's nickame to "Shinagawa Slender", you would need to populate the rest of the struct with the existing values so Frank's name, age, and other info isn't overwritten by the empty struct's values.

Luckily we've thought of this! By setting `sqlez.Params.SkipEmpty` to true when calling `Update`, you can tell sqlEZ to ignore unset values. That means you can do the following, and only Frank's nickname will be changed:
```
res, err := db.Update("wrestlers", WrestlerBio{Nickname: "Shinagawa Slender"}, sqlez.Params{Where: `name = "Frank"`, SkipEmpty: true})
```

`InsertInto` also has this feature, which will allow your SQL database to populate columns with default values if you don't want to set them. Enable it by passing `true` to `InsertInto`'s third parameter (`skipEmpty`).

#### Troubleshooting
If you want to examine the SQL command that sqlEZ has generated for you, the `ezsql.DB` object that `ezsql.Open()` returns includes a `LastQuery` variable which will contain the string that was last generated. This usually gets updated even if your SQL database returns an error, so printing out this string can be a quick way to see why things aren't working.

The following code with a typo in it...
```
res, err := db.SelectFrom("operators", WrestlerBio{}, sqlez.Params{Where: `name = "Frank"`})

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