package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

func main() {
	base := os.Getenv("SKILLHUB_DATA")
	if base == "" {
		base = `D:\skillhub-data`
	}
	db, err := sql.Open("sqlite", filepath.Join(base, "skillhub.db")+"?_pragma=busy_timeout(10000)")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	rows, err := db.Query(`SELECT s.id, s.name, s.status, s.origin, s.owner_id, s.current_version_id,
		COALESCE(v.proof_type,''), COALESCE(v.source_execution_id,0),
		s.created_at, u.username,
		v.goal, v.done_criteria, v.workflow, v.boundary, v.gotchas,
		(SELECT COUNT(*) FROM decisions d WHERE d.skill_id=s.id) AS ndec
		FROM skills s LEFT JOIN skill_versions v ON v.id = s.current_version_id
		LEFT JOIN users u ON u.id = s.owner_id
		ORDER BY s.id DESC LIMIT 8`)
	if err != nil {
		log.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var id, owner, cvid, srcExec, ndec int64
		var name, status, origin, ptype, goal, dc, wf, bd, gc string
		var createdAt, username sql.NullString
		if err := rows.Scan(&id, &name, &status, &origin, &owner, &cvid, &ptype, &srcExec, &createdAt, &username, &goal, &dc, &wf, &bd, &gc, &ndec); err != nil {
			log.Fatal(err)
		}
		fmt.Printf("skill=%d name=%q status=%q origin=%q owner=%d(%s) ver=%d proof=%q srcExec=%d ndec=%d created=%v\n",
			id, name, status, origin, owner, username.String, cvid, ptype, srcExec, ndec, createdAt.String)
		fmt.Printf("  goal=%q\n", truncate(goal))
		fmt.Printf("  done_criteria=%q\n", truncate(dc))
		fmt.Printf("  workflow=%q\n", truncate(wf))
		fmt.Printf("  boundary=%q\n", truncate(bd))
		fmt.Printf("  gotchas=%q\n", truncate(gc))
		// decisions 明细
		drows, _ := db.Query(`SELECT slot, trigger_signal, judgment FROM decisions WHERE skill_id=? AND invalidated_at IS NULL`, id)
		for drows.Next() {
			var slot, trig, judge string
			drows.Scan(&slot, &trig, &judge)
			fmt.Printf("  dec[%s]: %q -> %q\n", slot, truncate(trig), truncate(judge))
		}
		drows.Close()
		// eval_runs 情况
		var nevals int
		db.QueryRow(`SELECT COUNT(*) FROM eval_runs WHERE version_id=?`, cvid).Scan(&nevals)
		fmt.Printf("  eval_runs=%d\n", nevals)
		fmt.Println("---")
	}
}

func truncate(s string) string {
	if len(s) > 120 {
		return s[:120] + "..."
	}
	return s
}

func jstr(v interface{}) string {
	b, _ := json.Marshal(v)
	return string(b)
}
