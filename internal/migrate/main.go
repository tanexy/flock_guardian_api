//go:build ignore

package main

import (
	"fmt"
	"os"

	"flock_guardian_api/internal/brooders"
	"flock_guardian_api/internal/users"

	"ariga.io/atlas-provider-gorm/gormschema"
)

func main() {
	stmts, err := gormschema.New("sqlite").Load(
		&users.User{},
		&brooders.Brooder{},
		&brooders.HistoricalSensorData{},
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to load schema: %v\n", err)
		os.Exit(1)
	}
	fmt.Print(stmts)
}
