package sqlite

import "errors"

// ErrSchemaNewer classifies a database whose schema a newer release wrote.
var ErrSchemaNewer = errors.New("database schema is newer than this binary supports")

// ErrDatabaseConflict classifies a project whose legacy and current database
// paths both hold a database, so neither can be chosen safely.
var ErrDatabaseConflict = errors.New("legacy and project databases both exist")

// classified keeps an actionable message intact while letting errors.Is match
// the class it belongs to.
type classified struct {
	err   error
	class error
}

func (e classified) Error() string { return e.err.Error() }

func (e classified) Unwrap() []error { return []error{e.err, e.class} }
