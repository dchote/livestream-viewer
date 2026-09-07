module github.com/dchote/livestream-viewer

go 1.25.0

ignore (
	frontend
	addon
)

require (
	github.com/go-chi/chi/v5 v5.2.2
	github.com/golang-jwt/jwt/v5 v5.2.2
	github.com/pelletier/go-toml/v2 v2.2.4
	golang.org/x/crypto v0.40.0
	gorm.io/driver/sqlite v1.6.0
	gorm.io/gorm v1.30.0
)

require (
	github.com/jinzhu/inflection v1.0.0 // indirect
	github.com/jinzhu/now v1.1.5 // indirect
	github.com/mattn/go-sqlite3 v1.14.28 // indirect
	golang.org/x/text v0.27.0 // indirect
)
