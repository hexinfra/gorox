dsn := &mysql.DSN{
	Host: "127.0.0.1",
	Port: 3306,
	User: "root",
}
db, err := mysql.Dial(dsn, time.Second)
if err != nil {
	panic(err)
}
defer db.Close()
