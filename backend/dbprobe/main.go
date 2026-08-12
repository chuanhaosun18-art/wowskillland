package main

import (
	"database/sql"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

func main() {
	db, err := sql.Open("sqlite", `D:\skillhub-data\skillhub.db?_pragma=busy_timeout(5000)`)
	if err != nil {
		fmt.Println("open err:", err)
		return
	}
	defer db.Close()

	start := time.Now()
	var n int
	err = db.QueryRow("SELECT COUNT(*) FROM users").Scan(&n)
	fmt.Printf("SELECT users count: n=%d err=%v took=%v\n", n, err, time.Since(start))

	start = time.Now()
	res, err := db.Exec(`INSERT INTO users (username, email, password_hash) VALUES ('probe_test', NULL, 'x')`)
	if err != nil {
		fmt.Println("INSERT err:", err, "took=", time.Since(start))
		return
	}
	id, _ := res.LastInsertId()
	fmt.Println("INSERT ok id=", id, "took=", time.Since(start))
}
