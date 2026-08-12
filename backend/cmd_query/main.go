package main

import (
	"database/sql"
	"fmt"
	"log"
	"path/filepath"

	_ "modernc.org/sqlite"
)

func main() {
	db, err := sql.Open("sqlite", filepath.Join(`D:\skillhub-data`, "skillhub.db")+"?_pragma=busy_timeout(10000)")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	fmt.Println("=== 空 intent 的 corpus ===")
	rows, err := db.Query(`SELECT id, utterance, source FROM description_corpus WHERE task_intent IS NULL OR task_intent = '' ORDER BY id`)
	if err != nil {
		log.Fatal(err)
	}
	for rows.Next() {
		var id int64
		var u, src string
		rows.Scan(&id, &u, &src)
		fmt.Printf("  id=%d src=%s u=%q\n", id, src, u)
	}
	rows.Close()

	fmt.Println("=== 保研相关 skill ===")
	rows, err = db.Query(`SELECT s.id, s.name, COALESCE(s.task_intent,''), s.status, v.id FROM skills s
		LEFT JOIN skill_versions v ON v.id = s.current_version_id
		WHERE s.name LIKE '%保研%' OR s.name LIKE '%推免%' ORDER BY s.id`)
	if err != nil {
		log.Fatal(err)
	}
	for rows.Next() {
		var id int64
		var name, intent, status string
		var vid sql.NullInt64
		rows.Scan(&id, &name, &intent, &status, &vid)
		fmt.Printf("  id=%d name=%q intent=%q status=%q ver=%v\n", id, name, intent, status, vid)
	}
	rows.Close()

	fmt.Println("=== skill 34/35 的 skill_evals ===")
	rows, err = db.Query(`SELECT id, skill_id, version_id, eval_type, input FROM skill_evals
		WHERE skill_id IN (34,35) ORDER BY skill_id, eval_type, id`)
	if err != nil {
		log.Fatal(err)
	}
	for rows.Next() {
		var id, sid, vid int64
		var t, in string
		rows.Scan(&id, &sid, &vid, &t, &in)
		fmt.Printf("  skill=%d ver=%d type=%s input=%q\n", sid, vid, t, in)
	}
	rows.Close()

	fmt.Println("=== skill 35 草稿字段 ===")
	var goal, dc, wf, bd, desc, contract string
	err = db.QueryRow(`SELECT goal, done_criteria, workflow, boundary, description, contract FROM skill_versions WHERE id=25`).
		Scan(&goal, &dc, &wf, &bd, &desc, &contract)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("desc=%q\n", desc)
	fmt.Printf("goal=%q\n", goal)
	fmt.Printf("done_criteria=%s\n", dc)
	fmt.Printf("workflow=%s\n", wf)
	fmt.Printf("boundary=%s\n", bd)
	fmt.Printf("contract=%s\n", contract)
}
