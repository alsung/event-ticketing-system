module github.com/alsung/event-ticketing-system/services/api-gateway

go 1.24.1

replace github.com/alsung/event-ticketing-system/services/pkg => ../pkg

require (
	github.com/alsung/event-ticketing-system/services/pkg v0.0.0-00010101000000-000000000000
	github.com/golang-jwt/jwt/v5 v5.2.1
	github.com/joho/godotenv v1.5.1
)
