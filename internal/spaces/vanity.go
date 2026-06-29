package spaces

import (
	"errors"
	"regexp"

	"github.com/jackc/pgx/v5/pgconn"
)

var vanityRe = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{1,30}[a-z0-9])$`)

func validVanity(slug string) bool {
	if len(slug) < 3 || len(slug) > 32 {
		return false
	}
	if !vanityRe.MatchString(slug) {
		return false
	}
	return !containsDoubleHyphen(slug)
}

func containsDoubleHyphen(s string) bool {
	for i := 1; i < len(s); i++ {
		if s[i] == '-' && s[i-1] == '-' {
			return true
		}
	}
	return false
}

var reservedVanity = map[string]bool{
	"admin": true, "api": true, "app": true, "auth": true, "login": true,
	"logout": true, "register": true, "join": true, "settings": true,
	"support": true, "help": true, "about": true, "terms": true, "privacy": true,
	"krovara": true, "official": true, "staff": true, "system": true, "root": true,
	"www": true, "mail": true, "status": true, "blog": true, "docs": true,
	"discover": true, "explore": true, "home": true, "me": true, "billing": true,
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
