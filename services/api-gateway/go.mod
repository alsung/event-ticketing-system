module github.com/alsung/event-ticketing-system/services/api-gateway

go 1.25.0

replace github.com/alsung/event-ticketing-system/services/pkg => ../pkg

require (
	github.com/alsung/event-ticketing-system/services/pkg v0.0.0-00010101000000-000000000000
	github.com/joho/godotenv v1.5.1
)

require (
	github.com/golang-jwt/jwt/v5 v5.3.1 // indirect
	github.com/google/uuid v1.6.0 // indirect
)
