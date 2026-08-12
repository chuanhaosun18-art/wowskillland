package main

import (
	"database/sql"
	"flag"
	"fmt"
	"log"
	"path/filepath"

	_ "modernc.org/sqlite"
)

func main() {
	skillID := flag.Int64("skill", 0, "skill id to reset")
	flag.Parse()
	if *skillID == 0 {
		log.Fatal("need -skill <id>")
	}
	base := `D:\skillhub-data`
	db, err := sql.Open("sqlite", filepath.Join(base, "skillhub.db")+"?_pragma=busy_timeout(10000)")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	// 查当前状态
	var name, status string
	if err := db.QueryRow(`SELECT name, status FROM skills WHERE id=?`, *skillID).Scan(&name, &status); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("skill=%d name=%q before: status=%q\n", *skillID, name, status)

	res, err := db.Exec(`UPDATE skills SET status='gated' WHERE id=?`, *skillID)
	if err != nil {
		log.Fatal(err)
	}
	n, _ := res.RowsAffected()
	fmt.Printf("updated rows=%d -> status=gated\n", n)
}
