package main

import (
	"database/sql"
	"fmt"
	"log"

	_ "github.com/mattn/go-sqlite3"
)

type Database struct {
	DB *sql.DB
}

func (d *Database) Open(filename string) {
	var err error
	d.DB, err = sql.Open("sqlite3", fmt.Sprintf("file:%s?cache=shared&mode=rwc", filename))
	if err != nil {
		log.Fatalln("[Database] Error:", err)
	}
	d.DB.SetMaxOpenConns(1)
}

func (d *Database) Close() {
	d.DB.Close()
}
