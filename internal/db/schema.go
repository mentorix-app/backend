package db

const Schema = "mentorix"

func Table(name string) string {
	return Schema + "." + name
}
