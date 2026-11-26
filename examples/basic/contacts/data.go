package contacts

type Contact struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

func getContacts() []Contact {
	return []Contact{
		{ID: "123", Name: "John Doe", Email: "johndoe@gmail.com"},
		{ID: "321", Name: "Alice Smith", Email: "alice.smith@outlook.com"},
		{ID: "246", Name: "Edward Silva", Email: "edward.silva@zipmail.com"},
	}
}
