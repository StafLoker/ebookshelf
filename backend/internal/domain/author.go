package domain

import "strings"

type Author struct {
	ID         int64  `db:"id"`
	FirstName  string `db:"first_name"`
	MiddleName string `db:"middle_name"`
	LastName   string `db:"last_name"`
}

func (a Author) Name() string {
	return strings.Join(strings.Fields(a.FirstName+" "+a.MiddleName+" "+a.LastName), " ")
}

// "Last, First Middle"
func (a Author) SortName() string {
	name := a.LastName
	if given := strings.Join(strings.Fields(a.FirstName+" "+a.MiddleName), " "); given != "" {
		name += ", " + given
	}
	return name
}
