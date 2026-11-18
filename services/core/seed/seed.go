// Package seed embeds the development seed data.
package seed

import _ "embed"

// Dev is the SQL that fills a development database with fake users, projects, jobs in every
// state, and credit history. It is safe to run repeatedly. Never run it in production.
//
//go:embed dev.sql
var Dev string
