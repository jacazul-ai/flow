package migration

import "errors"

// ErrDatabaseExists classifies a library migration refused because the
// project already has a database; nothing is written.
var ErrDatabaseExists = errors.New("project database already exists")

// ErrRecordErrors classifies a migration that finished with ERROR issues.
var ErrRecordErrors = errors.New("migration completed with record errors")
